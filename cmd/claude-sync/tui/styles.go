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
	colorOverlay0 = lipgloss.Color(flavor.Overlay0().Hex)
)

// Tab bar styles.
var (
	// ActiveTabStyle is used for the currently selected profile tab.
	ActiveTabStyle = lipgloss.NewStyle().
			Foreground(colorBase).
			Background(colorBlue).
			Padding(0, 1).
			Bold(true)

	// InactiveTabStyle is used for non-selected profile tabs.
	InactiveTabStyle = lipgloss.NewStyle().
				Foreground(colorText).
				Background(colorSurface0).
				Padding(0, 1)

	// PlusTabStyle is used for the [+] new profile button.
	PlusTabStyle = lipgloss.NewStyle().
			Foreground(colorOverlay0).
			Background(colorSurface0).
			Padding(0, 1)
)

// Sidebar styles.
var (
	// ActiveSidebarStyle is used for the currently selected sidebar item.
	ActiveSidebarStyle = lipgloss.NewStyle().
				Foreground(colorBlue).
				Background(colorSurface1).
				Bold(true).
				PaddingLeft(1)

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

	// BaseTagStyle is used for the [base] diff marker on inherited items.
	BaseTagStyle = lipgloss.NewStyle().
			Foreground(colorOverlay0).
			Italic(true)
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
