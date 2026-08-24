package ui

import (
	"fmt"
	"image"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"git.hemmalab.se/scuttle/tuiplay/internal/lyrics"
	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"

	tea "github.com/charmbracelet/bubbletea"
) // crossfadeShared is shared between the model and the player's next-track
// provider. The provider runs on the player's own goroutine, so a mutex
// guards the fields. It lets the provider and the queue agree on the next
// index during a natural crossfade.
type crossfadeShared struct {
	mu sync.Mutex
	// snapshot of the queue selection state, refreshed by the UI.
	queue   []subsonic.Song
	current int
	repeat  bool
	shuffle bool
	consume bool
	// pending is the index the provider chose for the next crossfade, or
	// -1 when none.
	pending int
}

// viewKind names one of the three parallel right-pane views.
type viewKind int

const (
	viewBrowse viewKind = iota
	viewSearch
	viewLyrics
	viewCover
)

// model holds the whole interface state.
type model struct {
	client *subsonic.Client
	player *player.Player

	// styles holds the themed lipgloss styles the view renders with.
	styles themeStyles

	width  int
	height int

	// pane state
	rightShown bool      // the navigation panel is visible
	focus      paneFocus // the pane that the keys act on
	split      float64   // fraction of the width for the queue pane

	// The right pane holds three parallel views. Each keeps its own stack
	// and cursor, so switching between them preserves position. activeView
	// selects which stack the pane shows and the keys act on.
	//
	//   viewBrowse: the drill-down stack. browseNav[0] is the root. Song
	//     info and artist info push onto this stack.
	//   viewSearch: the search form.
	//   viewLyrics: the lyrics page for the currently playing song.
	//
	// The 2, 3, and 4 keys switch the active view. They do not stack.
	activeView viewKind
	browseNav  []navLevel
	searchNav  []navLevel
	lyricsNav  []navLevel

	// coverNav holds the cover-art view for the currently playing song.
	// coverMode selects the render technique; Tab or Space cycles it while
	// the cover view is focused.
	coverNav  []navLevel
	coverMode coverMode

	// consume removes the playing song from the queue when playback
	// leaves it. repeat wraps the queue at the end. They are exclusive.
	consume bool
	repeat  bool

	// shuffle picks a random next track on advance.
	shuffle bool

	// crossfade holds the crossfade state and length in seconds.
	crossfade    bool
	crossfadeSec int

	// radio holds the feeder mode state. When radio is on, the queue draws
	// random songs from radioPool and consumes played songs, so the queue
	// stays at radioSize songs and never empties. A manual add past
	// radioSize pauses the auto-add until consume drains it below the size.
	radio     bool
	radioPool []subsonic.Song
	radioSize int

	// transcoding is true when the current track plays through a
	// server-side transcode instead of a native decode. The status bar
	// shows a "t" flag while it is set.
	transcoding bool

	// xfShared is shared with the player's next-track provider. It lets
	// the provider and the queue agree on the next index during a natural
	// crossfade.
	xfShared *crossfadeShared

	// seekSec is the seek step in seconds.
	seekSec int

	// lastPrev is when the user last pressed the previous key. It powers
	// the restart-then-previous behavior.
	lastPrev time.Time

	// sourcePlaylistID is the ID of the playlist the queue came from, or
	// empty. Ctrl+s overwrites this playlist.
	sourcePlaylistID   string
	sourcePlaylistName string

	// keyAction maps a key string to an action name.
	keyAction map[string]string

	// nspPath is the directory for smart-playlist files. An empty value
	// hides the smart-playlist feature.
	nspPath string

	// lyricsSrc looks up synchronized lyrics through the cache, the lrclib
	// dump, and the network providers, in that order. It is never nil; call
	// its Usable method to test whether any tier can produce lyrics.
	lyricsSrc *lyrics.Source

	// queueSupported is true when the server advertises the
	// indexBasedQueue OpenSubsonic extension. tuiplay then saves and
	// restores the play queue on the server.
	queueSupported bool

	// searchEditKey names the search field the text prompt edits.
	searchEditKey string

	// smartEditIdx is the index of the condition being edited in the
	// smart-playlist builder, or -1 when a new condition is being added.
	// smartEditField and smartEditOp are the field key and operator chosen
	// during the guided flow.
	smartEditIdx   int
	smartEditField string
	smartEditOp    string
	// smartSortPick is true when the field picker chooses a sort field,
	// not a condition field.
	smartSortPick bool

	// prompt state. When promptActive is true, key input goes to the
	// text prompt instead of the normal key handler.
	promptActive bool
	promptLabel  string
	promptInput  string
	promptKind   promptKind

	// confirmActive shows a yes/no question in the status bar.
	confirmActive bool
	confirmLabel  string
	confirmAction func(model) (model, tea.Cmd)

	// showHelp shows the help card over the screen.
	showHelp bool

	// showVis shows the fullscreen visualizer. visMode picks the mode.
	// visFrame forces a redraw on each animation tick. visPeaks holds the
	// falling spectrum peak caps and visPeakVel their fall velocity for
	// gravity. visLevels smooths the spectrum between frames. visWave holds
	// the decaying waveform envelope. visPhase drifts the palette hue.
	// visEnergyBase tracks a smoothed loudness baseline and visBeat flags a
	// beat frame. visParticles holds the beat-spark state.
	showVis       bool
	visMode       visMode
	visFrame      int
	visPeaks      []int
	visPeakVel    []float64
	visLevels     []float64
	visWave       []float64
	visPhase      float64
	visEnergyBase float64
	visBeat       bool
	visParticles  particleState
	// visRadialLevels holds the smoothed radial-bloom band levels, so the
	// bloom falls off gradually instead of snapping down each frame.
	visRadialLevels []float64
	// visStereoL and visStereoR hold the smoothed stereo spectrum band
	// levels per channel, so the mirrored bars glide instead of flickering.
	visStereoL []float64
	visStereoR []float64
	// visSpark holds the resolved beat-spark tunables from the config.
	visSpark VisualizerSettings

	// queue data
	queue       []subsonic.Song
	queueCursor int
	queueIndex  int // index of the current track in the queue, or -1

	// status line
	status       string
	statusExpiry time.Time
	elapsed      time.Duration
	total        time.Duration

	// scanning is true while Navidrome scans the library. spinFrame
	// advances the scan spinner while a scan runs.
	scanning  bool
	spinFrame int
}

// newModel makes the initial model from the persisted UI state.
func newModel(cl *subsonic.Client, pl *player.Player, state UIState, set Settings, keys map[string]string, lyricsSrc *lyrics.Source, queueSupported bool) model {
	split := state.SplitRatio
	if split <= 0 || split >= 1 {
		split = defaultSplit
	}
	focus := focusRight
	if !state.RightShown {
		focus = focusQueue
	}
	// Invert the action-to-key map into a key-to-action map.
	keyAction := map[string]string{}
	for action, key := range keys {
		keyAction[key] = action
	}
	seek := set.SeekSeconds
	if seek <= 0 {
		seek = 10
	}
	cross := set.CrossfadeSeconds
	if cross <= 0 {
		cross = 4
	}
	radioSize := set.RadioQueueSize
	if radioSize <= 0 {
		radioSize = 10
	}
	return model{
		client:         cl,
		player:         pl,
		styles:         set.Theme.styles(),
		rightShown:     state.RightShown,
		focus:          focus,
		split:          split,
		activeView:     viewBrowse,
		browseNav:      []navLevel{rootLevel()},
		searchNav:      []navLevel{searchLevel()},
		lyricsNav:      []navLevel{lyricsPlaceholderLevel()},
		coverNav:       []navLevel{coverPlaceholderLevel()},
		queueIndex:     -1,
		keyAction:      keyAction,
		seekSec:        seek,
		crossfadeSec:   cross,
		radioSize:      radioSize,
		nspPath:        set.NSPPath,
		lyricsSrc:      lyricsSrc,
		queueSupported: queueSupported,
		xfShared:       &crossfadeShared{pending: -1},
		status:         "Welcome to tuiplay.",
		visSpark:       set.Visualizer,
	}
}

// refreshXF updates the shared snapshot the provider reads.
func (m *model) refreshXF() {
	if m.xfShared == nil {
		return
	}
	m.xfShared.mu.Lock()
	m.xfShared.queue = m.queue
	m.xfShared.current = m.queueIndex
	m.xfShared.repeat = m.repeat
	m.xfShared.shuffle = m.shuffle
	m.xfShared.consume = m.consume
	m.xfShared.mu.Unlock()
}

// promptKind names why the text prompt is open.
type promptKind int

const (
	promptSaveNew promptKind = iota
	promptCrossfade
	promptSearchField
	promptSmartName
	promptSmartValue
	promptSmartLimit
	promptRadio
)

// uiState returns the interface state to persist.
func (m model) uiState() UIState {
	return UIState{
		RightShown: m.rightShown,
		SplitRatio: m.split,
	}
}

// top returns a pointer to the current navigation level of the active view.
func (m *model) top() *navLevel {
	s := m.activeStack()
	return &(*s)[len(*s)-1]
}

// activeStack returns a pointer to the stack of the active view.
func (m *model) activeStack() *[]navLevel {
	switch m.activeView {
	case viewSearch:
		return &m.searchNav
	case viewLyrics:
		return &m.lyricsNav
	case viewCover:
		return &m.coverNav
	default:
		return &m.browseNav
	}
}

// currentSongID returns the ID of the current queue track, or an empty
// string when no track is current.
func (m model) currentSongID() string {
	if m.queueIndex >= 0 && m.queueIndex < len(m.queue) {
		return m.queue[m.queueIndex].ID
	}
	return ""
}

// currentSong returns the current queue track and true, or a zero song and
// false when no track is current.
func (m model) currentSong() (subsonic.Song, bool) {
	if m.queueIndex >= 0 && m.queueIndex < len(m.queue) {
		return m.queue[m.queueIndex], true
	}
	return subsonic.Song{}, false
}

// topLevel returns the current navigation level of the active view by value.
func (m model) topLevel() navLevel {
	switch m.activeView {
	case viewSearch:
		return m.searchNav[len(m.searchNav)-1]
	case viewLyrics:
		return m.lyricsNav[len(m.lyricsNav)-1]
	case viewCover:
		return m.coverNav[len(m.coverNav)-1]
	default:
		return m.browseNav[len(m.browseNav)-1]
	}
}

// pushLevel adds a new level to the active view's stack.
func (m *model) pushLevel(l navLevel) {
	s := m.activeStack()
	*s = append(*s, l)
}

// popLevel removes the current level of the active view if it is not the
// bottom of that stack.
func (m *model) popLevel() {
	s := m.activeStack()
	if len(*s) > 1 {
		*s = (*s)[:len(*s)-1]
	}
}

// Messages -------------------------------------------------------------

type trackEndedMsg struct{}

type tickMsg struct{}

type statusExpiredMsg struct{}

// spinTickMsg advances the scan spinner.
type spinTickMsg struct{}

// visTickMsg animates the visualizer.
type visTickMsg struct{}

// scanStatusMsg reports whether the server scans the library now.
type scanStatusMsg struct {
	scanning bool
	err      error
}

// levelMsg carries a loaded navigation level.
type levelMsg struct {
	level navLevel
	err   error
}

// enqueueMsg carries songs to append to the bottom of the queue.
type enqueueMsg struct {
	songs []subsonic.Song
	label string
	err   error
}

// radioMsg carries the songs that seed radio (feeder) mode. label names
// the source. err reports a load failure.
type radioMsg struct {
	songs []subsonic.Song
	label string
	err   error
}

// yearsMsg carries the distinct album years.
type yearsMsg struct {
	years []int
	err   error
}

// savedMsg reports the result of a save-playlist command.
type savedMsg struct {
	name string
	err  error
}

// deletedMsg reports the result of a delete-playlist command.
type deletedMsg struct {
	name string
	err  error
	// fileRemoved is true when a backing .nsp file was deleted.
	fileRemoved bool
	// fileErr is a soft error from removing the file or from the rescan.
	fileErr error
}

// nspSavedMsg reports the result of writing a smart-playlist file.
type nspSavedMsg struct {
	path    string
	scanErr error // non-nil when the scan trigger failed (soft)
	err     error // fatal write/serialize error
}

// songInfoMsg carries a loaded song for the info view.
type songInfoMsg struct {
	song subsonic.Song
	err  error
}

// lyricsMsg carries loaded lyric lines for the lyrics view. song is the ID
// of the song the lyrics belong to. missing is true when no lyrics matched.
type lyricsMsg struct {
	song    string
	title   string
	lines   []lyricLine
	synced  bool
	missing bool
}

// coverMsg carries a decoded cover-art image for the cover view. song is
// the ID of the song the art belongs to. missing is true when the song has
// no cover art or the download or decode failed.
type coverMsg struct {
	song    string
	title   string
	img     image.Image
	missing bool
}

// ratedMsg reports the result of a set-rating command. rating is the new
// rating (0 to 5). It carries the song ID so the model updates the right
// entries.
type ratedMsg struct {
	id     string
	rating int
	err    error
}

// restoreQueueMsg carries a play queue restored from the server.
type restoreQueueMsg struct {
	queue subsonic.PlayQueue
	ok    bool
	err   error
}

// replaceMsg carries songs that replace the whole queue. play is true when
// the first song should start at once.
type replaceMsg struct {
	songs      []subsonic.Song
	label      string
	sourceID   string
	sourceName string
	play       bool
	err        error
}

// controlMsg carries an external control command from the control socket.
// reply is a channel the handler uses to send a one-line result back to
// the connected client. The channel is buffered so the handler never
// blocks the Bubble Tea loop.
type controlMsg struct {
	name  string
	arg   string
	reply chan string
}

// Init loads the clock. The root level needs no network load. When the
// server supports the indexBasedQueue extension, Init also restores the
// saved play queue.
func (m model) Init() tea.Cmd {
	if m.queueSupported {
		return tea.Batch(tick(), m.restoreQueue())
	}
	return tick()
}

// tick sends a tickMsg every second to refresh the progress bar.
func tick() tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg {
		return tickMsg{}
	})
}

// spinInterval is the animation period of the scan spinner.
const spinInterval = 180 * time.Millisecond

// spinTick advances the scan spinner while a scan runs.
func spinTick() tea.Cmd {
	return tea.Tick(spinInterval, func(time.Time) tea.Msg {
		return spinTickMsg{}
	})
}

// visInterval is the frame period of the visualizer. About 30 frames per
// second gives smooth motion. The per-frame cost is one 1024-point FFT plus
// cheap cell rendering, so the CPU cost stays low.
const visInterval = 33 * time.Millisecond

// visTick drives the visualizer animation while it is open.
func visTick() tea.Cmd {
	return tea.Tick(visInterval, func(time.Time) tea.Msg {
		return visTickMsg{}
	})
}

// checkScanStatus asks the server whether it scans the library now.
func (m model) checkScanStatus() tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		if cl == nil {
			return scanStatusMsg{}
		}
		st, err := cl.GetScanStatus()
		return scanStatusMsg{scanning: st.Scanning, err: err}
	}
}

// Commands -------------------------------------------------------------

const trackSearchLimit = 500

// loadArtists loads the artist level.
func (m model) loadArtists() tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		a, err := cl.GetArtists()
		if err != nil {
			return levelMsg{err: err}
		}
		return levelMsg{level: navLevel{kind: navArtists, title: "Artists", rows: artistRows(a), cursor: 0}}
	}
}

// loadArtistAlbums loads the albums of one artist.
func (m model) loadArtistAlbums(id, name string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		a, err := cl.GetArtist(id)
		if err != nil {
			return levelMsg{err: err}
		}
		return levelMsg{level: navLevel{kind: navAlbums, title: name, rows: albumRows(a), cursor: 0}}
	}
}

// loadAllAlbums loads every album alphabetically.
func (m model) loadAllAlbums() tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		a, err := cl.GetAllAlbums()
		if err != nil {
			return levelMsg{err: err}
		}
		return levelMsg{level: navLevel{kind: navAlbums, title: "Albums", rows: albumRows(a), cursor: 0}}
	}
}

// loadGenres loads the genre level.
func (m model) loadGenres() tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		g, err := cl.GetGenres()
		if err != nil {
			return levelMsg{err: err}
		}
		sort.Slice(g, func(i, j int) bool { return g[i].Name < g[j].Name })
		return levelMsg{level: navLevel{kind: navGenres, title: "Genres", rows: genreRows(g), cursor: 0}}
	}
}

// loadGenreAlbums loads the albums of one genre.
func (m model) loadGenreAlbums(genre string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		a, err := cl.GetAlbumsByGenre(genre)
		if err != nil {
			return levelMsg{err: err}
		}
		return levelMsg{level: navLevel{kind: navAlbums, title: genre, rows: albumRows(a), cursor: 0}}
	}
}

// loadYearAlbums loads the albums of one year.
func (m model) loadYearAlbums(year int) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		a, err := cl.GetAlbumsByYear(year, year)
		if err != nil {
			return levelMsg{err: err}
		}
		title := yearTitle(year)
		return levelMsg{level: navLevel{kind: navAlbums, title: title, rows: albumRows(a), cursor: 0}}
	}
}

// loadAlbumTracks loads the tracks of one album.
func (m model) loadAlbumTracks(id, name string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		al, err := cl.GetAlbum(id)
		if err != nil {
			return levelMsg{err: err}
		}
		return levelMsg{level: navLevel{kind: navTracks, title: name, rows: trackRows(al.Songs), cursor: 0}}
	}
}

// loadTracks loads a broad set of tracks for the Tracks entrypoint.
func (m model) loadTracks() tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		s, err := cl.SearchSongs("", trackSearchLimit)
		if err != nil {
			return levelMsg{err: err}
		}
		sort.Slice(s, func(i, j int) bool { return s[i].Title < s[j].Title })
		return levelMsg{level: navLevel{kind: navTracks, title: "Tracks", rows: trackRows(s), cursor: 0}}
	}
}

// loadPlaylists loads the playlist level.
func (m model) loadPlaylists() tea.Cmd {
	cl := m.client
	showSmart := m.nspPath != ""
	return func() tea.Msg {
		p, err := cl.GetPlaylists()
		if err != nil {
			return levelMsg{err: err}
		}
		sort.Slice(p, func(i, j int) bool { return p[i].Name < p[j].Name })
		rows := playlistRows(p)
		if showSmart {
			rows = append([]navRow{{label: "[New smart playlist]", id: newSmartRowID}}, rows...)
		}
		return levelMsg{level: navLevel{kind: navPlaylists, title: "Playlists", rows: rows, cursor: 0}}
	}
}

// enqueuePlaylist adds every song of one playlist to the queue.
func (m model) enqueuePlaylist(id, name string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		pl, err := cl.GetPlaylist(id)
		if err != nil {
			return enqueueMsg{label: name, err: err}
		}
		return enqueueMsg{songs: pl.Songs, label: name}
	}
}

// replacePlaylist loads a playlist and replaces the whole queue with it.
func (m model) replacePlaylist(id, name string, play bool) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		pl, err := cl.GetPlaylist(id)
		if err != nil {
			return replaceMsg{label: name, err: err}
		}
		return replaceMsg{songs: pl.Songs, label: name, sourceID: id, sourceName: name, play: play}
	}
}

// loadRadioPlaylist fetches a playlist's songs and returns a radioMsg to
// seed radio (feeder) mode. It runs in a command so Update does not block.
func (m model) loadRadioPlaylist(id, name string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		pl, err := cl.GetPlaylist(id)
		if err != nil {
			return radioMsg{label: name, err: err}
		}
		return radioMsg{songs: pl.Songs, label: name}
	}
}

// playPlaylistByName finds a playlist by a case-insensitive name match and
// lookups in a command so the caller does not block. An exact
// case-insensitive match wins; otherwise the first prefix match is used.
func (m model) playPlaylistByName(name string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		pls, err := cl.GetPlaylists()
		if err != nil {
			return replaceMsg{label: name, err: err}
		}
		want := strings.ToLower(strings.TrimSpace(name))
		var id, matched string
		for _, p := range pls {
			if strings.ToLower(p.Name) == want {
				id, matched = p.ID, p.Name
				break
			}
		}
		if id == "" {
			for _, p := range pls {
				if strings.HasPrefix(strings.ToLower(p.Name), want) {
					id, matched = p.ID, p.Name
					break
				}
			}
		}
		if id == "" {
			return replaceMsg{label: name, err: fmt.Errorf("no playlist matches %q", name)}
		}
		pl, err := cl.GetPlaylist(id)
		if err != nil {
			return replaceMsg{label: matched, err: err}
		}
		return replaceMsg{songs: pl.Songs, label: matched, sourceID: id, sourceName: matched, play: true}
	}
}

// deletePlaylistCmd deletes a playlist on the server. When a matching .nsp
// file exists in nspPath, it also removes the file and triggers an
// incremental scan, so an imported smart playlist does not reappear.
func (m model) deletePlaylistCmd(id, name string) tea.Cmd {
	cl := m.client
	dir := m.nspPath
	return func() tea.Msg {
		if err := cl.DeletePlaylist(id); err != nil {
			return deletedMsg{name: name, err: err}
		}
		file := nspFileFor(dir, name)
		if file == "" {
			return deletedMsg{name: name}
		}
		if err := os.Remove(file); err != nil {
			return deletedMsg{name: name, fileErr: err}
		}
		var scanErr error
		if cl != nil {
			scanErr = cl.StartScan()
		}
		return deletedMsg{name: name, fileRemoved: true, fileErr: scanErr}
	}
}

// savePlaylist creates a Navidrome playlist from the current queue.
func (m model) savePlaylist(name string, songs []subsonic.Song) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		ids := make([]string, 0, len(songs))
		for _, s := range songs {
			ids = append(ids, s.ID)
		}
		err := cl.CreatePlaylist(name, ids)
		return savedMsg{name: name, err: err}
	}
}

// overwritePlaylist replaces the songs of an existing playlist.
func (m model) overwritePlaylist(id, name string, songs []subsonic.Song) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		ids := make([]string, 0, len(songs))
		for _, s := range songs {
			ids = append(ids, s.ID)
		}
		err := cl.UpdatePlaylistSongs(id, ids)
		return savedMsg{name: name, err: err}
	}
}

// loadSongInfo fetches the full details of one song for the info view.
func (m model) loadSongInfo(s subsonic.Song) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		full, err := cl.GetSong(s.ID)
		if err != nil {
			// Fall back to the song we already have.
			return songInfoMsg{song: s}
		}
		return songInfoMsg{song: full}
	}
}

// restoreQueue loads the saved play queue from the server.
func (m model) restoreQueue() tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		pq, ok, err := cl.GetPlayQueueByIndex()
		return restoreQueueMsg{queue: pq, ok: ok, err: err}
	}
}

// loadLyrics looks up the lyrics for one song through the tiered source:
// the cache, the lrclib dump, then the network providers.
func (m model) loadLyrics(s subsonic.Song) tea.Cmd {
	src := m.lyricsSrc
	return func() tea.Msg {
		res, ok := src.Lookup(s.Artist, s.Title, s.Album, s.Duration)
		if !ok {
			return lyricsMsg{song: s.ID, title: s.Title, missing: true}
		}
		lines := make([]lyricLine, 0, len(res.Lines))
		for _, ln := range res.Lines {
			ms := int64(-1)
			if ln.HasTime() {
				ms = ln.At.Milliseconds()
			}
			lines = append(lines, lyricLine{atMs: ms, text: ln.Text})
		}
		return lyricsMsg{song: s.ID, title: s.Title, lines: lines, synced: res.Synced}
	}
}

// coverArtSize is the pixel size requested from the server for cover art.
// It is large enough for a full-pane render and small enough to download
// fast.
const coverArtSize = 600

// loadCover downloads and decodes the cover art for one song. It returns a
// coverMsg. A song with no cover-art ID, a download error, or a decode
// error yields a missing result.
func (m model) loadCover(s subsonic.Song) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		if s.CoverArt == "" {
			return coverMsg{song: s.ID, title: s.Title, missing: true}
		}
		data, _, err := cl.CoverArt(s.CoverArt, coverArtSize)
		if err != nil {
			return coverMsg{song: s.ID, title: s.Title, missing: true}
		}
		img, err := decodeCover(data)
		if err != nil {
			return coverMsg{song: s.ID, title: s.Title, missing: true}
		}
		return coverMsg{song: s.ID, title: s.Title, img: img}
	}
}

// rateSong sets the user rating of one song on the server. The rating is
// clamped to 0 through 5 by the client.
func (m model) rateSong(id string, rating int) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		err := cl.SetRating(id, rating)
		return ratedMsg{id: id, rating: rating, err: err}
	}
}

// loadYears derives the distinct album years from the whole album list.
func (m model) loadYears() tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		albums, err := cl.GetAllAlbums()
		if err != nil {
			return yearsMsg{err: err}
		}
		seen := map[int]bool{}
		var years []int
		for _, a := range albums {
			if a.Year > 0 && !seen[a.Year] {
				seen[a.Year] = true
				years = append(years, a.Year)
			}
		}
		sort.Sort(sort.Reverse(sort.IntSlice(years)))
		return yearsMsg{years: years}
	}
}

// enqueueAlbum fetches the songs of one album for the queue.
func (m model) enqueueAlbum(id, name string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		al, err := cl.GetAlbum(id)
		if err != nil {
			return enqueueMsg{label: name, err: err}
		}
		return enqueueMsg{songs: al.Songs, label: name}
	}
}

// enqueueArtist fetches every song of every album of one artist.
func (m model) enqueueArtist(id, name string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		albums, err := cl.GetArtist(id)
		if err != nil {
			return enqueueMsg{label: name, err: err}
		}
		var songs []subsonic.Song
		for _, a := range albums {
			full, err := cl.GetAlbum(a.ID)
			if err != nil {
				return enqueueMsg{label: name, err: err}
			}
			songs = append(songs, full.Songs...)
		}
		return enqueueMsg{songs: songs, label: name}
	}
}

// enqueueAlbums fetches every song of a list of albums.
func (m model) enqueueAlbums(albums []subsonic.Album, label string) tea.Cmd {
	cl := m.client
	return func() tea.Msg {
		var songs []subsonic.Song
		for _, a := range albums {
			full, err := cl.GetAlbum(a.ID)
			if err != nil {
				return enqueueMsg{label: label, err: err}
			}
			songs = append(songs, full.Songs...)
		}
		return enqueueMsg{songs: songs, label: label}
	}
}

// yearTitle returns the pane title for one year.
func yearTitle(year int) string {
	return "Year " + strconv.Itoa(year)
}

// runSearch runs the search form. It queries the server with the joined
// text, then filters the results so each filled field matches its own tag.
func (m model) runSearch(form *searchForm) tea.Cmd {
	cl := m.client
	f := form
	return func() tea.Msg {
		songs, err := cl.SearchSongs(f.query(), trackSearchLimit)
		if err != nil {
			return levelMsg{err: err}
		}
		matched := f.filterSongs(songs)
		return levelMsg{level: navLevel{kind: navTracks, title: "Search results", rows: trackRows(matched), cursor: 0}}
	}
}

// writeNSP serializes the builder to a .nsp file in nspPath, then triggers
// an incremental library scan so Navidrome imports it. A scan failure is a
// soft error; the file is still written.
func (m model) writeNSP(b *smartBuilder) tea.Cmd {
	cl := m.client
	dir := m.nspPath
	return func() tea.Msg {
		data, err := b.toNSP()
		if err != nil {
			return nspSavedMsg{err: err}
		}
		name := slugify(b.name)
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, data, 0644); err != nil {
			return nspSavedMsg{err: err}
		}
		// When editing an existing file whose name changed, remove the old
		// file so the renamed playlist does not appear twice after a scan.
		if b.origFile != "" && b.origFile != path {
			_ = os.Remove(b.origFile)
		}
		var scanErr error
		if cl != nil {
			scanErr = cl.StartScan()
		}
		return nspSavedMsg{path: path, scanErr: scanErr}
	}
}
