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

// --- Filter mode tests ---

func TestAppModel_FilterMode_SlashEnters(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, Profiles: []string{"w"}, Plugins: []commands.PluginInfo{{Name: "t", Status: "upstream"}}}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	assert.True(t, app.filterMode)
	assert.Equal(t, "", app.filterText)
	assert.Equal(t, 0, app.actionCursor)
}

func TestAppModel_FilterMode_EscExits(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterMode = true
	m.filterText = "test"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app := updated.(AppModel)
	assert.False(t, app.filterMode)
	assert.Equal(t, "", app.filterText)
}

func TestAppModel_FilterMode_TypingAppendsText(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterMode = true
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("p")})
	app := updated.(AppModel)
	assert.Equal(t, "p", app.filterText)

	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("l")})
	app = updated.(AppModel)
	assert.Equal(t, "pl", app.filterText)
}

func TestAppModel_FilterMode_BackspaceRemovesChar(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterMode = true
	m.filterText = "plug"
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app := updated.(AppModel)
	assert.Equal(t, "plu", app.filterText)
}

func TestAppModel_FilterMode_CursorResetsOnKeystroke(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterMode = true
	m.actionCursor = 3
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	app := updated.(AppModel)
	assert.Equal(t, 0, app.actionCursor)
}

func TestAppModel_FilterMode_QDoesNotQuit(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterMode = true
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := updated.(AppModel)
	assert.False(t, app.quitting, "q should not quit while in filter mode")
	assert.Nil(t, cmd)
	assert.Equal(t, "q", app.filterText) // q typed into filter
}

func TestAppModel_FilterMode_EscWithFilterTextClearsFilter(t *testing.T) {
	// When there's active filter text and not in filterMode,
	// esc should clear the filter text instead of going to dashboard.
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterText = "plug"
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app := updated.(AppModel)
	assert.Equal(t, viewActions, app.activeView, "should stay on actions view")
	assert.Equal(t, "", app.filterText, "filter text should be cleared")
}

// --- Filter function tests ---

func TestFilterRecommendations(t *testing.T) {
	recs := []recommendation{
		{title: "Config is behind", detail: "3 commits"},
		{title: "Plugin update available"},
	}
	filtered := filterRecommendations(recs, "plugin")
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered[0].title, "Plugin")
}

func TestFilterRecommendations_MatchesDetail(t *testing.T) {
	recs := []recommendation{
		{title: "Config is behind", detail: "3 commits behind remote"},
		{title: "Plugin update available"},
	}
	filtered := filterRecommendations(recs, "commits")
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered[0].detail, "commits")
}

func TestFilterRecommendations_MatchesActionLabel(t *testing.T) {
	recs := []recommendation{
		{title: "Config is behind", action: actionItem{label: "Pull and apply now"}},
		{title: "Plugin update available", action: actionItem{label: "Update to v2"}},
	}
	filtered := filterRecommendations(recs, "pull")
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered[0].action.label, "Pull")
}

func TestFilterRecommendations_EmptyQuery(t *testing.T) {
	recs := []recommendation{{title: "test1"}, {title: "test2"}}
	assert.Len(t, filterRecommendations(recs, ""), 2)
}

func TestFilterRecommendations_CaseInsensitive(t *testing.T) {
	recs := []recommendation{
		{title: "Plugin Update Available"},
	}
	filtered := filterRecommendations(recs, "PLUGIN")
	assert.Len(t, filtered, 1)
}

func TestFilterIntents(t *testing.T) {
	intents := []intent{
		{label: "Add or discover new plugins"},
		{label: "Switch settings profile"},
		{label: "Push my local changes"},
	}
	filtered := filterIntents(intents, "plug")
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered[0].label, "plugins")
}

func TestFilterIntents_EmptyQuery(t *testing.T) {
	intents := []intent{{label: "test1"}, {label: "test2"}}
	assert.Len(t, filterIntents(intents, ""), 2)
}

func TestFilterIntents_NoMatch(t *testing.T) {
	intents := []intent{
		{label: "Add plugins"},
		{label: "Push changes"},
	}
	filtered := filterIntents(intents, "zzzzz")
	assert.Len(t, filtered, 0)
}

// --- Help overlay tests ---

func TestAppModel_HelpOverlay_QuestionMarkOpens(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	app := updated.(AppModel)
	assert.True(t, app.showHelp)
}

func TestAppModel_HelpOverlay_AnyKeyCloses(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.showHelp = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	app := updated.(AppModel)
	assert.False(t, app.showHelp)
}

func TestAppModel_HelpOverlay_EscCloses(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.showHelp = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app := updated.(AppModel)
	assert.False(t, app.showHelp)
}

func TestAppModel_HelpOverlay_QDoesNotQuit(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.showHelp = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := updated.(AppModel)
	assert.False(t, app.quitting, "q should dismiss help, not quit")
	assert.False(t, app.showHelp)
	assert.Nil(t, cmd)
}

func TestAppModel_HelpOverlay_ViewContainsHelpText(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.showHelp = true
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "Help")
	assert.Contains(t, view, "Navigation")
	assert.Contains(t, view, "Actions")
	assert.Contains(t, view, "press any key to close")
}

func TestAppModel_FilterMode_ViewShowsFilterBar(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterMode = true
	m.filterText = "plug"
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "plug")
}

func TestAppModel_FilterMode_NoResults(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.filterText = "zzzznonexistent"
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "No matching actions")
}
