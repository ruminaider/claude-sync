package plugins_test

import (
	"testing"
	"time"

	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPluginSources_ReadWriteRoundTrip(t *testing.T) {
	syncDir := t.TempDir()

	sources := plugins.PluginSources{
		Plugins: map[string]plugins.PluginSourceEntry{
			"bash-validator": {
				ActiveSource:                "claude-sync-forks",
				Suppressed:                  "bash-validator-marketplace",
				Relationship:                "active-dev",
				LocalRepo:                   "~/Repositories/bash-validator",
				DecidedAt:                   time.Date(2026, 3, 2, 0, 0, 0, 0, time.UTC),
				MarketplaceVersionAtDecision: "1.0.0",
				LastLocalCommitAtDecision:   time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC),
			},
		},
	}

	err := plugins.WritePluginSources(syncDir, sources)
	require.NoError(t, err)

	loaded, err := plugins.ReadPluginSources(syncDir)
	require.NoError(t, err)
	assert.Equal(t, "claude-sync-forks", loaded.Plugins["bash-validator"].ActiveSource)
	assert.Equal(t, "active-dev", loaded.Plugins["bash-validator"].Relationship)
	assert.Equal(t, "1.0.0", loaded.Plugins["bash-validator"].MarketplaceVersionAtDecision)
}

func TestPluginSources_ReadMissing_ReturnsEmpty(t *testing.T) {
	syncDir := t.TempDir()

	sources, err := plugins.ReadPluginSources(syncDir)
	require.NoError(t, err)
	assert.NotNil(t, sources.Plugins)
	assert.Empty(t, sources.Plugins)
}

func TestPluginSources_SnoozeUntil(t *testing.T) {
	syncDir := t.TempDir()
	snoozeDate := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)

	sources := plugins.PluginSources{
		Plugins: map[string]plugins.PluginSourceEntry{
			"bash-validator": {
				ActiveSource: "forks",
				Suppressed:   "marketplace",
				Relationship: "active-dev",
				SnoozeUntil:  &snoozeDate,
			},
		},
	}

	err := plugins.WritePluginSources(syncDir, sources)
	require.NoError(t, err)

	loaded, err := plugins.ReadPluginSources(syncDir)
	require.NoError(t, err)
	assert.NotNil(t, loaded.Plugins["bash-validator"].SnoozeUntil)
	assert.Equal(t, snoozeDate.Day(), loaded.Plugins["bash-validator"].SnoozeUntil.Day())
}
