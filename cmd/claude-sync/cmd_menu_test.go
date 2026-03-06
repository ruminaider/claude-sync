package main

import (
	"testing"

	"github.com/ruminaider/claude-sync/cmd/claude-sync/tui"
	"github.com/stretchr/testify/assert"
)

func TestDispatchAction_AllActionIDsHaveCases(t *testing.T) {
	// Verify every action ID constant from AllActionIDs() has a case in
	// dispatchAction. The commands will fail or panic (no sync dir, missing
	// args, etc.), but the dispatch routing should NOT return "unknown action".
	for _, id := range tui.AllActionIDs() {
		t.Run(id, func(t *testing.T) {
			action := tui.MenuAction{ID: id, Type: tui.ActionCLI}
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						// Command panicked due to missing state — that's OK,
						// it means the dispatch routed to a real command.
					}
				}()
				err = dispatchAction(nil, action)
			}()
			if err != nil {
				assert.NotContains(t, err.Error(), "unknown action",
					"action ID %q has no case in dispatchAction", id)
			}
		})
	}
}

func TestDispatchAction_UnknownAction(t *testing.T) {
	action := tui.MenuAction{ID: "nonexistent", Type: tui.ActionCLI}
	err := dispatchAction(nil, action)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "unknown action")
}

func TestDispatchAction_PluginPin_NoArgs(t *testing.T) {
	action := tui.MenuAction{ID: tui.ActionPluginPin, Type: tui.ActionCLI}
	err := dispatchAction(nil, action)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires a plugin key")
}

func TestDispatchAction_ProfileSet_EmptyArgs(t *testing.T) {
	action := tui.MenuAction{ID: tui.ActionProfileSet, Type: tui.ActionCLI}
	err := dispatchAction(nil, action)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires a profile name")
}

func TestDispatchAction_ProjectRemove_NoArgs(t *testing.T) {
	action := tui.MenuAction{ID: tui.ActionProjectRemove, Type: tui.ActionCLI}
	err := dispatchAction(nil, action)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires a project path")
}
