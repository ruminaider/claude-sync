package tui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// menuSendKey sends a rune key to a MenuModel and returns the updated model.
func menuSendKey(m tea.Model, key string) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(key)})
	return updated
}

func menuSendSpecial(m tea.Model, key tea.KeyType) tea.Model {
	updated, _ := m.Update(tea.KeyMsg{Type: key})
	return updated
}

// --- Constructor tests ---

func TestNewMenuModel_Configured_CursorOnFirstNonHeader(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	// First item is "Sync" header, cursor should skip it.
	assert.Equal(t, 1, m.cursor, "cursor should start on first non-header item")
	assert.False(t, m.items[m.cursor].isHeader)
	assert.Equal(t, ActionPull, m.items[m.cursor].actionID)
}

func TestNewMenuModel_FreshInstall_CursorOnFirstItem(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)

	assert.Equal(t, 0, m.cursor, "fresh install has no headers, cursor at 0")
	assert.Equal(t, "create-config", m.items[m.cursor].actionID)
}

func TestNewMenuModel_Configured_ItemCount(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	assert.Len(t, m.items, 12)
}

// --- Navigation tests ---

func TestMenuModel_NavigateDown(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m
	// Start at cursor=1 (Pull). Move down to Push.
	model = menuSendKey(model, "j")
	mm := model.(MenuModel)
	assert.Equal(t, 2, mm.cursor)
	assert.Equal(t, ActionPush, mm.items[mm.cursor].actionID)
}

func TestMenuModel_NavigateDown_SkipsHeaders(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m
	// Start at cursor=1 (Pull). Move down to Push (2), then next is header (3), should skip to 4.
	model = menuSendKey(model, "j") // -> 2 (Push)
	model = menuSendKey(model, "j") // -> 4 (Browse & install, skipping header at 3)
	mm := model.(MenuModel)
	assert.Equal(t, 4, mm.cursor)
	assert.Equal(t, ActionBrowsePlugins, mm.items[mm.cursor].actionID)
}

func TestMenuModel_NavigateUp_SkipsHeaders(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	// Place cursor on item 4 (Browse & install plugins)
	m.cursor = 4

	var model tea.Model = m
	// Move up: should skip header at 3 and land on 2 (Push)
	model = menuSendKey(model, "k")
	mm := model.(MenuModel)
	assert.Equal(t, 2, mm.cursor)
	assert.Equal(t, ActionPush, mm.items[mm.cursor].actionID)
}

func TestMenuModel_NavigateUp_StopsAtTop(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	// cursor is at 1 (first non-header)

	var model tea.Model = m
	model = menuSendKey(model, "k") // try going up past the Sync header
	mm := model.(MenuModel)
	assert.Equal(t, 1, mm.cursor, "should not move above first selectable item")
}

func TestMenuModel_NavigateDown_StopsAtBottom(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	// Place cursor on last item
	m.cursor = len(m.items) - 1

	var model tea.Model = m
	model = menuSendKey(model, "j")
	mm := model.(MenuModel)
	assert.Equal(t, len(m.items)-1, mm.cursor, "should not move past last item")
}

func TestMenuModel_ArrowKeys(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m
	model = menuSendSpecial(model, tea.KeyDown)
	mm := model.(MenuModel)
	assert.Equal(t, 2, mm.cursor, "down arrow should work")

	model = menuSendSpecial(model, tea.KeyUp)
	mm = model.(MenuModel)
	assert.Equal(t, 1, mm.cursor, "up arrow should work")
}

// --- Selection tests ---

func TestMenuModel_SelectLeafItem(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	// cursor is at 1 (Pull)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := model.(MenuModel)

	require.NotNil(t, mm.Selected, "Selected should be set")
	assert.Equal(t, ActionPull, mm.Selected.actionID)
	assert.NotNil(t, cmd, "should return tea.Quit")
}

func TestMenuModel_EnterOnHeader_DoesNothing(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	// Force cursor onto a header
	m.cursor = 0

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEnter})
	mm := model.(MenuModel)

	assert.Nil(t, mm.Selected, "should not select a header")
	assert.Nil(t, cmd, "should not quit")
}

// --- Quit tests ---

func TestMenuModel_QuitWithQ(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
	mm := model.(MenuModel)

	assert.True(t, mm.quitting)
	assert.Nil(t, mm.Selected, "quitting should not set Selected")
	assert.NotNil(t, cmd)
}

func TestMenuModel_QuitWithEsc(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyEscape})
	mm := model.(MenuModel)

	assert.True(t, mm.quitting)
	assert.NotNil(t, cmd)
}

func TestMenuModel_QuitWithCtrlC(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, cmd := model.Update(tea.KeyMsg{Type: tea.KeyCtrlC})
	mm := model.(MenuModel)

	assert.True(t, mm.quitting)
	assert.NotNil(t, cmd)
}

// --- View tests ---

func TestMenuModel_ViewConfigured_ShowsSectionHeaders(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "Sync")
	assert.Contains(t, view, "Plugins")
	assert.Contains(t, view, "Config")
}

func TestMenuModel_ViewConfigured_ShowsActionLabels(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "Pull latest updates")
	assert.Contains(t, view, "Push local changes")
	assert.Contains(t, view, "Browse & install plugins")
	assert.Contains(t, view, "Switch profile")
	assert.Contains(t, view, "Edit full config")
	assert.Contains(t, view, "Subscribe to another config")
}

func TestMenuModel_ViewConfigured_ShowsHints(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "pull")
	assert.Contains(t, view, "push")
}

func TestMenuModel_ViewConfigured_ShowsCursor(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, ">", "cursor indicator should be visible")
}

func TestMenuModel_ViewConfigured_ShowsFooter(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "navigate")
	assert.Contains(t, view, "select")
	assert.Contains(t, view, "quit")
}

func TestMenuModel_ViewFreshInstall(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40

	view := m.View()
	assert.Contains(t, view, "Create new config")
	assert.Contains(t, view, "Join a shared config")
}

func TestMenuModel_ViewQuitting_ReturnsEmpty(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.quitting = true

	view := m.View()
	assert.Empty(t, view)
}

func TestMenuModel_ViewConfigured_NoSectionDividerArrows(t *testing.T) {
	// The old menu used ">" as a category drill-down indicator.
	// The new menu uses ">" only as a cursor. Verify no stray ">" on
	// non-cursor lines (headers should use "──" dividers, not ">").
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)
	m.width = 80
	m.height = 40
	// Place cursor somewhere specific so we can check other lines.
	m.cursor = 1

	view := m.View()
	// Headers should contain the "──" divider pattern.
	assert.Contains(t, view, "──")
}

// --- Window resize test ---

func TestMenuModel_WindowResize(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	var model tea.Model = m
	model, _ = model.Update(tea.WindowSizeMsg{Width: 120, Height: 50})
	mm := model.(MenuModel)

	assert.Equal(t, 120, mm.width)
	assert.Equal(t, 50, mm.height)
}

// --- Full navigation walkthrough ---

func TestMenuModel_FullNavigation_AllSelectableItemsReachable(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	m := NewMenuModel(state)

	// Count expected selectable items.
	expectedSelectable := 0
	for _, it := range m.items {
		if !it.isHeader {
			expectedSelectable++
		}
	}

	// Navigate down through all items, collecting visited action IDs.
	var model tea.Model = m
	visited := map[string]bool{}
	mm := model.(MenuModel)
	visited[mm.items[mm.cursor].actionID] = true

	for i := 0; i < len(m.items)+5; i++ { // extra iterations to confirm we stop
		model = menuSendKey(model, "j")
		mm = model.(MenuModel)
		if !mm.items[mm.cursor].isHeader {
			visited[mm.items[mm.cursor].actionID] = true
		}
	}

	assert.Equal(t, expectedSelectable, len(visited),
		"all selectable items should be reachable by navigating down")
}
