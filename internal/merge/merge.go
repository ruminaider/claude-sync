package merge

import (
	"encoding/json"

	"github.com/ruminaider/claude-sync/internal/config"
)

// ConflictItem describes a single merge conflict.
type ConflictItem struct {
	Key         string
	LocalValue  any
	RemoteValue any
}

// MergePermissions performs a 3-way merge of permissions.
// Returns the merged result and whether a conflict was detected.
// Conflicts occur when one side adds and the other removes the same item.
func MergePermissions(base, local, remote config.Permissions) (config.Permissions, bool) {
	mergedAllow, allowConflict := mergeStringSlice(base.Allow, local.Allow, remote.Allow)
	mergedDeny, denyConflict := mergeStringSlice(base.Deny, local.Deny, remote.Deny)

	return config.Permissions{
		Allow: mergedAllow,
		Deny:  mergedDeny,
	}, allowConflict || denyConflict
}

// MergeSettings performs a 3-way merge of settings maps.
// Returns the merged result and any conflict items.
// Conflicts occur when both local and remote modify the same key to different values.
func MergeSettings(base, local, remote map[string]any) (map[string]any, []ConflictItem) {
	if base == nil {
		base = make(map[string]any)
	}
	if local == nil {
		local = make(map[string]any)
	}
	if remote == nil {
		remote = make(map[string]any)
	}

	result := make(map[string]any)
	var conflicts []ConflictItem

	// Start with base
	for k, v := range base {
		result[k] = v
	}

	// Determine local changes
	localAdded := make(map[string]any)
	localModified := make(map[string]any)
	localRemoved := make(map[string]bool)
	for k, v := range local {
		if baseV, ok := base[k]; !ok {
			localAdded[k] = v
		} else if !jsonEqual(baseV, v) {
			localModified[k] = v
		}
	}
	for k := range base {
		if _, ok := local[k]; !ok {
			localRemoved[k] = true
		}
	}

	// Determine remote changes
	remoteAdded := make(map[string]any)
	remoteModified := make(map[string]any)
	remoteRemoved := make(map[string]bool)
	for k, v := range remote {
		if baseV, ok := base[k]; !ok {
			remoteAdded[k] = v
		} else if !jsonEqual(baseV, v) {
			remoteModified[k] = v
		}
	}
	for k := range base {
		if _, ok := remote[k]; !ok {
			remoteRemoved[k] = true
		}
	}

	// Apply local additions
	for k, v := range localAdded {
		result[k] = v
	}

	// Apply remote additions
	for k, v := range remoteAdded {
		if localV, ok := localAdded[k]; ok {
			if !jsonEqual(localV, v) {
				conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: localV, RemoteValue: v})
			}
			// If same value, already applied from local
		} else {
			result[k] = v
		}
	}

	// Apply local modifications
	for k, v := range localModified {
		if remoteV, ok := remoteModified[k]; ok {
			// Both modified same key
			if !jsonEqual(v, remoteV) {
				conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: v, RemoteValue: remoteV})
			} else {
				result[k] = v // Same modification
			}
		} else if remoteRemoved[k] {
			// Local modified, remote removed — conflict
			conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: v, RemoteValue: nil})
		} else {
			result[k] = v
		}
	}

	// Apply remote modifications (skip already handled)
	for k, v := range remoteModified {
		if _, ok := localModified[k]; ok {
			continue // Already handled
		}
		if localRemoved[k] {
			// Remote modified, local removed — conflict
			conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: nil, RemoteValue: v})
		} else {
			result[k] = v
		}
	}

	// Apply local removals (skip conflicts)
	for k := range localRemoved {
		if _, ok := remoteModified[k]; ok {
			continue // Conflict already recorded
		}
		delete(result, k)
	}

	// Apply remote removals (skip conflicts)
	for k := range remoteRemoved {
		if _, ok := localModified[k]; ok {
			continue // Conflict already recorded
		}
		delete(result, k)
	}

	return result, conflicts
}

// MergeHooks performs a 3-way merge of hook maps.
// Returns the merged result and any conflict items.
func MergeHooks(base, local, remote map[string]json.RawMessage) (map[string]json.RawMessage, []ConflictItem) {
	if base == nil {
		base = make(map[string]json.RawMessage)
	}
	if local == nil {
		local = make(map[string]json.RawMessage)
	}
	if remote == nil {
		remote = make(map[string]json.RawMessage)
	}

	result := make(map[string]json.RawMessage)
	var conflicts []ConflictItem

	// Start with base
	for k, v := range base {
		result[k] = v
	}

	// Determine changes for both sides
	localAdded := make(map[string]json.RawMessage)
	localModified := make(map[string]json.RawMessage)
	localRemoved := make(map[string]bool)
	for k, v := range local {
		if baseV, ok := base[k]; !ok {
			localAdded[k] = v
		} else if string(baseV) != string(v) {
			localModified[k] = v
		}
	}
	for k := range base {
		if _, ok := local[k]; !ok {
			localRemoved[k] = true
		}
	}

	remoteAdded := make(map[string]json.RawMessage)
	remoteModified := make(map[string]json.RawMessage)
	remoteRemoved := make(map[string]bool)
	for k, v := range remote {
		if baseV, ok := base[k]; !ok {
			remoteAdded[k] = v
		} else if string(baseV) != string(v) {
			remoteModified[k] = v
		}
	}
	for k := range base {
		if _, ok := remote[k]; !ok {
			remoteRemoved[k] = true
		}
	}

	// Apply additions (union)
	for k, v := range localAdded {
		result[k] = v
	}
	for k, v := range remoteAdded {
		if localV, ok := localAdded[k]; ok {
			if string(localV) != string(v) {
				conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: string(localV), RemoteValue: string(v)})
			}
		} else {
			result[k] = v
		}
	}

	// Apply modifications
	for k, v := range localModified {
		if remoteV, ok := remoteModified[k]; ok {
			if string(v) != string(remoteV) {
				conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: string(v), RemoteValue: string(remoteV)})
			} else {
				result[k] = v
			}
		} else if remoteRemoved[k] {
			conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: string(v), RemoteValue: nil})
		} else {
			result[k] = v
		}
	}
	for k, v := range remoteModified {
		if _, ok := localModified[k]; ok {
			continue
		}
		if localRemoved[k] {
			conflicts = append(conflicts, ConflictItem{Key: k, LocalValue: nil, RemoteValue: string(v)})
		} else {
			result[k] = v
		}
	}

	// Apply removals
	for k := range localRemoved {
		if _, ok := remoteModified[k]; ok {
			continue
		}
		if remoteRemoved[k] {
			delete(result, k) // Both removed — no conflict
		} else {
			delete(result, k)
		}
	}
	for k := range remoteRemoved {
		if _, ok := localModified[k]; ok {
			continue
		}
		delete(result, k)
	}

	return result, conflicts
}

// mergeStringSlice performs a 3-way merge on string slices (used for permissions).
func mergeStringSlice(base, local, remote []string) ([]string, bool) {
	baseSet := toSet(base)
	localSet := toSet(local)
	remoteSet := toSet(remote)

	localAdded := setDiff(localSet, baseSet)
	localRemoved := setDiff(baseSet, localSet)
	remoteAdded := setDiff(remoteSet, baseSet)
	remoteRemoved := setDiff(baseSet, remoteSet)

	// Conflict: one adds what other removes (or vice versa)
	if hasOverlap(localAdded, remoteRemoved) || hasOverlap(remoteAdded, localRemoved) {
		return nil, true
	}

	// Union: base + all additions - all removals
	result := make(map[string]bool)
	for s := range baseSet {
		result[s] = true
	}
	for s := range localAdded {
		result[s] = true
	}
	for s := range remoteAdded {
		result[s] = true
	}
	for s := range localRemoved {
		delete(result, s)
	}
	for s := range remoteRemoved {
		delete(result, s)
	}

	return fromSet(result), false
}

func toSet(slice []string) map[string]bool {
	set := make(map[string]bool, len(slice))
	for _, s := range slice {
		set[s] = true
	}
	return set
}

func setDiff(a, b map[string]bool) map[string]bool {
	diff := make(map[string]bool)
	for s := range a {
		if !b[s] {
			diff[s] = true
		}
	}
	return diff
}

func hasOverlap(a, b map[string]bool) bool {
	for s := range a {
		if b[s] {
			return true
		}
	}
	return false
}

func fromSet(set map[string]bool) []string {
	result := make([]string, 0, len(set))
	for s := range set {
		result = append(result, s)
	}
	return result
}

func jsonEqual(a, b any) bool {
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}
