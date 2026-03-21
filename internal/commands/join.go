package commands

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/bundled"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// MissingConfigError is returned when the cloned repo has no config.yaml.
type MissingConfigError struct{}

func (e *MissingConfigError) Error() string {
	return "config repo has no config.yaml"
}

// AlreadyJoinedError is returned when the user joins a URL that matches the existing origin.
type AlreadyJoinedError struct {
	URL string
}

func (e *AlreadyJoinedError) Error() string {
	return fmt.Sprintf("already joined %s", e.URL)
}

// SubscribeNeededError is returned when the user joins a different URL than the existing origin.
type SubscribeNeededError struct {
	URL string
}

func (e *SubscribeNeededError) Error() string {
	return fmt.Sprintf("different config repo; subscribe to %s instead", e.URL)
}

// NormalizeURL strips trailing slashes and .git suffix for URL comparison.
func NormalizeURL(u string) string {
	u = strings.TrimRight(u, "/")
	u = strings.TrimSuffix(u, ".git")
	return u
}

// LocalPlugin describes a locally installed plugin not in the remote config.
type LocalPlugin struct {
	Key   string // e.g. "figma@claude-plugins-official"
	Scope string // e.g. "user" or "project"
}

// JoinResult describes local-only plugins detected after joining.
type JoinResult struct {
	LocalOnly    []LocalPlugin
	HasSettings  bool
	HasHooks     bool
	SettingsKeys []string // e.g. ["model"]
	HookNames    []string // e.g. ["PreCompact", "SessionStart"]
	HasProfiles  bool
	ProfileNames []string
	Warnings     []string
}

func Join(repoURL, claudeDir, syncDir string) (*JoinResult, error) {
	if _, err := os.Stat(syncDir); err == nil {
		// Sync dir exists — check if it's the same or different repo.
		existingURL, err := git.RemoteURL(syncDir, "origin")
		if err == nil && NormalizeURL(existingURL) == NormalizeURL(repoURL) {
			return nil, &AlreadyJoinedError{URL: repoURL}
		}
		return nil, &SubscribeNeededError{URL: repoURL}
	}

	if !claudecode.DirExists(claudeDir) {
		if err := claudecode.Bootstrap(claudeDir); err != nil {
			return nil, fmt.Errorf("bootstrapping Claude Code directory: %w", err)
		}
	}

	if err := git.Clone(repoURL, syncDir); err != nil {
		return nil, fmt.Errorf("cloning config repo: %w", err)
	}

	// If we cloned from a local path, look up that repo's origin remote
	// so we push to the real upstream instead of the non-bare local checkout.
	if git.IsLocalPath(repoURL) {
		if upstream, err := git.ResolveUpstreamURL(repoURL); err == nil && upstream != "" {
			if err := git.SetRemoteURL(syncDir, "origin", upstream); err != nil {
				return nil, fmt.Errorf("rewriting remote to upstream URL %q: %w", upstream, err)
			}
		}
	}

	// Detect local-only plugins: installed locally but not in the remote config.
	result := &JoinResult{}

	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, &MissingConfigError{}
		}
		return nil, fmt.Errorf("reading config.yaml: %w", err)
	}

	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config.yaml: %w", err)
	}

	// Ensure the bundled claude-sync plugin exists in the config repo.
	// This covers configs created before auto-install was added.
	bundledPluginDir := filepath.Join(syncDir, "plugins", bundled.PluginName)
	if err := bundled.ExtractPlugin(bundledPluginDir); err != nil {
		return nil, fmt.Errorf("extracting bundled plugin: %w", err)
	}

	// Register the bundled plugin in the Forked list so pull can find it.
	if !slices.Contains(cfg.Forked, bundled.PluginName) {
		cfg.Forked = append(cfg.Forked, bundled.PluginName)
		updatedData, err := config.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("marshalling config after forked update: %w", err)
		}
		if err := os.WriteFile(cfgPath, updatedData, 0644); err != nil {
			return nil, fmt.Errorf("writing config after forked update: %w", err)
		}
	}

	// Expose config categories so the CLI can prompt about them.
	if len(cfg.Settings) > 0 {
		result.HasSettings = true
		for k := range cfg.Settings {
			result.SettingsKeys = append(result.SettingsKeys, k)
		}
		sort.Strings(result.SettingsKeys)
	}
	if len(cfg.Hooks) > 0 {
		result.HasHooks = true
		for k := range cfg.Hooks {
			result.HookNames = append(result.HookNames, k)
		}
		sort.Strings(result.HookNames)
	}

	profileNames, err := profiles.ListProfiles(syncDir)
	if err != nil {
		return nil, fmt.Errorf("listing profiles: %w", err)
	}
	if len(profileNames) > 0 {
		result.HasProfiles = true
		result.ProfileNames = profileNames
	}

	installed, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return result, nil // No plugins file yet (fresh install) — skip detection
		}
		return nil, fmt.Errorf("detecting local plugins: %w", err)
	}

	configKeys := make(map[string]bool)
	for _, k := range cfg.AllPluginKeys() {
		configKeys[k] = true
	}

	for _, k := range installed.PluginKeys() {
		if !configKeys[k] {
			scope := "user"
			if installations, ok := installed.Plugins[k]; ok && len(installations) > 0 {
				scope = installations[0].Scope
			}
			result.LocalOnly = append(result.LocalOnly, LocalPlugin{Key: k, Scope: scope})
		}
	}

	return result, nil
}

// JoinReplace removes the existing sync dir and performs a fresh join.
func JoinReplace(repoURL, claudeDir, syncDir string) (*JoinResult, error) {
	if err := os.RemoveAll(syncDir); err != nil {
		return nil, fmt.Errorf("removing existing config: %w", err)
	}
	return Join(repoURL, claudeDir, syncDir)
}

// CleanupResult holds the outcome of a plugin removal attempt.
type CleanupResult struct {
	Plugin string
	Err    error
}

// JoinCleanup uninstalls the specified plugins and returns results for each.
func JoinCleanup(plugins []LocalPlugin) []CleanupResult {
	var results []CleanupResult
	for _, p := range plugins {
		err := uninstallPlugin(p.Key, p.Scope)
		results = append(results, CleanupResult{Plugin: p.Key, Err: err})
	}
	return results
}
