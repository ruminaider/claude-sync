package profiles_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// assertHookHasCommand checks that a json.RawMessage hook entry contains
// the expected command string in its first hook entry.
func assertHookHasCommand(t *testing.T, hookData json.RawMessage, expectedCmd string) {
	t.Helper()
	var entries []struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	require.NoError(t, json.Unmarshal(hookData, &entries))
	require.NotEmpty(t, entries)
	require.NotEmpty(t, entries[0].Hooks)
	assert.Equal(t, expectedCmd, entries[0].Hooks[0].Command)
}

func TestParseProfile(t *testing.T) {
	t.Run("full profile with all fields", func(t *testing.T) {
		input := []byte(`plugins:
  add:
    - foo@marketplace
    - bar@marketplace
  remove:
    - old@marketplace
settings:
  model: sonnet
  temperature: 0.5
hooks:
  add:
    PreCompact: "lint --fix"
  remove:
    - SessionStart
`)
		p, err := profiles.ParseProfile(input)
		require.NoError(t, err)
		assert.Equal(t, []string{"foo@marketplace", "bar@marketplace"}, p.Plugins.Add)
		assert.Equal(t, []string{"old@marketplace"}, p.Plugins.Remove)
		assert.Equal(t, "sonnet", p.Settings["model"])
		assert.Equal(t, 0.5, p.Settings["temperature"])
		assertHookHasCommand(t, p.Hooks.Add["PreCompact"], "lint --fix")
		assert.Equal(t, []string{"SessionStart"}, p.Hooks.Remove)
	})

	t.Run("hooks with raw JSON string", func(t *testing.T) {
		rawJSON := `[{"matcher":"","hooks":[{"type":"command","command":"bd prime"}]}]`
		input := []byte(`hooks:
  add:
    PreCompact: '` + rawJSON + `'
`)
		p, err := profiles.ParseProfile(input)
		require.NoError(t, err)
		assert.JSONEq(t, rawJSON, string(p.Hooks.Add["PreCompact"]))
	})

	t.Run("empty document", func(t *testing.T) {
		p, err := profiles.ParseProfile([]byte(""))
		require.NoError(t, err)
		assert.Empty(t, p.Plugins.Add)
		assert.Empty(t, p.Plugins.Remove)
		assert.Nil(t, p.Settings)
		assert.Nil(t, p.Hooks.Add)
		assert.Empty(t, p.Hooks.Remove)
	})

	t.Run("plugins only", func(t *testing.T) {
		input := []byte(`plugins:
  add:
    - alpha@m
`)
		p, err := profiles.ParseProfile(input)
		require.NoError(t, err)
		assert.Equal(t, []string{"alpha@m"}, p.Plugins.Add)
		assert.Nil(t, p.Settings)
	})

	t.Run("settings only", func(t *testing.T) {
		input := []byte(`settings:
  model: opus
`)
		p, err := profiles.ParseProfile(input)
		require.NoError(t, err)
		assert.Empty(t, p.Plugins.Add)
		assert.Equal(t, "opus", p.Settings["model"])
	})

	t.Run("invalid yaml", func(t *testing.T) {
		_, err := profiles.ParseProfile([]byte(`{{{`))
		assert.Error(t, err)
	})
}

func TestMarshalProfile(t *testing.T) {
	t.Run("round-trip with all fields", func(t *testing.T) {
		original := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add:    []string{"foo@m", "bar@m"},
				Remove: []string{"old@m"},
			},
			Settings: map[string]any{
				"model": "sonnet",
			},
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{
					"PreCompact": config.ExpandHookCommand("lint --fix"),
				},
				Remove: []string{"SessionStart"},
			},
		}

		data, err := profiles.MarshalProfile(original)
		require.NoError(t, err)

		parsed, err := profiles.ParseProfile(data)
		require.NoError(t, err)

		assert.Equal(t, original.Plugins.Add, parsed.Plugins.Add)
		assert.Equal(t, original.Plugins.Remove, parsed.Plugins.Remove)
		assert.Equal(t, original.Settings["model"], parsed.Settings["model"])
		assertHookHasCommand(t, parsed.Hooks.Add["PreCompact"], "lint --fix")
		assert.Equal(t, original.Hooks.Remove, parsed.Hooks.Remove)
	})

	t.Run("empty profile produces minimal yaml", func(t *testing.T) {
		p := profiles.Profile{}
		data, err := profiles.MarshalProfile(p)
		require.NoError(t, err)
		// Should produce an essentially empty document.
		assert.NotNil(t, data)
	})

	t.Run("plugins only", func(t *testing.T) {
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add: []string{"a@b"},
			},
		}
		data, err := profiles.MarshalProfile(p)
		require.NoError(t, err)

		parsed, err := profiles.ParseProfile(data)
		require.NoError(t, err)
		assert.Equal(t, []string{"a@b"}, parsed.Plugins.Add)
		assert.Nil(t, parsed.Settings)
	})

	t.Run("raw JSON hooks round-trip", func(t *testing.T) {
		rawJSON := `[{"matcher":"proj/.*","hooks":[{"type":"command","command":"lint"},{"type":"command","command":"test"}]}]`
		p := profiles.Profile{
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{
					"PreCompact": json.RawMessage(rawJSON),
				},
			},
		}
		data, err := profiles.MarshalProfile(p)
		require.NoError(t, err)

		parsed, err := profiles.ParseProfile(data)
		require.NoError(t, err)
		assert.JSONEq(t, rawJSON, string(parsed.Hooks.Add["PreCompact"]))
	})
}

func TestMergePlugins(t *testing.T) {
	t.Run("adds union with base", func(t *testing.T) {
		base := []string{"a@m", "b@m"}
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add: []string{"c@m", "d@m"},
			},
		}
		result := profiles.MergePlugins(base, p)
		assert.Equal(t, []string{"a@m", "b@m", "c@m", "d@m"}, result)
	})

	t.Run("removes subtract from base", func(t *testing.T) {
		base := []string{"a@m", "b@m", "c@m"}
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Remove: []string{"b@m"},
			},
		}
		result := profiles.MergePlugins(base, p)
		assert.Equal(t, []string{"a@m", "c@m"}, result)
	})

	t.Run("add and remove combined", func(t *testing.T) {
		base := []string{"a@m", "b@m"}
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add:    []string{"c@m"},
				Remove: []string{"a@m"},
			},
		}
		result := profiles.MergePlugins(base, p)
		assert.Equal(t, []string{"b@m", "c@m"}, result)
	})

	t.Run("no duplicates from add", func(t *testing.T) {
		base := []string{"a@m", "b@m"}
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add: []string{"b@m", "c@m"},
			},
		}
		result := profiles.MergePlugins(base, p)
		assert.Equal(t, []string{"a@m", "b@m", "c@m"}, result)
	})

	t.Run("empty profile returns base unchanged", func(t *testing.T) {
		base := []string{"a@m", "b@m"}
		p := profiles.Profile{}
		result := profiles.MergePlugins(base, p)
		assert.Equal(t, []string{"a@m", "b@m"}, result)
	})

	t.Run("empty base with adds", func(t *testing.T) {
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add: []string{"a@m"},
			},
		}
		result := profiles.MergePlugins(nil, p)
		assert.Equal(t, []string{"a@m"}, result)
	})

	t.Run("remove item not in base is no-op", func(t *testing.T) {
		base := []string{"a@m"}
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Remove: []string{"z@m"},
			},
		}
		result := profiles.MergePlugins(base, p)
		assert.Equal(t, []string{"a@m"}, result)
	})
}

func TestMergeSettings(t *testing.T) {
	t.Run("profile values override base", func(t *testing.T) {
		base := map[string]any{"model": "opus", "temperature": 0.7}
		p := profiles.Profile{
			Settings: map[string]any{"model": "sonnet"},
		}
		result := profiles.MergeSettings(base, p)
		assert.Equal(t, "sonnet", result["model"])
		assert.Equal(t, 0.7, result["temperature"])
	})

	t.Run("base-only values preserved", func(t *testing.T) {
		base := map[string]any{"model": "opus", "maxTokens": 1000}
		p := profiles.Profile{
			Settings: map[string]any{"theme": "dark"},
		}
		result := profiles.MergeSettings(base, p)
		assert.Equal(t, "opus", result["model"])
		assert.Equal(t, 1000, result["maxTokens"])
		assert.Equal(t, "dark", result["theme"])
	})

	t.Run("nil base returns profile settings copy", func(t *testing.T) {
		p := profiles.Profile{
			Settings: map[string]any{"model": "sonnet"},
		}
		result := profiles.MergeSettings(nil, p)
		assert.Equal(t, "sonnet", result["model"])
	})

	t.Run("nil profile settings returns base copy", func(t *testing.T) {
		base := map[string]any{"model": "opus"}
		p := profiles.Profile{}
		result := profiles.MergeSettings(base, p)
		assert.Equal(t, "opus", result["model"])
	})

	t.Run("both nil returns nil", func(t *testing.T) {
		result := profiles.MergeSettings(nil, profiles.Profile{})
		assert.Nil(t, result)
	})

	t.Run("result is a copy, not aliased", func(t *testing.T) {
		base := map[string]any{"model": "opus"}
		p := profiles.Profile{}
		result := profiles.MergeSettings(base, p)
		result["model"] = "changed"
		assert.Equal(t, "opus", base["model"]) // base should be unchanged
	})
}

func TestMergeHooks(t *testing.T) {
	t.Run("adds new hooks", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"SessionStart": config.ExpandHookCommand("pull"),
		}
		p := profiles.Profile{
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{
					"PreCompact": config.ExpandHookCommand("lint"),
				},
			},
		}
		result := profiles.MergeHooks(base, p)
		assert.Len(t, result, 2)
		assertHookHasCommand(t, result["SessionStart"], "pull")
		assertHookHasCommand(t, result["PreCompact"], "lint")
	})

	t.Run("removes listed hooks", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"SessionStart": config.ExpandHookCommand("pull"),
			"PreCompact":   config.ExpandHookCommand("lint"),
		}
		p := profiles.Profile{
			Hooks: profiles.ProfileHooks{
				Remove: []string{"PreCompact"},
			},
		}
		result := profiles.MergeHooks(base, p)
		assert.Len(t, result, 1)
		assertHookHasCommand(t, result["SessionStart"], "pull")
		assert.Nil(t, result["PreCompact"])
	})

	t.Run("add overrides existing hook", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"PreCompact": config.ExpandHookCommand("old-lint"),
		}
		p := profiles.Profile{
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{
					"PreCompact": config.ExpandHookCommand("new-lint"),
				},
			},
		}
		result := profiles.MergeHooks(base, p)
		assertHookHasCommand(t, result["PreCompact"], "new-lint")
	})

	t.Run("empty profile returns base copy", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"SessionStart": config.ExpandHookCommand("pull"),
		}
		p := profiles.Profile{}
		result := profiles.MergeHooks(base, p)
		assert.Len(t, result, 1)
		assertHookHasCommand(t, result["SessionStart"], "pull")
	})

	t.Run("nil base returns adds only", func(t *testing.T) {
		p := profiles.Profile{
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{
					"PreCompact": config.ExpandHookCommand("lint"),
				},
			},
		}
		result := profiles.MergeHooks(nil, p)
		assert.Len(t, result, 1)
		assertHookHasCommand(t, result["PreCompact"], "lint")
	})

	t.Run("remove nonexistent hook is no-op", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"SessionStart": config.ExpandHookCommand("pull"),
		}
		p := profiles.Profile{
			Hooks: profiles.ProfileHooks{
				Remove: []string{"NonExistent"},
			},
		}
		result := profiles.MergeHooks(base, p)
		assert.Len(t, result, 1)
	})
}

func TestListProfiles(t *testing.T) {
	t.Run("no profiles directory", func(t *testing.T) {
		dir := t.TempDir()
		names, err := profiles.ListProfiles(dir)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("empty profiles directory", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "profiles"), 0755))
		names, err := profiles.ListProfiles(dir)
		require.NoError(t, err)
		assert.Empty(t, names)
	})

	t.Run("single profile", func(t *testing.T) {
		dir := t.TempDir()
		profilesDir := filepath.Join(dir, "profiles")
		require.NoError(t, os.MkdirAll(profilesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "work.yaml"), []byte(""), 0644))

		names, err := profiles.ListProfiles(dir)
		require.NoError(t, err)
		assert.Equal(t, []string{"work"}, names)
	})

	t.Run("multiple profiles sorted", func(t *testing.T) {
		dir := t.TempDir()
		profilesDir := filepath.Join(dir, "profiles")
		require.NoError(t, os.MkdirAll(profilesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "zebra.yaml"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "alpha.yaml"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "mid.yaml"), []byte(""), 0644))

		names, err := profiles.ListProfiles(dir)
		require.NoError(t, err)
		assert.Equal(t, []string{"alpha", "mid", "zebra"}, names)
	})

	t.Run("ignores non-yaml files", func(t *testing.T) {
		dir := t.TempDir()
		profilesDir := filepath.Join(dir, "profiles")
		require.NoError(t, os.MkdirAll(profilesDir, 0755))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "work.yaml"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "notes.txt"), []byte(""), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, ".hidden"), []byte(""), 0644))

		names, err := profiles.ListProfiles(dir)
		require.NoError(t, err)
		assert.Equal(t, []string{"work"}, names)
	})

	t.Run("ignores subdirectories", func(t *testing.T) {
		dir := t.TempDir()
		profilesDir := filepath.Join(dir, "profiles")
		require.NoError(t, os.MkdirAll(filepath.Join(profilesDir, "subdir.yaml"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "work.yaml"), []byte(""), 0644))

		names, err := profiles.ListProfiles(dir)
		require.NoError(t, err)
		assert.Equal(t, []string{"work"}, names)
	})
}

func TestReadProfile(t *testing.T) {
	t.Run("reads and parses profile", func(t *testing.T) {
		dir := t.TempDir()
		profilesDir := filepath.Join(dir, "profiles")
		require.NoError(t, os.MkdirAll(profilesDir, 0755))

		content := []byte(`plugins:
  add:
    - foo@m
settings:
  model: sonnet
`)
		require.NoError(t, os.WriteFile(filepath.Join(profilesDir, "work.yaml"), content, 0644))

		p, err := profiles.ReadProfile(dir, "work")
		require.NoError(t, err)
		assert.Equal(t, []string{"foo@m"}, p.Plugins.Add)
		assert.Equal(t, "sonnet", p.Settings["model"])
	})

	t.Run("error when profile does not exist", func(t *testing.T) {
		dir := t.TempDir()
		_, err := profiles.ReadProfile(dir, "nonexistent")
		assert.Error(t, err)
	})
}

func TestActiveProfile(t *testing.T) {
	t.Run("read returns empty when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		name, err := profiles.ReadActiveProfile(dir)
		require.NoError(t, err)
		assert.Equal(t, "", name)
	})

	t.Run("write and read round-trip", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, profiles.WriteActiveProfile(dir, "work"))

		name, err := profiles.ReadActiveProfile(dir)
		require.NoError(t, err)
		assert.Equal(t, "work", name)
	})

	t.Run("write overwrites previous value", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, profiles.WriteActiveProfile(dir, "old"))
		require.NoError(t, profiles.WriteActiveProfile(dir, "new"))

		name, err := profiles.ReadActiveProfile(dir)
		require.NoError(t, err)
		assert.Equal(t, "new", name)
	})

	t.Run("delete when file exists", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, profiles.WriteActiveProfile(dir, "work"))
		require.NoError(t, profiles.DeleteActiveProfile(dir))

		name, err := profiles.ReadActiveProfile(dir)
		require.NoError(t, err)
		assert.Equal(t, "", name)
	})

	t.Run("delete when file does not exist", func(t *testing.T) {
		dir := t.TempDir()
		err := profiles.DeleteActiveProfile(dir)
		require.NoError(t, err) // should not error
	})
}

func TestProfileSummary(t *testing.T) {
	t.Run("all parts", func(t *testing.T) {
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add:    []string{"a@m", "b@m"},
				Remove: []string{"c@m"},
			},
			Settings: map[string]any{"model": "sonnet"},
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{
					"PreCompact": config.ExpandHookCommand("lint"),
				},
				Remove: []string{"SessionStart"},
			},
		}
		summary := profiles.ProfileSummary(p)
		assert.Contains(t, summary, "+2 plugins")
		assert.Contains(t, summary, "-1 plugin")
		assert.Contains(t, summary, "model \u2192 sonnet")
		assert.Contains(t, summary, "+1 hook")
		assert.Contains(t, summary, "-1 hook")
	})

	t.Run("plugins only", func(t *testing.T) {
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add: []string{"a@m"},
			},
		}
		assert.Equal(t, "+1 plugin", profiles.ProfileSummary(p))
	})

	t.Run("settings only", func(t *testing.T) {
		p := profiles.Profile{
			Settings: map[string]any{"model": "opus"},
		}
		assert.Equal(t, "model \u2192 opus", profiles.ProfileSummary(p))
	})

	t.Run("multiple settings sorted", func(t *testing.T) {
		p := profiles.Profile{
			Settings: map[string]any{"model": "opus", "autoCompact": true},
		}
		summary := profiles.ProfileSummary(p)
		// autoCompact comes before model alphabetically.
		assert.Equal(t, "autoCompact \u2192 true, model \u2192 opus", summary)
	})

	t.Run("empty profile", func(t *testing.T) {
		p := profiles.Profile{}
		assert.Equal(t, "no changes", profiles.ProfileSummary(p))
	})

	t.Run("hooks only", func(t *testing.T) {
		p := profiles.Profile{
			Hooks: profiles.ProfileHooks{
				Add: map[string]json.RawMessage{
					"A": config.ExpandHookCommand("a"),
					"B": config.ExpandHookCommand("b"),
				},
			},
		}
		assert.Equal(t, "+2 hooks", profiles.ProfileSummary(p))
	})

	t.Run("singular vs plural", func(t *testing.T) {
		p := profiles.Profile{
			Plugins: profiles.ProfilePlugins{
				Add: []string{"a@m"},
			},
			Hooks: profiles.ProfileHooks{
				Remove: []string{"X", "Y"},
			},
		}
		summary := profiles.ProfileSummary(p)
		assert.Contains(t, summary, "+1 plugin")
		assert.Contains(t, summary, "-2 hooks")
	})

	t.Run("permissions in summary", func(t *testing.T) {
		p := profiles.Profile{
			Permissions: profiles.ProfilePermissions{
				AddAllow: []string{"Bash(git *)", "Read"},
				AddDeny:  []string{"Bash(rm *)"},
			},
		}
		summary := profiles.ProfileSummary(p)
		assert.Contains(t, summary, "+2 allow permissions")
		assert.Contains(t, summary, "+1 deny permission")
	})

	t.Run("claude_md in summary", func(t *testing.T) {
		p := profiles.Profile{
			ClaudeMD: profiles.ProfileClaudeMD{
				Add:    []string{"a.md", "b.md"},
				Remove: []string{"c.md"},
			},
		}
		summary := profiles.ProfileSummary(p)
		assert.Contains(t, summary, "+2 claude_md includes")
		assert.Contains(t, summary, "-1 claude_md include")
	})

	t.Run("mcp in summary", func(t *testing.T) {
		p := profiles.Profile{
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{
					"pg": json.RawMessage(`{"command":"npx"}`),
				},
				Remove: []string{"old-server"},
			},
		}
		summary := profiles.ProfileSummary(p)
		assert.Contains(t, summary, "+1 mcp server")
		assert.Contains(t, summary, "-1 mcp server")
	})

	t.Run("keybindings in summary", func(t *testing.T) {
		p := profiles.Profile{
			Keybindings: profiles.ProfileKeybindings{
				Override: map[string]any{"ctrl+k": "clear", "ctrl+l": "redraw"},
			},
		}
		summary := profiles.ProfileSummary(p)
		assert.Contains(t, summary, "2 keybinding overrides")
	})
}

func TestMergePermissions(t *testing.T) {
	t.Run("appends without duplicates", func(t *testing.T) {
		base := config.Permissions{
			Allow: []string{"Bash(git *)", "Read"},
			Deny:  []string{"Bash(rm *)"},
		}
		p := profiles.Profile{
			Permissions: profiles.ProfilePermissions{
				AddAllow: []string{"Write", "Read"}, // Read is dup
				AddDeny:  []string{"Bash(sudo *)"},
			},
		}
		result := profiles.MergePermissions(base, p)
		assert.Equal(t, []string{"Bash(git *)", "Read", "Write"}, result.Allow)
		assert.Equal(t, []string{"Bash(rm *)", "Bash(sudo *)"}, result.Deny)
	})

	t.Run("nil base returns adds", func(t *testing.T) {
		p := profiles.Profile{
			Permissions: profiles.ProfilePermissions{
				AddAllow: []string{"Read"},
			},
		}
		result := profiles.MergePermissions(config.Permissions{}, p)
		assert.Equal(t, []string{"Read"}, result.Allow)
		assert.Empty(t, result.Deny)
	})

	t.Run("empty profile returns base", func(t *testing.T) {
		base := config.Permissions{Allow: []string{"Read"}}
		result := profiles.MergePermissions(base, profiles.Profile{})
		assert.Equal(t, []string{"Read"}, result.Allow)
	})
}

func TestMergeClaudeMD(t *testing.T) {
	t.Run("add and remove", func(t *testing.T) {
		base := []string{"a.md", "b.md"}
		p := profiles.Profile{
			ClaudeMD: profiles.ProfileClaudeMD{
				Add:    []string{"c.md"},
				Remove: []string{"a.md"},
			},
		}
		result := profiles.MergeClaudeMD(base, p)
		assert.Equal(t, []string{"b.md", "c.md"}, result)
	})

	t.Run("no duplicates from add", func(t *testing.T) {
		base := []string{"a.md", "b.md"}
		p := profiles.Profile{
			ClaudeMD: profiles.ProfileClaudeMD{
				Add: []string{"b.md", "c.md"},
			},
		}
		result := profiles.MergeClaudeMD(base, p)
		assert.Equal(t, []string{"a.md", "b.md", "c.md"}, result)
	})

	t.Run("empty profile returns base", func(t *testing.T) {
		base := []string{"a.md"}
		result := profiles.MergeClaudeMD(base, profiles.Profile{})
		assert.Equal(t, []string{"a.md"}, result)
	})

	t.Run("nil base with adds", func(t *testing.T) {
		p := profiles.Profile{
			ClaudeMD: profiles.ProfileClaudeMD{
				Add: []string{"a.md"},
			},
		}
		result := profiles.MergeClaudeMD(nil, p)
		assert.Equal(t, []string{"a.md"}, result)
	})
}

func TestMergeMCP(t *testing.T) {
	t.Run("add and remove", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"pg":    json.RawMessage(`{"command":"npx"}`),
			"redis": json.RawMessage(`{"command":"redis-cli"}`),
		}
		p := profiles.Profile{
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{
					"mongo": json.RawMessage(`{"command":"mongosh"}`),
				},
				Remove: []string{"redis"},
			},
		}
		result := profiles.MergeMCP(base, p)
		assert.Len(t, result, 2)
		assert.Contains(t, result, "pg")
		assert.Contains(t, result, "mongo")
		assert.NotContains(t, result, "redis")
	})

	t.Run("add overrides existing", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"pg": json.RawMessage(`{"command":"old"}`),
		}
		p := profiles.Profile{
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{
					"pg": json.RawMessage(`{"command":"new"}`),
				},
			},
		}
		result := profiles.MergeMCP(base, p)
		assert.JSONEq(t, `{"command":"new"}`, string(result["pg"]))
	})

	t.Run("nil base returns adds only", func(t *testing.T) {
		p := profiles.Profile{
			MCP: profiles.ProfileMCP{
				Add: map[string]json.RawMessage{
					"pg": json.RawMessage(`{"command":"npx"}`),
				},
			},
		}
		result := profiles.MergeMCP(nil, p)
		assert.Len(t, result, 1)
		assert.Contains(t, result, "pg")
	})

	t.Run("empty profile returns base copy", func(t *testing.T) {
		base := map[string]json.RawMessage{
			"pg": json.RawMessage(`{"command":"npx"}`),
		}
		result := profiles.MergeMCP(base, profiles.Profile{})
		assert.Len(t, result, 1)
	})
}

func TestMergeKeybindings(t *testing.T) {
	t.Run("override existing and add new", func(t *testing.T) {
		base := map[string]any{"ctrl+k": "clear", "ctrl+l": "redraw"}
		p := profiles.Profile{
			Keybindings: profiles.ProfileKeybindings{
				Override: map[string]any{"ctrl+k": "new-clear", "ctrl+j": "jump"},
			},
		}
		result := profiles.MergeKeybindings(base, p)
		assert.Equal(t, "new-clear", result["ctrl+k"])
		assert.Equal(t, "redraw", result["ctrl+l"])
		assert.Equal(t, "jump", result["ctrl+j"])
	})

	t.Run("nil base returns overrides", func(t *testing.T) {
		p := profiles.Profile{
			Keybindings: profiles.ProfileKeybindings{
				Override: map[string]any{"ctrl+k": "clear"},
			},
		}
		result := profiles.MergeKeybindings(nil, p)
		assert.Equal(t, "clear", result["ctrl+k"])
	})

	t.Run("empty profile returns base copy", func(t *testing.T) {
		base := map[string]any{"ctrl+k": "clear"}
		result := profiles.MergeKeybindings(base, profiles.Profile{})
		assert.Equal(t, "clear", result["ctrl+k"])
	})

	t.Run("both nil returns nil", func(t *testing.T) {
		result := profiles.MergeKeybindings(nil, profiles.Profile{})
		assert.Nil(t, result)
	})
}

func TestParseProfile_Permissions(t *testing.T) {
	input := []byte(`permissions:
  add_allow:
    - Bash(git *)
    - Read
  add_deny:
    - Bash(rm *)
`)
	p, err := profiles.ParseProfile(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"Bash(git *)", "Read"}, p.Permissions.AddAllow)
	assert.Equal(t, []string{"Bash(rm *)"}, p.Permissions.AddDeny)
}

func TestParseProfile_ClaudeMD(t *testing.T) {
	input := []byte(`claude_md:
  add:
    - shared/coding-standards.md
  remove:
    - old/deprecated.md
`)
	p, err := profiles.ParseProfile(input)
	require.NoError(t, err)
	assert.Equal(t, []string{"shared/coding-standards.md"}, p.ClaudeMD.Add)
	assert.Equal(t, []string{"old/deprecated.md"}, p.ClaudeMD.Remove)
}

func TestParseProfile_MCP(t *testing.T) {
	input := []byte(`mcp:
  add:
    postgres:
      command: npx
      args:
        - -y
        - server-postgres
  remove:
    - old-server
`)
	p, err := profiles.ParseProfile(input)
	require.NoError(t, err)
	require.Contains(t, p.MCP.Add, "postgres")
	var val map[string]any
	require.NoError(t, json.Unmarshal(p.MCP.Add["postgres"], &val))
	assert.Equal(t, "npx", val["command"])
	assert.Equal(t, []string{"old-server"}, p.MCP.Remove)
}

func TestParseProfile_Keybindings(t *testing.T) {
	input := []byte(`keybindings:
  override:
    ctrl+k: clear
    ctrl+l: redraw
`)
	p, err := profiles.ParseProfile(input)
	require.NoError(t, err)
	assert.Equal(t, "clear", p.Keybindings.Override["ctrl+k"])
	assert.Equal(t, "redraw", p.Keybindings.Override["ctrl+l"])
}

func TestMarshalProfile_Permissions(t *testing.T) {
	original := profiles.Profile{
		Permissions: profiles.ProfilePermissions{
			AddAllow: []string{"Read", "Write"},
			AddDeny:  []string{"Bash(rm *)"},
		},
	}
	data, err := profiles.MarshalProfile(original)
	require.NoError(t, err)

	parsed, err := profiles.ParseProfile(data)
	require.NoError(t, err)
	assert.Equal(t, original.Permissions.AddAllow, parsed.Permissions.AddAllow)
	assert.Equal(t, original.Permissions.AddDeny, parsed.Permissions.AddDeny)
}

func TestMarshalProfile_ClaudeMD(t *testing.T) {
	original := profiles.Profile{
		ClaudeMD: profiles.ProfileClaudeMD{
			Add:    []string{"a.md", "b.md"},
			Remove: []string{"c.md"},
		},
	}
	data, err := profiles.MarshalProfile(original)
	require.NoError(t, err)

	parsed, err := profiles.ParseProfile(data)
	require.NoError(t, err)
	assert.Equal(t, original.ClaudeMD.Add, parsed.ClaudeMD.Add)
	assert.Equal(t, original.ClaudeMD.Remove, parsed.ClaudeMD.Remove)
}

func TestMarshalProfile_MCP(t *testing.T) {
	original := profiles.Profile{
		MCP: profiles.ProfileMCP{
			Add: map[string]json.RawMessage{
				"pg": json.RawMessage(`{"command":"npx"}`),
			},
			Remove: []string{"old-server"},
		},
	}
	data, err := profiles.MarshalProfile(original)
	require.NoError(t, err)

	parsed, err := profiles.ParseProfile(data)
	require.NoError(t, err)
	require.Contains(t, parsed.MCP.Add, "pg")
	assert.JSONEq(t, `{"command":"npx"}`, string(parsed.MCP.Add["pg"]))
	assert.Equal(t, []string{"old-server"}, parsed.MCP.Remove)
}

func TestMarshalProfile_Keybindings(t *testing.T) {
	original := profiles.Profile{
		Keybindings: profiles.ProfileKeybindings{
			Override: map[string]any{"ctrl+k": "clear"},
		},
	}
	data, err := profiles.MarshalProfile(original)
	require.NoError(t, err)

	parsed, err := profiles.ParseProfile(data)
	require.NoError(t, err)
	assert.Equal(t, "clear", parsed.Keybindings.Override["ctrl+k"])
}
