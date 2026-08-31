package watermark

import (
	"bytes"
	"strings"
	"unicode/utf8"
)

// Category classifies a hidden or normalized character by why it matters.
type Category int

const (
	// ZeroWidth covers characters that render no glyph: zero-width spaces and
	// joiners, the word joiner, soft hyphens, and script-specific fillers.
	ZeroWidth Category = iota
	// Bidi covers directional-formatting controls. These are the mechanism
	// behind Trojan Source (CVE-2021-42574).
	Bidi
	// Tag covers the Unicode Tags block (U+E0000-U+E007F), an invisible ASCII
	// alphabet used to smuggle instructions and data past human review.
	Tag
	// VariationSelector covers the 256 variation selectors, which can encode
	// arbitrary bytes as an invisible sequence.
	VariationSelector
	// InvisibleMath covers the invisible mathematical operators
	// (U+2061-U+2064), usable as a binary encoding channel.
	InvisibleMath
	// ExoticSpace covers non-ASCII whitespace such as the no-break space and
	// the narrow no-break space, folded to a plain space.
	ExoticSpace
	// Separator covers the line and paragraph separators (U+2028, U+2029),
	// folded to a newline.
	Separator
	// Typography covers visible "smart" punctuation: curly quotes, en and em
	// dashes, ellipsis glyphs, primes, and the minus sign. Only reported and
	// folded when WithTypography is set.
	Typography
)

// String returns a short lowercase name for the category.
func (c Category) String() string {
	switch c {
	case ZeroWidth:
		return "zero-width"
	case Bidi:
		return "bidi-control"
	case Tag:
		return "tag-block"
	case VariationSelector:
		return "variation-selector"
	case InvisibleMath:
		return "invisible-math"
	case ExoticSpace:
		return "exotic-space"
	case Separator:
		return "separator"
	case Typography:
		return "typography"
	}
	return "unknown"
}

// Finding describes one character that Strip would remove or replace.
type Finding struct {
	// Rune is the offending character.
	Rune rune
	// Offset is its byte offset from the start of the input.
	Offset int
	// Line is the 1-based line number, counting U+000A line feeds only.
	Line int
	// Column is the 1-based position within the line, counted in runes.
	Column int
	// Category explains why the character was reported.
	Category Category
}

// Option configures Find and Strip. The zero set of options is the strict
// default: every invisible character is reported or removed, exotic whitespace
// is folded to ASCII, and visible typography is left alone.
type Option func(*config)

type config struct {
	typography             bool
	keepZWJ                bool
	keepVariationSelectors bool
}

// WithTypography additionally folds visible "smart" punctuation to plain ASCII:
// curly quotes, en and em dashes, ellipsis glyphs, primes, and the minus sign.
//
// This is off by default because those characters are legitimate, visible, and
// frequently intentional. Enable it only when normalizing to ASCII is the goal.
func WithTypography() Option {
	return func(c *config) { c.typography = true }
}

// PreserveZWJ keeps the zero-width joiner (U+200D), which is load-bearing in
// emoji sequences and in scripts such as Devanagari. Removing it silently
// changes rendering, so enable this when processing user-facing text that may
// legitimately contain either.
//
// Emoji sequences generally need PreserveVariationSelectors as well.
func PreserveZWJ() Option {
	return func(c *config) { c.keepZWJ = true }
}

// PreserveVariationSelectors keeps variation selectors (U+FE00-U+FE0F and
// U+E0100-U+E01EF), which select emoji versus text presentation and choose
// glyph variants in some fonts.
//
// They are also a practical steganographic carrier, so preserving them trades
// safety for fidelity. Enable it when rendering fidelity matters more than
// guaranteeing no hidden payload survives.
func PreserveVariationSelectors() Option {
	return func(c *config) { c.keepVariationSelectors = true }
}

func newConfig(opts []Option) config {
	var c config
	for _, o := range opts {
		o(&c)
	}
	return c
}

// disposition is what should happen to a classified rune. An empty replacement
// means the rune is deleted.
type disposition struct {
	category Category
	replace  string
}

// classify reports how r should be handled, returning ok=false to keep it
// verbatim. atStart marks the first rune of the input, where a byte-order mark
// is legitimate encoding metadata rather than a hidden character.
//
// Find and Strip both route through this function, so a character can never be
// reported by one and ignored by the other.
func classify(r rune, atStart bool, cfg config) (disposition, bool) {
	if r == bom && atStart {
		return disposition{}, false
	}
	if r == zwj && cfg.keepZWJ {
		return disposition{}, false
	}
	if inVariationSelectorRange(r) {
		if cfg.keepVariationSelectors {
			return disposition{}, false
		}
		return disposition{category: VariationSelector}, true
	}

	switch {
	case r == lineSep || r == paraSep:
		return disposition{category: Separator, replace: "\n"}, true
	case exoticSpaces[r]:
		return disposition{category: ExoticSpace, replace: " "}, true
	case invisibleMath[r]:
		return disposition{category: InvisibleMath}, true
	case bidiControls[r] || inBidiRange(r):
		return disposition{category: Bidi}, true
	case inTagRange(r):
		return disposition{category: Tag}, true
	case zeroWidth[r]:
		return disposition{category: ZeroWidth}, true
	}

	if cfg.typography {
		if repl, ok := typographyReplacements[r]; ok {
			return disposition{category: Typography, replace: repl}, true
		}
	}
	return disposition{}, false
}

// scan walks content and invokes fn for every rune that needs to change,
// in order. It is the single traversal shared by Find and Strip.
func scan(content []byte, cfg config, fn func(f Finding, replace string)) {
	line, col := 1, 1
	atStart := true

	for offset := 0; offset < len(content); {
		r, size := utf8.DecodeRune(content[offset:])

		if d, ok := classify(r, atStart, cfg); ok {
			fn(Finding{
				Rune:     r,
				Offset:   offset,
				Line:     line,
				Column:   col,
				Category: d.category,
			}, d.replace)
		}

		if r == '\n' {
			line++
			col = 1
		} else {
			col++
		}
		atStart = false
		offset += size
	}
}

// Find reports every character Strip would remove or replace, in the order they
// appear, without modifying the input. It returns nil when the content is
// clean, which makes it a cheap detection check:
//
//	if len(watermark.Find(data)) > 0 { ... }
//
// Positions are byte offsets plus 1-based line and rune-column numbers, so a
// finding can be pointed at directly in an editor or diagnostic.
func Find(content []byte, opts ...Option) []Finding {
	if isASCII(content) {
		return nil
	}
	var out []Finding
	scan(content, newConfig(opts), func(f Finding, _ string) {
		out = append(out, f)
	})
	return out
}

// Contains reports whether content holds any character Strip would change. It
// stops at the first hit, so prefer it over len(Find(...)) > 0 when the
// positions are not needed.
func Contains(content []byte, opts ...Option) bool {
	if isASCII(content) {
		return false
	}
	cfg := newConfig(opts)
	atStart := true
	for offset := 0; offset < len(content); {
		r, size := utf8.DecodeRune(content[offset:])
		if _, ok := classify(r, atStart, cfg); ok {
			return true
		}
		atStart = false
		offset += size
	}
	return false
}

// Strip removes hidden characters from content and folds exotic whitespace to
// plain ASCII, returning the cleaned bytes and whether anything changed. When
// nothing matches it returns the original slice unmodified.
//
// By default it deletes zero-width and joiner characters, bidi controls, the
// Unicode Tags block, variation selectors, and invisible math operators;
// replaces exotic spaces with U+0020 and line and paragraph separators with
// U+000A; and preserves a byte-order mark only as the first rune of the input.
// Visible typography is left alone unless WithTypography is set.
//
// Strip does not attempt to defeat statistical watermarks such as SynthID-Text,
// which encode provenance in word choice rather than in the bytes and cannot be
// removed by rewriting characters.
func Strip(content []byte, opts ...Option) ([]byte, bool) {
	// Every rune Strip touches is non-ASCII, so ASCII-only input is a
	// guaranteed no-op and can skip the rewrite entirely.
	if isASCII(content) {
		return content, false
	}

	// Copy the spans between changes rather than rune by rune, and allocate
	// only once the first change is seen: valid UTF-8 with no hidden
	// characters (accented prose, CJK) stays allocation-free.
	var b strings.Builder
	changed := false
	last := 0
	scan(content, newConfig(opts), func(f Finding, replace string) {
		if !changed {
			b.Grow(len(content))
			changed = true
		}
		b.Write(content[last:f.Offset])
		b.WriteString(replace)
		last = f.Offset + utf8.RuneLen(f.Rune)
	})

	if !changed {
		return content, false
	}
	b.Write(content[last:])
	return []byte(b.String()), true
}

// IsBinary reports whether content should be treated as non-text and left
// alone: it contains a NUL byte or is not valid UTF-8. Callers scanning a
// directory tree typically skip such files rather than rewriting them.
func IsBinary(content []byte) bool {
	if bytes.IndexByte(content, 0) >= 0 {
		return true
	}
	return !utf8.Valid(content)
}

// isASCII reports whether content contains only bytes below 0x80.
func isASCII(content []byte) bool {
	for _, b := range content {
		if b >= 0x80 {
			return false
		}
	}
	return true
}
