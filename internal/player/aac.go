// AAC decoding for MP4/M4A containers.
//
// Navidrome serves AAC inside an MP4/M4A container, not as a raw ADTS
// stream. go-m4a demuxes the container to access units. go-aac decodes the
// AAC-LC access units to interleaved little-endian S16 PCM. The pure-Go pair
// adds no C dependency beyond the existing ALSA cgo.
//
// go-aac decodes AAC-LC mono and stereo at 44.1 and 48 kHz only. It does not
// decode HE-AAC (SBR/PS) or ALAC. The container reader rejects those with an
// unsupported error. A caller catches ErrTranscodeNeeded and falls back to
// server-side transcoding.
//
// The go-aac decoder is forward only. It has no seek. The player needs seek
// for the progress bar, the f and b keys, and crossfade timing. So the
// decoder reads the whole track into an in-memory beep buffer. The buffer's
// streamer is a real beep.StreamSeekCloser with working Seek, Len, and
// Position.
package player

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gopxl/beep/v2"
	"github.com/tphakala/go-m4a/aacm4a"
)

// ErrTranscodeNeeded reports that the player cannot decode the source
// natively, so the caller should retry with a server-side transcode. It wraps
// the underlying decode error. The AAC path returns it for HE-AAC, ALAC, a
// corrupt stream, or a track larger than the in-memory decode ceiling.
var ErrTranscodeNeeded = errors.New("native decode failed, transcode needed")

// maxDecodedSamples caps the in-memory decode. The AAC decoder streams, so a
// pathological or very long file could otherwise exhaust memory. The ceiling
// is about 150 minutes of 44.1 kHz stereo, which is a whole double album and
// more than any single track. A track above it falls back to a server
// transcode. Each sample frame is one [2]float64, 16 bytes, so the ceiling is
// about 6 GiB of frames only in the pathological case; a normal track is far
// below it.
const maxDecodedSamples = 150 * 60 * 44100

// decodeAAC decodes an MP4/M4A AAC-LC file into an in-memory, seekable beep
// stream. It reads the whole track, so it returns a beep.StreamSeekCloser
// backed by a buffer. It closes f. A source it cannot decode returns an error
// wrapping ErrTranscodeNeeded.
func decodeAAC(f *os.File) (beep.StreamSeekCloser, beep.Format, error) {
	defer f.Close()

	dec, info, err := aacm4a.NewDecoder(f)
	if err != nil {
		return nil, beep.Format{}, fmt.Errorf("%w: open m4a: %w", ErrTranscodeNeeded, err)
	}

	channels := info.Channels
	if channels != 1 && channels != 2 {
		return nil, beep.Format{}, fmt.Errorf("%w: unsupported channel count %d", ErrTranscodeNeeded, channels)
	}

	format := beep.Format{
		SampleRate:  beep.SampleRate(info.SampleRate),
		NumChannels: 2,
		Precision:   2,
	}

	// The decoder emits every sample, including the leading encoder priming
	// and the trailing final-frame padding. Skip the priming, then keep only
	// the presentation duration. Both counts are per channel.
	skipFrames := int(info.EncoderDelay)
	keepFrames := int(info.Duration.Seconds() * float64(info.SampleRate))

	buf := beep.NewBuffer(format)
	frames := make([][2]float64, 0, 4096)

	// One PCM read chunk holds an integer number of sample frames.
	const chunkFrames = 4096
	raw := make([]byte, chunkFrames*channels*2)

	var produced int // sample frames appended to the buffer
	var decoded int  // sample frames read from the decoder (before trim)
	var carry []byte // leftover bytes that do not complete a frame

	frameBytes := channels * 2

	for {
		n, rerr := dec.Read(raw)
		if n > 0 {
			data := raw[:n]
			if len(carry) > 0 {
				data = append(carry, data...)
				carry = nil
			}
			full := (len(data) / frameBytes) * frameBytes
			rem := data[full:]
			data = data[:full]

			frames = frames[:0]
			for i := 0; i < len(data); i += frameBytes {
				var l, r float64
				if channels == 1 {
					s := int16(uint16(data[i]) | uint16(data[i+1])<<8)
					l = float64(s) / 32768
					r = l
				} else {
					sl := int16(uint16(data[i]) | uint16(data[i+1])<<8)
					sr := int16(uint16(data[i+2]) | uint16(data[i+3])<<8)
					l = float64(sl) / 32768
					r = float64(sr) / 32768
				}

				idx := decoded
				decoded++
				// Trim the priming and the trailing padding.
				if idx < skipFrames {
					continue
				}
				if keepFrames > 0 && produced >= keepFrames {
					continue
				}
				frames = append(frames, [2]float64{l, r})
				produced++
				if produced > maxDecodedSamples {
					return nil, beep.Format{}, fmt.Errorf("%w: track exceeds decode ceiling", ErrTranscodeNeeded)
				}
			}
			if len(frames) > 0 {
				buf.Append(beep.Streamer(&sliceStreamer{frames: frames}))
			}
			if len(rem) > 0 {
				carry = append(carry[:0], rem...)
			}
		}
		if rerr != nil {
			if errors.Is(rerr, io.EOF) {
				break
			}
			return nil, beep.Format{}, fmt.Errorf("%w: decode m4a: %w", ErrTranscodeNeeded, rerr)
		}
	}

	if buf.Len() == 0 {
		return nil, beep.Format{}, fmt.Errorf("%w: empty decode", ErrTranscodeNeeded)
	}

	return &bufferStreamer{StreamSeeker: buf.Streamer(0, buf.Len())}, format, nil
}

// sliceStreamer streams one slice of sample frames once. beep.Buffer.Append
// consumes a beep.Streamer, so this feeds the buffer from decoded frames.
type sliceStreamer struct {
	frames [][2]float64
	pos    int
}

func (s *sliceStreamer) Stream(samples [][2]float64) (int, bool) {
	if s.pos >= len(s.frames) {
		return 0, false
	}
	n := copy(samples, s.frames[s.pos:])
	s.pos += n
	return n, true
}

func (s *sliceStreamer) Err() error { return nil }

// bufferStreamer adds a no-op Close to a beep.Buffer streamer, so it satisfies
// beep.StreamSeekCloser. The buffer holds the whole track in memory; there is
// no file or network handle to close.
type bufferStreamer struct {
	beep.StreamSeeker
}

func (b *bufferStreamer) Close() error { return nil }
