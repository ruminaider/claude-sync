package commands_test

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAutoCommitEnv creates a claudeDir and syncDir with a config.yaml and git repo.
func setupAutoCommitEnv(t *testing.T) (claudeDir, syncDir string) {
	t.Helper()
	claudeDir = t.TempDir()
	syncDir = filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	// Create plugin dir so Init doesn't complain.
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"),
		[]byte(`{"version": 2, "plugins": {}}`), 0644)

	// Create a minimal config.
	cfg := config.ConfigV2{
		Version:  "1.0.0",
		Settings: map[string]any{"theme": "dark"},
	}
	cfgData, err := config.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644))

	// Initialize git repo.
	cmd := exec.Command("git", "init", syncDir)
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "config", "user.name", "Test")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "add", ".")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "commit", "-m", "Initial config")
	require.NoError(t, cmd.Run())

	return claudeDir, syncDir
}

func TestAutoCommit_NoChanges(t *testing.T) {
	claudeDir, syncDir := setupAutoCommitEnv(t)

	// Write settings.json matching the config.
	settings := map[string]json.RawMessage{
		"theme": json.RawMessage(`"dark"`),
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.False(t, result.Changed)
}

func TestAutoCommit_SettingsChanged(t *testing.T) {
	claudeDir, syncDir := setupAutoCommitEnv(t)

	// Write settings.json with a different value for "theme".
	settings := map[string]json.RawMessage{
		"theme": json.RawMessage(`"light"`),
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "update setting theme")
	assert.Contains(t, result.FilesChanged, "config.yaml")

	// Verify config was updated.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, "light", cfg.Settings["theme"])
}

func TestAutoCommit_ClaudeMDChanged(t *testing.T) {
	claudeDir, syncDir := setupAutoCommitEnv(t)

	// Create a CLAUDE.md with a section and import it into sync dir.
	claudeMD := "## Coding Standards\nUse gofmt for all Go code.\n"
	_, err := claudemd.ImportClaudeMD(syncDir, claudeMD)
	require.NoError(t, err)

	// Update config with the fragment include.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	cfg.ClaudeMD.Include = []string{"coding-standards"}
	newCfgData, err := config.Marshal(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), newCfgData, 0644)

	// Commit the fragment files.
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "Add claude-md").Run()

	// Now write an updated CLAUDE.md locally.
	updatedClaudeMD := "## Coding Standards\nUse gofmt and golint for all Go code.\n"
	os.WriteFile(filepath.Join(claudeDir, "CLAUDE.md"), []byte(updatedClaudeMD), 0644)

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "update coding-standards")
	assert.Contains(t, result.FilesChanged, "claude-md")
}

func TestAutoCommit_MCPChanged(t *testing.T) {
	claudeDir, syncDir := setupAutoCommitEnv(t)

	// Add MCP to config.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	cfg.MCP = map[string]json.RawMessage{
		"old-server": json.RawMessage(`{"command":"old"}`),
	}
	newCfgData, err := config.Marshal(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), newCfgData, 0644)
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "Add MCP").Run()

	// Write .mcp.json with a different server.
	mcpData := `{"mcpServers": {"new-server": {"command": "new"}}}`
	os.WriteFile(filepath.Join(claudeDir, ".mcp.json"), []byte(mcpData), 0644)

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "update MCP servers")
}

func TestAutoCommit_NonExistentSyncDir(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), "nonexistent")

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.False(t, result.Changed)
}

// setupProfileAwareEnv extends setupAutoCommitEnv with a project directory
// that has a .claude/.claude-sync.yaml pointing at a profile, and a
// profiles/<name>.yaml in the sync dir.
func setupProfileAwareEnv(t *testing.T, profileName string) (claudeDir, syncDir, projectDir string) {
	t.Helper()
	claudeDir, syncDir = setupAutoCommitEnv(t)

	// Create project directory with .claude/.claude-sync.yaml.
	projectDir = t.TempDir()
	claudeSyncDir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeSyncDir, 0755))
	projCfg := fmt.Sprintf("version: \"1\"\nprofile: %s\n", profileName)
	require.NoError(t, os.WriteFile(
		filepath.Join(claudeSyncDir, ".claude-sync.yaml"),
		[]byte(projCfg), 0644,
	))

	// Create an empty profile in the sync dir.
	profilesDir := filepath.Join(syncDir, "profiles")
	require.NoError(t, os.MkdirAll(profilesDir, 0755))
	require.NoError(t, os.WriteFile(
		filepath.Join(profilesDir, profileName+".yaml"),
		[]byte(""), 0644,
	))

	// Commit the profile.
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "Add profile").Run()

	return claudeDir, syncDir, projectDir
}

func TestAutoCommitWithContext_ProfileAware_SettingsToProfile(t *testing.T) {
	claudeDir, syncDir, projectDir := setupProfileAwareEnv(t, "evvy")

	// Write settings.json with a different value for "theme".
	settings := map[string]json.RawMessage{
		"theme": json.RawMessage(`"light"`),
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	result, err := commands.AutoCommitWithContext(commands.AutoCommitOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "auto(evvy):")
	assert.Contains(t, result.CommitMessage, "update setting theme")

	// Verify the profile was updated (not config.yaml).
	profile, err := profiles.ReadProfile(syncDir, "evvy")
	require.NoError(t, err)
	assert.Equal(t, "light", profile.Settings["theme"])

	// Verify config.yaml was NOT changed (theme should still be "dark").
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, "dark", cfg.Settings["theme"])
}

func TestAutoCommitWithContext_ProfileAware_MCPToProfile(t *testing.T) {
	claudeDir, syncDir, projectDir := setupProfileAwareEnv(t, "evvy")

	// Add a base MCP server to config.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	cfg.MCP = map[string]json.RawMessage{
		"base-server": json.RawMessage(`{"command":"base"}`),
	}
	newCfgData, err := config.Marshal(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), newCfgData, 0644)
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "Add base MCP").Run()

	// Write .mcp.json with the base server plus a new project-specific one.
	mcpData := `{"mcpServers": {"base-server": {"command":"base"}, "evvy-db": {"command":"evvy-db-cmd"}}}`
	os.WriteFile(filepath.Join(claudeDir, ".mcp.json"), []byte(mcpData), 0644)

	result, err := commands.AutoCommitWithContext(commands.AutoCommitOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "auto(evvy):")
	assert.Contains(t, result.CommitMessage, "update MCP servers")

	// Verify the new server went to the profile.
	profile, err := profiles.ReadProfile(syncDir, "evvy")
	require.NoError(t, err)
	assert.Contains(t, profile.MCP.Add, "evvy-db")

	// Verify config.yaml's MCP was NOT changed.
	cfgData, err = os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err = config.Parse(cfgData)
	require.NoError(t, err)
	_, hasEvvyDB := cfg.MCP["evvy-db"]
	assert.False(t, hasEvvyDB, "evvy-db should not be in base config")
}

func TestAutoCommitWithContext_NoProject_FallsBackToBase(t *testing.T) {
	claudeDir, syncDir := setupAutoCommitEnv(t)

	// Write settings.json with a different value.
	settings := map[string]json.RawMessage{
		"theme": json.RawMessage(`"light"`),
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	// Use a non-project directory (no .claude/.claude-sync.yaml).
	result, err := commands.AutoCommitWithContext(commands.AutoCommitOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		ProjectDir: t.TempDir(), // temp dir has no project config
	})
	require.NoError(t, err)
	assert.True(t, result.Changed)
	// Should use base commit format (no profile prefix).
	assert.Contains(t, result.CommitMessage, "auto: ")
	assert.NotContains(t, result.CommitMessage, "auto(")

	// Verify config.yaml was updated.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, "light", cfg.Settings["theme"])
}

func TestAutoCommitWithContext_BaseFlag_ForcesBase(t *testing.T) {
	claudeDir, syncDir, projectDir := setupProfileAwareEnv(t, "evvy")

	// Write settings.json with a different value.
	settings := map[string]json.RawMessage{
		"theme": json.RawMessage(`"light"`),
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	// ForceBase should write to config.yaml even though we're in a profiled project.
	result, err := commands.AutoCommitWithContext(commands.AutoCommitOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		ProjectDir: projectDir,
		ForceBase:  true,
	})
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "auto: ")
	assert.NotContains(t, result.CommitMessage, "auto(evvy)")

	// Verify config.yaml was updated (not the profile).
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Equal(t, "light", cfg.Settings["theme"])
}

func TestAutoCommit_MCPStripsSecrets(t *testing.T) {
	claudeDir, syncDir := setupAutoCommitEnv(t)

	// Add MCP to config (without secrets).
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	cfg.MCP = map[string]json.RawMessage{
		"old-server": json.RawMessage(`{"command":"old"}`),
	}
	newCfgData, err := config.Marshal(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), newCfgData, 0644)
	exec.Command("git", "-C", syncDir, "add", ".").Run()
	exec.Command("git", "-C", syncDir, "commit", "-m", "Add MCP").Run()

	// Write .mcp.json with a hardcoded secret.
	mcpData := `{"mcpServers": {"slack": {"command": "npx", "env": {"SLACK_TOKEN": "xoxb-secret-token-value", "APP_NAME": "my-app"}}}}`
	os.WriteFile(filepath.Join(claudeDir, ".mcp.json"), []byte(mcpData), 0644)

	result, err := commands.AutoCommit(claudeDir, syncDir)
	require.NoError(t, err)
	assert.True(t, result.Changed)

	// Read back and verify secret was replaced.
	cfgData, err = os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err = config.Parse(cfgData)
	require.NoError(t, err)

	var serverCfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(cfg.MCP["slack"], &serverCfg))
	assert.Equal(t, "${SLACK_TOKEN}", serverCfg.Env["SLACK_TOKEN"], "secret should be replaced")
	assert.Equal(t, "my-app", serverCfg.Env["APP_NAME"], "non-secret should be untouched")
}

func TestAutoCommitWithContext_ProfileMCPStripsSecrets(t *testing.T) {
	claudeDir, syncDir, projectDir := setupProfileAwareEnv(t, "evvy")

	// Write .mcp.json with a hardcoded secret.
	mcpData := `{"mcpServers": {"clew": {"command": "clew", "env": {"VOYAGE_API_KEY": "pa-SomeSecretKey123", "QDRANT_URL": "http://localhost:6333"}}}}`
	os.WriteFile(filepath.Join(claudeDir, ".mcp.json"), []byte(mcpData), 0644)

	result, err := commands.AutoCommitWithContext(commands.AutoCommitOptions{
		ClaudeDir:  claudeDir,
		SyncDir:    syncDir,
		ProjectDir: projectDir,
	})
	require.NoError(t, err)
	assert.True(t, result.Changed)

	// Read the profile and verify the secret was replaced.
	profile, err := profiles.ReadProfile(syncDir, "evvy")
	require.NoError(t, err)
	assert.Contains(t, profile.MCP.Add, "clew")

	var serverCfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(profile.MCP.Add["clew"], &serverCfg))
	assert.Equal(t, "${VOYAGE_API_KEY}", serverCfg.Env["VOYAGE_API_KEY"], "secret should be replaced")
	assert.Equal(t, "http://localhost:6333", serverCfg.Env["QDRANT_URL"], "non-secret should be untouched")
}

func TestAutoCommitWithContext_BackwardCompatible(t *testing.T) {
	claudeDir, syncDir := setupAutoCommitEnv(t)

	// Write settings.json with a different value.
	settings := map[string]json.RawMessage{
		"theme": json.RawMessage(`"light"`),
	}
	settingsData, _ := json.MarshalIndent(settings, "", "  ")
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), settingsData, 0644)

	// Empty ProjectDir should behave identically to AutoCommit().
	result, err := commands.AutoCommitWithContext(commands.AutoCommitOptions{
		ClaudeDir: claudeDir,
		SyncDir:   syncDir,
	})
	require.NoError(t, err)
	assert.True(t, result.Changed)
	assert.Contains(t, result.CommitMessage, "auto: update setting theme")
}
