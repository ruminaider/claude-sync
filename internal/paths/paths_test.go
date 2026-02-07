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

func TestForkedPluginsDir(t *testing.T) {
	assert.True(t, strings.HasSuffix(paths.ForkedPluginsDir(), "plugins"))
}
