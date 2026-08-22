package lyrics

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"
)

// LrcmuxBaseURL is the default base URL of the lrcmux aggregation API.
const LrcmuxBaseURL = "https://api.lrcmux.dev"

// LrcmuxClient fetches lyrics from the lrcmux aggregation API. lrcmux queries
// several providers and returns the best result. The client asks for the
// "lrc" format and a line sync level. It accepts a lower level, so a track
// with no sync still returns plain text.
type LrcmuxClient struct {
	base      string
	userAgent string
	http      httpDoer
}

// NewLrcmuxClient returns a client for the lrcmux API. userAgent is the
// value of the User-Agent header the client sends.
func NewLrcmuxClient(userAgent string) *LrcmuxClient {
	return &LrcmuxClient{
		base:      LrcmuxBaseURL,
		userAgent: userAgent,
		http:      &http.Client{Timeout: 15 * time.Second},
	}
}

// Name returns the provider name.
func (c *LrcmuxClient) Name() string { return "lrcmux" }

// Fetch fetches lyrics for one song from lrcmux. It requests the lrc format,
// so the body is ready-to-parse LRC or plain text.
func (c *LrcmuxClient) Fetch(artist, title, album string, durationSec int) (Result, bool, error) {
	q := url.Values{}
	q.Set("artist", artist)
	q.Set("title", title)
	if album != "" {
		q.Set("album", album)
	}
	if durationSec > 0 {
		q.Set("duration", strconv.Itoa(durationSec))
	}
	q.Set("format", "lrc")
	q.Set("level", "line")

	req, err := http.NewRequest(http.MethodGet, c.base+"/get?"+q.Encode(), nil)
	if err != nil {
		return Result{}, false, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/plain")

	resp, err := c.http.Do(req)
	if err != nil {
		return Result{}, false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return Result{}, false, err
		}
		res := parseText(string(body))
		if len(res.Lines) == 0 {
			return Result{}, false, nil
		}
		return res, true, nil
	case http.StatusNotFound:
		return Result{}, false, nil
	case http.StatusTooManyRequests:
		// Respect the rate limit. Return an error so the caller does not
		// cache a miss and retries later.
		ra := resp.Header.Get("Retry-After")
		return Result{}, false, fmt.Errorf("lrcmux: rate limited (Retry-After: %s)", ra)
	default:
		return Result{}, false, fmt.Errorf("lrcmux: status %d", resp.StatusCode)
	}
}
