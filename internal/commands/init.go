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
	forkedplugins "github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// InitScanResult holds what was found during scanning without writing anything.
type InitScanResult struct {
	PluginKeys []string          // all non-local plugin keys
	Upstream   []string          // portable marketplace plugins
	AutoForked []string          // non-portable plugins that would be forked
	Skipped    []string          // local-scope plugins
	Settings   map[string]any                // syncable settings found
	Hooks      map[string]json.RawMessage // hooks found (hookName -> raw JSON)
}

// InitResult describes how plugins were categorized during init.
type InitResult struct {
	Upstream         []string // portable marketplace plugins
	AutoForked       []string // non-portable plugins copied into sync repo
	Skipped          []string // local-scope plugins excluded entirely
	ExcludedPlugins  []string // plugins excluded by user selection
	RemotePushed     bool     // whether the initial commit was pushed to a remote
	IncludedSettings []string // settings keys written to config
	IncludedHooks    []string // hook names written to config
	ProfileNames     []string // profiles created
	ActiveProfile    string   // profile activated on this machine
}

// InitOptions configures what Init includes in the sync config.
type InitOptions struct {
	ClaudeDir       string
	SyncDir         string
	RemoteURL       string
	IncludeSettings bool                       // whether to write settings to config
	IncludeHooks    map[string]json.RawMessage // specific hooks to include (nil = all, empty = none)
	IncludePlugins  []string                   // specific plugin keys to include (nil = all, empty = none)
	Profiles        map[string]profiles.Profile // nil = no profiles, non-nil = write profile files
	ActiveProfile   string                      // profile to activate on this machine (empty = none)
}

// Fields from settings.json that should NOT be synced.
var excludedSettingsFields = map[string]bool{
	"enabledPlugins": true,
	"statusLine":     true,
	"permissions":    true,
}

// InitScan reads the current Claude Code setup and returns what was found
// without writing anything. Use this for the interactive selection phase.
func InitScan(claudeDir string) (*InitScanResult, error) {
	if !claudecode.DirExists(claudeDir) {
		return nil, fmt.Errorf("Claude Code directory not found at %s. Run Claude Code at least once first", claudeDir)
	}

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading plugins: %w", err)
	}

	pluginKeys := plugins.PluginKeys()
	sort.Strings(pluginKeys)

	result := &InitScanResult{
		Settings: make(map[string]any),
		Hooks:    make(map[string]json.RawMessage),
	}

	for _, key := range pluginKeys {
		installations := plugins.Plugins[key]
		if isLocalScope(installations) {
			result.Skipped = append(result.Skipped, key)
			continue
		}

		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			result.Upstream = append(result.Upstream, key)
			result.PluginKeys = append(result.PluginKeys, key)
			continue
		}
		_, mkt := parts[0], parts[1]

		// Skip plugins from our own managed marketplace — these are
		// artifacts of a previous init and shouldn't be re-scanned.
		if mkt == forkedplugins.MarketplaceName {
			continue
		}

		if marketplace.IsPortable(claudeDir, mkt) {
			result.Upstream = append(result.Upstream, key)
		} else {
			installPath := findInstallPath(installations)
			if installPath == "" {
				result.Upstream = append(result.Upstream, key)
			} else {
				result.AutoForked = append(result.AutoForked, key)
			}
		}
		result.PluginKeys = append(result.PluginKeys, key)
	}

	settingsRaw, err := claudecode.ReadSettings(claudeDir)
	if err == nil {
		if model, ok := settingsRaw["model"]; ok {
			var m string
			json.Unmarshal(model, &m)
			if m != "" {
				result.Settings["model"] = m
			}
		}

		if hooksRaw, ok := settingsRaw["hooks"]; ok {
			var hooks map[string]json.RawMessage
			if json.Unmarshal(hooksRaw, &hooks) == nil {
				for hookName, hookData := range hooks {
					result.Hooks[hookName] = hookData
				}
			}
		}
	}

	return result, nil
}

// Init scans the current Claude Code setup and creates ~/.claude-sync/config.yaml.
// If RemoteURL is non-empty, it adds the remote and pushes after the initial commit.
func Init(opts InitOptions) (*InitResult, error) {
	claudeDir := opts.ClaudeDir
	syncDir := opts.SyncDir

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

	result := &InitResult{}
	var upstream []string
	var forkedNames []string

	// Build plugin include set for filtering.
	var includeSet map[string]bool
	if opts.IncludePlugins != nil {
		includeSet = make(map[string]bool, len(opts.IncludePlugins))
		for _, k := range opts.IncludePlugins {
			includeSet[k] = true
		}
	}

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		return nil, fmt.Errorf("creating sync directory: %w", err)
	}

	for _, key := range pluginKeys {
		installations := plugins.Plugins[key]
		if isLocalScope(installations) {
			result.Skipped = append(result.Skipped, key)
			continue
		}

		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			// Check plugin filter before including.
			if includeSet != nil && !includeSet[key] {
				result.ExcludedPlugins = append(result.ExcludedPlugins, key)
				continue
			}
			upstream = append(upstream, key)
			result.Upstream = append(result.Upstream, key)
			continue
		}
		name, mkt := parts[0], parts[1]

		// Skip plugins from our own managed marketplace — these are
		// artifacts of a previous init and shouldn't be re-scanned.
		if mkt == forkedplugins.MarketplaceName {
			continue
		}

		// Check plugin filter before including.
		if includeSet != nil && !includeSet[key] {
			result.ExcludedPlugins = append(result.ExcludedPlugins, key)
			continue
		}

		if marketplace.IsPortable(claudeDir, mkt) {
			upstream = append(upstream, key)
			result.Upstream = append(result.Upstream, key)
		} else {
			installPath := findInstallPath(installations)
			if installPath == "" {
				upstream = append(upstream, key)
				result.Upstream = append(result.Upstream, key)
				continue
			}

			dstDir := filepath.Join(syncDir, "plugins", name)
			if err := copyDir(installPath, dstDir); err != nil {
				upstream = append(upstream, key)
				result.Upstream = append(result.Upstream, key)
				continue
			}

			forkedNames = append(forkedNames, name)
			result.AutoForked = append(result.AutoForked, key)
		}
	}

	// Scan settings and hooks from Claude Code.
	syncedSettings := make(map[string]any)
	syncedHooks := make(map[string]json.RawMessage)

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
					syncedHooks[hookName] = hookData
				}
			}
		}
	}

	// Apply settings filter.
	var cfgSettings map[string]any
	if opts.IncludeSettings {
		cfgSettings = syncedSettings
		for k := range syncedSettings {
			result.IncludedSettings = append(result.IncludedSettings, k)
		}
		sort.Strings(result.IncludedSettings)
	}

	// Apply hooks filter.
	var cfgHooks map[string]json.RawMessage
	if opts.IncludeHooks == nil {
		// nil = include all hooks (backward compat).
		cfgHooks = syncedHooks
		for k := range syncedHooks {
			result.IncludedHooks = append(result.IncludedHooks, k)
		}
	} else {
		// Non-nil = include only specified hooks.
		cfgHooks = make(map[string]json.RawMessage)
		for k, v := range opts.IncludeHooks {
			cfgHooks[k] = v
			result.IncludedHooks = append(result.IncludedHooks, k)
		}
	}
	sort.Strings(result.IncludedHooks)

	cfg := config.Config{
		Version:  "2.1.0",
		Upstream: upstream,
		Pinned:   map[string]string{},
		Forked:   forkedNames,
		Settings: cfgSettings,
		Hooks:    cfgHooks,
	}

	cfgData, err := config.Marshal(cfg)
	if err != nil {
		return nil, fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644); err != nil {
		return nil, fmt.Errorf("writing config: %w", err)
	}

	gitignore := "user-preferences.yaml\n.last_fetch\nplugins/.claude-plugin/\nactive-profile\n"
	if err := os.WriteFile(filepath.Join(syncDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return nil, fmt.Errorf("writing .gitignore: %w", err)
	}

	// Write profile files if provided.
	if len(opts.Profiles) > 0 {
		profilesDir := filepath.Join(syncDir, "profiles")
		if err := os.MkdirAll(profilesDir, 0755); err != nil {
			return nil, fmt.Errorf("creating profiles directory: %w", err)
		}

		var profileNames []string
		for name := range opts.Profiles {
			profileNames = append(profileNames, name)
		}
		sort.Strings(profileNames)

		for _, name := range profileNames {
			data, err := profiles.MarshalProfile(opts.Profiles[name])
			if err != nil {
				return nil, fmt.Errorf("marshaling profile %q: %w", name, err)
			}
			if err := os.WriteFile(filepath.Join(profilesDir, name+".yaml"), data, 0644); err != nil {
				return nil, fmt.Errorf("writing profile %q: %w", name, err)
			}
		}
		result.ProfileNames = profileNames
	}

	// Write active profile if specified.
	if opts.ActiveProfile != "" {
		if err := profiles.WriteActiveProfile(syncDir, opts.ActiveProfile); err != nil {
			return nil, fmt.Errorf("writing active profile: %w", err)
		}
	}
	result.ActiveProfile = opts.ActiveProfile

	if err := git.Init(syncDir); err != nil {
		return nil, fmt.Errorf("initializing git repo: %w", err)
	}

	if err := git.Add(syncDir, "."); err != nil {
		return nil, fmt.Errorf("staging files: %w", err)
	}
	if err := git.Commit(syncDir, "Initial claude-sync config"); err != nil {
		return nil, fmt.Errorf("creating initial commit: %w", err)
	}

	if len(forkedNames) > 0 {
		if err := forkedplugins.RegisterLocalMarketplace(claudeDir, syncDir); err != nil {
			return nil, fmt.Errorf("registering local marketplace: %w", err)
		}
	}

	if opts.RemoteURL != "" {
		if err := git.RemoteAdd(syncDir, "origin", opts.RemoteURL); err != nil {
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

// ExtractHookCommand extracts the first command string from a hook's raw JSON data.
func ExtractHookCommand(data json.RawMessage) string {
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
