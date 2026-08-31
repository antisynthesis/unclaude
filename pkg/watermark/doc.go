// Package watermark finds and removes invisible Unicode characters from text.
//
// It targets characters that render no glyph but survive copy-paste: zero-width
// spaces and joiners, directional-formatting controls, the Unicode Tags block,
// variation selectors, invisible mathematical operators, and non-ASCII
// whitespace. These appear as provenance marks in generated text, as accidental
// residue from word processors and web pages, and as deliberate attack payloads.
//
// # Security relevance
//
// Three of the covered ranges are active attack vectors rather than cosmetic
// noise:
//
//   - Bidi controls are the mechanism behind Trojan Source (CVE-2021-42574),
//     where a directional override makes source code render in an order that
//     differs from how the compiler reads it.
//   - The Unicode Tags block (U+E0000-U+E007F) is a complete invisible ASCII
//     alphabet, used to smuggle instructions past human reviewers and into
//     language models, and to exfiltrate data back out.
//   - Variation selectors number exactly 256, enough to encode any byte, which
//     makes them a practical carrier for arbitrary hidden payloads.
//
// Scanning untrusted input before it reaches a reviewer, a compiler, or a model
// prompt is the intended use.
//
// # Detecting and removing
//
// Find reports what is present without modifying anything, which suits linters,
// CI checks, and diagnostics:
//
//	for _, f := range watermark.Find(data) {
//	    fmt.Printf("%s:%d:%d: %s U+%04X\n", path, f.Line, f.Column, f.Category, f.Rune)
//	}
//
// Contains answers the same question when only a yes or no is needed, and stops
// at the first hit. Strip returns cleaned bytes and whether anything changed:
//
//	cleaned, changed := watermark.Strip(data)
//
// Find, Contains, and Strip route through one classification function and
// accept the same options, so what Find reports is exactly what Strip changes.
//
// # Defaults and options
//
// The default is strict: every hidden character is removed, exotic whitespace
// becomes U+0020, line and paragraph separators become U+000A, and a byte-order
// mark is preserved only as the first rune of the input.
//
// Two categories are legitimate often enough to warrant opting out. PreserveZWJ
// keeps the zero-width joiner, which is load-bearing in emoji sequences and in
// scripts such as Devanagari, and PreserveVariationSelectors keeps emoji and
// glyph-variant selectors. Both trade a hiding place for rendering fidelity.
// Conversely WithTypography additionally folds visible smart punctuation, which
// is off by default precisely because those characters are usually intentional.
//
// # Limits
//
// This package rewrites characters, so it cannot address statistical
// watermarks such as SynthID-Text, which bias a model's word choice rather than
// inserting anything. Text carrying one still carries it after Strip.
//
// Nor does a clean result prove text is human-written, or a dirty one prove it
// is not: a stray no-break space from a word processor is far more common than
// an attack.
//
// The package depends only on the standard library.
package watermark
