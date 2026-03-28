package tui

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
)

func TestBuildMenuItems_Configured(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	items := buildConfiguredMenu(state)

	// Expect 3 headers + 9 actions = 12 items total.
	assert.Len(t, items, 12)

	// Verify headers at the right positions.
	assert.True(t, items[0].isHeader, "item 0 should be Sync header")
	assert.Equal(t, "Sync", items[0].label)

	assert.True(t, items[3].isHeader, "item 3 should be Plugins header")
	assert.Equal(t, "Plugins", items[3].label)

	assert.True(t, items[8].isHeader, "item 8 should be Config header")
	assert.Equal(t, "Config", items[8].label)

	// Verify non-header items have action IDs.
	for _, it := range items {
		if it.isHeader {
			assert.Empty(t, it.actionID, "headers should have no action ID")
		} else {
			assert.NotEmpty(t, it.actionID, "non-headers should have an action ID: %s", it.label)
		}
	}
}

func TestBuildMenuItems_ConfiguredActionIDs(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	items := buildConfiguredMenu(state)

	ids := []string{}
	for _, it := range items {
		if !it.isHeader {
			ids = append(ids, it.actionID)
		}
	}

	expected := []string{
		ActionPull,
		ActionPush,
		ActionBrowsePlugins,
		ActionRemovePlugin,
		ActionForkPlugin,
		ActionPinPlugin,
		ActionProfileList,
		ActionConfigUpdate,
		ActionSubscribe,
	}
	assert.Equal(t, expected, ids)
}

func TestBuildMenuItems_SyncItemsAreCLI(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	items := buildConfiguredMenu(state)

	// Pull and Push should be CLI mode.
	assert.Equal(t, ModeCLI, items[1].mode, "Pull should be CLI")
	assert.Equal(t, ModeCLI, items[2].mode, "Push should be CLI")
}

func TestBuildMenuItems_PluginItemsAreTUI(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	items := buildConfiguredMenu(state)

	// Items 4-7 are plugin actions, all TUI.
	for i := 4; i <= 7; i++ {
		assert.Equal(t, ModeTUI, items[i].mode, "plugin item %d should be TUI", i)
	}
}

func TestBuildMenuItems_SyncItemsHaveHints(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	items := buildConfiguredMenu(state)

	assert.NotEmpty(t, items[1].hint, "Pull should have a hint")
	assert.NotEmpty(t, items[2].hint, "Push should have a hint")
}

func TestBuildMenuItems_HeadersAreNotSelectable(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	items := buildConfiguredMenu(state)

	for _, it := range items {
		if it.isHeader {
			assert.Empty(t, it.actionID, "header %q should have no action ID", it.label)
		}
	}
}

func TestBuildMenuItems_FreshInstall(t *testing.T) {
	items := buildFreshInstallMenu()
	assert.Len(t, items, 2)
	assert.Equal(t, "create-config", items[0].actionID)
	assert.Equal(t, "join-config", items[1].actionID)
	assert.NotEmpty(t, items[0].hint)
	assert.NotEmpty(t, items[1].hint)
}

func TestAllActionIDs_ContainsExpected(t *testing.T) {
	ids := AllActionIDs()
	expected := []string{
		ActionPull,
		ActionPush,
		ActionPushChanges,
		ActionBrowsePlugins,
		ActionRemovePlugin,
		ActionForkPlugin,
		ActionPinPlugin,
		ActionPluginUpdate,
		ActionProfileList,
		ActionConfigUpdate,
		ActionSubscribe,
		ActionApprove,
		ActionReject,
		ActionConflicts,
		ActionImportMCP,
	}
	assert.Equal(t, expected, ids)
}

func TestAllActionIDs_NoDuplicates(t *testing.T) {
	ids := AllActionIDs()
	seen := map[string]bool{}
	for _, id := range ids {
		assert.False(t, seen[id], "duplicate action ID: %s", id)
		seen[id] = true
	}
}
