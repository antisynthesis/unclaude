package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"--help"}, &stdout, &stderr); code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stderr.String(), "Usage:") {
		t.Errorf("help text missing Usage: section:\n%s", stderr.String())
	}
}

func TestRunUsageErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
	}{
		{"unknown flag", []string{"--definitely-not-a-flag"}},
		{"too many paths", []string{"one", "two"}},
		{"interactive without apply", []string{"--interactive"}},
		{"quiet and verbose", []string{"--quiet", "--verbose"}},
		{"only and skip together", []string{"--only=comments", "--skip=history"}},
		{"unknown only step", []string{"--only=bogus"}},
		{"unknown skip step", []string{"--skip=bogus"}},
		{"skip every step", []string{"--skip=history,artifacts,markdown,comments,watermarks"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			if code := run(tt.args, &stdout, &stderr); code != exitUsage {
				t.Errorf("exit code = %d, want %d (stderr: %s)", code, exitUsage, stderr.String())
			}
		})
	}
}

func TestRunNotAGitRepo(t *testing.T) {
	dir := t.TempDir()
	var stdout, stderr bytes.Buffer
	if code := run([]string{dir}, &stdout, &stderr); code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "not a git repository") {
		t.Errorf("stderr missing expected message: %s", stderr.String())
	}
}

func TestRunPreviewLeavesFilesUntouched(t *testing.T) {
	dir := newGitRepo(t)
	fp := filepath.Join(dir, "main.go")
	orig := "package main\n\nvar s = \"hi" + string(rune(0x200B)) + "there\"\n"
	if err := os.WriteFile(fp, []byte(orig), 0o644); err != nil {
		t.Fatal(err)
	}

	var stdout, stderr bytes.Buffer
	if code := run([]string{dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if !strings.Contains(stdout.String(), "preview mode") {
		t.Errorf("stdout missing preview banner:\n%s", stdout.String())
	}
	got, _ := os.ReadFile(fp)
	if string(got) != orig {
		t.Errorf("preview modified file:\ngot:  %q\nwant: %q", string(got), orig)
	}
}

func TestRunApplyStripsWatermark(t *testing.T) {
	dir := newGitRepo(t)
	fp := filepath.Join(dir, "main.go")
	// Commit the watermarked file so the working tree is clean when apply runs.
	// History rewrite (which runs first) requires a clean tree; the commit
	// message is trace-free so that step finds nothing and returns quietly.
	watermarked := "package main\n\nvar s = \"hi" + string(rune(0x200B)) + "there\"\n"
	if err := os.WriteFile(fp, []byte(watermarked), 0o644); err != nil {
		t.Fatal(err)
	}
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial commit")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--apply", "--json", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	got, _ := os.ReadFile(fp)
	if want := "package main\n\nvar s = \"hithere\"\n"; string(got) != want {
		t.Errorf("watermark not stripped:\ngot:  %q\nwant: %q", string(got), want)
	}
	// --json means every emitted line must be valid JSON.
	for _, line := range strings.Split(strings.TrimSpace(stdout.String()), "\n") {
		if line == "" {
			continue
		}
		if !strings.HasPrefix(line, "{") || !strings.HasSuffix(line, "}") {
			t.Errorf("non-JSON log line: %q", line)
		}
	}
}

func TestRunOnlySelectsSingleStep(t *testing.T) {
	dir := newGitRepo(t)
	// CLAUDE.md would be removed by the artifacts step; --only=watermarks
	// must leave it in place while still stripping the zero-width space.
	watermarked := "package main\n\nvar s = \"hi" + string(rune(0x200B)) + "there\"\n"
	writeFile(t, dir, "main.go", watermarked)
	writeFile(t, dir, "CLAUDE.md", "instructions")
	runGit(t, dir, "add", ".")
	runGit(t, dir, "commit", "-m", "initial commit")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--apply", "--only=watermarks", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); err != nil {
		t.Error("--only=watermarks should not have run the artifacts step")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "main.go"))
	if want := "package main\n\nvar s = \"hithere\"\n"; string(got) != want {
		t.Errorf("watermark not stripped:\ngot:  %q\nwant: %q", string(got), want)
	}
}

func TestRunSkipExcludesStep(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "CLAUDE.md", "instructions")
	writeFile(t, dir, "CHANGELOG.md", "# Changes")

	var stdout, stderr bytes.Buffer
	// --skip=history avoids needing a commit; markdown is skipped too, so
	// CHANGELOG.md survives while CLAUDE.md (artifacts step) does not.
	if code := run([]string{"--apply", "--skip=history,markdown", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("artifacts step should have removed CLAUDE.md")
	}
	if _, err := os.Stat(filepath.Join(dir, "CHANGELOG.md")); err != nil {
		t.Error("markdown step was skipped; CHANGELOG.md should remain")
	}
}

func TestRunBackupAndRestoreViaCLI(t *testing.T) {
	dir := newGitRepo(t)
	const original = "instructions"
	writeFile(t, dir, "CLAUDE.md", original)

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--apply", "--skip=history", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("clean exit code = %d, want %d (stderr: %s)", code, exitOK, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Fatal("expected CLAUDE.md to be removed")
	}
	if !strings.Contains(stdout.String(), "unclaude restore") {
		t.Errorf("expected an undo hint in the output:\n%s", stdout.String())
	}

	// Preview restore changes nothing.
	stdout.Reset()
	if code := run([]string{"restore", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("restore preview exit code = %d (stderr: %s)", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, "CLAUDE.md")); !os.IsNotExist(err) {
		t.Error("restore preview should not write files")
	}

	// Applied restore brings the file back byte for byte.
	stdout.Reset()
	if code := run([]string{"restore", "--apply", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("restore exit code = %d (stderr: %s)", code, stderr.String())
	}
	got, err := os.ReadFile(filepath.Join(dir, "CLAUDE.md"))
	if err != nil {
		t.Fatalf("CLAUDE.md not restored: %v", err)
	}
	if string(got) != original {
		t.Errorf("restored content = %q, want %q", string(got), original)
	}
}

func TestRunNoBackupSkipsBackupDir(t *testing.T) {
	dir := newGitRepo(t)
	writeFile(t, dir, "CLAUDE.md", "instructions")

	var stdout, stderr bytes.Buffer
	if code := run([]string{"--apply", "--skip=history", "--no-backup", dir}, &stdout, &stderr); code != exitOK {
		t.Fatalf("exit code = %d (stderr: %s)", code, stderr.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".unclaude-backup")); !os.IsNotExist(err) {
		t.Error("--no-backup should not create a backup directory")
	}
}

func TestRunRestoreWithoutBackupFails(t *testing.T) {
	dir := newGitRepo(t)
	var stdout, stderr bytes.Buffer
	if code := run([]string{"restore", "--apply", dir}, &stdout, &stderr); code != exitFailure {
		t.Errorf("exit code = %d, want %d", code, exitFailure)
	}
	if !strings.Contains(stderr.String(), "no backups found") {
		t.Errorf("stderr missing expected message: %s", stderr.String())
	}
}

func TestRunRestoreHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run([]string{"restore", "--help"}, &stdout, &stderr); code != exitOK {
		t.Errorf("exit code = %d, want %d", code, exitOK)
	}
	if !strings.Contains(stderr.String(), "unclaude restore") {
		t.Errorf("restore help missing header:\n%s", stderr.String())
	}
}

func TestRunHelpListsSteps(t *testing.T) {
	var stdout, stderr bytes.Buffer
	run([]string{"--help"}, &stdout, &stderr)
	for _, key := range []string{"history", "artifacts", "markdown", "comments", "watermarks"} {
		if !strings.Contains(stderr.String(), key) {
			t.Errorf("help text missing step %q", key)
		}
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func newGitRepo(t *testing.T) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	dir := t.TempDir()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	return dir
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}
