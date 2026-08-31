package cleaner

import (
	"fmt"
	"sort"
	"strings"
)

// Step is one named cleaning operation. Steps are exposed by key so callers can
// select a subset (see SelectSteps) without knowing the method set.
type Step struct {
	// Key is the short name used by --only and --skip.
	Key string
	// Description is a one-line summary for help output.
	Description string
	// Run executes the step.
	Run func() error
}

// Steps returns every cleaning step in the order they must run.
//
// History rewrite comes first deliberately: in apply mode it requires a clean
// working tree, so it has to happen before the file-modifying steps dirty it.
func (c *Cleaner) Steps() []Step {
	return []Step{
		{"history", "rewrite commit messages to drop generation footers", c.CleanGitHistory},
		{"artifacts", "remove AI tool directories and instruction files", c.CleanAIArtifacts},
		{"markdown", "remove generic Markdown outside the docs allowlist", c.CleanMarkdownFiles},
		{"comments", "strip AI-related comments from source files", c.CleanSourceComments},
		{"watermarks", "strip invisible watermark characters from text files", c.CleanWatermarks},
	}
}

// StepKeys returns every valid step key in canonical run order.
func StepKeys() []string {
	var c Cleaner
	steps := c.Steps()
	keys := make([]string, len(steps))
	for i, s := range steps {
		keys[i] = s.Key
	}
	return keys
}

// SelectSteps filters steps by the --only and --skip selections, preserving
// canonical order. At most one of only/skip may be non-empty. Unknown keys and
// a selection that would run nothing are reported as errors.
func SelectSteps(steps []Step, only, skip []string) ([]Step, error) {
	if len(only) > 0 && len(skip) > 0 {
		return nil, fmt.Errorf("--only and --skip are mutually exclusive")
	}

	valid := make(map[string]bool, len(steps))
	for _, s := range steps {
		valid[s.Key] = true
	}
	known := func(keys []string, flag string) error {
		for _, k := range keys {
			if !valid[k] {
				return fmt.Errorf("unknown step %q for %s (valid: %s)", k, flag, strings.Join(StepKeys(), ", "))
			}
		}
		return nil
	}

	if len(only) > 0 {
		if err := known(only, "--only"); err != nil {
			return nil, err
		}
		set := toSet(only)
		var out []Step
		for _, s := range steps {
			if set[s.Key] {
				out = append(out, s)
			}
		}
		return out, nil
	}

	if len(skip) > 0 {
		if err := known(skip, "--skip"); err != nil {
			return nil, err
		}
		set := toSet(skip)
		var out []Step
		for _, s := range steps {
			if !set[s.Key] {
				out = append(out, s)
			}
		}
		if len(out) == 0 {
			return nil, fmt.Errorf("--skip excludes every step; nothing to do")
		}
		return out, nil
	}

	return steps, nil
}

// ParseStepList splits a comma-separated flag value into normalized step keys.
// Empty fields are ignored and duplicates are collapsed, so "a,,b,a" yields
// [a b]. It returns nil for an empty or whitespace-only value.
func ParseStepList(value string) []string {
	seen := make(map[string]bool)
	var out []string
	for _, part := range strings.Split(value, ",") {
		key := strings.ToLower(strings.TrimSpace(part))
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, key)
	}
	return out
}

func toSet(keys []string) map[string]bool {
	set := make(map[string]bool, len(keys))
	for _, k := range keys {
		set[k] = true
	}
	return set
}

// StepHelp renders the step keys and descriptions for CLI help text, one
// "  key  description" line per step, sorted by key for stable output.
func StepHelp() string {
	var c Cleaner
	steps := c.Steps()
	sorted := make([]Step, len(steps))
	copy(sorted, steps)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].Key < sorted[j].Key })

	width := 0
	for _, s := range sorted {
		if len(s.Key) > width {
			width = len(s.Key)
		}
	}
	var b strings.Builder
	for _, s := range sorted {
		fmt.Fprintf(&b, "  %-*s  %s\n", width, s.Key, s.Description)
	}
	return b.String()
}
