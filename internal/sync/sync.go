package sync

import "fmt"

// PluginDiff represents the difference between desired and installed plugins.
type PluginDiff struct {
	Synced    []string // In both desired and installed
	ToInstall []string // In desired but not installed
	ToRemove  []string // In installed but not desired (only used in exact mode)
	Untracked []string // In installed but not desired (informational in union mode)
}

// SettingsDiff represents differences in settings.
type SettingsDiff struct {
	Changed map[string]SettingChange
}

// SettingChange represents a single setting difference.
type SettingChange struct {
	Desired any
	Current any
}

// ComputePluginDiff computes the difference between desired and installed plugin lists.
func ComputePluginDiff(desired, installed []string) PluginDiff {
	desiredSet := toSet(desired)
	installedSet := toSet(installed)

	var diff PluginDiff

	for _, p := range desired {
		if installedSet[p] {
			diff.Synced = append(diff.Synced, p)
		} else {
			diff.ToInstall = append(diff.ToInstall, p)
		}
	}

	for _, p := range installed {
		if !desiredSet[p] {
			diff.Untracked = append(diff.Untracked, p)
		}
	}

	return diff
}

// ApplyPluginPreferences applies user preferences to a desired plugin list.
func ApplyPluginPreferences(desired, unsubscribe, personal []string) []string {
	unsub := toSet(unsubscribe)
	var result []string
	for _, p := range desired {
		if !unsub[p] {
			result = append(result, p)
		}
	}
	result = append(result, personal...)
	return result
}

// ComputeSettingsDiff computes differences between desired and current settings.
func ComputeSettingsDiff(desired, current map[string]any) SettingsDiff {
	diff := SettingsDiff{Changed: make(map[string]SettingChange)}

	for key, desiredVal := range desired {
		currentVal, exists := current[key]
		if !exists || fmt.Sprintf("%v", desiredVal) != fmt.Sprintf("%v", currentVal) {
			diff.Changed[key] = SettingChange{
				Desired: desiredVal,
				Current: currentVal,
			}
		}
	}

	return diff
}

func toSet(items []string) map[string]bool {
	s := make(map[string]bool, len(items))
	for _, item := range items {
		s[item] = true
	}
	return s
}
