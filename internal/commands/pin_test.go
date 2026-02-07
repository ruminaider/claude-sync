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

// setupV2TestEnv creates a v2 config with upstream plugins in a git repo.
func setupV2TestEnv(t *testing.T) string {
	t.Helper()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	cfg := config.ConfigV2{
		Version: "2.0.0",
		Upstream: []string{
			"context7@claude-plugins-official",
			"beads@beads-marketplace",
		},
		Pinned: map[string]string{},
		Forked: []string{},
	}

	cfgData, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644))

	// Initialize a git repo and make an initial commit.
	cmd := exec.Command("git", "init", syncDir)
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "-C", syncDir, "config", "user.email", "test@test.com")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "config", "user.name", "Test")
	require.NoError(t, cmd.Run())

	cmd = exec.Command("git", "-C", syncDir, "add", ".")
	require.NoError(t, cmd.Run())
	cmd = exec.Command("git", "-C", syncDir, "commit", "-m", "Initial v2 config")
	require.NoError(t, cmd.Run())

	return syncDir
}

func TestPin(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	err := commands.Pin(syncDir, "context7@claude-plugins-official", "1.2.0")
	require.NoError(t, err)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Equal(t, "1.2.0", cfg.Pinned["context7@claude-plugins-official"])
	assert.NotContains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.Contains(t, cfg.Upstream, "beads@beads-marketplace")
}

func TestPin_AlreadyPinned(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	// Pin it first.
	err := commands.Pin(syncDir, "context7@claude-plugins-official", "1.0.0")
	require.NoError(t, err)

	// Pin again with a different version — should update.
	err = commands.Pin(syncDir, "context7@claude-plugins-official", "2.0.0")
	require.NoError(t, err)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	assert.Equal(t, "2.0.0", cfg.Pinned["context7@claude-plugins-official"])
}

func TestUnpin(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	// Pin first.
	err := commands.Pin(syncDir, "context7@claude-plugins-official", "1.0.0")
	require.NoError(t, err)

	// Unpin.
	err = commands.Unpin(syncDir, "context7@claude-plugins-official")
	require.NoError(t, err)

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)

	_, isPinned := cfg.Pinned["context7@claude-plugins-official"]
	assert.False(t, isPinned)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
}

func TestPin_NotInConfig(t *testing.T) {
	syncDir := setupV2TestEnv(t)

	err := commands.Pin(syncDir, "nonexistent@marketplace", "1.0.0")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not found")
}
