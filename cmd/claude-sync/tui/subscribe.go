package tui

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// SubscribeResult holds the user's selections from the subscribe TUI.
type SubscribeResult struct {
	Name       string
	Categories map[string]string   // category -> "all" or "none"
	Include    map[string][]string // items explicitly included (for "none" categories)
	Exclude    map[string][]string // items explicitly excluded (from "all" categories)
}

// SubscribeModel is a standalone Bubble Tea model for selecting subscription items.
type SubscribeModel struct {
	name       string
	url        string
	remoteCfg  config.Config
	picker     Picker
	categories []string
	activeCat  int
	width      int
	height     int
	ready      bool
	confirmed  bool
	cancelled  bool
}

// NewSubscribeModel creates a new subscription item picker.
func NewSubscribeModel(name, url string, remoteCfg config.Config) SubscribeModel {
	categories := availableCategories(remoteCfg)
	items := buildSubscribeItems(remoteCfg, categories)

	picker := NewPicker(items)
	picker.focused = true

	return SubscribeModel{
		name:       name,
		url:        url,
		remoteCfg:  remoteCfg,
		picker:     picker,
		categories: categories,
	}
}

func (m SubscribeModel) Init() tea.Cmd { return nil }

func (m SubscribeModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.picker.SetHeight(m.height - 6) // header + footer
		m.picker.SetWidth(m.width)
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.cancelled = true
			return m, tea.Quit
		case "enter":
			m.confirmed = true
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.picker, cmd = m.picker.Update(msg)
	return m, cmd
}

func (m SubscribeModel) View() string {
	if !m.ready {
		return "Loading..."
	}

	var b strings.Builder

	// Header.
	header := lipgloss.NewStyle().
		Bold(true).
		Foreground(colorBlue).
		Render(fmt.Sprintf("Subscribing to: %s (%s)", m.name, m.url))
	b.WriteString(header + "\n\n")

	// Picker.
	b.WriteString(m.picker.View())

	// Footer.
	footer := lipgloss.NewStyle().
		Foreground(colorSubtext0).
		Render("\n  Space: toggle  •  Enter: confirm  •  q: cancel")
	b.WriteString(footer)

	return b.String()
}

// Result returns the subscribe result, or nil if cancelled.
func (m SubscribeModel) Result() *SubscribeResult {
	if m.cancelled || !m.confirmed {
		return nil
	}

	selected := make(map[string]bool)
	for _, key := range m.picker.SelectedKeys() {
		selected[key] = true
	}

	result := &SubscribeResult{
		Name:       m.name,
		Categories: make(map[string]string),
		Include:    make(map[string][]string),
		Exclude:    make(map[string][]string),
	}

	// For each category, determine if the user selected all, none, or some items.
	for _, cat := range m.categories {
		allItems := CategoryItems(m.remoteCfg, cat)
		var selectedItems []string
		for _, item := range allItems {
			key := cat + ":" + item
			if selected[key] {
				selectedItems = append(selectedItems, item)
			}
		}

		if len(selectedItems) == len(allItems) {
			result.Categories[cat] = "all"
		} else if len(selectedItems) == 0 {
			result.Categories[cat] = "none"
		} else {
			// Determine if it's closer to "all - excludes" or "none + includes".
			if len(selectedItems) > len(allItems)/2 {
				result.Categories[cat] = "all"
				excludeSet := make(map[string]bool)
				for _, item := range allItems {
					excludeSet[item] = true
				}
				for _, item := range selectedItems {
					delete(excludeSet, item)
				}
				var excludes []string
				for item := range excludeSet {
					excludes = append(excludes, item)
				}
				sort.Strings(excludes)
				if len(excludes) > 0 {
					result.Exclude[cat] = excludes
				}
			} else {
				result.Categories[cat] = "none"
				sort.Strings(selectedItems)
				result.Include[cat] = selectedItems
			}
		}
	}

	return result
}

// buildSubscribeItems creates picker items grouped by category.
func buildSubscribeItems(cfg config.Config, categories []string) []PickerItem {
	var items []PickerItem

	for _, cat := range categories {
		catItems := CategoryItems(cfg, cat)
		if len(catItems) == 0 {
			continue
		}

		// Header for category.
		items = append(items, PickerItem{
			Display:  fmt.Sprintf("%s (%d)", categoryDisplayName(cat), len(catItems)),
			IsHeader: true,
		})

		// Items.
		for _, name := range catItems {
			items = append(items, PickerItem{
				Key:      cat + ":" + name,
				Display:  name,
				Selected: cat == "mcp" || cat == "plugins", // default: select MCP & plugins
			})
		}
	}

	return items
}

// CategoryItems returns the item names available in a category from the config.
func CategoryItems(cfg config.Config, category string) []string {
	switch category {
	case "mcp":
		keys := make([]string, 0, len(cfg.MCP))
		for k := range cfg.MCP {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	case "plugins":
		return cfg.AllPluginKeys()
	case "settings":
		keys := make([]string, 0, len(cfg.Settings))
		for k := range cfg.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	case "hooks":
		keys := make([]string, 0, len(cfg.Hooks))
		for k := range cfg.Hooks {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		return keys
	case "permissions":
		var all []string
		all = append(all, cfg.Permissions.Allow...)
		all = append(all, cfg.Permissions.Deny...)
		return all
	case "claude_md":
		return cfg.ClaudeMD.Include
	case "commands":
		return cfg.Commands
	case "skills":
		return cfg.Skills
	default:
		return nil
	}
}

func availableCategories(cfg config.Config) []string {
	var cats []string
	if len(cfg.MCP) > 0 {
		cats = append(cats, "mcp")
	}
	if len(cfg.AllPluginKeys()) > 0 {
		cats = append(cats, "plugins")
	}
	if len(cfg.Settings) > 0 {
		cats = append(cats, "settings")
	}
	if len(cfg.Hooks) > 0 {
		cats = append(cats, "hooks")
	}
	if len(cfg.Permissions.Allow) > 0 || len(cfg.Permissions.Deny) > 0 {
		cats = append(cats, "permissions")
	}
	if len(cfg.ClaudeMD.Include) > 0 {
		cats = append(cats, "claude_md")
	}
	if len(cfg.Commands) > 0 {
		cats = append(cats, "commands")
	}
	if len(cfg.Skills) > 0 {
		cats = append(cats, "skills")
	}
	return cats
}

func categoryDisplayName(cat string) string {
	names := map[string]string{
		"mcp":         "MCP Servers",
		"plugins":     "Plugins",
		"settings":    "Settings",
		"hooks":       "Hooks",
		"permissions": "Permissions",
		"claude_md":   "CLAUDE.md",
		"commands":    "Commands",
		"skills":      "Skills",
	}
	if name, ok := names[cat]; ok {
		return name
	}
	return cat
}

// Suppress unused import warning — json is used for future expansion.
var _ = json.Marshal
