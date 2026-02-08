package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/marketplace"
)

// InitResult describes how plugins were categorized during init.
type InitResult struct {
	Upstream     []string // portable marketplace plugins
	AutoForked   []string // non-portable plugins copied into sync repo
	Skipped      []string // local-scope plugins excluded entirely
	RemotePushed bool     // whether the initial commit was pushed to a remote
}

// Fields from settings.json that should NOT be synced.
var excludedSettingsFields = map[string]bool{
	"enabledPlugins": true,
	"statusLine":     true,
	"permissions":    true,
}

// Init scans the current Claude Code setup and creates ~/.claude-sync/config.yaml.
// If remoteURL is non-empty, it adds the remote and pushes after the initial commit.
func Init(claudeDir, syncDir, remoteURL string) (*InitResult, error) {
	if !claudecode.DirExists(claudeDir) {
		return nil, fmt.Errorf("Claude Code directory not found at %s. Run Claude Code at least once first", claudeDir)
	}

	if _, err := os.Stat(syncDir); err == nil {
		return nil, fmt.Errorf("%s already exists. Run 'claude-sync pull' to update, or remove it first", syncDir)
	}

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading plugins: %w", err)
	}

	pluginKeys := plugins.PluginKeys()
	sort.Strings(pluginKeys)

	// Categorize plugins based on scope and marketplace portability.
	result := &InitResult{}
	var upstream []string
	var forkedNames []string

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		return nil, fmt.Errorf("creating sync directory: %w", err)
	}

	for _, key := range pluginKeys {
		// Check if any installation has local scope — skip those entirely.
		installations := plugins.Plugins[key]
		if isLocalScope(installations) {
			result.Skipped = append(result.Skipped, key)
			continue
		}

		// Split key into name@marketplace.
		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			upstream = append(upstream, key)
			result.Upstream = append(result.Upstream, key)
			continue
		}
		name, mkt := parts[0], parts[1]

		if marketplace.IsPortableMarketplace(mkt) {
			upstream = append(upstream, key)
			result.Upstream = append(result.Upstream, key)
		} else {
			// Auto-fork: copy plugin files into sync repo.
			installPath := findInstallPath(installations)
			if installPath == "" {
				// No install path — fall back to upstream.
				upstream = append(upstream, key)
				result.Upstream = append(result.Upstream, key)
				continue
			}

			dstDir := filepath.Join(syncDir, "plugins", name)
			if err := copyDir(installPath, dstDir); err != nil {
				// If copy fails, fall back to upstream.
				upstream = append(upstream, key)
				result.Upstream = append(result.Upstream, key)
				continue
			}

			forkedNames = append(forkedNames, name)
			result.AutoForked = append(result.AutoForked, key)
		}
	}

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
		Upstream: upstream,
		Pinned:   map[string]string{},
		Forked:   forkedNames,
		Settings: syncedSettings,
		Hooks:    syncedHooks,
	}

	cfgData, err := config.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	gitignore := "user-preferences.yaml\n.last_fetch\n"
	if err := os.WriteFile(filepath.Join(syncDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return nil, fmt.Errorf("writing .gitignore: %w", err)
	}

	if err := git.Init(syncDir); err != nil {
		return nil, fmt.Errorf("initializing git repo: %w", err)
	}

	if err := git.Add(syncDir, "."); err != nil {
		return nil, fmt.Errorf("staging files: %w", err)
	}
	if err := git.Commit(syncDir, "Initial claude-sync config"); err != nil {
		return nil, fmt.Errorf("creating initial commit: %w", err)
	}

	if remoteURL != "" {
		if err := git.RemoteAdd(syncDir, "origin", remoteURL); err != nil {
			return nil, fmt.Errorf("adding remote: %w", err)
		}
		branch, err := git.CurrentBranch(syncDir)
		if err != nil {
			return nil, fmt.Errorf("detecting branch: %w", err)
		}
		if err := git.PushWithUpstream(syncDir, "origin", branch); err != nil {
			return nil, fmt.Errorf("pushing to remote: %w", err)
		}
		result.RemotePushed = true
	}

	return result, nil
}

// isLocalScope returns true if any installation has scope "local".
func isLocalScope(installations []claudecode.PluginInstallation) bool {
	for _, inst := range installations {
		if inst.Scope == "local" {
			return true
		}
	}
	return false
}

// findInstallPath returns the first non-empty InstallPath from the installations.
func findInstallPath(installations []claudecode.PluginInstallation) string {
	for _, inst := range installations {
		if inst.InstallPath != "" {
			return inst.InstallPath
		}
	}
	return ""
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
