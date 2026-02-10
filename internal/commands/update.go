package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/plugins"
)

// UpstreamStatus describes an upstream plugin and its installed version.
type UpstreamStatus struct {
	Key              string
	InstalledVersion string
}

// PinnedStatus describes a pinned plugin, its pinned version, and its installed version.
type PinnedStatus struct {
	Key              string
	PinnedVersion    string
	InstalledVersion string
}

// ForkedStatus describes a forked plugin by name.
type ForkedStatus struct {
	Name string
}

// UpdateResult holds the categorized update check results.
type UpdateResult struct {
	UpstreamPlugins []UpstreamStatus
	PinnedPlugins   []PinnedStatus
	ForkedPlugins   []ForkedStatus
}

// HasUpdates returns true if any category is non-empty.
func (r *UpdateResult) HasUpdates() bool {
	return len(r.UpstreamPlugins) > 0 || len(r.PinnedPlugins) > 0 || len(r.ForkedPlugins) > 0
}

// UpdateCheck reads the v2 config and installed_plugins.json, then categorizes
// all plugins with their installed versions.
func UpdateCheck(claudeDir, syncDir string) (*UpdateResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}

	installed, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	result := &UpdateResult{}

	// Build a lookup of installed plugin versions.
	installedVersions := make(map[string]string)
	for key, installs := range installed.Plugins {
		if len(installs) > 0 {
			installedVersions[key] = installs[0].Version
		}
	}

	// Categorize upstream plugins.
	for _, key := range cfg.Upstream {
		status := UpstreamStatus{
			Key:              key,
			InstalledVersion: installedVersions[key],
		}
		result.UpstreamPlugins = append(result.UpstreamPlugins, status)
	}

	// Categorize pinned plugins.
	for key, pinnedVersion := range cfg.Pinned {
		status := PinnedStatus{
			Key:              key,
			PinnedVersion:    pinnedVersion,
			InstalledVersion: installedVersions[key],
		}
		result.PinnedPlugins = append(result.PinnedPlugins, status)
	}

	// Categorize forked plugins.
	for _, name := range cfg.Forked {
		result.ForkedPlugins = append(result.ForkedPlugins, ForkedStatus{Name: name})
	}

	return result, nil
}

// UpdateApply reinstalls the given upstream/pinned plugin keys.
// It registers the local marketplace if forked plugins exist in the config.
// Returns slices of successfully installed and failed plugin keys.
func UpdateApply(claudeDir, syncDir string, pluginKeys []string, quiet bool) (installed, failed []string) {
	for _, key := range pluginKeys {
		if !quiet {
			fmt.Printf("  Reinstalling %s...\n", key)
		}
		if err := installPlugin(key); err != nil {
			failed = append(failed, key)
			if !quiet {
				fmt.Fprintf(os.Stderr, "  Failed: %s: %v\n", key, err)
			}
		} else {
			installed = append(installed, key)
			if !quiet {
				fmt.Printf("  Updated: %s\n", key)
			}
		}
	}

	// Register or unregister local marketplace based on forked plugins in config.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err == nil {
		cfg, err := config.Parse(cfgData)
		if err == nil {
			if len(cfg.Forked) > 0 {
				_ = plugins.RegisterLocalMarketplace(claudeDir, syncDir)
			} else {
				_ = plugins.UnregisterLocalMarketplace(claudeDir)
			}
		}
	}

	return installed, failed
}

// UpdateForkedPlugins reinstalls forked plugins using the local marketplace key format.
// Returns slices of successfully installed and failed plugin names.
func UpdateForkedPlugins(claudeDir, syncDir string, forkNames []string, quiet bool) (installed, failed []string) {
	if len(forkNames) > 0 {
		if err := plugins.RegisterLocalMarketplace(claudeDir, syncDir); err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Warning: failed to register local marketplace: %v\n", err)
			}
		}
	}

	for _, name := range forkNames {
		key := plugins.ForkedPluginKey(name)
		if !quiet {
			fmt.Printf("  Reinstalling forked plugin %s...\n", name)
		}
		if err := installPlugin(key); err != nil {
			failed = append(failed, name)
			if !quiet {
				fmt.Fprintf(os.Stderr, "  Failed: %s: %v\n", name, err)
			}
		} else {
			installed = append(installed, name)
			if !quiet {
				fmt.Printf("  Updated: %s\n", name)
			}
		}
	}

	return installed, failed
}
