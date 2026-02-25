package subscriptions

import (
	"encoding/json"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/stretchr/testify/assert"
)

func TestApplyToConfig_PluginsAdded(t *testing.T) {
	localCfg := config.Config{
		Version:  "2.1",
		Upstream: []string{"existing-plugin"},
	}
	merged := &MergedResult{
		Plugins:  []string{"new-plugin-a", "new-plugin-b"},
		Settings: make(map[string]any),
		MCP:      make(map[string]json.RawMessage),
		Hooks:    make(map[string]json.RawMessage),
	}

	ApplyToConfig(&localCfg, merged)

	assert.Len(t, localCfg.Upstream, 3)
	assert.Contains(t, localCfg.Upstream, "existing-plugin")
	assert.Contains(t, localCfg.Upstream, "new-plugin-a")
	assert.Contains(t, localCfg.Upstream, "new-plugin-b")
}

func TestApplyToConfig_PluginsDeduplicated(t *testing.T) {
	localCfg := config.Config{
		Version:  "2.1",
		Upstream: []string{"plugin-a"},
	}
	merged := &MergedResult{
		Plugins:  []string{"plugin-a"}, // already in local
		Settings: make(map[string]any),
		MCP:      make(map[string]json.RawMessage),
		Hooks:    make(map[string]json.RawMessage),
	}

	ApplyToConfig(&localCfg, merged)

	assert.Len(t, localCfg.Upstream, 1) // not duplicated
}

func TestApplyToConfig_SettingsLocalWins(t *testing.T) {
	localCfg := config.Config{
		Version:  "2.1",
		Settings: map[string]any{"theme": "light"},
	}
	merged := &MergedResult{
		Settings: map[string]any{
			"theme": "dark",  // local has "light", local wins
			"font":  "mono",  // new setting, gets added
		},
		MCP:   make(map[string]json.RawMessage),
		Hooks: make(map[string]json.RawMessage),
	}

	ApplyToConfig(&localCfg, merged)

	assert.Equal(t, "light", localCfg.Settings["theme"]) // local wins
	assert.Equal(t, "mono", localCfg.Settings["font"])    // new added
}

func TestApplyToConfig_MCPLocalWins(t *testing.T) {
	localCfg := config.Config{
		Version: "2.1",
		MCP: map[string]json.RawMessage{
			"sentry": json.RawMessage(`{"url":"local-sentry"}`),
		},
	}
	merged := &MergedResult{
		Settings: make(map[string]any),
		MCP: map[string]json.RawMessage{
			"sentry":  json.RawMessage(`{"url":"remote-sentry"}`),
			"grafana": json.RawMessage(`{"url":"grafana"}`),
		},
		Hooks: make(map[string]json.RawMessage),
	}

	ApplyToConfig(&localCfg, merged)

	assert.JSONEq(t, `{"url":"local-sentry"}`, string(localCfg.MCP["sentry"])) // local wins
	assert.JSONEq(t, `{"url":"grafana"}`, string(localCfg.MCP["grafana"]))     // new added
}

func TestApplyToConfig_PermissionsAdditive(t *testing.T) {
	localCfg := config.Config{
		Version: "2.1",
		Permissions: config.Permissions{
			Allow: []string{"Bash(npm test)"},
		},
	}
	merged := &MergedResult{
		Settings: make(map[string]any),
		MCP:      make(map[string]json.RawMessage),
		Hooks:    make(map[string]json.RawMessage),
		Permissions: config.Permissions{
			Allow: []string{"Bash(make build)", "Bash(npm test)"}, // npm test already local
			Deny:  []string{"Bash(rm -rf)"},
		},
	}

	ApplyToConfig(&localCfg, merged)

	assert.Len(t, localCfg.Permissions.Allow, 2) // npm test deduped
	assert.Contains(t, localCfg.Permissions.Allow, "Bash(npm test)")
	assert.Contains(t, localCfg.Permissions.Allow, "Bash(make build)")
	assert.Len(t, localCfg.Permissions.Deny, 1)
	assert.Contains(t, localCfg.Permissions.Deny, "Bash(rm -rf)")
}

func TestApplyToConfig_ClaudeMDAdditive(t *testing.T) {
	localCfg := config.Config{
		Version: "2.1",
		ClaudeMD: config.ClaudeMDConfig{
			Include: []string{"local-guide"},
		},
	}
	merged := &MergedResult{
		Settings: make(map[string]any),
		MCP:      make(map[string]json.RawMessage),
		Hooks:    make(map[string]json.RawMessage),
		ClaudeMD: []string{"remote-guide", "local-guide"}, // local-guide deduped
	}

	ApplyToConfig(&localCfg, merged)

	assert.Len(t, localCfg.ClaudeMD.Include, 2)
	assert.Contains(t, localCfg.ClaudeMD.Include, "local-guide")
	assert.Contains(t, localCfg.ClaudeMD.Include, "remote-guide")
}

func TestApplyToConfig_NilMapsInitialized(t *testing.T) {
	localCfg := config.Config{
		Version: "2.1",
		// Settings, MCP, Hooks are all nil
	}
	merged := &MergedResult{
		Settings: map[string]any{"theme": "dark"},
		MCP:      map[string]json.RawMessage{"sentry": json.RawMessage(`{}`)},
		Hooks:    map[string]json.RawMessage{"pre-commit": json.RawMessage(`{}`)},
	}

	ApplyToConfig(&localCfg, merged)

	assert.NotNil(t, localCfg.Settings)
	assert.Equal(t, "dark", localCfg.Settings["theme"])
	assert.NotNil(t, localCfg.MCP)
	assert.Contains(t, localCfg.MCP, "sentry")
	assert.NotNil(t, localCfg.Hooks)
	assert.Contains(t, localCfg.Hooks, "pre-commit")
}

func TestMergeStringSlices(t *testing.T) {
	tests := []struct {
		name     string
		base     []string
		add      []string
		expected []string
	}{
		{"empty add", []string{"a"}, nil, []string{"a"}},
		{"empty base", nil, []string{"a"}, []string{"a"}},
		{"no overlap", []string{"a"}, []string{"b"}, []string{"a", "b"}},
		{"overlap deduped", []string{"a", "b"}, []string{"b", "c"}, []string{"a", "b", "c"}},
		{"both empty", nil, nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := mergeStringSlices(tt.base, tt.add)
			assert.Equal(t, tt.expected, result)
		})
	}
}
