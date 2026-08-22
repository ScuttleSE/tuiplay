package ui

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"git.hemmalab.se/scuttle/tuiplay/internal/subsonic"
)

func TestToNSPStringCondition(t *testing.T) {
	b := &smartBuilder{
		name:       "Rock",
		conditions: []nspCondition{{field: "genre", operator: "is", value: "Rock"}},
	}
	data, err := b.toNSP()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["name"] != "Rock" {
		t.Errorf("name = %v", got["name"])
	}
	all, ok := got["all"].([]any)
	if !ok || len(all) != 1 {
		t.Fatalf("all = %v", got["all"])
	}
	cond := all[0].(map[string]any)
	is := cond["is"].(map[string]any)
	if is["genre"] != "Rock" {
		t.Errorf("genre = %v", is["genre"])
	}
}

func TestToNSPNumberRange(t *testing.T) {
	b := &smartBuilder{
		name:       "80s",
		conditions: []nspCondition{{field: "year", operator: "inTheRange", value: "1981,1990"}},
		sortField:  "year",
		order:      "desc",
		limit:      25,
	}
	data, err := b.toNSP()
	if err != nil {
		t.Fatal(err)
	}
	s := string(data)
	if !strings.Contains(s, "\"inTheRange\"") {
		t.Errorf("missing inTheRange: %s", s)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got["sort"] != "year" || got["order"] != "desc" {
		t.Errorf("sort/order = %v/%v", got["sort"], got["order"])
	}
	if got["limit"].(float64) != 25 {
		t.Errorf("limit = %v", got["limit"])
	}
	rng := got["all"].([]any)[0].(map[string]any)["inTheRange"].(map[string]any)["year"].([]any)
	if len(rng) != 2 || rng[0].(float64) != 1981 || rng[1].(float64) != 1990 {
		t.Errorf("range = %v", rng)
	}
}

func TestToNSPBoolAndDays(t *testing.T) {
	b := &smartBuilder{
		name: "Recent Loved",
		conditions: []nspCondition{
			{field: "loved", operator: "is", value: "true"},
			{field: "lastplayed", operator: "inTheLast", value: "30"},
		},
	}
	data, err := b.toNSP()
	if err != nil {
		t.Fatal(err)
	}
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	all := got["all"].([]any)
	loved := all[0].(map[string]any)["is"].(map[string]any)["loved"]
	if loved != true {
		t.Errorf("loved = %v (want bool true)", loved)
	}
	days := all[1].(map[string]any)["inTheLast"].(map[string]any)["lastplayed"]
	if days.(float64) != 30 {
		t.Errorf("days = %v", days)
	}
	// Omitted options must not appear.
	if _, ok := got["sort"]; ok {
		t.Errorf("sort should be omitted")
	}
	if _, ok := got["limit"]; ok {
		t.Errorf("limit should be omitted")
	}
}

func TestToNSPValidation(t *testing.T) {
	if _, err := (&smartBuilder{conditions: []nspCondition{{field: "genre", operator: "is", value: "x"}}}).toNSP(); err == nil {
		t.Error("expected error for missing name")
	}
	if _, err := (&smartBuilder{name: "x"}).toNSP(); err == nil {
		t.Error("expected error for no conditions")
	}
	if _, err := (&smartBuilder{name: "x", conditions: []nspCondition{{field: "year", operator: "is", value: "notanumber"}}}).toNSP(); err == nil {
		t.Error("expected error for bad number")
	}
}

func TestNSPFileFor(t *testing.T) {
	dir := t.TempDir()
	// A file whose JSON name differs from its filename stem.
	if err := os.WriteFile(filepath.Join(dir, "recent.nsp"), []byte(`{"name":"Recently Played","all":[]}`), 0644); err != nil {
		t.Fatal(err)
	}
	// A file matched only by its stem (no name field).
	if err := os.WriteFile(filepath.Join(dir, "Favourites.nsp"), []byte(`{"all":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	if got := nspFileFor(dir, "Recently Played"); got != filepath.Join(dir, "recent.nsp") {
		t.Errorf("name match = %q", got)
	}
	if got := nspFileFor(dir, "Favourites"); got != filepath.Join(dir, "Favourites.nsp") {
		t.Errorf("stem match = %q", got)
	}
	if got := nspFileFor(dir, "No Such Playlist"); got != "" {
		t.Errorf("expected no match, got %q", got)
	}
	if got := nspFileFor("", "Anything"); got != "" {
		t.Errorf("empty dir must return empty, got %q", got)
	}
}

func TestSlugify(t *testing.T) {
	cases := map[string]string{
		"Rock & Roll":   "Rock-Roll.nsp",
		"  spaced  ":    "spaced.nsp",
		"../etc/passwd": "etcpasswd.nsp",
		"":              "playlist.nsp",
		"a/b\\c":        "abc.nsp",
		"Multi   Space": "Multi-Space.nsp",
	}
	for in, want := range cases {
		if got := slugify(in); got != want {
			t.Errorf("slugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSearchFormMatches(t *testing.T) {
	f := newSearchForm()
	f.set(sfArtist, "electric")
	f.set(sfDate, "1978")
	yes := subsonic.Song{Artist: "Electric Light Orchestra", Title: "Mr. Blue Sky", Year: 1978}
	no := subsonic.Song{Artist: "Electric Light Orchestra", Title: "x", Year: 2001}
	if !f.matches(yes) {
		t.Error("expected match")
	}
	if f.matches(no) {
		t.Error("expected no match on year")
	}
	if f.query() != "electric 1978" {
		t.Errorf("query = %q", f.query())
	}
}

func TestParseNSPRoundTrip(t *testing.T) {
	orig := &smartBuilder{
		name: "Mix",
		conditions: []nspCondition{
			{field: "genre", operator: "is", value: "Rock"},
			{field: "year", operator: "inTheRange", value: "1981,1990"},
			{field: "loved", operator: "is", value: "true"},
			{field: "playcount", operator: "gt", value: "5"},
			{field: "lastplayed", operator: "inTheLast", value: "30"},
			{field: "filepath", operator: "contains", value: "Live/"},
		},
		sortField: "year",
		order:     "desc",
		limit:     50,
	}
	data, err := orig.toNSP()
	if err != nil {
		t.Fatal(err)
	}
	got, err := parseNSP(data)
	if err != nil {
		t.Fatal(err)
	}
	if got.name != orig.name || got.sortField != orig.sortField ||
		got.order != orig.order || got.limit != orig.limit {
		t.Errorf("meta mismatch: %+v", got)
	}
	if len(got.conditions) != len(orig.conditions) {
		t.Fatalf("condition count = %d, want %d", len(got.conditions), len(orig.conditions))
	}
	for i, c := range orig.conditions {
		g := got.conditions[i]
		if g.field != c.field || g.operator != c.operator || g.value != c.value {
			t.Errorf("condition %d = %+v, want %+v", i, g, c)
		}
	}
}

func TestParseNSPRejectsUnknownField(t *testing.T) {
	data := []byte(`{"name":"X","all":[{"is":{"bitrate":320}}]}`)
	if _, err := parseNSP(data); err == nil {
		t.Fatalf("expected an error for an unsupported field")
	}
}

func TestParseNSPRejectsNestedGroup(t *testing.T) {
	data := []byte(`{"name":"X","all":[{"any":[{"is":{"loved":true}}]}]}`)
	if _, err := parseNSP(data); err == nil {
		t.Fatalf("expected an error for a nested group")
	}
}

func TestParseNSPRejectsEmpty(t *testing.T) {
	data := []byte(`{"name":"X","all":[]}`)
	if _, err := parseNSP(data); err == nil {
		t.Fatalf("expected an error for no conditions")
	}
}
