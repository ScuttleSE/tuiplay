package ui

import (
	"testing"

	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"
)

func TestPlayGateSerializesRequests(t *testing.T) {
	g := &playGate{}
	g1 := g.next()
	if !g.enter(g1) {
		t.Fatal("the newest generation may use the player")
	}
	g2 := g.next()
	if g.enter(g1) {
		t.Fatal("a newer request invalidates the older generation")
	}
	if !g.enter(g2) {
		t.Fatal("the newest generation may use the player")
	}
}

func newGateTestModel(t *testing.T) model {
	t.Helper()
	pl, err := player.New()
	if err != nil {
		t.Skip("no audio device:", err)
	}
	return newModel(nil, pl, UIState{}, Settings{}, nil, nil, false)
}

func TestStopPlayerInvalidatesInFlightLoad(t *testing.T) {
	m := newGateTestModel(t)
	req := m.playRequestFor(subsonic.Song{}, "url", "mp3", false, playKindPlay)
	m.stopPlayer()
	msg := req.cmd()()
	ps, ok := msg.(playStartedMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want playStartedMsg", msg)
	}
	if !ps.dropped {
		t.Fatal("a stopped request must drop without touching the player")
	}
}

func TestStalePlayRequestDrops(t *testing.T) {
	m := newGateTestModel(t)
	req := m.playRequestFor(subsonic.Song{}, "url", "mp3", false, playKindPlay)
	// A newer request invalidates the first one.
	m.playRequestFor(subsonic.Song{}, "url2", "mp3", false, playKindPlay)
	msg := req.cmd()()
	ps, ok := msg.(playStartedMsg)
	if !ok {
		t.Fatalf("cmd returned %T, want playStartedMsg", msg)
	}
	if !ps.dropped {
		t.Fatal("a superseded request must drop without touching the player")
	}
}

func TestPlayStartedDropsStaleMessage(t *testing.T) {
	m := newGateTestModel(t)
	before := m.status
	tm, _ := m.playStarted(playStartedMsg{gen: 999})
	nm := tm.(model)
	if nm.status != before {
		t.Fatalf("a stale result changed the status to %q", nm.status)
	}
}
