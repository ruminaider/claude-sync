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

// FilterChip represents a toggleable filter chip in the chip bar.
type FilterChip int

const (
	ChipAll        FilterChip = iota
	ChipSelected              // matches Selected == true
	ChipUnselected            // matches Selected == false
	ChipBase                  // matches IsBase == true (profile tabs only)
	ChipLocked                // matches IsReadOnly == true
)

// PickerItem represents a single row in the multi-select picker.
type PickerItem struct {
	Key         string // unique identifier (plugin key, setting key, etc.)
	Display     string // display text
	Selected    bool
	IsHeader    bool   // section header, not selectable
	IsBase      bool   // inherited from base config (profile view)
	IsReadOnly  bool   // read-only items cannot be toggled (e.g. plugin-provided commands)
	Tag         string // type indicator: "[cmd]", "[skill]", etc.
	ProviderTag string // provenance: "via plugin-name" (only shown when IsReadOnly)
	Description string // optional description rendered below headers
}

// effectiveTag computes the display tag based on the item's state.
// Read-only items show Tag + ProviderTag (explains why locked).
// Base-inherited items show "●" + Tag (indicates inheritance).
// Normal items show Tag only.
func (it PickerItem) effectiveTag() string {
	var parts []string
	if it.IsReadOnly {
		if it.Tag != "" {
			parts = append(parts, it.Tag)
		}
		if it.ProviderTag != "" {
			parts = append(parts, it.ProviderTag)
		}
	} else if it.IsBase {
		parts = append(parts, "●")
		if it.Tag != "" {
			parts = append(parts, it.Tag)
		}
	} else {
		if it.Tag != "" {
			parts = append(parts, it.Tag)
		}
	}
	return strings.Join(parts, " ")
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

	// Collapse support: headers can be collapsed to hide their children.
	collapsed        map[int]bool // header indices that are currently collapsed
	CollapseReadOnly bool         // auto-collapse headers whose children are all read-only

	// Search status indicator.
	searching bool

	// Chip bar: toggleable filter chips above the text search bar.
	chipFocused  bool                // true when chip bar has focus (vs item list)
	chipCursor   int                 // index of focused chip in availableChips()
	activeChips  map[FilterChip]bool // which chips are toggled on
	isProfileTab bool                // controls whether Base chip is shown
}

// NewPicker creates a Picker with the given items. The cursor is placed on the
// first selectable (non-header, non-description) item.
func NewPicker(items []PickerItem) Picker {
	p := Picker{
		items:       items,
		height:      20, // sensible default
		collapsed:   make(map[int]bool),
		activeChips: map[FilterChip]bool{ChipAll: true},
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

// MCPPickerItems builds picker items from MCP server names. When source is
// non-empty and there are servers, a group header is prepended. All items are
// pre-selected.
func MCPPickerItems(mcp map[string]json.RawMessage, source string) []PickerItem {
	keys := make([]string, 0, len(mcp))
	for k := range mcp {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var items []PickerItem
	if source != "" && len(keys) > 0 {
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("%s (%d)", source, len(keys)),
			IsHeader: true,
		})
	}
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
			Display:  fmt.Sprintf("Installed plugins (%d)", len(pluginItems)),
			IsHeader: true,
		})
		items = append(items, PickerItem{
			Description: "Auto-detected from plugins — view only",
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
				Key:         item.Key(),
				Display:     item.Name,
				Selected:    true,
				IsReadOnly:  true,
				Tag:         typeTag,
				ProviderTag: "via " + item.SourceLabel,
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

// SetSearching sets whether the search action is currently in progress.
func (p *Picker) SetSearching(s bool) {
	p.searching = s
}

// SetCollapsed programmatically sets the collapsed state for a header index.
func (p *Picker) SetCollapsed(headerIdx int, collapsed bool) {
	if collapsed {
		p.collapsed[headerIdx] = true
	} else {
		delete(p.collapsed, headerIdx)
	}
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

// SetProfileTab controls whether the Base chip is available (profile tabs show it).
func (p *Picker) SetProfileTab(isProfile bool) {
	p.isProfileTab = isProfile
}

// availableChips returns the chips visible for the current context.
func (p Picker) availableChips() []FilterChip {
	chips := []FilterChip{ChipAll, ChipSelected, ChipUnselected}
	if p.isProfileTab {
		chips = append(chips, ChipBase)
	}
	chips = append(chips, ChipLocked)
	return chips
}

// toggleChip handles mutual exclusion and auto-All logic.
func (p *Picker) toggleChip(chip FilterChip) {
	if chip == ChipAll {
		p.activeChips = map[FilterChip]bool{ChipAll: true}
		return
	}

	delete(p.activeChips, ChipAll)

	if p.activeChips[chip] {
		delete(p.activeChips, chip)
	} else {
		p.activeChips[chip] = true
		if chip == ChipSelected {
			delete(p.activeChips, ChipUnselected)
		} else if chip == ChipUnselected {
			delete(p.activeChips, ChipSelected)
		}
	}

	if len(p.activeChips) == 0 {
		p.activeChips[ChipAll] = true
	}
}

// itemPassesChipFilter checks if an item passes the active chip filters (AND logic).
func (p Picker) itemPassesChipFilter(it PickerItem) bool {
	if p.activeChips[ChipAll] {
		return true
	}
	if p.activeChips[ChipSelected] && !it.Selected {
		return false
	}
	if p.activeChips[ChipUnselected] && it.Selected {
		return false
	}
	if p.activeChips[ChipBase] && !it.IsBase {
		return false
	}
	if p.activeChips[ChipLocked] && !it.IsReadOnly {
		return false
	}
	return true
}

// hasActiveChipFilter returns true when chips are filtering (not All).
func (p Picker) hasActiveChipFilter() bool {
	return !p.activeChips[ChipAll]
}

// isAtFirstVisible returns true if the cursor is on or before the first visible item.
func (p Picker) isAtFirstVisible() bool {
	indices := p.viewIndices()
	if len(indices) == 0 {
		return true
	}
	return p.cursor <= indices[0]
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

// refilter recomputes filterView based on current filterText and active chip filters.
// When both are inactive, filterView is set to nil (show all).
func (p *Picker) refilter() {
	hasTextFilter := p.filterText != ""
	hasChipFilter := p.hasActiveChipFilter()

	if !hasTextFilter && !hasChipFilter {
		p.filterView = nil
		return
	}

	needle := strings.ToLower(p.filterText)

	// First pass: determine which selectable items match both filters.
	matches := make([]bool, len(p.items))
	for i, it := range p.items {
		if it.IsHeader || it.Description != "" {
			continue
		}
		// Chip filter.
		if hasChipFilter && !p.itemPassesChipFilter(it) {
			continue
		}
		// Text filter.
		if hasTextFilter {
			haystack := strings.ToLower(it.Display + " " + it.effectiveTag())
			if !strings.Contains(haystack, needle) {
				continue
			}
		}
		matches[i] = true
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
	p.collapsed = make(map[int]bool)
	p.activeChips = map[FilterChip]bool{ChipAll: true}
	p.chipFocused = false
	p.chipCursor = 0
	if p.CollapseReadOnly {
		p.autoCollapseReadOnly()
	}
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

		// When chip bar is focused, handle chip navigation.
		if p.chipFocused {
			switch msg.String() {
			case "left":
				if p.chipCursor > 0 {
					p.chipCursor--
				}
			case "right":
				chips := p.availableChips()
				if p.chipCursor < len(chips)-1 {
					p.chipCursor++
				}
			case " ", "enter":
				chips := p.availableChips()
				if p.chipCursor >= 0 && p.chipCursor < len(chips) {
					p.toggleChip(chips[p.chipCursor])
					p.refilter()
					p.resetCursorToFirstVisible()
				}
			case "down":
				p.chipFocused = false
			case "esc":
				p.activeChips = map[FilterChip]bool{ChipAll: true}
				p.chipFocused = false
				p.refilter()
				p.resetCursorToFirstVisible()
			default:
				// Printable runes start text search and return to item list.
				if msg.Type == tea.KeyRunes && len(msg.Runes) > 0 {
					p.chipFocused = false
					p.filterText += string(msg.Runes)
					p.refilter()
					p.resetCursorToFirstVisible()
				}
			}
			return p, nil
		}

		switch msg.String() {
		case "up":
			if p.isAtFirstVisible() {
				p.chipFocused = true
				return p, nil
			}
			p.moveCursor(-1)
		case "down":
			p.moveCursor(+1)
		case " ":
			// Space on the search action row is a no-op.
			if !(p.hasSearchAction && p.cursor == len(p.items)) {
				p.toggleCurrent()
			}
		case "enter":
			// Enter on a header toggles its collapsed state.
			if p.cursor >= 0 && p.cursor < len(p.items) && p.items[p.cursor].IsHeader {
				if p.collapsed[p.cursor] {
					delete(p.collapsed, p.cursor)
				} else {
					p.collapsed[p.cursor] = true
				}
				p.clampScroll()
				return p, nil
			}
			// Enter on the search action row emits SearchRequestMsg.
			if p.hasSearchAction && p.cursor == len(p.items) {
				if p.searching {
					return p, nil // ignore re-trigger while searching
				}
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
			return p, func() tea.Msg {
				return FocusChangeMsg{Zone: FocusSidebar}
			}
		case "esc":
			if p.filterText != "" {
				p.filterText = ""
				p.refilter()
				p.resetCursorToFirstVisible()
				return p, nil
			}
			if p.hasActiveChipFilter() {
				p.activeChips = map[FilterChip]bool{ChipAll: true}
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

	// Render chip bar + filter bar (2 lines total).
	b.WriteString(p.renderChipBar())
	b.WriteString("\n")
	b.WriteString(p.renderFilterBar())
	b.WriteString("\n")

	// Determine visible item indices.
	indices := p.viewIndices()
	totalRows := len(indices)
	if p.hasSearchAction {
		totalRows++
	}

	if totalRows == 0 {
		if p.filterText != "" || p.hasActiveChipFilter() {
			return ContentPaneStyle.Render(b.String() + "  No matches")
		}
		return ContentPaneStyle.Render(b.String() + "  (no items)")
	}

	// Available height for items (subtract 2 for chip bar + filter bar).
	itemHeight := p.height - 2
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

	for vi := offset; vi < end; vi++ {
		// Virtual search action row at the end of visible items.
		if vi == len(indices) && p.hasSearchAction {
			cursor := "  "
			realIdx := len(p.items) // virtual row
			isCurrent := p.focused && p.cursor == realIdx
			if isCurrent {
				cursor = "> "
			}
			b.WriteString(cursor + RenderSearchAction(p.focused, isCurrent, p.searching) + "\n")
			continue
		}

		i := indices[vi] // real item index
		it := p.items[i]
		isCurrent := p.focused && i == p.cursor

		if it.IsHeader {
			indicator := "▾"
			if p.collapsed[i] {
				indicator = "▸"
			}
			line := fmt.Sprintf("── %s %s ──", indicator, it.Display)
			b.WriteString(RenderHeader(line, p.focused, isCurrent))
			b.WriteString("\n")
			continue
		}

		if it.Description != "" {
			b.WriteString("  " + DimStyle.Render(it.Description) + "\n")
			continue
		}

		// Cursor indicator: only show when focused.
		cursor := "  "
		if isCurrent {
			cursor = "> "
		}

		// Removed inherited item: muted red strikethrough.
		if it.IsBase && !it.Selected {
			b.WriteString(cursor + RenderRemovedBaseLine(it.Display, it.effectiveTag(), p.focused) + "\n")
			continue
		}

		// Read-only (plugin-controlled) item: lock icon, muted style.
		if it.IsReadOnly {
			b.WriteString(cursor + RenderLockedLine(it.Display, it.effectiveTag(), p.focused) + "\n")
			continue
		}

		checkbox := RenderCheckbox(p.focused, it.Selected)
		display := RenderItemText(it.Display, p.focused, isCurrent)
		tag := RenderTag(it.effectiveTag(), p.focused, p.tagColor)

		b.WriteString(cursor + checkbox + " " + display + tag + "\n")
	}

	if end < totalRows {
		b.WriteString(lipgloss.NewStyle().Foreground(colorOverlay0).Render("  ↓ more") + "\n")
	}

	return ContentPaneStyle.Render(strings.TrimRight(b.String(), "\n"))
}

// viewIndices returns the list of item indices to render.
// When a filter is active, returns filterView (ignores collapsed state).
// Otherwise, returns all items except children of collapsed headers.
func (p Picker) viewIndices() []int {
	if p.filterView != nil {
		return p.filterView
	}
	var indices []int
	for i := range p.items {
		if p.items[i].IsHeader {
			indices = append(indices, i)
			continue
		}
		// Skip non-header items under collapsed headers.
		header := p.headerForItem(i)
		if header >= 0 && p.collapsed[header] {
			continue
		}
		indices = append(indices, i)
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

// chipLabel returns the display label for a filter chip.
func chipLabel(c FilterChip) string {
	switch c {
	case ChipAll:
		return "All"
	case ChipSelected:
		return "✓ Sel"
	case ChipUnselected:
		return "○ Unsel"
	case ChipBase:
		return "● Base"
	case ChipLocked:
		return "⊘ Lock"
	}
	return ""
}

// chipStyle returns the lipgloss style for a chip based on its state.
// Active chips get a green filled pill. Focused chips get peach (orange) bg
// when inactive, or bold green when active. Arrow markers are added in render.
func chipStyle(active, focused, pickerFocused bool) lipgloss.Style {
	if focused {
		if active {
			return lipgloss.NewStyle().
				Foreground(colorBase).
				Background(colorGreen).
				Bold(true)
		}
		return lipgloss.NewStyle().
			Foreground(colorBase).
			Background(colorPeach).
			Bold(true)
	}
	if active {
		return lipgloss.NewStyle().
			Foreground(colorBase).
			Background(colorGreen)
	}
	if pickerFocused {
		return lipgloss.NewStyle().
			Foreground(colorOverlay0)
	}
	return lipgloss.NewStyle().
		Foreground(colorSurface1)
}

// renderChipBar renders the row of filter chips above the text search bar.
// Focused chip gets ▸◂ arrow markers for clear cursor indication.
func (p Picker) renderChipBar() string {
	chips := p.availableChips()
	var parts []string
	for i, chip := range chips {
		label := chipLabel(chip)
		active := p.activeChips[chip]
		focused := p.chipFocused && p.chipCursor == i
		style := chipStyle(active, focused, p.focused)

		pill := style.Render(" " + label + " ")
		if focused {
			// Arrow markers around the focused chip.
			arrow := lipgloss.NewStyle().Foreground(colorPeach)
			parts = append(parts, arrow.Render("▸")+pill+arrow.Render("◂"))
		} else if active {
			parts = append(parts, pill)
		} else {
			parts = append(parts, style.Render("("+label+")"))
		}
	}
	row := strings.Join(parts, " ")
	prefix := DimStyle.Render("Filters:")
	return " " + prefix + " " + row
}

// renderFilterBar renders the filter input line with a distinct background.
func (p Picker) renderFilterBar() string {
	barBg := colorSurface0

	// Build inner content.
	var inner string
	if p.filterText == "" {
		// Placeholder when empty.
		placeholder := lipgloss.NewStyle().Foreground(colorOverlay0).Background(barBg).Render("Type to search...")
		inner = " " + placeholder
	} else {
		label := lipgloss.NewStyle().Foreground(colorOverlay0).Background(barBg).Render("Filter: ")
		text := lipgloss.NewStyle().Foreground(colorText).Background(barBg).Render(p.filterText)
		cursor := lipgloss.NewStyle().Foreground(colorText).Background(barBg).Render("│")
		inner = " " + label + text + cursor
	}

	// Right-aligned match count when any filter is active.
	right := ""
	if p.filterText != "" || p.hasActiveChipFilter() {
		visible := p.visibleSelectableCount()
		total := p.TotalCount()
		right = DimStyle.Background(barBg).Render(fmt.Sprintf("%d/%d", visible, total))
	}

	if right != "" {
		gap := p.width - lipgloss.Width(inner) - lipgloss.Width(right) - 2
		if gap < 1 {
			gap = 1
		}
		inner = inner + lipgloss.NewStyle().Background(barBg).Render(strings.Repeat(" ", gap)) + right
	}

	// Render the full-width bar with background.
	barStyle := lipgloss.NewStyle().
		Background(barBg).
		Width(p.width)
	return barStyle.Render(inner)
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

	// Word-wrap lines to fit preview width instead of truncating.
	previewStr := lipgloss.NewStyle().Width(previewWidth).Render(
		strings.Join(visible, "\n"),
	)
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
		topBar = " " + DimStyle.Render("Filter: ") + p.filterText + "\n"
	}

	// Render a compact list on the left showing only visible item names.
	indices := p.viewIndices()
	var listB strings.Builder
	for _, idx := range indices {
		it := p.items[idx]
		if it.IsHeader || it.Description != "" {
			continue
		}
		isCurrent := p.focused && idx == p.cursor
		cursor := "  "
		if isCurrent {
			cursor = "> "
		}
		display := it.Display
		if len(display) > listWidth-4 {
			display = display[:listWidth-7] + "..."
		}
		display = RenderItemText(display, p.focused, isCurrent)
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

// isSkippable returns true if the item at index i should be skipped for initial
// cursor placement (headers and descriptions are skipped, as are items under
// collapsed headers and filtered-out items).
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
	// Items under a collapsed header are skippable.
	header := p.headerForItem(i)
	if header >= 0 && p.collapsed[header] {
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
			if p.items[realIdx].Description == "" {
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

// headerForItem returns the index of the nearest preceding header for item i,
// or -1 if there is no preceding header.
func (p *Picker) headerForItem(i int) int {
	for j := i - 1; j >= 0; j-- {
		if p.items[j].IsHeader {
			return j
		}
	}
	return -1
}

// autoCollapseReadOnly collapses headers whose children are all read-only.
func (p *Picker) autoCollapseReadOnly() {
	for i, it := range p.items {
		if !it.IsHeader {
			continue
		}
		allReadOnly := true
		hasChildren := false
		for j := i + 1; j < len(p.items); j++ {
			if p.items[j].IsHeader {
				break
			}
			if p.items[j].Description != "" {
				continue // skip description rows
			}
			hasChildren = true
			if !p.items[j].IsReadOnly {
				allReadOnly = false
				break
			}
		}
		if hasChildren && allReadOnly {
			p.collapsed[i] = true
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

	// Chip bar + filter bar take 2 lines.
	itemHeight := p.height - 2
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
