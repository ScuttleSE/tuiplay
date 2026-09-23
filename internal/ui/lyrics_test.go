package ui

import (
	"testing"
	"time"
)

func TestActiveLyricIndex(t *testing.T) {
	lines := []lyricLine{
		{atMs: 1000, text: "first"},
		{atMs: 5000, text: "second"},
		{atMs: 9000, text: "third"},
		{atMs: -1, text: "untimed"},
	}

	cases := []struct {
		elapsed int64
		offset  time.Duration
		want    int
	}{
		{elapsed: 0, offset: 0, want: -1},
		{elapsed: 999, offset: 0, want: -1},
		{elapsed: 1000, offset: 0, want: 0},
		{elapsed: 5000, offset: 0, want: 1},
		{elapsed: 12000, offset: 0, want: 2},

		// A negative offset makes lines active earlier.
		{elapsed: 900, offset: -200 * time.Millisecond, want: 0},
		{elapsed: 4800, offset: -500 * time.Millisecond, want: 1},

		// A positive offset makes lines active later.
		{elapsed: 1000, offset: 500 * time.Millisecond, want: -1},
		{elapsed: 5400, offset: 500 * time.Millisecond, want: 0},
		{elapsed: 5500, offset: 500 * time.Millisecond, want: 1},
	}
	for _, c := range cases {
		got := activeLyricIndex(lines, c.elapsed, c.offset)
		if got != c.want {
			t.Errorf("activeLyricIndex(elapsed=%dms, offset=%v) = %d, want %d",
				c.elapsed, c.offset, got, c.want)
		}
	}
}
