package cleaner

import (
	"strings"
	"testing"
)

func TestStepsCanonicalOrder(t *testing.T) {
	var c Cleaner
	keys := stepKeysOf(c.Steps())
	want := []string{"history", "artifacts", "markdown", "comments", "watermarks"}
	if strings.Join(keys, ",") != strings.Join(want, ",") {
		t.Errorf("step order = %v, want %v", keys, want)
	}
	// History must lead: in apply mode it requires a clean working tree.
	if keys[0] != "history" {
		t.Error("history must run first so the file steps do not dirty the tree")
	}
}

func TestSelectSteps(t *testing.T) {
	var c Cleaner
	all := c.Steps()

	tests := []struct {
		name    string
		only    []string
		skip    []string
		want    []string
		wantErr bool
	}{
		{name: "no selection runs everything", want: []string{"history", "artifacts", "markdown", "comments", "watermarks"}},
		{name: "only one", only: []string{"watermarks"}, want: []string{"watermarks"}},
		{name: "only several keeps canonical order", only: []string{"watermarks", "artifacts"}, want: []string{"artifacts", "watermarks"}},
		{name: "skip one", skip: []string{"history"}, want: []string{"artifacts", "markdown", "comments", "watermarks"}},
		{name: "skip several", skip: []string{"history", "markdown"}, want: []string{"artifacts", "comments", "watermarks"}},
		{name: "only and skip together", only: []string{"comments"}, skip: []string{"history"}, wantErr: true},
		{name: "unknown only key", only: []string{"bogus"}, wantErr: true},
		{name: "unknown skip key", skip: []string{"bogus"}, wantErr: true},
		{name: "skip everything", skip: []string{"history", "artifacts", "markdown", "comments", "watermarks"}, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := SelectSteps(all, tt.only, tt.skip)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected an error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("SelectSteps() error = %v", err)
			}
			if keys := stepKeysOf(got); strings.Join(keys, ",") != strings.Join(tt.want, ",") {
				t.Errorf("steps = %v, want %v", keys, tt.want)
			}
		})
	}
}

func TestParseStepList(t *testing.T) {
	tests := []struct {
		name  string
		value string
		want  []string
	}{
		{"empty", "", nil},
		{"whitespace only", "  ", nil},
		{"single", "comments", []string{"comments"}},
		{"multiple", "comments,watermarks", []string{"comments", "watermarks"}},
		{"spaces and case", " Comments , WATERMARKS ", []string{"comments", "watermarks"}},
		{"empty fields ignored", "comments,,watermarks", []string{"comments", "watermarks"}},
		{"duplicates collapsed", "comments,comments", []string{"comments"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ParseStepList(tt.value)
			if strings.Join(got, ",") != strings.Join(tt.want, ",") {
				t.Errorf("ParseStepList(%q) = %v, want %v", tt.value, got, tt.want)
			}
		})
	}
}

func TestStepHelpListsEveryStep(t *testing.T) {
	help := StepHelp()
	for _, key := range StepKeys() {
		if !strings.Contains(help, key) {
			t.Errorf("StepHelp() missing step %q:\n%s", key, help)
		}
	}
}

func TestStepKeys(t *testing.T) {
	keys := StepKeys()
	if len(keys) != 5 {
		t.Fatalf("StepKeys() returned %d keys, want 5", len(keys))
	}
	seen := make(map[string]bool)
	for _, k := range keys {
		if seen[k] {
			t.Errorf("duplicate step key %q", k)
		}
		seen[k] = true
	}
}

func stepKeysOf(steps []Step) []string {
	keys := make([]string, len(steps))
	for i, s := range steps {
		keys[i] = s.Key
	}
	return keys
}
