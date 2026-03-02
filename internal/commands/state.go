package commands

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// PluginInfo describes a plugin for menu display.
type PluginInfo struct {
	Key        string
	Name       string
	Status     string // "upstream", "pinned", or "forked"
	PinVersion string
}

// ProjectInfo describes a managed project for menu display.
type ProjectInfo struct {
	Path    string
	Profile string
}

// MenuState holds the detected state used to build the TUI menu.
type MenuState struct {
	ConfigExists  bool
	HasPending    bool
	HasConflicts  bool
	Profiles      []string
	ActiveProfile string
	Plugins       []PluginInfo
	Projects      []ProjectInfo
}

// DetectMenuState checks the current claude-sync state for menu rendering.
// It is designed to be fast and never error — unknown state defaults to false/empty.
func DetectMenuState(claudeDir, syncDir string) MenuState {
	var state MenuState

	// Check if sync dir exists (i.e., claude-sync is initialized)
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return state
	}
	state.ConfigExists = true

	// Check for pending approval changes
	pending, err := approval.ReadPending(syncDir)
	if err == nil && !pending.IsEmpty() {
		state.HasPending = true
	}

	// Check for merge conflicts
	state.HasConflicts = HasPendingConflicts(syncDir)

	// Check for profiles
	profileList, err := profiles.ListProfiles(syncDir)
	if err == nil {
		state.Profiles = profileList
	}

	// Check active profile
	active, err := profiles.ReadActiveProfile(syncDir)
	if err == nil {
		state.ActiveProfile = active
	}

	// Parse plugins from config
	cfgPath := filepath.Join(syncDir, "config.yaml")
	if cfgData, readErr := os.ReadFile(cfgPath); readErr == nil {
		if cfg, parseErr := config.Parse(cfgData); parseErr == nil {
			state.Plugins = buildPluginInfos(cfg)
		}
	}

	// Scan for managed projects
	if entries, scanErr := ProjectListScan(DefaultProjectSearchDirs()); scanErr == nil {
		for _, e := range entries {
			state.Projects = append(state.Projects, ProjectInfo{
				Path:    e.Path,
				Profile: e.Profile,
			})
		}
	}

	return state
}

// buildPluginInfos constructs a PluginInfo slice from a parsed config.
func buildPluginInfos(cfg config.Config) []PluginInfo {
	var infos []PluginInfo

	for _, key := range cfg.Upstream {
		infos = append(infos, PluginInfo{
			Key:    key,
			Name:   pluginNameFromKey(key),
			Status: "upstream",
		})
	}

	pinnedKeys := make([]string, 0, len(cfg.Pinned))
	for key := range cfg.Pinned {
		pinnedKeys = append(pinnedKeys, key)
	}
	sort.Strings(pinnedKeys)
	for _, key := range pinnedKeys {
		infos = append(infos, PluginInfo{
			Key:        key,
			Name:       pluginNameFromKey(key),
			Status:     "pinned",
			PinVersion: cfg.Pinned[key],
		})
	}

	for _, name := range cfg.Forked {
		key := name + "@" + config.ForkedMarketplace
		infos = append(infos, PluginInfo{
			Key:    key,
			Name:   name,
			Status: "forked",
		})
	}

	return infos
}

// pluginNameFromKey extracts the plugin name from a "name@marketplace" key.
func pluginNameFromKey(key string) string {
	parts := strings.SplitN(key, "@", 2)
	return parts[0]
}
