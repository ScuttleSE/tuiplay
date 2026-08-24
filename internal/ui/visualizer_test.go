package ui

import (
	"math"
	"strings"
	"testing"
)

// TestFFTImpulse checks that the FFT of a unit impulse is a flat spectrum.
func TestFFTImpulse(t *testing.T) {
	n := 8
	re := make([]float64, n)
	im := make([]float64, n)
	re[0] = 1
	fft(re, im)
	for i := 0; i < n; i++ {
		mag := math.Hypot(re[i], im[i])
		if math.Abs(mag-1) > 1e-9 {
			t.Errorf("bin %d magnitude = %v, want 1", i, mag)
		}
	}
}

// TestFFTSingleTone checks that a pure cosine puts energy in one bin.
func TestFFTSingleTone(t *testing.T) {
	n := 16
	k := 3 // frequency bin
	re := make([]float64, n)
	im := make([]float64, n)
	for i := 0; i < n; i++ {
		re[i] = math.Cos(2 * math.Pi * float64(k) * float64(i) / float64(n))
	}
	fft(re, im)
	// Energy should sit at bins k and n-k, and be near zero elsewhere.
	for i := 0; i < n; i++ {
		mag := math.Hypot(re[i], im[i])
		if i == k || i == n-k {
			if mag < float64(n)/2-1e-6 {
				t.Errorf("bin %d magnitude = %v, want ~%v", i, mag, float64(n)/2)
			}
		} else if mag > 1e-9 {
			t.Errorf("bin %d magnitude = %v, want ~0", i, mag)
		}
	}
}

// TestGradientHexEndpoints checks the color ramp endpoints and midpoint.
func TestGradientHexEndpoints(t *testing.T) {
	if got := gradientHex(0); got != "#00cc00" {
		t.Errorf("gradientHex(0) = %s, want #00cc00", got)
	}
	if got := gradientHex(1); got != "#dd2800" {
		t.Errorf("gradientHex(1) = %s, want #dd2800", got)
	}
	if got := gradientHex(0.5); got != "#ddcc00" {
		t.Errorf("gradientHex(0.5) = %s, want #ddcc00", got)
	}
	// Out-of-range values clamp.
	if gradientHex(-1) != "#00cc00" || gradientHex(2) != "#dd2800" {
		t.Errorf("gradientHex did not clamp out-of-range input")
	}
}

// TestVisStyleForClamps checks the level-to-style mapping stays in bounds.
func TestVisStyleForClamps(t *testing.T) {
	_ = visStyleFor(-5, 10)
	_ = visStyleFor(100, 10)
	_ = visStyleFor(0, 0)
}

func TestSpectrumColumnsWidth(t *testing.T) {
	samples := make([]float64, visSampleWindow)
	for i := range samples {
		samples[i] = math.Sin(float64(i) * 0.1)
	}
	cols := spectrumColumns(spectrumLevels(samples, 20, defaultSpectrumTilt, defaultMonstercat), 10)
	if len(cols) != 20 {
		t.Fatalf("len(cols) = %d, want 20", len(cols))
	}
	rows := renderBars(cols, nil, 20, 10, 0, false)
	if rows == "" {
		t.Fatal("renderBars returned empty")
	}
}

// TestRenderBarsRowCount checks renderBars returns the requested row count.
func TestRenderBarsRowCount(t *testing.T) {
	cols := make([]int, 30)
	for i := range cols {
		cols[i] = i * 8
	}
	peaks := make([]int, 30)
	rows := renderBars(cols, peaks, 30, 8, 0.2, true)
	if got := len(splitLines(rows)); got != 8 {
		t.Fatalf("renderBars rows = %d, want 8", got)
	}
}

// TestVividHexClamps checks the vivid palette stays in range and produces a
// valid hex string for edge inputs.
func TestVividHexClamps(t *testing.T) {
	cases := [][3]float64{{-1, -1, 0}, {2, 2, 0.5}, {0, 0, 10}, {0.5, 0.5, -3}}
	for _, c := range cases {
		got := vividHex(c[0], c[1], c[2])
		if len(got) != 7 || got[0] != '#' {
			t.Fatalf("vividHex(%v) = %q, want #rrggbb", c, got)
		}
	}
}

// TestRadialRowCount checks the radial bloom returns the requested rows for a
// small window.
func TestRadialRowCount(t *testing.T) {
	samples := make([]float64, visSampleWindow)
	for i := range samples {
		samples[i] = math.Sin(float64(i) * 0.05)
	}
	levels := spectrumLevels(samples, radialBands, defaultSpectrumTilt, defaultMonstercat)
	rows := radialRows(levels, rms(samples), 0.1, true, 1.0, 1.0, 24, 8)
	if got := len(splitLines(rows)); got != 8 {
		t.Fatalf("radialRows rows = %d, want 8", got)
	}
	// A larger reach should not shrink the number of lit cells.
	small := countLit(radialRows(levels, rms(samples), 0.1, false, 0.5, 1.0, 24, 8))
	big := countLit(radialRows(levels, rms(samples), 0.1, false, 2.0, 1.0, 24, 8))
	if big < small {
		t.Fatalf("larger reach lit fewer cells: small=%d big=%d", small, big)
	}
}

// countLit counts the non-space runes in rendered rows.
func countLit(s string) int {
	n := 0
	for _, r := range s {
		if r != ' ' && r != '\n' {
			n++
		}
	}
	return n
}

// TestParticleStepAndRender checks a beat spawns sparks, the cap holds, and
// the renderer returns the requested rows.
func TestParticleStepAndRender(t *testing.T) {
	var st particleState
	// Silence spawns nothing: no beat and zero energy.
	st.step(0, false, 0, 24, 8)
	if len(st.ps) != 0 {
		t.Fatalf("silent step spawned %d sparks, want 0", len(st.ps))
	}
	// A beat spawns sparks; many beats stay under the cap.
	for i := 0; i < 50; i++ {
		st.step(1.0, true, 0.1, 24, 8)
	}
	if len(st.ps) == 0 {
		t.Fatal("beat produced no sparks")
	}
	if len(st.ps) > particleMax {
		t.Fatalf("spark count %d exceeds cap %d", len(st.ps), particleMax)
	}
	rows := particleRows(&st, 0.1, true, 24, 8)
	if got := len(splitLines(rows)); got != 8 {
		t.Fatalf("particleRows rows = %d, want 8", got)
	}
}

// splitLines splits rendered rows on newlines.
func splitLines(s string) []string {
	return strings.Split(s, "\n")
}

// TestParticleTrailStreak checks that enabling a trail records past positions
// and the renderer still returns the requested rows.
func TestParticleTrailStreak(t *testing.T) {
	var st particleState
	st.configure(VisualizerSettings{SparkLife: 60, SparkTrail: 5, SparkSpeed: 1.4})
	// Spawn on beats, then advance so trails accumulate.
	for i := 0; i < 20; i++ {
		st.step(1.0, i%3 == 0, 0.1, 40, 12)
	}
	if len(st.ps) == 0 {
		t.Fatal("no sparks after beats")
	}
	sawTrail := false
	for _, p := range st.ps {
		if len(p.trail) > 0 {
			sawTrail = true
		}
		if len(p.trail) > 5 {
			t.Fatalf("trail length %d exceeds cap 5", len(p.trail))
		}
	}
	if !sawTrail {
		t.Fatal("trail enabled but no spark recorded a trail")
	}
	rows := particleRows(&st, 0.1, false, 40, 12)
	if got := len(splitLines(rows)); got != 12 {
		t.Fatalf("particleRows rows = %d, want 12", got)
	}
}

// TestParticleConfigureDefaults checks configure fills defaults for a zero
// settings value and clamps a negative gravity to zero.
func TestParticleConfigureDefaults(t *testing.T) {
	var st particleState
	st.configure(VisualizerSettings{})
	if st.cfg.life <= 0 || st.cfg.speed <= 0 || st.cfg.gravity <= 0 || st.cfg.count <= 0 {
		t.Fatalf("configure left an unset field: %+v", st.cfg)
	}
	st.configure(VisualizerSettings{SparkGravity: -1})
	if st.cfg.gravity != 0 {
		t.Fatalf("negative gravity should clamp to 0, got %v", st.cfg.gravity)
	}
}

func TestRainbowHexWraps(t *testing.T) {
	// The hue wheel wraps, so f and f+1 give the same color.
	if rainbowHex(0.25) != rainbowHex(1.25) {
		t.Errorf("rainbowHex did not wrap")
	}
}

// TestResolveVisDefaults checks resolveVis fills defaults for a zero settings
// value and clamps a negative hue speed to zero.
func TestResolveVisDefaults(t *testing.T) {
	v := resolveVis(VisualizerSettings{})
	if v.HueSpeed <= 0 || v.BeatSensitivity <= 0 || v.SpectrumSmoothing <= 0 ||
		v.SpectrumPeakGravity <= 0 || v.SpectrumTilt <= 0 || v.SpectrumMonstercat <= 0 ||
		v.WaveFalloff <= 0 || v.StereoSmoothing <= 0 || v.RadialDecay <= 0 ||
		v.RadialReach <= 0 || v.RadialCore <= 0 {
		t.Fatalf("resolveVis left an unset field: %+v", v)
	}
	if got := resolveVis(VisualizerSettings{HueSpeed: -1}).HueSpeed; got != 0 {
		t.Fatalf("negative hue speed should clamp to 0, got %v", got)
	}
}

// TestSpectrumTiltAndMonstercat checks the tunable spectrum path: a zero tilt
// and a disabled monstercat still return width band levels in range.
func TestSpectrumTiltAndMonstercat(t *testing.T) {
	samples := make([]float64, visSampleWindow)
	for i := range samples {
		samples[i] = math.Sin(float64(i) * 0.1)
	}
	// tilt 0, monstercat disabled (<=1).
	lv := spectrumLevels(samples, 24, 0, 1)
	if len(lv) != 24 {
		t.Fatalf("len = %d, want 24", len(lv))
	}
	for i, x := range lv {
		if x < 0 || x > 1 {
			t.Fatalf("level %d out of range: %v", i, x)
		}
	}
}
