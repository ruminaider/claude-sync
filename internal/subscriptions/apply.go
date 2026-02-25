package subscriptions

import (
	"encoding/json"

	"github.com/ruminaider/claude-sync/internal/config"
)

// ApplyToConfig merges subscription items into the local config.
// Local items always take precedence — subscription items are additive.
func ApplyToConfig(localCfg *config.Config, merged *MergedResult) {
	// Plugins — add subscription plugins to upstream (if not already present).
	localPluginSet := make(map[string]bool)
	for _, k := range localCfg.AllPluginKeys() {
		localPluginSet[k] = true
	}
	for _, p := range merged.Plugins {
		if !localPluginSet[p] {
			localCfg.Upstream = append(localCfg.Upstream, p)
		}
	}

	// Settings — overlay subscription settings (local keys win).
	if localCfg.Settings == nil && len(merged.Settings) > 0 {
		localCfg.Settings = make(map[string]any)
	}
	for k, v := range merged.Settings {
		if _, exists := localCfg.Settings[k]; !exists {
			localCfg.Settings[k] = v
		}
	}

	// MCP — overlay subscription MCP servers (local keys win).
	if localCfg.MCP == nil && len(merged.MCP) > 0 {
		localCfg.MCP = make(map[string]json.RawMessage)
	}
	for k, v := range merged.MCP {
		if _, exists := localCfg.MCP[k]; !exists {
			localCfg.MCP[k] = v
		}
	}

	// Hooks — overlay subscription hooks (local keys win).
	if localCfg.Hooks == nil && len(merged.Hooks) > 0 {
		localCfg.Hooks = make(map[string]json.RawMessage)
	}
	for k, v := range merged.Hooks {
		if _, exists := localCfg.Hooks[k]; !exists {
			localCfg.Hooks[k] = v
		}
	}

	// Permissions — additive merge.
	localCfg.Permissions.Allow = mergeStringSlices(localCfg.Permissions.Allow, merged.Permissions.Allow)
	localCfg.Permissions.Deny = mergeStringSlices(localCfg.Permissions.Deny, merged.Permissions.Deny)

	// CLAUDE.md fragments — additive.
	localCfg.ClaudeMD.Include = mergeStringSlices(localCfg.ClaudeMD.Include, merged.ClaudeMD)

	// Commands — additive.
	localCfg.Commands = mergeStringSlices(localCfg.Commands, merged.Commands)

	// Skills — additive.
	localCfg.Skills = mergeStringSlices(localCfg.Skills, merged.Skills)
}

// mergeStringSlices appends items from add to base, skipping duplicates.
func mergeStringSlices(base, add []string) []string {
	if len(add) == 0 {
		return base
	}
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	result := make([]string, len(base))
	copy(result, base)
	for _, s := range add {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
