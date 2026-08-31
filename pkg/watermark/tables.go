package watermark

// Codepoints are written as numeric (hex) values throughout: the characters
// this package deals in are by definition invisible, and spelling them
// literally would make the source unreviewable and prone to duplicate entries.

const (
	// bom is the byte-order mark. It is kept when it is the first rune of the
	// input and treated as a zero-width no-break space anywhere else.
	bom = 0xFEFF
	// zwj is the zero-width joiner, which is load-bearing inside emoji
	// sequences and some scripts. See PreserveZWJ.
	zwj = 0x200D
	// lineSep and paraSep are the Unicode line and paragraph separators.
	lineSep = 0x2028
	paraSep = 0x2029
)

// exoticSpaces are Unicode space characters folded to a plain ASCII space.
// Deleting them would fuse adjacent words, so they are replaced rather than
// dropped. The narrow no-break space (U+202F) in particular is a common
// generated-text tell in otherwise-ASCII prose. Regular space, tab, newline and
// carriage return are absent by design.
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

// zeroWidth are format characters that render no glyph and carry no meaning
// worth preserving in source or prose.
var zeroWidth = map[rune]bool{
	0x00AD: true, // soft hyphen
	0x034F: true, // combining grapheme joiner
	0x115F: true, // hangul choseong filler
	0x1160: true, // hangul jungseong filler
	0x17B4: true, // khmer vowel inherent aq
	0x17B5: true, // khmer vowel inherent aa
	0x180E: true, // mongolian vowel separator
	0x200B: true, // zero width space
	0x200C: true, // zero width non-joiner
	0x200D: true, // zero width joiner
	0x2060: true, // word joiner
	0x3164: true, // hangul filler
	0xFEFF: true, // zero width no-break space (mid-text BOM)
	0xFFA0: true, // halfwidth hangul filler
	0xFFF9: true, // interlinear annotation anchor
	0xFFFA: true, // interlinear annotation separator
	0xFFFB: true, // interlinear annotation terminator
}

// bidiControls are directional-formatting characters. Beyond their use as
// provenance marks they are the mechanism behind Trojan Source
// (CVE-2021-42574), where an override makes source code render in an order that
// differs from how a compiler reads it.
var bidiControls = map[rune]bool{
	0x061C: true, // arabic letter mark
	0x200E: true, // left-to-right mark
	0x200F: true, // right-to-left mark
}

// invisibleMath are the invisible mathematical operators. They have been
// demonstrated as a binary encoding channel for smuggling data past reviewers
// and model guardrails.
var invisibleMath = map[rune]bool{
	0x2061: true, // function application
	0x2062: true, // invisible times
	0x2063: true, // invisible separator
	0x2064: true, // invisible plus
}

// typographyReplacements maps visible "smart" typography to its plain-ASCII
// equivalent. Unlike every other table here these characters are legitimate and
// visible, which is why folding them is opt-in (see WithTypography).
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

// inBidiRange reports whether r is a bidi embedding, override, or isolate.
func inBidiRange(r rune) bool {
	switch {
	case r >= 0x202A && r <= 0x202E: // embeddings and overrides
		return true
	case r >= 0x2066 && r <= 0x206F: // isolates and deprecated format controls
		return true
	}
	return false
}

// inTagRange reports whether r is in the Unicode Tags block, the channel used
// for ASCII smuggling: a full ASCII alphabet that renders as nothing.
func inTagRange(r rune) bool {
	return r >= 0xE0000 && r <= 0xE007F
}

// inVariationSelectorRange reports whether r is a variation selector. There are
// 256 of them, exactly enough to encode any byte, which makes them a practical
// steganographic carrier for arbitrary data.
func inVariationSelectorRange(r rune) bool {
	switch {
	case r >= 0xFE00 && r <= 0xFE0F: // variation selectors 1-16
		return true
	case r >= 0xE0100 && r <= 0xE01EF: // variation selectors supplement
		return true
	}
	return false
}
