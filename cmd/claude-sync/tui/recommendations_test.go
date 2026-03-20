package tui

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// allGoodState returns a MenuState where everything is healthy — config exists,
// at least one profile and one plugin are present, nothing behind/pending/conflicted.
func allGoodState() commands.MenuState {
	return commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"default"},
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@beads-marketplace", Status: "upstream"},
		},
	}
}

func TestRecommendations_Empty_WhenAllGood(t *testing.T) {
	recs := buildRecommendations(allGoodState())
	assert.Empty(t, recs)
}

func TestRecommendations_Conflicts(t *testing.T) {
	state := allGoodState()
	state.HasConflicts = true
	recs := buildRecommendations(state)
	require.Len(t, recs, 1)
	assert.Contains(t, recs[0].title, "conflict")
	assert.Equal(t, "conflicts", recs[0].action.id)
	assert.True(t, recs[0].action.inline)
}

func TestRecommendations_BehindRemote(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, CommitsBehind: 3}
	recs := buildRecommendations(state)
	require.NotEmpty(t, recs)
	assert.Contains(t, recs[0].title, "3 commits behind")
	assert.Equal(t, "pull", recs[0].action.id)
	assert.True(t, recs[0].action.inline)
}

func TestRecommendations_PendingApproval(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, HasPending: true}
	recs := buildRecommendations(state)
	require.NotEmpty(t, recs)
	found := false
	for _, r := range recs {
		if r.action.id == "approve" {
			found = true
			assert.Contains(t, r.title, "ending")
			assert.False(t, r.action.inline) // sub-view, not inline
		}
	}
	assert.True(t, found)
}

func TestRecommendations_PluginUpdate(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"default"},
		Plugins: []commands.PluginInfo{
			{Name: "superpowers", Key: "superpowers@official", Status: "pinned", PinVersion: "1.2.3", LatestVersion: "1.3.0"},
		},
	}
	recs := buildRecommendations(state)
	require.NotEmpty(t, recs)
	assert.Contains(t, recs[0].title, "superpowers")
	assert.Contains(t, recs[0].title, "update")
	assert.Equal(t, "plugin-update", recs[0].action.id)
	assert.Equal(t, []string{"superpowers@official"}, recs[0].action.args)
}

func TestRecommendations_PluginUpdate_SkipsUpToDate(t *testing.T) {
	state := allGoodState()
	state.Plugins = []commands.PluginInfo{
		{Name: "beads", Status: "upstream"}, // no LatestVersion
	}
	recs := buildRecommendations(state)
	assert.Empty(t, recs) // no update recommendation
}

func TestRecommendations_NoProfiles(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, Profiles: nil}
	recs := buildRecommendations(state)
	found := false
	for _, r := range recs {
		if r.action.id == "create-profile" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestRecommendations_NoPlugins(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, Plugins: nil}
	recs := buildRecommendations(state)
	found := false
	for _, r := range recs {
		if r.action.id == "browse-plugins" {
			found = true
		}
	}
	assert.True(t, found)
}

func TestRecommendations_PriorityOrder(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		HasConflicts:  true,
		CommitsBehind: 3,
		HasPending:    true,
		Profiles:      []string{"default"},
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream"},
		},
	}
	recs := buildRecommendations(state)
	require.Len(t, recs, 3)
	// Conflicts first, then behind, then pending
	assert.Equal(t, "conflicts", recs[0].action.id)
	assert.Equal(t, "pull", recs[1].action.id)
	assert.Equal(t, "approve", recs[2].action.id)
}

func TestRecommendations_Multiple_Combined(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: 1,
		Plugins: []commands.PluginInfo{
			{Name: "sp", Key: "sp@o", Status: "pinned", PinVersion: "1.0", LatestVersion: "2.0"},
		},
	}
	recs := buildRecommendations(state)
	// Should have: behind + plugin update + no profiles
	ids := []string{}
	for _, r := range recs {
		ids = append(ids, r.action.id)
	}
	assert.Contains(t, ids, "pull")
	assert.Contains(t, ids, "plugin-update")
}

func TestRecommendations_CommitsBehindUnknown(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: -1,
		Profiles:      []string{"default"},
		Plugins:       []commands.PluginInfo{{Name: "test", Status: "upstream"}},
	}
	recs := buildRecommendations(state)
	// Should NOT generate "behind" recommendation when status unknown
	for _, r := range recs {
		assert.NotContains(t, r.title, "behind")
	}
}

func TestRecommendations_Warnings_Surfaced(t *testing.T) {
	state := allGoodState()
	state.Warnings = []string{"config.yaml: parse error"}
	recs := buildRecommendations(state)
	found := false
	for _, r := range recs {
		if r.action.id == "warning" {
			found = true
			assert.Contains(t, r.title, "config.yaml")
		}
	}
	assert.True(t, found, "warnings should be surfaced as recommendations")
}

func TestRecommendations_NotShown_WhenNoConfig(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	recs := buildRecommendations(state)
	assert.Empty(t, recs) // fresh install has no recommendations
}
