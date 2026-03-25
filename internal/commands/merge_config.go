package commands

import (
	"encoding/json"
	"slices"

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
