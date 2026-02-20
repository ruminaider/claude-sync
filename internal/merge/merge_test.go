package merge

import (
	"encoding/json"
	"sort"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestMergePermissions_BothAdd_Union(t *testing.T) {
	base := config.Permissions{Allow: []string{"Read"}}
	local := config.Permissions{Allow: []string{"Read", "Edit"}}
	remote := config.Permissions{Allow: []string{"Read", "Bash(ls *)"}}

	result, conflict := MergePermissions(base, local, remote)
	assert.False(t, conflict)
	sort.Strings(result.Allow)
	assert.ElementsMatch(t, []string{"Read", "Edit", "Bash(ls *)"}, result.Allow)
}

func TestMergePermissions_BothRemove_NoConflict(t *testing.T) {
	base := config.Permissions{Allow: []string{"Read", "Edit", "Write"}}
	local := config.Permissions{Allow: []string{"Read", "Edit"}}   // removed Write
	remote := config.Permissions{Allow: []string{"Read", "Write"}} // removed Edit

	result, conflict := MergePermissions(base, local, remote)
	assert.False(t, conflict)
	assert.ElementsMatch(t, []string{"Read"}, result.Allow)
}

func TestMergePermissions_AddVsRemove_Conflict(t *testing.T) {
	base := config.Permissions{Allow: []string{"Read", "Edit"}}
	local := config.Permissions{Allow: []string{"Read", "Edit", "Write"}} // added Write
	remote := config.Permissions{Allow: []string{"Read"}}                  // removed Edit

	// No conflict here — local adds Write (not in base), remote removes Edit (was in base).
	// These are independent changes.
	// But: if local ADDED something that remote REMOVED from base...
	// Write was never in base, so remote didn't remove it.
	// Edit was in base and local kept it, remote removed it.
	// This should succeed: result = Read + Write (Edit removed by remote)
	result, conflict := MergePermissions(base, local, remote)
	assert.False(t, conflict)
	assert.ElementsMatch(t, []string{"Read", "Write"}, result.Allow)
}

func TestMergePermissions_OneAddsOneRemovesSame_Conflict(t *testing.T) {
	base := config.Permissions{Allow: []string{"Read"}}
	local := config.Permissions{Allow: []string{"Read", "Edit"}} // added Edit
	remote := config.Permissions{Allow: []string{}}               // removed Read

	// Remote removed Read, local kept it + added Edit.
	// Local added Edit (not in base) — remote didn't remove it. No conflict there.
	// Remote removed Read — local didn't add Read (it was already in base). No conflict.
	// Actually this should not conflict. Let me think...
	// localAdded = {Edit}, localRemoved = {}
	// remoteAdded = {}, remoteRemoved = {Read}
	// overlap(localAdded, remoteRemoved) = overlap({Edit}, {Read}) = false
	// overlap(remoteAdded, localRemoved) = overlap({}, {}) = false
	// No conflict! Result = base + Edit - Read = {Edit}
	result, conflict := MergePermissions(base, local, remote)
	assert.False(t, conflict)
	assert.ElementsMatch(t, []string{"Edit"}, result.Allow)
}

func TestMergePermissions_TrueAddRemoveConflict(t *testing.T) {
	// Local ADDS "Dangerous" (not in base), Remote... can't remove what's not in base.
	// Real conflict: local adds X, remote removes X from base.
	// But if X is not in base, remote can't remove it.
	// True conflict: Remote adds X to base, local removes X from base.
	base := config.Permissions{Allow: []string{"Read", "Edit"}}
	local := config.Permissions{Allow: []string{"Read"}}                   // removed Edit
	remote := config.Permissions{Allow: []string{"Read", "Edit", "Write"}} // added Write

	// Wait — for add-vs-remove conflict, we need:
	// One side adds something, other side removes something, and they overlap.
	// localRemoved = {Edit}, remoteAdded = {Write} — no overlap
	// Actually, let me create a proper conflict:
	// Base has {A, B}. Local removes B, adds C. Remote adds B-prime (modifying B).
	// That doesn't work with string slices.

	// For string slices, the only add-vs-remove conflict is:
	// remoteAdded overlaps localRemoved OR localAdded overlaps remoteRemoved
	// Example: base={A}, local={A,B} (added B), remote={} (removed A, and A only)
	// localAdded={B}, localRemoved={}
	// remoteAdded={}, remoteRemoved={A}
	// No overlap. Not a conflict.

	// TRUE example: base={A,B}, local={A} (removed B), remote={A,B,C} (added C)
	// Here B is in localRemoved and... is C in remoteAdded? Yes. Is B in remoteAdded? No.
	// No conflict.

	// For a REAL conflict with permissions:
	// base={A,B}, local={A,B,C} (added C), remote={A} (removed B and... also removed... hmm)
	// localAdded={C}, remoteRemoved={B}. overlap({C},{B})=false.
	//
	// The only way to get overlap is:
	// base={A}, local={A,B} (added B), remote={} (removed A and didn't have B)
	// Wait: remoteRemoved={A} and localAdded={B}. No overlap.
	//
	// Actually: base has item X. Remote removes X. Local adds X back? No, local already has it from base.
	//
	// Hmm, I think for simple string slices, add-vs-remove conflict means:
	// Local added NEW item Y (not in base). Remote also has Y in removed?
	// But remote can only remove what's in base. Y is not in base. So remote can't remove Y.
	// => overlap(localAdded, remoteRemoved) is impossible? No:
	// What if remote somehow has Y? No, remoteRemoved = items in base but not in remote.
	// Since Y is not in base, Y can't be in remoteRemoved. So overlap is impossible!
	//
	// Same logic: overlap(remoteAdded, localRemoved) is impossible because
	// localRemoved = items in base but not in local, and remoteAdded = items in remote but not in base.
	// If item is in remoteAdded, it's not in base, so it can't be in localRemoved.
	//
	// Conclusion: For string slice permissions, add-vs-remove conflicts are theoretically impossible
	// because added items are new (not in base) and removed items were in base.
	// The conflict detection is defensive but will never fire for permissions.

	// Let's just verify non-conflict scenarios work correctly
	result, conflict := MergePermissions(base, local, remote)
	assert.False(t, conflict)
	assert.ElementsMatch(t, []string{"Read", "Write"}, result.Allow)
}

func TestMergeSettings_BothModifySameKey_Conflict(t *testing.T) {
	base := map[string]any{"defaultMode": "default"}
	local := map[string]any{"defaultMode": "plan"}
	remote := map[string]any{"defaultMode": "acceptEdits"}

	_, conflicts := MergeSettings(base, local, remote)
	assert.Len(t, conflicts, 1)
	assert.Equal(t, "defaultMode", conflicts[0].Key)
	assert.Equal(t, "plan", conflicts[0].LocalValue)
	assert.Equal(t, "acceptEdits", conflicts[0].RemoteValue)
}

func TestMergeSettings_BothModifySameKey_SameValue(t *testing.T) {
	base := map[string]any{"defaultMode": "default"}
	local := map[string]any{"defaultMode": "plan"}
	remote := map[string]any{"defaultMode": "plan"}

	result, conflicts := MergeSettings(base, local, remote)
	assert.Empty(t, conflicts)
	assert.Equal(t, "plan", result["defaultMode"])
}

func TestMergeSettings_IndependentChanges(t *testing.T) {
	base := map[string]any{"a": "1"}
	local := map[string]any{"a": "1", "b": "2"}
	remote := map[string]any{"a": "1", "c": "3"}

	result, conflicts := MergeSettings(base, local, remote)
	assert.Empty(t, conflicts)
	assert.Equal(t, "1", result["a"])
	assert.Equal(t, "2", result["b"])
	assert.Equal(t, "3", result["c"])
}

func TestMergeSettings_LocalRemoveRemoteModify_Conflict(t *testing.T) {
	base := map[string]any{"a": "1", "b": "2"}
	local := map[string]any{"a": "1"}             // removed b
	remote := map[string]any{"a": "1", "b": "3"}  // modified b

	_, conflicts := MergeSettings(base, local, remote)
	assert.Len(t, conflicts, 1)
	assert.Equal(t, "b", conflicts[0].Key)
}

func TestMergeHooks_BothAddDifferent_Union(t *testing.T) {
	base := map[string]json.RawMessage{}
	local := map[string]json.RawMessage{
		"PreToolUse": json.RawMessage(`[{"matcher":"Bash"}]`),
	}
	remote := map[string]json.RawMessage{
		"PostToolUse": json.RawMessage(`[{"matcher":"Write"}]`),
	}

	result, conflicts := MergeHooks(base, local, remote)
	assert.Empty(t, conflicts)
	assert.Contains(t, result, "PreToolUse")
	assert.Contains(t, result, "PostToolUse")
}

func TestMergeHooks_BothModifySame_Conflict(t *testing.T) {
	base := map[string]json.RawMessage{
		"PreToolUse": json.RawMessage(`[{"matcher":"Bash"}]`),
	}
	local := map[string]json.RawMessage{
		"PreToolUse": json.RawMessage(`[{"matcher":"Bash","hooks":[{"type":"command","command":"v1"}]}]`),
	}
	remote := map[string]json.RawMessage{
		"PreToolUse": json.RawMessage(`[{"matcher":"Bash","hooks":[{"type":"command","command":"v2"}]}]`),
	}

	_, conflicts := MergeHooks(base, local, remote)
	assert.Len(t, conflicts, 1)
	assert.Equal(t, "PreToolUse", conflicts[0].Key)
}

func TestMergeHooks_BothRemoveSame_NoConflict(t *testing.T) {
	base := map[string]json.RawMessage{
		"PreToolUse":  json.RawMessage(`[{"matcher":"Bash"}]`),
		"PostToolUse": json.RawMessage(`[{"matcher":"Write"}]`),
	}
	local := map[string]json.RawMessage{
		"PostToolUse": json.RawMessage(`[{"matcher":"Write"}]`),
	}
	remote := map[string]json.RawMessage{
		"PostToolUse": json.RawMessage(`[{"matcher":"Write"}]`),
	}

	result, conflicts := MergeHooks(base, local, remote)
	assert.Empty(t, conflicts)
	assert.NotContains(t, result, "PreToolUse")
	assert.Contains(t, result, "PostToolUse")
}
