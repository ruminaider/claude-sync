package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/commands"
)

// menuLevel tracks a position in the navigation stack.
type menuLevel struct {
	title  string
	items  []menuItem
	cursor int
}

// MenuModel is the Bubble Tea model for the main menu.
type MenuModel struct {
	items    []menuItem  // top-level items
	cursor   int
	stack    []menuLevel // navigation stack (pushed on drill-in)
	state    commands.MenuState
	width    int
	height   int
	Version  string
	Quitting bool
	Selected MenuAction // set when a leaf action is chosen
}

// NewMenuModel creates a menu model from detected state.
func NewMenuModel(state commands.MenuState) MenuModel {
	return MenuModel{
		items: BuildMenuItems(state),
		state: state,
	}
}

func (m MenuModel) Init() tea.Cmd {
	return nil
}

// currentItems returns the items visible at the current navigation depth.
func (m MenuModel) currentItems() []menuItem {
	if len(m.stack) == 0 {
		return m.items
	}
	return m.stack[len(m.stack)-1].items
}

// currentTitle returns the title of the current submenu, or "" for top level.
func (m MenuModel) currentTitle() string {
	if len(m.stack) == 0 {
		return ""
	}
	return m.stack[len(m.stack)-1].title
}

func (m MenuModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		return m, nil

	case tea.KeyMsg:
		items := m.currentItems()

		switch msg.String() {
		case "ctrl+c", "q":
			m.Quitting = true
			return m, tea.Quit

		case "up", "k":
			if m.cursor > 0 {
				m.cursor--
			}

		case "down", "j":
			if m.cursor < len(items)-1 {
				m.cursor++
			}

		case "enter":
			if m.cursor >= 0 && m.cursor < len(items) {
				selected := items[m.cursor]
				if selected.isCategory() {
					m.stack = append(m.stack, menuLevel{
						title:  selected.label,
						items:  selected.children,
						cursor: m.cursor,
					})
					m.cursor = 0
				} else {
					m.Selected = selected.action
					return m, tea.Quit
				}
			}

		case "esc":
			if len(m.stack) > 0 {
				prev := m.stack[len(m.stack)-1]
				m.stack = m.stack[:len(m.stack)-1]
				m.cursor = prev.cursor
			} else {
				m.Quitting = true
				return m, tea.Quit
			}
		}
	}

	return m, nil
}

func (m MenuModel) View() string {
	if m.Quitting {
		return ""
	}

	var b strings.Builder

	// Title
	titleStyle := lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	if !m.state.ConfigExists {
		b.WriteString(titleStyle.Render("claude-sync"))
		b.WriteString("\n")
		b.WriteString(lipgloss.NewStyle().Foreground(colorSubtext0).Render("Sync your Claude Code setup across machines."))
		b.WriteString("\n\n")
	} else if title := m.currentTitle(); title != "" {
		b.WriteString(titleStyle.Render("claude-sync"))
		breadcrumb := lipgloss.NewStyle().Foreground(colorSubtext0).Render(" > " + title)
		b.WriteString(breadcrumb)
		b.WriteString("\n\n")
	} else {
		b.WriteString(titleStyle.Render("claude-sync"))
		if m.Version != "" {
			versionStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
			b.WriteString(" " + versionStyle.Render("v"+m.Version))
		}
		b.WriteString("\n")
		if m.state.ConfigExists {
			summary := buildStatusSummary(m.state)
			if summary != "" {
				b.WriteString(lipgloss.NewStyle().Foreground(colorSubtext0).Render(summary))
				b.WriteString("\n")
			}
		}
		b.WriteString("\n")
	}

	// Menu items
	items := m.currentItems()
	for i, item := range items {
		cursor := "  "
		style := lipgloss.NewStyle()
		if i == m.cursor {
			cursor = "> "
			style = style.Bold(true).Foreground(colorBlue)
		}

		line := cursor + style.Render(item.label)
		if item.desc != "" {
			descStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
			line += " " + descStyle.Render(item.desc)
		}
		if item.isCategory() {
			arrowStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
			line += " " + arrowStyle.Render(">")
		}
		b.WriteString(line + "\n")
	}

	// Footer
	b.WriteString("\n")
	hintStyle := lipgloss.NewStyle().Foreground(colorSubtext0)
	if len(m.stack) > 0 {
		b.WriteString(hintStyle.Render("esc back  q quit"))
	} else {
		b.WriteString(hintStyle.Render("q quit"))
	}
	b.WriteString("\n")

	// Wrap in a box if we have dimensions
	content := b.String()
	if m.width > 0 {
		boxStyle := lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorSurface1).
			Padding(1, 2).
			Width(min(m.width-2, 50))
		content = boxStyle.Render(content)
	}

	return content
}

// buildStatusSummary returns a one-line status string for the configured menu header.
func buildStatusSummary(state commands.MenuState) string {
	var parts []string
	if state.ActiveProfile != "" {
		parts = append(parts, "profile: "+state.ActiveProfile)
	}
	if len(state.Profiles) > 0 {
		parts = append(parts, fmt.Sprintf("%d profiles", len(state.Profiles)))
	}
	if state.HasPending {
		parts = append(parts, "pending changes")
	}
	if state.HasConflicts {
		parts = append(parts, "conflicts")
	}
	return strings.Join(parts, " | ")
}