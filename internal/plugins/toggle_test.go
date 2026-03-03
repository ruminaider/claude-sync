package plugins_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestToggleEnabledPlugin_DisableOne(t *testing.T) {
	claudeDir := t.TempDir()
	enabledPlugins := map[string]bool{
		"bash-validator@marketplace": true,
		"bash-validator@forks":      true,
	}
	writeTestSettings(t, claudeDir, enabledPlugins)

	err := plugins.ToggleEnabledPlugin(claudeDir, "bash-validator@forks", false)
	require.NoError(t, err)

	result := readEnabledPlugins(t, claudeDir)
	assert.True(t, result["bash-validator@marketplace"])
	assert.False(t, result["bash-validator@forks"])
}

func TestToggleEnabledPlugin_PreservesOtherKeys(t *testing.T) {
	claudeDir := t.TempDir()
	enabledPlugins := map[string]bool{
		"bash-validator@marketplace": true,
		"bash-validator@forks":      true,
		"context7@marketplace":      true,
	}
	writeTestSettings(t, claudeDir, enabledPlugins)

	err := plugins.ToggleEnabledPlugin(claudeDir, "bash-validator@forks", false)
	require.NoError(t, err)

	result := readEnabledPlugins(t, claudeDir)
	assert.True(t, result["context7@marketplace"])
}

func readEnabledPlugins(t *testing.T, claudeDir string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(claudeDir, "settings.json"))
	require.NoError(t, err)
	var settings map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(data, &settings))
	var ep map[string]bool
	require.NoError(t, json.Unmarshal(settings["enabledPlugins"], &ep))
	return ep
}
