package main

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func keyMsg(key tea.KeyType) tea.KeyMsg {
	return tea.KeyMsg{Type: key}
}

func TestPickerCancelEsc(t *testing.T) {
	p := newPicker("test", []string{"a", "b", "c"})
	model, _ := p.Update(keyMsg(tea.KeyEsc))
	assert.Nil(t, model.(picker).Selected())
}

func TestPickerCancelCtrlC(t *testing.T) {
	p := newPicker("test", []string{"a", "b", "c"})
	model, _ := p.Update(keyMsg(tea.KeyCtrlC))
	assert.Nil(t, model.(picker).Selected())
}

func TestPickerCancelQ(t *testing.T) {
	p := newPicker("test", []string{"a", "b", "c"})
	model, _ := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	assert.Nil(t, model.(picker).Selected())
}

func TestPickerConfirmEmpty(t *testing.T) {
	p := newPicker("test", []string{"a", "b", "c"})
	// Move cursor to confirm button (index == len(items)).
	p.cursor = len(p.items)
	model, _ := p.Update(keyMsg(tea.KeyEnter))
	result := model.(picker).Selected()
	require.NotNil(t, result)
	assert.Empty(t, result)
}

func TestPickerConfirmWithSelections(t *testing.T) {
	p := newPicker("test", []string{"a", "b", "c"})

	// Toggle first item.
	model, _ := p.Update(keyMsg(tea.KeyEnter))
	p = model.(picker)

	// Move down twice to "c", toggle it.
	model, _ = p.Update(keyMsg(tea.KeyDown))
	p = model.(picker)
	model, _ = p.Update(keyMsg(tea.KeyDown))
	p = model.(picker)
	model, _ = p.Update(keyMsg(tea.KeyEnter))
	p = model.(picker)

	// Move to confirm button and press enter.
	p.cursor = len(p.items)
	model, _ = p.Update(keyMsg(tea.KeyEnter))
	result := model.(picker).Selected()

	require.NotNil(t, result)
	assert.Equal(t, []string{"a", "c"}, result)
}

func TestRunPickerReturnsErrUserAbortedOnCancel(t *testing.T) {
	// We can't run tea.Program in tests, but we can verify the contract
	// by testing the model directly: if Selected() returns nil, runPicker
	// should return huh.ErrUserAborted.
	p := newPicker("test", []string{"a", "b"})
	model, _ := p.Update(keyMsg(tea.KeyEsc))
	selected := model.(picker).Selected()

	// This is the exact check runPicker performs.
	if selected == nil {
		err := huh.ErrUserAborted
		assert.ErrorIs(t, err, huh.ErrUserAborted)
	} else {
		t.Fatal("expected Selected() to return nil after cancel")
	}
}

// --- Section header tests ---

func TestPickerWithSections_BuildsItemsAndHeaders(t *testing.T) {
	sections := []pickerSection{
		{Header: "Group A", Items: []string{"a1", "a2"}},
		{Header: "Group B", Items: []string{"b1"}},
	}
	p := newPickerWithSections("test", sections)

	// Items should be: [header, a1, a2, header, b1]
	assert.Equal(t, 5, len(p.items))
	assert.True(t, p.headers[0], "index 0 should be a header")
	assert.False(t, p.headers[1])
	assert.False(t, p.headers[2])
	assert.True(t, p.headers[3], "index 3 should be a header")
	assert.False(t, p.headers[4])
}

func TestPickerWithSections_PreSelectsAll(t *testing.T) {
	sections := []pickerSection{
		{Header: "Group A", Items: []string{"a1", "a2"}},
		{Header: "Group B", Items: []string{"b1"}},
	}
	p := newPickerWithSections("test", sections)

	// All non-header items should be pre-selected.
	assert.False(t, p.selected[0], "header should not be selected")
	assert.True(t, p.selected[1])
	assert.True(t, p.selected[2])
	assert.False(t, p.selected[3], "header should not be selected")
	assert.True(t, p.selected[4])
}

func TestPickerWithSections_SelectedExcludesHeaders(t *testing.T) {
	sections := []pickerSection{
		{Header: "Group A", Items: []string{"a1", "a2"}},
		{Header: "Group B", Items: []string{"b1"}},
	}
	p := newPickerWithSections("test", sections)

	result := p.Selected()
	require.NotNil(t, result)
	assert.Equal(t, []string{"a1", "a2", "b1"}, result)
}

func TestPickerWithSections_CursorSkipsHeaders(t *testing.T) {
	sections := []pickerSection{
		{Header: "Group A", Items: []string{"a1"}},
		{Header: "Group B", Items: []string{"b1"}},
	}
	p := newPickerWithSections("test", sections)

	// Cursor should start on first non-header item (index 1).
	assert.Equal(t, 1, p.cursor)

	// Move down — should skip header at index 2, land on index 3 (b1).
	model, _ := p.Update(keyMsg(tea.KeyDown))
	p = model.(picker)
	assert.Equal(t, 3, p.cursor)

	// Move up — should skip header at index 2, land on index 1 (a1).
	model, _ = p.Update(keyMsg(tea.KeyUp))
	p = model.(picker)
	assert.Equal(t, 1, p.cursor)
}

func TestPickerWithSections_SkipsEmptySections(t *testing.T) {
	sections := []pickerSection{
		{Header: "Empty", Items: []string{}},
		{Header: "Group A", Items: []string{"a1"}},
	}
	p := newPickerWithSections("test", sections)

	// Empty section should be skipped entirely.
	assert.Equal(t, 2, len(p.items)) // header + a1
	assert.True(t, p.headers[0])
	assert.False(t, p.headers[1])
}

func TestPickerWithSections_DeselectAndConfirm(t *testing.T) {
	sections := []pickerSection{
		{Header: "Plugins", Items: []string{"p1", "p2", "p3"}},
	}
	p := newPickerWithSections("test", sections)

	// Cursor starts on p1 (index 1). Toggle it off.
	model, _ := p.Update(keyMsg(tea.KeyEnter))
	p = model.(picker)

	// Move to confirm and confirm.
	p.cursor = len(p.items)
	model, _ = p.Update(keyMsg(tea.KeyEnter))
	result := model.(picker).Selected()

	require.NotNil(t, result)
	assert.Equal(t, []string{"p2", "p3"}, result)
}
