package cleaner

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

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

func TestCleanClaudeDirectory(t *testing.T) {
	tests := []struct {
		name      string
		setup     func(string)
		dryRun    bool
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
				claudeDir := filepath.Join(dir, ".claude")
				os.Mkdir(claudeDir, 0755)
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

			cleaner := New(tmpDir, tt.dryRun, false, false)
			err := cleaner.CleanClaudeDirectory()
			if err != nil {
				t.Fatalf("CleanClaudeDirectory() error = %v", err)
			}

			claudeDir := filepath.Join(tmpDir, ".claude")
			_, err = os.Stat(claudeDir)
			exists := !os.IsNotExist(err)

			if exists != tt.shouldExist {
				t.Errorf("directory exists = %v, want %v", exists, tt.shouldExist)
			}
		})
	}
}

func TestCleanMarkdownFiles(t *testing.T) {
	tests := []struct {
		name           string
		files          map[string]string
		expectedCount  int
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
			expectedCount:  1,
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
			expectedCount:  1,
			shouldExist:    []string{"vendor/lib.md", "node_modules/pkg.md", "doc/api.md", "adr/001-decision.md"},
			shouldNotExist: []string{"src/notes.md"},
		},
		{
			name: "case insensitive README detection",
			files: map[string]string{
				"readme.md":  "# Test",
				"README.MD":  "# Test",
				"guide.Md":   "# Guide",
				"src/doc.md": "# Doc",
			},
			expectedCount:  2,
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
			expectedCount:  2,
			shouldExist:    []string{"README.md", "docs/internal/db.md"},
			shouldNotExist: []string{"src/README.md", "src/lib/notes.md"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			setupTestFiles(t, tmpDir, tt.files)

			cleaner := New(tmpDir, false, false, false)
			err := cleaner.CleanMarkdownFiles()
			if err != nil {
				t.Fatalf("CleanMarkdownFiles() error = %v", err)
			}

			for _, file := range tt.shouldExist {
				path := filepath.Join(tmpDir, file)
				if _, err := os.Stat(path); os.IsNotExist(err) {
					t.Errorf("expected file to exist: %s", file)
				}
			}

			for _, file := range tt.shouldNotExist {
				path := filepath.Join(tmpDir, file)
				if _, err := os.Stat(path); !os.IsNotExist(err) {
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

	cleaner := New(tmpDir, true, false, false)
	err := cleaner.CleanMarkdownFiles()
	if err != nil {
		t.Fatalf("CleanMarkdownFiles() error = %v", err)
	}

	// Files should still exist in dry run mode
	for file := range files {
		path := filepath.Join(tmpDir, file)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file to still exist in dry run: %s", file)
		}
	}
}

func TestCleanSourceComments(t *testing.T) {
	tests := []struct {
		name            string
		file            string
		content         string
		expectedContent string
		shouldModify    bool
	}{
		{
			name: "removes single line Claude comment",
			file: "main.go",
			content: `package main

// This is a normal comment
// Generated with Claude Code
func main() {}
`,
			expectedContent: `package main

// This is a normal comment
func main() {}
`,
			shouldModify: true,
		},
		{
			name: "removes Claude mention",
			file: "test.go",
			content: `package test

// Claude helped with this function
func test() {}
`,
			expectedContent: `package test

func test() {}
`,
			shouldModify: true,
		},
		{
			name: "removes Anthropic reference",
			file: "app.js",
			content: `// Normal comment
// Anthropic AI assisted
function test() {}
`,
			expectedContent: `// Normal comment
function test() {}
`,
			shouldModify: true,
		},
		{
			name: "removes AI assisted comment",
			file: "util.py",
			content: `# Helper function
# AI assisted with this code
def helper():
    pass
`,
			expectedContent: `# Helper function
def helper():
    pass
`,
			shouldModify: true,
		},
		{
			name: "removes Python Claude comment",
			file: "script.py",
			content: `# Normal comment
# Generated with Claude Code
def main():
    pass
`,
			expectedContent: `# Normal comment
def main():
    pass
`,
			shouldModify: true,
		},
		{
			name: "case insensitive matching",
			file: "test.js",
			content: `// normal comment
// CLAUDE CODE helped here
const x = 1;
`,
			expectedContent: `// normal comment
const x = 1;
`,
			shouldModify: true,
		},
		{
			name: "no Claude comments",
			file: "clean.go",
			content: `package main

// Regular comment
func test() {}
`,
			expectedContent: `package main

// Regular comment
func test() {}
`,
			shouldModify: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tmpDir := t.TempDir()
			filePath := filepath.Join(tmpDir, tt.file)

			dir := filepath.Dir(filePath)
			if dir != tmpDir {
				os.MkdirAll(dir, 0755)
			}

			os.WriteFile(filePath, []byte(tt.content), 0644)

			cleaner := New(tmpDir, false, false, false)
			err := cleaner.CleanSourceComments()
			if err != nil {
				t.Fatalf("CleanSourceComments() error = %v", err)
			}

			result, err := os.ReadFile(filePath)
			if err != nil {
				t.Fatalf("failed to read result file: %v", err)
			}

			resultStr := string(result)
			if resultStr != tt.expectedContent {
				t.Errorf("content mismatch:\ngot:\n%s\nwant:\n%s", resultStr, tt.expectedContent)
			}
		})
	}
}

func TestCleanCommitMessage(t *testing.T) {
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
			name: "removes Generated with Claude Code",
			input: `Add new feature

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
`,
			expected: `Add new feature
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
			name: "removes multiple Claude references",
			input: `Initial commit

Generated with Claude Code
Co-Authored-By: Claude <noreply@anthropic.com>
See https://claude.com/claude-code for more info
`,
			expected: `Initial commit
`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cleaner := New("", false, false, false)
			result := cleaner.cleanCommitMessage(tt.input)

			if result != tt.expected {
				t.Errorf("cleanCommitMessage() mismatch:\ngot:\n%q\nwant:\n%q", result, tt.expected)
			}
		})
	}
}

func TestCleanGitHistory(t *testing.T) {
	// Skip if git is not available
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()

	// Initialize git repo
	runGit(t, tmpDir, "init")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "user.name", "Test User")

	// Create a file and commit with Claude references
	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test content"), 0644)
	runGit(t, tmpDir, "add", "test.txt")

	commitMsg := `Initial commit

Co-Authored-By: Claude <noreply@anthropic.com>
`
	runGit(t, tmpDir, "commit", "-m", commitMsg)

	// Clean the history
	cleaner := New(tmpDir, false, false, false)
	err := cleaner.CleanGitHistory()
	if err != nil {
		t.Fatalf("CleanGitHistory() error = %v", err)
	}

	// Get the commit message
	cmd := exec.Command("git", "-C", tmpDir, "log", "--format=%B", "-n", "1")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to get commit message: %v", err)
	}

	result := string(output)
	if strings.Contains(result, "Claude") || strings.Contains(result, "Co-Authored-By") {
		t.Errorf("commit message still contains Claude references:\n%s", result)
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

	// Initialize git repo
	runGit(t, tmpDir, "init")
	runGit(t, tmpDir, "config", "user.email", "test@example.com")
	runGit(t, tmpDir, "config", "user.name", "Test User")

	testFile := filepath.Join(tmpDir, "test.txt")
	os.WriteFile(testFile, []byte("test"), 0644)
	runGit(t, tmpDir, "add", "test.txt")
	runGit(t, tmpDir, "commit", "-m", "Test\n\nCo-Authored-By: Claude <noreply@anthropic.com>")

	// Get original commit hash
	cmd := exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD")
	origHash, _ := cmd.Output()

	// Run dry run
	cleaner := New(tmpDir, true, false, false)
	err := cleaner.CleanGitHistory()
	if err != nil {
		t.Fatalf("CleanGitHistory() error = %v", err)
	}

	// Commit hash should be unchanged
	cmd = exec.Command("git", "-C", tmpDir, "rev-parse", "HEAD")
	newHash, _ := cmd.Output()

	if string(origHash) != string(newHash) {
		t.Errorf("commit hash changed in dry run mode")
	}
}

func TestCleanGitHistoryNoCommits(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}

	tmpDir := t.TempDir()
	runGit(t, tmpDir, "init")

	cleaner := New(tmpDir, false, false, false)
	err := cleaner.CleanGitHistory()
	if err != nil {
		t.Errorf("CleanGitHistory() with no commits should not error: %v", err)
	}
}

// Helper functions

func setupTestFiles(t *testing.T, baseDir string, files map[string]string) {
	t.Helper()
	for path, content := range files {
		fullPath := filepath.Join(baseDir, path)
		dir := filepath.Dir(fullPath)

		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatalf("failed to create directory %s: %v", dir, err)
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
