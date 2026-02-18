package tui

// FocusZone identifies which component currently has keyboard focus.
type FocusZone int

const (
	FocusTabBar  FocusZone = iota
	FocusSidebar           // Section navigation
	FocusContent           // Main content pane (picker, editor)
	FocusPreview           // CLAUDE.md preview, read-only scroll
)

// Section identifies a config section in the sidebar.
type Section int

const (
	SectionPlugins    Section = iota
	SectionSettings           // Claude settings (model, env, etc.)
	SectionClaudeMD           // CLAUDE.md fragments
	SectionPermissions        // Allow/deny permission rules
	SectionMCP                // MCP server configs
	SectionKeybindings        // Key bindings
	SectionHooks              // Auto-sync hooks
)

// String returns the display name for a section.
func (s Section) String() string {
	switch s {
	case SectionPlugins:
		return "Plugins"
	case SectionSettings:
		return "Settings"
	case SectionClaudeMD:
		return "CLAUDE.md"
	case SectionPermissions:
		return "Permissions"
	case SectionMCP:
		return "MCP"
	case SectionKeybindings:
		return "Keybindings"
	case SectionHooks:
		return "Hooks"
	default:
		return "Unknown"
	}
}

// AllSections lists every section in sidebar display order.
var AllSections = []Section{
	SectionPlugins,
	SectionSettings,
	SectionClaudeMD,
	SectionPermissions,
	SectionMCP,
	SectionKeybindings,
	SectionHooks,
}

// --- Inter-component messages ---

// TabSwitchMsg is sent when the user switches to a different profile tab.
type TabSwitchMsg struct{ Name string }

// NewProfileRequestMsg is sent when the user clicks the [+] tab.
type NewProfileRequestMsg struct{}

// DeleteProfileMsg is sent when the user requests deletion of a profile tab.
type DeleteProfileMsg struct{ Name string }

// SectionSwitchMsg is sent when the sidebar active section changes.
type SectionSwitchMsg struct{ Section Section }

// FocusChangeMsg requests a focus zone transition.
type FocusChangeMsg struct{ Zone FocusZone }

// ResizeMsg distributes terminal dimensions from root to children.
type ResizeMsg struct{ Width, Height int }

// OverlayCloseMsg is emitted when any overlay is dismissed.
type OverlayCloseMsg struct {
	Result    string // Text result (for text input or choice) or empty
	Confirmed bool   // true = OK/Submit, false = Cancel/Esc
}

// SaveRequestMsg is sent when the user presses Ctrl+S.
type SaveRequestMsg struct{}

// ResetRequestMsg is sent when the user presses Ctrl+R to reset to defaults.
type ResetRequestMsg struct{}

// SelectionSummary carries counts for the status bar.
type SelectionSummary struct {
	Selected int
	Total    int
	Section  Section
}
