package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- extractRepoName tests ---

func TestExtractRepoName_HTTPS(t *testing.T) {
	name := extractRepoName("https://github.com/ruminaider/claude-code-config.git")
	assert.Equal(t, "ruminaider/claude-code-config", name)
}

func TestExtractRepoName_SSH(t *testing.T) {
	name := extractRepoName("git@github.com:ruminaider/claude-code-config.git")
	assert.Equal(t, "ruminaider/claude-code-config", name)
}

func TestExtractRepoName_HTTPSNoGit(t *testing.T) {
	name := extractRepoName("https://github.com/owner/repo")
	assert.Equal(t, "owner/repo", name)
}

func TestExtractRepoName_Empty(t *testing.T) {
	name := extractRepoName("")
	assert.Equal(t, "claude-sync", name)
}

func TestExtractRepoName_Fallback(t *testing.T) {
	name := extractRepoName("not-a-url")
	assert.Equal(t, "claude-sync", name)
}

// --- Banner tests ---

func TestBanner_ShowsRepoAndRole(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		RemoteURL:    "https://github.com/ruminaider/claude-code-config.git",
		Role:         "owner",
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "ruminaider/claude-code-config")
	assert.Contains(t, banner, "owner")
}

func TestBanner_ShowsSyncBehind(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		RemoteURL:     "https://github.com/owner/repo.git",
		CommitsBehind: 3,
		CommitsAhead:  1,
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "3 behind")
	assert.Contains(t, banner, "1 local")
}

func TestBanner_ShowsSynced(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		RemoteURL:     "https://github.com/owner/repo.git",
		CommitsBehind: 0,
		CommitsAhead:  0,
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "synced")
}

func TestBanner_ShowsSyncedWhenBothUnknown(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		CommitsBehind: -1,
		CommitsAhead:  -1,
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "synced")
}

func TestBanner_ShowsPendingWithCount(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		RemoteURL:    "https://github.com/owner/repo.git",
		HasPending:   true,
		PendingCount: 2,
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "2 pending changes")
	assert.Contains(t, banner, "review")
}

func TestBanner_ShowsConflicts(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		RemoteURL:    "https://github.com/owner/repo.git",
		HasConflicts: true,
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "conflicts need review")
}

func TestBanner_HidesPendingWhenNone(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		RemoteURL:    "https://github.com/owner/repo.git",
		HasPending:   false,
		HasConflicts: false,
	}
	banner := buildBanner(state)
	assert.NotContains(t, banner, "pending")
	assert.NotContains(t, banner, "\u26a0") // ⚠
}

func TestBanner_ShowsPluginCount(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		RemoteURL:    "https://github.com/owner/repo.git",
		PluginCount:  5,
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "5 plugins")
}

func TestBanner_ShowsProfile(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		RemoteURL:     "https://github.com/owner/repo.git",
		ActiveProfile: "work",
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "profile: work")
}

func TestBanner_ShowsProfileNone(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		RemoteURL:    "https://github.com/owner/repo.git",
	}
	banner := buildBanner(state)
	assert.Contains(t, banner, "profile: none")
}

// --- Banner integration in View() ---

func TestMenuView_ShowsBannerWhenConfigured(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		RemoteURL:    "https://github.com/owner/my-config.git",
		Role:         "owner",
	}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "owner/my-config")
	assert.Contains(t, view, "synced")
}

func TestMenuView_NoBannerWhenFreshInstall(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.NotContains(t, view, "synced")
	assert.NotContains(t, view, "profile:")
}

// --- 'r' shortcut tests ---

func TestMenu_RShortcut_Pending(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasPending:   true,
		PendingCount: 3,
	}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	mm := model.(MenuModel)

	require.NotNil(t, mm.Selected, "pressing 'r' with HasPending should select an action")
	assert.Equal(t, ActionApprove, mm.Selected.actionID)
	assert.NotNil(t, cmd, "should return tea.Quit")
}

func TestMenu_RShortcut_Conflicts(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasConflicts: true,
	}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	mm := model.(MenuModel)

	require.NotNil(t, mm.Selected, "pressing 'r' with HasConflicts should select an action")
	assert.Equal(t, ActionConflicts, mm.Selected.actionID)
	assert.NotNil(t, cmd, "should return tea.Quit")
}

func TestMenu_RShortcut_NoPending(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasPending:   false,
		HasConflicts: false,
	}
	m := NewMenuModel(state)
	originalCursor := m.cursor

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	mm := model.(MenuModel)

	assert.Nil(t, mm.Selected, "pressing 'r' without pending should not select anything")
	assert.Nil(t, cmd, "should not quit")
	assert.Equal(t, originalCursor, mm.cursor, "cursor should not move")
}

func TestMenu_RShortcut_PendingTakesPriorityOverConflicts(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		HasPending:   true,
		HasConflicts: true,
		PendingCount: 1,
	}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, _ = model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("r")})
	mm := model.(MenuModel)

	require.NotNil(t, mm.Selected)
	assert.Equal(t, ActionApprove, mm.Selected.actionID,
		"HasPending should take priority over HasConflicts")
}
