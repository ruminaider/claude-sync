package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"

	tea "github.com/charmbracelet/bubbletea"
)

func TestNewSidebar(t *testing.T) {
	s := NewSidebar()
	assert.Equal(t, len(AllSections), len(s.sections))
	assert.Equal(t, SectionPlugins, s.ActiveSection())
}

func TestSidebarUpdateCounts(t *testing.T) {
	s := NewSidebar()
	s.UpdateCounts(SectionPlugins, 5, 10)

	// Find the plugins entry and verify
	for _, e := range s.sections {
		if e.Section == SectionPlugins {
			assert.Equal(t, 5, e.Selected)
			assert.Equal(t, 10, e.Total)
			assert.True(t, e.Available)
			return
		}
	}
	t.Fatal("SectionPlugins not found")
}

func TestSidebarUpdateCounts_ZeroTotal(t *testing.T) {
	s := NewSidebar()
	s.UpdateCounts(SectionSettings, 0, 0)

	for _, e := range s.sections {
		if e.Section == SectionSettings {
			assert.Equal(t, 0, e.Selected)
			assert.Equal(t, 0, e.Total)
			assert.False(t, e.Available) // zero total = unavailable
			return
		}
	}
	t.Fatal("SectionSettings not found")
}

func TestSidebarNavigation(t *testing.T) {
	s := NewSidebar()

	// All sections are available by default (Total=0 but Available=true from NewSidebar)
	// After UpdateCounts they become unavailable if Total=0
	// Let's make some available
	s.UpdateCounts(SectionPlugins, 3, 5)
	s.UpdateCounts(SectionSettings, 2, 4)
	s.UpdateCounts(SectionClaudeMD, 1, 2)

	// Start at Plugins (index 0)
	assert.Equal(t, SectionPlugins, s.ActiveSection())

	// Navigate down to Settings
	var cmd tea.Cmd
	s, cmd = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, SectionSettings, s.ActiveSection())
	assert.NotNil(t, cmd)
	msg := cmd()
	sectionMsg, ok := msg.(SectionSwitchMsg)
	assert.True(t, ok)
	assert.Equal(t, SectionSettings, sectionMsg.Section)

	// Navigate down to ClaudeMD
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'j'}})
	assert.Equal(t, SectionClaudeMD, s.ActiveSection())
}

func TestSidebarNavSkipsUnavailable(t *testing.T) {
	s := NewSidebar()

	// Only Plugins and MCP are available
	s.UpdateCounts(SectionPlugins, 3, 5)
	s.UpdateCounts(SectionSettings, 0, 0)    // unavailable
	s.UpdateCounts(SectionClaudeMD, 0, 0)    // unavailable
	s.UpdateCounts(SectionPermissions, 0, 0) // unavailable
	s.UpdateCounts(SectionMCP, 2, 3)
	s.UpdateCounts(SectionKeybindings, 0, 0) // unavailable
	s.UpdateCounts(SectionHooks, 0, 0)       // unavailable

	// Start at Plugins
	assert.Equal(t, SectionPlugins, s.ActiveSection())

	// Navigate down: should skip Settings, ClaudeMD, Permissions and land on MCP
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, SectionMCP, s.ActiveSection())

	// Navigate up: should skip back to Plugins
	s, _ = s.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, SectionPlugins, s.ActiveSection())
}

func TestSidebarNavStaysAtBounds(t *testing.T) {
	s := NewSidebar()
	s.UpdateCounts(SectionPlugins, 3, 5)
	// All others unavailable
	for _, sec := range AllSections {
		if sec != SectionPlugins {
			s.UpdateCounts(sec, 0, 0)
		}
	}

	// Try to move down - should stay at Plugins since nothing below is available
	s, cmd := s.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, SectionPlugins, s.ActiveSection())
	assert.Nil(t, cmd) // no section switch since we didn't move

	// Try to move up - should stay at Plugins
	s, cmd = s.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, SectionPlugins, s.ActiveSection())
	assert.Nil(t, cmd)
}

func TestSidebarEnterEmitsFocusChange(t *testing.T) {
	s := NewSidebar()
	s.UpdateCounts(SectionPlugins, 3, 5)

	var cmd tea.Cmd
	s, cmd = s.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.NotNil(t, cmd)

	msg := cmd()
	focusMsg, ok := msg.(FocusChangeMsg)
	assert.True(t, ok)
	assert.Equal(t, FocusContent, focusMsg.Zone)
}

func TestSidebarSetActive(t *testing.T) {
	s := NewSidebar()
	s.SetActive(SectionMCP)
	assert.Equal(t, SectionMCP, s.ActiveSection())
}

func TestSidebarSetHeight(t *testing.T) {
	s := NewSidebar()
	s.SetHeight(30)
	assert.Equal(t, 30, s.height)
}
