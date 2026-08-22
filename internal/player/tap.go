package player

import (
	"sync"

	"github.com/gopxl/beep/v2"
)

// tapSize is the number of mono samples the tap keeps for the visualizer.
// It holds a small, fixed window of the most recent output.
const tapSize = 4096

// tap wraps a beep.Streamer. It passes samples through unchanged and copies
// the most recent output into a ring buffer. The visualizer reads the
// buffer through Samples. The tap is safe for concurrent use: the speaker
// goroutine writes, and the interface goroutine reads.
type tap struct {
	inner beep.Streamer

	mu  sync.Mutex
	buf [tapSize]float64    // mono ring buffer, newest at head-1
	st  [tapSize][2]float64 // stereo ring buffer, newest at head-1
	pos int                 // write index into buf and st
}

// newTap wraps a streamer with a sample tap.
func newTap(inner beep.Streamer) *tap {
	return &tap{inner: inner}
}

// Stream fills samples from the inner streamer and copies a mono mix into
// the ring buffer.
func (t *tap) Stream(samples [][2]float64) (int, bool) {
	n, ok := t.inner.Stream(samples)
	if n > 0 {
		t.mu.Lock()
		for i := 0; i < n; i++ {
			t.buf[t.pos] = (samples[i][0] + samples[i][1]) * 0.5
			t.st[t.pos] = samples[i]
			t.pos++
			if t.pos >= tapSize {
				t.pos = 0
			}
		}
		t.mu.Unlock()
	}
	return n, ok
}

// Err returns the inner streamer error.
func (t *tap) Err() error {
	return t.inner.Err()
}

// Samples copies the captured window into dst in time order, oldest first.
// It returns the number of samples written, which is min(len(dst), tapSize).
func (t *tap) Samples(dst []float64) int {
	n := len(dst)
	if n > tapSize {
		n = tapSize
	}
	t.mu.Lock()
	// The oldest of the last n samples starts n behind the write head.
	start := t.pos - n
	for i := 0; i < n; i++ {
		idx := start + i
		for idx < 0 {
			idx += tapSize
		}
		if idx >= tapSize {
			idx -= tapSize
		}
		dst[i] = t.buf[idx]
	}
	t.mu.Unlock()
	return n
}

// SamplesStereo copies the captured stereo window into dst in time order,
// oldest first. It returns the number of frames written, which is
// min(len(dst), tapSize).
func (t *tap) SamplesStereo(dst [][2]float64) int {
	n := len(dst)
	if n > tapSize {
		n = tapSize
	}
	t.mu.Lock()
	start := t.pos - n
	for i := 0; i < n; i++ {
		idx := start + i
		for idx < 0 {
			idx += tapSize
		}
		if idx >= tapSize {
			idx -= tapSize
		}
		dst[i] = t.st[idx]
	}
	t.mu.Unlock()
	return n
}
