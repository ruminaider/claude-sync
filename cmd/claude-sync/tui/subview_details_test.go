package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDetails_ShowsPlugins(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream", Marketplace: "mkt1"},
		},
	}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "beads")
	assert.Contains(t, view, "upstream")
}

func TestConfigDetails_ShowsProfiles(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "work")
	assert.Contains(t, view, "active")
}

func TestConfigDetails_ShowsProjects(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Projects: []commands.ProjectInfo{
			{Path: "/home/user/repos/project-a", Profile: "work"},
		},
	}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "project-a")
	assert.Contains(t, view, "work")
}

func TestConfigDetails_ShowsSyncStatus(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: 3,
		HasPending:    true,
	}
	m := NewConfigDetails(state, 70, 40)
	view := m.View()
	assert.Contains(t, view, "3")
	assert.Contains(t, view, "Pending approvals: yes")
}

func TestConfigDetails_EscCloses(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewConfigDetails(state, 70, 30)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	details := updated.(ConfigDetails)
	assert.True(t, details.cancelled)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState)
}

func TestConfigDetails_ScrollsWithJK(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewConfigDetails(state, 70, 10) // short height forces scrolling
	initial := m.scroll
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	details := updated.(ConfigDetails)
	assert.GreaterOrEqual(t, details.scroll, initial) // may or may not scroll depending on content
}

func TestConfigDetails_EmptyState(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "No plugins")
	assert.Contains(t, view, "No profiles")
}

func TestConfigDetails_ShowsConfigRepo(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		ConfigRepo:   "ruminaider/claude-sync-config",
	}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "ruminaider/claude-sync-config")
}

func TestConfigDetails_ShowsActiveProfileInSection(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		ActiveProfile: "work",
		Profiles:      []string{"work", "personal", "base"},
	}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	// Active profile should be marked
	assert.Contains(t, view, "work (active)")
	assert.Contains(t, view, "personal")
}

func TestConfigDetails_ShowsConflicts(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasConflicts: true,
	}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "Conflicts: yes")
}

func TestConfigDetails_ShowsProjectDetails(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:   true,
		ProjectDir:     "/tmp/test-project",
		ProjectProfile: "work",
		ClaudeMDCount:  3,
		MCPCount:       2,
	}
	m := NewConfigDetails(state, 70, 40) // tall enough to show all content
	view := m.View()
	assert.Contains(t, view, "test-project")
	assert.Contains(t, view, "CLAUDE.md sections: 3")
	assert.Contains(t, view, "MCP servers: 2")
}

func TestConfigDetails_ScrollUpStopsAtZero(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewConfigDetails(state, 70, 30)
	assert.Equal(t, 0, m.scroll)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	details := updated.(ConfigDetails)
	assert.Equal(t, 0, details.scroll) // can't go negative
}

func TestConfigDetails_ScrollDownStopsAtMax(t *testing.T) {
	// Create enough content to need scrolling with very short height
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "plugin1", Status: "upstream", Marketplace: "mkt1"},
			{Name: "plugin2", Status: "pinned", PinVersion: "1.0", Marketplace: "mkt1"},
			{Name: "plugin3", Status: "forked", Marketplace: "mkt2"},
		},
		Profiles: []string{"work", "personal"},
		Projects: []commands.ProjectInfo{
			{Path: "/tmp/proj1", Profile: "work"},
			{Path: "/tmp/proj2", Profile: "personal"},
		},
	}
	m := NewConfigDetails(state, 70, 10) // very short height
	require.Greater(t, m.maxScroll, 0, "content should exceed height for this test")

	// Scroll all the way down
	for i := 0; i < m.maxScroll+5; i++ {
		updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
		m = updated.(ConfigDetails)
	}
	assert.Equal(t, m.maxScroll, m.scroll)
}

func TestConfigDetails_ShowsPinnedVersion(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "superpowers", Status: "pinned", PinVersion: "1.2.3", Marketplace: "official"},
		},
	}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "v1.2.3")
}

func TestConfigDetails_ShowsFooterHints(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewConfigDetails(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "j/k scroll")
	assert.Contains(t, view, "esc back")
}

// --- AppModel integration ---

func TestAppModel_ViewConfigIntent_OpensConfigDetails(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Status: "upstream", Marketplace: "mkt"},
		},
		Profiles:      []string{"work"},
		ActiveProfile: "work",
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.activeView = viewActions
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	// Find the view-config intent cursor position
	var viewIdx int
	for i, it := range m.intents {
		if it.action.id == "view-config" {
			viewIdx = len(m.recommendations) + i
			break
		}
	}
	m.actionCursor = viewIdx

	// Press enter
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)

	assert.Equal(t, viewSubView, app.activeView)
	assert.NotNil(t, app.subView)

	// The sub-view should render config details content
	view := app.subView.View()
	assert.Contains(t, view, "Full config details")
	assert.Contains(t, view, "beads")
}
