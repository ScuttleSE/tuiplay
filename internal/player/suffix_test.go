package player

import "testing"

func TestSupportedSuffix(t *testing.T) {
	supported := []string{"mp3", "flac", "ogg", "oga", "vorbis", "MP3", "Flac", "m4a", "m4b", "mp4", "aac", "M4A"}
	for _, s := range supported {
		if !SupportedSuffix(s) {
			t.Errorf("SupportedSuffix(%q) = false, want true", s)
		}
	}
	unsupported := []string{"alac", "wav", "opus", "wma", ""}
	for _, s := range unsupported {
		if SupportedSuffix(s) {
			t.Errorf("SupportedSuffix(%q) = true, want false", s)
		}
	}
}
