package tui

import (
	catppuccin "github.com/catppuccin/go"
	"github.com/charmbracelet/lipgloss"
)

// SidebarWidth is the fixed width of the sidebar navigation pane.
const SidebarWidth = 22

// Catppuccin Mocha palette.
var flavor = catppuccin.Mocha

// Color constants extracted from the Mocha palette for convenience.
var (
	colorBase     = lipgloss.Color(flavor.Base().Hex)
	colorMantle   = lipgloss.Color(flavor.Mantle().Hex)
	colorSurface0 = lipgloss.Color(flavor.Surface0().Hex)
	colorSurface1 = lipgloss.Color(flavor.Surface1().Hex)
	colorText     = lipgloss.Color(flavor.Text().Hex)
	colorSubtext0 = lipgloss.Color(flavor.Subtext0().Hex)
	colorBlue     = lipgloss.Color(flavor.Blue().Hex)
	colorGreen    = lipgloss.Color(flavor.Green().Hex)
	colorRed      = lipgloss.Color(flavor.Red().Hex)
	colorYellow   = lipgloss.Color(flavor.Yellow().Hex)
	colorMauve    = lipgloss.Color(flavor.Mauve().Hex)
	colorPeach    = lipgloss.Color(flavor.Peach().Hex)
	colorTeal     = lipgloss.Color(flavor.Teal().Hex)
	colorPink     = lipgloss.Color(flavor.Pink().Hex)
	colorLavender = lipgloss.Color(flavor.Lavender().Hex)
	colorOverlay0 = lipgloss.Color(flavor.Overlay0().Hex)
	colorMaroon   = lipgloss.Color(flavor.Maroon().Hex)
)

// Catppuccin Latte colors (used for update banner contrast on Mocha background)
var (
	colorLatteMauve    = lipgloss.Color("#8839ef")
	colorLatteLavender = lipgloss.Color("#7287fd")
	colorLattePeach    = lipgloss.Color("#fe640b")
)

// ProfileTheme holds the accent color for a profile tab.
type ProfileTheme struct {
	Accent lipgloss.Color // bright accent (e.g. Blue)
}

// ProfileThemeRotation defines the color cycle for profile tabs.
// Base always gets index 0; subsequent profiles cycle through the rest.
var ProfileThemeRotation = []ProfileTheme{
	{colorBlue},
	{colorRed},
	{colorMauve},
	{colorPeach},
	{colorGreen},
	{colorTeal},
	{colorPink},
	{colorLavender},
}

// SectionAccent returns the Catppuccin accent color for a sidebar section.
func SectionAccent(s Section) lipgloss.Color {
	switch s {
	case SectionPlugins:
		return colorBlue
	case SectionSettings:
		return colorMauve
	case SectionClaudeMD:
		return colorGreen
	case SectionMemory:
		return colorPink
	case SectionPermissions:
		return colorRed
	case SectionMCP:
		return colorPeach
	case SectionCommandsSkills:
		return colorLavender
	case SectionKeybindings:
		return colorYellow
	case SectionHooks:
		return colorTeal
	default:
		return colorBlue
	}
}

// Tab bar styles.
var (
	// PlusTabStyle is used for the [+] new profile button.
	PlusTabStyle = lipgloss.NewStyle().
			Foreground(colorOverlay0).
			Background(colorSurface0).
			Padding(0, 1)
)

// Sidebar styles.
var (
	// InactiveSidebarStyle is used for non-selected sidebar items.
	InactiveSidebarStyle = lipgloss.NewStyle().
				Foreground(colorText).
				PaddingLeft(1)

	// UnavailableSidebarStyle is used for grayed-out sidebar items.
	UnavailableSidebarStyle = lipgloss.NewStyle().
				Foreground(colorOverlay0).
				PaddingLeft(1)
)

// Content pane styles.
var (
	// HeaderStyle is used for section headers in pickers.
	HeaderStyle = lipgloss.NewStyle().
			Foreground(colorMauve).
			Bold(true)

	// SelectedStyle is used for checked items.
	SelectedStyle = lipgloss.NewStyle().
			Foreground(colorGreen)

	// UnselectedStyle is used for unchecked items.
	UnselectedStyle = lipgloss.NewStyle().
			Foreground(colorText)

	// ContentPaneStyle wraps the main content area.
	ContentPaneStyle = lipgloss.NewStyle().
				PaddingLeft(1).
				PaddingRight(1)

	// BaseTagStyle is used for the "inherited" marker on base-inherited items.
	BaseTagStyle = lipgloss.NewStyle().
			Foreground(colorOverlay0).
			Italic(true)

	// DimStyle is used for unfocused items across picker and preview panels.
	DimStyle = lipgloss.NewStyle().
			Foreground(colorOverlay0)

	// RemovedBaseStyle is used for inherited items that the user has deselected
	// (explicitly removed from this profile). Maroon + strikethrough.
	RemovedBaseStyle = lipgloss.NewStyle().
				Foreground(colorMaroon).
				Strikethrough(true)

	// LockedStyle is used for plugin-controlled items that cannot be toggled.
	LockedStyle = lipgloss.NewStyle().
			Foreground(colorOverlay0)
)

// Status bar styles.
var (
	// StatusBarStyle is the base style for the bottom status bar.
	StatusBarStyle = lipgloss.NewStyle().
			Foreground(colorSubtext0).
			Background(colorSurface0).
			Padding(0, 1)

	// StatusBarKeyStyle highlights keyboard shortcuts in the status bar.
	StatusBarKeyStyle = lipgloss.NewStyle().
				Foreground(colorYellow).
				Background(colorSurface0).
				Bold(true)
)

// Overlay styles.
var (
	// OverlayStyle is the border and background for modal overlays.
	OverlayStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBlue).
			Background(colorMantle).
			Foreground(colorText).
			Padding(1, 2)

	// OverlayTitleStyle is used for the title text in overlays.
	OverlayTitleStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Bold(true)

	// OverlayButtonActiveStyle is used for the focused button in overlays.
	OverlayButtonActiveStyle = lipgloss.NewStyle().
					Foreground(colorBase).
					Background(colorBlue).
					Padding(0, 2)

	// OverlayButtonInactiveStyle is used for the unfocused button in overlays.
	OverlayButtonInactiveStyle = lipgloss.NewStyle().
					Foreground(colorText).
					Background(colorSurface1).
					Padding(0, 2)

	// OverlayChoiceCursorStyle is used for the cursor in choice overlays.
	OverlayChoiceCursorStyle = lipgloss.NewStyle().
					Foreground(colorBlue).
					Bold(true)

	// OverlayScrollHintStyle is used for scroll indicators in overlays.
	OverlayScrollHintStyle = lipgloss.NewStyle().
				Foreground(colorOverlay0)
)

// Helper text styles (contextual guide above content pane).
var (
	// HelperTextStyle is used for the dim helper text lines above content.
	HelperTextStyle = lipgloss.NewStyle().
			Foreground(colorOverlay0)

	// HelperSeparatorStyle is used for the thin line between helper and content.
	HelperSeparatorStyle = lipgloss.NewStyle().
				Foreground(colorSurface1)
)

// Shared text styles used across TUI views.
var (
	stDim     = lipgloss.NewStyle().Foreground(colorSubtext0)
	stText    = lipgloss.NewStyle().Foreground(colorText)
	stGreen   = lipgloss.NewStyle().Foreground(colorGreen)
	stRed     = lipgloss.NewStyle().Foreground(colorRed)
	stYellow  = lipgloss.NewStyle().Foreground(colorYellow)
	stBlue    = lipgloss.NewStyle().Bold(true).Foreground(colorBlue)
	stSection = lipgloss.NewStyle().Foreground(colorSurface1)
)

// clampWidth returns a maxWidth clamped to [30, 70] from terminal width,
// and an innerWidth accounting for border (2) + padding (4).
func clampWidth(termWidth int) (maxWidth, innerWidth int) {
	maxWidth = termWidth - 2
	if maxWidth > 70 {
		maxWidth = 70
	}
	if maxWidth < 30 {
		maxWidth = 30
	}
	innerWidth = maxWidth - 6
	if innerWidth < 20 {
		innerWidth = 20
	}
	return maxWidth, innerWidth
}

// contentBox returns a rounded-border box style at the given width and border color.
func contentBox(maxWidth int, borderColor lipgloss.Color) lipgloss.Style {
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Padding(1, 2).
		Width(maxWidth)
}

// Tab bar container style.
var (
	// TabBarStyle is the background strip for the tab bar row.
	TabBarStyle = lipgloss.NewStyle().
			Background(colorSurface0).
			Padding(0, 1)

	// SidebarContainerStyle wraps the sidebar column.
	SidebarContainerStyle = lipgloss.NewStyle().
				Width(SidebarWidth).
				BorderRight(true).
				BorderStyle(lipgloss.NormalBorder()).
				BorderForeground(colorSurface1)
)
