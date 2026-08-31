package watermark_test

import (
	"fmt"

	"github.com/antisynthesis/unclaude/pkg/watermark"
)

func ExampleStrip() {
	// "hi<U+200B>there" carries a zero-width space that survives copy-paste.
	input := []byte("hi\u200bthere")

	cleaned, changed := watermark.Strip(input)
	fmt.Printf("%q changed=%v\n", cleaned, changed)
	// Output: "hithere" changed=true
}

func ExampleStrip_typography() {
	input := []byte("yes—no “quoted”")

	// Visible smart punctuation is left alone by default.
	asIs, _ := watermark.Strip(input)
	fmt.Printf("default:    %s\n", asIs)

	folded, _ := watermark.Strip(input, watermark.WithTypography())
	fmt.Printf("typography: %s\n", folded)
	// Output:
	// default:    yes—no “quoted”
	// typography: yes--no "quoted"
}

func ExampleStrip_preserveEmoji() {
	// A man-technologist emoji is a ZWJ sequence: two emoji joined by U+200D.
	input := []byte("\U0001F468\u200d\U0001F4BB")

	stripped, _ := watermark.Strip(input)
	fmt.Printf("default:  %d runes\n", len([]rune(string(stripped))))

	kept, _ := watermark.Strip(input, watermark.PreserveZWJ())
	fmt.Printf("preserved: %d runes\n", len([]rune(string(kept))))
	// Output:
	// default:  2 runes
	// preserved: 3 runes
}

func ExampleFind() {
	// A bidi override on line 2: the Trojan Source vector.
	input := []byte("safe line\nif (isAdmin) \u202e{\n")

	for _, f := range watermark.Find(input) {
		fmt.Printf("%d:%d: %s U+%04X\n", f.Line, f.Column, f.Category, f.Rune)
	}
	// Output: 2:14: bidi-control U+202E
}

func ExampleContains() {
	fmt.Println(watermark.Contains([]byte("plain text")))
	fmt.Println(watermark.Contains([]byte("hidden\u200bpayload")))
	// Output:
	// false
	// true
}

func ExampleIsBinary() {
	fmt.Println(watermark.IsBinary([]byte("readable text")))
	fmt.Println(watermark.IsBinary([]byte{0x89, 'P', 'N', 'G', 0x00}))
	// Output:
	// false
	// true
}
