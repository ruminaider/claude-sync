package plugins_test

import (
	"testing"
	"time"

	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestResolveDuplicate_KeepLocal(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	writeTestSettings(t, claudeDir, map[string]bool{
		"bash-validator@marketplace": true,
		"bash-validator@forks":      true,
	})

	resolution := plugins.Resolution{
		PluginName:   "bash-validator",
		KeepSource:   "bash-validator@forks",
		RemoveSource: "bash-validator@marketplace",
		Relationship: "active-dev",
		LocalRepo:    "/tmp/bash-validator",
	}

	err := plugins.ApplyResolution(claudeDir, syncDir, resolution)
	require.NoError(t, err)

	// Verify settings.json updated
	ep := readEnabledPlugins(t, claudeDir)
	assert.True(t, ep["bash-validator@forks"])
	assert.False(t, ep["bash-validator@marketplace"])

	// Verify plugin-sources.yaml updated
	sources, err := plugins.ReadPluginSources(syncDir)
	require.NoError(t, err)
	entry := sources.Plugins["bash-validator"]
	assert.Equal(t, "forks", entry.ActiveSource)
	assert.Equal(t, "marketplace", entry.Suppressed)
	assert.Equal(t, "active-dev", entry.Relationship)
}

func TestResolveDuplicate_KeepMarketplace(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	writeTestSettings(t, claudeDir, map[string]bool{
		"bash-validator@marketplace": true,
		"bash-validator@forks":      true,
	})

	resolution := plugins.Resolution{
		PluginName:   "bash-validator",
		KeepSource:   "bash-validator@marketplace",
		RemoveSource: "bash-validator@forks",
		Relationship: "preference",
	}

	err := plugins.ApplyResolution(claudeDir, syncDir, resolution)
	require.NoError(t, err)

	ep := readEnabledPlugins(t, claudeDir)
	assert.True(t, ep["bash-validator@marketplace"])
	assert.False(t, ep["bash-validator@forks"])
}

func TestApplyReEvalSwitch(t *testing.T) {
	claudeDir := t.TempDir()
	syncDir := t.TempDir()

	writeTestSettings(t, claudeDir, map[string]bool{
		"bash-validator@forks": true,
	})

	// Pre-populate sources with active-dev entry
	sources := plugins.PluginSources{
		Plugins: map[string]plugins.PluginSourceEntry{
			"bash-validator": {
				ActiveSource: "forks",
				Suppressed:   "marketplace",
				Relationship: "active-dev",
				DecidedAt:    time.Now().AddDate(0, 0, -14),
			},
		},
	}
	plugins.WritePluginSources(syncDir, sources)

	err := plugins.ApplyReEvalSwitch(claudeDir, syncDir, "bash-validator")
	require.NoError(t, err)

	// Verify: marketplace re-enabled, forks disabled
	ep := readEnabledPlugins(t, claudeDir)
	assert.True(t, ep["bash-validator@marketplace"])
	assert.False(t, ep["bash-validator@forks"])

	// Verify: tracking cleared
	updated, _ := plugins.ReadPluginSources(syncDir)
	entry := updated.Plugins["bash-validator"]
	assert.Equal(t, "marketplace", entry.ActiveSource)
	assert.Equal(t, "preference", entry.Relationship)
}
