package main

import (
	"fmt"
	"os"

	"github.com/antisynthesis/unclaude/internal/cleaner"
	"github.com/spf13/cobra"
	"go.uber.org/zap"
)

var (
	apply       bool
	verbose     bool
	quiet       bool
	jsonLog     bool
	interactive bool
	purgeRefs   bool
	repoDir     string
)

var rootCmd = &cobra.Command{
	Use:   "unclaude [path]",
	Short: "Remove AI coding-assistant traces from a git repository",
	Long: `unclaude removes traces left behind by AI coding assistants — Claude Code,
Codex CLI, Cursor, Continue, Aider, GitHub Copilot — from a git repository:

  - AI tool directories: .claude/, .codex/, .cursor/, .continue/, .aider/
  - AI tool files: CLAUDE.md, AGENTS.md, .mcp.json, .claude.json,
                   .claudeignore, .cursorrules, .cursorignore,
                   .aider.* files, .github/copilot-instructions.md
  - Generic .md files (preserving root README.md and docs/, doc/, adr/)
  - AI-related source comments (Claude, Anthropic, Codex, ChatGPT, OpenAI, AI-assisted)
  - Co-Authored-By / generation footers / session URLs from commit messages
    (claude.com, claude.ai, anthropic.com, chatgpt.com, openai.com)

Runs in preview mode by default. Use --apply to make changes.`,
	Args: cobra.MaximumNArgs(1),
	RunE: runClean,
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolVar(&apply, "apply", false, "apply changes (default is preview/dry-run mode)")
	rootCmd.Flags().BoolVarP(&verbose, "verbose", "v", false, "show debug-level output")
	rootCmd.Flags().BoolVar(&quiet, "quiet", false, "suppress info-level output; only warnings and errors")
	rootCmd.Flags().BoolVar(&jsonLog, "json", false, "emit logs as JSON instead of human-readable console")
	rootCmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "prompt before deleting each markdown file (requires --apply)")
	rootCmd.Flags().BoolVar(&purgeRefs, "purge-refs", false, "after history rewrite, delete refs/original/ and run aggressive gc")
}

func runClean(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		repoDir = args[0]
	} else {
		var err error
		repoDir, err = os.Getwd()
		if err != nil {
			return fmt.Errorf("failed to get current directory: %w", err)
		}
	}

	if !cleaner.IsGitRepo(repoDir) {
		return fmt.Errorf("%s is not a git repository", repoDir)
	}

	dryRun := !apply
	if interactive && dryRun {
		return fmt.Errorf("--interactive requires --apply flag")
	}

	format := cleaner.LogFormatConsole
	if jsonLog {
		format = cleaner.LogFormatJSON
	}
	log := cleaner.NewLogger(cleaner.LoggerOptions{
		Verbose: verbose,
		Quiet:   quiet,
		Format:  format,
	})
	defer func() { _ = log.Sync() }()

	if dryRun {
		log.Info("preview mode (no changes will be made); use --apply to modify the repository")
	} else {
		log.Info("apply mode (changes will be made)")
	}

	c := cleaner.New(repoDir, cleaner.Options{
		DryRun:      dryRun,
		Interactive: interactive,
		PurgeRefs:   purgeRefs,
		Logger:      log,
	})

	steps := []struct {
		name string
		fn   func() error
	}{
		{"clean ai tool artifacts", c.CleanAIArtifacts},
		{"clean markdown files", c.CleanMarkdownFiles},
		{"clean source comments", c.CleanSourceComments},
		{"clean git history", c.CleanGitHistory},
	}
	for _, s := range steps {
		if err := s.fn(); err != nil {
			log.Error("step failed", zap.String("step", s.name), zap.Error(err))
			return fmt.Errorf("%s: %w", s.name, err)
		}
	}

	if dryRun {
		log.Info("preview complete; no changes were made (run with --apply to make permanent)")
	} else {
		log.Info("repository cleaned")
	}
	return nil
}
