package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildIntents_HasGroupedSections(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)

	// Collect action IDs (non-header items)
	var ids []string
	for _, it := range intents {
		if !it.isHeader {
			ids = append(ids, it.action.id)
		}
	}
	assert.Contains(t, ids, ActionPull)
	assert.Contains(t, ids, ActionPushChanges)
	assert.Contains(t, ids, ActionBrowsePlugins)
	assert.Contains(t, ids, ActionRemovePlugin)
	assert.Contains(t, ids, ActionForkPlugin)
	assert.Contains(t, ids, ActionPinPlugin)
	assert.Contains(t, ids, ActionProfileList)
	assert.Contains(t, ids, ActionConfigUpdate)
	assert.Contains(t, ids, ActionSubscribe)
	assert.Len(t, ids, 9, "should have 9 selectable actions")
}

func TestBuildIntents_HasThreeHeaders(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)

	var headers []string
	for _, it := range intents {
		if it.isHeader {
			headers = append(headers, it.label)
		}
	}
	assert.Equal(t, []string{"Sync", "Plugins", "Config"}, headers)
}

func TestBuildIntents_HeadersHaveNoAction(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)

	for _, it := range intents {
		if it.isHeader {
			assert.Empty(t, it.action.id, "header %q should have no action", it.label)
		}
	}
}

func TestBuildIntents_FirstSelectableIsPull(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)

	for _, it := range intents {
		if !it.isHeader {
			assert.Equal(t, ActionPull, it.action.id)
			break
		}
	}
}

func TestBuildIntents_SyncItemsAreInline(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)

	for _, it := range intents {
		if it.action.id == ActionPull || it.action.id == ActionPushChanges {
			assert.True(t, it.action.inline, "%s should be inline", it.action.id)
		}
	}
}

func TestBuildIntents_SyncItemsHaveHints(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)

	for _, it := range intents {
		if it.action.id == ActionPull {
			assert.Contains(t, it.hint, "pull")
		}
		if it.action.id == ActionPushChanges {
			assert.Contains(t, it.hint, "push")
		}
	}
}

func TestBuildIntents_TotalCount(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	// 3 headers + 9 actions = 12 items
	assert.Len(t, intents, 12)
}

func TestRenderActions_ShowsGroupedSections(t *testing.T) {
	recs := []recommendation{
		{icon: "\u26a0", title: "Config behind", action: actionItem{id: "pull", label: "Pull now", inline: true}},
	}
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderActions(recs, intents, 0, 70, 30)
	assert.Contains(t, view, "Needs attention")
	assert.Contains(t, view, "Sync")
	assert.Contains(t, view, "Plugins")
	assert.Contains(t, view, "Config")
	assert.Contains(t, view, "Config behind")
	assert.Contains(t, view, "Pull now")
}

func TestRenderActions_NothingToRecommend(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderActions(nil, intents, 0, 70, 30)
	assert.Contains(t, view, "Everything looks good")
	assert.Contains(t, view, "Sync")
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

func TestSelectedAction_SkipsHeaders(t *testing.T) {
	recs := []recommendation{}
	intents := []intent{
		{label: "Sync", isHeader: true},
		{action: actionItem{id: "pull"}},
		{action: actionItem{id: "push"}},
		{label: "Plugins", isHeader: true},
		{action: actionItem{id: "browse-plugins"}},
	}
	// Cursor 0 => intent[0] is header => nil
	a := selectedAction(recs, intents, 0)
	assert.Nil(t, a, "header should return nil")

	// Cursor 1 => intent[1] is pull
	a = selectedAction(recs, intents, 1)
	require.NotNil(t, a)
	assert.Equal(t, "pull", a.id)

	// Cursor 3 => intent[3] is header => nil
	a = selectedAction(recs, intents, 3)
	assert.Nil(t, a, "header should return nil")

	// Cursor 4 => intent[4] is browse-plugins
	a = selectedAction(recs, intents, 4)
	require.NotNil(t, a)
	assert.Equal(t, "browse-plugins", a.id)
}

func TestActionItemCount_ExcludesHeaders(t *testing.T) {
	recs := []recommendation{{}, {}}
	intents := []intent{
		{label: "H", isHeader: true},
		{action: actionItem{id: "a"}},
		{action: actionItem{id: "b"}},
		{label: "H2", isHeader: true},
		{action: actionItem{id: "c"}},
	}
	assert.Equal(t, 5, actionItemCount(recs, intents), "2 recs + 3 non-header intents")
}

func TestRawItemCount_IncludesHeaders(t *testing.T) {
	recs := []recommendation{{}, {}}
	intents := []intent{
		{label: "H", isHeader: true},
		{action: actionItem{id: "a"}},
		{action: actionItem{id: "b"}},
	}
	assert.Equal(t, 5, rawItemCount(recs, intents), "2 recs + 3 intents (including header)")
}

func TestIsIntentHeader(t *testing.T) {
	recs := []recommendation{{}}
	intents := []intent{
		{label: "H", isHeader: true},
		{action: actionItem{id: "a"}},
	}
	assert.False(t, isIntentHeader(recs, intents, 0), "rec position is not a header")
	assert.True(t, isIntentHeader(recs, intents, 1), "intent[0] is a header")
	assert.False(t, isIntentHeader(recs, intents, 2), "intent[1] is not a header")
}

// --- Filter function tests ---

func TestFilterRecommendations(t *testing.T) {
	recs := []recommendation{
		{title: "Config is behind", detail: "3 commits"},
		{title: "Plugin update available"},
	}
	filtered := filterRecommendations(recs, "plugin")
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered[0].title, "Plugin")
}

func TestFilterRecommendations_MatchesDetail(t *testing.T) {
	recs := []recommendation{
		{title: "Config is behind", detail: "3 commits behind remote"},
		{title: "Plugin update available"},
	}
	filtered := filterRecommendations(recs, "commits")
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered[0].detail, "commits")
}

func TestFilterRecommendations_MatchesActionLabel(t *testing.T) {
	recs := []recommendation{
		{title: "Config is behind", action: actionItem{label: "Pull and apply now"}},
		{title: "Plugin update available", action: actionItem{label: "Update to v2"}},
	}
	filtered := filterRecommendations(recs, "pull")
	assert.Len(t, filtered, 1)
	assert.Contains(t, filtered[0].action.label, "Pull")
}

func TestFilterRecommendations_EmptyQuery(t *testing.T) {
	recs := []recommendation{{title: "test1"}, {title: "test2"}}
	assert.Len(t, filterRecommendations(recs, ""), 2)
}

func TestFilterRecommendations_CaseInsensitive(t *testing.T) {
	recs := []recommendation{
		{title: "Plugin Update Available"},
	}
	filtered := filterRecommendations(recs, "PLUGIN")
	assert.Len(t, filtered, 1)
}

func TestFilterIntents(t *testing.T) {
	intents := []intent{
		{label: "Sync", isHeader: true},
		{action: actionItem{label: "Pull latest updates"}},
		{label: "Plugins", isHeader: true},
		{action: actionItem{label: "Browse & install plugins"}},
		{action: actionItem{label: "Switch profile"}},
	}
	filtered := filterIntents(intents, "plug")
	// Should keep "Plugins" header and "Browse & install plugins"
	assert.Len(t, filtered, 2)
	assert.True(t, filtered[0].isHeader)
	assert.Equal(t, "Plugins", filtered[0].label)
	assert.Contains(t, filtered[1].action.label, "plugins")
}

func TestFilterIntents_EmptyQuery(t *testing.T) {
	intents := []intent{
		{label: "H", isHeader: true},
		{action: actionItem{label: "test1"}},
		{action: actionItem{label: "test2"}},
	}
	assert.Len(t, filterIntents(intents, ""), 3)
}

func TestFilterIntents_NoMatch(t *testing.T) {
	intents := []intent{
		{label: "Sync", isHeader: true},
		{action: actionItem{label: "Add plugins"}},
		{action: actionItem{label: "Push changes"}},
	}
	filtered := filterIntents(intents, "zzzzz")
	assert.Len(t, filtered, 0, "no matches means no headers either")
}

func TestFilterIntents_HeaderKeptOnlyIfSectionHasMatch(t *testing.T) {
	intents := []intent{
		{label: "Sync", isHeader: true},
		{action: actionItem{label: "Pull updates"}},
		{label: "Plugins", isHeader: true},
		{action: actionItem{label: "Browse plugins"}},
	}
	filtered := filterIntents(intents, "pull")
	// Only "Sync" header and "Pull updates" should survive
	assert.Len(t, filtered, 2)
	assert.True(t, filtered[0].isHeader)
	assert.Equal(t, "Sync", filtered[0].label)
}

// --- Help overlay tests ---

func TestAppModel_HelpOverlay_QuestionMarkOpens(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("?")})
	app := updated.(AppModel)
	assert.True(t, app.showHelp)
}

func TestAppModel_HelpOverlay_AnyKeyCloses(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.showHelp = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("x")})
	app := updated.(AppModel)
	assert.False(t, app.showHelp)
}

func TestAppModel_HelpOverlay_EscCloses(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.showHelp = true

	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	app := updated.(AppModel)
	assert.False(t, app.showHelp)
}

func TestAppModel_HelpOverlay_QDoesNotQuit(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.showHelp = true

	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	app := updated.(AppModel)
	assert.False(t, app.quitting, "q should dismiss help, not quit")
	assert.False(t, app.showHelp)
	assert.Nil(t, cmd)
}
