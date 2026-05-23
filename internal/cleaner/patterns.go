package cleaner

import "regexp"

// CommitMessagePatterns are RE2/PCRE-compatible regex strings matched against
// individual lines of a commit message. Any matching line is removed.
//
// These are kept as plain strings (rather than pre-compiled) so the same list
// can be emitted into the inline Python passed to `git filter-repo
// --message-callback`. Syntax used here must be safe in both Go's regexp and
// Python's re; both implementations apply IgnoreCase + Multiline at scan time.
var CommitMessagePatterns = []string{
	// --- Claude / Anthropic ---
	`Co-Authored-By:\s*Claude\s*<.*@anthropic\.com>`,
	`Co-Authored-By:.*@anthropic\.com`,
	`Co-Authored-By:.*\bclaude\b.*`,
	`.*Generated with.*Claude.*`,
	`.*\[Claude( Code)?\].*`,
	`.*\(Claude( Code)?\).*`,
	`.*Created (with|by).*Claude.*`,
	`.*Built (with|by).*Claude.*`,
	`.*Assisted by.*Claude.*`,
	`.*claude\.com.*`,
	`.*claude\.ai.*`,
	`.*anthropic\.com.*`,

	// --- Codex / ChatGPT / OpenAI ---
	`Co-Authored-By:\s*Codex\s*<.*>`,
	`Co-Authored-By:\s*ChatGPT\s*<.*>`,
	`Co-Authored-By:.*@openai\.com`,
	`Co-Authored-By:.*\bcodex\b.*`,
	`Co-Authored-By:.*\bchatgpt\b.*`,
	`.*Generated with.*Codex.*`,
	`.*Generated with.*ChatGPT.*`,
	`.*\[Codex( CLI)?\].*`,
	`.*\(Codex( CLI)?\).*`,
	`.*Created (with|by).*Codex.*`,
	`.*Built (with|by).*Codex.*`,
	`.*Assisted by.*Codex.*`,
	`.*chatgpt\.com.*`,
	`.*chat\.openai\.com.*`,
	`.*openai\.com.*`,

	// --- Emoji badges ---
	`[🤖🔧✨].*\b(generated|created|built|powered).*`,
	`🤖.*Claude.*`,
	`🤖.*Anthropic.*`,
	`🤖.*Codex.*`,
	`🤖.*OpenAI.*`,

	// --- Generic AI markers ---
	`.*\bAI[-\s]+(assisted|generated|created|powered)\b.*`,
	`.*(Generated|Created|Built|Powered)\s+(with|by)\s+AI\b.*`,

	// --- Trailing signatures ---
	`.*-\s*(Claude|Anthropic|Codex|ChatGPT|OpenAI)(\s|$).*`,
}

// CompiledCommitMessagePatterns returns the patterns above compiled with
// case-insensitive matching, ready for in-process scanning.
func CompiledCommitMessagePatterns() []*regexp.Regexp {
	out := make([]*regexp.Regexp, 0, len(CommitMessagePatterns))
	for _, p := range CommitMessagePatterns {
		out = append(out, regexp.MustCompile(`(?i)`+p))
	}
	return out
}

// CommentAITokens are substrings (matched case-insensitively, with word
// boundaries where applicable) that mark a comment as AI-related. A comment
// is removed if any token matches anywhere in its text.
var CommentAITokens = []string{
	`\bclaude\b`,
	`\banthropic\b`,
	`claude\.ai`,
	`claude\.com`,
	`anthropic\.com`,
	`\bcodex\b`,
	`\bchatgpt\b`,
	`\bopenai\b`,
	`openai\.com`,
	`chatgpt\.com`,
	`\bai[-\s]+(assisted|generated|created|powered)\b`,
	`\b(generated|created|built|powered)\s+(with|by)\s+(ai|claude|codex|chatgpt|openai|anthropic)\b`,
	`\b(generated|assisted)\s+(with|by)\b`,
}

// CompiledCommentAIPattern returns a single regex that matches any AI-related
// token in comment text. Case-insensitive.
func CompiledCommentAIPattern() *regexp.Regexp {
	alt := ""
	for i, t := range CommentAITokens {
		if i > 0 {
			alt += "|"
		}
		alt += "(?:" + t + ")"
	}
	return regexp.MustCompile(`(?i)` + alt)
}
