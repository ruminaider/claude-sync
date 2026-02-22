package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/cmdskill"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// SkipFlags controls which sections are skipped (deselected) via CLI flags.
type SkipFlags struct {
	Plugins        bool
	Settings       bool
	Hooks          bool
	Permissions    bool
	ClaudeMD       bool
	MCP            bool
	Keybindings    bool
	CommandsSkills bool
}

// overlayContext tracks what the currently-active overlay was opened for.
type overlayContext int

const (
	overlayNone          overlayContext = iota
	overlayConfigStyle                         // initial "Simple vs Profiles" choice
	overlayProfileName                         // text input for new profile name (single, from [+] tab)
	overlayProfileNames                        // batch profile naming screen
	overlayDeleteConfirm                       // delete profile confirmation
	overlaySaveSummary                         // save summary with confirm/cancel
	overlayResetConfirm                        // reset all to defaults confirmation
	overlayQuitConfirm                         // quit without saving confirmation
	overlayHelp                                // help modal
)

// Model is the root bubbletea model that composes all TUI child components.
type Model struct {
	// Scan data (read-only after init).
	scanResult *commands.InitScanResult
	claudeDir  string
	syncDir    string
	remoteURL  string

	// Edit mode: true when editing an existing config.
	editMode       bool
	existingConfig *config.Config

	// User selections per section for base config.
	pickers map[Section]Picker // for plugins, settings, permissions, mcp, hooks, keybindings
	preview Preview            // for CLAUDE.md

	// Profile data.
	profilePickers  map[string]map[Section]Picker // profileName -> section -> picker
	profilePreviews map[string]Preview            // profileName -> CLAUDE.md preview
	useProfiles     bool

	// Layout components.
	tabBar    TabBar
	sidebar   Sidebar
	statusBar StatusBar
	overlay   Overlay

	// Overlay tracking.
	overlayCtx       overlayContext
	pendingDeleteTab string // profile name being deleted

	// State.
	focusZone     FocusZone
	activeTab     string // "Base" or profile name
	activeSection Section
	width, height int
	ready         bool // set after first WindowSizeMsg
	quitting      bool
	skipFlags     SkipFlags

	// Profile diffs: tracks explicit add/remove overrides relative to base per section.
	profileDiffs map[string]map[Section]*sectionDiff

	// Discovered MCP servers from project-level .mcp.json files.
	discoveredMCP  map[string]json.RawMessage // server name → config
	mcpSources     map[string]string          // server name → shortened source path
	mcpPluginKeys  map[string]string          // MCP server name → plugin key (for plugin-provided servers)

	// Discovered commands/skills from project-level .claude/ directories.
	discoveredCmdSkills []cmdskill.Item

	// Result -- set on save, read by cmd_init.go after TUI exits.
	Result *commands.InitOptions
}

// NewModel creates the root TUI model from scan results.
// If existingConfig is non-nil, the TUI enters edit mode: selections are
// pre-populated from the config, the initial overlay is skipped, and profiles
// are restored from existingProfiles.
func NewModel(scan *commands.InitScanResult, claudeDir, syncDir, remoteURL string, skipProfiles bool, skip SkipFlags,
	existingConfig *config.Config, existingProfiles map[string]profiles.Profile) Model {
	m := Model{
		scanResult:         scan,
		claudeDir:          claudeDir,
		syncDir:            syncDir,
		remoteURL:          remoteURL,
		editMode:           existingConfig != nil,
		existingConfig:     existingConfig,
		pickers:            make(map[Section]Picker),
		profilePickers:     make(map[string]map[Section]Picker),
		profilePreviews:    make(map[string]Preview),
		profileDiffs: make(map[string]map[Section]*sectionDiff),
		activeTab:          "Base",
		activeSection:      SectionPlugins,
		focusZone:          FocusSidebar,
		skipFlags:          skip,
		discoveredMCP:      make(map[string]json.RawMessage),
		mcpSources:         make(map[string]string),
		mcpPluginKeys:      make(map[string]string),
	}

	// Build pickers for each section from scan data.
	m.pickers[SectionPlugins] = NewPicker(PluginPickerItems(scan))
	m.pickers[SectionSettings] = NewPicker(SettingsPickerItems(scan.Settings))
	m.pickers[SectionPermissions] = NewPicker(PermissionPickerItems(scan.Permissions))
	mcpPicker := NewPicker(MCPPickerItems(scan.MCP, "~/.claude/.mcp.json"))
	mcpPicker.SetSearchAction(true)
	mcpPicker.searching = true
	m.pickers[SectionMCP] = mcpPicker
	m.pickers[SectionHooks] = NewPicker(HookPickerItems(scan.Hooks))
	m.pickers[SectionKeybindings] = NewPicker(KeybindingsPickerItems(scan.Keybindings))

	// Build Commands & Skills picker.
	csPicker := NewPicker(CommandsSkillsPickerItems(scan.CommandsSkills))
	csPicker.SetSearchAction(true)
	csPicker.searching = true
	csPicker.CollapseReadOnly = true
	csPicker.autoCollapseReadOnly()
	if scan.CommandsSkills != nil {
		previewContent := make(map[string]string)
		for _, item := range scan.CommandsSkills.Items {
			previewContent[item.Key()] = item.Content
		}
		csPicker.SetPreview(previewContent)
	}
	m.pickers[SectionCommandsSkills] = csPicker

	// Build CLAUDE.md preview.
	previewSections := ClaudeMDPreviewSections(scan.ClaudeMDSections, "~/.claude/CLAUDE.md")
	m.preview = NewPreview(previewSections)
	m.preview.searching = true

	// In edit mode, pre-populate selections from existing config.
	if existingConfig != nil {
		m.applyExistingConfig(existingConfig, existingProfiles)
	}

	// Apply skip flags: deselect everything for skipped sections.
	if skip.Plugins {
		m.deselectPicker(SectionPlugins)
	}
	if skip.Settings {
		m.deselectPicker(SectionSettings)
	}
	if skip.Permissions {
		m.deselectPicker(SectionPermissions)
	}
	if skip.MCP {
		m.deselectPicker(SectionMCP)
	}
	if skip.Hooks {
		m.deselectPicker(SectionHooks)
	}
	if skip.Keybindings {
		m.deselectPicker(SectionKeybindings)
	}
	if skip.CommandsSkills {
		m.deselectPicker(SectionCommandsSkills)
	}
	if skip.ClaudeMD {
		// Deselect all CLAUDE.md sections.
		for i := range m.preview.SelectedFragmentKeys() {
			m.preview.selected[i] = false
		}
		// Actually clear all:
		for i := range previewSections {
			m.preview.selected[i] = false
		}
	}

	// Initialize sidebar and tab bar.
	m.sidebar = NewSidebar()
	m.sidebar.SetAlwaysAvailable(SectionMCP)
	m.sidebar.SetAlwaysAvailable(SectionCommandsSkills)
	m.syncSidebarCounts()
	m.tabBar = NewTabBar(nil)

	// Initialize status bar.
	m.statusBar = NewStatusBar()

	// Show the config style choice overlay only for fresh creates (not edit mode).
	if !skipProfiles && hasScanData(scan) && existingConfig == nil {
		m.overlay = NewChoiceOverlay("Configuration style", []string{
			"Simple (single config)",
			"With profiles (e.g., work, personal)",
		})
		m.overlayCtx = overlayConfigStyle
	}

	// In edit mode, restore profiles after tab bar is initialized.
	if existingConfig != nil && len(existingProfiles) > 0 {
		m.restoreProfiles(existingConfig, existingProfiles)
	}

	return m
}

// Init satisfies tea.Model. Triggers auto-search for CLAUDE.md, MCP, and
// Commands/Skills so discovered items appear without requiring a manual button press.
func (m Model) Init() tea.Cmd {
	return tea.Batch(SearchClaudeMD(), SearchMCPConfigs(), SearchCommandsSkills())
}

// Update satisfies tea.Model. Routes messages to the correct child component.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.ready = true
		m.distributeSize()
		return m, nil

	case SearchDoneMsg:
		return m.handleSearchDone(msg), nil

	case MCPSearchDoneMsg:
		return m.handleMCPSearchDone(msg), nil

	case CmdSkillSearchDoneMsg:
		return m.handleCmdSkillSearchDone(msg), nil
	}

	// When overlay is active, route ALL messages to the overlay.
	if m.overlay.Active() {
		return m.updateOverlay(msg)
	}

	// Global key handling.
	if msg, ok := msg.(tea.KeyMsg); ok {
		switch msg.String() {
		case "ctrl+s":
			return m.triggerSave()
		case "ctrl+r":
			m.overlay = NewConfirmOverlay("Reset", "Reset all selections to scan defaults?")
			m.overlayCtx = overlayResetConfirm
			return m, nil
		case "ctrl+c":
			m.quitting = true
			return m, tea.Quit
		case "tab":
			// Save current profile diffs before switching away.
			m.saveAllProfileDiffs(m.activeTab)
			m.tabBar.CycleNext()
			if !m.tabBar.OnPlus() {
				m.activeTab = m.tabBar.ActiveTab()
				m.rebuildAllProfilePickers(m.activeTab)
				m.syncSidebarCounts()
			}
			m.syncStatusBar()
			return m, nil
		case "shift+tab":
			// Save current profile diffs before switching away.
			m.saveAllProfileDiffs(m.activeTab)
			m.tabBar.CyclePrev()
			if !m.tabBar.OnPlus() {
				m.activeTab = m.tabBar.ActiveTab()
				m.rebuildAllProfilePickers(m.activeTab)
				m.syncSidebarCounts()
			}
			m.syncStatusBar()
			return m, nil
		case "enter":
			// When [+] is focused, open new profile overlay.
			if m.tabBar.OnPlus() {
				m.overlay = NewTextInputOverlay("New profile name", "e.g. work, personal")
				m.overlayCtx = overlayProfileName
				return m, nil
			}
		case "+":
			// Global: add new profile.
			m.overlay = NewTextInputOverlay("New profile name", "e.g. work, personal")
			m.overlayCtx = overlayProfileName
			return m, nil
		case "ctrl+d":
			// Global: delete current profile (not Base).
			if m.activeTab != "Base" {
				m.pendingDeleteTab = m.activeTab
				m.overlay = NewConfirmOverlay("Delete profile",
					fmt.Sprintf("Delete profile %q?", m.activeTab))
				m.overlayCtx = overlayDeleteConfirm
				return m, nil
			}
		case "?":
			m.overlay = NewHelpOverlay()
			m.overlay.SetWidth(OverlayMaxWidth(m.width))
			m.overlay.SetHeight(m.height)
			m.overlayCtx = overlayHelp
			return m, nil
		case "h":
			// h opens help from sidebar; in content, h means "move left" (handled by picker/sidebar).
			if m.focusZone == FocusSidebar {
				m.overlay = NewHelpOverlay()
				m.overlay.SetWidth(OverlayMaxWidth(m.width))
				m.overlay.SetHeight(m.height)
				m.overlayCtx = overlayHelp
				return m, nil
			}
		case "esc":
			if m.focusZone == FocusContent || m.focusZone == FocusPreview {
				// If the active picker has a filter, let it handle Esc first
				// (clears filter). Only go to sidebar if filter was already empty.
				if m.activeSection != SectionClaudeMD {
					p := m.currentPicker()
					if p.filterText != "" {
						var escCmds []tea.Cmd
						return m.updatePicker(msg, &escCmds)
					}
				}
				m.focusZone = FocusSidebar
				return m, nil
			}
			// Esc on sidebar → quit confirmation.
			m.overlay = NewConfirmOverlay("Quit", "Discard selections and exit?")
			m.overlayCtx = overlayQuitConfirm
			return m, nil
		}
	}

	// Focus-based routing.
	switch m.focusZone {
	case FocusSidebar:
		return m.updateSidebar(msg, &cmds)
	case FocusContent, FocusPreview:
		return m.updateContent(msg, &cmds)
	}

	return m, tea.Batch(cmds...)
}

// View satisfies tea.Model.
func (m Model) View() string {
	if m.quitting {
		return ""
	}
	if !m.ready {
		return "Loading..."
	}
	// When overlay is active, skip rendering the full background — the overlay
	// blanks the viewport anyway. This avoids expensive picker/preview renders
	// on every keystroke in text input overlays.
	if m.overlay.Active() {
		blank := strings.Repeat(strings.Repeat(" ", m.width)+"\n", m.height-1)
		blank += strings.Repeat(" ", m.width)
		return Composite(blank, m.overlay.View(), m.width, m.height)
	}

	// Render status bar first so we can measure its actual height.
	statusView := m.statusBar.View()
	statusBarHeight := strings.Count(statusView, "\n") + 1
	mainHeight := m.height - statusBarHeight

	// Sidebar spans full height (minus status bar).
	m.sidebar.SetFocused(m.focusZone == FocusSidebar)
	sidebarView := m.sidebar.View()

	// Right pane: optional tab bar + helper text + content.
	hLines := helperLines(m.height)
	contentHeight := mainHeight - hLines
	tabBarView := m.tabBar.View() + "\n"
	contentHeight = mainHeight - 1 - hLines // tab bar takes 1 line

	// Contextual helper text.
	isProfile := m.activeTab != "Base"
	contentWidth := m.width - SidebarWidth - 2
	if contentWidth < 10 {
		contentWidth = 10
	}
	var accentColor lipgloss.Color
	if isProfile && (m.focusZone == FocusContent || m.focusZone == FocusPreview) {
		accentColor = m.tabBar.ActiveTheme().Accent
	}
	helperView := renderHelper(m.activeSection, isProfile, contentWidth, hLines, accentColor)

	contentView := m.currentContentView(contentHeight)

	// Add a colored left border matching the active profile's accent color.
	accent := m.tabBar.ActiveTheme().Accent
	borderedContent := lipgloss.NewStyle().
		BorderLeft(true).
		BorderStyle(lipgloss.ThickBorder()).
		BorderForeground(accent).
		Render(helperView + contentView)
	rightPane := tabBarView + borderedContent

	// Join sidebar and right pane horizontally.
	mainArea := lipgloss.JoinHorizontal(lipgloss.Top, sidebarView, rightPane)

	return mainArea + "\n" + statusView
}

// --- Update helpers ---

func (m Model) updateOverlay(msg tea.Msg) (tea.Model, tea.Cmd) {
	wasActive := m.overlay.Active()
	var cmd tea.Cmd
	m.overlay, cmd = m.overlay.Update(msg)

	// When the overlay just closed, the cmd is an OverlayCloseMsg producer.
	// Handle it directly instead of sending through the event loop.
	if wasActive && !m.overlay.Active() && cmd != nil {
		if closeMsg := extractOverlayClose(cmd); closeMsg != nil {
			return m.handleOverlayClose(*closeMsg)
		}
	}

	return m, cmd
}

func (m Model) handleOverlayClose(msg OverlayCloseMsg) (tea.Model, tea.Cmd) {
	ctx := m.overlayCtx
	m.overlayCtx = overlayNone

	switch ctx {
	case overlayConfigStyle:
		if !msg.Confirmed {
			// User cancelled the first screen — quit.
			m.quitting = true
			return m, tea.Quit
		}
		if strings.HasPrefix(msg.Result, "Simple") {
			m.useProfiles = false
		} else {
			m.useProfiles = true
			m.overlay = NewProfileListOverlay()
			m.overlayCtx = overlayProfileNames
			return m, nil
		}

	case overlayProfileNames:
		if !msg.Confirmed || len(msg.Results) == 0 {
			// Go back to config style choice.
			m.overlay = NewChoiceOverlay("Configuration style", []string{
				"Simple (single config)",
				"With profiles (e.g., work, personal)",
			})
			m.overlayCtx = overlayConfigStyle
			m.useProfiles = false
			return m, nil
		}
		for _, name := range msg.Results {
			m.createProfile(name)
		}
		// Switch to Base tab so user configures base first.
		m.activeTab = "Base"
		m.tabBar.SetActive(0)
		m.focusZone = FocusSidebar
		m.distributeSize()
		m.syncSidebarCounts()
		m.syncStatusBar()

	case overlayProfileName:
		if !msg.Confirmed || msg.Result == "" {
			return m, nil
		}
		name := strings.TrimSpace(strings.ToLower(msg.Result))
		if name == "" || name == "base" {
			return m, nil
		}
		// Check for duplicate.
		if _, exists := m.profilePickers[name]; exists {
			return m, nil
		}
		m.createProfile(name)

	case overlayDeleteConfirm:
		if msg.Confirmed && m.pendingDeleteTab != "" {
			m.deleteProfile(m.pendingDeleteTab)
			m.pendingDeleteTab = ""
		}

	case overlaySaveSummary:
		if msg.Confirmed {
			opts := m.buildInitOptions()
			m.Result = opts
			m.quitting = true
			return m, tea.Quit
		}

	case overlayResetConfirm:
		if msg.Confirmed {
			m.resetToDefaults()
		}

	case overlayQuitConfirm:
		if msg.Confirmed {
			m.quitting = true
			return m, tea.Quit
		}

	case overlayHelp:
		// No-op, just dismiss.
	}

	return m, nil
}

func (m Model) updateSidebar(msg tea.Msg, cmds *[]tea.Cmd) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	m.sidebar, cmd = m.sidebar.Update(msg)
	if cmd != nil {
		*cmds = append(*cmds, cmd)

		if sectionMsg := extractSectionSwitch(cmd); sectionMsg != nil {
			m.activeSection = sectionMsg.Section
			m.syncStatusBar()
		}
		if focusMsg := extractFocusChange(cmd); focusMsg != nil {
			m.focusZone = focusMsg.Zone
		}
	}

	return m, tea.Batch(*cmds...)
}

func (m Model) updateContent(msg tea.Msg, cmds *[]tea.Cmd) (tea.Model, tea.Cmd) {
	if m.activeSection == SectionClaudeMD {
		return m.updatePreview(msg, cmds)
	}
	return m.updatePicker(msg, cmds)
}

func (m Model) updatePicker(msg tea.Msg, cmds *[]tea.Cmd) (tea.Model, tea.Cmd) {
	p := m.currentPicker()
	var cmd tea.Cmd
	newP, cmd := p.Update(msg)
	m.setCurrentPicker(newP)

	if cmd != nil {
		*cmds = append(*cmds, cmd)
		if focusMsg := extractFocusChange(cmd); focusMsg != nil {
			m.focusZone = focusMsg.Zone
		}
		if extractSearchRequest(cmd) {
			// Set searching state on the active picker.
			sp := m.currentPicker()
			sp.searching = true
			m.setCurrentPicker(sp)
			switch m.activeSection {
			case SectionMCP:
				return m, SearchMCPConfigs()
			case SectionCommandsSkills:
				return m, SearchCommandsSkills()
			}
		}
	}

	m.syncSidebarCounts()
	m.syncStatusBar()

	return m, tea.Batch(*cmds...)
}

func (m Model) updatePreview(msg tea.Msg, cmds *[]tea.Cmd) (tea.Model, tea.Cmd) {
	preview := m.currentPreview()
	var cmd tea.Cmd
	newPreview, cmd := preview.Update(msg)
	m.setCurrentPreview(newPreview)

	if cmd != nil {
		*cmds = append(*cmds, cmd)
		if focusMsg := extractFocusChange(cmd); focusMsg != nil {
			m.focusZone = focusMsg.Zone
		}
		if extractSearchRequest(cmd) {
			// Set searching state on all previews.
			m.preview.searching = true
			for name, pp := range m.profilePreviews {
				pp.searching = true
				m.profilePreviews[name] = pp
			}
			return m, SearchClaudeMD()
		}
	}

	m.syncSidebarCounts()
	m.syncStatusBar()

	return m, tea.Batch(*cmds...)
}

// --- View helpers ---

func (m Model) currentContentView(height int) string {
	contentFocused := m.focusZone == FocusContent || m.focusZone == FocusPreview
	var view string
	if m.activeSection == SectionClaudeMD {
		p := m.currentPreview()
		p.focused = contentFocused
		view = p.View()
	} else {
		p := m.currentPicker()
		p.focused = contentFocused
		view = p.View()
	}
	return clampHeight(view, height)
}

// clampHeight truncates s to at most maxLines lines, preventing layout overflow.
func clampHeight(s string, maxLines int) string {
	if maxLines <= 0 {
		return ""
	}
	lines := strings.SplitN(s, "\n", maxLines+1)
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	return strings.Join(lines, "\n")
}

// currentPicker returns the picker for the current tab and section.
func (m Model) currentPicker() Picker {
	if m.activeTab != "Base" {
		if pm, ok := m.profilePickers[m.activeTab]; ok {
			if p, ok := pm[m.activeSection]; ok {
				return p
			}
		}
	}
	if p, ok := m.pickers[m.activeSection]; ok {
		return p
	}
	return NewPicker(nil)
}

func (m *Model) setCurrentPicker(p Picker) {
	if m.activeTab != "Base" {
		if pm, ok := m.profilePickers[m.activeTab]; ok {
			pm[m.activeSection] = p
			return
		}
	}
	m.pickers[m.activeSection] = p
}

// currentPreview returns the preview for the current tab.
func (m Model) currentPreview() Preview {
	if m.activeTab != "Base" {
		if p, ok := m.profilePreviews[m.activeTab]; ok {
			return p
		}
	}
	return m.preview
}

func (m *Model) setCurrentPreview(p Preview) {
	if m.activeTab != "Base" {
		m.profilePreviews[m.activeTab] = p
		return
	}
	m.preview = p
}

// helperLines returns the number of lines the contextual helper text occupies
// based on terminal height. Returns 3 (full), 2 (compact), or 0 (hidden).
func helperLines(termHeight int) int {
	switch {
	case termHeight >= 28:
		return 3 // description + shortcuts + separator
	case termHeight >= 24:
		return 2 // description + separator
	default:
		return 0 // hidden
	}
}

// helperText returns the two-line contextual description for a section.
// line1 is a description of what the section does, line2 is keyboard shortcuts.
func helperText(section Section, isProfile bool) (string, string) {
	var line1 string

	if isProfile {
		switch section {
		case SectionPlugins:
			line1 = "● = from base. Toggle to add or remove."
		case SectionSettings:
			line1 = "● = from base. Override settings for this profile."
		case SectionClaudeMD:
			line1 = "● = from base. Add or exclude sections for this profile."
		case SectionPermissions:
			line1 = "● = from base. Adjust rules for this profile."
		case SectionMCP:
			line1 = "● = from base. Add or remove servers for this profile."
		case SectionKeybindings:
			line1 = "● = from base. Override keybindings for this profile."
		case SectionHooks:
			line1 = "● = from base. Add or remove hooks for this profile."
		case SectionCommandsSkills:
			line1 = "● = from base. Add or remove for this profile."
		default:
			line1 = "Configure this section."
		}
	} else {
		switch section {
		case SectionPlugins:
			line1 = "Choose which Claude plugins to sync."
		case SectionSettings:
			line1 = "Select Claude settings to include."
		case SectionClaudeMD:
			line1 = "Pick which CLAUDE.md sections to sync."
		case SectionPermissions:
			line1 = "Select allow/deny permission rules."
		case SectionMCP:
			line1 = "Choose which MCP server configs to sync."
		case SectionKeybindings:
			line1 = "Include or exclude keybinding config."
		case SectionHooks:
			line1 = "Select which auto-sync hooks to include."
		case SectionCommandsSkills:
			line1 = "Choose commands and skills to sync."
		default:
			line1 = "Configure this section."
		}
	}

	// Line 2: keyboard shortcuts (same for base and profile).
	var line2 string
	switch section {
	case SectionClaudeMD:
		line2 = "Space: toggle \u00b7 a: all \u00b7 n: none \u00b7 →: preview content"
	case SectionCommandsSkills:
		line2 = "Space: toggle \u00b7 Ctrl+A: all \u00b7 Ctrl+N: none \u00b7 →: preview \u00b7 type to filter"
	case SectionKeybindings:
		line2 = "Space: toggle \u00b7 type to filter"
	default:
		line2 = "Space: toggle \u00b7 Ctrl+A: all \u00b7 Ctrl+N: none \u00b7 type to filter"
	}

	return line1, line2
}

// renderHelper renders the helper block above content.
// lines controls the format: 3 = full (desc + shortcuts + sep), 2 = compact (desc + sep), 0 = hidden.
// accentColor, when set, colors the ● marker in the helper text to match the profile tab.
func renderHelper(section Section, isProfile bool, width, lines int, accentColor lipgloss.Color) string {
	if lines <= 0 {
		return ""
	}

	line1, line2 := helperText(section, isProfile)

	// Color the ● marker to match the profile accent.
	if accentColor != "" && strings.Contains(line1, "●") {
		colored := lipgloss.NewStyle().Foreground(accentColor).Render("●")
		line1 = strings.Replace(line1, "●", colored, 1)
	}

	var b strings.Builder
	b.WriteString(HelperTextStyle.Render(" " + line1))
	b.WriteString("\n")
	if lines >= 3 {
		b.WriteString(HelperTextStyle.Render(" " + line2))
		b.WriteString("\n")
	}
	sep := strings.Repeat("─", width)
	b.WriteString(HelperSeparatorStyle.Render(sep))
	b.WriteString("\n")

	return b.String()
}

// --- Size distribution ---

func (m *Model) distributeSize() {
	statusBarHeight := 1
	mainHeight := m.height - statusBarHeight

	// Sidebar spans full main height.
	m.sidebar.SetHeight(mainHeight)

	// Content width = terminal - sidebar - sidebar border (2 = border char + space).
	contentWidth := m.width - SidebarWidth - 2
	if contentWidth < 10 {
		contentWidth = 10
	}

	// Tab bar sits above content, same width.
	m.tabBar.SetWidth(contentWidth)

	// Content height = main height minus tab bar minus helper text.
	contentHeight := mainHeight - 1 - helperLines(m.height)

	m.statusBar.SetWidth(m.width)

	overlayWidth := OverlayMaxWidth(m.width)
	m.overlay.SetWidth(overlayWidth)

	// Update all pickers and preview with current dimensions.
	for sec, p := range m.pickers {
		p.SetHeight(contentHeight)
		p.SetWidth(contentWidth)
		m.pickers[sec] = p
	}
	m.preview.SetSize(contentWidth, contentHeight)

	for name, pm := range m.profilePickers {
		for sec, p := range pm {
			p.SetHeight(contentHeight)
			p.SetWidth(contentWidth)
			pm[sec] = p
		}
		m.profilePickers[name] = pm
	}
	for name, p := range m.profilePreviews {
		p.SetSize(contentWidth, contentHeight)
		m.profilePreviews[name] = p
	}
}

// --- Sync helpers ---

func (m *Model) syncSidebarCounts() {
	// Sync plugin-provided MCP read-only state whenever counts are refreshed,
	// since this runs after every picker update (including plugin toggles).
	m.syncMCPPluginState()

	for _, sec := range AllSections {
		if sec == SectionClaudeMD {
			preview := m.currentPreview()
			m.sidebar.UpdateCounts(sec, preview.SelectedCount(), preview.TotalCount())
		} else {
			picker := m.currentPicker()
			if sec == m.activeSection {
				m.sidebar.UpdateCounts(sec, picker.SelectedCount(), picker.TotalCount())
			} else {
				// For non-active sections, use the stored picker.
				if m.activeTab != "Base" {
					if pm, ok := m.profilePickers[m.activeTab]; ok {
						if p, ok := pm[sec]; ok {
							m.sidebar.UpdateCounts(sec, p.SelectedCount(), p.TotalCount())
							continue
						}
					}
				}
				if p, ok := m.pickers[sec]; ok {
					m.sidebar.UpdateCounts(sec, p.SelectedCount(), p.TotalCount())
				}
			}
		}
	}
}

func (m *Model) syncStatusBar() {
	var summary SelectionSummary
	if m.activeSection == SectionClaudeMD {
		preview := m.currentPreview()
		summary = SelectionSummary{
			Selected: preview.SelectedCount(),
			Total:    preview.TotalCount(),
			Section:  SectionClaudeMD,
		}
	} else {
		picker := m.currentPicker()
		summary = SelectionSummary{
			Selected: picker.SelectedCount(),
			Total:    picker.TotalCount(),
			Section:  m.activeSection,
		}
	}
	m.statusBar.Update(summary, m.activeTab)
}

// --- Profile management ---

func (m *Model) createProfile(name string) {
	// Initialize empty pickers and diffs — rebuildAll will populate from base.
	m.profilePickers[name] = make(map[Section]Picker)
	m.profileDiffs[name] = make(map[Section]*sectionDiff)
	for _, sec := range AllSections {
		m.profileDiffs[name][sec] = newSectionDiff()
	}

	// Update tab bar and switch to new profile.
	m.tabBar.AddTab(name)
	m.activeTab = name
	m.useProfiles = true

	// Rebuild all sections from base + empty diffs (effective == base).
	m.rebuildAllProfilePickers(name)

	m.syncSidebarCounts()
	m.syncStatusBar()

	if m.ready {
		m.distributeSize()
	}
}

func (m *Model) deleteProfile(name string) {
	delete(m.profilePickers, name)
	delete(m.profilePreviews, name)
	delete(m.profileDiffs, name)
	m.tabBar.RemoveTab(name)
	m.activeTab = m.tabBar.ActiveTab()

	// If no profiles left, switch back to simple mode.
	if len(m.profilePickers) == 0 {
		m.useProfiles = false
	}

	m.syncSidebarCounts()
	m.syncStatusBar()

	if m.ready {
		m.distributeSize()
	}
}

// --- Deselect helpers ---

func (m *Model) deselectPicker(section Section) {
	if p, ok := m.pickers[section]; ok {
		for i := range p.items {
			if !p.items[i].IsHeader {
				p.items[i].Selected = false
			}
		}
		p.syncSelectAll()
		m.pickers[section] = p
	}
}

// --- Reset ---

func (m *Model) resetToDefaults() {
	m.pickers[SectionPlugins] = NewPicker(PluginPickerItems(m.scanResult))
	m.pickers[SectionSettings] = NewPicker(SettingsPickerItems(m.scanResult.Settings))
	m.pickers[SectionPermissions] = NewPicker(PermissionPickerItems(m.scanResult.Permissions))
	mcpPicker := NewPicker(MCPPickerItems(m.scanResult.MCP, "~/.claude/.mcp.json"))
	mcpPicker.SetSearchAction(true)
	m.pickers[SectionMCP] = mcpPicker
	m.pickers[SectionHooks] = NewPicker(HookPickerItems(m.scanResult.Hooks))
	m.pickers[SectionKeybindings] = NewPicker(KeybindingsPickerItems(m.scanResult.Keybindings))

	// Reset Commands & Skills picker.
	csPicker := NewPicker(CommandsSkillsPickerItems(m.scanResult.CommandsSkills))
	csPicker.SetSearchAction(true)
	csPicker.CollapseReadOnly = true
	csPicker.autoCollapseReadOnly()
	if m.scanResult.CommandsSkills != nil {
		previewContent := make(map[string]string)
		for _, item := range m.scanResult.CommandsSkills.Items {
			previewContent[item.Key()] = item.Content
		}
		csPicker.SetPreview(previewContent)
	}
	m.pickers[SectionCommandsSkills] = csPicker

	// Clear discovered MCP servers, project commands/skills, and profile diffs on reset.
	m.discoveredMCP = make(map[string]json.RawMessage)
	m.mcpSources = make(map[string]string)
	m.mcpPluginKeys = make(map[string]string)
	m.discoveredCmdSkills = nil
	m.profileDiffs = make(map[string]map[Section]*sectionDiff)

	previewSections := ClaudeMDPreviewSections(m.scanResult.ClaudeMDSections, "~/.claude/CLAUDE.md")
	m.preview = NewPreview(previewSections)

	// Re-apply skip flags.
	if m.skipFlags.Plugins {
		m.deselectPicker(SectionPlugins)
	}
	if m.skipFlags.Settings {
		m.deselectPicker(SectionSettings)
	}
	if m.skipFlags.Permissions {
		m.deselectPicker(SectionPermissions)
	}
	if m.skipFlags.MCP {
		m.deselectPicker(SectionMCP)
	}
	if m.skipFlags.Hooks {
		m.deselectPicker(SectionHooks)
	}
	if m.skipFlags.Keybindings {
		m.deselectPicker(SectionKeybindings)
	}
	if m.skipFlags.CommandsSkills {
		m.deselectPicker(SectionCommandsSkills)
	}
	if m.skipFlags.ClaudeMD {
		for i := range m.preview.sections {
			m.preview.selected[i] = false
		}
	}

	m.syncSidebarCounts()
	m.syncStatusBar()

	if m.ready {
		m.distributeSize()
	}
}

// --- Save/Build ---

func (m Model) triggerSave() (tea.Model, tea.Cmd) {
	// Compute summary stats for the overlay body.
	stats := make(map[string]int)
	stats["plugins"] = m.pickers[SectionPlugins].SelectedCount()
	stats["settings"] = m.pickers[SectionSettings].SelectedCount()
	stats["claudemd"] = m.preview.SelectedCount()
	stats["permissions"] = m.pickers[SectionPermissions].SelectedCount()
	stats["mcp"] = m.pickers[SectionMCP].SelectedCount()
	stats["keybindings"] = m.pickers[SectionKeybindings].SelectedCount()
	stats["hooks"] = m.pickers[SectionHooks].SelectedCount()
	stats["commands_skills"] = m.pickers[SectionCommandsSkills].SelectedCount()

	// Check that at least something is selected.
	total := 0
	for _, v := range stats {
		total += v
	}
	if total == 0 {
		// Nothing selected -- do not show summary.
		return m, nil
	}

	var profileNames []string
	for name := range m.profilePickers {
		profileNames = append(profileNames, name)
	}

	body := FormatSummaryBody(stats, profileNames)
	actionLabel := "Initialize"
	title := "Initialize sync config"
	if m.editMode {
		actionLabel = "Update"
		title = "Update sync config"
	}
	m.overlay = NewSummaryOverlay(title, body, actionLabel)
	m.overlayCtx = overlaySaveSummary
	return m, nil
}

// buildInitOptions translates TUI selections into the business logic contract.
func (m Model) buildInitOptions() *commands.InitOptions {
	opts := &commands.InitOptions{
		ClaudeDir: m.claudeDir,
		SyncDir:   m.syncDir,
		RemoteURL: m.remoteURL,
	}

	// Plugins: nil = all, [] = none, [...] = some.
	pluginKeys := m.pickers[SectionPlugins].SelectedKeys()
	pluginTotal := m.pickers[SectionPlugins].TotalCount()
	if len(pluginKeys) == pluginTotal && pluginTotal > 0 {
		opts.IncludePlugins = nil // all
	} else {
		opts.IncludePlugins = pluginKeys
		if opts.IncludePlugins == nil {
			opts.IncludePlugins = []string{} // explicitly empty = none
		}
	}

	// Settings: IncludeSettings + SettingsFilter.
	settingsKeys := m.pickers[SectionSettings].SelectedKeys()
	if len(settingsKeys) > 0 {
		opts.IncludeSettings = true
		settingsTotal := m.pickers[SectionSettings].TotalCount()
		if len(settingsKeys) == settingsTotal {
			opts.SettingsFilter = nil // nil = all
		} else {
			opts.SettingsFilter = settingsKeys
		}
	}

	// Permissions: extract allow/deny from keys prefixed with "allow:" / "deny:".
	permKeys := m.pickers[SectionPermissions].SelectedKeys()
	for _, k := range permKeys {
		if strings.HasPrefix(k, "allow:") {
			opts.Permissions.Allow = append(opts.Permissions.Allow, strings.TrimPrefix(k, "allow:"))
		} else if strings.HasPrefix(k, "deny:") {
			opts.Permissions.Deny = append(opts.Permissions.Deny, strings.TrimPrefix(k, "deny:"))
		}
	}

	// CLAUDE.md fragments.
	fragKeys := m.preview.SelectedFragmentKeys()
	if len(fragKeys) > 0 {
		opts.ImportClaudeMD = true
		if len(fragKeys) == m.preview.TotalCount() {
			opts.ClaudeMDFragments = nil // nil = all
		} else {
			opts.ClaudeMDFragments = fragKeys
		}
	}

	// MCP: map values from scanResult and discoveredMCP. Empty map = none.
	// Skip plugin-provided servers whose plugin is already selected (they come
	// with the plugin). SelectedKeys() already excludes IsReadOnly items, but
	// we guard explicitly in case the state is stale.
	selectedPlugins := toSet(m.pickers[SectionPlugins].SelectedKeys())
	mcpKeys := m.pickers[SectionMCP].SelectedKeys()
	if len(mcpKeys) > 0 {
		mcp := make(map[string]json.RawMessage)
		for _, k := range mcpKeys {
			if pluginKey, ok := m.mcpPluginKeys[k]; ok && selectedPlugins[pluginKey] {
				continue
			}
			if raw, ok := m.scanResult.MCP[k]; ok {
				mcp[k] = raw
			} else if raw, ok := m.discoveredMCP[k]; ok {
				mcp[k] = raw
			}
		}
		opts.MCP = mcp
	} else {
		opts.MCP = map[string]json.RawMessage{} // empty = none
	}

	// Hooks: map values from scanResult. Empty map = none.
	hookKeys := m.pickers[SectionHooks].SelectedKeys()
	if len(hookKeys) > 0 {
		hooks := make(map[string]json.RawMessage)
		for _, k := range hookKeys {
			if raw, ok := m.scanResult.Hooks[k]; ok {
				hooks[k] = raw
			}
		}
		opts.IncludeHooks = hooks
	} else {
		opts.IncludeHooks = map[string]json.RawMessage{} // empty = none
	}

	// Keybindings: include entire map if the toggle is selected.
	kbKeys := m.pickers[SectionKeybindings].SelectedKeys()
	if len(kbKeys) > 0 {
		opts.Keybindings = m.scanResult.Keybindings
	}

	// Commands & Skills: split selected keys by type prefix.
	csKeys := m.pickers[SectionCommandsSkills].SelectedKeys()
	for _, k := range csKeys {
		if strings.HasPrefix(k, "cmd:") {
			opts.Commands = append(opts.Commands, k)
		} else if strings.HasPrefix(k, "skill:") {
			opts.Skills = append(opts.Skills, k)
		}
	}

	// Profiles.
	if m.useProfiles && len(m.profilePickers) > 0 {
		opts.Profiles = m.buildProfiles()
	}

	return opts
}

// buildProfiles computes profile diffs relative to base selections.
func (m Model) buildProfiles() map[string]profiles.Profile {
	profs := make(map[string]profiles.Profile)
	for name := range m.profilePickers {
		profs[name] = m.diffsToProfile(name)
	}
	return profs
}

// --- Search handling ---

func (m Model) handleSearchDone(msg SearchDoneMsg) Model {
	// Clear searching state.
	m.preview.searching = false
	for name, pp := range m.profilePreviews {
		pp.searching = false
		m.profilePreviews[name] = pp
	}

	if len(msg.Paths) == 0 {
		return m
	}

	for _, path := range msg.Paths {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		content := string(data)
		sections := claudemd.Split(content)
		if len(sections) == 0 {
			continue
		}

		previewSections := ClaudeMDPreviewSections(sections, shortenPath(path))
		m.preview.AddSections(previewSections)

		// Also add to profile previews.
		for name, pp := range m.profilePreviews {
			pp.AddSections(previewSections)
			m.profilePreviews[name] = pp
		}
	}

	m.syncSidebarCounts()
	m.syncStatusBar()
	return m
}

func (m Model) handleMCPSearchDone(msg MCPSearchDoneMsg) Model {
	// Clear searching state on base and all profile pickers.
	if p, ok := m.pickers[SectionMCP]; ok {
		p.searching = false
		m.pickers[SectionMCP] = p
	}
	for name, pm := range m.profilePickers {
		if p, ok := pm[SectionMCP]; ok {
			p.searching = false
			pm[SectionMCP] = p
		}
		m.profilePickers[name] = pm
	}

	if len(msg.Servers) == 0 {
		return m
	}

	// Store discovered servers and sources.
	for name, cfg := range msg.Servers {
		// Skip servers already in the global scan or already discovered.
		if _, exists := m.scanResult.MCP[name]; exists {
			continue
		}
		if _, exists := m.discoveredMCP[name]; exists {
			continue
		}
		m.discoveredMCP[name] = cfg
		m.mcpSources[name] = msg.Sources[name]
	}

	// Build grouped picker items for newly discovered servers.
	// Use AllKeys to deduplicate against items already in the picker — this
	// prevents the count from growing on repeated searches.
	existingKeys := toSet(m.pickers[SectionMCP].AllKeys())

	// Group by source path, preserving discovery order.
	type group struct {
		source string
		items  []PickerItem
	}
	var groups []group
	sourceIdx := make(map[string]int) // source → index in groups

	// Sort server names for deterministic order.
	serverNames := make([]string, 0, len(msg.Servers))
	for name := range msg.Servers {
		serverNames = append(serverNames, name)
	}
	sort.Strings(serverNames)

	for _, name := range serverNames {
		if _, exists := m.scanResult.MCP[name]; exists {
			continue
		}
		if existingKeys[name] {
			continue
		}

		src := m.mcpSources[name]
		if src == "" {
			src = "project"
		}

		idx, ok := sourceIdx[src]
		if !ok {
			idx = len(groups)
			sourceIdx[src] = idx
			groups = append(groups, group{source: src})
		}

		// Detect if this server comes from a plugin directory.
		item := PickerItem{
			Key:      name,
			Display:  name,
			Selected: true,
		}
		if pluginKey := m.pluginKeyFromSource(src); pluginKey != "" {
			m.mcpPluginKeys[name] = pluginKey
			pluginName := filepath.Base(src)
			item.Tag = "via " + pluginName
		}

		groups[idx].items = append(groups[idx].items, item)
	}

	// Build newItems: header + items per group.
	var newItems []PickerItem
	for _, g := range groups {
		newItems = append(newItems, PickerItem{
			Display:  fmt.Sprintf("%s (%d)", g.source, len(g.items)),
			IsHeader: true,
		})
		newItems = append(newItems, g.items...)
	}

	if len(newItems) == 0 {
		return m
	}

	// Add to base MCP picker.
	if p, ok := m.pickers[SectionMCP]; ok {
		p.AddItems(newItems)
		m.pickers[SectionMCP] = p
	}

	// Add to profile MCP pickers.
	for name, pm := range m.profilePickers {
		if p, ok := pm[SectionMCP]; ok {
			p.AddItems(newItems)
			pm[SectionMCP] = p
		}
		m.profilePickers[name] = pm
	}

	// Apply read-only state for plugin-provided servers.
	m.syncMCPPluginState()

	m.syncSidebarCounts()
	m.syncStatusBar()
	return m
}

func (m Model) handleCmdSkillSearchDone(msg CmdSkillSearchDoneMsg) Model {
	// Clear searching state on base and all profile pickers.
	if p, ok := m.pickers[SectionCommandsSkills]; ok {
		p.searching = false
		m.pickers[SectionCommandsSkills] = p
	}
	for name, pm := range m.profilePickers {
		if p, ok := pm[SectionCommandsSkills]; ok {
			p.searching = false
			pm[SectionCommandsSkills] = p
		}
		m.profilePickers[name] = pm
	}

	if len(msg.Items) == 0 {
		return m
	}

	// Append discovered items to internal list.
	m.discoveredCmdSkills = append(m.discoveredCmdSkills, msg.Items...)

	// Build picker items from the discovered project items.
	var newItems []PickerItem
	existingKeys := toSet(m.pickers[SectionCommandsSkills].AllKeys())

	// Group by project.
	projectGroups := make(map[string][]cmdskill.Item)
	for _, item := range msg.Items {
		if existingKeys[item.Key()] {
			continue
		}
		projectGroups[item.SourceLabel] = append(projectGroups[item.SourceLabel], item)
	}

	projectNames := make([]string, 0, len(projectGroups))
	for name := range projectGroups {
		projectNames = append(projectNames, name)
	}
	sort.Strings(projectNames)

	for _, projName := range projectNames {
		items := projectGroups[projName]
		newItems = append(newItems, PickerItem{
			Display:  fmt.Sprintf("Project: %s (%d)", projName, len(items)),
			IsHeader: true,
		})
		sort.Slice(items, func(i, j int) bool {
			return items[i].Key() < items[j].Key()
		})
		for _, item := range items {
			typeTag := "[cmd]"
			if item.Type == cmdskill.TypeSkill {
				typeTag = "[skill]"
			}
			newItems = append(newItems, PickerItem{
				Key:      item.Key(),
				Display:  item.Name,
				Selected: true,
				Tag:      typeTag,
			})
		}
	}

	if len(newItems) == 0 {
		return m
	}

	// Add to base picker.
	if p, ok := m.pickers[SectionCommandsSkills]; ok {
		p.AddItems(newItems)
		// Also update preview content for the new items.
		for _, item := range msg.Items {
			if p.previewContent != nil {
				p.previewContent[item.Key()] = item.Content
			}
		}
		m.pickers[SectionCommandsSkills] = p
	}

	// Add to profile pickers.
	for name, pm := range m.profilePickers {
		if p, ok := pm[SectionCommandsSkills]; ok {
			p.AddItems(newItems)
			for _, item := range msg.Items {
				if p.previewContent != nil {
					p.previewContent[item.Key()] = item.Content
				}
			}
			pm[SectionCommandsSkills] = p
		}
		m.profilePickers[name] = pm
	}

	m.syncSidebarCounts()
	m.syncStatusBar()
	return m
}

// sortPickerItems sorts items by Key for deterministic order.
func sortPickerItems(items []PickerItem) {
	sort.Slice(items, func(i, j int) bool {
		return items[i].Key < items[j].Key
	})
}

// --- Plugin-aware MCP helpers ---

// pluginKeyFromSource returns the full plugin key if the given shortened source
// path is under ~/.claude/plugins/, or "" otherwise. It matches the directory
// name against scanResult.PluginKeys entries (format: name@marketplace).
func (m Model) pluginKeyFromSource(source string) string {
	if !strings.HasPrefix(source, "~/.claude/plugins/") {
		return ""
	}
	pluginName := filepath.Base(source)
	for _, key := range m.scanResult.PluginKeys {
		if idx := strings.Index(key, "@"); idx >= 0 && key[:idx] == pluginName {
			return key
		}
	}
	return ""
}

// syncMCPPluginState syncs read-only + selected state for plugin-provided MCP
// servers based on the current tab's plugin selections. When a plugin is selected,
// its MCP servers are read-only and auto-selected. When deselected, they become
// normal toggleable items.
func (m *Model) syncMCPPluginState() {
	if len(m.mcpPluginKeys) == 0 {
		return
	}

	selectedPlugins := toSet(m.pickers[SectionPlugins].SelectedKeys())
	if m.activeTab != "Base" {
		if pm, ok := m.profilePickers[m.activeTab]; ok {
			selectedPlugins = toSet(pm[SectionPlugins].SelectedKeys())
		}
	}

	// Update the MCP picker for the current tab.
	var mcpPicker *Picker
	if m.activeTab != "Base" {
		if pm, ok := m.profilePickers[m.activeTab]; ok {
			if p, ok := pm[SectionMCP]; ok {
				mcpPicker = &p
			}
		}
	} else {
		if p, ok := m.pickers[SectionMCP]; ok {
			mcpPicker = &p
		}
	}
	if mcpPicker == nil {
		return
	}

	for i, it := range mcpPicker.items {
		pluginKey, isPluginMCP := m.mcpPluginKeys[it.Key]
		if !isPluginMCP {
			continue
		}
		if selectedPlugins[pluginKey] {
			mcpPicker.items[i].IsReadOnly = true
			mcpPicker.items[i].Selected = true
		} else {
			mcpPicker.items[i].IsReadOnly = false
		}
	}

	// Write back the modified picker.
	if m.activeTab != "Base" {
		if pm, ok := m.profilePickers[m.activeTab]; ok {
			pm[SectionMCP] = *mcpPicker
			m.profilePickers[m.activeTab] = pm
		}
	} else {
		m.pickers[SectionMCP] = *mcpPicker
	}
}

// --- Edit mode helpers ---

// applyPickerSelection sets picker items to match a selected key set.
// Items whose Key is in selectedSet are selected; others are deselected.
func applyPickerSelection(p *Picker, selectedSet map[string]bool) {
	for i, it := range p.items {
		if isSelectableItem(it) {
			p.items[i].Selected = selectedSet[it.Key]
		}
	}
	p.syncSelectAll()
}

// applyExistingConfig pre-populates base picker selections from an existing config.
func (m *Model) applyExistingConfig(cfg *config.Config, existingProfiles map[string]profiles.Profile) {
	// Plugins: select only those in the existing config.
	pluginSet := toSet(cfg.AllPluginKeys())
	applyPickerSelection(pickerPtr(m.pickers, SectionPlugins), pluginSet)

	// Settings: select only those in the existing config.
	settingsSet := mapKeys(cfg.Settings)
	applyPickerSelection(pickerPtr(m.pickers, SectionSettings), settingsSet)

	// Permissions: build key set from allow/deny prefixes.
	permSet := make(map[string]bool)
	for _, rule := range cfg.Permissions.Allow {
		permSet["allow:"+rule] = true
	}
	for _, rule := range cfg.Permissions.Deny {
		permSet["deny:"+rule] = true
	}
	applyPickerSelection(pickerPtr(m.pickers, SectionPermissions), permSet)

	// MCP: select only those in the existing config.
	mcpSet := make(map[string]bool)
	for k := range cfg.MCP {
		mcpSet[k] = true
	}
	applyPickerSelection(pickerPtr(m.pickers, SectionMCP), mcpSet)

	// Hooks: select only those in the existing config.
	hookSet := make(map[string]bool)
	for k := range cfg.Hooks {
		hookSet[k] = true
	}
	applyPickerSelection(pickerPtr(m.pickers, SectionHooks), hookSet)

	// Keybindings: select only if the existing config has keybindings.
	kbSet := mapKeys(cfg.Keybindings)
	applyPickerSelection(pickerPtr(m.pickers, SectionKeybindings), kbSet)

	// CLAUDE.md: select only fragments in the existing config's include list.
	if len(cfg.ClaudeMD.Include) > 0 {
		fragSet := toSet(cfg.ClaudeMD.Include)
		for i, sec := range m.preview.sections {
			m.preview.selected[i] = fragSet[sec.FragmentKey]
		}
	} else {
		// No fragments in config = none selected.
		for i := range m.preview.sections {
			m.preview.selected[i] = false
		}
	}

	// Commands & Skills: select only those in the existing config.
	csSet := make(map[string]bool)
	for _, k := range cfg.Commands {
		csSet[k] = true
	}
	for _, k := range cfg.Skills {
		csSet[k] = true
	}
	applyPickerSelection(pickerPtr(m.pickers, SectionCommandsSkills), csSet)
}

// restoreProfiles creates profile tabs and applies profile-specific selections.
func (m *Model) restoreProfiles(cfg *config.Config, existingProfiles map[string]profiles.Profile) {
	m.useProfiles = true

	// Sort profile names for deterministic order.
	var names []string
	for name := range existingProfiles {
		names = append(names, name)
	}
	sort.Strings(names)

	for _, name := range names {
		profile := existingProfiles[name]

		// createProfile initializes empty diffs and rebuilds from base.
		m.createProfile(name)

		// Populate diffs from the saved profile YAML.
		m.profileDiffs[name] = profileToSectionDiffs(profile)

		// Rebuild all sections with the restored diffs applied.
		m.rebuildAllProfilePickers(name)
	}

	// Switch back to Base tab after restoring all profiles.
	m.activeTab = "Base"
	m.tabBar.SetActive(0)
	m.syncSidebarCounts()
}

// pickerPtr returns a pointer to a picker in a map, allowing in-place mutation.
func pickerPtr(m map[Section]Picker, sec Section) *Picker {
	p := m[sec]
	return &p
}

// mapKeys returns a set of keys from a map.
func mapKeys(m map[string]any) map[string]bool {
	s := make(map[string]bool, len(m))
	for k := range m {
		s[k] = true
	}
	return s
}

// --- Utility functions ---

func hasScanData(scan *commands.InitScanResult) bool {
	return len(scan.PluginKeys) > 0 ||
		len(scan.Settings) > 0 ||
		len(scan.Hooks) > 0 ||
		len(scan.Permissions.Allow) > 0 ||
		len(scan.Permissions.Deny) > 0 ||
		scan.ClaudeMDContent != "" ||
		len(scan.MCP) > 0 ||
		len(scan.Keybindings) > 0 ||
		(scan.CommandsSkills != nil && len(scan.CommandsSkills.Items) > 0)
}

// splitCmdSkillKeys splits a list of keys into command keys and skill keys
// based on their prefix.
func splitCmdSkillKeys(keys []string) (cmds, skills []string) {
	for _, k := range keys {
		if strings.HasPrefix(k, "cmd:") {
			cmds = append(cmds, k)
		} else if strings.HasPrefix(k, "skill:") {
			skills = append(skills, k)
		}
	}
	return cmds, skills
}

func toSet(keys []string) map[string]bool {
	s := make(map[string]bool, len(keys))
	for _, k := range keys {
		s[k] = true
	}
	return s
}

func setsEqual(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k := range a {
		if !b[k] {
			return false
		}
	}
	return true
}

// copyPickerItems creates a deep copy of a picker's items.
func copyPickerItems(p Picker) []PickerItem {
	items := make([]PickerItem, len(p.items))
	copy(items, p.items)
	return items
}

// --- Message extraction helpers ---
// These helpers run a tea.Cmd synchronously to extract the message it produces.
// This is safe because our commands are all simple closures returning a message.

func extractOverlayClose(cmd tea.Cmd) *OverlayCloseMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if m, ok := msg.(OverlayCloseMsg); ok {
		return &m
	}
	return nil
}


func extractSectionSwitch(cmd tea.Cmd) *SectionSwitchMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if m, ok := msg.(SectionSwitchMsg); ok {
		return &m
	}
	return nil
}

func extractFocusChange(cmd tea.Cmd) *FocusChangeMsg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if m, ok := msg.(FocusChangeMsg); ok {
		return &m
	}
	return nil
}

func extractSearchRequest(cmd tea.Cmd) bool {
	if cmd == nil {
		return false
	}
	msg := cmd()
	_, ok := msg.(SearchRequestMsg)
	return ok
}

// Ensure Model satisfies tea.Model at compile time.
var _ tea.Model = Model{}

// Ensure we use all imports. These are used in the file but the compiler
// may not detect some usages through interface methods.
var (
	_ = config.Permissions{}
	_ = fmt.Sprintf
)
