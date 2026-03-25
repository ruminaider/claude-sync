# Fix config update merge for profiles and marketplaces (#37)

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend the PR #36 merge logic so `config update` preserves profile-level add items and marketplace entries, not just base config items.

**Architecture:** A unified `MergeExisting` function replaces the current `MergeExistingConfig`, handling both base config items and profile-add items in a single call with shared per-section helpers. Marketplace entries are threaded through `InitOptions` and also collected from profile-add plugins. Original profile values for value-carrying sections (MCP, hooks, settings, keybindings) are stored in the TUI model so `diffsToProfile()` preserves them.

**Tech Stack:** Go, testify

---

## Context

PR #36 added `MergeExistingConfig()` to prevent `config update` from silently dropping items that exist only in config (not detected locally). Two gaps remain:

1. **Marketplace entries** are derived only from locally-installed plugins in `buildAndWriteConfig()`. Custom marketplace entries referencing plugins not installed locally are dropped.
2. **Profile-level items** (plugins.add, mcp.add, etc.) go through a separate code path. Profile-add items not in the local scan don't appear in pickers and are silently lost during reconstruction.

### Review decisions incorporated

This plan was updated after a thorough review (P0+P1 issues). Key structural changes from the original:

- **Shared merge helpers** (DRY): Existing `merge*` functions are refactored to accept data directly. Both base-config and profile merging call the same helpers.
- **Combined `MergeExisting` function**: Replaces separate `MergeExistingConfig`/`MergeExistingProfiles` calls, eliminating an implicit ordering dependency.
- **Forked-plugin routing**: Profile-add plugins are checked for the forked marketplace suffix and routed to `scan.AutoForked` when appropriate.
- **Profile marketplace generation**: `buildAndWriteConfig` collects marketplace IDs from profile-add plugins, not just base plugins.
- **Keybindings in `profileValues`**: All four value-carrying sections are covered.
- **Marketplace URL validation**: Added at the registration/clone path.
- **Full test matrix**: Every section x dimension combination has a named test.

---

## Task 1: Refactor merge helpers to accept data directly

**Files:**
- Modify: `internal/commands/merge_config.go`
- Test: `internal/commands/merge_config_test.go`

### Rationale

The existing `merge*` functions extract data from `config.Config` fields internally. Refactoring them to accept the data as parameters lets both base-config merging and profile merging call the same helpers, eliminating the DRY violation of maintaining 16 near-identical functions.

### Step 1: Refactor slice-based helpers

Change signatures to accept data directly instead of `*config.Config`:

```go
// Before:
func mergeUpstreamPlugins(scan *InitScanResult, cfg *config.Config) { ... }

// After:
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

func mergeForkedPlugins(scan *InitScanResult, keys []string) {
    for _, key := range keys {
        if slices.Contains(scan.PluginKeys, key) {
            continue
        }
        scan.AutoForked = append(scan.AutoForked, key)
        scan.PluginKeys = append(scan.PluginKeys, key)
        scan.ConfigOnly[configOnlyPluginPrefix+key] = true
    }
}
```

Apply the same refactor to all helpers:
- `mergeSettings(scan, settings map[string]any)`
- `mergeHooks(scan, hooks map[string]json.RawMessage)`
- `mergePermissions(scan, allow []string, deny []string)`
- `mergeMCP(scan, mcp map[string]json.RawMessage)`
- `mergeKeybindings(scan, keybindings map[string]any)`
- `mergeClaudeMDFragments(scan, fragments []string, syncDir string)`
- `mergeCommandsSkills(scan, commands []string, skills []string, syncDir string)`
- `mergeMemory(scan, memoryFiles []string)` (new, extracted from any inline logic)

### Step 2: Update MergeExistingConfig callers

Update `MergeExistingConfig` to pass extracted fields to the refactored helpers:

```go
func MergeExistingConfig(scan *InitScanResult, cfg *config.Config, syncDir string) {
    if scan.ConfigOnly == nil {
        scan.ConfigOnly = make(map[string]bool)
    }
    mergeUpstreamPlugins(scan, cfg.Upstream)
    mergeForkedPlugins(scan, cfg.Forked)
    mergeSettings(scan, cfg.Settings)
    mergeHooks(scan, cfg.Hooks)
    mergePermissions(scan, cfg.Permissions.Allow, cfg.Permissions.Deny)
    mergeMCP(scan, cfg.MCP)
    mergeKeybindings(scan, cfg.Keybindings)
    mergeClaudeMDFragments(scan, cfg.ClaudeMDFragments, syncDir)
    mergeCommandsSkills(scan, cfg.Commands, cfg.Skills, syncDir)
}
```

### Step 3: Verify existing tests still pass

```
go test ./internal/commands/... -v -run TestMergeExisting
```

The existing tests should pass unchanged since the behavior is identical; only the internal function signatures changed.

### Step 4: Commit

---

## Task 2: Add combined `MergeExisting` function with profile support

**Files:**
- Modify: `internal/commands/merge_config.go`
- Test: `internal/commands/merge_config_test.go`

### Rationale

Combining base-config and profile merging into a single function eliminates the implicit ordering dependency (base must run before profiles for correct dedup). The function first processes base config items, then iterates profiles, calling the same shared helpers.

### Step 1: Write failing tests

See the **Test Matrix** section below for the complete list. Write all tests first.

### Step 2: Run tests to verify they fail

```
go test ./internal/commands/... -v -run TestMergeExisting
```

### Step 3: Implement MergeExisting

Replace `MergeExistingConfig` with `MergeExisting`:

```go
// MergeExisting injects config-only items from the base config and profile-add
// sections into the scan, so they appear in TUI pickers with [config] tags.
// Base config items are processed first, then profile-add items. This ordering
// ensures profile dedup checks see base items that were already injected.
func MergeExisting(scan *InitScanResult, cfg *config.Config, existingProfiles map[string]profiles.Profile, syncDir string) {
    if scan.ConfigOnly == nil {
        scan.ConfigOnly = make(map[string]bool)
    }

    // Phase 1: Base config items
    if cfg != nil {
        mergeUpstreamPlugins(scan, cfg.Upstream)
        mergeForkedPlugins(scan, cfg.Forked)
        mergeSettings(scan, cfg.Settings)
        mergeHooks(scan, cfg.Hooks)
        mergePermissions(scan, cfg.Permissions.Allow, cfg.Permissions.Deny)
        mergeMCP(scan, cfg.MCP)
        mergeKeybindings(scan, cfg.Keybindings)
        mergeClaudeMDFragments(scan, cfg.ClaudeMDFragments, syncDir)
        mergeCommandsSkills(scan, cfg.Commands, cfg.Skills, syncDir)
    }

    // Phase 2: Profile-add items
    for _, profile := range existingProfiles {
        // Route profile plugins to upstream or forked based on marketplace suffix
        var upstreamKeys, forkedKeys []string
        for _, key := range profile.Plugins.Add {
            parts := strings.SplitN(key, "@", 2)
            if len(parts) == 2 && parts[1] == forkedplugins.MarketplaceName {
                forkedKeys = append(forkedKeys, key)
            } else {
                upstreamKeys = append(upstreamKeys, key)
            }
        }
        mergeUpstreamPlugins(scan, upstreamKeys)
        mergeForkedPlugins(scan, forkedKeys)

        mergeSettings(scan, profile.Settings)
        mergeHooks(scan, profile.Hooks.Add)
        mergePermissions(scan, profile.Permissions.Add.Allow, profile.Permissions.Add.Deny)
        mergeMCP(scan, profile.MCP.Add)
        mergeKeybindings(scan, profile.Keybindings.Override)
        mergeClaudeMDFragments(scan, profile.ClaudeMD.Add, syncDir)
        mergeCommandsSkills(scan, profile.Commands.Add, profile.Skills.Add, syncDir)
        mergeMemory(scan, profile.Memory.Add)
    }
}
```

### Step 4: Update `runConfigFlow` call site

In `cmd/claude-sync/cmd_init.go`, replace the `MergeExistingConfig` call (line 100-102):

```go
// Before:
if isUpdate && existingConfig != nil {
    commands.MergeExistingConfig(scan, existingConfig, syncDir)
}

// After:
if isUpdate {
    commands.MergeExisting(scan, existingConfig, existingProfiles, syncDir)
}
```

### Step 5: Remove the old `MergeExistingConfig` function

Delete `MergeExistingConfig` from `merge_config.go`. Update any remaining references (tests that call it directly should call `MergeExisting` instead).

### Step 6: Run tests, verify, commit

```
go test ./internal/commands/... -v -run TestMergeExisting
```

---

## Task 3: Marketplace entries

**Files:**
- Modify: `internal/commands/init.go` (InitOptions, buildAndWriteConfig)
- Modify: `cmd/claude-sync/tui/root.go` (buildInitOptions)
- Modify: `internal/marketplace/marketplace.go` (URL validation)
- Test: `internal/commands/init_test.go` or `marketplace_merge_test.go`

### Part A: Add ExtraMarketplaces to InitOptions

In `internal/commands/init.go`, add to `InitOptions` after the existing `ExtraSettings` field:

```go
ExtraMarketplaces map[string]config.MarketplaceSource // config-only marketplace sources to preserve
```

### Part B: Collect marketplace IDs from profile-add plugins

In `buildAndWriteConfig`, after the existing marketplace ID collection from `upstream` (lines 561-574), also collect from profile-add plugins:

```go
// Collect marketplace IDs from upstream plugins
mktIDs := make(map[string]bool)
for _, key := range upstream {
    parts := strings.SplitN(key, "@", 2)
    if len(parts) == 2 {
        mktIDs[parts[1]] = true
    }
}

// Also collect marketplace IDs from profile-add plugins
for _, profile := range opts.Profiles {
    for _, key := range profile.Plugins.Add {
        parts := strings.SplitN(key, "@", 2)
        if len(parts) == 2 {
            mktIDs[parts[1]] = true
        }
    }
}

var mktIDList []string
for id := range mktIDs {
    mktIDList = append(mktIDList, id)
}
customMkts := marketplace.CollectCustomMarketplaceSources(claudeDir, mktIDList)
```

### Part C: Merge ExtraMarketplaces

After the `CollectCustomMarketplaceSources` call, merge extras:

```go
for id, src := range opts.ExtraMarketplaces {
    if customMkts == nil {
        customMkts = make(map[string]config.MarketplaceSource)
    }
    if _, exists := customMkts[id]; !exists {
        customMkts[id] = src
    }
}
```

### Part D: Populate ExtraMarketplaces in TUI buildInitOptions

In `cmd/claude-sync/tui/root.go`, in `buildInitOptions()` around line 1267:

```go
if m.editMode && m.existingConfig != nil && len(m.existingConfig.Marketplaces) > 0 {
    opts.ExtraMarketplaces = m.existingConfig.Marketplaces
}
```

### Part E: Add marketplace URL validation

In `internal/marketplace/marketplace.go`, add validation in `EnsureRegistered` (line 744) before processing each source:

```go
func validateMarketplaceSource(id string, src config.MarketplaceSource) error {
    switch src.Source {
    case "github", "git":
        // valid source types
    default:
        return fmt.Errorf("marketplace %q has unknown source type %q", id, src.Source)
    }
    if src.Source == "github" {
        // Validate owner/repo format
        parts := strings.SplitN(src.Repo, "/", 3)
        if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
            return fmt.Errorf("marketplace %q has invalid github repo format %q (expected owner/repo)", id, src.Repo)
        }
    }
    return nil
}
```

Call `validateMarketplaceSource` in `EnsureRegistered` before `cloneMarketplace`.

### Part F: Write integration test

Set up a temp dir with a minimal filesystem, write an existing config with a marketplace entry, call the update flow with `ExtraMarketplaces`, verify the output `config.yaml` includes the marketplace entry. This tests the full plumbing, not just the merge logic.

### Step: Run tests, verify, commit

```
go test ./internal/commands/... ./internal/marketplace/... -v -run TestMarketplace
```

---

## Task 4: Store original profile values in TUI model

**Files:**
- Modify: `cmd/claude-sync/tui/root.go` (Model struct, New(), restoreProfiles(), resetToDefaults())
- Modify: `cmd/claude-sync/tui/profilediff.go` (diffsToProfile())
- Test: `cmd/claude-sync/tui/profilediff_test.go`

### Step 1: Add profileValues type and field

In `cmd/claude-sync/tui/root.go`, add a type (near the Model struct):

```go
// profileValues stores original values from a profile's add sections.
// Used when profile values differ from scan values (e.g., MCP server
// with uvx config in profile vs Docker config in local scan).
type profileValues struct {
    MCP         map[string]json.RawMessage
    Hooks       map[string]json.RawMessage
    Settings    map[string]any
    Keybindings map[string]any
}
```

Add field to Model struct:

```go
profileAddValues map[string]profileValues // profileName -> original add values
```

Initialize in `New()`:

```go
profileAddValues: make(map[string]profileValues),
```

Reset in `resetToDefaults()` (around line 1006, where `profileDiffs` and other state is cleared):

```go
m.profileAddValues = make(map[string]profileValues)
```

### Step 2: Populate profileAddValues during restoreProfiles

In `restoreProfiles()`, after `profile := existingProfiles[name]` (line 1753), store original values:

```go
vals := profileValues{}
if len(profile.MCP.Add) > 0 {
    vals.MCP = make(map[string]json.RawMessage, len(profile.MCP.Add))
    for k, v := range profile.MCP.Add {
        vals.MCP[k] = v
    }
}
if len(profile.Hooks.Add) > 0 {
    vals.Hooks = make(map[string]json.RawMessage, len(profile.Hooks.Add))
    for k, v := range profile.Hooks.Add {
        vals.Hooks[k] = v
    }
}
if len(profile.Settings) > 0 {
    vals.Settings = make(map[string]any, len(profile.Settings))
    for k, v := range profile.Settings {
        vals.Settings[k] = v
    }
}
if len(profile.Keybindings.Override) > 0 {
    vals.Keybindings = make(map[string]any, len(profile.Keybindings.Override))
    for k, v := range profile.Keybindings.Override {
        vals.Keybindings[k] = v
    }
}
m.profileAddValues[name] = vals
```

### Step 3: Update diffsToProfile to use stored values

In `cmd/claude-sync/tui/profilediff.go`, in `diffsToProfile()`:

**Settings** (around line 395):

```go
for k := range settingsDiff.adds {
    if vals, ok := m.profileAddValues[name]; ok {
        if v, ok := vals.Settings[k]; ok {
            p.Settings[k] = v
            continue
        }
    }
    if v, ok := m.scanResult.Settings[k]; ok {
        p.Settings[k] = v
    }
}
```

**MCP** (around line 429):

```go
for k := range mcpDiff.adds {
    if vals, ok := m.profileAddValues[name]; ok {
        if raw, ok := vals.MCP[k]; ok {
            mcpAdd[k] = raw
            continue
        }
    }
    if raw, ok := m.scanResult.MCP[k]; ok {
        mcpAdd[k] = raw
    } else if raw, ok := m.discoveredMCP[k]; ok {
        mcpAdd[k] = raw
    }
}
```

**Hooks** (around line 454):

```go
for k := range hooksDiff.adds {
    if vals, ok := m.profileAddValues[name]; ok {
        if raw, ok := vals.Hooks[k]; ok {
            hookAdd[k] = raw
            continue
        }
    }
    if raw, ok := m.scanResult.Hooks[k]; ok {
        hookAdd[k] = raw
    }
}
```

**Keybindings** (around line 469):

```go
for k := range keybindingsDiff.adds {
    if vals, ok := m.profileAddValues[name]; ok {
        if v, ok := vals.Keybindings[k]; ok {
            keybindingsOverride[k] = v
            continue
        }
    }
    if v, ok := m.scanResult.Keybindings[k]; ok {
        keybindingsOverride[k] = v
    }
}
```

### Step 4: Run tests, verify, commit

```
go test ./cmd/claude-sync/tui/... -v
```

---

## Test Matrix

All tests go in `internal/commands/merge_config_test.go` (merge helper tests) and `cmd/claude-sync/tui/profilediff_test.go` (TUI value preservation tests).

### Merge helper tests (merge_config_test.go)

Each shared helper needs tests for both base-config and profile-add sources.

| Dimension | Plugins | MCP | Hooks | Settings | Keybindings | Permissions | CLAUDE.md | Cmds/Skills | Memory | Marketplace |
|-----------|---------|-----|-------|----------|-------------|-------------|-----------|-------------|--------|-------------|
| **Inject** | `TestMergeUpstream_InjectsNew` | `TestMergeMCP_InjectsNew` | `TestMergeHooks_InjectsNew` | `TestMergeSettings_InjectsNew` | `TestMergeKeybindings_InjectsNew` | `TestMergePermissions_InjectsNew` | `TestMergeClaudeMD_InjectsNew` | `TestMergeCmdSkills_InjectsNew` | `TestMergeMemory_InjectsNew` | (integration) |
| **Dedup** | `TestMergeUpstream_SkipsExisting` | `TestMergeMCP_SkipsExisting` | `TestMergeHooks_SkipsExisting` | `TestMergeSettings_SkipsExisting` | `TestMergeKeybindings_SkipsExisting` | `TestMergePermissions_SkipsExisting` | `TestMergeClaudeMD_SkipsExisting` | `TestMergeCmdSkills_SkipsExisting` | `TestMergeMemory_SkipsExisting` | (integration) |
| **Nil-init** | `TestMergeUpstream_NilConfigOnly` | `TestMergeMCP_NilMap` | `TestMergeHooks_NilMap` | `TestMergeSettings_NilMap` | `TestMergeKeybindings_NilMap` | `TestMergePermissions_NilSlices` | `TestMergeClaudeMD_NilSections` | `TestMergeCmdSkills_NilResult` | `TestMergeMemory_NilSlice` | - |
| **Value preserve** | - | `TestMergeMCP_PreservesJSON` | `TestMergeHooks_PreservesJSON` | `TestMergeSettings_PreservesValue` | `TestMergeKeybindings_PreservesValue` | - | - | - | - | - |

### Combined MergeExisting tests (merge_config_test.go)

| Test | Description |
|------|-------------|
| `TestMergeExisting_BaseConfigOnly` | Base config items injected, no profiles |
| `TestMergeExisting_ProfilesOnly` | No base config, profile-add items injected |
| `TestMergeExisting_BaseAndProfiles` | Both base and profile items, profile dedup skips base items |
| `TestMergeExisting_MultiProfileDedup` | Two profiles adding same item, scan contains it once |
| `TestMergeExisting_ForkedProfilePlugin` | Profile-add plugin with `@claude-sync-forks` routed to `scan.AutoForked` |
| `TestMergeExisting_ConflictBaseVsProfile` | Same MCP key in base config and profile with different values; base value in scan, profile value in `profileAddValues` |
| `TestMergeExisting_EmptyScan` | Both base and profile items injected into completely empty scan |

### TUI value preservation tests (profilediff_test.go)

| Test | Description |
|------|-------------|
| `TestDiffsToProfile_PrefersMCPProfileValue` | Profile MCP value differs from scan; `diffsToProfile` returns the profile value |
| `TestDiffsToProfile_PrefersHooksProfileValue` | Profile hooks value differs from scan; `diffsToProfile` returns the profile value |
| `TestDiffsToProfile_PrefersSettingsProfileValue` | Profile setting value differs from scan; `diffsToProfile` returns the profile value |
| `TestDiffsToProfile_PrefersKeybindingsProfileValue` | Profile keybinding value differs from scan; `diffsToProfile` returns the profile value |
| `TestDiffsToProfile_FallsBackToScanValue` | Item not in `profileAddValues`; `diffsToProfile` uses scan value |
| `TestDiffsToProfile_SecretStrippingOnProfileValues` | Profile MCP value contains a raw secret; verify `DetectMCPSecrets`/`ReplaceSecrets` still strips it |
| `TestDiffsToProfile_DeselectedProfileItemRemoved` | Profile-add item deselected in picker; verify it does not appear in output |
| `TestProfileAddValues_PopulatedDuringRestore` | After `restoreProfiles`, `m.profileAddValues[name]` contains correct MCP/hooks/settings/keybindings |
| `TestProfileAddValues_ResetOnRebuildAll` | After `resetToDefaults`, `profileAddValues` is empty |

### Integration tests

| Test | Description |
|------|-------------|
| `TestMarketplace_ExtraPreservedInOutput` | Integration: temp dir with existing config containing marketplace entry; run update flow; verify output config.yaml contains the entry |
| `TestMarketplace_ProfilePluginGeneratesEntry` | Integration: profile-add plugin from custom marketplace; verify marketplace entry appears in output |
| `TestMarketplace_ValidatesRepoFormat` | Unit: invalid `Repo` format rejected by `validateMarketplaceSource` |
| `TestMarketplace_ValidatesSourceType` | Unit: unknown `Source` type rejected by `validateMarketplaceSource` |
| `TestEndToEnd_ProfileItemRoundTrip` | Integration: merge -> restore -> diffsToProfile -> buildInitOptions; verify profile-add items survive the full cycle |

---

## Verification

1. **Merge helper tests**: `go test ./internal/commands/... -v -run "TestMerge"`
2. **Combined merge tests**: `go test ./internal/commands/... -v -run "TestMergeExisting"`
3. **TUI value preservation tests**: `go test ./cmd/claude-sync/tui/... -v -run "TestDiffsToProfile|TestProfileAddValues"`
4. **Integration tests**: `go test ./internal/commands/... ./cmd/claude-sync/tui/... -v -run "TestMarketplace|TestEndToEnd"`
5. **Marketplace validation**: `go test ./internal/marketplace/... -v -run "TestValidate"`
6. **Full test suite**: `go test ./...`
7. **Manual test**:
   - Create a config with a custom marketplace entry and a profile with plugins.add/mcp.add
   - Run `claude-sync config update` on a machine where those plugins aren't installed
   - Verify the marketplace entry and profile items survive in the output config/profile YAML

## File Changes Summary

| File | Change |
|------|--------|
| `internal/commands/merge_config.go` | Refactor `merge*` helpers to accept data directly; replace `MergeExistingConfig` with `MergeExisting` that handles both base config and profiles; add forked-plugin routing for profile plugins; add `mergeMemory` helper |
| `internal/commands/merge_config_test.go` | Full test matrix: per-section inject/dedup/nil-init/value-preserve tests; combined `MergeExisting` tests for multi-profile dedup, forked routing, base-vs-profile conflict, empty scan |
| `internal/commands/init.go` | Add `ExtraMarketplaces` to `InitOptions`; collect marketplace IDs from profile-add plugins in `buildAndWriteConfig()`; merge `ExtraMarketplaces` after `CollectCustomMarketplaceSources` |
| `internal/commands/init_test.go` | Integration test for marketplace preservation plumbing |
| `internal/marketplace/marketplace.go` | Add `validateMarketplaceSource` for Repo/URL format and Source type validation; call from `EnsureRegistered` |
| `internal/marketplace/marketplace_test.go` | Tests for marketplace source validation |
| `cmd/claude-sync/cmd_init.go` | Replace `MergeExistingConfig` call with `MergeExisting(scan, existingConfig, existingProfiles, syncDir)` |
| `cmd/claude-sync/tui/root.go` | Add `profileValues` type (MCP, Hooks, Settings, Keybindings); add `profileAddValues` field to Model; populate in `restoreProfiles()`; reset in `resetToDefaults()`; populate `ExtraMarketplaces` in `buildInitOptions()` |
| `cmd/claude-sync/tui/profilediff.go` | Update `diffsToProfile()` to prefer original profile values from `profileAddValues` for MCP, hooks, settings, keybindings |
| `cmd/claude-sync/tui/profilediff_test.go` | Value preservation tests for all four sections; secret stripping test; deselection test; profileAddValues population and reset tests; end-to-end round-trip test |
