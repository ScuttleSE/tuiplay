package ui

import "testing"

func TestNextIndexSequential(t *testing.T) {
	idx, stop := nextIndexPure(3, 0, false, false, false)
	if stop || idx != 1 {
		t.Fatalf("got %d stop=%v", idx, stop)
	}
}

func TestNextIndexEndStops(t *testing.T) {
	_, stop := nextIndexPure(3, 2, false, false, false)
	if !stop {
		t.Fatal("should stop at end")
	}
}

func TestNextIndexRepeatWraps(t *testing.T) {
	idx, stop := nextIndexPure(3, 2, true, false, false)
	if stop || idx != 0 {
		t.Fatalf("repeat should wrap to 0, got %d stop=%v", idx, stop)
	}
}

func TestNextIndexConsumeRemoved(t *testing.T) {
	// After removing current at index 1 of a now-2-length queue, the next
	// song sits at index 1.
	idx, stop := nextIndexPure(2, 1, false, false, true)
	if stop || idx != 1 {
		t.Fatalf("consume next got %d stop=%v", idx, stop)
	}
}

func TestNextIndexConsumeEmpty(t *testing.T) {
	_, stop := nextIndexPure(0, 0, false, false, true)
	if !stop {
		t.Fatal("empty queue should stop")
	}
}

func TestNextIndexShuffleDiffers(t *testing.T) {
	// With 2 songs, shuffle must not pick the current one.
	for i := 0; i < 50; i++ {
		idx, stop := nextIndexPure(2, 0, false, true, false)
		if stop || idx == 0 {
			t.Fatalf("shuffle picked current: %d stop=%v", idx, stop)
		}
	}
}
