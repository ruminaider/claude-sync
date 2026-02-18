package tui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/x/ansi"
)

// StatusBar renders the bottom row with selection counts and keyboard shortcuts.
type StatusBar struct {
	selected    int
	total       int
	section     Section
	profileName string
	width       int
}

// NewStatusBar creates a status bar with default values.
func NewStatusBar() StatusBar {
	return StatusBar{
		profileName: "Base",
		section:     SectionPlugins,
	}
}

// SetWidth sets the available width for rendering.
func (s *StatusBar) SetWidth(w int) {
	s.width = w
}

// Update refreshes the status bar with a new selection summary and profile name.
func (s *StatusBar) Update(summary SelectionSummary, profile string) {
	s.selected = summary.Selected
	s.total = summary.Total
	s.section = summary.Section
	s.profileName = profile
}

// View renders the status bar.
func (s StatusBar) View() string {
	// Left side: selection info.
	left := fmt.Sprintf("%d/%d %s selected", s.selected, s.total, s.section.String())

	// Separator and profile context.
	context := fmt.Sprintf("%s config", s.profileName)

	leftPart := fmt.Sprintf("%s \u00b7 %s", left, context)

	// Right side: keyboard shortcuts.
	shortcuts := []string{
		StatusBarKeyStyle.Render("Ctrl+S") + ": save",
		StatusBarKeyStyle.Render("Tab") + ": profiles",
		StatusBarKeyStyle.Render("/") + ": search",
	}
	rightPart := strings.Join(shortcuts, " \u00b7 ")

	// Calculate padding between left and right.
	leftWidth := ansi.StringWidth(leftPart)
	rightWidth := ansi.StringWidth(rightPart)
	availableWidth := s.width - 2 // account for StatusBarStyle padding
	gap := availableWidth - leftWidth - rightWidth
	if gap < 1 {
		gap = 1
	}

	content := leftPart + strings.Repeat(" ", gap) + rightPart

	return StatusBarStyle.Width(s.width).Render(content)
}
