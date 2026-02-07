package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// Fields from settings.json that should NOT be synced.
var excludedSettingsFields = map[string]bool{
	"enabledPlugins": true,
	"statusLine":     true,
	"permissions":    true,
}

// Init scans the current Claude Code setup and creates ~/.claude-sync/config.yaml.
func Init(claudeDir, syncDir string) error {
	if !claudecode.DirExists(claudeDir) {
		return fmt.Errorf("Claude Code directory not found at %s. Run Claude Code at least once first", claudeDir)
	}

	if _, err := os.Stat(syncDir); err == nil {
		return fmt.Errorf("%s already exists. Run 'claude-sync pull' to update, or remove it first", syncDir)
	}

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return fmt.Errorf("reading plugins: %w", err)
	}

	pluginKeys := plugins.PluginKeys()
	sort.Strings(pluginKeys)

	syncedSettings := make(map[string]any)
	syncedHooks := make(map[string]string)

	settingsRaw, err := claudecode.ReadSettings(claudeDir)
	if err == nil {
		if model, ok := settingsRaw["model"]; ok {
			var m string
			json.Unmarshal(model, &m)
			if m != "" {
				syncedSettings["model"] = m
			}
		}

		if hooksRaw, ok := settingsRaw["hooks"]; ok {
			var hooks map[string]json.RawMessage
			if json.Unmarshal(hooksRaw, &hooks) == nil {
				for hookName, hookData := range hooks {
					cmd := extractHookCommand(hookData)
					if cmd != "" {
						syncedHooks[hookName] = cmd
					}
				}
			}
		}
	}

	cfg := config.Config{
		Version:  "2.0.0",
		Upstream: pluginKeys,
		Pinned:   map[string]string{},
		Settings: syncedSettings,
		Hooks:    syncedHooks,
	}

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		return fmt.Errorf("creating sync directory: %w", err)
	}

	cfgData, err := config.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	gitignore := "user-preferences.yaml\n.last_fetch\n"
	if err := os.WriteFile(filepath.Join(syncDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return fmt.Errorf("writing .gitignore: %w", err)
	}

	if err := git.Init(syncDir); err != nil {
		return fmt.Errorf("initializing git repo: %w", err)
	}

	if err := git.Add(syncDir, "."); err != nil {
		return fmt.Errorf("staging files: %w", err)
	}
	if err := git.Commit(syncDir, "Initial claude-sync config"); err != nil {
		return fmt.Errorf("creating initial commit: %w", err)
	}

	return nil
}

func extractHookCommand(data json.RawMessage) string {
	var hookEntries []struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(data, &hookEntries) != nil {
		return ""
	}
	if len(hookEntries) > 0 && len(hookEntries[0].Hooks) > 0 {
		return hookEntries[0].Hooks[0].Command
	}
	return ""
}
