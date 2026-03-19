package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginBrowser_ShowsInstalledPlugins(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@beads-mkt", Status: "upstream", Marketplace: "beads-mkt"},
			{Name: "superpowers", Key: "sp@official", Status: "pinned", PinVersion: "1.2.3", Marketplace: "official"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "beads")
	assert.Contains(t, view, "superpowers")
}

func TestPluginBrowser_GroupsByMarketplace(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt1", Marketplace: "mkt1"},
			{Name: "hookify", Key: "hookify@mkt1", Marketplace: "mkt1"},
			{Name: "superpowers", Key: "sp@mkt2", Marketplace: "mkt2"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "mkt1")
	assert.Contains(t, view, "mkt2")
}

func TestPluginBrowser_CursorSkipsHeaders(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt1", Marketplace: "mkt1"},
			{Name: "sp", Key: "sp@mkt2", Marketplace: "mkt2"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	// First selectable item should be beads (skip mkt1 header)
	assert.False(t, m.items[m.cursor].isHeader)
}

func TestPluginBrowser_SpaceToggles(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Marketplace: "mkt", Status: "upstream"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	// Find first non-header item
	for m.items[m.cursor].isHeader {
		m.cursor++
	}
	initial := m.items[m.cursor].selected
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(" ")})
	m = updated.(PluginBrowser)
	assert.NotEqual(t, initial, m.items[m.cursor].selected)
}

func TestPluginBrowser_EscCancels(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewPluginBrowser(state, 70, 30)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	browser := updated.(PluginBrowser)
	assert.True(t, browser.cancelled)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState)
}

func TestPluginBrowser_FilterMode(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Marketplace: "mkt"},
			{Name: "superpowers", Key: "sp@mkt", Marketplace: "mkt"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	// Enter filter mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(PluginBrowser)
	assert.True(t, m.filterMode)

	// Type "bea"
	for _, c := range "bea" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
		m = updated.(PluginBrowser)
	}
	view := m.View()
	assert.Contains(t, view, "beads")
	// superpowers should be hidden by filter
}

func TestPluginBrowser_InstalledPluginsPreChecked(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Marketplace: "mkt", Status: "upstream"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	for _, item := range m.items {
		if !item.isHeader && item.name == "beads" {
			assert.True(t, item.installed)
			assert.True(t, item.selected) // pre-checked because installed
		}
	}
}

func TestPluginBrowser_NoPlugins(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewPluginBrowser(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "No plugins")
}

func TestPluginBrowser_NavigationJK(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt1", Marketplace: "mkt1"},
			{Name: "hookify", Key: "hookify@mkt1", Marketplace: "mkt1"},
			{Name: "sp", Key: "sp@mkt2", Marketplace: "mkt2"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)

	// cursor should start on first non-header (beads)
	require.False(t, m.items[m.cursor].isHeader)
	assert.Equal(t, "beads", m.items[m.cursor].name)

	// j moves to hookify (next non-header)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(PluginBrowser)
	assert.False(t, m.items[m.cursor].isHeader)
	assert.Equal(t, "hookify", m.items[m.cursor].name)

	// j again moves to sp (skipping mkt2 header)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(PluginBrowser)
	assert.False(t, m.items[m.cursor].isHeader)
	assert.Equal(t, "sp", m.items[m.cursor].name)

	// j at end stays put
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(PluginBrowser)
	assert.Equal(t, "sp", m.items[m.cursor].name)

	// k moves back to hookify
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(PluginBrowser)
	assert.Equal(t, "hookify", m.items[m.cursor].name)
}

func TestPluginBrowser_EnterConfirms(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Marketplace: "mkt", Status: "upstream"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	browser := updated.(PluginBrowser)
	assert.True(t, browser.confirmed)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState) // no changes, so no refresh
}

func TestPluginBrowser_FilterModeEscClears(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Marketplace: "mkt"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)

	// Enter filter mode
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(PluginBrowser)
	assert.True(t, m.filterMode)

	// Type something
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	m = updated.(PluginBrowser)
	assert.Equal(t, "x", m.filterText)

	// Esc clears filter and exits filter mode (does NOT cancel the view)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = updated.(PluginBrowser)
	assert.False(t, m.filterMode)
	assert.Equal(t, "", m.filterText)
	assert.False(t, m.cancelled)
	assert.Nil(t, cmd)
}

func TestPluginBrowser_FilterBackspaceDeletesChar(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Marketplace: "mkt"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)

	// Enter filter mode and type "abc"
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("/")})
	m = updated.(PluginBrowser)
	for _, c := range "abc" {
		updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{c}})
		m = updated.(PluginBrowser)
	}
	assert.Equal(t, "abc", m.filterText)

	// Backspace removes last char
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = updated.(PluginBrowser)
	assert.Equal(t, "ab", m.filterText)
}

func TestPluginBrowser_ShowsVersionForPinned(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "superpowers", Key: "sp@official", Status: "pinned", PinVersion: "1.2.3", Marketplace: "official"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "v1.2.3")
}

func TestPluginBrowser_ShowsStatusLabel(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Status: "upstream", Marketplace: "mkt"},
			{Name: "sp", Key: "sp@mkt", Status: "pinned", PinVersion: "1.0.0", Marketplace: "mkt"},
			{Name: "fork", Key: "fork@local", Status: "forked", Marketplace: "local"},
		},
	}
	m := NewPluginBrowser(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "upstream")
	assert.Contains(t, view, "pinned")
	assert.Contains(t, view, "forked")
}

// --- AppModel integration test ---

func TestAppModel_BrowsePluginsIntent_OpensPluginBrowser(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Key: "beads@mkt", Status: "upstream", Marketplace: "mkt"},
		},
	}
	m := NewAppModel(state)
	m.width = 80
	m.height = 40
	m.activeView = viewActions
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	// Find the browse-plugins intent cursor position
	var browseIdx int
	for i, it := range m.intents {
		if it.action.id == "browse-plugins" {
			browseIdx = len(m.recommendations) + i
			break
		}
	}
	m.actionCursor = browseIdx

	// Press enter
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)

	assert.Equal(t, viewSubView, app.activeView)
	assert.NotNil(t, app.subView)

	// The sub-view should render plugin browser content
	view := app.subView.View()
	assert.Contains(t, view, "Add or discover new plugins")
	assert.Contains(t, view, "beads")
}
