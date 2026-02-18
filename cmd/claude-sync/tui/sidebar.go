package tui

import (
	"fmt"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
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
	sections []SidebarEntry
	active   int // index into sections
	height   int // available vertical space
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

// UpdateCounts sets the selected/total counts for a section.
func (s *Sidebar) UpdateCounts(section Section, selected, total int) {
	for i := range s.sections {
		if s.sections[i].Section == section {
			s.sections[i].Selected = selected
			s.sections[i].Total = total
			s.sections[i].Available = total > 0
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
func (s Sidebar) View() string {
	var b strings.Builder

	for i, e := range s.sections {
		if !e.Available {
			label := fmt.Sprintf("  %-12s", e.Section.String())
			b.WriteString(UnavailableSidebarStyle.Render(label))
			b.WriteString("\n")
			continue
		}

		// Build the count display.
		var count string
		if e.Total > 0 {
			count = fmt.Sprintf("(%d/%d)", e.Selected, e.Total)
		}

		// Cursor indicator.
		cursor := " "
		if i == s.active {
			cursor = ">"
		}

		label := fmt.Sprintf("%s %-11s %s", cursor, e.Section.String(), count)

		if i == s.active {
			b.WriteString(ActiveSidebarStyle.Render(label))
		} else {
			b.WriteString(InactiveSidebarStyle.Render(label))
		}
		b.WriteString("\n")
	}

	// Pad remaining height with empty lines.
	rendered := len(s.sections)
	for rendered < s.height {
		b.WriteString("\n")
		rendered++
	}

	return SidebarContainerStyle.Height(s.height).Render(b.String())
}
