package tui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestErrorGuidance_NilError(t *testing.T) {
	assert.Nil(t, ErrorGuidance(ActionPull, nil))
}

func TestErrorGuidance_NoMatch(t *testing.T) {
	assert.Nil(t, ErrorGuidance(ActionPull, fmt.Errorf("something unexpected")))
}

func TestErrorGuidance_PullNotFoundInMarketplace(t *testing.T) {
	help := ErrorGuidance(ActionPull, fmt.Errorf("plugin not found in marketplace: my-plugin"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "renamed or removed")
	assert.Contains(t, help.Action, "config.yaml")
}

func TestErrorGuidance_PullPluginNotFound(t *testing.T) {
	help := ErrorGuidance(ActionPull, fmt.Errorf("plugin not found"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "renamed or removed")
}

func TestErrorGuidance_Pull404(t *testing.T) {
	help := ErrorGuidance(ActionPull, fmt.Errorf("HTTP 404 fetching plugin"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "renamed or removed")
}

func TestErrorGuidance_PullNetwork(t *testing.T) {
	tests := []string{
		"could not resolve host github.com",
		"network is unreachable",
		"connection timed out: timeout",
		"connection refused",
		"authentication failed",
		"permission denied accessing repo",
		"access denied",
		"repository not found",
	}
	for _, errMsg := range tests {
		t.Run(errMsg, func(t *testing.T) {
			help := ErrorGuidance(ActionPull, fmt.Errorf("%s", errMsg))
			assert.NotNil(t, help)
			assert.Contains(t, help.Why, "could not be reached")
			assert.Contains(t, help.Action, "network")
		})
	}
}

func TestErrorGuidance_PullConflict(t *testing.T) {
	help := ErrorGuidance(ActionPull, fmt.Errorf("merge conflict in config.yaml"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "conflict")
	assert.Contains(t, help.Action, "conflicts")
}

func TestErrorGuidance_PushNonFastForward(t *testing.T) {
	help := ErrorGuidance(ActionPush, fmt.Errorf("non-fast-forward update rejected"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "changes you don't have")
	assert.Contains(t, help.Action, "Pull first")
}

func TestErrorGuidance_PushFetchFirst(t *testing.T) {
	help := ErrorGuidance("push-changes", fmt.Errorf("please fetch first"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "changes you don't have")
}

func TestErrorGuidance_PushPermissionDenied(t *testing.T) {
	help := ErrorGuidance(ActionPush, fmt.Errorf("permission denied to remote"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "No write access")
	assert.Contains(t, help.Action, "local")
}

func TestErrorGuidance_PushForbidden(t *testing.T) {
	help := ErrorGuidance(ActionPush, fmt.Errorf("forbidden: insufficient scope"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "No write access")
}

func TestErrorGuidance_PushNoChanges(t *testing.T) {
	help := ErrorGuidance(ActionPush, fmt.Errorf("no changes to push"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "matches")
}

func TestErrorGuidance_ApproveNoPending(t *testing.T) {
	help := ErrorGuidance(ActionApprove, fmt.Errorf("no pending changes"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "No high-risk")
	assert.Contains(t, help.Action, "Pull")
}

func TestErrorGuidance_ConflictsNone(t *testing.T) {
	help := ErrorGuidance(ActionConflicts, fmt.Errorf("no conflicts to resolve"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "resolved")
}

func TestErrorGuidance_ConfigUpdateNoConfig(t *testing.T) {
	help := ErrorGuidance(ActionConfigUpdate, fmt.Errorf("no config found"))
	assert.NotNil(t, help)
	assert.Contains(t, help.Why, "No config.yaml")
}

func TestErrorGuidance_UnknownAction(t *testing.T) {
	assert.Nil(t, ErrorGuidance("unknown-action", fmt.Errorf("some error")))
}

func TestErrorGuidance_RenderInRecSection(t *testing.T) {
	// Verify error guidance is rendered when an action result has an error
	recs := []recommendation{
		{icon: "\u26a0", title: "Test", action: actionItem{id: ActionPull, label: "Pull now", inline: true}},
	}
	results := map[string]actionResultMsg{
		ActionPull: {
			actionID: ActionPull,
			success:  false,
			err:      fmt.Errorf("plugin not found in marketplace: test-plugin"),
		},
	}
	view := renderRecsSectionWithState(recs, 0, 60, false, "", results)
	assert.Contains(t, view, "renamed or removed")
	assert.Contains(t, view, "config.yaml")
}

func TestErrorGuidance_RenderInIntentSection(t *testing.T) {
	// Verify error guidance is rendered when an intent action result has an error
	intents := []intent{
		{hint: "enter", action: actionItem{id: "push-changes", label: "Push", inline: true}},
	}
	results := map[string]actionResultMsg{
		"push-changes": {
			actionID: "push-changes",
			success:  false,
			err:      fmt.Errorf("non-fast-forward update rejected"),
		},
	}
	view := renderIntentsSectionWithState(intents, 0, 0, 60, false, "", results)
	assert.Contains(t, view, "changes you don't have")
	assert.Contains(t, view, "Pull first")
}
