package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// nspValueType names the value type of a smart-playlist field. It decides
// which operators are valid and how the value serializes.
type nspValueType int

const (
	nspString nspValueType = iota
	nspNumber
	nspBool
	nspDays // a number of days, used by inTheLast/notInTheLast
)

// nspField describes one selectable field of the rule builder.
type nspField struct {
	key   string // the JSON field name, e.g. "title"
	label string // the display label
	typ   nspValueType
}

// nspFields is the curated set of fields the builder supports.
var nspFields = []nspField{
	{"title", "Title", nspString},
	{"album", "Album", nspString},
	{"artist", "Artist", nspString},
	{"genre", "Genre", nspString},
	{"year", "Year", nspNumber},
	{"loved", "Loved", nspBool},
	{"rating", "Rating", nspNumber},
	{"playcount", "Play count", nspNumber},
	{"lastplayed", "Last played", nspDays},
	{"dateadded", "Date added", nspDays},
	{"filepath", "Filepath", nspString},
}

// nspFieldByKey returns the field for a key, or false.
func nspFieldByKey(key string) (nspField, bool) {
	for _, f := range nspFields {
		if f.key == key {
			return f, true
		}
	}
	return nspField{}, false
}

// operatorsFor returns the valid operators for a value type.
func operatorsFor(t nspValueType) []string {
	switch t {
	case nspString:
		return []string{"is", "isNot", "contains", "notContains", "startsWith", "endsWith"}
	case nspNumber:
		return []string{"is", "isNot", "gt", "lt", "inTheRange"}
	case nspBool:
		return []string{"is"}
	case nspDays:
		return []string{"inTheLast", "notInTheLast"}
	}
	return nil
}

// nspCondition is one rule condition: a field, an operator, and a value.
type nspCondition struct {
	field    string // a key from nspFields
	operator string
	value    string // the raw value the user typed
}

// smartBuilder holds the state of the rule builder.
type smartBuilder struct {
	name       string
	conditions []nspCondition
	sortField  string // empty means no sort
	order      string // "asc" or "desc"; empty means the default
	limit      int    // 0 means no limit

	// origFile is the path of the .nsp file the builder was loaded from,
	// when the builder edits an existing smart playlist. It is empty for a
	// new builder. On save, a rename removes this old file.
	origFile string
}

// newSmartBuilder returns an empty builder.
func newSmartBuilder() *smartBuilder {
	return &smartBuilder{}
}

// summary returns a short "field op value" string for a condition.
func (c nspCondition) summary() string {
	f, ok := nspFieldByKey(c.field)
	label := c.field
	if ok {
		label = f.label
	}
	v := c.value
	if v == "" {
		v = "?"
	}
	return fmt.Sprintf("%s %s %s", label, c.operator, v)
}

// encodeValue turns the raw string value into the JSON value for a field
// type and operator. inTheRange takes two numbers separated by a comma or a
// dash. It returns an error when the value does not parse.
func encodeValue(t nspValueType, operator, raw string) (any, error) {
	raw = strings.TrimSpace(raw)
	switch t {
	case nspString:
		return raw, nil
	case nspBool:
		switch strings.ToLower(raw) {
		case "true", "yes", "1", "":
			return true, nil
		case "false", "no", "0":
			return false, nil
		}
		return nil, fmt.Errorf("expected true or false, got %q", raw)
	case nspNumber:
		if operator == "inTheRange" {
			lo, hi, err := parseRange(raw)
			if err != nil {
				return nil, err
			}
			return []int{lo, hi}, nil
		}
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("expected a number, got %q", raw)
		}
		return n, nil
	case nspDays:
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("expected a number of days, got %q", raw)
		}
		return n, nil
	}
	return nil, fmt.Errorf("unknown field type")
}

// parseRange parses "lo,hi" or "lo-hi" into two integers.
func parseRange(raw string) (int, int, error) {
	sep := ","
	if !strings.Contains(raw, ",") && strings.Contains(raw, "-") {
		sep = "-"
	}
	parts := strings.SplitN(raw, sep, 2)
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("expected a range like 1981,1990")
	}
	lo, err := strconv.Atoi(strings.TrimSpace(parts[0]))
	if err != nil {
		return 0, 0, fmt.Errorf("bad range start %q", parts[0])
	}
	hi, err := strconv.Atoi(strings.TrimSpace(parts[1]))
	if err != nil {
		return 0, 0, fmt.Errorf("bad range end %q", parts[1])
	}
	return lo, hi, nil
}

// toNSP serializes the builder to the Navidrome .nsp JSON. It validates
// that the builder has a name and at least one complete condition.
func (b *smartBuilder) toNSP() ([]byte, error) {
	if strings.TrimSpace(b.name) == "" {
		return nil, fmt.Errorf("the playlist needs a name")
	}
	if len(b.conditions) == 0 {
		return nil, fmt.Errorf("add at least one condition")
	}

	all := make([]map[string]map[string]any, 0, len(b.conditions))
	for _, c := range b.conditions {
		f, ok := nspFieldByKey(c.field)
		if !ok {
			return nil, fmt.Errorf("unknown field %q", c.field)
		}
		if c.operator == "" {
			return nil, fmt.Errorf("condition on %q has no operator", f.label)
		}
		val, err := encodeValue(f.typ, c.operator, c.value)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", f.label, err)
		}
		all = append(all, map[string]map[string]any{
			c.operator: {c.field: val},
		})
	}

	// Build an ordered object. encoding/json sorts map keys, which is fine
	// for Navidrome, but we assemble a struct-like map for clarity.
	out := map[string]any{
		"name": b.name,
		"all":  all,
	}
	if b.sortField != "" {
		out["sort"] = b.sortField
		if b.order != "" {
			out["order"] = b.order
		}
	}
	if b.limit > 0 {
		out["limit"] = b.limit
	}
	return json.MarshalIndent(out, "", "  ")
}

// parseNSP parses a Navidrome .nsp file into a builder. It reverses toNSP.
// It reads the name, the top-level "all" conditions, and the sort, order,
// and limit. It supports only the curated field set and the flat "all"
// group that this builder writes. A nested group or an unknown field is an
// error, so the caller can fall back rather than silently drop rules.
func parseNSP(data []byte) (*smartBuilder, error) {
	var raw struct {
		Name  string            `json:"name"`
		All   []json.RawMessage `json:"all"`
		Sort  string            `json:"sort"`
		Order string            `json:"order"`
		Limit int               `json:"limit"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("bad .nsp JSON: %w", err)
	}
	b := &smartBuilder{
		name:      raw.Name,
		sortField: raw.Sort,
		order:     raw.Order,
		limit:     raw.Limit,
	}
	if b.sortField != "" {
		if _, ok := nspFieldByKey(b.sortField); !ok {
			return nil, fmt.Errorf("unsupported sort field %q", b.sortField)
		}
	}
	for _, entry := range raw.All {
		cond, err := parseCondition(entry)
		if err != nil {
			return nil, err
		}
		b.conditions = append(b.conditions, cond)
	}
	if len(b.conditions) == 0 {
		return nil, fmt.Errorf("no conditions to edit")
	}
	return b, nil
}

// parseCondition parses one {operator: {field: value}} object.
func parseCondition(entry json.RawMessage) (nspCondition, error) {
	var outer map[string]json.RawMessage
	if err := json.Unmarshal(entry, &outer); err != nil {
		return nspCondition{}, fmt.Errorf("bad condition: %w", err)
	}
	if len(outer) != 1 {
		return nspCondition{}, fmt.Errorf("unsupported condition shape")
	}
	for op, inner := range outer {
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(inner, &fields); err != nil {
			return nspCondition{}, fmt.Errorf("unsupported condition (nested group?)")
		}
		if len(fields) != 1 {
			return nspCondition{}, fmt.Errorf("unsupported condition shape")
		}
		for field, valRaw := range fields {
			f, ok := nspFieldByKey(field)
			if !ok {
				return nspCondition{}, fmt.Errorf("unsupported field %q", field)
			}
			val, err := decodeValue(f.typ, valRaw)
			if err != nil {
				return nspCondition{}, fmt.Errorf("%s: %w", f.label, err)
			}
			return nspCondition{field: field, operator: op, value: val}, nil
		}
	}
	return nspCondition{}, fmt.Errorf("empty condition")
}

// decodeValue turns a JSON value back into the raw string the builder holds.
// It reverses encodeValue for each field type.
func decodeValue(t nspValueType, raw json.RawMessage) (string, error) {
	switch t {
	case nspString:
		var s string
		if err := json.Unmarshal(raw, &s); err != nil {
			return "", fmt.Errorf("expected a string")
		}
		return s, nil
	case nspBool:
		var v bool
		if err := json.Unmarshal(raw, &v); err != nil {
			return "", fmt.Errorf("expected a boolean")
		}
		if v {
			return "true", nil
		}
		return "false", nil
	case nspDays:
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			return "", fmt.Errorf("expected a number of days")
		}
		return strconv.Itoa(n), nil
	case nspNumber:
		// A range serializes as [lo, hi]; a plain number as a scalar.
		var arr []int
		if json.Unmarshal(raw, &arr) == nil && len(arr) == 2 {
			return fmt.Sprintf("%d,%d", arr[0], arr[1]), nil
		}
		var n int
		if err := json.Unmarshal(raw, &n); err != nil {
			return "", fmt.Errorf("expected a number")
		}
		return strconv.Itoa(n), nil
	}
	return "", fmt.Errorf("unknown field type")
}

// slugify turns a playlist name into a safe .nsp filename.

// Row ID sentinels for the smart-playlist builder level.
const (
	newSmartRowID = "pl:new_smart" // the [New smart playlist] row on Playlists
	sbRowName     = "sb:name"
	sbRowAdd      = "sb:add"
	sbRowSort     = "sb:sort"
	sbRowOrder    = "sb:order"
	sbRowLimit    = "sb:limit"
	sbRowSave     = "sb:save"
	sbCondPrefix  = "sb:cond:" // followed by the condition index
)

// smartBuilderLevel builds the builder level from the current state. It
// rebuilds the rows so they reflect the name, conditions, and options.
func smartBuilderLevel(b *smartBuilder) navLevel {
	var rows []navRow
	name := b.name
	if name == "" {
		name = "<unnamed>"
	}
	rows = append(rows, navRow{label: "Name: " + name, id: sbRowName})
	for i, c := range b.conditions {
		rows = append(rows, navRow{label: "  " + c.summary(), id: fmt.Sprintf("%s%d", sbCondPrefix, i)})
	}
	rows = append(rows, navRow{label: "[+ Add condition]", id: sbRowAdd})

	sortLabel := "Sort: none"
	if b.sortField != "" {
		if f, ok := nspFieldByKey(b.sortField); ok {
			sortLabel = "Sort: " + f.label
		} else {
			sortLabel = "Sort: " + b.sortField
		}
	}
	order := b.order
	if order == "" {
		order = "asc"
	}
	limitLabel := "Limit: none"
	if b.limit > 0 {
		limitLabel = "Limit: " + strconv.Itoa(b.limit)
	}
	rows = append(rows,
		navRow{label: sortLabel, id: sbRowSort},
		navRow{label: "Order: " + order, id: sbRowOrder},
		navRow{label: limitLabel, id: sbRowLimit},
		navRow{label: "Save", id: sbRowSave},
	)
	title := "New smart playlist"
	if b.origFile != "" {
		title = "Edit smart playlist"
	}
	return navLevel{kind: navSmartBuilder, title: title, rows: rows, cursor: 0, builder: b}
}

// smartFieldLevel lists the fields to pick for a condition or a sort.
func smartFieldLevel(title string) navLevel {
	var rows []navRow
	for _, f := range nspFields {
		rows = append(rows, navRow{label: f.label, id: f.key})
	}
	return navLevel{kind: navSmartField, title: title, rows: rows, cursor: 0}
}

// smartOpLevel lists the operators valid for a field type.
func smartOpLevel(field nspField) navLevel {
	var rows []navRow
	for _, op := range operatorsFor(field.typ) {
		rows = append(rows, navRow{label: op, id: op})
	}
	return navLevel{kind: navSmartOp, title: "Operator for " + field.label, rows: rows, cursor: 0}
}

// slugify turns a playlist name into a safe .nsp filename.
func slugify(name string) string {
	name = strings.TrimSpace(name)
	var b strings.Builder
	for _, r := range name {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-' || r == '_':
			b.WriteRune('-')
		default:
			// drop path separators and other unsafe characters
		}
	}
	slug := strings.Trim(b.String(), "-")
	// collapse repeated dashes
	for strings.Contains(slug, "--") {
		slug = strings.ReplaceAll(slug, "--", "-")
	}
	if slug == "" {
		slug = "playlist"
	}
	return slug + ".nsp"
}

// nspFileFor finds the .nsp file in dir that backs the playlist named name.
// It matches on each file's "name" field, then on the filename stem. It
// returns an empty string when no file matches. An empty dir returns "".
func nspFileFor(dir, name string) string {
	if dir == "" {
		return ""
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	want := strings.TrimSpace(strings.ToLower(name))
	var stemMatch string
	for _, e := range entries {
		if e.IsDir() || !strings.EqualFold(filepath.Ext(e.Name()), ".nsp") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		// Prefer a match on the JSON "name" field.
		if data, err := os.ReadFile(path); err == nil {
			var head struct {
				Name string `json:"name"`
			}
			if json.Unmarshal(data, &head) == nil && head.Name != "" {
				if strings.EqualFold(strings.TrimSpace(head.Name), name) {
					return path
				}
			}
		}
		// Remember a filename-stem match as a fallback.
		stem := strings.TrimSuffix(e.Name(), filepath.Ext(e.Name()))
		if strings.ToLower(stem) == want || strings.EqualFold(e.Name(), slugify(name)) {
			stemMatch = path
		}
	}
	return stemMatch
}
