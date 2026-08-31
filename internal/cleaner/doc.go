// Package cleaner implements the repository-scrubbing operations behind the
// unclaude command. Each operation is a method on Cleaner and is safe to run in
// preview (dry-run) mode, where it reports what it would change without writing:
//
//   - CleanAIArtifacts removes AI-tool directories and instruction files.
//   - CleanMarkdownFiles removes generic Markdown outside a documentation allowlist.
//   - CleanSourceComments strips AI-related comments from source files.
//   - CleanWatermarks removes invisible watermark/smuggling characters (and,
//     optionally, normalizes visible "smart" typography in prose).
//   - CleanGitHistory rewrites commit messages to drop generation footers.
//
// Steps returns these operations as a keyed, ordered list so callers can run a
// subset (see SelectSteps). In apply mode the Cleaner copies every file it
// modifies or deletes into a timestamped .unclaude-backup directory; Restore
// reverses that, undoing a run's file changes.
//
// The package deliberately depends only on the Go standard library.
package cleaner
