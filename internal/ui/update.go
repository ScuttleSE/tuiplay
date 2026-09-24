package ui

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"

	"git.hemmalab.se/scuttle/tuiplay/internal/config"
	"git.hemmalab.se/scuttle/tuiplay/internal/lyrics"
	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"

	tea "github.com/charmbracelet/bubbletea"
)

// statusHold is how long a temporary status message stays visible.
const statusHold = 3500 * time.Millisecond

// Update handles one message and returns the new model. It wraps the real
// handler and forces a full repaint whenever the playing track changes.
// Bubble Tea's line diff can leave the previous playing row's bold styling
// on screen after an advance, so stale bold rows would otherwise pile up.
func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	prevIndex := m.queueIndex
	nm, cmd := m.updateInner(msg)
	if mm, ok := nm.(model); ok && mm.queueIndex != prevIndex {
		return nm, tea.Batch(cmd, tea.ClearScreen)
	}
	return nm, cmd
}

// updateInner handles one message and returns the new model.
func (m model) updateInner(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tickMsg:
		m.elapsed, m.total = m.player.Position()
		cmds := []tea.Cmd{tick(), m.checkScanStatus()}
		// Radio (feeder) mode keeps the queue topped up to radioSize.
		m = m.topUpRadio()
		// While the lyrics view is active, follow the playing song: reload
		// when the current song differs from the loaded lyrics.
		if m.activeView == viewLyrics {
			if c := m.refreshLyricsIfPlaying(); c != nil {
				cmds = append(cmds, c)
			}
		}
		// While the cover view is active, follow the playing song too.
		if m.activeView == viewCover {
			if c := m.refreshCoverIfPlaying(); c != nil {
				cmds = append(cmds, c)
			}
		}
		return m, tea.Batch(cmds...)

	case scanStatusMsg:
		if msg.err != nil {
			// Ignore poll errors so the status bar stays quiet.
			return m, nil
		}
		wasScanning := m.scanning
		m.scanning = msg.scanning
		if m.scanning && !wasScanning {
			// A scan just started: begin the spinner animation.
			return m, spinTick()
		}
		return m, nil

	case spinTickMsg:
		if !m.scanning {
			// The scan ended: stop animating.
			return m, nil
		}
		m.spinFrame++
		return m, spinTick()

	case visTickMsg:
		if !m.showVis {
			// The visualizer closed: stop animating.
			return m, nil
		}
		m.visFrame++
		m.advanceVis()
		return m, visTick()

	case statusExpiredMsg:
		// The view checks the expiry time. This message only forces a
		// redraw when the temporary status ends.
		return m, nil

	case trackEndedMsg:
		return m.advance(true)

	case playStartedMsg:
		return m.playStarted(msg)

	case levelMsg:
		if msg.err != nil {
			m.top().loading = false
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		// Replace the loading placeholder with the loaded level.
		if m.top().loading {
			*m.top() = msg.level
		} else {
			m.pushLevel(msg.level)
		}
		m.focus = focusRight
		m.rightShown = true
		return m, nil

	case yearsMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		lvl := navLevel{kind: navYears, title: "Release Years", rows: yearRows(msg.years), cursor: 0}
		if m.top().loading {
			*m.top() = lvl
		} else {
			m.pushLevel(lvl)
		}
		m.focus = focusRight
		m.rightShown = true
		return m, nil

	case enqueueMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		m.queue = append(m.queue, msg.songs...)
		m.status = fmt.Sprintf("Added: %s (%d songs)", msg.label, len(msg.songs))
		return m, nil

	case radioMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		return m.startRadio(msg.songs, msg.label)

	case replaceMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		m.stopPlayer()
		m.queue = msg.songs
		m.queueCursor = 0
		m.queueIndex = -1
		m.sourcePlaylistID = msg.sourceID
		m.sourcePlaylistName = msg.sourceName
		m.status = fmt.Sprintf("Playing playlist: %s (%d songs)", msg.label, len(msg.songs))
		if msg.play && len(m.queue) > 0 {
			if m.shuffle {
				m.queueIndex = rand.Intn(len(m.queue))
			} else {
				m.queueIndex = 0
			}
			return m.playCurrent()
		}
		return m, nil

	case deletedMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		text := fmt.Sprintf("Deleted playlist: %s", msg.name)
		if msg.fileRemoved {
			text += " (removed .nsp, scan started)"
			if msg.fileErr != nil {
				text = fmt.Sprintf("Deleted playlist: %s (removed .nsp; scan failed: %s)", msg.name, msg.fileErr.Error())
			}
		} else if msg.fileErr != nil {
			text = fmt.Sprintf("Deleted playlist: %s (could not remove .nsp: %s)", msg.name, msg.fileErr.Error())
		}
		tm, cmd := m.setTempStatus(text)
		nm := tm.(model)
		if nm.topLevel().kind == navPlaylists {
			return nm, tea.Batch(cmd, nm.loadPlaylists())
		}
		return nm, cmd

	case savedMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		tm, cmd := m.setTempStatus(fmt.Sprintf("Saved playlist: %s", msg.name))
		nm := tm.(model)
		// If the Playlists view is open, refresh it so the new playlist
		// shows up.
		if nm.topLevel().kind == navPlaylists {
			return nm, tea.Batch(cmd, nm.loadPlaylists())
		}
		return nm, cmd

	case songInfoMsg:
		if msg.err != nil {
			m.status = "Error: " + msg.err.Error()
			return m, nil
		}
		lvl := songInfoLevel(msg.song)
		if m.top().loading {
			*m.top() = lvl
		} else {
			m.pushLevel(lvl)
		}
		m.focus = focusRight
		m.rightShown = true
		return m, nil

	case lyricsMsg:
		// Drop a stale result: the current song may have changed while the
		// lookup ran. Only apply the result for the song that plays now, or
		// when nothing plays keep whatever loaded.
		if cur := m.currentSongID(); cur != "" && cur != msg.song {
			return m, nil
		}
		lvl := navLevel{
			kind:          navLyrics,
			title:         "Lyrics " + lyricsQualityMark(msg.synced, msg.missing) + msg.title,
			cursor:        -1,
			lyrics:        msg.lines,
			lyricsSynced:  msg.synced,
			lyricsSong:    msg.song,
			lyricsMissing: msg.missing,
		}
		m.lyricsNav = []navLevel{lvl}
		m.lyricsOffset = msg.offset
		return m, nil

	case lyricsSearchMsg:
		if len(msg.hits) == 0 {
			if msg.err != nil {
				return m.setTempStatus("Lyrics search failed: " + msg.err.Error())
			}
			return m.setTempStatus(fmt.Sprintf("No lyrics found for %q.", msg.query))
		}
		m.lyricsNav = append(m.lyricsNav, lyricsSearchLevel(msg.query, msg.hits))
		m.focus = focusRight
		m.rightShown = true
		if msg.err != nil {
			// One tier failed, but the shown hits are usable.
			return m.setTempStatus("Lyrics search: " + msg.err.Error())
		}
		return m, nil

	case coverMsg:
		// Drop a stale result: the current song may have changed while the
		// download ran. Apply only the result for the song that plays now,
		// or keep whatever loaded when nothing plays.
		if cur := m.currentSongID(); cur != "" && cur != msg.song {
			return m, nil
		}
		lvl := navLevel{
			kind:         navCover,
			title:        "Cover: " + msg.title,
			cursor:       -1,
			coverImg:     msg.img,
			coverSong:    msg.song,
			coverMissing: msg.missing || msg.img == nil,
		}
		m.coverNav = []navLevel{lvl}
		return m, nil

	case ratedMsg:
		if msg.err != nil {
			return m.setTempStatus("Rating error: " + msg.err.Error())
		}
		// Update every queue entry and nav row that holds this song.
		for i := range m.queue {
			if m.queue[i].ID == msg.id {
				m.queue[i].UserRating = msg.rating
			}
		}
		m.updateRatingInNav(msg.id, msg.rating)
		switch {
		case msg.rating == 0:
			return m.setTempStatus("Rating cleared")
		case msg.rating == 1:
			return m.setTempStatus("Disliked (1 star)")
		default:
			return m.setTempStatus(fmt.Sprintf("Rated %d stars", msg.rating))
		}

	case restoreQueueMsg:
		if msg.err != nil || !msg.ok || len(msg.queue.Songs) == 0 {
			return m, nil
		}
		// Drop a late restore once the user has taken over. A slow restore
		// command must not pause or replace a queue the user already
		// started (for example radio mode), which would silence playback.
		if m.radio || len(m.queue) > 0 || m.player.State() != player.StateStopped {
			return m, nil
		}
		return m.restoreFromQueue(msg.queue)

	case nspSavedMsg:
		if msg.err != nil {
			return m.setTempStatus("Smart playlist error: " + msg.err.Error())
		}
		note := "Wrote " + msg.path
		if msg.scanErr != nil {
			note += " (scan not started: " + msg.scanErr.Error() + "; imports on next scan)"
		} else {
			note += " (scan started)"
		}
		return m.setTempStatus(note)

	case controlMsg:
		return m.handleControl(msg)

	case tea.KeyMsg:
		if m.showHelp {
			m.showHelp = false
			return m, nil
		}
		if m.showVis {
			return m.handleVisKey(msg)
		}
		if m.confirmActive {
			return m.handleConfirmKey(msg)
		}
		if m.promptActive {
			return m.handlePromptKey(msg)
		}
		return m.handleKey(msg)
	}
	return m, nil
}

// handleVisKey handles a key while the fullscreen visualizer is open. Tab
// and Space switch the mode. Esc and the visualizer key close it. Playback
// keys still work; everything else is ignored so navigation does not run
// behind the overlay.
func (m model) handleVisKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	action := m.keyAction[key]
	switch key {
	case "tab", " ", "space":
		m.visMode = nextVisMode(m.visMode)
		return m, nil
	case "esc":
		m.showVis = false
		return m, nil
	}
	if action == config.ActVisualizer {
		m.showVis = false
		return m, nil
	}
	// Let playback controls pass through so the user keeps control.
	switch action {
	case config.ActPause, config.ActStop, config.ActNext, config.ActPrev,
		config.ActSeekForward, config.ActSeekBack,
		config.ActVolumeUp, config.ActVolumeDown,
		config.ActRateUp, config.ActRateDown, config.ActQuit:
		return m.handleKey(msg)
	}
	return m, nil
}

// handleConfirmKey handles a key while a yes/no confirm is active.
func (m model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "y", "Y":
		action := m.confirmAction
		m.confirmActive = false
		m.confirmAction = nil
		if action != nil {
			nm, cmd := action(m)
			return nm, cmd
		}
		return m, nil
	case "n", "N", "esc":
		m.confirmActive = false
		m.confirmAction = nil
		m.status = "Cancelled."
		return m, nil
	}
	return m, nil
}

// handlePromptKey handles a key while the text prompt is active.
func (m model) handlePromptKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEnter:
		input := strings.TrimSpace(m.promptInput)
		kind := m.promptKind
		m.promptActive = false
		m.promptInput = ""
		return m.confirmPrompt(kind, input)
	case tea.KeyEsc:
		m.promptActive = false
		m.promptInput = ""
		m.status = "Cancelled."
		return m, nil
	case tea.KeyBackspace:
		if len(m.promptInput) > 0 {
			r := []rune(m.promptInput)
			m.promptInput = string(r[:len(r)-1])
		}
		return m, nil
	case tea.KeyRunes, tea.KeySpace:
		m.promptInput += string(msg.Runes)
		return m, nil
	}
	return m, nil
}

// confirmPrompt runs the action for a finished text prompt.
func (m model) confirmPrompt(kind promptKind, input string) (tea.Model, tea.Cmd) {
	switch kind {
	case promptSaveNew:
		if input == "" {
			m.status = "Save cancelled."
			return m, nil
		}
		if len(m.queue) == 0 {
			m.status = "The queue is empty."
			return m, nil
		}
		m.status = "Saving playlist..."
		return m, m.savePlaylist(input, m.queue)
	case promptCrossfade:
		n, err := strconv.Atoi(input)
		if err != nil || n < 0 {
			m.status = "Invalid crossfade value."
			return m, nil
		}
		m.crossfadeSec = n
		if m.crossfade {
			m.player.SetCrossfade(n)
		}
		return m.setTempStatus(fmt.Sprintf("Crossfade: %d seconds", n))
	case promptRadio:
		n, err := strconv.Atoi(input)
		if err != nil || n < 1 {
			return m.setTempStatus("Invalid radio queue size.")
		}
		m.radioSize = n
		nm := m.topUpRadio()
		return nm.setTempStatus(fmt.Sprintf("Radio queue size: %d", n))
	case promptSearchField:
		lvl := m.top()
		if lvl.search != nil && m.searchEditKey != "" {
			lvl.search.set(searchFieldKey(m.searchEditKey), input)
		}
		m.searchEditKey = ""
		return m, nil
	case promptLyricsSearch:
		if input == "" {
			m.status = "Search cancelled."
			return m, nil
		}
		m.status = "Searching lyrics: " + input + "..."
		return m, m.searchLyricsCmd(input)
	case promptSmartName:
		if b := m.top().builder; b != nil {
			b.name = input
			m.rebuildSmartLevel(b)
		}
		return m, nil
	case promptSmartLimit:
		b := m.top().builder
		if b == nil {
			return m, nil
		}
		if input == "" {
			b.limit = 0
		} else {
			n, err := strconv.Atoi(input)
			if err != nil || n < 0 {
				return m.setTempStatus("Invalid limit value.")
			}
			b.limit = n
		}
		m.rebuildSmartLevel(b)
		return m, nil
	case promptSmartValue:
		b := m.top().builder
		if b == nil {
			return m, nil
		}
		cond := nspCondition{field: m.smartEditField, operator: m.smartEditOp, value: input}
		// Validate the value before storing it.
		if f, ok := nspFieldByKey(cond.field); ok {
			if _, err := encodeValue(f.typ, cond.operator, cond.value); err != nil {
				return m.setTempStatus("Invalid value: " + err.Error())
			}
		}
		if m.smartEditIdx >= 0 && m.smartEditIdx < len(b.conditions) {
			b.conditions[m.smartEditIdx] = cond
		} else {
			b.conditions = append(b.conditions, cond)
		}
		m.smartEditIdx = -1
		m.smartEditField = ""
		m.smartEditOp = ""
		m.rebuildSmartLevel(b)
		return m, nil
	}
	return m, nil
}

// handleKey maps a key press to an action, then runs the action.
func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	// Ctrl+C and the arrow keys are always available, whatever the config.
	if key == "ctrl+c" {
		m.stopPlayer()
		return m, tea.Quit
	}
	switch key {
	case "up":
		m.moveCursor(-1)
		return m, nil
	case "down":
		m.moveCursor(1)
		return m, nil
	}

	// A layout toggle (show or hide the navigation panel) changes the
	// width of the queue pane. Bubble Tea's line diff can leave a stale
	// row on screen after such a width change, so force a full repaint
	// when the panel visibility flips.
	prevShown := m.rightShown
	nm, cmd := m.dispatchAction(key)
	if mm, ok := nm.(model); ok && mm.rightShown != prevShown {
		return nm, tea.Batch(cmd, tea.ClearScreen)
	}
	return nm, cmd
}

// dispatchAction maps a resolved key to its action and runs it.
func (m model) dispatchAction(key string) (tea.Model, tea.Cmd) {
	action, ok := m.keyAction[key]
	if !ok {
		// The space key reports as " " but configs name it "space".
		if key == " " {
			action, ok = m.keyAction["space"]
		}
	}
	if !ok {
		return m, nil
	}

	// While the cover view is focused, Space cycles the render mode
	// instead of its normal action, so the modes are easy to compare.
	// Tab is left alone so it still moves focus back to the queue.
	if m.activeView == viewCover && m.focus == focusRight && action == config.ActAddAll {
		m.coverMode = (m.coverMode + 1) % coverModeCount
		return m, nil
	}

	switch action {
	case config.ActQuit:
		m.stopPlayer()
		return m, tea.Quit

	case config.ActHideRight:
		m.rightShown = false
		m.focus = focusQueue
		return m, nil
	case config.ActShowNav:
		// Switch to the browse view. Pop any info level pushed onto the
		// browse stack, so 2 returns to plain browsing.
		m.activeView = viewBrowse
		for len(m.browseNav) > 1 && m.browseNav[len(m.browseNav)-1].isOverlay() {
			m.browseNav = m.browseNav[:len(m.browseNav)-1]
		}
		m.rightShown = true
		m.focus = focusRight
		return m, nil
	case config.ActShowSearch:
		// Switch to the search view. The form keeps its state.
		m.activeView = viewSearch
		m.rightShown = true
		m.focus = focusRight
		return m, nil
	case config.ActShowLyrics:
		return m.showLyrics()
	case config.ActShowCover:
		return m.showCover()
	case config.ActLyricsSearch:
		return m.startLyricsSearch()
	case config.ActLyricsEarlier:
		return m.adjustLyricsOffset(-lyricsOffsetStep)
	case config.ActLyricsLater:
		return m.adjustLyricsOffset(lyricsOffsetStep)
	case config.ActRateUp:
		return m.rateUp()
	case config.ActRateDown:
		return m.rateDown()

	case config.ActFocusToggle:
		m.toggleFocus()
		return m, nil

	case config.ActConsume:
		return m.toggleConsume()
	case config.ActRepeat:
		return m.toggleRepeat()
	case config.ActShuffle:
		m.shuffle = !m.shuffle
		if m.shuffle {
			return m.setTempStatus("Random mode: on")
		}
		return m.setTempStatus("Random mode: off")
	case config.ActCrossfade:
		m.crossfade = !m.crossfade
		if m.crossfade {
			m.player.SetCrossfade(m.crossfadeSec)
			return m.setTempStatus(fmt.Sprintf("Crossfade: on (%d s)", m.crossfadeSec))
		}
		m.player.SetCrossfade(0)
		return m.setTempStatus("Crossfade: off")
	case config.ActCrossfadeSet:
		m.promptActive = true
		m.promptKind = promptCrossfade
		m.promptLabel = "Crossfade seconds"
		m.promptInput = ""
		return m, nil

	case config.ActRadio:
		return m.startRadioFromView()
	case config.ActRadioSet:
		m.promptActive = true
		m.promptKind = promptRadio
		m.promptLabel = "Radio queue size"
		m.promptInput = ""
		return m, nil

	case config.ActSavePlaylist:
		return m.startSavePrompt()
	case config.ActSaveOverwrite:
		return m.saveOverwrite()

	case config.ActDelete:
		return m.deleteSelected()
	case config.ActEdit:
		return m.editSmartPlaylist()
	case config.ActClear:
		return m.clearQueue()

	case config.ActSongInfo:
		return m.showSongInfo()
	case config.ActArtistInfo:
		return m.showArtistInfo()
	case config.ActHelp:
		m.showHelp = true
		return m, nil
	case config.ActVisualizer:
		m.showVis = true
		m.visPeaks = nil
		m.visPeakVel = nil
		m.visLevels = nil
		m.visRadialLevels = nil
		m.visStereoL = nil
		m.visStereoR = nil
		m.visWave = nil
		m.visParticles = particleState{}
		return m, visTick()

	case config.ActUp:
		m.moveCursor(-1)
		return m, nil
	case config.ActDown:
		m.moveCursor(1)
		return m, nil
	case config.ActHome:
		m.cursorHome()
		return m, nil
	case config.ActEnd:
		m.cursorEnd()
		return m, nil
	case config.ActPageUp:
		m.moveCursor(-m.pageSize())
		return m, nil
	case config.ActPageDown:
		m.moveCursor(m.pageSize())
		return m, nil

	case config.ActSelect:
		return m.openItem()
	case config.ActAdd:
		return m.addToQueue()
	case config.ActAddAll:
		return m.addAllToQueue()

	case config.ActPause:
		m.player.TogglePause()
		return m, nil
	case config.ActStop:
		m.stopPlayer()
		m.queueIndex = -1
		m.transcoding = false
		m.status = "Stopped."
		return m, nil
	case config.ActNext:
		return m.advance(false)
	case config.ActPrev:
		return m.smartPrev()
	case config.ActSeekForward:
		m.player.Seek(m.seekSec)
		return m, nil
	case config.ActSeekBack:
		m.player.Seek(-m.seekSec)
		return m, nil
	case config.ActVolumeUp:
		m.player.SetVolumePercent(m.player.VolumePercent() + volumeStep)
		return m, nil
	case config.ActVolumeDown:
		m.player.SetVolumePercent(m.player.VolumePercent() - volumeStep)
		return m, nil
	}
	return m, nil
}

// handleControl maps an external control command to the matching action.
// It replies to the client with a short status and returns the updated
// model and any command. A command that needs network data (playlist)
// runs in a tea.Cmd so Update does not block.
func (m model) handleControl(msg controlMsg) (tea.Model, tea.Cmd) {
	reply := func(s string) {
		if msg.reply != nil {
			select {
			case msg.reply <- s:
			default:
			}
		}
	}

	switch msg.name {
	case "pause", "play", "playpause", "toggle":
		m.player.TogglePause()
		reply("ok")
		return m, nil
	case "stop":
		m.stopPlayer()
		m.queueIndex = -1
		m.transcoding = false
		m.status = "Stopped."
		reply("ok")
		return m, nil
	case "next":
		reply("ok")
		return m.advance(false)
	case "prev", "previous":
		reply("ok")
		return m.smartPrev()
	case "seek":
		n, err := strconv.Atoi(strings.TrimSpace(msg.arg))
		if err != nil {
			reply("err: seek needs a number of seconds")
			return m, nil
		}
		m.player.Seek(n)
		reply("ok")
		return m, nil
	case "volume", "vol":
		return m.controlVolume(msg.arg, reply)
	case "playlist":
		name := strings.TrimSpace(msg.arg)
		if name == "" {
			reply("err: playlist needs a name")
			return m, nil
		}
		reply("ok")
		return m, m.playPlaylistByName(name)
	default:
		reply("err: unknown command " + msg.name)
		return m, nil
	}
}

// controlVolume applies a volume control argument: "+", "-", or an absolute
// percent number.
func (m model) controlVolume(arg string, reply func(string)) (tea.Model, tea.Cmd) {
	arg = strings.TrimSpace(arg)
	cur := m.player.VolumePercent()
	switch arg {
	case "+", "up":
		m.player.SetVolumePercent(cur + volumeStep)
		reply("ok")
		return m, nil
	case "-", "down":
		m.player.SetVolumePercent(cur - volumeStep)
		reply("ok")
		return m, nil
	}
	n, err := strconv.Atoi(arg)
	if err != nil {
		reply("err: volume needs +, -, or a percent")
		return m, nil
	}
	m.player.SetVolumePercent(n)
	reply("ok")
	return m, nil
}

// startSavePrompt opens the text prompt for a playlist name.
func (m model) startSavePrompt() (tea.Model, tea.Cmd) {
	if len(m.queue) == 0 {
		return m.setTempStatus("The queue is empty.")
	}
	m.promptActive = true
	m.promptKind = promptSaveNew
	m.promptLabel = "Save queue as playlist"
	m.promptInput = ""
	return m, nil
}

// saveOverwrite overwrites the source playlist with the current queue. If
// the queue did not come from a playlist, it asks for a name like S.
func (m model) saveOverwrite() (tea.Model, tea.Cmd) {
	if len(m.queue) == 0 {
		return m.setTempStatus("The queue is empty.")
	}
	if m.sourcePlaylistID == "" {
		return m.startSavePrompt()
	}
	m.status = "Overwriting playlist..."
	return m, m.overwritePlaylist(m.sourcePlaylistID, m.sourcePlaylistName, m.queue)
}

// deleteSelected removes the selected item. On the queue pane it removes
// the queue song. On the Playlists view it deletes the playlist after a
// confirm.
func (m model) deleteSelected() (tea.Model, tea.Cmd) {
	if m.focus == focusRight {
		lvl := m.top()
		if lvl.kind == navPlaylists {
			row, ok := lvl.selected()
			if !ok {
				return m, nil
			}
			if row.id == newSmartRowID {
				return m, nil
			}
			id, name := row.id, row.name
			label := fmt.Sprintf("Delete playlist %q? (y/n)", name)
			if nspFileFor(m.nspPath, name) != "" {
				label = fmt.Sprintf("Delete smart playlist %q and its .nsp file? (y/n)", name)
			}
			m.confirmActive = true
			m.confirmLabel = label
			m.confirmAction = func(mm model) (model, tea.Cmd) {
				mm.status = "Deleting playlist..."
				return mm, mm.deletePlaylistCmd(id, name)
			}
			return m, nil
		}
		return m, nil
	}
	// Queue pane: remove the selected song.
	i := m.queueCursor
	if i < 0 || i >= len(m.queue) {
		return m, nil
	}
	m.queue = append(m.queue[:i], m.queue[i+1:]...)
	if m.queueIndex == i {
		// The current song was deleted. It keeps playing, but it is no
		// longer in the queue. Mark it as not indexed.
		m.queueIndex = -1
	} else if m.queueIndex > i {
		m.queueIndex--
	}
	if m.queueCursor >= len(m.queue) && m.queueCursor > 0 {
		m.queueCursor = len(m.queue) - 1
	}
	return m, nil
}

// clearQueue asks to clear the whole queue when it is not empty.
func (m model) clearQueue() (tea.Model, tea.Cmd) {
	if len(m.queue) == 0 {
		return m, nil
	}
	m.confirmActive = true
	m.confirmLabel = "Clear the queue? (y/n)"
	m.confirmAction = func(mm model) (model, tea.Cmd) {
		mm.stopPlayer()
		mm.queue = nil
		mm.queueCursor = 0
		mm.queueIndex = -1
		mm.sourcePlaylistID = ""
		mm.sourcePlaylistName = ""
		mm.status = "Queue cleared."
		return mm, nil
	}
	return m, nil
}

// showSongInfo opens the info level for the selected song. It works on a
// queue song or a track in the panel.
func (m model) showSongInfo() (tea.Model, tea.Cmd) {
	var s subsonic.Song
	ok := false
	if m.focus == focusQueue {
		if m.queueCursor >= 0 && m.queueCursor < len(m.queue) {
			s = m.queue[m.queueCursor]
			ok = true
		}
	} else {
		lvl := m.top()
		if row, has := lvl.selected(); has && lvl.kind == navTracks {
			s = row.song
			ok = true
		}
	}
	if !ok {
		return m, nil
	}
	// Info levels live on the browse stack. Switch to it so a non-browse
	// active view (search, lyrics, cover) does not receive the push and
	// then overwrite it on the next refresh.
	m.activeView = viewBrowse
	m.pushLoading("Song info")
	return m, m.loadSongInfo(s)
}

// showArtistInfo opens the stub artist-info level.
func (m model) showArtistInfo() (tea.Model, tea.Cmd) {
	name := ""
	if m.focus == focusQueue {
		if m.queueCursor >= 0 && m.queueCursor < len(m.queue) {
			name = m.queue[m.queueCursor].Artist
		}
	} else {
		lvl := m.top()
		if row, has := lvl.selected(); has {
			switch lvl.kind {
			case navArtists:
				name = row.label
			case navTracks:
				name = row.song.Artist
			}
		}
	}
	if name == "" {
		return m, nil
	}
	// Info levels live on the browse stack (see showSongInfo).
	m.activeView = viewBrowse
	m.pushLevel(artistInfoLevel(name))
	m.focus = focusRight
	m.rightShown = true
	return m, nil
}

// selectedSong returns the song the keys act on: the queue song when the
// queue pane has focus, or the selected panel track when the right pane has
// focus. The bool is false when nothing suitable is selected.
func (m model) selectedSong() (subsonic.Song, bool) {
	if m.focus == focusQueue {
		if m.queueCursor >= 0 && m.queueCursor < len(m.queue) {
			return m.queue[m.queueCursor], true
		}
		return subsonic.Song{}, false
	}
	lvl := m.top()
	if row, has := lvl.selected(); has && row.song.ID != "" {
		return row.song, true
	}
	return subsonic.Song{}, false
}

// showLyrics switches to the lyrics view. The view shows the currently
// playing song's lyrics. When nothing plays, it shows an empty placeholder.
// It needs at least one usable lyrics tier: the cache, the dump, or a
// network provider. Without any, it shows a status message.
func (m model) showLyrics() (tea.Model, tea.Cmd) {
	if !m.lyricsSrc.Usable() {
		return m.setTempStatus("Lyrics are off.")
	}
	m.activeView = viewLyrics
	m.focus = focusRight
	m.rightShown = true

	s, ok := m.currentSong()
	if !ok {
		// Nothing is playing. Show the empty placeholder.
		m.lyricsNav = []navLevel{lyricsPlaceholderLevel()}
		return m, nil
	}
	// Already showing this song's lyrics (loaded or a lookup in flight)?
	// Keep them.
	if top := m.lyricsNav[len(m.lyricsNav)-1]; top.lyricsSong == s.ID {
		return m, nil
	}
	m.lyricsNav = []navLevel{lyricsLoadingLevel(s.ID)}
	return m, m.loadLyrics(s)
}

// showCover switches to the cover-art view for the currently playing song.
// When nothing plays, it shows an empty placeholder.
func (m model) showCover() (tea.Model, tea.Cmd) {
	m.activeView = viewCover
	m.focus = focusRight
	m.rightShown = true

	s, ok := m.currentSong()
	if !ok {
		m.coverNav = []navLevel{coverPlaceholderLevel()}
		return m, nil
	}
	// Already showing this song's art (loaded or a download in flight)?
	// Keep it.
	if top := m.coverNav[len(m.coverNav)-1]; top.coverSong == s.ID {
		return m, nil
	}
	m.coverNav = []navLevel{coverLoadingLevel(s.ID)}
	return m, m.loadCover(s)
}

// refreshCoverIfPlaying reloads the cover view for the current song when
// the view holds a different song. It returns a command to run, or nil.
// When nothing plays, it resets the view to the empty placeholder.
func (m *model) refreshCoverIfPlaying() tea.Cmd {
	if len(m.coverNav) == 0 {
		return nil
	}
	s, ok := m.currentSong()
	if !ok {
		m.coverNav = []navLevel{coverPlaceholderLevel()}
		return nil
	}
	top := m.coverNav[len(m.coverNav)-1]
	if top.coverSong == s.ID {
		return nil
	}
	m.coverNav = []navLevel{coverLoadingLevel(s.ID)}
	return m.loadCover(s)
}

// startLyricsSearch opens the freetext lyrics search prompt. It works
// only while the lyrics view is active.
func (m model) startLyricsSearch() (tea.Model, tea.Cmd) {
	if m.activeView != viewLyrics {
		return m.setTempStatus("Lyrics search works in the lyrics view (4).")
	}
	if !m.lyricsSrc.Usable() {
		return m.setTempStatus("Lyrics are off.")
	}
	m.promptActive = true
	m.promptKind = promptLyricsSearch
	m.promptLabel = "Search lyrics"
	m.promptInput = ""
	return m, nil
}

// lyricsOffsetStep is how much one timing-adjust key press shifts the
// lyric timestamps.
const lyricsOffsetStep = 100 * time.Millisecond

// lyricsOffsetMax bounds the timing offset in both directions.
const lyricsOffsetMax = 10 * time.Second

// adjustLyricsOffset shifts the lyric timing of the playing song by delta.
// A negative delta makes the lines show earlier, a positive one later. It
// works only while the lyrics view shows synced lyrics. The offset
// persists per song through the lyrics cache.
func (m model) adjustLyricsOffset(delta time.Duration) (tea.Model, tea.Cmd) {
	if m.activeView != viewLyrics {
		return m, nil
	}
	lvl := m.topLevel()
	if lvl.kind != navLyrics || !lvl.lyricsSynced || len(lvl.lyrics) == 0 {
		return m.setTempStatus("No synced lyrics to adjust.")
	}
	m.lyricsOffset += delta
	if m.lyricsOffset > lyricsOffsetMax {
		m.lyricsOffset = lyricsOffsetMax
	}
	if m.lyricsOffset < -lyricsOffsetMax {
		m.lyricsOffset = -lyricsOffsetMax
	}
	status := fmt.Sprintf("Lyrics offset: %+.1fs", m.lyricsOffset.Seconds())
	if s, ok := m.currentSong(); ok {
		tm, cmd := m.setTempStatus(status)
		return tm, tea.Batch(cmd, m.saveLyricsOffset(s))
	}
	return m.setTempStatus(status)
}

// pickLyricsHit shows the lyrics of one search hit. When a song plays, a
// command saves the lyrics to the cache for that exact song, so the
// automatic lookup finds them from now on.
func (m model) pickLyricsHit(hit lyrics.Hit) (tea.Model, tea.Cmd) {
	lvl := navLevel{
		kind:         navLyrics,
		title:        "Lyrics " + lyricsQualityMark(hit.Synced, false) + hit.Title,
		cursor:       -1,
		lyrics:       lyricLinesFrom(hit.Lyrics),
		lyricsSynced: hit.Synced,
		lyricsManual: true,
	}
	m.lyricsNav = []navLevel{lvl}
	note := "Lyrics: " + hit.Artist + " - " + hit.Title
	if s, ok := m.currentSong(); ok {
		m.status = note
		return m, m.saveManualLyrics(s, hit.Lyrics)
	}
	return m.setTempStatus(note)
}

// saveManualLyrics stores user-picked lyrics in the cache tier under the
// identity of one song. It writes nothing when the cache is off.
func (m model) saveManualLyrics(s subsonic.Song, res lyrics.Result) tea.Cmd {
	src := m.lyricsSrc
	return func() tea.Msg {
		src.Put(s.Artist, s.Title, s.Album, s.Duration, res)
		return nil
	}
}

// refreshLyricsIfPlaying reloads the lyrics view for the current song when
// the lyrics view holds a different song. It returns a command to run, or
// nil. When nothing plays, it resets the view to the empty placeholder.
func (m *model) refreshLyricsIfPlaying() tea.Cmd {
	if len(m.lyricsNav) == 0 {
		return nil
	}
	// An open search view stays open. The user leaves it by picking a
	// hit, pressing 4, or the [..] row.
	if len(m.lyricsNav) > 1 {
		return nil
	}
	s, ok := m.currentSong()
	if !ok {
		m.lyricsNav = []navLevel{lyricsPlaceholderLevel()}
		return nil
	}
	top := m.lyricsNav[len(m.lyricsNav)-1]
	// Already loaded, a lookup for this song is already in flight, or the
	// user pinned lyrics from a search hit.
	if top.lyricsSong == s.ID || top.lyricsManual {
		return nil
	}
	m.lyricsNav = []navLevel{lyricsLoadingLevel(s.ID)}
	return m.loadLyrics(s)
}

// prefetchLyrics starts a lyrics lookup for a song that just became the
// current track, so the lyrics are ready before the user opens the lyrics
// view. It runs regardless of the active view. It returns a command to run,
// or nil when the lyrics source is off or the lyrics are already loaded for
// this song. The lyricsMsg handler stores the result and guards staleness.
func (m *model) prefetchLyrics(s subsonic.Song) tea.Cmd {
	if !m.lyricsSrc.Usable() {
		return nil
	}
	// An open search view stays open; do not clobber it.
	if len(m.lyricsNav) > 1 {
		return nil
	}
	// Already loaded, or a lookup for this song is already in flight.
	if len(m.lyricsNav) > 0 {
		top := m.lyricsNav[len(m.lyricsNav)-1]
		if top.lyricsSong == s.ID {
			return nil
		}
	}
	m.lyricsNav = []navLevel{lyricsLoadingLevel(s.ID)}
	return m.loadLyrics(s)
}

// prefetchCover starts a cover-art download for a song that just became the
// current track, so the art is ready before the user opens the cover view.
// It runs regardless of the active view. It returns a command, or nil when
// the art is already loaded or in flight for this song.
func (m *model) prefetchCover(s subsonic.Song) tea.Cmd {
	if len(m.coverNav) > 0 {
		top := m.coverNav[len(m.coverNav)-1]
		if top.coverSong == s.ID {
			return nil
		}
	}
	m.coverNav = []navLevel{coverLoadingLevel(s.ID)}
	return m.loadCover(s)
}

func (m model) rateUp() (tea.Model, tea.Cmd) {
	s, ok := m.ratingTarget()
	if !ok {
		return m, nil
	}
	r := s.UserRating + 1
	if r > 5 {
		r = 5
	}
	return m, m.rateSong(s.ID, r)
}

// rateDown lowers the rating of the selected or current song by one, down
// to a minimum of 0. Subsonic ratings do not go negative; 0 means unrated
// and 1 marks a dislike.
func (m model) rateDown() (tea.Model, tea.Cmd) {
	s, ok := m.ratingTarget()
	if !ok {
		return m, nil
	}
	r := s.UserRating - 1
	if r < 0 {
		r = 0
	}
	return m, m.rateSong(s.ID, r)
}

// ratingTarget returns the song a rating action acts on: the selected
// song, or the current song when nothing is selected.
func (m model) ratingTarget() (subsonic.Song, bool) {
	s, ok := m.selectedSong()
	if !ok {
		if m.currentSongID() != "" {
			s = m.queue[m.queueIndex]
			ok = true
		}
	}
	return s, ok
}

// updateRatingInNav updates the rating of a song in every navigation
// level that holds it as a track row, across all three views.
func (m *model) updateRatingInNav(id string, rating int) {
	stacks := [][]navLevel{m.browseNav, m.searchNav, m.lyricsNav}
	for _, st := range stacks {
		for li := range st {
			lvl := &st[li]
			for ri := range lvl.rows {
				if lvl.rows[ri].song.ID == id {
					lvl.rows[ri].song.UserRating = rating
				}
			}
		}
	}
}

// restoreFromQueue replaces the queue with a queue saved on the server.
// It loads the current track and holds it paused at the saved position.
// It does not start sound. The load runs in a command, so the download
// does not block the interface loop. The playStartedMsg handler applies
// the result.
func (m model) restoreFromQueue(pq subsonic.PlayQueue) (tea.Model, tea.Cmd) {
	m.queue = pq.Songs
	m.queueCursor = 0
	m.queueIndex = pq.CurrentIndex
	if m.queueIndex < 0 || m.queueIndex >= len(m.queue) {
		m.queueIndex = -1
		m.refreshXF()
		return m.setTempStatus(fmt.Sprintf("Restored queue (%d songs)", len(m.queue)))
	}
	m.queueCursor = m.queueIndex
	s := m.queue[m.queueIndex]
	url, suffix, err := streamSource(m.client, s)
	if err != nil {
		m.status = "Restore error: " + err.Error()
		m.refreshXF()
		return m, nil
	}
	req := m.playRequestFor(s, url, suffix, false, playKindRestore)
	req.pausedAt = int(pq.Position / 1000)
	m.refreshXF()
	prefetch := m.prefetchLyrics(s)
	prefetchC := m.prefetchCover(s)
	tm, statusCmd := m.setTempStatus(fmt.Sprintf("Restored queue (%d songs, loading track)...", len(m.queue)))
	return tm, tea.Batch(req.cmd(), prefetch, prefetchC, statusCmd)
}

// smartPrev restarts the current song, or goes to the previous song when
func (m model) smartPrev() (tea.Model, tea.Cmd) {
	elapsed, _ := m.player.Position()
	within := time.Since(m.lastPrev) < 2*time.Second
	m.lastPrev = time.Now()

	// If we are past two seconds into the song, restart it.
	if elapsed >= 2*time.Second && !within {
		m.player.Seek(-int(elapsed.Seconds()) - 1)
		return m, nil
	}
	// Otherwise go to the previous song.
	if m.queueIndex <= 0 {
		// No previous song: restart the current one.
		m.player.Seek(-int(elapsed.Seconds()) - 1)
		return m, nil
	}
	m.queueIndex--
	return m.playCurrent()
}

// toggleConsume flips consume mode and shows a timed status message.
// Enabling consume turns repeat off.
func (m model) toggleConsume() (tea.Model, tea.Cmd) {
	m.consume = !m.consume
	if m.consume {
		if m.repeat {
			m.repeat = false
			return m.setTempStatus("Consume mode: on (repeat off)")
		}
		return m.setTempStatus("Consume mode: on")
	}
	return m.setTempStatus("Consume mode: off")
}

// toggleRepeat flips repeat mode and shows a timed status message.
// Enabling repeat turns consume off.
func (m model) toggleRepeat() (tea.Model, tea.Cmd) {
	m.repeat = !m.repeat
	if m.repeat {
		if m.consume {
			m.consume = false
			return m.setTempStatus("Repeat mode: on (consume off)")
		}
		return m.setTempStatus("Repeat mode: on")
	}
	return m.setTempStatus("Repeat mode: off")
}

// startRadioFromView starts radio (feeder) mode from the focused right-pane
// source. On a playlist row it loads the playlist songs over the network. On
// a Tracks level (including search results) it seeds from the level's songs.
// Other sources show a note. When radio is already on, it turns it off.
func (m model) startRadioFromView() (tea.Model, tea.Cmd) {
	if m.radio {
		return m.toggleRadioOff()
	}
	if m.focus != focusRight {
		return m.setTempStatus("Radio: focus a playlist or search result first.")
	}
	lvl := m.top()
	switch lvl.kind {
	case navPlaylists:
		row, ok := lvl.selected()
		if !ok || row.id == newSmartRowID {
			return m.setTempStatus("Radio: select a playlist.")
		}
		nm, cmd := m.setTempStatus("Radio: loading " + row.label + "...")
		return nm, tea.Batch(cmd, m.loadRadioPlaylist(row.id, row.name))
	case navTracks:
		songs := make([]subsonic.Song, 0, len(lvl.rows))
		for _, r := range lvl.rows {
			songs = append(songs, r.song)
		}
		return m.startRadio(songs, lvl.title)
	default:
		return m.setTempStatus("Radio: needs a playlist or a Tracks list.")
	}
}

// startRadio turns radio (feeder) mode on with pool as the source. It
// forces consume on and repeat off, replaces the queue with radioSize
// songs drawn from the pool, and starts playback at the first song. An
// empty pool shows a note and leaves the mode off.
func (m model) startRadio(pool []subsonic.Song, label string) (tea.Model, tea.Cmd) {
	if len(pool) == 0 {
		return m.setTempStatus("Radio: source has no songs.")
	}
	m.stopPlayer()
	m.radio = true
	m.radioPool = pool
	m.consume = true
	m.repeat = false
	m.queue = nil
	m.queueCursor = 0
	m.queueIndex = -1
	m.sourcePlaylistID = ""
	m.sourcePlaylistName = ""
	m.topUpInPlace()
	if len(m.queue) == 0 {
		return m.setTempStatus("Radio: source has no songs.")
	}
	m.queueIndex = 0
	tm, cmd := m.playCurrent()
	nm := tm.(model)
	nm.refreshXF()
	sm, scmd := nm.setTempStatus(fmt.Sprintf("Radio mode: on (%s)", label))
	return sm, tea.Batch(cmd, scmd)
}

// toggleRadioOff turns radio (feeder) mode off. It leaves the queue and
// playback as they are.
func (m model) toggleRadioOff() (tea.Model, tea.Cmd) {
	m.radio = false
	m.radioPool = nil
	return m.setTempStatus("Radio mode: off")
}

// topUpInPlace appends random pool songs until the queue holds radioSize
// songs. It avoids adding a song identical to the current queue tail. It
// does nothing when radio is off, the pool is empty, or the queue already
// holds radioSize or more songs.
func (m *model) topUpInPlace() {
	if !m.radio || len(m.radioPool) == 0 || m.radioSize < 1 {
		return
	}
	for len(m.queue) < m.radioSize {
		s := m.radioPool[rand.Intn(len(m.radioPool))]
		if len(m.queue) > 0 && len(m.radioPool) > 1 && m.queue[len(m.queue)-1].ID == s.ID {
			s = m.radioPool[rand.Intn(len(m.radioPool))]
		}
		m.queue = append(m.queue, s)
	}
}

// topUpRadio tops the queue up on a value receiver and refreshes the
// crossfade snapshot. It returns the updated model for use in Update.
func (m model) topUpRadio() model {
	if !m.radio {
		return m
	}
	before := len(m.queue)
	m.topUpInPlace()
	if len(m.queue) != before {
		m.refreshXF()
	}
	return m
}

// cursorHome moves the cursor of the focused pane to the top.
func (m *model) cursorHome() {
	if m.focus == focusQueue {
		m.queueCursor = 0
		return
	}
	lvl := m.top()
	if lvl.hasBackRow() {
		lvl.cursor = -1
	} else {
		lvl.cursor = 0
	}
}

// cursorEnd moves the cursor of the focused pane to the bottom.
func (m *model) cursorEnd() {
	if m.focus == focusQueue {
		if len(m.queue) > 0 {
			m.queueCursor = len(m.queue) - 1
		}
		return
	}
	lvl := m.top()
	if len(lvl.rows) > 0 {
		lvl.cursor = len(lvl.rows) - 1
	}
}

// setTempStatus shows text for statusHold, then lets the normal status
// return. It returns a command that forces a redraw when the time ends.
func (m model) setTempStatus(text string) (tea.Model, tea.Cmd) {
	m.status = text
	m.statusExpiry = time.Now().Add(statusHold)
	return m, tea.Tick(statusHold, func(time.Time) tea.Msg {
		return statusExpiredMsg{}
	})
}

// toggleFocus moves focus between the queue pane and the right pane. If
// the right pane is hidden, focus stays on the queue.
func (m *model) toggleFocus() {
	if !m.rightShown {
		m.focus = focusQueue
		return
	}
	if m.focus == focusQueue {
		m.focus = focusRight
	} else {
		m.focus = focusQueue
	}
}

// moveCursor moves the cursor of the focused pane by delta.
func (m *model) moveCursor(delta int) {
	if m.focus == focusQueue {
		m.queueCursor = clamp(m.queueCursor+delta, len(m.queue))
		return
	}
	lvl := m.top()
	low := 0
	if lvl.hasBackRow() {
		low = -1 // the back row sits above the first real row
	}
	c := lvl.cursor + delta
	if c < low {
		c = low
	}
	if c > len(lvl.rows)-1 {
		c = len(lvl.rows) - 1
	}
	if len(lvl.rows) == 0 {
		c = low
	}
	lvl.cursor = c
}

// clamp keeps an index inside the range 0..n-1.
func clamp(i, n int) int {
	if n == 0 {
		return 0
	}
	if i < 0 {
		return 0
	}
	if i >= n {
		return n - 1
	}
	return i
}

// openItem reacts to the Enter key on the focused pane.
func (m model) openItem() (tea.Model, tea.Cmd) {
	if m.focus == focusQueue {
		if len(m.queue) == 0 {
			return m, nil
		}
		m.queueIndex = m.queueCursor
		return m.playCurrent()
	}

	lvl := m.top()
	if lvl.onBackRow() {
		m.popLevel()
		return m, nil
	}
	row, ok := lvl.selected()
	if !ok {
		return m, nil
	}

	switch lvl.kind {
	case navRoot:
		return m.openEntrypoint(row.id)
	case navArtists:
		m.startLoad()
		return m, m.loadArtistAlbums(row.id, row.label)
	case navGenres:
		m.startLoad()
		return m, m.loadGenreAlbums(row.id)
	case navYears:
		m.startLoad()
		return m, m.loadYearAlbums(row.year)
	case navAlbums:
		m.startLoad()
		return m, m.loadAlbumTracks(row.id, row.label)
	case navTracks:
		// Enter on a track plays it now: insert at the top and play.
		if m.consume {
			m.consumeCurrent()
		}
		m.queue = append([]subsonic.Song{row.song}, m.queue...)
		m.queueIndex = 0
		return m.playCurrent()
	case navPlaylists:
		if row.id == newSmartRowID {
			return m.openSmartBuilder()
		}
		// Enter on a playlist replaces the queue with the playlist and
		// plays the first song, unless random mode is on.
		m.status = "Loading playlist: " + row.label + "..."
		return m, m.replacePlaylist(row.id, row.name, true)
	case navSearch:
		return m.openSearchRow(row)
	case navLyricsSearch:
		// Enter on a hit shows its lyrics; a playing song keeps them.
		if lvl.cursor >= 0 && lvl.cursor < len(lvl.lyricsHits) {
			return m.pickLyricsHit(lvl.lyricsHits[lvl.cursor])
		}
		return m, nil
	case navSmartBuilder:
		return m.openSmartBuilderRow(row)
	case navSmartField:
		return m.openSmartFieldRow(row)
	case navSmartOp:
		return m.openSmartOpRow(row)
	}
	return m, nil
}

// openSearchRow reacts to Enter on a search-form row. A field row opens the
// text prompt. The Search row runs the search. The Reset row clears the
// fields.
func (m model) openSearchRow(row navRow) (tea.Model, tea.Cmd) {
	lvl := m.top()
	if lvl.search == nil {
		return m, nil
	}
	switch row.id {
	case searchRowSearch:
		if lvl.search.empty() {
			return m.setTempStatus("Enter a search term first.")
		}
		form := lvl.search
		m.pushLoading("Search results")
		return m, m.runSearch(form)
	case searchRowReset:
		lvl.search.reset()
		return m.setTempStatus("Search fields cleared.")
	default:
		// A field row: open the text prompt to edit its value.
		key := searchFieldKey(row.id)
		m.searchEditKey = row.id
		m.promptActive = true
		m.promptKind = promptSearchField
		m.promptLabel = "Search " + row.label
		m.promptInput = lvl.search.value(key)
		return m, nil
	}
}

// openEntrypoint drills into one of the root entrypoints.
func (m model) openEntrypoint(id string) (tea.Model, tea.Cmd) {
	switch id {
	case epArtists:
		m.pushLoading("Artists")
		return m, m.loadArtists()
	case epAlbums:
		m.pushLoading("Albums")
		return m, m.loadAllAlbums()
	case epGenres:
		m.pushLoading("Genres")
		return m, m.loadGenres()
	case epYears:
		m.pushLoading("Release Years")
		return m, m.loadYears()
	case epTracks:
		m.pushLoading("Tracks")
		return m, m.loadTracks()
	case epPlaylists:
		m.pushLoading("Playlists")
		return m, m.loadPlaylists()
	}
	return m, nil
}

// pushLoading adds a placeholder level while a load command runs.
func (m *model) pushLoading(title string) {
	m.pushLevel(navLevel{kind: navRoot, title: title, loading: true})
	m.status = "Loading " + title + "..."
}

// startLoad marks the current level as reused for the next load. A new
// level replaces it when the load returns.
func (m *model) startLoad() {
	// Push a placeholder that the loaded level replaces.
	m.pushLevel(navLevel{loading: true, title: "Loading..."})
	m.status = "Loading..."
}

// addToQueue adds the selected track to the bottom of the queue.
func (m model) addToQueue() (tea.Model, tea.Cmd) {
	if m.focus != focusRight {
		return m, nil
	}
	lvl := m.top()
	if lvl.kind != navTracks {
		return m, nil
	}
	row, ok := lvl.selected()
	if !ok {
		return m, nil
	}
	m.queue = append(m.queue, row.song)
	m.status = "Added: " + row.song.Title
	return m, nil
}

// addAllToQueue adds every song under the selected item to the bottom of
// the queue. On a track it adds the track. On an album it adds the album.
// On an artist it adds every album. On a genre or a year it adds every
// album shown at the next level.
func (m model) addAllToQueue() (tea.Model, tea.Cmd) {
	if m.focus != focusRight {
		return m, nil
	}
	lvl := m.top()
	if lvl.onBackRow() {
		return m, nil
	}
	row, ok := lvl.selected()
	if !ok {
		return m, nil
	}

	switch lvl.kind {
	case navTracks:
		m.queue = append(m.queue, row.song)
		m.status = "Added: " + row.song.Title
		m.moveCursor(1)
		return m, nil
	case navAlbums:
		m.status = "Loading album: " + row.label + "..."
		m.moveCursor(1)
		return m, m.enqueueAlbum(row.id, row.label)
	case navArtists:
		m.status = "Loading artist: " + row.label + "..."
		m.moveCursor(1)
		return m, m.enqueueArtist(row.id, row.label)
	case navGenres:
		m.status = "Loading genre: " + row.label + "..."
		m.moveCursor(1)
		return m, m.enqueueGenre(row.id)
	case navYears:
		m.status = "Loading year: " + row.label + "..."
		m.moveCursor(1)
		return m, m.enqueueYear(row.year)
	case navPlaylists:
		m.status = "Loading playlist: " + row.label + "..."
		m.moveCursor(1)
		return m, m.enqueuePlaylist(row.id, row.label)
	}
	return m, nil
}

// enqueueGenre adds every song of every album of one genre.
func (m model) enqueueGenre(genre string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		albums, err := cl.GetAlbumsByGenre(genre)
		if err != nil {
			return enqueueMsg{label: genre, err: err}
		}
		return m.enqueueAlbums(albums, genre)()
	}
}

// enqueueYear adds every song of every album of one year.
func (m model) enqueueYear(year int) tea.Cmd {
	cl := m.client
	label := yearTitle(year)
	return func() tea.Msg {
		albums, err := cl.GetAlbumsByYear(year, year)
		if err != nil {
			return enqueueMsg{label: label, err: err}
		}
		return m.enqueueAlbums(albums, label)()
	}
}

// pageSize returns the number of list rows on one screen for the focused
// pane.
func (m model) pageSize() int {
	n := m.listHeight()
	if n < 1 {
		return 1
	}
	return n
}

// nextIndexPure computes the next queue index without mutating the model.
// It returns the index and stop=true when the queue should stop. It does
// not apply consume. When consumeRemoved is true, the current song was
// already removed, so the "next" song sits at the current index.
func nextIndexPure(queueLen, current int, repeat, shuffle, consumeRemoved bool) (idx int, stop bool) {
	if queueLen == 0 {
		return -1, true
	}
	if shuffle {
		if queueLen == 1 {
			return 0, false
		}
		n := rand.Intn(queueLen)
		for n == current {
			n = rand.Intn(queueLen)
		}
		return n, false
	}
	if consumeRemoved {
		// The current song is gone; the next one shifted into its slot.
		if current >= queueLen {
			return -1, true
		}
		return current, false
	}
	if current+1 >= queueLen {
		if repeat {
			return 0, false
		}
		return -1, true
	}
	return current + 1, false
}

// streamSource returns the stream URL and the suffix to decode for a song.
// When the player cannot decode the source suffix (for example m4a), it
// asks the server to transcode to a supported format and returns that
// format as the suffix. Otherwise it streams the original file.
func streamSource(cl *subsonic.Client, s subsonic.Song) (string, string, error) {
	if player.SupportedSuffix(s.Suffix) {
		u, err := cl.StreamURL(s.ID)
		return u, s.Suffix, err
	}
	u, err := cl.StreamURLFormat(s.ID, player.TranscodeFormat)
	return u, player.TranscodeFormat, err
}

// streamFor returns the stream URL and suffix for a queue index.
func (m model) streamFor(idx int) (url, suffix string, ok bool) {
	if idx < 0 || idx >= len(m.queue) {
		return "", "", false
	}
	s := m.queue[idx]
	u, suf, err := streamSource(m.client, s)
	if err != nil {
		return "", "", false
	}
	return u, suf, true
}

// playCurrent plays the track at queueIndex. The load runs in a command,
// so the download does not block the interface loop. The status shows
// Loading until the playStartedMsg handler applies the result.
func (m model) playCurrent() (tea.Model, tea.Cmd) {
	if m.queueIndex < 0 || m.queueIndex >= len(m.queue) {
		return m, nil
	}
	// Center the playing song in the queue view.
	m.queueCursor = m.queueIndex
	s := m.queue[m.queueIndex]
	url, suffix, err := streamSource(m.client, s)
	if err != nil {
		m.status = "Error: " + err.Error()
		return m, nil
	}
	// Manual play crossfades when the mode is on and a track already plays.
	crossfade := m.crossfade && m.player.State() != player.StateStopped
	req := m.playRequestFor(s, url, suffix, crossfade, playKindPlay)
	m.loadingPlay = true
	m.status = "Loading: " + s.Artist + " - " + s.Title
	m.refreshXF()
	return m, req.cmd()
}

// playStarted applies the result of one asynchronous play request. A
// dropped result does nothing: a newer request or a stop took over. On
// success it shows the playing status and prefetches the lyrics and the
// cover art.
func (m model) playStarted(msg playStartedMsg) (tea.Model, tea.Cmd) {
	if msg.dropped || !m.playGate.enter(msg.gen) {
		return m, nil
	}
	m.loadingPlay = false
	if msg.err != nil {
		if msg.kind == playKindRestore {
			m.status = "Restore error: " + msg.err.Error()
		} else {
			m.status = "Error: " + msg.err.Error()
		}
		return m, nil
	}
	m.transcoding = msg.transcoded
	switch msg.kind {
	case playKindRestore:
		if msg.transcoded {
			m.status = "Restored queue (paused, transcoding)"
		} else {
			m.status = fmt.Sprintf("Restored queue (%d songs, paused)", len(m.queue))
		}
	default:
		if msg.transcoded {
			m.status = "Transcoding: " + msg.song.Artist + " - " + msg.song.Title
		} else {
			m.status = "Playing: " + msg.song.Artist + " - " + msg.song.Title
		}
	}
	m.refreshXF()
	cmd := m.prefetchLyrics(msg.song)
	coverCmd := m.prefetchCover(msg.song)
	return m, tea.Batch(cmd, coverCmd)
}

// advance moves to the next track. ended is true when the current track
// finished on its own, false when the user skipped forward. In consume
// mode both cases remove the current song from the queue first.
//
// When crossfade is on and the track ended on its own, the player has
// already switched to the next track. In that case advance only updates
// the queue state and does not tell the player to play again.
func (m model) advance(ended bool) (tea.Model, tea.Cmd) {
	// When crossfade is on, the track ended on its own, consume is off,
	// and the provider pre-loaded a next track, the player already
	// switched. Use the index the provider chose, and do not replay.
	if ended && m.crossfade && !m.consume && m.xfShared != nil {
		m.xfShared.mu.Lock()
		pending := m.xfShared.pending
		m.xfShared.pending = -1
		m.xfShared.mu.Unlock()
		if pending >= 0 && pending < len(m.queue) {
			m.queueIndex = pending
			m.queueCursor = pending
			s := m.queue[pending]
			// The crossfade provider has no native-decode fallback, so a
			// transcode here happens only for a suffix the player never
			// decodes natively.
			m.transcoding = !player.SupportedSuffix(s.Suffix)
			if m.transcoding {
				m.status = "Transcoding: " + s.Artist + " - " + s.Title
			} else {
				m.status = "Playing: " + s.Artist + " - " + s.Title
			}
			go m.client.Scrobble(s.ID, false)
			m.refreshXF()
			cmd := m.prefetchLyrics(s)
			coverCmd := m.prefetchCover(s)
			return m, tea.Batch(cmd, coverCmd)
		}
		// No pre-load happened (end of queue). Fall through to stop.
	}

	consumeRemoved := false
	if m.consume && m.queueIndex >= 0 && m.queueIndex < len(m.queue) {
		m.consumeCurrent()
		consumeRemoved = true
	}

	// Radio mode refills the queue so it never runs dry after a consume.
	if m.radio {
		m.topUpInPlace()
	}

	idx, stop := nextIndexPure(len(m.queue), m.queueIndex, m.repeat, m.shuffle, consumeRemoved)
	if stop {
		m.stopPlayer()
		m.queueIndex = -1
		m.transcoding = false
		m.status = "Queue finished."
		m.refreshXF()
		return m, nil
	}
	m.queueIndex = idx
	tm, cmd := m.playCurrent()
	nm := tm.(model)
	nm.refreshXF()
	return nm, cmd
}

// consumeCurrent removes the current song from the queue. It keeps
// queueIndex pointing at the song that shifts into the vacated slot.
func (m *model) consumeCurrent() {
	i := m.queueIndex
	if i < 0 || i >= len(m.queue) {
		return
	}
	m.queue = append(m.queue[:i], m.queue[i+1:]...)
	if m.queueCursor >= len(m.queue) && m.queueCursor > 0 {
		m.queueCursor = len(m.queue) - 1
	}
}

// ensure the player import stays used.
var _ = player.StateStopped
