package control

import "testing"

func TestParse(t *testing.T) {
	cases := []struct {
		in       string
		wantName string
		wantArg  string
	}{
		{"next", "next", ""},
		{"  Pause  ", "pause", ""},
		{"seek -10", "seek", "-10"},
		{"volume +", "volume", "+"},
		{"playlist My Focus Mix", "playlist", "My Focus Mix"},
		{"PLAYLIST  Spaced  Name ", "playlist", "Spaced  Name"},
		{"", "", ""},
	}
	for _, c := range cases {
		got := Parse(c.in)
		if got.Name != c.wantName || got.Arg != c.wantArg {
			t.Errorf("Parse(%q) = {%q, %q}, want {%q, %q}",
				c.in, got.Name, got.Arg, c.wantName, c.wantArg)
		}
	}
}
