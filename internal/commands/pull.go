package commands

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/plugins"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

type PullResult struct {
	ToInstall        []string
	ToRemove         []string
	Synced           []string
	Untracked        []string
	EffectiveDesired []string
	Installed        []string
	Failed           []string
}

func PullDryRun(claudeDir, syncDir string) (*PullResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, err
	}

	// Register local marketplace if forked plugins exist.
	forks, _ := plugins.ListForkedPlugins(syncDir)
	if len(forks) > 0 {
		if err := plugins.RegisterLocalMarketplace(claudeDir, syncDir); err != nil {
			return nil, fmt.Errorf("registering local marketplace: %w", err)
		}
	}

	// Build complete desired list from all categories.
	var allDesired []string
	allDesired = append(allDesired, cfg.Upstream...)
	for k := range cfg.Pinned {
		allDesired = append(allDesired, k)
	}
	for _, name := range cfg.Forked {
		allDesired = append(allDesired, plugins.ForkedPluginKey(name))
	}

	prefs := config.DefaultUserPreferences()
	prefsPath := filepath.Join(syncDir, "user-preferences.yaml")
	if prefsData, err := os.ReadFile(prefsPath); err == nil {
		prefs, _ = config.ParseUserPreferences(prefsData)
	}

	effectiveDesired := csync.ApplyPluginPreferences(
		allDesired,
		prefs.Plugins.Unsubscribe,
		prefs.Plugins.Personal,
	)

	installedPlugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	diff := csync.ComputePluginDiff(effectiveDesired, installedPlugins.PluginKeys())

	result := &PullResult{
		ToInstall:        diff.ToInstall,
		Synced:           diff.Synced,
		Untracked:        diff.Untracked,
		EffectiveDesired: effectiveDesired,
	}

	if prefs.SyncMode == "exact" {
		result.ToRemove = diff.Untracked
		result.Untracked = nil
	}

	return result, nil
}

func Pull(claudeDir, syncDir string, quiet bool) (*PullResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	if git.HasRemote(syncDir, "origin") {
		if err := git.Pull(syncDir); err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Warning: git pull failed: %v\n", err)
			}
		}
	}

	result, err := PullDryRun(claudeDir, syncDir)
	if err != nil {
		return nil, err
	}

	for _, plugin := range result.ToInstall {
		if !quiet {
			fmt.Printf("  Installing %s...\n", plugin)
		}
		if err := installPlugin(plugin); err != nil {
			result.Failed = append(result.Failed, plugin)
			if !quiet {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", plugin, err)
			}
		} else {
			result.Installed = append(result.Installed, plugin)
			if !quiet {
				fmt.Printf("  ✓ %s\n", plugin)
			}
		}
	}

	if len(result.Failed) > 0 {
		if !quiet {
			fmt.Println("\nRetrying failed plugins...")
		}
		var stillFailed []string
		for _, plugin := range result.Failed {
			if err := installPlugin(plugin); err != nil {
				stillFailed = append(stillFailed, plugin)
			} else {
				result.Installed = append(result.Installed, plugin)
			}
		}
		result.Failed = stillFailed
	}

	return result, nil
}

func installPlugin(pluginKey string) error {
	cmd := exec.Command("claude", "plugin", "install", pluginKey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}

func uninstallPlugin(pluginKey, scope string) error {
	cmd := exec.Command("claude", "plugin", "uninstall", "--scope", scope, pluginKey)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", err, string(out))
	}
	return nil
}
