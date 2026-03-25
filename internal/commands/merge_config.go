package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/cmdskill"
	"github.com/ruminaider/claude-sync/internal/config"
	forkedplugins "github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// ConfigOnly key prefixes. Each section namespaces its keys to prevent
// collisions (e.g., a setting and MCP server sharing the same name).
// Permissions use "allow:"/"deny:" prefixes. Commands/skills use their
// existing "cmd:"/"skill:" key format. CLAUDE.md uses "fragment:".
const (
	configOnlyPluginPrefix     = "plugin:"
	configOnlySettingPrefix    = "setting:"
	configOnlyHookPrefix       = "hook:"
	configOnlyMCPPrefix        = "mcp:"
	configOnlyKeybindingPrefix = "keybinding:"
	configOnlyMemoryPrefix     = "memory:"
)

// ConfigOnlySectionPrefix maps TUI section identifiers to their ConfigOnly
// key prefix. Sections whose keys are already namespaced (permissions,
// commands/skills, CLAUDE.md) use an empty prefix.
var ConfigOnlySectionPrefix = map[string]string{
	"plugins":         configOnlyPluginPrefix,
	"settings":        configOnlySettingPrefix,
	"hooks":           configOnlyHookPrefix,
	"permissions":     "",
	"mcp":             configOnlyMCPPrefix,
	"keybindings":     configOnlyKeybindingPrefix,
	"commands_skills": "",
	"memory":          configOnlyMemoryPrefix,
}

// MergeExisting injects config-only items from the base config and profile-add
// sections into the scan, so they appear in TUI pickers with [config] tags.
// Base config items are processed first, then profile-add items. This ordering
// ensures profile dedup checks see base items that were already injected.
func MergeExisting(scan *InitScanResult, cfg *config.Config, existingProfiles map[string]profiles.Profile, syncDir string) {
	if scan.ConfigOnly == nil {
		scan.ConfigOnly = make(map[string]bool)
	}

	// Phase 1: Base config items.
	if cfg != nil {
		mergeUpstreamPlugins(scan, cfg.Upstream)
		mergeForkedPlugins(scan, cfg.Forked)
		mergeSettings(scan, cfg.Settings)
		mergeHooks(scan, cfg.Hooks)
		mergePermissions(scan, cfg.Permissions.Allow, cfg.Permissions.Deny)
		mergeMCP(scan, cfg.MCP)
		mergeKeybindings(scan, cfg.Keybindings)
		mergeClaudeMDFragments(scan, cfg.ClaudeMD.Include, syncDir)
		mergeCommandsSkills(scan, cfg.Commands, cfg.Skills, syncDir)
		mergeMemory(scan, cfg.Memory.Include)
	}

	// Phase 2: Profile-add items. Sort profile names for deterministic output
	// when two profiles add the same key with different values.
	profileNames := make([]string, 0, len(existingProfiles))
	for name := range existingProfiles {
		profileNames = append(profileNames, name)
	}
	sort.Strings(profileNames)
	for _, pname := range profileNames {
		profile := existingProfiles[pname]
		// Route profile plugins to upstream or forked based on marketplace suffix.
		var upstreamKeys, forkedKeys []string
		for _, key := range profile.Plugins.Add {
			name, mkt := splitPluginKey(key)
			if mkt == forkedplugins.MarketplaceName {
				// Forked plugins store the bare name (without marketplace suffix)
				// in the config. mergeForkedPlugins re-appends the suffix.
				forkedKeys = append(forkedKeys, name)
			} else {
				upstreamKeys = append(upstreamKeys, key)
			}
		}
		mergeUpstreamPlugins(scan, upstreamKeys)
		mergeForkedPlugins(scan, forkedKeys)

		mergeSettings(scan, profile.Settings)
		mergeHooks(scan, profile.Hooks.Add)
		mergePermissions(scan, profile.Permissions.AddAllow, profile.Permissions.AddDeny)
		mergeMCP(scan, profile.MCP.Add)
		mergeKeybindings(scan, profile.Keybindings.Override)
		mergeClaudeMDFragments(scan, profile.ClaudeMD.Add, syncDir)
		mergeCommandsSkills(scan, profile.Commands.Add, profile.Skills.Add, syncDir)
		mergeMemory(scan, profile.Memory.Add)
	}
}

// MergeExistingConfig injects items from an existing config into the scan
// result when those items were not detected locally. This prevents
// "config update" from silently dropping items that exist in the config but
// are absent from the current machine (e.g., plugins not installed locally).
//
// Each injected item is tracked in scan.ConfigOnly so the TUI can label it.
// Keys are prefixed by section to prevent cross-section collisions.
//
// Deprecated: Use MergeExisting instead. This delegates to MergeExisting
// with nil profiles for backward compatibility.
func MergeExistingConfig(scan *InitScanResult, cfg *config.Config, syncDir string) {
	MergeExisting(scan, cfg, nil, syncDir)
}

func mergeUpstreamPlugins(scan *InitScanResult, keys []string) {
	for _, key := range keys {
		if slices.Contains(scan.PluginKeys, key) {
			continue
		}
		scan.Upstream = append(scan.Upstream, key)
		scan.PluginKeys = append(scan.PluginKeys, key)
		scan.ConfigOnly[configOnlyPluginPrefix+key] = true
	}
}

func mergeForkedPlugins(scan *InitScanResult, names []string) {
	for _, name := range names {
		key := name + "@" + forkedplugins.MarketplaceName
		if slices.Contains(scan.PluginKeys, key) {
			continue
		}
		scan.AutoForked = append(scan.AutoForked, key)
		scan.PluginKeys = append(scan.PluginKeys, key)
		scan.ConfigOnly[configOnlyPluginPrefix+key] = true
	}
}

func mergeSettings(scan *InitScanResult, settings map[string]any) {
	if len(settings) == 0 {
		return
	}
	if scan.Settings == nil {
		scan.Settings = make(map[string]any)
	}
	for k, v := range settings {
		if _, exists := scan.Settings[k]; exists {
			continue
		}
		scan.Settings[k] = v
		scan.ConfigOnly[configOnlySettingPrefix+k] = true
	}
}

func mergeHooks(scan *InitScanResult, hooks map[string]json.RawMessage) {
	if len(hooks) == 0 {
		return
	}
	if scan.Hooks == nil {
		scan.Hooks = make(map[string]json.RawMessage)
	}
	for k, v := range hooks {
		if _, exists := scan.Hooks[k]; exists {
			continue
		}
		scan.Hooks[k] = v
		scan.ConfigOnly[configOnlyHookPrefix+k] = true
	}
}

// mergePermissions injects allow/deny rules that are missing from the scan.
// Keys in ConfigOnly use "allow:"+rule and "deny:"+rule prefixes to match
// the TUI picker convention.
func mergePermissions(scan *InitScanResult, allow []string, deny []string) {
	for _, rule := range allow {
		if slices.Contains(scan.Permissions.Allow, rule) {
			continue
		}
		scan.Permissions.Allow = append(scan.Permissions.Allow, rule)
		scan.ConfigOnly["allow:"+rule] = true
	}
	for _, rule := range deny {
		if slices.Contains(scan.Permissions.Deny, rule) {
			continue
		}
		scan.Permissions.Deny = append(scan.Permissions.Deny, rule)
		scan.ConfigOnly["deny:"+rule] = true
	}
}

func mergeMCP(scan *InitScanResult, mcp map[string]json.RawMessage) {
	if len(mcp) == 0 {
		return
	}
	if scan.MCP == nil {
		scan.MCP = make(map[string]json.RawMessage)
	}
	for k, v := range mcp {
		if _, exists := scan.MCP[k]; exists {
			continue
		}
		scan.MCP[k] = v
		scan.ConfigOnly[configOnlyMCPPrefix+k] = true
	}
}

func mergeKeybindings(scan *InitScanResult, keybindings map[string]any) {
	if len(keybindings) == 0 {
		return
	}
	if scan.Keybindings == nil {
		scan.Keybindings = make(map[string]any)
	}
	for k, v := range keybindings {
		if _, exists := scan.Keybindings[k]; exists {
			continue
		}
		scan.Keybindings[k] = v
		scan.ConfigOnly[configOnlyKeybindingPrefix+k] = true
	}
}

// mergeClaudeMDFragments injects CLAUDE.md fragment sections from the sync
// dir's claude-md/ directory when the given fragments are not present in the
// scan. Uses "fragment:" prefix for ConfigOnly keys to avoid collisions.
func mergeClaudeMDFragments(scan *InitScanResult, fragments []string, syncDir string) {
	if len(fragments) == 0 {
		return
	}

	// Build a set of fragment keys already present in the scan.
	existing := make(map[string]bool, len(scan.ClaudeMDSections))
	for _, sec := range scan.ClaudeMDSections {
		key := claudemd.HeaderToFragmentName(sec.Header)
		existing[key] = true
	}

	claudeMdDir := filepath.Join(syncDir, "claude-md")

	for _, fragKey := range fragments {
		if existing[fragKey] {
			continue
		}

		content, err := claudemd.ReadFragment(claudeMdDir, fragKey)
		if err != nil {
			// Fragment file not available locally; inject a placeholder.
			placeholder := claudemd.Section{
				Header:  fragKey,
				Content: fmt.Sprintf("## %s\n\n(fragment not available locally)", fragKey),
			}
			scan.ClaudeMDSections = append(scan.ClaudeMDSections, placeholder)
		} else {
			// Split the fragment content and append all resulting sections.
			sections := claudemd.Split(content)
			scan.ClaudeMDSections = append(scan.ClaudeMDSections, sections...)
		}

		scan.ConfigOnly["fragment:"+fragKey] = true
		existing[fragKey] = true
	}
}

// mergeCommandsSkills injects commands and skills that are missing from the
// scan. It reads content from the sync dir when available, falling back to a
// placeholder when the file doesn't exist locally.
func mergeCommandsSkills(scan *InitScanResult, commands []string, skills []string, syncDir string) {
	allKeys := make([]string, 0, len(commands)+len(skills))
	allKeys = append(allKeys, commands...)
	allKeys = append(allKeys, skills...)
	if len(allKeys) == 0 {
		return
	}

	if scan.CommandsSkills == nil {
		scan.CommandsSkills = &cmdskill.ScanResult{}
	}

	// Build set of existing keys from the scan.
	existing := make(map[string]bool, len(scan.CommandsSkills.Items))
	for _, item := range scan.CommandsSkills.Items {
		existing[item.Key()] = true
	}

	for _, key := range allKeys {
		if existing[key] {
			continue
		}

		// Parse key: "cmd:global:name" or "skill:global:name"
		parts := strings.SplitN(key, ":", 3)
		if len(parts) < 3 {
			continue
		}
		typePart, scopePart, name := parts[0], parts[1], parts[2]

		// Only handle global scope for now.
		if scopePart != "global" {
			continue
		}

		var itemType cmdskill.ItemType
		var filePath string
		switch typePart {
		case "cmd":
			itemType = cmdskill.TypeCommand
			filePath = filepath.Join(syncDir, "commands", name+".md")
		case "skill":
			itemType = cmdskill.TypeSkill
			filePath = filepath.Join(syncDir, "skills", name, "SKILL.md")
		default:
			continue
		}

		content := "(content not available locally)"
		if data, err := os.ReadFile(filePath); err == nil {
			content = string(data)
		}

		scan.CommandsSkills.Items = append(scan.CommandsSkills.Items, cmdskill.Item{
			Name:        name,
			Type:        itemType,
			Source:      cmdskill.SourceGlobal,
			SourceLabel: "global",
			Content:     content,
		})

		scan.ConfigOnly[key] = true
		existing[key] = true
	}
}

// mergeMemory injects memory fragment names that are missing from the scan.
func mergeMemory(scan *InitScanResult, memoryFiles []string) {
	for _, name := range memoryFiles {
		if slices.Contains(scan.MemoryFiles, name) {
			continue
		}
		scan.MemoryFiles = append(scan.MemoryFiles, name)
		scan.ConfigOnly[configOnlyMemoryPrefix+name] = true
	}
}
