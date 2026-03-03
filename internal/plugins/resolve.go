package plugins

import (
	"strings"
	"time"
)

// Resolution captures a user's decision about which duplicate source to keep.
type Resolution struct {
	PluginName   string
	KeepSource   string // full key to keep enabled
	RemoveSource string // full key to disable
	Relationship string // "active-dev" or "preference"
	LocalRepo    string // path to local git repo (only for active-dev)
}

// ApplyResolution disables the removed source and records the decision.
func ApplyResolution(claudeDir, syncDir string, r Resolution) error {
	if err := ToggleEnabledPlugin(claudeDir, r.RemoveSource, false); err != nil {
		return err
	}

	sources, err := ReadPluginSources(syncDir)
	if err != nil {
		return err
	}

	sources.Plugins[r.PluginName] = PluginSourceEntry{
		ActiveSource: sourceFromKey(r.KeepSource),
		Suppressed:   sourceFromKey(r.RemoveSource),
		Relationship: r.Relationship,
		LocalRepo:    r.LocalRepo,
		DecidedAt:    time.Now(),
	}

	return WritePluginSources(syncDir, sources)
}

// ApplyReEvalSwitch switches a tracked plugin from local back to marketplace.
func ApplyReEvalSwitch(claudeDir, syncDir, pluginName string) error {
	sources, err := ReadPluginSources(syncDir)
	if err != nil {
		return err
	}

	entry, ok := sources.Plugins[pluginName]
	if !ok {
		return nil
	}

	// Re-enable marketplace, disable local
	marketplaceKey := pluginName + "@" + entry.Suppressed
	localKey := pluginName + "@" + entry.ActiveSource

	if err := ToggleEnabledPlugin(claudeDir, marketplaceKey, true); err != nil {
		return err
	}
	if err := ToggleEnabledPlugin(claudeDir, localKey, false); err != nil {
		return err
	}

	// Update tracking: swap active/suppressed, clear relationship
	sources.Plugins[pluginName] = PluginSourceEntry{
		ActiveSource: entry.Suppressed,
		Suppressed:   entry.ActiveSource,
		Relationship: "preference",
		DecidedAt:    time.Now(),
	}

	return WritePluginSources(syncDir, sources)
}

// ApplySnooze sets a snooze for N days on a tracked plugin.
func ApplySnooze(syncDir, pluginName string, days int) error {
	sources, err := ReadPluginSources(syncDir)
	if err != nil {
		return err
	}

	entry, ok := sources.Plugins[pluginName]
	if !ok {
		return nil
	}

	snooze := time.Now().AddDate(0, 0, days)
	entry.SnoozeUntil = &snooze
	sources.Plugins[pluginName] = entry

	return WritePluginSources(syncDir, sources)
}

// ResetReEvalBaseline updates tracking timestamps so re-evaluation
// doesn't fire again until the next change occurs.
func ResetReEvalBaseline(syncDir, pluginName string) error {
	sources, err := ReadPluginSources(syncDir)
	if err != nil {
		return err
	}

	entry, ok := sources.Plugins[pluginName]
	if !ok {
		return nil
	}

	entry.LastLocalCommitAtDecision = time.Now()
	entry.SnoozeUntil = nil
	sources.Plugins[pluginName] = entry

	return WritePluginSources(syncDir, sources)
}

// sourceFromKey extracts the source portion from a "name@source" key.
func sourceFromKey(key string) string {
	if idx := strings.LastIndex(key, "@"); idx > 0 {
		return key[idx+1:]
	}
	return key
}
