package lyrics

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Cache stores lyric results as one file per song under a directory. A
// found result is a "<key>.lrc" file that holds the LRC or plain text. A
// known miss is an empty "<key>.miss" file. The cache rechecks a miss when
// the file is older than the recheck window.
type Cache struct {
	// dir is the cache directory. An empty dir disables the cache.
	dir string
	// recheck is how long a miss stays valid. A zero value means the miss
	// never goes stale.
	recheck time.Duration
}

// NewCache returns a cache rooted at dir. recheckDays is how many days a
// negative (miss) entry stays valid before the cache rechecks the network
// and the dump. A value of zero means the miss never goes stale. An empty
// dir disables the cache; its methods then do nothing.
func NewCache(dir string, recheckDays int) *Cache {
	var d time.Duration
	if recheckDays > 0 {
		d = time.Duration(recheckDays) * 24 * time.Hour
	}
	return &Cache{dir: dir, recheck: d}
}

// Writable reports whether the cache can store entries.
func (c *Cache) Writable() bool {
	return c != nil && c.dir != ""
}

// key returns the cache key for one song. It is the hex SHA-256 of the
// normalized artist, title, album, and duration. The normalization lowers
// the case and trims the space, so it matches the dump lookup.
func (c *Cache) key(artist, title, album string, durationSec int) string {
	norm := func(s string) string {
		return strings.ToLower(strings.TrimSpace(s))
	}
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%s|%d",
		norm(artist), norm(title), norm(album), durationSec)))
	return hex.EncodeToString(h[:])
}

// Get looks up a cached result for one song. hit is true when a stored
// result is returned. blocked is true when a fresh miss entry exists, so the
// caller must not refetch. A stale miss (older than the recheck window)
// reports neither hit nor blocked, so the caller refetches. When the cache
// is off, Get reports no hit and no block.
func (c *Cache) Get(artist, title, album string, durationSec int) (res Result, hit bool, blocked bool) {
	if c == nil || c.dir == "" {
		return Result{}, false, false
	}
	k := c.key(artist, title, album, durationSec)

	// A stored result wins.
	if data, err := os.ReadFile(filepath.Join(c.dir, k+".lrc")); err == nil {
		return parseText(string(data)), true, false
	}

	// A fresh miss entry blocks a refetch. A stale one does not.
	if info, err := os.Stat(filepath.Join(c.dir, k+".miss")); err == nil {
		if c.recheck == 0 || time.Since(info.ModTime()) <= c.recheck {
			return Result{}, false, true
		}
		return Result{}, false, false
	}

	return Result{}, false, false
}

// Put stores a found result for one song. A stored file removes any older
// miss entry for the same key. Put is a no-op when the cache is off.
func (c *Cache) Put(artist, title, album string, durationSec int, res Result) {
	if c == nil || c.dir == "" {
		return
	}
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return
	}
	k := c.key(artist, title, album, durationSec)
	body := formatText(res)
	if err := os.WriteFile(filepath.Join(c.dir, k+".lrc"), []byte(body), 0600); err == nil {
		os.Remove(filepath.Join(c.dir, k+".miss"))
	}
}

// PutMiss stores a negative (miss) entry for one song. The entry's mtime
// drives the recheck window. PutMiss is a no-op when the cache is off.
func (c *Cache) PutMiss(artist, title, album string, durationSec int) {
	if c == nil || c.dir == "" {
		return
	}
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return
	}
	k := c.key(artist, title, album, durationSec)
	path := filepath.Join(c.dir, k+".miss")
	// Write the file so the mtime updates even when it already exists.
	_ = os.WriteFile(path, nil, 0600)
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// GetOffset returns the stored timing offset for one song. It is the
// duration added to each lyric timestamp before the view picks the active
// line: a negative value makes the lines show earlier, a positive one
// later. A song with no stored offset returns zero. The offset is a no-op
// when the cache is off.
func (c *Cache) GetOffset(artist, title, album string, durationSec int) time.Duration {
	if c == nil || c.dir == "" {
		return 0
	}
	data, err := os.ReadFile(filepath.Join(c.dir, c.key(artist, title, album, durationSec)+".offset"))
	if err != nil {
		return 0
	}
	sec, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64)
	if err != nil {
		return 0
	}
	return time.Duration(sec * float64(time.Second))
}

// PutOffset stores the timing offset for one song in a "<key>.offset"
// sidecar file next to the lyrics. A zero offset removes the file. The
// offset persists per song, keyed like the lyrics cache.
func (c *Cache) PutOffset(artist, title, album string, durationSec int, off time.Duration) {
	if c == nil || c.dir == "" {
		return
	}
	if err := os.MkdirAll(c.dir, 0700); err != nil {
		return
	}
	path := filepath.Join(c.dir, c.key(artist, title, album, durationSec)+".offset")
	if off == 0 {
		os.Remove(path)
		return
	}
	_ = os.WriteFile(path, []byte(strconv.FormatFloat(off.Seconds(), 'f', -1, 64)), 0600)
}

// formatText renders a result to the text stored in a ".lrc" file. Synced
// lines get an LRC timestamp. Plain lines get their raw text. The lines join
// with a newline and carry no trailing newline, so a round trip through
// parseText yields the same line count.
func formatText(res Result) string {
	parts := make([]string, 0, len(res.Lines))
	for _, ln := range res.Lines {
		if res.Synced && ln.HasTime() {
			total := ln.At.Milliseconds()
			min := total / 60000
			sec := (total % 60000) / 1000
			cs := (total % 1000) / 10
			parts = append(parts, fmt.Sprintf("[%02d:%02d.%02d]%s", min, sec, cs, ln.Text))
		} else {
			parts = append(parts, ln.Text)
		}
	}
	return strings.Join(parts, "\n")
}

// parseText reads the text stored in a ".lrc" file back into a result. Text
// that holds an LRC timestamp parses as synced. Text with no timestamp
// parses as plain.
func parseText(text string) Result {
	if lrcTag.MatchString(text) {
		lines := parseLRC(text)
		if len(lines) > 0 {
			return Result{Lines: lines, Synced: true}
		}
	}
	return Result{Lines: parsePlain(text), Synced: false}
}
