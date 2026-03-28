package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- Constructor tests ---

func TestSubscribeFlow_InitialState(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	assert.Equal(t, 0, m.step, "should start at step 0 (URL input)")
	assert.Empty(t, m.repoURL, "repoURL should be empty initially")
	assert.False(t, m.cancelled)
	assert.False(t, m.executing)
	assert.False(t, m.resultDone)
	assert.Equal(t, 80, m.width)
	assert.Equal(t, 40, m.height)
}

// --- Step 0: URL input tests ---

func TestSubscribeFlow_EscCancels(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	flow := updated.(*SubscribeFlow)
	assert.True(t, flow.cancelled)

	// Should return a subViewCloseMsg command
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok, "should emit subViewCloseMsg")
	assert.False(t, closeMsg.refreshState)
}

func TestSubscribeFlow_EmptyURLStaysAtStep0(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	// URL input is empty by default
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*SubscribeFlow)
	assert.Equal(t, 0, flow.step, "should stay at step 0 with empty URL")
	assert.Empty(t, flow.repoURL)
	assert.Nil(t, cmd)
}

func TestSubscribeFlow_WhitespaceURLStaysAtStep0(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.urlInput.SetValue("   ")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*SubscribeFlow)
	assert.Equal(t, 0, flow.step, "should stay at step 0 with whitespace-only URL")
	assert.Nil(t, cmd)
}

func TestSubscribeFlow_ValidURLAdvancesToConfirm(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.urlInput.SetValue("user/config-repo")
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*SubscribeFlow)
	assert.Equal(t, 1, flow.step, "should advance to step 1 (confirm)")
	assert.Equal(t, "user/config-repo", flow.repoURL)
	assert.Nil(t, cmd)
}

func TestSubscribeFlow_URLTrimmed(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.urlInput.SetValue("  user/config-repo  ")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*SubscribeFlow)
	assert.Equal(t, 1, flow.step)
	assert.Equal(t, "user/config-repo", flow.repoURL)
}

// --- Step 1: confirm tests ---

func TestSubscribeFlow_ConfirmNo_Cancels(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.step = 1
	m.repoURL = "user/config-repo"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	flow := updated.(*SubscribeFlow)
	assert.True(t, flow.cancelled)

	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok, "should emit subViewCloseMsg on 'n'")
	assert.False(t, closeMsg.refreshState)
}

func TestSubscribeFlow_ConfirmEsc_Cancels(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.step = 1
	m.repoURL = "user/config-repo"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	flow := updated.(*SubscribeFlow)
	assert.True(t, flow.cancelled)

	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok, "should emit subViewCloseMsg on esc from confirm")
	assert.False(t, closeMsg.refreshState)
}

func TestSubscribeFlow_ConfirmYes_StartsExecution(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.step = 1
	m.repoURL = "user/config-repo"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	flow := updated.(*SubscribeFlow)
	assert.True(t, flow.executing, "should set executing to true")
	assert.NotNil(t, cmd, "should return a command for execution")
}

func TestSubscribeFlow_ConfirmEnter_StartsExecution(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.step = 1
	m.repoURL = "user/config-repo"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	flow := updated.(*SubscribeFlow)
	assert.True(t, flow.executing, "enter should also confirm")
	assert.NotNil(t, cmd)
}

// --- Result handling tests ---

func TestSubscribeFlow_SuccessResult(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.executing = true

	updated, _ := m.Update(subscribeResultMsg{success: true, message: "Subscribed OK"})
	flow := updated.(*SubscribeFlow)
	assert.True(t, flow.resultDone)
	assert.True(t, flow.resultOk)
	assert.False(t, flow.executing)
	assert.Contains(t, flow.resultMsg, "Subscribed OK")
}

func TestSubscribeFlow_ErrorResult(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.executing = true

	updated, _ := m.Update(subscribeResultMsg{success: false, message: "network error"})
	flow := updated.(*SubscribeFlow)
	assert.True(t, flow.resultDone)
	assert.False(t, flow.resultOk)
	assert.Contains(t, flow.resultMsg, "network error")
}

func TestSubscribeFlow_AnyKeyDismissesSuccessResult(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.resultDone = true
	m.resultOk = true
	m.resultMsg = "Subscribed OK"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = updated.(*SubscribeFlow)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.True(t, closeMsg.refreshState, "success should refresh state")
}

func TestSubscribeFlow_AnyKeyDismissesErrorResult(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.resultDone = true
	m.resultOk = false
	m.resultMsg = "failed"

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = updated.(*SubscribeFlow)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState, "failure should NOT refresh state")
}

// --- View tests ---

func TestSubscribeFlow_ViewRendersURLPrompt(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	view := m.View()
	assert.Contains(t, view, "repo", "step 0 view should mention repo")
	assert.Contains(t, view, "URL", "step 0 view should mention URL")
	assert.Contains(t, view, "esc", "step 0 view should show esc hint")
	assert.Contains(t, view, "enter", "step 0 view should show enter hint")
}

func TestSubscribeFlow_ViewRendersConfirmation(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.step = 1
	m.repoURL = "ruminaider/shared-config"
	view := m.View()
	assert.Contains(t, view, "ruminaider/shared-config", "confirm view should show the entered URL")
	assert.Contains(t, view, "Subscribe to", "confirm view should show subscribe prompt")
}

func TestSubscribeFlow_ViewRendersResult(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.resultDone = true
	m.resultOk = true
	m.resultMsg = "Run 'claude-sync subscribe user/repo' to complete"
	view := m.View()
	assert.Contains(t, view, "Subscribe")
	assert.Contains(t, view, "Run 'claude-sync subscribe user/repo' to complete")
	assert.Contains(t, view, "press any key")
}

func TestSubscribeFlow_ViewRendersFailedResult(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	m.resultDone = true
	m.resultOk = false
	m.resultMsg = "Could not access repo"
	view := m.View()
	assert.Contains(t, view, "failed")
	assert.Contains(t, view, "Could not access repo")
}

// --- Window resize test ---

func TestSubscribeFlow_WindowResize(t *testing.T) {
	m := NewSubscribeFlow(80, 40)
	updated, cmd := m.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	flow := updated.(*SubscribeFlow)
	assert.Equal(t, 120, flow.width)
	assert.Equal(t, 50, flow.height)
	assert.Nil(t, cmd)
}

// --- resolveSubscribeResultMsg tests ---

func TestResolveSubscribeResultMsg_Success(t *testing.T) {
	msg := resolveSubscribeResultMsg(true, "all good", nil)
	assert.Equal(t, "all good", msg)
}

func TestResolveSubscribeResultMsg_ErrorWithMessage(t *testing.T) {
	msg := resolveSubscribeResultMsg(false, "custom error", nil)
	assert.Equal(t, "custom error", msg)
}

func TestResolveSubscribeResultMsg_ErrorWithoutMessage(t *testing.T) {
	msg := resolveSubscribeResultMsg(false, "", assert.AnError)
	assert.Equal(t, assert.AnError.Error(), msg)
}

func TestResolveSubscribeResultMsg_UnknownError(t *testing.T) {
	msg := resolveSubscribeResultMsg(false, "", nil)
	assert.Equal(t, "Unknown error", msg)
}
