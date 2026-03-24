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

	// Switch away and back. Diffs are saved and rebuilt.
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

func TestDiffsToProfile_NewBaseItemNotSpuriouslyRemoved(t *testing.T) {
	// Reproduces issue #31: adding a new item to the base picker without visiting
	// the profile tab causes diffsToProfile() to generate a spurious remove: entry.
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	// Step 1: Create a "work" profile (inherits current base).
	m.createProfile("work")

	// Step 2: Save diffs and switch to Base tab.
	m.saveAllProfileDiffs("work")
	m.activeTab = "Base"

	// Step 3: Deselect "b@m" from base plugins, then rebuild the work profile.
	// This simulates the state where the profile was created without "b@m" in base.
	basePicker := m.pickers[SectionPlugins]
	for i := range basePicker.items {
		if basePicker.items[i].Key == "b@m" {
			basePicker.items[i].Selected = false
		}
	}
	m.pickers[SectionPlugins] = basePicker
	m.rebuildAllProfilePickers("work")
	m.saveAllProfileDiffs("work")

	// Step 4: Re-select "b@m" in base (simulates adding a new item to base).
	basePicker = m.pickers[SectionPlugins]
	for i := range basePicker.items {
		if basePicker.items[i].Key == "b@m" {
			basePicker.items[i].Selected = true
		}
	}
	m.pickers[SectionPlugins] = basePicker

	// Step 5: DO NOT visit the work profile tab (no rebuildAllProfilePickers call).
	// The work profile picker is now stale: it doesn't know about the re-added "b@m".

	// Step 6: syncProfilesBeforeSave fixes stale pickers, then diffsToProfile runs clean.
	m.syncProfilesBeforeSave()
	prof := m.diffsToProfile("work")

	// Without syncProfilesBeforeSave, the stale profile picker would cause a spurious remove.
	assert.NotContains(t, prof.Plugins.Remove, "b@m",
		"b@m was added to base; it should NOT appear as a profile removal")
	assert.Empty(t, prof.Plugins.Add, "no plugins were explicitly added to the profile")
}

func TestDiffsToProfile_NewSkillNotSpuriouslyRemoved(t *testing.T) {
	// Reproduces the exact scenario from issue #31: user creates a new skill,
	// it appears in base, but profiles generate remove: entries for it.
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	// Step 1: Create two profiles.
	m.createProfile("work")
	m.createProfile("personal")

	// Step 2: Save diffs for both profiles and switch to Base tab.
	m.saveAllProfileDiffs("work")
	m.saveAllProfileDiffs("personal")
	m.activeTab = "Base"

	// Step 3: Add a brand-new skill to the base CommandsSkills picker.
	// This simulates the user creating a new skill file that gets discovered.
	csPicker := m.pickers[SectionCommandsSkills]
	csPicker.items = append(csPicker.items, PickerItem{
		Key:      "skill:global:my-new-skill",
		Display:  "my-new-skill",
		Selected: true,
	})
	m.pickers[SectionCommandsSkills] = csPicker

	// Step 4: DO NOT visit either profile tab. The profile pickers are stale.

	// Step 5: syncProfilesBeforeSave fixes stale pickers, then diffsToProfile runs clean.
	m.syncProfilesBeforeSave()
	workProf := m.diffsToProfile("work")
	personalProf := m.diffsToProfile("personal")

	// Without syncProfilesBeforeSave, the stale profile pickers would cause spurious removes.
	assert.NotContains(t, workProf.Skills.Remove, "skill:global:my-new-skill",
		"work profile should NOT remove a newly-added base skill")
	assert.NotContains(t, personalProf.Skills.Remove, "skill:global:my-new-skill",
		"personal profile should NOT remove a newly-added base skill")

	// The new skill should also NOT appear as a profile add (it's inherited from base).
	assert.NotContains(t, workProf.Skills.Add, "skill:global:my-new-skill",
		"work profile should not explicitly add a base skill")
	assert.NotContains(t, personalProf.Skills.Add, "skill:global:my-new-skill",
		"personal profile should not explicitly add a base skill")
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

func TestSyncProfilesBeforeSave_PreservesIntentionalRemoves(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	// Create work profile (inherits base).
	m.createProfile("work")

	// User visits work profile and deselects "a@m" (intentional remove).
	pm := m.profilePickers["work"]
	pluginPicker := pm[SectionPlugins]
	for i := range pluginPicker.items {
		if pluginPicker.items[i].Key == "a@m" {
			pluginPicker.items[i].Selected = false
		}
	}
	pm[SectionPlugins] = pluginPicker
	m.profilePickers["work"] = pm

	// Save diffs and switch to Base tab.
	m.saveAllProfileDiffs("work")
	m.activeTab = "Base"

	m.syncProfilesBeforeSave()

	// Verify intentional remove is preserved.
	prof := m.diffsToProfile("work")
	assert.Contains(t, prof.Plugins.Remove, "a@m",
		"intentional remove of a@m should be preserved after syncProfilesBeforeSave")
}

func TestSyncProfilesBeforeSave_SavesActiveProfileChanges(t *testing.T) {
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	// Create work profile.
	m.createProfile("work")

	// Set activeTab to work (user is currently on work profile).
	m.activeTab = "work"

	// Deselect "b@m" in the work profile picker (unsaved change, not yet saved via saveAllProfileDiffs).
	pm := m.profilePickers["work"]
	pluginPicker := pm[SectionPlugins]
	for i := range pluginPicker.items {
		if pluginPicker.items[i].Key == "b@m" {
			pluginPicker.items[i].Selected = false
		}
	}
	pm[SectionPlugins] = pluginPicker
	m.profilePickers["work"] = pm

	// Call syncProfilesBeforeSave. This should capture the unsaved change.
	m.syncProfilesBeforeSave()

	// Verify the unsaved change was captured.
	prof := m.diffsToProfile("work")
	assert.Contains(t, prof.Plugins.Remove, "b@m",
		"unsaved deselection of b@m should be captured by syncProfilesBeforeSave")
}

func TestDiffsToProfile_NewMCPServerNotSpuriouslyRemoved(t *testing.T) {
	// Verifies the fix works for MCP servers (non-Plugin, non-Skill section).
	// The bug could affect any section with remove: semantics.
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	// Step 1: Create a profile.
	m.createProfile("work")
	m.saveAllProfileDiffs("work")
	m.activeTab = "Base"

	// Step 2: Add a new MCP server to base.
	mcpPicker := m.pickers[SectionMCP]
	mcpPicker.items = append(mcpPicker.items, PickerItem{
		Key:      "new-server",
		Display:  "new-server",
		Selected: true,
	})
	m.pickers[SectionMCP] = mcpPicker

	// Step 3: DO NOT visit the work profile tab. The profile picker is stale.

	// Step 4: syncProfilesBeforeSave fixes stale pickers, then diffsToProfile runs clean.
	m.syncProfilesBeforeSave()
	prof := m.diffsToProfile("work")

	assert.NotContains(t, prof.MCP.Remove, "new-server",
		"new MCP server added to base should NOT appear as a profile removal")
}

func TestBuildInitOptions_NoSpuriousRemovesEndToEnd(t *testing.T) {
	// End-to-end test through the production call chain:
	// syncProfilesBeforeSave -> buildInitOptions -> buildProfiles -> diffsToProfile.
	// Guards against someone removing the sync call from handleOverlayClose.
	scan := fullScan()
	m := testModel(scan)
	m.ready = true
	m.width = 80
	m.height = 30
	m.distributeSize()

	// Create two profiles and save their initial state.
	m.createProfile("work")
	m.saveAllProfileDiffs("work")
	m.createProfile("personal")
	m.saveAllProfileDiffs("personal")
	m.activeTab = "Base"

	// Deselect "b@m" from base, rebuild profiles to record this state, then
	// re-select "b@m" (simulates adding a new item to base). We use a scan-
	// result plugin because rebuildProfilePluginSection builds from scanResult.
	basePicker := m.pickers[SectionPlugins]
	for i := range basePicker.items {
		if basePicker.items[i].Key == "b@m" {
			basePicker.items[i].Selected = false
		}
	}
	m.pickers[SectionPlugins] = basePicker
	m.rebuildAllProfilePickers("work")
	m.rebuildAllProfilePickers("personal")
	m.saveAllProfileDiffs("work")
	m.saveAllProfileDiffs("personal")

	// Re-select "b@m" without visiting any profile tab.
	basePicker = m.pickers[SectionPlugins]
	for i := range basePicker.items {
		if basePicker.items[i].Key == "b@m" {
			basePicker.items[i].Selected = true
		}
	}
	m.pickers[SectionPlugins] = basePicker

	// Mirror the production save path: sync then build.
	m.syncProfilesBeforeSave()
	opts := m.buildInitOptions()

	require.NotNil(t, opts.Profiles)
	for _, name := range []string{"work", "personal"} {
		prof, ok := opts.Profiles[name]
		require.True(t, ok, "profile %s should exist in output", name)
		assert.NotContains(t, prof.Plugins.Remove, "b@m",
			"%s profile should not spuriously remove b@m", name)
		assert.Empty(t, prof.Plugins.Add,
			"%s profile should not explicitly add base-inherited plugins", name)
	}
}
