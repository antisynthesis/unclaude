package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"

	"github.com/antisynthesis/unclaude/internal/cleaner"
)

// usageText is printed for -h/--help and on flag parse errors.
const usageText = `unclaude - return your repository to human hands

Usage:
  unclaude [flags] [path]

Removes traces left behind by AI coding assistants (Claude Code, Codex CLI,
Cursor, Continue, Aider, GitHub Copilot):

  - AI tool directories: .claude/, .codex/, .cursor/, .continue/, .aider/
  - AI tool files: CLAUDE.md, AGENTS.md, .mcp.json, .cursorrules, .aider.*, ...
  - Generic .md files (preserving root README.md and docs/, doc/, adr/)
  - AI-related source comments and commit-message generation footers
  - Invisible watermark characters in code and prose: zero-width and joiner
    characters, bidi controls, the Unicode Tags block, variation selectors,
    invisible math operators, and exotic whitespace

Statistical (SynthID-style) watermarks live in word choice, not the bytes, and
cannot be removed this way. Runs in preview mode by default; use --apply to
write changes. [path] defaults to the current directory.

Flags:
  --apply                   apply changes (default is preview/dry-run mode)
  -i, --interactive         prompt before deleting each markdown file (requires --apply)
  -v, --verbose             show debug-level output
      --quiet               suppress info-level output; only warnings and errors
      --json                emit logs as JSON instead of human-readable text
      --purge-refs          after history rewrite, delete refs/original/ and run aggressive gc
      --normalize-typography  fold smart quotes, em/en dashes, ellipsis glyphs in prose to ASCII
  -h, --help                display this help
`

// exit codes returned by run.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// run parses args, wires up the cleaner, and executes every cleaning step.
// Log records are written to stdout; usage and flag errors to stderr. It
// returns a process exit code so main stays a one-liner and the whole CLI is
// testable without touching os.Exit or global flag state.
func run(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("unclaude", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usageText) }

	var (
		apply         bool
		verbose       bool
		quiet         bool
		jsonLog       bool
		interactive   bool
		purgeRefs     bool
		normalizeType bool
	)
	fs.BoolVar(&apply, "apply", false, "apply changes (default is preview/dry-run mode)")
	fs.BoolVar(&verbose, "verbose", false, "show debug-level output")
	fs.BoolVar(&verbose, "v", false, "show debug-level output (shorthand)")
	fs.BoolVar(&quiet, "quiet", false, "suppress info-level output")
	fs.BoolVar(&jsonLog, "json", false, "emit logs as JSON")
	fs.BoolVar(&interactive, "interactive", false, "prompt before deleting each markdown file")
	fs.BoolVar(&interactive, "i", false, "prompt before deleting each markdown file (shorthand)")
	fs.BoolVar(&purgeRefs, "purge-refs", false, "delete refs/original/ and gc after history rewrite")
	fs.BoolVar(&normalizeType, "normalize-typography", false, "fold smart punctuation in prose to ASCII")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	if fs.NArg() > 1 {
		fmt.Fprintf(stderr, "unclaude: expected at most one path, got %d\n", fs.NArg())
		return exitUsage
	}

	repoDir := "."
	if fs.NArg() == 1 {
		repoDir = fs.Arg(0)
	}

	dryRun := !apply
	if interactive && dryRun {
		fmt.Fprintln(stderr, "unclaude: --interactive requires --apply")
		return exitUsage
	}
	if quiet && verbose {
		fmt.Fprintln(stderr, "unclaude: --quiet and --verbose are mutually exclusive")
		return exitUsage
	}
	if !cleaner.IsGitRepo(repoDir) {
		fmt.Fprintf(stderr, "unclaude: %s is not a git repository\n", repoDir)
		return exitFailure
	}

	format := cleaner.LogFormatConsole
	if jsonLog {
		format = cleaner.LogFormatJSON
	}
	log := cleaner.NewLogger(cleaner.LoggerOptions{
		Verbose: verbose,
		Quiet:   quiet,
		Format:  format,
		Output:  stdout,
	})

	if dryRun {
		log.Info("preview mode (no changes will be made); use --apply to modify the repository")
	} else {
		log.Info("apply mode (changes will be made)")
	}

	c := cleaner.New(repoDir, cleaner.Options{
		DryRun:              dryRun,
		Interactive:         interactive,
		PurgeRefs:           purgeRefs,
		NormalizeTypography: normalizeType,
		Logger:              log,
	})

	// History rewrite runs first: in apply mode it requires a clean working
	// tree, so it must happen before the file-modifying steps dirty it.
	steps := []struct {
		name string
		fn   func() error
	}{
		{"clean git history", c.CleanGitHistory},
		{"clean ai tool artifacts", c.CleanAIArtifacts},
		{"clean markdown files", c.CleanMarkdownFiles},
		{"clean source comments", c.CleanSourceComments},
		{"clean watermark characters", c.CleanWatermarks},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			log.Error("step failed", slog.String("step", s.name), slog.Any("error", err))
			return exitFailure
		}
	}

	if dryRun {
		log.Info("preview complete; no changes were made (run with --apply to make permanent)")
	} else {
		log.Info("repository cleaned")
	}
	return exitOK
}
