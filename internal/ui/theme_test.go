package ui

import "testing"

func TestResolveThemeDefault(t *testing.T) {
	got, err := ResolveTheme("", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != builtinThemes["default"] {
		t.Fatalf("empty name did not resolve to default")
	}
}

func TestResolveThemeUnknown(t *testing.T) {
	if _, err := ResolveTheme("no-such-theme", nil); err == nil {
		t.Fatalf("expected an error for an unknown theme")
	}
}

func TestResolveThemeOverride(t *testing.T) {
	got, err := ResolveTheme("catppuccin-mocha", map[string]string{"album": "#123456"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Album != "#123456" {
		t.Fatalf("override not applied: got %q", got.Album)
	}
	// A non-overridden role keeps the base value.
	if got.Title != builtinThemes["catppuccin-mocha"].Title {
		t.Fatalf("non-overridden role changed")
	}
}

func TestResolveThemeOverrideCaseFolds(t *testing.T) {
	got, err := ResolveTheme("", map[string]string{"progress": "#ABCDEF"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Progress != "#abcdef" {
		t.Fatalf("hex not lower-cased: got %q", got.Progress)
	}
}

func TestResolveThemeBadRole(t *testing.T) {
	if _, err := ResolveTheme("", map[string]string{"nope": "#123456"}); err == nil {
		t.Fatalf("expected an error for an unknown role")
	}
}

func TestResolveThemeBadHex(t *testing.T) {
	cases := []string{"123456", "#12345", "#12345g", "red", ""}
	for _, v := range cases {
		if _, err := ResolveTheme("", map[string]string{"album": v}); err == nil {
			t.Fatalf("expected an error for bad hex %q", v)
		}
	}
}

// TestBuiltinThemesComplete checks that every shipped theme fills every role,
// so no role renders with an empty color.
func TestBuiltinThemesComplete(t *testing.T) {
	for name := range builtinThemes {
		th := builtinThemes[name]
		for _, r := range themeRoles {
			if v := *r.get(&th); v == "" {
				t.Errorf("theme %q leaves role %q empty", name, r.key)
			} else if !hexColor.MatchString(v) {
				t.Errorf("theme %q role %q has bad hex %q", name, r.key, v)
			}
		}
	}
}
