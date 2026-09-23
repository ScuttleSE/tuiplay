package ui

import (
	"fmt"
	"strings"
	"time"

	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"
)

// The colors come from the active theme. See theme.go.

// spinnerFrames are the braille frames of the scan spinner.
var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// Fixed column widths.
const (
	trackColWidth = 2 // e.g. "03"
	timeColWidth  = 5 // e.g. "5:04"
)

// View renders the whole screen.
func (m model) View() string {
	if m.width == 0 || m.height == 0 {
		return "Loading..."
	}
	top := m.renderTopBar()
	body := m.renderBody()
	progress := m.renderProgress()
	status := m.renderStatus()
	screen := top + "\n" + body + "\n" + progress + "\n" + status
	if m.showVis {
		return m.overlayVisualizer()
	}
	if m.showHelp {
		return m.overlayHelp(screen)
	}
	return screen
}

// overlayHelp centers the help card on top of the screen.
func (m model) overlayHelp(screen string) string {
	lines := []string{
		"tuiplay — common keys",
		"",
		"1 hide panel     2 navigation",
		"3 search         4 lyrics",
		"5 visualizer     6 cover art",
		"/ search lyrics  , . lyric timing",
		"Tab switch pane",
		"§ rate up        ½ rate down",
		"Enter select/drill",
		"a add track      Space add all",
		"Del delete       c clear queue",
		"p pause          s stop",
		"> next           < prev/restart",
		"f/b seek         +/- volume",
		"R consume        r repeat",
		"z random         x/X crossfade",
		"V/v radio (feeder queue)",
		"i song info      I artist info",
		"e edit smart pl  S save playlist",
		"Ctrl+s overwrite playlist",
		"Home/End         PgUp/PgDn",
		"",
		"F1 or any key to close   q quit",
	}
	card := m.styles.helpCard.Render(strings.Join(lines, "\n"))
	return lipgloss.Place(m.width, m.height, lipgloss.Center, lipgloss.Center, card,
		lipgloss.WithWhitespaceChars(" "))
}

// overlayVisualizer renders the fullscreen visualizer. It reads the most
// recent output samples from the player and draws either a frequency
// spectrum or a time-domain waveform.
func (m model) overlayVisualizer() string {
	w := m.width
	h := m.height
	if w < 1 || h < 1 {
		return ""
	}
	// One title row at the top; the rest is the plot.
	plotH := h - 1
	if plotH < 1 {
		plotH = 1
	}

	buf := make([]float64, visSampleWindow)
	n := m.player.Samples(buf)
	buf = buf[:n]

	var body string
	switch m.visMode {
	case visWave:
		body = waveRows(buf, m.visWave, w, plotH)
	case visSpectrumStereo:
		body = stereoSpectrumRows(m.visStereoL, m.visStereoR, w, plotH, m.visPhase, m.visBeat)
	case visRadial:
		body = radialRows(m.visRadialLevels, rms(buf), m.visPhase, m.visBeat,
			m.visSpark.RadialReach, m.visSpark.RadialCore, w, plotH)
	case visParticles:
		body = particleRows(&m.visParticles, m.visPhase, m.visBeat, w, plotH)
	default:
		cols := spectrumColumns(m.visLevels, plotH)
		body = renderBars(cols, m.visPeaks, w, plotH, m.visPhase, m.visBeat)
	}
	title := trim("Visualizer ["+visModeName(m.visMode)+"]  Tab switch  Esc close", w)
	return m.styles.colHeader.Render(title) + "\n" + body
}

// listHeight returns the number of list rows in a pane. The screen uses
// the top bar (1), the column header (1), the rule (1), the progress bar
// (1), and the status bar (1).
func (m model) listHeight() int {
	n := m.height - 5
	if n < 1 {
		return 1
	}
	return n
}

// queueWidth returns the width of the left queue pane in cells.
func (m model) queueWidth() int {
	if !m.rightShown {
		return m.width
	}
	w := int(float64(m.width) * m.split)
	if w < 20 {
		w = 20
	}
	if w > m.width-10 {
		w = m.width - 10
	}
	return w
}

// renderTopBar draws the top line: the queue summary and the volume.
func (m model) renderTopBar() string {
	left := fmt.Sprintf("Playlist (%d items, length: %s)", len(m.queue), queueLength(m.queue))
	right := fmt.Sprintf("Volume: %d%%", m.player.VolumePercent())
	space := m.width - lipgloss.Width(left) - lipgloss.Width(right)
	if space < 1 {
		space = 1
	}
	body := left + strings.Repeat(" ", space) + right
	return m.styles.topBar.Render(trim(body, m.width))
}

// renderBody draws the two panes side by side, or the queue alone.
func (m model) renderBody() string {
	h := m.listHeight() + 2 // list rows plus the column header and the rule
	if !m.rightShown {
		return m.renderQueuePane(m.width, h)
	}
	qw := m.queueWidth()
	rw := m.width - qw - 1
	if rw < 1 {
		rw = 1
	}
	left := m.renderQueuePane(qw, h)
	right := m.renderNavPane(rw, h)
	divider := m.renderDivider(h)
	return lipgloss.JoinHorizontal(lipgloss.Top, left, divider, right)
}

// renderDivider draws the vertical line between the panes.
func (m model) renderDivider(height int) string {
	lines := make([]string, height)
	for i := range lines {
		lines[i] = m.styles.divider.Render("│")
	}
	return strings.Join(lines, "\n")
}

// renderQueuePane draws the queue with its columns.
func (m model) renderQueuePane(width, height int) string {
	focused := m.focus == focusQueue
	header := m.queueColumnHeader(width, focused)
	rule := m.styles.rule.Render(strings.Repeat("─", width))

	var rows []string
	for i, s := range m.queue {
		selected := focused && i == m.queueCursor
		playing := i == m.queueIndex
		rows = append(rows, m.queueRow(s, width, selected, playing))
	}
	rows = fitRows(rows, m.queueCursor, height-2, width)
	return header + "\n" + rule + "\n" + strings.Join(rows, "\n")
}

// queueColumnHeader draws the Artist/Track Title/Album/Time header.
func (m model) queueColumnHeader(width int, focused bool) string {
	aw, tw, alw, tmw := queueColumnWidths(width)
	style := m.styles.colHeader
	if focused {
		style = m.styles.focusHeader
	}
	parts := []string{pad("Artist", aw), pad("Track Title", tw), pad("Album", alw), pad("Time", tmw)}
	return style.Render(trim(strings.Join(parts, " "), width))
}

// queueColumnWidths splits the pane width into the four queue columns.
func queueColumnWidths(width int) (artist, title, album, tm int) {
	rest := width - timeColWidth - 3
	if rest < 4 {
		rest = 4
	}
	artist = rest * 30 / 100
	album = rest * 30 / 100
	title = rest - artist - album
	if artist < 1 {
		artist = 1
	}
	if album < 1 {
		album = 1
	}
	if title < 1 {
		title = 1
	}
	return artist, title, album, timeColWidth
}

// ratingMark returns the prefix shown before a song title for its rating.
// A rating of 0 has no prefix. A rating of 1 marks a dislike with a
// thumbs-down glyph. A rating of 2 to 5 shows that many filled stars.
func ratingMark(rating int) string {
	switch {
	case rating <= 0:
		return ""
	case rating == 1:
		return "👎 "
	default:
		return strings.Repeat("★", rating) + " "
	}
}

// queueRow draws one queue row with its columns.
func (m model) queueRow(s subsonic.Song, width int, selected, playing bool) string {
	aw, tw, alw, tmw := queueColumnWidths(width)
	track := fmt.Sprintf("%0*d", trackColWidth, s.Track)
	// A rated song shows its rating before its title. A rating of 1
	// shows a thumbs-down marker; a higher rating shows filled stars.
	title := ratingMark(s.UserRating) + s.Title
	titleField := track + "  " + title
	timeField := fmtDur(time.Duration(s.Duration) * time.Second)

	if selected {
		artistCell := m.styles.selArtistBar.Render(pad(trim(s.Artist, aw), aw))
		titleCell := m.styles.selArtistBar.Render(pad(trim(titleField, tw), tw))
		albumCell := m.styles.selAlbumBar.Render(pad(trim(s.Album, alw), alw))
		timeCell := m.styles.selTimeBar.Render(pad(trim(timeField, tmw), tmw))
		return artistCell + m.styles.selArtistBar.Render(" ") + titleCell + m.styles.selAlbumBar.Render(" ") + albumCell + m.styles.selTimeBar.Render(" ") + timeCell
	}

	aStyle, trStyle, tiStyle, alStyle, tmStyle := m.styles.artist, m.styles.track, m.styles.title, m.styles.album, m.styles.time
	if playing {
		aStyle = aStyle.Bold(true)
		trStyle = trStyle.Bold(true)
		tiStyle = tiStyle.Bold(true)
		alStyle = alStyle.Bold(true)
		tmStyle = tmStyle.Bold(true)
	}
	trackText := trStyle.Render(track)
	titleText := tiStyle.Render(pad(trim(title, tw-trackColWidth-2), tw-trackColWidth-2))
	artistText := aStyle.Render(pad(trim(s.Artist, aw), aw))
	albumText := alStyle.Render(pad(trim(s.Album, alw), alw))
	timeText := tmStyle.Render(pad(trim(timeField, tmw), tmw))
	return artistText + " " + trackText + "  " + titleText + " " + albumText + " " + timeText
}

// renderNavPane draws the current navigation level.
func (m model) renderNavPane(width, height int) string {
	focused := m.focus == focusRight
	lvl := m.topLevel()

	style := m.styles.colHeader
	if focused {
		style = m.styles.focusHeader
	}
	title := lvl.title
	if lvl.kind == navCover {
		title += "  [" + coverModeName(m.coverMode) + "]  Space switch"
	}
	if lvl.kind == navLyrics && lvl.lyricsSynced && m.lyricsOffset != 0 {
		title += fmt.Sprintf("  [%+.1fs]", m.lyricsOffset.Seconds())
	}
	header := style.Render(trim(title, width))
	rule := m.styles.rule.Render(strings.Repeat("─", width))

	// Build display rows, with the back row first when present.
	var rows []string
	displayCursor := 0
	if lvl.hasBackRow() {
		backSelected := focused && lvl.cursor < 0
		rows = append(rows, m.backRowLine(width, backSelected))
		if lvl.cursor >= 0 {
			displayCursor = lvl.cursor + 1
		}
	}
	if len(lvl.info) > 0 {
		// An info level shows label/value pairs, not selectable rows.
		for _, line := range lvl.info {
			label := m.styles.colHeader.Render(line.label + ":")
			value := trim(line.value, width-lipgloss.Width(line.label)-2)
			rows = append(rows, label+" "+m.styles.album.Render(value))
		}
	} else if lvl.kind == navLyrics {
		lrows, active := m.lyricsRows(lvl, width)
		base := len(rows) // account for the back row already appended
		rows = append(rows, lrows...)
		if active >= 0 {
			displayCursor = base + active
		}
	} else if lvl.kind == navCover {
		rows = append(rows, m.coverRows(lvl, width, height-2)...)
	} else if lvl.kind == navSearch {
		rows = append(rows, m.renderSearchRows(lvl, width, focused)...)
	} else {
		for i, r := range lvl.rows {
			selected := focused && i == lvl.cursor
			rows = append(rows, m.navRowLine(lvl.kind, r, width, selected))
		}
	}
	if lvl.loading && len(lvl.rows) == 0 {
		rows = append(rows, m.styles.plain.Render(pad("Loading...", width)))
	}

	rows = fitRows(rows, displayCursor, height-2, width)
	return header + "\n" + rule + "\n" + strings.Join(rows, "\n")
}

// activeLyricIndex returns the index of the active lyric line for an
// elapsed time, or -1 when no line is active. The offset shifts the line
// timestamps: a negative offset makes lines active earlier, a positive
// one later. Untimed lines never match.
func activeLyricIndex(lines []lyricLine, elapsedMs int64, offset time.Duration) int {
	active := -1
	off := offset.Milliseconds()
	for i, ln := range lines {
		if ln.atMs >= 0 && ln.atMs+off <= elapsedMs {
			active = i
		}
	}
	return active
}

// lyricsRows draws the lyric lines. It returns the rendered rows and the
// index of the active line, or -1 when no line is active. For synced
// lyrics of the currently playing song, the active line is the last line
// whose timestamp is at or before the elapsed time; it renders highlighted.
func (m model) lyricsRows(lvl navLevel, width int) ([]string, int) {
	if lvl.lyricsMissing || len(lvl.lyrics) == 0 {
		msg := "No lyrics found."
		if lvl.lyricsSong == "" && !lvl.lyricsManual {
			// The empty placeholder: nothing is playing.
			msg = "No song playing."
		}
		return []string{m.styles.plain.Render(pad(msg, width))}, -1
	}

	active := -1
	if lvl.lyricsSynced && m.currentSongID() == lvl.lyricsSong && m.player.State() != player.StateStopped {
		active = activeLyricIndex(lvl.lyrics, m.elapsed.Milliseconds(), m.lyricsOffset)
	}

	out := make([]string, 0, len(lvl.lyrics))
	for i, ln := range lvl.lyrics {
		text := ln.text
		if text == "" {
			text = " "
		}
		if i == active {
			out = append(out, m.styles.selPlainBar.Render(pad(trim(text, width), width)))
		} else {
			out = append(out, m.styles.plain.Render(pad(trim(text, width), width)))
		}
	}
	return out, active
}

// coverRows renders the cover-art view into height rows of width cells. It
// shows a message when nothing plays, the art is missing, or a download is
// still in flight. Otherwise it renders the decoded image with the active
// render mode.
func (m model) coverRows(lvl navLevel, width, height int) []string {
	if height < 1 {
		height = 1
	}
	msg := ""
	switch {
	case lvl.loading && lvl.coverImg == nil:
		msg = "Loading cover..."
	case lvl.coverSong == "" && lvl.coverMissing:
		msg = "No song playing."
	case lvl.coverImg == nil:
		msg = "No cover art."
	}
	if msg != "" {
		out := make([]string, height)
		mid := height / 2
		for i := range out {
			if i == mid {
				out[i] = m.styles.plain.Render(pad(trim(msg, width), width))
			} else {
				out[i] = strings.Repeat(" ", width)
			}
		}
		return out
	}
	art := coverRender(lvl.coverImg, m.coverMode, width, height)
	rows := strings.Split(art, "\n")
	// Pad or trim to exactly height rows so the pane layout stays stable.
	for len(rows) < height {
		rows = append(rows, strings.Repeat(" ", width))
	}
	if len(rows) > height {
		rows = rows[:height]
	}
	return rows
}

// settings block, and the Search and Reset action rows.
func (m model) renderSearchRows(lvl navLevel, width int, focused bool) []string {
	var out []string
	if lvl.search == nil {
		return out
	}
	labelW := 0
	for _, f := range lvl.search.fields {
		if w := lipgloss.Width(f.label); w > labelW {
			labelW = w
		}
	}
	n := len(lvl.search.fields)
	for i, f := range lvl.search.fields {
		selected := focused && i == lvl.cursor
		value := f.value
		valueStyle := m.styles.album
		if value == "" {
			value = "<empty>"
			valueStyle = m.styles.artist
		}
		if selected {
			body := pad(f.label, labelW) + " : " + value
			out = append(out, m.styles.selPlainBar.Render(pad(trim(body, width), width)))
		} else {
			label := m.styles.colHeader.Render(pad(f.label, labelW)) + " : "
			out = append(out, label+valueStyle.Render(trim(value, width-labelW-3)))
		}
	}
	out = append(out, "")
	out = append(out, m.styles.colHeader.Render("Search in:")+" "+m.styles.plain.Render("Database"))
	out = append(out, m.styles.colHeader.Render("Search mode:")+" "+m.styles.plain.Render("Match if tag contains searched phrase"))
	out = append(out, "")
	for j, label := range []string{"Search", "Reset"} {
		selected := focused && n+j == lvl.cursor
		if selected {
			out = append(out, m.styles.selPlainBar.Render(pad(trim(label, width), width)))
		} else {
			out = append(out, m.styles.plain.Render(pad(label, width)))
		}
	}
	return out
}

// backRowLine draws the [..] back row.
func (m model) backRowLine(width int, selected bool) string {
	label := "[..]"
	if selected {
		return m.styles.selPlainBar.Render(pad(trim(label, width), width))
	}
	return m.styles.back.Render(pad(trim(label, width), width))
}

// navRowLine draws one real navigation row. A track row shows the time at
// the right. Other rows show the label.
func (m model) navRowLine(kind navKind, r navRow, width int, selected bool) string {
	if kind == navTracks {
		tmw := timeColWidth
		labelW := width - tmw - 1
		if labelW < 1 {
			labelW = 1
		}
		label := pad(trim(r.label, labelW), labelW)
		tm := pad(fmtDur(time.Duration(r.song.Duration)*time.Second), tmw)
		if selected {
			return m.styles.selPlainBar.Render(pad(label+" "+tm, width))
		}
		return m.styles.title.Render(label) + " " + m.styles.time.Render(tm)
	}

	if selected {
		return m.styles.selPlainBar.Render(pad(trim(r.label, width), width))
	}
	return m.styles.plain.Render(pad(trim(r.label, width), width))
}

// fitRows scrolls the rows around the cursor and pads to height with
// blank full-width lines so the panes align.
func fitRows(rows []string, cursor, height, width int) []string {
	blank := strings.Repeat(" ", width)
	if len(rows) == 0 {
		rows = []string{blank}
	}
	rows = window(rows, cursor, height)
	for len(rows) < height {
		rows = append(rows, blank)
	}
	return rows
}

// window returns the slice of rows around the cursor that fits height.
func window(rows []string, cursor, height int) []string {
	if len(rows) <= height {
		return rows
	}
	start := cursor - height/2
	if start < 0 {
		start = 0
	}
	if start+height > len(rows) {
		start = len(rows) - height
	}
	return rows[start : start+height]
}

// renderProgress draws the green progress bar.
func (m model) renderProgress() string {
	width := m.width
	if width < 1 {
		return ""
	}
	if m.total <= 0 {
		return m.styles.progress.Render(strings.Repeat(" ", width))
	}
	frac := float64(m.elapsed) / float64(m.total)
	if frac < 0 {
		frac = 0
	}
	if frac > 1 {
		frac = 1
	}
	filled := int(frac * float64(width))
	if filled < 1 {
		filled = 1
	}
	if filled > width {
		filled = width
	}
	bar := strings.Repeat("=", filled-1) + ">"
	if lipgloss.Width(bar) < width {
		bar += strings.Repeat(" ", width-lipgloss.Width(bar))
	}
	return m.styles.progress.Render(trim(bar, width))
}

// renderStatus draws the bottom status bar.
func (m model) renderStatus() string {
	// When a confirm is active, the status bar asks the question.
	if m.confirmActive {
		body := " " + m.confirmLabel
		return m.styles.status.Render(trim(pad(body, m.width), m.width))
	}

	// When the prompt is active, the status bar becomes the input line.
	if m.promptActive {
		body := " " + m.promptLabel + ": " + m.promptInput + "_"
		return m.styles.status.Render(trim(pad(body, m.width), m.width))
	}

	var left string
	// A temporary status message overrides the Playing line for a while.
	// A loading status stays until the play result replaces it.
	if time.Now().Before(m.statusExpiry) {
		left = m.status
	} else if m.loadingPlay {
		left = m.status
	} else {
		switch m.player.State() {
		case player.StatePlaying, player.StatePaused:
			if m.queueIndex >= 0 && m.queueIndex < len(m.queue) {
				s := m.queue[m.queueIndex]
				verb := "Playing"
				if m.player.State() == player.StatePaused {
					verb = "Paused"
				}
				year := ""
				if s.Year > 0 {
					year = fmt.Sprintf(" (%d)", s.Year)
				}
				left = fmt.Sprintf("%s: %s %q%s - %s", verb, s.Artist, s.Album, year, s.Title)
			} else {
				left = m.status
			}
		default:
			left = m.status
		}
	}

	right := ""
	if m.total > 0 {
		right = fmt.Sprintf("[%s/%s]", fmtDur(m.elapsed), fmtDur(m.total))
	}
	// Mode flags use single letters: R repeat, c consume, r random, x
	// crossfade, F radio (feeder), t transcoding. They join without spaces,
	// like [cr] or [Rxt].
	flags := ""
	if m.repeat {
		flags += "R"
	}
	if m.consume {
		flags += "c"
	}
	if m.shuffle {
		flags += "r"
	}
	if m.crossfade {
		flags += "x"
	}
	if m.radio {
		flags += "F"
	}
	if m.transcoding {
		flags += "t"
	}
	if flags != "" {
		right = "[" + flags + "] " + right
	}
	// While the server scans, show a braille spinner with the flags.
	if m.scanning {
		spin := spinnerFrames[m.spinFrame%len(spinnerFrames)]
		group := spin
		if flags != "" {
			group = spin + " " + flags
		}
		// Rebuild the right side: replace the flag group with the spinner
		// group, keeping the time part.
		if m.total > 0 {
			right = m.styles.spinner.Render("["+group+"]") + " " + fmt.Sprintf("[%s/%s]", fmtDur(m.elapsed), fmtDur(m.total))
		} else {
			right = m.styles.spinner.Render("[" + group + "]")
		}
	}

	space := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
	if space < 1 {
		space = 1
	}
	body := " " + left + strings.Repeat(" ", space) + right + " "
	return m.styles.status.Render(trim(body, m.width))
}

// queueLength returns the total duration of the queue in ncmpcpp phrasing.
func queueLength(songs []subsonic.Song) string {
	total := 0
	for _, s := range songs {
		total += s.Duration
	}
	h := total / 3600
	mnt := (total % 3600) / 60
	sec := total % 60
	if h > 0 {
		return fmt.Sprintf("%d hours, %d minutes, %d seconds", h, mnt, sec)
	}
	if mnt > 0 {
		return fmt.Sprintf("%d minutes, %d seconds", mnt, sec)
	}
	return fmt.Sprintf("%d seconds", sec)
}

// fmtDur formats a duration as m:ss.
func fmtDur(d time.Duration) string {
	total := int(d.Seconds())
	return fmt.Sprintf("%d:%02d", total/60, total%60)
}

// pad extends s with spaces to width cells. It does not trim.
func pad(s string, width int) string {
	w := lipgloss.Width(s)
	if w >= width {
		return s
	}
	return s + strings.Repeat(" ", width-w)
}

// trim cuts text to width visible cells. It counts the display width of
// each rune, so a wide rune (for example a CJK glyph) counts as two cells.
func trim(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	var b strings.Builder
	used := 0
	for _, r := range s {
		w := runewidth.RuneWidth(r)
		if used+w > width {
			break
		}
		b.WriteRune(r)
		used += w
	}
	return b.String()
}
