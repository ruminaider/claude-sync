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

func TestProjectLifecycle(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	// --- Step 1: Create global config with hooks + permissions ---
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

	// Pre-existing permissions in settings.local.json (simulates manual setup)
	existing := map[string]any{
		"permissions": map[string]any{
			"allow": []string{"Read", "Edit", "Bash(ls *)", "mcp__evvy_db__query"},
		},
		"enabledMcpjsonServers": []string{"evvy-db"},
	}
	ejson, _ := json.MarshalIndent(existing, "", "  ")
	os.WriteFile(filepath.Join(projectDir, ".claude", "settings.local.json"), ejson, 0644)

	// --- Step 2: Project init — import existing permissions ---
	initResult, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		Profile:       "",
		ProjectedKeys: []string{"hooks", "permissions"},
	})
	require.NoError(t, err)
	assert.True(t, initResult.Created)
	assert.Equal(t, 1, initResult.ImportedPermissions) // mcp__evvy_db__query is the extra one

	// Verify .claude-sync.yaml was created
	pcfg, err := project.ReadProjectConfig(projectDir)
	require.NoError(t, err)
	assert.Contains(t, pcfg.Overrides.Permissions.AddAllow, "mcp__evvy_db__query")
	assert.Equal(t, []string{"hooks", "permissions"}, pcfg.ProjectedKeys)

	// --- Step 3: Verify settings.local.json has hooks + permissions + overrides ---
	slj, err := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	require.NoError(t, err)
	var settings map[string]json.RawMessage
	json.Unmarshal(slj, &settings)
	// Hooks present
	assert.Contains(t, string(settings["hooks"]), "PreToolUse")
	// Base permissions + override present
	assert.Contains(t, string(settings["permissions"]), "Read")
	assert.Contains(t, string(settings["permissions"]), "mcp__evvy_db__query")
	// Unmanaged key preserved
	assert.NotNil(t, settings["enabledMcpjsonServers"])

	// --- Step 4: Simulate "Always allow" click ---
	var perms struct {
		Allow []string `json:"allow"`
	}
	json.Unmarshal(settings["permissions"], &perms)
	perms.Allow = append(perms.Allow, "Bash(docker compose:*)")
	permData, _ := json.Marshal(map[string]any{"allow": perms.Allow})
	settings["permissions"] = permData
	newData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(projectDir, ".claude", "settings.local.json"), newData, 0644)

	// --- Step 5: Push — capture new permission ---
	pushResult, err := commands.ProjectPush(commands.ProjectPushOptions{
		ProjectDir: projectDir,
		SyncDir:    syncDir,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, pushResult.NewPermissions)

	pcfg2, err := project.ReadProjectConfig(projectDir)
	require.NoError(t, err)
	assert.Contains(t, pcfg2.Overrides.Permissions.AddAllow, "Bash(docker compose:*)")

	// --- Step 6: Re-apply settings (simulates pull) ---
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	parsedCfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	resolved := commands.ResolveWithProfile(parsedCfg, syncDir, "")

	err = commands.ApplyProjectSettings(projectDir, resolved, pcfg2, syncDir)
	require.NoError(t, err)

	// Re-read settings.local.json
	slj2, err := os.ReadFile(filepath.Join(projectDir, ".claude", "settings.local.json"))
	require.NoError(t, err)
	var settings2 map[string]json.RawMessage
	json.Unmarshal(slj2, &settings2)
	// All permissions present
	assert.Contains(t, string(settings2["permissions"]), "Read")
	assert.Contains(t, string(settings2["permissions"]), "mcp__evvy_db__query")
	assert.Contains(t, string(settings2["permissions"]), "Bash(docker compose:*)")
	assert.Contains(t, string(settings2["permissions"]), "Bash(ls *)")

	// --- Step 7: Project remove ---
	err = commands.ProjectRemove(projectDir)
	require.NoError(t, err)
	_, err = project.ReadProjectConfig(projectDir)
	assert.ErrorIs(t, err, project.ErrNoProjectConfig)

	// --- Step 8: Settings.local.json still exists but no more managed ---
	_, err = os.Stat(filepath.Join(projectDir, ".claude", "settings.local.json"))
	assert.NoError(t, err) // file still exists
}
