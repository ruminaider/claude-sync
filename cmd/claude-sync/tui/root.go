package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// SkipFlags controls which sections are skipped (deselected) via CLI flags.
type SkipFlags struct {
	Plugins     bool
	Settings    bool
	Hooks       bool
	Permissions bool
	ClaudeMD    bool
	MCP         bool
	Keybindings bool
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

	// Discovered MCP servers from project-level .mcp.json files.
	discoveredMCP map[string]json.RawMessage // server name → config
	mcpSources    map[string]string          // server name → shortened source path

	// Result -- set on save, read by cmd_init.go after TUI exits.
	Result *commands.InitOptions
}

// NewModel creates the root TUI model from scan results.
func NewModel(scan *commands.InitScanResult, claudeDir, syncDir, remoteURL string, skipProfiles bool, skip SkipFlags) Model {
	m := Model{
		scanResult:      scan,
		claudeDir:       claudeDir,
		syncDir:         syncDir,
		remoteURL:       remoteURL,
		pickers:         make(map[Section]Picker),
		profilePickers:  make(map[string]map[Section]Picker),
		profilePreviews: make(map[string]Preview),
		activeTab:       "Base",
		activeSection:   SectionPlugins,
		focusZone:       FocusSidebar,
		skipFlags:       skip,
		discoveredMCP:   make(map[string]json.RawMessage),
		mcpSources:      make(map[string]string),
	}

	// Build pickers for each section from scan data.
	m.pickers[SectionPlugins] = NewPicker(PluginPickerItems(scan))
	m.pickers[SectionSettings] = NewPicker(SettingsPickerItems(scan.Settings))
	m.pickers[SectionPermissions] = NewPicker(PermissionPickerItems(scan.Permissions))
	mcpPicker := NewPicker(MCPPickerItems(scan.MCP))
	mcpPicker.SetSearchAction(true)
	m.pickers[SectionMCP] = mcpPicker
	m.pickers[SectionHooks] = NewPicker(HookPickerItems(scan.Hooks))
	m.pickers[SectionKeybindings] = NewPicker(KeybindingsPickerItems(scan.Keybindings))

	// Build CLAUDE.md preview.
	previewSections := ClaudeMDPreviewSections(scan.ClaudeMDSections, "~/.claude/CLAUDE.md")
	m.preview = NewPreview(previewSections)

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
	m.syncSidebarCounts()
	m.tabBar = NewTabBar(nil)

	// Initialize status bar.
	m.statusBar = NewStatusBar()

	// If not skipping profiles, show the config style choice overlay on first render.
	if !skipProfiles && hasScanData(scan) {
		m.overlay = NewChoiceOverlay("Configuration style", []string{
			"Simple (single config)",
			"With profiles (e.g., work, personal)",
		})
		m.overlayCtx = overlayConfigStyle
	}

	return m
}

// Init satisfies tea.Model.
func (m Model) Init() tea.Cmd { return nil }

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
			// Cycle to next tab (includes [+] stop).
			m.tabBar.CycleNext()
			if !m.tabBar.OnPlus() {
				m.activeTab = m.tabBar.ActiveTab()
				m.syncSidebarCounts()
			}
			m.syncStatusBar()
			return m, nil
		case "shift+tab":
			// Cycle to previous tab (includes [+] stop).
			m.tabBar.CyclePrev()
			if !m.tabBar.OnPlus() {
				m.activeTab = m.tabBar.ActiveTab()
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
	helperView := renderHelper(m.activeSection, isProfile, contentWidth, hLines)

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
		if extractSearchRequest(cmd) && m.activeSection == SectionMCP {
			return m, SearchMCPConfigs()
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
			line1 = "Add or remove plugins relative to the base config."
		case SectionSettings:
			line1 = "Override base settings for this profile."
		case SectionClaudeMD:
			line1 = "Add or exclude CLAUDE.md sections for this profile."
		case SectionPermissions:
			line1 = "Adjust permission rules for this profile."
		case SectionMCP:
			line1 = "Add or remove MCP servers for this profile."
		case SectionKeybindings:
			line1 = "Override keybindings for this profile."
		case SectionHooks:
			line1 = "Add or remove hooks for this profile."
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
		default:
			line1 = "Configure this section."
		}
	}

	// Line 2: keyboard shortcuts (same for base and profile).
	var line2 string
	switch section {
	case SectionClaudeMD:
		line2 = "Space: toggle \u00b7 a: all \u00b7 n: none \u00b7 →: preview content"
	case SectionKeybindings:
		line2 = "Space: toggle"
	default:
		line2 = "Space: toggle \u00b7 a: all \u00b7 n: none"
	}

	return line1, line2
}

// renderHelper renders the helper block above content.
// lines controls the format: 3 = full (desc + shortcuts + sep), 2 = compact (desc + sep), 0 = hidden.
func renderHelper(section Section, isProfile bool, width, lines int) string {
	if lines <= 0 {
		return ""
	}

	line1, line2 := helperText(section, isProfile)

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
	// Initialize profile pickers as copies of base selections.
	pm := make(map[Section]Picker)
	for sec, basePicker := range m.pickers {
		if sec == SectionPlugins {
			// Build profile plugin items showing base vs available.
			baseSelected := basePicker.SelectedKeys()
			items := ProfilePluginPickerItems(m.scanResult, baseSelected)
			pm[sec] = NewPicker(items)
		} else {
			// Copy base picker items.
			items := copyPickerItems(basePicker)
			p := NewPicker(items)
			if basePicker.hasSearchAction {
				p.SetSearchAction(true)
			}
			pm[sec] = p
		}
	}
	m.profilePickers[name] = pm

	// Copy preview sections for profile.
	baseFragKeys := m.preview.SelectedFragmentKeys()
	baseFragSet := toSet(baseFragKeys)
	profileSections := make([]PreviewSection, len(m.preview.sections))
	copy(profileSections, m.preview.sections)
	for i := range profileSections {
		profileSections[i].IsBase = baseFragSet[profileSections[i].FragmentKey]
	}
	pp := NewPreview(profileSections)
	m.profilePreviews[name] = pp

	// Update tab bar and switch to new profile.
	m.tabBar.AddTab(name)
	m.activeTab = name
	m.useProfiles = true
	m.syncSidebarCounts()
	m.syncStatusBar()

	if m.ready {
		m.distributeSize()
	}
}

func (m *Model) deleteProfile(name string) {
	delete(m.profilePickers, name)
	delete(m.profilePreviews, name)
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
	mcpPicker := NewPicker(MCPPickerItems(m.scanResult.MCP))
	mcpPicker.SetSearchAction(true)
	m.pickers[SectionMCP] = mcpPicker
	m.pickers[SectionHooks] = NewPicker(HookPickerItems(m.scanResult.Hooks))
	m.pickers[SectionKeybindings] = NewPicker(KeybindingsPickerItems(m.scanResult.Keybindings))

	// Clear discovered MCP servers on reset.
	m.discoveredMCP = make(map[string]json.RawMessage)
	m.mcpSources = make(map[string]string)

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
	m.overlay = NewSummaryOverlay("Initialize sync config", body)
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
	mcpKeys := m.pickers[SectionMCP].SelectedKeys()
	if len(mcpKeys) > 0 {
		mcp := make(map[string]json.RawMessage)
		for _, k := range mcpKeys {
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

	// Profiles.
	if m.useProfiles && len(m.profilePickers) > 0 {
		opts.Profiles = m.buildProfiles()
	}

	return opts
}

// buildProfiles computes profile diffs relative to base selections.
func (m Model) buildProfiles() map[string]profiles.Profile {
	profs := make(map[string]profiles.Profile)

	basePlugins := m.pickers[SectionPlugins].SelectedKeys()
	basePluginSet := toSet(basePlugins)

	baseSettings := m.pickers[SectionSettings].SelectedKeys()
	baseSettingsSet := toSet(baseSettings)

	basePermKeys := m.pickers[SectionPermissions].SelectedKeys()
	basePermSet := toSet(basePermKeys)

	baseFragKeys := m.preview.SelectedFragmentKeys()
	baseFragSet := toSet(baseFragKeys)

	baseMCPKeys := m.pickers[SectionMCP].SelectedKeys()
	baseMCPSet := toSet(baseMCPKeys)

	baseHookKeys := m.pickers[SectionHooks].SelectedKeys()
	baseHookSet := toSet(baseHookKeys)

	baseKBKeys := m.pickers[SectionKeybindings].SelectedKeys()
	baseKBSet := toSet(baseKBKeys)

	for name, pickerMap := range m.profilePickers {
		p := profiles.Profile{}

		// Plugins diff.
		profPlugins := pickerMap[SectionPlugins].SelectedKeys()
		profPluginSet := toSet(profPlugins)
		for _, k := range profPlugins {
			if !basePluginSet[k] {
				p.Plugins.Add = append(p.Plugins.Add, k)
			}
		}
		for _, k := range basePlugins {
			if !profPluginSet[k] {
				p.Plugins.Remove = append(p.Plugins.Remove, k)
			}
		}

		// Settings diff: profile can override settings values.
		profSettings := pickerMap[SectionSettings].SelectedKeys()
		profSettingsSet := toSet(profSettings)
		// Removed settings (in base but not in profile).
		for _, k := range baseSettings {
			if !profSettingsSet[k] {
				// Can't represent removal in Profile struct; skip.
				_ = k
			}
		}
		// Added settings (in profile but not in base).
		for _, k := range profSettings {
			if !baseSettingsSet[k] {
				// Profile adds a setting not in base.
				if p.Settings == nil {
					p.Settings = make(map[string]any)
				}
				if v, ok := m.scanResult.Settings[k]; ok {
					p.Settings[k] = v
				}
			}
		}

		// Permissions diff.
		profPermKeys := pickerMap[SectionPermissions].SelectedKeys()
		for _, k := range profPermKeys {
			if !basePermSet[k] {
				if strings.HasPrefix(k, "allow:") {
					p.Permissions.AddAllow = append(p.Permissions.AddAllow, strings.TrimPrefix(k, "allow:"))
				} else if strings.HasPrefix(k, "deny:") {
					p.Permissions.AddDeny = append(p.Permissions.AddDeny, strings.TrimPrefix(k, "deny:"))
				}
			}
		}

		// CLAUDE.md diff.
		if profPreview, ok := m.profilePreviews[name]; ok {
			profFragKeys := profPreview.SelectedFragmentKeys()
			profFragSet := toSet(profFragKeys)
			for _, k := range profFragKeys {
				if !baseFragSet[k] {
					p.ClaudeMD.Add = append(p.ClaudeMD.Add, k)
				}
			}
			for _, k := range baseFragKeys {
				if !profFragSet[k] {
					p.ClaudeMD.Remove = append(p.ClaudeMD.Remove, k)
				}
			}
		}

		// MCP diff.
		profMCPKeys := pickerMap[SectionMCP].SelectedKeys()
		profMCPSet := toSet(profMCPKeys)
		mcpAdd := make(map[string]json.RawMessage)
		for _, k := range profMCPKeys {
			if !baseMCPSet[k] {
				if raw, ok := m.scanResult.MCP[k]; ok {
					mcpAdd[k] = raw
				} else if raw, ok := m.discoveredMCP[k]; ok {
					mcpAdd[k] = raw
				}
			}
		}
		var mcpRemoves []string
		for _, k := range baseMCPKeys {
			if !profMCPSet[k] {
				mcpRemoves = append(mcpRemoves, k)
			}
		}
		if len(mcpAdd) > 0 || len(mcpRemoves) > 0 {
			p.MCP = profiles.ProfileMCP{Add: mcpAdd, Remove: mcpRemoves}
		}

		// Hooks diff.
		profHookKeys := pickerMap[SectionHooks].SelectedKeys()
		profHookSet := toSet(profHookKeys)
		hookAdd := make(map[string]json.RawMessage)
		for _, k := range profHookKeys {
			if !baseHookSet[k] {
				if raw, ok := m.scanResult.Hooks[k]; ok {
					hookAdd[k] = raw
				}
			}
		}
		var hookRemoves []string
		for _, k := range baseHookKeys {
			if !profHookSet[k] {
				hookRemoves = append(hookRemoves, k)
			}
		}
		if len(hookAdd) > 0 || len(hookRemoves) > 0 {
			p.Hooks = profiles.ProfileHooks{Add: hookAdd, Remove: hookRemoves}
		}

		// Keybindings diff.
		profKBKeys := pickerMap[SectionKeybindings].SelectedKeys()
		profKBSet := toSet(profKBKeys)
		if !setsEqual(baseKBSet, profKBSet) {
			// If profile has keybindings enabled but base does not, or vice versa.
			if len(profKBKeys) > 0 {
				p.Keybindings = profiles.ProfileKeybindings{
					Override: m.scanResult.Keybindings,
				}
			}
		}

		profs[name] = p
	}

	return profs
}

// --- Search handling ---

func (m Model) handleSearchDone(msg SearchDoneMsg) Model {
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

		previewSections := ClaudeMDPreviewSections(sections, path)
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

	// Build new picker items for the discovered servers.
	var newItems []PickerItem
	for name := range msg.Servers {
		if _, exists := m.scanResult.MCP[name]; exists {
			continue
		}
		// Only add items not already in the picker.
		tag := ""
		if src, ok := m.mcpSources[name]; ok {
			tag = "[" + src + "]"
		}
		newItems = append(newItems, PickerItem{
			Key:      name,
			Display:  name,
			Selected: true,
			Tag:      tag,
		})
	}

	if len(newItems) == 0 {
		return m
	}

	// Sort for deterministic order.
	sortPickerItems(newItems)

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

// --- Utility functions ---

func hasScanData(scan *commands.InitScanResult) bool {
	return len(scan.PluginKeys) > 0 ||
		len(scan.Settings) > 0 ||
		len(scan.Hooks) > 0 ||
		len(scan.Permissions.Allow) > 0 ||
		len(scan.Permissions.Deny) > 0 ||
		scan.ClaudeMDContent != "" ||
		len(scan.MCP) > 0 ||
		len(scan.Keybindings) > 0
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
