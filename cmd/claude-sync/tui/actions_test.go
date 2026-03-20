package tui

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIntents_AllPresent(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	ids := []string{}
	for _, i := range intents {
		ids = append(ids, i.action.id)
	}
	assert.Contains(t, ids, "view-plugins")
	assert.Contains(t, ids, "join-config")
	assert.Contains(t, ids, "browse-plugins")
	assert.Contains(t, ids, "switch-profile")
	assert.Contains(t, ids, "push-changes")
	assert.Contains(t, ids, "edit-config")
	assert.Contains(t, ids, "import-mcp")
	assert.Contains(t, ids, "view-config")
	assert.Len(t, intents, 8)
}

func TestBuildIntents_ViewPluginsIsFirst(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	assert.Equal(t, "view-plugins", intents[0].action.id)
}

func TestBuildIntents_ProfileLabel_Active(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		ActiveProfile: "work",
	}
	intents := buildIntents(state)
	var profileIntent *intent
	for i := range intents {
		if intents[i].action.id == "switch-profile" {
			profileIntent = &intents[i]
		}
	}
	require.NotNil(t, profileIntent)
	assert.Contains(t, profileIntent.label, "Switch settings profile")
	assert.Contains(t, profileIntent.hint, "work")
}

func TestBuildIntents_ProfileLabel_NoActive(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, Profiles: []string{"work"}}
	intents := buildIntents(state)
	var profileIntent *intent
	for i := range intents {
		if intents[i].action.id == "switch-profile" {
			profileIntent = &intents[i]
		}
	}
	require.NotNil(t, profileIntent)
	assert.Contains(t, profileIntent.label, "Activate a settings profile")
}

func TestRenderActions_ShowsBothSections(t *testing.T) {
	recs := []recommendation{
		{icon: "\u26a0", title: "Config behind", action: actionItem{id: "pull", label: "Pull now", inline: true}},
	}
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderActions(recs, intents, 0, 70, 30)
	assert.Contains(t, view, "Needs attention")
	assert.Contains(t, view, "I want to")
	assert.Contains(t, view, "Config behind")
	assert.Contains(t, view, "Pull now")
}

func TestRenderActions_NothingToRecommend(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderActions(nil, intents, 0, 70, 30)
	assert.Contains(t, view, "Everything looks good")
	assert.Contains(t, view, "I want to")
}

func TestRenderActions_CursorHighlightsItem(t *testing.T) {
	recs := []recommendation{
		{icon: "\u26a0", title: "Test rec", action: actionItem{id: "test", label: "Do test", inline: true}},
	}
	intents := buildIntents(commands.MenuState{ConfigExists: true})
	view := renderActions(recs, intents, 0, 70, 30)
	assert.Contains(t, view, ">") // cursor indicator
}

func TestRenderActions_Footer(t *testing.T) {
	intents := buildIntents(commands.MenuState{ConfigExists: true})
	view := renderActions(nil, intents, 0, 70, 30)
	assert.Contains(t, view, "quit")
}

func TestSelectedAction_InRecommendations(t *testing.T) {
	recs := []recommendation{
		{action: actionItem{id: "pull"}},
		{action: actionItem{id: "approve"}},
	}
	intents := []intent{
		{action: actionItem{id: "browse-plugins"}},
	}
	a := selectedAction(recs, intents, 0)
	require.NotNil(t, a)
	assert.Equal(t, "pull", a.id)

	a = selectedAction(recs, intents, 1)
	assert.Equal(t, "approve", a.id)
}

func TestSelectedAction_InIntents(t *testing.T) {
	recs := []recommendation{
		{action: actionItem{id: "pull"}},
	}
	intents := []intent{
		{action: actionItem{id: "browse-plugins"}},
		{action: actionItem{id: "edit-config"}},
	}
	a := selectedAction(recs, intents, 1)
	require.NotNil(t, a)
	assert.Equal(t, "browse-plugins", a.id)

	a = selectedAction(recs, intents, 2)
	assert.Equal(t, "edit-config", a.id)
}

func TestActionItemCount(t *testing.T) {
	recs := []recommendation{{}, {}}
	intents := []intent{{}, {}, {}}
	assert.Equal(t, 5, actionItemCount(recs, intents))
}
