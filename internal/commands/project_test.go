package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProjectTestEnv(t *testing.T) (projectDir, syncDir string) {
	t.Helper()
	projectDir = t.TempDir()
	syncDir = t.TempDir()

	// Create .claude dir in project
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)
	// Create sync dir
	os.MkdirAll(syncDir, 0755)
	return
}

func TestProjectInit_NewProject(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

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
	data, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	result, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		Profile:       "",
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
	assert.Contains(t, string(settings["permissions"]), "Read")
}

func TestProjectInit_ImportExisting(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	// Setup global config with base permissions
	cfg := config.Config{
		Version: "1.0.0",
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit"},
		},
	}
	data, _ := config.MarshalV2(cfg)
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

	result, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
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

func TestProjectInit_NoGlobalConfig(t *testing.T) {
	projectDir := t.TempDir()
	syncDir := t.TempDir()
	os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755)

	_, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"hooks", "permissions"},
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not initialized")
}

func TestProjectInit_GitignoreCreated(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	cfg := config.Config{Version: "1.0.0"}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	_, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"hooks"},
	})
	require.NoError(t, err)

	gitignore, err := os.ReadFile(filepath.Join(projectDir, ".gitignore"))
	require.NoError(t, err)
	assert.Contains(t, string(gitignore), ".claude/.claude-sync.yaml")
}

func TestProjectPush_CapturesNewPermissions(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	// Setup global config with base permissions
	cfg := config.Config{
		Version: "1.0.0",
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit"},
		},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	// Init project first
	_, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"permissions"},
	})
	require.NoError(t, err)

	// Simulate "Always allow" click: add new permission to settings.local.json
	slj, _ := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	var settings map[string]json.RawMessage
	json.Unmarshal(slj, &settings)
	var perms struct {
		Allow []string `json:"allow"`
	}
	json.Unmarshal(settings["permissions"], &perms)
	perms.Allow = append(perms.Allow, "Bash(curl:*)")
	permData, _ := json.Marshal(map[string]any{"allow": perms.Allow})
	settings["permissions"] = permData
	newData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(projectDir, ".claude", "settings.local.json"), newData, 0644)

	// Push should detect the new permission
	result, err := commands.ProjectPush(commands.ProjectPushOptions{
		ProjectDir: projectDir,
		SyncDir:    syncDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, result.NewPermissions)

	// Verify override was saved
	pcfg, _ := project.ReadProjectConfig(projectDir)
	assert.Contains(t, pcfg.Overrides.Permissions.AddAllow, "Bash(curl:*)")
}

func TestProjectPush_NoDrift(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	cfg := config.Config{
		Version: "1.0.0",
		Permissions: config.Permissions{
			Allow: []string{"Read", "Edit"},
		},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	_, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"permissions"},
	})
	require.NoError(t, err)

	// Push with no changes
	result, err := commands.ProjectPush(commands.ProjectPushOptions{
		ProjectDir: projectDir,
		SyncDir:    syncDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 0, result.NewPermissions)
	assert.Equal(t, 0, result.NewHooks)
}
