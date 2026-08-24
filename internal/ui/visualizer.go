package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The visualizer draws the audio output. It has several modes: a frequency
// spectrum built from an FFT, a time-domain waveform, a stereo mirror
// spectrum, a radial bloom spectrum, and a beat-driven particle burst. It
// reads the most recent output samples from the player through a sample tap.
//
// The stereo spectrum, the radial bloom, the peak gravity, and the spectrum
// smoothing reimplement ideas from cli-visualizer
// (https://github.com/PosixAlchemist/cli-visualizer, MIT) and C.A.V.A. No
// code from those projects is copied; only the visual concepts are ported
// to Go on top of tuiplay's own FFT and sample tap. The visualizer keeps its
// own vivid palette; the UI theme does not change it.

// visMode names the visualizer modes.
type visMode int

const (
	visSpectrum visMode = iota
	visWave
	visSpectrumStereo
	visRadial
	visParticles
	visModeCount
)

// nextVisMode returns the next mode in the cycle, wrapping to the start.
func nextVisMode(m visMode) visMode {
	return (m + 1) % visModeCount
}

// visModeName returns the display name of a mode.
func visModeName(m visMode) string {
	switch m {
	case visWave:
		return "waveform"
	case visSpectrumStereo:
		return "spectrum stereo"
	case visRadial:
		return "radial bloom"
	case visParticles:
		return "beat sparks"
	default:
		return "spectrum"
	}
}

// The vivid palette. The visualizer colors a cell by three inputs: the
// spectral position f (0 = bass, 1 = treble), the loudness level (0..1), and
// a slowly drifting global hue phase. Bass reads warm (magenta/orange),
// treble reads cool (cyan/violet). Louder cells brighten toward white. A
// beat frame lifts every cell's brightness, so the whole scene flashes.

// vividHex returns a hex color for spectral position f in 0..1, loudness
// level in 0..1, and a hue phase offset. It is the visualizer's own palette.
func vividHex(f, level, phase float64) string {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	if level < 0 {
		level = 0
	}
	if level > 1 {
		level = 1
	}
	// Hue sweeps from ~0.92 (magenta) through orange, green, to ~0.66
	// (violet) as f goes bass->treble, drifting with the phase.
	hue := 0.92 - f*0.78 + phase
	r, g, b := hsv(hue, 1.0, 1.0)
	// Loudness brightens the cell: a quiet cell is dim, a loud cell washes
	// toward white. A gamma keeps midtones colorful.
	bright := 0.35 + 0.65*level
	white := level * level * 0.55 // loud cells desaturate toward white
	r = r*bright*(1-white) + white
	g = g*bright*(1-white) + white
	b = b*bright*(1-white) + white
	return fmt.Sprintf("#%02x%02x%02x", clamp8(r), clamp8(g), clamp8(b))
}

// clamp8 maps a 0..1 float to a 0..255 byte with clamping.
func clamp8(v float64) int {
	n := int(v * 255)
	if n < 0 {
		return 0
	}
	if n > 255 {
		return 255
	}
	return n
}

// hsv converts hue (wrapped to 0..1), saturation, and value in 0..1 to rgb
// in 0..1.
func hsv(h, s, v float64) (float64, float64, float64) {
	h = h - math.Floor(h)
	i := int(h * 6)
	fr := h*6 - float64(i)
	p := v * (1 - s)
	q := v * (1 - s*fr)
	t := v * (1 - s*(1-fr))
	switch i % 6 {
	case 0:
		return v, t, p
	case 1:
		return q, v, p
	case 2:
		return p, v, t
	case 3:
		return p, q, v
	case 4:
		return t, p, v
	default:
		return v, p, q
	}
}

// vividPhase is the extra hue offset applied to every cell on a beat frame,
// on top of the brightness lift. It shifts the whole palette briefly.
const vividBeatHue = 0.06

// beatThreshold is the base multiple by which instantaneous energy must
// exceed the smoothed baseline to count as a beat, at beat sensitivity 1.0.
// beatBaselineRise and beatBaselineFall set how fast the long-term baseline
// tracks energy up and down. The baseline is a slow average, so a short-term
// spike above it reads as a beat.
const (
	beatThreshold    = 1.20
	beatBaselineRise = 0.04
	beatBaselineFall = 0.02
)

// resolveVis fills any unset (zero) field of the visualizer settings with its
// built-in default, so a model built without config still animates. It
// mirrors config.VisualizerConfig.Resolved for the ui-side settings type.
func resolveVis(v VisualizerSettings) VisualizerSettings {
	if v.HueSpeed == 0 {
		v.HueSpeed = 1.0
	} else if v.HueSpeed < 0 {
		v.HueSpeed = 0
	}
	if v.BeatSensitivity <= 0 {
		v.BeatSensitivity = 1.0
	}
	v.SpectrumSmoothing = resolveKeep(v.SpectrumSmoothing, 0.80)
	if v.SpectrumPeakGravity <= 0 {
		v.SpectrumPeakGravity = 0.9
	}
	if v.SpectrumTilt == 0 {
		v.SpectrumTilt = defaultSpectrumTilt
	} else if v.SpectrumTilt < 0 {
		v.SpectrumTilt = 0
	}
	if v.SpectrumMonstercat == 0 {
		v.SpectrumMonstercat = defaultMonstercat
	}
	v.WaveFalloff = resolveKeep(v.WaveFalloff, 0.90)
	v.StereoSmoothing = resolveKeep(v.StereoSmoothing, 0.80)
	v.RadialDecay = resolveKeep(v.RadialDecay, 0.90)
	if v.RadialReach <= 0 {
		v.RadialReach = 1.0
	}
	if v.RadialCore <= 0 {
		v.RadialCore = 1.0
	}
	return v
}

// resolveKeep resolves a 0..1 retained-fraction: 0 means the default, a
// negative value means 0 (no smoothing), and a value at or above 1 clamps.
func resolveKeep(v, def float64) float64 {
	switch {
	case v == 0:
		return def
	case v < 0:
		return 0
	case v >= 1:
		return 0.999
	default:
		return v
	}
}

// advanceVis updates the per-frame animation state on the visualizer tick.
// It runs in Update, not in View, so View stays a pure function. It reads
// the latest audio window from the player once per frame.
func (m *model) advanceVis() {
	v := resolveVis(m.visSpark)

	// Drift the palette hue used across all modes, scaled by the config.
	m.visPhase += 0.004 * v.HueSpeed

	buf := make([]float64, visSampleWindow)
	n := m.player.Samples(buf)
	buf = buf[:n]
	energy := rms(buf)

	// Track a smoothed energy baseline and flag a beat when the current
	// energy jumps well above it. The baseline rises quickly and falls
	// slowly, so a sustained loud passage does not keep flashing. A higher
	// beat sensitivity lowers the trigger threshold.
	if energy > m.visEnergyBase {
		m.visEnergyBase += (energy - m.visEnergyBase) * beatBaselineRise
	} else {
		m.visEnergyBase += (energy - m.visEnergyBase) * beatBaselineFall
	}
	threshold := beatThreshold / v.BeatSensitivity
	m.visBeat = m.visEnergyBase > 1e-4 && energy > m.visEnergyBase*threshold

	width := m.width
	if width < 1 {
		width = 1
	}
	plotH := m.height - 1
	if plotH < 1 {
		plotH = 1
	}

	switch m.visMode {
	case visSpectrum:
		// Smooth the band levels toward the previous frame, then advance
		// the falling peak caps with gravity.
		raw := spectrumLevels(buf, width, v.SpectrumTilt, v.SpectrumMonstercat)
		if len(m.visLevels) != len(raw) {
			m.visLevels = make([]float64, len(raw))
		}
		for c := range raw {
			decayed := m.visLevels[c] * v.SpectrumSmoothing
			if raw[c] > decayed {
				m.visLevels[c] = raw[c]
			} else {
				m.visLevels[c] = decayed
			}
		}
		cols := spectrumColumns(m.visLevels, plotH)
		if len(m.visPeaks) != len(cols) {
			m.visPeaks = make([]int, len(cols))
			m.visPeakVel = make([]float64, len(cols))
		}
		if len(m.visPeakVel) != len(cols) {
			m.visPeakVel = make([]float64, len(cols))
		}
		for c := range cols {
			if cols[c] >= m.visPeaks[c] {
				m.visPeaks[c] = cols[c]
				m.visPeakVel[c] = 0
			} else {
				m.visPeakVel[c] += v.SpectrumPeakGravity
				m.visPeaks[c] -= int(m.visPeakVel[c])
				if m.visPeaks[c] < cols[c] {
					m.visPeaks[c] = cols[c]
					m.visPeakVel[c] = 0
				}
			}
		}
	case visWave:
		// Decay a per-column envelope slowly, so the waveform falls off
		// gradually instead of snapping to each new frame.
		amp := waveAmplitudes(buf, width)
		if len(m.visWave) != len(amp) {
			m.visWave = make([]float64, len(amp))
		}
		for c := range amp {
			decayed := m.visWave[c] * v.WaveFalloff
			if amp[c] > decayed {
				m.visWave[c] = amp[c]
			} else {
				m.visWave[c] = decayed
			}
		}
	case visSpectrumStereo:
		// Smooth the stereo band levels per channel toward the previous
		// frame so the mirrored bars glide instead of flickering.
		frames := make([][2]float64, visSampleWindow)
		fn := m.player.SamplesStereo(frames)
		left, right := splitStereo(frames[:fn])
		lraw := spectrumLevels(left, width, v.SpectrumTilt, v.SpectrumMonstercat)
		rraw := spectrumLevels(right, width, v.SpectrumTilt, v.SpectrumMonstercat)
		m.visStereoL = smoothInto(m.visStereoL, lraw, v.StereoSmoothing)
		m.visStereoR = smoothInto(m.visStereoR, rraw, v.StereoSmoothing)
	case visParticles:
		m.visParticles.configure(m.visSpark)
		m.visParticles.step(energy, m.visBeat, m.visPhase, width, plotH)
	case visRadial:
		// Smooth the radial band levels toward the previous frame so the
		// bloom falls off gradually. A rising band snaps up; a falling band
		// decays by the configured retained fraction.
		raw := spectrumLevels(buf, radialBands, v.SpectrumTilt, v.SpectrumMonstercat)
		m.visRadialLevels = smoothInto(m.visRadialLevels, raw, v.RadialDecay)
	}
}

// smoothInto decays prev toward raw: a rising value snaps up, a falling value
// decays by keep. It reuses prev when the lengths match and returns the
// updated slice.
func smoothInto(prev, raw []float64, keep float64) []float64 {
	if len(prev) != len(raw) {
		prev = make([]float64, len(raw))
	}
	for c := range raw {
		decayed := prev[c] * keep
		if raw[c] > decayed {
			prev[c] = raw[c]
		} else {
			prev[c] = decayed
		}
	}
	return prev
}

// splitStereo separates interleaved stereo frames into left and right
// channels.
func splitStereo(frames [][2]float64) (left, right []float64) {
	left = make([]float64, len(frames))
	right = make([]float64, len(frames))
	for i, f := range frames {
		left[i] = f[0]
		right[i] = f[1]
	}
	return left, right
}

// visBlocks are the eight partial-height block glyphs, low to high.
var visBlocks = []rune{' ', '▁', '▂', '▃', '▄', '▅', '▆', '▇', '█'}

// visGradientSteps is the number of precomputed colors in the height
// gradient. A cell picks the nearest step for its level.
const visGradientSteps = 24

// visStyles holds one foreground style per gradient step. lipgloss detects
// the terminal color profile and degrades the hex color to the best
// available (truecolor, 256, or 16), so the same styles adapt per terminal.
var visStyles = buildVisStyles()

// buildVisStyles precomputes the height gradient: green at the bottom,
// yellow in the middle, red at the top, like an audio level meter.
func buildVisStyles() []lipgloss.Style {
	styles := make([]lipgloss.Style, visGradientSteps)
	for i := range styles {
		f := float64(i) / float64(visGradientSteps-1)
		styles[i] = lipgloss.NewStyle().Foreground(lipgloss.Color(gradientHex(f)))
	}
	return styles
}

// gradientHex returns a hex color for the fraction f in 0..1 along a
// green -> yellow -> red ramp.
func gradientHex(f float64) string {
	if f < 0 {
		f = 0
	}
	if f > 1 {
		f = 1
	}
	var r, g, b float64
	if f < 0.5 {
		// green (0,204,0) -> yellow (221,204,0)
		t := f / 0.5
		r = 0 + t*221
		g = 204
		b = 0
	} else {
		// yellow (221,204,0) -> red (221,40,0)
		t := (f - 0.5) / 0.5
		r = 221
		g = 204 - t*164
		b = 0
	}
	return fmt.Sprintf("#%02x%02x%02x", int(r), int(g), int(b))
}

// visStyleFor returns the gradient style for a level in 0..maxLevel.
func visStyleFor(level, maxLevel int) lipgloss.Style {
	if maxLevel <= 0 {
		return visStyles[0]
	}
	idx := level * (visGradientSteps - 1) / maxLevel
	if idx < 0 {
		idx = 0
	}
	if idx >= visGradientSteps {
		idx = visGradientSteps - 1
	}
	return visStyles[idx]
}

// rainbowHex returns a hex color for the fraction f in 0..1 around the full
// hue wheel. It is kept as a small standalone hue helper.
func rainbowHex(f float64) string {
	f = f - math.Floor(f) // wrap to 0..1
	h := f * 6
	x := 1 - math.Abs(math.Mod(h, 2)-1)
	var r, g, b float64
	switch int(h) {
	case 0:
		r, g, b = 1, x, 0
	case 1:
		r, g, b = x, 1, 0
	case 2:
		r, g, b = 0, 1, x
	case 3:
		r, g, b = 0, x, 1
	case 4:
		r, g, b = x, 0, 1
	default:
		r, g, b = 1, 0, x
	}
	return fmt.Sprintf("#%02x%02x%02x", int(r*221), int(g*221), int(b*221))
}

// fftSize is the window size for the spectrum FFT. It is a power of two.
const fftSize = 1024

// visSampleWindow is the number of samples the visualizer requests from the
// player each frame.
const visSampleWindow = 4096

// hann returns the Hann window value at index i of n.
func hann(i, n int) float64 {
	return 0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1))
}

// fft computes the discrete Fourier transform of re and im in place. The
// length must be a power of two. It is a standard radix-2 Cooley-Tukey
// transform.
func fft(re, im []float64) {
	n := len(re)
	if n <= 1 {
		return
	}
	// Bit-reversal permutation.
	for i, j := 1, 0; i < n; i++ {
		bit := n >> 1
		for ; j&bit != 0; bit >>= 1 {
			j ^= bit
		}
		j ^= bit
		if i < j {
			re[i], re[j] = re[j], re[i]
			im[i], im[j] = im[j], im[i]
		}
	}
	// Danielson-Lanczos butterflies.
	for length := 2; length <= n; length <<= 1 {
		ang := -2 * math.Pi / float64(length)
		wReal, wImag := math.Cos(ang), math.Sin(ang)
		for i := 0; i < n; i += length {
			curReal, curImag := 1.0, 0.0
			for j := 0; j < length/2; j++ {
				a := i + j
				b := i + j + length/2
				tReal := curReal*re[b] - curImag*im[b]
				tImag := curReal*im[b] + curImag*re[b]
				re[b] = re[a] - tReal
				im[b] = im[a] - tImag
				re[a] += tReal
				im[a] += tImag
				curReal, curImag = curReal*wReal-curImag*wImag, curReal*wImag+curImag*wReal
			}
		}
	}
}

// spectrumLevels turns the sample window into width normalized band levels
// in 0..1. It applies a Hann window, an FFT, and groups the magnitude bins
// on a log scale. tilt lifts the higher bands (0 disables it); monster is the
// monstercat neighbor-bleed weight (<=1 disables the filter). renderBars
// scales the result for display.
func spectrumLevels(samples []float64, width int, tilt, monster float64) []float64 {
	if width < 1 {
		return nil
	}
	re := make([]float64, fftSize)
	im := make([]float64, fftSize)
	// Take the newest fftSize samples.
	n := len(samples)
	off := n - fftSize
	if off < 0 {
		off = 0
	}
	for i := 0; i < fftSize && off+i < n; i++ {
		re[i] = samples[off+i] * hann(i, fftSize)
	}
	fft(re, im)

	// Usable bins are 1..fftSize/2. Map them to columns on a log scale.
	bins := fftSize / 2
	levels := make([]float64, width)
	maxBin := float64(bins)
	for c := 0; c < width; c++ {
		// Log-spaced band edges over the bin range.
		lo := int(math.Pow(maxBin, float64(c)/float64(width)))
		hi := int(math.Pow(maxBin, float64(c+1)/float64(width)))
		if lo < 1 {
			lo = 1
		}
		if hi <= lo {
			hi = lo + 1
		}
		if hi > bins {
			hi = bins
		}
		var sum float64
		var cnt int
		for b := lo; b < hi; b++ {
			mag := math.Hypot(re[b], im[b])
			sum += mag
			cnt++
		}
		var mag float64
		if cnt > 0 {
			mag = sum / float64(cnt)
		}
		// Normalize the FFT magnitude by the window size, then map a
		// decibel range to 0..1. The FFT here is unnormalized, so a bin
		// magnitude scales with fftSize; divide it out first.
		norm := mag / float64(fftSize)
		db := 20 * math.Log10(norm+1e-9)
		// Map roughly -70..-10 dB to 0..1. Quiet noise sits near the floor
		// and loud peaks reach the top, so bars do not saturate.
		v := (db + 70) / 60
		// Perceptual tilt: lift the higher bands a little so a small window
		// is not dominated by a couple of fat bass bars. The tilt grows
		// with the column fraction.
		if tilt > 0 && width > 1 {
			v += tilt * float64(c) / float64(width-1)
		}
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		levels[c] = v
	}
	if monster > 1 {
		monstercat(levels, monster)
	}
	return levels
}

// defaultSpectrumTilt and defaultMonstercatWeight are the built-in spectrum
// tilt and neighbor-bleed weight, used where no config is threaded in.
const (
	defaultSpectrumTilt = 0.12
	defaultMonstercat   = 2.8
)

// monstercat smooths the bar levels so a strong bar lifts its neighbors,
// which removes single-column spikes. weight is the bleed strength; a higher
// value spreads less. It runs in place. This is the monstercat filter from
// cli-visualizer/C.A.V.A.
func monstercat(levels []float64, weight float64) {
	n := len(levels)
	for i := 0; i < n; i++ {
		for j := i - 1; j >= 0; j-- {
			d := float64(i - j)
			v := levels[i] / math.Pow(weight, d)
			if v > levels[j] {
				levels[j] = v
			} else {
				break
			}
		}
		for j := i + 1; j < n; j++ {
			d := float64(j - i)
			v := levels[i] / math.Pow(weight, d)
			if v > levels[j] {
				levels[j] = v
			} else {
				break
			}
		}
	}
}

// spectrumColumns turns band levels into per-column heights in eighths in
// the range 0..height*8. peaks, when non-nil, holds a decaying peak level
// per column that renderBars draws as a bright cap.
func spectrumColumns(levels []float64, height int) []int {
	if height < 1 {
		return nil
	}
	cols := make([]int, len(levels))
	for c := range levels {
		cols[c] = int(levels[c] * float64(height) * 8) // in eighths
	}
	return cols
}

// renderBars draws vertical bars from a per-column height in eighths.
// Each column height is 0..height*8. It doubles the vertical resolution with
// half-block glyphs, so a short window still shows smooth motion. peaks, when
// non-nil, holds a decaying peak height per column in eighths; renderBars
// draws a bright cap at each peak. beat brightens every cell for one frame.
// It returns height text rows.
func renderBars(cols, peaks []int, width, height int, phase float64, beat bool) string {
	if width < 1 || height < 1 {
		return ""
	}
	if beat {
		phase += vividBeatHue
	}
	subH := height * 2 // number of half-row sub-cells, bottom..top
	rows := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		// The two half-rows this text row covers, measured from the bottom.
		topSub := (height - row) * 2 // exclusive upper bound
		botSub := topSub - 2         // this row spans [botSub, botSub+2)
		for c := 0; c < width && c < len(cols); c++ {
			// Column fill height in half-rows, 0..subH.
			fillHalf := cols[c] * subH / (height * 8)
			var peakHalf int = -1
			if peaks != nil && c < len(peaks) && peaks[c] > cols[c] {
				peakHalf = peaks[c] * subH / (height * 8)
			}
			// Spectral position drives the hue; loudness drives brightness.
			var f float64
			if width > 1 {
				f = float64(c) / float64(width-1)
			}
			level := float64(cols[c]) / float64(height*8)
			style := lipgloss.NewStyle().Foreground(lipgloss.Color(vividHex(f, level, phase)))
			lower := fillHalf > botSub   // bottom half of this cell filled
			upper := fillHalf > botSub+1 // top half filled
			// A peak cap sitting on this cell overrides an empty top half.
			capHere := peakHalf == botSub+1 || peakHalf == botSub+2
			switch {
			case upper:
				b.WriteString(style.Render("█"))
			case lower && capHere:
				b.WriteString(style.Render("█"))
			case lower:
				b.WriteString(style.Render("▄"))
			case capHere:
				capStyle := lipgloss.NewStyle().Foreground(lipgloss.Color(vividHex(f, 1, phase)))
				b.WriteString(capStyle.Render("▔"))
			default:
				b.WriteRune(' ')
			}
		}
		rows[row] = b.String()
	}
	return strings.Join(rows, "\n")
}

// stereoSpectrumRows draws two mirrored spectra: the left channel grows up
// from a center line and the right channel grows down. It copies the stereo
// spectrum idea from cli-visualizer. It colors with the vivid palette; a beat
// brightens the whole scene. leftLevels and rightLevels are the smoothed
// per-channel band levels; both hold width entries.
func stereoSpectrumRows(leftLevels, rightLevels []float64, width, height int, phase float64, beat bool) string {
	if width < 1 || height < 1 {
		return ""
	}
	if beat {
		phase += vividBeatHue
	}
	half := height / 2
	if half < 1 {
		half = 1
	}
	ll := spectrumColumns(leftLevels, half)
	rl := spectrumColumns(rightLevels, half)

	rows := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		if row < half {
			// Upper half: left channel, filling down toward the center.
			levelFromCenter := half - 1 - row
			for c := 0; c < width && c < len(ll); c++ {
				b.WriteString(spectrumCell(ll[c], levelFromCenter, half, c, width, phase))
			}
		} else {
			// Lower half: right channel, filling down from the center.
			levelFromCenter := row - half
			for c := 0; c < width && c < len(rl); c++ {
				b.WriteString(spectrumCell(rl[c], levelFromCenter, half, c, width, phase))
			}
		}
		rows[row] = b.String()
	}
	return strings.Join(rows, "\n")
}

// spectrumCell renders one bar cell given the column height in eighths and
// the level of this row measured from the bar base. It colors the cell with
// the vivid palette from the column's spectral position and its loudness.
func spectrumCell(colEighths, levelFromBase, height, col, width int, phase float64) string {
	eighths := colEighths - levelFromBase*8
	var r rune
	switch {
	case eighths <= 0:
		return " "
	case eighths >= 8:
		r = '█'
	default:
		r = visBlocks[eighths]
	}
	var f float64
	if width > 1 {
		f = float64(col) / float64(width-1)
	}
	level := float64(colEighths) / float64(height*8)
	style := lipgloss.NewStyle().Foreground(lipgloss.Color(vividHex(f, level, phase)))
	return style.Render(string(r))
}

// rms returns the root-mean-square amplitude of the sample window in 0..1.
func rms(samples []float64) float64 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, s := range samples {
		sum += s * s
	}
	return math.Sqrt(sum / float64(len(samples)))
}

// cellAspect corrects for terminal cells being about twice as tall as they
// are wide, so a circle reads as round rather than as an egg.
const cellAspect = 2.0

// radialBands is the number of spectrum bands the radial bloom maps around
// the circle. It is mirrored, so the bloom is symmetric.
const radialBands = 64

// radialRows draws a spectrum that radiates from the center outward around a
// circle. Each angle owns a spectrum band; the band level sets how far the
// ray reaches from the center. It colors rays with the vivid palette and
// brightens on a beat. It fills densely even in a small window.
//
// levels holds the smoothed band levels (radialBands entries). reach scales
// the ray length and core scales the pulsing inner radius; both default to 1.
func radialRows(levels []float64, energy, phase float64, beat bool, reach, coreScale float64, width, height int) string {
	if width < 1 || height < 1 {
		return strings.Repeat("\n", height-1)
	}
	if len(levels) < radialBands {
		return strings.Repeat("\n", height-1)
	}
	if beat {
		phase += vividBeatHue
	}
	if reach <= 0 {
		reach = 1
	}
	if coreScale <= 0 {
		coreScale = 1
	}
	cx := float64(width-1) / 2
	cy := float64(height-1) / 2

	// A pulsing inner core radius that grows with loudness.
	core := (0.10 + energy*0.25) * coreScale

	grid := make([][]rune, height)
	styleAt := make([][]lipgloss.Style, height)
	for y := 0; y < height; y++ {
		grid[y] = make([]rune, width)
		styleAt[y] = make([]lipgloss.Style, width)
		for x := 0; x < width; x++ {
			grid[y][x] = ' '
			dx := float64(x) - cx
			dy := (float64(y) - cy) * cellAspect
			dist := math.Hypot(dx, dy) / (cy * cellAspect)
			ang := math.Atan2(dy, dx) + math.Pi // 0..2pi
			t := ang / (2 * math.Pi)
			bi := int(t * radialBands)
			if bi >= radialBands {
				bi = radialBands - 1
			}
			// Mirror so opposite sides match, giving a symmetric bloom.
			mi := bi
			if mi >= radialBands/2 {
				mi = radialBands - 1 - mi
			}
			rayReach := core + levels[mi]*0.85*reach
			if dist <= rayReach {
				// Bright toward the tip, hot toward the core.
				level := 1 - dist/(rayReach+1e-6)
				grid[y][x] = '█'
				// Spectral position from the band index for the hue.
				f := float64(mi) / float64(radialBands/2)
				styleAt[y][x] = lipgloss.NewStyle().Foreground(
					lipgloss.Color(vividHex(f, 0.4+level*0.6, phase)))
			}
		}
	}
	return renderGrid(grid, styleAt)
}

// particle is one spark: a position and velocity on the braille sub-canvas,
// with a remaining life in frames. Its hue is fixed at spawn. trail holds the
// most recent past positions (newest first) for a streak, when enabled.
type particle struct {
	x, y   float64 // sub-canvas coordinates
	vx, vy float64
	life   int
	maxLd  int
	hue    float64
	trail  [][2]float64
}

// particleState holds the live sparks and the resolved tunables. Beats spawn
// new sparks from the center; each spark flies out under gravity, fades over
// its life, and may leave a fading streak.
type particleState struct {
	ps   []particle
	seed uint64
	cfg  sparkParams
}

// sparkParams holds the effective, resolved spark tunables the particle
// system reads. configure fills it, applying defaults for a zero value.
type sparkParams struct {
	life    int
	speed   float64
	gravity float64
	count   float64
	trail   int
}

// particleMax caps the live spark count so a long loud passage does not
// accumulate unbounded work.
const particleMax = 600

// configure sets the resolved spark tunables. It is safe to call every frame;
// it only copies values. A zero VisualizerSettings yields the built-in
// defaults, so an unconfigured state still animates.
func (s *particleState) configure(v VisualizerSettings) {
	p := sparkParams{
		life:    v.SparkLife,
		speed:   v.SparkSpeed,
		gravity: v.SparkGravity,
		count:   v.SparkCount,
		trail:   v.SparkTrail,
	}
	if p.life <= 0 {
		p.life = 45
	}
	if p.speed <= 0 {
		p.speed = 1.0
	}
	if p.gravity == 0 {
		p.gravity = 0.045
	} else if p.gravity < 0 {
		p.gravity = 0
	}
	if p.count <= 0 {
		p.count = 1.0
	}
	if p.trail < 0 {
		p.trail = 0
	}
	s.cfg = p
}

// rngNext is a small xorshift PRNG. The visualizer does not need crypto
// randomness; it needs cheap, deterministic-per-seed spread.
func (s *particleState) rngNext() uint64 {
	if s.seed == 0 {
		s.seed = 0x9e3779b97f4a7c15
	}
	s.seed ^= s.seed << 13
	s.seed ^= s.seed >> 7
	s.seed ^= s.seed << 17
	return s.seed
}

// randUnit returns a float in 0..1 from the PRNG.
func (s *particleState) randUnit() float64 {
	return float64(s.rngNext()>>11) / float64(1<<53)
}

// step advances the sparks. On a beat it spawns a burst whose size and speed
// scale with the audio energy and the configured tunables. It integrates
// every live spark, records its trail, and drops the dead ones. width and
// height are the text-cell dimensions.
func (s *particleState) step(energy float64, beat bool, phase float64, width, height int) {
	if s.cfg.life == 0 {
		s.configure(VisualizerSettings{})
	}
	dw := float64(width * 2)
	dh := float64(height * 4)
	cx := dw / 2
	cy := dh / 2

	// Move and age the live sparks.
	live := s.ps[:0]
	for _, p := range s.ps {
		// Record the trail before moving, newest first, capped to the
		// configured streak length.
		if s.cfg.trail > 0 {
			p.trail = append(p.trail, [2]float64{p.x, p.y})
			if len(p.trail) > s.cfg.trail {
				p.trail = p.trail[len(p.trail)-s.cfg.trail:]
			}
		}
		p.vy += s.cfg.gravity
		p.x += p.vx
		p.y += p.vy
		p.life--
		if p.life > 0 && p.x >= 0 && p.x < dw && p.y >= 0 && p.y < dh {
			live = append(live, p)
		}
	}
	s.ps = live

	// Spawn on a beat, sized by the audio energy. Also emit a light
	// continuous trickle so the field is never empty between beats, scaled
	// by the current loudness.
	spawn := 0
	if beat {
		spawn = int(28 + energy*260)
	} else if energy > 0.01 {
		// A light continuous trickle above a small noise floor, so the
		// field is never empty during playback but stays dark in silence.
		spawn = int(3 + energy*70)
	}
	spawn = int(float64(spawn) * s.cfg.count)
	if spawn > 0 && len(s.ps) < particleMax {
		// Scale the launch speed to the canvas so sparks reach the edges of
		// a wide or a narrow window in about the same number of frames,
		// instead of clustering in the center. A loudness term adds punch,
		// and the configured speed scales the whole throw.
		reach := dh / 2
		base := (reach/float64(s.cfg.life)*2.2 + energy*float64(reach)*0.04) * s.cfg.speed
		for i := 0; i < spawn && len(s.ps) < particleMax; i++ {
			ang := s.randUnit() * 2 * math.Pi
			sp := base * (0.55 + 0.45*s.randUnit())
			var tr [][2]float64
			if s.cfg.trail > 0 {
				tr = make([][2]float64, 0, s.cfg.trail)
			}
			s.ps = append(s.ps, particle{
				x:     cx,
				y:     cy,
				vx:    math.Cos(ang) * sp * cellAspect, // widen for cell aspect
				vy:    math.Sin(ang) * sp,
				life:  s.cfg.life,
				maxLd: s.cfg.life,
				hue:   phase + s.randUnit()*0.3,
				trail: tr,
			})
		}
	}
}

// plotSpark lights the braille sub-cell at (px,py) with the given brightness
// and hue, keeping the brightest contributor per cell.
func plotSpark(px, py, dw, dh, width int, dots []uint8, bright, hueAt []float64, b, hue float64) {
	if px < 0 || px >= dw || py < 0 || py >= dh {
		return
	}
	cell := (py/4)*width + (px / 2)
	dots[cell] |= brailleBit(px%2, py%4)
	if b > bright[cell] {
		bright[cell] = b
		hueAt[cell] = hue
	}
}

// particleRows renders the live sparks onto the braille sub-canvas. A spark
// dims as its life runs out. A trail draws behind it, dimming with age. beat
// brightens the whole burst.
func particleRows(st *particleState, phase float64, beat bool, width, height int) string {
	if width < 1 || height < 1 {
		return strings.Repeat("\n", height-1)
	}
	dw := width * 2
	dh := height * 4
	dots := make([]uint8, width*height)
	styleAt := make([][]lipgloss.Style, height)
	bright := make([]float64, width*height)
	hueAt := make([]float64, width*height)
	for y := range styleAt {
		styleAt[y] = make([]lipgloss.Style, width)
	}
	if st != nil {
		for _, p := range st.ps {
			maxLd := p.maxLd
			if maxLd <= 0 {
				maxLd = 45
			}
			head := float64(p.life) / float64(maxLd)
			// Draw the streak first, dimmer toward the tail (older points).
			n := len(p.trail)
			for i, pt := range p.trail {
				// Newer trail points sit later in the slice, so brightness
				// grows toward the head.
				age := float64(i+1) / float64(n+1)
				plotSpark(int(pt[0]), int(pt[1]), dw, dh, width, dots, bright, hueAt, head*age*0.8, p.hue)
			}
			// Draw the head last so it wins the cell.
			plotSpark(int(p.x), int(p.y), dw, dh, width, dots, bright, hueAt, head, p.hue)
		}
	}
	extra := 0.0
	if beat {
		extra = vividBeatHue
	}
	rows := make([]string, height)
	for y := 0; y < height; y++ {
		var b strings.Builder
		for x := 0; x < width; x++ {
			cell := y*width + x
			d := dots[cell]
			if d == 0 {
				b.WriteRune(' ')
				continue
			}
			style := lipgloss.NewStyle().Foreground(
				lipgloss.Color(vividHex(0.5, 0.3+bright[cell]*0.7, hueAt[cell]+extra)))
			b.WriteString(style.Render(string(rune(0x2800 + int(d)))))
		}
		rows[y] = b.String()
	}
	return strings.Join(rows, "\n")
}

// brailleBit returns the braille dot bit for a sub-cell at column dx (0..1)
// and row dy (0..3). The braille dot numbering is column-major.
func brailleBit(dx, dy int) uint8 {
	// Dot layout: (0,0)=1 (1,0)=8 (0,1)=2 (1,1)=16 (0,2)=4 (1,2)=32
	//             (0,3)=64 (1,3)=128
	switch {
	case dx == 0 && dy == 0:
		return 0x01
	case dx == 0 && dy == 1:
		return 0x02
	case dx == 0 && dy == 2:
		return 0x04
	case dx == 1 && dy == 0:
		return 0x08
	case dx == 1 && dy == 1:
		return 0x10
	case dx == 1 && dy == 2:
		return 0x20
	case dx == 0 && dy == 3:
		return 0x40
	default: // dx == 1 && dy == 3
		return 0x80
	}
}

// renderGrid renders a rune grid with a matching style grid to text rows.
// A space cell is written plain.
func renderGrid(grid [][]rune, styleAt [][]lipgloss.Style) string {
	rows := make([]string, len(grid))
	for y := range grid {
		var b strings.Builder
		for x := range grid[y] {
			r := grid[y][x]
			if r == ' ' || r == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteString(styleAt[y][x].Render(string(r)))
		}
		rows[y] = b.String()
	}
	return strings.Join(rows, "\n")
}

// waveAmplitudes returns the per-column peak absolute amplitude in 0..1.
// Each column takes the peak of its slice of the sample window.
func waveAmplitudes(samples []float64, width int) []float64 {
	amp := make([]float64, width)
	n := len(samples)
	for c := 0; c < width; c++ {
		lo := c * n / width
		hi := (c + 1) * n / width
		if hi > n {
			hi = n
		}
		var peak float64
		for i := lo; i < hi; i++ {
			a := samples[i]
			if a < 0 {
				a = -a
			}
			if a > peak {
				peak = a
			}
		}
		amp[c] = peak
	}
	return amp
}

// measured from the center line. It returns width values in 0..height/2*8.
func waveRows(samples []float64, envelope []float64, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	half := height / 2
	if half < 1 {
		half = 1
	}
	// Per-column amplitude, lifted by the slow-decaying envelope so the
	// waveform falls off gradually instead of snapping down each frame.
	amp := waveAmplitudes(samples, width)
	for c := range amp {
		if c < len(envelope) && envelope[c] > amp[c] {
			amp[c] = envelope[c]
		}
	}
	rows := make([]string, height)
	for row := 0; row < height; row++ {
		// Distance in rows from the center line.
		dist := row - half
		if dist < 0 {
			dist = -dist
		}
		style := visStyleFor(dist, half)
		var b strings.Builder
		for c := 0; c < width; c++ {
			// reach is the amplitude in eighths of a row from the center.
			reach := int(amp[c] * float64(half) * 8)
			cellBase := dist * 8
			eighths := reach - cellBase
			switch {
			case dist == 0:
				b.WriteString(style.Render("█"))
			case eighths >= 8:
				b.WriteString(style.Render("█"))
			case eighths > 0:
				// Partial block at the outer edge of the silhouette. Above
				// the center the fill sits at the bottom of the cell (toward
				// the center line); below the center it sits at the top.
				if row < half {
					b.WriteString(style.Render(string(visBlocks[eighths])))
				} else {
					b.WriteString(style.Render("▀"))
				}
			default:
				b.WriteRune(' ')
			}
		}
		rows[row] = b.String()
	}
	return strings.Join(rows, "\n")
}
