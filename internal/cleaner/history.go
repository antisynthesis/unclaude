package cleaner

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"log/slog"
)

// CleanGitHistory scans the repository's commit messages for AI traces and,
// if any are found, rewrites history to remove them. Prefers `git-filter-repo`
// when available; falls back to `git filter-branch` with a deprecation warning.
func (c *Cleaner) CleanGitHistory() error {
	statusCmd := exec.Command("git", "-C", c.repoDir, "status", "--porcelain")
	statusOutput, err := statusCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to check git status: %w", err)
	}
	if len(statusOutput) > 0 && !c.dryRun {
		return fmt.Errorf("repository has unstaged changes - commit or stash them before rewriting history")
	}

	checkCmd := exec.Command("git", "-C", c.repoDir, "rev-list", "--count", "HEAD")
	output, err := checkCmd.CombinedOutput()
	if err != nil {
		c.log.Debug("no commits to clean")
		return nil
	}
	if strings.TrimSpace(string(output)) == "0" {
		c.log.Debug("no commits to clean")
		return nil
	}

	c.log.Debug("scanning git commit history")

	listCmd := exec.Command("git", "-C", c.repoDir, "rev-list", "--reverse", "HEAD")
	output, err = listCmd.Output()
	if err != nil {
		return fmt.Errorf("failed to list commits: %w", err)
	}
	commits := strings.Split(strings.TrimSpace(string(output)), "\n")

	patterns := CompiledCommitMessagePatterns()
	modifiedCount := 0
	for _, commit := range commits {
		commit = strings.TrimSpace(commit)
		if commit == "" {
			continue
		}
		msgCmd := exec.Command("git", "-C", c.repoDir, "log", "--format=%B", "-n", "1", commit)
		msgOutput, err := msgCmd.Output()
		if err != nil {
			return fmt.Errorf("failed to get commit message for %s: %w", commit, err)
		}

		original := string(msgOutput)
		cleaned := cleanCommitMessage(original, patterns)
		if normalizeMessage(original) == cleaned {
			continue
		}
		modifiedCount++
		short := commit
		if len(commit) > 7 {
			short = commit[:7]
		}
		c.log.Debug("ai trace in commit", slog.String("commit", short))
	}

	if modifiedCount == 0 {
		c.log.Debug("no ai traces found in commit history")
		return nil
	}

	c.log.Info("ai traces detected in commit history", slog.Int("commits", modifiedCount))

	if c.dryRun {
		return nil
	}

	if !c.skipHistoryPrompt {
		fmt.Printf("\nFound %d commit(s) with AI assistance traces.\n", modifiedCount)
		fmt.Println("WARNING: Rewriting git history is DESTRUCTIVE and PERMANENT.")
		fmt.Println("All commit hashes will change. Signed commits will be invalidated.")
		fmt.Print("\nProceed with git history rewrite? [y/N]: ")

		var response string
		fmt.Scanln(&response)
		response = strings.ToLower(strings.TrimSpace(response))
		if response != "y" && response != "yes" {
			c.log.Warn("user declined git history rewrite")
			return nil
		}
	}

	if _, err := exec.LookPath("git-filter-repo"); err == nil {
		c.log.Info("rewriting git history with git-filter-repo")
		if err := c.rewriteWithFilterRepo(); err != nil {
			return err
		}
	} else {
		c.log.Warn("git-filter-repo not found; falling back to deprecated git filter-branch (install git-filter-repo for the recommended path)")
		if err := c.rewriteWithFilterBranch(); err != nil {
			return err
		}
	}

	if c.purgeRefs {
		c.log.Debug("purging backup refs and running gc")
		c.purgeBackupRefs()
	} else {
		c.log.Info("backup refs preserved under refs/original/ (re-run with --purge-refs to drop them)")
	}

	c.log.Info("cleaned commit messages", slog.Int("commits", modifiedCount))
	return nil
}

// rewriteWithFilterRepo invokes git-filter-repo with an inline Python
// --message-callback that applies CommitMessagePatterns.
func (c *Cleaner) rewriteWithFilterRepo() error {
	var py strings.Builder
	py.WriteString("import re\n")
	py.WriteString("patterns=[\n")
	for _, p := range CommitMessagePatterns {
		py.WriteString("    re.compile(rb'(?i)" + escapeForPythonRawBytes(p) + "'),\n")
	}
	py.WriteString("]\n")
	py.WriteString("text = message.decode('utf-8', 'replace')\n")
	py.WriteString("out_lines = []\n")
	py.WriteString("for line in text.split('\\n'):\n")
	py.WriteString("    drop = False\n")
	py.WriteString("    for p in patterns:\n")
	py.WriteString("        if p.search(line.encode('utf-8')):\n")
	py.WriteString("            drop = True; break\n")
	py.WriteString("    if not drop:\n")
	py.WriteString("        out_lines.append(line)\n")
	py.WriteString("while out_lines and out_lines[-1].strip() == '':\n")
	py.WriteString("    out_lines.pop()\n")
	py.WriteString("out_lines.append('')\n")
	py.WriteString("return '\\n'.join(out_lines).encode('utf-8')\n")

	cmd := exec.Command("git", "-C", c.repoDir, "filter-repo",
		"--force",
		"--message-callback", py.String(),
	)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git filter-repo failed: %w\nStderr: %s", err, stderr.String())
	}
	return nil
}

// escapeForPythonRawBytes escapes single quotes so the regex string can sit
// inside rb'...'. Backslashes are already raw-byte safe.
func escapeForPythonRawBytes(s string) string {
	return strings.ReplaceAll(s, `'`, `\'`)
}

func (c *Cleaner) rewriteWithFilterBranch() error {
	script, err := c.createFilterBranchScript()
	if err != nil {
		return err
	}
	defer os.Remove(script)

	cmd := exec.Command("git", "-C", c.repoDir, "filter-branch", "-f",
		"--msg-filter", "sh "+script,
		"--", "--all",
	)
	cmd.Env = append(os.Environ(), "FILTER_BRANCH_SQUELCH_WARNING=1")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("git filter-branch failed: %w\nStderr: %s", err, stderr.String())
	}
	return nil
}

// createFilterBranchScript writes a temporary sed-based message filter and
// returns its path. The patterns it emits parallel CommitMessagePatterns but
// use ERE syntax portable across the sed implementations shipped with macOS and
// Linux. The caller is responsible for removing the file.
func (c *Cleaner) createFilterBranchScript() (string, error) {
	tmp, err := os.CreateTemp("", "unclaude-filter-*.sh")
	if err != nil {
		return "", fmt.Errorf("failed to create filter script: %w", err)
	}
	defer tmp.Close()

	// Build a sed pipeline. Each pattern becomes `/.../Id` (delete matching lines, case-insensitive).
	var b strings.Builder
	b.WriteString("#!/bin/sh\ncat | sed -E")
	for _, p := range filterBranchSedPatterns {
		b.WriteString(" -e '/")
		b.WriteString(p)
		b.WriteString("/Id'")
	}
	b.WriteString("\n")

	if _, err := tmp.WriteString(b.String()); err != nil {
		return "", fmt.Errorf("failed to write filter script: %w", err)
	}
	if err := tmp.Chmod(0o755); err != nil {
		return "", fmt.Errorf("failed to chmod filter script: %w", err)
	}
	return tmp.Name(), nil
}

// filterBranchSedPatterns is the sed-flavour mirror of CommitMessagePatterns.
// Kept hand-written because sed's ERE doesn't accept all Go regex constructs
// (no `(?i)` - handled by the `I` flag on each address instead).
var filterBranchSedPatterns = []string{
	`Co-Authored-By:.*@anthropic\.com`,
	`Co-Authored-By:.*[Cc]laude`,
	`Co-Authored-By:.*@openai\.com`,
	`Co-Authored-By:.*[Cc]odex`,
	`Co-Authored-By:.*[Cc]hat[Gg]PT`,
	`Generated with.*Claude`,
	`Generated with.*Codex`,
	`Generated with.*ChatGPT`,
	`\[Claude( Code)?\]`,
	`\(Claude( Code)?\)`,
	`\[Codex( CLI)?\]`,
	`\(Codex( CLI)?\)`,
	`Created (with|by).*Claude`,
	`Built (with|by).*Claude`,
	`Assisted by.*Claude`,
	`Created (with|by).*Codex`,
	`Built (with|by).*Codex`,
	`Assisted by.*Codex`,
	`claude\.com`,
	`claude\.ai`,
	`anthropic\.com`,
	`chatgpt\.com`,
	`chat\.openai\.com`,
	`openai\.com`,
	`[🤖🔧✨].*([Gg]enerated|[Cc]reated|[Bb]uilt|[Pp]owered)`,
	`[🤖🔧✨].*([Cc]laude|[Aa]nthropic|[Cc]odex|[Oo]pen[Aa][Ii])`,
	`AI[-[:space:]]+(assisted|generated|created|powered)`,
	`(Generated|Created|Built|Powered)[[:space:]]+(with|by)[[:space:]]+AI`,
	`-[[:space:]]*(Claude|Anthropic|Codex|ChatGPT|OpenAI)([[:space:]]|$)`,
}

func (c *Cleaner) purgeBackupRefs() {
	refsCmd := exec.Command("git", "-C", c.repoDir, "for-each-ref", "--format=%(refname)", "refs/original/")
	if out, err := refsCmd.Output(); err == nil && len(out) > 0 {
		for _, ref := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if ref = strings.TrimSpace(ref); ref != "" {
				exec.Command("git", "-C", c.repoDir, "update-ref", "-d", ref).Run()
			}
		}
	}
	exec.Command("git", "-C", c.repoDir, "reflog", "expire", "--expire=now", "--all").Run()
	exec.Command("git", "-C", c.repoDir, "gc", "--prune=now", "--aggressive").Run()
}

// cleanCommitMessage strips lines matching any of patterns and removes
// trailing blank lines. Used both for detection and tests.
func cleanCommitMessage(msg string, patterns []*regexp.Regexp) string {
	lines := strings.Split(msg, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		drop := false
		for _, p := range patterns {
			if p.MatchString(line) {
				drop = true
				break
			}
		}
		if !drop {
			out = append(out, line)
		}
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n") + "\n"
}

// normalizeMessage strips trailing blank lines and re-terminates with one newline.
func normalizeMessage(msg string) string {
	lines := strings.Split(msg, "\n")
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	return strings.Join(lines, "\n") + "\n"
}
