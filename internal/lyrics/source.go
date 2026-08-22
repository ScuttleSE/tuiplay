package lyrics

import (
	"fmt"
	"os"
)

// Source looks up lyrics through a tiered chain. It checks the local cache
// first, then the lrclib SQLite dump, then the network providers in order.
// A hit from the dump or a provider back-fills the cache. A clean total miss
// writes a negative cache entry. A provider error does not write a miss.
type Source struct {
	// cache is the local file cache. It may be a disabled cache.
	cache *Cache
	// dump is the lrclib SQLite dump. It is nil when no dump is set.
	dump *DB
	// net is the ordered list of network providers. An empty list turns
	// off the network tier.
	net []Fetcher
}

// NewSource builds a lyrics source from a cache, an optional dump, and an
// ordered list of network providers. Any argument may be nil or empty to
// disable that tier.
func NewSource(cache *Cache, dump *DB, net []Fetcher) *Source {
	return &Source{cache: cache, dump: dump, net: net}
}

// Usable reports whether the source can produce lyrics. It is true when the
// cache can store entries, or a dump is set, or at least one network
// provider is set.
func (s *Source) Usable() bool {
	if s == nil {
		return false
	}
	if s.cache.Writable() {
		return true
	}
	if s.dump != nil {
		return true
	}
	return len(s.net) > 0
}

// Lookup finds the lyrics for one song through the tiered chain. The bool is
// false when no tier has lyrics.
func (s *Source) Lookup(artist, title, album string, durationSec int) (Result, bool) {
	if s == nil {
		return Result{}, false
	}

	// Tier 1: the cache. A hit wins. A fresh miss blocks the chain.
	res, hit, blocked := s.cache.Get(artist, title, album, durationSec)
	if hit {
		return res, true
	}
	if blocked {
		return Result{}, false
	}

	// Tier 2: the lrclib SQLite dump.
	if s.dump != nil {
		if r, ok := s.dump.Lookup(artist, title, album, durationSec); ok {
			s.cache.Put(artist, title, album, durationSec, r)
			return r, true
		}
	}

	// Tier 3: the network providers, in order. A provider error moves to
	// the next provider. A clean miss also moves on.
	provErr := false
	for _, f := range s.net {
		r, found, err := f.Fetch(artist, title, album, durationSec)
		if err != nil {
			provErr = true
			fmt.Fprintf(os.Stderr, "warning: lyrics provider %s: %v\n", f.Name(), err)
			continue
		}
		if found {
			s.cache.Put(artist, title, album, durationSec, r)
			return r, true
		}
	}

	// No tier had lyrics. Cache a miss only when no provider errored, so a
	// transient failure does not suppress a later retry.
	if !provErr {
		s.cache.PutMiss(artist, title, album, durationSec)
	}
	return Result{}, false
}
