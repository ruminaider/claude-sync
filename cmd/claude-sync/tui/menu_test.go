package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sendKey(m tea.Model, key string) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated
}

func sendSpecialKey(m tea.Model, key tea.KeyType) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	return updated
}

func TestMenuModel_FreshInstall_InitialState(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)

	assert.Equal(t, 0, m.cursor)
	assert.Len(t, m.stack, 0, "should start at top level with empty stack")
	assert.False(t, m.Quitting)
	assert.Equal(t, MenuAction{}, m.Selected)
}

func TestMenuModel_Navigation_DownUp(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)

	// Move down
	var model tea.Model = m
	model = sendKey(model, "j")
	menu := model.(MenuModel)
	assert.Equal(t, 1, menu.cursor)

	// Move up
	model = sendKey(model, "k")
	menu = model.(MenuModel)
	assert.Equal(t, 0, menu.cursor)
}

func TestMenuModel_Navigation_Wraparound(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state) // 2 items: Create, Join

	var model tea.Model = m

	// At top, press up -> should stay at 0 (no wrap)
	model = sendKey(model, "k")
	menu := model.(MenuModel)
	assert.Equal(t, 0, menu.cursor)

	// At bottom, press down -> should stay at last (no wrap)
	model = sendKey(model, "j") // cursor=1
	model = sendKey(model, "j") // should stay at 1
	menu = model.(MenuModel)
	assert.Equal(t, 1, menu.cursor)
}

func TestMenuModel_SelectLeafAction(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)

	var model tea.Model = m

	// Select "Create new config" (cursor=0, enter)
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	menu := model.(MenuModel)

	assert.Equal(t, ActionConfigCreate, menu.Selected.ID)
	assert.Equal(t, ActionTUI, menu.Selected.Type)
	// Should issue tea.Quit
	require.NotNil(t, cmd)
}

func TestMenuModel_DrillIntoCategory(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state) // 5 categories

	var model tea.Model = m

	// Enter "Sync" category (cursor=0)
	model = sendSpecialKey(model, tea.KeyEnter)
	menu := model.(MenuModel)

	// Should push onto the stack and show children
	assert.Len(t, menu.stack, 1)
	assert.Equal(t, 0, menu.cursor)
	assert.Equal(t, "Sync", menu.stack[0].title)
}

func TestMenuModel_EscGoesBack(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m

	// Drill into Sync
	model = sendSpecialKey(model, tea.KeyEnter)
	menu := model.(MenuModel)
	assert.Len(t, menu.stack, 1)

	// Press Esc to go back
	model = sendSpecialKey(model, tea.KeyEscape)
	menu = model.(MenuModel)
	assert.Len(t, menu.stack, 0, "should return to top level")
}

func TestMenuModel_EscFromTopLevel_Quits(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	menu := model.(MenuModel)

	assert.True(t, menu.Quitting)
	require.NotNil(t, cmd)
}

func TestMenuModel_QuitFromTopLevel(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	menu := model.(MenuModel)

	assert.True(t, menu.Quitting)
	require.NotNil(t, cmd)
}

func TestMenuModel_SelectCLIAction(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m

	// Drill into Sync (cursor=0)
	model = sendSpecialKey(model, tea.KeyEnter)

	// Select "Pull latest config" (cursor=0)
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	menu := model.(MenuModel)

	assert.Equal(t, ActionPull, menu.Selected.ID)
	assert.Equal(t, ActionCLI, menu.Selected.Type)
	require.NotNil(t, cmd)
}

func TestMenuModel_ViewFreshInstall(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)
	m.width = 50
	m.height = 20

	view := m.View()
	assert.Contains(t, view, "claude-sync")
	assert.Contains(t, view, "Create new config")
	assert.Contains(t, view, "Join existing config")
}

func TestMenuModel_ViewConfigured(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.width = 50
	m.height = 20

	view := m.View()
	assert.Contains(t, view, "Sync")
	assert.Contains(t, view, "Config")
	assert.Contains(t, view, "Plugins")
	assert.Contains(t, view, "Profiles")
	assert.Contains(t, view, "Advanced")
}
