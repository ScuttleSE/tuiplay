package ui

import (
	"errors"
	"sync"

	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"

	tea "github.com/charmbracelet/bubbletea"
)

// errStale marks a play request that a newer request or a stop superseded.
var errStale = errors.New("play request superseded")

// playGate serializes play requests. Each request takes a generation
// number. A request may call into the player only while its generation is
// the newest. A stop or a newer play request invalidates the older ones,
// so two loads cannot interleave inside the player.
type playGate struct {
	mu  sync.Mutex
	seq uint64
}

// next starts a new play request and returns its generation. It
// invalidates every older generation.
func (g *playGate) next() uint64 {
	g.mu.Lock()
	defer g.mu.Unlock()
	g.seq++
	return g.seq
}

// enter reports whether the generation gen may still use the player. It
// returns false when a newer request or a stop superseded it.
func (g *playGate) enter(gen uint64) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	return gen == g.seq
}

// playKind names why a play request runs. The restore kind holds the
// track paused at a position and does not scrobble.
type playKind int

const (
	playKindPlay playKind = iota
	playKindRestore
)

// playStartedMsg reports the result of one asynchronous play request.
// dropped is true when a newer request or a stop superseded it, so the
// handler ignores it.
type playStartedMsg struct {
	gen        uint64
	kind       playKind
	song       subsonic.Song
	transcoded bool
	dropped    bool
	err        error
}

// playRequest holds everything one asynchronous play needs. The model
// builds it on the Bubble Tea goroutine, so the command closure never
// reads the model on another goroutine.
type playRequest struct {
	gate      *playGate
	pl        *player.Player
	cl        *subsonic.Client
	song      subsonic.Song
	url       string
	suffix    string
	crossfade bool
	xfadeSec  int
	pausedAt  int // restore: the second to hold the track paused at
	kind      playKind
	gen       uint64
}

// cmd returns the command that runs the play request. It loads the track
// through the player with the transcode fallback, scrobbles a normal
// start, and returns a playStartedMsg.
func (r playRequest) cmd() tea.Cmd {
	return func() tea.Msg {
		transcoded, err := r.load()
		if errors.Is(err, errStale) {
			return playStartedMsg{gen: r.gen, kind: r.kind, song: r.song, dropped: true}
		}
		if err == nil && r.kind == playKindPlay && r.cl != nil {
			go r.cl.Scrobble(r.song.ID, false)
		}
		return playStartedMsg{gen: r.gen, kind: r.kind, song: r.song, transcoded: transcoded, err: err}
	}
}

// load calls into the player. A normal start plays the track at once, or
// crossfades into it when crossfade is set. A restore holds the track
// paused at pausedAt seconds. Both retry once through a server-side
// transcode when the source cannot decode natively. A superseded request
// returns errStale and touches the player not at all.
func (r playRequest) load() (transcoded bool, err error) {
	if !r.gate.enter(r.gen) {
		return false, errStale
	}
	if r.kind == playKindRestore {
		err = r.pl.PlayPausedAt(r.url, r.suffix, r.pausedAt)
	} else if r.crossfade {
		err = r.pl.Crossfade(r.url, r.suffix, r.xfadeSec)
	} else {
		err = r.pl.Play(r.url, r.suffix)
	}
	if err == nil {
		return false, nil
	}
	if !errors.Is(err, player.ErrTranscodeNeeded) {
		return false, err
	}
	if !r.gate.enter(r.gen) {
		return false, errStale
	}
	tu, tsuf, terr := r.transcodeURL()
	if terr != nil {
		return false, terr
	}
	if r.kind == playKindRestore {
		err = r.pl.PlayPausedAt(tu, tsuf, r.pausedAt)
	} else if r.crossfade {
		err = r.pl.Crossfade(tu, tsuf, r.xfadeSec)
	} else {
		err = r.pl.Play(tu, tsuf)
	}
	return true, err
}

// transcodeURL returns a server-transcoded stream URL and suffix for the
// song of this request.
func (r playRequest) transcodeURL() (string, string, error) {
	u, err := r.cl.StreamURLFormat(r.song.ID, player.TranscodeFormat)
	return u, player.TranscodeFormat, err
}

// playRequestFor builds the asynchronous play request for one song. It
// takes a fresh generation, which invalidates any older request.
func (m model) playRequestFor(s subsonic.Song, url, suffix string, crossfade bool, kind playKind) playRequest {
	return playRequest{
		gate:      m.playGate,
		pl:        m.player,
		cl:        m.client,
		song:      s,
		url:       url,
		suffix:    suffix,
		crossfade: crossfade,
		xfadeSec:  m.crossfadeSec,
		kind:      kind,
		gen:       m.playGate.next(),
	}
}

// stopPlayer stops playback and invalidates any in-flight play request,
// so a still-loading track cannot start after the stop.
func (m *model) stopPlayer() {
	m.playGate.next()
	m.loadingPlay = false
	m.player.Stop()
}
