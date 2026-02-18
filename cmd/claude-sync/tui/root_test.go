package tui

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testModel creates a Model from a scan result with the overlay dismissed
// so buildInitOptions can be tested directly.
func testModel(scan *commands.InitScanResult) Model {
	m := NewModel(scan, "/test/claude", "/test/sync", "", true, SkipFlags{})
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
	})
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
	})
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
	})
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
	})
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
	m := NewModel(scan, "/c", "/s", "https://git.example.com/repo", true, SkipFlags{})
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
	})
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
