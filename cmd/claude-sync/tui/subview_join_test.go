package tui

import (
	"fmt"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoinFlow_Step0_ShowsURLInput(t *testing.T) {
	m := NewJoinFlow(70, 30)
	assert.Equal(t, 0, m.step)
	view := m.View()
	assert.Contains(t, view, "Enter a config repo URL")
	assert.Contains(t, view, "user/repo")
	assert.Contains(t, view, "enter continue")
	assert.Contains(t, view, "esc back")
}

func TestJoinFlow_Step1_ShowsConfirmation(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.repoURL = "ruminaider/claude-sync-config"
	view := m.View()
	assert.Contains(t, view, "ruminaider/claude-sync-config")
	assert.Contains(t, view, "Join and apply this config")
	assert.Contains(t, view, "Clone the config repo")
	assert.Contains(t, view, "enter join")
}

func TestJoinFlow_EscFromStep0_Cancels(t *testing.T) {
	m := NewJoinFlow(70, 30)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	flow := updated.(*JoinFlow)
	assert.True(t, flow.cancelled)

	// Should return a subViewCloseMsg command
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState)
}

func TestJoinFlow_EscFromStep1_GoesBack(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.repoURL = "user/repo"
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	flow := updated.(*JoinFlow)
	assert.Equal(t, 0, flow.step)
	assert.False(t, flow.cancelled)
	assert.Empty(t, flow.repoURL)
	assert.Nil(t, cmd)
}

func TestJoinFlow_EnterOnStep0_AdvancesToStep1(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.urlInput.SetValue("user/repo")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*JoinFlow)
	assert.Equal(t, 1, flow.step)
	assert.Equal(t, "user/repo", flow.repoURL)
}

func TestJoinFlow_EnterOnStep0_EmptyURL_NoAdvance(t *testing.T) {
	m := NewJoinFlow(70, 30)
	// URL is empty
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*JoinFlow)
	assert.Equal(t, 0, flow.step) // stays on step 0
}

func TestJoinFlow_EnterOnStep0_WhitespaceURL_NoAdvance(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.urlInput.SetValue("   ")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*JoinFlow)
	assert.Equal(t, 0, flow.step)
}

func TestJoinFlow_EnterOnStep1_StartsExecution(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.repoURL = "user/repo"
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*JoinFlow)
	assert.True(t, flow.executing)
	assert.NotNil(t, cmd)
}

func TestJoinFlow_ExecutingView(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.repoURL = "user/repo"
	m.executing = true
	view := m.View()
	assert.Contains(t, view, "Joining user/repo")
}

func TestJoinFlow_IgnoresInputWhileExecuting(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.executing = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*JoinFlow)
	assert.True(t, flow.executing) // still executing
	assert.Nil(t, cmd)
}

func TestJoinFlow_JoinResult_Success(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.executing = true
	updated, _ := m.Update(joinResultMsg{success: true, message: "Joined successfully"})
	flow := updated.(*JoinFlow)
	assert.True(t, flow.resultDone)
	assert.True(t, flow.resultOk)
	assert.False(t, flow.executing)
	view := flow.View()
	assert.Contains(t, view, "Joined successfully")
	assert.Contains(t, view, "Press any key to go back")
}

func TestJoinFlow_JoinResult_Error(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.executing = true
	updated, _ := m.Update(joinResultMsg{success: false, err: fmt.Errorf("clone failed")})
	flow := updated.(*JoinFlow)
	assert.True(t, flow.resultDone)
	assert.False(t, flow.resultOk)
	view := flow.View()
	assert.Contains(t, view, "clone failed")
}

func TestJoinFlow_JoinResult_ErrorWithMessage(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.step = 1
	m.executing = true
	updated, _ := m.Update(joinResultMsg{success: false, message: "already joined"})
	flow := updated.(*JoinFlow)
	assert.True(t, flow.resultDone)
	assert.False(t, flow.resultOk)
	assert.Equal(t, "already joined", flow.resultMsg)
}

func TestJoinFlow_AnyKeyDismissesSuccessResult(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.resultDone = true
	m.resultOk = true
	m.resultMsg = "Joined successfully"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = updated.(*JoinFlow)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.True(t, closeMsg.refreshState) // success should refresh
}

func TestJoinFlow_AnyKeyDismissesErrorResult(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.resultDone = true
	m.resultOk = false
	m.resultMsg = "clone failed"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = updated.(*JoinFlow)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState) // failure should NOT refresh
}

func TestJoinFlow_URLTrimmed(t *testing.T) {
	m := NewJoinFlow(70, 30)
	m.urlInput.SetValue("  user/repo  ")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*JoinFlow)
	assert.Equal(t, 1, flow.step)
	assert.Equal(t, "user/repo", flow.repoURL)
}

func TestJoinFlow_ViewShowsHeader(t *testing.T) {
	m := NewJoinFlow(70, 30)
	view := m.View()
	assert.Contains(t, view, "Join a shared config")
}

// --- executeJoin function tests (exercises the tea.Cmd closure) ---

func TestExecuteJoin_InvalidURL(t *testing.T) {
	// Invalid repo URL → Join should fail
	cmd := executeJoin("not-a-valid-url", t.TempDir(), t.TempDir())
	msg := cmd()
	result := msg.(joinResultMsg)
	assert.False(t, result.success)
	assert.Error(t, result.err)
}

func TestExecuteJoin_EmptyURL(t *testing.T) {
	cmd := executeJoin("", t.TempDir(), t.TempDir())
	msg := cmd()
	result := msg.(joinResultMsg)
	assert.False(t, result.success)
	assert.Error(t, result.err)
}

func TestExecuteJoin_ReturnsCorrectMsgType(t *testing.T) {
	// Regardless of success/failure, the return type must be joinResultMsg
	cmd := executeJoin("user/repo", t.TempDir(), t.TempDir())
	msg := cmd()
	_, ok := msg.(joinResultMsg)
	assert.True(t, ok, "should return joinResultMsg")
}

// --- AppModel integration tests ---

func TestAppModel_JoinConfigIntent_OpensJoinFlow(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
	}
	m := NewAppModel(state)
	m.activeView = viewMain
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	// Find the join-config intent cursor position
	var joinIdx int
	for i, it := range m.intents {
		if it.action.id == "join-config" {
			joinIdx = len(m.recommendations) + i
			break
		}
	}
	m.actionCursor = joinIdx

	// Press enter
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)

	assert.Equal(t, viewSubView, app.activeView)
	assert.NotNil(t, app.subView)

	// The sub-view should render join flow content
	view := app.subView.View()
	assert.Contains(t, view, "Join a shared config")
	assert.Contains(t, view, "Enter a config repo URL")
}

func TestAppModel_JoinResultMsg_RoutesToSubView(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewSubView
	jf := NewJoinFlow(70, 30)
	jf.step = 1
	jf.executing = true
	m.subView = jf

	updated, _ := m.Update(joinResultMsg{success: true, message: "Joined successfully"})
	app := updated.(AppModel)

	// The result should have been routed to the sub-view
	assert.Equal(t, viewSubView, app.activeView)
	flow := app.subView.(*JoinFlow)
	assert.True(t, flow.resultDone)
	assert.Equal(t, "Joined successfully", flow.resultMsg)
}
