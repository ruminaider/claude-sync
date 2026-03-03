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

func TestDetectDuplicates_FindsDuplicates(t *testing.T) {
	claudeDir := t.TempDir()
	enabledPlugins := map[string]bool{
		"bash-validator@bash-validator-marketplace": true,
		"bash-validator@claude-sync-forks":          true,
		"superpowers@superpowers-marketplace":       true,
		"superpowers@claude-plugins-official":       true,
		"context7@claude-plugins-official":          true,
	}
	writeTestSettings(t, claudeDir, enabledPlugins)

	dupes, err := plugins.DetectDuplicates(claudeDir)
	require.NoError(t, err)
	assert.Len(t, dupes, 2)

	names := []string{dupes[0].Name, dupes[1].Name}
	assert.Contains(t, names, "bash-validator")
	assert.Contains(t, names, "superpowers")
}

func TestDetectDuplicates_NoDuplicates(t *testing.T) {
	claudeDir := t.TempDir()
	enabledPlugins := map[string]bool{
		"bash-validator@marketplace": true,
		"context7@marketplace":      true,
	}
	writeTestSettings(t, claudeDir, enabledPlugins)

	dupes, err := plugins.DetectDuplicates(claudeDir)
	require.NoError(t, err)
	assert.Empty(t, dupes)
}

func TestDetectDuplicates_IgnoresDisabled(t *testing.T) {
	claudeDir := t.TempDir()
	enabledPlugins := map[string]bool{
		"bash-validator@marketplace": true,
		"bash-validator@forks":      false,
	}
	writeTestSettings(t, claudeDir, enabledPlugins)

	dupes, err := plugins.DetectDuplicates(claudeDir)
	require.NoError(t, err)
	assert.Empty(t, dupes)
}

func writeTestSettings(t *testing.T, claudeDir string, enabledPlugins map[string]bool) {
	t.Helper()
	ep, _ := json.Marshal(enabledPlugins)
	settings := map[string]json.RawMessage{
		"enabledPlugins": ep,
	}
	data, _ := json.MarshalIndent(settings, "", "  ")
	err := os.WriteFile(filepath.Join(claudeDir, "settings.json"), data, 0644)
	require.NoError(t, err)
}
