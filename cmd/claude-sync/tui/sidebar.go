package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// SidebarEntry holds display data for one section in the sidebar.
type SidebarEntry struct {
	Section   Section
	Selected  int  // number of items currently selected
	Total     int  // total number of items available
	Available bool // false = section is grayed out (no data from scan)
}

// Sidebar renders the left-hand section navigation.
type Sidebar struct {
	sections        []SidebarEntry
	active          int  // index into sections
	height          int  // available vertical space
	focused         bool // true when sidebar has keyboard focus
	alwaysAvailable map[Section]bool
}

// NewSidebar creates a sidebar with entries for all sections.
func NewSidebar() Sidebar {
	entries := make([]SidebarEntry, len(AllSections))
	for i, s := range AllSections {
		entries[i] = SidebarEntry{
			Section:   s,
			Available: true,
		}
	}
	return Sidebar{sections: entries}
}

// SetHeight sets the available height for rendering.
func (s *Sidebar) SetHeight(h int) {
	s.height = h
}

// SetFocused sets whether the sidebar currently has keyboard focus.
func (s *Sidebar) SetFocused(f bool) {
	s.focused = f
}

// SetActive moves the sidebar cursor to the given section.
func (s *Sidebar) SetActive(section Section) {
	for i, e := range s.sections {
		if e.Section == section {
			s.active = i
			return
		}
	}
}

// ActiveSection returns the currently highlighted section.
func (s Sidebar) ActiveSection() Section {
	if s.active >= 0 && s.active < len(s.sections) {
		return s.sections[s.active].Section
	}
	return SectionPlugins
}

// SetAlwaysAvailable marks a section as navigable regardless of item count.
func (s *Sidebar) SetAlwaysAvailable(section Section) {
	if s.alwaysAvailable == nil {
		s.alwaysAvailable = make(map[Section]bool)
	}
	s.alwaysAvailable[section] = true
}

// UpdateCounts sets the selected/total counts for a section.
func (s *Sidebar) UpdateCounts(section Section, selected, total int) {
	for i := range s.sections {
		if s.sections[i].Section == section {
			s.sections[i].Selected = selected
			s.sections[i].Total = total
			s.sections[i].Available = total > 0 || s.alwaysAvailable[section]
			return
		}
	}
}

// nextAvailable finds the next available section index in the given direction.
// dir should be -1 (up) or +1 (down). Returns current index if no move is possible.
func (s Sidebar) nextAvailable(current, dir int) int {
	next := current + dir
	for next >= 0 && next < len(s.sections) {
		if s.sections[next].Available {
			return next
		}
		next += dir
	}
	return current
}

// Update handles key messages when the sidebar has focus.
func (s Sidebar) Update(msg tea.Msg) (Sidebar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "up", "k":
			prev := s.nextAvailable(s.active, -1)
			if prev != s.active {
				s.active = prev
				return s, func() tea.Msg {
					return SectionSwitchMsg{Section: s.sections[s.active].Section}
				}
			}
		case "down", "j":
			next := s.nextAvailable(s.active, +1)
			if next != s.active {
				s.active = next
				return s, func() tea.Msg {
					return SectionSwitchMsg{Section: s.sections[s.active].Section}
				}
			}
		case "enter", "right", "l":
			return s, func() tea.Msg {
				return FocusChangeMsg{Zone: FocusContent}
			}
		}
	}
	return s, nil
}

// View renders the sidebar as a vertical list of section names with counts.
// The active section gets a full-row background highlight instead of a cursor.
func (s Sidebar) View() string {
	// Row width fills the container (border is drawn outside Width).
	// Row styles have PaddingLeft(1), so text area = SidebarWidth - 1.
	rowWidth := SidebarWidth
	textWidth := rowWidth - 1 // minus PaddingLeft(1)

	lines := make([]string, 0, s.height)

	for i, e := range s.sections {
		name := e.Section.String()

		if !e.Available {
			label := fmt.Sprintf("%-*s", textWidth, name)
			lines = append(lines, UnavailableSidebarStyle.Render(label))
			continue
		}

		// Build label with right-aligned count.
		var label string
		if e.Total > 0 {
			count := fmt.Sprintf("%d/%d", e.Selected, e.Total)
			gap := textWidth - len(name) - len(count)
			if gap < 1 {
				gap = 1
			}
			label = name + strings.Repeat(" ", gap) + count
		} else {
			label = fmt.Sprintf("%-*s", textWidth, name)
		}

		// Set Width so the background highlight spans the full row.
		if i == s.active && s.focused {
			lines = append(lines, ActiveSidebarStyle.Width(rowWidth).Render(label))
		} else if i == s.active {
			// Active but unfocused: subtle highlight without bold/blue.
			dimActiveStyle := lipgloss.NewStyle().
				Foreground(colorSubtext0).
				Background(colorSurface0).
				PaddingLeft(1)
			lines = append(lines, dimActiveStyle.Width(rowWidth).Render(label))
		} else if s.focused {
			lines = append(lines, InactiveSidebarStyle.Width(rowWidth).Render(label))
		} else {
			dimStyle := lipgloss.NewStyle().
				Foreground(colorOverlay0).
				PaddingLeft(1)
			lines = append(lines, dimStyle.Width(rowWidth).Render(label))
		}
	}

	// Pad to exactly s.height lines.
	for len(lines) < s.height {
		lines = append(lines, "")
	}

	content := strings.Join(lines, "\n")

	borderColor := colorSurface1
	if s.focused {
		borderColor = colorBlue
	}
	return SidebarContainerStyle.
		Height(s.height).
		BorderForeground(borderColor).
		Render(content)
}
