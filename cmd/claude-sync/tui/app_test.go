package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test helpers — separate from menu_test.go helpers to avoid conflicts
// when menu_test.go is eventually deleted.
func appSendKey(m tea.Model, key string) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated
}

func appSendSpecialKey(m tea.Model, key tea.KeyType) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	return updated
}

func TestAppModel_InitialState_IsDashboard(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	assert.Equal(t, viewDashboard, m.activeView)
	assert.False(t, m.quitting)
	assert.Equal(t, 0, m.dashboardScroll)
	assert.Equal(t, 0, m.actionCursor)
	assert.Equal(t, "", m.version)
}

func TestAppModel_EnterFromDashboard_GoesToActions(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	model = appSendSpecialKey(model, tea.KeyEnter)
	app := model.(AppModel)

	assert.Equal(t, viewActions, app.activeView)
}

func TestAppModel_EscFromActions_GoesToDashboard(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	// Go to actions first
	model = appSendSpecialKey(model, tea.KeyEnter)
	app := model.(AppModel)
	assert.Equal(t, viewActions, app.activeView)

	// Esc back to dashboard
	model = appSendSpecialKey(model, tea.KeyEscape)
	app = model.(AppModel)
	assert.Equal(t, viewDashboard, app.activeView)
}

func TestAppModel_QuitFromDashboard(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := model.(AppModel)

	assert.True(t, app.quitting)
	require.NotNil(t, cmd)
}

func TestAppModel_QuitFromActions(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	// Go to actions first
	model = appSendSpecialKey(model, tea.KeyEnter)

	// Quit with q
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := model.(AppModel)

	assert.True(t, app.quitting)
	require.NotNil(t, cmd)
}

func TestAppModel_CtrlC_Quits(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	app := model.(AppModel)

	assert.True(t, app.quitting)
	require.NotNil(t, cmd)
}

func TestAppModel_WindowResize(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	app := model.(AppModel)

	assert.Equal(t, 120, app.width)
	assert.Equal(t, 40, app.height)
}

func TestAppModel_DashboardScroll(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m

	// Scroll down with j
	model = appSendKey(model, "j")
	app := model.(AppModel)
	assert.Equal(t, 1, app.dashboardScroll)

	// Scroll down again
	model = appSendKey(model, "j")
	app = model.(AppModel)
	assert.Equal(t, 2, app.dashboardScroll)

	// Scroll up with k
	model = appSendKey(model, "k")
	app = model.(AppModel)
	assert.Equal(t, 1, app.dashboardScroll)

	// Scroll up past zero should stay at zero
	model = appSendKey(model, "k")
	model = appSendKey(model, "k")
	app = model.(AppModel)
	assert.Equal(t, 0, app.dashboardScroll)
}

func TestAppModel_ActionCursor(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m

	// Go to actions view first
	model = appSendSpecialKey(model, tea.KeyEnter)
	app := model.(AppModel)
	assert.Equal(t, viewActions, app.activeView)

	// Move cursor down with j
	model = appSendKey(model, "j")
	app = model.(AppModel)
	assert.Equal(t, 1, app.actionCursor)

	// Move cursor down again
	model = appSendKey(model, "j")
	app = model.(AppModel)
	assert.Equal(t, 2, app.actionCursor)

	// Move cursor up with k
	model = appSendKey(model, "k")
	app = model.(AppModel)
	assert.Equal(t, 1, app.actionCursor)

	// Move up past zero should stay at zero
	model = appSendKey(model, "k")
	model = appSendKey(model, "k")
	app = model.(AppModel)
	assert.Equal(t, 0, app.actionCursor)
}

func TestAppModel_SetVersion(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	m.SetVersion("1.2.3")
	assert.Equal(t, "1.2.3", m.version)
}

func TestAppModel_ViewDashboard_ShowsDashboard(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	view := m.View()
	assert.Contains(t, view, "claude-sync")
	assert.Contains(t, view, "enter")
	assert.Contains(t, view, "quit")
}

func TestAppModel_ViewActions_ShowsActionScreen(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	// Transition to actions
	var model tea.Model = m
	model = appSendSpecialKey(model, tea.KeyEnter)
	app := model.(AppModel)

	view := app.View()
	assert.Contains(t, view, "Needs attention")
	assert.Contains(t, view, "I want to")
}

func TestAppModel_EditConfig_SetsLaunchFlag(t *testing.T) {
	// Use a state with profiles and plugins so there are zero recommendations,
	// making the intent index predictable.
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"work"},
		Plugins:      []commands.PluginInfo{{Name: "test", Status: "upstream"}},
	}
	m := NewAppModel(state)

	// Transition to actions view
	var model tea.Model = m
	model = appSendSpecialKey(model, tea.KeyEnter)
	app := model.(AppModel)
	require.Equal(t, viewActions, app.activeView)

	// Find the "edit-config" intent index
	editIdx := -1
	for i, it := range app.intents {
		if it.action.id == "edit-config" {
			editIdx = len(app.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, editIdx, "edit-config intent not found")

	// Move cursor to the edit-config intent
	for app.actionCursor < editIdx {
		model = appSendKey(model, "j")
		app = model.(AppModel)
	}
	require.Equal(t, editIdx, app.actionCursor)

	// Press enter to select edit-config
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(AppModel)

	assert.True(t, app.LaunchConfigEditor, "LaunchConfigEditor should be true")
	assert.NotNil(t, cmd, "should return a tea.Quit command")
}

func TestAppModel_EditConfig_DoesNotSetQuitting(t *testing.T) {
	// Selecting edit-config should set LaunchConfigEditor but not quitting,
	// so the caller can distinguish between user quit and config editor launch.
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"work"},
		Plugins:      []commands.PluginInfo{{Name: "test", Status: "upstream"}},
	}
	m := NewAppModel(state)

	var model tea.Model = m
	model = appSendSpecialKey(model, tea.KeyEnter)
	app := model.(AppModel)

	// Find edit-config index
	editIdx := -1
	for i, it := range app.intents {
		if it.action.id == "edit-config" {
			editIdx = len(app.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, editIdx)

	// Navigate to edit-config
	for app.actionCursor < editIdx {
		model = appSendKey(model, "j")
		app = model.(AppModel)
	}

	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(AppModel)

	assert.True(t, app.LaunchConfigEditor)
	assert.False(t, app.quitting, "quitting should remain false so caller can distinguish")
}

func TestAppModel_LaunchConfigEditor_DefaultFalse(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	assert.False(t, m.LaunchConfigEditor, "LaunchConfigEditor should default to false")
}

func TestAppModel_NormalQuit_DoesNotSetLaunchFlag(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := model.(AppModel)

	assert.True(t, app.quitting)
	assert.False(t, app.LaunchConfigEditor, "normal quit should not set LaunchConfigEditor")
}
