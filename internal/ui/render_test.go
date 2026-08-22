package ui

import (
	"testing"

	"git.hemmalab.se/scuttle/tuiplay/internal/config"
	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"

	tea "github.com/charmbracelet/bubbletea"
)

// invertedDefaults returns the default action-to-key map for tests.
func invertedDefaults() map[string]string {
	return config.DefaultKeys()
}

func newTestModel(t *testing.T) model {
	pl, err := player.New()
	if err != nil {
		t.Skip("no audio device:", err)
	}
	m := newModel(nil, pl, UIState{RightShown: true, SplitRatio: 0.6}, Settings{}, invertedDefaults(), nil, false)
	m.queue = []subsonic.Song{
		{Title: "Mr. Blue Sky", Artist: "Electric Light Orchestra", Album: "Top of the Pops 1978", Track: 3, Duration: 304, Year: 1978},
		{Title: "アイヴ", Artist: "IVE アイヴ", Album: "I’ve IVE (ver. 2)", Track: 1, Duration: 200, Year: 2023},
	}
	m.queueIndex = 0
	return m
}

// keyMsgFor builds a KeyMsg that triggers the given action under the
// model's key map.
func keyMsgFor(t *testing.T, m model, action string) tea.KeyMsg {
	t.Helper()
	for key, act := range m.keyAction {
		if act == action {
			return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)}
		}
	}
	t.Fatalf("no key bound to action %q", action)
	return tea.KeyMsg{}
}

func TestShowNavSwitchesToBrowse(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 80, 24
	// Drill into the browse view, then switch to the search view.
	m.pushLevel(navLevel{kind: navArtists, title: "Artists"})
	m.activeView = viewSearch
	if m.topLevel().kind != navSearch {
		t.Fatalf("setup: top = %v, want navSearch", m.topLevel().kind)
	}
	// show_nav switches back to the browse view, preserving its position.
	tm, _ := m.handleKey(keyMsgFor(t, m, config.ActShowNav))
	nm := tm.(model)
	if nm.activeView != viewBrowse {
		t.Fatalf("after show_nav: activeView = %v, want viewBrowse", nm.activeView)
	}
	if nm.topLevel().kind != navArtists {
		t.Fatalf("after show_nav: top = %v, want navArtists", nm.topLevel().kind)
	}
	if !nm.rightShown || nm.focus != focusRight {
		t.Fatalf("after show_nav: rightShown=%v focus=%v", nm.rightShown, nm.focus)
	}
}

func TestShowSearchSwitchesView(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 80, 24
	m.pushLevel(navLevel{kind: navArtists, title: "Artists"})
	// Switch to search, then back to browse: the browse position holds.
	tm, _ := m.handleKey(keyMsgFor(t, m, config.ActShowSearch))
	nm := tm.(model)
	if nm.activeView != viewSearch || nm.topLevel().kind != navSearch {
		t.Fatalf("show_search: activeView=%v top=%v", nm.activeView, nm.topLevel().kind)
	}
	tm, _ = nm.handleKey(keyMsgFor(t, nm, config.ActShowNav))
	nm = tm.(model)
	if nm.topLevel().kind != navArtists {
		t.Fatalf("browse position lost: top=%v", nm.topLevel().kind)
	}
}

func TestViewNoPanic(t *testing.T) {
	m := newTestModel(t)
	// Build a few nav levels to exercise rendering.
	m.pushLevel(navLevel{kind: navArtists, title: "Artists", rows: artistRows([]subsonic.Artist{{Name: "IVE アイヴ 日本語", ID: "1"}}), cursor: 0})
	for _, sz := range [][2]int{{142, 59}, {80, 24}, {40, 15}, {30, 10}} {
		m.width, m.height = sz[0], sz[1]
		for _, shown := range []bool{true, false} {
			m.rightShown = shown
			for _, f := range []paneFocus{focusQueue, focusRight} {
				m.focus = f
				// back row selected
				m.top().cursor = -1
				if m.View() == "" {
					t.Fatalf("empty view %v", sz)
				}
				m.top().cursor = 0
				if m.View() == "" {
					t.Fatalf("empty view %v", sz)
				}
			}
		}
	}
}

func TestScanSpinnerRender(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 100, 30
	m.scanning = true
	for _, f := range []int{0, 3, 9, 10} {
		m.spinFrame = f
		if m.View() == "" {
			t.Fatalf("empty view with spinner frame %d", f)
		}
	}
	m.repeat, m.crossfade = true, true
	if m.View() == "" {
		t.Fatal("empty view with spinner and flags")
	}
}

func TestSearchAndSmartRender(t *testing.T) {
	m := newTestModel(t)
	m.focus = focusRight
	levels := []navLevel{
		searchLevel(),
		smartBuilderLevel(&smartBuilder{
			name:       "Test",
			conditions: []nspCondition{{field: "genre", operator: "contains", value: "rock"}},
			sortField:  "year",
			order:      "desc",
			limit:      10,
		}),
		smartFieldLevel("Field"),
		smartOpLevel(nspFields[4]),
	}
	for _, lvl := range levels {
		m.browseNav = []navLevel{rootLevel(), lvl}
		for _, sz := range [][2]int{{142, 59}, {80, 24}, {40, 12}} {
			m.width, m.height = sz[0], sz[1]
			for _, c := range []int{-1, 0, len(lvl.rows) - 1} {
				m.top().cursor = c
				if m.View() == "" {
					t.Fatalf("empty view kind=%d size=%v cursor=%d", lvl.kind, sz, c)
				}
			}
		}
	}
}

func TestBackRowNavigation(t *testing.T) {
	m := newTestModel(t)
	m.pushLevel(navLevel{kind: navArtists, title: "Artists", rows: artistRows([]subsonic.Artist{{Name: "A", ID: "1"}}), cursor: 0})
	if len(m.browseNav) != 2 {
		t.Fatalf("want 2 levels, got %d", len(m.browseNav))
	}
	// Put cursor on the back row and press Enter.
	m.top().cursor = -1
	m.focus = focusRight
	out, _ := m.openItem()
	nm := out.(model)
	if len(nm.browseNav) != 1 {
		t.Fatalf("back row did not pop level: %d", len(nm.browseNav))
	}
}

func TestNewStatesNoPanic(t *testing.T) {
	m := newTestModel(t)
	m.width, m.height = 120, 30
	// help card
	m.showHelp = true
	if m.View() == "" {
		t.Fatal("help empty")
	}
	m.showHelp = false
	// confirm
	m.confirmActive = true
	m.confirmLabel = "Clear the queue? (y/n)"
	if m.View() == "" {
		t.Fatal("confirm empty")
	}
	m.confirmActive = false
	// prompt
	m.promptActive = true
	m.promptLabel = "Save queue as playlist"
	m.promptInput = "My List"
	if m.View() == "" {
		t.Fatal("prompt empty")
	}
	m.promptActive = false
	// song info level
	m.pushLevel(songInfoLevel(m.queue[0]))
	if m.View() == "" {
		t.Fatal("info empty")
	}
	// flags
	m.repeat, m.consume, m.shuffle, m.crossfade = true, false, true, true
	if m.View() == "" {
		t.Fatal("flags empty")
	}
}

func TestDeleteCurrentKeepsPlaying(t *testing.T) {
	m := newTestModel(t)
	m.queueIndex = 0
	m.focus = focusQueue
	m.queueCursor = 0
	out, _ := m.deleteSelected()
	nm := out.(model)
	if len(nm.queue) != 1 {
		t.Fatalf("want 1 left, got %d", len(nm.queue))
	}
	if nm.queueIndex != -1 {
		t.Fatalf("deleted current should set index -1, got %d", nm.queueIndex)
	}
}

func TestDeleteBelowCurrentShiftsIndex(t *testing.T) {
	m := newTestModel(t)
	// three songs
	m.queue = append(m.queue, m.queue[0])
	m.queueIndex = 2
	m.focus = focusQueue
	m.queueCursor = 0
	out, _ := m.deleteSelected()
	nm := out.(model)
	if nm.queueIndex != 1 {
		t.Fatalf("index should shift to 1, got %d", nm.queueIndex)
	}
}

func TestReplaceMsgPlays(t *testing.T) {
	m := newTestModel(t)
	songs := m.queue
	m.queue = nil
	out, _ := m.Update(replaceMsg{songs: songs, label: "PL", sourceID: "p1", sourceName: "PL", play: true})
	nm := out.(model)
	if len(nm.queue) != len(songs) {
		t.Fatalf("queue not replaced: %d", len(nm.queue))
	}
	if nm.queueIndex != 0 {
		t.Fatalf("should play first, index=%d", nm.queueIndex)
	}
	if nm.sourcePlaylistID != "p1" {
		t.Fatalf("source not set: %q", nm.sourcePlaylistID)
	}
}

func TestPlaylistDeleteConfirm(t *testing.T) {
	m := newTestModel(t)
	m.pushLevel(navLevel{kind: navPlaylists, rows: []navRow{{label: "PL (3)", id: "p1", name: "PL"}}, cursor: 0})
	m.focus = focusRight
	out, _ := m.deleteSelected()
	nm := out.(model)
	if !nm.confirmActive {
		t.Fatal("delete should ask to confirm")
	}
}
