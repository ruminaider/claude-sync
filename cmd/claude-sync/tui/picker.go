package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// PickerItem represents a single row in the multi-select picker.
type PickerItem struct {
	Key         string // unique identifier (plugin key, setting key, etc.)
	Display     string // display text
	Selected    bool
	IsHeader    bool   // section header, not selectable
	IsBase      bool   // inherited from base config (profile view)
	Tag         string // e.g. "[base]" for inherited items
	Description string // optional description rendered below headers
}

// Picker is an enhanced multi-select list used for Plugins, Permissions, MCP,
// Hooks, Settings, and Keybindings sections.
type Picker struct {
	items           []PickerItem
	cursor          int             // index of the highlighted row
	height          int             // viewport height (number of visible rows)
	width           int
	offset          int             // scroll offset for long lists
	selectAll       bool            // track whether all selectable items are selected
	focused         bool            // true when this picker has keyboard focus
	hasSearchAction bool            // when true, a [+ Search projects] row is appended
	tagColor        lipgloss.Color  // accent color for inherited-item tags (profile views)
}

// NewPicker creates a Picker with the given items. The cursor is placed on the
// first selectable (non-header, non-description) item.
func NewPicker(items []PickerItem) Picker {
	p := Picker{
		items:  items,
		height: 20, // sensible default
	}
	// Advance cursor to the first selectable item.
	for i := range p.items {
		if !p.isSkippable(i) {
			p.cursor = i
			break
		}
	}
	p.syncSelectAll()
	return p
}

// --- Helper constructors ---

// PluginPickerItems builds picker items from an InitScanResult with section
// headers for upstream and auto-forked plugins. All items are pre-selected.
func PluginPickerItems(scan *commands.InitScanResult) []PickerItem {
	var items []PickerItem

	if len(scan.Upstream) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Upstream (%d)", len(scan.Upstream)),
			IsHeader: true,
		})
		items = append(items, PickerItem{
			Description: "Marketplace plugins — synced by reference",
		})
		for _, key := range scan.Upstream {
			items = append(items, PickerItem{
				Key:      key,
				Display:  key,
				Selected: true,
			})
		}
	}

	if len(scan.AutoForked) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Auto-forked (%d)", len(scan.AutoForked)),
			IsHeader: true,
		})
		items = append(items, PickerItem{
			Description: "Local plugins — synced as full copies",
		})
		for _, key := range scan.AutoForked {
			items = append(items, PickerItem{
				Key:      key,
				Display:  key,
				Selected: true,
			})
		}
	}

	return items
}

// PluginPickerItemsForProfile builds the same Upstream/Auto-forked layout as
// PluginPickerItems, but uses effectiveSelected for checkbox state and marks
// items that are in baseSelected with an "inherited" tag so users can see what
// comes from the base config.
func PluginPickerItemsForProfile(scan *commands.InitScanResult, effectiveSelected, baseSelected map[string]bool) []PickerItem {
	var items []PickerItem

	if len(scan.Upstream) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Upstream (%d)", len(scan.Upstream)),
			IsHeader: true,
		})
		items = append(items, PickerItem{
			Description: "Marketplace plugins — synced by reference",
		})
		for _, key := range scan.Upstream {
			it := PickerItem{
				Key:      key,
				Display:  key,
				Selected: effectiveSelected[key],
			}
			if baseSelected[key] {
				it.IsBase = true
				it.Tag = "●"
			}
			items = append(items, it)
		}
	}

	if len(scan.AutoForked) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Auto-forked (%d)", len(scan.AutoForked)),
			IsHeader: true,
		})
		items = append(items, PickerItem{
			Description: "Local plugins — synced as full copies",
		})
		for _, key := range scan.AutoForked {
			it := PickerItem{
				Key:      key,
				Display:  key,
				Selected: effectiveSelected[key],
			}
			if baseSelected[key] {
				it.IsBase = true
				it.Tag = "●"
			}
			items = append(items, it)
		}
	}

	return items
}

// PermissionPickerItems builds picker items with "Allow" and "Deny" section
// headers. All items are pre-selected.
func PermissionPickerItems(perms config.Permissions) []PickerItem {
	var items []PickerItem

	if len(perms.Allow) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Allow (%d)", len(perms.Allow)),
			IsHeader: true,
		})
		for _, rule := range perms.Allow {
			items = append(items, PickerItem{
				Key:      "allow:" + rule,
				Display:  rule,
				Selected: true,
			})
		}
	}

	if len(perms.Deny) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Deny (%d)", len(perms.Deny)),
			IsHeader: true,
		})
		for _, rule := range perms.Deny {
			items = append(items, PickerItem{
				Key:      "deny:" + rule,
				Display:  rule,
				Selected: true,
			})
		}
	}

	return items
}

// MCPPickerItems builds a flat list of MCP server names. All items are
// pre-selected.
func MCPPickerItems(mcp map[string]json.RawMessage) []PickerItem {
	keys := make([]string, 0, len(mcp))
	for k := range mcp {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]PickerItem, 0, len(keys))
	for _, k := range keys {
		items = append(items, PickerItem{
			Key:      k,
			Display:  k,
			Selected: true,
		})
	}
	return items
}

// HookPickerItems builds picker items displayed as "HookName: command". All
// items are pre-selected.
func HookPickerItems(hooks map[string]json.RawMessage) []PickerItem {
	keys := make([]string, 0, len(hooks))
	for k := range hooks {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]PickerItem, 0, len(keys))
	for _, k := range keys {
		cmd := commands.ExtractHookCommand(hooks[k])
		display := k
		if cmd != "" {
			display = k + ": " + cmd
		}
		items = append(items, PickerItem{
			Key:      k,
			Display:  display,
			Selected: true,
		})
	}
	return items
}

// SettingsPickerItems builds picker items displayed as "key: value". All items
// are pre-selected.
func SettingsPickerItems(settings map[string]any) []PickerItem {
	keys := make([]string, 0, len(settings))
	for k := range settings {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	items := make([]PickerItem, 0, len(keys))
	for _, k := range keys {
		display := fmt.Sprintf("%s: %v", k, settings[k])
		items = append(items, PickerItem{
			Key:      k,
			Display:  display,
			Selected: true,
		})
	}
	return items
}

// KeybindingsPickerItems builds a single toggle item when keybindings are
// non-empty. The item is pre-selected.
func KeybindingsPickerItems(kb map[string]any) []PickerItem {
	if len(kb) == 0 {
		return nil
	}
	return []PickerItem{
		{
			Key:      "keybindings",
			Display:  "Include keybindings",
			Selected: true,
		},
	}
}

// --- Methods ---

// SetSearchAction enables or disables the virtual [+ Search projects] row.
func (p *Picker) SetSearchAction(enabled bool) {
	p.hasSearchAction = enabled
}

// SetTagColor sets the accent color used for inherited-item tag markers.
func (p *Picker) SetTagColor(c lipgloss.Color) {
	p.tagColor = c
}

// AddItems appends new items to the picker and marks them as selected.
func (p *Picker) AddItems(items []PickerItem) {
	p.items = append(p.items, items...)
	p.syncSelectAll()
	p.clampScroll()
}

// isSelectableItem returns true if the item is a regular selectable row
// (not a header or description).
func isSelectableItem(it PickerItem) bool {
	return !it.IsHeader && it.Description == ""
}

// SelectedKeys returns the keys of all selected selectable items.
func (p Picker) SelectedKeys() []string {
	var keys []string
	for _, it := range p.items {
		if isSelectableItem(it) && it.Selected {
			keys = append(keys, it.Key)
		}
	}
	return keys
}

// SelectedCount returns the number of selected selectable items.
func (p Picker) SelectedCount() int {
	n := 0
	for _, it := range p.items {
		if isSelectableItem(it) && it.Selected {
			n++
		}
	}
	return n
}

// TotalCount returns the number of selectable (non-header, non-description) items.
func (p Picker) TotalCount() int {
	n := 0
	for _, it := range p.items {
		if isSelectableItem(it) {
			n++
		}
	}
	return n
}

// SetHeight sets the viewport height.
func (p *Picker) SetHeight(h int) {
	p.height = h
	p.clampScroll()
}

// SetWidth sets the available width.
func (p *Picker) SetWidth(w int) {
	p.width = w
}

// SetFocused sets whether this picker currently has keyboard focus.
func (p *Picker) SetFocused(f bool) {
	p.focused = f
}

// SetItems replaces all items (for tab/section switch) and resets the cursor.
func (p *Picker) SetItems(items []PickerItem) {
	p.items = items
	p.cursor = 0
	p.offset = 0
	// Advance cursor to the first selectable item.
	for i := range p.items {
		if !p.isSkippable(i) {
			p.cursor = i
			break
		}
	}
	p.syncSelectAll()
	p.clampScroll()
}

// Update handles key messages when the picker has focus.
func (p Picker) Update(msg tea.Msg) (Picker, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			p.moveCursor(-1)
		case "down", "j":
			p.moveCursor(+1)
		case " ":
			// Space on the search action row is a no-op.
			if !(p.hasSearchAction && p.cursor == len(p.items)) {
				p.toggleCurrent()
			}
		case "enter":
			// Enter on the search action row emits SearchRequestMsg.
			if p.hasSearchAction && p.cursor == len(p.items) {
				return p, func() tea.Msg {
					return SearchRequestMsg{}
				}
			}
			p.toggleCurrent()
		case "a":
			p.doSelectAll()
		case "n":
			p.doSelectNone()
		case "left", "h":
			return p, func() tea.Msg {
				return FocusChangeMsg{Zone: FocusSidebar}
			}
		case "esc":
			return p, func() tea.Msg {
				return FocusChangeMsg{Zone: FocusSidebar}
			}
		}
	}
	return p, nil
}

// View renders the picker list with scrolling support.
func (p Picker) View() string {
	totalRows := len(p.items)
	if p.hasSearchAction {
		totalRows++ // virtual search action row
	}

	if totalRows == 0 {
		return ContentPaneStyle.Render("(no items)")
	}

	// Reserve lines for scroll indicators so total output stays within p.height.
	visibleItems := p.height
	hasAbove := p.offset > 0
	hasBelow := p.offset+p.height < totalRows
	if hasAbove {
		visibleItems--
	}
	if hasBelow {
		visibleItems--
	}
	if visibleItems < 1 {
		visibleItems = 1
	}

	var b strings.Builder

	if hasAbove {
		b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("  ↑ more") + "\n")
	}

	end := p.offset + visibleItems
	if end > totalRows {
		end = totalRows
	}

	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)

	for i := p.offset; i < end; i++ {
		// Virtual search action row at the end.
		if i == len(p.items) && p.hasSearchAction {
			cursor := "  "
			if p.focused && p.cursor == i {
				cursor = "> "
			}
			actionText := "[+ Search projects]"
			if p.focused && p.cursor == i {
				actionText = lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render(actionText)
			} else if p.focused {
				actionText = lipgloss.NewStyle().Foreground(colorBlue).Render(actionText)
			} else {
				actionText = dimStyle.Render(actionText)
			}
			b.WriteString(cursor + actionText + "\n")
			continue
		}

		it := p.items[i]

		if it.IsHeader {
			line := fmt.Sprintf("── %s ──", it.Display)
			if p.focused {
				b.WriteString(HeaderStyle.Render(line))
			} else {
				b.WriteString(dimStyle.Render(line))
			}
			b.WriteString("\n")
			continue
		}

		if it.Description != "" {
			b.WriteString("  " + dimStyle.Render(it.Description) + "\n")
			continue
		}

		// Cursor indicator: only show when focused.
		cursor := "  "
		if p.focused && i == p.cursor {
			cursor = "> "
		}

		// Checkbox.
		var checkbox string
		if p.focused {
			if it.Selected {
				checkbox = SelectedStyle.Render("[x]")
			} else {
				checkbox = UnselectedStyle.Render("[ ]")
			}
		} else {
			if it.Selected {
				checkbox = dimStyle.Render("[x]")
			} else {
				checkbox = dimStyle.Render("[ ]")
			}
		}

		// Display text.
		var display string
		if p.focused && i == p.cursor {
			display = lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(it.Display)
		} else if p.focused {
			display = it.Display
		} else {
			display = dimStyle.Render(it.Display)
		}

		// Tag (e.g. ● for inherited items).
		tag := ""
		if it.Tag != "" {
			if p.focused && p.tagColor != "" {
				tag = "  " + lipgloss.NewStyle().Foreground(p.tagColor).Render(it.Tag)
			} else if p.focused {
				tag = "  " + BaseTagStyle.Render(it.Tag)
			} else {
				tag = "  " + dimStyle.Render(it.Tag)
			}
		}

		b.WriteString(cursor + checkbox + " " + display + tag + "\n")
	}

	if end < totalRows {
		b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("  ↓ more") + "\n")
	}

	return ContentPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
}

// --- Internal helpers ---

// isSkippable returns true if the item at index i should be skipped by the cursor.
func (p *Picker) isSkippable(i int) bool {
	if i < 0 || i >= len(p.items) {
		return false
	}
	return p.items[i].IsHeader || p.items[i].Description != ""
}

// moveCursor advances the cursor in the given direction (+1 or -1), skipping
// header and description items. It also adjusts the scroll offset to keep the
// cursor visible.
func (p *Picker) moveCursor(dir int) {
	maxIndex := len(p.items) - 1
	if p.hasSearchAction {
		maxIndex = len(p.items) // virtual row at index len(items)
	}

	next := p.cursor + dir
	for next >= 0 && next <= maxIndex {
		// The virtual search action row is always selectable.
		if next == len(p.items) && p.hasSearchAction {
			p.cursor = next
			p.clampScroll()
			return
		}
		if next < len(p.items) && !p.isSkippable(next) {
			p.cursor = next
			p.clampScroll()
			return
		}
		next += dir
	}
}

// toggleCurrent toggles the selection of the item at the cursor position.
// Headers and descriptions are not togglable.
func (p *Picker) toggleCurrent() {
	if p.cursor >= 0 && p.cursor < len(p.items) && isSelectableItem(p.items[p.cursor]) {
		p.items[p.cursor].Selected = !p.items[p.cursor].Selected
		p.syncSelectAll()
	}
}

// doSelectAll selects all selectable items.
func (p *Picker) doSelectAll() {
	for i := range p.items {
		if isSelectableItem(p.items[i]) {
			p.items[i].Selected = true
		}
	}
	p.selectAll = true
}

// doSelectNone deselects all selectable items.
func (p *Picker) doSelectNone() {
	for i := range p.items {
		if isSelectableItem(p.items[i]) {
			p.items[i].Selected = false
		}
	}
	p.selectAll = false
}

// syncSelectAll updates the selectAll flag based on current item states.
func (p *Picker) syncSelectAll() {
	p.selectAll = true
	for _, it := range p.items {
		if isSelectableItem(it) && !it.Selected {
			p.selectAll = false
			return
		}
	}
}

// clampScroll ensures the cursor is within the visible window by adjusting
// the scroll offset.
func (p *Picker) clampScroll() {
	if p.height <= 0 {
		return
	}
	totalRows := len(p.items)
	if p.hasSearchAction {
		totalRows++
	}

	// When items overflow, scroll indicators take up to 2 lines.
	// Use reduced height so the cursor stays within visible items.
	effectiveHeight := p.height
	if totalRows > p.height {
		effectiveHeight -= 2
	}
	if effectiveHeight < 1 {
		effectiveHeight = 1
	}
	// Cursor above viewport: scroll up.
	if p.cursor < p.offset {
		p.offset = p.cursor
	}
	// Cursor below viewport: scroll down.
	if p.cursor >= p.offset+effectiveHeight {
		p.offset = p.cursor - effectiveHeight + 1
	}
	// Don't allow offset past the end.
	maxOffset := totalRows - effectiveHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}
