// Package ui is the terminal user interface. It uses Bubble Tea.
//
// The screen has two panes side by side, plus shared bars. The left pane
// is always the play queue. The right pane is a navigation panel. The
// panel is a stack of levels. The user drills down from the entrypoints
// (Artists, Albums, Genres, Release Years, Tracks) into the library. The
// user can hide the right pane so the queue fills the width.
//
// The layout follows ncmpcpp:
//
//   - a header line at the top with the queue summary and the volume;
//   - the two panes in the middle, split by a vertical divider;
//   - a progress bar above the status bar;
//   - a status bar at the bottom with the track and the times.
//
// The key bindings follow ncmpcpp:
//
//	1        hide the right pane; the queue fills the screen
//	2        show the navigation panel in the right pane
//	Tab      move focus between the queue pane and the right pane
//	Enter    on a queue song: play it;
//	         on a navigation entry: drill down;
//	         on a track in the panel: play it now;
//	         on the [..] row: go back one level
//	a        add the selected track to the bottom of the queue
//	Space    add everything under the panel item to the bottom of the queue
//	R        toggle consume mode
//	p        pause or resume
//	s        stop
//	>        next track
//	<        previous track
//	f        seek forward
//	b        seek back
//	+        raise the volume
//	-        lower the volume
//	j / down move down
//	k / up   move up
//	PgDn     move down one screen
//	PgUp     move up one screen
//	q        quit
package ui

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"time"

	"git.hemmalab.se/scuttle/tuiplay/internal/control"
	"git.hemmalab.se/scuttle/tuiplay/internal/lyrics"
	"git.hemmalab.se/scuttle/tuiplay/internal/player"
	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"
	"git.hemmalab.se/scuttle/tuiplay/internal/version"

	tea "github.com/charmbracelet/bubbletea"
)

// paneFocus names the pane that the keys act on.
type paneFocus int

const (
	focusQueue paneFocus = iota
	focusRight
)

// volumeStep is the volume change per key press, in percent.
const volumeStep = 5

// defaultSplit is the default fraction of the width for the queue pane.
const defaultSplit = 0.6

// UIState is the persisted interface state that Run reads and returns.
type UIState struct {
	// RightShown is true when the navigation panel is visible.
	RightShown bool
	SplitRatio float64
}

// Settings holds the tunable values from the config.
type Settings struct {
	// SeekSeconds is the seek step. A value of zero means 10.
	SeekSeconds int
	// CrossfadeSeconds is the crossfade length. A value of zero means 4.
	CrossfadeSeconds int
	// RadioQueueSize is the target queue size for radio (feeder) mode. A
	// value of zero means 10.
	RadioQueueSize int
	// NSPPath is the directory for smart-playlist files. An empty value
	// hides the smart-playlist feature.
	NSPPath string
	// LrclibDBPath is the path to an lrclib SQLite dump. An empty value
	// turns off the dump tier of the lyrics lookup.
	LrclibDBPath string
	// LyricsCacheDir is the directory where tuiplay caches lyrics. An
	// empty value turns off the cache tier.
	LyricsCacheDir string
	// LyricsMissRecheckDays is how many days a cached miss stays valid. A
	// zero value means a miss never goes stale.
	LyricsMissRecheckDays int
	// LyricsProviders is the ordered list of network lyrics providers.
	// Known names are "lrcmux" and "lrclib". An empty list turns off the
	// network tier.
	LyricsProviders []string
	// ControlSocket is the unix socket path for external control. An empty
	// value turns the feature off.
	ControlSocket string
	// Theme is the resolved color theme. The caller resolves it from the
	// config before it calls Run.
	Theme Theme
	// Visualizer holds all visualizer tunables, already
	// resolved to their effective values.
	Visualizer VisualizerSettings
}

// VisualizerSettings holds the effective visualizer tunables.
type VisualizerSettings struct {
	// SparkLife is how many frames a spark lives.
	SparkLife int
	// SparkSpeed scales the launch speed.
	SparkSpeed float64
	// SparkGravity is the downward pull in sub-cells per frame squared.
	SparkGravity float64
	// SparkCount scales how many sparks a beat spawns.
	SparkCount float64
	// SparkTrail is the number of trailing positions per spark.
	SparkTrail int
	// RadialDecay is the per-frame retained fraction of the radial bloom
	// band levels when the audio quiets. Higher falls slower.
	RadialDecay float64
	// RadialReach scales how far the bloom rays extend.
	RadialReach float64
	// RadialCore scales the pulsing inner core radius.
	RadialCore float64
	// HueSpeed scales the palette hue drift across all modes.
	HueSpeed float64
	// BeatSensitivity scales how easily a beat triggers.
	BeatSensitivity float64
	// SpectrumSmoothing is the spectrum bar falloff fraction.
	SpectrumSmoothing float64
	// SpectrumPeakGravity is the spectrum peak-cap fall acceleration.
	SpectrumPeakGravity float64
	// SpectrumTilt lifts the higher spectrum bars.
	SpectrumTilt float64
	// SpectrumMonstercat is the neighbor-bleed strength of the spectrum.
	SpectrumMonstercat float64
	// WaveFalloff is the waveform envelope decay fraction.
	WaveFalloff float64
	// StereoSmoothing is the stereo spectrum bar falloff fraction.
	StereoSmoothing float64
}

// Run starts the interface and blocks until the user quits. It reads the
// persisted UI state, the settings, and the resolved key map. It returns
// the state to save on exit.
func Run(cl *subsonic.Client, pl *player.Player, state UIState, set Settings, keys map[string]string) (UIState, error) {
	// Query the server's OpenSubsonic extensions. A failure is soft; the
	// dependent features stay off.
	_ = cl.LoadExtensions()
	queueSupported := cl.HasExtension("indexBasedQueue")

	// Open the lrclib lyrics dump when configured. A failure is soft; the
	// dump tier stays off.
	var lyricsDump *lyrics.DB
	if set.LrclibDBPath != "" {
		if db, err := lyrics.Open(set.LrclibDBPath); err == nil {
			lyricsDump = db
			defer db.Close()
		} else {
			fmt.Fprintln(os.Stderr, "warning: could not open lrclib database:", err)
		}
	}

	// Build the tiered lyrics source: the local cache, the dump, and the
	// ordered network providers. Each tier is optional.
	cache := lyrics.NewCache(set.LyricsCacheDir, set.LyricsMissRecheckDays)
	userAgent := fmt.Sprintf("tuiplay v%s (https://git.hemmalab.se/scuttle/tuiplay)", version.Version)
	var providers []lyrics.Fetcher
	for _, name := range set.LyricsProviders {
		switch name {
		case "lrcmux":
			providers = append(providers, lyrics.NewLrcmuxClient(userAgent))
		case "lrclib":
			providers = append(providers, lyrics.NewLrclibNetClient(userAgent))
		}
	}
	lyricsSrc := lyrics.NewSource(cache, lyricsDump, providers)

	m := newModel(cl, pl, state, set, keys, lyricsSrc, queueSupported)
	shared := m.xfShared
	prog := tea.NewProgram(m, tea.WithAltScreen())
	pl.SetOnEnd(func() {
		prog.Send(trackEndedMsg{})
	})

	// Start the external control listener when a socket path is set. A
	// failure to listen is soft: the interface still runs.
	if set.ControlSocket != "" {
		if ln, err := listenControl(set.ControlSocket); err == nil {
			go serveControl(ln, prog)
			defer func() {
				ln.Close()
				os.Remove(set.ControlSocket)
			}()
		} else {
			fmt.Fprintln(os.Stderr, "warning: could not open control socket:", err)
		}
	}
	// The provider picks the next track for a natural crossfade. It reads
	// the shared snapshot, so it does not touch the Bubble Tea model.
	pl.SetNextProvider(func() (string, string, bool) {
		shared.mu.Lock()
		defer shared.mu.Unlock()
		// Consume mode changes the queue on advance. Let the normal
		// advance path handle it, not the crossfade pre-load.
		if shared.consume {
			shared.pending = -1
			return "", "", false
		}
		idx, stop := nextIndexPure(len(shared.queue), shared.current, shared.repeat, shared.shuffle, false)
		if stop || idx < 0 || idx >= len(shared.queue) {
			shared.pending = -1
			return "", "", false
		}
		s := shared.queue[idx]
		u, suffix, err := streamSource(cl, s)
		if err != nil {
			shared.pending = -1
			return "", "", false
		}
		shared.pending = idx
		return u, suffix, true
	})
	out, err := prog.Run()
	if err != nil {
		return state, err
	}
	final, ok := out.(model)
	if !ok {
		return state, nil
	}

	// Save the play queue on the server when the extension is supported.
	// The current-track position is the elapsed time of the current track.
	if queueSupported {
		ids := make([]string, 0, len(final.queue))
		for _, s := range final.queue {
			ids = append(ids, s.ID)
		}
		elapsed, _ := pl.Position()
		if serr := cl.SavePlayQueueByIndex(ids, final.queueIndex, elapsed.Milliseconds()); serr != nil {
			fmt.Fprintln(os.Stderr, "warning: could not save play queue:", serr)
		}
	}

	return final.uiState(), nil
}

// listenControl opens the control unix socket. It removes any stale socket
// file first, then listens. The file mode is 0600 so only the user may
// connect.
func listenControl(path string) (net.Listener, error) {
	// Remove a stale socket left by a previous run.
	if _, err := os.Stat(path); err == nil {
		os.Remove(path)
	}
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	// Restrict the socket to the owning user.
	_ = os.Chmod(path, 0600)
	return ln, nil
}

// serveControl accepts control connections and forwards each command to the
// interface. It reads one command line per connection, sends a controlMsg
// into the Bubble Tea program, waits for the one-line reply, and writes it
// back. It returns when the listener closes.
func serveControl(ln net.Listener, prog *tea.Program) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // the listener closed on exit
		}
		go handleControlConn(conn, prog)
	}
}

// handleControlConn serves one control connection.
func handleControlConn(conn net.Conn, prog *tea.Program) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(5 * time.Second))

	line, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil && line == "" {
		return
	}
	cmd := control.Parse(line)
	if cmd.Name == "" {
		fmt.Fprintln(conn, "err: empty command")
		return
	}

	reply := make(chan string, 1)
	prog.Send(controlMsg{name: cmd.Name, arg: cmd.Arg, reply: reply})

	select {
	case r := <-reply:
		fmt.Fprintln(conn, r)
	case <-time.After(3 * time.Second):
		fmt.Fprintln(conn, "err: timeout")
	}
}
