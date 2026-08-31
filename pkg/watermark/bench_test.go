package watermark

import (
	"strings"
	"testing"
)

// benchProse builds a multi-kilobyte prose blob. When seeded is true, every
// tenth line carries a zero-width space and a narrow no-break space so the
// rewriting path (not just the scan) is exercised.
func benchProse(seeded bool) []byte {
	var b strings.Builder
	const line = "The quick brown fox jumps over the lazy dog while refactoring code.\n"
	for i := 0; i < 200; i++ {
		b.WriteString(line)
		if seeded && i%10 == 0 {
			b.WriteString("hidden" + cp(0x200B) + "mark" + cp(0x202F) + "here\n")
		}
	}
	return []byte(b.String())
}

// benchUnicodeClean is valid non-ASCII text with nothing to strip: the case
// that must not pay for a rewrite it does not need.
func benchUnicodeClean() []byte {
	return []byte(strings.Repeat("Le renard brun saute par-dessus le chien paresseux. 你好世界。\n", 100))
}

func BenchmarkStripSeeded(b *testing.B) {
	in := benchProse(true)
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		Strip(in)
	}
}

func BenchmarkStripASCIIClean(b *testing.B) {
	in := benchProse(false)
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		Strip(in)
	}
}

func BenchmarkStripUnicodeClean(b *testing.B) {
	in := benchUnicodeClean()
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		Strip(in)
	}
}

func BenchmarkStripTypography(b *testing.B) {
	unit := "A line with " + cp(0x2014) + " dashes, " +
		cp(0x201C) + "quotes" + cp(0x201D) + ", and " + cp(0x2026) + " ellipsis.\n"
	in := []byte(strings.Repeat(unit, 200))
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		Strip(in, WithTypography())
	}
}

func BenchmarkFindSeeded(b *testing.B) {
	in := benchProse(true)
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		Find(in)
	}
}

func BenchmarkContainsSeeded(b *testing.B) {
	in := benchProse(true)
	b.SetBytes(int64(len(in)))
	b.ReportAllocs()
	for b.Loop() {
		Contains(in)
	}
}
