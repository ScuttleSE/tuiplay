package ui

import (
	"os"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// openSmartBuilder pushes a fresh smart-playlist builder level.
func (m model) openSmartBuilder() (tea.Model, tea.Cmd) {
	if m.nspPath == "" {
		return m.setTempStatus("Set nsp_path in the config to create smart playlists.")
	}
	m.pushLevel(smartBuilderLevel(newSmartBuilder()))
	m.focus = focusRight
	m.rightShown = true
	return m, nil
}

// editSmartPlaylist opens the builder loaded with the selected playlist's
// .nsp file. It reacts to the edit key on a Playlists row. It works only for
// a playlist that has a matching .nsp file that this builder can parse.
func (m model) editSmartPlaylist() (tea.Model, tea.Cmd) {
	if m.focus != focusRight {
		return m, nil
	}
	lvl := m.top()
	if lvl.kind != navPlaylists {
		return m, nil
	}
	row, ok := lvl.selected()
	if !ok || row.id == newSmartRowID {
		return m, nil
	}
	if m.nspPath == "" {
		return m.setTempStatus("Set nsp_path in the config to edit smart playlists.")
	}
	file := nspFileFor(m.nspPath, row.name)
	if file == "" {
		return m.setTempStatus("No .nsp file for this playlist; it is not a smart playlist.")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return m.setTempStatus("Cannot read .nsp file: " + err.Error())
	}
	b, err := parseNSP(data)
	if err != nil {
		return m.setTempStatus("Cannot edit this smart playlist: " + err.Error())
	}
	b.origFile = file
	m.pushLevel(smartBuilderLevel(b))
	m.focus = focusRight
	m.rightShown = true
	return m, nil
}

// rebuildSmartLevel refreshes the builder level rows after a state change.
// It keeps the cursor position when it can.
func (m *model) rebuildSmartLevel(b *smartBuilder) {
	cur := m.top().cursor
	*m.top() = smartBuilderLevel(b)
	if cur >= 0 && cur < len(m.top().rows) {
		m.top().cursor = cur
	}
}

// openSmartBuilderRow reacts to Enter on a builder row.
func (m model) openSmartBuilderRow(row navRow) (tea.Model, tea.Cmd) {
	lvl := m.top()
	b := lvl.builder
	if b == nil {
		return m, nil
	}
	switch {
	case row.id == sbRowName:
		m.promptActive = true
		m.promptKind = promptSmartName
		m.promptLabel = "Playlist name"
		m.promptInput = b.name
		return m, nil
	case row.id == sbRowAdd:
		// Start the guided flow to add a new condition.
		m.smartEditIdx = -1
		m.smartSortPick = false
		m.pushLevel(smartFieldLevel("Field for condition"))
		return m, nil
	case strings.HasPrefix(row.id, sbCondPrefix):
		idx, err := strconv.Atoi(strings.TrimPrefix(row.id, sbCondPrefix))
		if err != nil || idx < 0 || idx >= len(b.conditions) {
			return m, nil
		}
		// Re-edit this condition through the guided flow.
		m.smartEditIdx = idx
		m.smartSortPick = false
		m.pushLevel(smartFieldLevel("Field for condition"))
		return m, nil
	case row.id == sbRowSort:
		m.smartSortPick = true
		m.pushLevel(smartFieldLevel("Sort field"))
		return m, nil
	case row.id == sbRowOrder:
		if b.order == "desc" {
			b.order = "asc"
		} else {
			b.order = "desc"
		}
		m.rebuildSmartLevel(b)
		return m, nil
	case row.id == sbRowLimit:
		m.promptActive = true
		m.promptKind = promptSmartLimit
		m.promptLabel = "Limit (0 for none)"
		if b.limit > 0 {
			m.promptInput = strconv.Itoa(b.limit)
		} else {
			m.promptInput = ""
		}
		return m, nil
	case row.id == sbRowSave:
		if _, err := b.toNSP(); err != nil {
			return m.setTempStatus("Cannot save: " + err.Error())
		}
		m.popLevel() // leave the builder
		return m, m.writeNSP(b)
	}
	return m, nil
}

// openSmartFieldRow reacts to Enter on the field picker. It stores the
// chosen field, then either sets the sort or opens the operator picker.
func (m model) openSmartFieldRow(row navRow) (tea.Model, tea.Cmd) {
	field, ok := nspFieldByKey(row.id)
	if !ok {
		return m, nil
	}
	m.popLevel() // remove the field picker
	if m.smartSortPick {
		b := m.top().builder
		if b == nil {
			return m, nil
		}
		b.sortField = field.key
		m.smartSortPick = false
		m.rebuildSmartLevel(b)
		return m, nil
	}
	// Condition flow: remember the field, open the operator picker.
	m.smartEditField = field.key
	m.pushLevel(smartOpLevel(field))
	return m, nil
}

// openSmartOpRow reacts to Enter on the operator picker. It stores the
// operator, then opens the value prompt.
func (m model) openSmartOpRow(row navRow) (tea.Model, tea.Cmd) {
	op := row.id
	m.popLevel() // remove the operator picker; the builder is now on top
	field, ok := nspFieldByKey(m.smartEditField)
	if !ok {
		return m, nil
	}
	m.promptActive = true
	m.promptKind = promptSmartValue
	m.promptLabel = valuePromptLabel(field, op)
	m.promptInput = ""
	// Stash the operator in searchEditKey-like storage: reuse a field.
	m.smartEditOp = op
	return m, nil
}

// valuePromptLabel returns a helpful prompt label for a field and operator.
func valuePromptLabel(f nspField, op string) string {
	switch f.typ {
	case nspBool:
		return f.label + " (true/false)"
	case nspDays:
		return f.label + " days"
	case nspNumber:
		if op == "inTheRange" {
			return f.label + " range (e.g. 1981,1990)"
		}
		return f.label + " number"
	}
	return f.label + " value"
}
