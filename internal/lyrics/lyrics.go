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
	// One connection only. The LIKE pragma below is per connection, and
	// the dump lookups serialize on a mutex anyway.
	sdb.SetMaxOpenConns(1)
	// The *_lower columns hold lower-case text with BINARY collation. This
	// pragma lets the search's prefix LIKE use the indexes on those
	// columns. Without it a prefix query scans all 30+ million rows.
	if _, err := sdb.Exec("PRAGMA case_sensitive_like = ON"); err != nil {
		sdb.Close()
		return nil, fmt.Errorf("like pragma: %w", err)
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

// Hit is one freetext search result. It carries the lyrics themselves, so
// the caller needs no second lookup when it picks a hit.
type Hit struct {
	Artist      string
	Title       string
	Album       string
	DurationSec int
	Synced      bool
	// Lyrics holds the lyric lines of the hit.
	Lyrics Result
}

// searchTokenMax caps how many words a search query may carry.
const searchTokenMax = 8

// Search finds tracks whose title or artist starts with the query. The
// first query token must match the start of the title or the artist; the
// other tokens filter loosely inside that match. A prefix search keeps the
// query on the index, so it stays fast on the multi-million-row dump. The
// results prefer synced lyrics and hold at most limit distinct artist and
// title pairs.
func (d *DB) Search(query string, limit int) ([]Hit, error) {
	if d == nil || d.db == nil {
		return nil, nil
	}
	tokens := searchTokens(query)
	if len(tokens) == 0 || limit < 1 {
		return nil, nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()

	var out []Hit
	seen := map[string]bool{}
	add := func(hits []Hit) bool {
		for _, h := range hits {
			key := strings.ToLower(h.Artist) + "\x00" + strings.ToLower(h.Title)
			if seen[key] {
				continue
			}
			seen[key] = true
			out = append(out, h)
			if len(out) >= limit {
				return true
			}
		}
		return false
	}
	// Four phases, most specific first. The multi-token phases match the
	// title, then the artist. Only when both found nothing do the bare
	// prefix phases relax the query, so a filtered query never pulls in
	// unrelated tracks.
	for _, phase := range []struct {
		col    string
		tokens []string
	}{
		{"tr.name_lower", tokens},
		{"tr.artist_name_lower", tokens},
	} {
		hits, err := d.searchLike(phase.col, phase.tokens, limit)
		if err != nil {
			return out, err
		}
		if add(hits) {
			return out, nil
		}
	}
	if len(out) == 0 {
		for _, phase := range []struct {
			col    string
			tokens []string
		}{
			{"tr.name_lower", tokens[:1]},
			{"tr.artist_name_lower", tokens[:1]},
		} {
			hits, err := d.searchLike(phase.col, phase.tokens, limit)
			if err != nil {
				return out, err
			}
			if add(hits) {
				break
			}
		}
	}
	return out, nil
}

// searchLike runs one phase of the search. The first token matches the
// start of col (an indexed lower-case column). The other tokens must occur
// somewhere in the title or the artist. It first fetches tracks with
// synced lyrics, then plain-only tracks, so synced results come first.
// Each query stops after limit rows: no ORDER BY, because a sort would
// materialize every match of a broad prefix before the LIMIT and read the
// large lyric text for each.
func (d *DB) searchLike(col string, tokens []string, limit int) ([]Hit, error) {
	where := col + ` LIKE ? ESCAPE '\'`
	args := []interface{}{likePrefix(tokens[0])}
	for _, t := range tokens[1:] {
		where += ` AND (tr.name_lower LIKE ? ESCAPE '\' OR tr.artist_name_lower LIKE ? ESCAPE '\')`
		args = append(args, likeContains(t), likeContains(t))
	}
	base := `SELECT tr.artist_name, tr.name, tr.album_name, tr.duration,
		     l.synced_lyrics, l.plain_lyrics
		 FROM tracks tr CROSS JOIN lyrics l ON l.id = tr.last_lyrics_id
		 WHERE ` + where + ` AND `
	syncedQ := base + `l.has_synced_lyrics = 1 LIMIT ?`
	plainQ := base + `(l.has_synced_lyrics IS NULL OR l.has_synced_lyrics = 0) LIMIT ?`

	var out []Hit
	for _, q := range []string{syncedQ, plainQ} {
		qargs := append(args, limit)
		rows, err := d.db.Query(q, qargs...)
		if err != nil {
			return out, err
		}
		hits, err := scanHits(rows)
		if err != nil {
			return out, err
		}
		out = append(out, hits...)
		if len(out) >= limit {
			break
		}
	}
	return out, nil
}

// scanHits drains a search result set into hits, skipping rows without
// usable lyrics.
func scanHits(rows *sql.Rows) ([]Hit, error) {
	defer rows.Close()
	var out []Hit
	for rows.Next() {
		var artist, name sql.NullString
		var album sql.NullString
		var dur sql.NullFloat64
		var synced, plain sql.NullString
		if err := rows.Scan(&artist, &name, &album, &dur, &synced, &plain); err != nil {
			return out, err
		}
		res, ok := resultFromLyrics(synced.String, plain.String)
		if !ok {
			continue // no usable lyrics (empty or instrumental)
		}
		out = append(out, Hit{
			Artist:      artist.String,
			Title:       name.String,
			Album:       album.String,
			DurationSec: int(dur.Float64 + 0.5),
			Synced:      res.Synced,
			Lyrics:      res,
		})
	}
	return out, rows.Err()
}

// searchTokens splits a search query into lower-case words. It caps the
// token count so a long paste cannot build a huge query.
func searchTokens(query string) []string {
	fields := strings.Fields(strings.ToLower(strings.TrimSpace(query)))
	if len(fields) > searchTokenMax {
		fields = fields[:searchTokenMax]
	}
	return fields
}

// likeEscape escapes the SQL LIKE wildcards in s for use with an ESCAPE
// backslash clause.
func likeEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `%`, `\%`)
	s = strings.ReplaceAll(s, `_`, `\_`)
	return s
}

// likePrefix builds a LIKE pattern that matches strings starting with t.
func likePrefix(t string) string { return likeEscape(t) + "%" }

// likeContains builds a LIKE pattern that matches strings containing t.
func likeContains(t string) string { return "%" + likeEscape(t) + "%" }

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
