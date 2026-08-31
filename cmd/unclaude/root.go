package main

import (
	"flag"
	"fmt"
	"io"
	"log/slog"
	"strings"

	"github.com/antisynthesis/unclaude/internal/cleaner"
)

// exit codes returned by run.
const (
	exitOK      = 0
	exitFailure = 1
	exitUsage   = 2
)

// usage renders the top-level help text. Step keys come from the cleaner so the
// list cannot drift from the steps that actually run.
func usage() string {
	return `unclaude - return your repository to human hands

Usage:
  unclaude [flags] [path]
  unclaude restore [flags] [path]

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

Steps (for --only and --skip):
` + cleaner.StepHelp() + `
Flags:
      --apply               apply changes (default is preview/dry-run mode)
      --only <steps>        run only these comma-separated steps
      --skip <steps>        run every step except these
      --no-backup           do not save originals before modifying or deleting
  -i, --interactive         prompt before deleting each markdown file (requires --apply)
  -v, --verbose             show debug-level output
      --quiet               suppress info-level output; only warnings and errors
      --json                emit logs as JSON instead of human-readable text
      --purge-refs          after history rewrite, delete refs/original/ and run aggressive gc
      --normalize-typography  fold smart quotes, em/en dashes, ellipsis glyphs in prose to ASCII
  -h, --help                display this help

In apply mode unclaude copies every file it modifies or deletes into
.unclaude-backup/ first. Undo the most recent run with:

  unclaude restore --apply [path]

Restore covers file changes only. To undo a history rewrite, use git's
refs/original/ backup refs or the reflog.
`
}

// run parses args, wires up the cleaner, and executes the selected steps. Log
// records go to stdout; usage and errors to stderr. It returns a process exit
// code so main stays a one-liner and the CLI is testable without touching
// os.Exit or global flag state.
func run(args []string, stdout, stderr io.Writer) int {
	if len(args) > 0 && args[0] == "restore" {
		return runRestore(args[1:], stdout, stderr)
	}

	fs := flag.NewFlagSet("unclaude", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { fmt.Fprint(stderr, usage()) }

	var (
		apply         bool
		verbose       bool
		quiet         bool
		jsonLog       bool
		interactive   bool
		purgeRefs     bool
		normalizeType bool
		noBackup      bool
		only          string
		skip          string
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
	fs.BoolVar(&noBackup, "no-backup", false, "do not save originals before modifying or deleting")
	fs.StringVar(&only, "only", "", "run only these comma-separated steps")
	fs.StringVar(&skip, "skip", "", "run every step except these")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	repoDir, ok := repoArg(fs, stderr)
	if !ok {
		return exitUsage
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

	// Validate the step selection before touching the filesystem so a typo is
	// reported as a usage error rather than masked by an environment failure.
	onlySteps, skipSteps := cleaner.ParseStepList(only), cleaner.ParseStepList(skip)
	if _, err := cleaner.SelectSteps(new(cleaner.Cleaner).Steps(), onlySteps, skipSteps); err != nil {
		fmt.Fprintf(stderr, "unclaude: %v\n", err)
		return exitUsage
	}

	if !cleaner.IsGitRepo(repoDir) {
		fmt.Fprintf(stderr, "unclaude: %s is not a git repository\n", repoDir)
		return exitFailure
	}

	log := newLogger(verbose, quiet, jsonLog, stdout)

	c := cleaner.New(repoDir, cleaner.Options{
		DryRun:              dryRun,
		Interactive:         interactive,
		PurgeRefs:           purgeRefs,
		NormalizeTypography: normalizeType,
		Backup:              !noBackup,
		Logger:              log,
	})

	steps, err := cleaner.SelectSteps(c.Steps(), onlySteps, skipSteps)
	if err != nil {
		fmt.Fprintf(stderr, "unclaude: %v\n", err)
		return exitUsage
	}

	if dryRun {
		log.Info("preview mode (no changes will be made); use --apply to modify the repository")
	} else {
		log.Info("apply mode (changes will be made)")
	}
	if only != "" || skip != "" {
		log.Info("running selected steps", slog.String("steps", strings.Join(stepKeys(steps), ",")))
	}

	for _, s := range steps {
		if err := s.Run(); err != nil {
			log.Error("step failed", slog.String("step", s.Key), slog.Any("error", err))
			reportBackup(c, log)
			return exitFailure
		}
	}
	reportBackup(c, log)

	if dryRun {
		log.Info("preview complete; no changes were made (run with --apply to make permanent)")
	} else {
		log.Info("repository cleaned")
	}
	return exitOK
}

// runRestore implements the `unclaude restore` subcommand.
func runRestore(args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("unclaude restore", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() {
		fmt.Fprint(stderr, `unclaude restore - undo the most recent unclaude run

Usage:
  unclaude restore [flags] [path]

Copies every file preserved in the newest .unclaude-backup/ entry back to its
original location, recreating deleted files and reverting modified ones.
Runs in preview mode by default; use --apply to write the restore.

Restore covers file changes only. To undo a history rewrite, use git's
refs/original/ backup refs or the reflog.

Flags:
      --apply     perform the restore (default is preview/dry-run mode)
  -v, --verbose   show debug-level output
      --quiet     suppress info-level output; only warnings and errors
      --json      emit logs as JSON instead of human-readable text
  -h, --help      display this help
`)
	}

	var apply, verbose, quiet, jsonLog bool
	fs.BoolVar(&apply, "apply", false, "perform the restore")
	fs.BoolVar(&verbose, "verbose", false, "show debug-level output")
	fs.BoolVar(&verbose, "v", false, "show debug-level output (shorthand)")
	fs.BoolVar(&quiet, "quiet", false, "suppress info-level output")
	fs.BoolVar(&jsonLog, "json", false, "emit logs as JSON")

	if err := fs.Parse(args); err != nil {
		if err == flag.ErrHelp {
			return exitOK
		}
		return exitUsage
	}
	repoDir, ok := repoArg(fs, stderr)
	if !ok {
		return exitUsage
	}
	if quiet && verbose {
		fmt.Fprintln(stderr, "unclaude: --quiet and --verbose are mutually exclusive")
		return exitUsage
	}

	log := newLogger(verbose, quiet, jsonLog, stdout)
	if err := cleaner.Restore(repoDir, cleaner.RestoreOptions{DryRun: !apply, Logger: log}); err != nil {
		fmt.Fprintf(stderr, "unclaude: %v\n", err)
		return exitFailure
	}
	return exitOK
}

// repoArg extracts the optional path operand, defaulting to the current
// directory. It reports a usage error when more than one path is given.
func repoArg(fs *flag.FlagSet, stderr io.Writer) (string, bool) {
	switch fs.NArg() {
	case 0:
		return ".", true
	case 1:
		return fs.Arg(0), true
	default:
		fmt.Fprintf(stderr, "unclaude: expected at most one path, got %d\n", fs.NArg())
		return "", false
	}
}

func newLogger(verbose, quiet, jsonLog bool, stdout io.Writer) *slog.Logger {
	format := cleaner.LogFormatConsole
	if jsonLog {
		format = cleaner.LogFormatJSON
	}
	return cleaner.NewLogger(cleaner.LoggerOptions{
		Verbose: verbose,
		Quiet:   quiet,
		Format:  format,
		Output:  stdout,
	})
}

// reportBackup finalizes the backup and tells the user how to undo the run.
// A failure to write the manifest is logged rather than returned: the cleaning
// itself already succeeded, and the copies are on disk either way.
func reportBackup(c *cleaner.Cleaner, log *slog.Logger) {
	dir, err := c.FinalizeBackup()
	if err != nil {
		log.Warn("failed to finalize backup", slog.Any("error", err))
		return
	}
	if dir != "" {
		log.Info("originals saved; undo with 'unclaude restore --apply'", slog.String("backup", dir))
	}
}

func stepKeys(steps []cleaner.Step) []string {
	keys := make([]string, len(steps))
	for i, s := range steps {
		keys[i] = s.Key
	}
	return keys
}
