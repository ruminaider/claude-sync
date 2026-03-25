# Design: Preserve Config-Only Items in `config update`

**Date**: 2026-03-24
**Issue**: #33

## Problem

`config update` regenerates config from current local state. If a configured
item is not detected at scan time (MCP server not running, hook not loaded,
plugin temporarily unavailable), `config update` treats the absence as an
intentional removal and drops it from the config.

### Root Cause

Two gaps cause data loss:

1. **Picker gap**: TUI pickers are built exclusively from `InitScan()` results
   (lines 129-152, `root.go`). Items in the existing config but absent from the
   scan never become picker items, so they cannot be selected.

2. **Raw value gap**: `buildInitOptions()` looks up raw values from
   `m.scanResult` (hooks at line 1163, MCP at line 1140, keybindings at line
   1175). Even if picker items were injected, raw config data must also be
   available.

## Approach

**Merge config items into scan results before TUI** (Approach A from
brainstorming).

Add a function `MergeExistingConfig(scan, cfg, syncDir)` that runs between
`InitScan()` and `NewModel()`. It finds items in the existing config that are
missing from scan results, injects them into the scan's data structures, and
records their keys in a new `scan.ConfigOnly` set. The TUI renders these with a
`[config]` tag so users can see they were preserved from previous config and can
deselect them to intentionally remove.

## Design

### 1. New field on `InitScanResult`

```go
ConfigOnly map[string]bool // keys present in existing config but not detected locally
```

### 2. New function: `MergeExistingConfig`

```go
func MergeExistingConfig(scan *InitScanResult, cfg *config.Config, syncDir string)
```

Called in `runConfigFlow()` between `InitScan()` and `NewModel()`, only for
update (edit mode).

Per-section injection:

| Section | Source | Injected into |
|---------|--------|---------------|
| Plugins (upstream) | `cfg.Upstream` | `scan.Upstream`, `scan.PluginKeys` |
| Plugins (forked) | `cfg.Forked` | `scan.AutoForked`, `scan.PluginKeys` |
| Settings | `cfg.Settings` | `scan.Settings` |
| Hooks | `cfg.Hooks` | `scan.Hooks` |
| Permissions | `cfg.Permissions` | `scan.Permissions` |
| MCP | `cfg.MCP` | `scan.MCP` |
| Keybindings | `cfg.Keybindings` | `scan.Keybindings` |
| CLAUDE.md | `cfg.ClaudeMD.Include` | `scan.ClaudeMDSections` (read from `syncDir/claude-md/`) |
| Commands/Skills | `cfg.Commands`, `cfg.Skills` | `scan.CommandsSkills.Items` (read from `syncDir/`) |

### 3. New fields on `InitOptions`

Three sections re-read from local files in `buildAndWriteConfig()`, so scan
injection alone is not sufficient. These need extra fields to carry config-only
values through:

```go
ExtraUpstream []string       // config-only upstream plugin keys to preserve
ExtraForked   []string       // config-only forked plugin names to preserve
ExtraSettings map[string]any // config-only settings key/value pairs to preserve
```

`buildInitOptions()` populates these from scan items where
`scan.ConfigOnly[key]` is true.

`buildAndWriteConfig()` merges them after local re-reads:
- Appends `ExtraUpstream` to the upstream list
- Appends `ExtraForked` to the forked names list
- Merges `ExtraSettings` into cfgSettings

### 4. TUI display

After building pickers in `NewModel()`, iterate picker items and tag
config-only items with `[config]`. These render visually distinct so users
know they were preserved from previous config, not detected locally.

For CLAUDE.md preview sections, config-only fragments get a `[config]`
indicator in the section header.

### 5. Sections that work without extra fields

These sections carry raw values through `InitOptions` directly:
- **Hooks**: `opts.IncludeHooks` is a `map[string]json.RawMessage`
- **MCP**: `opts.MCP` is a `map[string]json.RawMessage`
- **Permissions**: `opts.Permissions` carries allow/deny lists
- **Keybindings**: `opts.Keybindings` carries the whole map
- **Commands/Skills**: file content read from sync dir at apply time

### 6. Scope

Only affects `config update` (edit mode). `config create` starts fresh
and is unaffected.

## Files to change

| File | Change |
|------|--------|
| `internal/commands/init.go` | Add `ConfigOnly` field to `InitScanResult`, add `MergeExistingConfig()`, add `Extra*` fields to `InitOptions`, update `buildAndWriteConfig()` to merge extras |
| `cmd/claude-sync/cmd_init.go` | Call `MergeExistingConfig()` in `runConfigFlow()` for update mode |
| `cmd/claude-sync/tui/root.go` | Tag config-only picker items in `NewModel()`, populate `Extra*` fields in `buildInitOptions()` |
| `cmd/claude-sync/tui/picker.go` | Render `[config]` tag styling |
| Tests | Unit tests for `MergeExistingConfig`, integration test for config update preservation |
