package lyrics

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// LrclibNetBaseURL is the default base URL of the public lrclib.net API.
const LrclibNetBaseURL = "https://lrclib.net"

// LrclibNetClient fetches lyrics from the public lrclib.net REST API. It
// tries the exact "get" endpoint first. On a 404 it falls back to the
// "search" endpoint and takes the first result.
type LrclibNetClient struct {
	base      string
	userAgent string
	http      httpDoer
}

// NewLrclibNetClient returns a client for the lrclib.net API. userAgent is
// the value of the User-Agent header the client sends.
func NewLrclibNetClient(userAgent string) *LrclibNetClient {
	return &LrclibNetClient{
		base:      LrclibNetBaseURL,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the provider name.
func (c *LrclibNetClient) Name() string { return "lrclib" }

// lrclibResult is one lyrics record from the lrclib.net API.
type lrclibResult struct {
	SyncedLyrics string `json:"syncedLyrics"`
	PlainLyrics  string `json:"plainLyrics"`
}

// Fetch fetches lyrics for one song from lrclib.net.
func (c *LrclibNetClient) Fetch(artist, title, album string, durationSec int) (Result, bool, error) {
	// Exact match first.
	q := url.Values{}
	q.Set("artist_name", artist)
	q.Set("track_name", title)
	if album != "" {
		q.Set("album_name", album)
	}
	if durationSec > 0 {
		q.Set("duration", strconv.Itoa(durationSec))
	}
	var one lrclibResult
	status, err := c.getJSON(c.base+"/api/get?"+q.Encode(), &one)
	if err != nil {
		return Result{}, false, err
	}
	if status == http.StatusOK {
		if res, ok := resultFromLyrics(one.SyncedLyrics, one.PlainLyrics); ok {
			return res, true, nil
		}
		return Result{}, false, nil
	}
	if status != http.StatusNotFound {
		return Result{}, false, fmt.Errorf("lrclib get: status %d", status)
	}

	// Relaxed search on a 404.
	sq := url.Values{}
	sq.Set("artist_name", artist)
	sq.Set("track_name", title)
	if album != "" {
		sq.Set("album_name", album)
	}
	var list []lrclibResult
	status, err = c.getJSON(c.base+"/api/search?"+sq.Encode(), &list)
	if err != nil {
		return Result{}, false, err
	}
	if status != http.StatusOK {
		return Result{}, false, nil
	}
	for _, r := range list {
		if res, ok := resultFromLyrics(r.SyncedLyrics, r.PlainLyrics); ok {
			return res, true, nil
		}
	}
	return Result{}, false, nil
}

// getJSON performs a GET request and decodes a JSON body into v when the
// status is 200. It returns the HTTP status code.
func (c *LrclibNetClient) getJSON(rawURL string, v interface{}) (int, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusOK {
		if err := json.NewDecoder(resp.Body).Decode(v); err != nil {
			return resp.StatusCode, err
		}
	}
	return resp.StatusCode, nil
}

// resultFromLyrics builds a result from synced and plain lyrics text. It
// prefers synced lyrics. ok is false when both are empty.
func resultFromLyrics(synced, plain string) (Result, bool) {
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
