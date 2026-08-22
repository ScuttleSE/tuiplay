package ui

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Theme holds one hex color per semantic role of the interface. Each value
// is a "#rrggbb" string. A theme drives every text color of the interface,
// except the fullscreen visualizer, which keeps its own hardcoded colors.
type Theme struct {
	Header      string // column headers, pane titles, info labels, top bar
	HeaderFocus string // the header of the focused pane
	Rule        string // horizontal rules and the vertical divider
	Artist      string // queue artist column
	Track       string // queue track-number column
	Title       string // queue title column, nav track labels
	Album       string // queue album column, info values, search values
	Time        string // queue time column, nav track times
	Plain       string // generic rows, lyrics lines, static text
	Back        string // the [..] back row

	SelFg       string // foreground of a selected row
	SelArtistBg string // selected queue row, artist and title segment
	SelAlbumBg  string // selected row, album segment and right-pane rows
	SelTimeBg   string // selected queue row, time segment

	Progress string // the progress bar
	StatusFg string // the status bar foreground
	StatusBg string // the status bar background
	Spinner  string // the scan spinner

	HelpBorder string // the help card border
	HelpFg     string // the help card text
	HelpBg     string // the help card background
}

// themeRoles lists the config keys of the [theme_colors] override table and
// maps each to the matching Theme field. The order is stable for messages.
var themeRoles = []struct {
	key string
	get func(*Theme) *string
}{
	{"header", func(t *Theme) *string { return &t.Header }},
	{"header_focus", func(t *Theme) *string { return &t.HeaderFocus }},
	{"rule", func(t *Theme) *string { return &t.Rule }},
	{"artist", func(t *Theme) *string { return &t.Artist }},
	{"track", func(t *Theme) *string { return &t.Track }},
	{"title", func(t *Theme) *string { return &t.Title }},
	{"album", func(t *Theme) *string { return &t.Album }},
	{"time", func(t *Theme) *string { return &t.Time }},
	{"plain", func(t *Theme) *string { return &t.Plain }},
	{"back", func(t *Theme) *string { return &t.Back }},
	{"sel_fg", func(t *Theme) *string { return &t.SelFg }},
	{"sel_artist_bg", func(t *Theme) *string { return &t.SelArtistBg }},
	{"sel_album_bg", func(t *Theme) *string { return &t.SelAlbumBg }},
	{"sel_time_bg", func(t *Theme) *string { return &t.SelTimeBg }},
	{"progress", func(t *Theme) *string { return &t.Progress }},
	{"status_fg", func(t *Theme) *string { return &t.StatusFg }},
	{"status_bg", func(t *Theme) *string { return &t.StatusBg }},
	{"spinner", func(t *Theme) *string { return &t.Spinner }},
	{"help_border", func(t *Theme) *string { return &t.HelpBorder }},
	{"help_fg", func(t *Theme) *string { return &t.HelpFg }},
	{"help_bg", func(t *Theme) *string { return &t.HelpBg }},
}

// builtinThemes holds the named themes tuiplay ships. The "default" theme
// reproduces the historical ncmpcpp look with explicit xterm-16 hex, so it
// no longer follows the terminal palette. The catppuccin flavors use the
// official palette (https://catppuccin.com).
var builtinThemes = map[string]Theme{
	"default": {
		Header:      "#cdcd00", // yellow
		HeaderFocus: "#00cd00", // green
		Rule:        "#7f7f7f", // bright black
		Artist:      "#cdcdcd", // white
		Track:       "#00cd00", // green
		Title:       "#cdcdcd", // white
		Album:       "#00cdcd", // cyan
		Time:        "#00cdcd", // cyan
		Plain:       "#cdcdcd", // white
		Back:        "#0000ee", // blue
		SelFg:       "#000000", // black
		SelArtistBg: "#cdcd00", // yellow
		SelAlbumBg:  "#00cdcd", // cyan
		SelTimeBg:   "#cd00cd", // magenta
		Progress:    "#00cd00", // green
		StatusFg:    "#cdcdcd", // white
		StatusBg:    "#000000", // black
		Spinner:     "#cdcd00", // yellow
		HelpBorder:  "#0000ee", // blue
		HelpFg:      "#cdcdcd", // white
		HelpBg:      "#000000", // black
	},
	// Catppuccin Latte (light).
	"catppuccin-latte": {
		Header:      "#df8e1d", // yellow
		HeaderFocus: "#40a02b", // green
		Rule:        "#9ca0b0", // overlay0
		Artist:      "#4c4f69", // text
		Track:       "#40a02b", // green
		Title:       "#4c4f69", // text
		Album:       "#209fb5", // sky
		Time:        "#209fb5", // sky
		Plain:       "#4c4f69", // text
		Back:        "#1e66f5", // blue
		SelFg:       "#eff1f5", // base
		SelArtistBg: "#df8e1d", // yellow
		SelAlbumBg:  "#209fb5", // sky
		SelTimeBg:   "#8839ef", // mauve
		Progress:    "#40a02b", // green
		StatusFg:    "#4c4f69", // text
		StatusBg:    "#e6e9ef", // mantle
		Spinner:     "#df8e1d", // yellow
		HelpBorder:  "#1e66f5", // blue
		HelpFg:      "#4c4f69", // text
		HelpBg:      "#e6e9ef", // mantle
	},
	// Catppuccin Frappe.
	"catppuccin-frappe": {
		Header:      "#e5c890", // yellow
		HeaderFocus: "#a6d189", // green
		Rule:        "#737994", // overlay0
		Artist:      "#c6d0f5", // text
		Track:       "#a6d189", // green
		Title:       "#c6d0f5", // text
		Album:       "#99d1db", // sky
		Time:        "#99d1db", // sky
		Plain:       "#c6d0f5", // text
		Back:        "#8caaee", // blue
		SelFg:       "#303446", // base
		SelArtistBg: "#e5c890", // yellow
		SelAlbumBg:  "#99d1db", // sky
		SelTimeBg:   "#ca9ee6", // mauve
		Progress:    "#a6d189", // green
		StatusFg:    "#c6d0f5", // text
		StatusBg:    "#292c3c", // mantle
		Spinner:     "#e5c890", // yellow
		HelpBorder:  "#8caaee", // blue
		HelpFg:      "#c6d0f5", // text
		HelpBg:      "#292c3c", // mantle
	},
	// Catppuccin Macchiato.
	"catppuccin-macchiato": {
		Header:      "#eed49f", // yellow
		HeaderFocus: "#a6da95", // green
		Rule:        "#6e738d", // overlay0
		Artist:      "#cad3f5", // text
		Track:       "#a6da95", // green
		Title:       "#cad3f5", // text
		Album:       "#91d7e3", // sky
		Time:        "#91d7e3", // sky
		Plain:       "#cad3f5", // text
		Back:        "#8aadf4", // blue
		SelFg:       "#24273a", // base
		SelArtistBg: "#eed49f", // yellow
		SelAlbumBg:  "#91d7e3", // sky
		SelTimeBg:   "#c6a0f6", // mauve
		Progress:    "#a6da95", // green
		StatusFg:    "#cad3f5", // text
		StatusBg:    "#1e2030", // mantle
		Spinner:     "#eed49f", // yellow
		HelpBorder:  "#8aadf4", // blue
		HelpFg:      "#cad3f5", // text
		HelpBg:      "#1e2030", // mantle
	},
	// Catppuccin Mocha (dark).
	"catppuccin-mocha": {
		Header:      "#f9e2af", // yellow
		HeaderFocus: "#a6e3a1", // green
		Rule:        "#6c7086", // overlay0
		Artist:      "#cdd6f4", // text
		Track:       "#a6e3a1", // green
		Title:       "#cdd6f4", // text
		Album:       "#89dceb", // sky
		Time:        "#89dceb", // sky
		Plain:       "#cdd6f4", // text
		Back:        "#89b4fa", // blue
		SelFg:       "#1e1e2e", // base
		SelArtistBg: "#f9e2af", // yellow
		SelAlbumBg:  "#89dceb", // sky
		SelTimeBg:   "#cba6f7", // mauve
		Progress:    "#a6e3a1", // green
		StatusFg:    "#cdd6f4", // text
		StatusBg:    "#181825", // mantle
		Spinner:     "#f9e2af", // yellow
		HelpBorder:  "#89b4fa", // blue
		HelpFg:      "#cdd6f4", // text
		HelpBg:      "#181825", // mantle
	},
}

// hexColor matches a "#rrggbb" color value.
var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// builtinThemeNames returns the sorted list of shipped theme names.
func builtinThemeNames() []string {
	names := make([]string, 0, len(builtinThemes))
	for n := range builtinThemes {
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

// resolveTheme returns the theme named name with the per-role overrides
// applied. An empty name means "default". An unknown name is an error. An
// unknown override key or a malformed hex value is an error.
func ResolveTheme(name string, overrides map[string]string) (Theme, error) {
	if name == "" {
		name = "default"
	}
	base, ok := builtinThemes[name]
	if !ok {
		return Theme{}, fmt.Errorf("unknown theme %q; known themes are %s",
			name, strings.Join(builtinThemeNames(), ", "))
	}
	for key, val := range overrides {
		field := themeFieldByKey(&base, key)
		if field == nil {
			return Theme{}, fmt.Errorf("unknown theme color %q", key)
		}
		if !hexColor.MatchString(val) {
			return Theme{}, fmt.Errorf("theme color %q: expected a #rrggbb value, got %q", key, val)
		}
		*field = strings.ToLower(val)
	}
	return base, nil
}

// themeFieldByKey returns a pointer to the theme field for a role key, or
// nil when the key is unknown.
func themeFieldByKey(t *Theme, key string) *string {
	for _, r := range themeRoles {
		if r.key == key {
			return r.get(t)
		}
	}
	return nil
}

// themeStyles holds the lipgloss styles the view renders with. A theme
// builds one set of styles once.
type themeStyles struct {
	colHeader   lipgloss.Style
	rule        lipgloss.Style
	artist      lipgloss.Style
	track       lipgloss.Style
	title       lipgloss.Style
	album       lipgloss.Style
	time        lipgloss.Style
	plain       lipgloss.Style
	back        lipgloss.Style
	selArtistBar lipgloss.Style
	selAlbumBar  lipgloss.Style
	selTimeBar   lipgloss.Style
	selPlainBar  lipgloss.Style
	topBar      lipgloss.Style
	divider     lipgloss.Style
	progress    lipgloss.Style
	status      lipgloss.Style
	focusHeader lipgloss.Style
	spinner     lipgloss.Style
	helpCard    lipgloss.Style
}

// styles builds the style set from the theme.
func (t Theme) styles() themeStyles {
	c := func(s string) lipgloss.Color { return lipgloss.Color(s) }
	selFg := c(t.SelFg)
	return themeStyles{
		colHeader: lipgloss.NewStyle().Bold(true).Foreground(c(t.Header)),
		rule:      lipgloss.NewStyle().Foreground(c(t.Rule)),
		artist:    lipgloss.NewStyle().Foreground(c(t.Artist)),
		track:     lipgloss.NewStyle().Foreground(c(t.Track)),
		title:     lipgloss.NewStyle().Foreground(c(t.Title)),
		album:     lipgloss.NewStyle().Foreground(c(t.Album)),
		time:      lipgloss.NewStyle().Foreground(c(t.Time)),
		plain:     lipgloss.NewStyle().Foreground(c(t.Plain)),
		back:      lipgloss.NewStyle().Foreground(c(t.Back)),
		selArtistBar: lipgloss.NewStyle().Foreground(selFg).Background(c(t.SelArtistBg)),
		selAlbumBar:  lipgloss.NewStyle().Foreground(selFg).Background(c(t.SelAlbumBg)),
		selTimeBar:   lipgloss.NewStyle().Foreground(selFg).Background(c(t.SelTimeBg)),
		selPlainBar:  lipgloss.NewStyle().Foreground(selFg).Background(c(t.SelAlbumBg)),
		topBar:      lipgloss.NewStyle().Foreground(c(t.Header)),
		divider:     lipgloss.NewStyle().Foreground(c(t.Rule)),
		progress:    lipgloss.NewStyle().Foreground(c(t.Progress)),
		status: lipgloss.NewStyle().
			Foreground(c(t.StatusFg)).
			Background(c(t.StatusBg)),
		focusHeader: lipgloss.NewStyle().Bold(true).Foreground(c(t.HeaderFocus)),
		spinner:     lipgloss.NewStyle().Foreground(c(t.Spinner)),
		helpCard: lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(c(t.HelpBorder)).
			Padding(0, 2).
			Background(c(t.HelpBg)).
			Foreground(c(t.HelpFg)),
	}
}
