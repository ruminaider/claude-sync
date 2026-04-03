package tui

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
)

// --- renderSummary tests ---

func TestRenderSummary_ShowsConfigRepo(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		ConfigRepo:   "ruminaider/claude-sync-config",
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "ruminaider/claude-sync-config")
	assert.Contains(t, view, "synced")
}

func TestRenderSummary_SyncUnknown(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		ConfigRepo:    "ruminaider/claude-sync-config",
		CommitsBehind: -1,
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "sync unknown")
}

func TestRenderSummary_CommitsBehind(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		ConfigRepo:    "ruminaider/claude-sync-config",
		CommitsBehind: 5,
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "5 behind")
}

func TestRenderSummary_NoConfigRepo(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, ConfigRepo: ""}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "not configured")
}

func TestRenderSummary_ActiveProfile(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "work")
}

func TestRenderSummary_BaseProfile_WithOthers(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"work", "personal", "base"},
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "base (default)")
	assert.Contains(t, view, "3 others available")
}

func TestRenderSummary_BaseProfile_NoOthers(t *testing.T) {
	state := commands.MenuState{ConfigExists: true, Profiles: nil}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "base (default)")
}

func TestRenderSummary_PluginCount(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Plugins: []commands.PluginInfo{
			{Name: "a", Status: "upstream"},
			{Name: "b", Status: "upstream"},
			{Name: "c", Status: "upstream"},
		},
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "3 installed")
}

func TestRenderSummary_ProjectDir(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:       true,
		ProjectDir:         "/Users/test/Repos/claude-sync",
		ProjectInitialized: true,
		ProjectProfile:     "work",
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "claude-sync")
}

func TestRenderSummary_NoProject(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "Not in a project directory")
}

func TestRenderSummary_ProjectNotInitialized(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:       true,
		ProjectDir:         "/Users/test/Repos/myproject",
		ProjectInitialized: false,
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "No settings profile assigned")
}

func TestRenderSummary_ProjectWithProfile(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:       true,
		ProjectDir:         "/Users/test/Repos/myproject",
		ProjectInitialized: true,
		ProjectProfile:     "work",
	}
	view := renderSummary(state, "0.7.0")
	assert.Contains(t, view, "work")
}

// --- renderMainScreen tests ---

func TestRenderMainScreen_ShowsVersion(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderMainScreen(MainScreenParams{
		State: state, Intents: intents, Width: 70, Height: 30, Version: "0.7.0",
	})
	assert.Contains(t, view, "0.7.0")
}

func TestRenderMainScreen_ShowsSummaryAndActions(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		ConfigRepo:   "user/repo",
	}
	recs := buildRecommendations(state)
	intents := buildIntents(state)
	view := renderMainScreen(MainScreenParams{
		State: state, Recs: recs, Intents: intents, Width: 70, Height: 30, Version: "0.7.0",
	})
	// Summary
	assert.Contains(t, view, "User config")
	assert.Contains(t, view, "user/repo")
	// Grouped sections
	assert.Contains(t, view, "Sync")
	assert.Contains(t, view, "Plugins")
	assert.Contains(t, view, "Config")
}

func TestRenderMainScreen_NothingToRecommend(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderMainScreen(MainScreenParams{
		State: state, Intents: intents, Width: 70, Height: 30, Version: "0.7.0",
	})
	assert.Contains(t, view, "Everything looks good")
}

func TestRenderMainScreen_Footer(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderMainScreen(MainScreenParams{
		State: state, Intents: intents, Width: 70, Height: 30, Version: "0.7.0",
	})
	assert.Contains(t, view, "filter")
	assert.Contains(t, view, "help")
	assert.Contains(t, view, "quit")
}

func TestRenderMainScreen_FilterBar(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)
	view := renderMainScreen(MainScreenParams{
		State: state, Intents: intents, Width: 70, Height: 30, Version: "0.7.0",
		FilterMode: true, FilterText: "plug",
	})
	assert.Contains(t, view, "plug")
}

func TestRenderMainScreen_NoFilterResults(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	view := renderMainScreen(MainScreenParams{
		State: state, Width: 70, Height: 30, Version: "0.7.0",
		FilterText: "zzz",
	})
	assert.Contains(t, view, "No matching actions")
}

// --- renderFreshInstall tests ---

func TestRenderFreshInstall_ShowsOptions(t *testing.T) {
	view := renderFreshInstall(70, 30, "0.7.0", 0)
	assert.Contains(t, view, "No config found")
	assert.Contains(t, view, "Create")
	assert.Contains(t, view, "Join")
}

func TestRenderFreshInstall_ShowsVersion(t *testing.T) {
	view := renderFreshInstall(70, 30, "0.7.0", 0)
	assert.Contains(t, view, "0.7.0")
}

func TestRenderUpdateBanner_ContainsVersions(t *testing.T) {
	banner := renderUpdateBanner("0.11.0", "0.12.0", 60)
	assert.Contains(t, banner, "0.11.0")
	assert.Contains(t, banner, "0.12.0")
	assert.Contains(t, banner, "UPDATE AVAILABLE")
	assert.Contains(t, banner, "claude-sync update")
}

func TestRenderMainScreen_BannerPresence(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	intents := buildIntents(state)

	withBanner := renderMainScreen(MainScreenParams{
		State: state, Intents: intents, Width: 80, Height: 40, Version: "0.11.0",
		UpdateAvailable: true, LatestVersion: "0.12.0",
	})
	assert.Contains(t, withBanner, "UPDATE AVAILABLE")

	withoutBanner := renderMainScreen(MainScreenParams{
		State: state, Intents: intents, Width: 80, Height: 40, Version: "0.11.0",
	})
	assert.NotContains(t, withoutBanner, "UPDATE AVAILABLE")
}
