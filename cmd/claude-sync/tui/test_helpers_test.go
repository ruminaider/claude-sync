package tui

import tea "github.com/charmbracelet/bubbletea"

// testSendKey sends a rune key to a tea.Model and returns the updated model.
func testSendKey(m tea.Model, key string) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated
}

// testSendSpecial sends a special key (Enter, Esc, arrow, etc.) to a tea.Model
// and returns the updated model.
func testSendSpecial(m tea.Model, key tea.KeyType) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	return updated
}
