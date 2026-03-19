package tui

import (
	"fmt"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
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
	confirmed     bool
	newSelections []string // keys of newly selected plugins
}

// NewPluginBrowser creates a PluginBrowser from the current menu state.
// Items are organized by marketplace section, with installed plugins pre-checked.
func NewPluginBrowser(state commands.MenuState, width, height int) PluginBrowser {
	items := buildPluginBrowserItems(state.Plugins)

	m := PluginBrowser{
		items:  items,
		width:  width,
		height: height,
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
	case tea.KeyMsg:
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
		hasChanges := false
		var newSelections []string
		for _, item := range m.items {
			if item.isHeader {
				continue
			}
			if item.selected != item.installed {
				hasChanges = true
			}
			if item.selected && !item.installed {
				newSelections = append(newSelections, item.key)
			}
		}
		m.newSelections = newSelections
		return m, func() tea.Msg {
			return subViewCloseMsg{refreshState: hasChanges}
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
	maxWidth := m.width - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}

	innerWidth := maxWidth - 6
	if innerWidth < 20 {
		innerWidth = 20
	}

	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	boldBlue := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	sectionStyle := lipgloss.NewStyle().Foreground(colorSurface1)

	var lines []string

	lines = append(lines, headerStyle.Render("Add or discover new plugins"))
	lines = append(lines, "")

	// Filter line
	if m.filterMode {
		lines = append(lines, boldBlue.Render("/ "+m.filterText+"\u2588"))
	} else {
		lines = append(lines, dimStyle.Render("/ filter..."))
	}
	lines = append(lines, "")

	// No plugins state
	if len(m.items) == 0 {
		lines = append(lines, dimStyle.Render("No plugins installed."))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("Subscribe to more via 'Join a shared config'."))
		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("esc back"))
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
				lines = append(lines, sectionStyle.Render(headerLine))
				continue
			}

			hasVisiblePlugin = true

			// Checkbox
			checkbox := "[ ]"
			if item.selected {
				checkbox = greenStyle.Render("[\u2713]")
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
				label := fmt.Sprintf("%s %s", checkbox, boldBlue.Render(nameStr))
				metaText := dimStyle.Render(meta)
				gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(metaText)
				if gap < 1 {
					gap = 1
				}
				lines = append(lines, label+strings.Repeat(" ", gap)+metaText)
			} else {
				label := fmt.Sprintf("%s %s", checkbox, textStyle.Render(nameStr))
				metaText := dimStyle.Render(meta)
				gap := innerWidth - lipgloss.Width(label) - lipgloss.Width(metaText)
				if gap < 1 {
					gap = 1
				}
				lines = append(lines, label+strings.Repeat(" ", gap)+metaText)
			}
		}

		if !hasVisiblePlugin && m.filterText != "" {
			lines = append(lines, dimStyle.Render("No plugins match filter."))
		}

		lines = append(lines, "")

		// Footer info
		allInstalled := true
		for _, item := range m.items {
			if !item.isHeader && !item.installed {
				allInstalled = false
				break
			}
		}
		if allInstalled && len(m.items) > 0 {
			lines = append(lines, dimStyle.Render("All plugins are currently installed."))
		}
		lines = append(lines, dimStyle.Render("Subscribe to more via 'Join a shared config'."))

		lines = append(lines, "")
		lines = append(lines, dimStyle.Render("space toggle  enter confirm  / filter  esc back"))
	}

	content := strings.Join(lines, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}
