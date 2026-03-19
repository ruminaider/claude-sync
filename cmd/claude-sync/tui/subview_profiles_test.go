package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProfilePicker_ShowsAllProfiles(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	m := NewProfilePicker(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "work")
	assert.Contains(t, view, "personal")
	assert.Contains(t, view, "base")
	assert.Contains(t, view, "active")
}

func TestProfilePicker_CursorNavigation(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	m := NewProfilePicker(state, 70, 30)
	assert.Equal(t, 0, m.cursor)

	// Move down with j
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(ProfilePicker)
	assert.Equal(t, 1, m.cursor)

	// Move down again
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(ProfilePicker)
	assert.Equal(t, 2, m.cursor) // base option

	// Should not go past the last item
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(ProfilePicker)
	assert.Equal(t, 2, m.cursor)

	// Move up with k
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(ProfilePicker)
	assert.Equal(t, 1, m.cursor)

	// Move up past zero stays at zero
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(ProfilePicker)
	updated, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = updated.(ProfilePicker)
	assert.Equal(t, 0, m.cursor)
}

func TestProfilePicker_EscCancels(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	picker := updated.(ProfilePicker)
	assert.True(t, picker.cancelled)

	// Should return a subViewCloseMsg command
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState)
}

func TestProfilePicker_SelectActiveProfile_NoOp(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work"},
		ActiveProfile: "work",
	}
	m := NewProfilePicker(state, 70, 30)
	// Cursor is on "work" (index 0), which is active
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	picker := updated.(ProfilePicker)

	// Should show "Already active" without executing
	assert.True(t, picker.resultDone)
	assert.True(t, picker.resultSuccess)
	assert.Equal(t, "Already active", picker.resultMsg)
	assert.False(t, picker.executing)
	assert.Nil(t, cmd)
}

func TestProfilePicker_BaseOption(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work"},
		ActiveProfile: "work",
	}
	m := NewProfilePicker(state, 70, 30)
	// Should have base option
	view := m.View()
	assert.Contains(t, view, "base")

	// Base should be the last option (index 1 with 1 profile)
	require.Len(t, m.profiles, 2)
	assert.Equal(t, "", m.profiles[1].name)
	assert.Equal(t, "base (no profile)", m.profiles[1].displayName)
	assert.False(t, m.profiles[1].active) // work is active, not base
}

func TestProfilePicker_BaseOptionActiveWhenNoProfile(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "",
	}
	m := NewProfilePicker(state, 70, 30)
	// Base option should be active
	baseOpt := m.profiles[len(m.profiles)-1]
	assert.True(t, baseOpt.active)
	assert.Equal(t, "", baseOpt.name)
}

func TestProfilePicker_ShowsResultAfterSwitch(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)
	// Simulate receiving a result
	updated, _ := m.Update(profileSwitchResultMsg{success: true, message: "Switched to work"})
	picker := updated.(ProfilePicker)
	assert.True(t, picker.resultDone)
	assert.True(t, picker.resultSuccess)
	view := picker.View()
	assert.Contains(t, view, "Switched to work")
	assert.Contains(t, view, "Press any key to go back")
}

func TestProfilePicker_ShowsErrorResult(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)
	updated, _ := m.Update(profileSwitchResultMsg{
		success: false,
		message: "Profile set but pull failed: config.yaml missing",
	})
	picker := updated.(ProfilePicker)
	assert.True(t, picker.resultDone)
	assert.False(t, picker.resultSuccess)
	view := picker.View()
	assert.Contains(t, view, "pull failed")
}

func TestProfilePicker_AnyKeyDismissesResult(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)

	// Set result state
	updated, _ := m.Update(profileSwitchResultMsg{success: true, message: "Switched to work"})
	picker := updated.(ProfilePicker)

	// Press any key to dismiss
	updated, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = updated.(ProfilePicker)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.True(t, closeMsg.refreshState) // successful switch should refresh
}

func TestProfilePicker_FailedResultDismiss_NoRefresh(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)

	// Set failed result state
	updated, _ := m.Update(profileSwitchResultMsg{success: false, message: "failed"})
	picker := updated.(ProfilePicker)

	// Press any key to dismiss
	updated, cmd := picker.Update(tea.KeyMsg{Type: tea.KeyEnter})
	_ = updated.(ProfilePicker)
	require.NotNil(t, cmd)
	msg := cmd()
	closeMsg, ok := msg.(subViewCloseMsg)
	assert.True(t, ok)
	assert.False(t, closeMsg.refreshState) // failed switch should NOT refresh
}

func TestProfilePicker_SelectNonActiveProfile_StartsExecution(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	m := NewProfilePicker(state, 70, 30)

	// Move cursor to "personal" (index 1)
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = updated.(ProfilePicker)

	// Press enter to select personal
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	picker := updated.(ProfilePicker)
	assert.True(t, picker.executing)
	assert.Equal(t, "personal", picker.selected)
	assert.NotNil(t, cmd) // switchProfile command returned
}

func TestProfilePicker_IgnoresInputWhileExecuting(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)
	m.executing = true

	// j/k/enter should be ignored
	updated, cmd := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	picker := updated.(ProfilePicker)
	assert.Equal(t, 0, picker.cursor) // cursor unchanged
	assert.Nil(t, cmd)
}

func TestProfilePicker_ExecutingView(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work", "personal"}}
	m := NewProfilePicker(state, 70, 30)
	m.executing = true
	m.selected = "personal"

	view := m.View()
	assert.Contains(t, view, "Switching to personal")
}

func TestProfilePicker_ExecutingViewBase(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)
	m.executing = true
	m.selected = "" // selecting base

	view := m.View()
	assert.Contains(t, view, "Switching to base config")
}

func TestProfilePicker_ViewShowsHeader(t *testing.T) {
	state := commands.MenuState{Profiles: []string{"work"}}
	m := NewProfilePicker(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "Switch settings profile")
	assert.Contains(t, view, "Select a profile to activate")
	assert.Contains(t, view, "enter select")
	assert.Contains(t, view, "esc back")
}

func TestProfilePicker_ViewShowsCurrentlyActive(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "personal",
	}
	m := NewProfilePicker(state, 70, 30)
	view := m.View()
	assert.Contains(t, view, "currently active")
}

func TestProfilePicker_ProfileOrdering(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{"work", "personal", "dev"},
		ActiveProfile: "work",
	}
	m := NewProfilePicker(state, 70, 30)

	// Named profiles come first in order, then base
	require.Len(t, m.profiles, 4)
	assert.Equal(t, "work", m.profiles[0].name)
	assert.Equal(t, "personal", m.profiles[1].name)
	assert.Equal(t, "dev", m.profiles[2].name)
	assert.Equal(t, "", m.profiles[3].name) // base
}

func TestProfilePicker_NoProfiles_OnlyBase(t *testing.T) {
	state := commands.MenuState{
		Profiles:      []string{},
		ActiveProfile: "",
	}
	m := NewProfilePicker(state, 70, 30)

	require.Len(t, m.profiles, 1)
	assert.Equal(t, "", m.profiles[0].name)
	assert.True(t, m.profiles[0].active)
}

// --- AppModel integration tests ---

func TestAppModel_SwitchProfileIntent_OpensProfilePicker(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work", "personal"},
		ActiveProfile: "work",
	}
	m := NewAppModel(state)
	m.activeView = viewActions
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	// Find the switch-profile intent cursor position
	var profileIdx int
	for i, it := range m.intents {
		if it.action.id == "switch-profile" {
			profileIdx = len(m.recommendations) + i
			break
		}
	}
	m.actionCursor = profileIdx

	// Press enter
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	app := updated.(AppModel)

	assert.Equal(t, viewSubView, app.activeView)
	assert.NotNil(t, app.subView)

	// The sub-view should render profile picker content
	view := app.subView.View()
	assert.Contains(t, view, "Switch settings profile")
	assert.Contains(t, view, "work")
	assert.Contains(t, view, "personal")
}

func TestAppModel_SubViewCloseMsg_ReturnsToActions(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewAppModel(state)
	m.activeView = viewSubView
	m.subView = NewProfilePicker(state, 70, 30)

	updated, _ := m.Update(subViewCloseMsg{refreshState: false})
	app := updated.(AppModel)

	assert.Equal(t, viewActions, app.activeView)
	assert.Nil(t, app.subView)
}

func TestAppModel_SubViewCloseMsg_RefreshesState(t *testing.T) {
	state := commands.MenuState{
		ConfigExists:  true,
		Profiles:      []string{"work"},
		ActiveProfile: "work",
	}
	m := NewAppModel(state)
	m.activeView = viewSubView
	m.subView = NewProfilePicker(state, 70, 30)
	m.recommendations = buildRecommendations(m.state)
	m.intents = buildIntents(m.state)

	updated, _ := m.Update(subViewCloseMsg{refreshState: true})
	app := updated.(AppModel)

	assert.Equal(t, viewActions, app.activeView)
	assert.Nil(t, app.subView)
	// Recommendations and intents should be rebuilt
	assert.NotNil(t, app.recommendations)
	assert.NotNil(t, app.intents)
}

func TestAppModel_ProfileSwitchResultMsg_RoutesToSubView(t *testing.T) {
	state := commands.MenuState{
		ConfigExists: true,
		Profiles:     []string{"work"},
	}
	m := NewAppModel(state)
	m.activeView = viewSubView
	picker := NewProfilePicker(state, 70, 30)
	picker.executing = true
	m.subView = picker

	updated, _ := m.Update(profileSwitchResultMsg{success: true, message: "Switched to work"})
	app := updated.(AppModel)

	// The result should have been routed to the sub-view
	assert.Equal(t, viewSubView, app.activeView)
	p := app.subView.(ProfilePicker)
	assert.True(t, p.resultDone)
	assert.Equal(t, "Switched to work", p.resultMsg)
}
