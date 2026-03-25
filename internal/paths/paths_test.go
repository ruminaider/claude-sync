package paths_test

import (
	"os"
	"path/filepath"
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

func TestForkedPluginsDir(t *testing.T) {
	assert.True(t, strings.HasSuffix(paths.ForkedPluginsDir(), "plugins"))
}

func TestMemoryPaths(t *testing.T) {
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".claude-sync", "memory"), paths.SyncMemoryDir())
	assert.Equal(t, filepath.Join(home, ".claude", "memory"), paths.ClaudeMemoryDir())
}

func TestCCSDir(t *testing.T) {
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".ccs"), paths.CCSDir())
}

func TestCCSDetection(t *testing.T) {
	home, _ := os.UserHomeDir()
	ccsDir := filepath.Join(home, ".ccs", "instances")
	instances, exists := paths.CCSInstances()
	if _, err := os.Stat(ccsDir); err == nil {
		assert.True(t, exists)
		assert.NotEmpty(t, instances)
	} else {
		assert.False(t, exists)
	}
}

func TestCCSInstanceMemoryDir(t *testing.T) {
	result := paths.CCSInstanceMemoryDir("/some/instance/path")
	assert.Equal(t, filepath.Join("/some/instance/path", "memory"), result)
}
