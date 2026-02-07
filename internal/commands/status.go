package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

type StatusResult struct {
	Synced       []string
	NotInstalled []string
	Untracked    []string
	SettingsDiff csync.SettingsDiff
}

func Status(claudeDir, syncDir string) (*StatusResult, error) {
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

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	diff := csync.ComputePluginDiff(cfg.Plugins, plugins.PluginKeys())

	return &StatusResult{
		Synced:       diff.Synced,
		NotInstalled: diff.ToInstall,
		Untracked:    diff.Untracked,
	}, nil
}
