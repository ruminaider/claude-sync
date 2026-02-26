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
	csync "github.com/ruminaider/claude-sync/internal/sync"
)

type PushScanResult struct {
	AddedPlugins       []string
	RemovedPlugins     []string
	ChangedSettings    map[string]csync.SettingChange
	ChangedPermissions bool
	ChangedClaudeMD    *claudemd.ReconcileResult
	ChangedMCP         bool
	MCPSecrets         []DetectedSecret // secrets detected in MCP configs (will be auto-replaced)
	ChangedKeybindings bool
	ChangedCommands    bool
	ChangedSkills      bool
	OrphanedCommands   []string // command files in sync dir but not in config
	OrphanedSkills     []string // skill dirs in sync dir but not in config
	DirtyWorkingTree   bool     // sync repo has uncommitted changes (e.g. from config update)
}

func (r *PushScanResult) HasChanges() bool {
	return len(r.AddedPlugins) > 0 || len(r.RemovedPlugins) > 0 ||
		len(r.ChangedSettings) > 0 ||
		r.ChangedPermissions || r.ChangedClaudeMD != nil ||
		r.ChangedMCP || r.ChangedKeybindings ||
		r.ChangedCommands || r.ChangedSkills ||
		len(r.OrphanedCommands) > 0 || len(r.OrphanedSkills) > 0 ||
		r.DirtyWorkingTree
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

	result := &PushScanResult{
		AddedPlugins:   filtered,
		RemovedPlugins: diff.ToInstall,
	}

	// Scan permissions.
	settingsRaw, settErr := claudecode.ReadSettings(claudeDir)
	if settErr == nil {
		var currentPerms struct {
			Allow []string `json:"allow"`
			Deny  []string `json:"deny"`
		}
		if permRaw, ok := settingsRaw["permissions"]; ok {
			json.Unmarshal(permRaw, &currentPerms)
		}
		if !stringSlicesEqual(currentPerms.Allow, cfg.Permissions.Allow) ||
			!stringSlicesEqual(currentPerms.Deny, cfg.Permissions.Deny) {
			result.ChangedPermissions = true
		}
	}

	// Scan CLAUDE.md.
	claudeMDPath := filepath.Join(claudeDir, "CLAUDE.md")
	if claudeMDData, err := os.ReadFile(claudeMDPath); err == nil {
		reconcileResult, err := claudemd.Reconcile(syncDir, string(claudeMDData))
		if err == nil && (len(reconcileResult.Updated) > 0 || len(reconcileResult.New) > 0 ||
			len(reconcileResult.Deleted) > 0 || len(reconcileResult.Renamed) > 0) {
			result.ChangedClaudeMD = reconcileResult
		}
	}

	// Scan MCP.
	currentMCP, mcpErr := claudecode.ReadMCPConfig(claudeDir)
	if mcpErr == nil {
		if !jsonMapsEqual(currentMCP, cfg.MCP) {
			result.ChangedMCP = true
			result.MCPSecrets = DetectMCPSecrets(currentMCP)
		}
	}

	// Scan keybindings.
	currentKB, kbErr := claudecode.ReadKeybindings(claudeDir)
	if kbErr == nil {
		if !anyMapsEqual(currentKB, cfg.Keybindings) {
			result.ChangedKeybindings = true
		}
	}

	// Scan commands — compare items listed in config, or check for uncommitted changes.
	cmdNames := extractNamesFromKeys(cfg.Commands)
	if len(cmdNames) > 0 {
		result.ChangedCommands = filteredDirContentsDiffer(
			filepath.Join(claudeDir, "commands"),
			filepath.Join(syncDir, "commands"),
			cmdNames,
		)
	}
	if !result.ChangedCommands && git.HasUncommittedChanges(syncDir, "commands") {
		result.ChangedCommands = true
	}

	// Scan skills — compare items listed in config, or check for uncommitted changes.
	skillNames := extractNamesFromKeys(cfg.Skills)
	if len(skillNames) > 0 {
		result.ChangedSkills = filteredDirContentsDiffer(
			filepath.Join(claudeDir, "skills"),
			filepath.Join(syncDir, "skills"),
			skillNames,
		)
	}
	if !result.ChangedSkills && git.HasUncommittedChanges(syncDir, "skills") {
		result.ChangedSkills = true
	}

	// Detect orphaned commands/skills in sync dir that aren't in config.
	result.OrphanedCommands = findOrphanedFiles(filepath.Join(syncDir, "commands"), cmdNames)
	result.OrphanedSkills = findOrphanedDirs(filepath.Join(syncDir, "skills"), skillNames)

	// Check for uncommitted changes in the sync repo (e.g. from config update).
	if clean, err := git.IsClean(syncDir); err == nil && !clean {
		result.DirtyWorkingTree = true
	}

	return result, nil
}

// PushApplyOptions configures what PushApply does.
type PushApplyOptions struct {
	ClaudeDir          string
	SyncDir            string
	AddPlugins         []string
	RemovePlugins      []string
	ExcludePlugins     []string // plugins to add to cfg.Excluded
	ProfileTarget      string   // "" = base config, non-empty = profile name
	Message            string
	UpdatePermissions  bool
	UpdateClaudeMD     bool
	UpdateMCP          bool
	UpdateKeybindings  bool
	UpdateCommands     bool
	UpdateSkills       bool
	OrphanedCommands   []string // command files to remove from sync dir
	OrphanedSkills     []string // skill dirs to remove from sync dir
	DirtyWorkingTree   bool     // sync repo has uncommitted changes to include
	Force              bool     // force push (--force-with-lease)
}

func PushApply(opts PushApplyOptions) error {
	if HasPendingConflicts(opts.SyncDir) {
		return fmt.Errorf("pending conflicts must be resolved before pushing — run 'claude-sync conflicts' to review")
	}

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
		// Route changes to a profile instead of base config.
		profile, err := profiles.ReadProfile(opts.SyncDir, opts.ProfileTarget)
		if err != nil {
			profile = profiles.Profile{}
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

		// Route settings to profile overlay.
		if opts.UpdatePermissions {
			settingsRaw, err := claudecode.ReadSettings(opts.ClaudeDir)
			if err == nil {
				var perms struct {
					Allow []string `json:"allow"`
					Deny  []string `json:"deny"`
				}
				if permRaw, ok := settingsRaw["permissions"]; ok {
					json.Unmarshal(permRaw, &perms)
				}
				// Compute what's new vs base permissions.
				baseAllowSet := make(map[string]bool, len(cfg.Permissions.Allow))
				for _, a := range cfg.Permissions.Allow {
					baseAllowSet[a] = true
				}
				baseDenySet := make(map[string]bool, len(cfg.Permissions.Deny))
				for _, d := range cfg.Permissions.Deny {
					baseDenySet[d] = true
				}
				var newAllow, newDeny []string
				for _, a := range perms.Allow {
					if !baseAllowSet[a] {
						newAllow = append(newAllow, a)
					}
				}
				for _, d := range perms.Deny {
					if !baseDenySet[d] {
						newDeny = append(newDeny, d)
					}
				}
				if len(newAllow) > 0 {
					profile.Permissions.AddAllow = newAllow
				}
				if len(newDeny) > 0 {
					profile.Permissions.AddDeny = newDeny
				}
			}
			opts.UpdatePermissions = false // prevent base-config write below
		}

		// Route MCP to profile overlay.
		if opts.UpdateMCP {
			mcp, err := claudecode.ReadMCPConfig(opts.ClaudeDir)
			if err == nil {
				mcpAdd := make(map[string]json.RawMessage)
				for name, val := range mcp {
					baseVal, inBase := cfg.MCP[name]
					if !inBase || string(baseVal) != string(val) {
						mcpAdd[name] = val
					}
				}
				if len(mcpAdd) > 0 {
					// Strip secrets and normalize paths before writing to profile.
					if secrets := DetectMCPSecrets(mcpAdd); len(secrets) > 0 {
						mcpAdd = ReplaceSecrets(mcpAdd, secrets)
					}
					mcpAdd = NormalizeMCPPaths(mcpAdd)
					if profile.MCP.Add == nil {
						profile.MCP.Add = make(map[string]json.RawMessage)
					}
					for k, v := range mcpAdd {
						profile.MCP.Add[k] = v
					}
				}
			}
			opts.UpdateMCP = false // prevent base-config write below
		}

		// Route keybindings to profile overlay.
		if opts.UpdateKeybindings {
			kb, err := claudecode.ReadKeybindings(opts.ClaudeDir)
			if err == nil {
				kbOverride := make(map[string]any)
				for k, v := range kb {
					baseVal, inBase := cfg.Keybindings[k]
					if !inBase {
						kbOverride[k] = v
					} else {
						baseJSON, _ := json.Marshal(baseVal)
						curJSON, _ := json.Marshal(v)
						if string(baseJSON) != string(curJSON) {
							kbOverride[k] = v
						}
					}
				}
				if len(kbOverride) > 0 {
					profile.Keybindings.Override = kbOverride
				}
			}
			opts.UpdateKeybindings = false // prevent base-config write below
		}

		profileData, err := profiles.MarshalProfile(profile)
		if err != nil {
			return fmt.Errorf("marshaling profile %q: %w", opts.ProfileTarget, err)
		}
		profileDir := filepath.Join(opts.SyncDir, "profiles")
		os.MkdirAll(profileDir, 0755)
		profilePath := filepath.Join(profileDir, opts.ProfileTarget+".yaml")
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

	// Update permissions from current state.
	if opts.UpdatePermissions {
		settingsRaw, err := claudecode.ReadSettings(opts.ClaudeDir)
		if err == nil {
			var perms struct {
				Allow []string `json:"allow"`
				Deny  []string `json:"deny"`
			}
			if permRaw, ok := settingsRaw["permissions"]; ok {
				json.Unmarshal(permRaw, &perms)
			}
			cfg.Permissions = config.Permissions{
				Allow: perms.Allow,
				Deny:  perms.Deny,
			}
		}
	}

	// Update MCP from current state.
	if opts.UpdateMCP {
		mcp, err := claudecode.ReadMCPConfig(opts.ClaudeDir)
		if err == nil {
			// Strip secrets and normalize paths before writing to config.
			if secrets := DetectMCPSecrets(mcp); len(secrets) > 0 {
				mcp = ReplaceSecrets(mcp, secrets)
			}
			mcp = NormalizeMCPPaths(mcp)
			cfg.MCP = mcp
		}
	}

	// Update keybindings from current state.
	if opts.UpdateKeybindings {
		kb, err := claudecode.ReadKeybindings(opts.ClaudeDir)
		if err == nil {
			cfg.Keybindings = kb
		}
	}

	// Update commands from current state (only items listed in config).
	if opts.UpdateCommands {
		syncCopyCommands(opts.ClaudeDir, opts.SyncDir, extractNamesFromKeys(cfg.Commands))
	}

	// Update skills from current state (only items listed in config).
	if opts.UpdateSkills {
		syncCopySkills(opts.ClaudeDir, opts.SyncDir, extractNamesFromKeys(cfg.Skills))
	}

	// Remove orphaned commands/skills from sync dir.
	for _, name := range opts.OrphanedCommands {
		os.Remove(filepath.Join(opts.SyncDir, "commands", name+".md"))
	}
	for _, name := range opts.OrphanedSkills {
		os.RemoveAll(filepath.Join(opts.SyncDir, "skills", name))
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
	if opts.UpdateClaudeMD {
		claudeMdDir := filepath.Join(opts.SyncDir, "claude-md")
		if _, err := os.Stat(claudeMdDir); err == nil {
			if err := git.Add(opts.SyncDir, "claude-md"); err != nil {
				return fmt.Errorf("staging claude-md: %w", err)
			}
		}
	}
	if opts.UpdateCommands {
		cmdsDir := filepath.Join(opts.SyncDir, "commands")
		if _, err := os.Stat(cmdsDir); err == nil {
			if err := git.Add(opts.SyncDir, "commands"); err != nil {
				return fmt.Errorf("staging commands: %w", err)
			}
		}
	}
	if opts.UpdateSkills {
		skillsDir := filepath.Join(opts.SyncDir, "skills")
		if _, err := os.Stat(skillsDir); err == nil {
			if err := git.Add(opts.SyncDir, "skills"); err != nil {
				return fmt.Errorf("staging skills: %w", err)
			}
		}
	}
	// Stage orphan removals individually (the directories are already deleted).
	for _, name := range opts.OrphanedCommands {
		git.Add(opts.SyncDir, filepath.Join("commands", name+".md"))
	}
	for _, name := range opts.OrphanedSkills {
		git.Add(opts.SyncDir, filepath.Join("skills", name))
	}
	// Catch-all: stage any remaining uncommitted changes (e.g. from config update).
	if opts.DirtyWorkingTree {
		git.Add(opts.SyncDir, "-A")
	}
	if err := git.Commit(opts.SyncDir, message); err != nil {
		return fmt.Errorf("committing: %w", err)
	}
	if git.HasRemote(opts.SyncDir, "origin") {
		if !git.HasUpstream(opts.SyncDir) {
			branch, err := git.CurrentBranch(opts.SyncDir)
			if err != nil {
				return fmt.Errorf("detecting branch: %w", err)
			}
			pushFn := git.PushWithUpstream
			if opts.Force {
				pushFn = git.ForcePushWithUpstream
			}
			if err := pushFn(opts.SyncDir, "origin", branch); err != nil {
				return fmt.Errorf("pushing: %w", err)
			}
		} else {
			pushFn := git.Push
			if opts.Force {
				pushFn = git.ForcePush
			}
			if err := pushFn(opts.SyncDir); err != nil {
				return fmt.Errorf("pushing: %w", err)
			}
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

// stringSlicesEqual compares two string slices regardless of order.
func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	aSet := make(map[string]int, len(a))
	for _, s := range a {
		aSet[s]++
	}
	for _, s := range b {
		aSet[s]--
	}
	for _, count := range aSet {
		if count != 0 {
			return false
		}
	}
	return true
}

// findOrphanedFiles returns .md file names in dir that aren't in the allowed set.
func findOrphanedFiles(dir string, allowed map[string]bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var orphans []string
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := strings.TrimSuffix(entry.Name(), ".md")
		if !allowed[name] {
			orphans = append(orphans, name)
		}
	}
	return orphans
}

// findOrphanedDirs returns subdirectory names in dir that aren't in the allowed set.
func findOrphanedDirs(dir string, allowed map[string]bool) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var orphans []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		if !allowed[entry.Name()] {
			orphans = append(orphans, entry.Name())
		}
	}
	return orphans
}

// jsonMapsEqual compares two json.RawMessage maps by key set and value bytes.
func jsonMapsEqual(a, b map[string]json.RawMessage) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		vb, ok := b[k]
		if !ok {
			return false
		}
		if string(va) != string(vb) {
			return false
		}
	}
	return true
}

// readDirMDFiles reads all .md files in a directory into a map of name→content.
// For skill directories, reads SKILL.md from each subdirectory.
func readDirMDFiles(dir string) map[string]string {
	files := make(map[string]string)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return files
	}
	for _, entry := range entries {
		if entry.IsDir() {
			// Skill directory: read SKILL.md.
			skillFile := filepath.Join(dir, entry.Name(), "SKILL.md")
			if data, err := os.ReadFile(skillFile); err == nil {
				files[entry.Name()+"/SKILL.md"] = string(data)
			}
		} else if strings.HasSuffix(entry.Name(), ".md") {
			path := filepath.Join(dir, entry.Name())
			if data, err := os.ReadFile(path); err == nil {
				files[entry.Name()] = string(data)
			}
		}
	}
	return files
}

// extractNamesFromKeys parses the trailing name from key strings like
// "cmd:global:review-plan" or "skill:global:termdock-ast".
func extractNamesFromKeys(keys []string) map[string]bool {
	names := make(map[string]bool)
	for _, k := range keys {
		parts := strings.Split(k, ":")
		if len(parts) >= 3 {
			names[parts[len(parts)-1]] = true
		}
	}
	return names
}

// filteredDirContentsDiffer compares only the named items between two directories.
func filteredDirContentsDiffer(a, b string, names map[string]bool) bool {
	aFiles := readDirMDFiles(a)
	bFiles := readDirMDFiles(b)

	for name := range names {
		// For commands: name.md; for skills: name/SKILL.md.
		cmdKey := name + ".md"
		skillKey := name + "/SKILL.md"

		aContent, aOK := aFiles[cmdKey]
		bContent, bOK := bFiles[cmdKey]
		if !aOK {
			aContent, aOK = aFiles[skillKey]
			bContent, bOK = bFiles[skillKey]
		}
		if aOK != bOK {
			return true
		}
		if aOK && aContent != bContent {
			return true
		}
	}
	return false
}

// syncCopyCommands copies only the named .md files from claudeDir/commands/ to syncDir/commands/.
func syncCopyCommands(claudeDir, syncDir string, names map[string]bool) {
	if len(names) == 0 {
		return
	}
	srcDir := filepath.Join(claudeDir, "commands")
	dstDir := filepath.Join(syncDir, "commands")

	os.MkdirAll(dstDir, 0755)
	for name := range names {
		srcPath := filepath.Join(srcDir, name+".md")
		dstPath := filepath.Join(dstDir, name+".md")
		if data, err := os.ReadFile(srcPath); err == nil {
			os.WriteFile(dstPath, data, 0644)
		}
	}
}

// syncCopySkills copies only the named skill directories from claudeDir/skills/ to syncDir/skills/.
func syncCopySkills(claudeDir, syncDir string, names map[string]bool) {
	if len(names) == 0 {
		return
	}
	srcDir := filepath.Join(claudeDir, "skills")
	dstDir := filepath.Join(syncDir, "skills")

	for name := range names {
		srcSkillDir := filepath.Join(srcDir, name)
		dstSkillDir := filepath.Join(dstDir, name)
		if _, err := os.Stat(srcSkillDir); err == nil {
			copyDir(srcSkillDir, dstSkillDir)
		}
	}
}

// anyMapsEqual compares two map[string]any maps by JSON serialization.
func anyMapsEqual(a, b map[string]any) bool {
	if len(a) != len(b) {
		return false
	}
	if len(a) == 0 {
		return true
	}
	aJSON, _ := json.Marshal(a)
	bJSON, _ := json.Marshal(b)
	return string(aJSON) == string(bJSON)
}
