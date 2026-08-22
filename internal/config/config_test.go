package config

import (
	"strings"
	"testing"
)

// TestUpdateUISectionPreservesComments checks that saving the UI state into
// the full template keeps every comment and the commented-out [keys] block.
func TestUpdateUISectionPreservesComments(t *testing.T) {
	out := updateUISection(templateConfig, UIState{ActiveView: "hidden", SplitRatio: 0.42})

	// The commented key-bindings section and its keys must survive.
	for _, want := range []string{
		"# [keys]",
		"# quit = \"q\"",
		"# visualizer = \"5\"",
		"# Key bindings.",
		"# The last right-pane state",
		"# The fraction of the width",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("saved config lost %q", want)
		}
	}

	// The two keys must hold the new values.
	if !strings.Contains(out, `active_view = "hidden"`) {
		t.Errorf("active_view not updated:\n%s", out)
	}
	if !strings.Contains(out, "split_ratio = 0.42") {
		t.Errorf("split_ratio not updated:\n%s", out)
	}

	// There must be exactly one [ui] header.
	if strings.Count(out, "[ui]") != 1 {
		t.Errorf("want one [ui] section, got %d", strings.Count(out, "[ui]"))
	}
}

// TestUpdateUISectionAppendsWhenMissing checks that a file without a [ui]
// section gets one appended.
func TestUpdateUISectionAppendsWhenMissing(t *testing.T) {
	in := "server_url = \"x\"\n# a comment\n"
	out := updateUISection(in, UIState{ActiveView: "nav", SplitRatio: 0.6})
	if !strings.Contains(out, "server_url = \"x\"") || !strings.Contains(out, "# a comment") {
		t.Errorf("lost original content:\n%s", out)
	}
	if !strings.Contains(out, "[ui]") || !strings.Contains(out, `active_view = "nav"`) ||
		!strings.Contains(out, "split_ratio = 0.6") {
		t.Errorf("did not append [ui]:\n%s", out)
	}
}

// TestUpdateUISectionInPlaceOrder checks that only the values change when the
// keys are present with surrounding comments, and the comments stay put.
func TestUpdateUISectionInPlaceOrder(t *testing.T) {
	in := strings.Join([]string{
		"[ui]",
		"# keep me",
		`active_view = "nav"`,
		"# and me",
		"split_ratio = 0.6",
		"",
		"# [keys]",
		`# pause = "p"`,
	}, "\n")
	out := updateUISection(in, UIState{ActiveView: "hidden", SplitRatio: 0.3})
	for _, want := range []string{"# keep me", "# and me", "# [keys]", `# pause = "p"`} {
		if !strings.Contains(out, want) {
			t.Errorf("lost %q:\n%s", want, out)
		}
	}
	if !strings.Contains(out, `active_view = "hidden"`) || !strings.Contains(out, "split_ratio = 0.3") {
		t.Errorf("values not updated:\n%s", out)
	}
	if strings.Contains(out, `active_view = "nav"`) || strings.Contains(out, "split_ratio = 0.6") {
		t.Errorf("old values remain:\n%s", out)
	}
}

// TestUpdateUISectionIdempotent checks that saving twice is stable.
func TestUpdateUISectionIdempotent(t *testing.T) {
	first := updateUISection(templateConfig, UIState{ActiveView: "nav", SplitRatio: 0.6})
	second := updateUISection(first, UIState{ActiveView: "nav", SplitRatio: 0.6})
	if first != second {
		t.Errorf("save is not idempotent:\nfirst:\n%s\nsecond:\n%s", first, second)
	}
}

// TestUpdateUISectionInsertsMissingKeys checks that a [ui] section missing a
// key gets it inserted without losing the present key.
func TestUpdateUISectionInsertsMissingKeys(t *testing.T) {
	in := "[ui]\nactive_view = \"nav\"\n"
	out := updateUISection(in, UIState{ActiveView: "hidden", SplitRatio: 0.5})
	if !strings.Contains(out, `active_view = "hidden"`) {
		t.Errorf("active_view not updated:\n%s", out)
	}
	if !strings.Contains(out, "split_ratio = 0.5") {
		t.Errorf("split_ratio not inserted:\n%s", out)
	}
}
