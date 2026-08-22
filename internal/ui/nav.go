package ui

import (
	"fmt"

	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"
)

// navKind names the kind of a navigation level. The kind decides how a row
// renders and what Enter on a row does.
type navKind int

const (
	// navRoot is the entrypoint list: Artists, Albums, Genres, Release
	// Years, Tracks.
	navRoot navKind = iota
	navArtists
	navAlbums
	navGenres
	navYears
	navTracks
	navPlaylists
	navSongInfo
	navArtistInfo
	navSearch
	navSmartBuilder
	navSmartField
	navSmartOp
	navLyrics
)

// entrypoint IDs on the root level.
const (
	epArtists   = "artists"
	epAlbums    = "albums"
	epGenres    = "genres"
	epYears     = "years"
	epTracks    = "tracks"
	epPlaylists = "playlists"
)

// navRow is one row in a navigation level.
type navRow struct {
	// label is the text to show.
	label string
	// id is the item identifier used to drill down (artist ID, album ID,
	// genre name, year string, or an entrypoint constant).
	id string
	// name is a plain name for the item, without any count suffix.
	name string
	// song is set only when the row is a track. It holds the full song.
	song subsonic.Song
	// year is set only for a year row.
	year int
}

// navLevel is one level in the navigation stack.
type navLevel struct {
	kind    navKind
	title   string   // shown in the pane header path
	rows    []navRow // the real rows, without the back row
	cursor  int      // index into rows; -1 means the back row is selected
	loading bool     // true while a load command runs

	// info holds label/value pairs for an info level.
	info []infoLine

	// search holds the field values for a navSearch level.
	search *searchForm

	// builder holds the rule state for a navSmartBuilder level.
	builder *smartBuilder

	// lyrics holds the lyric lines for a navLyrics level. lyricsSynced
	// is true when the lines carry timestamps. lyricsSong is the ID of the
	// song the lyrics belong to, so the view highlights the active line
	// only while that song plays.
	lyrics        []lyricLine
	lyricsSynced  bool
	lyricsSong    string
	lyricsMissing bool
}

// lyricLine is one lyric line in a navLyrics level. atMs is the offset in
// milliseconds, or negative for an untimed (plain) line.
type lyricLine struct {
	atMs int64
	text string
}

// infoLine is one label/value row in an info view.
type infoLine struct {
	label string
	value string
}

// rootLevel returns the entrypoint level. Each label is bracketed, like a
// folder.
func rootLevel() navLevel {
	return navLevel{
		kind:  navRoot,
		title: "Navigation",
		rows: []navRow{
			{label: "[Artists]", id: epArtists},
			{label: "[Albums]", id: epAlbums},
			{label: "[Genres]", id: epGenres},
			{label: "[Release Years]", id: epYears},
			{label: "[Tracks]", id: epTracks},
			{label: "[Playlists]", id: epPlaylists},
		},
	}
}

// hasBackRow reports whether the level shows a [..] row. The browse root and
// the parallel search and lyrics views have no back row: the browse root is
// the bottom of its stack, and the 2, 3, and 4 keys switch views instead.
func (l navLevel) hasBackRow() bool {
	switch l.kind {
	case navRoot, navSearch, navLyrics:
		return false
	}
	return true
}

// isOverlay reports whether the level is a transient page pushed onto the
// browse stack. Only the info views push onto browse now; the show_nav key
// pops them to return to browsing.
func (l navLevel) isOverlay() bool {
	switch l.kind {
	case navSongInfo, navArtistInfo:
		return true
	}
	return false
}

// displayCount returns the number of visible rows, including the back row.
func (l navLevel) displayCount() int {
	if l.hasBackRow() {
		return len(l.rows) + 1
	}
	return len(l.rows)
}

// onBackRow reports whether the cursor is on the [..] row.
func (l navLevel) onBackRow() bool {
	return l.hasBackRow() && l.cursor < 0
}

// selected returns the selected real row and true, or a zero row and false
// when the cursor is on the back row or the level is empty.
func (l navLevel) selected() (navRow, bool) {
	if l.onBackRow() {
		return navRow{}, false
	}
	if l.cursor < 0 || l.cursor >= len(l.rows) {
		return navRow{}, false
	}
	return l.rows[l.cursor], true
}

// artistRows builds rows from a list of artists.
func artistRows(artists []subsonic.Artist) []navRow {
	rows := make([]navRow, 0, len(artists))
	for _, a := range artists {
		rows = append(rows, navRow{label: a.Name, id: a.ID})
	}
	return rows
}

// albumRows builds rows from a list of albums. The label shows the album
// name and the year.
func albumRows(albums []subsonic.Album) []navRow {
	rows := make([]navRow, 0, len(albums))
	for _, a := range albums {
		label := a.Name
		if a.Year > 0 {
			label = fmt.Sprintf("%s (%d)", a.Name, a.Year)
		}
		rows = append(rows, navRow{label: label, id: a.ID})
	}
	return rows
}

// genreRows builds rows from a list of genres.
func genreRows(genres []subsonic.Genre) []navRow {
	rows := make([]navRow, 0, len(genres))
	for _, g := range genres {
		rows = append(rows, navRow{label: g.Name, id: g.Name})
	}
	return rows
}

// yearRows builds rows from a sorted list of years, newest first.
func yearRows(years []int) []navRow {
	rows := make([]navRow, 0, len(years))
	for _, y := range years {
		rows = append(rows, navRow{label: fmt.Sprintf("%d", y), id: fmt.Sprintf("%d", y), year: y})
	}
	return rows
}

// trackRows builds rows from a list of songs.
func trackRows(songs []subsonic.Song) []navRow {
	rows := make([]navRow, 0, len(songs))
	for _, s := range songs {
		label := s.Title
		if s.Artist != "" {
			label = s.Artist + " - " + s.Title
		}
		rows = append(rows, navRow{label: label, id: s.ID, song: s})
	}
	return rows
}

// playlistRows builds rows from a list of playlists.
func playlistRows(pls []subsonic.Playlist) []navRow {
	rows := make([]navRow, 0, len(pls))
	for _, p := range pls {
		label := fmt.Sprintf("%s (%d)", p.Name, p.SongCount)
		rows = append(rows, navRow{label: label, id: p.ID, name: p.Name})
	}
	return rows
}

// songInfoLevel builds an info level from a song.
func songInfoLevel(s subsonic.Song) navLevel {
	val := func(v string) string {
		if v == "" {
			return "<empty>"
		}
		return v
	}
	num := func(n int) string {
		if n == 0 {
			return "<empty>"
		}
		return fmt.Sprintf("%d", n)
	}
	length := fmt.Sprintf("%d:%02d", s.Duration/60, s.Duration%60)
	track := num(s.Track)
	if s.DiscNumber > 0 {
		track = fmt.Sprintf("%02d (disc %d)", s.Track, s.DiscNumber)
	}
	info := []infoLine{
		{"Path", val(s.Path)},
		{"Length", length},
		{"Title", val(s.Title)},
		{"Artist", val(s.Artist)},
		{"Album", val(s.Album)},
		{"Year", num(s.Year)},
		{"Track", track},
		{"Genre", val(s.Genre)},
		{"Format", val(s.Suffix)},
		{"Bit rate", num(s.BitRate)},
		{"Rating", ratingLabel(s.UserRating)},
	}
	return navLevel{kind: navSongInfo, title: "Song info: " + s.Title, info: info, cursor: -1}
}

// ratingLabel describes a user rating for the song-info view.
func ratingLabel(rating int) string {
	switch {
	case rating <= 0:
		return "<unrated>"
	case rating == 1:
		return "1 (disliked)"
	default:
		return fmt.Sprintf("%d", rating)
	}
}

// artistInfoLevel builds a stub info level for an artist.
func artistInfoLevel(name string) navLevel {
	info := []infoLine{
		{"Artist", name},
		{"Info", "Artist info is not implemented yet."},
	}
	return navLevel{kind: navArtistInfo, title: "Artist info: " + name, info: info, cursor: -1}
}

// lyricsPlaceholderLevel builds the empty lyrics view. It shows before any
// song has loaded lyrics, and whenever nothing is playing.
func lyricsPlaceholderLevel() navLevel {
	return navLevel{kind: navLyrics, title: "Lyrics", cursor: -1, lyricsMissing: true}
}

// lyricsLoadingLevel returns a placeholder level that marks an in-flight
// lyrics lookup for a song. It records the song ID so a follow-up refresh
// does not start a second lookup for the same song.
func lyricsLoadingLevel(songID string) navLevel {
	return navLevel{kind: navLyrics, title: "Lyrics", loading: true, cursor: -1, lyricsSong: songID}
}

// lyricsQualityMark returns a glyph prefix that shows the quality of the
// loaded lyrics. A semiquaver (♬) marks synced, per-line timed lyrics. A
// quaver (♪) marks plain, untimed lyrics. A missing result gets no glyph.
// The returned string ends with a separator so it reads before the title.
func lyricsQualityMark(synced, missing bool) string {
	if missing {
		return ": "
	}
	if synced {
		return "♬ "
	}
	return "♪ "
}
