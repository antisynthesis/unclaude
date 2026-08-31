package cleaner

import (
	"regexp"
	"strings"
)

// commentSyntax describes how comments are written in a given language family.
type commentSyntax struct {
	lineToken   string // e.g. "//" or "#"; empty if no line comments
	blockOpen   string // e.g. "/*"; empty if no block comments
	blockClose  string // e.g. "*/"
	docOpen     string // python triple-quoted strings (treated as docstring-comments)
	docClose    string
	htmlComment bool // also strip <!-- --> blocks
}

var commentSyntaxByExt = map[string]commentSyntax{
	".go":   {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".js":   {lineToken: "//", blockOpen: "/*", blockClose: "*/", htmlComment: true},
	".jsx":  {lineToken: "//", blockOpen: "/*", blockClose: "*/", htmlComment: true},
	".ts":   {lineToken: "//", blockOpen: "/*", blockClose: "*/", htmlComment: true},
	".tsx":  {lineToken: "//", blockOpen: "/*", blockClose: "*/", htmlComment: true},
	".java": {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".c":    {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".cpp":  {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".h":    {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".hpp":  {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".rs":   {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".cs":   {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".php":  {lineToken: "//", blockOpen: "/*", blockClose: "*/"},
	".py":   {lineToken: "#", docOpen: `"""`, docClose: `"""`},
	".rb":   {lineToken: "#"},
}

// CleanCommentsInSource rewrites src by removing comments whose text matches
// the AI-token pattern.
//
// Behavior matrix:
//   - Standalone single-line comment (only whitespace before //): the entire
//     line is dropped.
//   - End-of-line comment (code before //): only the comment is stripped;
//     trailing whitespace on the code is trimmed; the code itself is kept.
//   - Multi-line block comment (/* ... */, """ ... """, <!-- ... -->): the
//     entire span is removed, including delimiters. If the open and close sit
//     on lines that become blank, those lines are dropped too.
//   - Consecutive blank lines produced by removals are collapsed to one.
func CleanCommentsInSource(src []byte, ext string, aiPattern *regexp.Regexp) ([]byte, bool) {
	syntax, ok := commentSyntaxByExt[strings.ToLower(ext)]
	if !ok {
		return src, false
	}

	text := string(src)
	modified := false

	if syntax.blockOpen != "" {
		var changed bool
		text, changed = stripMatchingBlocks(text, syntax.blockOpen, syntax.blockClose, aiPattern)
		modified = modified || changed
	}
	if syntax.docOpen != "" {
		var changed bool
		text, changed = stripMatchingBlocks(text, syntax.docOpen, syntax.docClose, aiPattern)
		modified = modified || changed
	}
	if syntax.htmlComment {
		var changed bool
		text, changed = stripMatchingBlocks(text, "<!--", "-->", aiPattern)
		modified = modified || changed
	}

	if syntax.lineToken != "" {
		var changed bool
		text, changed = stripMatchingLineComments(text, syntax.lineToken, aiPattern)
		modified = modified || changed
	}

	if modified {
		text = collapseBlankLines(text)
	}

	return []byte(text), modified
}

// stripMatchingBlocks removes block-comment spans whose interior text matches
// aiPattern. Spans are located by literal scanning for open/close - this does
// not parse strings, so a "/*" inside a quoted literal would be misread, but
// that's a rare false positive and the same limitation the original code had.
func stripMatchingBlocks(text, open, close string, aiPattern *regexp.Regexp) (string, bool) {
	var out strings.Builder
	out.Grow(len(text))

	modified := false
	i := 0
	for i < len(text) {
		start := strings.Index(text[i:], open)
		if start < 0 {
			out.WriteString(text[i:])
			break
		}
		start += i

		end := strings.Index(text[start+len(open):], close)
		if end < 0 {
			// Unterminated - leave the rest alone.
			out.WriteString(text[i:])
			break
		}
		end = start + len(open) + end + len(close)

		block := text[start:end]
		if !aiPattern.MatchString(block) {
			out.WriteString(text[i:end])
			i = end
			continue
		}

		modified = true
		// Trim leading whitespace on the start line up to the block,
		// and the trailing newline after the block if the close-line is now empty.
		lineStart := strings.LastIndexByte(text[i:start], '\n')
		if lineStart < 0 {
			lineStart = i
		} else {
			lineStart = i + lineStart + 1
		}
		leading := text[lineStart:start]
		if strings.TrimSpace(leading) == "" {
			// Standalone block - flush up to lineStart and skip past the newline after close.
			out.WriteString(text[i:lineStart])
			j := end
			if j < len(text) && text[j] == '\n' {
				j++
			}
			i = j
		} else {
			// Inline block - keep code before, drop block, keep what follows.
			out.WriteString(text[i:start])
			// Strip trailing whitespace just added.
			trimTrailingHSpace(&out)
			i = end
		}
	}

	return out.String(), modified
}

// stripMatchingLineComments handles `//` and `#` style line comments.
// Standalone matched comments cause the line to be dropped entirely.
// Inline matched comments are stripped leaving the code behind.
func stripMatchingLineComments(text, token string, aiPattern *regexp.Regexp) (string, bool) {
	lines := strings.Split(text, "\n")
	modified := false
	out := make([]string, 0, len(lines))

	for _, line := range lines {
		idx := findCommentStart(line, token)
		if idx < 0 {
			out = append(out, line)
			continue
		}

		commentText := line[idx:]
		if !aiPattern.MatchString(commentText) {
			out = append(out, line)
			continue
		}

		modified = true
		before := line[:idx]
		if strings.TrimSpace(before) == "" {
			// Standalone - drop the whole line.
			continue
		}
		// Inline - keep the code, trim trailing whitespace.
		out = append(out, strings.TrimRight(before, " \t"))
	}

	return strings.Join(out, "\n"), modified
}

// findCommentStart returns the byte offset of `token` outside of string
// literals on a single line. Returns -1 if not found.
//
// Handles "..." and '...' literals and (for languages that have them)
// `...` literals; treats backslash as an escape inside double-quoted strings.
// This is a heuristic - sufficient for stripping comments cleanly in the
// common case, without pulling in a full per-language tokenizer.
func findCommentStart(line, token string) int {
	tlen := len(token)
	if tlen == 0 {
		return -1
	}

	var quote byte
	for i := 0; i < len(line); i++ {
		c := line[i]
		if quote != 0 {
			if c == '\\' && quote != '`' && i+1 < len(line) {
				i++
				continue
			}
			if c == quote {
				quote = 0
			}
			continue
		}
		switch c {
		case '"', '\'', '`':
			quote = c
			continue
		}
		if i+tlen <= len(line) && line[i:i+tlen] == token {
			return i
		}
	}
	return -1
}

func trimTrailingHSpace(b *strings.Builder) {
	s := b.String()
	end := len(s)
	for end > 0 {
		c := s[end-1]
		if c != ' ' && c != '\t' {
			break
		}
		end--
	}
	if end == len(s) {
		return
	}
	b.Reset()
	b.WriteString(s[:end])
}

// collapseBlankLines collapses runs of 2+ blank lines into a single blank line.
// Preserves a single trailing newline at end-of-file.
func collapseBlankLines(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	blankRun := 0
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			blankRun++
			if blankRun > 1 {
				continue
			}
		} else {
			blankRun = 0
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}
