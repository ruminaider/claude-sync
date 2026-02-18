package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewTabBar(t *testing.T) {
	tb := NewTabBar(nil)
	assert.Equal(t, "Base", tb.ActiveTab())
	assert.Equal(t, 1, len(tb.tabs))
}

func TestNewTabBarWithProfiles(t *testing.T) {
	tb := NewTabBar([]string{"work", "personal"})
	assert.Equal(t, "Base", tb.ActiveTab()) // defaults to first (Base)
	assert.Equal(t, 3, len(tb.tabs))
	assert.Equal(t, "Base", tb.tabs[0])
	assert.Equal(t, "work", tb.tabs[1])
	assert.Equal(t, "personal", tb.tabs[2])
}

func TestTabBarAddTab(t *testing.T) {
	tb := NewTabBar(nil)
	assert.Equal(t, "Base", tb.ActiveTab())

	tb.AddTab("work")
	assert.Equal(t, "work", tb.ActiveTab()) // switches to new tab
	assert.Equal(t, 2, len(tb.tabs))

	tb.AddTab("personal")
	assert.Equal(t, "personal", tb.ActiveTab())
	assert.Equal(t, 3, len(tb.tabs))
}

func TestTabBarRemoveTab(t *testing.T) {
	tb := NewTabBar(nil)
	tb.AddTab("work")
	tb.AddTab("personal")
	assert.Equal(t, 3, len(tb.tabs))

	// Remove "work" (not active, active is "personal")
	tb.RemoveTab("work")
	assert.Equal(t, 2, len(tb.tabs))
	assert.Equal(t, "personal", tb.ActiveTab())
}

func TestTabBarRemoveActiveTab(t *testing.T) {
	tb := NewTabBar(nil)
	tb.AddTab("work")
	assert.Equal(t, "work", tb.ActiveTab())

	tb.RemoveTab("work")
	assert.Equal(t, 1, len(tb.tabs))
	assert.Equal(t, "Base", tb.ActiveTab())
}

func TestTabBarCannotRemoveBase(t *testing.T) {
	tb := NewTabBar(nil)
	tb.RemoveTab("Base")
	assert.Equal(t, 1, len(tb.tabs))
	assert.Equal(t, "Base", tb.ActiveTab())
}

func TestTabBarRemoveNonexistent(t *testing.T) {
	tb := NewTabBar(nil)
	tb.RemoveTab("doesnotexist")
	assert.Equal(t, 1, len(tb.tabs))
}

func TestTabBarNavigateLeftRight(t *testing.T) {
	tb := NewTabBar([]string{"work", "personal"})
	assert.Equal(t, "Base", tb.ActiveTab())

	// Navigate right to "work"
	var cmd tea.Cmd
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, "work", tb.ActiveTab())
	assert.NotNil(t, cmd)

	// Check the command produces a TabSwitchMsg
	msg := cmd()
	switchMsg, ok := msg.(TabSwitchMsg)
	assert.True(t, ok)
	assert.Equal(t, "work", switchMsg.Name)

	// Navigate right to "personal"
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, "personal", tb.ActiveTab())

	// Navigate right at the end should trigger NewProfileRequestMsg
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.NotNil(t, cmd)
	_, isNewProfile := cmd().(NewProfileRequestMsg)
	assert.True(t, isNewProfile)

	// Navigate left back to "work"
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, "work", tb.ActiveTab())

	// Navigate left back to "Base"
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, "Base", tb.ActiveTab())

	// Navigate left at start: no change
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, "Base", tb.ActiveTab())
	assert.Nil(t, cmd)
}

func TestTabBarPlusKey(t *testing.T) {
	tb := NewTabBar(nil)
	var cmd tea.Cmd
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	assert.NotNil(t, cmd)
	_, isNewProfile := cmd().(NewProfileRequestMsg)
	assert.True(t, isNewProfile)
}

func TestTabBarDeleteKey(t *testing.T) {
	tb := NewTabBar([]string{"work"})

	// Switch to "work" tab
	tb, _ = tb.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, "work", tb.ActiveTab())

	// Ctrl+D should emit DeleteProfileMsg
	var cmd tea.Cmd
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	assert.NotNil(t, cmd)
	deleteMsg, ok := cmd().(DeleteProfileMsg)
	assert.True(t, ok)
	assert.Equal(t, "work", deleteMsg.Name)
}

func TestTabBarDeleteKeyOnBase(t *testing.T) {
	tb := NewTabBar(nil)
	var cmd tea.Cmd
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	// Should not produce a command when on Base
	assert.Nil(t, cmd)
}

func TestTabBarActiveTabEmpty(t *testing.T) {
	tb := TabBar{tabs: nil, active: 0}
	assert.Equal(t, "", tb.ActiveTab())
}
