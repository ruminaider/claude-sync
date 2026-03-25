# Config Update: Preserve Config-Only Items - Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Make `config update` preserve items from the existing config that are not currently detected locally, preventing silent data loss.

**Architecture:** Add a `MergeExistingConfig()` function that runs between `InitScan()` and `NewModel()`. It injects config-only items into the scan result with provenance tracking (`ConfigOnly` set). The TUI renders them with `[config]` tags. Three new `Extra*` fields on `InitOptions` carry config-only values through sections that re-read from local files.

**Tech Stack:** Go, bubbletea TUI, testify

---

### Task 1: Add `ConfigOnly` field to `InitScanResult`

**Files:**
- Modify: `internal/commands/init.go:24-37`
- Test: `internal/commands/init_test.go`

**Step 1: Write the test**

In `internal/commands/init_test.go`, add a test that verifies `ConfigOnly` is initialized:

```go
func TestInitScanResult_ConfigOnlyFieldExists(t *testing.T) {
	result := &InitScanResult{
		ConfigOnly: map[string]bool{"test-key": true},
	}
	assert.True(t, result.ConfigOnly["test-key"])
	assert.False(t, result.ConfigOnly["missing-key"])
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestInitScanResult_ConfigOnlyFieldExists -v`
Expected: FAIL - `ConfigOnly` field does not exist

**Step 3: Add the field**

In `internal/commands/init.go`, add to `InitScanResult` struct after `CommandsSkills`:

```go
ConfigOnly      map[string]bool            // keys present in existing config but not detected locally
```

**Step 4: Run test to verify it passes**

Run: `go test ./internal/commands/ -run TestInitScanResult_ConfigOnlyFieldExists -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/commands/init.go internal/commands/init_test.go
git commit -m "feat: add ConfigOnly field to InitScanResult for tracking config-only items"
```

---

### Task 2: Write `MergeExistingConfig` for plugins

**Files:**
- Create: `internal/commands/merge_config.go`
- Create: `internal/commands/merge_config_test.go`

**Step 1: Write the test**

In `internal/commands/merge_config_test.go`:

```go
package commands

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMergeExistingConfig_Plugins(t *testing.T) {
	scan := &InitScanResult{
		PluginKeys: []string{"detected@marketplace"},
		Upstream:   []string{"detected@marketplace"},
		Settings:   make(map[string]any),
		Hooks:      make(map[string]json.RawMessage),
	}
	cfg := &config.Config{
		Upstream: []string{"detected@marketplace", "missing@marketplace"},
	}

	MergeExistingConfig(scan, cfg, "/tmp/sync")

	assert.Contains(t, scan.PluginKeys, "missing@marketplace")
	assert.Contains(t, scan.Upstream, "missing@marketplace")
	assert.True(t, scan.ConfigOnly["missing@marketplace"])
	// Detected item should NOT be in ConfigOnly.
	assert.False(t, scan.ConfigOnly["detected@marketplace"])
}

func TestMergeExistingConfig_ForkedPlugins(t *testing.T) {
	scan := &InitScanResult{
		PluginKeys: []string{},
		Upstream:   []string{},
		AutoForked: []string{},
		Settings:   make(map[string]any),
		Hooks:      make(map[string]json.RawMessage),
	}
	cfg := &config.Config{
		Forked: []string{"my-plugin"},
	}

	MergeExistingConfig(scan, cfg, "/tmp/sync")

	// Forked plugins are stored as bare names in config but displayed as
	// name@claude-sync-forks in the picker. MergeExistingConfig should add
	// the key the picker expects.
	require.NotNil(t, scan.ConfigOnly)
	// The forked plugin key should appear in AutoForked for picker display.
	assert.Contains(t, scan.AutoForked, "my-plugin@claude-sync-forks")
	assert.Contains(t, scan.PluginKeys, "my-plugin@claude-sync-forks")
	assert.True(t, scan.ConfigOnly["my-plugin@claude-sync-forks"])
}

func TestMergeExistingConfig_NoDuplicates(t *testing.T) {
	scan := &InitScanResult{
		PluginKeys: []string{"already@marketplace"},
		Upstream:   []string{"already@marketplace"},
		Settings:   make(map[string]any),
		Hooks:      make(map[string]json.RawMessage),
	}
	cfg := &config.Config{
		Upstream: []string{"already@marketplace"},
	}

	MergeExistingConfig(scan, cfg, "/tmp/sync")

	// Should not duplicate existing items.
	count := 0
	for _, k := range scan.PluginKeys {
		if k == "already@marketplace" {
			count++
		}
	}
	assert.Equal(t, 1, count)
	assert.Empty(t, scan.ConfigOnly) // nothing was config-only
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestMergeExistingConfig -v`
Expected: FAIL - `MergeExistingConfig` undefined

**Step 3: Write minimal implementation**

Create `internal/commands/merge_config.go`:

```go
package commands

import (
	"encoding/json"
	"slices"

	"github.com/ruminaider/claude-sync/internal/config"
	forkedplugins "github.com/ruminaider/claude-sync/internal/forked-plugins"
)

// MergeExistingConfig injects items from an existing config that are not
// present in the scan results. This prevents config update from silently
// dropping items that were intentionally configured but are not currently
// detected locally (e.g., MCP server not running, plugin temporarily
// unavailable). Injected items are tracked in scan.ConfigOnly so the TUI
// can display them with a [config] tag.
func MergeExistingConfig(scan *InitScanResult, cfg *config.Config, syncDir string) {
	if scan.ConfigOnly == nil {
		scan.ConfigOnly = make(map[string]bool)
	}

	mergePlugins(scan, cfg)
	mergeSettings(scan, cfg)
	mergeHooks(scan, cfg)
	mergePermissions(scan, cfg)
	mergeMCP(scan, cfg)
	mergeKeybindings(scan, cfg)
}

func mergePlugins(scan *InitScanResult, cfg *config.Config) {
	existing := toStringSet(scan.PluginKeys)

	// Upstream plugins.
	for _, key := range cfg.Upstream {
		if !existing[key] {
			scan.Upstream = append(scan.Upstream, key)
			scan.PluginKeys = append(scan.PluginKeys, key)
			scan.ConfigOnly[key] = true
		}
	}

	// Forked plugins: stored as bare names in config, displayed as
	// name@claude-sync-forks in the picker.
	for _, name := range cfg.Forked {
		key := name + "@" + forkedplugins.MarketplaceName
		if !existing[key] {
			scan.AutoForked = append(scan.AutoForked, key)
			scan.PluginKeys = append(scan.PluginKeys, key)
			scan.ConfigOnly[key] = true
		}
	}
}

func mergeSettings(scan *InitScanResult, cfg *config.Config) {
	if scan.Settings == nil {
		scan.Settings = make(map[string]any)
	}
	for key, val := range cfg.Settings {
		if _, ok := scan.Settings[key]; !ok {
			scan.Settings[key] = val
			scan.ConfigOnly[key] = true
		}
	}
}

func mergeHooks(scan *InitScanResult, cfg *config.Config) {
	if scan.Hooks == nil {
		scan.Hooks = make(map[string]json.RawMessage)
	}
	for key, val := range cfg.Hooks {
		if _, ok := scan.Hooks[key]; !ok {
			scan.Hooks[key] = val
			scan.ConfigOnly[key] = true
		}
	}
}

func mergePermissions(scan *InitScanResult, cfg *config.Config) {
	allowSet := toStringSet(scan.Permissions.Allow)
	for _, rule := range cfg.Permissions.Allow {
		if !allowSet[rule] {
			scan.Permissions.Allow = append(scan.Permissions.Allow, rule)
			scan.ConfigOnly["allow:"+rule] = true
		}
	}

	denySet := toStringSet(scan.Permissions.Deny)
	for _, rule := range cfg.Permissions.Deny {
		if !denySet[rule] {
			scan.Permissions.Deny = append(scan.Permissions.Deny, rule)
			scan.ConfigOnly["deny:"+rule] = true
		}
	}
}

func mergeMCP(scan *InitScanResult, cfg *config.Config) {
	if scan.MCP == nil {
		scan.MCP = make(map[string]json.RawMessage)
	}
	for key, val := range cfg.MCP {
		if _, ok := scan.MCP[key]; !ok {
			scan.MCP[key] = val
			scan.ConfigOnly[key] = true
		}
	}
}

func mergeKeybindings(scan *InitScanResult, cfg *config.Config) {
	if scan.Keybindings == nil {
		scan.Keybindings = make(map[string]any)
	}
	for key, val := range cfg.Keybindings {
		if _, ok := scan.Keybindings[key]; !ok {
			scan.Keybindings[key] = val
			scan.ConfigOnly[key] = true
		}
	}
}

func toStringSet(ss []string) map[string]bool {
	m := make(map[string]bool, len(ss))
	for _, s := range ss {
		m[s] = true
	}
	return m
}
```

Note: `slices` import may not be needed yet; remove if unused.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/commands/ -run TestMergeExistingConfig -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/commands/merge_config.go internal/commands/merge_config_test.go
git commit -m "feat: add MergeExistingConfig for plugins, settings, hooks, permissions, MCP, keybindings"
```

---

### Task 3: Add MergeExistingConfig for CLAUDE.md fragments

**Files:**
- Modify: `internal/commands/merge_config.go`
- Modify: `internal/commands/merge_config_test.go`

**Step 1: Write the test**

In `merge_config_test.go`:

```go
func TestMergeExistingConfig_ClaudeMDFragments(t *testing.T) {
	// Set up a sync dir with a fragment file.
	syncDir := t.TempDir()
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	require.NoError(t, os.MkdirAll(claudeMdDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeMdDir, "coding-standards.md"),
		[]byte("## Coding Standards\n\nUse gofmt."),
		0o644,
	))

	scan := &InitScanResult{
		ClaudeMDSections: []claudemd.Section{
			{Header: "Existing Section", Content: "## Existing Section\n\nContent."},
		},
		Settings: make(map[string]any),
		Hooks:    make(map[string]json.RawMessage),
	}
	cfg := &config.Config{
		ClaudeMD: config.ClaudeMDConfig{
			Include: []string{"existing-section", "coding-standards"},
		},
	}

	MergeExistingConfig(scan, cfg, syncDir)

	// The existing section should not be duplicated.
	// The missing fragment should be injected from the sync dir.
	assert.Len(t, scan.ClaudeMDSections, 2)
	assert.Equal(t, "Coding Standards", scan.ClaudeMDSections[1].Header)
	assert.True(t, scan.ConfigOnly["fragment:coding-standards"])
}

func TestMergeExistingConfig_ClaudeMDFragments_MissingFile(t *testing.T) {
	syncDir := t.TempDir()
	// No claude-md directory, fragment file doesn't exist.

	scan := &InitScanResult{
		ClaudeMDSections: []claudemd.Section{},
		Settings:         make(map[string]any),
		Hooks:            make(map[string]json.RawMessage),
	}
	cfg := &config.Config{
		ClaudeMD: config.ClaudeMDConfig{
			Include: []string{"nonexistent-fragment"},
		},
	}

	MergeExistingConfig(scan, cfg, syncDir)

	// Fragment file missing: inject a placeholder section so the user sees
	// it in the TUI and can decide to keep or remove it.
	assert.Len(t, scan.ClaudeMDSections, 1)
	assert.True(t, scan.ConfigOnly["fragment:nonexistent-fragment"])
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestMergeExistingConfig_ClaudeMD -v`
Expected: FAIL

**Step 3: Implement**

Add to `merge_config.go`:

```go
import (
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudemd"
)
```

Add the call in `MergeExistingConfig`:

```go
mergeClaudeMDFragments(scan, cfg, syncDir)
```

Add the function:

```go
func mergeClaudeMDFragments(scan *InitScanResult, cfg *config.Config, syncDir string) {
	if len(cfg.ClaudeMD.Include) == 0 {
		return
	}

	// Build set of existing fragment keys from scan sections.
	existingKeys := make(map[string]bool)
	for _, sec := range scan.ClaudeMDSections {
		key := claudemd.HeaderToFragmentName(sec.Header)
		existingKeys[key] = true
	}

	claudeMdDir := filepath.Join(syncDir, "claude-md")

	for _, fragKey := range cfg.ClaudeMD.Include {
		if existingKeys[fragKey] {
			continue
		}

		configOnlyKey := "fragment:" + fragKey

		// Try to read fragment content from sync dir.
		content, err := claudemd.ReadFragment(claudeMdDir, fragKey)
		if err == nil && content != "" {
			sections := claudemd.Split(content)
			if len(sections) > 0 {
				scan.ClaudeMDSections = append(scan.ClaudeMDSections, sections[0])
			} else {
				scan.ClaudeMDSections = append(scan.ClaudeMDSections, claudemd.Section{
					Header:  fragKey,
					Content: content,
				})
			}
		} else {
			// Fragment file not found: inject a placeholder.
			scan.ClaudeMDSections = append(scan.ClaudeMDSections, claudemd.Section{
				Header:  fragKey,
				Content: "## " + fragKey + "\n\n(content not available locally)",
			})
		}
		scan.ConfigOnly[configOnlyKey] = true
	}
}
```

**Step 4: Run test**

Run: `go test ./internal/commands/ -run TestMergeExistingConfig_ClaudeMD -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/commands/merge_config.go internal/commands/merge_config_test.go
git commit -m "feat: add CLAUDE.md fragment merging to MergeExistingConfig"
```

---

### Task 4: Add MergeExistingConfig for Commands/Skills

**Files:**
- Modify: `internal/commands/merge_config.go`
- Modify: `internal/commands/merge_config_test.go`

**Step 1: Write the test**

```go
func TestMergeExistingConfig_CommandsSkills(t *testing.T) {
	syncDir := t.TempDir()
	// Create a command file in the sync dir.
	cmdDir := filepath.Join(syncDir, "commands")
	require.NoError(t, os.MkdirAll(cmdDir, 0o755))
	require.NoError(t, os.WriteFile(
		filepath.Join(cmdDir, "review.md"),
		[]byte("---\ndescription: Review code\n---\nReview the code."),
		0o644,
	))

	scan := &InitScanResult{
		CommandsSkills: &cmdskill.ScanResult{
			Items: []cmdskill.Item{
				{Name: "existing", Type: cmdskill.TypeCommand, Source: cmdskill.SourceGlobal},
			},
		},
		Settings: make(map[string]any),
		Hooks:    make(map[string]json.RawMessage),
	}
	cfg := &config.Config{
		Commands: []string{"cmd:global:existing", "cmd:global:review"},
		Skills:   []string{},
	}

	MergeExistingConfig(scan, cfg, syncDir)

	// Should have 2 items: original + injected from sync dir.
	assert.Len(t, scan.CommandsSkills.Items, 2)
	assert.True(t, scan.ConfigOnly["cmd:global:review"])
	assert.False(t, scan.ConfigOnly["cmd:global:existing"])
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestMergeExistingConfig_CommandsSkills -v`
Expected: FAIL

**Step 3: Implement**

Add to `MergeExistingConfig`:

```go
mergeCommandsSkills(scan, cfg, syncDir)
```

Add the function:

```go
func mergeCommandsSkills(scan *InitScanResult, cfg *config.Config, syncDir string) {
	allKeys := append(cfg.Commands, cfg.Skills...)
	if len(allKeys) == 0 {
		return
	}

	if scan.CommandsSkills == nil {
		scan.CommandsSkills = &cmdskill.ScanResult{}
	}

	existingKeys := make(map[string]bool)
	for _, item := range scan.CommandsSkills.Items {
		existingKeys[item.Key()] = true
	}

	for _, key := range allKeys {
		if existingKeys[key] {
			continue
		}

		item, err := loadCmdSkillFromSyncDir(key, syncDir)
		if err != nil {
			// File not found in sync dir: inject a placeholder.
			item = cmdskillPlaceholder(key)
		}
		scan.CommandsSkills.Items = append(scan.CommandsSkills.Items, item)
		scan.ConfigOnly[key] = true
	}
}
```

Implement `loadCmdSkillFromSyncDir` and `cmdskillPlaceholder` as helpers.
`loadCmdSkillFromSyncDir` reads from `syncDir/commands/<name>.md` or
`syncDir/skills/<name>/SKILL.md` depending on the key prefix. Parse the key
format (`cmd:global:name` or `skill:global:name`) to determine file path.

`cmdskillPlaceholder` creates an Item with the parsed name, type, and
`"(content not available locally)"` as content.

**Step 4: Run test**

Run: `go test ./internal/commands/ -run TestMergeExistingConfig_CommandsSkills -v`
Expected: PASS

**Step 5: Commit**

```bash
git add internal/commands/merge_config.go internal/commands/merge_config_test.go
git commit -m "feat: add commands/skills merging to MergeExistingConfig"
```

---

### Task 5: Add `Extra*` fields to `InitOptions` and merge in `buildAndWriteConfig`

**Files:**
- Modify: `internal/commands/init.go:58-76` (InitOptions)
- Modify: `internal/commands/init.go:370-433` (plugin handling in buildAndWriteConfig)
- Modify: `internal/commands/init.go:466-486` (settings handling in buildAndWriteConfig)
- Test: `internal/commands/init_test.go`

**Step 1: Write the test**

In `init_test.go`, add a test for `buildAndWriteConfig` preserving extra settings:

```go
func TestBuildAndWriteConfig_ExtraSettings(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	// Write a settings.json with only "theme" detected locally.
	settingsJSON := `{"theme": "dark"}`
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settingsJSON), 0o644))

	opts := InitOptions{
		ClaudeDir:       claudeDir,
		SyncDir:         syncDir,
		IncludeSettings: true,
		SettingsFilter:  []string{"theme", "model"},
		ExtraSettings:   map[string]any{"model": "opus"},
	}

	_, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	// Read config.yaml and verify both settings are present.
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(data)
	require.NoError(t, err)

	assert.Equal(t, "dark", cfg.Settings["theme"])
	assert.Equal(t, "opus", cfg.Settings["model"])
}

func TestBuildAndWriteConfig_ExtraUpstream(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	opts := InitOptions{
		ClaudeDir:      claudeDir,
		SyncDir:        syncDir,
		IncludePlugins: []string{},
		ExtraUpstream:  []string{"missing@marketplace"},
	}

	_, _, err := buildAndWriteConfig(opts)
	require.NoError(t, err)

	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(data)
	require.NoError(t, err)

	assert.Contains(t, cfg.Upstream, "missing@marketplace")
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./internal/commands/ -run TestBuildAndWriteConfig_Extra -v`
Expected: FAIL - `ExtraSettings`, `ExtraUpstream` fields do not exist

**Step 3: Add fields to InitOptions**

In `internal/commands/init.go`, add after the `Skills` field (line 75):

```go
ExtraUpstream []string       // config-only upstream plugin keys to preserve
ExtraForked   []string       // config-only forked plugin names to preserve
ExtraSettings map[string]any // config-only setting values to preserve
```

**Step 4: Merge extras in buildAndWriteConfig**

After the plugin categorization loop (after line 433), add:

```go
// Preserve config-only upstream plugins.
for _, key := range opts.ExtraUpstream {
	if !slices.Contains(upstream, key) {
		upstream = append(upstream, key)
		result.Upstream = append(result.Upstream, key)
	}
}

// Preserve config-only forked plugins.
for _, name := range opts.ExtraForked {
	if !slices.Contains(forkedNames, name) {
		forkedNames = append(forkedNames, name)
		result.AutoForked = append(result.AutoForked, name+"@"+forkedplugins.MarketplaceName)
	}
}
```

After the settings filter block (after line 486), add:

```go
// Merge config-only settings that were not detected locally.
if opts.ExtraSettings != nil {
	if cfgSettings == nil {
		cfgSettings = make(map[string]any)
	}
	for k, v := range opts.ExtraSettings {
		if _, ok := cfgSettings[k]; !ok {
			cfgSettings[k] = v
			result.IncludedSettings = append(result.IncludedSettings, k)
		}
	}
}
```

**Step 5: Run tests**

Run: `go test ./internal/commands/ -run TestBuildAndWriteConfig_Extra -v`
Expected: PASS

**Step 6: Run full test suite to check for regressions**

Run: `go test ./internal/commands/ -v`
Expected: All tests pass

**Step 7: Commit**

```bash
git add internal/commands/init.go internal/commands/init_test.go
git commit -m "feat: add Extra* fields to InitOptions and merge in buildAndWriteConfig"
```

---

### Task 6: Wire `MergeExistingConfig` into `runConfigFlow`

**Files:**
- Modify: `cmd/claude-sync/cmd_init.go:87-95`

**Step 1: Add the call**

In `cmd/claude-sync/cmd_init.go`, after line 95 (`scan, err` check), add:

```go
// In update mode, inject config-only items into scan results so the TUI
// can display them and the user can choose to keep or remove them.
if isUpdate && existingConfig != nil {
	commands.MergeExistingConfig(scan, existingConfig, syncDir)
}
```

This goes between the `InitScan()` call and the `NewModel()` call.

**Step 2: Run build to verify compilation**

Run: `go build ./cmd/claude-sync/`
Expected: Compiles without errors

**Step 3: Commit**

```bash
git add cmd/claude-sync/cmd_init.go
git commit -m "feat: wire MergeExistingConfig into config update flow"
```

---

### Task 7: Tag config-only items in TUI pickers

**Files:**
- Modify: `cmd/claude-sync/tui/root.go:106-222` (NewModel)
- Test: `cmd/claude-sync/tui/root_test.go`

**Step 1: Write the test**

In `cmd/claude-sync/tui/root_test.go`:

```go
func TestNewModel_ConfigOnlyItemsGetTag(t *testing.T) {
	scan := &commands.InitScanResult{
		PluginKeys: []string{"local@mkt", "config-only@mkt"},
		Upstream:   []string{"local@mkt", "config-only@mkt"},
		Settings:   map[string]any{"theme": "dark", "model": "opus"},
		Hooks:      map[string]json.RawMessage{"PostToolUse": json.RawMessage(`[]`)},
		MCP:        map[string]json.RawMessage{"my-server": json.RawMessage(`{}`)},
		ConfigOnly: map[string]bool{
			"config-only@mkt": true,
			"model":           true,
			"my-server":       true,
		},
	}

	m := NewModel(scan, "/tmp/claude", "/tmp/sync", "", false, SkipFlags{}, nil, nil)

	// Check plugin picker: config-only plugin should have [config] tag.
	pluginPicker := m.pickers[SectionPlugins]
	for _, item := range pluginPicker.items {
		if item.Key == "config-only@mkt" {
			assert.Equal(t, "[config]", item.Tag)
		}
		if item.Key == "local@mkt" {
			assert.Empty(t, item.Tag)
		}
	}

	// Check settings picker.
	settingsPicker := m.pickers[SectionSettings]
	for _, item := range settingsPicker.items {
		if item.Key == "model" {
			assert.Equal(t, "[config]", item.Tag)
		}
		if item.Key == "theme" {
			assert.Empty(t, item.Tag)
		}
	}
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/claude-sync/tui/ -run TestNewModel_ConfigOnlyItemsGetTag -v`
Expected: FAIL - tags not set

**Step 3: Implement tagging in NewModel**

In `cmd/claude-sync/tui/root.go`, after the pickers are built (after line 152)
and before the edit-mode pre-population (line 159), add:

```go
// Tag config-only items so they render with [config] indicator.
if scan.ConfigOnly != nil {
	for sec := range m.pickers {
		p := m.pickers[sec]
		for i, it := range p.items {
			if scan.ConfigOnly[it.Key] {
				p.items[i].Tag = "[config]"
			}
		}
		m.pickers[sec] = p
	}
}
```

**Step 4: Run test**

Run: `go test ./cmd/claude-sync/tui/ -run TestNewModel_ConfigOnlyItemsGetTag -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/claude-sync/tui/root.go cmd/claude-sync/tui/root_test.go
git commit -m "feat: tag config-only items with [config] in TUI pickers"
```

---

### Task 8: Populate `Extra*` fields in `buildInitOptions`

**Files:**
- Modify: `cmd/claude-sync/tui/root.go:1074-1194` (buildInitOptions)
- Test: `cmd/claude-sync/tui/root_test.go`

**Step 1: Write the test**

```go
func TestBuildInitOptions_PopulatesExtraFields(t *testing.T) {
	scan := &commands.InitScanResult{
		PluginKeys: []string{"local@mkt", "config-upstream@mkt", "config-forked@claude-sync-forks"},
		Upstream:   []string{"local@mkt", "config-upstream@mkt"},
		AutoForked: []string{"config-forked@claude-sync-forks"},
		Settings:   map[string]any{"theme": "dark", "model": "opus"},
		Hooks:      make(map[string]json.RawMessage),
		ConfigOnly: map[string]bool{
			"config-upstream@mkt":            true,
			"config-forked@claude-sync-forks": true,
			"model":                          true,
		},
	}

	m := NewModel(scan, "/tmp/claude", "/tmp/sync", "", false, SkipFlags{}, nil, nil)
	// All items are pre-selected by default in pickers.
	opts := m.buildInitOptions()

	assert.Contains(t, opts.ExtraUpstream, "config-upstream@mkt")
	assert.Contains(t, opts.ExtraForked, "config-forked")
	assert.Equal(t, "opus", opts.ExtraSettings["model"])

	// Local items should NOT be in Extra fields.
	assert.NotContains(t, opts.ExtraUpstream, "local@mkt")
	_, hasTheme := opts.ExtraSettings["theme"]
	assert.False(t, hasTheme)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/claude-sync/tui/ -run TestBuildInitOptions_PopulatesExtraFields -v`
Expected: FAIL

**Step 3: Implement**

In `buildInitOptions()` in `root.go`, after the plugins section (~line 1091), add:

```go
// Populate ExtraUpstream and ExtraForked for config-only plugins.
if m.scanResult.ConfigOnly != nil {
	for _, key := range pluginKeys {
		if m.scanResult.ConfigOnly[key] {
			if strings.HasSuffix(key, "@"+forkedplugins.MarketplaceName) {
				name := strings.TrimSuffix(key, "@"+forkedplugins.MarketplaceName)
				opts.ExtraForked = append(opts.ExtraForked, name)
			} else {
				opts.ExtraUpstream = append(opts.ExtraUpstream, key)
			}
		}
	}
}
```

After the settings section (~line 1103), add:

```go
// Populate ExtraSettings for config-only settings.
if m.scanResult.ConfigOnly != nil {
	for _, key := range settingsKeys {
		if m.scanResult.ConfigOnly[key] {
			if opts.ExtraSettings == nil {
				opts.ExtraSettings = make(map[string]any)
			}
			opts.ExtraSettings[key] = m.scanResult.Settings[key]
		}
	}
}
```

Need to add the `forkedplugins` import to the file.

**Step 4: Run test**

Run: `go test ./cmd/claude-sync/tui/ -run TestBuildInitOptions_PopulatesExtraFields -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/claude-sync/tui/root.go cmd/claude-sync/tui/root_test.go
git commit -m "feat: populate Extra* fields in buildInitOptions for config-only items"
```

---

### Task 9: Handle CLAUDE.md config-only fragments in TUI preview

**Files:**
- Modify: `cmd/claude-sync/tui/root.go` (NewModel, around preview setup)
- Modify: `cmd/claude-sync/tui/preview.go` (if needed for display)
- Test: `cmd/claude-sync/tui/preview_test.go`

**Step 1: Write the test**

```go
func TestPreview_ConfigOnlyFragmentsTagged(t *testing.T) {
	sections := []PreviewSection{
		{Header: "Local Section", Content: "local content", FragmentKey: "local-section", Source: "~/.claude/CLAUDE.md"},
		{Header: "Config Section", Content: "config content", FragmentKey: "config-section", Source: "~/.claude/CLAUDE.md"},
	}
	configOnly := map[string]bool{"fragment:config-section": true}

	// Apply config-only tagging to preview sections.
	tagged := tagConfigOnlyPreviewSections(sections, configOnly)

	assert.Equal(t, "Local Section", tagged[0].Header)
	assert.Equal(t, "Config Section [config]", tagged[1].Header)
}
```

**Step 2: Run test to verify it fails**

Run: `go test ./cmd/claude-sync/tui/ -run TestPreview_ConfigOnlyFragmentsTagged -v`
Expected: FAIL

**Step 3: Implement**

Add a helper function in `root.go` (near the existing helper functions):

```go
// tagConfigOnlyPreviewSections appends [config] to headers of config-only fragments.
func tagConfigOnlyPreviewSections(sections []PreviewSection, configOnly map[string]bool) []PreviewSection {
	if configOnly == nil {
		return sections
	}
	result := make([]PreviewSection, len(sections))
	copy(result, sections)
	for i, sec := range result {
		if configOnly["fragment:"+sec.FragmentKey] {
			result[i].Header = sec.Header + " [config]"
		}
	}
	return result
}
```

In `NewModel()`, after building the preview (after line 156), add:

```go
if scan.ConfigOnly != nil {
	previewSections = tagConfigOnlyPreviewSections(previewSections, scan.ConfigOnly)
	m.preview = NewPreview(previewSections)
	m.preview.searching = true
}
```

**Step 4: Run test**

Run: `go test ./cmd/claude-sync/tui/ -run TestPreview_ConfigOnlyFragmentsTagged -v`
Expected: PASS

**Step 5: Commit**

```bash
git add cmd/claude-sync/tui/root.go cmd/claude-sync/tui/preview.go cmd/claude-sync/tui/preview_test.go
git commit -m "feat: tag config-only CLAUDE.md fragments with [config] in preview"
```

---

### Task 10: Full regression test

**Files:**
- All modified files

**Step 1: Run full test suite**

Run: `go test ./... -v`
Expected: All tests pass

**Step 2: Build the binary**

Run: `go build -o claude-sync ./cmd/claude-sync/`
Expected: Compiles without errors

**Step 3: Manual smoke test**

1. Install: `cp claude-sync ~/.local/bin/claude-sync`
2. Run `claude-sync config update`
3. Verify the TUI shows items from existing config with `[config]` tags
4. Verify saving without changes preserves the existing config
5. Verify deselecting a `[config]` item removes it from config

**Step 4: Commit any fixes**

```bash
git add -A
git commit -m "fix: address any regressions from config-only item preservation"
```

---

### Task 11: Final commit and cleanup

**Step 1: Verify all tests pass**

Run: `go test ./... -count=1`
Expected: All pass

**Step 2: Check for any leftover TODOs**

Run: `grep -r "TODO" internal/commands/merge_config.go cmd/claude-sync/tui/root.go`
Expected: No unresolved TODOs

**Step 3: Final commit if needed**

If any cleanup was done, commit it.
