package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/ruminaider/claude-sync/internal/project"
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
	SkippedCategories      []string
	ActiveProfile          string // active profile applied (empty = base only)
	PermissionsApplied     bool
	ClaudeMDAssembled      bool
	MCPApplied             []string
	MCPEnvWarnings         []string // unresolved ${VAR} references
	MCPProjectApplied      map[string][]string // project path -> server names written there
	KeybindingsApplied     bool
	PendingHighRisk            []approval.Change
	ProjectSettingsApplied     bool
	ProjectUnmanagedDetected   bool // CWD has settings.local.json but no .claude-sync.yaml
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
		ActiveProfile:    activeName,
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

			// Apply settings and hooks (hooks skipped in auto mode).
			settingsCfg := cfg
			if skipHooks {
				settingsCfg.Hooks = nil
			}
			applied, hookNames, applyErr := ApplySettings(claudeDir, settingsCfg)
			if applyErr != nil {
				if !quiet {
					fmt.Fprintf(os.Stderr, "Warning: failed to apply settings: %v\n", applyErr)
				}
			} else {
				result.SettingsApplied = applied
				result.HooksApplied = hookNames
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
				assembled, asmErr := claudemd.AssembleFromDir(syncDir, includes)
				if asmErr == nil && assembled != "" {
					claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
					if os.WriteFile(claudeMDPath, []byte(assembled), 0644) == nil {
						result.ClaudeMDAssembled = true
					}
				}
			}

			// Apply MCP servers (additive merge, skipped in auto mode).
			mcpServers := cfg.MCP
			if activeProfile != nil {
				mcpServers = profiles.MergeMCP(mcpServers, *activeProfile)
			}

			// Resolve ${VAR} env var references before writing.
			if len(mcpServers) > 0 {
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
				if claudecode.WriteKeybindings(claudeDir, kbConfig) == nil {
					result.KeybindingsApplied = true
				}
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
		// Detect unmanaged projects (settings.local.json exists but no .claude-sync.yaml).
		if cwd, cwdErr := os.Getwd(); cwdErr == nil {
			settingsPath := filepath.Join(cwd, ".claude", "settings.local.json")
			configPath := filepath.Join(cwd, ".claude", project.ConfigFileName)
			if _, statErr := os.Stat(settingsPath); statErr == nil {
				if _, statErr2 := os.Stat(configPath); os.IsNotExist(statErr2) {
					result.ProjectUnmanagedDetected = true
				}
			}
		}
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

// ApplySettings merges synced settings and hooks from config into settings.json.
// It preserves existing local settings and excluded fields.
func ApplySettings(claudeDir string, cfg config.Config) ([]string, []string, error) {
	if len(cfg.Settings) == 0 && len(cfg.Hooks) == 0 {
		return nil, nil, nil
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
	if len(cfg.Hooks) > 0 {
		var existingHooks map[string]json.RawMessage
		if hooksRaw, ok := settings["hooks"]; ok {
			json.Unmarshal(hooksRaw, &existingHooks)
		}
		if existingHooks == nil {
			existingHooks = make(map[string]json.RawMessage)
		}

		for hookName, hookData := range cfg.Hooks {
			existingHooks[hookName] = hookData
			hooksApplied = append(hooksApplied, hookName)
		}

		hooksData, err := json.Marshal(existingHooks)
		if err != nil {
			return settingsApplied, nil, fmt.Errorf("marshaling hooks: %w", err)
		}
		settings["hooks"] = json.RawMessage(hooksData)
	}

	if err := claudecode.WriteSettings(claudeDir, settings); err != nil {
		return nil, nil, fmt.Errorf("writing settings: %w", err)
	}

	sort.Strings(settingsApplied)
	sort.Strings(hooksApplied)
	return settingsApplied, hooksApplied, nil
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

