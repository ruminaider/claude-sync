package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
)

func TestAppModel_InlineExecution_SetsExecuting(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: 3,
		Profiles:      []string{"work"},
		Plugins:       []commands.PluginInfo{{Name: "test", Status: "upstream"}},
	}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	// Simulate action start
	updated, _ := m.Update(actionStartMsg{itemIndex: 0, actionID: "pull"})
	app := updated.(AppModel)
	assert.True(t, app.executing)
	assert.Equal(t, "pull", app.executingActionID)
}

func TestAppModel_InlineExecution_HandlesResult(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.executing = true
	m.executingActionID = "pull"

	updated, _ := m.Update(actionResultMsg{
		itemIndex: 0,
		actionID:  "pull",
		success:   true,
		message:   "Pulled 3 commits",
	})
	app := updated.(AppModel)
	assert.False(t, app.executing)
	result, ok := app.executionResults["pull"]
	assert.True(t, ok)
	assert.True(t, result.success)
	assert.Equal(t, "Pulled 3 commits", result.message)
}

func TestAppModel_InlineExecution_HandlesError(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.executing = true

	updated, _ := m.Update(actionResultMsg{
		itemIndex: 0,
		actionID:  "pull",
		success:   false,
		message:   "",
		err:       fmt.Errorf("merge conflict"),
	})
	app := updated.(AppModel)
	assert.False(t, app.executing)
	result := app.executionResults["pull"]
	assert.False(t, result.success)
	assert.Error(t, result.err)
}

func TestAppModel_IgnoresEnterWhileExecuting(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.executing = true
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.True(t, app.executing) // still executing
	assert.Nil(t, cmd)            // no new command dispatched
}

func TestAppModel_EnterOnInlineAction_StartsExecution(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: 3,
	}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.actionCursor = 0 // "Pull and apply now" (inline action)

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.True(t, app.executing)
	assert.Equal(t, "pull", app.executingActionID)
	assert.NotNil(t, cmd) // executeAction command was returned
}

func TestAppModel_EnterOnNonInlineAction_NoExecution(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasPending:   true,
	}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	// "Review and decide" is not inline (no inline flag)
	m.actionCursor = 0

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)
	assert.False(t, app.executing) // should not be executing
	assert.Nil(t, cmd)             // no command for non-inline
}

func TestAppModel_ResultRebuildsRecommendations(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: 3,
	}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)
	m.executing = true
	m.executingActionID = "pull"

	// After result, recommendations should be rebuilt from re-detected state
	updated, _ := m.Update(actionResultMsg{
		itemIndex: 0,
		actionID:  "pull",
		success:   true,
		message:   "Config is up to date",
	})
	app := updated.(AppModel)
	assert.False(t, app.executing)
	// Recommendations and intents should be non-nil (rebuilt)
	assert.NotNil(t, app.recommendations)
	assert.NotNil(t, app.intents)
}

func TestFormatPullResult_NoChanges(t *testing.T) {
	r := &commands.PullResult{}
	assert.Equal(t, "Config is up to date", formatPullResult(r))
}

func TestFormatPullResult_WithInstalls(t *testing.T) {
	r := &commands.PullResult{
		ToInstall: []string{"a", "b"},
	}
	result := formatPullResult(r)
	assert.Contains(t, result, "2 plugin(s) installed")
}

func TestFormatPullResult_WithRemovals(t *testing.T) {
	r := &commands.PullResult{
		ToRemove: []string{"a"},
	}
	result := formatPullResult(r)
	assert.Contains(t, result, "1 plugin(s) removed")
}

func TestFormatPullResult_WithSettings(t *testing.T) {
	r := &commands.PullResult{
		SettingsApplied: []string{"theme"},
	}
	result := formatPullResult(r)
	assert.Contains(t, result, "settings updated")
}

func TestFormatPullResult_PendingHighRisk(t *testing.T) {
	r := &commands.PullResult{
		PendingHighRisk: []approval.Change{
			{Category: "permissions", Description: "test"},
		},
	}
	result := formatPullResult(r)
	assert.Contains(t, result, "pending high-risk")
}

func TestFormatPullResult_Combined(t *testing.T) {
	r := &commands.PullResult{
		ToInstall:       []string{"a"},
		SettingsApplied: []string{"theme"},
	}
	result := formatPullResult(r)
	assert.Contains(t, result, "1 plugin(s) installed")
	assert.Contains(t, result, "settings updated")
	assert.True(t, len(result) > 0)
}

func TestFormatApproveResult_Empty(t *testing.T) {
	r := &commands.ApproveResult{}
	assert.Equal(t, "Changes approved", formatApproveResult(r))
}

func TestFormatApproveResult_WithPermissions(t *testing.T) {
	r := &commands.ApproveResult{PermissionsApplied: true}
	result := formatApproveResult(r)
	assert.Contains(t, result, "permissions applied")
}

func TestFormatApproveResult_WithHooks(t *testing.T) {
	r := &commands.ApproveResult{HooksApplied: []string{"hook1"}}
	result := formatApproveResult(r)
	assert.Contains(t, result, "1 hook(s) applied")
}

func TestFormatApproveResult_WithMCP(t *testing.T) {
	r := &commands.ApproveResult{MCPApplied: []string{"server1", "server2"}}
	result := formatApproveResult(r)
	assert.Contains(t, result, "2 MCP server(s) applied")
}

func TestFormatApproveResult_Combined(t *testing.T) {
	r := &commands.ApproveResult{
		PermissionsApplied: true,
		HooksApplied:       []string{"hook1"},
		MCPApplied:         []string{"s1"},
	}
	result := formatApproveResult(r)
	assert.Contains(t, result, "permissions applied")
	assert.Contains(t, result, "1 hook(s) applied")
	assert.Contains(t, result, "1 MCP server(s) applied")
}

func TestRenderActions_ShowsExecutingState(t *testing.T) {
	recs := []recommendation{
		{icon: "\u26a0", title: "Test", action: actionItem{id: "pull", label: "Pull now", inline: true}},
	}
	intents := buildIntents(commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"w"},
		Plugins:      []commands.PluginInfo{{Name: "t", Status: "upstream"}},
	})
	results := map[string]actionResultMsg{}

	// Test executing state
	view := renderActionsWithState(recs, intents, 0, 70, 30, true, "pull", results)
	assert.Contains(t, view, "Pull now") // action label still visible (in spinner text)

	// Test success result
	results["pull"] = actionResultMsg{actionID: "pull", success: true, message: "Done!"}
	view = renderActionsWithState(recs, intents, 0, 70, 30, false, "", results)
	assert.Contains(t, view, "Done!")

	// Test error result
	delete(results, "pull")
	results["pull"] = actionResultMsg{actionID: "pull", success: false, err: fmt.Errorf("failed")}
	view = renderActionsWithState(recs, intents, 0, 70, 30, false, "", results)
	assert.Contains(t, view, "failed")
}

func TestRenderActions_ShowsIntentExecutingState(t *testing.T) {
	recs := []recommendation{}
	intents := []intent{
		{
			label: "Push my local changes",
			hint:  "enter",
			action: actionItem{
				id:     "push-changes",
				label:  "Push my local changes",
				inline: true,
			},
		},
	}
	results := map[string]actionResultMsg{}

	// Test executing state on an intent (keyed by action ID)
	view := renderActionsWithState(recs, intents, 0, 70, 30, true, "push-changes", results)
	assert.Contains(t, view, "Push my local changes")

	// Test result on an intent
	results["push-changes"] = actionResultMsg{actionID: "push-changes", success: true, message: "Changes pushed successfully"}
	view = renderActionsWithState(recs, intents, 0, 70, 30, false, "", results)
	assert.Contains(t, view, "Changes pushed successfully")
}

func TestExecuteAction_PanicRecovery(t *testing.T) {
	cmd := executeAction(0, "__test_panic", nil, "", "")
	msg := cmd()
	result := msg.(actionResultMsg)
	assert.False(t, result.success)
	assert.Contains(t, result.message, "internal error")
	assert.Contains(t, result.err.Error(), "panic")
}

func TestExecuteAction_UnknownAction(t *testing.T) {
	cmd := executeAction(0, "nonexistent", nil, "/tmp/claude", "/tmp/sync")
	msg := cmd()
	result := msg.(actionResultMsg)
	assert.False(t, result.success)
	assert.Error(t, result.err)
	assert.Contains(t, result.err.Error(), "unknown action")
}

func TestExecuteAction_ConflictsAction(t *testing.T) {
	cmd := executeAction(0, "conflicts", nil, "/tmp/claude", "/tmp/sync")
	msg := cmd()
	result := msg.(actionResultMsg)
	assert.True(t, result.success)
	assert.Contains(t, result.message, "not yet available")
}

func TestExecuteAction_PluginUpdateAction(t *testing.T) {
	cmd := executeAction(0, "plugin-update", []string{"test-plugin"}, "/tmp/claude", "/tmp/sync")
	msg := cmd()
	result := msg.(actionResultMsg)
	assert.True(t, result.success)
	assert.Contains(t, result.message, "not yet available")
}

func TestExecuteAction_ImportMCPAction(t *testing.T) {
	cmd := executeAction(0, "import-mcp", nil, "/tmp/claude", "/tmp/sync")
	msg := cmd()
	result := msg.(actionResultMsg)
	assert.True(t, result.success)
	assert.Contains(t, result.message, "not yet available")
}
