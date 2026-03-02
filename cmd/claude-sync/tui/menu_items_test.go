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

	// No profiles => Profiles category hidden => 5 categories: Sync, Config, Plugins, Projects, Advanced
	assert.Len(t, items, 5)
	assert.Equal(t, "Sync", items[0].label)
	assert.Equal(t, "Config", items[1].label)
	assert.Equal(t, "Plugins", items[2].label)
	assert.Equal(t, "Projects", items[3].label)
	assert.Equal(t, "Advanced", items[4].label)

	// Sync category should have 3 children
	assert.Len(t, items[0].children, 3)
	assert.Equal(t, "Pull latest config", items[0].children[0].label)
	assert.Equal(t, "Push local changes", items[0].children[1].label)
	assert.Equal(t, "View sync status", items[0].children[2].label)
}

func TestBuildMenuItems_PluginsCategory_NoPlugins(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
	}

	items := BuildMenuItems(state)

	// With no plugins, Plugins category should just show Subscribe + List
	plugins := items[2]
	assert.Len(t, plugins.children, 2)
	assert.Equal(t, "Subscribe", plugins.children[0].label)
	assert.Equal(t, "List subscriptions", plugins.children[1].label)
}

func TestBuildMenuItems_PluginFirstLayout(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Key: "beads@beads-marketplace", Name: "beads", Status: "upstream"},
			{Key: "superpowers@claude-plugins-official", Name: "superpowers", Status: "pinned", PinVersion: "1.2.3"},
			{Key: "my-tool@claude-sync-forks", Name: "my-tool", Status: "forked"},
		},
	}

	items := BuildMenuItems(state)
	plugins := items[2]

	// 3 plugins + Subscribe + List subscriptions = 5 children
	assert.Len(t, plugins.children, 5)

	// First 3 are plugin categories
	assert.Equal(t, "beads", plugins.children[0].label)
	assert.True(t, plugins.children[0].isCategory())

	assert.Equal(t, "superpowers", plugins.children[1].label)
	assert.Contains(t, plugins.children[1].desc, "pinned v1.2.3")

	assert.Equal(t, "my-tool", plugins.children[2].label)
	assert.Contains(t, plugins.children[2].desc, "forked")

	// Last two are Subscribe + List
	assert.Equal(t, "Subscribe", plugins.children[3].label)
	assert.Equal(t, "List subscriptions", plugins.children[4].label)
}

func TestBuildMenuItems_PluginActions_Upstream(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Key: "beads@beads-marketplace", Name: "beads", Status: "upstream"},
		},
	}

	items := BuildMenuItems(state)
	plugins := items[2]
	beads := plugins.children[0]

	// Upstream: Pin to version, Fork for local edit, Update
	assert.Len(t, beads.children, 3)

	actionIDs := []string{}
	for _, child := range beads.children {
		actionIDs = append(actionIDs, child.action.ID)
	}
	assert.Contains(t, actionIDs, ActionPluginPin)
	assert.Contains(t, actionIDs, ActionPluginFork)
	assert.Contains(t, actionIDs, ActionPluginUpdate)
}

func TestBuildMenuItems_PluginActions_Pinned(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Key: "superpowers@claude-plugins-official", Name: "superpowers", Status: "pinned", PinVersion: "1.2.3"},
		},
	}

	items := BuildMenuItems(state)
	plugins := items[2]
	sp := plugins.children[0]

	// Pinned: Unpin, Update
	assert.Len(t, sp.children, 2)

	actionIDs := []string{}
	for _, child := range sp.children {
		actionIDs = append(actionIDs, child.action.ID)
	}
	assert.Contains(t, actionIDs, ActionPluginUnpin)
	assert.Contains(t, actionIDs, ActionPluginUpdate)
}

func TestBuildMenuItems_PluginActions_Forked(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Key: "my-tool@claude-sync-forks", Name: "my-tool", Status: "forked"},
		},
	}

	items := BuildMenuItems(state)
	plugins := items[2]
	tool := plugins.children[0]

	// Forked: Unfork only
	assert.Len(t, tool.children, 1)
	assert.Equal(t, ActionPluginUnfork, tool.children[0].action.ID)
}

func TestBuildMenuItems_PendingApprovals(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasPending:   true,
	}

	items := BuildMenuItems(state)

	// No profiles => Advanced is last category
	advanced := items[len(items)-1]
	assert.Equal(t, "Advanced", advanced.label)
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

	advanced := items[len(items)-1]
	assert.Equal(t, "Advanced", advanced.label)
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

	advanced := items[len(items)-1]
	assert.Equal(t, "Advanced", advanced.label)
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

	advanced := items[len(items)-1]
	assert.Equal(t, "Advanced", advanced.label)
	for _, child := range advanced.children {
		assert.NotEqual(t, "Resolve conflicts", child.label)
	}
}

func TestBuildMenuItems_ProfilePicker(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}

	items := BuildMenuItems(state)

	// With profiles, Profiles category should exist
	// Find Profiles category
	var profiles *menuItem
	for i := range items {
		if items[i].label == "Profiles" {
			profiles = &items[i]
			break
		}
	}
	assert.NotNil(t, profiles, "Profiles category should exist")

	// "base" first (deactivate), then each profile, then Show active at bottom
	// base + work + personal + Show active = 4 children
	assert.Len(t, profiles.children, 4)

	// base is first
	assert.Equal(t, "base", profiles.children[0].label)
	assert.Equal(t, ActionProfileSet, profiles.children[0].action.ID)
	assert.Equal(t, []string{""}, profiles.children[0].action.Args)

	// work is active
	assert.Equal(t, "work", profiles.children[1].label)
	assert.Contains(t, profiles.children[1].desc, "active")
	assert.Equal(t, ActionProfileSet, profiles.children[1].action.ID)
	assert.Equal(t, []string{"work"}, profiles.children[1].action.Args)

	// personal
	assert.Equal(t, "personal", profiles.children[2].label)
	assert.Equal(t, ActionProfileSet, profiles.children[2].action.ID)
	assert.Equal(t, []string{"personal"}, profiles.children[2].action.Args)

	// Show active profile at bottom
	assert.Equal(t, "Show active profile", profiles.children[3].label)
	assert.Equal(t, ActionProfileShow, profiles.children[3].action.ID)
}

func TestBuildMenuItems_ProfilePicker_NoProfiles(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     nil,
	}

	items := BuildMenuItems(state)

	// With no profiles, Profiles category should be hidden
	for _, item := range items {
		assert.NotEqual(t, "Profiles", item.label, "Profiles category should be hidden when no profiles")
	}
}

func TestBuildMenuItems_ProjectManager(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Projects: []commands.ProjectInfo{
			{Path: "/home/user/Work/project-a", Profile: "work"},
			{Path: "/home/user/Repositories/project-b", Profile: ""},
		},
	}

	items := BuildMenuItems(state)

	// Find Projects category
	var projects *menuItem
	for i := range items {
		if items[i].label == "Projects" {
			projects = &items[i]
			break
		}
	}
	assert.NotNil(t, projects, "Projects category should exist")

	// 2 projects + "+ Initialize new project" = 3 children
	assert.Len(t, projects.children, 3)

	// Project A with profile tag
	assert.Equal(t, "project-a", projects.children[0].label)
	assert.Contains(t, projects.children[0].desc, "[work]")
	assert.True(t, projects.children[0].isCategory())
	// Should have "Remove project" action inside
	assert.Len(t, projects.children[0].children, 1)
	assert.Equal(t, ActionProjectRemove, projects.children[0].children[0].action.ID)
	assert.Equal(t, []string{"/home/user/Work/project-a"}, projects.children[0].children[0].action.Args)

	// Project B with base tag
	assert.Equal(t, "project-b", projects.children[1].label)
	assert.Contains(t, projects.children[1].desc, "[base]")

	// Init at bottom
	assert.Equal(t, "+ Initialize new project", projects.children[2].label)
	assert.Equal(t, ActionProjectInit, projects.children[2].action.ID)
}

func TestBuildMenuItems_ProjectManager_Empty(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Projects:     nil,
	}

	items := BuildMenuItems(state)

	// Find Projects category
	var projects *menuItem
	for i := range items {
		if items[i].label == "Projects" {
			projects = &items[i]
			break
		}
	}
	assert.NotNil(t, projects, "Projects category should exist even with no projects")

	// Only "+ Initialize new project"
	assert.Len(t, projects.children, 1)
	assert.Equal(t, "+ Initialize new project", projects.children[0].label)
	assert.Equal(t, ActionProjectInit, projects.children[0].action.ID)
}

func TestAllActionIDs_IncludesPhase2(t *testing.T) {
	ids := AllActionIDs()
	phase2 := []string{
		ActionPluginPin, ActionPluginUnpin, ActionPluginFork, ActionPluginUnfork,
		ActionPluginUpdate, ActionProfileSet, ActionProjectInit, ActionProjectRemove,
	}
	for _, id := range phase2 {
		assert.Contains(t, ids, id, "AllActionIDs should include %s", id)
	}
}
