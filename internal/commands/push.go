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
	"github.com/ruminaider/claude-sync/internal/profiles"
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

// PushApplyOptions configures what PushApply does.
type PushApplyOptions struct {
	ClaudeDir      string
	SyncDir        string
	AddPlugins     []string
	RemovePlugins  []string
	ExcludePlugins []string // plugins to add to cfg.Excluded
	ProfileTarget  string   // "" = base config, non-empty = profile name
	Message        string
}

func PushApply(opts PushApplyOptions) error {
	cfgPath := filepath.Join(opts.SyncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return err
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return err
	}

	if opts.ProfileTarget != "" {
		// Route plugins to a profile instead of base config.
		profile, err := profiles.ReadProfile(opts.SyncDir, opts.ProfileTarget)
		if err != nil {
			return fmt.Errorf("reading profile %q: %w", opts.ProfileTarget, err)
		}

		// Append new plugins to profile's Add list (dedup).
		addSet := make(map[string]bool, len(profile.Plugins.Add))
		for _, p := range profile.Plugins.Add {
			addSet[p] = true
		}
		for _, p := range opts.AddPlugins {
			if !addSet[p] {
				profile.Plugins.Add = append(profile.Plugins.Add, p)
			}
		}

		profileData, err := profiles.MarshalProfile(profile)
		if err != nil {
			return fmt.Errorf("marshaling profile %q: %w", opts.ProfileTarget, err)
		}
		profilePath := filepath.Join(opts.SyncDir, "profiles", opts.ProfileTarget+".yaml")
		if err := os.WriteFile(profilePath, profileData, 0644); err != nil {
			return fmt.Errorf("writing profile %q: %w", opts.ProfileTarget, err)
		}
	} else {
		// Add new plugins to upstream (base config).
		upstreamSet := make(map[string]bool)
		for _, p := range cfg.Upstream {
			upstreamSet[p] = true
		}
		for _, p := range opts.AddPlugins {
			upstreamSet[p] = true
		}
		for _, p := range opts.RemovePlugins {
			delete(upstreamSet, p)
			delete(cfg.Pinned, p)
		}

		cfg.Upstream = make([]string, 0, len(upstreamSet))
		for p := range upstreamSet {
			cfg.Upstream = append(cfg.Upstream, p)
		}
		sort.Strings(cfg.Upstream)
	}

	// Remove newly-added plugins from the excluded list.
	if len(cfg.Excluded) > 0 && len(opts.AddPlugins) > 0 {
		addSet := make(map[string]bool, len(opts.AddPlugins))
		for _, p := range opts.AddPlugins {
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

	// Append excluded plugins (dedup, sort).
	if len(opts.ExcludePlugins) > 0 {
		excludedSet := make(map[string]bool, len(cfg.Excluded))
		for _, e := range cfg.Excluded {
			excludedSet[e] = true
		}
		for _, p := range opts.ExcludePlugins {
			if !excludedSet[p] {
				cfg.Excluded = append(cfg.Excluded, p)
			}
		}
		sort.Strings(cfg.Excluded)
	}

	// Always write config (excluded list may change even for profile-targeted pushes).
	newData, err := config.Marshal(cfg)
	if err != nil {
		return err
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return err
	}

	message := opts.Message
	if message == "" {
		message = generateCommitMessage(opts.AddPlugins, opts.RemovePlugins, opts.ProfileTarget)
	}

	if err := git.Add(opts.SyncDir, "config.yaml"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if opts.ProfileTarget != "" {
		profileRelPath := filepath.Join("profiles", opts.ProfileTarget+".yaml")
		if err := git.Add(opts.SyncDir, profileRelPath); err != nil {
			return fmt.Errorf("staging profile: %w", err)
		}
	}
	if err := git.Commit(opts.SyncDir, message); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	if git.HasRemote(opts.SyncDir, "origin") {
		if err := git.Push(opts.SyncDir); err != nil {
			return fmt.Errorf("pushing: %w", err)
		}
	}

	return nil
}

func generateCommitMessage(added, removed []string, profileTarget string) string {
	var parts []string
	if len(added) > 0 {
		names := strings.Join(shortNames(added), ", ")
		if profileTarget != "" {
			parts = append(parts, "Add "+names+" to profile "+profileTarget)
		} else {
			parts = append(parts, "Add "+names)
		}
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
