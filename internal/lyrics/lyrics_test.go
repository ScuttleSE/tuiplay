package lyrics

import (
	"testing"
	"time"
)

func TestParseLRC(t *testing.T) {
	text := "[00:12.00]Line one\n[00:15.30]Line two\n[bad]ignored tag body\n"
	lines := parseLRC(text)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].Text != "Line one" || lines[0].At != 12*time.Second {
		t.Errorf("line 0 = %+v", lines[0])
	}
	if lines[1].At != 15*time.Second+300*time.Millisecond {
		t.Errorf("line 1 at = %v", lines[1].At)
	}
	if !lines[0].HasTime() {
		t.Error("synced line should have time")
	}
}

func TestParseLRCMultiTag(t *testing.T) {
	// One body with two timestamps yields two lines.
	lines := parseLRC("[00:01.00][00:05.00]Repeat")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].At != time.Second || lines[1].At != 5*time.Second {
		t.Errorf("times = %v, %v", lines[0].At, lines[1].At)
	}
}

func TestParsePlain(t *testing.T) {
	lines := parsePlain("a\nb")
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	if lines[0].HasTime() {
		t.Error("plain line should not have time")
	}
}
