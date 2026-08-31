package cleaner

import (
	"os"
	"path/filepath"
	"testing"
)

// zw builds a string from a codepoint so tests never embed literal invisible
// characters in source (which would be unreviewable and easy to corrupt).
func zw(cp rune) string { return string(cp) }

func TestStripInvisible(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "zero width space deleted",
			input:   "he" + zw(0x200B) + "llo",
			want:    "hello",
			changed: true,
		},
		{
			name:    "zero width joiner deleted",
			input:   "a" + zw(0x200D) + "b",
			want:    "ab",
			changed: true,
		},
		{
			name:    "word joiner deleted",
			input:   "a" + zw(0x2060) + "b",
			want:    "ab",
			changed: true,
		},
		{
			name:    "soft hyphen deleted",
			input:   "co" + zw(0x00AD) + "operate",
			want:    "cooperate",
			changed: true,
		},
		{
			name:    "invisible plus deleted",
			input:   "1" + zw(0x2064) + "2",
			want:    "12",
			changed: true,
		},
		{
			name:    "narrow no-break space normalized to ascii space",
			input:   "1" + zw(0x202F) + "kg",
			want:    "1 kg",
			changed: true,
		},
		{
			name:    "no-break space normalized to ascii space",
			input:   "a" + zw(0x00A0) + "b",
			want:    "a b",
			changed: true,
		},
		{
			name:    "bidi override deleted",
			input:   "x" + zw(0x202E) + "y",
			want:    "xy",
			changed: true,
		},
		{
			name:    "bidi isolate deleted",
			input:   "x" + zw(0x2066) + "y" + zw(0x2069),
			want:    "xy",
			changed: true,
		},
		{
			name:    "variation selector 16 deleted",
			input:   "a" + zw(0xFE0F) + "b",
			want:    "ab",
			changed: true,
		},
		{
			name:    "tag block character deleted",
			input:   "hi" + zw(0xE0041) + "!",
			want:    "hi!",
			changed: true,
		},
		{
			name:    "variation selector supplement deleted",
			input:   "a" + zw(0xE0100) + "b",
			want:    "ab",
			changed: true,
		},
		{
			name:    "line separator becomes newline",
			input:   "a" + zw(0x2028) + "b",
			want:    "a\nb",
			changed: true,
		},
		{
			name:    "paragraph separator becomes newline",
			input:   "a" + zw(0x2029) + "b",
			want:    "a\nb",
			changed: true,
		},
		{
			name:    "leading BOM preserved",
			input:   zw(0xFEFF) + "package main",
			want:    zw(0xFEFF) + "package main",
			changed: false,
		},
		{
			name:    "mid-file BOM deleted",
			input:   "a" + zw(0xFEFF) + "b",
			want:    "ab",
			changed: true,
		},
		{
			name:    "clean ascii unchanged",
			input:   "package main\n\nfunc main() {}\n",
			want:    "package main\n\nfunc main() {}\n",
			changed: false,
		},
		{
			name:    "multiple watermarks in one string",
			input:   "start" + zw(0x200B) + "mid" + zw(0x202F) + "end" + zw(0xE0042),
			want:    "startmid end",
			changed: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := StripInvisible([]byte(tt.input))
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if string(got) != tt.want {
				t.Errorf("output mismatch:\ngot:  %q\nwant: %q", string(got), tt.want)
			}
		})
	}
}

func TestNormalizeTypography(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		changed bool
	}{
		{
			name:    "em dash to double hyphen",
			input:   "yes" + zw(0x2014) + "no",
			want:    "yes--no",
			changed: true,
		},
		{
			name:    "en dash to hyphen",
			input:   "2010" + zw(0x2013) + "2020",
			want:    "2010-2020",
			changed: true,
		},
		{
			name:    "curly double quotes to straight",
			input:   zw(0x201C) + "hi" + zw(0x201D),
			want:    "\"hi\"",
			changed: true,
		},
		{
			name:    "curly single quotes to straight",
			input:   zw(0x2018) + "hi" + zw(0x2019),
			want:    "'hi'",
			changed: true,
		},
		{
			name:    "ellipsis glyph to three dots",
			input:   "wait" + zw(0x2026),
			want:    "wait...",
			changed: true,
		},
		{
			name:    "minus sign to hyphen",
			input:   zw(0x2212) + "5",
			want:    "-5",
			changed: true,
		},
		{
			name:    "plain ascii unchanged",
			input:   "just plain text",
			want:    "just plain text",
			changed: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := NormalizeTypography([]byte(tt.input))
			if changed != tt.changed {
				t.Errorf("changed = %v, want %v", changed, tt.changed)
			}
			if string(got) != tt.want {
				t.Errorf("output mismatch:\ngot:  %q\nwant: %q", string(got), tt.want)
			}
		})
	}
}

func TestLooksBinary(t *testing.T) {
	tests := []struct {
		name    string
		content []byte
		want    bool
	}{
		{"nul byte", []byte{'a', 0x00, 'b'}, true},
		{"valid utf8", []byte("héllo — world"), false},
		{"invalid utf8", []byte{0xff, 0xfe, 0x41}, true},
		{"empty", []byte{}, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := looksBinary(tt.content); got != tt.want {
				t.Errorf("looksBinary() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCleanWatermarksStripsFilesAndSkipsBinary(t *testing.T) {
	tmpDir := t.TempDir()

	goSrc := "package main\n\nvar s = \"clean" + zw(0x200B) + "code\"\n"
	mdDoc := "# Title" + zw(0x202F) + "here\n\nA dash" + zw(0x2014) + "and more.\n"
	binary := []byte{0x00, 0x01, 0x02, 0xff}

	setupTestFiles(t, tmpDir, map[string]string{
		"main.go":   goSrc,
		"README.md": mdDoc,
	})
	if err := os.WriteFile(filepath.Join(tmpDir, "logo.png"), binary, 0644); err != nil {
		t.Fatal(err)
	}

	// Default run: invisible chars stripped, typography left alone.
	c := New(tmpDir, Options{DryRun: false})
	if err := c.CleanWatermarks(); err != nil {
		t.Fatalf("CleanWatermarks() error = %v", err)
	}

	gotGo, _ := os.ReadFile(filepath.Join(tmpDir, "main.go"))
	if want := "package main\n\nvar s = \"cleancode\"\n"; string(gotGo) != want {
		t.Errorf("main.go mismatch:\ngot:  %q\nwant: %q", string(gotGo), want)
	}

	gotMd, _ := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	if want := "# Title here\n\nA dash" + zw(0x2014) + "and more.\n"; string(gotMd) != want {
		t.Errorf("README.md mismatch:\ngot:  %q\nwant: %q", string(gotMd), want)
	}

	gotBin, _ := os.ReadFile(filepath.Join(tmpDir, "logo.png"))
	if string(gotBin) != string(binary) {
		t.Errorf("binary file was modified: %v", gotBin)
	}
}

func TestCleanWatermarksTypographyOptIn(t *testing.T) {
	tmpDir := t.TempDir()
	mdDoc := "A dash" + zw(0x2014) + "and " + zw(0x201C) + "quotes" + zw(0x201D) + ".\n"
	setupTestFiles(t, tmpDir, map[string]string{"README.md": mdDoc})

	c := New(tmpDir, Options{DryRun: false, NormalizeTypography: true})
	if err := c.CleanWatermarks(); err != nil {
		t.Fatalf("CleanWatermarks() error = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(tmpDir, "README.md"))
	if want := "A dash--and \"quotes\".\n"; string(got) != want {
		t.Errorf("README.md mismatch:\ngot:  %q\nwant: %q", string(got), want)
	}
}

func TestCleanWatermarksTypographyProseOnly(t *testing.T) {
	tmpDir := t.TempDir()
	// An em dash inside a Go string must survive even with typography on:
	// typography folding is scoped to prose files, not source.
	goSrc := "package main\n\nvar s = \"a" + zw(0x2014) + "b\"\n"
	setupTestFiles(t, tmpDir, map[string]string{"main.go": goSrc})

	c := New(tmpDir, Options{DryRun: false, NormalizeTypography: true})
	if err := c.CleanWatermarks(); err != nil {
		t.Fatalf("CleanWatermarks() error = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(tmpDir, "main.go"))
	if string(got) != goSrc {
		t.Errorf("source em dash should be preserved:\ngot:  %q\nwant: %q", string(got), goSrc)
	}
}

func TestCleanWatermarksDryRun(t *testing.T) {
	tmpDir := t.TempDir()
	orig := "hi" + zw(0x200B) + "there\n"
	setupTestFiles(t, tmpDir, map[string]string{"main.go": orig})

	c := New(tmpDir, Options{DryRun: true})
	if err := c.CleanWatermarks(); err != nil {
		t.Fatalf("CleanWatermarks() error = %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(tmpDir, "main.go"))
	if string(got) != orig {
		t.Errorf("dry run modified file:\ngot:  %q\nwant: %q", string(got), orig)
	}
}
