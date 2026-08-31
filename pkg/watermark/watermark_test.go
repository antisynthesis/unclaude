package watermark

import (
	"strings"
	"testing"
)

// cp builds a string from a codepoint so tests never embed literal invisible
// characters in source, which would be unreviewable and easy to corrupt.
func cp(r rune) string { return string(r) }

func TestStrip(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		opts    []Option
		want    string
		changed bool
	}{
		{name: "zero width space deleted", input: "he" + cp(0x200B) + "llo", want: "hello", changed: true},
		{name: "zero width non-joiner deleted", input: "a" + cp(0x200C) + "b", want: "ab", changed: true},
		{name: "zero width joiner deleted", input: "a" + cp(0x200D) + "b", want: "ab", changed: true},
		{name: "word joiner deleted", input: "a" + cp(0x2060) + "b", want: "ab", changed: true},
		{name: "soft hyphen deleted", input: "co" + cp(0x00AD) + "operate", want: "cooperate", changed: true},
		{name: "invisible plus deleted", input: "1" + cp(0x2064) + "2", want: "12", changed: true},
		{name: "invisible times deleted", input: "1" + cp(0x2062) + "2", want: "12", changed: true},
		{name: "narrow no-break space folded", input: "1" + cp(0x202F) + "kg", want: "1 kg", changed: true},
		{name: "no-break space folded", input: "a" + cp(0x00A0) + "b", want: "a b", changed: true},
		{name: "ideographic space folded", input: "a" + cp(0x3000) + "b", want: "a b", changed: true},
		{name: "ltr mark deleted", input: "x" + cp(0x200E) + "y", want: "xy", changed: true},
		{name: "bidi override deleted", input: "x" + cp(0x202E) + "y", want: "xy", changed: true},
		{name: "bidi isolate deleted", input: "x" + cp(0x2066) + "y" + cp(0x2069), want: "xy", changed: true},
		{name: "variation selector 16 deleted", input: "a" + cp(0xFE0F) + "b", want: "ab", changed: true},
		{name: "variation selector supplement deleted", input: "a" + cp(0xE0100) + "b", want: "ab", changed: true},
		{name: "tag block deleted", input: "hi" + cp(0xE0041) + "!", want: "hi!", changed: true},
		{name: "line separator becomes newline", input: "a" + cp(0x2028) + "b", want: "a\nb", changed: true},
		{name: "paragraph separator becomes newline", input: "a" + cp(0x2029) + "b", want: "a\nb", changed: true},
		{name: "leading BOM preserved", input: cp(0xFEFF) + "package main", want: cp(0xFEFF) + "package main"},
		{name: "mid-text BOM deleted", input: "a" + cp(0xFEFF) + "b", want: "ab", changed: true},
		{name: "clean ascii unchanged", input: "package main\n\nfunc main() {}\n", want: "package main\n\nfunc main() {}\n"},
		{name: "clean non-ascii unchanged", input: "café naïve 你好", want: "café naïve 你好"},
		{
			name:    "multiple watermarks in one string",
			input:   "start" + cp(0x200B) + "mid" + cp(0x202F) + "end" + cp(0xE0042),
			want:    "startmid end",
			changed: true,
		},
		{
			name:    "adjacent watermarks",
			input:   "a" + cp(0x200B) + cp(0x200B) + cp(0x202F) + "b",
			want:    "a b",
			changed: true,
		},
		{name: "watermark at start", input: cp(0x200B) + "abc", want: "abc", changed: true},
		{name: "watermark at end", input: "abc" + cp(0x200B), want: "abc", changed: true},
		{name: "typography untouched by default", input: "a" + cp(0x2014) + "b", want: "a" + cp(0x2014) + "b"},

		// Options.
		{name: "typography folded when enabled", input: "a" + cp(0x2014) + "b", opts: []Option{WithTypography()}, want: "a--b", changed: true},
		{name: "en dash folded", input: "2010" + cp(0x2013) + "2020", opts: []Option{WithTypography()}, want: "2010-2020", changed: true},
		{name: "curly double quotes folded", input: cp(0x201C) + "hi" + cp(0x201D), opts: []Option{WithTypography()}, want: "\"hi\"", changed: true},
		{name: "curly single quotes folded", input: cp(0x2018) + "hi" + cp(0x2019), opts: []Option{WithTypography()}, want: "'hi'", changed: true},
		{name: "ellipsis folded", input: "wait" + cp(0x2026), opts: []Option{WithTypography()}, want: "wait...", changed: true},
		{name: "minus sign folded", input: cp(0x2212) + "5", opts: []Option{WithTypography()}, want: "-5", changed: true},
		{name: "zwj preserved when requested", input: "a" + cp(0x200D) + "b", opts: []Option{PreserveZWJ()}, want: "a" + cp(0x200D) + "b"},
		{
			name:    "zwj preserved but other hidden chars still removed",
			input:   "a" + cp(0x200D) + "b" + cp(0x200B) + "c",
			opts:    []Option{PreserveZWJ()},
			want:    "a" + cp(0x200D) + "bc",
			changed: true,
		},
		{
			name:  "variation selectors preserved when requested",
			input: "a" + cp(0xFE0F) + "b",
			opts:  []Option{PreserveVariationSelectors()},
			want:  "a" + cp(0xFE0F) + "b",
		},
		{
			name:  "emoji zwj sequence survives with both options",
			input: "\U0001F468" + cp(0x200D) + "\U0001F4BB",
			opts:  []Option{PreserveZWJ(), PreserveVariationSelectors()},
			want:  "\U0001F468" + cp(0x200D) + "\U0001F4BB",
		},
		{
			name:    "tag block still removed with preserve options",
			input:   "hi" + cp(0xE0041),
			opts:    []Option{PreserveZWJ(), PreserveVariationSelectors()},
			want:    "hi",
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := Strip([]byte(tt.input), tt.opts...)
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if string(got) != tt.want {
				t.Errorf("output mismatch:\ngot:  %q\nwant: %q", string(got), tt.want)
			}
		})
	}
}

func TestStripEmptyInput(t *testing.T) {
	got, changed := Strip(nil)
	if changed || len(got) != 0 {
		t.Errorf("Strip(nil) = %q, %v; want empty, false", got, changed)
	}
}

func TestFindReportsCategoryAndPosition(t *testing.T) {
	// Line 2 holds a zero-width space after "ab"; line 3 a tag character.
	input := "clean\n" + "ab" + cp(0x200B) + "cd\n" + "x" + cp(0xE0041) + "\n"
	got := Find([]byte(input))

	if len(got) != 2 {
		t.Fatalf("Find() returned %d findings, want 2: %+v", len(got), got)
	}

	first := got[0]
	if first.Rune != 0x200B {
		t.Errorf("first rune = U+%04X, want U+200B", first.Rune)
	}
	if first.Category != ZeroWidth {
		t.Errorf("first category = %v, want %v", first.Category, ZeroWidth)
	}
	if first.Line != 2 || first.Column != 3 {
		t.Errorf("first position = %d:%d, want 2:3", first.Line, first.Column)
	}
	if want := strings.Index(input, cp(0x200B)); first.Offset != want {
		t.Errorf("first offset = %d, want %d", first.Offset, want)
	}

	second := got[1]
	if second.Category != Tag {
		t.Errorf("second category = %v, want %v", second.Category, Tag)
	}
	if second.Line != 3 || second.Column != 2 {
		t.Errorf("second position = %d:%d, want 3:2", second.Line, second.Column)
	}
}

func TestFindCategories(t *testing.T) {
	tests := []struct {
		name  string
		input string
		opts  []Option
		want  Category
	}{
		{"zero width", cp(0x200B), nil, ZeroWidth},
		{"soft hyphen", cp(0x00AD), nil, ZeroWidth},
		{"bidi mark", cp(0x200E), nil, Bidi},
		{"bidi override", cp(0x202E), nil, Bidi},
		{"bidi isolate", cp(0x2066), nil, Bidi},
		{"arabic letter mark", cp(0x061C), nil, Bidi},
		{"tag block", cp(0xE0041), nil, Tag},
		{"variation selector", cp(0xFE0F), nil, VariationSelector},
		{"variation selector supplement", cp(0xE0100), nil, VariationSelector},
		{"invisible times", cp(0x2062), nil, InvisibleMath},
		{"exotic space", cp(0x202F), nil, ExoticSpace},
		{"line separator", cp(0x2028), nil, Separator},
		{"em dash with typography", cp(0x2014), []Option{WithTypography()}, Typography},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Find([]byte("a"+tt.input+"b"), tt.opts...)
			if len(got) != 1 {
				t.Fatalf("Find() returned %d findings, want 1", len(got))
			}
			if got[0].Category != tt.want {
				t.Errorf("category = %v, want %v", got[0].Category, tt.want)
			}
		})
	}
}

func TestFindReturnsNilWhenClean(t *testing.T) {
	for _, input := range []string{"", "plain ascii text\n", "café 你好"} {
		if got := Find([]byte(input)); got != nil {
			t.Errorf("Find(%q) = %+v, want nil", input, got)
		}
	}
}

// TestFindAndStripAgree is the invariant that justifies the shared classifier:
// Find must report exactly the characters Strip changes, under any options.
func TestFindAndStripAgree(t *testing.T) {
	inputs := []string{
		"a" + cp(0x200B) + "b" + cp(0x202F) + "c" + cp(0xE0041),
		cp(0xFEFF) + "leading bom then" + cp(0x200D) + "joiner",
		"typography " + cp(0x2014) + " and " + cp(0x201C) + "quotes" + cp(0x201D),
		"clean ascii",
		"café naïve",
		"\U0001F468" + cp(0x200D) + "\U0001F4BB",
	}
	optSets := [][]Option{
		nil,
		{WithTypography()},
		{PreserveZWJ()},
		{PreserveVariationSelectors()},
		{WithTypography(), PreserveZWJ(), PreserveVariationSelectors()},
	}

	for _, in := range inputs {
		for i, opts := range optSets {
			findings := Find([]byte(in), opts...)
			_, changed := Strip([]byte(in), opts...)
			if (len(findings) > 0) != changed {
				t.Errorf("disagreement for %q optSet %d: %d findings but changed=%v", in, i, len(findings), changed)
			}
			if got := Contains([]byte(in), opts...); got != changed {
				t.Errorf("Contains(%q) optSet %d = %v, want %v", in, i, got, changed)
			}
		}
	}
}

func TestContains(t *testing.T) {
	if Contains([]byte("plain ascii")) {
		t.Error("Contains() = true for clean ASCII")
	}
	if !Contains([]byte("a" + cp(0x200B))) {
		t.Error("Contains() = false for zero-width space")
	}
	if Contains([]byte("a" + cp(0x2014))) {
		t.Error("Contains() = true for typography without WithTypography")
	}
	if !Contains([]byte("a"+cp(0x2014)), WithTypography()) {
		t.Error("Contains() = false for em dash with WithTypography")
	}
}

func TestCategoryString(t *testing.T) {
	for _, c := range []Category{ZeroWidth, Bidi, Tag, VariationSelector, InvisibleMath, ExoticSpace, Separator, Typography} {
		if s := c.String(); s == "" || s == "unknown" {
			t.Errorf("Category(%d).String() = %q", int(c), s)
		}
	}
	if got := Category(99).String(); got != "unknown" {
		t.Errorf("Category(99).String() = %q, want unknown", got)
	}
}

func TestIsBinary(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"nul byte", []byte{'a', 0x00, 'b'}, true},
		{"valid utf8", []byte("héllo — world"), false},
		{"invalid utf8", []byte{0xff, 0xfe, 0x41}, true},
		{"empty", []byte{}, false},
		{"plain ascii", []byte("hello"), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsBinary(tt.content); got != tt.want {
				t.Errorf("IsBinary() = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestStripIsIdempotent guards against a rewrite that introduces characters it
// would itself remove on a second pass.
func TestStripIsIdempotent(t *testing.T) {
	input := []byte("a" + cp(0x200B) + "b" + cp(0x202F) + cp(0x2028) + cp(0x2014) + "c")
	once, _ := Strip(input, WithTypography())
	twice, changed := Strip(once, WithTypography())
	if changed {
		t.Errorf("second Strip changed output again: %q -> %q", once, twice)
	}
}
