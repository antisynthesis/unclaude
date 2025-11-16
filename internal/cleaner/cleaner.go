package cleaner

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

// Cleaner handles the removal of Claude Code traces from a git repository.
type Cleaner struct {
	repoDir     string
	dryRun      bool
	verbose     bool
	interactive bool
	yesToAll    bool
}

// New creates a new Cleaner instance.
func New(repoDir string, dryRun, verbose, interactive bool) *Cleaner {
	return &Cleaner{
		repoDir:     repoDir,
		dryRun:      dryRun,
		verbose:     verbose,
		interactive: interactive,
		yesToAll:    false,
	}
}

// IsGitRepo checks if the given directory is a git repository.
func IsGitRepo(dir string) bool {
	gitDir := filepath.Join(dir, ".git")
	info, err := os.Stat(gitDir)
	if err != nil {
		return false
	}
	return info.IsDir()
}

// promptForDeletion asks the user whether to delete a file.
// Returns true if the file should be deleted, false otherwise.
func (c *Cleaner) promptForDeletion(relPath string) bool {
	if c.yesToAll {
		return true
	}

	fmt.Printf("\nDelete %s? [y/N/a] (y=yes, n=no, a=yes to all): ", relPath)

	var response string
	fmt.Scanln(&response)

	response = strings.ToLower(strings.TrimSpace(response))

	switch response {
	case "a", "all":
		c.yesToAll = true
		return true
	case "y", "yes":
		return true
	default:
		return false
	}
}

// CleanClaudeDirectory removes the .claude directory and all its contents.
func (c *Cleaner) CleanClaudeDirectory() error {
	claudeDir := filepath.Join(c.repoDir, ".claude")

	// Check if .claude directory exists
	info, err := os.Stat(claudeDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	if !info.IsDir() {
		return nil
	}

	if c.verbose {
		fmt.Println("Removing .claude directory")
	}

	if !c.dryRun {
		if err := os.RemoveAll(claudeDir); err != nil {
			return fmt.Errorf("failed to remove .claude directory: %w", err)
		}
	}

	fmt.Println("Removed .claude directory")
	return nil
}

// CleanMarkdownFiles removes markdown files from the repository.
// It recursively walks the directory tree and removes all .md files,
// excluding certain directories like .git, node_modules, vendor, docs, doc, and adr.
// The root README.md is also preserved.
func (c *Cleaner) CleanMarkdownFiles() error {
	excludeDirs := map[string]bool{
		".git":         true,
		".claude":      true,
		"node_modules": true,
		"vendor":       true,
		"docs":         true,
		"doc":          true,
		"adr":          true,
	}

	var filesToRemove []string

	err := filepath.Walk(c.repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip excluded directories
		if info.IsDir() {
			if excludeDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if it's a markdown file
		if strings.HasSuffix(strings.ToLower(info.Name()), ".md") {
			// Preserve root README.md
			if filepath.Dir(path) == c.repoDir && strings.ToLower(info.Name()) == "readme.md" {
				return nil
			}
			filesToRemove = append(filesToRemove, path)
		}

		return nil
	})

	if err != nil {
		return err
	}

	// Remove the files
	removedCount := 0
	skippedCount := 0

	for _, file := range filesToRemove {
		relPath, _ := filepath.Rel(c.repoDir, file)

		// In interactive mode, prompt for each file
		if c.interactive && !c.dryRun {
			if !c.promptForDeletion(relPath) {
				skippedCount++
				if c.verbose {
					fmt.Printf("Skipped: %s\n", relPath)
				}
				continue
			}
		}

		if c.verbose || (c.interactive && !c.yesToAll) {
			fmt.Printf("Removing markdown file: %s\n", relPath)
		}

		if !c.dryRun {
			if err := os.Remove(file); err != nil {
				return fmt.Errorf("failed to remove %s: %w", relPath, err)
			}
		}
		removedCount++
	}

	if removedCount > 0 {
		fmt.Printf("Removed %d markdown file(s)\n", removedCount)
	}
	if skippedCount > 0 {
		fmt.Printf("Skipped %d markdown file(s)\n", skippedCount)
	}

	return nil
}

// CleanSourceComments removes Claude-related comments from source files.
// It processes common source file extensions and removes comments that
// reference Claude or Claude Code.
func (c *Cleaner) CleanSourceComments() error {
	extensions := []string{".go", ".js", ".ts", ".jsx", ".tsx", ".py", ".java", ".c", ".cpp", ".h", ".hpp", ".rs", ".rb", ".php", ".cs"}
	excludeDirs := map[string]bool{
		".git":         true,
		"node_modules": true,
		"vendor":       true,
	}

	// Pattern to match Claude-related comments
	claudePatterns := []*regexp.Regexp{
		// Direct Claude mentions
		regexp.MustCompile(`(?i)//.*\bclaude\b`),
		regexp.MustCompile(`(?i)#.*\bclaude\b`),
		regexp.MustCompile(`(?i)/\*.*\bclaude\b.*\*/`),
		regexp.MustCompile(`(?i)<!--.*\bclaude\b.*-->`),
		// Anthropic references
		regexp.MustCompile(`(?i)//.*\banthropic\b`),
		regexp.MustCompile(`(?i)#.*\banthropic\b`),
		// AI assistance markers
		regexp.MustCompile(`(?i)//.*\bai\s+(assisted|generated|created)`),
		regexp.MustCompile(`(?i)#.*\bai\s+(assisted|generated|created)`),
		regexp.MustCompile(`(?i)//.*\bgenerated\s+with\b`),
		regexp.MustCompile(`(?i)#.*\bgenerated\s+with\b`),
		// Claude Code specific
		regexp.MustCompile(`(?i)//.*claude\s*code`),
		regexp.MustCompile(`(?i)#.*claude\s*code`),
	}

	var modifiedFiles int

	err := filepath.Walk(c.repoDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		if info.IsDir() {
			if excludeDirs[info.Name()] {
				return filepath.SkipDir
			}
			return nil
		}

		// Check if file has a relevant extension
		hasExt := false
		for _, ext := range extensions {
			if strings.HasSuffix(strings.ToLower(info.Name()), ext) {
				hasExt = true
				break
			}
		}

		if !hasExt {
			return nil
		}

		// Process the file
		modified, err := c.cleanFileComments(path, claudePatterns)
		if err != nil {
			return err
		}

		if modified {
			modifiedFiles++
			if c.verbose {
				relPath, _ := filepath.Rel(c.repoDir, path)
				fmt.Printf("Cleaned comments in: %s\n", relPath)
			}
		}

		return nil
	})

	if err != nil {
		return err
	}

	if modifiedFiles > 0 {
		fmt.Printf("Cleaned comments in %d file(s)\n", modifiedFiles)
	}

	return nil
}

// cleanFileComments removes Claude-related comments from a single file.
func (c *Cleaner) cleanFileComments(path string, patterns []*regexp.Regexp) (bool, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return false, err
	}

	originalContent := content
	lines := bytes.Split(content, []byte("\n"))
	var newLines [][]byte
	modified := false

	for _, line := range lines {
		shouldRemove := false
		for _, pattern := range patterns {
			if pattern.Match(line) {
				shouldRemove = true
				modified = true
				break
			}
		}

		if !shouldRemove {
			newLines = append(newLines, line)
		}
	}

	if !modified {
		return false, nil
	}

	if !c.dryRun {
		newContent := bytes.Join(newLines, []byte("\n"))
		// Preserve original file permissions
		info, err := os.Stat(path)
		if err != nil {
			return false, err
		}

		if err := os.WriteFile(path, newContent, info.Mode()); err != nil {
			return false, err
		}
	}

	// Double check if content actually changed
	return !bytes.Equal(originalContent, bytes.Join(newLines, []byte("\n"))), nil
}

// CleanGitHistory removes Claude-related information from git commit messages.
// This uses git filter-branch to rewrite commit messages, removing:
// - Co-Authored-By: Claude lines
// - Generated with Claude Code footers
func (c *Cleaner) CleanGitHistory() error {
	// Check if there are any commits
	checkCmd := exec.Command("git", "-C", c.repoDir, "rev-list", "--count", "HEAD")
	output, err := checkCmd.CombinedOutput()
	if err != nil {
		// No commits yet
		if c.verbose {
			fmt.Println("No commits to clean")
		}
		return nil
	}

	commitCount := strings.TrimSpace(string(output))
	if commitCount == "0" {
		if c.verbose {
			fmt.Println("No commits to clean")
		}
		return nil
	}

	if c.verbose {
		fmt.Println("Cleaning git commit history...")
	}

	// Get all commit hashes in reverse order (oldest first)
	listCmd := exec.Command("git", "-C", c.repoDir, "rev-list", "--reverse", "HEAD")
	output, err = listCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list commits: %w", err)
	}

	commits := strings.Split(strings.TrimSpace(string(output)), "\n")
	if len(commits) == 0 {
		return nil
	}

	modifiedCount := 0

	for _, commit := range commits {
		commit = strings.TrimSpace(commit)
		if commit == "" {
			continue
		}

		// Get the commit message
		msgCmd := exec.Command("git", "-C", c.repoDir, "log", "--format=%B", "-n", "1", commit)
		msgOutput, err := msgCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get commit message for %s: %w", commit, err)
		}

		originalMsg := string(msgOutput)
		cleanedMsg := c.cleanCommitMessage(originalMsg)

		if originalMsg != cleanedMsg {
			modifiedCount++
			if c.verbose {
				shortHash := commit
				if len(commit) > 7 {
					shortHash = commit[:7]
				}
				fmt.Printf("Cleaning commit: %s\n", shortHash)
			}

			if !c.dryRun {
				// We'll use git filter-branch later for actual rewriting
				// For now, just count what would be changed
			}
		}
	}

	if modifiedCount > 0 {
		if !c.dryRun {
			// Use git filter-repo approach via filter-branch
			// Create a message filter script
			filterScript := c.createMessageFilterScript()
			defer os.Remove(filterScript)

			cmd := exec.Command("git", "-C", c.repoDir, "filter-branch", "-f", "--msg-filter",
				fmt.Sprintf("sh %s", filterScript), "--", "--all")

			var stderr bytes.Buffer
			cmd.Stderr = &stderr

			if err := cmd.Run(); err != nil {
				return fmt.Errorf("failed to rewrite git history: %w\nStderr: %s", err, stderr.String())
			}

			// Clean up backup refs
			cleanupCmd := exec.Command("git", "-C", c.repoDir, "for-each-ref", "--format=%(refname)", "refs/original/")
			refOutput, err := cleanupCmd.Output()
			if err == nil && len(refOutput) > 0 {
				refs := strings.Split(strings.TrimSpace(string(refOutput)), "\n")
				for _, ref := range refs {
					ref = strings.TrimSpace(ref)
					if ref != "" {
						exec.Command("git", "-C", c.repoDir, "update-ref", "-d", ref).Run()
					}
				}
			}

			// Cleanup reflog and gc
			exec.Command("git", "-C", c.repoDir, "reflog", "expire", "--expire=now", "--all").Run()
			exec.Command("git", "-C", c.repoDir, "gc", "--prune=now", "--aggressive").Run()
		}

		fmt.Printf("Cleaned %d commit message(s)\n", modifiedCount)
	}

	return nil
}

// createMessageFilterScript creates a temporary shell script for git filter-branch.
func (c *Cleaner) createMessageFilterScript() string {
	tmpFile, err := os.CreateTemp("", "unclaude-filter-*.sh")
	if err != nil {
		return ""
	}
	defer tmpFile.Close()

	script := `#!/bin/sh
cat | sed -e '/Co-Authored-By: Claude <noreply@anthropic.com>/d' \
          -e '/Co-Authored-By:.*anthropic\.com/d' \
          -e '/🤖 Generated with \[Claude Code\]/d' \
          -e '/Generated with Claude Code/d' \
          -e '/\[Claude Code\]/d' \
          -e '/claude\.com\/claude-code/d' \
          -e '/anthropic\.com/d' \
          -e '/AI assisted/d' \
          -e '/AI-assisted/d' \
          -e '/AI generated/d' \
          -e '/AI-generated/d'
`
	tmpFile.WriteString(script)
	tmpFile.Chmod(0755)

	return tmpFile.Name()
}

// cleanCommitMessage removes Claude-related lines from a commit message.
func (c *Cleaner) cleanCommitMessage(msg string) string {
	scanner := bufio.NewScanner(strings.NewReader(msg))
	var lines []string

	claudePatterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)Co-Authored-By:\s*Claude\s*<.*@anthropic\.com>`),
		regexp.MustCompile(`(?i)Co-Authored-By:.*@anthropic\.com`),
		regexp.MustCompile(`(?i).*Generated with.*Claude.*`),
		regexp.MustCompile(`(?i).*\[Claude Code\].*`),
		regexp.MustCompile(`(?i).*claude\.com.*`),
		regexp.MustCompile(`(?i).*anthropic\.com.*`),
		regexp.MustCompile(`🤖.*Claude.*`),
		regexp.MustCompile(`(?i).*AI\s+(assisted|generated|created).*`),
		regexp.MustCompile(`(?i).*AI-(assisted|generated|created).*`),
	}

	for scanner.Scan() {
		line := scanner.Text()
		shouldRemove := false

		for _, pattern := range claudePatterns {
			if pattern.MatchString(line) {
				shouldRemove = true
				break
			}
		}

		if !shouldRemove {
			lines = append(lines, line)
		}
	}

	// Remove trailing empty lines
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}

	return strings.Join(lines, "\n") + "\n"
}
