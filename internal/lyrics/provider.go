package lyrics

import "net/http"

// httpDoer performs one HTTP request. The standard *http.Client satisfies
// it. A test injects a stub.
type httpDoer interface {
	Do(req *http.Request) (*http.Response, error)
}

// Fetcher fetches lyrics for one song from a network provider. found is
// false when the provider has no lyrics for the song. A non-nil error marks
// a transport failure or a rate limit, not a clean miss; the caller does not
// cache a miss on an error.
type Fetcher interface {
	// Fetch returns the lyrics for one song. name reports a short label for
	// logs and errors.
	Fetch(artist, title, album string, durationSec int) (res Result, found bool, err error)
	// Name returns the provider's short name.
	Name() string
}
