# Project Initialization Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Extend claude-sync to manage per-project `.claude/settings.local.json` files, projecting hooks and permissions from profiles into project settings to work around Claude Code bug #19487.

**Architecture:** Each project gets a `.claude/.claude-sync.yaml` config that references a profile and stores overrides. Pull resolves base → profile → project overrides and writes managed keys to settings.local.json. Push captures drift (e.g., "Always allow" clicks) back into overrides. Conflict resolution uses YAML-aware 3-way merge with interactive prompts.

**Tech Stack:** Go 1.23, Cobra CLI, go.yaml.in/yaml/v3, charmbracelet TUI, testify

**Design doc:** `docs/2026-02-19-project-init-design.md`

---

## Task 1: Project Config Types

Create the `internal/project` package with types for reading/writing `.claude/.claude-sync.yaml`.

**Files:**
- Create: `internal/project/project.go`
- Create: `internal/project/project_test.go`

**Step 1: Write the failing tests**

```go
// internal/project/project_test.go
package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	yaml := `version: "1.0.0"
profile: work
initialized: "2026-02-19T10:30:00Z"
projected_keys:
  - hooks
  - permissions
overrides:
  permissions:
    add_allow:
      - "mcp__evvy_db__query"
      - "Bash(docker compose:*)"
`
	os.WriteFile(filepath.Join(claudeDir, ".claude-sync.yaml"), []byte(yaml), 0644)

	cfg, err := ReadProjectConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, "work", cfg.Profile)
	assert.Equal(t, []string{"hooks", "permissions"}, cfg.ProjectedKeys)
	assert.Equal(t, []string{"mcp__evvy_db__query", "Bash(docker compose:*)"}, cfg.Overrides.Permissions.AddAllow)
}

func TestReadProjectConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadProjectConfig(dir)
	assert.ErrorIs(t, err, ErrNoProjectConfig)
}

func TestReadProjectConfig_Declined(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	yaml := `version: "1.0.0"
declined: true
`
	os.WriteFile(filepath.Join(claudeDir, ".claude-sync.yaml"), []byte(yaml), 0644)

	cfg, err := ReadProjectConfig(dir)
	require.NoError(t, err)
	assert.True(t, cfg.Declined)
}

func TestWriteProjectConfig(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	cfg := ProjectConfig{
		Version:       "1.0.0",
		Profile:       "work",
		Initialized:   "2026-02-19T10:30:00Z",
		ProjectedKeys: []string{"hooks", "permissions"},
	}

	err := WriteProjectConfig(dir, cfg)
	require.NoError(t, err)

	// Read back and verify
	got, err := ReadProjectConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, cfg.Profile, got.Profile)
	assert.Equal(t, cfg.ProjectedKeys, got.ProjectedKeys)
}

func TestFindProjectRoot(t *testing.T) {
	// Create nested directory with .claude-sync.yaml at root
	root := t.TempDir()
	claudeDir := filepath.Join(root, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, ".claude-sync.yaml"), []byte("version: \"1.0.0\"\nprofile: work\n"), 0644)

	nested := filepath.Join(root, "src", "pkg")
	os.MkdirAll(nested, 0755)

	found, err := FindProjectRoot(nested)
	require.NoError(t, err)
	assert.Equal(t, root, found)
}

func TestFindProjectRoot_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindProjectRoot(dir)
	assert.ErrorIs(t, err, ErrNoProjectConfig)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/albertgwo/Repositories/claude-sync && go test ./internal/project/ -v`
Expected: compilation error — package doesn't exist

**Step 3: Implement the types and functions**

```go
// internal/project/project.go
package project

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"go.yaml.in/yaml/v3"
)

var ErrNoProjectConfig = errors.New("no .claude-sync.yaml found")

const ConfigFileName = ".claude-sync.yaml"

// ProjectConfig represents .claude/.claude-sync.yaml in a project directory.
type ProjectConfig struct {
	Version       string           `yaml:"version"`
	Profile       string           `yaml:"profile,omitempty"`
	Initialized   string           `yaml:"initialized,omitempty"`
	Declined      bool             `yaml:"declined,omitempty"`
	ProjectedKeys []string         `yaml:"projected_keys,omitempty"`
	Overrides     ProjectOverrides `yaml:"overrides,omitempty"`
}

type ProjectOverrides struct {
	Permissions ProjectPermissionOverrides      `yaml:"permissions,omitempty"`
	Hooks       ProjectHookOverrides            `yaml:"hooks,omitempty"`
	ClaudeMD    ProjectClaudeMDOverrides        `yaml:"claude_md,omitempty"`
	MCP         ProjectMCPOverrides             `yaml:"mcp,omitempty"`
}

type ProjectPermissionOverrides struct {
	AddAllow []string `yaml:"add_allow,omitempty"`
	AddDeny  []string `yaml:"add_deny,omitempty"`
}

type ProjectHookOverrides struct {
	Add    map[string]json.RawMessage `yaml:"add,omitempty"`
	Remove []string                   `yaml:"remove,omitempty"`
}

type ProjectClaudeMDOverrides struct {
	Add    []string `yaml:"add,omitempty"`
	Remove []string `yaml:"remove,omitempty"`
}

type ProjectMCPOverrides struct {
	Add    map[string]json.RawMessage `yaml:"add,omitempty"`
	Remove []string                   `yaml:"remove,omitempty"`
}

func configPath(projectDir string) string {
	return filepath.Join(projectDir, ".claude", ConfigFileName)
}

func ReadProjectConfig(projectDir string) (ProjectConfig, error) {
	data, err := os.ReadFile(configPath(projectDir))
	if err != nil {
		if os.IsNotExist(err) {
			return ProjectConfig{}, ErrNoProjectConfig
		}
		return ProjectConfig{}, err
	}
	var cfg ProjectConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return ProjectConfig{}, err
	}
	return cfg, nil
}

func WriteProjectConfig(projectDir string, cfg ProjectConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(projectDir), data, 0644)
}

// FindProjectRoot walks up from dir looking for .claude/.claude-sync.yaml.
func FindProjectRoot(dir string) (string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(configPath(dir)); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", ErrNoProjectConfig
		}
		dir = parent
	}
}
```

**Step 4: Run tests to verify they pass**

Run: `cd /Users/albertgwo/Repositories/claude-sync && go test ./internal/project/ -v`
Expected: all PASS

**Step 5: Commit**

```bash
cd /Users/albertgwo/Repositories/claude-sync
git add internal/project/
git commit -m "feat: add project config types for .claude-sync.yaml"
```

---

## Task 2: Rename `init` → `config create` and `join` → `config join`

Introduce a `config` parent command and move init/join under it, keeping hidden aliases.

**Files:**
- Create: `cmd/claude-sync/cmd_config.go`
- Modify: `cmd/claude-sync/cmd_init.go` — move RunE to config create, add alias
- Modify: `cmd/claude-sync/cmd_join.go` — move RunE to config join, add alias
- Modify: `cmd/claude-sync/main.go` — register `configCmd` instead of initCmd/joinCmd directly

**Step 1: Create cmd_config.go with parent command**

```go
// cmd/claude-sync/cmd_config.go
package main

import "github.com/spf13/cobra"

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage global claude-sync configuration",
}

func init() {
	configCmd.AddCommand(configCreateCmd)
	configCmd.AddCommand(configJoinCmd)
}
```

**Step 2: Create configCreateCmd that wraps existing initCmd logic**

In `cmd_init.go`, rename `initCmd` to `configCreateCmd`:
- Change `Use` from `"init"` to `"create"`
- Keep all existing flags and RunE logic
- Add hidden alias:

```go
var initAliasCmd = &cobra.Command{
	Use:        "init",
	Hidden:     true,
	Deprecated: "Use 'claude-sync config create' instead",
	RunE:       configCreateCmd.RunE,
}
```

**Step 3: Do the same for join → config join**

In `cmd_join.go`, rename `joinCmd` to `configJoinCmd`:
- Change `Use` from `"join"` to `"join"`
- Add hidden alias

**Step 4: Update main.go**

```go
func init() {
	rootCmd.AddCommand(configCmd)       // new parent
	rootCmd.AddCommand(initAliasCmd)    // hidden alias
	rootCmd.AddCommand(joinAliasCmd)    // hidden alias
	// ... rest unchanged
}
```

**Step 5: Verify existing init tests still pass**

Run: `cd /Users/albertgwo/Repositories/claude-sync && go test ./... -v -count=1`
Expected: all PASS

**Step 6: Commit**

```bash
git add cmd/claude-sync/
git commit -m "refactor: rename init/join to config create/config join

Hidden aliases for backward compatibility with deprecation notices."
```

---

## Task 3: Project Init — Settings Import

Add the core project init logic that imports an existing settings.local.json into project overrides.

**Files:**
- Create: `internal/commands/project.go`
- Create: `internal/commands/project_test.go`

**Step 1: Write the failing test**

```go
// internal/commands/project_test.go
package commands

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProjectTestEnv(t *testing.T) (projectDir, claudeDir, syncDir string) {
	t.Helper()
	projectDir = t.TempDir()
	claudeDir = filepath.Join(t.TempDir(), ".claude")
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")

	// Create .claude dir in project
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)

	// Create minimal global config
	os.MkdirAll(syncDir, 0755)
	// Write a minimal config.yaml with hooks and permissions
	// (use config.Marshal to create it properly)
	return
}

func TestProjectInit_NewProject(t *testing.T) {
	projectDir, claudeDir, syncDir := setupProjectTestEnv(t)

	// Setup: write config.yaml with hooks and permissions
	cfg := config.Config{
		Version: "1.0.0",
		Hooks: map[string]json.RawMessage{
			"PreToolUse": json.RawMessage(`[{"matcher":"^Bash$","hooks":[{"type":"command","command":"python3 validator.py"}]}]`),
		},
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit", "Bash(ls *)"},
		},
	}
	data, _ := config.Marshal(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	result, err := ProjectInit(ProjectInitOptions{
		ProjectDir:    projectDir,
		ClaudeDir:     claudeDir,
		SyncDir:       syncDir,
		Profile:       "",  // no profile
		ProjectedKeys: []string{"hooks", "permissions"},
	})
	require.NoError(t, err)
	assert.True(t, result.Created)

	// Verify .claude-sync.yaml was created
	pcfg, err := project.ReadProjectConfig(projectDir)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", pcfg.Version)
	assert.Equal(t, []string{"hooks", "permissions"}, pcfg.ProjectedKeys)

	// Verify settings.local.json was written with hooks and permissions
	slj, err := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	require.NoError(t, err)
	var settings map[string]json.RawMessage
	json.Unmarshal(slj, &settings)
	assert.Contains(t, string(settings["hooks"]), "PreToolUse")
}

func TestProjectInit_ImportExisting(t *testing.T) {
	projectDir, claudeDir, syncDir := setupProjectTestEnv(t)

	// Setup global config with base permissions
	cfg := config.Config{
		Version: "1.0.0",
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit"},
		},
	}
	data, _ := config.Marshal(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	// Setup existing settings.local.json with extra permissions
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Read", "Edit", "mcp__evvy_db__query", "Bash(docker compose:*)"},
		},
		"enabledMcpjsonServers": []string{"evvy-db", "render"},
	}
	ejson, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(projectDir, ".claude", "settings.local.json"), ejson, 0644)

	result, err := ProjectInit(ProjectInitOptions{
		ProjectDir:    projectDir,
		ClaudeDir:     claudeDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"permissions"},
	})
	require.NoError(t, err)

	// Verify project-specific permissions were captured as overrides
	pcfg, _ := project.ReadProjectConfig(projectDir)
	assert.Contains(t, pcfg.Overrides.Permissions.AddAllow, "mcp__evvy_db__query")
	assert.Contains(t, pcfg.Overrides.Permissions.AddAllow, "Bash(docker compose:*)")
	// Base permissions should NOT be in overrides
	assert.NotContains(t, pcfg.Overrides.Permissions.AddAllow, "Read")

	// Verify unmanaged keys preserved in settings.local.json
	slj, _ := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	var settings map[string]json.RawMessage
	json.Unmarshal(slj, &settings)
	assert.NotNil(t, settings["enabledMcpjsonServers"])

	// Verify imported count
	assert.Equal(t, 2, result.ImportedPermissions)
}
```

**Step 2: Run tests to verify they fail**

Run: `cd /Users/albertgwo/Repositories/claude-sync && go test ./internal/commands/ -run TestProject -v`
Expected: compilation error — ProjectInit not defined

**Step 3: Implement ProjectInit**

```go
// internal/commands/project.go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/ruminaider/claude-sync/internal/project"
)

type ProjectInitOptions struct {
	ProjectDir    string
	ClaudeDir     string
	SyncDir       string
	Profile       string
	ProjectedKeys []string
	Yes           bool // non-interactive
}

type ProjectInitResult struct {
	Created             bool
	Profile             string
	ProjectedKeys       []string
	ImportedPermissions int
	ImportedHooks       int
}

func ProjectInit(opts ProjectInitOptions) (*ProjectInitResult, error) {
	// 1. Verify global config exists
	cfgPath := filepath.Join(opts.SyncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("claude-sync not initialized — run 'claude-sync config create' first")
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 2. Resolve profile
	resolved := resolveWithProfile(cfg, opts.SyncDir, opts.Profile)

	// 3. Import existing settings.local.json
	var overrides project.ProjectOverrides
	var importedPerms, importedHooks int
	settingsPath := filepath.Join(opts.ProjectDir, ".claude", "settings.local.json")
	if data, err := os.ReadFile(settingsPath); err == nil {
		var existing map[string]json.RawMessage
		if json.Unmarshal(data, &existing) == nil {
			importedPerms = importPermissionOverrides(&overrides, existing, resolved.permissions)
			importedHooks = importHookOverrides(&overrides, existing, resolved.hooks)
		}
	}

	// 4. Write .claude-sync.yaml
	pcfg := project.ProjectConfig{
		Version:       "1.0.0",
		Profile:       opts.Profile,
		Initialized:   time.Now().UTC().Format(time.RFC3339),
		ProjectedKeys: opts.ProjectedKeys,
		Overrides:     overrides,
	}
	if err := project.WriteProjectConfig(opts.ProjectDir, pcfg); err != nil {
		return nil, fmt.Errorf("failed to write project config: %w", err)
	}

	// 5. Apply projected keys to settings.local.json
	if err := applyProjectSettings(opts.ProjectDir, resolved, pcfg); err != nil {
		return nil, fmt.Errorf("failed to apply project settings: %w", err)
	}

	// 6. Ensure .claude-sync.yaml is in .gitignore
	ensureGitignore(opts.ProjectDir, ".claude/"+project.ConfigFileName)

	return &ProjectInitResult{
		Created:             true,
		Profile:             opts.Profile,
		ProjectedKeys:       opts.ProjectedKeys,
		ImportedPermissions: importedPerms,
		ImportedHooks:       importedHooks,
	}, nil
}

// resolvedConfig holds the fully merged base + profile result.
type resolvedConfig struct {
	hooks       map[string]json.RawMessage
	permissions config.Permissions
	settings    map[string]any
	claudeMD    []string
	mcp         map[string]json.RawMessage
}

func resolveWithProfile(cfg config.Config, syncDir, profileName string) resolvedConfig {
	rc := resolvedConfig{
		hooks:       cfg.Hooks,
		permissions: cfg.Permissions,
		settings:    cfg.Settings,
		claudeMD:    cfg.ClaudeMD.Include,
		mcp:         cfg.MCP,
	}
	if profileName == "" {
		profileName, _ = profiles.ReadActiveProfile(syncDir)
	}
	if profileName != "" {
		if p, err := profiles.ReadProfile(syncDir, profileName); err == nil {
			rc.hooks = profiles.MergeHooks(rc.hooks, p)
			rc.permissions = profiles.MergePermissions(rc.permissions, p)
			rc.settings = profiles.MergeSettings(rc.settings, p)
			rc.claudeMD = profiles.MergeClaudeMD(rc.claudeMD, p)
			rc.mcp = profiles.MergeMCP(rc.mcp, p)
		}
	}
	return rc
}

func importPermissionOverrides(overrides *project.ProjectOverrides, existing map[string]json.RawMessage, basePerms config.Permissions) int {
	permRaw, ok := existing["permissions"]
	if !ok {
		return 0
	}
	var perms struct {
		Allow []string `json:"allow"`
	}
	if json.Unmarshal(permRaw, &perms) != nil {
		return 0
	}

	baseSet := make(map[string]bool, len(basePerms.Allow))
	for _, p := range basePerms.Allow {
		baseSet[p] = true
	}

	var added int
	for _, p := range perms.Allow {
		if !baseSet[p] {
			overrides.Permissions.AddAllow = append(overrides.Permissions.AddAllow, p)
			added++
		}
	}
	return added
}

func importHookOverrides(overrides *project.ProjectOverrides, existing map[string]json.RawMessage, baseHooks map[string]json.RawMessage) int {
	hooksRaw, ok := existing["hooks"]
	if !ok {
		return 0
	}
	var hooks map[string]json.RawMessage
	if json.Unmarshal(hooksRaw, &hooks) != nil {
		return 0
	}

	var added int
	for name, raw := range hooks {
		if _, inBase := baseHooks[name]; !inBase {
			if overrides.Hooks.Add == nil {
				overrides.Hooks.Add = make(map[string]json.RawMessage)
			}
			overrides.Hooks.Add[name] = raw
			added++
		}
	}
	return added
}

// applyProjectSettings writes managed keys to settings.local.json.
func applyProjectSettings(projectDir string, resolved resolvedConfig, pcfg project.ProjectConfig) error {
	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")

	// Read existing settings (preserve unmanaged keys)
	var settings map[string]json.RawMessage
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]json.RawMessage)
	}

	// Apply project overrides on top of resolved config
	finalPerms := resolved.permissions
	finalPerms.Allow = append(finalPerms.Allow, pcfg.Overrides.Permissions.AddAllow...)
	finalPerms.Deny = append(finalPerms.Deny, pcfg.Overrides.Permissions.AddDeny...)

	finalHooks := resolved.hooks
	// Apply hook overrides (add)
	for name, raw := range pcfg.Overrides.Hooks.Add {
		finalHooks[name] = raw
	}

	// Write projected keys
	for _, key := range pcfg.ProjectedKeys {
		switch key {
		case "hooks":
			data, _ := json.Marshal(finalHooks)
			settings["hooks"] = data
		case "permissions":
			p := map[string]any{"allow": finalPerms.Allow}
			if len(finalPerms.Deny) > 0 {
				p["deny"] = finalPerms.Deny
			}
			data, _ := json.Marshal(p)
			settings["permissions"] = data
		}
	}

	// Write back
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, append(data, '\n'), 0644)
}

func ensureGitignore(projectDir, entry string) {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	content, _ := os.ReadFile(gitignorePath)
	if contains(string(content), entry) {
		return
	}
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(entry + "\n")
}

func contains(s, substr string) bool {
	for _, line := range splitLines(s) {
		if line == substr {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		lines = append(lines, s[start:])
	}
	return lines
}
```

**Step 4: Run tests**

Run: `cd /Users/albertgwo/Repositories/claude-sync && go test ./internal/commands/ -run TestProject -v`
Expected: all PASS

**Step 5: Commit**

```bash
git add internal/commands/project.go internal/commands/project_test.go
git commit -m "feat: add ProjectInit with settings import and override capture"
```

---

## Task 4: `project init` CLI Command

Wire ProjectInit to a Cobra command with interactive profile/key pickers.

**Files:**
- Create: `cmd/claude-sync/cmd_project.go`
- Modify: `cmd/claude-sync/main.go` — register projectCmd

**Step 1: Create the project command group and init subcommand**

```go
// cmd/claude-sync/cmd_project.go
package main

import (
	"fmt"
	"os"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/spf13/cobra"
)

var projectCmd = &cobra.Command{
	Use:   "project",
	Short: "Manage per-project claude-sync settings",
}

var projectInitProfile string
var projectInitKeys string
var projectInitYes bool

var projectInitCmd = &cobra.Command{
	Use:   "init [path]",
	Short: "Initialize a project's settings.local.json from a profile",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		projectDir := "."
		if len(args) > 0 {
			projectDir = args[0]
		}
		projectDir, _ = filepath.Abs(projectDir)

		claudeDir := defaultClaudeDir()
		syncDir := defaultSyncDir()

		// Profile selection
		profile := projectInitProfile
		if profile == "" && !projectInitYes {
			// Interactive: list profiles and let user pick
			available, _ := profiles.ListProfiles(syncDir)
			active, _ := profiles.ReadActiveProfile(syncDir)
			if len(available) > 0 {
				// TUI picker or simple prompt
				profile = promptProfileSelection(available, active)
			} else {
				profile = active
			}
		}

		// Key selection
		keys := []string{"hooks", "permissions"}
		if projectInitKeys != "" {
			keys = strings.Split(projectInitKeys, ",")
		}

		result, err := commands.ProjectInit(commands.ProjectInitOptions{
			ProjectDir:    projectDir,
			ClaudeDir:     claudeDir,
			SyncDir:       syncDir,
			Profile:       profile,
			ProjectedKeys: keys,
			Yes:           projectInitYes,
		})
		if err != nil {
			return err
		}

		fmt.Printf("Project initialized at %s\n", projectDir)
		fmt.Printf("  Profile: %s\n", result.Profile)
		fmt.Printf("  Projected keys: %v\n", result.ProjectedKeys)
		if result.ImportedPermissions > 0 {
			fmt.Printf("  Imported %d project-specific permissions\n", result.ImportedPermissions)
		}
		if result.ImportedHooks > 0 {
			fmt.Printf("  Imported %d project-specific hooks\n", result.ImportedHooks)
		}
		return nil
	},
}

func init() {
	projectInitCmd.Flags().StringVar(&projectInitProfile, "profile", "", "Profile to use (skip picker)")
	projectInitCmd.Flags().StringVar(&projectInitKeys, "keys", "", "Comma-separated keys to project (default: hooks,permissions)")
	projectInitCmd.Flags().BoolVar(&projectInitYes, "yes", false, "Non-interactive mode")
	projectCmd.AddCommand(projectInitCmd)
}
```

**Step 2: Register in main.go**

Add `rootCmd.AddCommand(projectCmd)` in main.go's init function.

**Step 3: Manual test**

Run: `cd /Users/albertgwo/Repositories/claude-sync && go build -o /tmp/claude-sync-test ./cmd/claude-sync/`
Run: `/tmp/claude-sync-test project init --help`
Expected: shows usage with --profile, --keys, --yes flags

**Step 4: Commit**

```bash
git add cmd/claude-sync/cmd_project.go cmd/claude-sync/main.go
git commit -m "feat: add 'project init' CLI command with profile selection"
```

---

## Task 5: `project list` and `project remove` Commands

**Files:**
- Modify: `cmd/claude-sync/cmd_project.go` — add list and remove subcommands
- Create: `internal/commands/project_list.go`
- Create: `internal/commands/project_remove.go`
- Create: `internal/commands/project_list_test.go`

**Step 1: Write failing test for ProjectList**

```go
func TestProjectList(t *testing.T) {
	// Create two project dirs with .claude-sync.yaml
	proj1 := t.TempDir()
	proj2 := t.TempDir()
	os.MkdirAll(filepath.Join(proj1, ".claude"), 0755)
	os.MkdirAll(filepath.Join(proj2, ".claude"), 0755)

	project.WriteProjectConfig(proj1, project.ProjectConfig{Version: "1.0.0", Profile: "work"})
	project.WriteProjectConfig(proj2, project.ProjectConfig{Version: "1.0.0", Profile: "personal"})

	results, err := ProjectList([]string{proj1, proj2})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}
```

**Step 2: Implement ProjectList**

```go
// internal/commands/project_list.go
func ProjectList(searchDirs []string) ([]ProjectListEntry, error) {
	var entries []ProjectListEntry
	for _, dir := range searchDirs {
		cfg, err := project.ReadProjectConfig(dir)
		if err != nil {
			continue
		}
		if cfg.Declined {
			continue
		}
		entries = append(entries, ProjectListEntry{
			Path:    dir,
			Profile: cfg.Profile,
		})
	}
	return entries, nil
}
```

**Step 3: Implement ProjectRemove**

```go
// internal/commands/project_remove.go
func ProjectRemove(projectDir string) error {
	configPath := filepath.Join(projectDir, ".claude", project.ConfigFileName)
	return os.Remove(configPath)
}
```

**Step 4: Wire to CLI in cmd_project.go**

Add `projectListCmd` and `projectRemoveCmd` subcommands.

**Step 5: Run tests, commit**

```bash
go test ./internal/commands/ -run "TestProject" -v
git add internal/commands/project_list.go internal/commands/project_remove.go internal/commands/project_list_test.go cmd/claude-sync/cmd_project.go
git commit -m "feat: add 'project list' and 'project remove' commands"
```

---

## Task 6: Pull Extension — Apply Project Settings

Extend PullWithOptions to detect `.claude-sync.yaml` and apply project settings.

**Files:**
- Modify: `internal/commands/pull.go` — add project detection and application after global pull
- Modify: `internal/commands/pull_test.go` — add project pull tests

**Step 1: Write the failing test**

```go
func TestPull_AppliesProjectSettings(t *testing.T) {
	claudeDir, syncDir := setupTestEnv(t)
	projectDir := t.TempDir()
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)

	// Write global config with hooks
	// ...setup config.yaml with PreToolUse hooks...

	// Write project config pointing to base
	project.WriteProjectConfig(projectDir, project.ProjectConfig{
		Version:       "1.0.0",
		Profile:       "",
		ProjectedKeys: []string{"hooks", "permissions"},
		Overrides: project.ProjectOverrides{
			Permissions: project.ProjectPermissionOverrides{
				AddAllow: []string{"mcp__evvy_db__query"},
			},
		},
	})

	result, err := PullWithOptions(PullOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		ProjectDir: projectDir, // NEW field
	})
	require.NoError(t, err)

	// Verify settings.local.json has hooks + permissions with override
	slj, _ := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	var settings map[string]json.RawMessage
	json.Unmarshal(slj, &settings)
	assert.Contains(t, string(settings["hooks"]), "PreToolUse")
	assert.Contains(t, string(settings["permissions"]), "mcp__evvy_db__query")
}
```

**Step 2: Add `ProjectDir` to PullOptions**

```go
type PullOptions struct {
	ClaudeDir         string
	SyncDir           string
	Quiet             bool
	Auto              bool
	MCPTargetResolver func(serverName, suggestedPath string) string
	ProjectDir        string // NEW: if set, apply project settings after global pull
}
```

**Step 3: Add project application at end of PullWithOptions**

After the existing global pull logic completes, add:

```go
// Apply project settings if in a project directory
projectDir := opts.ProjectDir
if projectDir == "" {
	// Auto-detect from CWD
	if cwd, err := os.Getwd(); err == nil {
		projectDir, _ = project.FindProjectRoot(cwd)
	}
}
if projectDir != "" {
	pcfg, err := project.ReadProjectConfig(projectDir)
	if err == nil && !pcfg.Declined {
		resolved := resolveWithProfile(cfg, opts.SyncDir, pcfg.Profile)
		applyProjectSettings(projectDir, resolved, pcfg)
		// Add to result
		result.ProjectSettingsApplied = true
	}
}
```

**Step 4: Run tests, commit**

```bash
go test ./internal/commands/ -run TestPull -v
git add internal/commands/pull.go internal/commands/pull_test.go
git commit -m "feat: extend pull to apply project settings from .claude-sync.yaml"
```

---

## Task 7: Push Extension — Capture Project Drift

Extend push to detect new permissions/hooks in settings.local.json and save to overrides.

**Files:**
- Modify: `internal/commands/push.go`
- Modify: `internal/commands/project.go` — add ProjectPush function
- Modify: `internal/commands/project_test.go`

**Step 1: Write failing test**

```go
func TestProjectPush_CapturesNewPermissions(t *testing.T) {
	projectDir, _, syncDir := setupProjectTestEnv(t)

	// Init project with base permissions
	// ...setup config, project init...

	// Simulate "Always allow" click: add new permission to settings.local.json
	slj, _ := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	var settings map[string]json.RawMessage
	json.Unmarshal(slj, &settings)
	var perms struct{ Allow []string }
	json.Unmarshal(settings["permissions"], &perms)
	perms.Allow = append(perms.Allow, "Bash(curl:*)")
	permData, _ := json.Marshal(map[string]any{"allow": perms.Allow})
	settings["permissions"] = permData
	newData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(projectDir, ".claude", "settings.local.json"), newData, 0644)

	// Push should detect the new permission
	result, err := ProjectPush(ProjectPushOptions{
		ProjectDir: projectDir,
		SyncDir:    syncDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.NewPermissions)

	// Verify override was saved
	pcfg, _ := project.ReadProjectConfig(projectDir)
	assert.Contains(t, pcfg.Overrides.Permissions.AddAllow, "Bash(curl:*)")
}
```

**Step 2: Implement ProjectPush**

```go
type ProjectPushOptions struct {
	ProjectDir string
	SyncDir    string
	Auto       bool
}

type ProjectPushResult struct {
	NewPermissions int
	NewHooks       int
}

func ProjectPush(opts ProjectPushOptions) (*ProjectPushResult, error) {
	pcfg, err := project.ReadProjectConfig(opts.ProjectDir)
	if err != nil {
		return nil, err
	}

	cfgData, _ := os.ReadFile(filepath.Join(opts.SyncDir, "config.yaml"))
	cfg, _ := config.Parse(cfgData)
	resolved := resolveWithProfile(cfg, opts.SyncDir, pcfg.Profile)

	// Read current settings.local.json
	settingsPath := filepath.Join(opts.ProjectDir, ".claude", "settings.local.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return nil, err
	}
	var settings map[string]json.RawMessage
	json.Unmarshal(data, &settings)

	// Diff permissions
	newPerms := diffPermissions(settings, resolved.permissions, pcfg.Overrides.Permissions.AddAllow)
	newHooks := diffHooks(settings, resolved.hooks, pcfg.Overrides.Hooks.Add)

	// Save to overrides
	if len(newPerms) > 0 {
		pcfg.Overrides.Permissions.AddAllow = append(pcfg.Overrides.Permissions.AddAllow, newPerms...)
		// dedup
	}
	// ... similar for hooks

	project.WriteProjectConfig(opts.ProjectDir, pcfg)

	return &ProjectPushResult{
		NewPermissions: len(newPerms),
		NewHooks:       len(newHooks),
	}, nil
}
```

**Step 3: Run tests, commit**

```bash
go test ./internal/commands/ -run TestProjectPush -v
git add internal/commands/push.go internal/commands/project.go internal/commands/project_test.go
git commit -m "feat: extend push to capture project settings drift"
```

---

## Task 8: Per-Project CLAUDE.md Fragments

Extend project overrides to add/remove CLAUDE.md fragments.

**Files:**
- Modify: `internal/commands/project.go` — extend applyProjectSettings to handle claude_md
- Modify: `internal/commands/pull.go` — assemble project CLAUDE.md
- Create: `internal/commands/project_claudemd_test.go`

**Step 1: Write failing test**

```go
func TestProjectPull_AssemblesClaudeMD(t *testing.T) {
	// Setup: base config has fragments ["coding-standards"]
	// Profile adds ["testing-patterns"]
	// Project overrides add ["evvy-conventions"]
	// Verify assembled CLAUDE.md at project/.claude/CLAUDE.md includes all three
}
```

**Step 2: Extend resolvedConfig to include CLAUDE.md assembly**

During pull, after resolving base → profile → project overrides for claude_md:
1. Read fragment files from `~/.claude-sync/claude-md/`
2. Assemble into a single CLAUDE.md
3. Write to `<project>/.claude/CLAUDE.md` (project-scoped, not global)

**Step 3: Run tests, commit**

```bash
go test ./internal/commands/ -run TestProjectPull_Assembles -v
git commit -m "feat: per-project CLAUDE.md fragment assembly"
```

---

## Task 9: Per-Project MCP Server Management

Extend project overrides to add/remove MCP servers.

**Files:**
- Modify: `internal/commands/project.go` — extend applyProjectSettings for MCP
- Modify: `internal/commands/project_test.go`

**Step 1: Write failing test**

```go
func TestProjectInit_MCPOverrides(t *testing.T) {
	// Setup project with MCP overrides
	// Verify .mcp.json written to project dir with project-specific servers
	// Verify global .mcp.json not affected
}
```

**Step 2: Implement**

During applyProjectSettings, if "mcp" is in projected_keys:
1. Resolve base → profile → project MCP overrides
2. Write to `<project>/.mcp.json` using existing `claudecode.WriteMCPConfigFile()`

This leverages the existing project MCP infrastructure (`MCPServerMeta.SourceProject`).

**Step 3: Run tests, commit**

```bash
git commit -m "feat: per-project MCP server management via overrides"
```

---

## Task 10: SessionStart Auto-Detection

Extend the SessionStart hook to detect uninitialized projects and prompt for init.

**Files:**
- Modify: `plugin/hooks/session-start.sh` — add project detection logic
- Modify: `internal/commands/pull.go` — add project detection output

**Step 1: Extend pull --auto to detect uninitialized projects**

When `claude-sync pull --auto` runs and detects:
- CWD has `.claude/settings.local.json` but NO `.claude/.claude-sync.yaml`
- Print a notification message suggesting initialization
- If `--auto` mode: just notify, don't prompt interactively

```bash
# In session-start.sh or in pull output:
echo "⚠ This project has settings.local.json but isn't managed by claude-sync."
echo "  Run 'claude-sync project init' to sync hooks and permissions."
```

**Step 2: Add decline handling**

If user runs `claude-sync project init` and declines (or `--decline` flag), write:
```yaml
version: "1.0.0"
declined: true
```

Future pulls skip this project.

**Step 3: Test and commit**

```bash
git commit -m "feat: SessionStart detection for uninitialized projects"
```

---

## Task 11: YAML-Aware 3-Way Merge

Implement the merge engine for handling config conflicts.

**Files:**
- Create: `internal/merge/merge.go`
- Create: `internal/merge/merge_test.go`

**Step 1: Write failing tests for each merge scenario**

```go
func TestMerge_BothAddPermissions_Union(t *testing.T) {
	base := config.Permissions{Allow: []string{"Read"}}
	local := config.Permissions{Allow: []string{"Read", "Edit"}}
	remote := config.Permissions{Allow: []string{"Read", "Bash(ls *)"}}

	result, conflict := MergePermissions(base, local, remote)
	assert.False(t, conflict)
	assert.ElementsMatch(t, []string{"Read", "Edit", "Bash(ls *)"}, result.Allow)
}

func TestMerge_BothModifySameSetting_Conflict(t *testing.T) {
	base := map[string]any{"defaultMode": "default"}
	local := map[string]any{"defaultMode": "plan"}
	remote := map[string]any{"defaultMode": "acceptEdits"}

	_, conflicts := MergeSettings(base, local, remote)
	assert.Len(t, conflicts, 1)
	assert.Equal(t, "defaultMode", conflicts[0].Key)
}

func TestMerge_OneAddsOneRemoves_Conflict(t *testing.T) {
	base := config.Permissions{Allow: []string{"Read", "Edit"}}
	local := config.Permissions{Allow: []string{"Read", "Edit", "Write"}}
	remote := config.Permissions{Allow: []string{"Read"}} // removed Edit

	_, conflict := MergePermissions(base, local, remote)
	assert.True(t, conflict)
}
```

**Step 2: Implement merge functions**

```go
// internal/merge/merge.go
package merge

type ConflictItem struct {
	Key        string
	LocalValue any
	RemoteValue any
}

func MergePermissions(base, local, remote config.Permissions) (config.Permissions, bool) {
	// Compute diffs
	localAdded := setDiff(local.Allow, base.Allow)
	localRemoved := setDiff(base.Allow, local.Allow)
	remoteAdded := setDiff(remote.Allow, base.Allow)
	remoteRemoved := setDiff(base.Allow, remote.Allow)

	// Conflict: one adds what other removes (or vice versa)
	if hasOverlap(localAdded, remoteRemoved) || hasOverlap(remoteAdded, localRemoved) {
		return config.Permissions{}, true
	}

	// Union: base + all additions - all removals
	result := setUnion(base.Allow, localAdded, remoteAdded)
	result = setSubtract(result, localRemoved, remoteRemoved)
	return config.Permissions{Allow: result}, false
}
```

**Step 3: Run tests, commit**

```bash
go test ./internal/merge/ -v
git commit -m "feat: YAML-aware 3-way merge for permissions, hooks, settings"
```

---

## Task 12: Interactive Conflict Resolution

Wire the merge engine into pull with interactive prompts.

**Files:**
- Create: `internal/commands/conflicts.go`
- Modify: `internal/commands/pull.go` — integrate conflict detection
- Create: `cmd/claude-sync/cmd_conflicts.go`

**Step 1: Implement conflict storage**

```go
type PendingConflict struct {
	Timestamp  string
	Key        string
	LocalValue json.RawMessage
	RemoteValue json.RawMessage
}

func SaveConflict(syncDir string, conflict PendingConflict) error {
	// Write to ~/.claude-sync/conflicts/<timestamp>.yaml
}

func HasPendingConflicts(syncDir string) bool {
	// Check if conflicts/ dir has files
}
```

**Step 2: Integrate into pull**

After git pull, if rebase fails:
1. Attempt YAML-aware auto-merge
2. If auto-merge succeeds → apply merged config
3. If auto-merge fails → interactive prompt per conflict
4. "Defer" choices saved to conflicts/

**Step 3: Block push on pending conflicts**

In PushApply, check `HasPendingConflicts()` before proceeding.

**Step 4: Add `claude-sync conflicts` command**

- `claude-sync conflicts` — list pending
- `claude-sync conflicts resolve` — interactive resolution
- `claude-sync conflicts discard` — discard local changes

**Step 5: Run tests, commit**

```bash
go test ./internal/commands/ -run TestConflict -v
git commit -m "feat: interactive conflict resolution with push blocking"
```

---

## Task 13: Integration Test — Full Lifecycle

End-to-end test of the complete project lifecycle.

**Files:**
- Create: `internal/commands/project_integration_test.go`

**Step 1: Write integration test**

```go
func TestProjectLifecycle(t *testing.T) {
	// 1. config create — initialize global config with hooks + permissions
	// 2. project init — initialize project, import existing permissions
	// 3. pull — verify settings.local.json has hooks + permissions + overrides
	// 4. Simulate "Always allow" click — add permission to settings.local.json
	// 5. push — verify new permission captured in overrides
	// 6. pull again — verify settings.local.json regenerated with all permissions
	// 7. project remove — verify .claude-sync.yaml deleted
	// 8. pull — verify no project settings applied
}
```

**Step 2: Run test**

Run: `cd /Users/albertgwo/Repositories/claude-sync && go test ./internal/commands/ -run TestProjectLifecycle -v -count=1`
Expected: PASS

**Step 3: Commit**

```bash
git commit -m "test: add full project lifecycle integration test"
```

---

## Task 14: Update README and Help Text

**Files:**
- Modify: `README.md` — add project management section
- Modify: all command `Short`/`Long` descriptions as needed

**Step 1: Add project section to README**

Document:
- `claude-sync project init [path]` with examples
- `claude-sync project list`
- `claude-sync project remove [path]`
- The settings.local.json ownership model
- How "Always allow" clicks are captured on push

**Step 2: Commit**

```bash
git commit -m "docs: add project initialization documentation"
```
