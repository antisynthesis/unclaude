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
