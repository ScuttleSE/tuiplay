# tuiplay

> **Built with AI.** This project was written largely by an AI agent. It comes
> with **no guarantees** of any kind: no promise that it works, is correct, is
> secure, or is maintained. Use it at your own risk. The code is released into
> the public domain (see [LICENSE](LICENSE)); the author claims no ownership.

A terminal music player that streams from a [Navidrome](https://www.navidrome.org/) server. The look and feel take inspiration from [ncmpcpp](https://github.com/ncmpcpp/ncmpcpp).

`tuiplay` talks to Navidrome through the Subsonic / OpenSubsonic API. It browses your library by artist and album, builds a play queue, and plays audio directly through your system's sound device.

## Features

- Browse the library by artist, album, genre, release year, and track (organized by ID3 tags).
- Search the library with an ncmpcpp-style multi-field form.
- Build and play a queue of tracks, with consume, repeat, random, and crossfade modes.
- Radio (feeder) mode: a bounded, self-refilling queue drawn from a playlist or a search result.
- Playback controls: play/pause, stop, next, previous, seek, and volume.
- Playlists: play, save, overwrite, delete, and build or edit Navidrome smart playlists (`.nsp`).
- Rate songs 0-5 (saved on the server), with a thumbs-down marker for a dislike.
- Synchronized lyrics that follow the playing song, from a local cache, an lrclib dump, or network providers.
- Album cover art in the right pane, rendered in three text-graphics modes (half-block, braille, blocks).
- A fullscreen audio visualizer with five modes: spectrum, waveform, stereo spectrum, ellipse, and Lorenz attractor.
- Color themes, including several Catppuccin variants, with per-role color overrides.
- External control over a unix socket, so a streamdeck or a script can drive playback.
- A status bar showing the current track, the playback state, the progress, and the mode flags.
- Scrobble now-playing notifications back to Navidrome.
- Plays MP3, FLAC, Ogg Vorbis, and AAC-LC (in an m4a/mp4 container) directly; other formats (HE-AAC, ALAC) play through server-side transcoding.
- ncmpcpp-style key bindings and layout. Every key is rebindable.

## Requirements

To **run** the player you need a working audio setup:

- Linux with ALSA, or PipeWire with its ALSA compatibility layer (`pipewire-alsa`).

To **build** the player from source you additionally need:

- Go 1.24 or newer.
- `pkg-config` and the ALSA development headers (`libasound2-dev` on Debian/Ubuntu). These are needed by the audio backend at compile time only.

## Installation

### Prebuilt binaries

Each release attaches Linux binaries for `amd64` and `arm64` on the
[GitHub Releases page](https://github.com/ScuttleSE/tuiplay/releases).
Download the one for your architecture, make it executable, and run it:

```sh
chmod +x tuiplay-*-linux-amd64
./tuiplay-*-linux-amd64
```

The binaries are dynamically linked against ALSA, so you need the ALSA
runtime library installed (`libasound2` on Debian/Ubuntu, `alsa-lib` on
Arch). They are built on Debian 11 (glibc 2.31), so they run on that and
newer distributions.

### From source

Build it from source:

```sh
git clone https://github.com/ScuttleSE/tuiplay.git
cd tuiplay
go build -o tuiplay ./cmd/tuiplay
```

## Configuration

`tuiplay` reads a TOML configuration file. By default it looks for the file at:

```
~/.config/tuiplay/config.toml
```

You can point to a different file with the `--config` flag.

An example configuration:

```toml
# The base URL of your Navidrome server.
server_url = "https://navidrome.example.com"

# Your Navidrome account.
username = "yourname"
password = "yourpassword"

# Optional: limit the stream bit rate in kilobits per second.
# A value of 0 (the default) means no limit.
max_bit_rate = 0

# Optional: the seek step in seconds for the seek keys. Default is 10.
seek_seconds = 10

# Optional: the crossfade length in seconds. Default is 4.
crossfade_seconds = 4

# Optional: the target queue size for radio (feeder) mode. Default is 10.
radio_queue_size = 10

# Optional: the color theme. Ships: default, catppuccin-latte,
# catppuccin-frappe, catppuccin-macchiato, catppuccin-mocha. Empty or omitted
# means the classic ncmpcpp "default" look.
# theme = "catppuccin-mocha"

# Optional: the directory where tuiplay writes smart-playlist files (.nsp).
# Point it at a directory Navidrome scans. Empty hides the feature.
# nsp_path = "/srv/navidrome/playlists"

# Optional: the path to an lrclib SQLite database dump for offline lyrics.
# Download one from https://lrclib.net/db-dumps. Empty turns off this tier.
# lrclib_db_path = "/srv/lrclib/db.sqlite3"

# Optional: the directory where tuiplay caches lyrics. Empty uses the default.
# lyrics_cache_path = "/home/you/.config/tuiplay/lyrics"

# Optional: how many days a cached "no lyrics" result stays valid. Default 7.
# lyrics_miss_recheck_days = 7

# Optional: ordered list of network lyrics providers. Default ["lrcmux", "lrclib"].
# Set [] to turn the network tier off.
# lyrics_providers = ["lrcmux", "lrclib"]

# Optional: a unix socket path for external control (see "External control").
# Empty turns the feature off. A leading ~ expands to your home directory.
# control_socket = "~/.config/tuiplay/control.sock"

# Optional: override individual theme colors. Each key is a role (for example
# album or progress) and each value is a #rrggbb hex color. These apply on top
# of the named theme. A table must come after all the scalar keys above.
# [theme_colors]
# album = "#89b4fa"
# progress = "#a6e3a1"
```

The password never leaves your machine as clear text. The client sends a salted MD5 token to the server on every request, as the Subsonic API specifies.

On the first run tuiplay writes a full template config (with every optional key documented) to the default path and exits, so you can edit it and run again.

Keep this file private. It contains your password. The file must never be committed to a git repository.

## Usage

```sh
tuiplay              # use the default config path
tuiplay --config /path/to/config.toml
tuiplay --version    # print the version and exit
tuiplay ctl <command> [args]   # send a control command to a running instance
```

### Layout

The screen has two panes side by side:

- The **left pane is always the play queue**. It shows the queued tracks in
  four columns: Artist, Track + Title, Album, and Time. The current track is
  shown in bold. The selected row shows a colored bar. A rating shows as
  filled stars before the title, or a thumbs-down glyph for a dislike.
- The **right pane holds four parallel views**: the browse navigation, the
  search form, the lyrics of the playing song, and the album cover art. The
  `2`, `3`, `4`, and `6` keys switch between them. Each view keeps its own
  state and position, so switching away and back does not reset it.

You can hide the right pane so the queue fills the whole width.

A header line at the top shows the queue summary and the volume. A green
progress bar and a status bar sit at the bottom. The status bar shows the
mode flags at the right, like `[Rx]` (`R` repeat, `c` consume, `r` random,
`x` crossfade, `F` radio, `t` transcoding), and a spinner while the server
scans the library.

The right pane remembers whether it was shown and reopens in that state on
the next run.

### Navigation panel

Press `2` to show the browse view. It opens at the entrypoints:

- **Artists** → all artists → an artist's albums → an album's tracks.
- **Albums** → all albums (alphabetical) → an album's tracks.
- **Genres** → all genres → a genre's albums → an album's tracks.
- **Release Years** → all years (newest first) → that year's albums → tracks.
- **Tracks** → a broad list of tracks.
- **Playlists** → your saved playlists → Enter queues the whole playlist.

`Enter` drills down. The `[..]` row at the top of every level goes back up
one level. `Enter` on a track plays it now. `a` adds a track to the queue.
`Space` adds everything under the selected item (a track, an album, an
artist, a genre, a year, or a playlist) to the bottom of the queue.

### Search

Press `3` to show the search form. It has tag fields (Any, Artist, Title,
Album, Genre, Date, Comment, Filename) styled like the ncmpcpp search
screen. `Enter` on a field edits its value. `Enter` on `Search` runs the
query and shows the matching tracks; `Enter` on `Reset` clears the fields.
The server does the free-text search; tuiplay then filters the results so
each filled field matches its own tag.

### Lyrics

Press `4` to show synchronized lyrics for the playing song. tuiplay looks
the song up through a chain of tiers: a local per-song cache, then a local
lrclib SQLite dump (if `lrclib_db_path` is set), then the network providers
in `lyrics_providers` order. For timed lyrics the active line is highlighted
and the view scrolls to follow playback. The page title carries a note
glyph: `♬` for synced, per-line timed lyrics and `♪` for plain, untimed
lyrics. The view follows the song as it changes.

Press `/` while the lyrics view is open to search for lyrics by freetext.
The query matches the start of a title or artist in the local dump and
against lrclib.net, so it also finds songs that are not playing. `Enter` on
a hit shows its lyrics; while a song plays, they are saved to the cache for
that song, so the automatic lookup finds them from then on. `[..]` returns
to the playing song's lyrics.

If the synced lyrics run slightly ahead of or behind the music, press `,`
to shift the lines earlier and `.` to shift them later, in 0.1-second steps
(up to ±10 s). The offset shows in the page header and is saved per song,
so it comes back with the song.

### Cover art

Press `6` to show the album cover of the playing song in the right pane. The
image is downloaded from the server, decoded, and drawn as colored terminal
text. While the cover view is focused, `Space` cycles three render modes:

- **half-block** — two full-color pixels per cell using the `▀` glyph; the
  best look for a color cover.
- **braille** — 2×4 dots per cell for a high-resolution, near-monochrome look.
- **blocks** — shade glyphs picked by brightness with a per-cell color.

`Tab` still moves focus back to the queue. The view follows the song as it
changes. When nothing plays it shows "No song playing"; a song with no cover
art shows "No cover art".

### Radio (feeder) mode

Radio mode keeps a bounded queue that refills itself, so it never empties.
Focus a playlist row (on the Playlists level) or a Tracks list (a Tracks
entrypoint or a search result) and press `V` to turn it on; press `V` again
to turn it off. `v` sets the target queue size at runtime (the default comes
from `radio_queue_size`, 10).

Turning it on snapshots the source songs into a pool, forces consume on and
repeat off, and fills the queue with that many random songs. It refills after
each consumed song and on each one-second tick, and avoids repeating the same
song back to back. A manual add past the target size pauses the auto-add until
the queue drains below the size again. The status bar shows an `F` flag while
radio mode is on.

### Smart playlists

When `nsp_path` points at a directory Navidrome scans, the Playlists level
shows a `[New smart playlist]` row. It opens a guided builder for a name,
one or more conditions (a field, an operator, and a value), an optional
sort field, order, and limit. `Save` writes a Navidrome `.nsp` file and
triggers an incremental scan so Navidrome imports it. The condition fields
include title, album, artist, genre, year, loved, rating, playcount,
lastplayed, dateadded, and filepath. Press `e` on a playlist row to reopen
the builder loaded with that playlist's `.nsp` file and edit it. Deleting such
a playlist also removes its `.nsp` file so it does not reappear on the next
scan.

### Ratings

`§` raises the rating of the selected or current song by one, up to 5. `½`
lowers it by one, down to 0. Ratings are saved on the Navidrome server. A
rating of 0 is unrated; a rating of 1 marks a dislike and shows as a
thumbs-down glyph in the queue. Higher ratings show as filled stars.

### Visualizer

`5` opens a fullscreen audio visualizer. `Tab` or `Space` cycles five modes:
a frequency spectrum, a time-domain waveform, a stereo mirror spectrum, a
pulsing ellipse, and a Lorenz attractor. `Esc` or `5` closes it. The spectrum
and waveform use a green-yellow-red height gradient; the ellipse and Lorenz
use a rainbow hue wheel. All colors adapt to your terminal's color support.
Playback keys still work while it is open.

### Themes

The `theme` config key picks a color theme. tuiplay ships `default` (the
classic ncmpcpp look), plus `catppuccin-latte`, `catppuccin-frappe`,
`catppuccin-macchiato`, and `catppuccin-mocha`. An empty or omitted value uses
`default`. The optional `[theme_colors]` table overrides single roles (for
example `album` or `progress`) with `#rrggbb` hex colors on top of the named
theme. An unknown theme name, an unknown role, or a malformed hex value is a
config error. The fullscreen visualizer keeps its own colors and is not
affected by the theme.

### Key bindings

The bindings follow ncmpcpp defaults where practical. Every action is
rebindable in the config; see the `[keys]` section below.

| Key            | Action                                   |
|----------------|------------------------------------------|
| `1`            | Hide the right pane; the queue fills the screen |
| `2`            | Show the browse navigation view          |
| `3`            | Show the search form                     |
| `4`            | Show the lyrics of the playing song      |
| `/`            | In the lyrics view: search lyrics by freetext |
| `,` / `.`      | In the lyrics view: shift synced lyric timing earlier / later |
| `5`            | Open the fullscreen visualizer           |
| `6`            | Show the album cover of the playing song |
| `Tab`          | Move focus between the queue pane and the right pane |
| `j` / `Down`   | Move the cursor down                     |
| `k` / `Up`     | Move the cursor up                       |
| `Home`         | Jump to the top of the list              |
| `End`          | Jump to the bottom of the list           |
| `PgDn`         | Move down one screen                     |
| `PgUp`         | Move up one screen                       |
| `Enter`        | On a queue song: play it. On a navigation entry: drill down. On a track: play it now. On a playlist: replace the queue with it and play. On the `[..]` row: go back one level. |
| `Delete`       | Remove the selected song from the queue, or delete the selected playlist (asks to confirm) |
| `a`            | Add the selected track to the bottom of the queue |
| `Space`        | Add everything under the selected item to the bottom of the queue, then move down |
| `c`            | Clear the queue (asks to confirm)        |
| `i`            | Show detailed info for the selected song |
| `I`            | Show artist info (not implemented yet)   |
| `e`            | Edit the selected smart playlist (Playlists level) |
| `§`            | Raise the song rating by one (max 5)     |
| `½`            | Lower the song rating by one (min 0)     |
| `R`            | Toggle consume mode                      |
| `r`            | Toggle repeat mode                       |
| `z`            | Toggle random mode                       |
| `x`            | Toggle crossfade                         |
| `X`            | Set the crossfade length in seconds      |
| `V`            | Toggle radio (feeder) mode from the focused source |
| `v`            | Set the radio target queue size          |
| `S`            | Save the queue as a new playlist         |
| `Ctrl+s`       | Overwrite the source playlist, or save as new |
| `F1`           | Show the help card                       |
| `p`            | Pause or resume                          |
| `s`            | Stop                                     |
| `>`            | Next track                               |
| `<`            | Previous track (restart, then previous within two seconds) |
| `f`            | Seek forward (default 10 seconds)        |
| `b`            | Seek back (default 10 seconds)           |
| `+` / `=`      | Raise the volume by 5%                   |
| `-`            | Lower the volume by 5%                   |
| `q` / `Ctrl+C` | Quit                                     |

### Modes

- `R` consume: remove the current song when playback leaves it.
- `r` repeat: wrap the queue at the end.
- `z` random: pick a random next track on advance; the visible order stays.
- Repeat and consume are mutually exclusive. Random works together with
  either one; with consume, the next random song is picked from the songs
  that remain.
- `x` / `X` crossfade: `x` toggles it, `X` sets the length. When on, the
  player overlaps the outgoing and incoming tracks with an equal-power
  transition.
- `V` / `v` radio (feeder): `V` toggles a bounded, self-refilling queue drawn
  from the focused playlist or search result; `v` sets its target size. See
  the "Radio (feeder) mode" section above.

### Deleting and clearing

`Delete` removes the selected song from the queue. If it is the song that is
playing, playback continues until the song ends or you play another. `c`
clears the whole queue after a `y`/`n` confirmation in the status bar.

### Song and artist info

`i` shows detailed information for the selected song (title, artist, album,
year, track, genre, format, bit rate, path). `I` is reserved for artist
info; it is not implemented yet.

### Seek amount

`f` and `b` seek by `seek_seconds` from the config (default 10).

### Saving a playlist

Press `S` to save the current queue as a new playlist; a prompt asks for a
name. Press `Ctrl+s` to overwrite the playlist the queue came from; if the
queue did not come from a playlist, `Ctrl+s` acts like `S`.

### Rebindable keys

The config file has an optional `[keys]` section that maps an action name to
a key. An unset action uses its default. Two actions must not share a key,
and an unknown action name is an error. See `config.example.toml` for the
full list of actions and their defaults.

### Consume mode

When consume mode is on, the current song leaves the queue as soon as
playback moves off it: when it ends, when you skip to the next track, or when
you press Enter on another song. The queue keeps its order otherwise.

### Volume

The volume is the player's own output level, shown as a percentage from 0 to
100. It does not change the system mixer, so it does not affect other
programs.

## External control

tuiplay can accept commands from another program, such as a streamdeck
button or a script. Set `control_socket` in the config to a unix socket
path to turn the feature on:

```toml
control_socket = "~/.config/tuiplay/control.sock"
```

While tuiplay runs, send a command with the `ctl` subcommand. It reads the
socket path from the same config, so a streamdeck button can simply run:

```sh
tuiplay ctl next
tuiplay ctl pause
tuiplay ctl playlist "Focus Mix"
tuiplay ctl volume +
tuiplay ctl seek -10
```

The commands are:

| Command                 | Effect                                            |
|-------------------------|---------------------------------------------------|
| `play`, `pause`, `playpause`, `toggle` | Toggle pause                       |
| `stop`                  | Stop playback                                     |
| `next`                  | Advance to the next track                         |
| `prev` (or `previous`)  | Restart the song, or go to the previous one       |
| `seek <seconds>`        | Seek by a signed number of seconds, e.g. `seek -10` |
| `volume <+\|-\|N>` (or `vol`) | Raise, lower, or set the volume percent      |
| `playlist <name>`       | Replace the queue with a named playlist and play it |

The playlist match is case-insensitive: an exact name wins, otherwise the
first prefix match. With random mode on, playback starts on a random track.

The socket is created with owner-only permissions (mode 0600), so only your
user can connect. An optional `--config <path>` before the command overrides
the config path. On an error the subcommand prints the reason and exits with
a non-zero status.

## How it works

- The library data comes from the Subsonic / OpenSubsonic API. tuiplay browses by artist, album, genre, year, and playlist, searches with `search3`, and reads playlists, ratings, and (when supported) a server-side play queue.
- To play a track, the client downloads the stream from the `stream` endpoint to a temporary file, then decodes and plays it. The temporary file is removed when playback ends. A temporary file is used because audio decoding needs a seekable source, and a network response is not seekable.
- Supported audio formats are MP3, FLAC, Ogg Vorbis, and AAC-LC in an m4a/mp4 container. The AAC decoder is pure Go (go-m4a demuxes the container, go-aac decodes the audio), so it needs no extra codec on your machine. For a source the player cannot decode natively (HE-AAC, ALAC, or any other unsupported format), tuiplay asks Navidrome to transcode it to MP3 on the fly. While a track plays through a transcode, the status bar shows a `t` flag next to the other mode flags.
- When the server supports the `indexBasedQueue` OpenSubsonic extension, tuiplay saves your play queue on exit and restores it on the next run, paused at the saved position.

## Version

The version format is `<major>.<minor>.<build>`. It starts at `0.0.0`. Each commit increases the build number by one.

## License

This project is released into the public domain under [The Unlicense](LICENSE).
The author claims no ownership and provides no warranty of any kind.

Note: tuiplay depends on third-party libraries that keep their own licenses
(for example, `go-aac` is LGPL-2.1-or-later). The Unlicense applies to this
project's own code, not to its dependencies.
