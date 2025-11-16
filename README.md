# unclaude

A utility that returns your repository to human hands.

Removes Claude Code's operational artifacts—.claude directories, metadata files, and automated traces—leaving only your intentional work behind. For when you need a clean slate, a handoff to another developer, or simply want your repository to reflect human authorship alone.

Not a rejection of AI assistance, but a choice about what remains.

Tools for the digital human experience.

## What Gets Removed

- `.claude/` directory and all contents (commands, skills, configurations)
- Markdown documentation files (excluding root README.md and docs/, doc/, adr/)
- Source code comments containing:
  - References to Claude, Claude Code, or Anthropic
  - AI assistance markers (AI-assisted, AI-generated, etc.)
  - Generated-with signatures
- Git commit history traces:
  - Co-authorship attributions (`Co-Authored-By: Claude`)
  - Generation footers and automation signatures
  - Links to claude.com or anthropic.com

## Installation

```bash
go install github.com/antisynthesis/unclaude@latest
```

Build from source:

```bash
git clone https://github.com/antisynthesis/unclaude.git
cd unclaude
make build
```

## Usage

Run within your repository:

```bash
unclaude
```

Specify a path:

```bash
unclaude /path/to/repo
```

### Options

```
-n, --dry-run       Preview changes without modification
-i, --interactive   Prompt before deleting each markdown file
-v, --verbose       Detailed operation output
-h, --help          Display usage information
```

### Examples

Preview before execution:
```bash
unclaude --dry-run
```

Interactive mode with confirmation prompts:
```bash
unclaude --interactive
```

Detailed operation log:
```bash
unclaude --verbose
```

Combined flags:
```bash
unclaude --interactive --verbose ~/projects/my-repo
```

**Important:** The tool scans commit history first. If AI traces are found, you'll be prompted to confirm before any destructive changes. If no traces are found, no prompt appears and git history is left untouched.

## Details

### `.claude/` Directory
Complete removal of Claude Code's configuration directory including:
- Custom slash commands
- Skill definitions
- Tool configurations
- Any metadata files

### Markdown Files
All `.md` files throughout the repository tree, with exceptions:
- Root `README.md` is preserved
- Documentation directories remain untouched: `docs/`, `doc/`, `adr/`
- Dependencies and version control: `.git/`, `node_modules/`, `vendor/`

Interactive mode (`--interactive`) prompts before each deletion:
- `y` or `yes` - Delete this file
- `n` or `no` (default) - Skip this file
- `a` or `all` - Delete this and all remaining files (no more prompts)

### Source Comments
Comprehensive pattern matching removes AI-related comments from:
- Go, JavaScript, TypeScript, JSX, TSX
- Python, Java, C, C++, Rust
- Ruby, PHP, C#

Patterns detect:
- Direct mentions: "Claude", "Claude Code", "Anthropic"
- AI markers: "AI-assisted", "AI-generated", "AI created"
- Generation tags: "Generated with", "Assisted by"

### Commit History
Git history is rewritten using `filter-branch` to remove:
- Co-authorship lines (`Co-Authored-By: Claude <noreply@anthropic.com>`)
- Generation footers and emoji badges
- Tool attribution links (claude.com, anthropic.com)
- AI assistance markers

**Safety measures:**
- Scans commit history first to detect AI traces
- Only prompts for confirmation if changes are actually needed
- Checks for unstaged changes and aborts if found
- Displays clear warning about permanent, destructive nature
- Dry-run mode shows what would be changed without prompting

The tool will NOT prompt or attempt rewriting if no AI traces are found in your commit history. History rewriting is permanent and destructive. Always use `--dry-run` first to preview changes.

## Requirements

- Go 1.21+
- Git (for history operations)

## Development

Tests:
```bash
go test ./... -v
```

Structure:
```
cmd/unclaude/       Command interface
internal/cleaner/   Core operations
```

## License

MIT

## Contributing

Contributions accepted. Maintain test coverage and idiomatic Go.
