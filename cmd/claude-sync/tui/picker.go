package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/cmdskill"
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
	IsReadOnly  bool   // read-only items cannot be toggled (e.g. plugin-provided commands)
	Tag         string // e.g. "[base]" for inherited items
	Description string // optional description rendered below headers
}

// Picker is an enhanced multi-select list used for Plugins, Permissions, MCP,
// Hooks, Settings, Keybindings, and Commands & Skills sections.
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

	// Filter mode: type-to-filter narrows the visible item list.
	filterText string
	filterView []int // nil = no filter; otherwise, indices into items to render

	// Preview mode: when enabled, → shows item content in a side viewport.
	hasPreview     bool
	previewContent map[string]string // key → markdown content for preview
	previewActive  bool              // true when the preview viewport is shown
	previewScroll  int               // scroll offset within preview content
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

// CommandsSkillsPickerItems builds picker items from a cmdskill.ScanResult,
// grouping by source with plugin items marked read-only.
func CommandsSkillsPickerItems(scan *cmdskill.ScanResult) []PickerItem {
	if scan == nil || len(scan.Items) == 0 {
		return nil
	}

	// Group items by source.
	var pluginItems, globalCmds, globalSkills []cmdskill.Item
	projectGroups := make(map[string][]cmdskill.Item) // sourceLabel -> items

	for _, item := range scan.Items {
		switch item.Source {
		case cmdskill.SourcePlugin:
			pluginItems = append(pluginItems, item)
		case cmdskill.SourceGlobal:
			if item.Type == cmdskill.TypeCommand {
				globalCmds = append(globalCmds, item)
			} else {
				globalSkills = append(globalSkills, item)
			}
		case cmdskill.SourceProject:
			projectGroups[item.SourceLabel] = append(projectGroups[item.SourceLabel], item)
		}
	}

	var items []PickerItem

	// Plugin-provided items (read-only).
	if len(pluginItems) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Plugin-provided (%d)", len(pluginItems)),
			IsHeader: true,
		})
		items = append(items, PickerItem{
			Description: "From installed plugins — read-only",
		})
		sort.Slice(pluginItems, func(i, j int) bool {
			return pluginItems[i].Key() < pluginItems[j].Key()
		})
		for _, item := range pluginItems {
			typeTag := "[cmd]"
			if item.Type == cmdskill.TypeSkill {
				typeTag = "[skill]"
			}
			items = append(items, PickerItem{
				Key:        item.Key(),
				Display:    item.Name,
				Selected:   true,
				IsReadOnly: true,
				Tag:        typeTag + " " + item.SourceLabel,
			})
		}
	}

	// Global commands.
	if len(globalCmds) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Global commands (%d)", len(globalCmds)),
			IsHeader: true,
		})
		sort.Slice(globalCmds, func(i, j int) bool {
			return globalCmds[i].Name < globalCmds[j].Name
		})
		for _, item := range globalCmds {
			items = append(items, PickerItem{
				Key:      item.Key(),
				Display:  item.Name,
				Selected: true,
				Tag:      "[cmd]",
			})
		}
	}

	// Global skills.
	if len(globalSkills) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Global skills (%d)", len(globalSkills)),
			IsHeader: true,
		})
		sort.Slice(globalSkills, func(i, j int) bool {
			return globalSkills[i].Name < globalSkills[j].Name
		})
		for _, item := range globalSkills {
			items = append(items, PickerItem{
				Key:      item.Key(),
				Display:  item.Name,
				Selected: true,
				Tag:      "[skill]",
			})
		}
	}

	// Project-local items.
	projectLabels := make([]string, 0, len(projectGroups))
	for label := range projectGroups {
		projectLabels = append(projectLabels, label)
	}
	sort.Strings(projectLabels)

	for _, label := range projectLabels {
		projItems := projectGroups[label]
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("Project: %s (%d)", label, len(projItems)),
			IsHeader: true,
		})
		sort.Slice(projItems, func(i, j int) bool {
			return projItems[i].Key() < projItems[j].Key()
		})
		for _, item := range projItems {
			typeTag := "[cmd]"
			if item.Type == cmdskill.TypeSkill {
				typeTag = "[skill]"
			}
			items = append(items, PickerItem{
				Key:      item.Key(),
				Display:  item.Name,
				Selected: true,
				Tag:      typeTag,
			})
		}
	}

	return items
}

// --- Methods ---

// SetSearchAction enables or disables the virtual [+ Search projects] row.
func (p *Picker) SetSearchAction(enabled bool) {
	p.hasSearchAction = enabled
}

// SetPreview enables preview mode and sets the content map (key → markdown).
func (p *Picker) SetPreview(content map[string]string) {
	p.hasPreview = true
	p.previewContent = content
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
// (not a header, description, or read-only).
func isSelectableItem(it PickerItem) bool {
	return !it.IsHeader && it.Description == "" && !it.IsReadOnly
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

// AllKeys returns all keys in the picker (selectable and read-only, excluding headers).
func (p Picker) AllKeys() []string {
	var keys []string
	for _, it := range p.items {
		if !it.IsHeader && it.Key != "" {
			keys = append(keys, it.Key)
		}
	}
	return keys
}

// refilter recomputes filterView based on current filterText.
// When filterText is empty, filterView is set to nil (show all).
func (p *Picker) refilter() {
	if p.filterText == "" {
		p.filterView = nil
		return
	}

	needle := strings.ToLower(p.filterText)

	// First pass: determine which selectable items match.
	matches := make([]bool, len(p.items))
	for i, it := range p.items {
		if it.IsHeader || it.Description != "" {
			continue
		}
		haystack := strings.ToLower(it.Display + " " + it.Tag)
		if strings.Contains(haystack, needle) {
			matches[i] = true
		}
	}

	// Second pass: build visible indices, including headers/descriptions
	// only if they have at least one matching child.
	vis := make([]int, 0)
	for i, it := range p.items {
		if matches[i] {
			vis = append(vis, i)
			continue
		}
		if it.IsHeader {
			// Include header if any child before the next header matches.
			for j := i + 1; j < len(p.items); j++ {
				if p.items[j].IsHeader {
					break
				}
				if matches[j] {
					vis = append(vis, i)
					break
				}
			}
		} else if it.Description != "" {
			// Include description if its preceding header was included.
			if len(vis) > 0 && vis[len(vis)-1] == i-1 {
				vis = append(vis, i)
			}
		}
	}

	p.filterView = vis
}

// isFilteredOut returns true if the item at index i is hidden by the active filter.
func (p *Picker) isFilteredOut(i int) bool {
	if p.filterView == nil {
		return false
	}
	for _, vi := range p.filterView {
		if vi == i {
			return false
		}
	}
	return true
}

// visibleSelectableCount returns the number of selectable items currently visible.
func (p Picker) visibleSelectableCount() int {
	n := 0
	for i, it := range p.items {
		if isSelectableItem(it) && !p.isFilteredOut(i) {
			n++
		}
	}
	return n
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
	p.filterText = ""
	p.filterView = nil
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
		// When preview is active, handle preview-specific keys first.
		if p.previewActive {
			switch msg.String() {
			case "left", "esc":
				p.previewActive = false
				p.previewScroll = 0
				return p, nil
			case "up":
				if p.previewScroll > 0 {
					p.previewScroll--
				}
				return p, nil
			case "down":
				p.previewScroll++
				return p, nil
			}
			return p, nil
		}

		switch msg.String() {
		case "up":
			p.moveCursor(-1)
		case "down":
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
		case "right":
			if p.filterText == "" && p.hasPreview && p.cursor >= 0 && p.cursor < len(p.items) {
				key := p.items[p.cursor].Key
				if _, ok := p.previewContent[key]; ok {
					p.previewActive = true
					p.previewScroll = 0
					return p, nil
				}
			}
		case "left":
			if p.filterText == "" {
				return p, func() tea.Msg {
					return FocusChangeMsg{Zone: FocusSidebar}
				}
			}
		case "esc":
			if p.filterText != "" {
				p.filterText = ""
				p.refilter()
				p.resetCursorToFirstVisible()
				return p, nil
			}
			return p, func() tea.Msg {
				return FocusChangeMsg{Zone: FocusSidebar}
			}
		case "ctrl+a":
			p.doSelectAll()
		case "ctrl+n":
			p.doSelectNone()
		case "backspace":
			if len(p.filterText) > 0 {
				p.filterText = p.filterText[:len(p.filterText)-1]
				p.refilter()
				p.resetCursorToFirstVisible()
			}
		default:
			// Printable rune input goes to filter.
			if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
				p.filterText += string(msg.Runes)
				p.refilter()
				p.resetCursorToFirstVisible()
			}
		}
	}
	return p, nil
}

// resetCursorToFirstVisible moves the cursor to the first visible selectable item.
func (p *Picker) resetCursorToFirstVisible() {
	for i := range p.items {
		if !p.isSkippable(i) {
			p.cursor = i
			p.offset = 0
			p.clampScroll()
			return
		}
	}
	p.cursor = 0
	p.offset = 0
}

// View renders the picker list with scrolling support.
func (p Picker) View() string {
	// When preview is active, show split view.
	if p.previewActive && p.hasPreview {
		return p.viewWithPreview()
	}

	var b strings.Builder

	// Render filter bar (always visible, takes 1 line).
	b.WriteString(p.renderFilterBar())
	b.WriteString("\n")

	// Determine visible item indices.
	indices := p.viewIndices()
	totalRows := len(indices)
	if p.hasSearchAction {
		totalRows++
	}

	if totalRows == 0 {
		if p.filterText != "" {
			return ContentPaneStyle.Render(b.String() + "  No matches")
		}
		return ContentPaneStyle.Render(b.String() + "  (no items)")
	}

	// Available height for items (subtract 1 for filter bar).
	itemHeight := p.height - 1
	if itemHeight < 1 {
		itemHeight = 1
	}

	// Compute scroll offset based on cursor position within visible list.
	cursorPos := p.cursorVisiblePos(indices)
	offset := p.computeScrollOffset(cursorPos, totalRows, itemHeight)

	// Reserve lines for scroll indicators.
	visibleItems := itemHeight
	hasAbove := offset > 0
	hasBelow := offset+itemHeight < totalRows
	if hasAbove {
		visibleItems--
	}
	if hasBelow {
		visibleItems--
	}
	if visibleItems < 1 {
		visibleItems = 1
	}

	if hasAbove {
		b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("  ↑ more") + "\n")
	}

	end := offset + visibleItems
	if end > totalRows {
		end = totalRows
	}

	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)

	for vi := offset; vi < end; vi++ {
		// Virtual search action row at the end of visible items.
		if vi == len(indices) && p.hasSearchAction {
			cursor := "  "
			realIdx := len(p.items) // virtual row
			if p.focused && p.cursor == realIdx {
				cursor = "> "
			}
			actionText := "[+ Search projects]"
			if p.focused && p.cursor == realIdx {
				actionText = lipgloss.NewStyle().Bold(true).Foreground(colorBlue).Render(actionText)
			} else if p.focused {
				actionText = lipgloss.NewStyle().Foreground(colorBlue).Render(actionText)
			} else {
				actionText = dimStyle.Render(actionText)
			}
			b.WriteString(cursor + actionText + "\n")
			continue
		}

		i := indices[vi] // real item index
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

// viewIndices returns the list of item indices to render.
// When no filter is active, returns all item indices.
func (p Picker) viewIndices() []int {
	if p.filterView != nil {
		return p.filterView
	}
	indices := make([]int, len(p.items))
	for i := range p.items {
		indices[i] = i
	}
	return indices
}

// cursorVisiblePos returns the cursor's position within the visible indices.
func (p Picker) cursorVisiblePos(indices []int) int {
	for vi, i := range indices {
		if i == p.cursor {
			return vi
		}
	}
	// Cursor is on search action row.
	if p.hasSearchAction && p.cursor == len(p.items) {
		return len(indices)
	}
	return 0
}

// computeScrollOffset computes the scroll offset to keep cursorPos visible.
func (p Picker) computeScrollOffset(cursorPos, totalRows, viewHeight int) int {
	effectiveHeight := viewHeight
	if totalRows > viewHeight {
		effectiveHeight -= 2 // scroll indicators
	}
	if effectiveHeight < 1 {
		effectiveHeight = 1
	}

	offset := p.offset
	if cursorPos < offset {
		offset = cursorPos
	}
	if cursorPos >= offset+effectiveHeight {
		offset = cursorPos - effectiveHeight + 1
	}
	maxOffset := totalRows - effectiveHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if offset > maxOffset {
		offset = maxOffset
	}
	return offset
}

// renderFilterBar renders the filter input line.
func (p Picker) renderFilterBar() string {
	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)
	label := dimStyle.Render("Filter: ")

	filterDisplay := p.filterText
	cursor := dimStyle.Render("_")
	if p.focused {
		cursor = lipgloss.NewStyle().Foreground(colorText).Render("_")
	}

	right := ""
	if p.filterText != "" {
		visible := p.visibleSelectableCount()
		total := p.TotalCount()
		right = dimStyle.Render(fmt.Sprintf("%d/%d", visible, total))
	}

	left := label + filterDisplay + cursor
	if right != "" {
		// Right-align the count.
		gap := p.width - lipgloss.Width(left) - lipgloss.Width(right) - 2
		if gap < 1 {
			gap = 1
		}
		return " " + left + strings.Repeat(" ", gap) + right
	}
	return " " + left
}

// viewWithPreview renders a split view: list on the left, content preview on the right.
func (p Picker) viewWithPreview() string {
	// Split width: 40% list, 60% preview.
	listWidth := p.width * 2 / 5
	if listWidth < 20 {
		listWidth = 20
	}
	previewWidth := p.width - listWidth - 3 // 3 = border + padding
	if previewWidth < 10 {
		previewWidth = 10
	}

	// Get current item's preview content.
	var content string
	if p.cursor >= 0 && p.cursor < len(p.items) {
		content = p.previewContent[p.items[p.cursor].Key]
	}
	if content == "" {
		content = "(no preview available)"
	}

	// Render preview with scroll.
	previewLines := strings.Split(content, "\n")
	maxScroll := len(previewLines) - p.height
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := p.previewScroll
	if scroll > maxScroll {
		scroll = maxScroll
	}
	end := scroll + p.height
	if end > len(previewLines) {
		end = len(previewLines)
	}
	visible := previewLines[scroll:end]

	// Truncate lines to fit preview width.
	for i, line := range visible {
		if len(line) > previewWidth {
			visible[i] = line[:previewWidth]
		}
	}

	previewStr := strings.Join(visible, "\n")
	previewView := lipgloss.NewStyle().
		Width(previewWidth).
		PaddingLeft(1).
		BorderLeft(true).
		BorderStyle(lipgloss.NormalBorder()).
		BorderForeground(colorSurface1).
		Render(previewStr)

	// Render filter bar if filter is active.
	var topBar string
	if p.filterText != "" {
		dimFilter := lipgloss.NewStyle().Foreground(colorOverlay0)
		topBar = " " + dimFilter.Render("Filter: ") + p.filterText + "\n"
	}

	// Render a compact list on the left showing only visible item names.
	indices := p.viewIndices()
	var listB strings.Builder
	dimStyle := lipgloss.NewStyle().Foreground(colorOverlay0)
	for _, idx := range indices {
		it := p.items[idx]
		if it.IsHeader || it.Description != "" {
			continue
		}
		cursor := "  "
		if p.focused && idx == p.cursor {
			cursor = "> "
		}
		display := it.Display
		if len(display) > listWidth-4 {
			display = display[:listWidth-7] + "..."
		}
		if p.focused && idx == p.cursor {
			display = lipgloss.NewStyle().Bold(true).Foreground(colorText).Render(display)
		} else if !p.focused {
			display = dimStyle.Render(display)
		}
		listB.WriteString(cursor + display + "\n")
	}
	listView := lipgloss.NewStyle().Width(listWidth).Render(
		strings.TrimRight(listB.String(), "\n"),
	)

	joined := lipgloss.JoinHorizontal(lipgloss.Top, listView, previewView)
	if topBar != "" {
		return topBar + joined
	}
	return joined
}

// --- Internal helpers ---

// isSkippable returns true if the item at index i should be skipped by the cursor.
func (p *Picker) isSkippable(i int) bool {
	if i < 0 || i >= len(p.items) {
		return false
	}
	if p.items[i].IsHeader || p.items[i].Description != "" {
		return true
	}
	if p.isFilteredOut(i) {
		return true
	}
	return false
}

// moveCursor advances the cursor in the given direction (+1 or -1), skipping
// header and description items. It operates on visible indices so filtered-out
// items are also skipped.
func (p *Picker) moveCursor(dir int) {
	indices := p.viewIndices()

	// Find current position in visible list.
	curPos := -1
	for vi, i := range indices {
		if i == p.cursor {
			curPos = vi
			break
		}
	}
	// Handle search action row.
	if p.hasSearchAction && p.cursor == len(p.items) {
		curPos = len(indices)
	}

	// Move within visible items, skipping headers/descriptions.
	nextPos := curPos + dir
	for nextPos >= 0 && nextPos <= len(indices) {
		// Search action row.
		if nextPos == len(indices) && p.hasSearchAction {
			p.cursor = len(p.items)
			p.clampScroll()
			return
		}
		if nextPos < len(indices) {
			realIdx := indices[nextPos]
			if !p.items[realIdx].IsHeader && p.items[realIdx].Description == "" {
				p.cursor = realIdx
				p.clampScroll()
				return
			}
		}
		nextPos += dir
	}
}

// toggleCurrent toggles the selection of the item at the cursor position.
// Headers, descriptions, and read-only items are not togglable.
func (p *Picker) toggleCurrent() {
	if p.cursor >= 0 && p.cursor < len(p.items) && isSelectableItem(p.items[p.cursor]) && !p.items[p.cursor].IsReadOnly {
		p.items[p.cursor].Selected = !p.items[p.cursor].Selected
		p.syncSelectAll()
	}
}

// doSelectAll selects all visible selectable items.
func (p *Picker) doSelectAll() {
	for i := range p.items {
		if isSelectableItem(p.items[i]) && !p.isFilteredOut(i) {
			p.items[i].Selected = true
		}
	}
	p.syncSelectAll()
}

// doSelectNone deselects all visible selectable items.
func (p *Picker) doSelectNone() {
	for i := range p.items {
		if isSelectableItem(p.items[i]) && !p.isFilteredOut(i) {
			p.items[i].Selected = false
		}
	}
	p.syncSelectAll()
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
// the scroll offset. Works with visible indices for filter support.
func (p *Picker) clampScroll() {
	if p.height <= 0 {
		return
	}

	indices := p.viewIndices()
	totalRows := len(indices)
	if p.hasSearchAction {
		totalRows++
	}

	// Filter bar takes 1 line.
	itemHeight := p.height - 1
	if itemHeight < 1 {
		itemHeight = 1
	}

	// When items overflow, scroll indicators take up to 2 lines.
	effectiveHeight := itemHeight
	if totalRows > itemHeight {
		effectiveHeight -= 2
	}
	if effectiveHeight < 1 {
		effectiveHeight = 1
	}

	cursorPos := p.cursorVisiblePos(indices)
	if cursorPos < p.offset {
		p.offset = cursorPos
	}
	if cursorPos >= p.offset+effectiveHeight {
		p.offset = cursorPos - effectiveHeight + 1
	}
	maxOffset := totalRows - effectiveHeight
	if maxOffset < 0 {
		maxOffset = 0
	}
	if p.offset > maxOffset {
		p.offset = maxOffset
	}
}
