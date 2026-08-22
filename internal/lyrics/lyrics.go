// Package lyrics reads synchronized lyrics from a local lrclib SQLite
// database dump.
//
// lrclib publishes a full database dump as an SQLite file. Download it from
// https://lrclib.net/db-dumps. The dump has a tracks table and a lyrics
// table. tuiplay opens the file read only and looks up the current song by
// its artist, title, album, and duration. The dump has no Subsonic IDs, so
// the match runs on the text tags.
//
// The package uses the pure-Go modernc.org/sqlite driver. It adds no C
// dependency to the build.
package lyrics

import (
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

// durationSlackSec is how far the song duration may differ from the lrclib
// track duration and still count as a match.
const durationSlackSec = 3

// Line is one lyric line. At is the offset from the start of the track. For
// plain (unsynced) lyrics At is negative to mark it as untimed.
type Line struct {
	At   time.Duration
	Text string
}

// HasTime reports whether the line carries a timestamp.
func (l Line) HasTime() bool {
	return l.At >= 0
}

// Result holds the lyrics for one song.
type Result struct {
	// Lines holds the lyric lines in order.
	Lines []Line
	// Synced is true when the lines carry timestamps.
	Synced bool
}

// DB is a read-only handle to an lrclib SQLite dump.
type DB struct {
	mu sync.Mutex
	db *sql.DB
}

// Open opens the lrclib SQLite dump at path for reading.
func Open(path string) (*DB, error) {
	// The immutable and mode=ro flags open the file read only and skip
	// the write-ahead log, which suits a static dump.
	dsn := fmt.Sprintf("file:%s?mode=ro&immutable=1", path)
	sdb, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	if err := sdb.Ping(); err != nil {
		sdb.Close()
		return nil, err
	}
	return &DB{db: sdb}, nil
}

// Close closes the database handle.
func (d *DB) Close() error {
	if d == nil || d.db == nil {
		return nil
	}
	return d.db.Close()
}

// Lookup finds the lyrics for one song. It first matches the artist, title,
// album, and duration. If that fails it relaxes to the artist and title
// only. It prefers synced lyrics over plain lyrics. The bool is false when
// no lyrics are found.
func (d *DB) Lookup(artist, title, album string, durationSec int) (Result, bool) {
	if d == nil || d.db == nil {
		return Result{}, false
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	a := strings.ToLower(strings.TrimSpace(artist))
	t := strings.ToLower(strings.TrimSpace(title))
	al := strings.ToLower(strings.TrimSpace(album))

	// Strict match: artist, title, album, and a close duration.
	synced, plain, ok := d.query(
		`SELECT l.synced_lyrics, l.plain_lyrics
		 FROM tracks tr JOIN lyrics l ON l.id = tr.last_lyrics_id
		 WHERE tr.artist_name_lower = ? AND tr.name_lower = ?
		   AND tr.album_name_lower = ?
		   AND abs(tr.duration - ?) <= ?
		 ORDER BY (l.synced_lyrics IS NOT NULL) DESC
		 LIMIT 1`,
		a, t, al, durationSec, durationSlackSec,
	)
	if !ok {
		// Relaxed match: artist and title only, prefer synced.
		synced, plain, ok = d.query(
			`SELECT l.synced_lyrics, l.plain_lyrics
			 FROM tracks tr JOIN lyrics l ON l.id = tr.last_lyrics_id
			 WHERE tr.artist_name_lower = ? AND tr.name_lower = ?
			 ORDER BY (l.synced_lyrics IS NOT NULL) DESC
			 LIMIT 1`,
			a, t,
		)
	}
	if !ok {
		return Result{}, false
	}

	if synced != "" {
		lines := parseLRC(synced)
		if len(lines) > 0 {
			return Result{Lines: lines, Synced: true}, true
		}
	}
	if plain != "" {
		return Result{Lines: parsePlain(plain), Synced: false}, true
	}
	return Result{}, false
}

// query runs a lookup that returns at most one row of synced and plain
// lyrics. The bool is false when no row matches.
func (d *DB) query(q string, args ...interface{}) (synced, plain string, ok bool) {
	var s, p sql.NullString
	row := d.db.QueryRow(q, args...)
	if err := row.Scan(&s, &p); err != nil {
		return "", "", false
	}
	return s.String, p.String, true
}

// lrcTag matches one LRC timestamp tag like [01:23.45] or [1:02].
var lrcTag = regexp.MustCompile(`\[(\d+):(\d{1,2})(?:[.:](\d{1,3}))?\]`)

// parseLRC parses LRC text into timed lines. A single text line may carry
// several timestamps; each yields one line. Lines with no timestamp are
// dropped, except that the result stays sorted by time.
func parseLRC(text string) []Line {
	var out []Line
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimRight(raw, "\r")
		tags := lrcTag.FindAllStringSubmatch(raw, -1)
		if len(tags) == 0 {
			continue
		}
		body := strings.TrimSpace(lrcTag.ReplaceAllString(raw, ""))
		for _, m := range tags {
			min, _ := strconv.Atoi(m[1])
			sec, _ := strconv.Atoi(m[2])
			frac := 0
			if m[3] != "" {
				// Normalize the fraction to milliseconds.
				f := m[3]
				for len(f) < 3 {
					f += "0"
				}
				frac, _ = strconv.Atoi(f[:3])
			}
			at := time.Duration(min)*time.Minute +
				time.Duration(sec)*time.Second +
				time.Duration(frac)*time.Millisecond
			out = append(out, Line{At: at, Text: body})
		}
	}
	sortByTime(out)
	return out
}

// parsePlain parses plain lyrics into untimed lines.
func parsePlain(text string) []Line {
	var out []Line
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimRight(raw, "\r")
		out = append(out, Line{At: -1, Text: raw})
	}
	return out
}

// sortByTime sorts lines by their timestamp with a simple insertion sort.
// The input is nearly ordered, so this is cheap.
func sortByTime(lines []Line) {
	for i := 1; i < len(lines); i++ {
		for j := i; j > 0 && lines[j].At < lines[j-1].At; j-- {
			lines[j], lines[j-1] = lines[j-1], lines[j]
		}
	}
}
