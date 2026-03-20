package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// ConfigDetails is a read-only scrollable sub-view that displays the full
// config state: repo, plugins, profiles, projects, sync status, and current project.
type ConfigDetails struct {
	content   string // pre-rendered content
	scroll    int
	maxScroll int
	width     int
	height    int
	cancelled bool
}

// NewConfigDetails creates a ConfigDetails sub-view from the current menu state.
func NewConfigDetails(state commands.MenuState, width, height int) ConfigDetails {
	content := buildConfigDetailsContent(state)
	lines := strings.Split(content, "\n")

	// Available height inside box: height minus border (2) + padding (2) + footer (1)
	innerHeight := height - 5
	if innerHeight < 1 {
		innerHeight = 1
	}

	maxScroll := len(lines) - innerHeight
	if maxScroll < 0 {
		maxScroll = 0
	}

	return ConfigDetails{
		content:   content,
		scroll:    0,
		maxScroll: maxScroll,
		width:     width,
		height:    height,
	}
}

// buildConfigDetailsContent builds the pre-rendered text content from MenuState.
func buildConfigDetailsContent(state commands.MenuState) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)

	var lines []string

	lines = append(lines, headerStyle.Render("Full config details"))
	lines = append(lines, "")

	// --- Config section ---
	lines = append(lines, sectionLine(stSection, "Config", 60))
	if state.ConfigRepo != "" {
		lines = append(lines, stText.Render("Repository: "+state.ConfigRepo))
	} else {
		lines = append(lines, stDim.Render("Repository: (none)"))
	}
	if state.ActiveProfile != "" {
		lines = append(lines, stText.Render("Active profile: "+state.ActiveProfile))
	} else {
		lines = append(lines, stDim.Render("Active profile: (base)"))
	}
	lines = append(lines, "")

	// --- Plugins section ---
	pluginCount := len(state.Plugins)
	lines = append(lines, sectionLine(stSection, fmt.Sprintf("Plugins (%d)", pluginCount), 60))
	if pluginCount == 0 {
		lines = append(lines, stDim.Render("No plugins installed"))
	} else {
		for _, p := range state.Plugins {
			meta := p.Status
			if p.PinVersion != "" {
				meta += "  v" + p.PinVersion
			}
			if p.Marketplace != "" {
				meta += "  " + p.Marketplace
			}
			lines = append(lines, stText.Render(fmt.Sprintf("  %-16s %s", p.Name, meta)))
		}
	}
	lines = append(lines, "")

	// --- Profiles section ---
	profileCount := len(state.Profiles)
	lines = append(lines, sectionLine(stSection, fmt.Sprintf("Profiles (%d)", profileCount), 60))
	if profileCount == 0 {
		lines = append(lines, stDim.Render("No profiles configured"))
	} else {
		for _, name := range state.Profiles {
			if name == state.ActiveProfile {
				lines = append(lines, stGreen.Render(fmt.Sprintf("  \u25cf %s (active)", name)))
			} else {
				lines = append(lines, stText.Render("    "+name))
			}
		}
	}
	lines = append(lines, "")

	// --- Projects section ---
	lines = append(lines, sectionLine(stSection, "Projects", 60))
	if len(state.Projects) == 0 {
		lines = append(lines, stDim.Render("No managed projects"))
	} else {
		for _, proj := range state.Projects {
			path := shortenPath(proj.Path)
			profileTag := ""
			if proj.Profile != "" {
				profileTag = " [" + proj.Profile + "]"
			}
			lines = append(lines, stText.Render("  "+path+profileTag))
		}
	}
	lines = append(lines, "")

	// --- Sync Status section ---
	lines = append(lines, sectionLine(stSection, "Sync Status", 60))
	lines = append(lines, stText.Render(fmt.Sprintf("Behind: %d commits", state.CommitsBehind)))
	if state.HasPending {
		lines = append(lines, stYellow.Render("Pending approvals: yes"))
	} else {
		lines = append(lines, stDim.Render("Pending approvals: none"))
	}
	if state.HasConflicts {
		lines = append(lines, stYellow.Render("Conflicts: yes"))
	} else {
		lines = append(lines, stDim.Render("Conflicts: none"))
	}
	lines = append(lines, "")

	// --- This Project section ---
	lines = append(lines, sectionLine(stSection, "This Project", 60))
	if state.ProjectDir != "" {
		lines = append(lines, stText.Render("Path: "+shortenPath(state.ProjectDir)))
		if state.ProjectProfile != "" {
			lines = append(lines, stText.Render("Profile: "+state.ProjectProfile))
		} else {
			lines = append(lines, stDim.Render("Profile: (base)"))
		}
	} else {
		lines = append(lines, stDim.Render("Not in a managed project"))
	}
	if state.ClaudeMDCount > 0 {
		lines = append(lines, stText.Render(fmt.Sprintf("CLAUDE.md sections: %d", state.ClaudeMDCount)))
	}
	if state.MCPCount > 0 {
		lines = append(lines, stText.Render(fmt.Sprintf("MCP servers: %d", state.MCPCount)))
	}

	return strings.Join(lines, "\n")
}

// sectionLine renders a section header like "── Plugins (4) ───────────────"
func sectionLine(style lipgloss.Style, label string, width int) string {
	prefix := "\u2500\u2500 " + label + " "
	remaining := width - lipgloss.Width(prefix)
	if remaining > 0 {
		prefix += strings.Repeat("\u2500", remaining)
	}
	return style.Render(prefix)
}

func (m ConfigDetails) Init() tea.Cmd {
	return nil
}

func (m ConfigDetails) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		// Recalculate maxScroll based on new height
		allLines := strings.Split(m.content, "\n")
		innerHeight := m.height - 6
		if innerHeight < 1 {
			innerHeight = 1
		}
		m.maxScroll = len(allLines) - innerHeight
		if m.maxScroll < 0 {
			m.maxScroll = 0
		}
		if m.scroll > m.maxScroll {
			m.scroll = m.maxScroll
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "esc":
			m.cancelled = true
			return m, func() tea.Msg {
				return subViewCloseMsg{refreshState: false}
			}
		case "j", "down":
			if m.scroll < m.maxScroll {
				m.scroll++
			}
		case "k", "up":
			if m.scroll > 0 {
				m.scroll--
			}
		}
	}
	return m, nil
}

func (m ConfigDetails) View() string {
	maxWidth := m.width - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}

	// Split content into lines and apply scroll
	allLines := strings.Split(m.content, "\n")

	// Available height inside box: height minus border (2) + padding (2) + footer line (2)
	innerHeight := m.height - 6
	if innerHeight < 1 {
		innerHeight = 1
	}

	start := m.scroll
	end := start + innerHeight
	if end > len(allLines) {
		end = len(allLines)
	}
	if start > end {
		start = end
	}

	visibleLines := allLines[start:end]

	var output []string
	output = append(output, strings.Join(visibleLines, "\n"))
	output = append(output, "")
	output = append(output, stDim.Render("j/k scroll  esc back"))

	content := strings.Join(output, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

