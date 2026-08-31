package cleaner

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Watermark handling.
//
// Modern LLM output carries two very different kinds of "watermark":
//
//  1. Invisible / zero-width characters embedded in the text. These include
//     zero-width spaces and joiners, directional (bidi) controls, the Unicode
//     Tags block, variation selectors, and invisible math operators. They are
//     used both as crude provenance marks and as data-smuggling / prompt-
//     injection vectors ("ASCII smuggling"). They carry no visible meaning, so
//     they can be removed losslessly -- StripInvisible does this.
//
//  2. A statistical watermark (Anthropic ships a variant of Google DeepMind's
//     SynthID-Text). This is a bias in *which words the model chose*, not any
//     hidden character. It cannot be removed by rewriting bytes and this tool
//     makes no attempt to -- see the README.
//
// The visible typography an assistant tends to emit -- em dashes, "smart"
// quotes, ellipsis glyphs -- is not a watermark, but it is a recognisable tell.
// NormalizeTypography folds those back to plain ASCII. It edits legitimately
// authored, visible content, so it is opt-in and applied to prose only.
//
// Codepoints are written as numeric (hex) values throughout: the characters
// this file deals in are by definition invisible, and spelling them literally
// would make the source unreviewable and prone to duplicate-entry bugs.

const (
	// bom is the byte-order mark (U+FEFF). Kept when it is the first rune of a
	// file, deleted anywhere else (where it acts as a zero-width no-break space).
	bom = 0xFEFF
	// lineSep and paraSep are the Unicode line/paragraph separators, normalized
	// to a plain newline rather than deleted.
	lineSep = 0x2028
	paraSep = 0x2029
)

// exoticSpaces are Unicode space characters normalized to a plain ASCII space.
// Deleting them would fuse adjacent words, so they are replaced, not dropped.
// The narrow no-break space (U+202F) in particular is a common assistant tell
// in otherwise-ASCII prose. Regular space, tab, newline and CR are absent by
// design.
var exoticSpaces = map[rune]bool{
	0x00A0: true, // no-break space
	0x1680: true, // ogham space mark
	0x2000: true, // en quad
	0x2001: true, // em quad
	0x2002: true, // en space
	0x2003: true, // em space
	0x2004: true, // three-per-em space
	0x2005: true, // four-per-em space
	0x2006: true, // six-per-em space
	0x2007: true, // figure space
	0x2008: true, // punctuation space
	0x2009: true, // thin space
	0x200A: true, // hair space
	0x202F: true, // narrow no-break space
	0x205F: true, // medium mathematical space
	0x3000: true, // ideographic space
}

// invisibleRunes are format/zero-width characters that render no glyph and are
// safe to delete outright.
var invisibleRunes = map[rune]bool{
	0x00AD: true, // soft hyphen
	0x034F: true, // combining grapheme joiner
	0x061C: true, // arabic letter mark
	0x115F: true, // hangul choseong filler
	0x1160: true, // hangul jungseong filler
	0x17B4: true, // khmer vowel inherent aq
	0x17B5: true, // khmer vowel inherent aa
	0x180E: true, // mongolian vowel separator
	0x200B: true, // zero width space
	0x200C: true, // zero width non-joiner
	0x200D: true, // zero width joiner
	0x200E: true, // left-to-right mark
	0x200F: true, // right-to-left mark
	0x2060: true, // word joiner
	0x2061: true, // function application
	0x2062: true, // invisible times
	0x2063: true, // invisible separator
	0x2064: true, // invisible plus
	0x3164: true, // hangul filler
	0xFEFF: true, // zero width no-break space (mid-file BOM)
	0xFFA0: true, // halfwidth hangul filler
	0xFFF9: true, // interlinear annotation anchor
	0xFFFA: true, // interlinear annotation separator
	0xFFFB: true, // interlinear annotation terminator
}

// isInvisibleRune reports whether r renders no visible glyph and carries no
// meaning worth preserving in source or prose -- i.e. it is safe to delete.
//
// This deliberately includes zero-width joiners and variation selectors even
// though they have legitimate uses in emoji sequences and in some scripts:
// they are primary data-smuggling and watermark vectors, and their legitimate
// appearance in a source repository is rare. Preview mode (the default) shows
// exactly what would change before anything is written.
func isInvisibleRune(r rune) bool {
	if invisibleRunes[r] {
		return true
	}
	switch {
	case r >= 0x202A && r <= 0x202E: // bidi embeddings and overrides
		return true
	case r >= 0x2066 && r <= 0x206F: // bidi isolates + deprecated format controls
		return true
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors 1-16
		return true
	case r >= 0xE0000 && r <= 0xE007F: // Unicode Tags block (ASCII smuggling)
		return true
	case r >= 0xE0100 && r <= 0xE01EF: // variation selectors supplement
		return true
	}
	return false
}

// StripInvisible removes invisible/zero-width watermark and smuggling
// characters from content and normalizes exotic whitespace to ASCII space.
// A byte-order mark is preserved only when it is the first rune of the file.
// Line (U+2028) and paragraph (U+2029) separators become a plain newline.
// It returns the original slice unchanged (and false) when nothing matched.
func StripInvisible(content []byte) ([]byte, bool) {
	// Every rune this function touches is non-ASCII, so pure-ASCII content
	// (the common case for source code) can never change. Skip the allocation.
	if isASCII(content) {
		return content, false
	}

	s := string(content)
	var b strings.Builder
	b.Grow(len(s))

	modified := false
	first := true
	for _, r := range s {
		if first {
			first = false
			if r == bom { // legitimate leading BOM
				b.WriteRune(r)
				continue
			}
		}
		if r == lineSep || r == paraSep {
			b.WriteByte('\n')
			modified = true
			continue
		}
		if exoticSpaces[r] {
			b.WriteByte(' ')
			modified = true
			continue
		}
		if isInvisibleRune(r) {
			modified = true
			continue
		}
		b.WriteRune(r)
	}

	if !modified {
		return content, false
	}
	return []byte(b.String()), true
}

// typographyReplacements maps visible "smart" typography to its plain-ASCII
// equivalent. These characters are legitimate -- this is a stylistic
// normalization, not watermark removal -- which is why NormalizeTypography is
// opt-in and scoped to prose.
var typographyReplacements = map[rune]string{
	0x2013: "-",   // en dash
	0x2014: "--",  // em dash
	0x2015: "--",  // horizontal bar
	0x2018: "'",   // left single quotation mark
	0x2019: "'",   // right single quotation mark
	0x201A: "'",   // single low-9 quotation mark
	0x201B: "'",   // single high-reversed-9 quotation mark
	0x201C: "\"",  // left double quotation mark
	0x201D: "\"",  // right double quotation mark
	0x201E: "\"",  // double low-9 quotation mark
	0x201F: "\"",  // double high-reversed-9 quotation mark
	0x2026: "...", // horizontal ellipsis
	0x2032: "'",   // prime
	0x2033: "\"",  // double prime
	0x2212: "-",   // minus sign
}

// NormalizeTypography folds "smart" punctuation (curly quotes, en/em dashes,
// ellipsis glyphs, primes, the minus sign) back to plain ASCII. It returns the
// original slice unchanged (and false) when nothing matched.
func NormalizeTypography(content []byte) ([]byte, bool) {
	// Every replaced rune is non-ASCII; pure-ASCII content can never change.
	if isASCII(content) {
		return content, false
	}

	s := string(content)
	var b strings.Builder
	b.Grow(len(s))

	modified := false
	for _, r := range s {
		if repl, ok := typographyReplacements[r]; ok {
			b.WriteString(repl)
			modified = true
			continue
		}
		b.WriteRune(r)
	}

	if !modified {
		return content, false
	}
	return []byte(b.String()), true
}

// isASCII reports whether content contains only bytes below 0x80. Because every
// rune the watermark functions delete or replace is non-ASCII, ASCII-only input
// is a guaranteed no-op and can bypass the rune-by-rune rewrite entirely.
func isASCII(content []byte) bool {
	for _, b := range content {
		if b >= 0x80 {
			return false
		}
	}
	return true
}

// looksBinary reports whether content should be left untouched by watermark
// scrubbing: any NUL byte, or invalid UTF-8, marks it as non-text.
func looksBinary(content []byte) bool {
	if bytes.IndexByte(content, 0) >= 0 {
		return true
	}
	return !utf8.Valid(content)
}
