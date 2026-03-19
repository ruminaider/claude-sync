package tui

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
)

func TestRenderDashboard_ShowsVersion(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "0.7.0")
}

func TestRenderDashboard_ShowsConfigRepo(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		ConfigRepo:   "ruminaider/claude-sync-config",
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "ruminaider/claude-sync-config")
}

func TestRenderDashboard_SyncUpToDate(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "up to date")
}

func TestRenderDashboard_SyncBehind(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, CommitsBehind: 3}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "3 commits behind")
}

func TestRenderDashboard_PendingChanges(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, HasPending: true}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "pending")
}

func TestRenderDashboard_Conflicts(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, HasConflicts: true}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "conflict")
}

func TestRenderDashboard_ShowsPlugins(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream", Marketplace: "beads-marketplace"},
			{Name: "superpowers", Status: "pinned", PinVersion: "1.2.3"},
			{Name: "my-tool", Status: "forked"},
		},
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "beads")
	assert.Contains(t, view, "superpowers")
	assert.Contains(t, view, "pinned")
	assert.Contains(t, view, "my-tool")
	assert.Contains(t, view, "forked")
}

func TestRenderDashboard_NoPlugins(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "No plugins")
}

func TestRenderDashboard_ShowsActiveProfile(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	// Active profile shown in User Config section
	assert.Contains(t, view, "work")
}

func TestRenderDashboard_ShowsProjectInfo(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:       true,
		ProjectDir:         "/Users/test/Repos/claude-sync",
		ProjectInitialized: true,
		ProjectProfile:     "work",
		ClaudeMDCount:      3,
		MCPCount:           2,
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "claude-sync")
	assert.Contains(t, view, "3")
	assert.Contains(t, view, "2")
}

func TestRenderDashboard_NoProject(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "Not in a project directory")
}

func TestRenderDashboard_FreshInstall(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "No config found")
	assert.Contains(t, view, "Create")
	assert.Contains(t, view, "Join")
}

func TestRenderDashboard_Footer(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "enter")
	assert.Contains(t, view, "see what you can do")
	assert.Contains(t, view, "q quit")
}

func TestRenderDashboard_NoConfigRepo(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, ConfigRepo: ""}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "not configured")
}

func TestRenderDashboard_WithConfigRepo(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, ConfigRepo: "user/repo"}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "user/repo")
	assert.Contains(t, view, "connected")
}

func TestRenderDashboard_NoProfilesShowsNone(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, Profiles: nil}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	// No separate profiles section, User Config shows "base (default)"
	assert.Contains(t, view, "base (default)")
}

func TestRenderDashboard_PluginUpstreamShowsMarketplace(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream", Marketplace: "beads-marketplace"},
		},
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "beads-marketplace")
}

func TestRenderDashboard_PluginPinnedShowsVersion(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "superpowers", Status: "pinned", PinVersion: "1.2.3"},
		},
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "v1.2.3")
}

func TestRenderDashboard_PluginPinnedShowsLatestVersion(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "superpowers", Status: "pinned", PinVersion: "1.2.3", LatestVersion: "1.3.0"},
		},
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "v1.2.3")
	assert.Contains(t, view, "latest: v1.3.0")
}

func TestRenderDashboard_PluginForkedShowsLocalEdits(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "my-tool", Status: "forked"},
		},
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "local edits")
}

func TestRenderDashboard_PluginCount(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "a", Status: "upstream"},
			{Name: "b", Status: "upstream"},
			{Name: "c", Status: "upstream"},
		},
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	assert.Contains(t, view, "Active Plugins (3)")
}

func TestRenderDashboard_ProfileCountInUserConfig(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"work", "personal", "base"},
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	// Profile count shown in User Config section
	assert.Contains(t, view, "3 other profiles available")
}

func TestRenderDashboard_ActiveProfileInUserConfig(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	view := renderDashboard(state, 70, 30, "0.7.0", 0)
	// Active profile shown in User Config section with bullet marker
	assert.Contains(t, view, "●")
	assert.Contains(t, view, "work")
}

func TestRenderDashboard_UntrackedPlugins(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "beads", Status: "upstream", Marketplace: "beads-marketplace"},
		},
		UntrackedPlugins: []string{"playwright@some-mkt", "episodic-memory@other-mkt"},
	}
	view := renderDashboard(state, 80, 30, "0.7.0", 0)
	assert.Contains(t, view, "Active Plugins (1)")
	assert.Contains(t, view, "Not in synced config")
	assert.Contains(t, view, "playwright")
	assert.Contains(t, view, "episodic-memory")
	assert.Contains(t, view, "installed locally")
}

func TestRenderDashboard_UntrackedPluginsOnly(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:     true,
		UntrackedPlugins: []string{"playwright@some-mkt"},
	}
	view := renderDashboard(state, 80, 30, "0.7.0", 0)
	assert.Contains(t, view, "Active Plugins (0)")
	assert.Contains(t, view, "Not in synced config")
	assert.Contains(t, view, "playwright")
}
