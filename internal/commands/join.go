package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// JoinResult describes local-only plugins detected after joining.
type JoinResult struct {
	LocalOnly []string // plugins installed locally but not in the remote config
}

func Join(repoURL, claudeDir, syncDir string) (*JoinResult, error) {
	if _, err := os.Stat(syncDir); err == nil {
		return nil, fmt.Errorf("%s already exists. Run 'claude-sync pull' instead", syncDir)
	}

	if !claudecode.DirExists(claudeDir) {
		if err := claudecode.Bootstrap(claudeDir); err != nil {
			return nil, fmt.Errorf("bootstrapping Claude Code directory: %w", err)
		}
	}

	if err := git.Clone(repoURL, syncDir); err != nil {
		return nil, fmt.Errorf("cloning config repo: %w", err)
	}

	// Detect local-only plugins: installed locally but not in the remote config.
	result := &JoinResult{}

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		// Config unreadable — skip detection, not a fatal error.
		return result, nil
	}

	cfg, err := config.Parse(cfgData)
	if err != nil {
		return result, nil
	}

	installed, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return result, nil
	}

	configKeys := make(map[string]bool)
	for _, k := range cfg.AllPluginKeys() {
		configKeys[k] = true
	}

	for _, k := range installed.PluginKeys() {
		if !configKeys[k] {
			result.LocalOnly = append(result.LocalOnly, k)
		}
	}

	return result, nil
}

// CleanupResult holds the outcome of a plugin removal attempt.
type CleanupResult struct {
	Plugin string
	Err    error
}

// JoinCleanup uninstalls the specified plugins and returns results for each.
func JoinCleanup(plugins []string) []CleanupResult {
	var results []CleanupResult
	for _, p := range plugins {
		err := uninstallPlugin(p)
		results = append(results, CleanupResult{Plugin: p, Err: err})
	}
	return results
}
