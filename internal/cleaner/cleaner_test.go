package cleaner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func newTestCleaner(repo string, dryRun bool) *Cleaner {
	return New(repo, Options{DryRun: dryRun})
}

func TestIsGitRepo(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(string)
		expected bool
	}{
		{
			name: "valid git repository",
			setup: func(dir string) {
				os.Mkdir(filepath.Join(dir, ".git"), 0755)
			},
			expected: true,
		},
		{
			name:     "not a git repository",
			setup:    func(dir string) {},
			expected: false,
		},
		{
			name: ".git is a file not directory",
			setup: func(dir string) {
				os.WriteFile(filepath.Join(dir, ".git"), []byte("gitdir: ../"), 0644)
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			result := IsGitRepo(tmpDir)
			if result != tt.expected {
				t.Errorf("IsGitRepo() = %v, want %v", result, tt.expected)
			}
		})
	}
}

func TestCleanAIArtifactsRemovesDirectories(t *testing.T) {
	tests := []struct {
		name        string
		setup       func(string)
		dryRun      bool
		shouldExist bool
	}{
		{
			name: "removes .claude directory",
			setup: func(dir string) {
				claudeDir := filepath.Join(dir, ".claude")
				os.Mkdir(claudeDir, 0755)
				os.Mkdir(filepath.Join(claudeDir, "commands"), 0755)
				os.WriteFile(filepath.Join(claudeDir, "commands", "test.md"), []byte("test"), 0644)
			},
			dryRun:      false,
			shouldExist: false,
		},
		{
			name: "dry run preserves .claude directory",
			setup: func(dir string) {
				os.Mkdir(filepath.Join(dir, ".claude"), 0755)
			},
			dryRun:      true,
			shouldExist: true,
		},
		{
			name:        "no .claude directory",
			setup:       func(dir string) {},
			dryRun:      false,
			shouldExist: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			tt.setup(tmpDir)

			c := newTestCleaner(tmpDir, tt.dryRun)
			if err := c.CleanAIArtifacts(); err != nil {
				t.Fatalf("CleanAIArtifacts() error = %v", err)
			}

			_, err := os.Stat(filepath.Join(tmpDir, ".claude"))
			exists := !os.IsNotExist(err)
			if exists != tt.shouldExist {
				t.Errorf("directory exists = %v, want %v", exists, tt.shouldExist)
			}
		})
	}
}

func TestCleanAIArtifactsRemovesAllAgentDirs(t *testing.T) {
	tmpDir := t.TempDir()
	for _, d := range []string{".claude", ".codex", ".cursor", ".continue", ".aider"} {
		if err := os.Mkdir(filepath.Join(tmpDir, d), 0755); err != nil {
			t.Fatal(err)
		}
	}
	c := newTestCleaner(tmpDir, false)
	if err := c.CleanAIArtifacts(); err != nil {
		t.Fatalf("CleanAIArtifacts() error = %v", err)
	}
	for _, d := range []string{".claude", ".codex", ".cursor", ".continue", ".aider"} {
		if _, err := os.Stat(filepath.Join(tmpDir, d)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", d)
		}
	}
}

func TestCleanAIArtifactsRemovesAgentFiles(t *testing.T) {
	tmpDir := t.TempDir()
	files := map[string]string{
		"CLAUDE.md":                       "instructions",
		"AGENTS.md":                       "agent guide",
		".mcp.json":                       "{}",
		".claude.json":                    "{}",
		".claudeignore":                   "",
		".cursorrules":                    "rules",
		".cursorignore":                   "",
		".aider.conf.yml":                 "model: gpt-4",
		".aider.input.history":            "history",
		".aider.chat.history.md":          "chat",
		"docs/CLAUDE.md":                  "nested instructions",
		".github/copilot-instructions.md": "copilot",
		"src/legitimate.go":               "package src",
		"README.md":                       "# project",
	}
	setupTestFiles(t, tmpDir, files)

	c := newTestCleaner(tmpDir, false)
	if err := c.CleanAIArtifacts(); err != nil {
		t.Fatalf("CleanAIArtifacts() error = %v", err)
	}

	shouldBeGone := []string{
		"CLAUDE.md", "AGENTS.md", ".mcp.json", ".claude.json",
		".claudeignore", ".cursorrules", ".cursorignore",
		".aider.conf.yml", ".aider.input.history", ".aider.chat.history.md",
		"docs/CLAUDE.md", ".github/copilot-instructions.md",
	}
	for _, f := range shouldBeGone {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); !os.IsNotExist(err) {
			t.Errorf("expected %s to be removed", f)
		}
	}
	shouldStay := []string{"src/legitimate.go", "README.md"}
	for _, f := range shouldStay {
		if _, err := os.Stat(filepath.Join(tmpDir, f)); os.IsNotExist(err) {
			t.Errorf("expected %s to be preserved", f)
		}
	}
}

func TestCleanMarkdownFiles(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string
		shouldExist    []string
		shouldNotExist []string
	}{
		{
			name: "preserves root README.md and docs directory",
			files: map[string]string{
				"README.md":     "# Test",
				"docs/guide.md": "# Guide",
				"main.go":       "package main",
				"CHANGELOG.md":  "# Changes",
			},
			shouldExist:    []string{"main.go", "README.md", "docs/guide.md"},
			shouldNotExist: []string{"CHANGELOG.md"},
		},
		{
			name: "skips vendor, node_modules, and doc directories",
			files: map[string]string{
				"vendor/lib.md":       "# Vendor",
				"node_modules/pkg.md": "# Node",
				"doc/api.md":          "# API",
				"adr/001-decision.md": "# ADR",
				"src/notes.md":        "# Notes",
			},
			shouldExist:    []string{"vendor/lib.md", "node_modules/pkg.md", "doc/api.md", "adr/001-decision.md"},
			shouldNotExist: []string{"src/notes.md"},
		},
		{
			name: "case insensitive README detection at root only",
			files: map[string]string{
				"readme.md":  "# Test",
				"README.MD":  "# Test",
				"guide.Md":   "# Guide",
				"src/doc.md": "# Doc",
			},
			shouldExist:    []string{"readme.md", "README.MD"},
			shouldNotExist: []string{"guide.Md", "src/doc.md"},
		},
		{
			name: "removes nested markdown outside protected dirs",
			files: map[string]string{
				"README.md":           "# Root",
				"src/README.md":       "# Src",
				"src/lib/notes.md":    "# Notes",
				"docs/internal/db.md": "# DB",
			},
			shouldExist:    []string{"README.md", "docs/internal/db.md"},
			shouldNotExist: []string{"src/README.md", "src/lib/notes.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			setupTestFiles(t, tmpDir, tt.files)

			c := newTestCleaner(tmpDir, false)
			if err := c.CleanMarkdownFiles(); err != nil {
				t.Fatalf("CleanMarkdownFiles() error = %v", err)
			}

			for _, file := range tt.shouldExist {
				if _, err := os.Stat(filepath.Join(tmpDir, file)); os.IsNotExist(err) {
					t.Errorf("expected file to exist: %s", file)
				}
			}
			for _, file := range tt.shouldNotExist {
				if _, err := os.Stat(filepath.Join(tmpDir, file)); !os.IsNotExist(err) {
					t.Errorf("expected file to not exist: %s", file)
				}
			}
		})
	}
}

func TestCleanMarkdownFilesDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	files := map[string]string{
		"README.md": "# Test",
		"doc.md":    "# Doc",
	}
	setupTestFiles(t, tmpDir, files)

	c := newTestCleaner(tmpDir, true)
	if err := c.CleanMarkdownFiles(); err != nil {
		t.Fatalf("CleanMarkdownFiles() error = %v", err)
	}
	for file := range files {
		if _, err := os.Stat(filepath.Join(tmpDir, file)); os.IsNotExist(err) {
			t.Errorf("expected file to still exist in dry run: %s", file)
		}
	}
}

func TestCleanSourceCommentsStandaloneAndInline(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		input    string
		expected string
	}{
		{
			name: "standalone Claude comment removed",
			file: "main.go",
			input: `package main

// Generated by Claude
// Normal comment
func main() {}
`,
			expected: `package main

// Normal comment
func main() {}
`,
		},
		{
			name: "inline Claude comment stripped, code preserved",
			file: "main.go",
			input: `package main

func main() {
	x := compute() // generated by Claude
	_ = x
}
`,
			expected: `package main

func main() {
	x := compute()
	_ = x
}
`,
		},
		{
			name: "multi-line JSDoc block removed",
			file: "app.js",
			input: `/**
 * Generated by Claude.
 * Helper function.
 */
function foo() {}
`,
			expected: `function foo() {}
`,
		},
		{
			name: "Python docstring with Codex removed",
			file: "script.py",
			input: `def foo():
    """Generated by Codex."""
    return 1
`,
			expected: `def foo():
    return 1
`,
		},
		{
			name: "ChatGPT inline Python comment",
			file: "script.py",
			input: `x = 1  # generated by ChatGPT
y = 2
`,
			expected: `x = 1
y = 2
`,
		},
		{
			name: "OpenAI session URL in comment",
			file: "app.ts",
			input: `// see https://chatgpt.com/session/abc
const x = 1;
`,
			expected: `const x = 1;
`,
		},
		{
			name: "claude.ai URL in comment",
			file: "lib.go",
			input: `package lib

// see https://claude.ai/code/session_01H
func F() {}
`,
			expected: `package lib

func F() {}
`,
		},
		{
			name: "comment-like substring inside string preserved",
			file: "main.go",
			input: `package main

func main() {
	s := "// generated by Claude"
	_ = s
}
`,
			expected: `package main

func main() {
	s := "// generated by Claude"
	_ = s
}
`,
		},
		{
			name: "no AI comments leaves file untouched",
			file: "clean.go",
			input: `package main

// Regular comment
func test() {}
`,
			expected: `package main

// Regular comment
func test() {}
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			fp := filepath.Join(tmpDir, tt.file)
			if err := os.WriteFile(fp, []byte(tt.input), 0644); err != nil {
				t.Fatal(err)
			}

			c := newTestCleaner(tmpDir, false)
			if err := c.CleanSourceComments(); err != nil {
				t.Fatalf("CleanSourceComments() error = %v", err)
			}

			out, err := os.ReadFile(fp)
			if err != nil {
				t.Fatal(err)
			}
			if string(out) != tt.expected {
				t.Errorf("content mismatch:\ngot:\n%q\nwant:\n%q", string(out), tt.expected)
			}
		})
	}
}

func TestCleanCommitMessage(t *testing.T) {
	patterns := CompiledCommitMessagePatterns()
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name: "removes Co-Authored-By Claude",
			input: `Fix bug in parser

Co-Authored-By: Claude <noreply@anthropic.com>
`,
			expected: `Fix bug in parser
`,
		},
		{
			name: "removes claude.ai/code session URL",
			input: `Add feature

https://claude.ai/code/session_01HKABCDEF
`,
			expected: `Add feature
`,
		},
		{
			name: "removes Codex co-author",
			input: `Refactor api

Co-Authored-By: Codex <codex@openai.com>
`,
			expected: `Refactor api
`,
		},
		{
			name: "removes chatgpt.com link",
			input: `Improve docs

See: https://chatgpt.com/share/xyz
`,
			expected: `Improve docs
`,
		},
		{
			name: "preserves normal commit message",
			input: `Update documentation

Added examples and clarifications.
`,
			expected: `Update documentation

Added examples and clarifications.
`,
		},
		{
			name: "removes multiple references across vendors",
			input: `Initial commit

Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>
Co-Authored-By: ChatGPT <noreply@openai.com>
https://claude.ai/code/session_X
`,
			expected: `Initial commit
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := cleanCommitMessage(tt.input, patterns)
			if result != tt.expected {
				t.Errorf("cleanCommitMessage() mismatch:\ngot:\n%q\nwant:\n%q", result, tt.expected)
			}
		})
	}
}

func TestCleanGitHistory(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)
	runGit(t, tmpDir, "add", "test.txt")

	commitMsg := `Initial commit

Co-Authored-By: Claude <noreply@anthropic.com>
https://claude.ai/code/session_01H
`
	runGit(t, tmpDir, "commit", "-m", commitMsg)

	c := newTestCleaner(tmpDir, false)
	c.SetSkipHistoryPrompt(true)
	if err := c.CleanGitHistory(); err != nil {
		t.Fatalf("CleanGitHistory() error = %v", err)
	}

	cmd := exec.Command("git", "-C", tmpDir, "log", "--format=%B", "-n", "1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit message: %v", err)
	}
	result := string(output)
	if strings.Contains(result, "Claude") ||
		strings.Contains(result, "Co-Authored-By") ||
		strings.Contains(result, "claude.ai") {
		t.Errorf("commit message still contains AI references:\n%s", result)
	}
	if !strings.Contains(result, "Initial commit") {
		t.Errorf("commit message lost original content:\n%s", result)
	}
}

func TestCleanGitHistoryDryRun(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	runGit(t, tmpDir, "add", "test.txt")
	runGit(t, tmpDir, "commit", "-m", "Test\n\nCo-Authored-By: Claude <noreply@anthropic.com>")

	origHash, _ := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").Output()

	c := newTestCleaner(tmpDir, true)
	if err := c.CleanGitHistory(); err != nil {
		t.Fatalf("CleanGitHistory() error = %v", err)
	}

	newHash, _ := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD").Output()
	if string(origHash) != string(newHash) {
		t.Errorf("commit hash changed in dry run mode")
	}
}

func TestCleanGitHistoryNoCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	initTestRepo(t, tmpDir)

	c := newTestCleaner(tmpDir, false)
	if err := c.CleanGitHistory(); err != nil {
		t.Errorf("CleanGitHistory() with no commits should not error: %v", err)
	}
}

// initTestRepo creates a fresh git repo at dir with signing disabled and a
// known author. Needed in environments that otherwise force commit signing.
func initTestRepo(t *testing.T, dir string) {
	t.Helper()
	runGit(t, dir, "init")
	runGit(t, dir, "config", "user.email", "test@example.com")
	runGit(t, dir, "config", "user.name", "Test User")
	runGit(t, dir, "config", "commit.gpgsign", "false")
	runGit(t, dir, "config", "tag.gpgsign", "false")
	runGit(t, dir, "config", "gpg.format", "openpgp")
}

func setupTestFiles(t *testing.T, baseDir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		fullPath := filepath.Join(baseDir, path)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0755); err != nil {
			t.Fatalf("failed to create directory: %v", err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0644); err != nil {
			t.Fatalf("failed to write file %s: %v", path, err)
		}
	}
}

func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmdArgs := append([]string{"-C", dir}, args...)
	cmd := exec.Command("git", cmdArgs...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\nOutput: %s", args, err, output)
	}
}
