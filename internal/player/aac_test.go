package player

import (
	"errors"
	"os"
	"testing"
)

// TestDecodeAACRejectsGarbage checks that a file that is not a valid MP4/M4A
// container fails with ErrTranscodeNeeded, so the caller falls back to a
// server-side transcode instead of crashing.
func TestDecodeAACRejectsGarbage(t *testing.T) {
	f, err := os.CreateTemp(t.TempDir(), "notm4a-*.m4a")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString("this is not an mp4 container at all"); err != nil {
		t.Fatal(err)
	}
	if _, err := f.Seek(0, 0); err != nil {
		t.Fatal(err)
	}

	_, _, derr := decodeAAC(f)
	if derr == nil {
		t.Fatal("decodeAAC on garbage: got nil error, want ErrTranscodeNeeded")
	}
	if !errors.Is(derr, ErrTranscodeNeeded) {
		t.Errorf("decodeAAC on garbage: error %v does not wrap ErrTranscodeNeeded", derr)
	}
}
