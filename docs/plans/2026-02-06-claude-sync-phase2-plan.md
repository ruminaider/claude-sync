# claude-sync Phase 2 (Plugin Intelligence) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add plugin categorization (upstream/pinned/forked), marketplace version checking, forked plugin storage as a local marketplace, interactive v1→v2 config migration, and five new CLI commands (update, fork, unfork, pin, unpin) plus plugin slash commands.

**Architecture:** Extends the existing Go CLI. Config v2 introduces categorized plugins (upstream/pinned/forked) replacing the flat list. A new `internal/marketplace` package queries marketplace git repos for version info. Forked plugins are stored in `~/.claude-sync/plugins/` and exposed as a local marketplace entry in `known_marketplaces.json`. The `pull` and `status` commands gain version awareness and categorized output. A `/sync` slash command file provides in-session access.

**Tech Stack:** Go 1.22+, Cobra v1.9+, go.yaml.in/yaml/v3, charmbracelet/huh v0.8.0, stretchr/testify v1.11+, os/exec for git. No new dependencies.

---

## Task 1: Config V2 — Categorized Plugin Schema

**Files:**
- Modify: `internal/config/config.go`
- Modify: `internal/config/config_test.go`

This task introduces the v2 config schema with categorized plugins while maintaining backward compatibility with v1 during parsing (v1 auto-detects and treats all plugins as upstream).

**Step 1: Write failing tests for v2 parsing**

Add to `internal/config/config_test.go`:

```go
func TestParseConfigV2(t *testing.T) {
	input := `
version: "2.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
    - playwright@claude-plugins-official
  pinned:
    - beads@beads-marketplace: "0.44.0"
  forked:
    - compound-engineering-django-ts
    - figma-minimal
settings:
  model: opus
hooks:
  PreCompact: "bd prime"
`
	cfg, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", cfg.Version)
	assert.Equal(t, []string{"context7@claude-plugins-official", "playwright@claude-plugins-official"}, cfg.Upstream)
	assert.Equal(t, map[string]string{"beads@beads-marketplace": "0.44.0"}, cfg.Pinned)
	assert.Equal(t, []string{"compound-engineering-django-ts", "figma-minimal"}, cfg.Forked)
}

func TestParseConfigV1Compat(t *testing.T) {
	input := `
version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
`
	cfg, err := Parse(input)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", cfg.Version)
	// V1 flat list → all treated as upstream
	assert.Equal(t, []string{"context7@claude-plugins-official", "beads@beads-marketplace"}, cfg.Upstream)
	assert.Empty(t, cfg.Pinned)
	assert.Empty(t, cfg.Forked)
}

func TestMarshalConfigV2(t *testing.T) {
	cfg := ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{"beads@beads-marketplace": "0.44.0"},
		Forked:   []string{"my-fork"},
		Settings: map[string]any{"model": "opus"},
		Hooks:    map[string]string{"PreCompact": "bd prime"},
	}
	data, err := MarshalV2(cfg)
	require.NoError(t, err)
	assert.Contains(t, string(data), "upstream:")
	assert.Contains(t, string(data), "pinned:")
	assert.Contains(t, string(data), "forked:")

	// Round-trip
	parsed, err := Parse(data)
	require.NoError(t, err)
	assert.Equal(t, cfg.Upstream, parsed.Upstream)
	assert.Equal(t, cfg.Pinned, parsed.Pinned)
	assert.Equal(t, cfg.Forked, parsed.Forked)
}

func TestAllPluginKeys(t *testing.T) {
	cfg := ConfigV2{
		Upstream: []string{"a@m1", "b@m2"},
		Pinned:   map[string]string{"c@m3": "1.0"},
		Forked:   []string{"d"},
	}
	keys := cfg.AllPluginKeys()
	assert.ElementsMatch(t, []string{"a@m1", "b@m2", "c@m3", "d"}, keys)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -v -run "TestParseConfigV2|TestParseConfigV1Compat|TestMarshalConfigV2|TestAllPluginKeys"`
Expected: FAIL — types and functions don't exist yet

**Step 3: Implement ConfigV2 and dual-format parsing**

Replace `internal/config/config.go` with:

```go
package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// ConfigV2 represents ~/.claude-sync/config.yaml (Phase 2 categorized format).
type ConfigV2 struct {
	Version  string            `yaml:"version"`
	Upstream []string          `yaml:"-"` // parsed from plugins.upstream
	Pinned   map[string]string `yaml:"-"` // parsed from plugins.pinned (key: plugin, value: version)
	Forked   []string          `yaml:"-"` // parsed from plugins.forked
	Settings map[string]any    `yaml:"settings,omitempty"`
	Hooks    map[string]string `yaml:"hooks,omitempty"`
}

// AllPluginKeys returns all plugin keys across all categories.
func (c *ConfigV2) AllPluginKeys() []string {
	var keys []string
	keys = append(keys, c.Upstream...)
	for k := range c.Pinned {
		keys = append(keys, k)
	}
	keys = append(keys, c.Forked...)
	return keys
}

// Config is a type alias for backward compatibility.
// Phase 1 code that used Config can still work.
type Config = ConfigV2

// UserPreferences represents ~/.claude-sync/user-preferences.yaml.
type UserPreferences struct {
	SyncMode string            `yaml:"sync_mode"`
	Settings map[string]any    `yaml:"settings,omitempty"`
	Plugins  UserPluginPrefs   `yaml:"plugins,omitempty"`
	Pins     map[string]string `yaml:"pins,omitempty"`
}

// UserPluginPrefs holds plugin override preferences.
type UserPluginPrefs struct {
	Unsubscribe []string `yaml:"unsubscribe,omitempty"`
	Personal    []string `yaml:"personal,omitempty"`
}

// rawConfig is used for initial YAML parsing before determining version.
type rawConfig struct {
	Version  string         `yaml:"version"`
	Plugins  yaml.Node      `yaml:"plugins"`
	Settings map[string]any `yaml:"settings,omitempty"`
	Hooks    map[string]string `yaml:"hooks,omitempty"`
}

// v2Plugins is the categorized plugins structure.
type v2Plugins struct {
	Upstream []string               `yaml:"upstream,omitempty"`
	Pinned   yaml.Node              `yaml:"pinned,omitempty"`
	Forked   []string               `yaml:"forked,omitempty"`
}

// Parse parses config.yaml bytes, auto-detecting v1 (flat list) vs v2 (categorized).
func Parse(data []byte) (ConfigV2, error) {
	var raw rawConfig
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return ConfigV2{}, fmt.Errorf("parsing config: %w", err)
	}

	cfg := ConfigV2{
		Version:  raw.Version,
		Settings: raw.Settings,
		Hooks:    raw.Hooks,
		Pinned:   make(map[string]string),
	}

	if raw.Plugins.Kind == 0 {
		// No plugins section
		return cfg, nil
	}

	// Try v1 format first: plugins is a sequence (flat list)
	if raw.Plugins.Kind == yaml.SequenceNode {
		var flat []string
		if err := raw.Plugins.Decode(&flat); err != nil {
			return ConfigV2{}, fmt.Errorf("parsing v1 plugins: %w", err)
		}
		cfg.Upstream = flat
		return cfg, nil
	}

	// V2 format: plugins is a mapping with upstream/pinned/forked keys
	if raw.Plugins.Kind == yaml.MappingNode {
		var v2 v2Plugins
		if err := raw.Plugins.Decode(&v2); err != nil {
			return ConfigV2{}, fmt.Errorf("parsing v2 plugins: %w", err)
		}
		cfg.Upstream = v2.Upstream
		cfg.Forked = v2.Forked

		// Parse pinned: each entry is a mapping of plugin-key → version string
		if v2.Pinned.Kind == yaml.SequenceNode {
			for _, item := range v2.Pinned.Content {
				if item.Kind == yaml.MappingNode && len(item.Content) == 2 {
					cfg.Pinned[item.Content[0].Value] = item.Content[1].Value
				}
			}
		}

		return cfg, nil
	}

	return ConfigV2{}, fmt.Errorf("unexpected plugins format")
}

// v2MarshalHelper is the struct used to marshal v2 config.
type v2MarshalHelper struct {
	Version  string            `yaml:"version"`
	Plugins  v2PluginsMarshal  `yaml:"plugins"`
	Settings map[string]any    `yaml:"settings,omitempty"`
	Hooks    map[string]string `yaml:"hooks,omitempty"`
}

type v2PluginsMarshal struct {
	Upstream []string               `yaml:"upstream,omitempty"`
	Pinned   []map[string]string    `yaml:"pinned,omitempty"`
	Forked   []string               `yaml:"forked,omitempty"`
}

// MarshalV2 serializes a ConfigV2 to YAML bytes in v2 format.
func MarshalV2(cfg ConfigV2) ([]byte, error) {
	var pinnedList []map[string]string
	for k, v := range cfg.Pinned {
		pinnedList = append(pinnedList, map[string]string{k: v})
	}

	helper := v2MarshalHelper{
		Version: cfg.Version,
		Plugins: v2PluginsMarshal{
			Upstream: cfg.Upstream,
			Pinned:   pinnedList,
			Forked:   cfg.Forked,
		},
		Settings: cfg.Settings,
		Hooks:    cfg.Hooks,
	}
	return yaml.Marshal(helper)
}

// Marshal serializes a Config. If version is 2.x, uses v2 format. Otherwise v1 flat list.
func Marshal(cfg ConfigV2) ([]byte, error) {
	if cfg.Version >= "2" {
		return MarshalV2(cfg)
	}
	// V1 compat: flat list from Upstream (legacy Plugins field)
	type v1Config struct {
		Version  string            `yaml:"version"`
		Plugins  []string          `yaml:"plugins"`
		Settings map[string]any    `yaml:"settings,omitempty"`
		Hooks    map[string]string `yaml:"hooks,omitempty"`
	}
	return yaml.Marshal(v1Config{
		Version:  cfg.Version,
		Plugins:  cfg.Upstream,
		Settings: cfg.Settings,
		Hooks:    cfg.Hooks,
	})
}

// ParseUserPreferences parses user-preferences.yaml.
func ParseUserPreferences(data []byte) (UserPreferences, error) {
	var prefs UserPreferences
	if err := yaml.Unmarshal(data, &prefs); err != nil {
		return UserPreferences{}, fmt.Errorf("parsing user preferences: %w", err)
	}
	if prefs.SyncMode == "" {
		prefs.SyncMode = "union"
	}
	return prefs, nil
}

// DefaultUserPreferences returns preferences with default values.
func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		SyncMode: "union",
		Pins:     map[string]string{},
	}
}
```

**Step 4: Run tests**

Run: `go test ./internal/config/ -v`
Expected: All tests PASS (both old v1 tests and new v2 tests)

**Step 5: Fix callers — update references from `cfg.Plugins` to `cfg.Upstream` / `cfg.AllPluginKeys()`**

The `Config = ConfigV2` alias means all callers still compile, but any code reading `cfg.Plugins` needs updating since that field no longer exists as a simple `[]string`. Update all callers in:

- `internal/commands/init.go` — writes config: set `cfg.Upstream = pluginKeys` instead of `cfg.Plugins`
- `internal/commands/status.go` — reads config: use `cfg.AllPluginKeys()`
- `internal/commands/pull.go` — reads config: use `cfg.AllPluginKeys()`
- `internal/commands/push.go` — reads/writes config: use `cfg.Upstream` for the plugin set (push adds to upstream by default)

**Step 6: Run full test suite**

Run: `go test ./... -v`
Expected: ALL tests PASS

**Step 7: Commit**

```bash
git add internal/config/ internal/commands/
git commit -m "feat: add config v2 with categorized plugins (upstream/pinned/forked)"
```

---

## Task 2: Interactive V1 → V2 Migration

**Files:**
- Create: `internal/commands/migrate.go`
- Create: `internal/commands/migrate_test.go`
- Modify: `cmd/claude-sync/main.go` (add migrate subcommand)
- Create: `cmd/claude-sync/cmd_migrate.go`

This task implements the interactive migration that prompts users to categorize each plugin when upgrading from v1 to v2 config format.

**Step 1: Write failing tests**

Create `internal/commands/migrate_test.go`:

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMigrateDetectsV1(t *testing.T) {
	syncDir := t.TempDir()
	v1Config := `version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
settings:
  model: opus
`
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(v1Config), 0644)

	needed, err := MigrateNeeded(syncDir)
	require.NoError(t, err)
	assert.True(t, needed)
}

func TestMigrateNotNeededForV2(t *testing.T) {
	syncDir := t.TempDir()
	v2Config := `version: "2.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
`
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(v2Config), 0644)

	needed, err := MigrateNeeded(syncDir)
	require.NoError(t, err)
	assert.False(t, needed)
}

func TestMigrateApply(t *testing.T) {
	syncDir := setupMigrateTestEnv(t)

	// Simulate user choosing: all upstream except beads (pinned)
	categories := map[string]string{
		"context7@claude-plugins-official":     "upstream",
		"beads@beads-marketplace":              "pinned",
		"my-custom@local":                      "forked",
	}
	versions := map[string]string{
		"beads@beads-marketplace": "0.44.0",
	}

	err := MigrateApply(syncDir, categories, versions)
	require.NoError(t, err)

	// Verify new config
	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", cfg.Version)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.Equal(t, "0.44.0", cfg.Pinned["beads@beads-marketplace"])
	assert.Contains(t, cfg.Forked, "my-custom@local")
}

func setupMigrateTestEnv(t *testing.T) string {
	t.Helper()
	syncDir := t.TempDir()
	v1Config := `version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
  - my-custom@local
settings:
  model: opus
`
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(v1Config), 0644)

	// Init as git repo so commit works
	exec.Command("git", "init", syncDir).Run()
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run()
	return syncDir
}
```

Note: Add needed imports (`os/exec`, `github.com/ruminaider/claude-sync/internal/config`).

**Step 2: Run tests to verify failure**

Run: `go test ./internal/commands/ -v -run TestMigrate`
Expected: FAIL

**Step 3: Implement migration logic**

Create `internal/commands/migrate.go`:

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// MigrateNeeded checks if the config is v1 and needs migration to v2.
func MigrateNeeded(syncDir string) (bool, error) {
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return false, err
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return false, err
	}
	return cfg.Version < "2", nil
}

// MigratePlugins returns the list of plugins from a v1 config for categorization.
func MigratePlugins(syncDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return nil, err
	}
	return cfg.Upstream, nil // v1 puts all in Upstream
}

// MigrateApply writes a v2 config based on user categorization choices.
func MigrateApply(syncDir string, categories map[string]string, versions map[string]string) error {
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return err
	}
	oldCfg, err := config.Parse(data)
	if err != nil {
		return err
	}

	newCfg := config.ConfigV2{
		Version:  "2.0.0",
		Pinned:   make(map[string]string),
		Settings: oldCfg.Settings,
		Hooks:    oldCfg.Hooks,
	}

	for _, plugin := range oldCfg.Upstream {
		cat, ok := categories[plugin]
		if !ok {
			cat = "upstream" // default
		}
		switch cat {
		case "pinned":
			ver := versions[plugin]
			if ver == "" {
				ver = "latest"
			}
			newCfg.Pinned[plugin] = ver
		case "forked":
			newCfg.Forked = append(newCfg.Forked, plugin)
		default:
			newCfg.Upstream = append(newCfg.Upstream, plugin)
		}
	}

	newData, err := config.MarshalV2(newCfg)
	if err != nil {
		return err
	}

	cfgPath := filepath.Join(syncDir, "config.yaml")
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	if err := git.Add(syncDir, "config.yaml"); err != nil {
		return err
	}
	return git.Commit(syncDir, "Migrate config to v2 (categorized plugins)")
}
```

**Step 4: Wire up CLI command**

Create `cmd/claude-sync/cmd_migrate.go`:

```go
package main

import (
	"fmt"

	"github.com/charmbracelet/huh"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var migrateCmd = &cobra.Command{
	Use:   "migrate",
	Short: "Migrate config.yaml from v1 to v2 (categorized plugins)",
	RunE: func(cmd *cobra.Command, args []string) error {
		syncDir := paths.SyncDir()

		needed, err := commands.MigrateNeeded(syncDir)
		if err != nil {
			return err
		}
		if !needed {
			fmt.Println("Config is already v2. No migration needed.")
			return nil
		}

		plugins, err := commands.MigratePlugins(syncDir)
		if err != nil {
			return err
		}

		fmt.Println("Migrating config.yaml from v1 → v2 (categorized plugins)")
		fmt.Println()

		categories := make(map[string]string)
		versions := make(map[string]string)

		for _, plugin := range plugins {
			var choice string
			err := huh.NewForm(
				huh.NewGroup(
					huh.NewSelect[string]().
						Title(fmt.Sprintf("Category for %s:", plugin)).
						Options(
							huh.NewOption("upstream (auto-update from marketplace)", "upstream"),
							huh.NewOption("pinned (lock to specific version)", "pinned"),
							huh.NewOption("forked (synced from your repo)", "forked"),
						).
						Value(&choice),
				),
			).Run()
			if err != nil {
				return err
			}
			categories[plugin] = choice

			if choice == "pinned" {
				var ver string
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewInput().
							Title(fmt.Sprintf("Pin version for %s:", plugin)).
							Placeholder("e.g., 0.44.0 or latest").
							Value(&ver),
					),
				).Run()
				if err != nil {
					return err
				}
				if ver == "" {
					ver = "latest"
				}
				versions[plugin] = ver
			}
		}

		if err := commands.MigrateApply(syncDir, categories, versions); err != nil {
			return err
		}

		fmt.Println("\n✓ Config migrated to v2 successfully.")
		fmt.Println("  Review: cat ~/.claude-sync/config.yaml")
		return nil
	},
}
```

Add to `cmd/claude-sync/main.go` init(): `rootCmd.AddCommand(migrateCmd)`

**Step 5: Run tests**

Run: `go test ./... -v`
Expected: ALL tests PASS

**Step 6: Commit**

```bash
git add internal/commands/migrate.go internal/commands/migrate_test.go cmd/claude-sync/cmd_migrate.go cmd/claude-sync/main.go
git commit -m "feat: add interactive v1 → v2 config migration"
```

---

## Task 3: Forked Plugin Storage and Local Marketplace Registration

**Files:**
- Create: `internal/plugins/forked.go`
- Create: `internal/plugins/forked_test.go`

This task implements the core forked plugin mechanics: storing plugin files in `~/.claude-sync/plugins/`, registering as a local marketplace in `known_marketplaces.json`, and detecting forked plugins.

**Step 1: Write failing tests**

Create `internal/plugins/forked_test.go`:

```go
package plugins

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterLocalMarketplace(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	err := RegisterLocalMarketplace(claudeDir, syncDir)
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(pluginDir, "known_marketplaces.json"))
	var mkts map[string]json.RawMessage
	json.Unmarshal(data, &mkts)
	assert.Contains(t, mkts, "claude-sync-forks")
}

func TestRegisterLocalMarketplace_PreservesExisting(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"),
		[]byte(`{"other-marketplace": {"source": {}}}`), 0644)

	err := RegisterLocalMarketplace(claudeDir, syncDir)
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(pluginDir, "known_marketplaces.json"))
	var mkts map[string]json.RawMessage
	json.Unmarshal(data, &mkts)
	assert.Contains(t, mkts, "claude-sync-forks")
	assert.Contains(t, mkts, "other-marketplace")
}

func TestListForkedPlugins(t *testing.T) {
	syncDir := t.TempDir()
	pluginsDir := filepath.Join(syncDir, "plugins")

	// Create two forked plugins with plugin.json
	for _, name := range []string{"my-fork", "another-fork"} {
		dir := filepath.Join(pluginsDir, name, ".claude-plugin")
		os.MkdirAll(dir, 0755)
		os.WriteFile(filepath.Join(dir, "plugin.json"), []byte(`{"name":"`+name+`"}`), 0644)
	}

	forks, err := ListForkedPlugins(syncDir)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"my-fork", "another-fork"}, forks)
}

func TestListForkedPlugins_EmptyDir(t *testing.T) {
	syncDir := t.TempDir()
	forks, err := ListForkedPlugins(syncDir)
	require.NoError(t, err)
	assert.Empty(t, forks)
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./internal/plugins/ -v`
Expected: FAIL

**Step 3: Implement forked plugin functions**

Create `internal/plugins/forked.go`:

```go
package plugins

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ruminaider/claude-sync/internal/claudecode"
)

const MarketplaceName = "claude-sync-forks"

// RegisterLocalMarketplace adds the claude-sync-forks entry to known_marketplaces.json.
func RegisterLocalMarketplace(claudeDir, syncDir string) error {
	mkts, err := claudecode.ReadMarketplaces(claudeDir)
	if err != nil {
		return fmt.Errorf("reading marketplaces: %w", err)
	}

	pluginsPath := filepath.Join(syncDir, "plugins")
	installPath := filepath.Join(claudeDir, "plugins", "marketplaces", MarketplaceName)

	entry := map[string]any{
		"source": map[string]string{
			"source": "directory",
			"path":   pluginsPath,
		},
		"installLocation": installPath,
		"lastUpdated":     time.Now().UTC().Format(time.RFC3339),
	}

	data, err := json.Marshal(entry)
	if err != nil {
		return err
	}
	mkts[MarketplaceName] = json.RawMessage(data)

	return claudecode.WriteMarketplaces(claudeDir, mkts)
}

// ListForkedPlugins returns names of forked plugins in ~/.claude-sync/plugins/.
// A valid forked plugin has a .claude-plugin/plugin.json file.
func ListForkedPlugins(syncDir string) ([]string, error) {
	pluginsDir := filepath.Join(syncDir, "plugins")
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var forks []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginsDir, entry.Name(), ".claude-plugin", "plugin.json")
		if _, err := os.Stat(manifestPath); err == nil {
			forks = append(forks, entry.Name())
		}
	}
	return forks, nil
}

// ForkedPluginKey returns the plugin key for a forked plugin (name@claude-sync-forks).
func ForkedPluginKey(name string) string {
	return name + "@" + MarketplaceName
}
```

**Step 4: Run tests**

Run: `go test ./internal/plugins/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/plugins/
git commit -m "feat: add forked plugin storage and local marketplace registration"
```

---

## Task 4: Marketplace Version Checking

**Files:**
- Create: `internal/marketplace/marketplace.go`
- Create: `internal/marketplace/marketplace_test.go`

This task implements the marketplace client that queries git repos for version information. It uses `git ls-remote` to check tags/versions without cloning.

**Step 1: Write failing tests**

Create `internal/marketplace/marketplace_test.go`:

```go
package marketplace

import (
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseMarketplaceSource(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOrg  string
		wantRepo string
	}{
		{"standard", "claude-plugins-official", "anthropics", "claude-plugins-official"},
		{"custom-org", "myorg/my-marketplace", "myorg", "my-marketplace"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			org, repo := ParseMarketplaceSource(tt.input)
			assert.Equal(t, tt.wantOrg, org)
			assert.Equal(t, tt.wantRepo, repo)
		})
	}
}

func TestQueryPluginVersion_LocalRepo(t *testing.T) {
	// Create a local "marketplace" git repo with a tagged plugin
	repoDir := t.TempDir()
	pluginDir := filepath.Join(repoDir, "my-plugin", ".claude-plugin")
	os.MkdirAll(pluginDir, 0755)
	os.WriteFile(filepath.Join(pluginDir, "plugin.json"),
		[]byte(`{"name":"my-plugin","version":"1.2.3"}`), 0644)

	exec.Command("git", "init", repoDir).Run()
	exec.Command("git", "-C", repoDir, "add", ".").Run()
	exec.Command("git", "-C", repoDir, "commit", "-m", "init").Run()

	sha, _ := exec.Command("git", "-C", repoDir, "rev-parse", "HEAD").Output()

	info, err := QueryPluginVersion(repoDir, "my-plugin")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", info.Version)
	assert.Equal(t, strings.TrimSpace(string(sha)), info.CommitSHA)
}

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name     string
		current  string
		latest   string
		expected bool // true = update available
	}{
		{"same version", "1.0.0", "1.0.0", false},
		{"patch update", "1.0.0", "1.0.1", true},
		{"minor update", "1.0.0", "1.1.0", true},
		{"major update", "1.0.0", "2.0.0", true},
		{"sha different", "abc123", "def456", true},
		{"sha same", "abc123", "abc123", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, HasUpdate(tt.current, tt.latest))
		})
	}
}
```

Note: Add needed imports (`os`, `strings`).

**Step 2: Run tests to verify failure**

Run: `go test ./internal/marketplace/ -v`
Expected: FAIL

**Step 3: Implement marketplace client**

Create `internal/marketplace/marketplace.go`:

```go
package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PluginVersionInfo holds version information for a plugin.
type PluginVersionInfo struct {
	Name      string
	Version   string // semantic version or "unknown"
	CommitSHA string // git commit SHA
}

// ParseMarketplaceSource extracts org/repo from a marketplace identifier.
// "claude-plugins-official" → ("anthropics", "claude-plugins-official")
// "myorg/my-marketplace" → ("myorg", "my-marketplace")
func ParseMarketplaceSource(marketplace string) (org, repo string) {
	if strings.Contains(marketplace, "/") {
		parts := strings.SplitN(marketplace, "/", 2)
		return parts[0], parts[1]
	}
	// Default known marketplaces
	knownOrgs := map[string]string{
		"claude-plugins-official": "anthropics",
		"superpowers-marketplace": "anthropics",
	}
	if org, ok := knownOrgs[marketplace]; ok {
		return org, marketplace
	}
	return marketplace, marketplace
}

// QueryPluginVersion queries a marketplace repo for a plugin's latest version.
// repoPath can be a local path or a remote URL.
func QueryPluginVersion(repoPath, pluginName string) (*PluginVersionInfo, error) {
	// Read plugin.json from the plugin subdirectory
	manifestPath := filepath.Join(repoPath, pluginName, ".claude-plugin", "plugin.json")
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		return nil, fmt.Errorf("reading plugin manifest: %w", err)
	}

	var manifest struct {
		Name    string `json:"name"`
		Version string `json:"version"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		return nil, fmt.Errorf("parsing manifest: %w", err)
	}

	// Get commit SHA for the plugin directory
	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--format=%H", "--", pluginName)
	sha, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("getting commit SHA: %w", err)
	}

	return &PluginVersionInfo{
		Name:      manifest.Name,
		Version:   manifest.Version,
		CommitSHA: strings.TrimSpace(string(sha)),
	}, nil
}

// HasUpdate returns true if latest differs from current.
// Handles both semantic versions and SHA-based versions.
func HasUpdate(current, latest string) bool {
	return current != latest
}

// QueryRemoteVersion uses git ls-remote to check the latest commit for a plugin
// in a remote marketplace repo without cloning it.
func QueryRemoteVersion(marketplaceURL, pluginName string) (string, error) {
	cmd := exec.Command("git", "ls-remote", marketplaceURL, "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("querying remote: %w", err)
	}
	fields := strings.Fields(string(out))
	if len(fields) < 1 {
		return "", fmt.Errorf("unexpected ls-remote output")
	}
	return fields[0], nil
}
```

**Step 4: Run tests**

Run: `go test ./internal/marketplace/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/marketplace/
git commit -m "feat: add marketplace version checking via git queries"
```

---

## Task 5: Pin and Unpin Commands

**Files:**
- Create: `internal/commands/pin.go`
- Create: `internal/commands/pin_test.go`
- Create: `cmd/claude-sync/cmd_pin.go`
- Create: `cmd/claude-sync/cmd_unpin.go`
- Modify: `cmd/claude-sync/main.go`

**Step 1: Write failing tests**

Create `internal/commands/pin_test.go`:

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupV2TestEnv(t *testing.T) string {
	t.Helper()
	syncDir := t.TempDir()

	cfg := config.ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official", "beads@beads-marketplace"},
		Pinned:   map[string]string{},
		Settings: map[string]any{"model": "opus"},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	// Init git repo
	exec.Command("git", "init", syncDir).Run()
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run()
	return syncDir
}

func TestPin(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	err := Pin(syncDir, "beads@beads-marketplace", "0.44.0")
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(data)
	assert.NotContains(t, cfg.Upstream, "beads@beads-marketplace")
	assert.Equal(t, "0.44.0", cfg.Pinned["beads@beads-marketplace"])
}

func TestPin_AlreadyPinned(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	// Pin once
	Pin(syncDir, "beads@beads-marketplace", "0.44.0")

	// Pin to different version
	err := Pin(syncDir, "beads@beads-marketplace", "0.45.0")
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(data)
	assert.Equal(t, "0.45.0", cfg.Pinned["beads@beads-marketplace"])
}

func TestUnpin(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	// Pin first
	Pin(syncDir, "beads@beads-marketplace", "0.44.0")

	// Unpin
	err := Unpin(syncDir, "beads@beads-marketplace")
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(data)
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace")
	assert.Empty(t, cfg.Pinned)
}

func TestPin_NotInConfig(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	err := Pin(syncDir, "nonexistent@marketplace", "1.0")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
```

Note: Add `os/exec` import.

**Step 2: Run tests to verify failure**

Run: `go test ./internal/commands/ -v -run TestPin`
Expected: FAIL

**Step 3: Implement Pin and Unpin**

Create `internal/commands/pin.go`:

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// Pin moves a plugin from upstream to pinned with a specific version.
func Pin(syncDir, pluginKey, version string) error {
	cfgPath := filepath.Join(syncDir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return err
	}

	// Remove from upstream if present
	found := false
	var newUpstream []string
	for _, p := range cfg.Upstream {
		if p == pluginKey {
			found = true
		} else {
			newUpstream = append(newUpstream, p)
		}
	}

	// Check if already pinned (updating version)
	if _, ok := cfg.Pinned[pluginKey]; ok {
		found = true
	}

	if !found {
		return fmt.Errorf("plugin %s not found in config", pluginKey)
	}

	cfg.Upstream = newUpstream
	if cfg.Pinned == nil {
		cfg.Pinned = make(map[string]string)
	}
	cfg.Pinned[pluginKey] = version
	cfg.Version = "2.0.0"

	newData, err := config.MarshalV2(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	git.Add(syncDir, "config.yaml")
	return git.Commit(syncDir, fmt.Sprintf("Pin %s to %s", pluginKey, version))
}

// Unpin moves a plugin from pinned back to upstream.
func Unpin(syncDir, pluginKey string) error {
	cfgPath := filepath.Join(syncDir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return err
	}

	if _, ok := cfg.Pinned[pluginKey]; !ok {
		return fmt.Errorf("plugin %s is not pinned", pluginKey)
	}

	delete(cfg.Pinned, pluginKey)
	cfg.Upstream = append(cfg.Upstream, pluginKey)
	cfg.Version = "2.0.0"

	newData, err := config.MarshalV2(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	git.Add(syncDir, "config.yaml")
	return git.Commit(syncDir, fmt.Sprintf("Unpin %s", pluginKey))
}
```

**Step 4: Wire up CLI commands**

Create `cmd/claude-sync/cmd_pin.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var pinCmd = &cobra.Command{
	Use:   "pin <plugin> [version]",
	Short: "Pin a plugin to a specific version",
	Args:  cobra.RangeArgs(1, 2),
	RunE: func(cmd *cobra.Command, args []string) error {
		plugin := args[0]
		version := "latest"
		if len(args) > 1 {
			version = args[1]
		}
		if err := commands.Pin(paths.SyncDir(), plugin, version); err != nil {
			return err
		}
		fmt.Printf("Pinned %s to %s\n", plugin, version)
		return nil
	},
}
```

Create `cmd/claude-sync/cmd_unpin.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var unpinCmd = &cobra.Command{
	Use:   "unpin <plugin>",
	Short: "Unpin a plugin (enable auto-updates)",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := commands.Unpin(paths.SyncDir(), args[0]); err != nil {
			return err
		}
		fmt.Printf("Unpinned %s — will auto-update from marketplace\n", args[0])
		return nil
	},
}
```

Add to `main.go` init(): `rootCmd.AddCommand(pinCmd)` and `rootCmd.AddCommand(unpinCmd)`

**Step 5: Run tests**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/commands/pin.go internal/commands/pin_test.go cmd/claude-sync/cmd_pin.go cmd/claude-sync/cmd_unpin.go cmd/claude-sync/main.go
git commit -m "feat: add pin and unpin commands for version-locking plugins"
```

---

## Task 6: Fork and Unfork Commands

**Files:**
- Create: `internal/commands/fork.go`
- Create: `internal/commands/fork_test.go`
- Create: `cmd/claude-sync/cmd_fork.go`
- Create: `cmd/claude-sync/cmd_unfork.go`
- Modify: `cmd/claude-sync/main.go`

Fork copies a plugin's files from the Claude Code cache into `~/.claude-sync/plugins/` and moves it from upstream/pinned to forked. Unfork removes the fork and moves back to upstream.

**Step 1: Write failing tests**

Create `internal/commands/fork_test.go`:

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupForkTestEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = t.TempDir()

	// Create Claude dir with a cached plugin
	pluginCacheDir := filepath.Join(claudeDir, "plugins", "marketplaces", "my-marketplace", "my-plugin")
	os.MkdirAll(filepath.Join(pluginCacheDir, ".claude-plugin"), 0755)
	os.WriteFile(filepath.Join(pluginCacheDir, ".claude-plugin", "plugin.json"),
		[]byte(`{"name":"my-plugin","version":"1.0.0"}`), 0644)
	os.WriteFile(filepath.Join(pluginCacheDir, "README.md"), []byte("# My Plugin"), 0644)

	// Create installed_plugins.json
	os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{"my-plugin@my-marketplace":[{"scope":"user","installPath":"`+pluginCacheDir+`","version":"1.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]}}`), 0644)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"), []byte("{}"), 0644)

	// Create v2 config
	cfg := config.ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"my-plugin@my-marketplace"},
		Pinned:   map[string]string{},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	exec.Command("git", "init", syncDir).Run()
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run()
	return
}

func TestFork(t *testing.T) {
	claudeDir, syncDir := setupForkTestEnv(t)

	err := Fork(claudeDir, syncDir, "my-plugin@my-marketplace")
	require.NoError(t, err)

	// Verify plugin files copied to sync dir
	forkDir := filepath.Join(syncDir, "plugins", "my-plugin")
	assert.DirExists(t, forkDir)
	assert.FileExists(t, filepath.Join(forkDir, ".claude-plugin", "plugin.json"))
	assert.FileExists(t, filepath.Join(forkDir, "README.md"))

	// Verify config updated
	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(data)
	assert.NotContains(t, cfg.Upstream, "my-plugin@my-marketplace")
	assert.Contains(t, cfg.Forked, "my-plugin")
}

func TestUnfork(t *testing.T) {
	claudeDir, syncDir := setupForkTestEnv(t)

	// Fork first
	Fork(claudeDir, syncDir, "my-plugin@my-marketplace")

	// Unfork
	err := Unfork(syncDir, "my-plugin", "my-marketplace")
	require.NoError(t, err)

	// Verify fork directory removed
	forkDir := filepath.Join(syncDir, "plugins", "my-plugin")
	assert.NoDirExists(t, forkDir)

	// Verify config updated
	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(data)
	assert.Contains(t, cfg.Upstream, "my-plugin@my-marketplace")
	assert.NotContains(t, cfg.Forked, "my-plugin")
}
```

Note: Add `os/exec` import.

**Step 2: Run tests to verify failure**

Run: `go test ./internal/commands/ -v -run TestFork`
Expected: FAIL

**Step 3: Implement Fork and Unfork**

Create `internal/commands/fork.go`:

```go
package commands

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// Fork copies a plugin from Claude Code cache to ~/.claude-sync/plugins/ and updates config.
func Fork(claudeDir, syncDir, pluginKey string) error {
	parts := strings.SplitN(pluginKey, "@", 2)
	if len(parts) != 2 {
		return fmt.Errorf("invalid plugin key: %s (expected name@marketplace)", pluginKey)
	}
	pluginName := parts[0]
	marketplace := parts[1]

	// Find plugin install path from installed_plugins.json
	installed, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return err
	}
	installations, ok := installed.Plugins[pluginKey]
	if !ok || len(installations) == 0 {
		return fmt.Errorf("plugin %s is not installed", pluginKey)
	}
	srcPath := installations[0].InstallPath
	if srcPath == "" {
		// Fallback: construct path from marketplace
		srcPath = filepath.Join(claudeDir, "plugins", "marketplaces", marketplace, pluginName)
	}

	// Copy plugin files to sync dir
	dstPath := filepath.Join(syncDir, "plugins", pluginName)
	if err := copyDir(srcPath, dstPath); err != nil {
		return fmt.Errorf("copying plugin files: %w", err)
	}

	// Update config: move from upstream/pinned to forked
	cfgPath := filepath.Join(syncDir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return err
	}

	// Remove from upstream
	var newUpstream []string
	for _, p := range cfg.Upstream {
		if p != pluginKey {
			newUpstream = append(newUpstream, p)
		}
	}
	cfg.Upstream = newUpstream

	// Remove from pinned
	delete(cfg.Pinned, pluginKey)

	// Add to forked (use plugin name without marketplace)
	cfg.Forked = append(cfg.Forked, pluginName)
	cfg.Version = "2.0.0"

	newData, err := config.MarshalV2(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	// Stage and commit
	git.Add(syncDir, ".")
	return git.Commit(syncDir, fmt.Sprintf("Fork %s for customization", pluginName))
}

// Unfork removes a forked plugin and moves it back to upstream.
func Unfork(syncDir, pluginName, marketplace string) error {
	// Remove fork directory
	forkDir := filepath.Join(syncDir, "plugins", pluginName)
	if err := os.RemoveAll(forkDir); err != nil {
		return fmt.Errorf("removing fork directory: %w", err)
	}

	// Update config
	cfgPath := filepath.Join(syncDir, "config.yaml")
	data, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(data)
	if err != nil {
		return err
	}

	// Remove from forked
	var newForked []string
	for _, f := range cfg.Forked {
		if f != pluginName {
			newForked = append(newForked, f)
		}
	}
	cfg.Forked = newForked

	// Add back to upstream
	pluginKey := pluginName + "@" + marketplace
	cfg.Upstream = append(cfg.Upstream, pluginKey)
	cfg.Version = "2.0.0"

	newData, err := config.MarshalV2(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	git.Add(syncDir, ".")
	return git.Commit(syncDir, fmt.Sprintf("Unfork %s — returning to upstream", pluginName))
}

// copyDir recursively copies a directory.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		relPath, _ := filepath.Rel(src, path)
		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, d.Type().Perm())
	})
}
```

**Step 4: Wire up CLI commands**

Create `cmd/claude-sync/cmd_fork.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var forkCmd = &cobra.Command{
	Use:   "fork <plugin>",
	Short: "Fork an upstream plugin for customization",
	Long:  "Copies plugin files to ~/.claude-sync/plugins/ and moves it from upstream to forked category.",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := commands.Fork(paths.ClaudeDir(), paths.SyncDir(), args[0]); err != nil {
			return err
		}
		fmt.Printf("Forked %s — plugin files now in ~/.claude-sync/plugins/\n", args[0])
		fmt.Println("Edit the files, then 'claude-sync push' to sync changes.")
		return nil
	},
}
```

Create `cmd/claude-sync/cmd_unfork.go`:

```go
package main

import (
	"fmt"
	"strings"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var unforkMarketplace string

var unforkCmd = &cobra.Command{
	Use:   "unfork <plugin-name>",
	Short: "Return a forked plugin to upstream",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		pluginName := args[0]
		marketplace := unforkMarketplace
		if marketplace == "" {
			// Try to infer marketplace from plugin name
			if strings.Contains(pluginName, "@") {
				parts := strings.SplitN(pluginName, "@", 2)
				pluginName = parts[0]
				marketplace = parts[1]
			} else {
				return fmt.Errorf("specify marketplace with --marketplace flag or use name@marketplace format")
			}
		}
		if err := commands.Unfork(paths.SyncDir(), pluginName, marketplace); err != nil {
			return err
		}
		fmt.Printf("Unforked %s — returning to upstream from %s\n", pluginName, marketplace)
		return nil
	},
}

func init() {
	unforkCmd.Flags().StringVar(&unforkMarketplace, "marketplace", "", "Original marketplace to return to")
}
```

Add to `main.go` init(): `rootCmd.AddCommand(forkCmd)` and `rootCmd.AddCommand(unforkCmd)`

**Step 5: Run tests**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/commands/fork.go internal/commands/fork_test.go cmd/claude-sync/cmd_fork.go cmd/claude-sync/cmd_unfork.go cmd/claude-sync/main.go
git commit -m "feat: add fork and unfork commands for plugin customization"
```

---

## Task 7: Update Command

**Files:**
- Create: `internal/commands/update.go`
- Create: `internal/commands/update_test.go`
- Create: `cmd/claude-sync/cmd_update.go`
- Modify: `cmd/claude-sync/main.go`

The `update` command installs available plugin updates. For upstream plugins, it reinstalls from marketplace. For pinned plugins, it notifies but doesn't auto-update. For forked plugins, it syncs from the git repo.

**Step 1: Write failing tests**

Create `internal/commands/update_test.go`:

```go
package commands

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupUpdateTestEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = t.TempDir()

	// Create Claude dir
	os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{
			"context7@claude-plugins-official":[{"scope":"user","installPath":"/p","version":"1.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace":[{"scope":"user","installPath":"/p","version":"0.44.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}}`), 0644)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	// Create v2 config
	cfg := config.ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{"beads@beads-marketplace": "0.44.0"},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	return
}

func TestUpdateCheck(t *testing.T) {
	claudeDir, syncDir := setupUpdateTestEnv(t)

	result, err := UpdateCheck(claudeDir, syncDir)
	require.NoError(t, err)
	// Should identify upstream and pinned plugins
	assert.Len(t, result.UpstreamPlugins, 1)
	assert.Len(t, result.PinnedPlugins, 1)
	assert.Equal(t, "context7@claude-plugins-official", result.UpstreamPlugins[0].Key)
	assert.Equal(t, "beads@beads-marketplace", result.PinnedPlugins[0].Key)
	assert.Equal(t, "0.44.0", result.PinnedPlugins[0].PinnedVersion)
}

func TestUpdateResultHasUpdates(t *testing.T) {
	result := &UpdateResult{
		UpstreamPlugins: []UpstreamStatus{{Key: "a@b", InstalledVersion: "1.0"}},
	}
	assert.True(t, result.HasUpdates())

	empty := &UpdateResult{}
	assert.False(t, empty.HasUpdates())
}
```

**Step 2: Run tests to verify failure**

Run: `go test ./internal/commands/ -v -run TestUpdate`
Expected: FAIL

**Step 3: Implement Update logic**

Create `internal/commands/update.go`:

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/plugins"
)

// UpstreamStatus holds info about an upstream plugin's update status.
type UpstreamStatus struct {
	Key              string
	InstalledVersion string
}

// PinnedStatus holds info about a pinned plugin.
type PinnedStatus struct {
	Key              string
	PinnedVersion    string
	InstalledVersion string
}

// ForkedStatus holds info about a forked plugin.
type ForkedStatus struct {
	Name string
}

// UpdateResult holds the result of an update check.
type UpdateResult struct {
	UpstreamPlugins []UpstreamStatus
	PinnedPlugins   []PinnedStatus
	ForkedPlugins   []ForkedStatus
}

// HasUpdates returns true if there are any plugins that could be updated.
func (r *UpdateResult) HasUpdates() bool {
	return len(r.UpstreamPlugins) > 0 || len(r.PinnedPlugins) > 0 || len(r.ForkedPlugins) > 0
}

// UpdateCheck checks for available updates across all plugin categories.
func UpdateCheck(claudeDir, syncDir string) (*UpdateResult, error) {
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, err
	}

	installed, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, err
	}

	result := &UpdateResult{}

	// Check upstream plugins
	for _, key := range cfg.Upstream {
		status := UpstreamStatus{Key: key}
		if installs, ok := installed.Plugins[key]; ok && len(installs) > 0 {
			status.InstalledVersion = installs[0].Version
		}
		result.UpstreamPlugins = append(result.UpstreamPlugins, status)
	}

	// Check pinned plugins
	for key, pinnedVer := range cfg.Pinned {
		status := PinnedStatus{Key: key, PinnedVersion: pinnedVer}
		if installs, ok := installed.Plugins[key]; ok && len(installs) > 0 {
			status.InstalledVersion = installs[0].Version
		}
		result.PinnedPlugins = append(result.PinnedPlugins, status)
	}

	// Check forked plugins
	forkedKeys := make(map[string]bool)
	for _, name := range cfg.Forked {
		forkedKeys[name] = true
		result.ForkedPlugins = append(result.ForkedPlugins, ForkedStatus{Name: name})
	}

	return result, nil
}

// UpdateApply reinstalls upstream plugins to get latest versions.
func UpdateApply(claudeDir, syncDir string, pluginKeys []string, quiet bool) (installed, failed []string) {
	// Register local marketplace if forked plugins exist
	forks, _ := plugins.ListForkedPlugins(syncDir)
	if len(forks) > 0 {
		plugins.RegisterLocalMarketplace(claudeDir, syncDir)
	}

	for _, key := range pluginKeys {
		if !quiet {
			fmt.Printf("  Updating %s...\n", key)
		}
		if err := installPlugin(key); err != nil {
			failed = append(failed, key)
			if !quiet {
				fmt.Printf("  ✗ %s: %v\n", key, err)
			}
		} else {
			installed = append(installed, key)
			if !quiet {
				fmt.Printf("  ✓ %s\n", key)
			}
		}
	}
	return
}

// UpdateForkedPlugins reinstalls forked plugins from the local marketplace.
func UpdateForkedPlugins(claudeDir, syncDir string, forkNames []string, quiet bool) (installed, failed []string) {
	for _, name := range forkNames {
		key := plugins.ForkedPluginKey(name)
		if !quiet {
			fmt.Printf("  Updating forked %s...\n", name)
		}
		if err := installPlugin(key); err != nil {
			failed = append(failed, name)
			if !quiet {
				fmt.Printf("  ✗ %s: %v\n", name, err)
			}
		} else {
			installed = append(installed, name)
			if !quiet {
				fmt.Printf("  ✓ %s\n", name)
			}
		}
	}
	return
}
```

**Step 4: Wire up CLI command**

Create `cmd/claude-sync/cmd_update.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var updateCmd = &cobra.Command{
	Use:   "update",
	Short: "Apply available plugin updates",
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		result, err := commands.UpdateCheck(claudeDir, syncDir)
		if err != nil {
			return err
		}

		if !result.HasUpdates() {
			fmt.Println("All plugins up to date.")
			return nil
		}

		// Reinstall upstream plugins
		if len(result.UpstreamPlugins) > 0 {
			fmt.Println("UPSTREAM PLUGINS (reinstalling for latest)")
			var keys []string
			for _, p := range result.UpstreamPlugins {
				keys = append(keys, p.Key)
			}
			installed, failed := commands.UpdateApply(claudeDir, syncDir, keys, false)
			fmt.Printf("  %d updated, %d failed\n\n", len(installed), len(failed))
		}

		// Report pinned plugins
		if len(result.PinnedPlugins) > 0 {
			fmt.Println("PINNED PLUGINS (not auto-updated)")
			for _, p := range result.PinnedPlugins {
				fmt.Printf("  📌 %s pinned at %s\n", p.Key, p.PinnedVersion)
			}
			fmt.Println()
		}

		// Update forked plugins
		if len(result.ForkedPlugins) > 0 {
			fmt.Println("FORKED PLUGINS (reinstalling from local marketplace)")
			var names []string
			for _, f := range result.ForkedPlugins {
				names = append(names, f.Name)
			}
			installed, failed := commands.UpdateForkedPlugins(claudeDir, syncDir, names, false)
			fmt.Printf("  %d updated, %d failed\n", len(installed), len(failed))
		}

		return nil
	},
}
```

Add to `main.go` init(): `rootCmd.AddCommand(updateCmd)`

**Step 5: Run tests**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/commands/update.go internal/commands/update_test.go cmd/claude-sync/cmd_update.go cmd/claude-sync/main.go
git commit -m "feat: add update command for applying plugin updates"
```

---

## Task 8: Enhanced Status with Categories and --json Flag

**Files:**
- Modify: `internal/commands/status.go`
- Modify: `internal/commands/status_test.go`
- Modify: `cmd/claude-sync/cmd_status.go`

This task upgrades the status command to show categorized output (upstream/pinned/forked) with version info and adds `--json` output for the SessionStart hook.

**Step 1: Write failing tests**

Add to `internal/commands/status_test.go`:

```go
func TestStatusV2(t *testing.T) {
	claudeDir, syncDir := setupV2StatusEnv(t)

	result, err := Status(claudeDir, syncDir)
	require.NoError(t, err)

	assert.Len(t, result.UpstreamSynced, 1)
	assert.Len(t, result.PinnedSynced, 1)
	assert.Equal(t, "0.44.0", result.PinnedSynced[0].PinnedVersion)
}

func TestStatusJSON(t *testing.T) {
	claudeDir, syncDir := setupV2StatusEnv(t)

	result, err := Status(claudeDir, syncDir)
	require.NoError(t, err)

	jsonData, err := result.JSON()
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "upstream_synced")
	assert.Contains(t, string(jsonData), "pinned_synced")
}

func setupV2StatusEnv(t *testing.T) (string, string) {
	t.Helper()
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{
			"context7@claude-plugins-official":[{"scope":"user","installPath":"/p","version":"1.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace":[{"scope":"user","installPath":"/p","version":"0.44.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}}`), 0644)

	cfg := config.ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{"beads@beads-marketplace": "0.44.0"},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	return claudeDir, syncDir
}
```

Note: Add imports for `os`, `path/filepath`, and `config`.

**Step 2: Run tests to verify failure**

Run: `go test ./internal/commands/ -v -run "TestStatusV2|TestStatusJSON"`
Expected: FAIL

**Step 3: Update status implementation**

Replace `internal/commands/status.go`:

```go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

// PluginStatus represents a single plugin's sync status.
type PluginStatus struct {
	Key              string `json:"key"`
	InstalledVersion string `json:"installed_version,omitempty"`
	PinnedVersion    string `json:"pinned_version,omitempty"`
	Installed        bool   `json:"installed"`
}

// StatusResult holds the result of a status check.
type StatusResult struct {
	// Phase 1 compat fields
	Synced       []string          `json:"synced"`
	NotInstalled []string          `json:"not_installed"`
	Untracked    []string          `json:"untracked"`
	SettingsDiff csync.SettingsDiff `json:"-"`

	// Phase 2 categorized fields
	UpstreamSynced    []PluginStatus `json:"upstream_synced,omitempty"`
	UpstreamMissing   []PluginStatus `json:"upstream_missing,omitempty"`
	PinnedSynced      []PluginStatus `json:"pinned_synced,omitempty"`
	PinnedMissing     []PluginStatus `json:"pinned_missing,omitempty"`
	ForkedSynced      []PluginStatus `json:"forked_synced,omitempty"`
	ForkedMissing     []PluginStatus `json:"forked_missing,omitempty"`
	ConfigVersion     string         `json:"config_version"`
}

// JSON serializes the status result.
func (r *StatusResult) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// HasPendingChanges returns true if there are plugins to install or untracked plugins.
func (r *StatusResult) HasPendingChanges() bool {
	return len(r.NotInstalled) > 0 || len(r.Untracked) > 0
}

func Status(claudeDir, syncDir string) (*StatusResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	installedKeys := plugins.PluginKeys()
	installedSet := make(map[string]bool)
	for _, k := range installedKeys {
		installedSet[k] = true
	}

	// Build version lookup from installed plugins
	versionOf := func(key string) string {
		if installs, ok := plugins.Plugins[key]; ok && len(installs) > 0 {
			return installs[0].Version
		}
		return ""
	}

	result := &StatusResult{
		ConfigVersion: cfg.Version,
	}

	// Phase 1 compat: compute flat diff
	allDesired := cfg.AllPluginKeys()
	diff := csync.ComputePluginDiff(allDesired, installedKeys)
	result.Synced = diff.Synced
	result.NotInstalled = diff.ToInstall
	result.Untracked = diff.Untracked

	// Phase 2: categorize by type
	if cfg.Version >= "2" {
		// Upstream
		for _, key := range cfg.Upstream {
			ps := PluginStatus{Key: key, InstalledVersion: versionOf(key), Installed: installedSet[key]}
			if ps.Installed {
				result.UpstreamSynced = append(result.UpstreamSynced, ps)
			} else {
				result.UpstreamMissing = append(result.UpstreamMissing, ps)
			}
		}

		// Pinned
		for key, ver := range cfg.Pinned {
			ps := PluginStatus{Key: key, InstalledVersion: versionOf(key), PinnedVersion: ver, Installed: installedSet[key]}
			if ps.Installed {
				result.PinnedSynced = append(result.PinnedSynced, ps)
			} else {
				result.PinnedMissing = append(result.PinnedMissing, ps)
			}
		}

		// Forked — forked plugins appear as name@claude-sync-forks
		for _, name := range cfg.Forked {
			forkKey := name + "@claude-sync-forks"
			ps := PluginStatus{Key: forkKey, InstalledVersion: versionOf(forkKey), Installed: installedSet[forkKey]}
			// Also check plain name
			if !ps.Installed {
				ps.Installed = installedSet[name]
				ps.InstalledVersion = versionOf(name)
			}
			if ps.Installed {
				result.ForkedSynced = append(result.ForkedSynced, ps)
			} else {
				result.ForkedMissing = append(result.ForkedMissing, ps)
			}
		}
	}

	return result, nil
}
```

**Step 4: Update CLI to support --json and categorized display**

Replace `cmd/claude-sync/cmd_status.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var jsonOutput bool

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := commands.Status(paths.ClaudeDir(), paths.SyncDir())
		if err != nil {
			return err
		}

		if jsonOutput {
			data, err := result.JSON()
			if err != nil {
				return err
			}
			fmt.Println(string(data))
			return nil
		}

		// V2 categorized display
		if result.ConfigVersion >= "2" {
			if len(result.UpstreamSynced) > 0 || len(result.UpstreamMissing) > 0 {
				fmt.Println("UPSTREAM PLUGINS (auto-update)")
				for _, p := range result.UpstreamSynced {
					fmt.Printf("  ✓ %s  %s\n", p.Key, p.InstalledVersion)
				}
				for _, p := range result.UpstreamMissing {
					fmt.Printf("  ⚠️  %s  (not installed)\n", p.Key)
				}
				fmt.Println()
			}

			if len(result.PinnedSynced) > 0 || len(result.PinnedMissing) > 0 {
				fmt.Println("PINNED PLUGINS (manual update)")
				for _, p := range result.PinnedSynced {
					fmt.Printf("  📌 %s  %s (pinned: %s)\n", p.Key, p.InstalledVersion, p.PinnedVersion)
				}
				for _, p := range result.PinnedMissing {
					fmt.Printf("  ⚠️  %s  pinned at %s (not installed)\n", p.Key, p.PinnedVersion)
				}
				fmt.Println()
			}

			if len(result.ForkedSynced) > 0 || len(result.ForkedMissing) > 0 {
				fmt.Println("FORKED PLUGINS (from your repo)")
				for _, p := range result.ForkedSynced {
					fmt.Printf("  🔧 %s  %s\n", p.Key, p.InstalledVersion)
				}
				for _, p := range result.ForkedMissing {
					fmt.Printf("  ⚠️  %s  (not installed)\n", p.Key)
				}
				fmt.Println()
			}

			if len(result.Untracked) > 0 {
				fmt.Println("UNTRACKED (run 'claude-sync push' to add to config)")
				for _, p := range result.Untracked {
					fmt.Printf("  ? %s\n", p)
				}
				fmt.Println()
			}

			if !result.HasPendingChanges() {
				fmt.Println("Everything is in sync.")
			}
			return nil
		}

		// V1 fallback display
		if len(result.Synced) > 0 {
			fmt.Println("SYNCED")
			for _, p := range result.Synced {
				fmt.Printf("  ✓ %s\n", p)
			}
			fmt.Println()
		}

		if len(result.NotInstalled) > 0 {
			fmt.Println("NOT INSTALLED (run 'claude-sync pull' to install)")
			for _, p := range result.NotInstalled {
				fmt.Printf("  ⚠️  %s\n", p)
			}
			fmt.Println()
		}

		if len(result.Untracked) > 0 {
			fmt.Println("UNTRACKED (run 'claude-sync push' to add to config)")
			for _, p := range result.Untracked {
				fmt.Printf("  ? %s\n", p)
			}
			fmt.Println()
		}

		if !result.HasPendingChanges() {
			fmt.Println("Everything is in sync.")
		}

		return nil
	},
}

func init() {
	statusCmd.Flags().BoolVar(&jsonOutput, "json", false, "Output in JSON format")
}
```

**Step 5: Run tests**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 6: Commit**

```bash
git add internal/commands/status.go internal/commands/status_test.go cmd/claude-sync/cmd_status.go
git commit -m "feat: enhance status with categorized display and --json output"
```

---

## Task 9: Update Pull to Handle Forked Plugins and Local Marketplace

**Files:**
- Modify: `internal/commands/pull.go`
- Modify: `internal/commands/pull_test.go`

This task updates the `pull` command to register the local marketplace before installing plugins and to handle forked plugins correctly.

**Step 1: Write failing test**

Add to `internal/commands/pull_test.go`:

```go
func TestPull_RegistersLocalMarketplace(t *testing.T) {
	claudeDir, syncDir := setupV2PullEnv(t)

	// Create a forked plugin in sync dir
	forkDir := filepath.Join(syncDir, "plugins", "my-fork", ".claude-plugin")
	os.MkdirAll(forkDir, 0755)
	os.WriteFile(filepath.Join(forkDir, "plugin.json"), []byte(`{"name":"my-fork"}`), 0644)

	// Commit the fork
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "add fork").Run()

	_, err := PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify local marketplace was registered
	data, _ := os.ReadFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"))
	assert.Contains(t, string(data), "claude-sync-forks")
}

func setupV2PullEnv(t *testing.T) (string, string) {
	t.Helper()
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{"context7@claude-plugins-official":[{"scope":"user","installPath":"/p","version":"1.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]}}`), 0644)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"), []byte("{}"), 0644)

	cfg := config.ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{},
		Forked:   []string{"my-fork"},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	exec.Command("git", "init", syncDir).Run()
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run()

	return claudeDir, syncDir
}
```

Note: Add `os/exec` import.

**Step 2: Run tests to verify failure**

Run: `go test ./internal/commands/ -v -run TestPull_RegistersLocalMarketplace`
Expected: FAIL

**Step 3: Update pull.go to handle v2 config and forked plugins**

Update `PullDryRun` in `internal/commands/pull.go`:

After parsing config and before computing diff, add:

```go
// Register local marketplace if forked plugins exist
forks, _ := plugins.ListForkedPlugins(syncDir)
if len(forks) > 0 {
	plugins.RegisterLocalMarketplace(claudeDir, syncDir)
}

// Build effective desired list including forked plugin keys
var forkedKeys []string
for _, name := range cfg.Forked {
	forkedKeys = append(forkedKeys, plugins.ForkedPluginKey(name))
}
```

Update the `effectiveDesired` computation to include forked plugin keys and pinned plugin keys:

```go
// Combine all categories into effective desired
var allDesired []string
allDesired = append(allDesired, cfg.Upstream...)
for k := range cfg.Pinned {
	allDesired = append(allDesired, k)
}
allDesired = append(allDesired, forkedKeys...)

effectiveDesired := csync.ApplyPluginPreferences(
	allDesired,
	prefs.Plugins.Unsubscribe,
	prefs.Plugins.Personal,
)
```

Add import for `"github.com/ruminaider/claude-sync/internal/plugins"`.

**Step 4: Run tests**

Run: `go test ./internal/commands/ -v`
Expected: ALL PASS (including existing pull tests)

**Step 5: Commit**

```bash
git add internal/commands/pull.go internal/commands/pull_test.go
git commit -m "feat: update pull to register local marketplace and handle forked plugins"
```

---

## Task 10: Update Push for V2 Config Categories

**Files:**
- Modify: `internal/commands/push.go`
- Modify: `internal/commands/push_test.go`

Update push to work with v2 config — new plugins default to upstream category.

**Step 1: Write failing test**

Add to `internal/commands/push_test.go`:

```go
func TestPushScan_V2(t *testing.T) {
	claudeDir, syncDir := setupV2PushEnv(t)

	scan, err := PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, scan.AddedPlugins, "new-plugin@marketplace")
}

func TestPushApply_V2(t *testing.T) {
	claudeDir, syncDir := setupV2PushEnv(t)

	err := PushApply(claudeDir, syncDir, []string{"new-plugin@marketplace"}, nil, "Add new plugin")
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(data)
	// New plugins should be added to upstream
	assert.Contains(t, cfg.Upstream, "new-plugin@marketplace")
	assert.Equal(t, "2.0.0", cfg.Version)
}

func setupV2PushEnv(t *testing.T) (string, string) {
	t.Helper()
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	// Claude dir with existing + new plugin
	os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755)
	os.WriteFile(filepath.Join(claudeDir, "plugins", "installed_plugins.json"),
		[]byte(`{"version":2,"plugins":{
			"context7@claude-plugins-official":[{"scope":"user","installPath":"/p","version":"1.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"new-plugin@marketplace":[{"scope":"user","installPath":"/p","version":"1.0.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}}`), 0644)

	cfg := config.ConfigV2{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	exec.Command("git", "init", syncDir).Run()
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "init").Run()
	return claudeDir, syncDir
}
```

Note: Add `os/exec` and `config` imports.

**Step 2: Run tests to verify failure**

Run: `go test ./internal/commands/ -v -run "TestPushScan_V2|TestPushApply_V2"`
Expected: FAIL

**Step 3: Update push.go for v2**

Update `PushScan` to compute diff using `cfg.AllPluginKeys()` and update `PushApply` to add new plugins to `cfg.Upstream` (and use `MarshalV2` when version is 2+):

In `PushScan`:
```go
diff := csync.ComputePluginDiff(cfg.AllPluginKeys(), plugins.PluginKeys())
```

In `PushApply`, replace the flat pluginSet logic with category-aware logic:
```go
// Add new plugins to upstream category
upstreamSet := make(map[string]bool)
for _, p := range cfg.Upstream {
	upstreamSet[p] = true
}
for _, p := range addPlugins {
	upstreamSet[p] = true
}
for _, p := range removePlugins {
	delete(upstreamSet, p)
	delete(cfg.Pinned, p)
}

cfg.Upstream = make([]string, 0, len(upstreamSet))
for p := range upstreamSet {
	cfg.Upstream = append(cfg.Upstream, p)
}
sort.Strings(cfg.Upstream)
```

Use `config.Marshal(cfg)` which auto-selects v1 or v2 format.

**Step 4: Run tests**

Run: `go test ./internal/commands/ -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/commands/push.go internal/commands/push_test.go
git commit -m "feat: update push command for v2 categorized config"
```

---

## Task 11: Update Init for V2 Config Output

**Files:**
- Modify: `internal/commands/init.go`
- Modify: `internal/commands/init_test.go`

New installations should default to v2 format with all plugins as upstream.

**Step 1: Write failing test**

Add to `internal/commands/init_test.go`:

```go
func TestInit_CreatesV2Config(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)
	err := Init(claudeDir, syncDir)
	require.NoError(t, err)

	data, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, "2.0.0", cfg.Version)
	assert.NotEmpty(t, cfg.Upstream)
	assert.Empty(t, cfg.Pinned)
	assert.Empty(t, cfg.Forked)
}
```

**Step 2: Run test to verify failure**

Run: `go test ./internal/commands/ -v -run TestInit_CreatesV2Config`
Expected: FAIL (currently creates version "1.0.0")

**Step 3: Update init.go**

Change the config creation in `Init()`:

```go
cfg := config.ConfigV2{
	Version:  "2.0.0",
	Upstream: pluginKeys,
	Pinned:   map[string]string{},
	Settings: syncedSettings,
	Hooks:    syncedHooks,
}
```

And use `config.MarshalV2(cfg)` instead of `yamlv3.Marshal(cfg)`.

Remove the `yamlv3` import, add `config` import if not already present.

**Step 4: Run tests**

Run: `go test ./... -v`
Expected: ALL PASS

**Step 5: Commit**

```bash
git add internal/commands/init.go internal/commands/init_test.go
git commit -m "feat: init command now creates v2 config by default"
```

---

## Task 12: Plugin Slash Commands (/sync)

**Files:**
- Create: `plugin/commands/sync.md`

This creates the Claude Code plugin slash command that provides in-session access to sync operations.

**Step 1: Create slash command**

Create `plugin/commands/sync.md`:

```markdown
---
name: sync
description: Manage claude-sync configuration
arguments:
  - name: action
    description: "Action: status, pull, apply"
    required: false
    type: string
---

# /sync — Claude-Sync Plugin Commands

You have access to the claude-sync CLI tool for managing plugin synchronization.

## Available Actions

### /sync status (default)
Show the current sync status. Run: `claude-sync status --json`
Parse the JSON output and present a human-readable summary showing:
- Upstream plugins and their versions
- Pinned plugins with their locked versions
- Forked plugins from the sync repo
- Any untracked or missing plugins

### /sync pull
Pull latest configuration from remote. Run: `claude-sync pull`
Report what was installed, updated, or failed.

### /sync apply
Apply pending plugin updates. Run: `claude-sync update`
This reinstalls upstream plugins to get latest versions and syncs forked plugins.
Report results including any failures.

## Instructions

1. If no action is specified, default to `status`
2. Always use the CLI tool via bash — do not modify config files directly
3. For `status`, use `--json` flag and format the output nicely
4. Report errors clearly and suggest next steps
5. If claude-sync is not initialized, guide the user to run `claude-sync init` or `claude-sync join <url>`
```

**Step 2: Commit**

```bash
git add plugin/commands/
git commit -m "feat: add /sync slash command for in-session plugin management"
```

---

## Task 13: Update Integration Test for V2

**Files:**
- Modify: `tests/integration_test.go`

Update the integration test to exercise v2 config features: migration, pin, fork, and categorized status.

**Step 1: Update integration test**

Add new test function to `tests/integration_test.go`:

```go
func TestV2Workflow(t *testing.T) {
	machine1Claude := t.TempDir()
	machine1Sync := filepath.Join(t.TempDir(), ".claude-sync")

	setupMockClaude(t, machine1Claude, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
		"my-plugin@my-marketplace",
	})

	// Init creates v2 config
	err := commands.Init(machine1Claude, machine1Sync)
	require.NoError(t, err)

	// Verify v2 format
	data, _ := os.ReadFile(filepath.Join(machine1Sync, "config.yaml"))
	cfg, _ := config.Parse(data)
	assert.Equal(t, "2.0.0", cfg.Version)
	assert.Len(t, cfg.Upstream, 3)

	// Pin a plugin
	err = commands.Pin(machine1Sync, "beads@beads-marketplace", "0.44.0")
	require.NoError(t, err)

	data, _ = os.ReadFile(filepath.Join(machine1Sync, "config.yaml"))
	cfg, _ = config.Parse(data)
	assert.Len(t, cfg.Upstream, 2) // beads moved out
	assert.Equal(t, "0.44.0", cfg.Pinned["beads@beads-marketplace"])

	// Status shows categorized view
	status, err := commands.Status(machine1Claude, machine1Sync)
	require.NoError(t, err)
	assert.NotEmpty(t, status.UpstreamSynced)
	assert.NotEmpty(t, status.PinnedSynced)

	// JSON output works
	jsonData, err := status.JSON()
	require.NoError(t, err)
	assert.Contains(t, string(jsonData), "upstream_synced")

	// Unpin
	err = commands.Unpin(machine1Sync, "beads@beads-marketplace")
	require.NoError(t, err)

	data, _ = os.ReadFile(filepath.Join(machine1Sync, "config.yaml"))
	cfg, _ = config.Parse(data)
	assert.Len(t, cfg.Upstream, 3)
	assert.Empty(t, cfg.Pinned)
}
```

**Step 2: Run integration tests**

Run: `go test ./tests/ -tags=integration -v`
Expected: ALL PASS

**Step 3: Commit**

```bash
git add tests/
git commit -m "test: add v2 integration test for pin/unpin/status workflow"
```

---

## Task 14: Final Build Verification

**Files:**
- Modify: `Makefile` (update if needed)

**Step 1: Build and verify all commands**

Run:
```bash
make build
make test
./claude-sync version
./claude-sync --help
./claude-sync status
./claude-sync setup
```

Expected:
- Build succeeds
- All tests pass
- `--help` shows all new commands: `pin`, `unpin`, `fork`, `unfork`, `update`, `migrate`
- `status` shows not-initialized error
- `setup` shows alias instructions

**Step 2: Verify integration tests**

Run: `make test-integration` (or `go test ./tests/ -tags=integration -v`)
Expected: ALL PASS

**Step 3: Commit any Makefile changes**

```bash
git add Makefile
git commit -m "chore: update Makefile for Phase 2"
```

---

## Summary

### Phase 2 delivers:

| Command | What it does |
|---------|-------------|
| `migrate` | Interactive v1 → v2 config migration |
| `pin <plugin> [version]` | Lock plugin to specific version |
| `unpin <plugin>` | Return to auto-updates |
| `fork <plugin>` | Copy plugin to sync repo for customization |
| `unfork <plugin>` | Return forked plugin to upstream |
| `update` | Reinstall plugins for latest versions |
| `status --json` | JSON output for automation |
| `/sync` | In-session slash command |

### New/modified packages:

| Package | Change |
|---------|--------|
| `internal/config` | V2 schema with upstream/pinned/forked categories |
| `internal/plugins` | Forked plugin storage, local marketplace registration |
| `internal/marketplace` | Version checking via git queries |
| `internal/commands` | New commands + enhanced status/pull/push for v2 |
| `plugin/commands` | `/sync` slash command |

### What's deferred to Phase 3:
- Profile composition (`base.yaml` + `personal.yaml`)
- Config merge semantics with `$remove`/`$replace` escape hatches
- Project mapping via git remote
