package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/marketplace"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/ruminaider/claude-sync/internal/subscriptions"
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

type PullResult struct {
	ToInstall              []string
	ToRemove               []string
	Synced                 []string
	Untracked              []string
	EffectiveDesired       []string
	Installed              []string
	Failed                 []string
	SettingsApplied        []string
	HooksApplied           []string
	HooksSkipped           []string // hooks skipped due to missing script files
	SkippedCategories      []string
	ActiveProfile          string // active profile applied (empty = base only)
	PermissionsApplied     bool
	ClaudeMDAssembled      bool
	MCPApplied             []string
	MCPEnvWarnings         []string // unresolved ${VAR} references
	MCPProjectApplied      map[string][]string // project path -> server names written there
	KeybindingsApplied     bool
	CommandsRestored           int
	CommandsSkipped            int
	SkillsRestored             int
	SkillsSkipped              int
	PendingHighRisk            []approval.Change
	Updated                    []string // plugins refreshed due to version mismatch
	UpdateFailed               []string // plugins that failed to refresh
	UndefinedMarketplaces      map[string][]string // marketplace name -> plugin names referencing it
	ProjectSettingsApplied     bool
	ProjectUnmanagedDetected   bool     // CWD has settings.local.json but no .claude-sync.yaml
	ProjectInitEligible        bool     // detected project root has no .claude-sync.yaml and profiles exist
	ProjectInitDir             string   // suggested directory for project init (project root, not CWD)
	AvailableProfiles          []string // profile names from sync dir (set when ProjectInitEligible)
	DuplicatePlugins           []plugins.Duplicate // unresolved duplicate plugins (auto mode)
	SettingsSkipped            bool     // settings.json had local modifications
	ClaudeMDSkipped            bool     // CLAUDE.md had local modifications
	MCPSkipped                 bool     // .mcp.json had local modifications
	KeybindingsSkipped         bool     // keybindings.json had local modifications
}

// PullOptions configures pull behavior.
type PullOptions struct {
	ClaudeDir string
	SyncDir   string
	Quiet     bool
	Auto      bool // auto mode: safe changes auto-apply, high-risk deferred to pending
	// MCPTargetResolver resolves the destination path for project-scoped MCP servers.
	// Called with (serverName, suggestedProjectPath) and returns the confirmed path
	// (empty string means write to global instead). If nil, uses suggested paths as-is.
	MCPTargetResolver func(serverName, suggestedPath string) string
	ProjectDir        string // if set, apply project settings after global pull
	// DuplicateResolver is called when duplicate plugins are detected.
	// nil means skip resolution (duplicates left as-is).
	DuplicateResolver func(dupes []plugins.Duplicate) error
	// ReEvalResolver is called when tracked active-dev plugins need re-evaluation.
	// nil means skip re-evaluation.
	ReEvalResolver func(signals []plugins.ReEvalSignal) error
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

	// Register declared custom marketplaces before any plugin operations.
	if len(cfg.Marketplaces) > 0 {
		if err := marketplace.EnsureRegistered(claudeDir, cfg.Marketplaces); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to register marketplaces: %v\n", err)
		}
	}

	// Register local marketplace if forked plugins exist, otherwise clean up stale entry.
	forks, _ := plugins.ListForkedPlugins(syncDir)
	if len(forks) > 0 {
		if err := plugins.RegisterLocalMarketplace(claudeDir, syncDir); err != nil {
			return nil, fmt.Errorf("registering local marketplace: %w", err)
		}
	} else {
		_ = plugins.UnregisterLocalMarketplace(claudeDir)
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

	// Apply active profile to desired plugins.
	activeName, _ := profiles.ReadActiveProfile(syncDir)
	if activeName != "" {
		p, err := profiles.ReadProfile(syncDir, activeName)
		if err == nil {
			allDesired = profiles.MergePlugins(allDesired, p)
		}
	}

	// Auto-register marketplaces referenced by plugins but not in config.yaml.
	// Best-effort: partial successes are fine since FindUndefinedMarketplaces
	// (called next) catches anything not resolved.
	// NOTE: Side effects (writes known_marketplaces.json, may clone repos)
	// mirror the existing EnsureRegistered call above.
	registered, regErr := marketplace.AutoRegisterFromPlugins(claudeDir, allDesired, cfg.Marketplaces)
	if regErr != nil {
		fmt.Fprintf(os.Stderr, "Warning: auto-registering marketplaces (registered %d): %v\n",
			len(registered), regErr)
	}

	// Check for plugins referencing undefined marketplaces.
	undefinedMkts := marketplace.FindUndefinedMarketplaces(claudeDir, allDesired, cfg.Marketplaces)

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
		ToInstall:             diff.ToInstall,
		Synced:                diff.Synced,
		Untracked:             diff.Untracked,
		EffectiveDesired:      effectiveDesired,
		ActiveProfile:         activeName,
		UndefinedMarketplaces: undefinedMkts,
	}

	if prefs.SyncMode == "exact" {
		result.ToRemove = diff.Untracked
		result.Untracked = nil
	}

	return result, nil
}

// Pull is the backward-compatible wrapper.
func Pull(claudeDir, syncDir string, quiet bool) (*PullResult, error) {
	return PullWithOptions(PullOptions{
		ClaudeDir: claudeDir,
		SyncDir:   syncDir,
		Quiet:     quiet,
	})
}

func PullWithOptions(opts PullOptions) (*PullResult, error) {
	claudeDir := opts.ClaudeDir
	syncDir := opts.SyncDir
	quiet := opts.Quiet

	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("claude-sync not initialized. Run 'claude-sync init' or 'claude-sync join <url>'")
	}

	// In auto mode, push any unpushed commits before pulling.
	if opts.Auto && git.HasRemote(syncDir, "origin") && git.HasUnpushedCommits(syncDir) {
		_ = git.Push(syncDir) // best-effort push before pull
	}

	if git.HasRemote(syncDir, "origin") {
		if err := git.Pull(syncDir); err != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Warning: git pull failed: %v\n", err)
			}
		}
	}

	// Fetch and merge subscriptions before computing plugin diff.
	if err := pullSubscriptions(syncDir, opts.Auto, quiet); err != nil && !quiet {
		fmt.Fprintf(os.Stderr, "Warning: subscription pull failed: %v\n", err)
	}

	// Detect duplicate plugins before computing diff.
	var unresolvedDupes []plugins.Duplicate
	dupes, dupErr := plugins.DetectDuplicates(claudeDir)
	if dupErr == nil && len(dupes) > 0 {
		if opts.DuplicateResolver != nil {
			if err := opts.DuplicateResolver(dupes); err != nil {
				return nil, err
			}
			// Resolved — don't report in result.
		} else {
			unresolvedDupes = dupes
		}
	}

	// Re-evaluate tracked active-dev plugins.
	if opts.ReEvalResolver != nil {
		sources, srcErr := plugins.ReadPluginSources(syncDir)
		if srcErr == nil && len(sources.Plugins) > 0 {
			signals := plugins.CheckReEvaluation(sources, 7)
			if len(signals) > 0 {
				if err := opts.ReEvalResolver(signals); err != nil {
					return nil, err
				}
			}
		}
	}

	result, err := PullDryRun(claudeDir, syncDir)
	if err != nil {
		return nil, err
	}

	appliedHashes := LoadAppliedHashes(syncDir)

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

	// Record content hashes for newly installed directory-based plugins.
	if len(result.Installed) > 0 {
		updatePluginContentHashes(claudeDir, result.Installed)
	}

	// Refresh stale plugins (version mismatch between marketplace and cache).
	if stale := detectStalePlugins(claudeDir, result.Synced); len(stale) > 0 {
		installed, _ := claudecode.ReadInstalledPlugins(claudeDir)
		result.Updated, result.UpdateFailed = refreshStalePlugins(claudeDir, stale, installed, quiet)
	}

	// Read user preferences for category skip logic.
	prefs := config.DefaultUserPreferences()
	prefsPath := filepath.Join(syncDir, "user-preferences.yaml")
	if prefsData, err := os.ReadFile(prefsPath); err == nil {
		prefs, _ = config.ParseUserPreferences(prefsData)
	}

	// Parse config for settings, hooks, and new surfaces.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err == nil {
		cfg, err := config.Parse(cfgData)
		if err == nil {
			// Merge active profile into config before applying.
			activeName, _ := profiles.ReadActiveProfile(syncDir)
			var activeProfile *profiles.Profile
			if activeName != "" {
				p, pErr := profiles.ReadProfile(syncDir, activeName)
				if pErr == nil {
					activeProfile = &p
					cfg.Settings = profiles.MergeSettings(cfg.Settings, p)
					cfg.Hooks = profiles.MergeHooks(cfg.Hooks, p)
					cfg.Permissions = profiles.MergePermissions(cfg.Permissions, p)
				}
			}

			if prefs.ShouldSkip(config.CategorySettings) {
				cfg.Settings = nil
				result.SkippedCategories = append(result.SkippedCategories, string(config.CategorySettings))
			}
			if prefs.ShouldSkip(config.CategoryHooks) {
				cfg.Hooks = nil
				result.SkippedCategories = append(result.SkippedCategories, string(config.CategoryHooks))
			}

			// Determine which high-risk items to skip in auto mode.
			skipHooks := opts.Auto
			skipPermissions := opts.Auto
			skipMCP := opts.Auto

			// Clean up legacy session lifecycle hooks now handled by the plugin.
			if cleanErr := CleanupLegacyHooks(claudeDir); cleanErr != nil && !quiet {
				fmt.Fprintf(os.Stderr, "Warning: failed to clean up legacy hooks: %v\n", cleanErr)
			}

			// Apply settings and hooks (hooks skipped in auto mode).
			settingsPath := filepath.Join(claudeDir, "settings.json")
			if appliedHashes.IsLocallyModified("settings", settingsPath) {
				result.SettingsSkipped = true
			} else {
				settingsCfg := cfg
				if skipHooks {
					settingsCfg.Hooks = nil
				}
				applied, hookNames, skippedHooks, applyErr := ApplySettings(claudeDir, settingsCfg)
				result.HooksSkipped = skippedHooks // always propagate skip info
				if applyErr != nil {
					if !quiet {
						fmt.Fprintf(os.Stderr, "Warning: failed to apply settings: %v\n", applyErr)
					}
				} else {
					result.SettingsApplied = applied
					result.HooksApplied = hookNames
					if data, err := os.ReadFile(settingsPath); err == nil {
						appliedHashes.Set("settings", string(data))
					}
				}
			}

			// Apply permissions (additive merge, skipped in auto mode).
			if !skipPermissions && (len(cfg.Permissions.Allow) > 0 || len(cfg.Permissions.Deny) > 0) {
				if !prefs.ShouldSkip(config.CategoryPermissions) {
					if applyPermissions(claudeDir, cfg.Permissions) == nil {
						result.PermissionsApplied = true
					}
				}
			}

			// Assemble CLAUDE.md from fragments (always safe).
			includes := cfg.ClaudeMD.Include
			if activeProfile != nil {
				includes = profiles.MergeClaudeMD(includes, *activeProfile)
			}
			if len(includes) > 0 {
				claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
				if appliedHashes.IsLocallyModified("claude-md", claudeMDPath) {
					result.ClaudeMDSkipped = true
				} else {
					assembled, asmErr := claudemd.AssembleFromDir(syncDir, includes)
					if asmErr == nil && assembled != "" {
						if os.WriteFile(claudeMDPath, []byte(assembled), 0644) == nil {
							result.ClaudeMDAssembled = true
							appliedHashes.Set("claude-md", assembled)
						}
					}
				}
			}

			// Apply MCP servers (additive merge, skipped in auto mode).
			mcpServers := cfg.MCP
			if activeProfile != nil {
				mcpServers = profiles.MergeMCP(mcpServers, *activeProfile)
			}

			// Expand ~/... paths and resolve ${VAR} env var references before writing.
			if len(mcpServers) > 0 {
				mcpServers = ExpandMCPPaths(mcpServers)
				mcpServers, result.MCPEnvWarnings = ResolveMCPEnvVars(mcpServers)
			}

			if !skipMCP && len(mcpServers) > 0 {
				if !prefs.ShouldSkip(config.CategoryMCP) {
					// Partition servers by destination using MCPMeta.
					globalServers := make(map[string]json.RawMessage)
					projectServers := make(map[string]map[string]json.RawMessage) // path -> servers

					for name, raw := range mcpServers {
						meta, hasMeta := cfg.MCPMeta[name]
						if hasMeta && meta.SourceProject != "" {
							targetPath := meta.SourceProject
							if opts.MCPTargetResolver != nil {
								targetPath = opts.MCPTargetResolver(name, targetPath)
							}
							if targetPath != "" {
								if projectServers[targetPath] == nil {
									projectServers[targetPath] = make(map[string]json.RawMessage)
								}
								projectServers[targetPath][name] = raw
								continue
							}
						}
						globalServers[name] = raw
					}

					// Write global servers.
					if len(globalServers) > 0 {
						mcpPath := filepath.Join(claudeDir, ".mcp.json")
						if appliedHashes.IsLocallyModified("mcp", mcpPath) {
							result.MCPSkipped = true
						} else {
							existing, _ := claudecode.ReadMCPConfig(claudeDir)
							for k, v := range globalServers {
								existing[k] = v
							}
							if claudecode.WriteMCPConfig(claudeDir, existing) == nil {
								result.MCPApplied = make([]string, 0, len(globalServers))
								for k := range globalServers {
									result.MCPApplied = append(result.MCPApplied, k)
								}
								sort.Strings(result.MCPApplied)
								if data, err := os.ReadFile(mcpPath); err == nil {
									appliedHashes.Set("mcp", string(data))
								}
							}
						}
					}

					// Write project-scoped servers.
					if len(projectServers) > 0 {
						result.MCPProjectApplied = make(map[string][]string)
						for projectPath, servers := range projectServers {
							mcpPath := filepath.Join(projectPath, ".mcp.json")
							existing, _ := claudecode.ReadMCPConfigFile(mcpPath)
							for k, v := range servers {
								existing[k] = v
							}
							if claudecode.WriteMCPConfigFile(mcpPath, existing) == nil {
								names := make([]string, 0, len(servers))
								for k := range servers {
									names = append(names, k)
								}
								sort.Strings(names)
								result.MCPProjectApplied[projectPath] = names
							}
						}
					}
				}
			}

			// Apply keybindings (always safe).
			kbConfig := cfg.Keybindings
			if activeProfile != nil {
				kbConfig = profiles.MergeKeybindings(kbConfig, *activeProfile)
			}
			if len(kbConfig) > 0 {
				kbPath := filepath.Join(claudeDir, "keybindings.json")
				if appliedHashes.IsLocallyModified("keybindings", kbPath) {
					result.KeybindingsSkipped = true
				} else {
					if claudecode.WriteKeybindings(claudeDir, kbConfig) == nil {
						result.KeybindingsApplied = true
						if data, err := os.ReadFile(kbPath); err == nil {
							appliedHashes.Set("keybindings", string(data))
						}
					}
				}
			}

			// Restore commands and skills from sync dir (always safe).
			effectiveCommands := cfg.Commands
			effectiveSkills := cfg.Skills
			if activeProfile != nil {
				effectiveCommands = profiles.MergeCommands(effectiveCommands, *activeProfile)
				effectiveSkills = profiles.MergeSkills(effectiveSkills, *activeProfile)
			}
			if len(effectiveCommands) > 0 || len(effectiveSkills) > 0 {
				cr, cs, sr, ss := restoreCommandsSkills(claudeDir, syncDir, effectiveCommands, effectiveSkills)
				result.CommandsRestored = cr
				result.CommandsSkipped = cs
				result.SkillsRestored = sr
				result.SkillsSkipped = ss
			}

			// In auto mode, write high-risk items to pending.
			if opts.Auto {
				changes := approval.ConfigChanges{
					Settings:       cfg.Settings,
					HasHookChanges: len(cfg.Hooks) > 0,
				}
				if len(cfg.Permissions.Allow) > 0 || len(cfg.Permissions.Deny) > 0 {
					changes.Permissions = &approval.PermissionChanges{
						Allow: cfg.Permissions.Allow,
						Deny:  cfg.Permissions.Deny,
					}
				}
				if len(mcpServers) > 0 {
					changes.HasMCPChanges = true
				}
				if len(kbConfig) > 0 {
					changes.Keybindings = true
				}
				if len(includes) > 0 {
					changes.ClaudeMD = includes
				}

				classified := approval.Classify(changes)

				if len(classified.HighRisk) > 0 {
					pending := approval.PendingChanges{
						PendingSince: time.Now().UTC().Format(time.RFC3339),
					}
					if len(cfg.Permissions.Allow) > 0 || len(cfg.Permissions.Deny) > 0 {
						pending.Permissions = &approval.PendingPermissions{
							Allow: cfg.Permissions.Allow,
							Deny:  cfg.Permissions.Deny,
						}
					}
					if len(mcpServers) > 0 {
						pending.MCP = mcpServers
					}
					if len(cfg.Hooks) > 0 {
						pending.Hooks = cfg.Hooks
					}

					_ = approval.WritePending(syncDir, pending)
					result.PendingHighRisk = classified.HighRisk
				}
			}
		}
	}

	_ = appliedHashes.Save()

	// Apply project settings if in a project directory.
	projectDir := opts.ProjectDir
	if projectDir == "" {
		// Auto-detect from CWD
		if cwd, err := os.Getwd(); err == nil {
			projectDir, _ = project.FindProjectRoot(cwd)
		}
	}
	if projectDir != "" {
		pcfg, pErr := project.ReadProjectConfig(projectDir)
		if pErr == nil && !pcfg.Declined {
			// Re-parse config for project resolution (need the raw Config struct)
			cfgData2, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
			cfg2, _ := config.Parse(cfgData2)
			resolved := ResolveWithProfile(cfg2, syncDir, pcfg.Profile)
			if applyErr := ApplyProjectSettings(projectDir, resolved, pcfg, syncDir); applyErr == nil {
				result.ProjectSettingsApplied = true
			}
		}
	} else {
		// Detect directories without claude-sync project config.
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			// Find the project root: walk up looking for .git/ or .claude/.
			projectRoot := findProjectRoot(cwd)
			if projectRoot != "" {
				configPath := filepath.Join(projectRoot, ".claude", project.ConfigFileName)
				if _, statErr := os.Stat(configPath); os.IsNotExist(statErr) {
					// Flag existing settings.local.json without .claude-sync.yaml.
					settingsPath := filepath.Join(projectRoot, ".claude", "settings.local.json")
					if _, settingsErr := os.Stat(settingsPath); settingsErr == nil {
						result.ProjectUnmanagedDetected = true
					}

					// If profiles exist, this directory is eligible for project init.
					profileNames, _ := profiles.ListProfiles(syncDir)
					if len(profileNames) > 0 {
						result.ProjectInitEligible = true
						result.ProjectInitDir = projectRoot
						result.AvailableProfiles = profileNames
					}
				}
			}
		}
	}

	result.DuplicatePlugins = unresolvedDupes

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

// CleanupLegacyHooks removes session lifecycle hook entries from settings.json
// that are now handled by the claude-sync plugin. This is a migration step
// that runs on every pull but is idempotent.
func CleanupLegacyHooks(claudeDir string) error {
	settings, err := claudecode.ReadSettings(claudeDir)
	if err != nil {
		return nil // no settings = nothing to clean
	}

	var hooks map[string]json.RawMessage
	if hooksRaw, ok := settings["hooks"]; ok {
		if json.Unmarshal(hooksRaw, &hooks) != nil {
			return nil
		}
	}
	if hooks == nil {
		return nil
	}

	// Patterns that identify legacy hooks now handled by the plugin.
	legacyPatterns := map[string][]string{
		"SessionStart": {
			"claude-sync-session-start.sh",
			"claude-sync pull --auto",
		},
		"SessionEnd": {
			"claude-sync-session-end.sh",
			"claude-sync push --auto",
		},
		"Stop": {
			"claude-sync-stop-check.sh",
			"stop-next-steps.sh",
		},
	}

	changed := false
	for eventName, patterns := range legacyPatterns {
		raw, ok := hooks[eventName]
		if !ok {
			continue
		}

		type hookAction struct {
			Type    string `json:"type"`
			Command string `json:"command"`
		}
		type hookRule struct {
			Matcher string       `json:"matcher"`
			Hooks   []hookAction `json:"hooks"`
		}

		var rules []hookRule
		if json.Unmarshal(raw, &rules) != nil {
			continue
		}

		var kept []hookRule
		for _, rule := range rules {
			isLegacy := false
			for _, h := range rule.Hooks {
				for _, pat := range patterns {
					if strings.Contains(h.Command, pat) {
						isLegacy = true
						break
					}
				}
				if isLegacy {
					break
				}
			}
			if !isLegacy {
				kept = append(kept, rule)
			}
		}

		if len(kept) != len(rules) {
			changed = true
			if len(kept) == 0 {
				delete(hooks, eventName)
			} else {
				data, err := json.Marshal(kept)
				if err != nil {
					continue
				}
				hooks[eventName] = json.RawMessage(data)
			}
		}
	}

	if !changed {
		return nil
	}

	hooksData, err := json.Marshal(hooks)
	if err != nil {
		return fmt.Errorf("marshaling hooks: %w", err)
	}
	settings["hooks"] = json.RawMessage(hooksData)

	return claudecode.WriteSettings(claudeDir, settings)
}

// ApplySettings merges synced settings and hooks from config into settings.json.
// It preserves existing local settings and excluded fields.
// Returns (settingsApplied, hooksApplied, hooksSkipped, error).
// Hooks referencing non-existent script files are skipped with warnings.
func ApplySettings(claudeDir string, cfg config.Config) ([]string, []string, []string, error) {
	if len(cfg.Settings) == 0 && len(cfg.Hooks) == 0 {
		return nil, nil, nil, nil
	}

	settings, err := claudecode.ReadSettings(claudeDir)
	if err != nil {
		settings = make(map[string]json.RawMessage)
	}

	var settingsApplied []string
	for key, val := range cfg.Settings {
		if excludedSettingsFields[key] {
			continue
		}
		data, err := json.Marshal(val)
		if err != nil {
			continue
		}
		settings[key] = json.RawMessage(data)
		settingsApplied = append(settingsApplied, key)
	}

	var hooksApplied []string
	var hooksSkipped []string
	if len(cfg.Hooks) > 0 {
		var existingHooks map[string]json.RawMessage
		if hooksRaw, ok := settings["hooks"]; ok {
			json.Unmarshal(hooksRaw, &existingHooks)
		}
		if existingHooks == nil {
			existingHooks = make(map[string]json.RawMessage)
		}

		for hookName, hookData := range cfg.Hooks {
			missing, parseErr := findMissingHookScripts(hookData)
			if parseErr != nil {
				hooksSkipped = append(hooksSkipped, fmt.Sprintf("hook %q: %v", hookName, parseErr))
				continue
			}
			if len(missing) > 0 {
				for _, m := range missing {
					hooksSkipped = append(hooksSkipped, fmt.Sprintf("hook %q: script not found: %s", hookName, m))
				}
				continue
			}
			existingHooks[hookName] = hookData
			hooksApplied = append(hooksApplied, hookName)
		}

		hooksData, err := json.Marshal(existingHooks)
		if err != nil {
			return settingsApplied, nil, hooksSkipped, fmt.Errorf("marshaling hooks: %w", err)
		}
		settings["hooks"] = json.RawMessage(hooksData)
	}

	if err := claudecode.WriteSettings(claudeDir, settings); err != nil {
		return nil, nil, nil, fmt.Errorf("writing settings: %w", err)
	}

	sort.Strings(settingsApplied)
	sort.Strings(hooksApplied)
	sort.Strings(hooksSkipped)
	return settingsApplied, hooksApplied, hooksSkipped, nil
}

// scriptInterpreters are command prefixes that indicate the next argument is a script file.
var scriptInterpreters = map[string]bool{
	"bash": true, "sh": true, "zsh": true,
	"python": true, "python3": true,
	"ruby": true, "perl": true, "node": true,
}

// hookEntry represents a single hook entry in the Claude hook JSON format.
type hookEntry struct {
	Hooks []struct {
		Command string `json:"command"`
	} `json:"hooks"`
}

// findMissingHookScripts inspects a hook's JSON data for commands that reference
// external script files. Returns paths of scripts that don't exist on disk.
// Returns an error if the hook JSON is malformed.
func findMissingHookScripts(hookData json.RawMessage) ([]string, error) {
	var entries []hookEntry
	if err := json.Unmarshal(hookData, &entries); err != nil {
		return nil, fmt.Errorf("malformed hook JSON: %w", err)
	}
	var missing []string
	for _, entry := range entries {
		for _, h := range entry.Hooks {
			path := extractScriptPath(h.Command)
			if path == "" {
				continue
			}
			expanded := expandHome(path)
			if _, err := os.Stat(expanded); err != nil {
				if os.IsNotExist(err) {
					missing = append(missing, path)
				} else {
					return nil, fmt.Errorf("checking script %s: %w", path, err)
				}
			}
		}
	}
	return missing, nil
}

// extractScriptPath returns the script file path from a command string like
// "bash ~/.claude/hooks/foo.sh" or "python3 /path/to/script.py".
// Returns empty string if the command doesn't reference an external script.
func extractScriptPath(command string) string {
	parts := strings.Fields(command)
	if len(parts) < 2 {
		return ""
	}
	if !scriptInterpreters[parts[0]] {
		return ""
	}
	// Skip flags (e.g. -e, -x) to find the first positional argument.
	for _, arg := range parts[1:] {
		if strings.HasPrefix(arg, "-") {
			continue
		}
		path := strings.Trim(arg, `"'`)
		if strings.HasPrefix(path, "/") || strings.HasPrefix(path, "~/") {
			return path
		}
		return ""
	}
	return ""
}

// expandHome replaces a leading ~/ with the user's home directory.
func expandHome(path string) string {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return path
		}
		return filepath.Join(home, path[2:])
	}
	return path
}

// applyPermissions additively merges permissions into settings.json.
func applyPermissions(claudeDir string, perms config.Permissions) error {
	settings, err := claudecode.ReadSettings(claudeDir)
	if err != nil {
		settings = make(map[string]json.RawMessage)
	}

	var existingPerms struct {
		Allow []string `json:"allow"`
		Deny  []string `json:"deny"`
	}
	if permRaw, ok := settings["permissions"]; ok {
		json.Unmarshal(permRaw, &existingPerms)
	}

	mergedAllow := appendUniqueStrings(existingPerms.Allow, perms.Allow)
	mergedDeny := appendUniqueStrings(existingPerms.Deny, perms.Deny)

	permData, err := json.Marshal(map[string]any{
		"allow": mergedAllow,
		"deny":  mergedDeny,
	})
	if err != nil {
		return fmt.Errorf("marshaling permissions: %w", err)
	}
	settings["permissions"] = json.RawMessage(permData)

	return claudecode.WriteSettings(claudeDir, settings)
}

// restoreCommandsSkills copies command .md files and skill directories from
// syncDir to claudeDir, filtered by the effective key lists.
// Tracks content hashes to avoid overwriting locally-modified files.
// Returns (restored, skipped) counts for commands and skills.
func restoreCommandsSkills(claudeDir, syncDir string, commandKeys, skillKeys []string) (cmdsRestored, cmdsSkipped, skillsRestored, skillsSkipped int) {
	// Build set of command filenames from keys (e.g. "cmd:global:review-pr" → "review-pr.md").
	cmdNames := make(map[string]bool)
	for _, k := range commandKeys {
		parts := strings.Split(k, ":")
		if len(parts) >= 2 {
			cmdNames[parts[len(parts)-1]+".md"] = true
		}
	}

	// Copy commands with local-modification protection.
	srcCmdsDir := filepath.Join(syncDir, "commands")
	if entries, err := os.ReadDir(srcCmdsDir); err == nil {
		dstCmdsDir := filepath.Join(claudeDir, "commands")
		os.MkdirAll(dstCmdsDir, 0755)

		cmdHashes, _ := claudecode.ReadContentHashes(dstCmdsDir)

		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
				continue
			}
			if len(cmdNames) > 0 && !cmdNames[entry.Name()] {
				continue
			}
			srcPath := filepath.Join(srcCmdsDir, entry.Name())
			dstPath := filepath.Join(dstCmdsDir, entry.Name())

			srcData, err := os.ReadFile(srcPath)
			if err != nil {
				continue
			}

			// Check if local file was modified since last sync.
			if localData, err := os.ReadFile(dstPath); err == nil {
				localHash := claudemd.ContentHash(string(localData))
				if storedHash, ok := cmdHashes.Hashes[entry.Name()]; ok && localHash != storedHash {
					// Local file was modified — skip overwrite.
					cmdsSkipped++
					continue
				}
			}

			if os.WriteFile(dstPath, srcData, 0644) == nil {
				cmdHashes.Hashes[entry.Name()] = claudemd.ContentHash(string(srcData))
				cmdsRestored++
			}
		}

		claudecode.WriteContentHashes(dstCmdsDir, cmdHashes)
	}

	// Build set of skill names from keys (e.g. "skill:global:brainstorming" → "brainstorming").
	skillNames := make(map[string]bool)
	for _, k := range skillKeys {
		parts := strings.Split(k, ":")
		if len(parts) >= 2 {
			skillNames[parts[len(parts)-1]] = true
		}
	}

	// Copy skills (entire directories) with local-modification protection.
	srcSkillsDir := filepath.Join(syncDir, "skills")
	if entries, err := os.ReadDir(srcSkillsDir); err == nil {
		skillsDir := filepath.Join(claudeDir, "skills")
		os.MkdirAll(skillsDir, 0755)

		skillHashes, _ := claudecode.ReadContentHashes(skillsDir)

		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			if len(skillNames) > 0 && !skillNames[entry.Name()] {
				continue
			}
			srcDir := filepath.Join(srcSkillsDir, entry.Name())
			dstDir := filepath.Join(claudeDir, "skills", entry.Name())

			// Hash the SKILL.md content for modification detection.
			srcSkillMD := filepath.Join(srcDir, "SKILL.md")
			srcData, err := os.ReadFile(srcSkillMD)
			if err != nil {
				// No SKILL.md in source — copy without tracking.
				if copyDir(srcDir, dstDir) == nil {
					skillsRestored++
				}
				continue
			}

			// Check if local SKILL.md was modified since last sync.
			dstSkillMD := filepath.Join(dstDir, "SKILL.md")
			if localData, err := os.ReadFile(dstSkillMD); err == nil {
				localHash := claudemd.ContentHash(string(localData))
				if storedHash, ok := skillHashes.Hashes[entry.Name()]; ok && localHash != storedHash {
					skillsSkipped++
					continue
				}
			}

			if copyDir(srcDir, dstDir) == nil {
				skillHashes.Hashes[entry.Name()] = claudemd.ContentHash(string(srcData))
				skillsRestored++
			}
		}

		claudecode.WriteContentHashes(skillsDir, skillHashes)
	}

	return
}

// appendUniqueStrings appends items from add to base without duplicates.
func appendUniqueStrings(base, add []string) []string {
	seen := make(map[string]bool, len(base))
	for _, s := range base {
		seen[s] = true
	}
	result := make([]string, len(base))
	copy(result, base)
	for _, s := range add {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}
	return result
}

// detectStalePlugins returns synced plugin keys whose cache is out of date.
// For directory-based marketplaces, it compares content hashes of the source
// files. For remote marketplaces, it uses the existing version-string comparison.
func detectStalePlugins(claudeDir string, syncedKeys []string) []string {
	installed, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil
	}

	storedHashes, _ := claudecode.ReadPluginContentHashes(claudeDir)

	var stale []string
	for _, key := range syncedKeys {
		installations, ok := installed.Plugins[key]
		if !ok || len(installations) == 0 {
			continue
		}

		// Extract marketplace name from "plugin@marketplace".
		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			continue
		}
		marketplaceName := parts[1]

		srcType := marketplace.MarketplaceSourceType(claudeDir, marketplaceName)

		if srcType == "directory" {
			// Content-hash comparison for directory-based marketplaces.
			sourceDir, err := marketplace.ResolvePluginSourceDir(claudeDir, key)
			if err != nil {
				continue
			}
			currentHash, err := marketplace.ComputePluginContentHash(sourceDir)
			if err != nil {
				continue
			}
			storedHash := storedHashes.Hashes[key]
			if storedHash == "" || storedHash != currentHash {
				stale = append(stale, key)
			}
		} else {
			// Version-string comparison for remote marketplaces.
			installedVersion := installations[0].Version
			mkplVersion, err := marketplace.ReadMarketplacePluginVersion(claudeDir, key)
			if err != nil {
				continue
			}
			if marketplace.HasUpdate(installedVersion, mkplVersion) {
				stale = append(stale, key)
			}
		}
	}
	return stale
}

// refreshStalePlugins removes old cache directories and reinstalls plugins
// to pick up version changes from the marketplace source.
func refreshStalePlugins(claudeDir string, staleKeys []string, installed *claudecode.InstalledPlugins, quiet bool) (updated, failed []string) {
	for _, key := range staleKeys {
		installations, ok := installed.Plugins[key]
		if !ok || len(installations) == 0 {
			failed = append(failed, key)
			continue
		}

		installPath := installations[0].InstallPath
		if installPath != "" {
			os.RemoveAll(installPath)
		}

		if !quiet {
			fmt.Printf("  Updating %s...\n", key)
		}

		if err := installPlugin(key); err != nil {
			failed = append(failed, key)
			if !quiet {
				fmt.Fprintf(os.Stderr, "  ✗ %s: %v\n", key, err)
			}
		} else {
			updated = append(updated, key)
			if !quiet {
				fmt.Printf("  ✓ %s\n", key)
			}
		}
	}

	// Record content hashes for successfully updated directory-based plugins.
	if len(updated) > 0 {
		updatePluginContentHashes(claudeDir, updated)
	}

	return updated, failed
}

// updatePluginContentHashes computes and stores content hashes for
// directory-based plugins in the sidecar file.
func updatePluginContentHashes(claudeDir string, keys []string) {
	pch, _ := claudecode.ReadPluginContentHashes(claudeDir)

	for _, key := range keys {
		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			continue
		}
		marketplaceName := parts[1]

		if marketplace.MarketplaceSourceType(claudeDir, marketplaceName) != "directory" {
			continue
		}

		sourceDir, err := marketplace.ResolvePluginSourceDir(claudeDir, key)
		if err != nil {
			continue
		}

		hash, err := marketplace.ComputePluginContentHash(sourceDir)
		if err != nil {
			continue
		}

		pch.Hashes[key] = hash
	}

	_ = claudecode.WritePluginContentHashes(claudeDir, pch)
}

// pullSubscriptions fetches all subscriptions and merges their items into the
// local config.yaml. In auto mode, only previously-accepted items are applied;
// new items are queued for the next interactive pull. In interactive mode,
// all resolved items are applied (the approval TUI is handled by the CLI layer).
func pullSubscriptions(syncDir string, auto, quiet bool) error {
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil // no config = nothing to do
	}
	cfg, err := config.Parse(cfgData)
	if err != nil || len(cfg.Subscriptions) == 0 {
		return nil
	}

	state, _ := subscriptions.ReadState(syncDir)

	// Fetch all subscriptions.
	results := subscriptions.FetchAll(syncDir, cfg.Subscriptions, state)
	for _, r := range results {
		if r.Error != nil {
			if !quiet {
				fmt.Fprintf(os.Stderr, "Warning: subscription %q: %v\n", r.Name, r.Error)
			}
			continue
		}
		// Update state with new SHA.
		subscriptions.UpdateState(&state, r.Name, r.CommitSHA, nil)
	}

	// Merge all subscription items.
	merged, conflicts, err := subscriptions.MergeAll(syncDir, cfg.Subscriptions, cfg)
	if err != nil {
		return fmt.Errorf("merging subscriptions: %w", err)
	}
	if len(conflicts) > 0 {
		if !quiet {
			fmt.Fprintf(os.Stderr, "\n%s\n", subscriptions.FormatConflicts(conflicts))
		}
		return fmt.Errorf("%d subscription conflict(s) found", len(conflicts))
	}

	// Apply merged items to local config.
	subscriptions.ApplyToConfig(&cfg, merged)

	// Write updated config.
	newData, err := config.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config after subscription merge: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), newData, 0644); err != nil {
		return fmt.Errorf("writing config after subscription merge: %w", err)
	}

	// Save state.
	subscriptions.WriteState(syncDir, state)

	return nil
}

// findProjectRoot walks up from dir looking for .git/ or .claude/ to identify
// the root of a project. Returns empty string if no project root is found.
// Stops at the user's home directory to avoid walking all the way to /.
func findProjectRoot(dir string) string {
	home, _ := os.UserHomeDir()
	dir, err := filepath.Abs(dir)
	if err != nil {
		return ""
	}
	for {
		// Don't treat home itself as a project root.
		if dir == home {
			return ""
		}
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		if _, err := os.Stat(filepath.Join(dir, ".claude")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

