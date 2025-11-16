package main

import (
	"fmt"
	"os"

	"github.com/antisynthesis/unclaude/internal/cleaner"
	"github.com/spf13/cobra"
)

var (
	apply       bool
	verbose     bool
	interactive bool
	repoDir     string
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "unclaude [path]",
	Short: "Remove all traces of Claude Code assistance from a git repository",
	Long: `unclaude removes all traces that Claude Code assisted in the development
of a git repository. This includes:
  - Removing .claude/ directory (commands, skills, configurations)
  - Removing .md files (preserving root README.md and docs/, doc/, adr/ directories)
  - Removing AI-related comments from source files (Claude, Anthropic, AI-assisted)
  - Removing Co-Authored-By and generation footers from git commits
  - Removing tool attribution links (claude.com, anthropic.com)

By default, runs in preview mode (dry-run). Use --apply to make actual changes.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVar(&apply, "apply", false, "apply changes (default is preview/dry-run mode)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show detailed output")
	rootCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "prompt before deleting each markdown file (requires --apply)")
}

func runClean(cmd *cobra.Command, args []string) error {
	// Determine repository directory
	if len(args) > 0 {
		repoDir = args[0]
	} else {
		var err error
		repoDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	// Verify it's a git repository
	if !cleaner.IsGitRepo(repoDir) {
		return fmt.Errorf("%s is not a git repository", repoDir)
	}

	// Default to dry-run unless --apply is specified
	dryRun := !apply

	// Show mode banner
	if dryRun {
		fmt.Println("Running in PREVIEW mode (no changes will be made)")
		fmt.Println("Use --apply to actually modify the repository")
		fmt.Println()
	} else {
		fmt.Println("Running in APPLY mode - changes will be made")
		fmt.Println()
	}

	// Interactive mode requires apply
	if interactive && dryRun {
		return fmt.Errorf("--interactive requires --apply flag")
	}

	c := cleaner.New(repoDir, dryRun, verbose, interactive)

	if err := c.CleanClaudeDirectory(); err != nil {
		return fmt.Errorf("failed to clean .claude directory: %w", err)
	}

	// Clean markdown files
	if err := c.CleanMarkdownFiles(); err != nil {
		return fmt.Errorf("failed to clean markdown files: %w", err)
	}

	// Clean source code comments
	if err := c.CleanSourceComments(); err != nil {
		return fmt.Errorf("failed to clean source comments: %w", err)
	}

	// Clean git commit history
	if err := c.CleanGitHistory(); err != nil {
		return fmt.Errorf("failed to clean git history: %w", err)
	}

	if dryRun {
		fmt.Println("\n✓ Preview completed. No changes were made.")
		fmt.Println("Run with --apply to make these changes permanent.")
	} else {
		fmt.Println("\n✓ Repository cleaned successfully.")
	}

	return nil
}
