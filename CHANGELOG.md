# Changelog

All notable changes to tuiplay are recorded here. The format groups changes
under Added, Changed, Fixed, and Removed.

## [Unreleased]

## [0.7.20] - 2026-08-24

### Added
- Album cover-art view. `6` (`show_cover`) shows the cover of the currently
  playing song in the right pane. It follows the playing song and prefetches
  the art in the background. The renderer has three modes, cycled with `Space`
  while the cover view is focused: half-block (full color), braille
  (high-resolution monochrome), and shaded blocks. It scales the image with a
  Catmull-Rom kernel and corrects for the terminal cell aspect so the art is
  not stretched.
- A completely reworked fullscreen visualizer. Every mode now shares a vivid,
  reactive palette (`vividHex`): the spectral position sets the hue (bass warm,
  treble cool), loudness sets the brightness, a drifting phase breathes the
  scene, and a detected beat flashes the whole screen. The spectrum draws with
  half-block glyphs for double vertical resolution, so a small window still
  reads smooth. Two new modes replace the old ellipse and Lorenz modes: a
  radial bloom that radiates the spectrum from a pulsing center, and beat
  sparks that burst particles from the center on each beat (with an optional
  fading streak). The animation tick runs at about 30 fps.
- A `[visualizer]` config table that tunes every visualizer mode. Shared:
  `hue_speed`, `beat_sensitivity`. Spectrum: `spectrum_smoothing`,
  `spectrum_peak_gravity`, `spectrum_tilt`, `spectrum_monstercat`. Waveform:
  `wave_falloff`. Stereo: `stereo_smoothing`. Radial bloom: `radial_decay`,
  `radial_reach`, `radial_core`. Beat sparks: `spark_life`, `spark_speed`,
  `spark_gravity` (a negative value gives straight streaks), `spark_count`,
  and `spark_trail` (comet streaks). Each key is optional and falls back to a
  built-in default.

### Changed
- The visualizer keeps its own vivid palette; the UI theme no longer changes
  it. The spectrum, waveform, radial bloom, and stereo spectrum now smooth
  their levels toward the previous frame, so bars glide and blooms fall off
  gradually instead of snapping. The spectrum peak caps fall under gravity.
- Song info (`i`) and artist info (`I`) now push onto the browse stack from any
  right-pane view, not only from the browse view.

### Fixed
- The cover-art aspect ratio is corrected so the image is not stretched
  vertically.
- A late queue restore that paused freshly started playback is dropped.

## [0.7.4] - 2026-08-23

### Fixed
- The GitHub release-binaries workflow failed to build because the container
  install used `--no-install-recommends` and dropped `libc6-dev`, so cgo could
  not find the C standard headers. The workflow now installs `libc6-dev`
  explicitly. This is the first release to publish working `linux/amd64` and
  `linux/arm64` binaries on the GitHub Release.

## [0.7.3] - 2026-08-23

### Added
- GitHub release binaries. A new `.github/workflows/release.yml` builds Linux
  binaries for `linux/amd64` and `linux/arm64` on a `v*` tag and attaches them
  to the GitHub Release. The binaries build inside a Debian 11 "bullseye"
  container (glibc 2.31) with cgo and dynamic ALSA linking, so they run on a
  wide range of distributions but need `libasound2` (`alsa-lib`) installed at
  runtime. The amd64 job builds natively; the arm64 job builds under QEMU
  emulation. The README documents the prebuilt binaries and the runtime
  dependency.

### Changed
- The release documentation now states that the GitHub release is the primary
  target and its body must list every change since the last GitHub release,
  while the Gitea changelog is secondary. It also documents writing the
  `CHANGELOG.md` section before tagging and fixing a wrong release body with a
  `PATCH` to the GitHub Releases API.

## [0.7.1] - 2026-08-23

### Added
- Radio (feeder) queue mode. It keeps a bounded, self-refilling queue drawn
  from a playlist or a search result. `V` (`radio`) turns it on from the
  focused source and off again; `v` (`radio_set`) sets the target queue size.
  The mode snapshots the source songs into a candidate pool, forces consume on
  and repeat off, and keeps the queue at `radio_queue_size` random songs
  (default 10). It refills on each one-second tick and after a consume, so the
  queue never empties. It avoids adding a song equal to the current queue tail,
  so it does not repeat back to back. A manual add past the target size pauses
  the auto-add until consume drains the queue below the size again. The status
  bar shows an `F` flag while radio mode is on. The new `radio_queue_size`
  config key sets the default target size.

## [0.7.0] - 2026-08-22

### Added
- A `LICENSE` file. tuiplay's own code is released into the public domain under
  The Unlicense. The author claims no ownership and gives no warranty. The Arch
  package now declares `Unlicense` plus `LGPL2.1` for the statically linked
  go-aac decoder.

### Changed
- The README now carries a clear notice that the project is built with AI and
  comes with no guarantees, and points at the public-domain license.
- The README installation section no longer references prebuilt binaries or a
  releases page. It documents building from source only, and its clone URL now
  points at the public GitHub repository.

## [0.6.0] - 2026-08-22

### Added
- GitHub mirror tooling. tuiplay can publish a scrubbed source snapshot to a
  public GitHub repository (`ScuttleSE/tuiplay`) on a release tag. The mirror
  excludes the CI workflows and the AI working files (`.gitea/`, `AGENTS.md`,
  `HANDOFF.md`, `scripts/`, `github-release.env`), so only the source reaches
  GitHub. `scripts/github-release.sh` builds a squashed `release: vX.Y.Z`
  commit on a chained `github-main` branch, pushes it and the tag over SSH,
  and creates the GitHub Release with the matching `CHANGELOG.md` section as
  the body. `scripts/install-hooks.sh` installs a `pre-push` hook that runs the
  mirror automatically when a `v*` tag is pushed to `origin`. The mirror refuses
  any version below `0.6.0`.

## [0.5.0] - 2026-08-22

### Added
- Native AAC-LC playback. The player decodes AAC-LC in an MP4/M4A container
  (`m4a`, `m4b`, `mp4`, `aac`) with pure-Go libraries: `github.com/tphakala/go-m4a`
  demuxes the container and `github.com/tphakala/go-aac` decodes the audio. This
  streams the original file instead of a server-side transcode, so it needs no
  extra codec and keeps the original quality. HE-AAC (SBR/PS) and ALAC are still
  transcoded server-side, as go-aac cannot decode them; the player detects the
  unsupported source and falls back automatically.
- Transcoding indicator. The status bar shows a `t` flag next to the other mode
  flags while the current track plays through a server-side transcode, and shows
  a `Transcoding: <artist> - <title>` status message on the fallback.

## [0.4.0] - 2026-08-22

### Added
- Color themes. The `theme` config key picks a named theme: `default`,
  `catppuccin-latte`, `catppuccin-frappe`, `catppuccin-macchiato`, or
  `catppuccin-mocha`. A `[theme_colors]` table overrides single roles with
  `#rrggbb` hex values. The interface renders every text color from the
  resolved theme. The main program resolves the theme on start and exits on an
  unknown name, an unknown role, or a malformed hex value.
- Edit imported smart playlists. `e` on a playlist row on the Playlists level
  opens the smart-playlist builder loaded with that playlist. It reads and
  parses the matching `.nsp` file back into the builder. A renamed playlist
  writes a new file and removes the old one on save. It works only for a
  playlist whose `.nsp` file uses the curated field set and the flat `all`
  group the builder writes.
- Filepath condition in the smart-playlist builder. The new `filepath` field
  matches the Navidrome file path, which is relative to the music library
  folder.
- Three more visualizer modes. The fullscreen visualizer (`5`) now cycles five
  modes with `Tab` or `Space`: frequency spectrum, waveform, stereo mirror
  spectrum, a pulsing rainbow ellipse, and a Lorenz attractor. The stereo
  spectrum reads a separate stereo sample tap.
- Lyrics prefetch. tuiplay starts the lyrics lookup in the background as soon
  as a song begins, so the lyrics page opens instantly instead of fetching on
  open. It runs regardless of the active view and is a no-op when the lyrics
  source is off.
- Arch Linux package. A `packaging/arch/PKGBUILD` and a
  `.gitea/workflows/archpkg.yaml` workflow build a `.pkg.tar.zst` on a `v*` tag
  and attach it to the release. A manual run from the Gitea Actions tab
  packages the current checkout and updates the `rolling` pre-release under a
  stable asset name. The workflow signs the package with GPG when a
  `GPG_PRIVATE_KEY` secret is set.

### Changed
- The `default` theme now uses explicit hex colors that reproduce the classic
  ncmpcpp look. It no longer follows the terminal palette. The fullscreen
  visualizer keeps its own hardcoded colors.
- The visualizer spectrum no longer saturates. Magnitudes are normalized and
  mapped through a dB window, the monstercat smoothing weight is tuned, and a
  falling peak cap sits over each bar. The waveform fills solid with block
  glyphs and falls off gradually through a per-column envelope. The ellipse is
  a filled rainbow blob whose radius follows the spectrum; the Lorenz mode
  plots a long persistent trajectory scaled by the audio energy.

### Fixed
- The Arch package builds as a single `tuiplay` package with the binary in
  `/usr/bin`, not an empty `tuiplay-debug` split. The PKGBUILD disables
  makepkg's debug and strip handling because the Go build already strips.

## [0.3.0] - 2026-08-22

### Added
- Lyrics reader. tuiplay shows synchronized lyrics for the playing song. It
  looks the song up through a chain of tiers: a local cache, a local lrclib
  SQLite dump (`lrclib_db_path`), and the network providers `lrcmux` and
  `lrclib` in the configured order (`lyrics_providers`). A hit back-fills the
  cache; a clean miss is cached and rechecked after `lyrics_miss_recheck_days`.
  The `4` key opens the lyrics page for the current song. Synced lyrics
  highlight the active line and scroll to follow playback.
- Lyrics quality glyph. The lyrics page title carries a semiquaver (`♬`) for
  synced, per-line timed lyrics and a quaver (`♪`) for plain, untimed lyrics.
- Parallel right-pane views. The `2`, `3`, and `4` keys switch between the
  browse, search, and lyrics views. Each keeps its own state and position.
- Song rating. `§` (`rate_up`) raises the rating of the selected or current
  song up to 5; `½` (`rate_down`) lowers it to 0. Ratings are saved on the
  Navidrome server through `setRating`. The queue shows filled stars, or a
  thumbs-down glyph for a rating of 1 (a dislike). The song-info view shows the
  rating.
- Fullscreen audio visualizer. The `5` key opens it. It has two modes: a
  frequency spectrum from a hand-rolled FFT and a time-domain waveform. `Tab`
  or `Space` switches the mode; `Esc` or `5` closes it. Each cell is colored
  along a green, yellow, red height gradient that adapts to the terminal color
  profile. Playback keys still work while it is open.
- External control. When `control_socket` is set, tuiplay listens on a unix
  socket and the `tuiplay ctl <command> [args]` subcommand sends commands to
  it, for a streamdeck or a script. Commands: `play`, `pause`, `playpause`,
  `stop`, `next`, `prev`, `seek <sec>`, `volume <+|-|N>`, and
  `playlist <name>` (case-insensitive match, honors random mode).
- OpenSubsonic extension cache and index-based server play queue. tuiplay
  restores the saved queue on start and saves it on exit when the server
  supports `indexBasedQueue`.
- Comment tag field in the search form.

### Changed
- On a track change the queue cursor snaps to the playing song, so the queue
  window centers the current track. This keeps the current song visible in a
  large or random queue.
- The player plays m4a and other formats it cannot decode natively (AAC, ALAC,
  and similar) by asking Navidrome to transcode them to mp3 on the fly.

### Fixed
- Saving the config on exit no longer removes comments or commented-out
  sections. The `[ui]` write now edits the two values in place, skips
  commented lines, and stops only at a real table header, so a commented
  `# [keys]` block and all other comments survive.

## [0.2.0] - 2026-08-22

### Added
- Search view. The `3` key opens a search form in the right pane, with fields
  Any, Artist, Title, Album, Genre, Date, and Filename. `Enter` on a field
  edits its value. `Enter` on `Search` queries the server and filters the
  results so each filled field matches its own tag. `Enter` on `Reset` clears
  the fields. The results show as a Tracks level.
- Smart-playlist builder. When `nsp_path` is set in the config, the Playlists
  level shows a `[New smart playlist]` row. It opens a rule builder that sets a
  name, one or more conditions (a curated field, an operator valid for the
  field type, and a value, joined under a single `all` group), an optional
  sort field, order, and limit. `Save` writes a Navidrome `.nsp` file to
  `nsp_path` and triggers an incremental library scan so Navidrome imports it.
  A scan failure is a soft error; the file is still written.
- The Subsonic client gained `StartScan`, which asks the server to scan the
  library. This imports new playlist files, including `.nsp` files.
- A scan indicator. The interface polls `GetScanStatus` once per second. While
  the server scans the library, a braille spinner joins the mode-flag group in
  the status bar, like `[⠙ Rx]`. A faster tick animates it only during a scan.
- Audio crossfade between tracks. The player feeds a persistent mixer and
  overlaps the outgoing and incoming tracks with equal-power volume
  transitions. A monitor starts the crossfade before the current track ends.
  Manual next, previous, and play also crossfade. `x` toggles it; `X` sets
  the length in seconds.

### Changed
- Deleting a playlist that has a matching `.nsp` file in `nsp_path` now also
  removes that file and triggers an incremental scan, so an imported smart
  playlist does not reappear on the next scan. The confirm names the `.nsp`
  file when one is found. The `[New smart playlist]` row is not deletable.

### Fixed
- The text prompt no longer doubles spaces. Typed spaces appeared twice
  because the space key carried a space rune and the handler added another.
- The status bar now uses a black background with light text, so it matches
  the ncmpcpp look instead of a white bar.

## [0.1.0] - 2026-08-22

The first tagged release. tuiplay is a terminal music player that streams
from a Navidrome server through the Subsonic API. The look follows ncmpcpp.

### Added
- Two-pane interface: a play queue on the left and a navigation panel on the
  right. The right pane can be hidden so the queue fills the width.
- Navigation panel with entrypoints: Artists, Albums, Genres, Release Years,
  Tracks, and Playlists. Each entrypoint is a virtual folder that drills down.
  A `[..]` row goes back one level.
- Play queue with ncmpcpp-style columns: Artist, Track + Title, Album, Time.
  The current track is bold. The selected row shows a segmented colored bar.
- Playback: play, pause, stop, next, previous, seek, and volume as a percent.
- Smart previous (`<`): restart the current song, then go to the previous
  song on a second press within two seconds.
- Configurable seek step (`seek_seconds`, default 10).
- Consume mode (`R`), repeat mode (`r`), and random mode (`z`). Repeat and
  consume are exclusive. Random works together with either.
- Crossfade keys (`x` toggles, `X` sets the length). The setting is stored;
  the audio crossfade is not implemented yet.
- Queue editing: `Delete` removes a song; `c` clears the queue after a
  confirm. Space adds an item to the queue and moves the cursor down.
- Song info view (`i`) loaded with getSong. Artist info stub (`I`).
- F1 help card.
- Playlists: browse, play (Enter replaces the queue and plays), save the
  queue as a new playlist (`S`), overwrite the source playlist (`Ctrl+s`),
  and delete a playlist (`Delete` with a confirm).
- Home and End keys, and Page Up and Page Down.
- Rebindable keys through a `[keys]` config section, with conflict checks.
- Config loader with a first-run template and a persisted `[ui]` section.
- Now-playing scrobble.
- Continuous build workflow that publishes a rolling pre-release.
- Release workflow that builds a stripped binary on a `v*` tag.

### Changed
- Volume is the player's own stream level, shown as a percent, not the
  system mixer.
- Status bar mode flags are single letters: `R` repeat, `c` consume,
  `r` random, `x` crossfade.
