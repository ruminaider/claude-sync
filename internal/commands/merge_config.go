package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/cmdskill"
	"github.com/ruminaider/claude-sync/internal/config"
	forkedplugins "github.com/ruminaider/claude-sync/internal/plugins"
)

// MergeExistingConfig injects items from an existing config into the scan
// result when those items were not detected locally. This prevents
// "config update" from silently dropping items that exist in the config but
// are absent from the current machine (e.g., plugins not installed locally).
//
// Each injected item is tracked in scan.ConfigOnly so the TUI can label it.
func MergeExistingConfig(scan *InitScanResult, cfg *config.Config, syncDir string) {
	if scan.ConfigOnly == nil {
		scan.ConfigOnly = make(map[string]bool)
	}

	mergeUpstreamPlugins(scan, cfg)
	mergeForkedPlugins(scan, cfg)
	mergeSettings(scan, cfg)
	mergeHooks(scan, cfg)
	mergePermissions(scan, cfg)
	mergeMCP(scan, cfg)
	mergeKeybindings(scan, cfg)
	mergeClaudeMDFragments(scan, cfg, syncDir)
	mergeCommandsSkills(scan, cfg, syncDir)
}

// mergeUpstreamPlugins injects upstream plugin keys from cfg that are missing
// from the scan.
func mergeUpstreamPlugins(scan *InitScanResult, cfg *config.Config) {
	for _, key := range cfg.Upstream {
		if slices.Contains(scan.PluginKeys, key) {
			continue
		}
		scan.Upstream = append(scan.Upstream, key)
		scan.PluginKeys = append(scan.PluginKeys, key)
		scan.ConfigOnly[key] = true
	}
}

// mergeForkedPlugins injects forked plugin names from cfg (bare names) that
// are missing from the scan, converting them to qualified keys.
func mergeForkedPlugins(scan *InitScanResult, cfg *config.Config) {
	for _, name := range cfg.Forked {
		key := name + "@" + forkedplugins.MarketplaceName
		if slices.Contains(scan.PluginKeys, key) {
			continue
		}
		scan.AutoForked = append(scan.AutoForked, key)
		scan.PluginKeys = append(scan.PluginKeys, key)
		scan.ConfigOnly[key] = true
	}
}

// mergeSettings injects setting keys from cfg that are missing from the scan.
func mergeSettings(scan *InitScanResult, cfg *config.Config) {
	if len(cfg.Settings) == 0 {
		return
	}
	if scan.Settings == nil {
		scan.Settings = make(map[string]any)
	}
	for k, v := range cfg.Settings {
		if _, exists := scan.Settings[k]; exists {
			continue
		}
		scan.Settings[k] = v
		scan.ConfigOnly[k] = true
	}
}

// mergeHooks injects hooks from cfg that are missing from the scan.
func mergeHooks(scan *InitScanResult, cfg *config.Config) {
	if len(cfg.Hooks) == 0 {
		return
	}
	if scan.Hooks == nil {
		scan.Hooks = make(map[string]json.RawMessage)
	}
	for k, v := range cfg.Hooks {
		if _, exists := scan.Hooks[k]; exists {
			continue
		}
		scan.Hooks[k] = v
		scan.ConfigOnly[k] = true
	}
}

// mergePermissions injects allow/deny rules from cfg that are missing from
// the scan. Keys in ConfigOnly use "allow:"+rule and "deny:"+rule prefixes
// to match the TUI picker convention.
func mergePermissions(scan *InitScanResult, cfg *config.Config) {
	for _, rule := range cfg.Permissions.Allow {
		if slices.Contains(scan.Permissions.Allow, rule) {
			continue
		}
		scan.Permissions.Allow = append(scan.Permissions.Allow, rule)
		scan.ConfigOnly["allow:"+rule] = true
	}
	for _, rule := range cfg.Permissions.Deny {
		if slices.Contains(scan.Permissions.Deny, rule) {
			continue
		}
		scan.Permissions.Deny = append(scan.Permissions.Deny, rule)
		scan.ConfigOnly["deny:"+rule] = true
	}
}

// mergeMCP injects MCP server configs from cfg that are missing from the scan.
func mergeMCP(scan *InitScanResult, cfg *config.Config) {
	if len(cfg.MCP) == 0 {
		return
	}
	if scan.MCP == nil {
		scan.MCP = make(map[string]json.RawMessage)
	}
	for k, v := range cfg.MCP {
		if _, exists := scan.MCP[k]; exists {
			continue
		}
		scan.MCP[k] = v
		scan.ConfigOnly[k] = true
	}
}

// mergeKeybindings injects keybindings from cfg that are missing from the scan.
func mergeKeybindings(scan *InitScanResult, cfg *config.Config) {
	if len(cfg.Keybindings) == 0 {
		return
	}
	if scan.Keybindings == nil {
		scan.Keybindings = make(map[string]any)
	}
	for k, v := range cfg.Keybindings {
		if _, exists := scan.Keybindings[k]; exists {
			continue
		}
		scan.Keybindings[k] = v
		scan.ConfigOnly[k] = true
	}
}

// mergeClaudeMDFragments injects CLAUDE.md fragment sections from the sync
// dir's claude-md/ directory when the config references fragments not present
// in the scan. Uses "fragment:" prefix for ConfigOnly keys to avoid collisions.
func mergeClaudeMDFragments(scan *InitScanResult, cfg *config.Config, syncDir string) {
	if len(cfg.ClaudeMD.Include) == 0 {
		return
	}

	// Build a set of fragment keys already present in the scan.
	existing := make(map[string]bool, len(scan.ClaudeMDSections))
	for _, sec := range scan.ClaudeMDSections {
		key := claudemd.HeaderToFragmentName(sec.Header)
		existing[key] = true
	}

	claudeMdDir := filepath.Join(syncDir, "claude-md")

	for _, fragKey := range cfg.ClaudeMD.Include {
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

// mergeCommandsSkills injects commands and skills from the config that are
// missing from the scan. It reads content from the sync dir when available,
// falling back to a placeholder when the file doesn't exist locally.
func mergeCommandsSkills(scan *InitScanResult, cfg *config.Config, syncDir string) {
	allKeys := append(cfg.Commands, cfg.Skills...)
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
