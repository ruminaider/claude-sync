package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// MenuModel is a standalone Bubble Tea model for a flat grouped menu.
// Items with isHeader=true are rendered as section dividers and skipped
// during cursor navigation.
type MenuModel struct {
	items  []menuItem
	cursor int
	width  int
	height int
	state  commands.MenuState

	// Selected is set when the user presses Enter on a leaf item.
	// The caller inspects this after tea.Quit to decide what to do.
	Selected *menuItem

	// quitting is true after the user presses q/esc/ctrl+c.
	quitting bool
}

// NewMenuModel creates a MenuModel from the detected state.
// For a configured installation it builds the flat grouped menu;
// for a fresh install it builds the two-item welcome menu.
func NewMenuModel(state commands.MenuState) MenuModel {
	var items []menuItem
	if state.ConfigExists {
		items = buildConfiguredMenu(state)
	} else {
		items = buildFreshInstallMenu()
	}

	m := MenuModel{items: items, state: state}
	// Place cursor on first non-header item.
	for i, it := range items {
		if !it.isHeader {
			m.cursor = i
			break
		}
	}
	return m
}

func (m MenuModel) Init() tea.Cmd {
	return nil
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "q", "esc":
			m.quitting = true
			return m, tea.Quit
		case "up", "k":
			m.moveCursor(-1)
		case "down", "j":
			m.moveCursor(1)
		case "r":
			if m.state.HasPending {
				m.Selected = &menuItem{label: "Review pending", actionID: ActionApprove, mode: ModeCLI}
				return m, tea.Quit
			} else if m.state.HasConflicts {
				m.Selected = &menuItem{label: "Resolve conflicts", actionID: ActionConflicts, mode: ModeCLI}
				return m, tea.Quit
			}
		case "enter":
			if m.cursor >= 0 && m.cursor < len(m.items) && !m.items[m.cursor].isHeader {
				sel := m.items[m.cursor]
				m.Selected = &sel
				return m, tea.Quit
			}
		}
	}
	return m, nil
}

// moveCursor moves the cursor by delta (+1 or -1), skipping header items.
func (m *MenuModel) moveCursor(delta int) {
	next := m.cursor
	for {
		next += delta
		if next < 0 || next >= len(m.items) {
			return // hit boundary, don't move
		}
		if !m.items[next].isHeader {
			m.cursor = next
			return
		}
	}
}

func (m MenuModel) View() string {
	if m.quitting {
		return ""
	}

	maxWidth, innerWidth := clampWidth(m.width)

	headerLineStyle := lipgloss.NewStyle().Foreground(colorSurface1)
	headerLabelStyle := lipgloss.NewStyle().Foreground(colorBlue).Bold(true)
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	normalStyle := lipgloss.NewStyle().Foreground(colorText)
	hintStyle := lipgloss.NewStyle().Foreground(colorSubtext0)

	var lines []string

	// Show banner for configured installations.
	if m.state.ConfigExists {
		banner := buildBanner(m.state)
		if banner != "" {
			lines = append(lines, banner)
			lines = append(lines, "")
		}
	}

	for i, item := range m.items {
		if item.isHeader {
			// Render as "── Label ──"
			label := headerLabelStyle.Render(item.label)
			labelWidth := lipgloss.Width(label)
			// left bar: 2 chars, right bar: fill remaining
			rightLen := innerWidth - 4 - labelWidth
			if rightLen < 2 {
				rightLen = 2
			}
			line := headerLineStyle.Render("──") + " " + label + " " + headerLineStyle.Render(strings.Repeat("─", rightLen))

			// Add blank line before headers (except the first one)
			if i > 0 {
				lines = append(lines, "")
			}
			lines = append(lines, line)
			continue
		}

		// Build the line
		prefix := "  "
		var labelStr string
		if i == m.cursor {
			prefix = "> "
			labelStr = selectedStyle.Render(prefix + item.label)
		} else {
			labelStr = normalStyle.Render(prefix + item.label)
		}

		if item.hint != "" {
			hintStr := hintStyle.Render(item.hint)
			gap := innerWidth - lipgloss.Width(labelStr) - lipgloss.Width(hintStr)
			if gap < 1 {
				gap = 1
			}
			lines = append(lines, labelStr+strings.Repeat(" ", gap)+hintStr)
		} else {
			lines = append(lines, labelStr)
		}
	}

	// Footer
	lines = append(lines, "")
	lines = append(lines, stDim.Render("j/k navigate  enter select  q quit"))

	content := strings.Join(lines, "\n")
	boxStyle := contentBox(maxWidth, colorSurface1)
	return boxStyle.Render(content)
}
