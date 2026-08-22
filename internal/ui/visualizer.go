package ui

import (
	"fmt"
	"math"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// The visualizer draws the audio output. It has several modes: a frequency
// spectrum built from an FFT, a time-domain waveform, a stereo mirror
// spectrum, a pulsing ellipse, and a Lorenz attractor. It reads the most
// recent output samples from the player through a sample tap.
//
// The ellipse, Lorenz, stereo spectrum, and the spectrum smoothing and
// peak-falloff effects reimplement ideas from cli-visualizer
// (https://github.com/PosixAlchemist/cli-visualizer, MIT) and C.A.V.A. No
// code from those projects is copied; only the visual concepts are ported
// to Go on top of tuiplay's own FFT and sample tap.

// visMode names the visualizer modes.
type visMode int

const (
	visSpectrum visMode = iota
	visWave
	visSpectrumStereo
	visEllipse
	visLorenz
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
	case visEllipse:
		return "ellipse"
	case visLorenz:
		return "lorenz"
	default:
		return "spectrum"
	}
}

// peakFalloff is the exponential decay applied to the spectrum peak caps
// each frame. A value near 1 falls slowly.
const peakFalloff = 0.90

// advanceVis updates the per-frame animation state on the visualizer tick.
// It runs in Update, not in View, so View stays a pure function. It reads
// the latest audio window from the player once per frame.
func (m *model) advanceVis() {
	// Drift the rainbow hue for the ellipse and Lorenz modes.
	m.visPhase += 0.01

	buf := make([]float64, visSampleWindow)
	n := m.player.Samples(buf)
	buf = buf[:n]
	energy := rms(buf)

	switch m.visMode {
	case visSpectrum:
		// Advance the falling peak caps against the current spectrum.
		width := m.width
		if width < 1 {
			width = 1
		}
		plotH := m.height - 1
		if plotH < 1 {
			plotH = 1
		}
		cols := spectrumColumns(spectrumLevels(buf, width), plotH)
		if len(m.visPeaks) != len(cols) {
			m.visPeaks = make([]int, len(cols))
		}
		for c := range cols {
			decayed := int(float64(m.visPeaks[c]) * peakFalloff)
			if cols[c] > decayed {
				m.visPeaks[c] = cols[c]
			} else {
				m.visPeaks[c] = decayed
			}
		}
	case visWave:
		// Decay a per-column envelope slowly, so the waveform falls off
		// gradually instead of snapping to each new frame.
		width := m.width
		if width < 1 {
			width = 1
		}
		amp := waveAmplitudes(buf, width)
		if len(m.visWave) != len(amp) {
			m.visWave = make([]float64, len(amp))
		}
		for c := range amp {
			decayed := m.visWave[c] * waveFalloff
			if amp[c] > decayed {
				m.visWave[c] = amp[c]
			} else {
				m.visWave[c] = decayed
			}
		}
	case visLorenz:
		m.visLorenz.step(energy)
	}
}

// waveFalloff is the exponential decay applied to the waveform envelope each
// frame. A value near 1 falls slowly, giving the waveform a slower falloff.
const waveFalloff = 0.90

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
// hue wheel. It gives the ellipse and Lorenz modes their rainbow look.
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

// rainbowStyleFor returns a foreground style along the hue wheel for the
// fraction f in 0..1.
func rainbowStyleFor(f float64) lipgloss.Style {
	return lipgloss.NewStyle().Foreground(lipgloss.Color(rainbowHex(f)))
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
// on a log scale. renderBars scales the result for display.
func spectrumLevels(samples []float64, width int) []float64 {
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
		if v < 0 {
			v = 0
		}
		if v > 1 {
			v = 1
		}
		levels[c] = v
	}
	monstercat(levels)
	return levels
}

// monstercatWeight controls how strongly a bar bleeds into its neighbors.
// A higher value spreads more. This is the monstercat smoothing filter
// from cli-visualizer/C.A.V.A.
const monstercatWeight = 2.8

// monstercat smooths the bar levels so a strong bar lifts its neighbors,
// which removes single-column spikes. It runs in place.
func monstercat(levels []float64) {
	n := len(levels)
	for i := 0; i < n; i++ {
		for j := i - 1; j >= 0; j-- {
			d := float64(i - j)
			v := levels[i] / math.Pow(monstercatWeight, d)
			if v > levels[j] {
				levels[j] = v
			} else {
				break
			}
		}
		for j := i + 1; j < n; j++ {
			d := float64(j - i)
			v := levels[i] / math.Pow(monstercatWeight, d)
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
// Each column height is 0..height*8. peaks, when non-nil, holds a decaying
// peak height per column in eighths; renderBars draws a bright cap at each
// peak. It returns height text rows.
func renderBars(cols, peaks []int, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	rows := make([]string, height)
	for row := 0; row < height; row++ {
		// row 0 is the top; the bar fills from the bottom up.
		levelFromBottom := height - 1 - row
		style := visStyleFor(levelFromBottom, height-1)
		var b strings.Builder
		for c := 0; c < width && c < len(cols); c++ {
			// Draw the falling peak cap if it sits in this cell and above
			// the bar top.
			if peaks != nil && c < len(peaks) {
				peakRow := (peaks[c] - 1) / 8
				if peakRow == levelFromBottom && peaks[c] > cols[c] {
					b.WriteString(style.Render("▔"))
					continue
				}
			}
			eighths := cols[c] - levelFromBottom*8
			var r rune
			switch {
			case eighths <= 0:
				r = ' '
			case eighths >= 8:
				r = '█'
			default:
				r = visBlocks[eighths]
			}
			if r == ' ' {
				b.WriteRune(' ')
			} else {
				b.WriteString(style.Render(string(r)))
			}
		}
		rows[row] = b.String()
	}
	return strings.Join(rows, "\n")
}

// stereoSpectrumRows draws two mirrored spectra: the left channel grows up
// from a center line and the right channel grows down. It copies the stereo
// spectrum idea from cli-visualizer.
func stereoSpectrumRows(frames [][2]float64, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	n := len(frames)
	left := make([]float64, n)
	right := make([]float64, n)
	for i, f := range frames {
		left[i] = f[0]
		right[i] = f[1]
	}
	half := height / 2
	if half < 1 {
		half = 1
	}
	ll := spectrumColumns(spectrumLevels(left, width), half)
	rl := spectrumColumns(spectrumLevels(right, width), half)

	rows := make([]string, height)
	for row := 0; row < height; row++ {
		var b strings.Builder
		if row < half {
			// Upper half: left channel, filling down toward the center.
			levelFromCenter := half - 1 - row
			style := visStyleFor(levelFromCenter, half-1)
			for c := 0; c < width && c < len(ll); c++ {
				b.WriteString(spectrumCell(ll[c], levelFromCenter, style))
			}
		} else {
			// Lower half: right channel, filling down from the center.
			levelFromCenter := row - half
			style := visStyleFor(levelFromCenter, half-1)
			for c := 0; c < width && c < len(rl); c++ {
				b.WriteString(spectrumCell(rl[c], levelFromCenter, style))
			}
		}
		rows[row] = b.String()
	}
	return strings.Join(rows, "\n")
}

// spectrumCell renders one bar cell given the column height in eighths and
// the level of this row measured from the bar base.
func spectrumCell(colEighths, levelFromBase int, style lipgloss.Style) string {
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

// ellipseRows draws a filled, rainbow-colored blob whose radius per angle
// follows the audio spectrum, so it pulses and deforms with the music. It
// reimplements the ellipse visualizer idea from cli-visualizer.
func ellipseRows(samples []float64, phase float64, width, height int) string {
	if width < 1 || height < 1 {
		return ""
	}
	cx := float64(width-1) / 2
	cy := float64(height-1) / 2

	// A ring of spectrum bands drives the blob radius per angle. Use a
	// modest band count and mirror it so opposite sides match, which keeps
	// the shape organic rather than jittery.
	const bands = 48
	levels := spectrumLevels(samples, bands)
	energy := rms(samples)

	// baseR is the resting radius; the spectrum adds a per-angle bulge.
	baseR := 0.30 + energy*1.2
	if baseR > 0.6 {
		baseR = 0.6
	}

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
			// Map the angle onto a mirrored band index.
			t := ang / (2 * math.Pi)
			bi := int(t * bands)
			if bi >= bands {
				bi = bands - 1
			}
			// Mirror the second half so left and right match.
			mi := bi
			if mi >= bands/2 {
				mi = bands - 1 - mi
			}
			bulge := levels[mi] * 0.45
			edge := baseR + bulge
			if dist <= edge {
				grid[y][x] = '█'
				// Color by radius: hue drifts outward and over time.
				styleAt[y][x] = rainbowStyleFor(dist*0.8 + phase)
			}
		}
	}
	return renderGrid(grid, styleAt)
}

// lorenzState holds the running Lorenz attractor trajectory.
type lorenzState struct {
	x, y, z float64
	trail   [][3]float64 // recent points, oldest first
}

// lorenzTrailLen caps the number of trajectory points kept. A long trail
// keeps the full butterfly shape on screen instead of a short moving arc.
const lorenzTrailLen = 8000

// reset seeds the attractor at a fixed point on its manifold.
func (s *lorenzState) reset() {
	s.x, s.y, s.z = 0.1, 0, 0
	s.trail = s.trail[:0]
}

// step integrates the Lorenz system. The audio energy scales the number of
// integration steps, so louder audio traces the attractor faster.
func (s *lorenzState) step(energy float64) {
	const (
		sigma = 10.0
		rho   = 28.0
		beta  = 8.0 / 3.0
		dt    = 0.006
	)
	if len(s.trail) == 0 {
		s.reset()
	}
	steps := 12 + int(energy*80)
	for i := 0; i < steps; i++ {
		dx := sigma * (s.y - s.x)
		dy := s.x*(rho-s.z) - s.y
		dz := s.x*s.y - beta*s.z
		s.x += dx * dt
		s.y += dy * dt
		s.z += dz * dt
		s.trail = append(s.trail, [3]float64{s.x, s.y, s.z})
	}
	if len(s.trail) > lorenzTrailLen {
		s.trail = s.trail[len(s.trail)-lorenzTrailLen:]
	}
}

// lorenzRows plots the attractor trail with braille glyphs. Each cell packs
// a 2x4 dot grid, so the plot resolution is higher than one point per cell.
// It reimplements the Lorenz visualizer from cli-visualizer.
func lorenzRows(st *lorenzState, phase float64, width, height int) string {
	if width < 1 || height < 1 || st == nil || len(st.trail) == 0 {
		return strings.Repeat("\n", height-1)
	}
	// Sub-pixel canvas: 2 dots wide, 4 dots tall per cell.
	dw := width * 2
	dh := height * 4
	dots := make([]uint8, width*height)
	styleAt := make([][]lipgloss.Style, height)
	for y := range styleAt {
		styleAt[y] = make([]lipgloss.Style, width)
	}
	// The Lorenz attractor roughly spans x,z in [-30,30] and [0,50].
	// Project the x/z plane onto the canvas.
	for _, p := range st.trail {
		nx := (p[0] + 25) / 50    // 0..1
		nz := 1 - (p[2]-5)/45     // flip so up is high z
		px := int(nx * float64(dw-1))
		py := int(nz * float64(dh-1))
		if px < 0 || px >= dw || py < 0 || py >= dh {
			continue
		}
		cell := (py/4)*width + (px / 2)
		// Braille dot bit for the sub-cell position.
		dots[cell] |= brailleBit(px%2, py%4)
		// Color by depth (z) so the two lobes read differently, with the
		// hue drifting slowly over time.
		depth := (p[2] - 5) / 45
		styleAt[py/4][px/2] = rainbowStyleFor(depth*0.7 + phase)
	}
	rows := make([]string, height)
	for y := 0; y < height; y++ {
		var b strings.Builder
		for x := 0; x < width; x++ {
			d := dots[y*width+x]
			if d == 0 {
				b.WriteRune(' ')
				continue
			}
			b.WriteString(styleAt[y][x].Render(string(rune(0x2800 + int(d)))))
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
