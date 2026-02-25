package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/ruminaider/claude-sync/internal/project"
)

// AutoCommitResult holds the result of an auto-commit operation.
type AutoCommitResult struct {
	Changed       bool
	CommitMessage string
	FilesChanged  []string
}

// AutoCommitOptions configures profile-aware auto-commit behavior.
type AutoCommitOptions struct {
	ClaudeDir  string
	SyncDir    string
	ProjectDir string // CWD from Claude Code hook stdin; used to resolve profile
	ForceBase  bool   // --base flag: always write to base config, ignore profile
}

// AutoCommit checks for local changes to CLAUDE.md, settings, and MCP,
// then creates a local git commit if anything changed. Does NOT push.
// This is the backward-compatible wrapper; use AutoCommitWithContext for profile awareness.
func AutoCommit(claudeDir, syncDir string) (*AutoCommitResult, error) {
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return &AutoCommitResult{}, nil
	}

	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	var changes []string
	var stagedFiles []string
	configChanged := false

	// Check CLAUDE.md changes.
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	if claudeMDData, err := os.ReadFile(claudeMDPath); err == nil {
		reconcileResult, err := claudemd.Reconcile(syncDir, string(claudeMDData))
		if err == nil {
			if len(reconcileResult.Updated) > 0 {
				changes = append(changes, "update "+strings.Join(reconcileResult.Updated, ", "))
				stagedFiles = append(stagedFiles, "claude-md")
			}
			if len(reconcileResult.New) > 0 {
				names := make([]string, len(reconcileResult.New))
				for i, s := range reconcileResult.New {
					names[i] = claudemd.HeaderToFragmentName(s.Header)
					// Write new fragment files.
					claudeMdDir := filepath.Join(syncDir, "claude-md")
					os.MkdirAll(claudeMdDir, 0755)
					claudemd.WriteFragment(claudeMdDir, names[i], s.Content)
				}
				changes = append(changes, "add "+strings.Join(names, ", "))
				stagedFiles = append(stagedFiles, "claude-md")
				// Update config.yaml to include new fragments.
				for _, name := range names {
					cfg.ClaudeMD.Include = append(cfg.ClaudeMD.Include, name)
				}
				configChanged = true
			}
		}
	}

	// Check settings changes.
	settingsRaw, settErr := claudecode.ReadSettings(claudeDir)
	if settErr == nil && cfg.Settings != nil {
		for key, val := range cfg.Settings {
			if raw, ok := settingsRaw[key]; ok {
				var current any
				json.Unmarshal(raw, &current)
				currentJSON, _ := json.Marshal(current)
				cfgJSON, _ := json.Marshal(val)
				if string(currentJSON) != string(cfgJSON) {
					cfg.Settings[key] = current
					configChanged = true
					changes = append(changes, "update setting "+key)
				}
			}
		}
	}

	// Check MCP changes.
	currentMCP, mcpErr := claudecode.ReadMCPConfig(claudeDir)
	if mcpErr == nil && len(currentMCP) > 0 {
		if !jsonMapsEqual(currentMCP, cfg.MCP) {
			cfg.MCP = currentMCP
			configChanged = true
			changes = append(changes, "update MCP servers")
		}
	}

	if len(changes) == 0 {
		return &AutoCommitResult{}, nil
	}

	// Write updated config if needed.
	if configChanged {
		newData, err := config.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("marshaling config: %w", err)
		}
		cfgPath := filepath.Join(syncDir, "config.yaml")
		if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
		stagedFiles = append(stagedFiles, "config.yaml")
	}

	// Stage and commit.
	sort.Strings(stagedFiles)
	// Deduplicate.
	seen := make(map[string]bool)
	var deduped []string
	for _, f := range stagedFiles {
		if !seen[f] {
			seen[f] = true
			deduped = append(deduped, f)
		}
	}

	for _, f := range deduped {
		if err := git.Add(syncDir, f); err != nil {
			return nil, fmt.Errorf("staging %s: %w", f, err)
		}
	}

	commitMsg := "auto: " + strings.Join(changes, ", ")
	if err := git.Commit(syncDir, commitMsg); err != nil {
		return nil, fmt.Errorf("committing: %w", err)
	}

	return &AutoCommitResult{
		Changed:       true,
		CommitMessage: commitMsg,
		FilesChanged:  deduped,
	}, nil
}

// AutoCommitWithContext is the profile-aware version of AutoCommit.
// It resolves the active profile from the project directory and writes
// changes to the profile yaml instead of the base config when appropriate.
func AutoCommitWithContext(opts AutoCommitOptions) (*AutoCommitResult, error) {
	if _, err := os.Stat(opts.SyncDir); os.IsNotExist(err) {
		return &AutoCommitResult{}, nil
	}

	// Resolve profile from project directory.
	profileName := ""
	if !opts.ForceBase && opts.ProjectDir != "" {
		if projectRoot, err := project.FindProjectRoot(opts.ProjectDir); err == nil {
			if projCfg, err := project.ReadProjectConfig(projectRoot); err == nil && projCfg.Profile != "" {
				profileName = projCfg.Profile
			}
		}
	}
	// Fallback: active-profile file.
	if !opts.ForceBase && profileName == "" {
		if active, err := profiles.ReadActiveProfile(opts.SyncDir); err == nil && active != "" {
			profileName = active
		}
	}

	// No profile resolved — delegate to the original base-config path.
	if profileName == "" {
		return AutoCommit(opts.ClaudeDir, opts.SyncDir)
	}

	// Read base config.
	cfgData, err := os.ReadFile(filepath.Join(opts.SyncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	// Read profile.
	profile, err := profiles.ReadProfile(opts.SyncDir, profileName)
	if err != nil {
		// Profile doesn't exist yet — start with an empty one.
		profile = profiles.Profile{}
	}

	// Compute effective config (base + profile merged) for comparison.
	effectiveSettings := profiles.MergeSettings(cfg.Settings, profile)
	effectiveMCP := profiles.MergeMCP(cfg.MCP, profile)

	var changes []string
	var stagedFiles []string
	profileChanged := false
	configChanged := false

	// Check CLAUDE.md changes — these still go to base config (fragments are shared).
	claudeMDPath := filepath.Join(opts.ClaudeDir, "CLAUDE.md")
	if claudeMDData, err := os.ReadFile(claudeMDPath); err == nil {
		reconcileResult, err := claudemd.Reconcile(opts.SyncDir, string(claudeMDData))
		if err == nil {
			if len(reconcileResult.Updated) > 0 {
				changes = append(changes, "update "+strings.Join(reconcileResult.Updated, ", "))
				stagedFiles = append(stagedFiles, "claude-md")
			}
			if len(reconcileResult.New) > 0 {
				names := make([]string, len(reconcileResult.New))
				for i, s := range reconcileResult.New {
					names[i] = claudemd.HeaderToFragmentName(s.Header)
					claudeMdDir := filepath.Join(opts.SyncDir, "claude-md")
					os.MkdirAll(claudeMdDir, 0755)
					claudemd.WriteFragment(claudeMdDir, names[i], s.Content)
				}
				changes = append(changes, "add "+strings.Join(names, ", "))
				stagedFiles = append(stagedFiles, "claude-md")
				for _, name := range names {
					cfg.ClaudeMD.Include = append(cfg.ClaudeMD.Include, name)
				}
				configChanged = true
			}
		}
	}

	// Check settings changes against effective config.
	settingsRaw, settErr := claudecode.ReadSettings(opts.ClaudeDir)
	if settErr == nil && effectiveSettings != nil {
		for key, val := range effectiveSettings {
			if raw, ok := settingsRaw[key]; ok {
				var current any
				json.Unmarshal(raw, &current)
				currentJSON, _ := json.Marshal(current)
				cfgJSON, _ := json.Marshal(val)
				if string(currentJSON) != string(cfgJSON) {
					// Write the change to the profile's settings overlay.
					if profile.Settings == nil {
						profile.Settings = make(map[string]any)
					}
					profile.Settings[key] = current
					profileChanged = true
					changes = append(changes, "update setting "+key)
				}
			}
		}
	}

	// Check MCP changes against effective config.
	currentMCP, mcpErr := claudecode.ReadMCPConfig(opts.ClaudeDir)
	if mcpErr == nil && len(currentMCP) > 0 {
		if !jsonMapsEqual(currentMCP, effectiveMCP) {
			// Compute the delta: new/changed servers go to profile.Add,
			// removed servers go to profile.Remove.
			newAdd := make(map[string]json.RawMessage)
			// Start with existing profile adds.
			for k, v := range profile.MCP.Add {
				newAdd[k] = v
			}
			// Add servers that are new or changed vs base.
			for name, val := range currentMCP {
				baseVal, inBase := cfg.MCP[name]
				if !inBase || string(baseVal) != string(val) {
					newAdd[name] = val
				}
			}
			// Servers in effective but not in current → add to remove list.
			var newRemove []string
			removeSet := make(map[string]bool)
			for _, r := range profile.MCP.Remove {
				removeSet[r] = true
			}
			for name := range effectiveMCP {
				if _, inCurrent := currentMCP[name]; !inCurrent {
					if _, inBase := cfg.MCP[name]; inBase && !removeSet[name] {
						newRemove = append(newRemove, name)
					}
					// Also remove from profile adds if it was there.
					delete(newAdd, name)
				}
			}
			newRemove = append(newRemove, profile.MCP.Remove...)
			// Deduplicate remove list.
			seen := make(map[string]bool)
			var dedupedRemove []string
			for _, r := range newRemove {
				if !seen[r] {
					seen[r] = true
					dedupedRemove = append(dedupedRemove, r)
				}
			}

			profile.MCP.Add = newAdd
			profile.MCP.Remove = dedupedRemove
			profileChanged = true
			changes = append(changes, "update MCP servers")
		}
	}

	if len(changes) == 0 {
		return &AutoCommitResult{}, nil
	}

	// Write updated base config if CLAUDE.md fragments changed.
	if configChanged {
		newData, err := config.Marshal(cfg)
		if err != nil {
			return nil, fmt.Errorf("marshaling config: %w", err)
		}
		cfgPath := filepath.Join(opts.SyncDir, "config.yaml")
		if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
			return nil, fmt.Errorf("writing config: %w", err)
		}
		stagedFiles = append(stagedFiles, "config.yaml")
	}

	// Write updated profile.
	if profileChanged {
		profileData, err := profiles.MarshalProfile(profile)
		if err != nil {
			return nil, fmt.Errorf("marshaling profile %q: %w", profileName, err)
		}
		profileDir := filepath.Join(opts.SyncDir, "profiles")
		os.MkdirAll(profileDir, 0755)
		profilePath := filepath.Join(profileDir, profileName+".yaml")
		if err := os.WriteFile(profilePath, profileData, 0644); err != nil {
			return nil, fmt.Errorf("writing profile %q: %w", profileName, err)
		}
		stagedFiles = append(stagedFiles, filepath.Join("profiles", profileName+".yaml"))
	}

	// Stage and commit.
	sort.Strings(stagedFiles)
	seen := make(map[string]bool)
	var deduped []string
	for _, f := range stagedFiles {
		if !seen[f] {
			seen[f] = true
			deduped = append(deduped, f)
		}
	}

	for _, f := range deduped {
		if err := git.Add(opts.SyncDir, f); err != nil {
			return nil, fmt.Errorf("staging %s: %w", f, err)
		}
	}

	commitMsg := "auto(" + profileName + "): " + strings.Join(changes, ", ")
	if err := git.Commit(opts.SyncDir, commitMsg); err != nil {
		return nil, fmt.Errorf("committing: %w", err)
	}

	return &AutoCommitResult{
		Changed:       true,
		CommitMessage: commitMsg,
		FilesChanged:  deduped,
	}, nil
}
