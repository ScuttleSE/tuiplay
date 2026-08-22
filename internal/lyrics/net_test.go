package lyrics

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

// stubDoer returns a canned response for each request URL substring.
type stubDoer struct {
	fn func(req *http.Request) (*http.Response, error)
}

func (s stubDoer) Do(req *http.Request) (*http.Response, error) {
	return s.fn(req)
}

func mkResp(status int, body string, header http.Header) *http.Response {
	if header == nil {
		header = http.Header{}
	}
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     header,
	}
}

func TestLrcmuxSynced(t *testing.T) {
	c := &LrcmuxClient{
		base:      LrcmuxBaseURL,
		userAgent: "test",
		http: stubDoer{fn: func(req *http.Request) (*http.Response, error) {
			if req.Header.Get("User-Agent") != "test" {
				t.Fatal("missing user agent")
			}
			return mkResp(200, "[00:01.00]hello\n[00:02.50]world\n", nil), nil
		}},
	}
	res, found, err := c.Fetch("A", "B", "C", 100)
	if err != nil || !found {
		t.Fatalf("expected found, got found=%v err=%v", found, err)
	}
	if !res.Synced || len(res.Lines) != 2 || res.Lines[0].Text != "hello" {
		t.Fatalf("bad parse: %+v", res)
	}
}

func TestLrcmuxNotFound(t *testing.T) {
	c := &LrcmuxClient{base: LrcmuxBaseURL, userAgent: "t",
		http: stubDoer{fn: func(*http.Request) (*http.Response, error) {
			return mkResp(404, "", nil), nil
		}}}
	_, found, err := c.Fetch("A", "B", "C", 1)
	if found || err != nil {
		t.Fatalf("404 must be a clean miss, got found=%v err=%v", found, err)
	}
}

func TestLrcmuxRateLimited(t *testing.T) {
	h := http.Header{}
	h.Set("Retry-After", "30")
	c := &LrcmuxClient{base: LrcmuxBaseURL, userAgent: "t",
		http: stubDoer{fn: func(*http.Request) (*http.Response, error) {
			return mkResp(429, "", h), nil
		}}}
	_, found, err := c.Fetch("A", "B", "C", 1)
	if found || err == nil {
		t.Fatalf("429 must be an error, got found=%v err=%v", found, err)
	}
}

func TestLrclibNetGet(t *testing.T) {
	c := &LrclibNetClient{base: LrclibNetBaseURL, userAgent: "t",
		http: stubDoer{fn: func(req *http.Request) (*http.Response, error) {
			if !strings.Contains(req.URL.Path, "/api/get") {
				t.Fatalf("expected get, got %s", req.URL.Path)
			}
			return mkResp(200, `{"syncedLyrics":"[00:01.00]hi","plainLyrics":"hi"}`, nil), nil
		}}}
	res, found, err := c.Fetch("A", "B", "C", 100)
	if err != nil || !found || !res.Synced {
		t.Fatalf("bad get: found=%v synced=%v err=%v", found, res.Synced, err)
	}
}

func TestLrclibNetSearchFallback(t *testing.T) {
	c := &LrclibNetClient{base: LrclibNetBaseURL, userAgent: "t",
		http: stubDoer{fn: func(req *http.Request) (*http.Response, error) {
			if strings.Contains(req.URL.Path, "/api/get") {
				return mkResp(404, "", nil), nil
			}
			return mkResp(200, `[{"syncedLyrics":"","plainLyrics":"found it"}]`, nil), nil
		}}}
	res, found, err := c.Fetch("A", "B", "C", 100)
	if err != nil || !found || res.Synced {
		t.Fatalf("expected plain search hit: found=%v synced=%v err=%v", found, res.Synced, err)
	}
	if res.Lines[0].Text != "found it" {
		t.Fatalf("bad search text: %+v", res.Lines)
	}
}
