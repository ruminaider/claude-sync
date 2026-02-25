package tui

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSectionDiff_Empty(t *testing.T) {
	diff := newSectionDiff()
	assert.Empty(t, diff.adds)
	assert.Empty(t, diff.removes)

	// Effective of empty diff is the base unchanged.
	base := []string{"a", "b", "c"}
	effective := diff.effectiveKeys(base)
	assert.Len(t, effective, 3)
	assert.True(t, effective["a"])
	assert.True(t, effective["b"])
	assert.True(t, effective["c"])
}

func TestSectionDiff_AddsAndRemoves(t *testing.T) {
	diff := newSectionDiff()
	diff.adds["d"] = true
	diff.removes["b"] = true

	base := []string{"a", "b", "c"}
	effective := diff.effectiveKeys(base)

	assert.True(t, effective["a"], "a should remain")
	assert.False(t, effective["b"], "b should be removed")
	assert.True(t, effective["c"], "c should remain")
	assert.True(t, effective["d"], "d should be added")
	assert.Len(t, effective, 3) // a, c, d
}

func TestComputeSectionDiff(t *testing.T) {
	base := []string{"a", "b", "c"}
	profile := []string{"b", "c", "d"}

	diff := computeSectionDiff(base, profile)
	assert.True(t, diff.adds["d"], "d should be added")
	assert.True(t, diff.removes["a"], "a should be removed")
	assert.Len(t, diff.adds, 1)
	assert.Len(t, diff.removes, 1)
}

func TestComputeSectionDiff_Identical(t *testing.T) {
	keys := []string{"x", "y"}
	diff := computeSectionDiff(keys, keys)
	assert.Empty(t, diff.adds)
	assert.Empty(t, diff.removes)
}

func TestSortedKeys(t *testing.T) {
	m := map[string]bool{"c": true, "a": true, "b": true}
	assert.Equal(t, []string{"a", "b", "c"}, sortedKeys(m))
}

func TestSortedKeys_Empty(t *testing.T) {
	assert.Empty(t, sortedKeys(nil))
	assert.Empty(t, sortedKeys(map[string]bool{}))
}

func TestProfileToSectionDiffs_Plugins(t *testing.T) {
	p := profiles.Profile{
		Plugins: profiles.ProfilePlugins{
			Add:    []string{"x@m"},
			Remove: []string{"y@m"},
		},
	}
	diffs := profileToSectionDiffs(p)
	d := diffs[SectionPlugins]
	require.NotNil(t, d)
	assert.True(t, d.adds["x@m"])
	assert.True(t, d.removes["y@m"])
}

func TestProfileToSectionDiffs_Settings(t *testing.T) {
	p := profiles.Profile{
		Settings: map[string]any{"model": "opus"},
	}
	diffs := profileToSectionDiffs(p)
	d := diffs[SectionSettings]
	require.NotNil(t, d)
	assert.True(t, d.adds["model"])
	assert.Empty(t, d.removes)
}

func TestProfileToSectionDiffs_Permissions(t *testing.T) {
	p := profiles.Profile{
		Permissions: profiles.ProfilePermissions{
			AddAllow: []string{"Bash(git *)"},
			AddDeny:  []string{"Bash(rm *)"},
		},
	}
	diffs := profileToSectionDiffs(p)
	d := diffs[SectionPermissions]
	require.NotNil(t, d)
	assert.True(t, d.adds["allow:Bash(git *)"])
	assert.True(t, d.adds["deny:Bash(rm *)"])
	assert.Empty(t, d.removes)
}

func TestProfileToSectionDiffs_ClaudeMD(t *testing.T) {
	p := profiles.Profile{
		ClaudeMD: profiles.ProfileClaudeMD{
			Add:    []string{"frag-new"},
			Remove: []string{"frag-old"},
		},
	}
	diffs := profileToSectionDiffs(p)
	d := diffs[SectionClaudeMD]
	require.NotNil(t, d)
	assert.True(t, d.adds["frag-new"])
	assert.True(t, d.removes["frag-old"])
}

func TestProfileToSectionDiffs_CommandsSkills(t *testing.T) {
	p := profiles.Profile{
		Commands: profiles.ProfileCommands{
			Add:    []string{"cmd:review-pr"},
			Remove: []string{"cmd:old-cmd"},
		},
		Skills: profiles.ProfileSkills{
			Add:    []string{"skill:tdd"},
			Remove: []string{"skill:old-skill"},
		},
	}
	diffs := profileToSectionDiffs(p)
	d := diffs[SectionCommandsSkills]
	require.NotNil(t, d)
	assert.True(t, d.adds["cmd:review-pr"])
	assert.True(t, d.adds["skill:tdd"])
	assert.True(t, d.removes["cmd:old-cmd"])
	assert.True(t, d.removes["skill:old-skill"])
}

func TestProfileToSectionDiffs_Keybindings(t *testing.T) {
	p := profiles.Profile{
		Keybindings: profiles.ProfileKeybindings{
			Override: map[string]any{"ctrl+s": "save"},
		},
	}
	diffs := profileToSectionDiffs(p)
	d := diffs[SectionKeybindings]
	require.NotNil(t, d)
	assert.True(t, d.adds["keybindings"])
}

func TestProfileToSectionDiffs_Empty(t *testing.T) {
	p := profiles.Profile{}
	diffs := profileToSectionDiffs(p)
	for _, sec := range AllSections {
		d := diffs[sec]
		require.NotNil(t, d, "section %v should have a diff", sec)
		assert.Empty(t, d.adds, "section %v should have no adds", sec)
		assert.Empty(t, d.removes, "section %v should have no removes", sec)
	}
}

func TestRebuildProfilePicker_Settings(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	// Verify settings picker inherited base settings with IsBase markers.
	workSettings := m.profilePickers["work"][SectionSettings]
	for _, it := range workSettings.items {
		if isSelectableItem(it) && it.Selected {
			assert.True(t, it.IsBase, "selected setting %s should have IsBase=true", it.Key)
		}
	}
}

func TestRebuildProfilePicker_MCP(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	// Verify MCP picker inherited base MCP with IsBase markers.
	workMCP := m.profilePickers["work"][SectionMCP]
	for _, it := range workMCP.items {
		if isSelectableItem(it) && it.Selected {
			assert.True(t, it.IsBase, "selected MCP %s should have IsBase=true", it.Key)
		}
	}
}

func TestRebuildProfilePicker_Hooks(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	workHooks := m.profilePickers["work"][SectionHooks]
	for _, it := range workHooks.items {
		if isSelectableItem(it) && it.Selected {
			assert.True(t, it.IsBase, "selected hook %s should have IsBase=true", it.Key)
		}
	}
}

func TestRebuildProfilePicker_Permissions(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	workPerms := m.profilePickers["work"][SectionPermissions]
	for _, it := range workPerms.items {
		if isSelectableItem(it) && it.Selected {
			assert.True(t, it.IsBase, "selected perm %s should have IsBase=true", it.Key)
		}
	}
}

func TestRebuildProfilePicker_ClaudeMD(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	pp := m.profilePreviews["work"]
	for _, sec := range pp.sections {
		assert.True(t, sec.IsBase, "section %s should be marked IsBase", sec.Header)
	}
}

func TestTabSwitchRebuildsAllSections(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	// Deselect a setting in the work profile.
	pm := m.profilePickers["work"]
	settingsPicker := pm[SectionSettings]
	for i := range settingsPicker.items {
		if settingsPicker.items[i].Key == "model" {
			settingsPicker.items[i].Selected = false
		}
	}
	pm[SectionSettings] = settingsPicker
	m.profilePickers["work"] = pm

	// Switch away and back — diffs are saved and rebuilt.
	m.saveAllProfileDiffs("work")
	m.activeTab = "Base"

	// Change base: deselect "env" from base settings.
	basePicker := m.pickers[SectionSettings]
	for i := range basePicker.items {
		if basePicker.items[i].Key == "env" {
			basePicker.items[i].Selected = false
		}
	}
	m.pickers[SectionSettings] = basePicker

	// Switch back to work.
	m.rebuildAllProfilePickers("work")
	m.activeTab = "work"

	workSettings := m.profilePickers["work"][SectionSettings]

	// "model" should remain deselected (profile removed it).
	// "env" should now also be gone (base removed it).
	for _, it := range workSettings.items {
		if it.Key == "model" {
			assert.False(t, it.Selected, "model should stay deselected (profile override)")
		}
		if it.Key == "env" {
			assert.False(t, it.Selected, "env should be deselected (removed from base)")
			assert.False(t, it.IsBase, "env should not be IsBase (not in base)")
		}
	}

	// Check the stored diff.
	diff := m.profileDiffs["work"][SectionSettings]
	require.NotNil(t, diff)
	assert.True(t, diff.removes["model"], "model should be in removes")
}

func TestDiffsToProfile_RoundTrip(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	// Make modifications in the work profile.
	pm := m.profilePickers["work"]

	// Deselect a plugin.
	pluginPicker := pm[SectionPlugins]
	for i := range pluginPicker.items {
		if pluginPicker.items[i].Key == "b@m" {
			pluginPicker.items[i].Selected = false
		}
	}
	pm[SectionPlugins] = pluginPicker
	m.profilePickers["work"] = pm

	// Build profile from current state.
	prof := m.diffsToProfile("work")

	assert.Contains(t, prof.Plugins.Remove, "b@m", "b@m should be in removes")
	assert.Empty(t, prof.Plugins.Add, "no plugins were added")

	// Convert to diffs and verify round-trip.
	diffs := profileToSectionDiffs(prof)
	pluginDiff := diffs[SectionPlugins]
	assert.True(t, pluginDiff.removes["b@m"])
	assert.Empty(t, pluginDiff.adds)
}

func TestRebuildAfterBaseChange_AllSectionsUpdate(t *testing.T) {
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m", "b@m"},
		Settings: map[string]any{"model": "opus"},
	}
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("dev")

	// Verify both settings and plugins inherit base.
	devPlugins := m.profilePickers["dev"][SectionPlugins]
	assert.Equal(t, 2, devPlugins.SelectedCount(), "should inherit 2 plugins")

	devSettings := m.profilePickers["dev"][SectionSettings]
	assert.Equal(t, 1, devSettings.SelectedCount(), "should inherit 1 setting")

	// Save diffs, modify base (deselect a@m), rebuild.
	m.saveAllProfileDiffs("dev")
	basePicker := m.pickers[SectionPlugins]
	for i := range basePicker.items {
		if basePicker.items[i].Key == "a@m" {
			basePicker.items[i].Selected = false
		}
	}
	m.pickers[SectionPlugins] = basePicker

	m.rebuildAllProfilePickers("dev")

	devPlugins = m.profilePickers["dev"][SectionPlugins]
	assert.Equal(t, 1, devPlugins.SelectedCount(), "should only have 1 plugin after base change")
}

func TestDiffsToProfile_MCPSecretsStripped(t *testing.T) {
	// Create scan with MCP containing a secret.
	scan := &commands.InitScanResult{
		Upstream: []string{"a@m"},
		MCP: map[string]json.RawMessage{
			"render": json.RawMessage(`{"command":"npx","env":{"RENDER_API_KEY":"rnd_abc123"}}`),
		},
	}
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	m.createProfile("work")

	// Deselect MCP from base so it becomes a profile-only add.
	baseMCP := m.pickers[SectionMCP]
	for i := range baseMCP.items {
		if baseMCP.items[i].Key == "render" {
			baseMCP.items[i].Selected = false
		}
	}
	m.pickers[SectionMCP] = baseMCP

	prof := m.diffsToProfile("work")

	// The profile should have "render" as an MCP add, with secrets replaced.
	require.Contains(t, prof.MCP.Add, "render")

	var cfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(prof.MCP.Add["render"], &cfg))
	assert.Equal(t, "${RENDER_API_KEY}", cfg.Env["RENDER_API_KEY"], "secret should be replaced with env var ref")
}
