package ui

import (
	"bytes"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strings"

	"github.com/charmbracelet/lipgloss"
	xdraw "golang.org/x/image/draw"
)

// coverMode names a cover-art render technique. The 6 view cycles through
// the modes with Tab or Space while it is focused, so the modes are easy to
// compare.
type coverMode int

const (
	// coverHalfBlock draws the upper half block "▀" with a foreground and a
	// background color, so each cell holds two vertical pixels in full
	// color. This is the highest-fidelity mode for a color photo.
	coverHalfBlock coverMode = iota
	// coverBraille draws braille dots at 2x4 sub-cell resolution. It
	// thresholds the luminance, so it reads as a monochrome, high spatial
	// resolution rendering. It suits line art better than a color photo.
	coverBraille
	// coverBlocks draws shade glyphs " ░▒▓█" chosen by luminance, with a
	// per-cell foreground color. It gives a soft, retro look.
	coverBlocks
)

// coverModeCount is the number of cover render modes.
const coverModeCount = 3

// coverModeName returns a short label for a cover render mode.
func coverModeName(m coverMode) string {
	switch m {
	case coverBraille:
		return "braille"
	case coverBlocks:
		return "blocks"
	default:
		return "half-block"
	}
}

// decodeCover decodes image bytes into an image.Image. It accepts any
// format registered by the blank imports (jpeg, png, gif).
func decodeCover(data []byte) (image.Image, error) {
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}

// resizeCover scales src to exactly w by h pixels with a high-quality
// Catmull-Rom kernel. The caller picks w and h to match the target cell
// grid and the chosen sub-cell resolution.
func resizeCover(src image.Image, w, h int) *image.RGBA {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	dst := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.CatmullRom.Scale(dst, dst.Bounds(), src, src.Bounds(), xdraw.Over, nil)
	return dst
}

// coverFit computes the pixel size that keeps the source aspect ratio
// inside a cellW by cellH cell grid, given how many pixels one cell holds
// horizontally (px) and vertically (py). It works in physical screen units:
// a cell is one unit wide and cellAspect units tall, so a terminal cell
// being taller than wide does not stretch the image. It returns the fitted
// pixel size and the top-left offset that centers it on the canvas.
func coverFit(src image.Image, cellW, cellH, px, py int) (pw, ph, offX, offY int) {
	b := src.Bounds()
	iw := b.Dx()
	ih := b.Dy()
	if iw < 1 || ih < 1 {
		return px, py, 0, 0
	}
	// The pixel canvas is cellW*px by cellH*py pixels.
	canW := cellW * px
	canH := cellH * py

	// Work in physical screen units. Treat a cell as 1 unit wide and
	// cellAspect units tall. Then one horizontal pixel is 1/px units wide
	// and one vertical pixel is cellAspect/py units tall.
	pxW := 1.0 / float64(px)              // width of one pixel, in units
	pxH := cellAspect / float64(py)       // height of one pixel, in units
	boxW := float64(canW) * pxW           // canvas width, in units
	boxH := float64(canH) * pxH           // canvas height, in units

	// The image's physical aspect (width/height) must be preserved. Find
	// the largest scale s (units per source pixel, isotropic) that fits.
	sByW := boxW / float64(iw)
	sByH := boxH / float64(ih)
	s := sByW
	if sByH < s {
		s = sByH
	}
	// Physical fitted size in units.
	fitW := s * float64(iw)
	fitH := s * float64(ih)
	// Convert back to pixel counts on the canvas.
	pw = int(fitW/pxW + 0.5)
	ph = int(fitH/pxH + 0.5)
	if pw < 1 {
		pw = 1
	}
	if pw > canW {
		pw = canW
	}
	if ph < 1 {
		ph = 1
	}
	if ph > canH {
		ph = canH
	}
	offX = (canW - pw) / 2
	offY = (canH - ph) / 2
	return pw, ph, offX, offY
}

// rgbHex formats an 8-bit RGB triple as a lipgloss hex color. lipgloss
// degrades the truecolor value to the terminal color profile.
func rgbHex(r, g, b uint8) lipgloss.Color {
	const digits = "0123456789abcdef"
	buf := []byte{'#', 0, 0, 0, 0, 0, 0}
	buf[1] = digits[r>>4]
	buf[2] = digits[r&0xf]
	buf[3] = digits[g>>4]
	buf[4] = digits[g&0xf]
	buf[5] = digits[b>>4]
	buf[6] = digits[b&0xf]
	return lipgloss.Color(string(buf))
}

// luma returns the perceived brightness of an RGB triple, 0 to 255.
func luma(r, g, b uint8) int {
	return (int(r)*299 + int(g)*587 + int(b)*114) / 1000
}

// coverRender turns a decoded image into a block of colored text sized to
// cellW by cellH cells, using the chosen render mode.
func coverRender(src image.Image, mode coverMode, cellW, cellH int) string {
	if src == nil || cellW < 1 || cellH < 1 {
		return ""
	}
	switch mode {
	case coverBraille:
		return coverBrailleRender(src, cellW, cellH)
	case coverBlocks:
		return coverBlocksRender(src, cellW, cellH)
	default:
		return coverHalfBlockRender(src, cellW, cellH)
	}
}

// coverHalfBlockRender draws the image with the upper half block. Each cell
// holds two vertical pixels: the top pixel is the foreground color and the
// bottom pixel is the background color. The vertical pixel count is twice
// the cell height.
func coverHalfBlockRender(src image.Image, cellW, cellH int) string {
	px, py := 1, 2
	pw, ph, offX, offY := coverFit(src, cellW, cellH, px, py)
	// Round the height down to an even number so every cell has two rows.
	if ph%2 == 1 {
		ph--
	}
	if ph < 2 {
		ph = 2
	}
	img := resizeCover(src, pw, ph)
	rows := make([]string, 0, cellH)
	for cy := 0; cy < cellH; cy++ {
		var sb strings.Builder
		for cx := 0; cx < cellW; cx++ {
			topX := cx - offX
			topY := cy*2 - offY
			botY := topY + 1
			if topX < 0 || topX >= pw || topY < 0 || topY >= ph {
				sb.WriteByte(' ')
				continue
			}
			tr, tg, tb := rgbAt(img, topX, topY)
			var st lipgloss.Style
			if botY >= 0 && botY < ph {
				br, bg, bb := rgbAt(img, topX, botY)
				st = lipgloss.NewStyle().Foreground(rgbHex(tr, tg, tb)).Background(rgbHex(br, bg, bb))
			} else {
				st = lipgloss.NewStyle().Foreground(rgbHex(tr, tg, tb))
			}
			sb.WriteString(st.Render("▀"))
		}
		rows = append(rows, sb.String())
	}
	return strings.Join(rows, "\n")
}

// coverBlocksRender draws shade glyphs picked by luminance, colored by the
// cell's average pixel. Each cell samples one pixel.
func coverBlocksRender(src image.Image, cellW, cellH int) string {
	shades := []rune{' ', '░', '▒', '▓', '█'}
	px, py := 1, 1
	pw, ph, offX, offY := coverFit(src, cellW, cellH, px, py)
	img := resizeCover(src, pw, ph)
	rows := make([]string, 0, cellH)
	for cy := 0; cy < cellH; cy++ {
		var sb strings.Builder
		for cx := 0; cx < cellW; cx++ {
			ix := cx - offX
			iy := cy - offY
			if ix < 0 || ix >= pw || iy < 0 || iy >= ph {
				sb.WriteByte(' ')
				continue
			}
			r, g, b := rgbAt(img, ix, iy)
			l := luma(r, g, b)
			glyph := shades[l*len(shades)/256]
			st := lipgloss.NewStyle().Foreground(rgbHex(r, g, b))
			sb.WriteString(st.Render(string(glyph)))
		}
		rows = append(rows, sb.String())
	}
	return strings.Join(rows, "\n")
}

// coverBrailleRender draws the image with braille dots at 2x4 sub-cell
// resolution. It thresholds each sub-pixel against the mean luminance, so
// the result is a monochrome, high spatial resolution rendering. Each cell
// takes the color of its brightest lit dot.
func coverBrailleRender(src image.Image, cellW, cellH int) string {
	px, py := 2, 4
	pw, ph, offX, offY := coverFit(src, cellW, cellH, px, py)
	img := resizeCover(src, pw, ph)

	// Compute the mean luminance as the on/off threshold.
	var sum, count int
	for y := 0; y < ph; y++ {
		for x := 0; x < pw; x++ {
			r, g, b := rgbAt(img, x, y)
			sum += luma(r, g, b)
			count++
		}
	}
	thresh := 128
	if count > 0 {
		thresh = sum / count
	}

	rows := make([]string, 0, cellH)
	for cy := 0; cy < cellH; cy++ {
		var sb strings.Builder
		for cx := 0; cx < cellW; cx++ {
			var bits int
			var br, bg, bb uint8
			bestLuma := -1
			for dy := 0; dy < 4; dy++ {
				for dx := 0; dx < 2; dx++ {
					ix := cx*2 + dx - offX
					iy := cy*4 + dy - offY
					if ix < 0 || ix >= pw || iy < 0 || iy >= ph {
						continue
					}
					r, g, b := rgbAt(img, ix, iy)
					l := luma(r, g, b)
					if l >= thresh {
						bits |= int(brailleBit(dx, dy))
						if l > bestLuma {
							bestLuma = l
							br, bg, bb = r, g, b
						}
					}
				}
			}
			if bits == 0 {
				sb.WriteByte(' ')
				continue
			}
			st := lipgloss.NewStyle().Foreground(rgbHex(br, bg, bb))
			sb.WriteString(st.Render(string(rune(0x2800 + bits))))
		}
		rows = append(rows, sb.String())
	}
	return strings.Join(rows, "\n")
}

// rgbAt returns the 8-bit RGB of a pixel in an RGBA image.
func rgbAt(img *image.RGBA, x, y int) (uint8, uint8, uint8) {
	i := img.PixOffset(x, y)
	return img.Pix[i], img.Pix[i+1], img.Pix[i+2]
}
