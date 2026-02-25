package subscriptions

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupSubConfig creates a subscription directory with a config.yaml.
func setupSubConfig(t *testing.T, syncDir, subName string, cfg config.ConfigV2) {
	t.Helper()
	subDir := SubDir(syncDir, subName)
	require.NoError(t, os.MkdirAll(subDir, 0755))

	data, err := config.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(subDir, "config.yaml"), data, 0644))
}

func TestMergeAll_SingleSubscription_MCPAll(t *testing.T) {
	syncDir := t.TempDir()

	// Remote config with 3 MCP servers.
	remoteCfg := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry":  json.RawMessage(`{"url":"http://sentry"}`),
			"grafana": json.RawMessage(`{"url":"http://grafana"}`),
			"datadog": json.RawMessage(`{"url":"http://datadog"}`),
		},
	}
	setupSubConfig(t, syncDir, "team-backend", remoteCfg)

	// Local config with no MCP.
	localCfg := config.Config{
		Version: "2.1",
	}

	subs := map[string]config.SubscriptionEntry{
		"team-backend": {
			URL: "git@github.com:org/team-backend.git",
			Categories: map[string]any{
				"mcp": "all",
			},
		},
	}

	merged, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	assert.Empty(t, conflicts)
	assert.Len(t, merged.MCP, 3)
	assert.Contains(t, merged.MCP, "sentry")
	assert.Contains(t, merged.MCP, "grafana")
	assert.Contains(t, merged.MCP, "datadog")
}

func TestMergeAll_LocalWins(t *testing.T) {
	syncDir := t.TempDir()

	// Remote config with MCP server "sentry".
	remoteCfg := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://remote-sentry"}`),
		},
	}
	setupSubConfig(t, syncDir, "team-backend", remoteCfg)

	// Local config also has "sentry" with different value.
	localCfg := config.Config{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://local-sentry"}`),
		},
	}

	subs := map[string]config.SubscriptionEntry{
		"team-backend": {
			URL:        "git@github.com:org/team-backend.git",
			Categories: map[string]any{"mcp": "all"},
		},
	}

	merged, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	assert.Empty(t, conflicts)
	// "sentry" should NOT be in merged because local wins (it's excluded).
	assert.Empty(t, merged.MCP)
}

func TestMergeAll_ConflictBetweenSubscriptions(t *testing.T) {
	syncDir := t.TempDir()

	// Two subscriptions both have "sentry" MCP server.
	remoteCfgA := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://sentry-a"}`),
		},
	}
	remoteCfgB := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://sentry-b"}`),
		},
	}
	setupSubConfig(t, syncDir, "team-alpha", remoteCfgA)
	setupSubConfig(t, syncDir, "team-beta", remoteCfgB)

	localCfg := config.Config{Version: "2.1"}

	subs := map[string]config.SubscriptionEntry{
		"team-alpha": {
			URL:        "git@github.com:org/team-alpha.git",
			Categories: map[string]any{"mcp": "all"},
		},
		"team-beta": {
			URL:        "git@github.com:org/team-beta.git",
			Categories: map[string]any{"mcp": "all"},
		},
	}

	_, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	assert.Len(t, conflicts, 1)
	assert.Equal(t, "mcp", conflicts[0].Category)
	assert.Equal(t, "sentry", conflicts[0].ItemName)
}

func TestMergeAll_PreferResolvesConflict(t *testing.T) {
	syncDir := t.TempDir()

	remoteCfgA := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://sentry-a"}`),
		},
	}
	remoteCfgB := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://sentry-b"}`),
		},
	}
	setupSubConfig(t, syncDir, "team-alpha", remoteCfgA)
	setupSubConfig(t, syncDir, "team-beta", remoteCfgB)

	localCfg := config.Config{Version: "2.1"}

	subs := map[string]config.SubscriptionEntry{
		"team-alpha": {
			URL:        "git@github.com:org/team-alpha.git",
			Categories: map[string]any{"mcp": "all"},
			Prefer:     map[string][]string{"mcp": {"sentry"}},
		},
		"team-beta": {
			URL:        "git@github.com:org/team-beta.git",
			Categories: map[string]any{"mcp": "all"},
		},
	}

	merged, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	assert.Empty(t, conflicts)
	assert.Contains(t, merged.MCP, "sentry")
	// Alpha's version should win due to prefer directive.
	assert.JSONEq(t, `{"url":"http://sentry-a"}`, string(merged.MCP["sentry"]))
	assert.Equal(t, "team-alpha", merged.Provenance["mcp"]["sentry"])
}

func TestMergeAll_BothPrefer_FailsLoudly(t *testing.T) {
	syncDir := t.TempDir()

	remoteCfgA := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://sentry-a"}`),
		},
	}
	remoteCfgB := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"http://sentry-b"}`),
		},
	}
	setupSubConfig(t, syncDir, "team-alpha", remoteCfgA)
	setupSubConfig(t, syncDir, "team-beta", remoteCfgB)

	localCfg := config.Config{Version: "2.1"}

	subs := map[string]config.SubscriptionEntry{
		"team-alpha": {
			URL:        "git@github.com:org/team-alpha.git",
			Categories: map[string]any{"mcp": "all"},
			Prefer:     map[string][]string{"mcp": {"sentry"}},
		},
		"team-beta": {
			URL:        "git@github.com:org/team-beta.git",
			Categories: map[string]any{"mcp": "all"},
			Prefer:     map[string][]string{"mcp": {"sentry"}},
		},
	}

	_, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	// Both claim prefer → unresolved conflict.
	assert.Len(t, conflicts, 1)
}

func TestMergeAll_PluginsFirstWins(t *testing.T) {
	syncDir := t.TempDir()

	remoteCfgA := config.ConfigV2{
		Version:  "2.1",
		Upstream: []string{"plugin-a", "shared-plugin"},
	}
	remoteCfgB := config.ConfigV2{
		Version:  "2.1",
		Upstream: []string{"plugin-b", "shared-plugin"},
	}
	setupSubConfig(t, syncDir, "team-alpha", remoteCfgA)
	setupSubConfig(t, syncDir, "team-beta", remoteCfgB)

	localCfg := config.Config{Version: "2.1"}

	subs := map[string]config.SubscriptionEntry{
		"team-alpha": {
			URL:        "git@github.com:org/team-alpha.git",
			Categories: map[string]any{"plugins": "all"},
		},
		"team-beta": {
			URL:        "git@github.com:org/team-beta.git",
			Categories: map[string]any{"plugins": "all"},
		},
	}

	merged, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	assert.Empty(t, conflicts) // plugins don't conflict, first wins
	assert.Contains(t, merged.Plugins, "plugin-a")
	assert.Contains(t, merged.Plugins, "plugin-b")
	assert.Contains(t, merged.Plugins, "shared-plugin")
	// shared-plugin should be attributed to team-alpha (sorted first).
	assert.Equal(t, "team-alpha", merged.Provenance["plugins"]["shared-plugin"])
}

func TestMergeAll_SettingsConflict_SameValue_NoConflict(t *testing.T) {
	syncDir := t.TempDir()

	remoteCfgA := config.ConfigV2{
		Version:  "2.1",
		Settings: map[string]any{"theme": "dark"},
	}
	remoteCfgB := config.ConfigV2{
		Version:  "2.1",
		Settings: map[string]any{"theme": "dark"},
	}
	setupSubConfig(t, syncDir, "team-alpha", remoteCfgA)
	setupSubConfig(t, syncDir, "team-beta", remoteCfgB)

	localCfg := config.Config{Version: "2.1"}

	subs := map[string]config.SubscriptionEntry{
		"team-alpha": {
			URL:        "git@github.com:org/team-alpha.git",
			Categories: map[string]any{"settings": "all"},
		},
		"team-beta": {
			URL:        "git@github.com:org/team-beta.git",
			Categories: map[string]any{"settings": "all"},
		},
	}

	merged, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	// Same value → no conflict.
	assert.Empty(t, conflicts)
	assert.Equal(t, "dark", merged.Settings["theme"])
}

func TestMergeAll_FilteredCategories(t *testing.T) {
	syncDir := t.TempDir()

	remoteCfg := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry":  json.RawMessage(`{"url":"http://sentry"}`),
			"grafana": json.RawMessage(`{"url":"http://grafana"}`),
		},
		Settings: map[string]any{"theme": "dark"},
	}
	setupSubConfig(t, syncDir, "team-backend", remoteCfg)

	localCfg := config.Config{Version: "2.1"}

	subs := map[string]config.SubscriptionEntry{
		"team-backend": {
			URL: "git@github.com:org/team-backend.git",
			Categories: map[string]any{
				"mcp":      "all",
				"settings": "none", // explicitly not subscribing to settings
			},
			Exclude: map[string][]string{
				"mcp": {"grafana"},
			},
		},
	}

	merged, conflicts, err := MergeAll(syncDir, subs, localCfg)
	require.NoError(t, err)
	assert.Empty(t, conflicts)

	// Only sentry MCP (grafana excluded).
	assert.Len(t, merged.MCP, 1)
	assert.Contains(t, merged.MCP, "sentry")

	// Settings not included (mode=none).
	assert.Empty(t, merged.Settings)
}

func TestIncrementalDiff_NewItems(t *testing.T) {
	syncDir := t.TempDir()

	remoteCfg := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry":     json.RawMessage(`{}`),
			"grafana":    json.RawMessage(`{}`),
			"prometheus": json.RawMessage(`{}`),
		},
	}
	setupSubConfig(t, syncDir, "team-backend", remoteCfg)

	sub := config.SubscriptionEntry{
		URL:        "git@github.com:org/team-backend.git",
		Categories: map[string]any{"mcp": "all"},
	}

	// State shows sentry was previously accepted.
	state := SubscriptionState{
		Subscriptions: map[string]SubState{
			"team-backend": {
				AcceptedItems: map[string][]string{
					"mcp": {"sentry"},
				},
			},
		},
	}

	newItems, err := IncrementalDiff("team-backend", sub, syncDir, state)
	require.NoError(t, err)
	assert.Len(t, newItems["mcp"], 2) // grafana and prometheus are new
	assert.Contains(t, newItems["mcp"], "grafana")
	assert.Contains(t, newItems["mcp"], "prometheus")
}

func TestIncrementalDiff_NoPreviousState(t *testing.T) {
	syncDir := t.TempDir()

	remoteCfg := config.ConfigV2{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry":  json.RawMessage(`{}`),
			"grafana": json.RawMessage(`{}`),
		},
	}
	setupSubConfig(t, syncDir, "team-backend", remoteCfg)

	sub := config.SubscriptionEntry{
		URL:        "git@github.com:org/team-backend.git",
		Categories: map[string]any{"mcp": "all"},
	}

	state := SubscriptionState{Subscriptions: make(map[string]SubState)}

	newItems, err := IncrementalDiff("team-backend", sub, syncDir, state)
	require.NoError(t, err)
	// All items are new since no previous state.
	assert.Len(t, newItems["mcp"], 2)
}

func TestFormatConflicts_Empty(t *testing.T) {
	assert.Empty(t, FormatConflicts(nil))
}

func TestFormatConflicts_NonEmpty(t *testing.T) {
	conflicts := []Conflict{
		{Category: "mcp", ItemName: "sentry", SourceA: "team-a", SourceB: "team-b"},
	}
	s := FormatConflicts(conflicts)
	assert.Contains(t, s, "Subscription conflicts detected")
	assert.Contains(t, s, "sentry")
	assert.Contains(t, s, "team-a")
	assert.Contains(t, s, "team-b")
	assert.Contains(t, s, "prefer")
}
