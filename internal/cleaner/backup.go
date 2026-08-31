package cleaner

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// backupDirName is the repository-relative directory under which unclaude keeps
// copies of files it modifies or deletes. It is always skipped by the cleaning
// walks (see dirAlwaysSkipped).
const backupDirName = ".unclaude-backup"

// backupManifestName is the JSON index written into each timestamped backup.
const backupManifestName = "manifest.json"

// backupEntry records one preserved file so restore can put it back exactly.
type backupEntry struct {
	// Path is repository-relative and slash-separated (portable in JSON).
	Path string `json:"path"`
	// Mode is the original file permission bits, reapplied on restore.
	Mode os.FileMode `json:"mode"`
}

// backupManifest is the on-disk index of a single backup directory.
type backupManifest struct {
	Created string        `json:"created"`
	Entries []backupEntry `json:"entries"`
}

// backup accumulates copies of a repository's original files before the cleaner
// changes them. The timestamped destination directory is created lazily on the
// first save, so a run that changes nothing leaves no trace on disk.
type backup struct {
	repoDir string
	dir     string // absolute path to this run's backup dir; "" until first save
	nowUTC  time.Time
	entries []backupEntry
	seen    map[string]bool
}

// newBackup returns a backup rooted at repoDir. The directory is not created
// until the first file is saved.
func newBackup(repoDir string) *backup {
	return &backup{
		repoDir: repoDir,
		nowUTC:  time.Now().UTC(),
		seen:    make(map[string]bool),
	}
}

// ensureDir lazily creates the timestamped backup directory.
func (b *backup) ensureDir() error {
	if b.dir != "" {
		return nil
	}
	ts := b.nowUTC.Format("20060102-150405.000000000")
	b.dir = filepath.Join(b.repoDir, backupDirName, ts)
	if err := os.MkdirAll(b.dir, 0o755); err != nil {
		return fmt.Errorf("failed to create backup directory: %w", err)
	}
	return nil
}

// save copies the current contents of path (a file or directory) into the
// backup before the caller modifies or removes it. Directories are saved
// recursively. Saving the same path twice, or a path that no longer exists, is
// a no-op so callers can back up unconditionally.
func (b *backup) save(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	if info.IsDir() {
		return filepath.WalkDir(path, func(p string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			return b.saveFile(p)
		})
	}
	return b.saveFile(path)
}

// saveFile copies a single regular file into the backup.
func (b *backup) saveFile(path string) error {
	rel, err := filepath.Rel(b.repoDir, path)
	if err != nil {
		return err
	}
	relSlash := filepath.ToSlash(rel)
	if b.seen[relSlash] {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if err := b.ensureDir(); err != nil {
		return err
	}
	dst := filepath.Join(b.dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return err
	}
	b.seen[relSlash] = true
	b.entries = append(b.entries, backupEntry{Path: relSlash, Mode: info.Mode().Perm()})
	return nil
}

// finalize writes the manifest and returns the backup directory. It returns an
// empty path (and no error) when nothing was saved.
func (b *backup) finalize() (string, error) {
	if b.dir == "" {
		return "", nil
	}
	manifest := backupManifest{
		Created: b.nowUTC.Format(time.RFC3339),
		Entries: b.entries,
	}
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(b.dir, backupManifestName), data, 0o600); err != nil {
		return "", fmt.Errorf("failed to write backup manifest: %w", err)
	}
	return b.dir, nil
}

// RestoreOptions configure Restore.
type RestoreOptions struct {
	// DryRun reports what would be restored without writing anything.
	DryRun bool
	Logger *slog.Logger
}

// Restore reverses the most recent backup in repoDir, copying every preserved
// file back to its original path (recreating deleted files and overwriting
// modified ones). It does not touch git history; use git's reflog or
// refs/original/ to undo a history rewrite.
func Restore(repoDir string, opts RestoreOptions) error {
	log := opts.Logger
	if log == nil {
		log = newNopLogger()
	}

	dir, err := latestBackup(repoDir)
	if err != nil {
		return err
	}
	if dir == "" {
		return fmt.Errorf("no backups found under %s", filepath.Join(repoDir, backupDirName))
	}

	data, err := os.ReadFile(filepath.Join(dir, backupManifestName))
	if err != nil {
		return fmt.Errorf("failed to read backup manifest: %w", err)
	}
	var manifest backupManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		return fmt.Errorf("failed to parse backup manifest: %w", err)
	}

	rel, _ := filepath.Rel(repoDir, dir)
	log.Info("restoring from backup", slog.String("backup", rel), slog.Int("files", len(manifest.Entries)))

	restored := 0
	for _, e := range manifest.Entries {
		src := filepath.Join(dir, filepath.FromSlash(e.Path))
		dst := filepath.Join(repoDir, filepath.FromSlash(e.Path))
		log.Info("restoring file", slog.String("path", e.Path))
		if opts.DryRun {
			restored++
			continue
		}
		content, err := os.ReadFile(src)
		if err != nil {
			return fmt.Errorf("failed to read backed-up %s: %w", e.Path, err)
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		mode := e.Mode
		if mode == 0 {
			mode = 0o644
		}
		if err := os.WriteFile(dst, content, mode); err != nil {
			return fmt.Errorf("failed to restore %s: %w", e.Path, err)
		}
		restored++
	}

	if opts.DryRun {
		log.Info("restore preview complete; no changes were made (run with --apply to restore)", slog.Int("files", restored))
	} else {
		log.Info("restore complete", slog.Int("files", restored))
	}
	return nil
}

// latestBackup returns the newest timestamped backup directory containing a
// manifest, or "" when none exist. Timestamps sort chronologically as strings.
func latestBackup(repoDir string) (string, error) {
	base := filepath.Join(repoDir, backupDirName)
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", err
	}

	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(base, e.Name(), backupManifestName)); err == nil {
			dirs = append(dirs, e.Name())
		}
	}
	if len(dirs) == 0 {
		return "", nil
	}
	sort.Strings(dirs)
	return filepath.Join(base, dirs[len(dirs)-1]), nil
}
