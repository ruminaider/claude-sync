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

// setupMigrateSyncDir creates a temporary syncDir with a git repo and an
// initial committed config.yaml containing the given content.
func setupMigrateSyncDir(t *testing.T, cfgContent string) string {
	t.Helper()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")
	require.NoError(t, os.MkdirAll(syncDir, 0755))

	cfgPath := filepath.Join(syncDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte(cfgContent), 0644))

	// Initialize git repo and make an initial commit so MigrateApply can commit.
	gitInit := exec.Command("git", "init")
	gitInit.Dir = syncDir
	require.NoError(t, gitInit.Run())

	gitAdd := exec.Command("git", "add", ".")
	gitAdd.Dir = syncDir
	require.NoError(t, gitAdd.Run())

	gitCommit := exec.Command("git", "-c", "user.name=test", "-c", "user.email=test@test.com", "commit", "-m", "initial")
	gitCommit.Dir = syncDir
	require.NoError(t, gitCommit.Run())

	return syncDir
}

func TestMigrateDetectsV1(t *testing.T) {
	v1Config := `version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
settings:
  model: opus
`
	syncDir := setupMigrateSyncDir(t, v1Config)

	needed, err := commands.MigrateNeeded(syncDir)
	require.NoError(t, err)
	assert.True(t, needed, "v1 config should need migration")
}

func TestMigrateNotNeededForV2(t *testing.T) {
	v2Config := `version: "2.0.0"
plugins:
  upstream:
    - context7@claude-plugins-official
  pinned:
    - beads@beads-marketplace: "0.44.0"
  forked:
    - figma-minimal
`
	syncDir := setupMigrateSyncDir(t, v2Config)

	needed, err := commands.MigrateNeeded(syncDir)
	require.NoError(t, err)
	assert.False(t, needed, "v2 config should not need migration")
}

func TestMigrateApply(t *testing.T) {
	v1Config := `version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
  - figma-minimal@claude-plugins-official
settings:
  model: opus
hooks:
  PreCompact: "bd prime"
`
	syncDir := setupMigrateSyncDir(t, v1Config)

	categories := map[string]string{
		"context7@claude-plugins-official":      "upstream",
		"beads@beads-marketplace":               "pinned",
		"figma-minimal@claude-plugins-official":  "forked",
	}
	versions := map[string]string{
		"beads@beads-marketplace": "0.44.0",
	}

	err := commands.MigrateApply(syncDir, categories, versions)
	require.NoError(t, err)

	// Read back and verify the written config.
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)

	cfg, err := config.Parse(data)
	require.NoError(t, err)

	assert.Equal(t, "2.0.0", cfg.Version)
	assert.Contains(t, cfg.Upstream, "context7@claude-plugins-official")
	assert.Equal(t, "0.44.0", cfg.Pinned["beads@beads-marketplace"])
	assert.Contains(t, cfg.Forked, "figma-minimal@claude-plugins-official")

	// Settings and hooks should be preserved.
	assert.Equal(t, "opus", cfg.Settings["model"])
	assert.Equal(t, "bd prime", cfg.Hooks["PreCompact"])

	// Verify git committed the change.
	gitLog := exec.Command("git", "log", "--oneline", "-1")
	gitLog.Dir = syncDir
	out, err := gitLog.Output()
	require.NoError(t, err)
	assert.Contains(t, string(out), "Migrate config to v2")
}

func TestMigrateApply_PinnedRequiresVersion(t *testing.T) {
	v1Config := `version: "1.0.0"
plugins:
  - beads@beads-marketplace
`
	syncDir := setupMigrateSyncDir(t, v1Config)

	categories := map[string]string{
		"beads@beads-marketplace": "pinned",
	}
	// Empty versions map - should error.
	versions := map[string]string{}

	err := commands.MigrateApply(syncDir, categories, versions)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "requires a version")
}

func TestMigratePlugins(t *testing.T) {
	v1Config := `version: "1.0.0"
plugins:
  - context7@claude-plugins-official
  - beads@beads-marketplace
`
	syncDir := setupMigrateSyncDir(t, v1Config)

	plugins, err := commands.MigratePlugins(syncDir)
	require.NoError(t, err)
	assert.Equal(t, []string{"context7@claude-plugins-official", "beads@beads-marketplace"}, plugins)
}
