package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
)

func TestActivePluginsView_ShowsPlugins(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream", Marketplace: "beads-marketplace"},
			{Name: "superpowers", Status: "pinned", PinVersion: "1.2.3"},
			{Name: "my-tool", Status: "forked"},
		},
	}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()

	assert.Contains(t, view, "Active Plugins (3)")
	assert.Contains(t, view, "beads")
	assert.Contains(t, view, "superpowers")
	assert.Contains(t, view, "my-tool")
	assert.Contains(t, view, "upstream")
	assert.Contains(t, view, "pinned")
	assert.Contains(t, view, "forked")
}

func TestActivePluginsView_ShowsMarketplace(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream", Marketplace: "beads-marketplace"},
		},
	}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()
	assert.Contains(t, view, "beads-marketplace")
}

func TestActivePluginsView_ShowsPinnedVersion(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "sp", Status: "pinned", PinVersion: "1.2.3"},
		},
	}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()
	assert.Contains(t, view, "v1.2.3")
}

func TestActivePluginsView_ShowsLatestVersion(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "sp", Status: "pinned", PinVersion: "1.2.3", LatestVersion: "1.3.0"},
		},
	}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()
	assert.Contains(t, view, "v1.2.3")
	assert.Contains(t, view, "latest: v1.3.0")
}

func TestActivePluginsView_ShowsForked(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "my-tool", Status: "forked"},
		},
	}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()
	assert.Contains(t, view, "local edits")
}

func TestActivePluginsView_ShowsUntrackedPlugins(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream"},
		},
		UntrackedPlugins: []string{"playwright@some-mkt", "episodic-memory@other-mkt"},
	}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()

	assert.Contains(t, view, "Not in synced config")
	assert.Contains(t, view, "playwright")
	assert.Contains(t, view, "episodic-memory")
	assert.Contains(t, view, "installed locally")
}

func TestActivePluginsView_UntrackedOnly(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:     true,
		UntrackedPlugins: []string{"playwright@some-mkt"},
	}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()

	assert.Contains(t, view, "Active Plugins (0)")
	assert.Contains(t, view, "playwright")
}

func TestActivePluginsView_NoPlugins(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()
	assert.Contains(t, view, "No plugins configured")
}

func TestActivePluginsView_ScrollDown(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "a", Status: "upstream"},
			{Name: "b", Status: "upstream"},
		},
	}
	v := NewActivePluginsView(state, 80, 10) // small height to allow scrolling
	assert.Equal(t, 0, v.scroll)

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	v2 := updated.(ActivePluginsView)
	// Scroll should increment if maxScroll > 0
	if v2.maxScroll > 0 {
		assert.Equal(t, 1, v2.scroll)
	}
}

func TestActivePluginsView_ScrollUp(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "a", Status: "upstream"},
		},
	}
	v := NewActivePluginsView(state, 80, 10)
	v.scroll = 1

	updated, _ := v.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	v2 := updated.(ActivePluginsView)
	assert.Equal(t, 0, v2.scroll)
}

func TestActivePluginsView_EscClosesSubView(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	v := NewActivePluginsView(state, 80, 40)

	_, cmd := v.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.NotNil(t, cmd)

	// Execute the command and check the message type
	msg := cmd()
	_, ok := msg.(subViewCloseMsg)
	assert.True(t, ok, "should emit subViewCloseMsg")
}

func TestActivePluginsView_Footer(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	v := NewActivePluginsView(state, 80, 40)
	view := v.View()
	assert.Contains(t, view, "esc back")
}
