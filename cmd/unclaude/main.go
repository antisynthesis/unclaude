// Command unclaude removes traces left behind by AI coding assistants from a
// git repository: tool directories and instruction files, AI-related source
// comments, invisible watermark characters in code and prose, and generation
// footers in commit messages. It runs in preview mode by default and only
// modifies the repository when invoked with --apply.
package main

import "os"

func main() {
	os.Exit(run(os.Args[1:], os.Stdout, os.Stderr))
}
