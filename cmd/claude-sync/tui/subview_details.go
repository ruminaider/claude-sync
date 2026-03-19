package tui

import (
	"fmt"
	"os"
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
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	textStyle := lipgloss.NewStyle().Foreground(colorText)
	greenStyle := lipgloss.NewStyle().Foreground(colorGreen)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	sectionStyle := lipgloss.NewStyle().Foreground(colorSurface1)

	var lines []string

	lines = append(lines, headerStyle.Render("Full config details"))
	lines = append(lines, "")

	// --- Config section ---
	lines = append(lines, sectionLine(sectionStyle, "Config", 60))
	if state.ConfigRepo != "" {
		lines = append(lines, textStyle.Render("Repository: "+state.ConfigRepo))
	} else {
		lines = append(lines, dimStyle.Render("Repository: (none)"))
	}
	if state.ActiveProfile != "" {
		lines = append(lines, textStyle.Render("Active profile: "+state.ActiveProfile))
	} else {
		lines = append(lines, dimStyle.Render("Active profile: (base)"))
	}
	lines = append(lines, "")

	// --- Plugins section ---
	pluginCount := len(state.Plugins)
	lines = append(lines, sectionLine(sectionStyle, fmt.Sprintf("Plugins (%d)", pluginCount), 60))
	if pluginCount == 0 {
		lines = append(lines, dimStyle.Render("No plugins installed"))
	} else {
		for _, p := range state.Plugins {
			meta := p.Status
			if p.PinVersion != "" {
				meta += "  v" + p.PinVersion
			}
			if p.Marketplace != "" {
				meta += "  " + p.Marketplace
			}
			lines = append(lines, textStyle.Render(fmt.Sprintf("  %-16s %s", p.Name, meta)))
		}
	}
	lines = append(lines, "")

	// --- Profiles section ---
	profileCount := len(state.Profiles)
	lines = append(lines, sectionLine(sectionStyle, fmt.Sprintf("Profiles (%d)", profileCount), 60))
	if profileCount == 0 {
		lines = append(lines, dimStyle.Render("No profiles configured"))
	} else {
		for _, name := range state.Profiles {
			if name == state.ActiveProfile {
				lines = append(lines, greenStyle.Render(fmt.Sprintf("  \u25cf %s (active)", name)))
			} else {
				lines = append(lines, textStyle.Render("    "+name))
			}
		}
	}
	lines = append(lines, "")

	// --- Projects section ---
	lines = append(lines, sectionLine(sectionStyle, "Projects", 60))
	if len(state.Projects) == 0 {
		lines = append(lines, dimStyle.Render("No managed projects"))
	} else {
		for _, proj := range state.Projects {
			path := shortenHomePath(proj.Path)
			profileTag := ""
			if proj.Profile != "" {
				profileTag = " [" + proj.Profile + "]"
			}
			lines = append(lines, textStyle.Render("  "+path+profileTag))
		}
	}
	lines = append(lines, "")

	// --- Sync Status section ---
	lines = append(lines, sectionLine(sectionStyle, "Sync Status", 60))
	lines = append(lines, textStyle.Render(fmt.Sprintf("Behind: %d commits", state.CommitsBehind)))
	if state.HasPending {
		lines = append(lines, yellowStyle.Render("Pending approvals: yes"))
	} else {
		lines = append(lines, dimStyle.Render("Pending approvals: none"))
	}
	if state.HasConflicts {
		lines = append(lines, yellowStyle.Render("Conflicts: yes"))
	} else {
		lines = append(lines, dimStyle.Render("Conflicts: none"))
	}
	lines = append(lines, "")

	// --- This Project section ---
	lines = append(lines, sectionLine(sectionStyle, "This Project", 60))
	if state.ProjectDir != "" {
		lines = append(lines, textStyle.Render("Path: "+shortenHomePath(state.ProjectDir)))
		if state.ProjectProfile != "" {
			lines = append(lines, textStyle.Render("Profile: "+state.ProjectProfile))
		} else {
			lines = append(lines, dimStyle.Render("Profile: (base)"))
		}
	} else {
		lines = append(lines, dimStyle.Render("Not in a managed project"))
	}
	if state.ClaudeMDCount > 0 {
		lines = append(lines, textStyle.Render(fmt.Sprintf("CLAUDE.md sections: %d", state.ClaudeMDCount)))
	}
	if state.MCPCount > 0 {
		lines = append(lines, textStyle.Render(fmt.Sprintf("MCP servers: %d", state.MCPCount)))
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

// shortenHomePath replaces the home directory prefix with ~.
func shortenHomePath(path string) string {
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return path
	}
	if strings.HasPrefix(path, home) {
		return "~" + path[len(home):]
	}
	return path
}

func (m ConfigDetails) Init() tea.Cmd {
	return nil
}

func (m ConfigDetails) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
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

	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)

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
	output = append(output, dimStyle.Render("j/k scroll  esc back"))

	content := strings.Join(output, "\n")

	boxStyle := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(colorSurface1).
		Padding(1, 2).
		Width(maxWidth)

	return boxStyle.Render(content)
}

