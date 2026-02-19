package tui

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// TabBar renders profile tabs along the top of the TUI.
// The first tab is always "Base" and cannot be deleted.
// The last position is always the [+] button.
// Each tab is assigned a color from ProfileThemeRotation.
type TabBar struct {
	tabs   []string        // tab names; tabs[0] is always "Base"
	themes []ProfileTheme  // parallel to tabs; color for each tab
	active int             // index of the selected tab
	width  int             // available horizontal space
	onPlus bool            // true when the [+] button is focused
}

// themeForIndex returns the ProfileTheme for the given tab index,
// cycling through ProfileThemeRotation.
func themeForIndex(i int) ProfileTheme {
	return ProfileThemeRotation[i%len(ProfileThemeRotation)]
}

// NewTabBar creates a tab bar with the Base tab and optional profile tabs.
func NewTabBar(profiles []string) TabBar {
	n := 1 + len(profiles)
	tabs := make([]string, 0, n)
	themes := make([]ProfileTheme, 0, n)
	tabs = append(tabs, "Base")
	themes = append(themes, themeForIndex(0))
	for i, p := range profiles {
		tabs = append(tabs, p)
		themes = append(themes, themeForIndex(i+1))
	}
	return TabBar{tabs: tabs, themes: themes, active: 0}
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

// ActiveTheme returns the ProfileTheme of the currently active tab.
func (t TabBar) ActiveTheme() ProfileTheme {
	if t.active >= 0 && t.active < len(t.themes) {
		return t.themes[t.active]
	}
	return ProfileThemeRotation[0]
}

// AddTab appends a new profile tab and switches to it.
func (t *TabBar) AddTab(name string) {
	t.tabs = append(t.tabs, name)
	t.themes = append(t.themes, themeForIndex(len(t.tabs)-1))
	t.active = len(t.tabs) - 1
	t.onPlus = false
}

// SetActive sets the active tab by index and clears the [+] focus.
func (t *TabBar) SetActive(i int) {
	if i >= 0 && i < len(t.tabs) {
		t.active = i
		t.onPlus = false
	}
}

// OnPlus returns true when the [+] button is focused.
func (t TabBar) OnPlus() bool {
	return t.onPlus
}

// CycleNext advances to the next tab, including the [+] button, wrapping around.
// Order: Base → work → … → [+] → Base → …
func (t *TabBar) CycleNext() {
	if t.onPlus {
		t.onPlus = false
		t.active = 0
		return
	}
	if t.active == len(t.tabs)-1 {
		t.onPlus = true
		return
	}
	t.active++
}

// CyclePrev moves to the previous tab, including the [+] button, wrapping around.
// Order: Base → [+] → … → work → Base → …
func (t *TabBar) CyclePrev() {
	if t.onPlus {
		t.onPlus = false
		return
	}
	if t.active == 0 {
		t.onPlus = true
		t.active = len(t.tabs) - 1
		return
	}
	t.active--
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
			t.themes = append(t.themes[:i], t.themes[i+1:]...)
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
			if t.onPlus {
				return t, func() tea.Msg {
					return NewProfileRequestMsg{}
				}
			}
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
// Each tab is colored with its assigned accent color.
func (t TabBar) View() string {
	var parts []string
	for i, name := range t.tabs {
		accent := t.themes[i].Accent
		if !t.onPlus && i == t.active {
			// Active tab: accent background, dark foreground, bold.
			style := lipgloss.NewStyle().
				Foreground(colorBase).
				Background(accent).
				Padding(0, 1).
				Bold(true)
			parts = append(parts, style.Render(name))
		} else {
			// Inactive tab: accent foreground, Surface0 background.
			style := lipgloss.NewStyle().
				Foreground(accent).
				Background(colorSurface0).
				Padding(0, 1)
			parts = append(parts, style.Render(name))
		}
	}
	// The [+] button is always last, highlighted when focused.
	if t.onPlus {
		// Use the next rotation color as a hint.
		nextAccent := themeForIndex(len(t.tabs)).Accent
		style := lipgloss.NewStyle().
			Foreground(colorBase).
			Background(nextAccent).
			Padding(0, 1).
			Bold(true)
		parts = append(parts, style.Render("+"))
	} else {
		parts = append(parts, PlusTabStyle.Render("+"))
	}

	row := strings.Join(parts, " ")

	// Fill the rest of the width with the tab bar background.
	rendered := TabBarStyle.Width(t.width).Render(row)
	return rendered
}
