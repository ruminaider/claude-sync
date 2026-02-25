package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/subscriptions"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

// PluginStatus represents the status of an individual plugin with version info.
type PluginStatus struct {
	Key              string `json:"key"`
	InstalledVersion string `json:"installed_version,omitempty"`
	PinnedVersion    string `json:"pinned_version,omitempty"`
	Installed        bool   `json:"installed"`
}

// SubscriptionInfo represents the status of a single subscription.
type SubscriptionInfo struct {
	Name        string    `json:"name"`
	URL         string    `json:"url"`
	Ref         string    `json:"ref"`
	LastFetched time.Time `json:"last_fetched,omitempty"`
	CommitSHA   string    `json:"commit_sha,omitempty"`
	Healthy     bool      `json:"healthy"`
}

// StatusResult contains the computed status of plugins relative to the config.
type StatusResult struct {
	Synced       []string           `json:"synced"`
	NotInstalled []string           `json:"not_installed"`
	Untracked    []string           `json:"untracked"`
	SettingsDiff csync.SettingsDiff `json:"-"`

	// V2 categorized fields
	UpstreamSynced  []PluginStatus `json:"upstream_synced,omitempty"`
	UpstreamMissing []PluginStatus `json:"upstream_missing,omitempty"`
	PinnedSynced    []PluginStatus `json:"pinned_synced,omitempty"`
	PinnedMissing   []PluginStatus `json:"pinned_missing,omitempty"`
	ForkedSynced    []PluginStatus `json:"forked_synced,omitempty"`
	ForkedMissing   []PluginStatus          `json:"forked_missing,omitempty"`
	ConfigVersion   string                   `json:"config_version"`
	PendingChanges  *approval.PendingChanges `json:"pending_changes,omitempty"`
	Subscriptions   []SubscriptionInfo       `json:"subscriptions,omitempty"`
}

// JSON returns the StatusResult as indented JSON bytes.
func (r *StatusResult) JSON() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}

// HasPendingChanges returns true if there are plugins to install or untracked plugins.
func (r *StatusResult) HasPendingChanges() bool {
	return len(r.NotInstalled) > 0 || len(r.Untracked) > 0
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

	installedPlugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}

	diff := csync.ComputePluginDiff(cfg.AllPluginKeys(), installedPlugins.PluginKeys())

	result := &StatusResult{
		Synced:        diff.Synced,
		NotInstalled:  diff.ToInstall,
		Untracked:     diff.Untracked,
		ConfigVersion: cfg.Version,
	}

	// Categorized status for upstream, pinned, and forked plugins.
	installedSet := make(map[string]bool)
	for _, k := range installedPlugins.PluginKeys() {
		installedSet[k] = true
	}

	// Build version lookup from installed plugins
	versionLookup := make(map[string]string)
	for key, installs := range installedPlugins.Plugins {
		if len(installs) > 0 {
			versionLookup[key] = installs[0].Version
		}
	}

	// Upstream plugins
	for _, key := range cfg.Upstream {
		ps := PluginStatus{
			Key:              key,
			InstalledVersion: versionLookup[key],
			Installed:        installedSet[key],
		}
		if installedSet[key] {
			result.UpstreamSynced = append(result.UpstreamSynced, ps)
		} else {
			result.UpstreamMissing = append(result.UpstreamMissing, ps)
		}
	}

	// Pinned plugins
	for key, pinnedVer := range cfg.Pinned {
		ps := PluginStatus{
			Key:              key,
			InstalledVersion: versionLookup[key],
			PinnedVersion:    pinnedVer,
			Installed:        installedSet[key],
		}
		if installedSet[key] {
			result.PinnedSynced = append(result.PinnedSynced, ps)
		} else {
			result.PinnedMissing = append(result.PinnedMissing, ps)
		}
	}

	// Forked plugins — check both name@claude-sync-forks and plain name
	for _, name := range cfg.Forked {
		forkKey := plugins.ForkedPluginKey(name)
		installed := installedSet[forkKey] || installedSet[name]
		ver := versionLookup[forkKey]
		if ver == "" {
			ver = versionLookup[name]
		}
		ps := PluginStatus{
			Key:              name,
			InstalledVersion: ver,
			Installed:        installed,
		}
		if installed {
			result.ForkedSynced = append(result.ForkedSynced, ps)
		} else {
			result.ForkedMissing = append(result.ForkedMissing, ps)
		}
	}

	// Check for pending high-risk changes.
	pending, err := approval.ReadPending(syncDir)
	if err == nil && !pending.IsEmpty() {
		result.PendingChanges = &pending
	}

	// Subscription health.
	if len(cfg.Subscriptions) > 0 {
		state, _ := subscriptions.ReadState(syncDir)
		for name, sub := range cfg.Subscriptions {
			ref := sub.Ref
			if ref == "" {
				ref = "main"
			}
			info := SubscriptionInfo{
				Name: name,
				URL:  sub.URL,
				Ref:  ref,
			}
			if ss, ok := state.Subscriptions[name]; ok {
				info.LastFetched = ss.LastFetched
				info.CommitSHA = ss.CommitSHA
				info.Healthy = true
			}
			result.Subscriptions = append(result.Subscriptions, info)
		}
	}

	return result, nil
}
