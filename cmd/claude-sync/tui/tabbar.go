package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TabBar renders profile tabs along the top of the TUI.
// The first tab is always "Base" and cannot be deleted.
// The last position is always the [+] button.
type TabBar struct {
	tabs   []string // tab names; tabs[0] is always "Base"
	active int      // index of the selected tab
	width  int      // available horizontal space
}

// NewTabBar creates a tab bar with the Base tab and optional profile tabs.
func NewTabBar(profiles []string) TabBar {
	tabs := make([]string, 0, 1+len(profiles))
	tabs = append(tabs, "Base")
	tabs = append(tabs, profiles...)
	return TabBar{tabs: tabs, active: 0}
}

// SetWidth sets the available width for rendering.
func (t *TabBar) SetWidth(w int) {
	t.width = w
}

// ActiveTab returns the name of the currently active tab.
func (t TabBar) ActiveTab() string {
	if t.active >= 0 && t.active < len(t.tabs) {
		return t.tabs[t.active]
	}
	return ""
}

// AddTab appends a new profile tab and switches to it.
func (t *TabBar) AddTab(name string) {
	t.tabs = append(t.tabs, name)
	t.active = len(t.tabs) - 1
}

// SetActive sets the active tab by index.
func (t *TabBar) SetActive(i int) {
	if i >= 0 && i < len(t.tabs) {
		t.active = i
	}
}

// RemoveTab removes a profile tab by name. The Base tab cannot be removed.
// If the removed tab was active, focus moves to the previous tab.
func (t *TabBar) RemoveTab(name string) {
	if name == "Base" {
		return
	}
	for i, tab := range t.tabs {
		if tab == name {
			t.tabs = append(t.tabs[:i], t.tabs[i+1:]...)
			if t.active >= len(t.tabs) {
				t.active = len(t.tabs) - 1
			}
			return
		}
	}
}

// Update handles key messages when the tab bar has focus.
func (t TabBar) Update(msg tea.Msg) (TabBar, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "left", "h":
			if t.active > 0 {
				t.active--
				return t, func() tea.Msg {
					return TabSwitchMsg{Name: t.tabs[t.active]}
				}
			}
		case "right", "l":
			if t.active < len(t.tabs)-1 {
				t.active++
				return t, func() tea.Msg {
					return TabSwitchMsg{Name: t.tabs[t.active]}
				}
			}
			// If at last tab, right arrow enters [+]
			if t.active == len(t.tabs)-1 {
				return t, func() tea.Msg {
					return NewProfileRequestMsg{}
				}
			}
		case "enter":
			// Enter on current position: if cursor would be on [+], create new profile.
			// Otherwise it is a no-op (tab already active).
		case "+":
			return t, func() tea.Msg {
				return NewProfileRequestMsg{}
			}
		case "ctrl+d":
			// Delete current profile (not Base).
			if t.active > 0 {
				name := t.tabs[t.active]
				return t, func() tea.Msg {
					return DeleteProfileMsg{Name: name}
				}
			}
		}
	}
	return t, nil
}

// View renders the tab bar as a single horizontal line.
func (t TabBar) View() string {
	var parts []string
	for i, name := range t.tabs {
		var style lipgloss.Style
		if i == t.active {
			style = ActiveTabStyle
		} else {
			style = InactiveTabStyle
		}
		parts = append(parts, style.Render(name))
	}
	// The [+] button is always last.
	parts = append(parts, PlusTabStyle.Render("+"))

	row := strings.Join(parts, " ")

	// Fill the rest of the width with the tab bar background.
	rendered := TabBarStyle.Width(t.width).Render(row)
	return rendered
}
