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
	"github.com/ruminaider/claude-sync/internal/cmdskill"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/marketplace"
	forkedplugins "github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// InitScanResult holds what was found during scanning without writing anything.
type InitScanResult struct {
	PluginKeys      []string                   // all plugin keys
	Upstream        []string                   // portable marketplace plugins
	AutoForked      []string                   // non-portable plugins that would be forked
	Settings        map[string]any             // syncable settings found
	Hooks           map[string]json.RawMessage // hooks found (hookName -> raw JSON)
	Permissions     config.Permissions         // permissions found
	ClaudeMDContent string                     // raw content from ~/.claude/CLAUDE.md
	ClaudeMDSections []claudemd.Section        // pre-split sections for picker display
	MCP             map[string]json.RawMessage // MCP server configs found
	MCPSecrets      []DetectedSecret           // secrets detected in MCP configs
	Keybindings     map[string]any             // keybindings found
	CommandsSkills  *cmdskill.ScanResult
}

// InitResult describes how plugins were categorized during init.
type InitResult struct {
	Upstream            []string // portable marketplace plugins
	AutoForked          []string // non-portable plugins copied into sync repo
	ExcludedPlugins     []string // plugins excluded by user selection
	RemotePushed        bool     // whether the initial commit was pushed to a remote
	IncludedSettings    []string // settings keys written to config
	IncludedHooks       []string // hook names written to config
	ProfileNames        []string // profiles created
	ActiveProfile       string   // profile activated on this machine
	PermissionsIncluded bool     // whether permissions were included
	ClaudeMDFragments   []string // CLAUDE.md fragment names created
	MCPIncluded         []string // MCP server names included
	KeybindingsIncluded bool     // whether keybindings were included
	CommandsIncluded    int
	SkillsIncluded      int
}

// InitOptions configures what Init includes in the sync config.
type InitOptions struct {
	ClaudeDir         string
	SyncDir           string
	RemoteURL         string
	IncludeSettings   bool                       // whether to write settings to config
	SettingsFilter    []string                   // optional: specific keys to include (nil = all when IncludeSettings is true)
	IncludeHooks      map[string]json.RawMessage // specific hooks to include (nil = all, empty = none)
	IncludePlugins    []string                   // specific plugin keys to include (nil = all, empty = none)
	Profiles          map[string]profiles.Profile // nil = no profiles, non-nil = write profile files
	ActiveProfile     string                      // profile to activate on this machine (empty = none)
	Permissions       config.Permissions          // permissions to include
	ImportClaudeMD             bool                        // whether to import CLAUDE.md
	ClaudeMDFragments          []string                    // fragment names to include (nil = all when ImportClaudeMD is true)
	ProjectClaudeMDFragments   []string                    // selected project CLAUDE.md fragment keys (qualified with "::")
	MCP               map[string]json.RawMessage  // MCP server configs to include
	Keybindings       map[string]any              // keybindings to include
	Commands          []string                    // selected command keys to include
	Skills            []string                    // selected skill keys to include
}

// Fields from settings.json that should NOT be synced.
var excludedSettingsFields = map[string]bool{
	"enabledPlugins": true,
	"hooks":          true,
	"permissions":    true,
}

// InitScan reads the current Claude Code setup and returns what was found
// without writing anything. Use this for the interactive selection phase.
func InitScan(claudeDir string) (*InitScanResult, error) {
	if !claudecode.DirExists(claudeDir) {
		return nil, fmt.Errorf("Claude Code directory not found at %s. Run Claude Code at least once first", claudeDir)
	}

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, fmt.Errorf("reading plugins: %w", err)
	}

	// Upgrade directory-source marketplaces that are actually GitHub repos,
	// so the scan correctly categorizes their plugins as upstream.
	marketplace.UpgradeDirectoryMarketplaces(claudeDir)

	pluginKeys := plugins.PluginKeys()
	sort.Strings(pluginKeys)

	result := &InitScanResult{
		Settings: make(map[string]any),
		Hooks:    make(map[string]json.RawMessage),
	}

	for _, key := range pluginKeys {
		installations := plugins.Plugins[key]

		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			result.Upstream = append(result.Upstream, key)
			result.PluginKeys = append(result.PluginKeys, key)
			continue
		}
		_, mkt := parts[0], parts[1]

		// Skip plugins from our own managed marketplace — these are
		// artifacts of a previous init and shouldn't be re-scanned.
		if mkt == forkedplugins.MarketplaceName {
			continue
		}

		if marketplace.IsPortable(claudeDir, mkt) {
			result.Upstream = append(result.Upstream, key)
		} else {
			installPath := findInstallPath(installations)
			if installPath == "" {
				result.Upstream = append(result.Upstream, key)
			} else {
				result.AutoForked = append(result.AutoForked, key)
			}
		}
		result.PluginKeys = append(result.PluginKeys, key)
	}

	settingsRaw, err := claudecode.ReadSettings(claudeDir)
	if err == nil {
		for key, raw := range settingsRaw {
			if excludedSettingsFields[key] {
				continue
			}
			var val any
			if json.Unmarshal(raw, &val) == nil && val != nil {
				result.Settings[key] = val
			}
		}

		if hooksRaw, ok := settingsRaw["hooks"]; ok {
			var hooks map[string]json.RawMessage
			if json.Unmarshal(hooksRaw, &hooks) == nil {
				for hookName, hookData := range hooks {
					result.Hooks[hookName] = hookData
				}
			}
		}

		if permRaw, ok := settingsRaw["permissions"]; ok {
			var permData struct {
				Allow []string `json:"allow"`
				Deny  []string `json:"deny"`
			}
			if json.Unmarshal(permRaw, &permData) == nil {
				result.Permissions = config.Permissions{
					Allow: permData.Allow,
					Deny:  permData.Deny,
				}
			}
		}
	}

	// Read CLAUDE.md
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	if data, err := os.ReadFile(claudeMDPath); err == nil {
		result.ClaudeMDContent = string(data)
		if result.ClaudeMDContent != "" {
			result.ClaudeMDSections = claudemd.Split(result.ClaudeMDContent)
		}
	}

	// Read MCP config
	mcp, err := claudecode.ReadMCPConfig(claudeDir)
	if err == nil && len(mcp) > 0 {
		result.MCP = mcp
		result.MCPSecrets = DetectMCPSecrets(mcp)
	}

	// Read keybindings
	kb, err := claudecode.ReadKeybindings(claudeDir)
	if err == nil && len(kb) > 0 {
		result.Keybindings = kb
	}

	// Scan commands and skills
	cs, csErr := cmdskill.ScanAll(claudeDir, nil)
	if csErr == nil {
		result.CommandsSkills = cs
	}

	return result, nil
}

// Init scans the current Claude Code setup and creates ~/.claude-sync/config.yaml.
// If RemoteURL is non-empty, it adds the remote and pushes after the initial commit.
func Init(opts InitOptions) (*InitResult, error) {
	claudeDir := opts.ClaudeDir
	syncDir := opts.SyncDir

	if !claudecode.DirExists(claudeDir) {
		return nil, fmt.Errorf("Claude Code directory not found at %s. Run Claude Code at least once first", claudeDir)
	}

	if _, err := os.Stat(syncDir); err == nil {
		return nil, fmt.Errorf("%s already exists. Run 'claude-sync pull' to update, or remove it first", syncDir)
	}

	if err := os.MkdirAll(syncDir, 0755); err != nil {
		return nil, fmt.Errorf("creating sync directory: %w", err)
	}

	result, forkedNames, err := buildAndWriteConfig(opts)
	if err != nil {
		return nil, err
	}

	gitignore := "user-preferences.yaml\n.last_fetch\nplugins/.claude-plugin/\nactive-profile\npending-changes.yaml\n"
	if err := os.WriteFile(filepath.Join(syncDir, ".gitignore"), []byte(gitignore), 0644); err != nil {
		return nil, fmt.Errorf("writing .gitignore: %w", err)
	}

	// Ensure the plugins directory exists in git even when empty or when its
	// only contents are gitignored. Without this, GitHub's UI collapses the
	// path to show plugins/<only-child> instead of plugins/ as a directory.
	pluginsDir := filepath.Join(syncDir, "plugins")
	os.MkdirAll(pluginsDir, 0755)
	os.WriteFile(filepath.Join(pluginsDir, ".gitkeep"), []byte{}, 0644)

	if err := git.Init(syncDir); err != nil {
		return nil, fmt.Errorf("initializing git repo: %w", err)
	}

	if err := git.Add(syncDir, "."); err != nil {
		return nil, fmt.Errorf("staging files: %w", err)
	}
	if err := git.Commit(syncDir, "Initial claude-sync config"); err != nil {
		return nil, fmt.Errorf("creating initial commit: %w", err)
	}

	if len(forkedNames) > 0 {
		if err := forkedplugins.RegisterLocalMarketplace(claudeDir, syncDir); err != nil {
			return nil, fmt.Errorf("registering local marketplace: %w", err)
		}
	}

	if opts.RemoteURL != "" {
		if err := git.RemoteAdd(syncDir, "origin", opts.RemoteURL); err != nil {
			return nil, fmt.Errorf("adding remote: %w", err)
		}
		branch, err := git.CurrentBranch(syncDir)
		if err != nil {
			return nil, fmt.Errorf("detecting branch: %w", err)
		}
		if err := git.PushWithUpstream(syncDir, "origin", branch); err != nil {
			return nil, fmt.Errorf("pushing to remote: %w", err)
		}
		result.RemotePushed = true
	}

	return result, nil
}

// Update updates an existing ~/.claude-sync/ config in place.
// Unlike Init, it does not create the directory, write .gitignore, or init git.
func Update(opts InitOptions) (*InitResult, error) {
	syncDir := opts.SyncDir

	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return nil, fmt.Errorf("no config found at %s", syncDir)
	}

	if !claudecode.DirExists(opts.ClaudeDir) {
		return nil, fmt.Errorf("Claude Code directory not found at %s. Run Claude Code at least once first", opts.ClaudeDir)
	}

	// Clean up old profile files that are no longer in the new config.
	profilesDir := filepath.Join(syncDir, "profiles")
	if existingNames, err := profiles.ListProfiles(syncDir); err == nil {
		newProfileSet := make(map[string]bool)
		for name := range opts.Profiles {
			newProfileSet[name] = true
		}
		for _, name := range existingNames {
			if !newProfileSet[name] {
				os.Remove(filepath.Join(profilesDir, name+".yaml"))
			}
		}
	}

	result, forkedNames, err := buildAndWriteConfig(opts)
	if err != nil {
		return nil, err
	}

	// Ensure plugins/.gitkeep exists for repos initialized before it was added.
	pluginsDir := filepath.Join(syncDir, "plugins")
	os.MkdirAll(pluginsDir, 0755)
	os.WriteFile(filepath.Join(pluginsDir, ".gitkeep"), []byte{}, 0644)

	if len(forkedNames) > 0 {
		if err := forkedplugins.RegisterLocalMarketplace(opts.ClaudeDir, syncDir); err != nil {
			return nil, fmt.Errorf("registering local marketplace: %w", err)
		}
	}

	return result, nil
}

// buildAndWriteConfig contains the shared config-building logic used by both
// Init and Update. It categorizes plugins, filters settings/hooks, writes
// config.yaml, imports CLAUDE.md, writes profiles, and copies commands/skills.
func buildAndWriteConfig(opts InitOptions) (*InitResult, []string, error) {
	claudeDir := opts.ClaudeDir
	syncDir := opts.SyncDir

	plugins, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading plugins: %w", err)
	}

	// Upgrade directory-source marketplaces that are actually GitHub repos.
	// This must happen before plugin categorization so IsPortable() sees the
	// corrected source type.
	marketplace.UpgradeDirectoryMarketplaces(claudeDir)

	pluginKeys := plugins.PluginKeys()
	sort.Strings(pluginKeys)

	result := &InitResult{}
	var upstream []string
	var forkedNames []string

	// Build plugin include set for filtering.
	var includeSet map[string]bool
	if opts.IncludePlugins != nil {
		includeSet = make(map[string]bool, len(opts.IncludePlugins))
		for _, k := range opts.IncludePlugins {
			includeSet[k] = true
		}
	}

	for _, key := range pluginKeys {
		installations := plugins.Plugins[key]

		parts := strings.SplitN(key, "@", 2)
		if len(parts) != 2 {
			// Check plugin filter before including.
			if includeSet != nil && !includeSet[key] {
				result.ExcludedPlugins = append(result.ExcludedPlugins, key)
				continue
			}
			upstream = append(upstream, key)
			result.Upstream = append(result.Upstream, key)
			continue
		}
		name, mkt := parts[0], parts[1]

		// Skip plugins from our own managed marketplace — these are
		// artifacts of a previous init and shouldn't be re-scanned.
		if mkt == forkedplugins.MarketplaceName {
			continue
		}

		// Check plugin filter before including.
		if includeSet != nil && !includeSet[key] {
			result.ExcludedPlugins = append(result.ExcludedPlugins, key)
			continue
		}

		if marketplace.IsPortable(claudeDir, mkt) {
			upstream = append(upstream, key)
			result.Upstream = append(result.Upstream, key)
		} else {
			installPath := findInstallPath(installations)
			if installPath == "" {
				upstream = append(upstream, key)
				result.Upstream = append(result.Upstream, key)
				continue
			}

			dstDir := filepath.Join(syncDir, "plugins", name)
			if err := copyDir(installPath, dstDir); err != nil {
				upstream = append(upstream, key)
				result.Upstream = append(result.Upstream, key)
				continue
			}

			forkedNames = append(forkedNames, name)
			result.AutoForked = append(result.AutoForked, key)
		}
	}

	// Scan settings and hooks from Claude Code.
	syncedSettings := make(map[string]any)
	syncedHooks := make(map[string]json.RawMessage)

	settingsRaw, err := claudecode.ReadSettings(claudeDir)
	if err == nil {
		for key, raw := range settingsRaw {
			if excludedSettingsFields[key] {
				continue
			}
			var val any
			if json.Unmarshal(raw, &val) == nil && val != nil {
				syncedSettings[key] = val
			}
		}

		if hooksRaw, ok := settingsRaw["hooks"]; ok {
			var hooks map[string]json.RawMessage
			if json.Unmarshal(hooksRaw, &hooks) == nil {
				for hookName, hookData := range hooks {
					syncedHooks[hookName] = hookData
				}
			}
		}
	}

	// Apply settings filter.
	var cfgSettings map[string]any
	if opts.IncludeSettings {
		if opts.SettingsFilter == nil {
			// nil = include all settings
			cfgSettings = syncedSettings
			for k := range syncedSettings {
				result.IncludedSettings = append(result.IncludedSettings, k)
			}
		} else {
			// non-nil = include only specified keys
			cfgSettings = make(map[string]any)
			for _, k := range opts.SettingsFilter {
				if v, ok := syncedSettings[k]; ok {
					cfgSettings[k] = v
					result.IncludedSettings = append(result.IncludedSettings, k)
				}
			}
		}
		sort.Strings(result.IncludedSettings)
	}

	// Apply hooks filter.
	var cfgHooks map[string]json.RawMessage
	if opts.IncludeHooks == nil {
		// nil = include all hooks (backward compat).
		cfgHooks = syncedHooks
		for k := range syncedHooks {
			result.IncludedHooks = append(result.IncludedHooks, k)
		}
	} else {
		// Non-nil = include only specified hooks.
		cfgHooks = make(map[string]json.RawMessage)
		for k, v := range opts.IncludeHooks {
			cfgHooks[k] = v
			result.IncludedHooks = append(result.IncludedHooks, k)
		}
	}
	sort.Strings(result.IncludedHooks)

	// Collect unique marketplace IDs from upstream plugins to detect custom
	// marketplaces that need entries in the config's marketplaces section.
	mktIDs := make(map[string]bool)
	for _, key := range upstream {
		parts := strings.SplitN(key, "@", 2)
		if len(parts) == 2 {
			mktIDs[parts[1]] = true
		}
	}
	var mktIDList []string
	for id := range mktIDs {
		mktIDList = append(mktIDList, id)
	}
	customMkts := marketplace.CollectCustomMarketplaceSources(claudeDir, mktIDList)

	cfg := config.Config{
		Version:      "1.0.0",
		Upstream:     upstream,
		Pinned:       map[string]string{},
		Forked:       forkedNames,
		Excluded:     result.ExcludedPlugins,
		Settings:     cfgSettings,
		Hooks:        cfgHooks,
		Permissions:  opts.Permissions,
		MCP:          opts.MCP,
		Keybindings:  opts.Keybindings,
		Commands:     opts.Commands,
		Skills:       opts.Skills,
		Marketplaces: customMkts,
	}

	cfgData, err := config.Marshal(cfg)
	if err != nil {
		return nil, nil, fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644); err != nil {
		return nil, nil, fmt.Errorf("writing config: %w", err)
	}

	// Import CLAUDE.md if requested.
	if opts.ImportClaudeMD {
		claudeMDPath := filepath.Join(opts.ClaudeDir, "CLAUDE.md")
		claudeMDData, readErr := os.ReadFile(claudeMDPath)
		if readErr == nil {
			// Always import all sections to disk (fragment files + manifest).
			importResult, importErr := claudemd.ImportClaudeMD(syncDir, string(claudeMDData))
			if importErr != nil {
				return nil, nil, fmt.Errorf("importing CLAUDE.md: %w", importErr)
			}
			// Determine which fragments to include in config.
			if opts.ClaudeMDFragments == nil {
				// nil = include all fragments
				cfg.ClaudeMD.Include = importResult.FragmentNames
				result.ClaudeMDFragments = importResult.FragmentNames
			} else {
				// non-nil = include only user-selected fragments
				cfg.ClaudeMD.Include = opts.ClaudeMDFragments
				result.ClaudeMDFragments = opts.ClaudeMDFragments
			}
			// Re-write config with updated ClaudeMD.Include.
			cfgData, err = config.Marshal(cfg)
			if err != nil {
				return nil, nil, fmt.Errorf("marshaling config with CLAUDE.md: %w", err)
			}
			if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644); err != nil {
				return nil, nil, fmt.Errorf("writing config with CLAUDE.md: %w", err)
			}
		}
	}

	// Append project CLAUDE.md fragment selections to config. These are qualified
	// keys (with "::") that track which project fragments the user selected.
	if len(opts.ProjectClaudeMDFragments) > 0 {
		cfg.ClaudeMD.Include = append(cfg.ClaudeMD.Include, opts.ProjectClaudeMDFragments...)
		cfgData, err = config.Marshal(cfg)
		if err != nil {
			return nil, nil, fmt.Errorf("marshaling config with project CLAUDE.md: %w", err)
		}
		if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), cfgData, 0644); err != nil {
			return nil, nil, fmt.Errorf("writing config with project CLAUDE.md: %w", err)
		}
	}

	// Set result fields for new surfaces.
	if len(opts.Permissions.Allow) > 0 || len(opts.Permissions.Deny) > 0 {
		result.PermissionsIncluded = true
	}
	if len(opts.MCP) > 0 {
		result.MCPIncluded = make([]string, 0, len(opts.MCP))
		for k := range opts.MCP {
			result.MCPIncluded = append(result.MCPIncluded, k)
		}
		sort.Strings(result.MCPIncluded)
	}
	if len(opts.Keybindings) > 0 {
		result.KeybindingsIncluded = true
	}

	// Copy selected commands and skills to sync directory.
	if len(opts.Commands) > 0 || len(opts.Skills) > 0 {
		cs, csErr := cmdskill.ScanAll(claudeDir, nil)
		if csErr == nil {
			selectedSet := make(map[string]bool)
			for _, k := range opts.Commands {
				selectedSet[k] = true
			}
			for _, k := range opts.Skills {
				selectedSet[k] = true
			}
			for _, item := range cs.Items {
				if !selectedSet[item.Key()] {
					continue
				}
				if item.Source == cmdskill.SourcePlugin {
					continue // don't copy plugin items
				}
				switch item.Type {
				case cmdskill.TypeCommand:
					dstDir := filepath.Join(syncDir, "commands")
					os.MkdirAll(dstDir, 0755)
					dstPath := filepath.Join(dstDir, filepath.Base(item.FilePath))
					data, err := os.ReadFile(item.FilePath)
					if err == nil {
						os.WriteFile(dstPath, data, 0644)
						result.CommandsIncluded++
					}
				case cmdskill.TypeSkill:
					// Copy entire skill directory
					skillName := item.Name
					srcDir := filepath.Dir(item.FilePath) // parent of SKILL.md
					dstDir := filepath.Join(syncDir, "skills", skillName)
					if err := copyDir(srcDir, dstDir); err == nil {
						result.SkillsIncluded++
					}
				}
			}
		}
	}

	// Write profile files if provided.
	if len(opts.Profiles) > 0 {
		profilesDir := filepath.Join(syncDir, "profiles")
		if err := os.MkdirAll(profilesDir, 0755); err != nil {
			return nil, nil, fmt.Errorf("creating profiles directory: %w", err)
		}

		var profileNames []string
		for name := range opts.Profiles {
			profileNames = append(profileNames, name)
		}
		sort.Strings(profileNames)

		for _, name := range profileNames {
			data, err := profiles.MarshalProfile(opts.Profiles[name])
			if err != nil {
				return nil, nil, fmt.Errorf("marshaling profile %q: %w", name, err)
			}
			if err := os.WriteFile(filepath.Join(profilesDir, name+".yaml"), data, 0644); err != nil {
				return nil, nil, fmt.Errorf("writing profile %q: %w", name, err)
			}
		}
		result.ProfileNames = profileNames
	}

	// Write active profile if specified.
	if opts.ActiveProfile != "" {
		if err := profiles.WriteActiveProfile(syncDir, opts.ActiveProfile); err != nil {
			return nil, nil, fmt.Errorf("writing active profile: %w", err)
		}
	}
	result.ActiveProfile = opts.ActiveProfile

	return result, forkedNames, nil
}

// findInstallPath returns the first non-empty InstallPath from the installations.
func findInstallPath(installations []claudecode.PluginInstallation) string {
	for _, inst := range installations {
		if inst.InstallPath != "" {
			return inst.InstallPath
		}
	}
	return ""
}

// ExtractHookCommand extracts the first command string from a hook's raw JSON data.
func ExtractHookCommand(data json.RawMessage) string {
	var hookEntries []struct {
		Hooks []struct {
			Command string `json:"command"`
		} `json:"hooks"`
	}
	if json.Unmarshal(data, &hookEntries) != nil {
		return ""
	}
	if len(hookEntries) > 0 && len(hookEntries[0].Hooks) > 0 {
		return hookEntries[0].Hooks[0].Command
	}
	return ""
}
