package cleaner

import (
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/antisynthesis/unclaude/pkg/watermark"
)

// Cleaner is the orchestrator for repository scrubbing. One instance handles
// one run against one repoDir; instances are not safe for concurrent use.
type Cleaner struct {
	repoDir             string
	dryRun              bool
	interactive         bool
	yesToAll            bool
	skipHistoryPrompt   bool
	purgeRefs           bool
	normalizeTypography bool
	backup              *backup
	log                 *slog.Logger
}

// Options configure New.
type Options struct {
	DryRun      bool
	Interactive bool
	PurgeRefs   bool
	// NormalizeTypography additionally folds visible "smart" punctuation
	// (curly quotes, em/en dashes, ellipsis glyphs) in prose files back to
	// plain ASCII. Off by default because it alters authored, visible content.
	NormalizeTypography bool
	// Backup preserves a copy of every file the cleaner modifies or deletes so
	// the run can be undone with Restore. It has no effect in DryRun mode.
	Backup bool
	Logger *slog.Logger
}

// New creates a Cleaner. If Options.Logger is nil, a no-op logger is used.
func New(repoDir string, opts Options) *Cleaner {
	log := opts.Logger
	if log == nil {
		log = newNopLogger()
	}
	c := &Cleaner{
		repoDir:             repoDir,
		dryRun:              opts.DryRun,
		interactive:         opts.Interactive,
		purgeRefs:           opts.PurgeRefs,
		normalizeTypography: opts.NormalizeTypography,
		log:                 log,
	}
	if opts.Backup && !opts.DryRun {
		c.backup = newBackup(repoDir)
	}
	return c
}

// preserve backs up path (a file or directory) before a destructive change,
// when backups are enabled. It is a no-op in dry-run mode or with --no-backup.
func (c *Cleaner) preserve(path string) error {
	if c.backup == nil {
		return nil
	}
	return c.backup.save(path)
}

// FinalizeBackup writes the backup manifest and returns the backup directory,
// or "" when nothing was backed up. Call it once after all steps complete.
func (c *Cleaner) FinalizeBackup() (string, error) {
	if c.backup == nil {
		return "", nil
	}
	return c.backup.finalize()
}

// IsGitRepo reports whether dir contains a .git directory.
func IsGitRepo(dir string) bool {
	info, err := os.Stat(filepath.Join(dir, ".git"))
	if err != nil {
		return false
	}
	return info.IsDir()
}

func (c *Cleaner) promptForDeletion(relPath string) bool {
	if c.yesToAll {
		return true
	}
	fmt.Printf("\nDelete %s? [y/N/a] (y=yes, n=no, a=yes to all): ", relPath)
	var response string
	fmt.Scanln(&response)
	switch strings.ToLower(strings.TrimSpace(response)) {
	case "a", "all":
		c.yesToAll = true
		return true
	case "y", "yes":
		return true
	default:
		return false
	}
}

// dirAlwaysSkipped reports directories that are never traversed - these are
// either source-control internals (.git) or dependency caches whose contents
// the user did not author. AI-tool directories like .claude and .codex are
// removed wholesale by CleanAIArtifacts and therefore also listed here so
// that subsequent walks don't visit their interior.
func dirAlwaysSkipped(name string) bool {
	switch name {
	case ".git", "node_modules", "vendor", backupDirName,
		".claude", ".codex", ".cursor", ".continue", ".aider":
		return true
	}
	return false
}

// CleanAIArtifacts removes directories and specific files left behind by AI
// coding tools - Claude Code, Codex CLI, Cursor, Continue, Aider, Copilot.
// These artifacts apply everywhere in the tree (no docs/ allowlist), since
// they are tooling, not user-authored documentation.
func (c *Cleaner) CleanAIArtifacts() error {
	dirs := []string{".claude", ".codex", ".cursor", ".continue", ".aider"}
	for _, d := range dirs {
		path := filepath.Join(c.repoDir, d)
		info, err := os.Stat(path)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return err
		}
		if !info.IsDir() {
			continue
		}
		c.log.Info("removing ai tool directory", slog.String("path", d))
		if !c.dryRun {
			if err := c.preserve(path); err != nil {
				return fmt.Errorf("failed to back up %s: %w", d, err)
			}
			if err := os.RemoveAll(path); err != nil {
				return fmt.Errorf("failed to remove %s: %w", d, err)
			}
		}
	}

	// File names removed wherever they appear in the tree (subject to
	// dirAlwaysSkipped). Distinct from CleanMarkdownFiles because these
	// must override the docs/doc/adr allowlist.
	targetFiles := map[string]bool{
		"CLAUDE.md":              true,
		"AGENTS.md":              true,
		".mcp.json":              true,
		".claude.json":           true,
		".claudeignore":          true,
		".cursorrules":           true,
		".cursorignore":          true,
		".aider.conf.yml":        true,
		".aider.input.history":   true,
		".aider.chat.history.md": true,
	}
	specificPaths := []string{
		filepath.Join(".github", "copilot-instructions.md"),
	}

	var matches []string
	err := filepath.WalkDir(c.repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if dirAlwaysSkipped(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if targetFiles[d.Name()] {
			matches = append(matches, path)
		}
		return nil
	})
	if err != nil {
		return err
	}
	for _, p := range specificPaths {
		full := filepath.Join(c.repoDir, p)
		if _, err := os.Stat(full); err == nil {
			matches = append(matches, full)
		}
	}

	for _, m := range matches {
		rel, _ := filepath.Rel(c.repoDir, m)
		c.log.Info("removing ai tool file", slog.String("path", rel))
		if !c.dryRun {
			if err := c.preserve(m); err != nil {
				return fmt.Errorf("failed to back up %s: %w", rel, err)
			}
			if err := os.Remove(m); err != nil {
				return fmt.Errorf("failed to remove %s: %w", rel, err)
			}
		}
	}
	return nil
}

// markdownAllowlistDirs are documentation directories whose generic .md files
// are preserved by CleanMarkdownFiles. AI-tool files (CLAUDE.md, AGENTS.md)
// inside these are NOT preserved - they're removed by CleanAIArtifacts, which
// runs first and uses a separate allowlist that does not include these dirs.
func markdownAllowlistDirs() map[string]bool {
	return map[string]bool{
		"docs": true, "doc": true, "adr": true,
	}
}

// CleanMarkdownFiles removes .md files outside the documentation allowlist,
// preserving the root README.md. Files owned by CleanAIArtifacts (CLAUDE.md,
// AGENTS.md, .aider.chat.history.md, copilot-instructions.md) are not
// reported here to avoid duplicate output in dry-run mode.
func (c *Cleaner) CleanMarkdownFiles() error {
	allow := markdownAllowlistDirs()
	ownedByArtifacts := map[string]bool{
		"claude.md":               true,
		"agents.md":               true,
		".aider.chat.history.md":  true,
		"copilot-instructions.md": true,
	}

	var filesToRemove []string
	err := filepath.WalkDir(c.repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if dirAlwaysSkipped(d.Name()) || allow[d.Name()] {
				return filepath.SkipDir
			}
			return nil
		}
		name := strings.ToLower(d.Name())
		if !strings.HasSuffix(name, ".md") {
			return nil
		}
		if ownedByArtifacts[name] {
			return nil
		}
		if filepath.Dir(path) == c.repoDir && name == "readme.md" {
			return nil
		}
		filesToRemove = append(filesToRemove, path)
		return nil
	})
	if err != nil {
		return err
	}

	removed, skipped := 0, 0
	for _, file := range filesToRemove {
		rel, _ := filepath.Rel(c.repoDir, file)
		if c.interactive && !c.dryRun {
			if !c.promptForDeletion(rel) {
				skipped++
				c.log.Debug("skipped markdown file", slog.String("path", rel))
				continue
			}
		}
		c.log.Info("removing markdown file", slog.String("path", rel))
		if !c.dryRun {
			if err := c.preserve(file); err != nil {
				return fmt.Errorf("failed to back up %s: %w", rel, err)
			}
			if err := os.Remove(file); err != nil {
				return fmt.Errorf("failed to remove %s: %w", rel, err)
			}
		}
		removed++
	}
	if removed > 0 {
		c.log.Info("markdown removal summary", slog.Int("removed", removed))
	}
	if skipped > 0 {
		c.log.Info("markdown skip summary", slog.Int("skipped", skipped))
	}
	return nil
}

var sourceExtensions = []string{
	".go", ".js", ".ts", ".jsx", ".tsx",
	".py", ".java", ".c", ".cpp", ".h", ".hpp",
	".rs", ".rb", ".php", ".cs",
}

// CleanSourceComments strips AI-related comments from source files. Inline
// comments preserve the code they trail; standalone comments cause the line
// to be dropped; multi-line block comments are removed as a unit; runs of
// blank lines produced by removal are collapsed to one.
func (c *Cleaner) CleanSourceComments() error {
	aiPattern := CompiledCommentAIPattern()
	modified := 0

	err := filepath.WalkDir(c.repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if dirAlwaysSkipped(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		ext := strings.ToLower(filepath.Ext(d.Name()))
		if !hasExt(ext, sourceExtensions) {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		newContent, changed := CleanCommentsInSource(content, ext, aiPattern)
		if !changed {
			return nil
		}
		rel, _ := filepath.Rel(c.repoDir, path)
		c.log.Debug("cleaned comments", slog.String("path", rel))
		modified++
		if c.dryRun {
			return nil
		}
		if err := c.preserve(path); err != nil {
			return fmt.Errorf("failed to back up %s: %w", rel, err)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, newContent, info.Mode())
	})
	if err != nil {
		return err
	}
	if modified > 0 {
		c.log.Info("source comment cleanup summary", slog.Int("files_modified", modified))
	}
	return nil
}

// proseExtensions are text files treated as prose for typography normalization.
// Invisible-character stripping is applied to every UTF-8 text file regardless;
// only the opt-in typography pass is limited to these.
var proseExtensions = []string{
	".md", ".markdown", ".mdx", ".txt", ".rst", ".adoc", ".asciidoc",
}

// CleanWatermarks strips invisible watermark and smuggling characters from
// every UTF-8 text file in the tree and normalizes exotic whitespace to plain
// ASCII spaces, using package watermark. When typography normalization is
// enabled, prose files additionally have their "smart" punctuation folded back
// to ASCII.
//
// It does not attempt to remove statistical (SynthID-style) watermarks, which
// live in word choice rather than in the bytes and cannot be scrubbed this way.
func (c *Cleaner) CleanWatermarks() error {
	modified := 0

	err := filepath.WalkDir(c.repoDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if dirAlwaysSkipped(d.Name()) {
				return filepath.SkipDir
			}
			return nil
		}
		if !d.Type().IsRegular() {
			return nil
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if watermark.IsBinary(content) {
			return nil
		}

		// Typography folding is scoped to prose: an em dash inside source code
		// is far more likely to be intentional than a stylistic artifact.
		var opts []watermark.Option
		if c.normalizeTypography && hasExt(strings.ToLower(filepath.Ext(d.Name())), proseExtensions) {
			opts = append(opts, watermark.WithTypography())
		}
		newContent, changed := watermark.Strip(content, opts...)
		if !changed {
			return nil
		}

		rel, _ := filepath.Rel(c.repoDir, path)
		c.log.Info("stripped watermark characters", slog.String("path", rel))
		modified++
		if c.dryRun {
			return nil
		}
		if err := c.preserve(path); err != nil {
			return fmt.Errorf("failed to back up %s: %w", rel, err)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		return os.WriteFile(path, newContent, info.Mode())
	})
	if err != nil {
		return err
	}
	if modified > 0 {
		c.log.Info("watermark cleanup summary", slog.Int("files_modified", modified))
	}
	return nil
}

func hasExt(ext string, list []string) bool {
	for _, e := range list {
		if e == ext {
			return true
		}
	}
	return false
}
