package ui

import (
	"math"
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
	cols := spectrumColumns(spectrumLevels(samples, 20), 10)
	if len(cols) != 20 {
		t.Fatalf("len(cols) = %d, want 20", len(cols))
	}
	rows := renderBars(cols, nil, 20, 10)
	if rows == "" {
		t.Fatal("renderBars returned empty")
	}
}

func TestRainbowHexWraps(t *testing.T) {
	// The hue wheel wraps, so f and f+1 give the same color.
	if rainbowHex(0.25) != rainbowHex(1.25) {
		t.Errorf("rainbowHex did not wrap")
	}
}

func TestLorenzStep(t *testing.T) {
	var s lorenzState
	s.reset()
	s.step(0.5)
	if len(s.trail) == 0 {
		t.Fatal("step produced no trail points")
	}
	// The trail is capped.
	for i := 0; i < 20; i++ {
		s.step(1.0)
	}
	if len(s.trail) > lorenzTrailLen {
		t.Errorf("trail length %d exceeds cap %d", len(s.trail), lorenzTrailLen)
	}
}
