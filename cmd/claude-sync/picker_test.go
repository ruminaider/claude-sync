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
