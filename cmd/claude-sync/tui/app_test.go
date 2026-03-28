package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

func TestAppModel_WindowResize_ForwardedToSubView(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.subView = NewConfigDetails(state, 80, 40)
	m.activeView = viewSubView

	updated, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	app := updated.(AppModel)

	assert.Equal(t, 120, app.width)
	assert.Equal(t, 50, app.height)
	// Sub-view should also have updated dimensions
	details := app.subView.(ConfigDetails)
	assert.Equal(t, 120, details.width)
	assert.Equal(t, 50, details.height)
}

func TestAppModel_ActionCursor(t *testing.T) {
	// With no recommendations, cursor starts at 0 which is a header (Sync).
	// The first "j" should land on the first non-header intent.
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"w"},
		Plugins:      []commands.PluginInfo{{Name: "t", Status: "upstream"}},
	}
	m := NewAppModel(state)
	require.Empty(t, m.recommendations, "expect no recommendations for this state")

	var model tea.Model = m

	// Intent layout: [0]=Sync(H), [1]=Pull, [2]=Push, [3]=Plugins(H), [4]=Browse, ...
	// Move down from 0 => skips header at 0, lands on 1 (Pull)
	model = testSendKey(model, "j")
	app := model.(AppModel)
	assert.Equal(t, 1, app.actionCursor)

	// Move down again => 2 (Push)
	model = testSendKey(model, "j")
	app = model.(AppModel)
	assert.Equal(t, 2, app.actionCursor)

	// Move down again => skips header at 3, lands on 4 (Browse)
	model = testSendKey(model, "j")
	app = model.(AppModel)
	assert.Equal(t, 4, app.actionCursor)

	// Move up from 4 => skips header at 3, lands on 2 (Push)
	model = testSendKey(model, "k")
	app = model.(AppModel)
	assert.Equal(t, 2, app.actionCursor)

	// Move up => 1 (Pull)
	model = testSendKey(model, "k")
	app = model.(AppModel)
	assert.Equal(t, 1, app.actionCursor)

	// Move up from 1 => 0 is a header, cursor stays at 1 (headers are skipped)
	model = testSendKey(model, "k")
	app = model.(AppModel)
	assert.Equal(t, 1, app.actionCursor)
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
	assert.Contains(t, view, "Sync")
	assert.Contains(t, view, "Plugins")
	assert.Contains(t, view, "Config")
	assert.Contains(t, view, "quit")
}

func TestAppModel_ViewMain_ShowsSummaryAndActions(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		ConfigRepo:    "user/repo",
		CommitsBehind: 3,
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	// Summary section
	assert.Contains(t, view, "User config")
	assert.Contains(t, view, "user/repo")
	assert.Contains(t, view, "3 behind")
	// Recommendations
	assert.Contains(t, view, "Needs attention")
	// Grouped intent sections
	assert.Contains(t, view, "Sync")
	assert.Contains(t, view, "Plugins")
	assert.Contains(t, view, "Config")
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

	// Find the "edit-config" intent raw index (including headers)
	editIdx := -1
	for i, it := range m.intents {
		if it.action.id == ActionConfigUpdate {
			editIdx = len(m.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, editIdx, "edit-config intent not found")

	// Navigate to the edit-config intent using j (skips headers automatically)
	app := model.(AppModel)
	for app.actionCursor < editIdx {
		model = testSendKey(model, "j")
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

	// Find edit-config raw index
	editIdx := -1
	for i, it := range m.intents {
		if it.action.id == ActionConfigUpdate {
			editIdx = len(m.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, editIdx)

	// Navigate to edit-config
	app := model.(AppModel)
	for app.actionCursor < editIdx {
		model = testSendKey(model, "j")
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

// Filter function and help overlay tests are in actions_test.go

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

// --- Filter-mode Enter interaction tests ---

func TestAppModel_FilterMode_EnterOnInlineAction_StartsExecution(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: 3,
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.filterMode = true
	m.filterText = "Pull" // matches "Pull and apply now" (inline action)
	m.actionCursor = 0

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.False(t, app.filterMode, "filter mode should be exited")
	assert.True(t, app.executing, "inline action should start executing")
	assert.Equal(t, "pull", app.executingActionID)
	assert.NotNil(t, cmd, "executeAction command should be returned")
}

func TestAppModel_FilterMode_EnterOnNonInlineAction_OpensSubView(t *testing.T) {
	// Use a state with plugins and profiles so "Browse" only matches
	// the intent (not a recommendation about missing plugins).
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"w"},
		Plugins:      []commands.PluginInfo{{Name: "t", Status: "upstream"}},
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.filterMode = true
	m.filterText = "Browse" // matches "Browse & install plugins" (non-inline, opens sub-view)

	// With no recs matching "Browse", filtered list is:
	// [0] Plugins header, [1] Browse & install plugins
	// Cursor 1 => Browse action
	m.actionCursor = 1

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.False(t, app.filterMode, "filter mode should be exited")
	assert.False(t, app.executing, "non-inline action should not execute inline")
	assert.Equal(t, viewSubView, app.activeView, "should open sub-view")
	assert.NotNil(t, app.subView)
}

func TestAppModel_FilterMode_EnterOnNoResults_NoOp(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.filterMode = true
	m.filterText = "zzzznonexistent"
	m.actionCursor = 0

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.False(t, app.filterMode, "filter mode should still be exited")
	assert.False(t, app.executing)
	assert.Equal(t, viewMain, app.activeView)
	assert.Nil(t, cmd, "no command when nothing matches")
}

func TestAppModel_FilterMode_EnterWhileExecuting_NoOp(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: 3,
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.filterMode = true
	m.filterText = "Pull"
	m.executing = true // already executing
	m.actionCursor = 0

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.True(t, app.executing, "should still be executing")
	assert.Nil(t, cmd, "no new command dispatched while executing")
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

// --- Browse plugins sub-view test ---

func TestAppModel_BrowsePlugins_OpensSubView(t *testing.T) {
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

	// Find the "browse-plugins" intent index
	browseIdx := -1
	for i, it := range m.intents {
		if it.action.id == ActionBrowsePlugins {
			browseIdx = len(m.recommendations) + i
			break
		}
	}
	require.NotEqual(t, -1, browseIdx, "browse-plugins intent not found")

	var model tea.Model = m
	// Navigate to it
	app := model.(AppModel)
	for app.actionCursor < browseIdx {
		model = testSendKey(model, "j")
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
