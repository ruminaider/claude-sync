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
	nameStyle := lipgloss.NewStyle().Foreground(colorMauve)
	blueStyle := lipgloss.NewStyle().Foreground(colorBlue)
	pinkStyle := lipgloss.NewStyle().Foreground(colorPink)
	peachStyle := lipgloss.NewStyle().Foreground(colorPeach)

	var lines []string

	lines = append(lines, headerStyle.Render(fmt.Sprintf("Active Plugins (%d)", len(state.Plugins))))
	lines = append(lines, "")

	if len(state.Plugins) == 0 && len(state.UntrackedPlugins) == 0 {
		lines = append(lines, stDim.Render("No plugins configured"))
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
		lines = append(lines, stSection.Render(prefix))

		for _, p := range state.Plugins {
			name := fmt.Sprintf("%-*s", maxNameLen, p.Name)
			var statusTag, extra string

			switch p.Status {
			case "upstream":
				statusTag = stDim.Render("upstream")
				if p.Marketplace != "" {
					extra = stDim.Render(p.Marketplace)
				}
			case "pinned":
				statusTag = blueStyle.Render("pinned")
				extra = stDim.Render("v" + p.PinVersion)
				if p.LatestVersion != "" {
					extra += peachStyle.Render(" (latest: v" + p.LatestVersion + ")")
				}
			case "forked":
				statusTag = pinkStyle.Render("forked")
				extra = stDim.Render("local edits")
			default:
				statusTag = stDim.Render(p.Status)
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
		lines = append(lines, stSection.Render(prefix))

		for _, key := range state.UntrackedPlugins {
			name := key
			if idx := strings.Index(key, "@"); idx >= 0 {
				name = key[:idx]
			}
			paddedName := fmt.Sprintf("%-*s", maxNameLen, name)
			lines = append(lines, "  "+stYellow.Render(paddedName)+"  "+
				stDim.Render("installed locally"))
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
		m.scroll, m.maxScroll = recalcScroll(m.content, m.height, m.scroll)
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
	return renderScrollable(m.content, m.width, m.height, m.scroll)
}
