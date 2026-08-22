package ui

import (
	"github.com/charmbracelet/lipgloss"
	"testing"
)

func TestTrimWideRunes(t *testing.T) {
	cases := []string{
		"ascii only",
		"I’ve IVE (ver. 2)", // curly apostrophe
		"日本語のタイトル",          // wide CJK
		"混合 mixed 文字列",      // mixed
		"",
	}
	for _, s := range cases {
		for w := 0; w <= 20; w++ {
			out := trim(s, w)
			if lipgloss.Width(out) > w {
				t.Fatalf("trim(%q,%d)=%q width %d > %d", s, w, out, lipgloss.Width(out), w)
			}
		}
	}
}
