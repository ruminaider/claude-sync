package tui

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// sectionDiff tracks explicit profile overrides relative to base for any section.
type sectionDiff struct {
	adds    map[string]bool // keys to add (not in base, user selected in profile)
	removes map[string]bool // keys to remove (in base, user deselected in profile)
}

func newSectionDiff() *sectionDiff {
	return &sectionDiff{
		adds:    make(map[string]bool),
		removes: make(map[string]bool),
	}
}

// sortedKeys returns a sorted slice of keys from a bool map.
func sortedKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// computeSectionDiff computes a diff between base and profile key sets.
func computeSectionDiff(baseKeys, profileKeys []string) *sectionDiff {
	baseSet := toSet(baseKeys)
	profSet := toSet(profileKeys)
	diff := newSectionDiff()
	for k := range profSet {
		if !baseSet[k] {
			diff.adds[k] = true
		}
	}
	for k := range baseSet {
		if !profSet[k] {
			diff.removes[k] = true
		}
	}
	return diff
}

// effectiveKeys computes the effective key set from base + diff.
func (d *sectionDiff) effectiveKeys(baseKeys []string) map[string]bool {
	effective := make(map[string]bool, len(baseKeys))
	for _, k := range baseKeys {
		if !d.removes[k] {
			effective[k] = true
		}
	}
	for k := range d.adds {
		effective[k] = true
	}
	return effective
}

// --- Model methods for saving/rebuilding diffs ---

// getProfileDiff returns the stored diff for a profile+section, or an empty one.
func (m *Model) getProfileDiff(profileName string, sec Section) *sectionDiff {
	if diffs, ok := m.profileDiffs[profileName]; ok {
		if d, ok := diffs[sec]; ok {
			return d
		}
	}
	return newSectionDiff()
}

// saveProfileDiff computes and stores the diff for a single section by comparing
// the profile's current picker/preview selections against the base.
func (m *Model) saveProfileDiff(profileName string, sec Section) {
	if profileName == "Base" {
		return
	}
	if _, ok := m.profileDiffs[profileName]; !ok {
		m.profileDiffs[profileName] = make(map[Section]*sectionDiff)
	}

	if sec == SectionClaudeMD {
		pp, ok := m.profilePreviews[profileName]
		if !ok {
			return
		}
		baseKeys := m.preview.SelectedFragmentKeys()
		profKeys := pp.SelectedFragmentKeys()
		m.profileDiffs[profileName][sec] = computeSectionDiff(baseKeys, profKeys)
		return
	}

	pm, ok := m.profilePickers[profileName]
	if !ok {
		return
	}
	picker, ok := pm[sec]
	if !ok {
		return
	}
	baseKeys := m.pickers[sec].SelectedKeys()
	profKeys := picker.SelectedKeys()
	m.profileDiffs[profileName][sec] = computeSectionDiff(baseKeys, profKeys)
}

// saveAllProfileDiffs saves diffs for all sections of a profile.
func (m *Model) saveAllProfileDiffs(profileName string) {
	if profileName == "Base" {
		return
	}
	for _, sec := range AllSections {
		m.saveProfileDiff(profileName, sec)
	}
}

// rebuildProfileSection rebuilds a single section's picker/preview for a profile
// from the current base state plus stored diffs. Dispatches to section-specific
// rebuild logic.
func (m *Model) rebuildProfileSection(profileName string, sec Section) {
	if profileName == "Base" {
		return
	}
	diff := m.getProfileDiff(profileName, sec)

	switch sec {
	case SectionPlugins:
		m.rebuildProfilePluginSection(profileName, diff)
	case SectionClaudeMD:
		m.rebuildProfileClaudeMDSection(profileName, diff)
	default:
		m.rebuildProfileGenericSection(profileName, sec, diff)
	}
}

// rebuildAllProfilePickers rebuilds all sections for a profile.
func (m *Model) rebuildAllProfilePickers(profileName string) {
	if profileName == "Base" {
		return
	}
	for _, sec := range AllSections {
		m.rebuildProfileSection(profileName, sec)
	}
}

// rebuildProfilePluginSection rebuilds the Plugins section using the scan-aware
// PluginPickerItemsForProfile builder.
func (m *Model) rebuildProfilePluginSection(profileName string, diff *sectionDiff) {
	pm, ok := m.profilePickers[profileName]
	if !ok {
		return
	}
	baseSelected := toSet(m.pickers[SectionPlugins].SelectedKeys())
	effective := diff.effectiveKeys(m.pickers[SectionPlugins].SelectedKeys())

	items := PluginPickerItemsForProfile(m.scanResult, effective, baseSelected)
	p := NewPicker(items)
	p.SetTagColor(m.tabBar.ActiveTheme().Accent)
	p.SetProfileTab(true)

	// Preserve dimensions from existing picker.
	if old, ok := pm[SectionPlugins]; ok {
		p.SetHeight(old.height)
		p.SetWidth(old.width)
	}
	pm[SectionPlugins] = p
	m.profilePickers[profileName] = pm
}

// rebuildProfileClaudeMDSection rebuilds the CLAUDE.md preview for a profile.
func (m *Model) rebuildProfileClaudeMDSection(profileName string, diff *sectionDiff) {
	baseFragKeys := m.preview.SelectedFragmentKeys()
	effective := diff.effectiveKeys(baseFragKeys)

	// Copy base sections and mark IsBase flags.
	baseFragSet := toSet(baseFragKeys)
	profileSections := make([]PreviewSection, len(m.preview.sections))
	copy(profileSections, m.preview.sections)
	for i := range profileSections {
		profileSections[i].IsBase = baseFragSet[profileSections[i].FragmentKey]
	}

	pp := NewPreview(profileSections)
	// Apply effective selection.
	for i, sec := range pp.sections {
		pp.selected[i] = effective[sec.FragmentKey]
	}

	// Preserve dimensions from existing preview.
	if old, ok := m.profilePreviews[profileName]; ok {
		pp.SetSize(old.totalWidth, old.totalHeight)
		pp.SetFocused(old.focused)
	}
	m.profilePreviews[profileName] = pp
}

// rebuildProfileGenericSection rebuilds any non-Plugin, non-ClaudeMD section
// by copying base picker items, applying the effective selection, and marking
// inherited items with IsBase/Tag.
func (m *Model) rebuildProfileGenericSection(profileName string, sec Section, diff *sectionDiff) {
	pm, ok := m.profilePickers[profileName]
	if !ok {
		return
	}
	basePicker, ok := m.pickers[sec]
	if !ok {
		return
	}

	baseKeys := basePicker.SelectedKeys()
	effective := diff.effectiveKeys(baseKeys)
	baseSet := toSet(baseKeys)

	// Deep copy items from base picker.
	items := copyPickerItems(basePicker)
	for i := range items {
		if !isSelectableItem(items[i]) {
			continue
		}
		items[i].Selected = effective[items[i].Key]

		// Reset IsBase to clean state before reapplying.
		items[i].IsBase = false

		// Mark items that come from base.
		if baseSet[items[i].Key] {
			items[i].IsBase = true
		}
	}

	p := NewPicker(items)
	p.SetTagColor(m.tabBar.ActiveTheme().Accent)
	p.SetProfileTab(true)

	// Preserve picker properties from existing picker.
	if old, ok := pm[sec]; ok {
		p.SetHeight(old.height)
		p.SetWidth(old.width)
	}
	if basePicker.hasSearchAction {
		p.SetSearchAction(true)
	}
	if basePicker.CollapseReadOnly {
		p.CollapseReadOnly = true
		p.autoCollapseReadOnly()
	}
	if basePicker.hasPreview {
		p.hasPreview = true
		p.previewContent = basePicker.previewContent
	}

	pm[sec] = p
	m.profilePickers[profileName] = pm
}

// --- Profile <-> sectionDiff conversion ---

// profileToSectionDiffs converts a profiles.Profile into per-section diffs.
func profileToSectionDiffs(p profiles.Profile) map[Section]*sectionDiff {
	diffs := make(map[Section]*sectionDiff)

	// Plugins
	d := newSectionDiff()
	for _, k := range p.Plugins.Add {
		d.adds[k] = true
	}
	for _, k := range p.Plugins.Remove {
		d.removes[k] = true
	}
	diffs[SectionPlugins] = d

	// Settings (only adds — profile can't remove base settings)
	d = newSectionDiff()
	for k := range p.Settings {
		d.adds[k] = true
	}
	diffs[SectionSettings] = d

	// Permissions (only adds)
	d = newSectionDiff()
	for _, rule := range p.Permissions.AddAllow {
		d.adds["allow:"+rule] = true
	}
	for _, rule := range p.Permissions.AddDeny {
		d.adds["deny:"+rule] = true
	}
	diffs[SectionPermissions] = d

	// CLAUDE.md
	d = newSectionDiff()
	for _, k := range p.ClaudeMD.Add {
		d.adds[k] = true
	}
	for _, k := range p.ClaudeMD.Remove {
		d.removes[k] = true
	}
	diffs[SectionClaudeMD] = d

	// MCP
	d = newSectionDiff()
	for k := range p.MCP.Add {
		d.adds[k] = true
	}
	for _, k := range p.MCP.Remove {
		d.removes[k] = true
	}
	diffs[SectionMCP] = d

	// Hooks
	d = newSectionDiff()
	for k := range p.Hooks.Add {
		d.adds[k] = true
	}
	for _, k := range p.Hooks.Remove {
		d.removes[k] = true
	}
	diffs[SectionHooks] = d

	// Keybindings: if override is set, the profile has keybindings enabled.
	d = newSectionDiff()
	if len(p.Keybindings.Override) > 0 {
		d.adds["keybindings"] = true
	}
	diffs[SectionKeybindings] = d

	// Commands & Skills (combined into one section with prefixed keys)
	d = newSectionDiff()
	for _, k := range p.Commands.Add {
		d.adds[k] = true
	}
	for _, k := range p.Commands.Remove {
		d.removes[k] = true
	}
	for _, k := range p.Skills.Add {
		d.adds[k] = true
	}
	for _, k := range p.Skills.Remove {
		d.removes[k] = true
	}
	diffs[SectionCommandsSkills] = d

	return diffs
}

// diffsToProfile converts per-section diffs into a profiles.Profile struct.
// This is the value-receiver version that computes diffs inline from picker state,
// suitable for use in buildProfiles (which also uses a value receiver).
func (m Model) diffsToProfile(name string) profiles.Profile {
	p := profiles.Profile{}

	pm := m.profilePickers[name]

	// Plugins
	basePlugins := m.pickers[SectionPlugins].SelectedKeys()
	profPlugins := pm[SectionPlugins].SelectedKeys()
	pluginDiff := computeSectionDiff(basePlugins, profPlugins)
	p.Plugins.Add = sortedKeys(pluginDiff.adds)
	p.Plugins.Remove = sortedKeys(pluginDiff.removes)

	// Settings (only adds, look up values from scan)
	baseSettings := m.pickers[SectionSettings].SelectedKeys()
	profSettings := pm[SectionSettings].SelectedKeys()
	settingsDiff := computeSectionDiff(baseSettings, profSettings)
	if len(settingsDiff.adds) > 0 {
		p.Settings = make(map[string]any)
		for k := range settingsDiff.adds {
			if v, ok := m.scanResult.Settings[k]; ok {
				p.Settings[k] = v
			}
		}
	}

	// Permissions (only adds, split by prefix)
	basePerms := m.pickers[SectionPermissions].SelectedKeys()
	profPerms := pm[SectionPermissions].SelectedKeys()
	permsDiff := computeSectionDiff(basePerms, profPerms)
	for _, k := range sortedKeys(permsDiff.adds) {
		if strings.HasPrefix(k, "allow:") {
			p.Permissions.AddAllow = append(p.Permissions.AddAllow, strings.TrimPrefix(k, "allow:"))
		} else if strings.HasPrefix(k, "deny:") {
			p.Permissions.AddDeny = append(p.Permissions.AddDeny, strings.TrimPrefix(k, "deny:"))
		}
	}

	// CLAUDE.md
	if profPreview, ok := m.profilePreviews[name]; ok {
		baseFrags := m.preview.SelectedFragmentKeys()
		profFrags := profPreview.SelectedFragmentKeys()
		claudeDiff := computeSectionDiff(baseFrags, profFrags)
		p.ClaudeMD.Add = sortedKeys(claudeDiff.adds)
		p.ClaudeMD.Remove = sortedKeys(claudeDiff.removes)
	}

	// MCP (adds need raw JSON values)
	baseMCP := m.pickers[SectionMCP].SelectedKeys()
	profMCP := pm[SectionMCP].SelectedKeys()
	mcpDiff := computeSectionDiff(baseMCP, profMCP)
	mcpAdd := make(map[string]json.RawMessage)
	for k := range mcpDiff.adds {
		if raw, ok := m.scanResult.MCP[k]; ok {
			mcpAdd[k] = raw
		} else if raw, ok := m.discoveredMCP[k]; ok {
			mcpAdd[k] = raw
		}
	}
	// Strip secrets from profile MCP additions (same treatment as base config).
	if len(mcpAdd) > 0 {
		profileSecrets := commands.DetectMCPSecrets(mcpAdd)
		if len(profileSecrets) > 0 {
			mcpAdd = commands.ReplaceSecrets(mcpAdd, profileSecrets)
		}
		mcpAdd = commands.NormalizeMCPPaths(mcpAdd)
	}
	mcpRemoves := sortedKeys(mcpDiff.removes)
	if len(mcpAdd) > 0 || len(mcpRemoves) > 0 {
		p.MCP = profiles.ProfileMCP{Add: mcpAdd, Remove: mcpRemoves}
	}

	// Hooks (adds need raw JSON values)
	baseHooks := m.pickers[SectionHooks].SelectedKeys()
	profHooks := pm[SectionHooks].SelectedKeys()
	hooksDiff := computeSectionDiff(baseHooks, profHooks)
	hookAdd := make(map[string]json.RawMessage)
	for k := range hooksDiff.adds {
		if raw, ok := m.scanResult.Hooks[k]; ok {
			hookAdd[k] = raw
		}
	}
	hookRemoves := sortedKeys(hooksDiff.removes)
	if len(hookAdd) > 0 || len(hookRemoves) > 0 {
		p.Hooks = profiles.ProfileHooks{Add: hookAdd, Remove: hookRemoves}
	}

	// Keybindings
	baseKB := m.pickers[SectionKeybindings].SelectedKeys()
	profKB := pm[SectionKeybindings].SelectedKeys()
	if !setsEqual(toSet(baseKB), toSet(profKB)) {
		if len(profKB) > 0 {
			p.Keybindings = profiles.ProfileKeybindings{
				Override: m.scanResult.Keybindings,
			}
		}
	}

	// Commands & Skills
	baseCS := m.pickers[SectionCommandsSkills].SelectedKeys()
	profCS := pm[SectionCommandsSkills].SelectedKeys()
	baseCmds, baseSkills := splitCmdSkillKeys(baseCS)
	profCmds, profSkills := splitCmdSkillKeys(profCS)

	cmdDiff := computeSectionDiff(baseCmds, profCmds)
	p.Commands.Add = sortedKeys(cmdDiff.adds)
	p.Commands.Remove = sortedKeys(cmdDiff.removes)

	skillDiff := computeSectionDiff(baseSkills, profSkills)
	p.Skills.Add = sortedKeys(skillDiff.adds)
	p.Skills.Remove = sortedKeys(skillDiff.removes)

	return p
}
