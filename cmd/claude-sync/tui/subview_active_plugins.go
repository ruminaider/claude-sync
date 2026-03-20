package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// ActivePluginsView is a read-only scrollable sub-view showing all active plugins
// with source attribution and untracked detection.
type ActivePluginsView struct {
	content   string // pre-rendered content
	scroll    int
	maxScroll int
	width     int
	height    int
}

// NewActivePluginsView creates an ActivePluginsView from the current menu state.
func NewActivePluginsView(state commands.MenuState, width, height int) ActivePluginsView {
	content := buildActivePluginsContent(state)
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

	return ActivePluginsView{
		content:   content,
		scroll:    0,
		maxScroll: maxScroll,
		width:     width,
		height:    height,
	}
}

// buildActivePluginsContent builds the pre-rendered text for the active plugins view.
func buildActivePluginsContent(state commands.MenuState) string {
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(colorText)
	dimStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	nameStyle := lipgloss.NewStyle().Foreground(colorMauve)
	blueStyle := lipgloss.NewStyle().Foreground(colorBlue)
	pinkStyle := lipgloss.NewStyle().Foreground(colorPink)
	yellowStyle := lipgloss.NewStyle().Foreground(colorYellow)
	peachStyle := lipgloss.NewStyle().Foreground(colorPeach)
	sectionStyle := lipgloss.NewStyle().Foreground(colorSurface1)

	var lines []string

	lines = append(lines, headerStyle.Render(fmt.Sprintf("Active Plugins (%d)", len(state.Plugins))))
	lines = append(lines, "")

	if len(state.Plugins) == 0 && len(state.UntrackedPlugins) == 0 {
		lines = append(lines, dimStyle.Render("No plugins configured"))
		return strings.Join(lines, "\n")
	}

	// Calculate max name width for alignment (across both synced and untracked)
	maxNameLen := 0
	for _, p := range state.Plugins {
		if len(p.Name) > maxNameLen {
			maxNameLen = len(p.Name)
		}
	}
	for _, key := range state.UntrackedPlugins {
		name := key
		if idx := strings.Index(key, "@"); idx >= 0 {
			name = key[:idx]
		}
		if len(name) > maxNameLen {
			maxNameLen = len(name)
		}
	}

	// Synced plugins
	if len(state.Plugins) > 0 {
		prefix := "── Synced "
		remaining := 56 - lipgloss.Width(prefix)
		if remaining > 0 {
			prefix += strings.Repeat("─", remaining)
		}
		lines = append(lines, sectionStyle.Render(prefix))

		for _, p := range state.Plugins {
			name := fmt.Sprintf("%-*s", maxNameLen, p.Name)
			var statusTag, extra string

			switch p.Status {
			case "upstream":
				statusTag = dimStyle.Render("upstream")
				if p.Marketplace != "" {
					extra = dimStyle.Render(p.Marketplace)
				}
			case "pinned":
				statusTag = blueStyle.Render("pinned")
				extra = dimStyle.Render("v" + p.PinVersion)
				if p.LatestVersion != "" {
					extra += peachStyle.Render(" (latest: v" + p.LatestVersion + ")")
				}
			case "forked":
				statusTag = pinkStyle.Render("forked")
				extra = dimStyle.Render("local edits")
			default:
				statusTag = dimStyle.Render(p.Status)
			}

			line := "  " + nameStyle.Render(name) + "  " + statusTag
			if extra != "" {
				line += "  " + extra
			}
			lines = append(lines, line)
		}
	}

	// Untracked plugins subsection
	if len(state.UntrackedPlugins) > 0 {
		lines = append(lines, "")
		prefix := "── Not in synced config "
		remaining := 56 - lipgloss.Width(prefix)
		if remaining > 0 {
			prefix += strings.Repeat("─", remaining)
		}
		lines = append(lines, sectionStyle.Render(prefix))

		for _, key := range state.UntrackedPlugins {
			name := key
			if idx := strings.Index(key, "@"); idx >= 0 {
				name = key[:idx]
			}
			paddedName := fmt.Sprintf("%-*s", maxNameLen, name)
			lines = append(lines, "  "+yellowStyle.Render(paddedName)+"  "+
				dimStyle.Render("installed locally"))
		}
	}

	return strings.Join(lines, "\n")
}

func (m ActivePluginsView) Init() tea.Cmd {
	return nil
}

func (m ActivePluginsView) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
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

func (m ActivePluginsView) View() string {
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
