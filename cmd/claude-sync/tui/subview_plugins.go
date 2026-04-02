package tui

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/sliceutil"
)

// pluginBrowserItem represents a single row in the plugin browser (either a header or a plugin).
type pluginBrowserItem struct {
	key         string // full key: name@marketplace
	name        string
	desc        string // short description
	version     string
	marketplace string
	status      string // "upstream", "pinned", "forked"
	installed   bool   // already installed
	selected    bool   // user toggled this on
	isHeader    bool   // marketplace section header (not selectable)
}

// pluginBrowserResultMsg carries the outcome of applying plugin selections.
type pluginBrowserResultMsg struct {
	success bool
	message string
	err     error
}

// PluginBrowser is a sub-view for browsing and selecting plugins.
type PluginBrowser struct {
	items      []pluginBrowserItem
	cursor     int
	width      int
	height     int
	cancelled  bool
	filterText string
	filterMode bool
	// After confirmation
	confirmed         bool
	newSelections     []string // keys of newly selected plugins
	removedSelections []string // keys of deselected installed plugins

	// Paths for execution
	syncDir string

	// Execution state
	executing  bool
	resultDone bool
	resultOk   bool
	resultMsg  string
}

// NewPluginBrowser creates a PluginBrowser from the current menu state.
// Items are organized by marketplace section, with installed plugins pre-checked.
func NewPluginBrowser(state commands.MenuState, syncDir string, width, height int) PluginBrowser {
	items := buildPluginBrowserItems(state.Plugins)

	m := PluginBrowser{
		items:   items,
		syncDir: syncDir,
		width:   width,
		height:  height,
	}

	// Position cursor on the first non-header item
	for i, item := range m.items {
		if !item.isHeader {
			m.cursor = i
			break
		}
	}

	return m
}

// buildPluginBrowserItems groups plugins by marketplace and builds the item list.
func buildPluginBrowserItems(plugins []commands.PluginInfo) []pluginBrowserItem {
	if len(plugins) == 0 {
		return nil
	}

	// Group by marketplace, preserving order of first occurrence
	type mktGroup struct {
		name    string
		plugins []commands.PluginInfo
	}
	seen := map[string]int{}
	var groups []mktGroup

	for _, p := range plugins {
		mkt := p.Marketplace
		if mkt == "" {
			mkt = "default"
		}
		if idx, ok := seen[mkt]; ok {
			groups[idx].plugins = append(groups[idx].plugins, p)
		} else {
			seen[mkt] = len(groups)
			groups = append(groups, mktGroup{name: mkt, plugins: []commands.PluginInfo{p}})
		}
	}

	// Sort groups alphabetically for stable output
	sort.Slice(groups, func(i, j int) bool {
		return groups[i].name < groups[j].name
	})

	var items []pluginBrowserItem
	for _, g := range groups {
		// Section header
		items = append(items, pluginBrowserItem{
			isHeader:    true,
			marketplace: g.name,
			name:        g.name,
		})

		// Plugins under this marketplace
		for _, p := range g.plugins {
			version := p.PinVersion
			if version == "" {
				version = p.LatestVersion
			}
			items = append(items, pluginBrowserItem{
				key:         p.Key,
				name:        p.Name,
				version:     version,
				marketplace: p.Marketplace,
				status:      p.Status,
				installed:   true, // all current plugins are installed
				selected:    true, // pre-checked because installed
			})
		}
	}

	return items
}

func (m PluginBrowser) Init() tea.Cmd {
	return nil
}

func (m PluginBrowser) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case pluginBrowserResultMsg:
		m.executing = false
		m.resultDone = true
		m.resultOk = msg.success
		m.resultMsg = resolveResultMsg(msg.success, msg.message, msg.err)
		return m, nil
	case tea.KeyMsg:
		// After result is shown, any key dismisses
		if m.resultDone {
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: m.resultOk}
			}
		}
		// While executing, ignore input
		if m.executing {
			return m, nil
		}
		if m.filterMode {
			return m.updateFilterMode(msg)
		}
		return m.updateNormalMode(msg)
	}
	return m, nil
}

func (m PluginBrowser) updateNormalMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.cancelled = true
		return m, func() tea.Msg {
			return subViewCloseMsg{refreshState: false}
		}
	case "j", "down":
		m.cursor = m.nextSelectableItem(m.cursor)
	case "k", "up":
		m.cursor = m.prevSelectableItem(m.cursor)
	case " ":
		if m.cursor >= 0 && m.cursor < len(m.items) && !m.items[m.cursor].isHeader {
			m.items[m.cursor].selected = !m.items[m.cursor].selected
		}
	case "/":
		m.filterMode = true
	case "enter":
		m.confirmed = true
		// Check if selections changed from initial installed state
		var newSelections []string
		var removedSelections []string
		for _, item := range m.items {
			if item.isHeader {
				continue
			}
			if item.selected && !item.installed {
				newSelections = append(newSelections, item.key)
			}
			if !item.selected && item.installed {
				removedSelections = append(removedSelections, item.key)
			}
		}
		m.newSelections = newSelections
		m.removedSelections = removedSelections

		if len(newSelections) > 0 || len(removedSelections) > 0 {
			m.executing = true
			return m, applyPluginSelections(m.syncDir, newSelections, removedSelections)
		}
		// No changes, close immediately
		return m, func() tea.Msg {
			return subViewCloseMsg{refreshState: false}
		}
	}
	return m, nil
}

func (m PluginBrowser) updateFilterMode(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.Type {
	case tea.KeyEscape:
		m.filterMode = false
		m.filterText = ""
		// Reset cursor to first visible non-header
		for i, item := range m.items {
			if !item.isHeader {
				m.cursor = i
				break
			}
		}
		return m, nil
	case tea.KeyBackspace:
		if len(m.filterText) > 0 {
			m.filterText = m.filterText[:len(m.filterText)-1]
		}
		m.adjustCursorForFilter()
		return m, nil
	case tea.KeyEnter:
		// Confirm from filter mode
		m.filterMode = false
		return m, nil
	case tea.KeyRunes:
		m.filterText += string(msg.Runes)
		m.adjustCursorForFilter()
		return m, nil
	}
	return m, nil
}

// adjustCursorForFilter ensures the cursor is on a visible, non-header item after filter changes.
func (m *PluginBrowser) adjustCursorForFilter() {
	visible := m.visibleItems()
	// Check if current cursor item is visible and non-header
	if m.cursor >= 0 && m.cursor < len(m.items) {
		if _, ok := visible[m.cursor]; ok && !m.items[m.cursor].isHeader {
			return
		}
	}
	// Find first visible non-header
	for i, item := range m.items {
		if _, ok := visible[i]; ok && !item.isHeader {
			m.cursor = i
			return
		}
	}
}

// nextSelectableItem returns the index of the next non-header item after pos.
func (m PluginBrowser) nextSelectableItem(pos int) int {
	visible := m.visibleItems()
	for i := pos + 1; i < len(m.items); i++ {
		if _, ok := visible[i]; ok && !m.items[i].isHeader {
			return i
		}
	}
	return pos
}

// prevSelectableItem returns the index of the previous non-header item before pos.
func (m PluginBrowser) prevSelectableItem(pos int) int {
	visible := m.visibleItems()
	for i := pos - 1; i >= 0; i-- {
		if _, ok := visible[i]; ok && !m.items[i].isHeader {
			return i
		}
	}
	return pos
}

// visibleItems returns a set of item indices that are visible given the current filter.
// Headers are visible if they have at least one visible child.
func (m PluginBrowser) visibleItems() map[int]struct{} {
	visible := make(map[int]struct{})
	filter := strings.ToLower(m.filterText)

	if filter == "" {
		// No filter: all items visible
		for i := range m.items {
			visible[i] = struct{}{}
		}
		return visible
	}

	// First pass: mark matching non-header items
	for i, item := range m.items {
		if item.isHeader {
			continue
		}
		if strings.Contains(strings.ToLower(item.name), filter) {
			visible[i] = struct{}{}
		}
	}

	// Second pass: mark headers that have at least one visible child
	for i, item := range m.items {
		if !item.isHeader {
			continue
		}
		// Check if any subsequent non-header items (before next header) are visible
		for j := i + 1; j < len(m.items); j++ {
			if m.items[j].isHeader {
				break
			}
			if _, ok := visible[j]; ok {
				visible[i] = struct{}{}
				break
			}
		}
	}

	return visible
}

func (m PluginBrowser) View() string {
	maxWidth, innerWidth := clampWidth(m.width)

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)

	var lines []string

	lines = append(lines, headerStyle.Render("Add or discover new plugins"))
	lines = append(lines, "")

	// Result state
	if m.resultDone {
		lines = renderResultLines(lines, m.resultOk, m.resultMsg)
		content := strings.Join(lines, "\n")
		boxStyle := contentBox(maxWidth, colorSurface1)
		return boxStyle.Render(content)
	}

	// Executing state
	if m.executing {
		lines = append(lines, stYellow.Render("\u27f3 Applying plugin changes..."))
		content := strings.Join(lines, "\n")
		boxStyle := contentBox(maxWidth, colorSurface1)
		return boxStyle.Render(content)
	}

	// Filter line
	if m.filterMode {
		lines = append(lines, stBlue.Render("/ "+m.filterText+"\u2588"))
	} else {
		lines = append(lines, stDim.Render("/ filter..."))
	}
	lines = append(lines, "")

	// No plugins state
	if len(m.items) == 0 {
		lines = append(lines, stDim.Render("No plugins installed."))
		lines = append(lines, "")
		lines = append(lines, stDim.Render("Subscribe to more via 'Join a shared config'."))
		lines = append(lines, "")
		lines = append(lines, stDim.Render("esc back"))
	} else {
		visible := m.visibleItems()
		hasVisiblePlugin := false

		for i, item := range m.items {
			if _, ok := visible[i]; !ok {
				continue
			}

			if item.isHeader {
				// Section header
				headerLine := fmt.Sprintf("\u2500\u2500 %s ", item.marketplace)
				remaining := innerWidth - lipgloss.Width(headerLine)
				if remaining > 0 {
					headerLine += strings.Repeat("\u2500", remaining)
				}
				lines = append(lines, stSection.Render(headerLine))
				continue
			}

			hasVisiblePlugin = true

			// Checkbox
			checkbox := "[ ]"
			if item.selected {
				checkbox = stGreen.Render("[\u2713]")
			}

			// Status and version
			meta := item.status
			if item.version != "" {
				meta += "  v" + item.version
			}

			// Build the line
			nameStr := item.name
			isCurrent := m.cursor == i

			if isCurrent {
				label := fmt.Sprintf("%s %s", checkbox, stBlue.Render(nameStr))
				metaText := stDim.Render(meta)
				gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(metaText)
				if gap < 1 {
					gap = 1
				}
				lines = append(lines, label+strings.Repeat(" ", gap)+metaText)
			} else {
				label := fmt.Sprintf("%s %s", checkbox, stText.Render(nameStr))
				metaText := stDim.Render(meta)
				gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(metaText)
				if gap < 1 {
					gap = 1
				}
				lines = append(lines, label+strings.Repeat(" ", gap)+metaText)
			}
		}

		if !hasVisiblePlugin && m.filterText != "" {
			lines = append(lines, stDim.Render("No plugins match filter."))
		}

		lines = append(lines, "")

		// Footer info: dynamic selection counts
		var installed, toAdd, toRemove int
		for _, item := range m.items {
			if item.isHeader {
				continue
			}
			if item.installed && item.selected {
				installed++
			} else if !item.installed && item.selected {
				toAdd++
			} else if item.installed && !item.selected {
				toRemove++
			}
		}
		var counts []string
		if installed > 0 {
			counts = append(counts, fmt.Sprintf("%d installed", installed))
		}
		if toAdd > 0 {
			counts = append(counts, fmt.Sprintf("%d to add", toAdd))
		}
		if toRemove > 0 {
			counts = append(counts, fmt.Sprintf("%d to remove", toRemove))
		}
		if len(counts) > 0 {
			lines = append(lines, stDim.Render(strings.Join(counts, ", ")))
		} else {
			lines = append(lines, stDim.Render("No plugins selected."))
		}
		lines = append(lines, stDim.Render("Subscribe to more via 'Join a shared config'."))

		lines = append(lines, "")
		lines = append(lines, stDim.Render("space toggle  enter confirm  / filter  esc back"))
	}

	content := strings.Join(lines, "\n")

	boxStyle := contentBox(maxWidth, colorSurface1)

	return boxStyle.Render(content)
}

// applyPluginSelections modifies config.yaml to add or exclude plugins, then commits.
func applyPluginSelections(syncDir string, toAdd, toRemove []string) tea.Cmd {
	if syncDir == "" {
		return func() tea.Msg {
			return pluginBrowserResultMsg{
				success: false,
				message: "Sync directory is not configured",
				err:     fmt.Errorf("syncDir is empty"),
			}
		}
	}

	return func() (result tea.Msg) {
		defer func() {
			if r := recover(); r != nil {
				result = pluginBrowserResultMsg{
					success: false,
					message: fmt.Sprintf("Internal error: %v", r),
					err:     fmt.Errorf("panic: %v", r),
				}
			}
		}()

		cfgPath := filepath.Join(syncDir, "config.yaml")
		cfgData, err := os.ReadFile(cfgPath)
		if err != nil {
			return pluginBrowserResultMsg{
				success: false,
				message: "Could not read config.yaml",
				err:     err,
			}
		}

		cfg, err := config.Parse(cfgData)
		if err != nil {
			return pluginBrowserResultMsg{
				success: false,
				message: "Could not parse config.yaml",
				err:     err,
			}
		}

		// Add newly selected plugins to Upstream, remove them from Excluded
		if len(toAdd) > 0 {
			cfg.Upstream = sliceutil.AppendUnique(cfg.Upstream, toAdd)
			cfg.Excluded = sliceutil.RemoveAll(cfg.Excluded, toAdd)
		}

		// Add deselected plugins to Excluded, remove them from Upstream
		if len(toRemove) > 0 {
			cfg.Excluded = sliceutil.AppendUnique(cfg.Excluded, toRemove)
			cfg.Upstream = sliceutil.RemoveAll(cfg.Upstream, toRemove)
		}

		newData, err := config.Marshal(cfg)
		if err != nil {
			return pluginBrowserResultMsg{
				success: false,
				message: "Could not serialize config",
				err:     err,
			}
		}

		if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
			return pluginBrowserResultMsg{
				success: false,
				message: "Could not write config.yaml",
				err:     err,
			}
		}

		// From here, the file is modified on disk. Rollback on failure.
		if err := git.Add(syncDir, "config.yaml"); err != nil {
			os.WriteFile(cfgPath, cfgData, 0644) // restore original
			return pluginBrowserResultMsg{
				success: false,
				message: "Could not stage config.yaml",
				err:     err,
			}
		}

		var parts []string
		if len(toAdd) > 0 {
			parts = append(parts, fmt.Sprintf("added %d", len(toAdd)))
		}
		if len(toRemove) > 0 {
			parts = append(parts, fmt.Sprintf("excluded %d", len(toRemove)))
		}
		summary := strings.Join(parts, ", ")

		if err := git.Commit(syncDir, "Update plugins: "+summary); err != nil {
			os.WriteFile(cfgPath, cfgData, 0644) // restore original
			return pluginBrowserResultMsg{
				success: false,
				message: "Could not commit changes",
				err:     err,
			}
		}

		return pluginBrowserResultMsg{success: true, message: "Updated plugins: " + summary}
	}
}
