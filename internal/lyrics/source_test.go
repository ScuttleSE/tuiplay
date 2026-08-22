package lyrics

import (
	"errors"
	"testing"
)

// stubFetcher is a test provider. It records calls and returns a canned
// result or error.
type stubFetcher struct {
	name  string
	res   Result
	found bool
	err   error
	calls int
}

func (s *stubFetcher) Name() string { return s.name }

func (s *stubFetcher) Fetch(artist, title, album string, durationSec int) (Result, bool, error) {
	s.calls++
	return s.res, s.found, s.err
}

func TestSourceCacheHit(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	c.Put("A", "B", "C", 10, plainResult())
	f := &stubFetcher{name: "net", res: syncedResult(), found: true}
	src := NewSource(c, nil, []Fetcher{f})

	res, ok := src.Lookup("A", "B", "C", 10)
	if !ok || res.Synced {
		t.Fatalf("expected cached plain hit, got ok=%v synced=%v", ok, res.Synced)
	}
	if f.calls != 0 {
		t.Fatalf("cache hit must not call the network, got %d calls", f.calls)
	}
}

func TestSourceNetHitBackfills(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	f := &stubFetcher{name: "net", res: syncedResult(), found: true}
	src := NewSource(c, nil, []Fetcher{f})

	if _, ok := src.Lookup("A", "B", "C", 10); !ok {
		t.Fatal("expected a network hit")
	}
	// The second lookup must hit the cache, not the network.
	if _, ok := src.Lookup("A", "B", "C", 10); !ok {
		t.Fatal("expected a cache hit on the second lookup")
	}
	if f.calls != 1 {
		t.Fatalf("expected one network call, got %d", f.calls)
	}
}

func TestSourceChainFallbackOnError(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	a := &stubFetcher{name: "a", err: errors.New("boom")}
	b := &stubFetcher{name: "b", res: plainResult(), found: true}
	src := NewSource(c, nil, []Fetcher{a, b})

	if _, ok := src.Lookup("A", "B", "C", 10); !ok {
		t.Fatal("expected provider b to answer")
	}
	if a.calls != 1 || b.calls != 1 {
		t.Fatalf("expected both providers tried, got a=%d b=%d", a.calls, b.calls)
	}
}

func TestSourceChainFallbackOnMiss(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	a := &stubFetcher{name: "a", found: false}
	b := &stubFetcher{name: "b", res: plainResult(), found: true}
	src := NewSource(c, nil, []Fetcher{a, b})

	if _, ok := src.Lookup("A", "B", "C", 10); !ok {
		t.Fatal("expected provider b to answer after a clean miss")
	}
	if a.calls != 1 || b.calls != 1 {
		t.Fatalf("got a=%d b=%d", a.calls, b.calls)
	}
}

func TestSourceCleanMissWritesMiss(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	f := &stubFetcher{name: "net", found: false}
	src := NewSource(c, nil, []Fetcher{f})

	if _, ok := src.Lookup("A", "B", "C", 10); ok {
		t.Fatal("expected a miss")
	}
	// A fresh miss must block a second network call.
	if _, ok := src.Lookup("A", "B", "C", 10); ok {
		t.Fatal("expected the cached miss to hold")
	}
	if f.calls != 1 {
		t.Fatalf("expected one network call, got %d", f.calls)
	}
}

func TestSourceProviderErrorNoMiss(t *testing.T) {
	c := NewCache(t.TempDir(), 7)
	f := &stubFetcher{name: "net", err: errors.New("network down")}
	src := NewSource(c, nil, []Fetcher{f})

	if _, ok := src.Lookup("A", "B", "C", 10); ok {
		t.Fatal("expected a miss on error")
	}
	// An error must not cache a miss; the second lookup retries.
	if _, ok := src.Lookup("A", "B", "C", 10); ok {
		t.Fatal("still expected a miss")
	}
	if f.calls != 2 {
		t.Fatalf("expected a retry after an error, got %d calls", f.calls)
	}
}

func TestSourceUsable(t *testing.T) {
	if (&Source{}).Usable() {
		t.Fatal("an empty source is not usable")
	}
	c := NewCache(t.TempDir(), 7)
	if !NewSource(c, nil, nil).Usable() {
		t.Fatal("a writable cache makes the source usable")
	}
	if !NewSource(NewCache("", 7), nil, []Fetcher{&stubFetcher{}}).Usable() {
		t.Fatal("a network provider makes the source usable")
	}
	if NewSource(NewCache("", 7), nil, nil).Usable() {
		t.Fatal("no tier means not usable")
	}
}
