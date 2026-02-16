package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
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
