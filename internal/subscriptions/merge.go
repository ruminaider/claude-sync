package subscriptions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/config"
)

// MergedResult holds the merged output from all subscriptions.
type MergedResult struct {
	Plugins     []string                      // plugin keys to add
	Settings    map[string]any                // setting key -> value
	MCP         map[string]json.RawMessage    // server name -> config
	Hooks       map[string]json.RawMessage    // hook name -> config
	Permissions config.Permissions
	ClaudeMD    []string                      // fragment names to add
	Commands    []string                      // command keys
	Skills      []string                      // skill keys
	Provenance  map[string]map[string]string  // category -> item -> source subscription name
}

// MergeAll merges items from all subscriptions according to their filters.
// Returns the merged result and any conflicts found.
// Local config items always win (they are excluded from conflict detection).
func MergeAll(syncDir string, subs map[string]config.SubscriptionEntry, localCfg config.Config) (*MergedResult, []Conflict, error) {
	result := &MergedResult{
		Settings:   make(map[string]any),
		MCP:        make(map[string]json.RawMessage),
		Hooks:      make(map[string]json.RawMessage),
		Provenance: make(map[string]map[string]string),
	}
	var conflicts []Conflict

	// Track which subscription owns each item for conflict detection.
	type itemOwner struct {
		source string
		value  json.RawMessage
	}
	mcpOwners := make(map[string]itemOwner)
	settingsOwners := make(map[string]itemOwner)
	hooksOwners := make(map[string]itemOwner)
	pluginsSet := make(map[string]string) // plugin -> source

	// Sort subscription names for deterministic processing.
	subNames := make([]string, 0, len(subs))
	for name := range subs {
		subNames = append(subNames, name)
	}
	sort.Strings(subNames)

	// Build local item sets for "local wins" exclusion.
	localMCPSet := make(map[string]bool, len(localCfg.MCP))
	for k := range localCfg.MCP {
		localMCPSet[k] = true
	}
	localSettingsSet := make(map[string]bool, len(localCfg.Settings))
	for k := range localCfg.Settings {
		localSettingsSet[k] = true
	}
	localHooksSet := make(map[string]bool, len(localCfg.Hooks))
	for k := range localCfg.Hooks {
		localHooksSet[k] = true
	}
	localPluginsSet := make(map[string]bool)
	for _, k := range localCfg.AllPluginKeys() {
		localPluginsSet[k] = true
	}

	for _, subName := range subNames {
		sub := subs[subName]
		subDir := SubDir(syncDir, subName)

		// Read the subscription's config.yaml.
		remoteCfg, err := readRemoteConfig(subDir)
		if err != nil {
			return nil, nil, fmt.Errorf("reading config for subscription %q: %w", subName, err)
		}

		// Convert subscription entry to filter-compatible Subscription type.
		filterSub := toFilterSubscription(sub)

		// --- MCP ---
		mcpNames := mapKeys(remoteCfg.MCP)
		selectedMCP := ResolveItems(filterSub, "mcp", mcpNames)
		for name := range selectedMCP {
			if localMCPSet[name] {
				continue // local wins
			}
			val := remoteCfg.MCP[name]
			if prev, exists := mcpOwners[name]; exists {
				// Conflict — check prefer directives.
				winner := resolveConflict(subName, prev.source, sub, subs[prev.source], "mcp", name)
				if winner == "" {
					conflicts = append(conflicts, Conflict{
						Category: "mcp",
						ItemName: name,
						SourceA:  prev.source,
						SourceB:  subName,
					})
					continue
				}
				if winner == subName {
					mcpOwners[name] = itemOwner{source: subName, value: val}
				}
				// else prev winner keeps it
			} else {
				mcpOwners[name] = itemOwner{source: subName, value: val}
			}
		}

		// --- Plugins ---
		pluginNames := remoteCfg.AllPluginKeys()
		selectedPlugins := ResolveItems(filterSub, "plugins", pluginNames)
		for name := range selectedPlugins {
			if localPluginsSet[name] {
				continue
			}
			if _, exists := pluginsSet[name]; !exists {
				pluginsSet[name] = subName
			}
			// Plugins don't really conflict (same key = same plugin), so first wins.
		}

		// --- Settings ---
		settingsNames := mapKeysAny(remoteCfg.Settings)
		selectedSettings := ResolveItems(filterSub, "settings", settingsNames)
		for name := range selectedSettings {
			if localSettingsSet[name] {
				continue
			}
			valJSON, _ := json.Marshal(remoteCfg.Settings[name])
			if prev, exists := settingsOwners[name]; exists {
				winner := resolveConflict(subName, prev.source, sub, subs[prev.source], "settings", name)
				if winner == "" {
					if string(valJSON) != string(prev.value) {
						conflicts = append(conflicts, Conflict{
							Category: "settings",
							ItemName: name,
							SourceA:  prev.source,
							SourceB:  subName,
						})
					}
					continue
				}
				if winner == subName {
					settingsOwners[name] = itemOwner{source: subName, value: valJSON}
				}
			} else {
				settingsOwners[name] = itemOwner{source: subName, value: valJSON}
			}
		}

		// --- Hooks ---
		hookNames := mapKeysRaw(remoteCfg.Hooks)
		selectedHooks := ResolveItems(filterSub, "hooks", hookNames)
		for name := range selectedHooks {
			if localHooksSet[name] {
				continue
			}
			val := remoteCfg.Hooks[name]
			if prev, exists := hooksOwners[name]; exists {
				winner := resolveConflict(subName, prev.source, sub, subs[prev.source], "hooks", name)
				if winner == "" {
					if string(val) != string(prev.value) {
						conflicts = append(conflicts, Conflict{
							Category: "hooks",
							ItemName: name,
							SourceA:  prev.source,
							SourceB:  subName,
						})
					}
					continue
				}
				if winner == subName {
					hooksOwners[name] = itemOwner{source: subName, value: val}
				}
			} else {
				hooksOwners[name] = itemOwner{source: subName, value: val}
			}
		}

		// --- CLAUDE.md ---
		claudeMDNames := remoteCfg.ClaudeMD.Include
		selectedClaudeMD := ResolveItems(filterSub, "claude_md", claudeMDNames)
		for name := range selectedClaudeMD {
			result.ClaudeMD = appendUnique(result.ClaudeMD, name)
		}

		// --- Commands ---
		selectedCommands := ResolveItems(filterSub, "commands", remoteCfg.Commands)
		for name := range selectedCommands {
			result.Commands = appendUnique(result.Commands, name)
		}

		// --- Skills ---
		selectedSkills := ResolveItems(filterSub, "skills", remoteCfg.Skills)
		for name := range selectedSkills {
			result.Skills = appendUnique(result.Skills, name)
		}

		// --- Permissions (additive, no conflict) ---
		permAllowNames := remoteCfg.Permissions.Allow
		selectedAllows := ResolveItems(filterSub, "permissions", permAllowNames)
		for name := range selectedAllows {
			result.Permissions.Allow = appendUnique(result.Permissions.Allow, name)
		}
		permDenyNames := remoteCfg.Permissions.Deny
		selectedDenies := ResolveItems(filterSub, "permissions", permDenyNames)
		for name := range selectedDenies {
			result.Permissions.Deny = appendUnique(result.Permissions.Deny, name)
		}
	}

	// Build final results from owners.
	result.Provenance["mcp"] = make(map[string]string)
	for name, owner := range mcpOwners {
		result.MCP[name] = owner.value
		result.Provenance["mcp"][name] = owner.source
	}

	result.Provenance["settings"] = make(map[string]string)
	for name, owner := range settingsOwners {
		var val any
		json.Unmarshal(owner.value, &val)
		result.Settings[name] = val
		result.Provenance["settings"][name] = owner.source
	}

	result.Provenance["hooks"] = make(map[string]string)
	for name, owner := range hooksOwners {
		result.Hooks[name] = owner.value
		result.Provenance["hooks"][name] = owner.source
	}

	result.Provenance["plugins"] = make(map[string]string)
	for name, source := range pluginsSet {
		result.Plugins = append(result.Plugins, name)
		result.Provenance["plugins"][name] = source
	}
	sort.Strings(result.Plugins)

	return result, conflicts, nil
}

// resolveConflict uses prefer directives to determine which subscription wins.
// Returns the winning subscription name, or "" if unresolved.
func resolveConflict(subA, subB string, entryA, entryB config.SubscriptionEntry, category, itemName string) string {
	aPrefers := isPreferredEntry(entryA, category, itemName)
	bPrefers := isPreferredEntry(entryB, category, itemName)

	if aPrefers && bPrefers {
		return "" // both claim prefer — config error, fail loudly
	}
	if aPrefers {
		return subA
	}
	if bPrefers {
		return subB
	}
	return "" // no prefer — unresolved
}

func isPreferredEntry(entry config.SubscriptionEntry, category, itemName string) bool {
	items, ok := entry.Prefer[category]
	if !ok {
		return false
	}
	for _, name := range items {
		if name == itemName {
			return true
		}
	}
	return false
}

// toFilterSubscription converts a config.SubscriptionEntry to a subscriptions.Subscription
// for use with the filter functions.
func toFilterSubscription(entry config.SubscriptionEntry) Subscription {
	sub := Subscription{
		URL:     entry.URL,
		Ref:     entry.Ref,
		Exclude: entry.Exclude,
		Include: entry.Include,
		Prefer:  entry.Prefer,
	}
	// Parse categories from the flexible map.
	if entry.Categories != nil {
		sub.Categories = parseCategories(entry.Categories)
	}
	return sub
}

func parseCategories(cats map[string]any) SubscriptionCategories {
	var sc SubscriptionCategories
	for key, val := range cats {
		mode := CategoryNone
		if s, ok := val.(string); ok && s == "all" {
			mode = CategoryAll
		}
		switch key {
		case "mcp":
			sc.MCP = mode
		case "plugins":
			sc.Plugins = mode
		case "settings":
			sc.Settings = mode
		case "hooks":
			sc.Hooks = mode
		case "permissions":
			sc.Permissions = mode
		case "claude_md":
			sc.ClaudeMD = mode
		case "commands":
			if s, ok := val.(string); ok {
				sc.Commands = &SubscriptionCommandsMode{Mode: CategoryMode(s)}
			}
		case "skills":
			sc.Skills = mode
		}
	}
	return sc
}

// readRemoteConfig reads and parses the config.yaml from a subscription's local clone.
func readRemoteConfig(subDir string) (config.Config, error) {
	cfgPath := filepath.Join(subDir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return config.Config{}, err
	}
	return config.Parse(data)
}

// Helper functions for extracting map keys.
func mapKeys(m map[string]json.RawMessage) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func mapKeysAny(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

func mapKeysRaw(m map[string]json.RawMessage) []string {
	return mapKeys(m) // same signature
}

func appendUnique(slice []string, item string) []string {
	for _, s := range slice {
		if s == item {
			return slice
		}
	}
	return append(slice, item)
}

// IncrementalDiff computes which items from a subscription are new (not previously accepted).
func IncrementalDiff(subName string, sub config.SubscriptionEntry, syncDir string, state SubscriptionState) (newItems map[string][]string, err error) {
	subDir := SubDir(syncDir, subName)
	remoteCfg, err := readRemoteConfig(subDir)
	if err != nil {
		return nil, fmt.Errorf("reading config for %q: %w", subName, err)
	}

	prevState, hasPrev := state.Subscriptions[subName]
	filterSub := toFilterSubscription(sub)

	newItems = make(map[string][]string)

	// Check each category for new items.
	categories := map[string][]string{
		"mcp":      mapKeys(remoteCfg.MCP),
		"plugins":  remoteCfg.AllPluginKeys(),
		"settings": mapKeysAny(remoteCfg.Settings),
		"hooks":    mapKeysRaw(remoteCfg.Hooks),
	}

	for category, allNames := range categories {
		selected := ResolveItems(filterSub, category, allNames)
		accepted := make(map[string]bool)
		if hasPrev {
			for _, name := range prevState.AcceptedItems[category] {
				accepted[name] = true
			}
		}
		for name := range selected {
			if !accepted[name] {
				newItems[category] = append(newItems[category], name)
			}
		}
		sort.Strings(newItems[category])
	}

	// Remove empty categories.
	for k, v := range newItems {
		if len(v) == 0 {
			delete(newItems, k)
		}
	}

	return newItems, nil
}

// FormatConflicts returns a human-readable string describing all conflicts.
func FormatConflicts(conflicts []Conflict) string {
	if len(conflicts) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("Subscription conflicts detected:\n")
	for _, c := range conflicts {
		fmt.Fprintf(&b, "  - %s\n", c)
	}
	b.WriteString("\nResolve by adding 'prefer' directives to the winning subscription in config.yaml.")
	return b.String()
}
