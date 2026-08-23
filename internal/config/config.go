// Package config loads the tuiplay configuration.
//
// The configuration file is a TOML file. The default path is
// ~/.config/tuiplay/config.toml. The file holds the Navidrome server
// URL and the login credentials. The file must not enter the git repo.
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/BurntSushi/toml"
)

// templateConfig is the content that Load writes on first run when no
// config file exists. The user must edit the values.
const templateConfig = `# Configuration for tuiplay.
# Edit the values, then run tuiplay again.
# Keep this file private. It holds your password.

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

# Optional: the crossfade length in seconds. Default is 4. The user turns
# crossfade on and off with a key.
crossfade_seconds = 4

# Optional: the target queue size for radio (feeder) mode. Default is 10.
# In radio mode the queue draws random songs from a playlist or a search
# result and consumes played songs, so the queue never empties and never
# grows past this size on its own.
radio_queue_size = 10

# Optional: the directory where tuiplay writes smart-playlist files (.nsp).
# Point this at a directory that Navidrome scans, such as a path listed in
# the server PlaylistsPath, or a folder inside a music library. The path may
# be a local directory or a network mount. When this value is empty, the
# smart-playlist feature is hidden.
# nsp_path = "/srv/navidrome/playlists"

# Optional: the path to an lrclib SQLite database dump. tuiplay reads it to
# show synchronized lyrics. Download a dump from https://lrclib.net/db-dumps
# and point this at the .sqlite3 file. The file is read only. This is one
# tier of the lyrics lookup. When this value is empty, the dump tier is off.
# lrclib_db_path = "/srv/lrclib/db.sqlite3"

# Optional: the directory where tuiplay caches lyrics, one .lrc file per
# song. The lyrics lookup checks this cache first. When this value is empty,
# tuiplay uses the default directory (a "lyrics" folder next to this file).
# lyrics_cache_path = "/home/you/.config/tuiplay/lyrics"

# Optional: how many days a cached "no lyrics" result stays valid. After
# this many days tuiplay rechecks the dump and the network. Default is 7.
# A value of 0 means a miss never goes stale.
# lyrics_miss_recheck_days = 7

# Optional: the ordered list of network lyrics providers. tuiplay tries them
# in order when the cache and the dump miss. Known names are "lrcmux" and
# "lrclib".
#   - List both names to enable both, in priority order (first tried first).
#   - Reorder the names to change which provider tuiplay prefers.
#   - Remove a name to disable that one provider.
#   - Set an empty list ([]) to turn the whole network tier off.
#   - Omit this key to use the default: ["lrcmux", "lrclib"].
# lrcmux aggregates several providers (Genius, KuGou, LRCLIB, Musixmatch,
# YouTube Music), so it already includes lrclib.
# lyrics_providers = ["lrcmux", "lrclib"]

# Optional: a unix socket path for external control. When set, tuiplay
# listens on this socket and the "tuiplay ctl" subcommand sends commands to
# it, for example "tuiplay ctl next" or "tuiplay ctl playlist Focus". This
# lets a streamdeck or a script control playback. When empty, the feature
# is off. A leading ~ expands to your home directory.
# control_socket = "~/.config/tuiplay/control.sock"

# Optional: the color theme. tuiplay ships these names:
#   default, catppuccin-latte, catppuccin-frappe, catppuccin-macchiato,
#   catppuccin-mocha.
# The "default" theme reproduces the classic ncmpcpp look. Omit this key or
# set an empty value to use it.
# theme = "catppuccin-mocha"

# Optional: override individual theme colors. Each key is a role and each
# value is a #rrggbb hex color. These apply on top of the named theme.
# The roles are:
#   header, header_focus, rule, artist, track, title, album, time, plain,
#   back, sel_fg, sel_artist_bg, sel_album_bg, sel_time_bg, progress,
#   status_fg, status_bg, spinner, help_border, help_fg, help_bg.
# [theme_colors]
# album = "#89b4fa"
# progress = "#f38ba8"

# Interface state. tuiplay updates this section on exit.
[ui]
# The last right-pane state: "nav" (navigation panel shown) or "hidden".
active_view = "nav"
# The fraction of the width for the left queue pane, from 0 to 1.
split_ratio = 0.6

# Key bindings. Uncomment a line to change a key. An unset action uses its
# default. Two actions must not share a key. Keys are case sensitive.
# [keys]
# quit = "q"
# hide_right = "1"
# show_nav = "2"
# show_search = "3"
# show_lyrics = "4"
# focus_toggle = "tab"
# up = "k"
# down = "j"
# page_up = "pgup"
# page_down = "pgdown"
# home = "home"
# end = "end"
# select = "enter"
# add = "a"
# add_all = "space"
# consume = "R"
# repeat = "r"
# shuffle = "z"
# crossfade = "x"
# crossfade_set = "X"# save_playlist = "S"
# save_overwrite = "ctrl+s"
# delete = "delete"
# clear = "c"
# song_info = "i"
# artist_info = "I"
# help = "f1"
# pause = "p"
# stop = "s"
# next = ">"
# prev = "<"
# seek_forward = "f"
# seek_back = "b"
# volume_up = "+"
# volume_down = "-"
# rate_up = "§"
# rate_down = "½"
# visualizer = "5"
# edit = "e"
# radio = "V"
# radio_set = "v"
`

// ErrCreatedTemplate reports that Load found no config file and wrote a
// template. The caller must tell the user to edit the file, then exit.
var ErrCreatedTemplate = errors.New("config: created a template file; edit it and run again")

// Config holds the runtime settings.
type Config struct {
	// ServerURL is the base URL of the Navidrome server.
	// Example: https://navidrome.example.com
	ServerURL string `toml:"server_url"`

	// Username is the Navidrome account name.
	Username string `toml:"username"`

	// Password is the Navidrome account password in clear text.
	// The client sends a salted hash. The client does not send the
	// clear password to the server.
	Password string `toml:"password"`

	// MaxBitRate limits the stream bit rate in kilobits per second.
	// A value of zero sets no limit.
	MaxBitRate int `toml:"max_bit_rate"`

	// SeekSeconds is the seek step for the seek keys. A value of zero
	// means the default of 10 seconds.
	SeekSeconds int `toml:"seek_seconds"`

	// CrossfadeSeconds is the crossfade length. A value of zero disables
	// crossfade until the user turns it on.
	CrossfadeSeconds int `toml:"crossfade_seconds"`

	// RadioQueueSize is the target queue size for radio (feeder) mode. A
	// value of zero means the default of 10.
	RadioQueueSize int `toml:"radio_queue_size"`

	// NSPPath is the directory where tuiplay writes smart-playlist files
	// (.nsp). An empty value hides the smart-playlist feature.
	NSPPath string `toml:"nsp_path"`
	// LrclibDBPath is the path to an lrclib SQLite database dump. tuiplay
	// reads it to show synchronized lyrics. An empty value turns off the
	// dump tier of the lyrics lookup.
	LrclibDBPath string `toml:"lrclib_db_path"`

	// LyricsCachePath is the directory where tuiplay caches lyrics. An
	// empty value means the default directory next to the config file.
	LyricsCachePath string `toml:"lyrics_cache_path"`

	// LyricsMissRecheckDays is how many days a cached miss stays valid. A
	// zero value means the default of seven days. A negative value means a
	// miss never goes stale.
	LyricsMissRecheckDays int `toml:"lyrics_miss_recheck_days"`

	// LyricsProviders is the ordered list of network lyrics providers.
	// A nil value means the default order. An empty (non-nil) list turns
	// off the network tier.
	LyricsProviders []string `toml:"lyrics_providers"`

	// ControlSocket is the path to a unix socket for external control.
	// When set, tuiplay listens on it and the "tuiplay ctl" subcommand
	// sends commands to it. An empty value turns the feature off.
	ControlSocket string `toml:"control_socket"`

	// Theme is the name of the color theme. An empty value means the
	// "default" theme. Known names ship with tuiplay.
	Theme string `toml:"theme"`

	// ThemeColors overrides individual theme colors by role. Each key is a
	// role name and each value is a "#rrggbb" hex color.
	ThemeColors map[string]string `toml:"theme_colors"`

	// UI holds the interface state that persists across runs.
	UI UIState `toml:"ui"`

	// Keys maps an action name to a key. An unset action uses its default.
	Keys map[string]string `toml:"keys"`
}

// UIState holds the persisted interface state.
type UIState struct {
	// ActiveView is the last right-pane state. One of: "nav" or "hidden".
	// An empty value means the default.
	ActiveView string `toml:"active_view"`

	// SplitRatio is the fraction of the width for the left queue pane.
	// A value of zero means the default.
	SplitRatio float64 `toml:"split_ratio"`
}

// DefaultPath returns the default configuration file path.
func DefaultPath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tuiplay", "config.toml"), nil
}

// DefaultLyricsCacheDir returns the default lyrics cache directory. It is a
// "lyrics" folder inside the tuiplay config directory.
func DefaultLyricsCacheDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "tuiplay", "lyrics"), nil
}

// DefaultLyricsProviders is the network provider order when the config sets
// no lyrics_providers key.
var DefaultLyricsProviders = []string{"lrcmux", "lrclib"}

// knownLyricsProviders is the set of valid provider names.
var knownLyricsProviders = map[string]bool{"lrcmux": true, "lrclib": true}

// LyricsCacheDir returns the cache directory to use. It is the configured
// path, or the default when the path is empty.
func (c *Config) LyricsCacheDir() (string, error) {
	if c.LyricsCachePath != "" {
		return c.LyricsCachePath, nil
	}
	return DefaultLyricsCacheDir()
}

// LyricsRecheckDays returns the miss recheck window in days. A zero config
// value means the default of seven days. A negative value means the miss
// never goes stale; it returns zero, which the cache reads as "never".
func (c *Config) LyricsRecheckDays() int {
	switch {
	case c.LyricsMissRecheckDays == 0:
		return 7
	case c.LyricsMissRecheckDays < 0:
		return 0
	default:
		return c.LyricsMissRecheckDays
	}
}

// ResolvedLyricsProviders returns the network provider order. A nil config
// value means the default order. A non-nil list is used as written.
func (c *Config) ResolvedLyricsProviders() []string {
	if c.LyricsProviders == nil {
		return DefaultLyricsProviders
	}
	return c.LyricsProviders
}

// RadioSize returns the radio (feeder) target queue size. A zero config
// value means the default of 10.
func (c *Config) RadioSize() int {
	if c.RadioQueueSize <= 0 {
		return 10
	}
	return c.RadioQueueSize
}

// ControlSocketPath returns the control socket path with a leading ~
// expanded to the user home directory. An empty value stays empty.
func (c *Config) ControlSocketPath() string {
	return expandHome(c.ControlSocket)
}

// expandHome expands a leading ~ in a path to the user home directory. It
// returns the path unchanged when it has no ~ prefix or the home lookup
// fails.
func expandHome(path string) string {
	if path == "" {
		return ""
	}
	if path == "~" {
		if home, err := os.UserHomeDir(); err == nil {
			return home
		}
		return path
	}
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, path[2:])
		}
	}
	return path
}

// Load reads and validates the configuration file at path.
// If path is empty, Load uses DefaultPath.
func Load(path string) (*Config, error) {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return nil, err
		}
		path = p
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			if werr := writeTemplate(path); werr != nil {
				return nil, werr
			}
			return nil, fmt.Errorf("%w: %s", ErrCreatedTemplate, path)
		}
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	var c Config
	if err := toml.Unmarshal(data, &c); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}

	if err := c.validate(); err != nil {
		return nil, err
	}
	return &c, nil
}

// writeTemplate creates the parent directory and writes the template
// config to path. The file mode is 0600 because the file holds a password.
func writeTemplate(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("create config dir %s: %w", dir, err)
	}
	if err := os.WriteFile(path, []byte(templateConfig), 0600); err != nil {
		return fmt.Errorf("write template config %s: %w", path, err)
	}
	return nil
}

// validate checks that the required fields are present.
func (c *Config) validate() error {
	if c.ServerURL == "" {
		return fmt.Errorf("config: server_url is empty")
	}
	if c.Username == "" {
		return fmt.Errorf("config: username is empty")
	}
	if c.Password == "" {
		return fmt.Errorf("config: password is empty")
	}
	for _, p := range c.LyricsProviders {
		if !knownLyricsProviders[p] {
			return fmt.Errorf("config: unknown lyrics provider %q (known: lrcmux, lrclib)", p)
		}
	}
	return nil
}

// SaveUIState writes the UI state into the config file at path. It updates
// only the [ui] section. It keeps every comment and every other key that
// the user wrote. If path is empty, SaveUIState uses DefaultPath.
func SaveUIState(path string, ui UIState) error {
	if path == "" {
		p, err := DefaultPath()
		if err != nil {
			return err
		}
		path = p
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read config %s: %w", path, err)
	}

	updated := updateUISection(string(data), ui)
	if err := os.WriteFile(path, []byte(updated), 0600); err != nil {
		return fmt.Errorf("write config %s: %w", path, err)
	}
	return nil
}

// updateUISection returns the config text with the [ui] section set to ui.
// It rewrites only the active_view and split_ratio keys inside [ui]. It
// leaves all other text, including comments and commented-out sections,
// unchanged. If no [ui] section exists, it appends one.
func updateUISection(text string, ui UIState) string {
	lines := strings.Split(text, "\n")

	// Find the [ui] header line. A commented "# [ui]" does not count.
	start := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == "[ui]" {
			start = i
			break
		}
	}

	if start == -1 {
		// No [ui] section. Append one. Keep one blank line before it.
		block := fmt.Sprintf("[ui]\nactive_view = %q\nsplit_ratio = %s\n",
			ui.ActiveView, strconv.FormatFloat(ui.SplitRatio, 'f', -1, 64))
		out := strings.TrimRight(text, "\n")
		if out != "" {
			out += "\n\n"
		}
		return out + block
	}

	// Find the end of the [ui] section: the next line that starts a new
	// real table. A commented line (for example "# [keys]") does not end
	// the section, so its comment block is preserved.
	end := len(lines)
	for i := start + 1; i < len(lines); i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "#") {
			continue
		}
		if strings.HasPrefix(t, "[") {
			end = i
			break
		}
	}

	splitStr := strconv.FormatFloat(ui.SplitRatio, 'f', -1, 64)

	// Edit the two keys in place inside the section body. Track whether
	// each key was found so a missing key is inserted after the header.
	haveView, haveSplit := false, false
	for i := start + 1; i < end; i++ {
		t := strings.TrimSpace(lines[i])
		if strings.HasPrefix(t, "#") {
			continue
		}
		switch {
		case keyMatches(t, "active_view"):
			lines[i] = fmt.Sprintf("active_view = %q", ui.ActiveView)
			haveView = true
		case keyMatches(t, "split_ratio"):
			lines[i] = fmt.Sprintf("split_ratio = %s", splitStr)
			haveSplit = true
		}
	}

	// Insert any missing key right after the [ui] header, in a stable
	// order: active_view first, then split_ratio.
	var inserts []string
	if !haveSplit {
		inserts = append(inserts, "split_ratio = "+splitStr)
	}
	if !haveView {
		inserts = append(inserts, fmt.Sprintf("active_view = %q", ui.ActiveView))
	}
	if len(inserts) > 0 {
		head := append([]string{}, lines[:start+1]...)
		tail := append([]string{}, lines[start+1:]...)
		lines = append(head, append(inserts, tail...)...)
	}

	return strings.Join(lines, "\n")
}

// keyMatches reports whether a config line assigns the named key. It
// ignores leading whitespace and matches only the key before the "=".
func keyMatches(line, key string) bool {
	eq := strings.IndexByte(line, '=')
	if eq < 0 {
		return false
	}
	return strings.TrimSpace(line[:eq]) == key
}
