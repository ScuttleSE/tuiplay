package ui

import (
	"sort"
	"strconv"
	"strings"

	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"
)

// searchFieldKey names one searchable field. It also names the song tag the
// field filters on.
type searchFieldKey string

const (
	sfAny      searchFieldKey = "any"
	sfArtist   searchFieldKey = "artist"
	sfTitle    searchFieldKey = "title"
	sfAlbum    searchFieldKey = "album"
	sfGenre    searchFieldKey = "genre"
	sfDate     searchFieldKey = "date"
	sfComment  searchFieldKey = "comment"
	sfFilename searchFieldKey = "filename"
)

// searchField is one field row in the search form.
type searchField struct {
	key   searchFieldKey
	label string
	value string
}

// searchForm holds the values the user typed into the search form.
type searchForm struct {
	fields []searchField
}

// newSearchForm returns an empty search form with the supported fields.
func newSearchForm() *searchForm {
	return &searchForm{fields: []searchField{
		{key: sfAny, label: "Any"},
		{key: sfArtist, label: "Artist"},
		{key: sfTitle, label: "Title"},
		{key: sfAlbum, label: "Album"},
		{key: sfGenre, label: "Genre"},
		{key: sfDate, label: "Date"},
		{key: sfComment, label: "Comment"},
		{key: sfFilename, label: "Filename"},
	}}
}

// value returns the value of one field, or an empty string.
func (f *searchForm) value(key searchFieldKey) string {
	for _, fld := range f.fields {
		if fld.key == key {
			return fld.value
		}
	}
	return ""
}

// set writes a value into one field.
func (f *searchForm) set(key searchFieldKey, value string) {
	for i := range f.fields {
		if f.fields[i].key == key {
			f.fields[i].value = value
			return
		}
	}
}

// reset clears every field value.
func (f *searchForm) reset() {
	for i := range f.fields {
		f.fields[i].value = ""
	}
}

// empty reports whether every field is blank.
func (f *searchForm) empty() bool {
	for _, fld := range f.fields {
		if strings.TrimSpace(fld.value) != "" {
			return false
		}
	}
	return true
}

// query builds one free-text query from the filled fields. The Subsonic
// search3 API takes a single query, so the values join with spaces.
func (f *searchForm) query() string {
	var parts []string
	for _, fld := range f.fields {
		v := strings.TrimSpace(fld.value)
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, " ")
}

// matches reports whether a song satisfies every filled field. Each filled
// field must be a case-insensitive substring of its own tag. The Any field
// matches when any tag contains the value.
func (f *searchForm) matches(s subsonic.Song) bool {
	for _, fld := range f.fields {
		v := strings.ToLower(strings.TrimSpace(fld.value))
		if v == "" {
			continue
		}
		if !fieldMatches(fld.key, v, s) {
			return false
		}
	}
	return true
}

// fieldMatches reports whether one field value matches a song.
func fieldMatches(key searchFieldKey, v string, s subsonic.Song) bool {
	contains := func(hay string) bool {
		return strings.Contains(strings.ToLower(hay), v)
	}
	year := ""
	if s.Year > 0 {
		year = strconv.Itoa(s.Year)
	}
	switch key {
	case sfAny:
		return contains(s.Title) || contains(s.Artist) || contains(s.Album) ||
			contains(s.Genre) || contains(s.Path) || contains(s.Comment) ||
			strings.Contains(year, v)
	case sfArtist:
		return contains(s.Artist)
	case sfTitle:
		return contains(s.Title)
	case sfAlbum:
		return contains(s.Album)
	case sfGenre:
		return contains(s.Genre)
	case sfDate:
		return strings.Contains(year, v)
	case sfComment:
		return contains(s.Comment)
	case sfFilename:
		return contains(s.Path)
	}
	return false
}

// filterSongs keeps the songs that match the form and sorts them by title.
func (f *searchForm) filterSongs(songs []subsonic.Song) []subsonic.Song {
	out := make([]subsonic.Song, 0, len(songs))
	for _, s := range songs {
		if f.matches(s) {
			out = append(out, s)
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	return out
}

// searchRow IDs mark the action rows of the search form.
const (
	searchRowSearch = "act:search"
	searchRowReset  = "act:reset"
)

// searchLevel builds a fresh search form level. Its rows are the field rows
// followed by the Search and Reset action rows.
func searchLevel() navLevel {
	form := newSearchForm()
	var rows []navRow
	for _, fld := range form.fields {
		rows = append(rows, navRow{label: fld.label, id: string(fld.key)})
	}
	rows = append(rows,
		navRow{label: "Search", id: searchRowSearch},
		navRow{label: "Reset", id: searchRowReset},
	)
	return navLevel{kind: navSearch, title: "Search", rows: rows, cursor: 0, search: form}
}
