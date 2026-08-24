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

// TestVisualizerResolvedDefaults checks a zero visualizer config resolves to
// the built-in defaults.
func TestVisualizerResolvedDefaults(t *testing.T) {
	r := (VisualizerConfig{}).Resolved()
	if r.SparkLife != defaultSparkLife {
		t.Errorf("SparkLife = %d, want %d", r.SparkLife, defaultSparkLife)
	}
	if r.SparkSpeed != defaultSparkSpeed {
		t.Errorf("SparkSpeed = %v, want %v", r.SparkSpeed, defaultSparkSpeed)
	}
	if r.SparkGravity != defaultSparkGravity {
		t.Errorf("SparkGravity = %v, want %v", r.SparkGravity, defaultSparkGravity)
	}
	if r.SparkCount != defaultSparkCount {
		t.Errorf("SparkCount = %v, want %v", r.SparkCount, defaultSparkCount)
	}
	if r.SparkTrail != 0 {
		t.Errorf("SparkTrail = %d, want 0", r.SparkTrail)
	}
	if r.RadialDecay != defaultRadialDecay {
		t.Errorf("RadialDecay = %v, want %v", r.RadialDecay, defaultRadialDecay)
	}
	if r.RadialReach != defaultRadialReach {
		t.Errorf("RadialReach = %v, want %v", r.RadialReach, defaultRadialReach)
	}
	if r.RadialCore != defaultRadialCore {
		t.Errorf("RadialCore = %v, want %v", r.RadialCore, defaultRadialCore)
	}
	if r.HueSpeed != defaultHueSpeed {
		t.Errorf("HueSpeed = %v, want %v", r.HueSpeed, defaultHueSpeed)
	}
	if r.BeatSensitivity != defaultBeatSensitivity {
		t.Errorf("BeatSensitivity = %v, want %v", r.BeatSensitivity, defaultBeatSensitivity)
	}
	if r.SpectrumSmoothing != defaultSpectrumSmoothing {
		t.Errorf("SpectrumSmoothing = %v, want %v", r.SpectrumSmoothing, defaultSpectrumSmoothing)
	}
	if r.SpectrumPeakGravity != defaultSpectrumPeakGravity {
		t.Errorf("SpectrumPeakGravity = %v, want %v", r.SpectrumPeakGravity, defaultSpectrumPeakGravity)
	}
	if r.SpectrumTilt != defaultSpectrumTilt {
		t.Errorf("SpectrumTilt = %v, want %v", r.SpectrumTilt, defaultSpectrumTilt)
	}
	if r.SpectrumMonstercat != defaultSpectrumMonstercat {
		t.Errorf("SpectrumMonstercat = %v, want %v", r.SpectrumMonstercat, defaultSpectrumMonstercat)
	}
	if r.WaveFalloff != defaultWaveFalloff {
		t.Errorf("WaveFalloff = %v, want %v", r.WaveFalloff, defaultWaveFalloff)
	}
	if r.StereoSmoothing != defaultStereoSmoothing {
		t.Errorf("StereoSmoothing = %v, want %v", r.StereoSmoothing, defaultStereoSmoothing)
	}
}

// TestVisualizerResolvedOverrides checks explicit values pass through and a
// negative gravity means "no gravity".
func TestVisualizerResolvedOverrides(t *testing.T) {
	r := VisualizerConfig{
		SparkLife:    60,
		SparkSpeed:   1.5,
		SparkGravity: -1,
		SparkCount:   2,
		SparkTrail:   6,
		RadialDecay:  0.98,
		RadialReach:  1.5,
		RadialCore:   0.5,
	}.Resolved()
	if r.SparkLife != 60 || r.SparkSpeed != 1.5 || r.SparkCount != 2 || r.SparkTrail != 6 {
		t.Errorf("overrides not preserved: %+v", r)
	}
	if r.SparkGravity != 0 {
		t.Errorf("negative gravity should clamp to 0, got %v", r.SparkGravity)
	}
	if r.RadialDecay != 0.98 || r.RadialReach != 1.5 || r.RadialCore != 0.5 {
		t.Errorf("radial overrides not preserved: %+v", r)
	}
	// A radial_decay at or above 1 clamps to just under 1.
	if got := (VisualizerConfig{RadialDecay: 1.5}).Resolved().RadialDecay; got >= 1 {
		t.Errorf("RadialDecay = %v, want < 1", got)
	}
	// A negative radial_decay disables smoothing (instant falloff).
	if got := (VisualizerConfig{RadialDecay: -1}).Resolved().RadialDecay; got != 0 {
		t.Errorf("RadialDecay = %v, want 0 for negative input", got)
	}
	// Shared and per-mode tunables pass through and clamp sensibly.
	r2 := VisualizerConfig{
		HueSpeed:            2,
		BeatSensitivity:     1.5,
		SpectrumSmoothing:   0.95,
		SpectrumPeakGravity: 1.4,
		SpectrumTilt:        -1, // clamps to 0
		SpectrumMonstercat:  2.0,
		WaveFalloff:         0.7,
		StereoSmoothing:     1.5, // clamps below 1
	}.Resolved()
	if r2.HueSpeed != 2 || r2.BeatSensitivity != 1.5 || r2.SpectrumSmoothing != 0.95 ||
		r2.SpectrumPeakGravity != 1.4 || r2.SpectrumMonstercat != 2.0 || r2.WaveFalloff != 0.7 {
		t.Errorf("tunable overrides not preserved: %+v", r2)
	}
	if r2.SpectrumTilt != 0 {
		t.Errorf("negative tilt should clamp to 0, got %v", r2.SpectrumTilt)
	}
	if r2.StereoSmoothing >= 1 {
		t.Errorf("stereo_smoothing >= 1 should clamp, got %v", r2.StereoSmoothing)
	}
	// A negative hue_speed freezes the hue (clamps to 0).
	if got := (VisualizerConfig{HueSpeed: -1}).Resolved().HueSpeed; got != 0 {
		t.Errorf("HueSpeed = %v, want 0 for negative input", got)
	}
}
