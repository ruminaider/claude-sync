# Full Configuration Sync Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend claude-sync from plugin-only syncing to full Claude Code configuration syncing (CLAUDE.md fragments, permissions, MCP servers, keybindings, expanded settings) with profile support, tiered approval, and auto-sync hooks.

**Architecture:** Each new sync surface follows the existing pattern: add fields to Config/Profile structs, extend Parse/Marshal, extend InitScan/Init/Pull/Push, add tests. The CLAUDE.md fragment system is a new `internal/claudemd/` package. Tiered approval uses a `pending-changes.yaml` file for non-interactive deferred approval.

**Tech Stack:** Go, `go.yaml.in/yaml/v3`, `encoding/json`, `testify/assert` + `testify/require`, `charmbracelet/huh` for CLI.

**Design doc:** `docs/plans/2026-02-13-full-config-sync-design.md` — read this first for full context on design decisions.

---

## Phase 1: Settings Expansion + Permissions

### Task 1: Expand settings scanning in InitScan

Currently `InitScan` only picks up the `model` field from `settings.json`. We need to scan `statusLine` and `env` too, and remove them from the excluded list.

**Files:**
- Modify: `internal/commands/init.go:52-57` (excludedSettingsFields)
- Modify: `internal/commands/init.go:222-244` (settings scanning in InitScan)
- Test: `internal/commands/init_test.go`

**Step 1: Write the failing test**

In `internal/commands/init_test.go`, add a test that verifies InitScan returns `statusLine` and `env` settings:

```go
func TestInitScan_ExpandedSettings(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	// Write installed_plugins.json (v2, empty)
	require.NoError(t, os.WriteFile(
		filepath.Join(pluginDir, "installed_plugins.json"),
		[]byte(`{"version": 2, "plugins": {}}`), 0644))
	require.NoError(t, os.WriteFile(
		filepath.Join(pluginDir, "known_marketplaces.json"),
		[]byte(`{}`), 0644))

	// Write settings.json with model, statusLine, env, and excluded fields
	settings := map[string]any{
		"model": "claude-opus-4-6",
		"statusLine": map[string]any{
			"type":    "command",
			"command": "echo status",
		},
		"env": map[string]any{
			"CLAUDE_CODE_EXPERIMENTAL_AGENT_TEAMS": "1",
		},
		"enabledPlugins": map[string]any{"foo": true}, // should be excluded
		"permissions":    map[string]any{},             // should be excluded
	}
	data, _ := json.Marshal(settings)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644))

	result, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	// model, statusLine, env should all be present
	assert.Equal(t, "claude-opus-4-6", result.Settings["model"])
	assert.NotNil(t, result.Settings["statusLine"])
	assert.NotNil(t, result.Settings["env"])

	// enabledPlugins and permissions should NOT be present
	_, hasEnabled := result.Settings["enabledPlugins"]
	assert.False(t, hasEnabled)
	_, hasPerms := result.Settings["permissions"]
	assert.False(t, hasPerms)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestInitScan_ExpandedSettings -v`
Expected: FAIL — `statusLine` and `env` are not returned because InitScan only scans `model`.

**Step 3: Implement the changes**

In `internal/commands/init.go`:

1. Update `excludedSettingsFields` — remove `statusLine` (keep `enabledPlugins` and `permissions`):

```go
var excludedSettingsFields = map[string]bool{
	"enabledPlugins": true,
	"permissions":    true,
	"hooks":          true, // hooks are handled separately
}
```

2. Replace the model-only scan logic (around line 222-244) with a generic scan that picks up all non-excluded fields:

```go
// Scan all syncable settings (skip excluded fields and hooks).
for key, rawVal := range settingsRaw {
	if excludedSettingsFields[key] {
		continue
	}
	var val any
	if err := json.Unmarshal(rawVal, &val); err != nil {
		continue
	}
	result.Settings[key] = val
}
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestInitScan_ExpandedSettings -v`
Expected: PASS

**Step 5: Run all existing tests to verify no regressions**

Run: `go test ./internal/commands/ -v`
Expected: All pass. Some tests may need updating if they assert on specific settings content.

**Step 6: Commit**

```bash
git add internal/commands/init.go internal/commands/init_test.go
git commit -m "feat: expand settings scanning to include statusLine, env, and all non-excluded fields"
```

---

### Task 2: Add Permissions to Config struct and parsing

Add `Permissions` as a new top-level section in ConfigV2 with `Allow` and `Deny` string slices.

**Files:**
- Modify: `internal/config/config.go` (ConfigV2 struct, Parse, MarshalV2)
- Test: `internal/config/config_test.go`

**Step 1: Write the failing test**

In `internal/config/config_test.go`:

```go
func TestParse_Permissions(t *testing.T) {
	data := []byte(`
version: "1.0.0"
plugins:
  upstream:
    - test@marketplace
permissions:
  allow:
    - Read
    - Edit
    - "Bash(git status*)"
  deny:
    - "Bash(rm -rf *)"
    - "Bash(sudo *)"
`)
	cfg, err := config.Parse(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"Read", "Edit", "Bash(git status*)"}, cfg.Permissions.Allow)
	assert.Equal(t, []string{"Bash(rm -rf *)", "Bash(sudo *)"}, cfg.Permissions.Deny)
}

func TestMarshal_Permissions(t *testing.T) {
	cfg := config.Config{
		Version:  "1.0.0",
		Upstream: []string{"test@marketplace"},
		Pinned:   map[string]string{},
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit"},
			Deny:  []string{"Bash(rm -rf *)"},
		},
	}

	data, err := config.Marshal(cfg)
	require.NoError(t, err)

	// Round-trip: parse the output and verify
	parsed, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, cfg.Permissions.Allow, parsed.Permissions.Allow)
	assert.Equal(t, cfg.Permissions.Deny, parsed.Permissions.Deny)
}

func TestParse_NoPermissions(t *testing.T) {
	data := []byte(`
version: "1.0.0"
plugins:
  upstream:
    - test@marketplace
`)
	cfg, err := config.Parse(data)
	require.NoError(t, err)

	// Permissions should be zero value (empty slices)
	assert.Empty(t, cfg.Permissions.Allow)
	assert.Empty(t, cfg.Permissions.Deny)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/config/ -run TestParse_Permissions -v`
Expected: FAIL — `cfg.Permissions` doesn't exist.

**Step 3: Implement**

In `internal/config/config.go`:

1. Add the `Permissions` struct and field:

```go
// Permissions holds allow/deny permission rules for Claude Code.
type Permissions struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

type ConfigV2 struct {
	Version     string                     `yaml:"version"`
	Upstream    []string                   `yaml:"-"`
	Pinned      map[string]string          `yaml:"-"`
	Forked      []string                   `yaml:"-"`
	Excluded    []string                   `yaml:"-"`
	Settings    map[string]any             `yaml:"settings,omitempty"`
	Hooks       map[string]json.RawMessage `yaml:"-"`
	Permissions Permissions                `yaml:"-"` // parsed manually
}
```

2. In `Parse()`, add a case for `"permissions"` in the switch:

```go
case "permissions":
	var perms Permissions
	if err := valNode.Decode(&perms); err != nil {
		return Config{}, fmt.Errorf("parsing config permissions: %w", err)
	}
	cfg.Permissions = perms
```

3. In `MarshalV2()`, add permissions serialization after hooks:

```go
// permissions
if len(cfg.Permissions.Allow) > 0 || len(cfg.Permissions.Deny) > 0 {
	var permsNode yaml.Node
	if err := permsNode.Encode(cfg.Permissions); err != nil {
		return nil, fmt.Errorf("encoding permissions: %w", err)
	}
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "permissions", Tag: "!!str"},
		&permsNode,
	)
}
```

**Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: All pass including new permission tests.

**Step 5: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "feat: add permissions (allow/deny) section to config"
```

---

### Task 3: Add Permissions to Profile struct and merge

Extend the Profile with `ProfilePermissions` (add_allow, add_deny) and a `MergePermissions` function.

**Files:**
- Modify: `internal/profiles/profile.go` (Profile struct, ParseProfile, MarshalProfile, new MergePermissions)
- Test: `internal/profiles/profile_test.go`

**Step 1: Write the failing test**

In `internal/profiles/profile_test.go`:

```go
func TestMergePermissions(t *testing.T) {
	base := config.Permissions{
		Allow: []string{"Read", "Edit"},
		Deny:  []string{"Bash(rm -rf *)"},
	}
	profile := profiles.Profile{
		Permissions: profiles.ProfilePermissions{
			AddAllow: []string{"Bash(kubectl *)", "Bash(docker *)"},
			AddDeny:  []string{"Bash(git push --force*)"},
		},
	}

	result := profiles.MergePermissions(base, profile)

	assert.Equal(t, []string{"Read", "Edit", "Bash(kubectl *)", "Bash(docker *)"}, result.Allow)
	assert.Equal(t, []string{"Bash(rm -rf *)", "Bash(git push --force*)"}, result.Deny)
}

func TestMergePermissions_NoDuplicates(t *testing.T) {
	base := config.Permissions{
		Allow: []string{"Read", "Edit"},
	}
	profile := profiles.Profile{
		Permissions: profiles.ProfilePermissions{
			AddAllow: []string{"Read", "Bash(kubectl *)"},  // "Read" is duplicate
		},
	}

	result := profiles.MergePermissions(base, profile)

	assert.Equal(t, []string{"Read", "Edit", "Bash(kubectl *)"}, result.Allow)
}

func TestParseProfile_Permissions(t *testing.T) {
	data := []byte(`
plugins:
  add:
    - test@marketplace
permissions:
  add_allow:
    - "Bash(kubectl *)"
  add_deny:
    - "Bash(git push --force*)"
`)
	p, err := profiles.ParseProfile(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"Bash(kubectl *)"}, p.Permissions.AddAllow)
	assert.Equal(t, []string{"Bash(git push --force*)"}, p.Permissions.AddDeny)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/profiles/ -run TestMergePermissions -v`
Expected: FAIL — `Profile.Permissions` doesn't exist.

**Step 3: Implement**

In `internal/profiles/profile.go`:

1. Add `ProfilePermissions` struct and field:

```go
type ProfilePermissions struct {
	AddAllow []string `yaml:"add_allow,omitempty"`
	AddDeny  []string `yaml:"add_deny,omitempty"`
}

type Profile struct {
	Plugins     ProfilePlugins     `yaml:"plugins,omitempty"`
	Settings    map[string]any     `yaml:"settings,omitempty"`
	Hooks       ProfileHooks       `yaml:"hooks,omitempty"`
	Permissions ProfilePermissions `yaml:"permissions,omitempty"`
}
```

2. In `ParseProfile()`, add a case for `"permissions"`:

```go
case "permissions":
	var perms ProfilePermissions
	if err := valNode.Decode(&perms); err != nil {
		return Profile{}, fmt.Errorf("parsing profile permissions: %w", err)
	}
	p.Permissions = perms
```

3. Add `MergePermissions` function:

```go
// MergePermissions appends profile permissions to base (no duplicates).
func MergePermissions(base config.Permissions, profile Profile) config.Permissions {
	result := config.Permissions{
		Allow: appendUnique(base.Allow, profile.Permissions.AddAllow),
		Deny:  appendUnique(base.Deny, profile.Permissions.AddDeny),
	}
	return result
}

func appendUnique(base, add []string) []string {
	seen := make(map[string]bool, len(base))
	result := make([]string, len(base))
	copy(result, base)
	for _, s := range base {
		seen[s] = true
	}
	for _, s := range add {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
```

4. Update `MarshalProfile` to include permissions section.

5. Update `ProfileSummary` to include permissions counts.

**Step 4: Run tests**

Run: `go test ./internal/profiles/ -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/profiles/profile.go internal/profiles/profile_test.go
git commit -m "feat: add permissions to profile with merge support"
```

---

### Task 4: Add permissions scanning to InitScan

Scan `settings.json` for existing `permissions.allow` and `permissions.deny` arrays during InitScan.

**Files:**
- Modify: `internal/commands/init.go` (InitScanResult struct, InitScan function)
- Test: `internal/commands/init_test.go`

**Step 1: Write the failing test**

```go
func TestInitScan_Permissions(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	require.NoError(t, os.WriteFile(
		filepath.Join(pluginDir, "installed_plugins.json"),
		[]byte(`{"version": 2, "plugins": {}}`), 0644))
	require.NoError(t, os.WriteFile(
		filepath.Join(pluginDir, "known_marketplaces.json"),
		[]byte(`{}`), 0644))

	settings := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Read", "Edit", "Bash(git status*)"},
			"deny":  []string{"Bash(rm -rf *)"},
		},
	}
	data, _ := json.Marshal(settings)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644))

	result, err := commands.InitScan(claudeDir)
	require.NoError(t, err)

	assert.Equal(t, []string{"Read", "Edit", "Bash(git status*)"}, result.Permissions.Allow)
	assert.Equal(t, []string{"Bash(rm -rf *)"}, result.Permissions.Deny)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestInitScan_Permissions -v`
Expected: FAIL — `InitScanResult.Permissions` doesn't exist.

**Step 3: Implement**

1. Add `Permissions config.Permissions` to `InitScanResult`
2. In `InitScan`, after reading `settingsRaw`, extract permissions:

```go
if permsRaw, ok := settingsRaw["permissions"]; ok {
	var permsMap struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	if json.Unmarshal(permsRaw, &permsMap) == nil {
		result.Permissions.Allow = permsMap.Allow
		result.Permissions.Deny = permsMap.Deny
	}
}
```

3. Add `Permissions config.Permissions` to `InitOptions` and `InitResult`

**Step 4: Run tests**

Run: `go test ./internal/commands/ -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/commands/init.go internal/commands/init_test.go
git commit -m "feat: scan permissions from settings.json during InitScan"
```

---

### Task 5: Write permissions to config during Init

When `Init()` is called with permissions, write them to config.yaml.

**Files:**
- Modify: `internal/commands/init.go` (Init function)
- Test: `internal/commands/init_test.go`

**Step 1: Write the failing test**

```go
func TestInit_WritesPermissions(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	opts := defaultInitOpts(claudeDir, syncDir, "")
	opts.Permissions = config.Permissions{
		Allow: []string{"Read", "Edit"},
		Deny:  []string{"Bash(rm -rf *)"},
	}

	_, err := commands.Init(opts)
	require.NoError(t, err)

	// Read config.yaml and verify permissions are present
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.Parse(data)
	require.NoError(t, err)

	assert.Equal(t, []string{"Read", "Edit"}, cfg.Permissions.Allow)
	assert.Equal(t, []string{"Bash(rm -rf *)"}, cfg.Permissions.Deny)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestInit_WritesPermissions -v`
Expected: FAIL — `InitOptions.Permissions` doesn't exist or permissions not written.

**Step 3: Implement**

In `Init()`, when building the config struct, copy `opts.Permissions` to `cfg.Permissions`:

```go
cfg.Permissions = opts.Permissions
```

Add `Permissions config.Permissions` to `InitOptions` (if not already done in Task 4).

**Step 4: Run tests**

Run: `go test ./internal/commands/ -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/commands/init.go internal/commands/init_test.go
git commit -m "feat: write permissions to config.yaml during init"
```

---

### Task 6: Apply permissions during Pull (additive merge)

When Pull applies settings, also apply permissions from config additively to `settings.json`.

**Files:**
- Modify: `internal/commands/pull.go` (ApplySettings, Pull, PullResult)
- Test: `internal/commands/pull_test.go`

**Step 1: Write the failing test**

```go
func TestApplySettings_Permissions(t *testing.T) {
	claudeDir := t.TempDir()

	// Write settings.json with existing permissions
	existing := map[string]json.RawMessage{
		"permissions": json.RawMessage(`{"allow":["Read","Bash(tailscale *)"],"deny":[]}`),
	}
	data, _ := json.MarshalIndent(existing, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644))

	cfg := config.Config{
		Pinned: map[string]string{},
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit", "Write"},
			Deny:  []string{"Bash(rm -rf *)"},
		},
	}

	_, _, err := commands.ApplySettings(claudeDir, cfg)
	require.NoError(t, err)

	// Read back settings.json
	settings, err := claudecode.ReadSettings(claudeDir)
	require.NoError(t, err)

	var perms struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	require.NoError(t, json.Unmarshal(settings["permissions"], &perms))

	// Should be additive: existing + synced, no duplicates
	assert.Contains(t, perms.Allow, "Read")
	assert.Contains(t, perms.Allow, "Edit")
	assert.Contains(t, perms.Allow, "Write")
	assert.Contains(t, perms.Allow, "Bash(tailscale *)") // local perm preserved
	assert.Contains(t, perms.Deny, "Bash(rm -rf *)")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestApplySettings_Permissions -v`
Expected: FAIL — permissions are skipped (in excludedSettingsFields).

**Step 3: Implement**

In `ApplySettings()` in `pull.go`, after the settings loop, add permissions merge logic:

```go
// Apply permissions (additive merge — synced rules are added to local, never removed)
if len(cfg.Permissions.Allow) > 0 || len(cfg.Permissions.Deny) > 0 {
	var existingPerms struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	if raw, ok := settings["permissions"]; ok {
		json.Unmarshal(raw, &existingPerms)
	}

	// Additive merge: add synced rules without removing existing local ones
	mergedAllow := appendUniqueStrings(existingPerms.Allow, cfg.Permissions.Allow)
	mergedDeny := appendUniqueStrings(existingPerms.Deny, cfg.Permissions.Deny)

	permsData, _ := json.Marshal(map[string][]string{
		"allow": mergedAllow,
		"deny":  mergedDeny,
	})
	settings["permissions"] = json.RawMessage(permsData)
	settingsApplied = append(settingsApplied, "permissions")
}
```

Add the helper function:

```go
func appendUniqueStrings(base, add []string) []string {
	seen := make(map[string]bool, len(base))
	result := make([]string, len(base))
	copy(result, base)
	for _, s := range base {
		seen[s] = true
	}
	for _, s := range add {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}
```

**Step 4: Run tests**

Run: `go test ./internal/commands/ -v`
Expected: All pass. Verify existing tests for "permissions should be excluded" still pass — they test that `permissions` is not overwritten by the settings loop. Permissions is now handled separately via additive merge, so the field is still excluded from the settings loop but applied through its own logic.

**Step 5: Commit**

```bash
git add internal/commands/pull.go internal/commands/pull_test.go
git commit -m "feat: apply permissions additively during pull"
```

---

### Task 7: Add tiered approval classification

Create a new package `internal/approval/` that classifies config changes as safe (auto-apply) vs high-risk (requires approval).

**Files:**
- Create: `internal/approval/approval.go`
- Create: `internal/approval/approval_test.go`

**Step 1: Write the failing test**

```go
package approval_test

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestClassify_PermissionsAreHighRisk(t *testing.T) {
	changes := approval.ConfigChanges{
		Permissions: &config.Permissions{
			Allow: []string{"Bash(kubectl *)"},
		},
	}

	result := approval.Classify(changes)

	assert.NotEmpty(t, result.HighRisk)
	assert.Equal(t, "permissions", result.HighRisk[0].Category)
}

func TestClassify_SettingsAreSafe(t *testing.T) {
	changes := approval.ConfigChanges{
		Settings: map[string]any{"model": "claude-opus-4-6"},
	}

	result := approval.Classify(changes)

	assert.NotEmpty(t, result.Safe)
	assert.Empty(t, result.HighRisk)
}

func TestClassify_HooksAreHighRisk(t *testing.T) {
	changes := approval.ConfigChanges{
		HasHookChanges: true,
	}

	result := approval.Classify(changes)

	assert.NotEmpty(t, result.HighRisk)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/approval/ -v`
Expected: FAIL — package doesn't exist.

**Step 3: Implement**

Create `internal/approval/approval.go`:

```go
package approval

import "github.com/ruminaider/claude-sync/internal/config"

// ConfigChanges represents the set of changes detected from a pull.
type ConfigChanges struct {
	Settings       map[string]any
	Permissions    *config.Permissions
	HasHookChanges bool
	HasMCPChanges  bool
	ClaudeMD       []string // changed fragment names
	Keybindings    bool
}

// Change represents a single classified change.
type Change struct {
	Category    string // "permissions", "hooks", "mcp", "settings", "claude_md", "keybindings"
	Description string
}

// ClassifiedChanges splits changes into safe (auto-apply) and high-risk (requires approval).
type ClassifiedChanges struct {
	Safe     []Change
	HighRisk []Change
}

// Classify categorizes changes by risk level.
// High-risk: permissions, hooks, MCP servers (can execute arbitrary code).
// Safe: settings, keybindings, CLAUDE.md fragments.
func Classify(changes ConfigChanges) ClassifiedChanges {
	var result ClassifiedChanges

	if len(changes.Settings) > 0 {
		result.Safe = append(result.Safe, Change{
			Category:    "settings",
			Description: "settings updated",
		})
	}

	if len(changes.ClaudeMD) > 0 {
		result.Safe = append(result.Safe, Change{
			Category:    "claude_md",
			Description: "CLAUDE.md fragments updated",
		})
	}

	if changes.Keybindings {
		result.Safe = append(result.Safe, Change{
			Category:    "keybindings",
			Description: "keybindings updated",
		})
	}

	if changes.Permissions != nil && (len(changes.Permissions.Allow) > 0 || len(changes.Permissions.Deny) > 0) {
		result.HighRisk = append(result.HighRisk, Change{
			Category:    "permissions",
			Description: "permission rules changed",
		})
	}

	if changes.HasHookChanges {
		result.HighRisk = append(result.HighRisk, Change{
			Category:    "hooks",
			Description: "hooks changed",
		})
	}

	if changes.HasMCPChanges {
		result.HighRisk = append(result.HighRisk, Change{
			Category:    "mcp",
			Description: "MCP servers changed",
		})
	}

	return result
}
```

**Step 4: Run tests**

Run: `go test ./internal/approval/ -v`
Expected: All pass.

**Step 5: Commit**

```bash
git add internal/approval/
git commit -m "feat: add tiered approval classification for config changes"
```

---

### Task 8: Add pending-changes.yaml read/write

Create functions to read/write `pending-changes.yaml` (gitignored) for deferred high-risk approvals.

**Files:**
- Modify: `internal/approval/approval.go` (add PendingChanges struct, Read/Write functions)
- Test: `internal/approval/approval_test.go`

**Step 1: Write the failing test**

```go
func TestPendingChanges_WriteAndRead(t *testing.T) {
	dir := t.TempDir()

	pending := approval.PendingChanges{
		PendingSince: "2026-02-13T10:30:00Z",
		Commit:       "abc1234",
		Permissions: &config.Permissions{
			Allow: []string{"Bash(kubectl *)"},
		},
	}

	err := approval.WritePending(dir, pending)
	require.NoError(t, err)

	loaded, err := approval.ReadPending(dir)
	require.NoError(t, err)

	assert.Equal(t, "abc1234", loaded.Commit)
	assert.Equal(t, []string{"Bash(kubectl *)"}, loaded.Permissions.Allow)
}

func TestPendingChanges_ReadMissing(t *testing.T) {
	dir := t.TempDir()

	loaded, err := approval.ReadPending(dir)
	require.NoError(t, err)
	assert.True(t, loaded.IsEmpty())
}

func TestPendingChanges_Clear(t *testing.T) {
	dir := t.TempDir()

	pending := approval.PendingChanges{Commit: "abc"}
	require.NoError(t, approval.WritePending(dir, pending))

	require.NoError(t, approval.ClearPending(dir))

	loaded, err := approval.ReadPending(dir)
	require.NoError(t, err)
	assert.True(t, loaded.IsEmpty())
}
```

**Step 2: Implement PendingChanges struct and Read/Write/Clear functions**

**Step 3: Run tests**

Run: `go test ./internal/approval/ -v`
Expected: All pass.

**Step 4: Add `pending-changes.yaml` to the .gitignore template in Init**

In `internal/commands/init.go`, find where `.gitignore` is written and add `pending-changes.yaml`.

**Step 5: Commit**

```bash
git add internal/approval/ internal/commands/init.go
git commit -m "feat: add pending-changes.yaml for deferred high-risk approvals"
```

---

### Task 9: Integrate tiered approval into Pull

Modify Pull to classify changes, auto-apply safe ones, and either prompt for high-risk (interactive) or write to pending-changes.yaml (non-interactive).

**Files:**
- Modify: `internal/commands/pull.go` (Pull function, add `--auto` flag awareness)
- Modify: `internal/commands/pull.go` (PullResult struct — add new fields)
- Test: `internal/commands/pull_test.go`

**Step 1: Write failing tests**

```go
func TestPull_PermissionsInResult(t *testing.T) {
	// Setup with a config that has permissions
	// Verify PullResult includes PermissionsApplied field
}
```

**Step 2: Implement**

Add `Auto bool` parameter to `Pull()` (or create a `PullOptions` struct to avoid expanding the parameter list):

```go
type PullOptions struct {
	ClaudeDir string
	SyncDir   string
	Quiet     bool
	Auto      bool // non-interactive mode: defer high-risk changes
}
```

In Pull, after computing changes:
- If `Auto` is true and there are high-risk changes: write to pending-changes.yaml, skip applying them
- If `Auto` is false: apply all changes (CLI layer handles the interactive prompt)

**Step 3: Run tests**

Run: `go test ./internal/commands/ -v`
Expected: All pass.

**Step 4: Commit**

```bash
git add internal/commands/pull.go internal/commands/pull_test.go
git commit -m "feat: integrate tiered approval into pull flow"
```

---

### Task 10: Update Init and Push CLI for permissions

Add permissions selection to the init wizard and permissions diffing to push scan.

**Files:**
- Modify: `cmd/claude-sync/cmd_init.go` (add permissions step)
- Modify: `internal/commands/push.go` (PushScan — detect permission changes)
- Modify: `cmd/claude-sync/cmd_push.go` (display permission changes)

**Step 1: Add permissions step to init wizard**

After the settings step (`stepSettings`), add a `stepPermissions` step that shows discovered permissions and asks which to include:

```go
case stepPermissions:
	if len(scanResult.Permissions.Allow) == 0 && len(scanResult.Permissions.Deny) == 0 {
		step = stepHookStrategy // skip if no permissions found
		continue
	}
	// Show permissions and ask to include
```

**Step 2: Extend PushScan to detect permission changes**

Compare current `settings.json` permissions against config.yaml permissions. Add `ChangedPermissions` to `PushScanResult`.

**Step 3: Run all tests**

Run: `go test ./... -v`
Expected: All pass.

**Step 4: Commit**

```bash
git add cmd/claude-sync/cmd_init.go internal/commands/push.go cmd/claude-sync/cmd_push.go
git commit -m "feat: add permissions to init wizard and push scan"
```

---

## Phase 2: CLAUDE.md Fragment System

### Task 11: Create claudemd package with core types

New package for fragment management: splitting, assembling, manifest handling.

**Files:**
- Create: `internal/claudemd/claudemd.go`
- Create: `internal/claudemd/claudemd_test.go`

**Step 1: Write failing tests for Split**

```go
package claudemd_test

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplit_BasicSections(t *testing.T) {
	content := `# My Preferences

Some preamble text.

## Git Conventions

No Co-Authored-By lines.

## Color Preferences

Use Catppuccin Mocha.
`

	sections := claudemd.Split(content)

	require.Len(t, sections, 3)

	assert.Equal(t, "", sections[0].Header)         // preamble
	assert.Contains(t, sections[0].Content, "preamble text")

	assert.Equal(t, "Git Conventions", sections[1].Header)
	assert.Contains(t, sections[1].Content, "Co-Authored-By")

	assert.Equal(t, "Color Preferences", sections[2].Header)
	assert.Contains(t, sections[2].Content, "Catppuccin")
}

func TestSplit_NoPreamble(t *testing.T) {
	content := `## Section One

Content one.

## Section Two

Content two.
`

	sections := claudemd.Split(content)
	require.Len(t, sections, 2)
	assert.Equal(t, "Section One", sections[0].Header)
}

func TestSplit_EmptyContent(t *testing.T) {
	sections := claudemd.Split("")
	assert.Empty(t, sections)
}
```

**Step 2: Implement**

```go
package claudemd

import (
	"strings"
)

// Section represents a single section of a CLAUDE.md file.
type Section struct {
	Header  string // "" for preamble (content before first ## header)
	Content string // full content including the ## header line
}

// Split splits markdown content on "## " headers into sections.
// Content before the first ## header becomes a preamble section with empty Header.
func Split(content string) []Section {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	var sections []Section
	lines := strings.Split(content, "\n")
	var current *Section

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			if current != nil {
				current.Content = strings.TrimRight(current.Content, "\n") + "\n"
				sections = append(sections, *current)
			}
			header := strings.TrimPrefix(line, "## ")
			current = &Section{
				Header:  strings.TrimSpace(header),
				Content: line + "\n",
			}
		} else {
			if current == nil {
				current = &Section{Header: "", Content: ""}
			}
			current.Content += line + "\n"
		}
	}

	if current != nil && strings.TrimSpace(current.Content) != "" {
		current.Content = strings.TrimRight(current.Content, "\n") + "\n"
		sections = append(sections, *current)
	}

	return sections
}
```

**Step 3: Run tests**

Run: `go test ./internal/claudemd/ -v`
Expected: All pass.

**Step 4: Commit**

```bash
git add internal/claudemd/
git commit -m "feat: add claudemd package with Split function"
```

---

### Task 12: Add Assemble and fragment name generation

Add Assemble (reverse of Split) and `HeaderToFragmentName` for generating filenames from headers.

**Files:**
- Modify: `internal/claudemd/claudemd.go`
- Test: `internal/claudemd/claudemd_test.go`

**Step 1: Write failing tests**

```go
func TestAssemble(t *testing.T) {
	sections := []Section{
		{Header: "", Content: "# My Preferences\n\nPreamble.\n"},
		{Header: "Git Conventions", Content: "## Git Conventions\n\nNo Co-Authored-By.\n"},
	}

	result := claudemd.Assemble(sections)

	assert.Contains(t, result, "# My Preferences")
	assert.Contains(t, result, "## Git Conventions")
	assert.Contains(t, result, "No Co-Authored-By")
}

func TestAssemble_RoundTrip(t *testing.T) {
	original := "# Title\n\nPreamble.\n\n## Section A\n\nContent A.\n\n## Section B\n\nContent B.\n"
	sections := claudemd.Split(original)
	reassembled := claudemd.Assemble(sections)

	// Split again and verify sections match
	resplit := claudemd.Split(reassembled)
	require.Len(t, resplit, len(sections))
	for i := range sections {
		assert.Equal(t, sections[i].Header, resplit[i].Header)
	}
}

func TestHeaderToFragmentName(t *testing.T) {
	assert.Equal(t, "git-conventions", claudemd.HeaderToFragmentName("Git Conventions"))
	assert.Equal(t, "tool-authentication-requirements", claudemd.HeaderToFragmentName("Tool Authentication Requirements"))
	assert.Equal(t, "readme-writing", claudemd.HeaderToFragmentName("README Writing"))
	assert.Equal(t, "_preamble", claudemd.HeaderToFragmentName(""))
}
```

**Step 2: Implement**

```go
// Assemble concatenates sections into a single markdown string.
func Assemble(sections []Section) string {
	var b strings.Builder
	for i, s := range sections {
		if i > 0 {
			b.WriteString("\n")
		}
		b.WriteString(s.Content)
	}
	return b.String()
}

// HeaderToFragmentName converts a section header to a filename-safe fragment name.
// Empty header (preamble) returns "_preamble".
func HeaderToFragmentName(header string) string {
	if header == "" {
		return "_preamble"
	}
	name := strings.ToLower(header)
	name = strings.ReplaceAll(name, " ", "-")
	// Remove non-alphanumeric characters except hyphens
	var cleaned strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' {
			cleaned.WriteRune(r)
		}
	}
	return cleaned.String()
}
```

**Step 3: Run tests**

Run: `go test ./internal/claudemd/ -v`
Expected: All pass.

**Step 4: Commit**

```bash
git add internal/claudemd/
git commit -m "feat: add Assemble and HeaderToFragmentName to claudemd"
```

---

### Task 13: Add Manifest read/write and content hashing

Manifest tracks fragment metadata (header, content_hash) and ordering.

**Files:**
- Modify: `internal/claudemd/claudemd.go` (add Manifest struct, Read/Write, ContentHash)
- Test: `internal/claudemd/claudemd_test.go`

**Step 1: Write failing tests**

```go
func TestManifest_WriteAndRead(t *testing.T) {
	dir := t.TempDir()

	manifest := claudemd.Manifest{
		Fragments: map[string]claudemd.FragmentMeta{
			"_preamble":        {Header: "", ContentHash: "abc123"},
			"git-conventions":  {Header: "Git Conventions", ContentHash: "def456"},
		},
		Order: []string{"_preamble", "git-conventions"},
	}

	err := claudemd.WriteManifest(dir, manifest)
	require.NoError(t, err)

	loaded, err := claudemd.ReadManifest(dir)
	require.NoError(t, err)

	assert.Equal(t, manifest.Order, loaded.Order)
	assert.Equal(t, "Git Conventions", loaded.Fragments["git-conventions"].Header)
}

func TestContentHash(t *testing.T) {
	hash1 := claudemd.ContentHash("hello world")
	hash2 := claudemd.ContentHash("hello world")
	hash3 := claudemd.ContentHash("different content")

	assert.Equal(t, hash1, hash2)
	assert.NotEqual(t, hash1, hash3)
}
```

**Step 2: Implement Manifest types and ContentHash**

```go
type FragmentMeta struct {
	Header      string `yaml:"header"`
	ContentHash string `yaml:"content_hash"`
}

type Manifest struct {
	Fragments map[string]FragmentMeta `yaml:"fragments"`
	Order     []string               `yaml:"order"`
}

func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:8]) // 16-char hex prefix
}

func WriteManifest(claudeMdDir string, m Manifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(claudeMdDir, "manifest.yaml"), data, 0644)
}

func ReadManifest(claudeMdDir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(claudeMdDir, "manifest.yaml"))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{Fragments: map[string]FragmentMeta{}}, nil
		}
		return Manifest{}, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	if m.Fragments == nil {
		m.Fragments = map[string]FragmentMeta{}
	}
	return m, nil
}
```

**Step 3: Run tests and commit**

```bash
git add internal/claudemd/
git commit -m "feat: add manifest read/write and content hashing to claudemd"
```

---

### Task 14: Add fragment file read/write and full import flow

Import an existing CLAUDE.md by splitting it into fragment files + manifest.

**Files:**
- Modify: `internal/claudemd/claudemd.go` (add ImportClaudeMD, WriteFragment, ReadFragments, AssembleFromDir)
- Test: `internal/claudemd/claudemd_test.go`

**Step 1: Write failing tests**

```go
func TestImportClaudeMD(t *testing.T) {
	syncDir := t.TempDir()

	content := "# Title\n\nPreamble.\n\n## Git Conventions\n\nNo force push.\n\n## Colors\n\nUse Catppuccin.\n"

	result, err := claudemd.ImportClaudeMD(syncDir, content)
	require.NoError(t, err)

	assert.Len(t, result.FragmentNames, 3)
	assert.Equal(t, "_preamble", result.FragmentNames[0])
	assert.Equal(t, "git-conventions", result.FragmentNames[1])
	assert.Equal(t, "colors", result.FragmentNames[2])

	// Verify files exist
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "_preamble.md"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "git-conventions.md"))
	assert.NoError(t, err)
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "manifest.yaml"))
	assert.NoError(t, err)
}

func TestAssembleFromDir(t *testing.T) {
	syncDir := t.TempDir()

	original := "# Title\n\nPreamble.\n\n## Git Conventions\n\nNo force push.\n"
	_, err := claudemd.ImportClaudeMD(syncDir, original)
	require.NoError(t, err)

	assembled, err := claudemd.AssembleFromDir(syncDir, []string{"_preamble", "git-conventions"})
	require.NoError(t, err)

	assert.Contains(t, assembled, "# Title")
	assert.Contains(t, assembled, "## Git Conventions")
}
```

**Step 2: Implement ImportClaudeMD and AssembleFromDir**

```go
type ImportResult struct {
	FragmentNames []string
}

func ImportClaudeMD(syncDir, content string) (*ImportResult, error) {
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	if err := os.MkdirAll(claudeMdDir, 0755); err != nil {
		return nil, err
	}

	sections := Split(content)
	manifest := Manifest{Fragments: make(map[string]FragmentMeta), Order: []string{}}
	var names []string

	for _, s := range sections {
		name := HeaderToFragmentName(s.Header)
		if err := WriteFragment(claudeMdDir, name, s.Content); err != nil {
			return nil, err
		}
		manifest.Fragments[name] = FragmentMeta{
			Header:      s.Header,
			ContentHash: ContentHash(s.Content),
		}
		manifest.Order = append(manifest.Order, name)
		names = append(names, name)
	}

	if err := WriteManifest(claudeMdDir, manifest); err != nil {
		return nil, err
	}

	return &ImportResult{FragmentNames: names}, nil
}

func WriteFragment(claudeMdDir, name, content string) error {
	return os.WriteFile(filepath.Join(claudeMdDir, name+".md"), []byte(content), 0644)
}

func AssembleFromDir(syncDir string, include []string) (string, error) {
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	var sections []Section

	for _, name := range include {
		data, err := os.ReadFile(filepath.Join(claudeMdDir, name+".md"))
		if err != nil {
			return "", fmt.Errorf("reading fragment %q: %w", name, err)
		}
		// Read header from manifest for metadata (not strictly needed for assembly)
		sections = append(sections, Section{Content: string(data)})
	}

	return Assemble(sections), nil
}
```

**Step 3: Run tests and commit**

```bash
git add internal/claudemd/
git commit -m "feat: add fragment import and assembly from directory"
```

---

### Task 15: Add content similarity matching for rename detection

Implement Levenshtein or Jaccard similarity for detecting renamed sections during push disassembly.

**Files:**
- Modify: `internal/claudemd/claudemd.go` (add ContentSimilarity function)
- Test: `internal/claudemd/claudemd_test.go`

**Step 1: Write failing tests**

```go
func TestContentSimilarity_Identical(t *testing.T) {
	sim := claudemd.ContentSimilarity("hello world", "hello world")
	assert.InDelta(t, 1.0, sim, 0.01)
}

func TestContentSimilarity_Similar(t *testing.T) {
	a := "No Co-Authored-By lines in commits.\nUse conventional commit format."
	b := "No Co-Authored-By lines in commits.\nUse conventional commit format.\nSign all commits."
	sim := claudemd.ContentSimilarity(a, b)
	assert.Greater(t, sim, 0.8)
}

func TestContentSimilarity_Different(t *testing.T) {
	sim := claudemd.ContentSimilarity("hello world", "completely different text entirely")
	assert.Less(t, sim, 0.5)
}
```

**Step 2: Implement using Jaccard similarity on word sets**

```go
func ContentSimilarity(a, b string) float64 {
	wordsA := wordSet(a)
	wordsB := wordSet(b)

	if len(wordsA) == 0 && len(wordsB) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range wordsA {
		if wordsB[w] {
			intersection++
		}
	}

	union := len(wordsA) + len(wordsB) - intersection
	if union == 0 {
		return 1.0
	}
	return float64(intersection) / float64(union)
}

func wordSet(s string) map[string]bool {
	words := strings.Fields(strings.ToLower(s))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}
```

**Step 3: Run tests and commit**

```bash
git add internal/claudemd/
git commit -m "feat: add content similarity matching for rename detection"
```

---

### Task 16: Add claude_md section to Config and Profile

Add `ClaudeMD` to the Config struct with an `Include` list, and to Profile with `Add`/`Remove` lists.

**Files:**
- Modify: `internal/config/config.go` (ConfigV2 struct, Parse, MarshalV2)
- Modify: `internal/profiles/profile.go` (Profile struct, ParseProfile, MarshalProfile, new MergeClaudeMD)
- Test: `internal/config/config_test.go`, `internal/profiles/profile_test.go`

**Step 1: Write failing tests**

Config test:
```go
func TestParse_ClaudeMD(t *testing.T) {
	data := []byte(`
version: "1.0.0"
claude_md:
  include:
    - _preamble
    - git-conventions
    - color-preferences
`)
	cfg, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, []string{"_preamble", "git-conventions", "color-preferences"}, cfg.ClaudeMD.Include)
}
```

Profile test:
```go
func TestMergeClaudeMD(t *testing.T) {
	base := []string{"_preamble", "git-conventions", "color-preferences"}
	profile := profiles.Profile{
		ClaudeMD: profiles.ProfileClaudeMD{
			Add:    []string{"work-conventions"},
			Remove: []string{"color-preferences"},
		},
	}

	result := profiles.MergeClaudeMD(base, profile)
	assert.Equal(t, []string{"_preamble", "git-conventions", "work-conventions"}, result)
}
```

**Step 2: Implement**

Add to Config:
```go
type ClaudeMDConfig struct {
	Include []string `yaml:"include,omitempty"`
}
```

Add to Profile:
```go
type ProfileClaudeMD struct {
	Add    []string `yaml:"add,omitempty"`
	Remove []string `yaml:"remove,omitempty"`
}
```

`MergeClaudeMD` follows the same pattern as `MergePlugins`: base + add - remove.

**Step 3: Run tests and commit**

```bash
git add internal/config/ internal/profiles/
git commit -m "feat: add claude_md section to config and profile"
```

---

### Task 17: Integrate CLAUDE.md into Pull flow

During Pull, assemble CLAUDE.md from fragments and write to `~/.claude/CLAUDE.md`.

**Files:**
- Modify: `internal/commands/pull.go` (add fragment assembly after parsing config)
- Test: `internal/commands/pull_test.go`

**Step 1: Write failing test**

```go
func TestPull_AssemblesClaudeMD(t *testing.T) {
	claudeDir, syncDir := setupTestEnvWithClaudeMD(t)
	// setupTestEnvWithClaudeMD creates:
	//   syncDir/claude-md/_preamble.md
	//   syncDir/claude-md/git-conventions.md
	//   syncDir/claude-md/manifest.yaml
	//   syncDir/config.yaml with claude_md.include: [_preamble, git-conventions]

	result, err := commands.Pull(claudeDir, syncDir, true)
	require.NoError(t, err)

	// Verify CLAUDE.md was assembled
	claudeMdContent, err := os.ReadFile(filepath.Join(claudeDir, "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(claudeMdContent), "Git Conventions")
}
```

**Step 2: Implement**

In `Pull()`, after parsing config and merging profile, assemble CLAUDE.md:

```go
// Assemble CLAUDE.md from fragments
if len(cfg.ClaudeMD.Include) > 0 {
	include := cfg.ClaudeMD.Include
	if activeProfile != "" {
		profile, _ := profiles.ReadProfile(syncDir, activeProfile)
		include = profiles.MergeClaudeMD(include, profile)
	}
	content, err := claudemd.AssembleFromDir(syncDir, include)
	if err == nil {
		os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(content), 0644)
	}
}
```

**Step 3: Run tests and commit**

```bash
git add internal/commands/pull.go internal/commands/pull_test.go
git commit -m "feat: assemble CLAUDE.md from fragments during pull"
```

---

### Task 18: Integrate CLAUDE.md into Push flow (disassembly)

During Push, detect changes to `~/.claude/CLAUDE.md`, split into sections, match to fragments, update changed fragments.

**Files:**
- Create: `internal/claudemd/reconcile.go` (Reconcile function)
- Create: `internal/claudemd/reconcile_test.go`
- Modify: `internal/commands/push.go` (call reconcile during PushScan)

**Step 1: Write failing tests for Reconcile**

```go
func TestReconcile_UpdatedSection(t *testing.T) {
	syncDir := t.TempDir()

	// Import initial content
	_, err := claudemd.ImportClaudeMD(syncDir, "## Git Conventions\n\nOriginal content.\n")
	require.NoError(t, err)

	// Simulate agent editing CLAUDE.md
	modified := "## Git Conventions\n\nUpdated content with new rules.\n"

	result, err := claudemd.Reconcile(syncDir, modified)
	require.NoError(t, err)

	assert.Len(t, result.Updated, 1)
	assert.Equal(t, "git-conventions", result.Updated[0])
	assert.Empty(t, result.New)
	assert.Empty(t, result.Deleted)
}

func TestReconcile_NewSection(t *testing.T) {
	syncDir := t.TempDir()

	_, err := claudemd.ImportClaudeMD(syncDir, "## Git Conventions\n\nOriginal.\n")
	require.NoError(t, err)

	modified := "## Git Conventions\n\nOriginal.\n\n## Debugging\n\nNew section.\n"

	result, err := claudemd.Reconcile(syncDir, modified)
	require.NoError(t, err)

	assert.Len(t, result.New, 1)
	assert.Equal(t, "Debugging", result.New[0].Header)
}

func TestReconcile_RenamedSection(t *testing.T) {
	syncDir := t.TempDir()

	_, err := claudemd.ImportClaudeMD(syncDir, "## Git Conventions\n\nNo force push. Use conventional commits.\n")
	require.NoError(t, err)

	modified := "## Git Commit Rules\n\nNo force push. Use conventional commits.\n"

	result, err := claudemd.Reconcile(syncDir, modified)
	require.NoError(t, err)

	assert.Len(t, result.Renamed, 1)
	assert.Equal(t, "git-conventions", result.Renamed[0].OldName)
	assert.Equal(t, "Git Commit Rules", result.Renamed[0].NewHeader)
}
```

**Step 2: Implement Reconcile**

```go
type ReconcileResult struct {
	Updated []string          // fragment names with updated content
	New     []Section         // sections not matching any fragment
	Deleted []string          // fragment names not found in current content
	Renamed []RenamedFragment // sections with similar content but different header
}

type RenamedFragment struct {
	OldName   string
	NewHeader string
}

func Reconcile(syncDir, currentContent string) (*ReconcileResult, error) {
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	manifest, err := ReadManifest(claudeMdDir)
	if err != nil {
		return nil, err
	}

	sections := Split(currentContent)
	result := &ReconcileResult{}

	matched := make(map[string]bool)   // fragment names that matched
	consumed := make(map[int]bool)     // section indices that matched

	// Pass 1: exact header match
	for i, s := range sections {
		for name, meta := range manifest.Fragments {
			if meta.Header == s.Header && !matched[name] {
				matched[name] = true
				consumed[i] = true
				// Check if content changed
				if ContentHash(s.Content) != meta.ContentHash {
					result.Updated = append(result.Updated, name)
					WriteFragment(claudeMdDir, name, s.Content)
					manifest.Fragments[name] = FragmentMeta{
						Header:      s.Header,
						ContentHash: ContentHash(s.Content),
					}
				}
				break
			}
		}
	}

	// Pass 2: unmatched sections — check for renames via content similarity
	for i, s := range sections {
		if consumed[i] {
			continue
		}
		bestMatch := ""
		bestSim := 0.0
		for name, meta := range manifest.Fragments {
			if matched[name] {
				continue
			}
			sim := ContentSimilarity(s.Content, readFragmentContent(claudeMdDir, name))
			if sim > bestSim {
				bestSim = sim
				bestMatch = name
			}
		}
		if bestSim > 0.8 && bestMatch != "" {
			result.Renamed = append(result.Renamed, RenamedFragment{
				OldName:   bestMatch,
				NewHeader: s.Header,
			})
			matched[bestMatch] = true
			consumed[i] = true
		} else {
			result.New = append(result.New, s)
			consumed[i] = true
		}
	}

	// Pass 3: unmatched fragments — deletions
	for name := range manifest.Fragments {
		if !matched[name] {
			result.Deleted = append(result.Deleted, name)
		}
	}

	WriteManifest(claudeMdDir, manifest)
	return result, nil
}
```

**Step 3: Run tests and commit**

```bash
git add internal/claudemd/
git commit -m "feat: add reconcile function for CLAUDE.md push disassembly"
```

---

### Task 19: Add CLAUDE.md import to Init flow

Update InitScan to detect `~/.claude/CLAUDE.md` and Init to import it into fragments.

**Files:**
- Modify: `internal/commands/init.go` (InitScanResult, InitOptions, Init)
- Modify: `cmd/claude-sync/cmd_init.go` (add step for CLAUDE.md import)
- Test: `internal/commands/init_test.go`

**Step 1: Write failing test**

```go
func TestInit_ImportsClaudeMD(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	// Write a CLAUDE.md file
	claudeMd := "# Preferences\n\n## Git Rules\n\nNo force push.\n"
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(claudeMd), 0644))

	opts := defaultInitOpts(claudeDir, syncDir, "")
	opts.ImportClaudeMD = true

	result, err := commands.Init(opts)
	require.NoError(t, err)

	assert.NotEmpty(t, result.ClaudeMDFragments)

	// Verify fragment files exist
	_, err = os.Stat(filepath.Join(syncDir, "claude-md", "manifest.yaml"))
	assert.NoError(t, err)

	// Verify config.yaml has claude_md section
	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(data)
	assert.NotEmpty(t, cfg.ClaudeMD.Include)
}
```

**Step 2: Implement**

Add `ImportClaudeMD bool` to InitOptions. In `Init()`, if true:

```go
if opts.ImportClaudeMD {
	claudeMdPath := filepath.Join(opts.ClaudeDir, "CLAUDE.md")
	if content, err := os.ReadFile(claudeMdPath); err == nil {
		importResult, err := claudemd.ImportClaudeMD(opts.SyncDir, string(content))
		if err == nil {
			cfg.ClaudeMD.Include = importResult.FragmentNames
			result.ClaudeMDFragments = importResult.FragmentNames
		}
	}
}
```

**Step 3: Add CLI wizard step**

In `cmd_init.go`, add `stepClaudeMD` after `stepConfigStyle` that checks for `~/.claude/CLAUDE.md` and asks if the user wants to import it.

**Step 4: Run tests and commit**

```bash
git add internal/commands/init.go internal/commands/init_test.go cmd/claude-sync/cmd_init.go
git commit -m "feat: add CLAUDE.md fragment import to init flow"
```

---

## Phase 3: MCP + Keybindings

### Task 20: Add MCP read/write to claudecode package

Add functions to read/write `~/.claude/.mcp.json`.

**Files:**
- Modify: `internal/claudecode/claudecode.go` (ReadMCPConfig, WriteMCPConfig)
- Test: `internal/claudecode/claudecode_test.go`

**Step 1: Write failing tests**

```go
func TestReadWriteMCPConfig(t *testing.T) {
	dir := t.TempDir()

	mcp := map[string]json.RawMessage{
		"context7": json.RawMessage(`{"type":"stdio","command":"npx","args":["-y","@context7/mcp"]}`),
	}

	err := claudecode.WriteMCPConfig(dir, mcp)
	require.NoError(t, err)

	loaded, err := claudecode.ReadMCPConfig(dir)
	require.NoError(t, err)
	assert.Contains(t, loaded, "context7")
}

func TestReadMCPConfig_Missing(t *testing.T) {
	dir := t.TempDir()
	loaded, err := claudecode.ReadMCPConfig(dir)
	require.NoError(t, err)
	assert.Empty(t, loaded)
}
```

**Step 2: Implement**

```go
func ReadMCPConfig(claudeDir string) (map[string]json.RawMessage, error) {
	path := filepath.Join(claudeDir, ".mcp.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, fmt.Errorf("reading MCP config: %w", err)
	}
	var mcp map[string]json.RawMessage
	if err := json.Unmarshal(data, &mcp); err != nil {
		return nil, fmt.Errorf("parsing MCP config: %w", err)
	}
	return mcp, nil
}

func WriteMCPConfig(claudeDir string, mcp map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(mcp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP config: %w", err)
	}
	path := filepath.Join(claudeDir, ".mcp.json")
	return os.WriteFile(path, append(data, '\n'), 0644)
}
```

**Step 3: Run tests and commit**

```bash
git add internal/claudecode/
git commit -m "feat: add MCP config read/write to claudecode package"
```

---

### Task 21: Add MCP and Keybindings to Config and Profile

Add `MCP map[string]json.RawMessage` and `Keybindings map[string]any` to Config. Add profile support with add/remove.

**Files:**
- Modify: `internal/config/config.go` (ConfigV2, Parse, MarshalV2)
- Modify: `internal/profiles/profile.go` (Profile, ParseProfile, MarshalProfile, MergeMCP, MergeKeybindings)
- Test: both test files

**Step 1: Write failing tests for Config**

```go
func TestParse_MCP(t *testing.T) {
	data := []byte(`
version: "1.0.0"
mcp:
  context7:
    type: stdio
    command: npx
    args: [-y, "@context7/mcp"]
`)
	cfg, err := config.Parse(data)
	require.NoError(t, err)
	assert.Contains(t, cfg.MCP, "context7")
}
```

**Step 2: Write failing tests for Profile**

```go
func TestMergeMCP(t *testing.T) {
	base := map[string]json.RawMessage{
		"context7": json.RawMessage(`{"type":"stdio"}`),
	}
	profile := profiles.Profile{
		MCP: profiles.ProfileMCP{
			Add:    map[string]json.RawMessage{"linear": json.RawMessage(`{"type":"stdio"}`)},
			Remove: []string{"context7"},
		},
	}

	result := profiles.MergeMCP(base, profile)
	assert.Contains(t, result, "linear")
	assert.NotContains(t, result, "context7")
}
```

**Step 3: Implement**

Add to ConfigV2:
```go
MCP        map[string]json.RawMessage `yaml:"-"`
Keybindings map[string]any            `yaml:"keybindings,omitempty"`
```

Add profile types:
```go
type ProfileMCP struct {
	Add    map[string]json.RawMessage `yaml:"add,omitempty"`
	Remove []string                   `yaml:"remove,omitempty"`
}

type ProfileKeybindings struct {
	Override map[string]any `yaml:"override,omitempty"`
}
```

MergeMCP follows same pattern as MergeHooks. MergeKeybindings follows MergeSettings.

Parse MCP values as `map[string]any` in YAML and convert to `json.RawMessage` (same pattern hooks use for storing JSON in YAML).

**Step 4: Run tests and commit**

```bash
git add internal/config/ internal/profiles/
git commit -m "feat: add MCP and keybindings to config and profile"
```

---

### Task 22: Integrate MCP and Keybindings into Pull and Push

Apply MCP to `~/.claude/.mcp.json` and keybindings to `~/.claude/keybindings.json` during pull. Scan both during push.

**Files:**
- Modify: `internal/commands/pull.go` (apply MCP and keybindings)
- Modify: `internal/commands/push.go` (scan MCP and keybindings)
- Test: both test files

**Step 1: Write failing tests**

```go
func TestPull_AppliesMCP(t *testing.T) {
	// Setup config with MCP servers
	// Pull and verify ~/.claude/.mcp.json was written
}

func TestPull_AppliesKeybindings(t *testing.T) {
	// Setup config with keybindings
	// Pull and verify ~/.claude/keybindings.json was written
}
```

**Step 2: Implement**

MCP application: additive merge (same as permissions — add new servers, don't remove existing local ones).

Keybindings application: simple overlay (same as settings).

MCP and hooks are classified as high-risk in the tiered approval system. Keybindings are safe.

**Step 3: Run tests and commit**

```bash
git add internal/commands/pull.go internal/commands/push.go internal/commands/pull_test.go internal/commands/push_test.go
git commit -m "feat: integrate MCP and keybindings into pull and push flows"
```

---

### Task 23: Add MCP and Keybindings to Init flow

Scan for existing `.mcp.json` and `keybindings.json` during InitScan. Add CLI wizard steps.

**Files:**
- Modify: `internal/commands/init.go`
- Modify: `cmd/claude-sync/cmd_init.go`
- Test: `internal/commands/init_test.go`

**Step 1: Implement scanning**

```go
// In InitScan, add MCP scanning
mcpConfig, err := claudecode.ReadMCPConfig(claudeDir)
if err == nil && len(mcpConfig) > 0 {
	result.MCP = mcpConfig
}

// Keybindings scanning
keybindingsPath := filepath.Join(claudeDir, "keybindings.json")
if data, err := os.ReadFile(keybindingsPath); err == nil {
	var kb map[string]any
	if json.Unmarshal(data, &kb) == nil && len(kb) > 0 {
		result.Keybindings = kb
	}
}
```

**Step 2: Add CLI steps for MCP and keybindings selection**

**Step 3: Run tests and commit**

```bash
git add internal/commands/init.go cmd/claude-sync/cmd_init.go internal/commands/init_test.go
git commit -m "feat: add MCP and keybindings scanning to init"
```

---

## Phase 4: Auto-Sync Hooks

### Task 24: Add auto-commit command

New CLI command that checks watched files for changes and creates local commits.

**Files:**
- Create: `cmd/claude-sync/cmd_autocommit.go`
- Create: `internal/commands/autocommit.go`
- Test: `internal/commands/autocommit_test.go`

**Step 1: Write failing test**

```go
func TestAutoCommit_DetectsClaudeMDChange(t *testing.T) {
	claudeDir, syncDir := setupInitializedEnvWithClaudeMD(t)

	// Modify CLAUDE.md
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte("## New\n\nContent.\n"), 0644)

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "auto:")

	// Verify git commit was created
	log := gitRun(t, syncDir, "log", "--oneline", "-1")
	assert.Contains(t, log, "auto:")
}

func TestAutoCommit_NoChangeNoCommit(t *testing.T) {
	claudeDir, syncDir := setupInitializedEnvWithClaudeMD(t)

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.False(t, result.Changed)
}
```

**Step 2: Implement**

```go
package commands

type AutoCommitResult struct {
	Changed       bool
	CommitMessage string
	FilesChanged  []string
}

func AutoCommit(claudeDir, syncDir string) (*AutoCommitResult, error) {
	result := &AutoCommitResult{}

	// Check CLAUDE.md
	claudeMdPath := filepath.Join(claudeDir, "CLAUDE.md")
	if content, err := os.ReadFile(claudeMdPath); err == nil {
		reconcileResult, err := claudemd.Reconcile(syncDir, string(content))
		if err == nil && (len(reconcileResult.Updated) > 0 || len(reconcileResult.New) > 0) {
			result.Changed = true
			result.FilesChanged = append(result.FilesChanged, "CLAUDE.md")
		}
	}

	// Check settings.json — compare syncable fields against config
	// ... similar logic for settings, MCP

	if result.Changed {
		// Stage and commit
		result.CommitMessage = "auto: " + strings.Join(result.FilesChanged, ", ")
		git.StageAll(syncDir)
		git.Commit(syncDir, result.CommitMessage)
	}

	return result, nil
}
```

**Step 3: Add CLI command**

```go
// cmd/claude-sync/cmd_autocommit.go
var autoCommitCmd = &cobra.Command{
	Use:   "auto-commit",
	Short: "Check watched files and commit changes",
	RunE: func(cmd *cobra.Command, args []string) error {
		ifChanged, _ := cmd.Flags().GetBool("if-changed")
		// ... invoke commands.AutoCommit
	},
}
```

**Step 4: Run tests and commit**

```bash
git add cmd/claude-sync/cmd_autocommit.go internal/commands/autocommit.go internal/commands/autocommit_test.go
git commit -m "feat: add auto-commit command for incremental change tracking"
```

---

### Task 25: Add --auto mode to pull and push

Add push-first behavior to `pull --auto` and quiet batch push to `push --auto`.

**Files:**
- Modify: `internal/commands/pull.go` (push-first logic)
- Modify: `internal/commands/push.go` (--auto quiet mode)
- Modify: `cmd/claude-sync/cmd_pull.go` (add --auto flag)
- Modify: `cmd/claude-sync/cmd_push.go` (add --auto flag)
- Test: both test files

**Step 1: Write failing test for push-first**

```go
func TestPull_PushesUnpushedCommitsFirst(t *testing.T) {
	// Setup: init with remote, make local auto-commits without pushing
	// Run Pull with Auto=true
	// Verify local commits were pushed before pull
}
```

**Step 2: Implement**

In Pull, when `Auto` is true:
```go
if opts.Auto {
	// Push any unpushed local auto-commits first
	if git.HasUnpushedCommits(syncDir) {
		git.Push(syncDir)
	}
}
```

**Step 3: Run tests and commit**

```bash
git add internal/commands/pull.go internal/commands/push.go cmd/claude-sync/
git commit -m "feat: add --auto mode to pull (push-first) and push (quiet batch)"
```

---

### Task 26: Update setup command to register auto-sync hooks

When `claude-sync setup` is run, register PostToolUse, SessionEnd, and SessionStart hooks for auto-sync.

**Files:**
- Modify: `cmd/claude-sync/cmd_setup.go` or `internal/commands/setup.go`
- Test: integration test

**Step 1: Implement**

Register hooks in `settings.json`:

```json
{
  "hooks": {
    "PostToolUse": [{"matcher": "Write|Edit", "hooks": [{"type": "command", "command": "claude-sync auto-commit --if-changed"}]}],
    "SessionEnd": [{"matcher": "", "hooks": [{"type": "command", "command": "claude-sync push --auto --quiet"}]}],
    "SessionStart": [{"matcher": "", "hooks": [{"type": "command", "command": "claude-sync pull --auto"}]}]
  }
}
```

Important: Merge these hooks alongside any existing hooks, don't overwrite.

**Step 2: Run tests and commit**

```bash
git add cmd/claude-sync/ internal/commands/
git commit -m "feat: register auto-sync hooks during setup"
```

---

## Phase 5: Approve/Reject Commands

### Task 27: Add approve command

Interactive review and approval of pending high-risk changes.

**Files:**
- Create: `cmd/claude-sync/cmd_approve.go`
- Create: `internal/commands/approve.go`
- Test: `internal/commands/approve_test.go`

**Step 1: Write failing test**

```go
func TestApprove_AppliesPendingChanges(t *testing.T) {
	claudeDir, syncDir := setupInitializedEnv(t)

	// Write pending-changes.yaml with permission changes
	pending := approval.PendingChanges{
		Commit: "abc123",
		Permissions: &config.Permissions{
			Allow: []string{"Bash(kubectl *)"},
		},
	}
	require.NoError(t, approval.WritePending(syncDir, pending))

	// Approve
	err := commands.Approve(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify permissions were applied
	settings, _ := claudecode.ReadSettings(claudeDir)
	var perms struct{ Allow []string }
	json.Unmarshal(settings["permissions"], &perms)
	assert.Contains(t, perms.Allow, "Bash(kubectl *)")

	// Verify pending-changes.yaml was cleared
	loaded, _ := approval.ReadPending(syncDir)
	assert.True(t, loaded.IsEmpty())
}
```

**Step 2: Implement**

```go
func Approve(claudeDir, syncDir string) error {
	pending, err := approval.ReadPending(syncDir)
	if err != nil {
		return err
	}
	if pending.IsEmpty() {
		return fmt.Errorf("no pending changes to approve")
	}

	// Apply each pending change
	if pending.Permissions != nil {
		// Additive merge into settings.json
	}
	if pending.MCP != nil {
		// Additive merge into .mcp.json
	}
	if pending.Hooks != nil {
		// Merge into settings.json hooks
	}

	return approval.ClearPending(syncDir)
}
```

**Step 3: Add CLI command**

**Step 4: Run tests and commit**

```bash
git add cmd/claude-sync/cmd_approve.go internal/commands/approve.go internal/commands/approve_test.go
git commit -m "feat: add approve command for pending high-risk changes"
```

---

### Task 28: Add reject command

Reject and clear pending high-risk changes without applying them.

**Files:**
- Create: `cmd/claude-sync/cmd_reject.go`
- Create: `internal/commands/reject.go`
- Test: `internal/commands/reject_test.go`

**Step 1: Write failing test**

```go
func TestReject_ClearsPendingChanges(t *testing.T) {
	syncDir := t.TempDir()

	pending := approval.PendingChanges{Commit: "abc123"}
	require.NoError(t, approval.WritePending(syncDir, pending))

	err := commands.Reject(syncDir)
	require.NoError(t, err)

	loaded, _ := approval.ReadPending(syncDir)
	assert.True(t, loaded.IsEmpty())
}
```

**Step 2: Implement**

```go
func Reject(syncDir string) error {
	return approval.ClearPending(syncDir)
}
```

**Step 3: Run tests and commit**

```bash
git add cmd/claude-sync/cmd_reject.go internal/commands/reject.go internal/commands/reject_test.go
git commit -m "feat: add reject command to clear pending high-risk changes"
```

---

### Task 29: Update status command to show pending changes

When `claude-sync status` is run, show any pending high-risk changes waiting for approval.

**Files:**
- Modify: `internal/commands/status.go` (StatusResult struct)
- Modify: `cmd/claude-sync/cmd_status.go` (display pending changes)
- Test: `internal/commands/status_test.go`

**Step 1: Implement**

Add to StatusResult:
```go
PendingChanges *approval.PendingChanges
```

In Status(), read pending-changes.yaml:
```go
pending, _ := approval.ReadPending(syncDir)
if !pending.IsEmpty() {
	result.PendingChanges = &pending
}
```

In the CLI, display pending changes with a warning banner.

**Step 2: Run tests and commit**

```bash
git add internal/commands/status.go cmd/claude-sync/cmd_status.go internal/commands/status_test.go
git commit -m "feat: show pending changes in status command"
```

---

### Task 30: End-to-end integration test

Full workflow test: init with all surfaces → push → join on second machine → pull with tiered approval.

**Files:**
- Modify: `tests/integration_test.go`

**Step 1: Write integration test**

```go
func TestFullConfigSync_EndToEnd(t *testing.T) {
	// Machine 1: init with plugins, settings, permissions, CLAUDE.md, MCP
	// Push to remote
	// Machine 2: join, pull
	// Verify all surfaces were synced correctly
	// Verify high-risk changes went to pending
	// Approve and verify applied
}
```

**Step 2: Run integration tests**

Run: `go test ./tests/ -tags integration -v`
Expected: All pass.

**Step 3: Commit**

```bash
git add tests/integration_test.go
git commit -m "test: add end-to-end integration test for full config sync"
```

---

## Summary

| Phase | Tasks | New Files | Modified Files |
|---|---|---|---|
| 1: Settings + Permissions | 1-10 | `internal/approval/` | `config.go`, `profile.go`, `init.go`, `pull.go`, `push.go`, CLI files |
| 2: CLAUDE.md Fragments | 11-19 | `internal/claudemd/` | `config.go`, `profile.go`, `init.go`, `pull.go`, `push.go`, CLI files |
| 3: MCP + Keybindings | 20-23 | — | `claudecode.go`, `config.go`, `profile.go`, `init.go`, `pull.go`, `push.go` |
| 4: Auto-Sync Hooks | 24-26 | `cmd_autocommit.go`, `autocommit.go` | `pull.go`, `push.go`, `setup.go` |
| 5: Approve/Reject | 27-30 | `cmd_approve.go`, `cmd_reject.go`, `approve.go`, `reject.go` | `status.go`, integration tests |

Each phase is independently shippable. Phase 1 is the highest-value starting point because it unlocks permissions and expanded settings with no new conceptual complexity.
