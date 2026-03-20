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

func TestAppModel_InitialState_IsMain(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	assert.Equal(t, viewMain, m.activeView)
	assert.False(t, m.quitting)
	assert.Equal(t, 0, m.actionCursor)
	assert.Equal(t, "", m.version)
}

func TestAppModel_InitialState_BuildsRecsAndIntents(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, CommitsBehind: 3}
	m := NewAppModel(state)

	assert.NotEmpty(t, m.recommendations, "recommendations should be built at init")
	assert.NotEmpty(t, m.intents, "intents should be built at init")
}

func TestAppModel_QuitFromMain(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := model.(AppModel)

	assert.True(t, app.quitting)
	require.NotNil(t, cmd)
}

func TestAppModel_EscFromMain_Quits(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app := model.(AppModel)

	assert.True(t, app.quitting)
	require.NotNil(t, cmd)
}

func TestAppModel_EscFromMain_WithFilterText_ClearsFilter(t *testing.T) {
	// When there's active filter text, esc should clear it instead of quitting.
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.filterText = "plug"

	var model tea.Model = m
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app := model.(AppModel)

	assert.Equal(t, viewMain, app.activeView, "should stay on main view")
	assert.Equal(t, "", app.filterText, "filter text should be cleared")
	assert.False(t, app.quitting, "should not quit when clearing filter")
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

func TestAppModel_ActionCursor(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	var model tea.Model = m

	// Move cursor down with j
	model = appSendKey(model, "j")
	app := model.(AppModel)
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

func TestAppModel_ViewMain_ShowsContent(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.SetVersion("0.7.0")

	view := m.View()
	assert.Contains(t, view, "claude-sync")
	assert.Contains(t, view, "0.7.0")
	assert.Contains(t, view, "I want to")
	assert.Contains(t, view, "quit")
}

func TestAppModel_ViewMain_ShowsSummaryAndActions(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		ConfigRepo:   "user/repo",
		CommitsBehind: 3,
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	// Summary section
	assert.Contains(t, view, "User config")
	assert.Contains(t, view, "user/repo")
	assert.Contains(t, view, "connected")
	// Recommendations
	assert.Contains(t, view, "Needs attention")
	// Intents
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

	var model tea.Model = m

	// Find the "edit-config" intent index
	editIdx := -1
	for i, it := range m.intents {
		if it.action.id == "edit-config" {
			editIdx = len(m.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, editIdx, "edit-config intent not found")

	// Move cursor to the edit-config intent
	app := model.(AppModel)
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

	// Find edit-config index
	editIdx := -1
	for i, it := range m.intents {
		if it.action.id == "edit-config" {
			editIdx = len(m.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, editIdx)

	// Navigate to edit-config
	app := model.(AppModel)
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

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	app := updated.(AppModel)
	assert.True(t, app.filterMode)
	assert.Equal(t, "", app.filterText)
	assert.Equal(t, 0, app.actionCursor)
}

func TestAppModel_FilterMode_EscExits(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
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
	m.filterMode = true

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
	m.filterMode = true
	m.filterText = "plug"

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	app := updated.(AppModel)
	assert.Equal(t, "plu", app.filterText)
}

func TestAppModel_FilterMode_CursorResetsOnKeystroke(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.filterMode = true
	m.actionCursor = 3

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	app := updated.(AppModel)
	assert.Equal(t, 0, app.actionCursor)
}

func TestAppModel_FilterMode_QDoesNotQuit(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.filterMode = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := updated.(AppModel)
	assert.False(t, app.quitting, "q should not quit while in filter mode")
	assert.Nil(t, cmd)
	assert.Equal(t, "q", app.filterText) // q typed into filter
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

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	app := updated.(AppModel)
	assert.True(t, app.showHelp)
}

func TestAppModel_HelpOverlay_AnyKeyCloses(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.showHelp = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	app := updated.(AppModel)
	assert.False(t, app.showHelp)
}

func TestAppModel_HelpOverlay_EscCloses(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.showHelp = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app := updated.(AppModel)
	assert.False(t, app.showHelp)
}

func TestAppModel_HelpOverlay_QDoesNotQuit(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
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
	m.filterMode = true
	m.filterText = "plug"
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "plug")
}

func TestAppModel_FilterMode_NoResults(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.filterText = "zzzznonexistent"
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "No matching actions")
}

// --- Fresh install flow tests ---

func TestAppModel_FreshInstall_CursorNavigation(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewAppModel(state)
	assert.Equal(t, 0, m.freshInstallCursor)

	// Move down
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app := updated.(AppModel)
	assert.Equal(t, 1, app.freshInstallCursor)

	// Move down again (should stay at 1)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	app = updated.(AppModel)
	assert.Equal(t, 1, app.freshInstallCursor)

	// Move up
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	app = updated.(AppModel)
	assert.Equal(t, 0, app.freshInstallCursor)

	// Move up again (should stay at 0)
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	app = updated.(AppModel)
	assert.Equal(t, 0, app.freshInstallCursor)
}

func TestAppModel_FreshInstall_CreateLaunchesEditor(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewAppModel(state)
	m.freshInstallCursor = 0 // Create

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.True(t, app.LaunchConfigEditor)
	assert.NotNil(t, cmd) // tea.Quit
}

func TestAppModel_FreshInstall_JoinLaunchesJoinFlow(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewAppModel(state)
	m.freshInstallCursor = 1 // Join

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.Equal(t, viewSubView, app.activeView)
	assert.NotNil(t, app.subView)
}

func TestRenderFreshInstall_ShowsCursor(t *testing.T) {
	view := renderFreshInstall(70, 30, "0.7.0", 0)
	assert.Contains(t, view, "Create")
	assert.Contains(t, view, "Join")
	assert.Contains(t, view, ">") // cursor on first item
}

func TestAppModel_FreshInstall_EnterDoesNotQuit(t *testing.T) {
	// In fresh install mode, Enter should NOT quit normally
	state := commands.MenuState{ConfigExists: false}
	m := NewAppModel(state)
	m.freshInstallCursor = 0

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	// Should either launch editor or join flow, NOT stay on main
	assert.True(t, app.LaunchConfigEditor || app.activeView == viewSubView)
}

func TestAppModel_FreshInstall_ArrowKeysWork(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewAppModel(state)

	// Down arrow
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	app := updated.(AppModel)
	assert.Equal(t, 1, app.freshInstallCursor)

	// Up arrow
	updated, _ = app.Update(tea.KeyMsg{Type: tea.KeyUp})
	app = updated.(AppModel)
	assert.Equal(t, 0, app.freshInstallCursor)
}

func TestRenderFreshInstall_CursorOnJoin(t *testing.T) {
	view := renderFreshInstall(70, 30, "0.7.0", 1)
	assert.Contains(t, view, "Create")
	assert.Contains(t, view, "Join")
	assert.Contains(t, view, ">")
}

// --- View plugins sub-view test ---

func TestAppModel_ViewPlugins_OpensSubView(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"work"},
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream", Marketplace: "beads-mkt"},
		},
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40

	// Find the "view-plugins" intent index
	viewIdx := -1
	for i, it := range m.intents {
		if it.action.id == "view-plugins" {
			viewIdx = len(m.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, viewIdx, "view-plugins intent not found")

	var model tea.Model = m
	// Navigate to it
	app := model.(AppModel)
	for app.actionCursor < viewIdx {
		model = appSendKey(model, "j")
		app = model.(AppModel)
	}

	// Press enter to select
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app = model.(AppModel)

	assert.Equal(t, viewSubView, app.activeView)
	assert.NotNil(t, app.subView)
}

// --- openSubView Init tests ---

func TestAppModel_OpenSubView_JoinConfig_ReturnsInitCmd(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40

	updated, cmd := m.openSubView("join-config")
	app := updated.(AppModel)
	assert.Equal(t, viewSubView, app.activeView)
	assert.NotNil(t, cmd, "JoinFlow.Init() returns textinput.Blink, should not be nil")
}

// --- Sub-view q key does not quit ---

func TestAppModel_QDoesNotQuitInSubView(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.subView = NewConfigDetails(state, 80, 40)
	m.activeView = viewSubView

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := updated.(AppModel)
	assert.False(t, app.quitting, "q should not quit when in sub-view")
	assert.Equal(t, viewSubView, app.activeView)
	assert.Nil(t, cmd)
}

// --- Sub-view close returns to main ---

func TestAppModel_SubViewClose_ReturnsToMain(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewSubView
	m.subView = NewConfigDetails(state, 80, 40)

	updated, _ := m.Update(subViewCloseMsg{refreshState: false})
	app := updated.(AppModel)

	assert.Equal(t, viewMain, app.activeView)
	assert.Nil(t, app.subView)
}
