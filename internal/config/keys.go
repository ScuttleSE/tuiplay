package config

import (
	"fmt"
	"sort"
)

// Action names for the key bindings. Each action maps to one key.
const (
	ActQuit          = "quit"
	ActHideRight     = "hide_right"
	ActShowNav       = "show_nav"
	ActShowSearch    = "show_search"
	ActShowLyrics    = "show_lyrics"
	ActFocusToggle   = "focus_toggle"
	ActUp            = "up"
	ActDown          = "down"
	ActPageUp        = "page_up"
	ActPageDown      = "page_down"
	ActHome          = "home"
	ActEnd           = "end"
	ActSelect        = "select"
	ActAdd           = "add"
	ActAddAll        = "add_all"
	ActConsume       = "consume"
	ActRepeat        = "repeat"
	ActShuffle       = "shuffle"
	ActCrossfade     = "crossfade"
	ActCrossfadeSet  = "crossfade_set"
	ActSavePlaylist  = "save_playlist"
	ActSaveOverwrite = "save_overwrite"
	ActDelete        = "delete"
	ActClear         = "clear"
	ActSongInfo      = "song_info"
	ActArtistInfo    = "artist_info"
	ActHelp          = "help"
	ActPause         = "pause"
	ActStop          = "stop"
	ActNext          = "next"
	ActPrev          = "prev"
	ActSeekForward   = "seek_forward"
	ActSeekBack      = "seek_back"
	ActVolumeUp      = "volume_up"
	ActVolumeDown    = "volume_down"
	ActRateUp        = "rate_up"
	ActRateDown      = "rate_down"
	ActVisualizer    = "visualizer"
	ActEdit          = "edit"
	ActRadio         = "radio"
	ActRadioSet      = "radio_set"
)

// DefaultKeys returns the built-in action-to-key map.
func DefaultKeys() map[string]string {
	return map[string]string{
		ActQuit:          "q",
		ActHideRight:     "1",
		ActShowNav:       "2",
		ActShowSearch:    "3",
		ActShowLyrics:    "4",
		ActFocusToggle:   "tab",
		ActUp:            "k",
		ActDown:          "j",
		ActPageUp:        "pgup",
		ActPageDown:      "pgdown",
		ActHome:          "home",
		ActEnd:           "end",
		ActSelect:        "enter",
		ActAdd:           "a",
		ActAddAll:        "space",
		ActConsume:       "R",
		ActRepeat:        "r",
		ActShuffle:       "z",
		ActCrossfade:     "x",
		ActCrossfadeSet:  "X",
		ActSavePlaylist:  "S",
		ActSaveOverwrite: "ctrl+s",
		ActDelete:        "delete",
		ActClear:         "c",
		ActSongInfo:      "i",
		ActArtistInfo:    "I",
		ActHelp:          "f1",
		ActPause:         "p",
		ActStop:          "s",
		ActNext:          ">",
		ActPrev:          "<",
		ActSeekForward:   "f",
		ActSeekBack:      "b",
		ActVolumeUp:      "+",
		ActVolumeDown:    "-",
		ActRateUp:        "§",
		ActRateDown:      "½",
		ActVisualizer:    "5",
		ActEdit:          "e",
		ActRadio:         "V",
		ActRadioSet:      "v",
	}
}

// knownAction reports whether name is a valid action.
func knownAction(name string) bool {
	_, ok := DefaultKeys()[name]
	return ok
}

// ResolveKeys builds the final action-to-key map. It starts from the
// defaults and applies the overrides from the [keys] section. It returns
// an error if an override names an unknown action, uses an empty key, or
// if two actions resolve to the same key.
func ResolveKeys(overrides map[string]string) (map[string]string, error) {
	out := DefaultKeys()

	// Apply overrides in a stable order for a deterministic error.
	names := make([]string, 0, len(overrides))
	for name := range overrides {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		if !knownAction(name) {
			return nil, fmt.Errorf("config: unknown key action %q", name)
		}
		key := overrides[name]
		if key == "" {
			return nil, fmt.Errorf("config: action %q has an empty key", name)
		}
		out[name] = key
	}

	// Check for two actions bound to the same key.
	seen := map[string]string{}
	actions := make([]string, 0, len(out))
	for a := range out {
		actions = append(actions, a)
	}
	sort.Strings(actions)
	for _, a := range actions {
		key := out[a]
		if other, ok := seen[key]; ok {
			return nil, fmt.Errorf("config: key %q is bound to both %q and %q", key, other, a)
		}
		seen[key] = a
	}
	return out, nil
}
