package tui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/cmdskill"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testModel creates a Model from a scan result with the overlay dismissed
// so buildInitOptions can be tested directly.
func testModel(scan *commands.InitScanResult) Model {
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, nil, nil)
	// Dismiss any initial overlay so tests can call buildInitOptions.
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone
	return m
}

// fullScan returns a scan result with data in every section.
func fullScan() *commands.InitScanResult {
	return &commands.InitScanResult{
		Upstream:    []string{"a@m", "b@m"},
		AutoForked:  []string{"c@local"},
		Settings:    map[string]any{"model": "opus", "env": "prod"},
		Hooks:       map[string]json.RawMessage{"PreToolUse": json.RawMessage(`[{"hooks":[{"command":"lint"}]}]`)},
		Permissions: config.Permissions{
			Allow: []string{"Bash(git *)"},
			Deny:  []string{"Bash(rm *)"},
		},
		ClaudeMDContent: "## Test\ncontent\n## Colors\nblue",
		ClaudeMDSections: []claudemd.Section{
			{Header: "Test", Content: "## Test\ncontent"},
			{Header: "Colors", Content: "## Colors\nblue"},
		},
		MCP:         map[string]json.RawMessage{"server1": json.RawMessage(`{"url":"http://s1"}`)},
		Keybindings: map[string]any{"ctrl+s": "save"},
		CommandsSkills: &cmdskill.ScanResult{
			Items: []cmdskill.Item{
				{Name: "review-pr", Type: cmdskill.TypeCommand, Source: cmdskill.SourceGlobal, SourceLabel: "global", Content: "# Review PR"},
				{Name: "tdd", Type: cmdskill.TypeSkill, Source: cmdskill.SourceGlobal, SourceLabel: "global", Content: "# TDD"},
			},
		},
	}
}

func TestBuildInitOptions_AllSelected(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	opts := m.buildInitOptions()
	require.NotNil(t, opts)

	// Directories
	assert.Equal(t, "/test/claude", opts.ClaudeDir)
	assert.Equal(t, "/test/sync", opts.SyncDir)

	// Plugins: nil means "all selected" when total > 0
	assert.Nil(t, opts.IncludePlugins)

	// Settings: all selected
	assert.True(t, opts.IncludeSettings)
	assert.Nil(t, opts.SettingsFilter) // nil = all

	// CLAUDE.md: all selected
	assert.True(t, opts.ImportClaudeMD)
	assert.Nil(t, opts.ClaudeMDFragments) // nil = all

	// MCP: all servers present
	require.NotEmpty(t, opts.MCP)
	assert.Contains(t, opts.MCP, "server1")

	// Hooks: all hooks present
	require.NotEmpty(t, opts.IncludeHooks)
	assert.Contains(t, opts.IncludeHooks, "PreToolUse")

	// Keybindings: included
	assert.Equal(t, scan.Keybindings, opts.Keybindings)

	// Permissions
	assert.Contains(t, opts.Permissions.Allow, "Bash(git *)")
	assert.Contains(t, opts.Permissions.Deny, "Bash(rm *)")

	// No profiles
	assert.Nil(t, opts.Profiles)
}

func TestBuildInitOptions_NoneSelected(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		Settings: map[string]any{"model": "opus"},
		MCP:      map[string]json.RawMessage{"server": json.RawMessage(`{}`)},
		Hooks:    map[string]json.RawMessage{"Pre": json.RawMessage(`[{"hooks":[{"command":"cmd"}]}]`)},
		Permissions: config.Permissions{
			Allow: []string{"rule1"},
		},
		Keybindings:      map[string]any{"ctrl+s": "save"},
		ClaudeMDContent:  "## Section\ntext",
		ClaudeMDSections: []claudemd.Section{{Header: "Section", Content: "## Section\ntext"}},
	}
	m := testModel(scan)

	// Deselect everything
	m.deselectPicker(SectionPlugins)
	m.deselectPicker(SectionSettings)
	m.deselectPicker(SectionMCP)
	m.deselectPicker(SectionHooks)
	m.deselectPicker(SectionPermissions)
	m.deselectPicker(SectionKeybindings)
	for i := range m.preview.sections {
		m.preview.selected[i] = false
	}

	opts := m.buildInitOptions()
	require.NotNil(t, opts)

	// Plugins: empty slice (NOT nil) means "none"
	assert.Equal(t, []string{}, opts.IncludePlugins)
	assert.NotNil(t, opts.IncludePlugins)

	// Settings
	assert.False(t, opts.IncludeSettings)

	// CLAUDE.md
	assert.False(t, opts.ImportClaudeMD)

	// MCP: empty map means "none"
	assert.Equal(t, map[string]json.RawMessage{}, opts.MCP)

	// Hooks: empty map means "none"
	assert.Equal(t, map[string]json.RawMessage{}, opts.IncludeHooks)

	// Keybindings: nil when not selected
	assert.Nil(t, opts.Keybindings)

	// Permissions: should be empty
	assert.Empty(t, opts.Permissions.Allow)
	assert.Empty(t, opts.Permissions.Deny)
}

func TestBuildInitOptions_PartialPlugins(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m", "c@m"},
	}
	m := testModel(scan)

	// Deselect one plugin (b@m)
	p := m.pickers[SectionPlugins]
	for i := range p.items {
		if p.items[i].Key == "b@m" {
			p.items[i].Selected = false
		}
	}
	m.pickers[SectionPlugins] = p

	opts := m.buildInitOptions()

	// Partial selection = specific keys (not nil)
	require.NotNil(t, opts.IncludePlugins)
	assert.Contains(t, opts.IncludePlugins, "a@m")
	assert.Contains(t, opts.IncludePlugins, "c@m")
	assert.NotContains(t, opts.IncludePlugins, "b@m")
	assert.Len(t, opts.IncludePlugins, 2)
}

func TestBuildInitOptions_PartialSettings(t *testing.T) {
	scan := &commands.InitScanResult{
		Settings: map[string]any{"model": "opus", "env": "prod", "theme": "dark"},
	}
	m := testModel(scan)

	// Deselect "env" setting
	p := m.pickers[SectionSettings]
	for i := range p.items {
		if p.items[i].Key == "env" {
			p.items[i].Selected = false
		}
	}
	m.pickers[SectionSettings] = p

	opts := m.buildInitOptions()

	assert.True(t, opts.IncludeSettings)
	require.NotNil(t, opts.SettingsFilter)
	assert.Contains(t, opts.SettingsFilter, "model")
	assert.Contains(t, opts.SettingsFilter, "theme")
	assert.NotContains(t, opts.SettingsFilter, "env")
}

func TestBuildInitOptions_PartialClaudeMD(t *testing.T) {
	scan := &commands.InitScanResult{
		ClaudeMDContent: "## A\ntext\n## B\ntext",
		ClaudeMDSections: []claudemd.Section{
			{Header: "A", Content: "## A\ntext"},
			{Header: "B", Content: "## B\ntext"},
		},
	}
	m := testModel(scan)

	// Deselect section B (index 1)
	m.preview.selected[1] = false

	opts := m.buildInitOptions()

	assert.True(t, opts.ImportClaudeMD)
	require.NotNil(t, opts.ClaudeMDFragments)
	assert.Contains(t, opts.ClaudeMDFragments, "a")
	assert.NotContains(t, opts.ClaudeMDFragments, "b")
}

func TestBuildInitOptions_PartialMCP(t *testing.T) {
	scan := &commands.InitScanResult{
		MCP: map[string]json.RawMessage{
			"alpha": json.RawMessage(`{"a":1}`),
			"bravo": json.RawMessage(`{"b":2}`),
		},
	}
	m := testModel(scan)

	// Deselect "bravo"
	p := m.pickers[SectionMCP]
	for i := range p.items {
		if p.items[i].Key == "bravo" {
			p.items[i].Selected = false
		}
	}
	m.pickers[SectionMCP] = p

	opts := m.buildInitOptions()

	require.NotEmpty(t, opts.MCP)
	assert.Contains(t, opts.MCP, "alpha")
	assert.NotContains(t, opts.MCP, "bravo")
}

func TestBuildInitOptions_PartialHooks(t *testing.T) {
	scan := &commands.InitScanResult{
		Hooks: map[string]json.RawMessage{
			"Pre":  json.RawMessage(`[{"hooks":[{"command":"lint"}]}]`),
			"Post": json.RawMessage(`[{"hooks":[{"command":"format"}]}]`),
		},
	}
	m := testModel(scan)

	// Deselect "Post"
	p := m.pickers[SectionHooks]
	for i := range p.items {
		if p.items[i].Key == "Post" {
			p.items[i].Selected = false
		}
	}
	m.pickers[SectionHooks] = p

	opts := m.buildInitOptions()

	require.NotEmpty(t, opts.IncludeHooks)
	assert.Contains(t, opts.IncludeHooks, "Pre")
	assert.NotContains(t, opts.IncludeHooks, "Post")
}

func TestBuildInitOptions_PartialPermissions(t *testing.T) {
	scan := &commands.InitScanResult{
		Permissions: config.Permissions{
			Allow: []string{"rule1", "rule2"},
			Deny:  []string{"deny1"},
		},
	}
	m := testModel(scan)

	// Deselect allow:rule2
	p := m.pickers[SectionPermissions]
	for i := range p.items {
		if p.items[i].Key == "allow:rule2" {
			p.items[i].Selected = false
		}
	}
	m.pickers[SectionPermissions] = p

	opts := m.buildInitOptions()

	assert.Contains(t, opts.Permissions.Allow, "rule1")
	assert.NotContains(t, opts.Permissions.Allow, "rule2")
	assert.Contains(t, opts.Permissions.Deny, "deny1")
}

func TestBuildInitOptions_EmptyScan(t *testing.T) {
	scan := &commands.InitScanResult{}
	m := testModel(scan)

	opts := m.buildInitOptions()
	require.NotNil(t, opts)

	// Everything should be empty/false/nil
	assert.Equal(t, []string{}, opts.IncludePlugins)
	assert.False(t, opts.IncludeSettings)
	assert.False(t, opts.ImportClaudeMD)
	assert.Equal(t, map[string]json.RawMessage{}, opts.MCP)
	assert.Equal(t, map[string]json.RawMessage{}, opts.IncludeHooks)
	assert.Nil(t, opts.Keybindings)
}

// --- SkipFlags tests ---

func TestSkipFlags_Plugins(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m"},
	}
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{
		Plugins: true,
	}, nil, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	assert.Equal(t, 0, m.pickers[SectionPlugins].SelectedCount())
	assert.Equal(t, 2, m.pickers[SectionPlugins].TotalCount())
}

func TestSkipFlags_Settings(t *testing.T) {
	scan := &commands.InitScanResult{
		Settings: map[string]any{"model": "opus"},
	}
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{
		Settings: true,
	}, nil, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	assert.Equal(t, 0, m.pickers[SectionSettings].SelectedCount())
}

func TestSkipFlags_Multiple(t *testing.T) {
	scan := fullScan()
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{
		Plugins:     true,
		Settings:    true,
		Permissions: true,
		MCP:         true,
		Hooks:       true,
		Keybindings: true,
		ClaudeMD:    true,
	}, nil, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	assert.Equal(t, 0, m.pickers[SectionPlugins].SelectedCount())
	assert.Equal(t, 0, m.pickers[SectionSettings].SelectedCount())
	assert.Equal(t, 0, m.pickers[SectionPermissions].SelectedCount())
	assert.Equal(t, 0, m.pickers[SectionMCP].SelectedCount())
	assert.Equal(t, 0, m.pickers[SectionHooks].SelectedCount())
	assert.Equal(t, 0, m.pickers[SectionKeybindings].SelectedCount())
	assert.Equal(t, 0, m.preview.SelectedCount())
}

func TestSkipFlags_VerifyBuildInitOptions(t *testing.T) {
	scan := fullScan()
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{
		Plugins:  true,
		Settings: true,
	}, nil, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	opts := m.buildInitOptions()

	// Plugins should be empty (skipped)
	assert.Equal(t, []string{}, opts.IncludePlugins)
	// Settings should be false (skipped)
	assert.False(t, opts.IncludeSettings)
	// But other sections should still have their data
	assert.True(t, opts.ImportClaudeMD)
	assert.NotEmpty(t, opts.MCP)
	assert.NotEmpty(t, opts.IncludeHooks)
}

// --- Profile tests ---

func TestCreateProfile(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m", "c@m"},
	}
	m := testModel(scan)

	m.createProfile("work")

	assert.True(t, m.useProfiles)
	assert.Equal(t, "work", m.activeTab)
	assert.Contains(t, m.profilePickers, "work")
	assert.Contains(t, m.profilePreviews, "work")
}

func TestCreateProfile_BaseMarkersOnAllSections(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.createProfile("work")

	pm := m.profilePickers["work"]

	// Commands & Skills: selectable items from base should be marked IsBase.
	csPicker := pm[SectionCommandsSkills]
	for _, it := range csPicker.items {
		if isSelectableItem(it) && it.Selected && !it.IsReadOnly {
			assert.True(t, it.IsBase, "%s should be marked as base", it.Key)
		}
	}

	// Settings: selectable items should be marked IsBase.
	settingsPicker := pm[SectionSettings]
	for _, it := range settingsPicker.items {
		if isSelectableItem(it) && it.Selected {
			assert.True(t, it.IsBase, "%s should be marked as base", it.Key)
		}
	}

	// MCP: selectable items should be marked IsBase.
	mcpPicker := pm[SectionMCP]
	for _, it := range mcpPicker.items {
		if isSelectableItem(it) && it.Selected {
			assert.True(t, it.IsBase, "%s should be marked as base", it.Key)
		}
	}
}

func TestDeleteProfile(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
	}
	m := testModel(scan)

	m.createProfile("work")
	assert.True(t, m.useProfiles)

	m.deleteProfile("work")
	assert.False(t, m.useProfiles) // no profiles left
	assert.Equal(t, "Base", m.activeTab)
	assert.NotContains(t, m.profilePickers, "work")
	assert.NotContains(t, m.profilePreviews, "work")
}

func TestBuildProfiles_PluginsDiff(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m", "c@m"},
	}
	m := testModel(scan)

	// Base has all 3 selected
	m.createProfile("work")

	// In the profile, the picker uses ProfilePluginPickerItems.
	// All base items (a@m, b@m, c@m) are in the "Base" section and pre-selected.
	// There are no "Available" items since all plugins are in base.
	// To test diff: deselect one base plugin (simulating removal in profile)
	pm := m.profilePickers["work"]
	pluginPicker := pm[SectionPlugins]
	for i := range pluginPicker.items {
		if pluginPicker.items[i].Key == "b@m" {
			pluginPicker.items[i].Selected = false
		}
	}
	pm[SectionPlugins] = pluginPicker
	m.profilePickers["work"] = pm

	profs := m.buildProfiles()
	require.Contains(t, profs, "work")

	workProf := profs["work"]
	// b@m is in base but not in profile -> should be in Remove
	assert.Contains(t, workProf.Plugins.Remove, "b@m")
	// a@m and c@m are in both -> no add/remove
	assert.NotContains(t, workProf.Plugins.Add, "a@m")
	assert.NotContains(t, workProf.Plugins.Add, "c@m")
}

func TestBuildInitOptions_WithProfiles(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m"},
	}
	m := testModel(scan)

	m.createProfile("work")

	opts := m.buildInitOptions()
	require.NotNil(t, opts.Profiles)
	assert.Contains(t, opts.Profiles, "work")
}

func TestProfilePluginsSyncWithBase(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m", "c@m"},
	}
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	// Create profile "work" — starts with all base selections inherited.
	m.createProfile("work")
	assert.Equal(t, "work", m.activeTab)

	workPlugins := m.profilePickers["work"][SectionPlugins]
	assert.Equal(t, 3, workPlugins.SelectedCount(), "profile should inherit all 3 base plugins")

	// Switch back to Base and deselect "b@m".
	m.saveAllProfileDiffs("work")
	m.activeTab = "Base"
	basePicker := m.pickers[SectionPlugins]
	for i := range basePicker.items {
		if basePicker.items[i].Key == "b@m" {
			basePicker.items[i].Selected = false
		}
	}
	m.pickers[SectionPlugins] = basePicker

	// Switch back to work — should reflect updated base (b@m deselected).
	m.rebuildAllProfilePickers("work")
	m.activeTab = "work"

	workPlugins = m.profilePickers["work"][SectionPlugins]
	assert.Equal(t, 2, workPlugins.SelectedCount(), "profile should drop b@m after base deselected it")

	// Verify inherited tags: a@m and c@m are inherited, b@m is not selected.
	for _, it := range workPlugins.items {
		if it.Key == "a@m" || it.Key == "c@m" {
			assert.True(t, it.IsBase, "%s should be marked inherited", it.Key)
			assert.True(t, it.Selected)
		}
		if it.Key == "b@m" {
			assert.False(t, it.IsBase, "b@m should NOT be marked inherited")
			assert.False(t, it.Selected, "b@m should be deselected")
		}
	}
}

func TestProfilePluginDiff_Preserved(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m", "c@m"},
	}
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	// In the work profile, deselect "a@m" (explicit removal from base).
	pm := m.profilePickers["work"]
	picker := pm[SectionPlugins]
	for i := range picker.items {
		if picker.items[i].Key == "a@m" {
			picker.items[i].Selected = false
		}
	}
	pm[SectionPlugins] = picker
	m.profilePickers["work"] = pm

	// Switch away — saves the diff (removes: a@m).
	m.saveAllProfileDiffs("work")
	diff := m.profileDiffs["work"][SectionPlugins]
	assert.True(t, diff.removes["a@m"], "a@m should be in removes")

	// Switch back — rebuild preserves the removal.
	m.rebuildAllProfilePickers("work")

	workPlugins := m.profilePickers["work"][SectionPlugins]
	for _, it := range workPlugins.items {
		if it.Key == "a@m" {
			assert.False(t, it.Selected, "a@m should stay deselected after rebuild")
		}
		if it.Key == "b@m" || it.Key == "c@m" {
			assert.True(t, it.Selected, "%s should stay selected", it.Key)
		}
	}
}

// --- Nil vs empty semantics ---

func TestNilVsEmptySemantics_Plugins(t *testing.T) {
	// All selected -> nil (meaning "all")
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m"},
	}
	m := testModel(scan)
	opts := m.buildInitOptions()
	assert.Nil(t, opts.IncludePlugins, "all selected should produce nil IncludePlugins")

	// None selected -> empty slice (meaning "none")
	m.deselectPicker(SectionPlugins)
	opts = m.buildInitOptions()
	assert.NotNil(t, opts.IncludePlugins, "none selected should produce non-nil IncludePlugins")
	assert.Empty(t, opts.IncludePlugins, "none selected should produce empty IncludePlugins")

	// Partial -> specific keys
	m2 := testModel(scan)
	p := m2.pickers[SectionPlugins]
	for i := range p.items {
		if p.items[i].Key == "b@m" {
			p.items[i].Selected = false
		}
	}
	m2.pickers[SectionPlugins] = p
	opts = m2.buildInitOptions()
	assert.NotNil(t, opts.IncludePlugins)
	assert.Len(t, opts.IncludePlugins, 1)
}

func TestNilVsEmptySemantics_Settings(t *testing.T) {
	scan := &commands.InitScanResult{
		Settings: map[string]any{"model": "opus", "env": "prod"},
	}

	// All selected -> IncludeSettings=true, SettingsFilter=nil
	m := testModel(scan)
	opts := m.buildInitOptions()
	assert.True(t, opts.IncludeSettings)
	assert.Nil(t, opts.SettingsFilter, "all settings selected should produce nil filter")

	// None selected -> IncludeSettings=false
	m.deselectPicker(SectionSettings)
	opts = m.buildInitOptions()
	assert.False(t, opts.IncludeSettings)
}

func TestNilVsEmptySemantics_MCP(t *testing.T) {
	scan := &commands.InitScanResult{
		MCP: map[string]json.RawMessage{"s1": json.RawMessage(`{}`)},
	}

	// All selected -> non-empty map
	m := testModel(scan)
	opts := m.buildInitOptions()
	assert.NotEmpty(t, opts.MCP)

	// None selected -> empty map (not nil)
	m.deselectPicker(SectionMCP)
	opts = m.buildInitOptions()
	assert.NotNil(t, opts.MCP)
	assert.Empty(t, opts.MCP)
}

func TestNilVsEmptySemantics_Hooks(t *testing.T) {
	scan := &commands.InitScanResult{
		Hooks: map[string]json.RawMessage{"h1": json.RawMessage(`[{"hooks":[{"command":"c"}]}]`)},
	}

	// All selected -> non-empty map
	m := testModel(scan)
	opts := m.buildInitOptions()
	assert.NotEmpty(t, opts.IncludeHooks)

	// None selected -> empty map (not nil)
	m.deselectPicker(SectionHooks)
	opts = m.buildInitOptions()
	assert.NotNil(t, opts.IncludeHooks)
	assert.Empty(t, opts.IncludeHooks)
}

func TestNilVsEmptySemantics_ClaudeMD(t *testing.T) {
	scan := &commands.InitScanResult{
		ClaudeMDContent:  "## A\ntext",
		ClaudeMDSections: []claudemd.Section{{Header: "A", Content: "## A\ntext"}},
	}

	// All selected -> ImportClaudeMD=true, ClaudeMDFragments=nil
	m := testModel(scan)
	opts := m.buildInitOptions()
	assert.True(t, opts.ImportClaudeMD)
	assert.Nil(t, opts.ClaudeMDFragments)

	// None selected -> ImportClaudeMD=false
	for i := range m.preview.sections {
		m.preview.selected[i] = false
	}
	opts = m.buildInitOptions()
	assert.False(t, opts.ImportClaudeMD)
}

// --- Model initialization tests ---

func TestNewModel_InitializesAllPickers(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// Every section should have a picker
	for _, sec := range AllSections {
		if sec == SectionClaudeMD {
			continue // handled by preview
		}
		_, ok := m.pickers[sec]
		assert.True(t, ok, "picker for section %s should exist", sec)
	}
}

func TestNewModel_DefaultState(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
	}
	m := testModel(scan)

	assert.Equal(t, "Base", m.activeTab)
	assert.Equal(t, SectionPlugins, m.activeSection)
	assert.Equal(t, FocusSidebar, m.focusZone)
	assert.False(t, m.quitting)
	assert.Nil(t, m.Result)
}

func TestNewModel_RemoteURL(t *testing.T) {
	scan := &commands.InitScanResult{}
	m := NewModel(scan, "/c", "/s", "https://git.example.com/repo", true, SkipFlags{}, nil, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	opts := m.buildInitOptions()
	assert.Equal(t, "https://git.example.com/repo", opts.RemoteURL)
}

// --- Utility function tests ---

func TestToSet(t *testing.T) {
	s := toSet([]string{"a", "b", "c"})
	assert.True(t, s["a"])
	assert.True(t, s["b"])
	assert.True(t, s["c"])
	assert.False(t, s["d"])
}

func TestToSetEmpty(t *testing.T) {
	s := toSet(nil)
	assert.Empty(t, s)
}

func TestSetsEqual(t *testing.T) {
	a := map[string]bool{"x": true, "y": true}
	b := map[string]bool{"y": true, "x": true}
	assert.True(t, setsEqual(a, b))
}

func TestSetsEqual_Different(t *testing.T) {
	a := map[string]bool{"x": true}
	b := map[string]bool{"y": true}
	assert.False(t, setsEqual(a, b))
}

func TestSetsEqual_DifferentSize(t *testing.T) {
	a := map[string]bool{"x": true}
	b := map[string]bool{"x": true, "y": true}
	assert.False(t, setsEqual(a, b))
}

func TestSetsEqual_BothEmpty(t *testing.T) {
	assert.True(t, setsEqual(map[string]bool{}, map[string]bool{}))
}

func TestCopyPickerItems(t *testing.T) {
	original := NewPicker([]PickerItem{
		{Key: "a", Display: "a", Selected: true},
		{Key: "b", Display: "b", Selected: false},
	})

	copied := copyPickerItems(original)
	require.Len(t, copied, 2)

	// Modify copy should not affect original
	copied[0].Selected = false
	assert.True(t, original.items[0].Selected)
}

func TestHasScanData(t *testing.T) {
	// Empty scan
	assert.False(t, hasScanData(&commands.InitScanResult{}))

	// With plugin keys
	assert.True(t, hasScanData(&commands.InitScanResult{
		PluginKeys: []string{"key"},
	}))

	// With settings
	assert.True(t, hasScanData(&commands.InitScanResult{
		Settings: map[string]any{"k": "v"},
	}))

	// With hooks
	assert.True(t, hasScanData(&commands.InitScanResult{
		Hooks: map[string]json.RawMessage{"h": json.RawMessage(`{}`)},
	}))

	// With allow permissions
	assert.True(t, hasScanData(&commands.InitScanResult{
		Permissions: config.Permissions{Allow: []string{"r"}},
	}))

	// With deny permissions
	assert.True(t, hasScanData(&commands.InitScanResult{
		Permissions: config.Permissions{Deny: []string{"r"}},
	}))

	// With CLAUDE.md content
	assert.True(t, hasScanData(&commands.InitScanResult{
		ClaudeMDContent: "content",
	}))

	// With MCP
	assert.True(t, hasScanData(&commands.InitScanResult{
		MCP: map[string]json.RawMessage{"s": json.RawMessage(`{}`)},
	}))

	// With keybindings
	assert.True(t, hasScanData(&commands.InitScanResult{
		Keybindings: map[string]any{"k": "v"},
	}))
}

// --- resetToDefaults tests ---

func TestResetToDefaults(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// Deselect everything
	m.deselectPicker(SectionPlugins)
	m.deselectPicker(SectionSettings)
	m.deselectPicker(SectionMCP)
	assert.Equal(t, 0, m.pickers[SectionPlugins].SelectedCount())

	// Reset
	m.resetToDefaults()

	// Everything should be back to default (all selected)
	assert.Equal(t, 3, m.pickers[SectionPlugins].SelectedCount()) // 2 upstream + 1 forked
	assert.Equal(t, 2, m.pickers[SectionSettings].SelectedCount())
	assert.Equal(t, 1, m.pickers[SectionMCP].SelectedCount())
}

func TestResetToDefaults_ReappliesSkipFlags(t *testing.T) {
	scan := fullScan()
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{
		Plugins: true,
	}, nil, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	// Plugins should be deselected due to skip flag
	assert.Equal(t, 0, m.pickers[SectionPlugins].SelectedCount())

	// Manually select some plugins
	p := m.pickers[SectionPlugins]
	for i := range p.items {
		if !p.items[i].IsHeader {
			p.items[i].Selected = true
		}
	}
	m.pickers[SectionPlugins] = p
	assert.Greater(t, m.pickers[SectionPlugins].SelectedCount(), 0)

	// Reset should reapply the skip flag
	m.resetToDefaults()
	assert.Equal(t, 0, m.pickers[SectionPlugins].SelectedCount())
}

// --- deselectPicker tests ---

func TestDeselectPicker(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m"},
	}
	m := testModel(scan)

	assert.Equal(t, 2, m.pickers[SectionPlugins].SelectedCount())

	m.deselectPicker(SectionPlugins)

	assert.Equal(t, 0, m.pickers[SectionPlugins].SelectedCount())
	// Total should remain unchanged
	assert.Equal(t, 2, m.pickers[SectionPlugins].TotalCount())
}

func TestProfileCreationFlow_ViewHasSidebarAndTabBar(t *testing.T) {
	scan := fullScan()
	// Create model WITH the config style overlay (skipProfiles=false).
	m := NewModel(scan, "/test/claude", "/test/sync", "", false, SkipFlags{}, nil, nil)

	// Simulate WindowSizeMsg.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = result.(Model)

	// At this point the config style overlay should be active.
	require.True(t, m.overlay.Active())
	require.Equal(t, overlayConfigStyle, m.overlayCtx)

	// Simulate user choosing "With profiles" (second option, enter).
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	// Should now show profile list overlay (batch input).
	require.True(t, m.overlay.Active())
	require.Equal(t, overlayProfileNames, m.overlayCtx)
	require.Equal(t, OverlayProfileList, m.overlay.overlayType)

	// Type "work" into first field.
	for _, ch := range "work" {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = result.(Model)
	}
	// Hit Enter to add second row.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)
	require.Len(t, m.overlay.inputs, 2)

	// Type "personal" into second field.
	for _, ch := range "personal" {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = result.(Model)
	}

	// Navigate Down to Done button.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	require.Equal(t, len(m.overlay.inputs), m.overlay.activeLine)

	// Press Enter on Done.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	// Overlay should be dismissed.
	assert.False(t, m.overlay.Active(), "overlay should be closed after profile creation")
	assert.True(t, m.useProfiles, "useProfiles should be true")
	// After batch creation, active tab should be "Base" (not last profile).
	assert.Equal(t, "Base", m.activeTab)

	// Verify tab bar contains all expected tabs.
	assert.Equal(t, []string{"Base", "work", "personal"}, m.tabBar.tabs)

	// Verify profiles were created.
	assert.Contains(t, m.profilePickers, "work")
	assert.Contains(t, m.profilePickers, "personal")
	assert.Contains(t, m.profilePreviews, "work")
	assert.Contains(t, m.profilePreviews, "personal")

	// Render the view and verify basic structure.
	view := m.View()

	// The view should contain sidebar section names.
	assert.Contains(t, view, "Plugins", "view should contain Plugins in sidebar")
	assert.Contains(t, view, "Settings", "view should contain Settings in sidebar")

	// View should have exactly m.height lines (no overflow).
	lines := strings.Split(view, "\n")
	t.Logf("View total lines: %d, expected: %d", len(lines), 40)
	for i, line := range lines {
		w := lipgloss.Width(line)
		t.Logf("Line %2d: width=%3d %q", i, w, line)
	}
	assert.Equal(t, 40, len(lines), "view line count must equal terminal height")

	// First line should contain tab names (tab bar).
	firstLine := lines[0]
	assert.Contains(t, firstLine, "Base", "first line should contain Base tab")
}

func TestFullFlowThenNavigation(t *testing.T) {
	scan := fullScan()
	m := NewModel(scan, "/test/claude", "/test/sync", "", false, SkipFlags{}, nil, nil)

	// WindowSizeMsg.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = result.(Model)

	// Choose "With profiles" (down + enter).
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	// Type "dev" + Enter to add second row.
	for _, ch := range "dev" {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = result.(Model)
	}
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	// Type "staging" in second row.
	for _, ch := range "staging" {
		result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{ch}})
		m = result.(Model)
	}

	// Down to Done, Enter.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = result.(Model)

	// --- Post-overlay state assertions ---
	require.False(t, m.overlay.Active(), "overlay should be closed")
	require.True(t, m.useProfiles)
	require.Equal(t, "Base", m.activeTab)
	require.Equal(t, FocusSidebar, m.focusZone)
	require.Equal(t, []string{"Base", "dev", "staging"}, m.tabBar.tabs)

	// --- Tab forward: Base → dev → staging → [+] → Base ---
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "dev", m.activeTab)

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "staging", m.activeTab)

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.True(t, m.tabBar.OnPlus(), "should land on [+]")

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "Base", m.activeTab, "should wrap back to Base")

	// --- Shift+Tab backward: Base → [+] → staging → dev → Base ---
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.True(t, m.tabBar.OnPlus(), "should land on [+]")

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.Equal(t, "staging", m.activeTab)

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.Equal(t, "dev", m.activeTab)

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.Equal(t, "Base", m.activeTab)

	// --- Right arrow: sidebar → content ---
	assert.Equal(t, FocusSidebar, m.focusZone)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = result.(Model)
	assert.Equal(t, FocusContent, m.focusZone, "right should move to content")

	// --- Left arrow: content → sidebar ---
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(Model)
	assert.Equal(t, FocusSidebar, m.focusZone, "left should return to sidebar")

	// --- Tab works from content zone too ---
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRight})
	m = result.(Model)
	assert.Equal(t, FocusContent, m.focusZone)

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "dev", m.activeTab, "Tab should work from content zone")

	// --- Sidebar up/down still works after tab cycling ---
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(Model)
	assert.Equal(t, FocusSidebar, m.focusZone)

	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	assert.Equal(t, SectionSettings, m.activeSection, "sidebar navigation should work")

	// --- Verify view renders correctly ---
	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 40, len(lines), "view line count must match terminal height")
	// First line should have both sidebar and tab bar content.
	assert.Contains(t, lines[0], "Base", "tab bar should be on first line")
}

func TestDeselectPicker_HeadersUnaffected(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"a@m"},
		AutoForked: []string{"b@local"},
	}
	m := testModel(scan)

	m.deselectPicker(SectionPlugins)

	// Headers should still have IsHeader=true
	p := m.pickers[SectionPlugins]
	for _, item := range p.items {
		if item.IsHeader {
			assert.False(t, item.Selected) // headers don't have Selected = true
		}
	}
}

// --- Navigation tests ---

func testModelWithProfiles(t *testing.T) Model {
	t.Helper()
	scan := fullScan()
	m := testModel(scan)

	m.createProfile("work")
	m.createProfile("personal")
	m.activeTab = "Base"
	m.tabBar.SetActive(0)

	// Simulate window size.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	return result.(Model)
}

func TestTabCyclesProfiles(t *testing.T) {
	m := testModelWithProfiles(t)
	assert.Equal(t, "Base", m.activeTab)

	// Tab → work
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "work", m.activeTab)
	assert.False(t, m.tabBar.OnPlus())

	// Tab → personal
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "personal", m.activeTab)
	assert.False(t, m.tabBar.OnPlus())

	// Tab → [+] (content stays on personal)
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "personal", m.activeTab)
	assert.True(t, m.tabBar.OnPlus())

	// Tab → wraps to Base
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "Base", m.activeTab)
	assert.False(t, m.tabBar.OnPlus())
}

func TestShiftTabCyclesProfilesBackward(t *testing.T) {
	m := testModelWithProfiles(t)
	assert.Equal(t, "Base", m.activeTab)

	// Shift+Tab → [+] (content stays on Base, active moves to last real tab)
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.Equal(t, "Base", m.activeTab) // content unchanged
	assert.True(t, m.tabBar.OnPlus())

	// Shift+Tab → personal (leaves [+])
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.Equal(t, "personal", m.activeTab)
	assert.False(t, m.tabBar.OnPlus())

	// Shift+Tab → work
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.Equal(t, "work", m.activeTab)

	// Shift+Tab → Base
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyShiftTab})
	m = result.(Model)
	assert.Equal(t, "Base", m.activeTab)
}

func TestLeftFromContentGoesToSidebar(t *testing.T) {
	m := testModelWithProfiles(t)
	m.focusZone = FocusContent

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyLeft})
	m = result.(Model)
	assert.Equal(t, FocusSidebar, m.focusZone)
}

func TestEscFromContentGoesToSidebar(t *testing.T) {
	m := testModelWithProfiles(t)
	m.focusZone = FocusContent

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(Model)
	assert.Equal(t, FocusSidebar, m.focusZone)
}

func TestGlobalPlusOpensOverlay(t *testing.T) {
	m := testModelWithProfiles(t)
	m.focusZone = FocusSidebar

	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'+'}})
	m = result.(Model)
	assert.True(t, m.overlay.Active())
	assert.Equal(t, overlayProfileName, m.overlayCtx)
}

func TestGlobalCtrlDDeletesProfile(t *testing.T) {
	m := testModelWithProfiles(t)
	// Switch to "work" tab first.
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyTab})
	m = result.(Model)
	assert.Equal(t, "work", m.activeTab)

	// Ctrl+D should open delete confirmation.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = result.(Model)
	assert.True(t, m.overlay.Active())
	assert.Equal(t, overlayDeleteConfirm, m.overlayCtx)
	assert.Equal(t, "work", m.pendingDeleteTab)
}

func TestGlobalCtrlDIgnoredOnBase(t *testing.T) {
	m := testModelWithProfiles(t)
	assert.Equal(t, "Base", m.activeTab)

	// Ctrl+D on Base should do nothing.
	result, _ := m.Update(tea.KeyMsg{Type: tea.KeyCtrlD})
	m = result.(Model)
	assert.False(t, m.overlay.Active())
}

func TestTabBarInRightPane(t *testing.T) {
	m := testModelWithProfiles(t)
	view := m.View()

	lines := strings.Split(view, "\n")
	require.True(t, len(lines) > 0)

	// First line should contain sidebar content AND tab bar content.
	// The tab bar should NOT span from column 0.
	firstLine := lines[0]
	assert.Contains(t, firstLine, "Base", "first line should contain Base tab")

	// The sidebar portion of the first line should contain "Plugins" (first section).
	assert.Contains(t, firstLine, "Plugins", "first line should also contain sidebar section")
}

// --- Help overlay tests ---

func TestHelpOverlayOpensOnQuestionMark(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = result.(Model)

	// Press ? from sidebar.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(Model)

	assert.True(t, m.overlay.Active(), "help overlay should be active")
	assert.Equal(t, overlayHelp, m.overlayCtx)
	assert.Equal(t, OverlayHelp, m.overlay.overlayType)
}

func TestHelpOverlayOpensOnH_Sidebar(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = result.(Model)

	assert.Equal(t, FocusSidebar, m.focusZone)

	// Press h from sidebar should open help.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = result.(Model)

	assert.True(t, m.overlay.Active(), "h from sidebar should open help")
	assert.Equal(t, overlayHelp, m.overlayCtx)
}

func TestHKeyInContentDoesNotOpenHelp(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = result.(Model)

	// Move to content zone.
	m.focusZone = FocusContent

	// Press h from content should NOT open help (it should go to sidebar via picker/left).
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'h'}})
	m = result.(Model)

	assert.False(t, m.overlay.Active(), "h from content should not open help")
}

func TestHelpOverlayClosesOnEsc(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = result.(Model)

	// Open help.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(Model)
	require.True(t, m.overlay.Active())

	// Esc closes help.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = result.(Model)

	assert.False(t, m.overlay.Active(), "Esc should close help")
	assert.Equal(t, overlayNone, m.overlayCtx)
}

func TestHelpOverlayScrolls(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	// Use a small height to force scrolling.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 20})
	m = result.(Model)

	// Open help.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'?'}})
	m = result.(Model)
	require.True(t, m.overlay.Active())

	// Initial scroll offset should be 0.
	assert.Equal(t, 0, m.overlay.scrollOffset)

	// Scroll down.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = result.(Model)
	assert.Equal(t, 1, m.overlay.scrollOffset, "down should increment scrollOffset")

	// Scroll back up.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = result.(Model)
	assert.Equal(t, 0, m.overlay.scrollOffset, "up should decrement scrollOffset")

	// Can't scroll above 0.
	result, _ = m.Update(tea.KeyMsg{Type: tea.KeyUp})
	m = result.(Model)
	assert.Equal(t, 0, m.overlay.scrollOffset, "should clamp at 0")
}

// --- Helper text tests ---

func TestHelperTextPerSection(t *testing.T) {
	for _, sec := range AllSections {
		line1, line2 := helperText(sec, false, 0)
		assert.NotEmpty(t, line1, "section %s should have line1", sec)
		assert.NotEmpty(t, line2, "section %s should have line2", sec)
	}
}

func TestHelperTextProfileVsBase(t *testing.T) {
	for _, sec := range AllSections {
		baseLine1, _ := helperText(sec, false, 0)
		profLine1, _ := helperText(sec, true, 0)
		assert.NotEqual(t, baseLine1, profLine1,
			"section %s: profile text should differ from base text", sec)
	}
}

func TestViewLineCountWithHelper(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// Set window size.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	m = result.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 40, len(lines), "view line count must equal terminal height")
}

func TestStatusBarContainsHelpShortcut(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)
	sb.Update(SelectionSummary{Selected: 2, Total: 5, Section: SectionPlugins}, "Base")
	view := sb.View()
	assert.Contains(t, view, "help", "status bar should show ?: help")
}

// --- Responsive breakpoint tests ---

func TestStatusBarTier_Full(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(120)
	sb.Update(SelectionSummary{Selected: 2, Total: 5, Section: SectionPlugins}, "Base")
	view := sb.View()
	// Full tier (>= 110) includes "Ctrl+S" and all shortcuts.
	assert.Contains(t, view, "Ctrl+S", "full tier should use Ctrl+S")
	assert.Contains(t, view, "profiles", "full tier should show profiles shortcut")
	assert.Contains(t, view, "search", "full tier should show search shortcut")
}

func TestStatusBarTier_Compact(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(85)
	sb.Update(SelectionSummary{Selected: 2, Total: 5, Section: SectionPlugins}, "Base")
	view := sb.View()
	// Compact tier (>= 80, < 110) shows only essential shortcuts.
	assert.Contains(t, view, "save", "compact tier should show save")
	assert.Contains(t, view, "help", "compact tier should show help")
	assert.Contains(t, view, "quit", "compact tier should show quit")
	assert.NotContains(t, view, "Ctrl+S", "compact tier should NOT use Ctrl+S")
	assert.NotContains(t, view, "profiles", "compact tier should NOT show profiles")
	assert.NotContains(t, view, "search", "compact tier should NOT show search")
}

func TestStatusBarTier_Minimal(t *testing.T) {
	sb := NewStatusBar()
	sb.SetWidth(60)
	sb.Update(SelectionSummary{Selected: 2, Total: 5, Section: SectionPlugins}, "Base")
	view := sb.View()
	// Minimal tier (< 80) shows only quit.
	assert.Contains(t, view, "quit", "minimal tier should show quit")
	assert.NotContains(t, view, "save", "minimal tier should NOT show save")
	assert.NotContains(t, view, "profiles", "minimal tier should NOT show profiles")
}

func TestHelperLines_Full(t *testing.T) {
	assert.Equal(t, 3, helperLines(30), "height 30 should give 3 helper lines")
	assert.Equal(t, 3, helperLines(28), "height 28 should give 3 helper lines")
}

func TestHelperLines_Compact(t *testing.T) {
	assert.Equal(t, 2, helperLines(27), "height 27 should give 2 helper lines")
	assert.Equal(t, 2, helperLines(24), "height 24 should give 2 helper lines")
}

func TestHelperLines_Hidden(t *testing.T) {
	assert.Equal(t, 0, helperLines(23), "height 23 should give 0 helper lines")
	assert.Equal(t, 0, helperLines(15), "height 15 should give 0 helper lines")
}

func TestViewLineCount_SmallTerminal(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// 80x24: compact helper (2 lines), abbreviated status bar.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 80, Height: 24})
	m = result.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 24, len(lines), "view line count must equal terminal height at 80x24")
}

func TestViewLineCount_TinyTerminal(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// 60x20: no helper, minimal status bar.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 60, Height: 20})
	m = result.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 20, len(lines), "view line count must equal terminal height at 60x20")
}

func TestViewLineCount_LargeTerminal(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// 120x40: full helper, full status bar.
	result, _ := m.Update(tea.WindowSizeMsg{Width: 120, Height: 40})
	m = result.(Model)

	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 40, len(lines), "view line count must equal terminal height at 120x40")
}

func TestRenderHelper_Compact(t *testing.T) {
	result := renderHelper(SectionPlugins, false, 60, 2, "", 0)
	// Should contain description but NOT shortcuts line.
	assert.Contains(t, result, "Choose which Claude plugins to sync")
	assert.NotContains(t, result, "Space: toggle")
	// Should contain separator.
	assert.Contains(t, result, "─")
}

func TestRenderHelper_Hidden(t *testing.T) {
	result := renderHelper(SectionPlugins, false, 60, 0, "", 0)
	assert.Equal(t, "", result, "0 lines should produce empty string")
}

// TestViewLineCount_ManyPermissions verifies that sections with many items
// (which fill the picker viewport and trigger scroll indicators) still produce
// the correct total line count. This catches the trailing-newline overflow bug
// where ContentPaneStyle.Render added an extra blank line.
func TestViewLineCount_ManyPermissions(t *testing.T) {
	scan := fullScan()
	// Add many permission rules to fill the viewport.
	scan.Permissions.Allow = make([]string, 30)
	for i := range scan.Permissions.Allow {
		scan.Permissions.Allow[i] = fmt.Sprintf("Bash(cmd%d *)", i)
	}
	m := testModel(scan)

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = result.(Model)

	// Switch to permissions section.
	m.activeSection = SectionPermissions
	m.syncSidebarCounts()
	m.syncStatusBar()

	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 30, len(lines), "view line count must equal terminal height even with many permissions")
}

// TestViewLineCount_ClaudeMD verifies the CLAUDE.md preview section doesn't
// overflow due to the divider trailing newline.
func TestViewLineCount_ClaudeMD(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	result, _ := m.Update(tea.WindowSizeMsg{Width: 100, Height: 30})
	m = result.(Model)

	// Switch to CLAUDE.md section.
	m.activeSection = SectionClaudeMD
	m.syncSidebarCounts()
	m.syncStatusBar()

	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 30, len(lines), "view line count must equal terminal height on CLAUDE.md section")
}

// TestViewLineCount_ClaudeMD_Narrow verifies the CLAUDE.md section on a narrow
// terminal. The CLAUDE.md helper has the longest shortcuts line ("Space: toggle
// · a: all · n: none · →: preview content") which previously wrapped when
// Width(width) was applied, adding an extra line and clipping the tab bar.
func TestViewLineCount_ClaudeMD_Narrow(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// 70 cols: contentWidth = 70 - 22 - 2 = 46, which is less than
	// the CLAUDE.md shortcuts line length (~54 chars).
	result, _ := m.Update(tea.WindowSizeMsg{Width: 70, Height: 30})
	m = result.(Model)

	m.activeSection = SectionClaudeMD
	m.syncSidebarCounts()
	m.syncStatusBar()

	view := m.View()
	lines := strings.Split(view, "\n")
	assert.Equal(t, 30, len(lines), "view line count must equal terminal height on narrow CLAUDE.md")
}

// TestRenderHelper_NoWrap verifies that helper text doesn't wrap to extra lines
// even when the text is longer than the available width.
func TestRenderHelper_NoWrap(t *testing.T) {
	// CLAUDE.md has the longest line2. Render at a narrow width.
	result := renderHelper(SectionClaudeMD, false, 30, 3, "", 0)
	lineCount := strings.Count(result, "\n")
	assert.Equal(t, 3, lineCount, "helper must produce exactly 3 newlines (3 lines) even at narrow width")
}

// --- MCP discovery tests ---

func TestMCPAlwaysNavigable(t *testing.T) {
	// Scan with NO global MCP servers.
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
	}
	m := testModel(scan)

	// MCP should be navigable even with 0 servers.
	for _, entry := range m.sidebar.sections {
		if entry.Section == SectionMCP {
			assert.True(t, entry.Available, "MCP section should be available even with 0 servers")
			return
		}
	}
	t.Fatal("MCP section not found in sidebar")
}

func TestMCPSearchDoneMsg(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"global-server": json.RawMessage(`{"url":"http://g"}`)},
	}
	m := testModel(scan)

	// Simulate MCP search returning discovered servers.
	msg := MCPSearchDoneMsg{
		Servers: map[string]json.RawMessage{
			"project-server": json.RawMessage(`{"url":"http://p"}`),
			"global-server":  json.RawMessage(`{"url":"http://g2"}`), // duplicate
		},
		Sources: map[string]string{
			"project-server": "~/Repos/myproject",
			"global-server":  "~/Repos/other",
		},
	}
	m = m.handleMCPSearchDone(msg)

	// Discovered server should be in the map.
	assert.Contains(t, m.discoveredMCP, "project-server")
	assert.Equal(t, "~/Repos/myproject", m.mcpSources["project-server"])

	// Global server should NOT be in discovered (it's in scanResult).
	assert.NotContains(t, m.discoveredMCP, "global-server")

	// Picker should now have 2 selectable items (global + discovered).
	p := m.pickers[SectionMCP]
	assert.Equal(t, 2, p.TotalCount(), "picker should have global + discovered server")

	// Sidebar count should reflect the new total.
	for _, entry := range m.sidebar.sections {
		if entry.Section == SectionMCP {
			assert.Equal(t, 2, entry.Total)
			assert.Equal(t, 2, entry.Selected)
			return
		}
	}
}

func TestMCPSearchDoneMsg_NoDuplicateOnSecondSearch(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"global-server": json.RawMessage(`{"url":"http://g"}`)},
	}
	m := testModel(scan)

	msg := MCPSearchDoneMsg{
		Servers: map[string]json.RawMessage{
			"project-server": json.RawMessage(`{"url":"http://p"}`),
		},
		Sources: map[string]string{
			"project-server": "~/Repos/myproject",
		},
	}

	// First search adds the server.
	m = m.handleMCPSearchDone(msg)
	countAfterFirst := m.pickers[SectionMCP].TotalCount()
	assert.Equal(t, 2, countAfterFirst, "should have global + discovered")

	// Second search with the same server should NOT change the count.
	m = m.handleMCPSearchDone(msg)
	countAfterSecond := m.pickers[SectionMCP].TotalCount()
	assert.Equal(t, countAfterFirst, countAfterSecond,
		"second search should not add duplicates")
}

func TestMCPSearchDoneMsg_GroupedBySource(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"global-server": json.RawMessage(`{}`)},
	}
	m := testModel(scan)

	msg := MCPSearchDoneMsg{
		Servers: map[string]json.RawMessage{
			"srv-a": json.RawMessage(`{}`),
			"srv-b": json.RawMessage(`{}`),
			"srv-c": json.RawMessage(`{}`),
		},
		Sources: map[string]string{
			"srv-a": "~/Work/proj1",
			"srv-b": "~/Work/proj1",
			"srv-c": "~/Work/proj2",
		},
	}
	m = m.handleMCPSearchDone(msg)

	p := m.pickers[SectionMCP]

	// Count headers: should have initial global header + 2 project headers.
	var headers []string
	for _, it := range p.items {
		if it.IsHeader {
			headers = append(headers, it.Display)
		}
	}
	assert.Len(t, headers, 3, "should have 3 headers: global + 2 project groups")

	// Selectable items: 1 global + 3 discovered = 4.
	assert.Equal(t, 4, p.TotalCount())

	// Verify project headers contain counts.
	foundProj1, foundProj2 := false, false
	for _, h := range headers {
		if strings.Contains(h, "proj1") && strings.Contains(h, "(2)") {
			foundProj1 = true
		}
		if strings.Contains(h, "proj2") && strings.Contains(h, "(1)") {
			foundProj2 = true
		}
	}
	assert.True(t, foundProj1, "should have proj1 header with count 2")
	assert.True(t, foundProj2, "should have proj2 header with count 1")
}

func TestBuildInitOptions_WithDiscoveredMCP(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"global": json.RawMessage(`{"url":"http://g"}`)},
	}
	m := testModel(scan)

	// Add discovered MCP server.
	m.discoveredMCP["project-mcp"] = json.RawMessage(`{"url":"http://p"}`)
	m.mcpSources["project-mcp"] = "~/Repos/proj"

	// Add to picker.
	p := m.pickers[SectionMCP]
	p.AddItems([]PickerItem{
		{Key: "project-mcp", Display: "project-mcp", Selected: true, Tag: "[~/Repos/proj]"},
	})
	m.pickers[SectionMCP] = p

	opts := m.buildInitOptions()

	// Both global and discovered should be in the final MCP map.
	require.NotEmpty(t, opts.MCP)
	assert.Contains(t, opts.MCP, "global")
	assert.Contains(t, opts.MCP, "project-mcp")
}

func TestBuildInitOptions_DiscoveredMCPSecretsStripped(t *testing.T) {
	// Global MCP has no secrets; discovered (project-level) MCP has secrets.
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"safe-server": json.RawMessage(`{"url":"http://s"}`)},
	}
	m := testModel(scan)

	// Add discovered MCP server with a hardcoded secret.
	m.discoveredMCP["slack"] = json.RawMessage(`{"command":"npx","env":{"SLACK_TOKEN":"xoxb-secret-value","APP_NAME":"my-app"}}`)
	m.mcpSources["slack"] = "~/Work/evvy"

	// Add to picker.
	p := m.pickers[SectionMCP]
	p.AddItems([]PickerItem{
		{Key: "slack", Display: "slack", Selected: true, Tag: "[~/Work/evvy]"},
	})
	m.pickers[SectionMCP] = p

	opts := m.buildInitOptions()

	require.Contains(t, opts.MCP, "slack")

	// Secret should be replaced with ${VAR} reference.
	var cfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(opts.MCP["slack"], &cfg))
	assert.Equal(t, "${SLACK_TOKEN}", cfg.Env["SLACK_TOKEN"], "discovered MCP secret should be replaced")
	assert.Equal(t, "my-app", cfg.Env["APP_NAME"], "non-secret should be untouched")
}

func TestMCPSearchDoneMsg_Empty(t *testing.T) {
	scan := &commands.InitScanResult{Upstream: []string{"a@m"}}
	m := testModel(scan)
	originalCount := m.pickers[SectionMCP].TotalCount()

	// Empty search results should be a no-op.
	m = m.handleMCPSearchDone(MCPSearchDoneMsg{})
	assert.Equal(t, originalCount, m.pickers[SectionMCP].TotalCount())
}

func TestMCPPickerHasSearchAction(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"s1": json.RawMessage(`{}`)},
	}
	m := testModel(scan)

	p := m.pickers[SectionMCP]
	assert.True(t, p.hasSearchAction, "MCP picker should have search action enabled")
}

func TestResetToDefaults_ClearsDiscoveredMCP(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)

	// Simulate discovered MCP.
	m.discoveredMCP["found"] = json.RawMessage(`{}`)
	m.mcpSources["found"] = "~/test"

	m.resetToDefaults()

	assert.Empty(t, m.discoveredMCP, "discovered MCP should be cleared on reset")
	assert.Empty(t, m.mcpSources, "MCP sources should be cleared on reset")
	assert.Empty(t, m.mcpPluginKeys, "mcpPluginKeys should be cleared on reset")
	assert.True(t, m.pickers[SectionMCP].hasSearchAction, "search action should be preserved on reset")
}

// --- Plugin-aware MCP tests ---

func TestPluginKeyFromSource(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"figma-minimal@figma-marketplace"},
		PluginKeys: []string{"figma-minimal@figma-marketplace"},
	}
	m := testModel(scan)

	// Plugin path should resolve to full key.
	assert.Equal(t, "figma-minimal@figma-marketplace",
		m.pluginKeyFromSource("~/.claude/plugins/figma-minimal"))

	// Non-plugin path should return "".
	assert.Equal(t, "", m.pluginKeyFromSource("~/Repos/myproject"))

	// Different plugin name should not match.
	assert.Equal(t, "", m.pluginKeyFromSource("~/.claude/plugins/unknown-plugin"))

	// Path without the prefix should return "".
	assert.Equal(t, "", m.pluginKeyFromSource("/absolute/path/figma-minimal"))
}

func TestMCPPluginServers_ReadOnlyWhenPluginSelected(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"figma-minimal@figma-marketplace"},
		PluginKeys: []string{"figma-minimal@figma-marketplace"},
		MCP:        map[string]json.RawMessage{},
	}
	m := testModel(scan)

	// Simulate MCP search discovering a server from the plugin directory.
	msg := MCPSearchDoneMsg{
		Servers: map[string]json.RawMessage{
			"figma-mcp": json.RawMessage(`{"url":"http://figma"}`),
		},
		Sources: map[string]string{
			"figma-mcp": "~/.claude/plugins/figma-minimal",
		},
	}
	m = m.handleMCPSearchDone(msg)

	// The server should be mapped to the plugin key.
	assert.Equal(t, "figma-minimal@figma-marketplace", m.mcpPluginKeys["figma-mcp"])

	// Plugin is selected → server should be read-only and selected.
	p := m.pickers[SectionMCP]
	for _, it := range p.items {
		if it.Key == "figma-mcp" {
			assert.True(t, it.IsReadOnly, "plugin-provided MCP should be read-only when plugin is selected")
			assert.True(t, it.Selected, "plugin-provided MCP should be selected when plugin is selected")
			assert.Equal(t, "via figma-minimal", it.ProviderTag, "should have plugin provider tag")
			return
		}
	}
	t.Fatal("figma-mcp item not found in picker")
}

func TestMCPPluginServers_SelectableWhenPluginDeselected(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"figma-minimal@figma-marketplace"},
		PluginKeys: []string{"figma-minimal@figma-marketplace"},
		MCP:        map[string]json.RawMessage{},
	}
	m := testModel(scan)

	// Simulate MCP search discovering a plugin server.
	msg := MCPSearchDoneMsg{
		Servers: map[string]json.RawMessage{
			"figma-mcp": json.RawMessage(`{"url":"http://figma"}`),
		},
		Sources: map[string]string{
			"figma-mcp": "~/.claude/plugins/figma-minimal",
		},
	}
	m = m.handleMCPSearchDone(msg)

	// Verify initially read-only (plugin is selected).
	p := m.pickers[SectionMCP]
	found := false
	for _, it := range p.items {
		if it.Key == "figma-mcp" {
			assert.True(t, it.IsReadOnly, "should start as read-only")
			found = true
		}
	}
	require.True(t, found)

	// Deselect the plugin.
	pluginPicker := m.pickers[SectionPlugins]
	for i := range pluginPicker.items {
		if pluginPicker.items[i].Key == "figma-minimal@figma-marketplace" {
			pluginPicker.items[i].Selected = false
		}
	}
	m.pickers[SectionPlugins] = pluginPicker

	// Trigger sync (normally happens via syncSidebarCounts after picker update).
	m.syncMCPPluginState()

	// Server should now be toggleable (not read-only).
	p = m.pickers[SectionMCP]
	for _, it := range p.items {
		if it.Key == "figma-mcp" {
			assert.False(t, it.IsReadOnly, "should be toggleable when plugin is deselected")
			return
		}
	}
	t.Fatal("figma-mcp item not found in picker after deselect")
}

func TestBuildInitOptions_ExcludesPluginMCP(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"figma-minimal@figma-marketplace"},
		PluginKeys: []string{"figma-minimal@figma-marketplace"},
		MCP:        map[string]json.RawMessage{"global-server": json.RawMessage(`{"url":"http://g"}`)},
	}
	m := testModel(scan)

	// Simulate discovered plugin MCP server.
	m.discoveredMCP["figma-mcp"] = json.RawMessage(`{"url":"http://figma"}`)
	m.mcpSources["figma-mcp"] = "~/.claude/plugins/figma-minimal"
	m.mcpPluginKeys["figma-mcp"] = "figma-minimal@figma-marketplace"

	// Add the server to the picker (selected but NOT read-only for this test —
	// buildInitOptions should still exclude it based on mcpPluginKeys + selected plugins).
	p := m.pickers[SectionMCP]
	p.AddItems([]PickerItem{
		{Key: "figma-mcp", Display: "figma-mcp", Selected: true, Tag: "[figma-minimal]"},
	})
	m.pickers[SectionMCP] = p

	opts := m.buildInitOptions()

	// Plugin is selected → plugin MCP server should NOT be in the output.
	assert.Contains(t, opts.MCP, "global-server", "global server should be in output")
	assert.NotContains(t, opts.MCP, "figma-mcp", "plugin-provided MCP should be excluded when plugin is selected")
}

func TestBuildInitOptions_IncludesPluginMCP_WhenPluginDeselected(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:   []string{"figma-minimal@figma-marketplace"},
		PluginKeys: []string{"figma-minimal@figma-marketplace"},
		MCP:        map[string]json.RawMessage{"global-server": json.RawMessage(`{"url":"http://g"}`)},
	}
	m := testModel(scan)

	// Simulate discovered plugin MCP server.
	m.discoveredMCP["figma-mcp"] = json.RawMessage(`{"url":"http://figma"}`)
	m.mcpSources["figma-mcp"] = "~/.claude/plugins/figma-minimal"
	m.mcpPluginKeys["figma-mcp"] = "figma-minimal@figma-marketplace"

	// Add to picker.
	p := m.pickers[SectionMCP]
	p.AddItems([]PickerItem{
		{Key: "figma-mcp", Display: "figma-mcp", Selected: true, Tag: "[figma-minimal]"},
	})
	m.pickers[SectionMCP] = p

	// Deselect the plugin.
	pluginPicker := m.pickers[SectionPlugins]
	for i := range pluginPicker.items {
		if pluginPicker.items[i].Key == "figma-minimal@figma-marketplace" {
			pluginPicker.items[i].Selected = false
		}
	}
	m.pickers[SectionPlugins] = pluginPicker

	opts := m.buildInitOptions()

	// Plugin is NOT selected → plugin MCP server SHOULD be in the output.
	assert.Contains(t, opts.MCP, "global-server", "global server should be in output")
	assert.Contains(t, opts.MCP, "figma-mcp", "plugin MCP should be included when plugin is deselected")
}

// --- Edit Mode Tests ---

func TestNewModel_EditMode_PrePopulatesSelections(t *testing.T) {
	scan := fullScan()

	// Existing config with only a subset selected.
	existingCfg := &config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@m"}, // only a@m, not b@m
		Settings: map[string]any{"model": "opus"}, // only model, not env
		Hooks:    map[string]json.RawMessage{"PreToolUse": json.RawMessage(`[{"hooks":[{"command":"lint"}]}]`)},
		Permissions: config.Permissions{
			Allow: []string{"Bash(git *)"},
			// no deny rules
		},
		ClaudeMD: config.ClaudeMDConfig{
			Include: []string{"test.md"},
		},
		MCP:         map[string]json.RawMessage{"server1": json.RawMessage(`{"url":"http://s1"}`)},
		Keybindings: map[string]any{"ctrl+s": "save"},
		Commands:    []string{"review-pr"},
		// no skills
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, existingCfg, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	// Plugins: only a@m selected, not b@m or c@local.
	pluginPicker := m.pickers[SectionPlugins]
	for _, it := range pluginPicker.items {
		if it.Key == "a@m" {
			assert.True(t, it.Selected, "a@m should be selected")
		}
		if it.Key == "b@m" {
			assert.False(t, it.Selected, "b@m should NOT be selected")
		}
	}

	// Settings: only "model" selected, not "env".
	settingsPicker := m.pickers[SectionSettings]
	for _, it := range settingsPicker.items {
		if it.Key == "model" {
			assert.True(t, it.Selected, "model should be selected")
		}
		if it.Key == "env" {
			assert.False(t, it.Selected, "env should NOT be selected")
		}
	}

	// Permissions: allow rule selected, deny not.
	permPicker := m.pickers[SectionPermissions]
	for _, it := range permPicker.items {
		if it.Key == "allow:Bash(git *)" {
			assert.True(t, it.Selected, "allow rule should be selected")
		}
		if it.Key == "deny:Bash(rm *)" {
			assert.False(t, it.Selected, "deny rule should NOT be selected")
		}
	}

	// Commands selected, skills not.
	csPicker := m.pickers[SectionCommandsSkills]
	for _, it := range csPicker.items {
		if it.Key == "review-pr" {
			assert.True(t, it.Selected, "review-pr command should be selected")
		}
		if it.Key == "tdd" {
			assert.False(t, it.Selected, "tdd skill should NOT be selected")
		}
	}

	// editMode flag should be set.
	assert.True(t, m.editMode, "editMode should be true")
}

func TestNewModel_EditMode_SkipsOverlay(t *testing.T) {
	scan := fullScan()

	existingCfg := &config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@m"},
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", false, SkipFlags{}, existingCfg, nil)

	// With skipProfiles=false and scan data, overlay would normally show.
	// But in edit mode, it should be skipped.
	assert.Equal(t, overlayNone, m.overlayCtx, "overlay should not show in edit mode")
}

func TestNewModel_EditMode_NilConfig_ShowsOverlay(t *testing.T) {
	scan := fullScan()

	// No existing config — fresh create.
	m := NewModel(scan, "/test/claude", "/test/sync", "", false, SkipFlags{}, nil, nil)

	// Overlay should show for fresh creates.
	assert.Equal(t, overlayConfigStyle, m.overlayCtx, "overlay should show for fresh creates")
}

func TestNewModel_EditMode_RestoresProfiles(t *testing.T) {
	scan := fullScan()

	existingCfg := &config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@m", "b@m"},
		Forked:   []string{"c"},
		Settings: map[string]any{"model": "opus", "env": "prod"},
	}

	existingProfs := map[string]profiles.Profile{
		"work": {
			Plugins: profiles.ProfilePlugins{
				Remove: []string{"b@m"},
			},
			Settings: map[string]any{"debug": true},
		},
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, existingCfg, existingProfs)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	// useProfiles should be true.
	assert.True(t, m.useProfiles, "useProfiles should be true")

	// Profile "work" should exist.
	require.Contains(t, m.profilePickers, "work", "work profile should exist")

	// Check profile plugin selections: a@m yes, b@m no (removed), c@local yes.
	workPlugins := m.profilePickers["work"][SectionPlugins]
	for _, it := range workPlugins.items {
		if it.Key == "a@m" {
			assert.True(t, it.Selected, "work profile: a@m should be selected")
		}
		if it.Key == "b@m" {
			assert.False(t, it.Selected, "work profile: b@m should NOT be selected (removed)")
		}
		if it.Key == "c@local" {
			assert.True(t, it.Selected, "work profile: c@local should be selected")
		}
	}

	// Profile plugin diff should be stored.
	require.Contains(t, m.profileDiffs, "work")
	require.Contains(t, m.profileDiffs["work"], SectionPlugins)
	assert.True(t, m.profileDiffs["work"][SectionPlugins].removes["b@m"], "b@m should be in profile removes")
}

func TestClaudeMDSearchDone_EditMode_RespectsExistingConfig(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream:        []string{"a@m"},
		ClaudeMDContent: "## Base\ncontent",
		ClaudeMDSections: []claudemd.Section{
			{Header: "Base", Content: "## Base\ncontent"},
		},
	}

	// Existing config includes the global fragment and one project fragment.
	existingCfg := &config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@m"},
		ClaudeMD: config.ClaudeMDConfig{
			Include: []string{"base", "~/project::saved-section"},
		},
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, existingCfg, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	// Create temp project CLAUDE.md files to simulate discovery.
	tmpDir := t.TempDir()
	projectMD := tmpDir + "/.claude/CLAUDE.md"
	require.NoError(t, os.MkdirAll(tmpDir+"/.claude", 0755))
	require.NoError(t, os.WriteFile(projectMD, []byte("## Saved Section\nkept\n## Unsaved Section\nnot kept"), 0644))

	msg := SearchDoneMsg{Paths: []string{projectMD}}
	m = m.handleSearchDone(msg)

	// Check: the project sections should respect the saved config.
	shortPath := shortenPath(projectMD)
	savedKey := shortPath + "::saved-section"
	unsavedKey := shortPath + "::unsaved-section"

	for i, sec := range m.preview.sections {
		switch sec.FragmentKey {
		case savedKey:
			assert.True(t, m.preview.selected[i], "saved project section should be selected")
		case unsavedKey:
			assert.False(t, m.preview.selected[i], "unsaved project section should be deselected in edit mode")
		}
	}
}

func TestMCPSearchDone_EditMode_RespectsExistingConfig(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"global-server": json.RawMessage(`{"url":"http://g"}`)},
	}

	// Existing config only includes global-server, NOT project-server.
	existingCfg := &config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@m"},
		MCP:      map[string]json.RawMessage{"global-server": json.RawMessage(`{"url":"http://g"}`)},
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, existingCfg, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	// Simulate MCP search returning a server not in the saved config.
	msg := MCPSearchDoneMsg{
		Servers: map[string]json.RawMessage{
			"project-server": json.RawMessage(`{"url":"http://p"}`),
		},
		Sources: map[string]string{
			"project-server": "~/Repos/myproject",
		},
	}
	m = m.handleMCPSearchDone(msg)

	// project-server should be deselected because it's not in existingConfig.MCP.
	p := m.pickers[SectionMCP]
	for _, it := range p.items {
		if it.Key == "project-server" {
			assert.False(t, it.Selected, "project-server should be deselected in edit mode")
		}
		if it.Key == "global-server" {
			assert.True(t, it.Selected, "global-server should remain selected")
		}
	}
}

func TestCmdSkillSearchDone_EditMode_RespectsExistingConfig(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
	}

	// Existing config has only one command saved.
	existingCfg := &config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@m"},
		Commands: []string{"cmd:project:saved-cmd"},
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, existingCfg, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	// Simulate commands/skills search returning items.
	msg := CmdSkillSearchDoneMsg{
		Items: []cmdskill.Item{
			{Name: "saved-cmd", Type: cmdskill.TypeCommand, Source: cmdskill.SourceProject, SourceLabel: "myproject"},
			{Name: "unsaved-cmd", Type: cmdskill.TypeCommand, Source: cmdskill.SourceProject, SourceLabel: "myproject"},
			{Name: "unsaved-skill", Type: cmdskill.TypeSkill, Source: cmdskill.SourceProject, SourceLabel: "myproject"},
		},
	}
	m = m.handleCmdSkillSearchDone(msg)

	// Check selections: only saved-cmd should be selected.
	p := m.pickers[SectionCommandsSkills]
	for _, it := range p.items {
		switch it.Key {
		case "cmd:project:saved-cmd":
			assert.True(t, it.Selected, "saved-cmd should be selected")
		case "cmd:project:unsaved-cmd":
			assert.False(t, it.Selected, "unsaved-cmd should be deselected in edit mode")
		case "skill:project:unsaved-skill":
			assert.False(t, it.Selected, "unsaved-skill should be deselected in edit mode")
		}
	}
}

func TestBuildInitOptions_EditMode(t *testing.T) {
	scan := fullScan()

	existingCfg := &config.Config{
		Version:  "1.0.0",
		Upstream: []string{"a@m"},
		Settings: map[string]any{"model": "opus"},
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, existingCfg, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	opts := m.buildInitOptions()

	// Only a@m selected, so IncludePlugins should contain just "a@m".
	require.NotNil(t, opts.IncludePlugins, "IncludePlugins should not be nil when subset selected")
	assert.Contains(t, opts.IncludePlugins, "a@m")
	assert.NotContains(t, opts.IncludePlugins, "b@m")

	// Settings: only "model" selected.
	assert.True(t, opts.IncludeSettings)
	require.NotNil(t, opts.SettingsFilter)
	assert.Contains(t, opts.SettingsFilter, "model")
	assert.NotContains(t, opts.SettingsFilter, "env")
}

func TestPermissions_EditMode_RoundTrip(t *testing.T) {
	// Simulate a realistic permission set: many allow rules, some with special chars.
	allowRules := []string{
		"Read", "Edit", "Write", "Glob", "Grep", "WebFetch", "WebSearch",
		"Task", "NotebookEdit", "Skill",
		"mcp__plugin_episodic-memory_episodic-memory",
		"mcp__plugin_greptile_greptile",
		"Bash(npm *)", "Bash(npx *)", "Bash(yarn *)", "Bash(bun *)",
		"Bash(python *)", "Bash(go *)", "Bash(cargo *)",
		// These simulate the git bash rules user deselects:
		"Bash(git)", "Bash(git *status)", "Bash(git *status *)",
		"Bash(git *diff)", "Bash(git *diff *)", "Bash(git *log)",
		"Bash(git *add *)", "Bash(git *commit *)", "Bash(git *branch)",
		"Bash(curl *)", "Bash(wget *)", "Bash(dig *)",
	}

	scan := &commands.InitScanResult{
		Upstream:   []string{"a@m"},
		Settings:   map[string]any{"model": "opus"},
		Hooks:      map[string]json.RawMessage{},
		Permissions: config.Permissions{
			Allow: allowRules,
		},
		MCP:         map[string]json.RawMessage{},
		Keybindings: map[string]any{},
	}

	// Step 1: First run — user selects a subset (deselects git-related rules).
	m := testModel(scan)

	// Verify all permissions start selected.
	permPicker := m.pickers[SectionPermissions]
	assert.Equal(t, len(allowRules), permPicker.TotalCount(), "all rules should be present")
	assert.Equal(t, len(allowRules), permPicker.SelectedCount(), "all rules should start selected")

	// Deselect the git-related rules.
	deselected := map[string]bool{
		"allow:Bash(git)":            true,
		"allow:Bash(git *status)":    true,
		"allow:Bash(git *status *)":  true,
		"allow:Bash(git *diff)":      true,
		"allow:Bash(git *diff *)":    true,
		"allow:Bash(git *log)":       true,
		"allow:Bash(git *add *)":     true,
		"allow:Bash(git *commit *)":  true,
		"allow:Bash(git *branch)":    true,
	}
	for i, it := range permPicker.items {
		if deselected[it.Key] {
			permPicker.items[i].Selected = false
		}
	}
	m.pickers[SectionPermissions] = permPicker

	expectedSelected := len(allowRules) - len(deselected)
	assert.Equal(t, expectedSelected, permPicker.SelectedCount(), "after deselecting git rules")

	// Step 2: Build init options (simulates save).
	opts := m.buildInitOptions()
	assert.Equal(t, expectedSelected, len(opts.Permissions.Allow), "saved allow count")
	assert.Empty(t, opts.Permissions.Deny, "no deny rules")

	// Verify deselected rules are NOT in the saved config.
	savedSet := toSet(opts.Permissions.Allow)
	for k := range deselected {
		rule := strings.TrimPrefix(k, "allow:")
		assert.False(t, savedSet[rule], "deselected rule %q should NOT be saved", rule)
	}

	// Step 3: Simulate reload — create config from saved opts.
	savedConfig := &config.Config{
		Version: "1.0.0",
		Permissions: opts.Permissions,
	}

	// Create a new model with the SAME scan but existing config.
	m2 := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, savedConfig, nil)
	m2.overlay = Overlay{}
	m2.overlayCtx = overlayNone

	// Step 4: Verify the reloaded model has the correct selections.
	permPicker2 := m2.pickers[SectionPermissions]
	assert.Equal(t, len(allowRules), permPicker2.TotalCount(), "reload: all rules should still be present")
	assert.Equal(t, expectedSelected, permPicker2.SelectedCount(),
		"reload: should have %d selected, got %d", expectedSelected, permPicker2.SelectedCount())

	// Verify each item's selection state matches.
	for _, it := range permPicker2.items {
		if it.IsHeader {
			continue
		}
		if deselected[it.Key] {
			assert.False(t, it.Selected, "reload: %q should be deselected", it.Key)
		} else {
			assert.True(t, it.Selected, "reload: %q should be selected", it.Key)
		}
	}

	// Step 5: Build opts from reloaded model and verify it matches original.
	opts2 := m2.buildInitOptions()
	assert.Equal(t, len(opts.Permissions.Allow), len(opts2.Permissions.Allow),
		"re-saved allow count should match original")
}

func TestPermissions_EditMode_RoundTrip_WithProfiles(t *testing.T) {
	allowRules := []string{
		"Read", "Edit", "Write", "Glob", "Grep",
		"Bash(npm *)", "Bash(git)", "Bash(git *status)", "Bash(curl *)",
	}

	scan := &commands.InitScanResult{
		Upstream:   []string{"a@m"},
		Settings:   map[string]any{"model": "opus"},
		Hooks:      map[string]json.RawMessage{},
		Permissions: config.Permissions{
			Allow: allowRules,
		},
		MCP:         map[string]json.RawMessage{},
		Keybindings: map[string]any{},
	}

	// Saved config: user deselected "Bash(git)" and "Bash(git *status)".
	selectedAllow := []string{
		"Read", "Edit", "Write", "Glob", "Grep",
		"Bash(npm *)", "Bash(curl *)",
	}
	savedConfig := &config.Config{
		Version: "1.0.0",
		Permissions: config.Permissions{Allow: selectedAllow},
	}

	// Profiles: "work" adds back Bash(git), "personal" removes Bash(curl *).
	existingProfiles := map[string]profiles.Profile{
		"work": {
			Permissions: profiles.ProfilePermissions{
				AddAllow: []string{"Bash(git)"},
			},
		},
		"personal": {
			Permissions: profiles.ProfilePermissions{
				// No adds, but conceptual removes (not directly tested here).
			},
		},
	}

	m := NewModel(scan, "/test/claude", "/test/sync", "", false, SkipFlags{}, savedConfig, existingProfiles)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	// Verify BASE picker has correct selections.
	basePicker := m.pickers[SectionPermissions]
	t.Logf("Base picker: %d/%d selected", basePicker.SelectedCount(), basePicker.TotalCount())
	assert.Equal(t, len(allowRules), basePicker.TotalCount())
	assert.Equal(t, len(selectedAllow), basePicker.SelectedCount(),
		"base should have %d selected", len(selectedAllow))

	// Verify specific items.
	for _, it := range basePicker.items {
		if it.IsHeader {
			continue
		}
		switch it.Key {
		case "allow:Read", "allow:Edit", "allow:Write", "allow:Glob", "allow:Grep",
			"allow:Bash(npm *)", "allow:Bash(curl *)":
			assert.True(t, it.Selected, "base: %q should be selected", it.Key)
		case "allow:Bash(git)", "allow:Bash(git *status)":
			assert.False(t, it.Selected, "base: %q should be deselected", it.Key)
		}
	}

	// Build opts and verify permission round-trip.
	opts := m.buildInitOptions()
	assert.Equal(t, len(selectedAllow), len(opts.Permissions.Allow),
		"saved permissions should match")
}

func TestPermissions_EditMode_RealWorldScale(t *testing.T) {
	// Simulate the real-world scenario: 174 permission rules, save 142, reload.
	allAllow := make([]string, 0, 174)
	// Simple tool permissions.
	simpleTools := []string{
		"Read", "Edit", "Write", "Glob", "Grep", "WebFetch", "WebSearch",
		"Task", "NotebookEdit", "Skill",
	}
	allAllow = append(allAllow, simpleTools...)
	// MCP plugin permissions.
	mcpPerms := []string{
		"mcp__plugin_episodic-memory_episodic-memory",
		"mcp__plugin_greptile_greptile",
		"mcp__plugin_context7_context7",
		"mcp__plugin_playwright_playwright",
	}
	allAllow = append(allAllow, mcpPerms...)
	// Bash permissions (many with glob patterns).
	bashTools := []string{
		"Bash(npm *)", "Bash(npx *)", "Bash(yarn *)", "Bash(pnpm *)",
		"Bash(bun *)", "Bash(deno *)", "Bash(node *)", "Bash(tsx *)",
		"Bash(python *)", "Bash(python3 *)", "Bash(pip *)",
		"Bash(go *)", "Bash(cargo *)", "Bash(make *)", "Bash(cmake *)",
	}
	allAllow = append(allAllow, bashTools...)
	// Git-specific bash (will be deselected).
	gitBash := []string{
		"Bash(git)", "Bash(git *status)", "Bash(git *status *)",
		"Bash(git *diff)", "Bash(git *diff *)", "Bash(git *log)",
		"Bash(git *log *)", "Bash(git *show)", "Bash(git *show *)",
		"Bash(git *add *)", "Bash(git *commit *)", "Bash(git *fetch)",
		"Bash(git *fetch *)", "Bash(git *blame *)", "Bash(git *rev-parse)",
		"Bash(git *rev-parse *)", "Bash(git *ls-files)", "Bash(git *ls-files *)",
		"Bash(git *remote)", "Bash(git *remote *)", "Bash(git *config)",
		"Bash(git *config *)", "Bash(git *grep *)", "Bash(git *tag)",
		"Bash(git *tag *)", "Bash(git *stash)", "Bash(git *stash *)",
		"Bash(git *branch)", "Bash(git *branch *)",
	}
	allAllow = append(allAllow, gitBash...)
	// More tools to get close to 174.
	moreTools := make([]string, 0)
	for i := 0; i < 174-len(allAllow); i++ {
		moreTools = append(moreTools, fmt.Sprintf("Bash(tool%d *)", i))
	}
	allAllow = append(allAllow, moreTools...)
	require.Equal(t, 174, len(allAllow), "should have 174 rules")

	// Build deselected set (the git-bash ones).
	deselectedRules := make(map[string]bool)
	for _, r := range gitBash {
		deselectedRules[r] = true
	}
	// Expected selected = total minus deselected.
	selectedAllow := make([]string, 0)
	for _, r := range allAllow {
		if !deselectedRules[r] {
			selectedAllow = append(selectedAllow, r)
		}
	}
	expectedCount := len(selectedAllow)
	t.Logf("Total: %d, Deselected: %d, Expected selected: %d",
		len(allAllow), len(deselectedRules), expectedCount)

	scan := &commands.InitScanResult{
		Upstream:    []string{},
		Settings:    map[string]any{},
		Hooks:       map[string]json.RawMessage{},
		Permissions: config.Permissions{Allow: allAllow},
		MCP:         map[string]json.RawMessage{},
		Keybindings: map[string]any{},
	}

	savedConfig := &config.Config{
		Version:     "1.0.0",
		Permissions: config.Permissions{Allow: selectedAllow},
	}

	// Load in edit mode.
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{}, savedConfig, nil)
	m.overlay = Overlay{}
	m.overlayCtx = overlayNone

	picker := m.pickers[SectionPermissions]
	t.Logf("After edit-mode load: %d/%d selected", picker.SelectedCount(), picker.TotalCount())
	assert.Equal(t, 174, picker.TotalCount())
	assert.Equal(t, expectedCount, picker.SelectedCount(),
		"expected %d selected after edit-mode load", expectedCount)

	// Check specific items.
	mismatches := 0
	for _, it := range picker.items {
		if it.IsHeader {
			continue
		}
		rule := strings.TrimPrefix(it.Key, "allow:")
		if deselectedRules[rule] && it.Selected {
			t.Logf("BUG: %q should be deselected but is selected", it.Key)
			mismatches++
		}
		if !deselectedRules[rule] && !it.Selected {
			t.Logf("BUG: %q should be selected but is deselected", it.Key)
			mismatches++
		}
	}
	assert.Zero(t, mismatches, "found %d selection mismatches", mismatches)

	// Round-trip: build opts and check.
	opts := m.buildInitOptions()
	assert.Equal(t, expectedCount, len(opts.Permissions.Allow),
		"saved permissions count after round-trip")
}
