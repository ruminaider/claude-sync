package tui

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
)

func TestBuildMenuItems_FreshInstall(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: false,
	}

	items := BuildMenuItems(state)

	// Fresh install should show exactly 2 items: Create and Join
	assert.Len(t, items, 2)
	assert.Equal(t, "Create new config", items[0].label)
	assert.Equal(t, "Join existing config", items[1].label)

	// Both should be TUI actions
	assert.Equal(t, ActionTUI, items[0].action.Type)
	assert.Equal(t, ActionTUI, items[1].action.Type)
}

func TestBuildMenuItems_Configured(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
	}

	items := BuildMenuItems(state)

	// Should have 5 categories: Sync, Config, Plugins, Profiles, Advanced
	assert.Len(t, items, 5)
	assert.Equal(t, "Sync", items[0].label)
	assert.Equal(t, "Config", items[1].label)
	assert.Equal(t, "Plugins", items[2].label)
	assert.Equal(t, "Profiles", items[3].label)
	assert.Equal(t, "Advanced", items[4].label)

	// Sync category should have 3 children
	assert.Len(t, items[0].children, 3)
	assert.Equal(t, "Pull latest config", items[0].children[0].label)
	assert.Equal(t, "Push local changes", items[0].children[1].label)
	assert.Equal(t, "View sync status", items[0].children[2].label)
}

func TestBuildMenuItems_PluginsCategory(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
	}

	items := BuildMenuItems(state)

	// Phase 1: Plugins should only show commands that don't require arguments
	plugins := items[2]
	assert.Len(t, plugins.children, 2)
	assert.Equal(t, "Subscribe", plugins.children[0].label)
	assert.Equal(t, "List subscriptions", plugins.children[1].label)
}

func TestBuildMenuItems_PendingApprovals(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasPending:   true,
	}

	items := BuildMenuItems(state)

	// Advanced category should include Approve and Reject
	advanced := items[4]
	labels := []string{}
	for _, child := range advanced.children {
		labels = append(labels, child.label)
	}
	assert.Contains(t, labels, "Approve pending changes")
	assert.Contains(t, labels, "Reject pending changes")
}

func TestBuildMenuItems_NoPendingApprovals(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasPending:   false,
	}

	items := BuildMenuItems(state)

	// Advanced category should NOT include Approve/Reject
	advanced := items[4]
	for _, child := range advanced.children {
		assert.NotEqual(t, "Approve pending changes", child.label)
		assert.NotEqual(t, "Reject pending changes", child.label)
	}
}

func TestBuildMenuItems_WithConflicts(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasConflicts: true,
	}

	items := BuildMenuItems(state)

	advanced := items[4]
	labels := []string{}
	for _, child := range advanced.children {
		labels = append(labels, child.label)
	}
	assert.Contains(t, labels, "Resolve conflicts")
}

func TestBuildMenuItems_NoConflicts(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasConflicts: false,
	}

	items := BuildMenuItems(state)

	advanced := items[4]
	for _, child := range advanced.children {
		assert.NotEqual(t, "Resolve conflicts", child.label)
	}
}

func TestBuildMenuItems_WithProfiles(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}

	items := BuildMenuItems(state)

	profiles := items[3]
	labels := []string{}
	for _, child := range profiles.children {
		labels = append(labels, child.label)
	}
	assert.Contains(t, labels, "List profiles")
	assert.Contains(t, labels, "Show active profile")
}
