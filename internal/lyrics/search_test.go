package lyrics

import (
	"database/sql"
	"path/filepath"
	"testing"
)

// searchFixture builds a small dump-shaped database and opens it through
// the real Open path, so the test covers the schema and the pragma.
func searchFixture(t *testing.T) *DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dump.sqlite3")
	sdb, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open fixture: %v", err)
	}
	defer sdb.Close()
	schema := `
		CREATE TABLE tracks (
			id INTEGER PRIMARY KEY,
			name TEXT,
			name_lower TEXT,
			artist_name TEXT,
			artist_name_lower TEXT,
			album_name TEXT,
			album_name_lower TEXT,
			duration REAL,
			last_lyrics_id INTEGER
		);
		CREATE TABLE lyrics (
			id INTEGER PRIMARY KEY,
			plain_lyrics TEXT,
			synced_lyrics TEXT,
			has_plain_lyrics INTEGER,
			has_synced_lyrics INTEGER
		);
		CREATE INDEX idx_tracks_name_lower ON tracks (name_lower);
		CREATE INDEX idx_tracks_artist_name_lower ON tracks (artist_name_lower);
		INSERT INTO lyrics (id, plain_lyrics, synced_lyrics, has_plain_lyrics, has_synced_lyrics) VALUES
			(1, NULL, '[00:01.00]Is this the real life?
[00:05.00]Is this just fantasy?', 0, 1),
			(2, 'nothing really matters to me', NULL, 1, 0),
			(3, NULL, NULL, 0, 0);
		INSERT INTO tracks (id, name, name_lower, artist_name, artist_name_lower, album_name, album_name_lower, duration, last_lyrics_id) VALUES
			(1, 'Bohemian Rhapsody', 'bohemian rhapsody', 'Queen', 'queen', 'A Night at the Opera', 'a night at the opera', 355.0, 1),
			(2, 'Bohemian Rhapsody', 'bohemian rhapsody', 'The Braids', 'the braids', 'High School High', 'high school high', 237.0, 1),
			(3, 'Bohemian Like You', 'bohemian like you', 'The Dandy Warhols', 'the dandy warhols', 'Thirteen Tales', 'thirteen tales', 210.0, 2),
			(4, 'Instrumental Piece', 'instrumental piece', 'Someone', 'someone', 'Album', 'album', 100.0, 3),
			(5, 'Under Pressure', 'under pressure', 'Queen', 'queen', 'Hot Space', 'hot space', 248.0, 2),
			(6, 'Bohemian Rhapsody', 'bohemian rhapsody', 'Queen', 'queen', 'Greatest Hits', 'greatest hits', 354.0, 1);
	`
	if _, err := sdb.Exec(schema); err != nil {
		t.Fatalf("fixture schema: %v", err)
	}
	db, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func TestSearchTitlePrefix(t *testing.T) {
	db := searchFixture(t)
	hits, err := db.Search("bohe", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	got := map[string]bool{}
	for _, h := range hits {
		got[h.Artist+" - "+h.Title] = true
	}
	want := []string{
		"Queen - Bohemian Rhapsody",
		"The Braids - Bohemian Rhapsody",
		"The Dandy Warhols - Bohemian Like You",
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing hit %q in %v", w, got)
		}
	}
	if len(hits) != len(want) {
		t.Errorf("got %d hits, want %d: %v", len(hits), len(want), hits)
	}
	// The Queen row dedupes across its two albums and carries lyrics.
	if hits[0].Artist != "Queen" || hits[0].Album != "A Night at the Opera" {
		t.Errorf("first hit = %+v", hits[0])
	}
	if !hits[0].Synced || len(hits[0].Lyrics.Lines) == 0 {
		t.Errorf("first hit has no synced lyrics: %+v", hits[0])
	}
}

func TestSearchMultiTokenFilters(t *testing.T) {
	db := searchFixture(t)
	hits, err := db.Search("bohemian rhapsody", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	for _, h := range hits {
		if h.Title == "Bohemian Like You" {
			t.Errorf("multi-token query should drop %q", h.Title)
		}
	}
	if len(hits) != 2 {
		t.Errorf("got %d hits, want 2: %v", len(hits), hits)
	}
}

func TestSearchArtistPrefix(t *testing.T) {
	db := searchFixture(t)
	hits, err := db.Search("queen", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	titles := map[string]bool{}
	for _, h := range hits {
		if h.Artist != "Queen" {
			t.Errorf("unexpected artist %q", h.Artist)
		}
		titles[h.Title] = true
	}
	if !titles["Under Pressure"] || !titles["Bohemian Rhapsody"] {
		t.Errorf("missing Queen titles in %v", titles)
	}
}

func TestSearchPrefersSynced(t *testing.T) {
	db := searchFixture(t)
	// The dump search is prefix-only, so the query must match the start
	// of the artist. A mid-string query is the network tier's job.
	hits, err := db.Search("the dandy", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 1 {
		t.Fatalf("got %d hits, want 1", len(hits))
	}
	if hits[0].Synced {
		t.Errorf("plain-only track reported as synced")
	}
}

func TestSearchSkipsEmptyLyrics(t *testing.T) {
	db := searchFixture(t)
	hits, err := db.Search("instrumental", 10)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("got hits for a track without lyrics: %v", hits)
	}
}

func TestSearchEmptyAndWildcard(t *testing.T) {
	db := searchFixture(t)
	if hits, err := db.Search("   ", 10); err != nil || len(hits) != 0 {
		t.Errorf("empty query: hits=%v err=%v", hits, err)
	}
	// A percent in the query must stay a literal, not match everything.
	hits, err := db.Search("100%", 10)
	if err != nil {
		t.Fatalf("wildcard query: %v", err)
	}
	if len(hits) != 0 {
		t.Errorf("percent query matched %d hits", len(hits))
	}
}

func TestSearchLimit(t *testing.T) {
	db := searchFixture(t)
	hits, err := db.Search("bohemian", 2)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(hits) > 2 {
		t.Errorf("got %d hits, want at most 2", len(hits))
	}
}
