package commands

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

type PushScanResult struct {
	AddedPlugins    []string
	RemovedPlugins  []string
	ChangedSettings map[string]csync.SettingChange
}

func (r *PushScanResult) HasChanges() bool {
	return len(r.AddedPlugins) > 0 || len(r.RemovedPlugins) > 0 || len(r.ChangedSettings) > 0
}

func PushScan(claudeDir, syncDir string) (*PushScanResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized")
	}

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, err
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, err
	}

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, err
	}

	diff := csync.ComputePluginDiff(cfg.AllPluginKeys(), plugins.PluginKeys())

	// Build filter sets.
	excludedSet := make(map[string]bool, len(cfg.Excluded))
	for _, e := range cfg.Excluded {
		excludedSet[e] = true
	}
	forkedSet := make(map[string]bool, len(cfg.Forked))
	for _, f := range cfg.Forked {
		forkedSet[f] = true
	}

	var filtered []string
	for _, p := range diff.Untracked {
		// 1. Exact match against excluded list.
		if excludedSet[p] {
			continue
		}

		name := p
		mkt := ""
		if idx := strings.Index(p, "@"); idx > 0 {
			name = p[:idx]
			mkt = p[idx+1:]
		}

		// 2. @claude-sync-forks entries are always init artifacts.
		if mkt == "claude-sync-forks" {
			continue
		}

		// 3. Forked name match (e.g. "figma-minimal" in cfg.Forked
		//    covers "figma-minimal@figma-minimal-marketplace").
		if forkedSet[name] {
			continue
		}

		filtered = append(filtered, p)
	}

	return &PushScanResult{
		AddedPlugins:   filtered,
		RemovedPlugins: diff.ToInstall,
	}, nil
}

func PushApply(claudeDir, syncDir string, addPlugins, removePlugins []string, message string) error {
	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return err
	}

	// Add new plugins to upstream category
	upstreamSet := make(map[string]bool)
	for _, p := range cfg.Upstream {
		upstreamSet[p] = true
	}
	for _, p := range addPlugins {
		upstreamSet[p] = true
	}
	for _, p := range removePlugins {
		delete(upstreamSet, p)
		delete(cfg.Pinned, p)
	}

	// Remove newly-added plugins from the excluded list.
	if len(cfg.Excluded) > 0 && len(addPlugins) > 0 {
		addSet := make(map[string]bool, len(addPlugins))
		for _, p := range addPlugins {
			addSet[p] = true
		}
		var remaining []string
		for _, e := range cfg.Excluded {
			if !addSet[e] {
				remaining = append(remaining, e)
			}
		}
		cfg.Excluded = remaining
	}

	cfg.Upstream = make([]string, 0, len(upstreamSet))
	for p := range upstreamSet {
		cfg.Upstream = append(cfg.Upstream, p)
	}
	sort.Strings(cfg.Upstream)

	newData, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	if message == "" {
		message = generateCommitMessage(addPlugins, removePlugins)
	}

	if err := git.Add(syncDir, "config.yaml"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := git.Commit(syncDir, message); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	if git.HasRemote(syncDir, "origin") {
		if err := git.Push(syncDir); err != nil {
			return fmt.Errorf("pushing: %w", err)
		}
	}

	return nil
}

func generateCommitMessage(added, removed []string) string {
	var parts []string
	if len(added) > 0 {
		parts = append(parts, "Add "+strings.Join(shortNames(added), ", "))
	}
	if len(removed) > 0 {
		parts = append(parts, "Remove "+strings.Join(shortNames(removed), ", "))
	}
	if len(parts) == 0 {
		return "Update config"
	}
	return strings.Join(parts, "; ")
}

func shortNames(plugins []string) []string {
	names := make([]string, len(plugins))
	for i, p := range plugins {
		if idx := strings.Index(p, "@"); idx > 0 {
			names[i] = p[:idx]
		} else {
			names[i] = p
		}
	}
	return names
}
