// Package player plays audio streams from the Navidrome server.
//
// The player downloads each stream to a temporary file first. A network
// body does not support seek. A file supports seek. The decoder needs
// seek for the format that beep reads. The player removes the temporary
// file when its track drains or stops.
//
// The player feeds one persistent mixer to the speaker. It plays one
// track at a time, or two tracks at once during a crossfade. The caller
// manages the queue.
package player

import (
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/gopxl/beep/v2"
	"github.com/gopxl/beep/v2/effects"
	"github.com/gopxl/beep/v2/flac"
	"github.com/gopxl/beep/v2/mp3"
	"github.com/gopxl/beep/v2/speaker"
	"github.com/gopxl/beep/v2/vorbis"
)

// sampleRate is the output sample rate of the speaker.
const sampleRate beep.SampleRate = 44100

// State describes the playback state.
type State int

const (
	// StateStopped means no track is loaded.
	StateStopped State = iota
	// StatePlaying means a track plays now.
	StatePlaying
	// StatePaused means a track is loaded but on hold.
	StatePaused
)

// track holds one playing song.
type track struct {
	streamer beep.StreamSeekCloser // the raw decoded stream (for seek/len)
	ctrl     *beep.Ctrl            // pause control for this track
	fade     *effects.Volume       // per-track fade gain (unity when no fade)
	format   beep.Format
	tempFile string
}

// close stops the track and removes its temporary file.
func (t *track) close() {
	if t == nil {
		return
	}
	if t.streamer != nil {
		t.streamer.Close()
		t.streamer = nil
	}
	if t.tempFile != "" {
		os.Remove(t.tempFile)
		t.tempFile = ""
	}
}

// Player controls audio playback through a persistent mixer.
type Player struct {
	mu     sync.Mutex
	state  State
	mixer  *beep.Mixer
	master *effects.Volume // master volume on the mixer output
	tap    *tap            // sample tap on the master output, for the visualizer

	current *track
	// fading is true while a crossfade is in progress.
	fading bool

	onEnd    func()
	provider func() (url, suffix string, ok bool)

	// volPercent is the output volume as a percent, 0..100. 100 is unity.
	volPercent int

	// crossfadeSec is the crossfade length in seconds. 0 disables it.
	crossfadeSec int

	// monitorStop stops the end monitor goroutine.
	monitorStop chan struct{}
}

// New makes a player and starts the speaker with a persistent mixer.
func New() (*Player, error) {
	if err := speaker.Init(sampleRate, sampleRate.N(time.Second/10)); err != nil {
		return nil, fmt.Errorf("speaker init: %w", err)
	}
	mixer := &beep.Mixer{}
	mixer.KeepAlive(true)
	master := &effects.Volume{Streamer: mixer, Base: 2, Volume: 0}
	tp := newTap(master)

	p := &Player{
		state:       StateStopped,
		mixer:       mixer,
		master:      master,
		tap:         tp,
		volPercent:  100,
		monitorStop: make(chan struct{}),
	}
	speaker.Play(tp)
	go p.monitor()
	return p, nil
}

// Samples copies the most recent output samples into dst for a
// visualizer. The samples are a mono mix in the range -1 to 1 at the
// output sample rate. It returns the number of samples written. The oldest
// sample is first.
func (p *Player) Samples(dst []float64) int {
	return p.tap.Samples(dst)
}

// SamplesStereo copies the most recent output frames into dst for a
// visualizer. Each frame holds a left and a right sample in the range -1
// to 1 at the output sample rate. It returns the number of frames written.
// The oldest frame is first.
func (p *Player) SamplesStereo(dst [][2]float64) int {
	return p.tap.SamplesStereo(dst)
}

// SampleRate returns the output sample rate in Hz.
func (p *Player) SampleRate() int {
	return int(sampleRate)
}

// SetOnEnd sets a callback that runs when a track reaches its end.
func (p *Player) SetOnEnd(fn func()) {
	p.mu.Lock()
	p.onEnd = fn
	p.mu.Unlock()
}

// SetNextProvider sets a callback that returns the next stream to play.
// The player calls it to pre-load the next track for a crossfade.
func (p *Player) SetNextProvider(fn func() (url, suffix string, ok bool)) {
	p.mu.Lock()
	p.provider = fn
	p.mu.Unlock()
}

// SetCrossfade sets the crossfade length in seconds. Zero disables it.
func (p *Player) SetCrossfade(seconds int) {
	p.mu.Lock()
	if seconds < 0 {
		seconds = 0
	}
	p.crossfadeSec = seconds
	p.mu.Unlock()
}

// loadTrack downloads and decodes a stream into a new track at unity fade.
func (p *Player) loadTrack(url, suffix string) (*track, error) {
	tmp, err := download(url)
	if err != nil {
		return nil, err
	}
	f, err := os.Open(tmp)
	if err != nil {
		os.Remove(tmp)
		return nil, err
	}
	streamer, format, err := decode(f, suffix)
	if err != nil {
		f.Close()
		os.Remove(tmp)
		return nil, fmt.Errorf("decode stream: %w", err)
	}
	resampled := beep.Resample(4, format.SampleRate, sampleRate, streamer)
	fade := &effects.Volume{Streamer: resampled, Base: 2, Volume: 0}
	ctrl := &beep.Ctrl{Streamer: fade, Paused: false}
	return &track{
		streamer: streamer,
		ctrl:     ctrl,
		fade:     fade,
		format:   format,
		tempFile: tmp,
	}, nil
}

// Play stops any current track and starts a new one at full volume.
func (p *Player) Play(url, suffix string) error {
	t, err := p.loadTrack(url, suffix)
	if err != nil {
		return err
	}

	p.mu.Lock()
	old := p.current
	p.clearMixerLocked()
	p.current = t
	p.fading = false
	p.state = StatePlaying
	onEnd := p.onEnd
	p.mu.Unlock()

	old.close()

	speaker.Lock()
	p.mixer.Add(beep.Seq(t.ctrl, beep.Callback(func() {
		// Natural end with crossfade off: notify the queue.
		p.mu.Lock()
		fading := p.fading
		p.mu.Unlock()
		if !fading && onEnd != nil {
			onEnd()
		}
	})))
	speaker.Unlock()
	return nil
}

// PlayPausedAt loads a track, seeks to atSec seconds, and holds it paused.
// It restores a saved play queue without starting sound. The caller resumes
// with Resume or TogglePause.
func (p *Player) PlayPausedAt(url, suffix string, atSec int) error {
	if err := p.Play(url, suffix); err != nil {
		return err
	}
	if atSec > 0 {
		p.Seek(atSec)
	}
	p.Pause()
	return nil
}

// Crossfade fades from the current track into a new one over seconds. If
// seconds is zero or there is no current track, it behaves like Play.
func (p *Player) Crossfade(url, suffix string, seconds int) error {
	p.mu.Lock()
	hasCurrent := p.current != nil
	p.mu.Unlock()

	if seconds <= 0 || !hasCurrent {
		return p.Play(url, suffix)
	}

	in, err := p.loadTrack(url, suffix)
	if err != nil {
		return err
	}

	p.mu.Lock()
	out := p.current
	// Clamp the fade length to what the outgoing and incoming allow.
	n := sampleRate.N(time.Duration(seconds) * time.Second)
	if out != nil {
		remain := out.streamer.Len() - out.streamer.Position()
		if remain > 0 && remain < n {
			n = remain
		}
	}
	if in.streamer.Len() < n {
		n = in.streamer.Len()
	}
	if n < 1 {
		n = 1
	}

	// Wrap the incoming track's stream in a rising transition.
	inFade := effects.Transition(in.ctrl, n, 0.0, 1.0, effects.TransitionEqualPower)
	// Wrap the outgoing track in a falling transition.
	var outStream beep.Streamer
	if out != nil {
		outStream = effects.Transition(out.ctrl, n, 1.0, 0.0, effects.TransitionEqualPower)
	}

	p.current = in
	p.fading = true
	p.state = StatePlaying
	onEnd := p.onEnd
	p.mu.Unlock()

	speaker.Lock()
	// Rebuild the mixer with exactly the outgoing and incoming streams.
	// Each underlying streamer must appear once, or two mixer entries read
	// the same stream at once and the audio garbles.
	p.mixer.Clear()
	if outStream != nil {
		p.mixer.Add(beep.Seq(outStream, beep.Callback(func() {
			out.close()
		})))
	}
	p.mixer.Add(beep.Seq(inFade, beep.Callback(func() {
		p.mu.Lock()
		fading := p.fading
		p.mu.Unlock()
		if !fading && onEnd != nil {
			onEnd()
		}
	})))
	speaker.Unlock()

	// The fade window is short. Mark the fade done after it elapses so the
	// end monitor and the incoming callback resume normal behavior.
	go func() {
		time.Sleep(sampleRate.D(n))
		p.mu.Lock()
		p.fading = false
		p.mu.Unlock()
	}()
	return nil
}

// monitor watches the current track and triggers a crossfade near its end.
func (p *Player) monitor() {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-p.monitorStop:
			return
		case <-ticker.C:
			p.maybeCrossfade()
		}
	}
}

// maybeCrossfade starts a crossfade if the current track is near its end.
func (p *Player) maybeCrossfade() {
	p.mu.Lock()
	if p.state != StatePlaying || p.current == nil || p.fading || p.crossfadeSec <= 0 || p.provider == nil {
		p.mu.Unlock()
		return
	}
	cur := p.current
	speaker.Lock()
	remain := cur.streamer.Len() - cur.streamer.Position()
	speaker.Unlock()
	remainDur := cur.format.SampleRate.D(remain)
	if remainDur > time.Duration(p.crossfadeSec)*time.Second {
		p.mu.Unlock()
		return
	}
	provider := p.provider
	seconds := p.crossfadeSec
	// Set fading now so the tick does not fire twice.
	p.fading = true
	p.mu.Unlock()

	url, suffix, ok := provider()
	if !ok {
		p.mu.Lock()
		p.fading = false
		p.mu.Unlock()
		return
	}
	if err := p.Crossfade(url, suffix, seconds); err != nil {
		p.mu.Lock()
		p.fading = false
		p.mu.Unlock()
		return
	}
	// The queue must advance its own state after a natural crossfade.
	p.mu.Lock()
	onEnd := p.onEnd
	p.mu.Unlock()
	if onEnd != nil {
		onEnd()
	}
}

// clearMixerLocked clears the mixer. The caller holds p.mu.
func (p *Player) clearMixerLocked() {
	speaker.Lock()
	p.mixer.Clear()
	speaker.Unlock()
}

// download fetches url into a new temporary file and returns its path.
func download(url string) (string, error) {
	resp, err := http.Get(url)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("stream returned status %d", resp.StatusCode)
	}
	tmp, err := os.CreateTemp("", "tuiplay-*.audio")
	if err != nil {
		return "", err
	}
	if _, err := io.Copy(tmp, resp.Body); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return "", err
	}
	tmp.Close()
	return tmp.Name(), nil
}

// decode chooses a decoder by the file suffix.
func decode(f *os.File, suffix string) (beep.StreamSeekCloser, beep.Format, error) {
	switch strings.ToLower(suffix) {
	case "mp3":
		return mp3.Decode(f)
	case "flac":
		return flac.Decode(f)
	case "ogg", "oga", "vorbis":
		return vorbis.Decode(f)
	case "m4a", "m4b", "mp4", "aac":
		return decodeAAC(f)
	default:
		return mp3.Decode(f)
	}
}

// TranscodeFormat is the format the caller should ask the server to
// transcode an unsupported file to. The player decodes it natively.
const TranscodeFormat = "mp3"

// SupportedSuffix reports whether the player decodes the given file suffix
// natively. It decodes mp3, flac, ogg vorbis, and AAC-LC in an MP4/M4A
// container. It cannot decode HE-AAC or ALAC, which also carry an "m4a"
// suffix; a load of such a file fails with ErrTranscodeNeeded, and the caller
// then asks the server to transcode to TranscodeFormat. A suffix that is not
// listed here needs a server-side transcode up front.
func SupportedSuffix(suffix string) bool {
	switch strings.ToLower(suffix) {
	case "mp3", "flac", "ogg", "oga", "vorbis", "m4a", "m4b", "mp4", "aac":
		return true
	default:
		return false
	}
}

// Pause holds the current track.
func (p *Player) Pause() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return
	}
	speaker.Lock()
	p.current.ctrl.Paused = true
	speaker.Unlock()
	p.state = StatePaused
}

// Resume continues a held track.
func (p *Player) Resume() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return
	}
	speaker.Lock()
	p.current.ctrl.Paused = false
	speaker.Unlock()
	p.state = StatePlaying
}

// TogglePause switches between play and pause.
func (p *Player) TogglePause() {
	p.mu.Lock()
	s := p.state
	p.mu.Unlock()
	switch s {
	case StatePlaying:
		p.Pause()
	case StatePaused:
		p.Resume()
	}
}

// Stop ends playback and removes the temporary files.
func (p *Player) Stop() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		p.state = StateStopped
		return
	}
	speaker.Lock()
	p.mixer.Clear()
	speaker.Unlock()
	p.current.close()
	p.current = nil
	p.fading = false
	p.state = StateStopped
}

// Seek moves playback by delta seconds on the current track.
func (p *Player) Seek(delta int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return
	}
	speaker.Lock()
	defer speaker.Unlock()
	s := p.current.streamer
	cur := s.Position()
	target := cur + p.current.format.SampleRate.N(time.Duration(delta)*time.Second)
	if target < 0 {
		target = 0
	}
	if target >= s.Len() {
		target = s.Len() - 1
	}
	s.Seek(target)
}

// gainFor maps a volume percent (0..100) to a beep volume exponent and a
// silent flag. Base is 2, so a return of 0 is unity (100%).
func gainFor(percent int) (exponent float64, silent bool) {
	if percent <= 0 {
		return 0, true
	}
	if percent > 100 {
		percent = 100
	}
	return math.Log2(float64(percent) / 100.0), false
}

// SetVolumePercent sets the master output volume from 0 to 100.
func (p *Player) SetVolumePercent(percent int) {
	if percent < 0 {
		percent = 0
	}
	if percent > 100 {
		percent = 100
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.volPercent = percent
	gain, silent := gainFor(percent)
	speaker.Lock()
	p.master.Volume = gain
	p.master.Silent = silent
	speaker.Unlock()
}

// VolumePercent returns the master output volume from 0 to 100.
func (p *Player) VolumePercent() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.volPercent
}

// State returns the current playback state.
func (p *Player) State() State {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.state
}

// Position returns the elapsed time and the total time of the current
// track.
func (p *Player) Position() (elapsed, total time.Duration) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.current == nil {
		return 0, 0
	}
	speaker.Lock()
	defer speaker.Unlock()
	s := p.current.streamer
	elapsed = p.current.format.SampleRate.D(s.Position())
	total = p.current.format.SampleRate.D(s.Len())
	return elapsed, total
}
