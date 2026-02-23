package tui

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	tea "github.com/charmbracelet/bubbletea"
)

// --- Helper constructor tests ---

func TestPluginPickerItems(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"a@market", "b@market"},
		AutoForked: []string{"c@local"},
	}
	items := PluginPickerItems(scan)

	// 2 headers + 2 descriptions + 3 items = 7 total
	require.Len(t, items, 7)

	// First header: Upstream (2)
	assert.True(t, items[0].IsHeader)
	assert.Equal(t, "Upstream (2)", items[0].Display)

	// Description for upstream
	assert.NotEmpty(t, items[1].Description)

	// Upstream items
	assert.Equal(t, "a@market", items[2].Key)
	assert.True(t, items[2].Selected)
	assert.False(t, items[2].IsHeader)

	assert.Equal(t, "b@market", items[3].Key)
	assert.True(t, items[3].Selected)

	// Second header: Auto-forked (1)
	assert.True(t, items[4].IsHeader)
	assert.Equal(t, "Auto-forked (1)", items[4].Display)

	// Description for auto-forked
	assert.NotEmpty(t, items[5].Description)

	// Auto-forked item
	assert.Equal(t, "c@local", items[6].Key)
	assert.True(t, items[6].Selected)
}

func TestPluginPickerItems_UpstreamOnly(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"x@m"},
	}
	items := PluginPickerItems(scan)
	require.Len(t, items, 3) // 1 header + 1 description + 1 item
	assert.True(t, items[0].IsHeader)
	assert.NotEmpty(t, items[1].Description)
	assert.Equal(t, "x@m", items[2].Key)
}

func TestPluginPickerItems_Empty(t *testing.T) {
	scan := &commands.InitScanResult{}
	items := PluginPickerItems(scan)
	assert.Empty(t, items)
}

func TestPluginPickerItemsForProfile(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"a@m", "b@m", "c@m"},
		AutoForked: []string{"d@local"},
	}
	baseSelected := map[string]bool{"a@m": true, "b@m": true}
	// Profile inherits base selections (a@m, b@m selected).
	items := PluginPickerItemsForProfile(scan, baseSelected, baseSelected)

	// Upstream header + desc + 3 upstream items + Auto-forked header + desc + 1 item = 8
	require.Len(t, items, 8)

	// Upstream header
	assert.True(t, items[0].IsHeader)
	assert.Equal(t, "Upstream (3)", items[0].Display)

	// Description
	assert.NotEmpty(t, items[1].Description)

	// Upstream items (a@m and b@m are base-inherited, c@m is not)
	assert.Equal(t, "a@m", items[2].Key)
	assert.True(t, items[2].IsBase)
	assert.Empty(t, items[2].Tag)
	assert.True(t, items[2].Selected)

	assert.Equal(t, "b@m", items[3].Key)
	assert.True(t, items[3].IsBase)
	assert.Empty(t, items[3].Tag)
	assert.True(t, items[3].Selected)

	assert.Equal(t, "c@m", items[4].Key)
	assert.False(t, items[4].IsBase)
	assert.False(t, items[4].Selected)

	// Auto-forked header
	assert.True(t, items[5].IsHeader)
	assert.Equal(t, "Auto-forked (1)", items[5].Display)

	// Description
	assert.NotEmpty(t, items[6].Description)

	// Auto-forked item (not in base)
	assert.Equal(t, "d@local", items[7].Key)
	assert.False(t, items[7].IsBase)
	assert.False(t, items[7].Selected)
}

func TestPluginPickerItemsForProfile_Deselected(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m"},
	}
	baseSelected := map[string]bool{"a@m": true} // only a@m in base
	// Profile inherits a@m, has explicitly added b@m.
	effective := map[string]bool{"a@m": true, "b@m": true}
	items := PluginPickerItemsForProfile(scan, effective, baseSelected)

	// Header + desc + 2 items = 4
	require.Len(t, items, 4)

	// a@m: inherited from base
	assert.True(t, items[2].Selected)
	assert.True(t, items[2].IsBase)
	assert.Empty(t, items[2].Tag)

	// b@m: NOT in base, but selected by profile
	assert.True(t, items[3].Selected)
	assert.False(t, items[3].IsBase)
	assert.Empty(t, items[3].Tag)
}

func TestPermissionPickerItems(t *testing.T) {
	perms := config.Permissions{
		Allow: []string{"Bash(git *)"},
		Deny:  []string{"Bash(rm *)"},
	}
	items := PermissionPickerItems(perms)

	// 2 headers + 2 items = 4
	require.Len(t, items, 4)

	// Allow header
	assert.True(t, items[0].IsHeader)
	assert.Equal(t, "Allow (1)", items[0].Display)

	// Allow item
	assert.Equal(t, "allow:Bash(git *)", items[1].Key)
	assert.Equal(t, "Bash(git *)", items[1].Display)
	assert.True(t, items[1].Selected)

	// Deny header
	assert.True(t, items[2].IsHeader)
	assert.Equal(t, "Deny (1)", items[2].Display)

	// Deny item
	assert.Equal(t, "deny:Bash(rm *)", items[3].Key)
	assert.Equal(t, "Bash(rm *)", items[3].Display)
	assert.True(t, items[3].Selected)
}

func TestPermissionPickerItems_AllowOnly(t *testing.T) {
	perms := config.Permissions{
		Allow: []string{"rule1", "rule2"},
	}
	items := PermissionPickerItems(perms)
	require.Len(t, items, 3) // 1 header + 2 items
}

func TestPermissionPickerItems_Empty(t *testing.T) {
	perms := config.Permissions{}
	items := PermissionPickerItems(perms)
	assert.Empty(t, items)
}

func TestMCPPickerItems(t *testing.T) {
	mcp := map[string]json.RawMessage{
		"bravo":   json.RawMessage(`{"url":"http://b"}`),
		"alpha":   json.RawMessage(`{"url":"http://a"}`),
		"charlie": json.RawMessage(`{}`),
	}
	items := MCPPickerItems(mcp, "~/.claude/.mcp.json")

	// 1 header + 3 items = 4
	require.Len(t, items, 4)

	// First item is the header.
	assert.True(t, items[0].IsHeader)
	assert.Contains(t, items[0].Display, "(3)")

	// Selectable items sorted by name
	assert.Equal(t, "alpha", items[1].Key)
	assert.Equal(t, "alpha", items[1].Display)
	assert.True(t, items[1].Selected)

	assert.Equal(t, "bravo", items[2].Key)
	assert.Equal(t, "charlie", items[3].Key)
}

func TestMCPPickerItems_NoSource(t *testing.T) {
	mcp := map[string]json.RawMessage{
		"alpha": json.RawMessage(`{}`),
	}
	items := MCPPickerItems(mcp, "")

	// No header when source is empty.
	require.Len(t, items, 1)
	assert.Equal(t, "alpha", items[0].Key)
	assert.False(t, items[0].IsHeader)
}

func TestMCPPickerItems_Empty(t *testing.T) {
	items := MCPPickerItems(nil, "~/.claude/.mcp.json")
	assert.Empty(t, items)
}

func TestHookPickerItems(t *testing.T) {
	hooks := map[string]json.RawMessage{
		"PreToolUse": json.RawMessage(`[{"hooks":[{"command":"lint"}]}]`),
		"PostToolUse": json.RawMessage(`[{"hooks":[{"command":"format"}]}]`),
	}
	items := HookPickerItems(hooks)

	require.Len(t, items, 2)

	// Sorted: PostToolUse, PreToolUse
	assert.Equal(t, "PostToolUse", items[0].Key)
	assert.Equal(t, "PostToolUse: format", items[0].Display)
	assert.True(t, items[0].Selected)

	assert.Equal(t, "PreToolUse", items[1].Key)
	assert.Equal(t, "PreToolUse: lint", items[1].Display)
	assert.True(t, items[1].Selected)
}

func TestHookPickerItems_NoCommand(t *testing.T) {
	hooks := map[string]json.RawMessage{
		"Hook1": json.RawMessage(`invalid`),
	}
	items := HookPickerItems(hooks)
	require.Len(t, items, 1)
	// When command extraction fails, display is just the key
	assert.Equal(t, "Hook1", items[0].Display)
}

func TestSettingsPickerItems(t *testing.T) {
	settings := map[string]any{
		"model":    "opus",
		"autoApprove": true,
	}
	items := SettingsPickerItems(settings)

	require.Len(t, items, 2)

	// Sorted: autoApprove, model
	assert.Equal(t, "autoApprove", items[0].Key)
	assert.Equal(t, "autoApprove: true", items[0].Display)
	assert.True(t, items[0].Selected)

	assert.Equal(t, "model", items[1].Key)
	assert.Equal(t, "model: opus", items[1].Display)
	assert.True(t, items[1].Selected)
}

func TestSettingsPickerItems_Empty(t *testing.T) {
	items := SettingsPickerItems(nil)
	assert.Empty(t, items)
}

func TestKeybindingsPickerItems(t *testing.T) {
	kb := map[string]any{"ctrl+s": "save"}
	items := KeybindingsPickerItems(kb)

	require.Len(t, items, 1)
	assert.Equal(t, "keybindings", items[0].Key)
	assert.Equal(t, "Include keybindings", items[0].Display)
	assert.True(t, items[0].Selected)
}

func TestKeybindingsPickerItems_Empty(t *testing.T) {
	items := KeybindingsPickerItems(nil)
	assert.Nil(t, items)

	items2 := KeybindingsPickerItems(map[string]any{})
	assert.Nil(t, items2)
}

// --- Picker methods tests ---

func TestPickerSelectedKeys(t *testing.T) {
	items := []PickerItem{
		{Display: "Header", IsHeader: true},
		{Key: "a", Display: "a", Selected: true},
		{Key: "b", Display: "b", Selected: false},
		{Key: "c", Display: "c", Selected: true},
	}
	p := NewPicker(items)

	keys := p.SelectedKeys()
	assert.Equal(t, []string{"a", "c"}, keys)
	assert.Equal(t, 2, p.SelectedCount())
	assert.Equal(t, 3, p.TotalCount())
}

func TestPickerSelectedKeys_AllSelected(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
		{Key: "b", Display: "b", Selected: true},
	}
	p := NewPicker(items)
	assert.Equal(t, []string{"a", "b"}, p.SelectedKeys())
	assert.Equal(t, 2, p.SelectedCount())
	assert.Equal(t, 2, p.TotalCount())
}

func TestPickerSelectedKeys_NoneSelected(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: false},
		{Key: "b", Display: "b", Selected: false},
	}
	p := NewPicker(items)
	assert.Nil(t, p.SelectedKeys())
	assert.Equal(t, 0, p.SelectedCount())
	assert.Equal(t, 2, p.TotalCount())
}

func TestPickerEmpty(t *testing.T) {
	p := NewPicker(nil)
	assert.Nil(t, p.SelectedKeys())
	assert.Equal(t, 0, p.SelectedCount())
	assert.Equal(t, 0, p.TotalCount())
}

func TestNewPickerCursorSkipsHeader(t *testing.T) {
	items := []PickerItem{
		{Display: "Header 1", IsHeader: true},
		{Display: "Header 2", IsHeader: true},
		{Key: "a", Display: "a", Selected: true},
		{Key: "b", Display: "b", Selected: true},
	}
	p := NewPicker(items)
	// Cursor should be on the first non-header item (index 2)
	assert.Equal(t, 2, p.cursor)
}

func TestPickerNavigation(t *testing.T) {
	items := []PickerItem{
		{Display: "Header", IsHeader: true},
		{Key: "a", Display: "a", Selected: true},
		{Display: "Header 2", IsHeader: true},
		{Key: "b", Display: "b", Selected: true},
		{Key: "c", Display: "c", Selected: true},
	}
	p := NewPicker(items)

	// Cursor starts on "a" (index 1) — initial placement skips headers.
	assert.Equal(t, 1, p.cursor)

	// Move down: lands on Header 2 (index 2) — headers are navigable.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, p.cursor)

	// Move down again: lands on "b" (index 3)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 3, p.cursor)

	// Move down again: lands on "c" (index 4)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 4, p.cursor)

	// Move down at end: should stay at 4
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 4, p.cursor)

	// Move up: should land on "b" (index 3)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 3, p.cursor)

	// Move up again: lands on Header 2 (index 2) — headers are navigable.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 2, p.cursor)

	// Move up again: lands on "a" (index 1)
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 1, p.cursor)

	// Move up again: lands on Header (index 0) — headers are navigable.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, p.cursor)

	// Move up at start: should stay at 0
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, p.cursor)
}

func TestPickerToggle(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
		{Key: "b", Display: "b", Selected: false},
	}
	p := NewPicker(items)

	// Cursor is on "a" (index 0), which is selected
	assert.True(t, p.items[0].Selected)

	// Toggle with space
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	assert.False(t, p.items[0].Selected)

	// Toggle back with enter
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'\n'}})
	// Note: enter key is "enter" string
	// Use tea.KeyEnter instead
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	// Two toggles: false -> true -> false (we toggled once with enter above as rune, which may not match)
	// Let me be more careful. After the space toggle, it's false. Let me just re-toggle with space.
}

func TestPickerToggle_SpaceKey(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	assert.True(t, p.items[0].Selected)

	// Toggle off
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	assert.False(t, p.items[0].Selected)

	// Toggle on
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	assert.True(t, p.items[0].Selected)
}

func TestPickerToggle_EnterKey(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	assert.True(t, p.items[0].Selected)

	// Toggle off with enter
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, p.items[0].Selected)
}

func TestPickerSelectAll(t *testing.T) {
	items := []PickerItem{
		{Display: "Header", IsHeader: true},
		{Key: "a", Display: "a", Selected: false},
		{Key: "b", Display: "b", Selected: false},
		{Key: "c", Display: "c", Selected: true},
	}
	p := NewPicker(items)

	// Press Ctrl+A to select all
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	assert.True(t, p.items[1].Selected)
	assert.True(t, p.items[2].Selected)
	assert.True(t, p.items[3].Selected)
	// Header should not be affected
	assert.False(t, p.items[0].Selected)

	assert.Equal(t, 3, p.SelectedCount())
}

func TestPickerSelectNone(t *testing.T) {
	items := []PickerItem{
		{Display: "Header", IsHeader: true},
		{Key: "a", Display: "a", Selected: true},
		{Key: "b", Display: "b", Selected: true},
		{Key: "c", Display: "c", Selected: true},
	}
	p := NewPicker(items)

	// Press Ctrl+N to select none
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.False(t, p.items[1].Selected)
	assert.False(t, p.items[2].Selected)
	assert.False(t, p.items[3].Selected)

	assert.Equal(t, 0, p.SelectedCount())
}

func TestPickerLeftKeyGoesToSidebar(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)

	var cmd tea.Cmd
	p, cmd = p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	require.NotNil(t, cmd)
	msg := cmd()
	focusMsg, ok := msg.(FocusChangeMsg)
	assert.True(t, ok)
	assert.Equal(t, FocusSidebar, focusMsg.Zone)
}

func TestPickerHKeyGoesToFilter(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)

	// 'h' now types into filter instead of going to sidebar
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	assert.Equal(t, "h", p.filterText)
	assert.Nil(t, cmd, "h should not emit focus change")
}

func TestPickerSetItems(t *testing.T) {
	p := NewPicker([]PickerItem{
		{Key: "a", Display: "a", Selected: true},
	})
	assert.Equal(t, 1, p.TotalCount())

	newItems := []PickerItem{
		{Display: "Header", IsHeader: true},
		{Key: "x", Display: "x", Selected: true},
		{Key: "y", Display: "y", Selected: false},
	}
	p.SetItems(newItems)

	assert.Equal(t, 2, p.TotalCount())
	assert.Equal(t, 1, p.cursor) // should skip header
}

// --- Search action tests ---

func TestPickerSearchAction_Renders(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.SetSearchAction(true)
	p.focused = true
	p.SetHeight(10)

	view := p.View()
	assert.Contains(t, view, "[↻ Re-scan]", "search action should be rendered")
}

func TestPickerSearchAction_CursorCanReach(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.SetSearchAction(true)
	p.SetHeight(10)

	// Cursor starts on "a" (index 0)
	assert.Equal(t, 0, p.cursor)

	// Move down to search action row (index 1 = len(items))
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, p.cursor, "cursor should reach the search action row")

	// Move down again: should stay at search action row
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, p.cursor, "cursor should stay at search action row")

	// Move back up
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, p.cursor, "cursor should go back to item a")
}

func TestPickerSearchAction_EnterEmitsSearchRequest(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.SetSearchAction(true)
	p.SetHeight(10)

	// Move to search action row
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, p.cursor)

	// Press Enter
	var cmd tea.Cmd
	p, cmd = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	require.NotNil(t, cmd)
	msg := cmd()
	_, ok := msg.(SearchRequestMsg)
	assert.True(t, ok, "Enter on search action should emit SearchRequestMsg")
}

func TestPickerSearchAction_SpaceIsNoOp(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.SetSearchAction(true)
	p.SetHeight(10)

	// Move to search action row
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 1, p.cursor)

	// Press Space — should not toggle anything or crash
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	assert.Nil(t, cmd, "Space on search action should produce no command")
	assert.True(t, p.items[0].Selected, "item a should still be selected")
}

func TestPickerAddItems(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	assert.Equal(t, 1, p.TotalCount())
	assert.Equal(t, 1, p.SelectedCount())

	newItems := []PickerItem{
		{Key: "b", Display: "b", Selected: true},
		{Key: "c", Display: "c", Selected: true},
	}
	p.AddItems(newItems)

	assert.Equal(t, 3, p.TotalCount())
	assert.Equal(t, 3, p.SelectedCount())
	assert.Equal(t, []string{"a", "b", "c"}, p.SelectedKeys())
}

func TestPickerSearchAction_WithHeaders(t *testing.T) {
	items := []PickerItem{
		{Display: "Global", IsHeader: true},
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.SetSearchAction(true)
	p.SetHeight(10)

	// Cursor starts on "a" (index 1, skipping header)
	assert.Equal(t, 1, p.cursor)

	// Move down: should reach search action row (index 2 = len(items))
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.Equal(t, 2, p.cursor, "cursor should reach search action row past items")
}

// --- effectiveTag tests ---

func TestEffectiveTag_ReadOnly(t *testing.T) {
	it := PickerItem{IsReadOnly: true, Tag: "[cmd]", ProviderTag: "via beads"}
	assert.Equal(t, "[cmd] via beads", it.effectiveTag())
}

func TestEffectiveTag_ReadOnly_NoTypeTag(t *testing.T) {
	it := PickerItem{IsReadOnly: true, ProviderTag: "via figma-minimal"}
	assert.Equal(t, "via figma-minimal", it.effectiveTag())
}

func TestEffectiveTag_Base(t *testing.T) {
	it := PickerItem{IsBase: true}
	assert.Equal(t, "●", it.effectiveTag())
}

func TestEffectiveTag_Base_WithTypeTag(t *testing.T) {
	it := PickerItem{IsBase: true, Tag: "[cmd]"}
	assert.Equal(t, "● [cmd]", it.effectiveTag())
}

func TestEffectiveTag_ReadOnlyAndBase(t *testing.T) {
	// When both ReadOnly and Base are set, ReadOnly takes precedence.
	// The "via" already explains the lock; "●" would be noise.
	it := PickerItem{IsReadOnly: true, IsBase: true, Tag: "[cmd]", ProviderTag: "via beads"}
	assert.Equal(t, "[cmd] via beads", it.effectiveTag())
}

func TestEffectiveTag_Normal(t *testing.T) {
	it := PickerItem{Tag: "[skill]"}
	assert.Equal(t, "[skill]", it.effectiveTag())
}

func TestEffectiveTag_Normal_NoTag(t *testing.T) {
	it := PickerItem{}
	assert.Equal(t, "", it.effectiveTag())
}

func TestEffectiveTag_ProviderTagIgnoredWhenNotReadOnly(t *testing.T) {
	// ProviderTag should only be shown when IsReadOnly — otherwise "via X" is misleading.
	it := PickerItem{Tag: "[cmd]", ProviderTag: "via beads"}
	assert.Equal(t, "[cmd]", it.effectiveTag())
}

// --- Filter infrastructure tests ---

func testItems() []PickerItem {
	return []PickerItem{
		{Display: "Upstream (2)", IsHeader: true},
		{Description: "Marketplace plugins"},
		{Key: "a@m", Display: "alpha-plugin", Selected: true},
		{Key: "b@m", Display: "beta-plugin", Selected: true, Tag: "[cmd] beads"},
		{Display: "Auto-forked (1)", IsHeader: true},
		{Description: "Local plugins"},
		{Key: "c@local", Display: "charlie", Selected: true, Tag: "[skill]"},
	}
}

func TestRefilter_NoFilter(t *testing.T) {
	p := NewPicker(testItems())
	p.refilter()
	assert.Nil(t, p.filterView, "no filter text should produce nil filterView")
}

func TestRefilter_MatchesByDisplay(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "alpha"
	p.refilter()
	// Should show: Upstream header, description, alpha-plugin
	assert.NotNil(t, p.filterView)
	assert.Equal(t, 3, len(p.filterView)) // header + desc + alpha
}

func TestRefilter_MatchesByTag(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "beads"
	p.refilter()
	// Should show: Upstream header, description, beta-plugin (matched via tag)
	assert.Equal(t, 3, len(p.filterView))
}

func TestRefilter_CaseInsensitive(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "ALPHA"
	p.refilter()
	assert.Equal(t, 3, len(p.filterView))
}

func TestRefilter_NoMatches(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "zzz"
	p.refilter()
	assert.NotNil(t, p.filterView)
	assert.Equal(t, 0, len(p.filterView))
}

func TestRefilter_HidesEmptyHeaders(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "charlie"
	p.refilter()
	// Should show: Auto-forked header, description, charlie
	// NOT Upstream header (no matching children)
	assert.Equal(t, 3, len(p.filterView))
	assert.Equal(t, 4, p.filterView[0]) // Auto-forked header at index 4
}

func TestRefilter_MatchesSkill(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "skill"
	p.refilter()
	// [skill] tag on charlie matches
	assert.Equal(t, 3, len(p.filterView)) // Auto-forked header + desc + charlie
}

func TestVisibleSelectableCount(t *testing.T) {
	p := NewPicker(testItems())
	assert.Equal(t, 3, p.visibleSelectableCount()) // 3 selectable items

	p.filterText = "alpha"
	p.refilter()
	assert.Equal(t, 1, p.visibleSelectableCount())
}

// --- Filter keyboard handling tests ---

func TestFilterKeyInput(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true

	// Type "a" - should go to filter
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'a'}})
	assert.Equal(t, "a", p.filterText)

	// Type "l" - should append
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'l'}})
	assert.Equal(t, "al", p.filterText)

	// Backspace - should delete last char
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "a", p.filterText)

	// Backspace again - empty
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	assert.Equal(t, "", p.filterText)
}

func TestEscClearsFilter(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.filterText = "alpha"
	p.refilter()

	// Esc should clear filter text (not emit focus change)
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.Equal(t, "", p.filterText)
	assert.Nil(t, p.filterView, "filterView should be nil after clearing")
	assert.Nil(t, cmd, "should not emit focus change when clearing filter")
}

func TestEscEmitsFocusWhenFilterEmpty(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.filterText = ""

	// Esc with empty filter should emit focus change
	_, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.NotNil(t, cmd, "should emit focus change when filter is empty")
}

func TestCtrlASelectsVisibleOnly(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	// Deselect all first
	for i := range p.items {
		p.items[i].Selected = false
	}

	// Filter to "alpha"
	p.filterText = "alpha"
	p.refilter()

	// Ctrl+A should only select visible items
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlA})
	assert.True(t, p.items[2].Selected, "alpha should be selected")
	assert.False(t, p.items[3].Selected, "beta should NOT be selected")
	assert.False(t, p.items[6].Selected, "charlie should NOT be selected")
}

func TestCtrlNDeselectsVisibleOnly(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	// All start selected

	// Filter to "alpha"
	p.filterText = "alpha"
	p.refilter()

	// Ctrl+N should only deselect visible items
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyCtrlN})
	assert.False(t, p.items[2].Selected, "alpha should be deselected")
	assert.True(t, p.items[3].Selected, "beta should still be selected")
	assert.True(t, p.items[6].Selected, "charlie should still be selected")
}

func TestArrowKeysStillNavigate(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true

	startCursor := p.cursor
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.NotEqual(t, startCursor, p.cursor, "down arrow should move cursor")
}

func TestFilterResetsOnSetItems(t *testing.T) {
	p := NewPicker(testItems())
	p.filterText = "alpha"
	p.refilter()

	p.SetItems(testItems())
	assert.Equal(t, "", p.filterText)
	assert.Nil(t, p.filterView)
}

// --- Filter view rendering tests ---

func TestViewShowsFilterBar(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)

	view := p.View()
	assert.Contains(t, view, "Type to search", "empty filter should show placeholder")

	// With filter text active, should show "Filter:" label.
	p.filterText = "a"
	p.refilter()
	view = p.View()
	assert.Contains(t, view, "Filter:", "active filter should show label")
}

func TestViewFilterBarShowsCount(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "alpha"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "1/3", "should show filtered/total count")
}

func TestViewShowsNoMatches(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "zzzzz"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "No matches", "should show no matches message")
}

func TestViewHidesFilteredItems(t *testing.T) {
	p := NewPicker(testItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "alpha"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "alpha", "matching item should be visible")
	assert.NotContains(t, view, "beta", "non-matching item should be hidden")
	assert.NotContains(t, view, "charlie", "non-matching item should be hidden")
}

func TestViewSearchActionAlwaysVisible(t *testing.T) {
	p := NewPicker(testItems())
	p.SetSearchAction(true)
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)
	p.filterText = "zzzzz"
	p.refilter()

	view := p.View()
	assert.Contains(t, view, "Re-scan", "search action should always be visible")
}

// --- Collapse toggle tests ---

func TestCollapseToggle(t *testing.T) {
	items := []PickerItem{
		{Display: "Section A (2)", IsHeader: true},
		{Key: "a1", Display: "a1", Selected: true},
		{Key: "a2", Display: "a2", Selected: true},
		{Display: "Section B (1)", IsHeader: true},
		{Key: "b1", Display: "b1", Selected: true},
	}
	p := NewPicker(items)
	p.SetHeight(20)

	// All 5 items visible initially.
	indices := p.viewIndices()
	assert.Len(t, indices, 5)

	// Navigate to header at index 0.
	p.cursor = 0

	// Press Enter to collapse Section A.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.True(t, p.collapsed[0], "header should be collapsed")

	// Items under Section A are hidden; header is still visible.
	indices = p.viewIndices()
	assert.Len(t, indices, 3) // Section A header + Section B header + b1

	// Press Enter again to expand.
	p.cursor = 0
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.False(t, p.collapsed[0], "header should be expanded")

	indices = p.viewIndices()
	assert.Len(t, indices, 5)
}

func TestCollapsedHeaderRendering(t *testing.T) {
	items := []PickerItem{
		{Display: "Section A (2)", IsHeader: true},
		{Key: "a1", Display: "a1", Selected: true},
		{Key: "a2", Display: "a2", Selected: true},
	}
	p := NewPicker(items)
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)

	// Expanded: should show ▾
	view := p.View()
	assert.Contains(t, view, "▾", "expanded header should show ▾")

	// Collapse.
	p.collapsed[0] = true
	view = p.View()
	assert.Contains(t, view, "▸", "collapsed header should show ▸")
}

func TestAutoCollapseReadOnly(t *testing.T) {
	items := []PickerItem{
		{Display: "Read-only section (2)", IsHeader: true},
		{Description: "All items are read-only"},
		{Key: "r1", Display: "r1", Selected: true, IsReadOnly: true},
		{Key: "r2", Display: "r2", Selected: true, IsReadOnly: true},
		{Display: "Editable section (1)", IsHeader: true},
		{Key: "e1", Display: "e1", Selected: true},
	}
	p := NewPicker(items)
	p.CollapseReadOnly = true
	p.autoCollapseReadOnly()

	// Read-only section should be auto-collapsed.
	assert.True(t, p.collapsed[0], "all-read-only section should be collapsed")
	// Editable section should not be collapsed.
	assert.False(t, p.collapsed[4], "editable section should not be collapsed")
}

func TestFilterIgnoresCollapse(t *testing.T) {
	items := []PickerItem{
		{Display: "Section A (2)", IsHeader: true},
		{Key: "a1", Display: "alpha", Selected: true},
		{Key: "a2", Display: "another", Selected: true},
		{Display: "Section B (1)", IsHeader: true},
		{Key: "b1", Display: "bravo", Selected: true},
	}
	p := NewPicker(items)
	p.SetHeight(20)

	// Collapse Section A.
	p.collapsed[0] = true

	// Without filter, collapsed items are hidden.
	indices := p.viewIndices()
	assert.Len(t, indices, 3) // Section A header + Section B header + bravo

	// Type a filter that matches a collapsed item.
	p.filterText = "alpha"
	p.refilter()

	// Filter should find the item regardless of collapsed state.
	indices = p.viewIndices()
	found := false
	for _, idx := range indices {
		if p.items[idx].Key == "a1" {
			found = true
			break
		}
	}
	assert.True(t, found, "filter should find items inside collapsed sections")
}

// --- Search status tests ---

func TestSearchStatusRendering(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.SetSearchAction(true)
	p.focused = true
	p.SetHeight(10)
	p.SetWidth(60)

	// Default: shows search action text.
	view := p.View()
	assert.Contains(t, view, "Re-scan")
	assert.NotContains(t, view, "Searching...")

	// Set searching.
	p.SetSearching(true)
	view = p.View()
	assert.Contains(t, view, "Searching...", "should show searching indicator")
	assert.NotContains(t, view, "Re-scan", "should not show action text while searching")

	// Clear searching.
	p.SetSearching(false)
	view = p.View()
	assert.Contains(t, view, "Re-scan")
	assert.NotContains(t, view, "Searching...")
}

func TestEnterBlockedDuringSearch(t *testing.T) {
	items := []PickerItem{
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.SetSearchAction(true)
	p.SetHeight(10)
	p.searching = true

	// Move to search action row.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	// Should be on the search action row (cursor == len(items) == 1).
	assert.Equal(t, 1, p.cursor)

	// Press Enter — should NOT emit SearchRequestMsg because searching is true.
	var cmd tea.Cmd
	p, cmd = p.Update(tea.KeyMsg{Type: tea.KeyEnter})
	assert.Nil(t, cmd, "Enter on search row while searching should not emit command")
}

func TestSetItemsResetsCollapsed(t *testing.T) {
	items := []PickerItem{
		{Display: "Header", IsHeader: true},
		{Key: "a", Display: "a", Selected: true},
	}
	p := NewPicker(items)
	p.collapsed[0] = true

	// SetItems should reset collapsed state.
	p.SetItems(items)
	assert.Empty(t, p.collapsed, "collapsed state should be reset after SetItems")
}

func TestSetItemsAutoCollapsesReadOnly(t *testing.T) {
	items := []PickerItem{
		{Display: "RO Section (1)", IsHeader: true},
		{Key: "r1", Display: "r1", Selected: true, IsReadOnly: true},
		{Display: "RW Section (1)", IsHeader: true},
		{Key: "e1", Display: "e1", Selected: true},
	}
	p := NewPicker(nil)
	p.CollapseReadOnly = true
	p.SetItems(items)

	assert.True(t, p.collapsed[0], "read-only section should be auto-collapsed after SetItems")
	assert.False(t, p.collapsed[2], "editable section should not be collapsed")
}

// --- Chip filter tests ---

// chipTestItems returns a fixture with mixed states for chip filter testing.
// Index 0: header
// Index 1: selected, normal
// Index 2: selected, base-inherited
// Index 3: unselected, base-inherited
// Index 4: selected, read-only (locked)
// Index 5: unselected, normal
func chipTestItems() []PickerItem {
	return []PickerItem{
		{Display: "Section (5)", IsHeader: true},
		{Key: "a", Display: "alpha", Selected: true},
		{Key: "b", Display: "bravo", Selected: true, IsBase: true},
		{Key: "c", Display: "charlie", Selected: false, IsBase: true},
		{Key: "d", Display: "delta", Selected: true, IsReadOnly: true, Tag: "[cmd]", ProviderTag: "via plugin"},
		{Key: "e", Display: "echo", Selected: false},
	}
}

func TestChipFilter_All(t *testing.T) {
	p := NewPicker(chipTestItems())
	// Default: All active → no filtering.
	assert.True(t, p.activeChips[ChipAll])
	p.refilter()
	assert.Nil(t, p.filterView, "All chip should not filter anything")
}

func TestChipFilter_Selected(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipSelected)
	p.refilter()
	assert.NotNil(t, p.filterView)
	// Should show: header + alpha, bravo, delta (selected items).
	selectableCount := 0
	for _, idx := range p.filterView {
		if !p.items[idx].IsHeader {
			selectableCount++
			assert.True(t, p.items[idx].Selected, "only selected items should pass")
		}
	}
	assert.Equal(t, 3, selectableCount)
}

func TestChipFilter_Unselected(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipUnselected)
	p.refilter()
	assert.NotNil(t, p.filterView)
	selectableCount := 0
	for _, idx := range p.filterView {
		if !p.items[idx].IsHeader {
			selectableCount++
			assert.False(t, p.items[idx].Selected, "only unselected items should pass")
		}
	}
	assert.Equal(t, 2, selectableCount) // charlie, echo
}

func TestChipFilter_Base(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.SetProfileTab(true)
	p.toggleChip(ChipBase)
	p.refilter()
	assert.NotNil(t, p.filterView)
	selectableCount := 0
	for _, idx := range p.filterView {
		if !p.items[idx].IsHeader {
			selectableCount++
			assert.True(t, p.items[idx].IsBase, "only base items should pass")
		}
	}
	assert.Equal(t, 2, selectableCount) // bravo, charlie
}

func TestChipFilter_Locked(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipLocked)
	p.refilter()
	assert.NotNil(t, p.filterView)
	selectableCount := 0
	for _, idx := range p.filterView {
		if !p.items[idx].IsHeader {
			selectableCount++
			assert.True(t, p.items[idx].IsReadOnly, "only locked items should pass")
		}
	}
	assert.Equal(t, 1, selectableCount) // delta
}

func TestChipFilter_Combined_SelectedAndBase(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.SetProfileTab(true)
	p.toggleChip(ChipSelected)
	p.toggleChip(ChipBase) // AND: selected AND base
	p.refilter()
	assert.NotNil(t, p.filterView)
	selectableCount := 0
	for _, idx := range p.filterView {
		if !p.items[idx].IsHeader {
			selectableCount++
			assert.True(t, p.items[idx].Selected && p.items[idx].IsBase,
				"only selected+base items should pass")
		}
	}
	assert.Equal(t, 1, selectableCount) // bravo only
}

func TestChipFilter_MutualExclusion(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipSelected)
	assert.True(t, p.activeChips[ChipSelected])
	assert.False(t, p.activeChips[ChipUnselected])

	// Toggling Unselected should deactivate Selected.
	p.toggleChip(ChipUnselected)
	assert.False(t, p.activeChips[ChipSelected])
	assert.True(t, p.activeChips[ChipUnselected])

	// And vice versa.
	p.toggleChip(ChipSelected)
	assert.True(t, p.activeChips[ChipSelected])
	assert.False(t, p.activeChips[ChipUnselected])
}

func TestChipFilter_AllResetsOthers(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipSelected)
	p.toggleChip(ChipLocked)
	assert.True(t, p.activeChips[ChipSelected])
	assert.True(t, p.activeChips[ChipLocked])

	// Toggling All clears everything else.
	p.toggleChip(ChipAll)
	assert.True(t, p.activeChips[ChipAll])
	assert.False(t, p.activeChips[ChipSelected])
	assert.False(t, p.activeChips[ChipLocked])
}

func TestChipFilter_AutoAll(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipSelected) // All off, Selected on.
	assert.False(t, p.activeChips[ChipAll])
	assert.True(t, p.activeChips[ChipSelected])

	// Deactivate Selected — nothing active → All auto-activates.
	p.toggleChip(ChipSelected)
	assert.True(t, p.activeChips[ChipAll])
}

func TestChipFilter_WithTextSearch(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipSelected)   // only selected items
	p.filterText = "bravo"        // AND text match
	p.refilter()

	// Only bravo passes both: selected AND text match "bravo".
	selectableCount := 0
	for _, idx := range p.filterView {
		if !p.items[idx].IsHeader {
			selectableCount++
		}
	}
	assert.Equal(t, 1, selectableCount)
}

func TestChipNavigation_UpFromList(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	// Cursor starts at first selectable item (index 1); header is at 0.
	assert.Equal(t, 1, p.cursor)

	// First Up: goes to header (index 0) — headers are navigable.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.Equal(t, 0, p.cursor, "should move to header first")
	assert.False(t, p.chipFocused)

	// Second Up: at first visible item, enters chip bar.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyUp})
	assert.True(t, p.chipFocused, "should enter chip bar from first visible item")
}

func TestChipNavigation_DownFromChips(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	p.chipFocused = true

	// Press down: should return to item list.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyDown})
	assert.False(t, p.chipFocused, "down from chip bar should return to item list")
}

func TestChipNavigation_LeftRight(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	p.chipFocused = true
	p.chipCursor = 0

	// Move right.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 1, p.chipCursor)

	// Move right again.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRight})
	assert.Equal(t, 2, p.chipCursor)

	// Move left.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 1, p.chipCursor)

	// Move all the way left.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 0, p.chipCursor)

	// Can't go past 0.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyLeft})
	assert.Equal(t, 0, p.chipCursor)
}

func TestChipNavigation_SpaceToggles(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	p.chipFocused = true
	p.chipCursor = 1 // ChipSelected

	// Space toggles the chip.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{' '}})
	assert.True(t, p.activeChips[ChipSelected])
	assert.False(t, p.activeChips[ChipAll])
}

func TestChipFilter_EscCascade(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	p.filterText = "alpha"
	p.toggleChip(ChipSelected)
	p.refilter()

	// First Esc: clears text filter, chips remain.
	p, cmd := p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.Equal(t, "", p.filterText, "first esc should clear text")
	assert.True(t, p.activeChips[ChipSelected], "chips should remain after first esc")
	assert.Nil(t, cmd, "should not emit focus change yet")

	// Re-filter to update filterView after text clear.
	p.refilter()

	// Second Esc: clears chip filter.
	p, cmd = p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.True(t, p.activeChips[ChipAll], "second esc should reset to All")
	assert.Nil(t, cmd, "should not emit focus change yet")

	// Third Esc: goes to sidebar.
	p, cmd = p.Update(tea.KeyMsg{Type: tea.KeyEscape})
	assert.NotNil(t, cmd, "third esc should emit focus change")
	msg := cmd()
	focusMsg, ok := msg.(FocusChangeMsg)
	assert.True(t, ok)
	assert.Equal(t, FocusSidebar, focusMsg.Zone)
}

func TestAvailableChips_BaseTab(t *testing.T) {
	p := NewPicker(chipTestItems())
	// Default: isProfileTab = false → no Base chip.
	chips := p.availableChips()
	assert.Len(t, chips, 4) // All, Selected, Unselected, Locked
	for _, c := range chips {
		assert.NotEqual(t, ChipBase, c, "base tab should not have Base chip")
	}
}

func TestAvailableChips_ProfileTab(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.SetProfileTab(true)
	chips := p.availableChips()
	assert.Len(t, chips, 5) // All, Selected, Unselected, Base, Locked
	found := false
	for _, c := range chips {
		if c == ChipBase {
			found = true
		}
	}
	assert.True(t, found, "profile tab should have Base chip")
}

func TestChipFilter_ResetOnSetItems(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.toggleChip(ChipSelected)
	p.chipFocused = true
	p.chipCursor = 2

	p.SetItems(chipTestItems())
	assert.True(t, p.activeChips[ChipAll], "SetItems should reset chips to All")
	assert.False(t, p.chipFocused, "SetItems should unfocus chip bar")
	assert.Equal(t, 0, p.chipCursor, "SetItems should reset chip cursor")
}

func TestChipFilter_ChipBarRendering(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)

	view := p.View()
	assert.Contains(t, view, "Filters:", "should render Filters: prefix")
	// Active chip (All) gets padded pill; inactive chips get round parens.
	assert.Contains(t, view, "All", "should render All chip")
	assert.Contains(t, view, "(✓ Sel)", "should render Selected chip in parens")
	assert.Contains(t, view, "(○ Unsel)", "should render Unselected chip in parens")
	assert.Contains(t, view, "(⊘ Lock)", "should render Locked chip in parens")

	// Profile tab should also show Base chip.
	p.SetProfileTab(true)
	view = p.View()
	assert.Contains(t, view, "(● Base)", "profile tab should render Base chip in parens")
}

func TestChipFilter_PrintableExitsChipBar(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	p.chipFocused = true

	// Typing a letter should exit chip bar and start text search.
	p, _ = p.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	assert.False(t, p.chipFocused, "printable rune should exit chip bar")
	assert.Equal(t, "x", p.filterText, "printable rune should start text search")
}

func TestChipFilter_CountReflectsBothFilters(t *testing.T) {
	p := NewPicker(chipTestItems())
	p.focused = true
	p.SetHeight(20)
	p.SetWidth(60)

	// Chip filter: only selected. visibleSelectableCount excludes read-only items,
	// so delta (read-only) doesn't count → alpha and bravo.
	p.toggleChip(ChipSelected)
	p.refilter()
	assert.Equal(t, 2, p.visibleSelectableCount())

	// Add text filter narrowing further.
	p.filterText = "alpha"
	p.refilter()
	assert.Equal(t, 1, p.visibleSelectableCount())
}
