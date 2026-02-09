package commands_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupRemoteRepo creates a bare git repo with a config.yaml containing the given plugins.
func setupRemoteRepo(t *testing.T, pluginKeys []string) string {
	t.Helper()
	remote := t.TempDir()
	exec.Command("git", "init", remote).Run()
	exec.Command("git", "-C", remote, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", remote, "config", "user.name", "Test").Run()

	cfg := config.Config{
		Version:  "2.0.0",
		Upstream: pluginKeys,
		Pinned:   map[string]string{},
	}
	cfgData, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(remote, "config.yaml"), cfgData, 0644)
	exec.Command("git", "-C", remote, "add", ".").Run()
	exec.Command("git", "-C", remote, "commit", "-m", "init").Run()
	return remote
}

// setupLocalClaude creates a Claude Code directory with installed plugins.
func setupLocalClaude(t *testing.T, pluginKeys []string) string {
	t.Helper()
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	os.MkdirAll(pluginDir, 0755)

	pluginsMap := make(map[string][]claudecode.PluginInstallation)
	for _, p := range pluginKeys {
		pluginsMap[p] = []claudecode.PluginInstallation{
			{
				Scope:       "user",
				InstallPath: "/mock/" + p,
				Version:     "1.0.0",
				InstalledAt: "2026-01-01T00:00:00Z",
				LastUpdated: "2026-01-01T00:00:00Z",
			},
		}
	}

	installed := claudecode.InstalledPlugins{
		Version: 2,
		Plugins: pluginsMap,
	}
	data, _ := json.MarshalIndent(installed, "", "  ")
	os.WriteFile(filepath.Join(pluginDir, "installed_plugins.json"), data, 0644)
	os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte("{}"), 0644)
	os.WriteFile(filepath.Join(claudeDir, "settings.json"), []byte("{}"), 0644)

	return claudeDir
}

func TestJoin(t *testing.T) {
	remote := t.TempDir()
	exec.Command("git", "init", remote).Run()
	exec.Command("git", "-C", remote, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", remote, "config", "user.name", "Test").Run()

	cfgContent := "version: \"1.0.0\"\nplugins:\n  - context7@claude-plugins-official\n"
	os.WriteFile(filepath.Join(remote, "config.yaml"), []byte(cfgContent), 0644)
	exec.Command("git", "-C", remote, "add", ".").Run()
	exec.Command("git", "-C", remote, "commit", "-m", "init").Run()

	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	result, err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)
	require.NotNil(t, result)

	_, err = os.Stat(filepath.Join(syncDir, "config.yaml"))
	assert.NoError(t, err)
}

func TestJoin_AlreadyExists(t *testing.T) {
	syncDir := t.TempDir()
	_, err := commands.Join("http://example.com/repo.git", "/tmp/claude", syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already")
}

func TestJoin_BootstrapsClaudeDir(t *testing.T) {
	remote := t.TempDir()
	exec.Command("git", "init", remote).Run()
	exec.Command("git", "-C", remote, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", remote, "config", "user.name", "Test").Run()
	os.WriteFile(filepath.Join(remote, "config.yaml"), []byte("version: \"1.0.0\"\nplugins: []\n"), 0644)
	exec.Command("git", "-C", remote, "add", ".").Run()
	exec.Command("git", "-C", remote, "commit", "-m", "init").Run()

	claudeDir := filepath.Join(t.TempDir(), ".claude")
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	result, err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)
	require.NotNil(t, result)

	// Claude dir should have been bootstrapped
	_, err = os.Stat(filepath.Join(claudeDir, "plugins", "installed_plugins.json"))
	assert.NoError(t, err)
}

func TestJoin_DetectsLocalOnlyPlugins(t *testing.T) {
	// Remote config has 2 plugins.
	remote := setupRemoteRepo(t, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	})

	// Local machine has 3 plugins (extra one not in config).
	claudeDir := setupLocalClaude(t, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
		"local-tool@local-custom-plugins",
	})

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	result, err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	assert.Len(t, result.LocalOnly, 1)
	assert.Equal(t, "local-tool@local-custom-plugins", result.LocalOnly[0].Key)
}

func TestJoin_NoLocalOnlyPlugins(t *testing.T) {
	remote := setupRemoteRepo(t, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	})

	claudeDir := setupLocalClaude(t, []string{
		"context7@claude-plugins-official",
		"beads@beads-marketplace",
	})

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	result, err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	assert.Empty(t, result.LocalOnly)
}

func TestJoin_ExposesConfigCategories(t *testing.T) {
	// Create a remote repo with settings and hooks in config.yaml.
	remote := t.TempDir()
	exec.Command("git", "init", remote).Run()
	exec.Command("git", "-C", remote, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", remote, "config", "user.name", "Test").Run()

	cfg := config.Config{
		Version:  "2.0.0",
		Upstream: []string{"context7@claude-plugins-official"},
		Pinned:   map[string]string{},
		Settings: map[string]any{"model": "opus"},
		Hooks:    map[string]string{"PreCompact": "bd prime", "SessionStart": "pull"},
	}
	cfgData, err := config.Marshal(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(remote, "config.yaml"), cfgData, 0644)
	exec.Command("git", "-C", remote, "add", ".").Run()
	exec.Command("git", "-C", remote, "commit", "-m", "init").Run()

	claudeDir := setupLocalClaude(t, []string{})
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	result, err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	assert.True(t, result.HasSettings)
	assert.True(t, result.HasHooks)
	assert.Equal(t, []string{"model"}, result.SettingsKeys)
	assert.Equal(t, []string{"PreCompact", "SessionStart"}, result.HookNames)
}

func TestJoin_NoSettingsOrHooks(t *testing.T) {
	remote := setupRemoteRepo(t, []string{"context7@claude-plugins-official"})
	claudeDir := setupLocalClaude(t, []string{})
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	result, err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	assert.False(t, result.HasSettings)
	assert.False(t, result.HasHooks)
	assert.Empty(t, result.SettingsKeys)
	assert.Empty(t, result.HookNames)
}

func TestJoin_EmptyLocalInstallation(t *testing.T) {
	remote := setupRemoteRepo(t, []string{
		"context7@claude-plugins-official",
	})

	// Fresh machine with no plugins installed.
	claudeDir := setupLocalClaude(t, []string{})

	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	result, err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	assert.Empty(t, result.LocalOnly)
}
