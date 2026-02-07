# claude-sync Phase 1 (MVP) Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Build the minimal viable sync tool — `init`, `join`, `pull`, `push`, `status` commands that synchronize Claude Code plugins and settings across machines via a git-backed config repo.

**Architecture:** Go CLI using Cobra for commands, YAML config stored in `~/.claude-sync/` (git repo), shelling out to `git` binary for operations, `charmbracelet/huh` for interactive push selector. Reads/writes Claude Code files at `~/.claude/plugins/installed_plugins.json`, `~/.claude/plugins/known_marketplaces.json`, and `~/.claude/settings.json`.

**Tech Stack:** Go 1.22+, Cobra v1.9+, go.yaml.in/yaml/v3, charmbracelet/huh v2, stretchr/testify v1.10+, os/exec for git

**Key Design Decision:** Same-session freshness is a hard requirement. The primary update mechanism is a shell alias (`alias claude='claude-sync pull --quiet && command claude'`) that runs before Claude Code starts. The SessionStart hook provides *notification* of config drift only.

---

## Task 1: Project Setup — Dependencies and Module Structure

**Files:**
- Modify: `go.mod`
- Modify: `cmd/claude-sync/main.go`

**Step 1: Add dependencies**

Run:
```bash
cd /Users/albertgwo/Repositories/claude-sync
go get github.com/spf13/cobra@latest
go get go.yaml.in/yaml/v3@latest
go get github.com/stretchr/testify@latest
```

**Step 2: Restructure main.go to use Cobra**

Replace `cmd/claude-sync/main.go` with:

```go
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var version = "0.1.0-dev"

var rootCmd = &cobra.Command{
	Use:   "claude-sync",
	Short: "Sync Claude Code configuration across machines",
	Long:  "claude-sync synchronizes Claude Code plugins, settings, and hooks across multiple machines using a git-backed config repo.",
	Run: func(cmd *cobra.Command, args []string) {
		// Default behavior: show status
		statusCmd.Run(cmd, args)
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Show version",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("claude-sync %s\n", version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
	rootCmd.AddCommand(initCmd)
	rootCmd.AddCommand(joinCmd)
	rootCmd.AddCommand(statusCmd)
	rootCmd.AddCommand(pullCmd)
	rootCmd.AddCommand(pushCmd)
}

func main() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
```

**Step 3: Create stub command files**

Create `cmd/claude-sync/cmd_init.go`:
```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create new config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync init: not yet implemented")
		return nil
	},
}
```

Create `cmd/claude-sync/cmd_join.go`:
```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var joinCmd = &cobra.Command{
	Use:   "join <url>",
	Short: "Join existing config repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync join: not yet implemented")
		return nil
	},
}
```

Create `cmd/claude-sync/cmd_status.go`:
```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync status: not yet implemented")
		return nil
	},
}
```

Create `cmd/claude-sync/cmd_pull.go`:
```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var quietFlag bool

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest config and apply locally",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync pull: not yet implemented")
		return nil
	},
}

func init() {
	pullCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress output")
}
```

Create `cmd/claude-sync/cmd_push.go`:
```go
package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

var pushMessage string
var pushAll bool

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push changes to remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Println("claude-sync push: not yet implemented")
		return nil
	},
}

func init() {
	pushCmd.Flags().StringVarP(&pushMessage, "message", "m", "", "Commit message")
	pushCmd.Flags().BoolVar(&pushAll, "all", false, "Push all changes without interactive selection")
}
```

**Step 4: Verify it builds and runs**

Run: `go build -o claude-sync ./cmd/claude-sync && ./claude-sync version`
Expected: `claude-sync 0.1.0-dev`

Run: `./claude-sync --help`
Expected: Help text showing all subcommands

**Step 5: Commit**

```bash
git add go.mod go.sum cmd/claude-sync/
git commit -m "feat: restructure CLI with Cobra framework and stub commands"
```

---

## Task 2: Internal Git Package — Wrapper Around git Binary

**Files:**
- Create: `internal/git/git.go`
- Create: `internal/git/git_test.go`

**Step 1: Write failing tests for git wrapper**

Create `internal/git/git_test.go`:

```go
package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
	// Configure git user for commits
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	return dir
}

func TestRun(t *testing.T) {
	dir := initTestRepo(t)
	out, err := git.Run(dir, "status", "--porcelain")
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestIsRepo(t *testing.T) {
	t.Run("valid repo", func(t *testing.T) {
		dir := initTestRepo(t)
		assert.True(t, git.IsRepo(dir))
	})

	t.Run("not a repo", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, git.IsRepo(dir))
	})

	t.Run("nonexistent dir", func(t *testing.T) {
		assert.False(t, git.IsRepo("/nonexistent/path"))
	})
}

func TestIsClean(t *testing.T) {
	dir := initTestRepo(t)

	t.Run("clean repo", func(t *testing.T) {
		clean, err := git.IsClean(dir)
		require.NoError(t, err)
		assert.True(t, clean)
	})

	t.Run("dirty repo", func(t *testing.T) {
		os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
		clean, err := git.IsClean(dir)
		require.NoError(t, err)
		assert.False(t, clean)
	})
}

func TestRevParse(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()

	sha, err := git.RevParse(dir, "HEAD")
	require.NoError(t, err)
	assert.Len(t, sha, 40)
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	err := git.Init(dir)
	require.NoError(t, err)
	assert.True(t, git.IsRepo(dir))
}

func TestClone(t *testing.T) {
	// Create a source repo with a commit
	src := initTestRepo(t)
	os.WriteFile(filepath.Join(src, "test.txt"), []byte("hello"), 0644)
	exec.Command("git", "-C", src, "add", ".").Run()
	exec.Command("git", "-C", src, "commit", "-m", "initial").Run()

	// Clone it
	dst := filepath.Join(t.TempDir(), "clone")
	err := git.Clone(src, dst)
	require.NoError(t, err)
	assert.True(t, git.IsRepo(dst))

	// Verify file exists
	data, err := os.ReadFile(filepath.Join(dst, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestAddAndCommit(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	err := git.Add(dir, "test.txt")
	require.NoError(t, err)

	err = git.Commit(dir, "test commit")
	require.NoError(t, err)

	clean, _ := git.IsClean(dir)
	assert.True(t, clean)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/git/ -v`
Expected: Compilation error — package doesn't exist yet

**Step 3: Implement git wrapper**

Create `internal/git/git.go`:

```go
package git

import (
	"os/exec"
	"strings"
)

// Run executes a git command in the given directory and returns trimmed stdout.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRepo returns true if dir is a git repository.
func IsRepo(dir string) bool {
	_, err := Run(dir, "rev-parse", "--git-dir")
	return err == nil
}

// IsClean returns true if the working tree has no uncommitted changes.
func IsClean(dir string) (bool, error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// RevParse returns the resolved SHA for a ref.
func RevParse(dir, ref string) (string, error) {
	return Run(dir, "rev-parse", ref)
}

// Init initializes a new git repo in dir.
func Init(dir string) error {
	_, err := Run(dir, "init")
	return err
}

// Clone clones src into dst.
func Clone(src, dst string) error {
	cmd := exec.Command("git", "clone", src, dst)
	_, err := cmd.CombinedOutput()
	return err
}

// Add stages a file.
func Add(dir string, paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := Run(dir, args...)
	return err
}

// Commit creates a commit with the given message.
func Commit(dir, message string) error {
	_, err := Run(dir, "commit", "-m", message)
	return err
}

// Pull runs git pull --ff-only.
func Pull(dir string) error {
	_, err := Run(dir, "pull", "--ff-only")
	return err
}

// Push runs git push.
func Push(dir string) error {
	_, err := Run(dir, "push")
	return err
}

// Fetch runs git fetch --quiet.
func Fetch(dir string) error {
	_, err := Run(dir, "fetch", "--quiet")
	return err
}

// RemoteAdd adds a remote.
func RemoteAdd(dir, name, url string) error {
	_, err := Run(dir, "remote", "add", name, url)
	return err
}

// HasRemote returns true if the named remote exists.
func HasRemote(dir, name string) bool {
	_, err := Run(dir, "remote", "get-url", name)
	return err == nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/git/ -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/git/
git commit -m "feat: add internal git wrapper package with tests"
```

---

## Task 3: Config Package — YAML Parsing and Serialization

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

**Step 1: Write failing tests for config parsing**

Create `internal/config/config_test.go`:

```go
package config_test

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseConfig(t *testing.T) {
	t.Run("phase 1 simple format", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - episodic-memory@superpowers-marketplace
  - beads@beads-marketplace
settings:
  model: opus
hooks:
  PreCompact: "bd prime"
  SessionStart: "claude-sync pull --quiet"
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", cfg.Version)
		assert.Len(t, cfg.Plugins, 3)
		assert.Contains(t, cfg.Plugins, "context7@claude-plugins-official")
		assert.Equal(t, "opus", cfg.Settings["model"])
		assert.Equal(t, "bd prime", cfg.Hooks["PreCompact"])
	})

	t.Run("empty config", func(t *testing.T) {
		input := []byte(`version: "1.0.0"
plugins: []
`)
		cfg, err := config.Parse(input)
		require.NoError(t, err)
		assert.Empty(t, cfg.Plugins)
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := config.Parse([]byte(`{{{`))
		assert.Error(t, err)
	})
}

func TestMarshalConfig(t *testing.T) {
	cfg := config.Config{
		Version: "1.0.0",
		Plugins: []string{"context7@claude-plugins-official"},
		Settings: map[string]any{
			"model": "opus",
		},
		Hooks: map[string]string{
			"SessionStart": "claude-sync pull --quiet",
		},
	}

	data, err := config.Marshal(cfg)
	require.NoError(t, err)

	// Round-trip: parse back
	parsed, err := config.Parse(data)
	require.NoError(t, err)
	assert.Equal(t, cfg.Version, parsed.Version)
	assert.Equal(t, cfg.Plugins, parsed.Plugins)
}

func TestParseUserPreferences(t *testing.T) {
	input := []byte(`sync_mode: union
settings:
  model: opus
plugins:
  unsubscribe:
    - ralph-wiggum
    - greptile
  personal:
    - some-niche-plugin
pins:
  episodic-memory: "1.0.15"
`)
	prefs, err := config.ParseUserPreferences(input)
	require.NoError(t, err)
	assert.Equal(t, "union", prefs.SyncMode)
	assert.Contains(t, prefs.Plugins.Unsubscribe, "ralph-wiggum")
	assert.Contains(t, prefs.Plugins.Personal, "some-niche-plugin")
	assert.Equal(t, "1.0.15", prefs.Pins["episodic-memory"])
}

func TestDefaultUserPreferences(t *testing.T) {
	prefs := config.DefaultUserPreferences()
	assert.Equal(t, "union", prefs.SyncMode)
	assert.Empty(t, prefs.Plugins.Unsubscribe)
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/ -v`
Expected: Compilation error — package doesn't exist yet

**Step 3: Implement config package**

Create `internal/config/config.go`:

```go
package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Config represents ~/.claude-sync/config.yaml (Phase 1 format).
type Config struct {
	Version  string            `yaml:"version"`
	Plugins  []string          `yaml:"plugins"`
	Settings map[string]any    `yaml:"settings,omitempty"`
	Hooks    map[string]string `yaml:"hooks,omitempty"`
}

// UserPreferences represents ~/.claude-sync/user-preferences.yaml.
type UserPreferences struct {
	SyncMode string              `yaml:"sync_mode"`
	Settings map[string]any      `yaml:"settings,omitempty"`
	Plugins  UserPluginPrefs     `yaml:"plugins,omitempty"`
	Pins     map[string]string   `yaml:"pins,omitempty"`
}

// UserPluginPrefs holds plugin override preferences.
type UserPluginPrefs struct {
	Unsubscribe []string `yaml:"unsubscribe,omitempty"`
	Personal    []string `yaml:"personal,omitempty"`
}

// Parse parses config.yaml bytes into a Config.
func Parse(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// Marshal serializes a Config to YAML bytes.
func Marshal(cfg Config) ([]byte, error) {
	return yaml.Marshal(cfg)
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

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/ -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/config/
git commit -m "feat: add config package for YAML parsing and user preferences"
```

---

## Task 4: Claude Code Package — Read/Write Claude Code Files

**Files:**
- Create: `internal/claudecode/claudecode.go`
- Create: `internal/claudecode/claudecode_test.go`

This package reads and writes the Claude Code files that claude-sync integrates with:
- `~/.claude/plugins/installed_plugins.json`
- `~/.claude/plugins/known_marketplaces.json`
- `~/.claude/settings.json`

**Step 1: Write failing tests**

Create `internal/claudecode/claudecode_test.go`:

```go
package claudecode_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupClaudeDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "plugins")
	os.MkdirAll(pluginDir, 0755)
	return dir
}

func TestReadInstalledPlugins(t *testing.T) {
	dir := setupClaudeDir(t)

	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{
				"scope": "user",
				"installPath": "/path/to/cache/context7/1.0.0",
				"version": "1.0.0",
				"installedAt": "2026-01-05T00:00:00.000Z",
				"lastUpdated": "2026-01-05T00:00:00.000Z"
			}],
			"beads@beads-marketplace": [{
				"scope": "user",
				"installPath": "/path/to/cache/beads/0.44.0",
				"version": "0.44.0",
				"installedAt": "2026-01-05T00:00:00.000Z",
				"lastUpdated": "2026-01-05T00:00:00.000Z"
			}]
		}
	}`
	os.WriteFile(filepath.Join(dir, "plugins", "installed_plugins.json"), []byte(data), 0644)

	plugins, err := claudecode.ReadInstalledPlugins(dir)
	require.NoError(t, err)
	assert.Len(t, plugins.Plugins, 2)
	assert.Contains(t, plugins.Plugins, "context7@claude-plugins-official")
}

func TestReadInstalledPlugins_FileNotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := claudecode.ReadInstalledPlugins(dir)
	assert.Error(t, err)
}

func TestPluginKeys(t *testing.T) {
	dir := setupClaudeDir(t)

	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(dir, "plugins", "installed_plugins.json"), []byte(data), 0644)

	plugins, err := claudecode.ReadInstalledPlugins(dir)
	require.NoError(t, err)

	keys := plugins.PluginKeys()
	assert.ElementsMatch(t, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	}, keys)
}

func TestReadSettings(t *testing.T) {
	dir := setupClaudeDir(t)

	data := `{
		"hooks": {
			"PreCompact": [{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]
		},
		"env": {"SOME_VAR": "value"},
		"enabledPlugins": {"beads@beads-marketplace": true}
	}`
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte(data), 0644)

	settings, err := claudecode.ReadSettings(dir)
	require.NoError(t, err)
	assert.Contains(t, settings, "hooks")
	assert.Contains(t, settings, "enabledPlugins")
}

func TestReadMarketplaces(t *testing.T) {
	dir := setupClaudeDir(t)

	data := `{
		"claude-plugins-official": {
			"source": {"source": "github", "repo": "anthropics/claude-plugins-official"},
			"installLocation": "/path/to/marketplaces/claude-plugins-official",
			"lastUpdated": "2026-01-01T00:00:00.000Z"
		}
	}`
	os.WriteFile(filepath.Join(dir, "plugins", "known_marketplaces.json"), []byte(data), 0644)

	mkts, err := claudecode.ReadMarketplaces(dir)
	require.NoError(t, err)
	assert.Contains(t, mkts, "claude-plugins-official")
}

func TestWriteMarketplaces(t *testing.T) {
	dir := setupClaudeDir(t)

	mkts := map[string]json.RawMessage{
		"test-marketplace": json.RawMessage(`{"source":{"source":"directory","path":"/test"},"installLocation":"/test","lastUpdated":"2026-01-01T00:00:00Z"}`),
	}

	err := claudecode.WriteMarketplaces(dir, mkts)
	require.NoError(t, err)

	// Read back
	readBack, err := claudecode.ReadMarketplaces(dir)
	require.NoError(t, err)
	assert.Contains(t, readBack, "test-marketplace")
}

func TestClaudeDirExists(t *testing.T) {
	t.Run("exists", func(t *testing.T) {
		dir := setupClaudeDir(t)
		assert.True(t, claudecode.DirExists(dir))
	})

	t.Run("not exists", func(t *testing.T) {
		assert.False(t, claudecode.DirExists("/nonexistent"))
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/claudecode/ -v`
Expected: Compilation error — package doesn't exist yet

**Step 3: Implement claudecode package**

Create `internal/claudecode/claudecode.go`:

```go
package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// InstalledPlugins represents ~/.claude/plugins/installed_plugins.json.
type InstalledPlugins struct {
	Version int                                `json:"version"`
	Plugins map[string][]PluginInstallation    `json:"plugins"`
}

// PluginInstallation represents a single plugin installation entry.
type PluginInstallation struct {
	Scope        string `json:"scope"`
	InstallPath  string `json:"installPath"`
	ProjectPath  string `json:"projectPath,omitempty"`
	Version      string `json:"version"`
	InstalledAt  string `json:"installedAt"`
	LastUpdated  string `json:"lastUpdated"`
	GitCommitSha string `json:"gitCommitSha,omitempty"`
}

// PluginKeys returns a sorted list of plugin keys (e.g., "beads@beads-marketplace").
func (ip *InstalledPlugins) PluginKeys() []string {
	keys := make([]string, 0, len(ip.Plugins))
	for k := range ip.Plugins {
		keys = append(keys, k)
	}
	return keys
}

// DefaultClaudeDir returns the default ~/.claude path.
func DefaultClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// DirExists returns true if the Claude Code directory exists.
func DirExists(claudeDir string) bool {
	info, err := os.Stat(claudeDir)
	return err == nil && info.IsDir()
}

// ReadInstalledPlugins reads installed_plugins.json.
func ReadInstalledPlugins(claudeDir string) (*InstalledPlugins, error) {
	path := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}
	var plugins InstalledPlugins
	if err := json.Unmarshal(data, &plugins); err != nil {
		return nil, fmt.Errorf("parsing installed plugins: %w", err)
	}
	return &plugins, nil
}

// ReadSettings reads settings.json as a generic map.
func ReadSettings(claudeDir string) (map[string]json.RawMessage, error) {
	path := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing settings: %w", err)
	}
	return settings, nil
}

// WriteSettings writes settings.json.
func WriteSettings(claudeDir string, settings map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	path := filepath.Join(claudeDir, "settings.json")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// ReadMarketplaces reads known_marketplaces.json.
func ReadMarketplaces(claudeDir string) (map[string]json.RawMessage, error) {
	path := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading marketplaces: %w", err)
	}
	var mkts map[string]json.RawMessage
	if err := json.Unmarshal(data, &mkts); err != nil {
		return nil, fmt.Errorf("parsing marketplaces: %w", err)
	}
	return mkts, nil
}

// WriteMarketplaces writes known_marketplaces.json.
func WriteMarketplaces(claudeDir string, mkts map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(mkts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling marketplaces: %w", err)
	}
	path := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// Bootstrap creates minimal Claude Code directory structure for fresh machines.
func Bootstrap(claudeDir string) error {
	pluginDir := filepath.Join(claudeDir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	pluginsFile := filepath.Join(pluginDir, "installed_plugins.json")
	if _, err := os.Stat(pluginsFile); os.IsNotExist(err) {
		data := []byte(`{"version": 2, "plugins": {}}` + "\n")
		if err := os.WriteFile(pluginsFile, data, 0644); err != nil {
			return err
		}
	}

	mktsFile := filepath.Join(pluginDir, "known_marketplaces.json")
	if _, err := os.Stat(mktsFile); os.IsNotExist(err) {
		if err := os.WriteFile(mktsFile, []byte("{}\n"), 0644); err != nil {
			return err
		}
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/claudecode/ -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/claudecode/
git commit -m "feat: add claudecode package for reading/writing Claude Code files"
```

---

## Task 5: Sync Package — Core Diff and Reconciliation Logic

**Files:**
- Create: `internal/sync/sync.go`
- Create: `internal/sync/sync_test.go`

This is the core logic: compare desired state (config.yaml) vs actual state (installed plugins) and compute what needs to change.

**Step 1: Write failing tests**

Create `internal/sync/sync_test.go`:

```go
package sync_test

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/sync"
	"github.com/stretchr/testify/assert"
)

func TestComputeDiff(t *testing.T) {
	t.Run("install missing plugins", func(t *testing.T) {
		desired := []string{
			"context7@claude-plugins-official",
			"beads@beads-marketplace",
			"episodic-memory@superpowers-marketplace",
		}
		installed := []string{
			"context7@claude-plugins-official",
		}

		diff := sync.ComputePluginDiff(desired, installed)
		assert.ElementsMatch(t, []string{
			"beads@beads-marketplace",
			"episodic-memory@superpowers-marketplace",
		}, diff.ToInstall)
		assert.Empty(t, diff.ToRemove)
		assert.ElementsMatch(t, []string{
			"context7@claude-plugins-official",
		}, diff.Synced)
	})

	t.Run("detect untracked plugins", func(t *testing.T) {
		desired := []string{"context7@claude-plugins-official"}
		installed := []string{
			"context7@claude-plugins-official",
			"local-plugin@some-marketplace",
		}

		diff := sync.ComputePluginDiff(desired, installed)
		assert.Empty(t, diff.ToInstall)
		assert.ElementsMatch(t, []string{
			"local-plugin@some-marketplace",
		}, diff.Untracked)
	})

	t.Run("union mode keeps extras", func(t *testing.T) {
		desired := []string{"context7@official"}
		installed := []string{"context7@official", "extra@local"}

		diff := sync.ComputePluginDiff(desired, installed)
		assert.Empty(t, diff.ToRemove)
		assert.Contains(t, diff.Untracked, "extra@local")
	})
}

func TestApplyUserPreferences(t *testing.T) {
	t.Run("unsubscribe removes from desired", func(t *testing.T) {
		desired := []string{
			"context7@official",
			"greptile@official",
			"ralph-wiggum@official",
		}
		unsubscribe := []string{"ralph-wiggum@official", "greptile@official"}
		personal := []string{"my-plugin@local"}

		result := sync.ApplyPluginPreferences(desired, unsubscribe, personal)
		assert.Contains(t, result, "context7@official")
		assert.Contains(t, result, "my-plugin@local")
		assert.NotContains(t, result, "ralph-wiggum@official")
		assert.NotContains(t, result, "greptile@official")
	})
}

func TestComputeSettingsDiff(t *testing.T) {
	t.Run("detect changed settings", func(t *testing.T) {
		desired := map[string]any{"model": "opus"}
		current := map[string]any{"model": "sonnet"}

		diff := sync.ComputeSettingsDiff(desired, current)
		assert.Len(t, diff.Changed, 1)
		assert.Equal(t, "opus", diff.Changed["model"].Desired)
		assert.Equal(t, "sonnet", diff.Changed["model"].Current)
	})

	t.Run("no diff when equal", func(t *testing.T) {
		desired := map[string]any{"model": "opus"}
		current := map[string]any{"model": "opus"}

		diff := sync.ComputeSettingsDiff(desired, current)
		assert.Empty(t, diff.Changed)
	})
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/sync/ -v`
Expected: Compilation error

**Step 3: Implement sync package**

Create `internal/sync/sync.go`:

```go
package sync

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
// Removes unsubscribed plugins and adds personal plugins.
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
```

Note: Add `"fmt"` to the imports in sync.go.

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/sync/ -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/sync/
git commit -m "feat: add sync package with plugin diff and user preference logic"
```

---

## Task 6: Paths Package — Centralized Path Constants

**Files:**
- Create: `internal/paths/paths.go`
- Create: `internal/paths/paths_test.go`

**Step 1: Write failing test**

Create `internal/paths/paths_test.go`:

```go
package paths_test

import (
	"os"
	"strings"
	"testing"

	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/stretchr/testify/assert"
)

func TestSyncDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	assert.True(t, strings.HasPrefix(paths.SyncDir(), home))
	assert.True(t, strings.HasSuffix(paths.SyncDir(), ".claude-sync"))
}

func TestClaudeDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	assert.True(t, strings.HasPrefix(paths.ClaudeDir(), home))
	assert.True(t, strings.HasSuffix(paths.ClaudeDir(), ".claude"))
}

func TestConfigFile(t *testing.T) {
	assert.True(t, strings.HasSuffix(paths.ConfigFile(), "config.yaml"))
}

func TestUserPreferencesFile(t *testing.T) {
	assert.True(t, strings.HasSuffix(paths.UserPreferencesFile(), "user-preferences.yaml"))
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/paths/ -v`
Expected: Compilation error

**Step 3: Implement paths package**

Create `internal/paths/paths.go`:

```go
package paths

import (
	"os"
	"path/filepath"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// SyncDir returns ~/.claude-sync.
func SyncDir() string {
	return filepath.Join(home(), ".claude-sync")
}

// ClaudeDir returns ~/.claude.
func ClaudeDir() string {
	return filepath.Join(home(), ".claude")
}

// ConfigFile returns ~/.claude-sync/config.yaml.
func ConfigFile() string {
	return filepath.Join(SyncDir(), "config.yaml")
}

// UserPreferencesFile returns ~/.claude-sync/user-preferences.yaml.
func UserPreferencesFile() string {
	return filepath.Join(SyncDir(), "user-preferences.yaml")
}

// ForkedPluginsDir returns ~/.claude-sync/plugins.
func ForkedPluginsDir() string {
	return filepath.Join(SyncDir(), "plugins")
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/paths/ -v`
Expected: All tests PASS

**Step 5: Commit**

```bash
git add internal/paths/
git commit -m "feat: add paths package for centralized path constants"
```

---

## Task 7: Implement `init` Command

**Files:**
- Modify: `cmd/claude-sync/cmd_init.go`
- Create: `internal/commands/init.go`
- Create: `internal/commands/init_test.go`

**Step 1: Write failing tests for init logic**

Create `internal/commands/init_test.go`:

```go
package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")

	// Create Claude Code structure
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	// installed_plugins.json
	plugins := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), []byte(plugins), 0644)

	// known_marketplaces.json
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)

	// settings.json with hooks
	settings := `{
		"hooks": {
			"PreCompact": [{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]
		},
		"enabledPlugins": {"beads@beads-marketplace": true}
	}`
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte(settings), 0644)

	return claudeDir, syncDir
}

func TestInit(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify config.yaml was created
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Contains(t, cfg.Plugins, "context7@claude-plugins-official")
	assert.Contains(t, cfg.Plugins, "beads@beads-marketplace")
}

func TestInit_AlreadyExists(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)
	os.MkdirAll(syncDir, 0755)

	err := commands.Init(claudeDir, syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already exists")
}

func TestInit_NoClaudeDir(t *testing.T) {
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	err := commands.Init("/nonexistent", syncDir)
	assert.Error(t, err)
}

func TestInit_ExtractsHooks(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)

	// Should have the user-defined hook
	assert.Equal(t, "bd prime", cfg.Hooks["PreCompact"])
}

// settingsFieldsAreFiltered verifies that sensitive fields are not synced
func TestInit_FiltersSettings(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)

	// enabledPlugins should NOT be in synced settings
	_, hasEnabled := cfg.Settings["enabledPlugins"]
	assert.False(t, hasEnabled)
}

func TestInit_CreatesGitRepo(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	// Verify .git directory exists
	_, err = os.Stat(filepath.Join(syncDir, ".git"))
	assert.NoError(t, err)
}

func TestInit_GitignoresUserPreferences(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)

	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(syncDir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gitignore), "user-preferences.yaml")
}

// Compile-time check that json is used
var _ = json.Marshal
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/commands/ -v`
Expected: Compilation error

**Step 3: Implement init command logic**

Create `internal/commands/init.go`:

```go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"

	yamlv3 "go.yaml.in/yaml/v3"
)

// Fields from settings.json that should NOT be synced.
var excludedSettingsFields = map[string]bool{
	"enabledPlugins": true,
	"statusLine":     true,
	"permissions":    true,
}

// Init scans the current Claude Code setup and creates ~/.claude-sync/config.yaml.
func Init(claudeDir, syncDir string) error {
	// Check Claude Code directory exists
	if !claudecode.DirExists(claudeDir) {
		return fmt.Errorf("Claude Code directory not found at %s. Run Claude Code at least once first", claudeDir)
	}

	// Check sync dir doesn't already exist
	if _, err := os.Stat(syncDir); err == nil {
		return fmt.Errorf("%s already exists. Run 'claude-sync pull' to update, or remove it first", syncDir)
	}

	// Read installed plugins
	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return fmt.Errorf("reading plugins: %w", err)
	}

	// Extract plugin keys, sorted
	pluginKeys := plugins.PluginKeys()
	sort.Strings(pluginKeys)

	// Read settings and extract syncable fields
	syncedSettings := make(map[string]any)
	syncedHooks := make(map[string]string)

	settingsRaw, err := claudecode.ReadSettings(claudeDir)
	if err == nil {
		// Extract model if present
		if model, ok := settingsRaw["model"]; ok {
			var m string
			json.Unmarshal(model, &m)
			if m != "" {
				syncedSettings["model"] = m
			}
		}

		// Extract hooks (filtering out plugin-contributed hooks)
		if hooksRaw, ok := settingsRaw["hooks"]; ok {
			var hooks map[string]json.RawMessage
			if json.Unmarshal(hooksRaw, &hooks) == nil {
				for hookName, hookData := range hooks {
					// Extract command from the hook structure
					cmd := extractHookCommand(hookData)
					if cmd != "" {
						syncedHooks[hookName] = cmd
					}
				}
			}
		}
	}

	// Build config
	cfg := config.Config{
		Version:  "1.0.0",
		Plugins:  pluginKeys,
		Settings: syncedSettings,
		Hooks:    syncedHooks,
	}

	// Create sync directory
	if err := os.MkdirAll(syncDir, 0755); err != nil {
		return fmt.Errorf("creating sync directory: %w", err)
	}

	// Write config.yaml
	cfgData, err := yamlv3.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Write .gitignore
	gitignore := "user-preferences.yaml\n.last_fetch\n"
	if err := os.WriteFile(filepath.Join(syncDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	// Initialize git repo
	if err := git.Init(syncDir); err != nil {
		return fmt.Errorf("initializing git repo: %w", err)
	}

	// Initial commit
	if err := git.Add(syncDir, "."); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}
	if err := git.Commit(syncDir, "Initial claude-sync config"); err != nil {
		return fmt.Errorf("creating initial commit: %w", err)
	}

	return nil
}

// extractHookCommand extracts the command string from a hook configuration.
// Hook format: [{"matcher":"","hooks":[{"type":"command","command":"..."}]}]
func extractHookCommand(data json.RawMessage) string {
	var hookEntries []struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(data, &hookEntries) != nil {
		return ""
	}
	if len(hookEntries) > 0 && len(hookEntries[0].Hooks) > 0 {
		return hookEntries[0].Hooks[0].Command
	}
	return ""
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/commands/ -v`
Expected: All tests PASS

**Step 5: Wire init command to CLI**

Update `cmd/claude-sync/cmd_init.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var initCmd = &cobra.Command{
	Use:   "init",
	Short: "Create new config from current Claude Code setup",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := commands.Init(paths.ClaudeDir(), paths.SyncDir()); err != nil {
			return err
		}
		fmt.Println("✓ Created ~/.claude-sync/")
		fmt.Println("✓ Generated config.yaml from current Claude Code setup")
		fmt.Println("✓ Initialized git repository")
		fmt.Println()
		fmt.Println("Next steps:")
		fmt.Println("  1. Review config: cat ~/.claude-sync/config.yaml")
		fmt.Println("  2. Add remote: cd ~/.claude-sync && git remote add origin <url>")
		fmt.Println("  3. Push: claude-sync push -m \"Initial config\"")
		return nil
	},
}
```

**Step 6: Run full build and test**

Run: `go build ./cmd/claude-sync && go test ./... -v`
Expected: Build succeeds, all tests pass

**Step 7: Commit**

```bash
git add internal/commands/ cmd/claude-sync/cmd_init.go
git commit -m "feat: implement init command — scan Claude Code and create config.yaml"
```

---

## Task 8: Implement `status` Command

**Files:**
- Create: `internal/commands/status.go`
- Create: `internal/commands/status_test.go`
- Modify: `cmd/claude-sync/cmd_status.go`

**Step 1: Write failing tests**

Create `internal/commands/status_test.go`:

```go
package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupStatusEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir, syncDir = setupTestEnv(t)

	// Run init first to create sync dir
	err := commands.Init(claudeDir, syncDir)
	require.NoError(t, err)

	return claudeDir, syncDir
}

func TestStatus(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)

	// Both plugins from setupTestEnv should be synced
	assert.Contains(t, result.Synced, "context7@claude-plugins-official")
	assert.Contains(t, result.Synced, "beads@beads-marketplace")
	assert.Empty(t, result.NotInstalled)
	assert.Empty(t, result.Untracked)
}

func TestStatus_NotInstalled(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Add a plugin to config that isn't installed
	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	// Append a plugin
	newCfg := string(cfgData) + "  - new-plugin@some-marketplace\n"
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), []byte(newCfg), 0644)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.NotInstalled, "new-plugin@some-marketplace")
}

func TestStatus_Untracked(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Add a plugin to installed that isn't in config
	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"extra-plugin@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	result, err := commands.Status(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, result.Untracked, "extra-plugin@local")
}

func TestStatus_NoSyncDir(t *testing.T) {
	_, err := commands.Status("/tmp/test-claude", "/nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/commands/ -v -run TestStatus`
Expected: Compilation error

**Step 3: Implement status command logic**

Create `internal/commands/status.go`:

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

// StatusResult holds the result of a status check.
type StatusResult struct {
	Synced       []string
	NotInstalled []string
	Untracked    []string
	SettingsDiff csync.SettingsDiff
}

// Status compares config.yaml desired state against installed plugins.
func Status(claudeDir, syncDir string) (*StatusResult, error) {
	// Verify sync dir exists
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	// Read config.yaml
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}

	// Read installed plugins
	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	// Compute diff
	diff := csync.ComputePluginDiff(cfg.Plugins, plugins.PluginKeys())

	return &StatusResult{
		Synced:       diff.Synced,
		NotInstalled: diff.ToInstall,
		Untracked:    diff.Untracked,
	}, nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/commands/ -v -run TestStatus`
Expected: All tests PASS

**Step 5: Wire status command to CLI**

Update `cmd/claude-sync/cmd_status.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var statusCmd = &cobra.Command{
	Use:   "status",
	Short: "Show sync status",
	RunE: func(cmd *cobra.Command, args []string) error {
		result, err := commands.Status(paths.ClaudeDir(), paths.SyncDir())
		if err != nil {
			return err
		}

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
				fmt.Printf("  ⚠️ %s\n", p)
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

		if len(result.NotInstalled) == 0 && len(result.Untracked) == 0 {
			fmt.Println("Everything is in sync.")
		}

		return nil
	},
}
```

**Step 6: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 7: Commit**

```bash
git add internal/commands/status.go internal/commands/status_test.go cmd/claude-sync/cmd_status.go
git commit -m "feat: implement status command — compare config vs installed plugins"
```

---

## Task 9: Implement `pull` Command

**Files:**
- Create: `internal/commands/pull.go`
- Create: `internal/commands/pull_test.go`
- Modify: `cmd/claude-sync/cmd_pull.go`

The pull command: git pull config, compute diff, install missing plugins via `claude plugin install`, merge settings.

**Step 1: Write failing tests**

Create `internal/commands/pull_test.go`:

```go
package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPull_InstallsMissing(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Add a plugin to config that doesn't exist locally
	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, _ := os.ReadFile(cfgPath)
	// We can't actually run "claude plugin install" in tests,
	// so we test the diff computation and report generation.

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, result.ToInstall) // Everything already installed
}

func TestPull_NoSyncDir(t *testing.T) {
	_, err := commands.PullDryRun("/tmp/test-claude", "/nonexistent")
	assert.Error(t, err)
}

func TestPull_GitPull(t *testing.T) {
	// Create a "remote" repo
	remote := t.TempDir()
	exec.Command("git", "init", "--bare", remote).Run()

	// Create a sync dir that tracks the remote
	claudeDir, syncDir := setupStatusEnv(t)
	exec.Command("git", "-C", syncDir, "remote", "add", "origin", remote).Run()
	exec.Command("git", "-C", syncDir, "push", "-u", "origin", "master").Run()

	// Pull should succeed (nothing to pull)
	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, result.ToInstall)
}

func TestPull_RespectsUserPreferences(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Write user preferences that unsubscribe from beads
	prefs := `sync_mode: union
plugins:
  unsubscribe:
    - beads@beads-marketplace
`
	os.WriteFile(filepath.Join(syncDir, "user-preferences.yaml"), []byte(prefs), 0644)

	result, err := commands.PullDryRun(claudeDir, syncDir)
	require.NoError(t, err)

	// beads should NOT appear in effective desired list
	assert.NotContains(t, result.EffectiveDesired, "beads@beads-marketplace")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/commands/ -v -run TestPull`
Expected: Compilation error

**Step 3: Implement pull command logic**

Create `internal/commands/pull.go`:

```go
package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

// PullResult holds the result of a pull operation.
type PullResult struct {
	ToInstall        []string
	ToRemove         []string
	Synced           []string
	Untracked        []string
	EffectiveDesired []string
	Installed        []string
	Failed           []string
}

// PullDryRun computes what a pull would do without making changes.
func PullDryRun(claudeDir, syncDir string) (*PullResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	// Read config
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, err
	}

	// Read user preferences
	prefs := config.DefaultUserPreferences()
	prefsPath := filepath.Join(syncDir, "user-preferences.yaml")
	if prefsData, err := os.ReadFile(prefsPath); err == nil {
		prefs, _ = config.ParseUserPreferences(prefsData)
	}

	// Apply preferences to desired list
	effectiveDesired := csync.ApplyPluginPreferences(
		cfg.Plugins,
		prefs.Plugins.Unsubscribe,
		prefs.Plugins.Personal,
	)

	// Read installed plugins
	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	// Compute diff
	diff := csync.ComputePluginDiff(effectiveDesired, plugins.PluginKeys())

	result := &PullResult{
		ToInstall:        diff.ToInstall,
		Synced:           diff.Synced,
		Untracked:        diff.Untracked,
		EffectiveDesired: effectiveDesired,
	}

	// In exact mode, untracked becomes to_remove
	if prefs.SyncMode == "exact" {
		result.ToRemove = diff.Untracked
		result.Untracked = nil
	}

	return result, nil
}

// Pull executes a full pull: git pull, compute diff, install/remove plugins.
func Pull(claudeDir, syncDir string, quiet bool) (*PullResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	// Git pull (skip if no remote)
	if git.HasRemote(syncDir, "origin") {
		if err := git.Pull(syncDir); err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Warning: git pull failed: %v\n", err)
			}
		}
	}

	// Compute what needs to change
	result, err := PullDryRun(claudeDir, syncDir)
	if err != nil {
		return nil, err
	}

	// Install missing plugins
	for _, plugin := range result.ToInstall {
		if !quiet {
			fmt.Printf("  Installing %s...\n", plugin)
		}
		if err := installPlugin(plugin); err != nil {
			result.Failed = append(result.Failed, plugin)
			if !quiet {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", plugin, err)
			}
		} else {
			result.Installed = append(result.Installed, plugin)
			if !quiet {
				fmt.Printf("  ✓ %s\n", plugin)
			}
		}
	}

	// Retry failed once
	if len(result.Failed) > 0 {
		if !quiet {
			fmt.Println("\nRetrying failed plugins...")
		}
		var stillFailed []string
		for _, plugin := range result.Failed {
			if err := installPlugin(plugin); err != nil {
				stillFailed = append(stillFailed, plugin)
			} else {
				result.Installed = append(result.Installed, plugin)
			}
		}
		result.Failed = stillFailed
	}

	return result, nil
}

func installPlugin(pluginKey string) error {
	cmd := exec.Command("claude", "plugin", "install", pluginKey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/commands/ -v -run TestPull`
Expected: All tests PASS

**Step 5: Wire pull command to CLI**

Update `cmd/claude-sync/cmd_pull.go`:

```go
package main

import (
	"fmt"
	"os"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var quietFlag bool

var pullCmd = &cobra.Command{
	Use:   "pull",
	Short: "Pull latest config and apply locally",
	RunE: func(cmd *cobra.Command, args []string) error {
		if !quietFlag {
			fmt.Println("Pulling latest config...")
		}

		result, err := commands.Pull(paths.ClaudeDir(), paths.SyncDir(), quietFlag)
		if err != nil {
			return err
		}

		if quietFlag {
			return nil
		}

		if len(result.Installed) > 0 {
			fmt.Printf("\n✓ %d plugin(s) installed\n", len(result.Installed))
		}

		if len(result.Failed) > 0 {
			fmt.Fprintf(os.Stderr, "\n⚠️ %d plugin(s) failed:\n", len(result.Failed))
			for _, p := range result.Failed {
				fmt.Fprintf(os.Stderr, "  • %s\n", p)
			}
		}

		if len(result.Untracked) > 0 {
			fmt.Printf("\nNote: %d plugin(s) installed locally but not in config:\n", len(result.Untracked))
			for _, p := range result.Untracked {
				fmt.Printf("  • %s\n", p)
			}
			fmt.Println("Run 'claude-sync push' to add them, or keep as local-only.")
		}

		if len(result.ToInstall) == 0 && len(result.Failed) == 0 {
			fmt.Println("Everything up to date.")
		}

		return nil
	},
}

func init() {
	pullCmd.Flags().BoolVarP(&quietFlag, "quiet", "q", false, "Suppress output")
}
```

**Step 6: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 7: Commit**

```bash
git add internal/commands/pull.go internal/commands/pull_test.go cmd/claude-sync/cmd_pull.go
git commit -m "feat: implement pull command — sync config and install plugins"
```

---

## Task 10: Implement `push` Command (with Interactive TUI)

**Files:**
- Create: `internal/commands/push.go`
- Create: `internal/commands/push_test.go`
- Modify: `cmd/claude-sync/cmd_push.go`

**Step 1: Add huh dependency**

Run:
```bash
go get github.com/charmbracelet/huh/v2@latest
```

**Step 2: Write failing tests**

Create `internal/commands/push_test.go`:

```go
package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPushScan(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Add a new plugin locally that isn't in config
	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"new-plugin@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Contains(t, scan.AddedPlugins, "new-plugin@local")
}

func TestPushApply(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	// Add remote so push can work
	remote := t.TempDir()
	exec.Command("git", "init", "--bare", remote).Run()
	exec.Command("git", "-C", syncDir, "remote", "add", "origin", remote).Run()
	exec.Command("git", "-C", syncDir, "push", "-u", "origin", "master").Run()

	// Add a new plugin locally
	pluginsPath := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data := `{
		"version": 2,
		"plugins": {
			"context7@claude-plugins-official": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"beads@beads-marketplace": [{"scope":"user","installPath":"/p","version":"0.44","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}],
			"new-plugin@local": [{"scope":"user","installPath":"/p","version":"1.0","installedAt":"2026-01-01T00:00:00Z","lastUpdated":"2026-01-01T00:00:00Z"}]
		}
	}`
	os.WriteFile(pluginsPath, []byte(data), 0644)

	// Apply push with specific selections
	err := commands.PushApply(claudeDir, syncDir, []string{"new-plugin@local"}, nil, "Add new plugin")
	require.NoError(t, err)

	// Verify config.yaml was updated
	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Contains(t, cfg.Plugins, "new-plugin@local")
}

func TestPushScan_NoChanges(t *testing.T) {
	claudeDir, syncDir := setupStatusEnv(t)

	scan, err := commands.PushScan(claudeDir, syncDir)
	require.NoError(t, err)
	assert.Empty(t, scan.AddedPlugins)
	assert.Empty(t, scan.RemovedPlugins)
}
```

**Step 3: Run tests to verify they fail**

Run: `go test ./internal/commands/ -v -run TestPush`
Expected: Compilation error

**Step 4: Implement push command logic**

Create `internal/commands/push.go`:

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

// PushScanResult holds detected local changes that could be pushed.
type PushScanResult struct {
	AddedPlugins   []string // Installed locally but not in config
	RemovedPlugins []string // In config but not installed locally
	ChangedSettings map[string]csync.SettingChange
}

// HasChanges returns true if there's anything to push.
func (r *PushScanResult) HasChanges() bool {
	return len(r.AddedPlugins) > 0 || len(r.RemovedPlugins) > 0 || len(r.ChangedSettings) > 0
}

// PushScan scans for local changes that differ from config.yaml.
func PushScan(claudeDir, syncDir string) (*PushScanResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized")
	}

	// Read config
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, err
	}

	// Read installed plugins
	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, err
	}

	diff := csync.ComputePluginDiff(cfg.Plugins, plugins.PluginKeys())

	return &PushScanResult{
		AddedPlugins:   diff.Untracked,   // Installed but not in config
		RemovedPlugins: diff.ToInstall,    // In config but not installed (reversed perspective)
	}, nil
}

// PushApply applies selected changes to config.yaml, commits, and pushes.
func PushApply(claudeDir, syncDir string, addPlugins, removePlugins []string, message string) error {
	// Read current config
	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return err
	}

	// Apply changes
	pluginSet := make(map[string]bool)
	for _, p := range cfg.Plugins {
		pluginSet[p] = true
	}
	for _, p := range addPlugins {
		pluginSet[p] = true
	}
	for _, p := range removePlugins {
		delete(pluginSet, p)
	}

	cfg.Plugins = make([]string, 0, len(pluginSet))
	for p := range pluginSet {
		cfg.Plugins = append(cfg.Plugins, p)
	}
	sort.Strings(cfg.Plugins)

	// Write updated config
	newData, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	// Generate commit message if not provided
	if message == "" {
		message = generateCommitMessage(addPlugins, removePlugins)
	}

	// Git add, commit, push
	if err := git.Add(syncDir, "config.yaml"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := git.Commit(syncDir, message); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	if git.HasRemote(syncDir, "origin") {
		if err := git.Push(syncDir); err != nil {
			return fmt.Errorf("pushing: %w", err)
		}
	}

	return nil
}

func generateCommitMessage(added, removed []string) string {
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "Add "+strings.Join(shortNames(added), ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "Remove "+strings.Join(shortNames(removed), ", "))
	}
	if len(parts) == 0 {
		return "Update config"
	}
	return strings.Join(parts, "; ")
}

func shortNames(plugins []string) []string {
	names := make([]string, len(plugins))
	for i, p := range plugins {
		if idx := strings.Index(p, "@"); idx > 0 {
			names[i] = p[:idx]
		} else {
			names[i] = p
		}
	}
	return names
}
```

**Step 5: Run tests to verify they pass**

Run: `go test ./internal/commands/ -v -run TestPush`
Expected: All tests PASS

**Step 6: Wire push command to CLI with huh interactive selector**

Update `cmd/claude-sync/cmd_push.go`:

```go
package main

import (
	"fmt"
	"os"

	huh "github.com/charmbracelet/huh/v2"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var pushMessage string
var pushAll bool

var pushCmd = &cobra.Command{
	Use:   "push",
	Short: "Push changes to remote",
	RunE: func(cmd *cobra.Command, args []string) error {
		claudeDir := paths.ClaudeDir()
		syncDir := paths.SyncDir()

		fmt.Println("Scanning local state...")
		scan, err := commands.PushScan(claudeDir, syncDir)
		if err != nil {
			return err
		}

		if !scan.HasChanges() {
			fmt.Println("Nothing to push. Everything matches config.")
			return nil
		}

		var selectedAdd []string
		var selectedRemove []string

		if pushAll {
			selectedAdd = scan.AddedPlugins
			selectedRemove = scan.RemovedPlugins
		} else {
			// Interactive selection with huh
			if len(scan.AddedPlugins) > 0 {
				var options []huh.Option[string]
				for _, p := range scan.AddedPlugins {
					options = append(options, huh.NewOption(p, p).Selected(true))
				}
				err := huh.NewForm(
					huh.NewGroup(
						huh.NewMultiSelect[string]().
							Title("Plugins to add to config:").
							Options(options...).
							Value(&selectedAdd),
					),
				).Run()
				if err != nil {
					return err
				}
			}
		}

		if len(selectedAdd) == 0 && len(selectedRemove) == 0 {
			fmt.Println("No changes selected.")
			return nil
		}

		if err := commands.PushApply(claudeDir, syncDir, selectedAdd, selectedRemove, pushMessage); err != nil {
			return err
		}

		fmt.Println("✓ Changes pushed successfully.")
		return nil
	},
}

func init() {
	pushCmd.Flags().StringVarP(&pushMessage, "message", "m", "", "Commit message")
	pushCmd.Flags().BoolVar(&pushAll, "all", false, "Push all changes without interactive selection")
}

// Ensure os is used
var _ = os.Stdin
```

**Step 7: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 8: Commit**

```bash
git add go.mod go.sum internal/commands/push.go internal/commands/push_test.go cmd/claude-sync/cmd_push.go
git commit -m "feat: implement push command with interactive TUI selection"
```

---

## Task 11: Implement `join` Command

**Files:**
- Create: `internal/commands/join.go`
- Create: `internal/commands/join_test.go`
- Modify: `cmd/claude-sync/cmd_join.go`

**Step 1: Write failing tests**

Create `internal/commands/join_test.go`:

```go
package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoin(t *testing.T) {
	// Create a "remote" config repo
	remote := t.TempDir()
	exec.Command("git", "init", remote).Run()
	exec.Command("git", "-C", remote, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", remote, "config", "user.name", "Test").Run()

	// Write a config.yaml
	cfgContent := `version: "1.0.0"
plugins:
  - context7@claude-plugins-official
`
	os.WriteFile(filepath.Join(remote, "config.yaml"), []byte(cfgContent), 0644)
	exec.Command("git", "-C", remote, "add", ".").Run()
	exec.Command("git", "-C", remote, "commit", "-m", "init").Run()

	// Set up test dirs
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	// Verify config.yaml exists
	_, err = os.Stat(filepath.Join(syncDir, "config.yaml"))
	assert.NoError(t, err)
}

func TestJoin_AlreadyExists(t *testing.T) {
	syncDir := t.TempDir()
	err := commands.Join("http://example.com/repo.git", "/tmp/claude", syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test ./internal/commands/ -v -run TestJoin`
Expected: Compilation error

**Step 3: Implement join command**

Create `internal/commands/join.go`:

```go
package commands

import (
	"fmt"
	"os"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/git"
)

// Join clones an existing config repo and applies it locally.
func Join(repoURL, claudeDir, syncDir string) error {
	// Check sync dir doesn't already exist
	if _, err := os.Stat(syncDir); err == nil {
		return fmt.Errorf("%s already exists. Run 'claude-sync pull' instead", syncDir)
	}

	// Bootstrap Claude Code dir if it doesn't exist
	if !claudecode.DirExists(claudeDir) {
		if err := claudecode.Bootstrap(claudeDir); err != nil {
			return fmt.Errorf("bootstrapping Claude Code directory: %w", err)
		}
	}

	// Clone config repo
	if err := git.Clone(repoURL, syncDir); err != nil {
		return fmt.Errorf("cloning config repo: %w", err)
	}

	return nil
}
```

**Step 4: Run tests to verify they pass**

Run: `go test ./internal/commands/ -v -run TestJoin`
Expected: All tests PASS

**Step 5: Wire join command to CLI**

Update `cmd/claude-sync/cmd_join.go`:

```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/paths"
	"github.com/spf13/cobra"
)

var joinCmd = &cobra.Command{
	Use:   "join <url>",
	Short: "Join existing config repo",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		repoURL := args[0]
		fmt.Printf("Cloning config from %s...\n", repoURL)

		if err := commands.Join(repoURL, paths.ClaudeDir(), paths.SyncDir()); err != nil {
			return err
		}

		fmt.Println("✓ Cloned config repo to ~/.claude-sync/")
		fmt.Println()
		fmt.Println("Run 'claude-sync pull' to apply the config.")
		return nil
	},
}
```

**Step 6: Run full test suite**

Run: `go test ./... -v`
Expected: All tests PASS

**Step 7: Commit**

```bash
git add internal/commands/join.go internal/commands/join_test.go cmd/claude-sync/cmd_join.go
git commit -m "feat: implement join command — clone existing config repo"
```

---

## Task 12: SessionStart Hook Script

**Files:**
- Create: `plugin/hooks/session-start.sh`
- Create: `plugin/.claude-plugin/plugin.json`
- Create: `plugin/hooks/hooks.json`

This task creates the Claude Code plugin structure that ships with claude-sync. The hook notifies users of pending updates on session start.

**Step 1: Create plugin.json**

Create `plugin/.claude-plugin/plugin.json`:

```json
{
  "name": "claude-sync",
  "version": "0.1.0",
  "description": "Sync Claude Code configuration across machines",
  "author": {
    "name": "Albert Gwo"
  },
  "repository": "https://github.com/ruminaider/claude-sync"
}
```

**Step 2: Create hooks.json**

Create `plugin/hooks/hooks.json`:

```json
{
  "hooks": {
    "SessionStart": [
      {
        "matcher": "startup",
        "hooks": [
          {
            "type": "command",
            "command": "${CLAUDE_PLUGIN_ROOT}/hooks/session-start.sh",
            "timeout": 10
          }
        ]
      }
    ]
  }
}
```

**Step 3: Create session-start.sh**

Create `plugin/hooks/session-start.sh`:

```bash
#!/bin/bash
set -e

# Determine platform and select bundled binary
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ARCH=$(uname -m)
OS=$(uname -s | tr '[:upper:]' '[:lower:]')

case "$ARCH" in
  x86_64) ARCH="amd64" ;;
  aarch64|arm64) ARCH="arm64" ;;
esac

CLAUDE_SYNC="${SCRIPT_DIR}/bin/claude-sync-${OS}-${ARCH}"

# Fallback: if bundled binary doesn't exist, try PATH
if [ ! -x "$CLAUDE_SYNC" ]; then
  CLAUDE_SYNC=$(command -v claude-sync 2>/dev/null || echo "")
fi

# If claude-sync is not available, exit silently
if [ -z "$CLAUDE_SYNC" ]; then
  echo "{}"
  exit 0
fi

# Check if ~/.claude-sync exists
if [ ! -d "$HOME/.claude-sync" ]; then
  echo "{}"
  exit 0
fi

SYNC_DIR="$HOME/.claude-sync"

# Timestamp-based skip to avoid redundant fetches from concurrent sessions
LAST_FETCH_FILE="${SYNC_DIR}/.last_fetch"
FETCH_INTERVAL=30

now=$(date +%s)
last_fetch=$(cat "$LAST_FETCH_FILE" 2>/dev/null || echo 0)

if [ $((now - last_fetch)) -gt $FETCH_INTERVAL ]; then
  # Quick git fetch with 2-second timeout
  cd "$SYNC_DIR"
  timeout 2 git fetch --quiet 2>/dev/null || true
  echo "$now" > "$LAST_FETCH_FILE"
fi

# Check if behind remote
cd "$SYNC_DIR"
LOCAL=$(git rev-parse HEAD 2>/dev/null || echo "")
REMOTE=$(git rev-parse @{u} 2>/dev/null || echo "$LOCAL")

CONFIG_CHANGES=false
if [ -n "$LOCAL" ] && [ "$LOCAL" != "$REMOTE" ]; then
  CONFIG_CHANGES=true
fi

# Quick diff check using claude-sync
PLUGIN_DIFF=$("$CLAUDE_SYNC" status --json 2>/dev/null || echo "")

# Determine if there are pending updates
HAS_NOT_INSTALLED=false
if echo "$PLUGIN_DIFF" | grep -q '"not_installed"'; then
  HAS_NOT_INSTALLED=true
fi

# Build notification
if [ "$CONFIG_CHANGES" = true ] || [ "$HAS_NOT_INSTALLED" = true ]; then
  MSG="claude-sync: Updates available."
  if [ "$CONFIG_CHANGES" = true ]; then
    MSG="$MSG Config changes pending."
  fi
  MSG="$MSG Run '/sync status' for details. Restart session to apply."

  cat <<EOF
{
  "hookSpecificOutput": {
    "hookEventName": "SessionStart",
    "additionalContext": "$MSG"
  }
}
EOF
else
  echo "{}"
fi
```

**Step 4: Make hook executable**

Run: `chmod +x plugin/hooks/session-start.sh`

**Step 5: Commit**

```bash
git add plugin/
git commit -m "feat: add Claude Code plugin with SessionStart notification hook"
```

---

## Task 13: End-to-End Integration Test

**Files:**
- Create: `tests/integration_test.go`

**Step 1: Write integration test**

Create `tests/integration_test.go`:

```go
//go:build integration

package tests

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullWorkflow tests init → status → push → join → pull cycle
func TestFullWorkflow(t *testing.T) {
	// Set up mock Claude Code directories for two "machines"
	machine1Claude := t.TempDir()
	machine1Sync := filepath.Join(t.TempDir(), ".claude-sync")
	machine2Claude := t.TempDir()
	machine2Sync := filepath.Join(t.TempDir(), ".claude-sync")

	// Create a bare "remote" repo
	remote := t.TempDir()
	exec.Command("git", "init", "--bare", remote).Run()

	// --- Machine 1: Setup ---
	setupMockClaude(t, machine1Claude, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	})

	// Machine 1: Init
	err := commands.Init(machine1Claude, machine1Sync)
	require.NoError(t, err)

	// Machine 1: Add remote and push
	exec.Command("git", "-C", machine1Sync, "remote", "add", "origin", remote).Run()
	exec.Command("git", "-C", machine1Sync, "push", "-u", "origin", "master").Run()

	// Machine 1: Status should show everything synced
	status, err := commands.Status(machine1Claude, machine1Sync)
	require.NoError(t, err)
	assert.Len(t, status.Synced, 2)
	assert.Empty(t, status.NotInstalled)

	// --- Machine 2: Join ---
	setupMockClaude(t, machine2Claude, []string{"context7@claude-plugins-official"})

	err = commands.Join(remote, machine2Claude, machine2Sync)
	require.NoError(t, err)

	// Machine 2: Status should show beads as not installed
	status2, err := commands.Status(machine2Claude, machine2Sync)
	require.NoError(t, err)
	assert.Contains(t, status2.NotInstalled, "beads@beads-marketplace")

	// --- Machine 1: Add new plugin and push ---
	addPluginToMockClaude(t, machine1Claude, "new-plugin@local")
	err = commands.PushApply(machine1Claude, machine1Sync, []string{"new-plugin@local"}, nil, "Add new plugin")
	require.NoError(t, err)

	// Verify config was updated
	cfgData, _ := os.ReadFile(filepath.Join(machine1Sync, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	assert.Contains(t, cfg.Plugins, "new-plugin@local")
}

func setupMockClaude(t *testing.T, dir string, plugins []string) {
	t.Helper()
	pluginDir := filepath.Join(dir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	pluginMap := make(map[string]interface{})
	for _, p := range plugins {
		pluginMap[p] = []map[string]string{{
			"scope":       "user",
			"installPath": "/p",
			"version":     "1.0",
			"installedAt": "2026-01-01T00:00:00Z",
			"lastUpdated": "2026-01-01T00:00:00Z",
		}}
	}

	data, _ := json.Marshal(map[string]interface{}{
		"version": 2,
		"plugins": pluginMap,
	})
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), data, 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(dir, "settings.json"), []byte("{}"), 0644)
}

func addPluginToMockClaude(t *testing.T, dir, plugin string) {
	t.Helper()
	path := filepath.Join(dir, "plugins", "installed_plugins.json")
	data, _ := os.ReadFile(path)
	var installed map[string]interface{}
	json.Unmarshal(data, &installed)

	plugins := installed["plugins"].(map[string]interface{})
	plugins[plugin] = []map[string]string{{
		"scope":       "user",
		"installPath": "/p",
		"version":     "1.0",
		"installedAt": "2026-01-01T00:00:00Z",
		"lastUpdated": "2026-01-01T00:00:00Z",
	}}

	newData, _ := json.Marshal(installed)
	os.WriteFile(path, newData, 0644)
}
```

Note: Add `"encoding/json"` to imports.

**Step 2: Run integration tests**

Run: `go test ./tests/ -tags=integration -v`
Expected: All tests PASS

**Step 3: Commit**

```bash
git add tests/
git commit -m "test: add end-to-end integration test for full sync workflow"
```

---

## Task 14: Shell Alias Setup and Documentation

**Files:**
- Create: `internal/commands/setup.go`
- Modify: `cmd/claude-sync/main.go` (add `setup` subcommand)

This implements the shell alias setup that ensures same-session freshness.

**Step 1: Implement setup command**

Create `internal/commands/setup.go`:

```go
package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// SetupShellAlias detects the user's shell and prints the alias command.
func SetupShellAlias() string {
	shell := os.Getenv("SHELL")
	alias := `alias claude='claude-sync pull --quiet 2>/dev/null; command claude'`

	var rcFile string
	switch {
	case strings.HasSuffix(shell, "zsh"):
		rcFile = filepath.Join(os.Getenv("HOME"), ".zshrc")
	case strings.HasSuffix(shell, "bash"):
		rcFile = filepath.Join(os.Getenv("HOME"), ".bashrc")
	case strings.HasSuffix(shell, "fish"):
		alias = `alias claude 'claude-sync pull --quiet 2>/dev/null; command claude'`
		rcFile = filepath.Join(os.Getenv("HOME"), ".config", "fish", "config.fish")
	default:
		rcFile = "your shell's rc file"
	}

	return fmt.Sprintf(`To ensure Claude Code always starts with fresh config, add this alias:

  %s

Add it to %s, then restart your shell or run:

  source %s
`, alias, rcFile, rcFile)
}
```

**Step 2: Add setup command to CLI**

Add to `cmd/claude-sync/main.go` init():
```go
rootCmd.AddCommand(setupCmd)
```

Create `cmd/claude-sync/cmd_setup.go`:
```go
package main

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/spf13/cobra"
)

var setupCmd = &cobra.Command{
	Use:   "setup",
	Short: "Show shell alias setup instructions",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Print(commands.SetupShellAlias())
	},
}
```

**Step 3: Commit**

```bash
git add internal/commands/setup.go cmd/claude-sync/cmd_setup.go cmd/claude-sync/main.go
git commit -m "feat: add setup command for shell alias configuration"
```

---

## Task 15: Build, Cross-Compile, and Final Verification

**Files:**
- Create: `Makefile`

**Step 1: Create Makefile**

```makefile
VERSION ?= 0.1.0-dev
LDFLAGS := -ldflags "-X main.version=$(VERSION)"
BINARY := claude-sync

.PHONY: build test test-integration clean install

build:
	go build $(LDFLAGS) -o $(BINARY) ./cmd/claude-sync

test:
	go test ./... -v

test-integration:
	go test ./tests/ -tags=integration -v

install: build
	cp $(BINARY) $(GOPATH)/bin/$(BINARY) 2>/dev/null || cp $(BINARY) /usr/local/bin/$(BINARY)

cross-compile:
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-darwin-arm64 ./cmd/claude-sync
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-darwin-amd64 ./cmd/claude-sync
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-linux-arm64 ./cmd/claude-sync
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o plugin/bin/$(BINARY)-linux-amd64 ./cmd/claude-sync

clean:
	rm -f $(BINARY)
	rm -f plugin/bin/$(BINARY)-*
```

**Step 2: Run full verification**

Run:
```bash
make build
make test
./claude-sync version
./claude-sync --help
./claude-sync status   # should error: not initialized
./claude-sync setup    # should show alias instructions
```

Expected:
- Build succeeds
- All tests pass
- CLI shows correct version and help
- status shows "not initialized" error
- setup shows shell alias instructions

**Step 3: Commit**

```bash
git add Makefile
git commit -m "feat: add Makefile for build, test, and cross-compilation"
```

---

## Summary

### Phase 1 delivers:

| Command | What it does |
|---------|-------------|
| `init` | Scans `~/.claude/` and creates `~/.claude-sync/config.yaml` with git repo |
| `join <url>` | Clones existing config repo, bootstraps Claude Code dir if needed |
| `status` | Shows synced/not-installed/untracked plugins |
| `pull` | Git pulls config, installs missing plugins via `claude plugin install` |
| `push` | Interactive TUI to select local changes, commits and pushes to git |
| `setup` | Shows shell alias for same-session freshness |
| `version` | Shows version |

### Packages:

| Package | Purpose |
|---------|---------|
| `internal/git` | Thin wrapper around git binary |
| `internal/config` | YAML config parsing and serialization |
| `internal/claudecode` | Read/write Claude Code files |
| `internal/sync` | Core diff and reconciliation logic |
| `internal/paths` | Centralized path constants |
| `internal/commands` | Business logic for each command |

### What's deferred to Phase 2:
- Upstream/pinned/forked plugin categorization
- Marketplace version checking
- `/sync apply` command
- Forked plugin storage and local marketplace registration
- `--json` output flag for status command (needed by session-start hook)
