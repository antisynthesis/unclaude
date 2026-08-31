![unclaude](img/unclaude-social.png)

# unclaude

A utility that returns your repository to human hands.

Removes traces left behind by AI coding assistants — Claude Code, Codex CLI, Cursor, Continue, Aider, GitHub Copilot — including tool directories, instruction files, source comments, and commit-message footers. The repository is left reflecting only your intentional work.

Not a rejection of AI assistance, but a choice about what remains.

**Safe by default:** Runs in preview mode unless you explicitly use `--apply`.

Tools for the digital human experience.

## What Gets Removed

**AI tool directories** (anywhere in the tree):
- `.claude/`, `.codex/`, `.cursor/`, `.continue/`, `.aider/`

**AI tool files** (anywhere in the tree — these override the `docs/` allowlist):
- `CLAUDE.md`, `AGENTS.md`
- `.mcp.json`, `.claude.json`, `.claudeignore`
- `.cursorrules`, `.cursorignore`
- `.aider.conf.yml`, `.aider.input.history`, `.aider.chat.history.md`
- `.github/copilot-instructions.md`

**Markdown files** (with allowlist):
- Root `README.md` preserved
- `docs/`, `doc/`, `adr/` subtrees preserved (except AI tool files above)

**Source code comments** containing:
- Vendor mentions: Claude, Anthropic, Codex, ChatGPT, OpenAI
- AI assistance markers: AI-assisted, AI-generated, AI-created, AI-powered
- Generation tags: Generated/Created/Built with/by (AI|Claude|Codex|…)
- Session URLs: claude.ai, claude.com, anthropic.com, chatgpt.com, openai.com

Comments are removed correctly whether they're standalone (whole line dropped), end-of-line (only the comment stripped, code preserved), or multi-line block comments (`/* … */`, `""" … """`, `<!-- … -->`).

**Commit message footers**:
- `Co-Authored-By: Claude <…@anthropic.com>` and friends
- `Co-Authored-By: Codex|ChatGPT <…@openai.com>`
- Generation footers: `[Claude Code]`, `(Codex CLI)`, `Generated with …`
- Session permalinks: `claude.ai/code/session_…`, `chatgpt.com/share/…`
- Emoji badges (🤖, 🔧, ✨) with generation markers
- Trailing signatures (`- Claude`, `- Codex`, …)

**Watermark characters** (in every UTF-8 text file — code *and* prose):
- Zero-width & joiner characters: zero-width space (U+200B), ZWNJ (U+200C),
  ZWJ (U+200D), word joiner (U+2060), soft hyphen (U+00AD), BOM mid-file (U+FEFF)
- Invisible math operators: function application / invisible times / separator /
  plus (U+2061–U+2064) — a documented data-smuggling channel
- Directional (bidi) controls: LRM/RLM, embeddings, overrides, isolates
  (U+200E–U+200F, U+202A–U+202E, U+2066–U+206F)
- The Unicode Tags block (U+E0000–U+E007F) and variation selectors
  (U+FE00–U+FE0F, U+E0100–U+E01EF) — the vectors behind "ASCII smuggling"
- Exotic whitespace normalized to a plain space: narrow no-break space (U+202F),
  no-break space (U+00A0), en/em/thin/hair spaces, ideographic space, and more
- Line/paragraph separators (U+2028/U+2029) normalized to a newline

A leading byte-order mark is preserved; binary files are detected and skipped.

**Typography tells** (opt-in, prose files only, via `--normalize-typography`):
- Em/en dashes and the horizontal bar → `--` / `-`
- "Smart" single and double quotes → `'` and `"`
- Ellipsis glyph (…) → `...`, primes (′ ″) → `'` `"`, minus sign (−) → `-`

These are *visible, legitimately authored* characters, so folding them is off by
default — it is a stylistic normalization, not watermark removal.

### On statistical watermarks

Since August 2026, Anthropic applies a **statistical** watermark to Claude's
text output — a variant of Google DeepMind's [SynthID-Text](https://www.nature.com/articles/s41586-024-08025-4).
It is a bias in *which words the model chose*, not a hidden character, so it
survives copy-paste and **cannot be removed by rewriting bytes**. `unclaude`
does not attempt to; it removes the invisible-character marks above, which is a
different and fully removable class of watermark.

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

**By default, `unclaude` runs in preview mode** — showing what would be changed without modifying anything.

Preview your repository:
```bash
unclaude
```

Apply changes:
```bash
unclaude --apply
```

Specify a path:
```bash
unclaude --apply /path/to/repo
```

### Options

```
--apply                  Apply changes (default is preview mode)
-i, --interactive        Prompt before deleting each markdown file (requires --apply)
-v, --verbose            Show debug-level output
    --quiet              Suppress info-level output; only warnings and errors
    --json               Emit logs as JSON instead of human-readable console
    --purge-refs         After history rewrite, delete refs/original/ and run aggressive gc
    --normalize-typography  Fold smart quotes, em/en dashes, and ellipsis glyphs in prose to plain ASCII
-h, --help               Display usage information
```

### Output

`unclaude` logs to stdout using the standard library's [`log/slog`](https://pkg.go.dev/log/slog) with RFC3339 timestamps. The default is human-readable text; pass `--json` for structured logs (useful in CI pre-commit hooks).

Text (default):
```
time=2026-08-30T14:02:18.512-06:00 level=INFO msg="preview mode (no changes will be made); use --apply to modify the repository"
time=2026-08-30T14:02:18.512-06:00 level=INFO msg="removing ai tool directory" path=.claude
time=2026-08-30T14:02:18.513-06:00 level=INFO msg="removing ai tool file" path=CLAUDE.md
time=2026-08-30T14:02:18.513-06:00 level=INFO msg="markdown removal summary" removed=2
```

JSON (`--json`):
```json
{"time":"2026-08-30T14:02:18.512-06:00","level":"INFO","msg":"markdown removal summary","removed":2}
```

### Examples

Preview mode (default, safe):
```bash
unclaude
unclaude --verbose
unclaude --json    # machine-readable
```

Apply changes with interactive prompts:
```bash
unclaude --apply --interactive
```

Apply changes and purge git backup refs:
```bash
unclaude --apply --purge-refs
```

**Important:** Preview mode is the default. Use `--apply` only when you're ready to make permanent changes.

## Git History Rewriting

`unclaude` prefers [`git-filter-repo`](https://github.com/newren/git-filter-repo) when it is available on `PATH` — this is the rewriting tool the Git project recommends since `filter-branch` was deprecated. If `git-filter-repo` is not installed, `unclaude` falls back to `git filter-branch` and emits a warning.

After the rewrite, `unclaude` leaves the original refs under `refs/original/` by default so you can recover if something looks wrong. Pass `--purge-refs` to delete them and run `git gc --prune=now --aggressive` immediately.

**Safety measures:**
- Preview mode by default — no changes without `--apply`
- History rewrite runs first (while the tree is clean), then the file-modifying steps
- Aborts the run if the working tree has uncommitted changes (in apply mode) — commit or stash first
- Only prompts for confirmation if AI traces are actually present in history
- Backup refs preserved by default (`refs/original/`)
- Warns that signed commits will be invalidated by the rewrite

History rewriting is permanent and changes all commit hashes downstream of the rewritten commits. Coordinate with your team before running on a shared branch.

## Requirements

- Go 1.25+ (standard library only — **zero external dependencies**)
- Git (for history operations)
- Optional but recommended: `git-filter-repo` (for the non-deprecated rewrite path)

## Development

Tests and benchmarks:
```bash
go test ./... -race
go test ./internal/cleaner -bench=. -benchmem
```

Structure:
```
cmd/unclaude/             Command interface
    main.go               Entry point
    root.go               Flag parsing + step orchestration (stdlib flag)
internal/cleaner/         Core operations
    doc.go                Package overview
    cleaner.go            Orchestrator + struct
    logger.go             slog setup (RFC3339, text/JSON)
    patterns.go           Centralised regex definitions
    comments.go           Source-comment scrubbing (inline + multi-line)
    watermark.go          Invisible-character stripping + typography normalization
    history.go            Git history rewrite (filter-repo / filter-branch)
```

The CLI uses the standard library's `flag` package and `log/slog`; there are no
third-party dependencies. CI runs `gofmt`, `go vet`, `go build`, and
`go test -race` on every push and PR via `.github/workflows/ci.yml`.

## License

MIT

## Contributing

Contributions accepted. Maintain test coverage and idiomatic Go.
