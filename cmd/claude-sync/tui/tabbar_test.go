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

func TestTabBarCycleNext(t *testing.T) {
	tb := NewTabBar([]string{"work", "personal"})
	assert.Equal(t, "Base", tb.ActiveTab())

	tb.CycleNext()
	assert.Equal(t, "work", tb.ActiveTab())
	assert.False(t, tb.OnPlus())

	tb.CycleNext()
	assert.Equal(t, "personal", tb.ActiveTab())
	assert.False(t, tb.OnPlus())

	// Next lands on [+].
	tb.CycleNext()
	assert.True(t, tb.OnPlus())
	assert.Equal(t, "personal", tb.ActiveTab()) // real tab unchanged

	// Next wraps around to Base.
	tb.CycleNext()
	assert.False(t, tb.OnPlus())
	assert.Equal(t, "Base", tb.ActiveTab())
}

func TestTabBarCyclePrev(t *testing.T) {
	tb := NewTabBar([]string{"work", "personal"})
	assert.Equal(t, "Base", tb.ActiveTab())

	// Wraps to [+].
	tb.CyclePrev()
	assert.True(t, tb.OnPlus())
	assert.Equal(t, "personal", tb.ActiveTab()) // active moves to last real tab

	// Prev from [+] goes to last real tab.
	tb.CyclePrev()
	assert.False(t, tb.OnPlus())
	assert.Equal(t, "personal", tb.ActiveTab())

	tb.CyclePrev()
	assert.Equal(t, "work", tb.ActiveTab())

	tb.CyclePrev()
	assert.Equal(t, "Base", tb.ActiveTab())
}

func TestTabBarCycleNext_SingleTab(t *testing.T) {
	tb := NewTabBar(nil) // only Base
	assert.Equal(t, "Base", tb.ActiveTab())

	// Single tab cycles to [+].
	tb.CycleNext()
	assert.True(t, tb.OnPlus())

	// Then wraps back to Base.
	tb.CycleNext()
	assert.False(t, tb.OnPlus())
	assert.Equal(t, "Base", tb.ActiveTab())
}

func TestTabBarCyclePrev_SingleTab(t *testing.T) {
	tb := NewTabBar(nil) // only Base

	// Prev wraps to [+].
	tb.CyclePrev()
	assert.True(t, tb.OnPlus())

	// Prev from [+] goes back to Base.
	tb.CyclePrev()
	assert.False(t, tb.OnPlus())
	assert.Equal(t, "Base", tb.ActiveTab())
}

func TestTabBarOnPlusClearedByAddTab(t *testing.T) {
	tb := NewTabBar(nil)
	tb.CycleNext() // move to [+]
	assert.True(t, tb.OnPlus())

	tb.AddTab("work")
	assert.False(t, tb.OnPlus())
	assert.Equal(t, "work", tb.ActiveTab())
}

func TestTabBarOnPlusClearedBySetActive(t *testing.T) {
	tb := NewTabBar([]string{"work"})
	tb.CycleNext() // work
	tb.CycleNext() // [+]
	assert.True(t, tb.OnPlus())

	tb.SetActive(0)
	assert.False(t, tb.OnPlus())
	assert.Equal(t, "Base", tb.ActiveTab())
}

func TestTabBarEnterOnPlus(t *testing.T) {
	tb := NewTabBar(nil)
	tb.CycleNext() // move to [+]
	assert.True(t, tb.OnPlus())

	var cmd tea.Cmd
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, cmd)
	_, isNewProfile := cmd().(NewProfileRequestMsg)
	assert.True(t, isNewProfile)
}

func TestTabBarEnterNotOnPlus(t *testing.T) {
	tb := NewTabBar([]string{"work"})

	// Enter on a real tab is a no-op.
	var cmd tea.Cmd
	tb, cmd = tb.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Nil(t, cmd)
}

func TestTabBarActiveTabEmpty(t *testing.T) {
	tb := TabBar{tabs: nil, active: 0}
	assert.Equal(t, "", tb.ActiveTab())
}
