package commands

import (
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/marketplace"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/ruminaider/claude-sync/internal/project"
)

// DefaultProjectSearchDirs returns the parent directories to scan for managed projects.
// It is a var so tests can override it.
var DefaultProjectSearchDirs = func() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	var dirs []string
	for _, name := range []string{"Work", "Projects", "Repositories", "repos", "src", "code", "dev"} {
		dir := filepath.Join(home, name)
		if info, err := os.Stat(dir); err == nil && info.IsDir() {
			dirs = append(dirs, dir)
		}
	}
	return dirs
}

// PluginInfo holds display information about a single plugin.
type PluginInfo struct {
	Key           string
	Name          string
	Status        string // "upstream", "pinned", "forked"
	PinVersion    string
	Marketplace   string
	LatestVersion string // empty if unknown or up to date
}

// ProjectInfo holds display information about a managed project.
type ProjectInfo struct {
	Path    string
	Profile string
}

// MenuState holds the detected state used to build the TUI menu.
type MenuState struct {
	// Existing fields
	ConfigExists  bool
	HasPending    bool
	HasConflicts  bool
	Profiles      []string
	ActiveProfile string

	// Dashboard fields
	ConfigRepo     string       // remote URL or repo shortname
	CommitsBehind  int          // how many commits behind remote
	Plugins        []PluginInfo // all plugins with status
	Projects       []ProjectInfo
	ProjectDir         string // current $PWD (always set)
	ProjectInitialized bool   // true if project has .claude-sync.yaml
	ProjectProfile     string // profile assigned to current project
	ClaudeMDCount  int    // number of synced CLAUDE.md sections
	MCPCount       int    // number of MCP servers configured
}

// DetectMenuState checks the current claude-sync state for menu rendering.
// It is designed to be fast and never error — unknown state defaults to false/empty.
func DetectMenuState(claudeDir, syncDir string) MenuState {
	pwd, _ := os.Getwd()
	return detectMenuStateWithPwd(claudeDir, syncDir, pwd)
}

// detectMenuStateWithPwd is the internal implementation that accepts an explicit working directory.
func detectMenuStateWithPwd(claudeDir, syncDir, pwd string) MenuState {
	var state MenuState

	// Check if sync dir exists (i.e., claude-sync is initialized)
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return state
	}
	state.ConfigExists = true

	// Check for pending approval changes
	pending, err := approval.ReadPending(syncDir)
	if err == nil && !pending.IsEmpty() {
		state.HasPending = true
	}

	// Check for merge conflicts
	state.HasConflicts = HasPendingConflicts(syncDir)

	// Check for profiles
	profileList, err := profiles.ListProfiles(syncDir)
	if err == nil {
		state.Profiles = profileList
	}

	// Check active profile
	active, err := profiles.ReadActiveProfile(syncDir)
	if err == nil {
		state.ActiveProfile = active
	}

	// Parse config.yaml for plugins, MCP count
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err == nil {
		cfg, parseErr := config.Parse(cfgData)
		if parseErr == nil {
			state.Plugins = buildPluginInfos(cfg)
			state.MCPCount = len(cfg.MCP)
		}
	}

	// Config repo: read git remote URL
	state.ConfigRepo = detectConfigRepo(syncDir)

	// Commits behind: check local tracking info
	state.CommitsBehind = detectCommitsBehind(syncDir)

	// Projects: scan default search dirs
	if searchDirs := DefaultProjectSearchDirs(); len(searchDirs) > 0 {
		entries, scanErr := ProjectListScan(searchDirs)
		if scanErr == nil {
			for _, e := range entries {
				state.Projects = append(state.Projects, ProjectInfo{
					Path:    e.Path,
					Profile: e.Profile,
				})
			}
		}
	}

	// Project dir: always set to pwd, check if initialized
	if pwd != "" {
		state.ProjectDir = pwd
		pcfg, readErr := project.ReadProjectConfig(pwd)
		if readErr == nil && !pcfg.Declined {
			state.ProjectInitialized = true
			state.ProjectProfile = pcfg.Profile
		}
	}

	// ClaudeMD count: count .md files in claude-md/ directory
	state.ClaudeMDCount = countClaudeMDFragments(syncDir)

	return state
}

// buildPluginInfos extracts PluginInfo entries from a parsed config.
func buildPluginInfos(cfg config.Config) []PluginInfo {
	var plugins []PluginInfo

	// Upstream plugins
	for _, key := range cfg.Upstream {
		name, mkt := splitPluginKey(key)
		plugins = append(plugins, PluginInfo{
			Key:         key,
			Name:        name,
			Status:      "upstream",
			Marketplace: mkt,
		})
	}

	// Pinned plugins
	for key, version := range cfg.Pinned {
		name, mkt := splitPluginKey(key)
		plugins = append(plugins, PluginInfo{
			Key:         key,
			Name:        name,
			Status:      "pinned",
			PinVersion:  version,
			Marketplace: mkt,
		})
	}

	// Forked plugins
	for _, name := range cfg.Forked {
		key := name + "@" + config.ForkedMarketplace
		plugins = append(plugins, PluginInfo{
			Key:         key,
			Name:        name,
			Status:      "forked",
			Marketplace: config.ForkedMarketplace,
		})
	}

	return plugins
}

// splitPluginKey splits "name@marketplace" into name and marketplace parts.
func splitPluginKey(key string) (name, mkt string) {
	parts := strings.SplitN(key, "@", 2)
	name = parts[0]
	if len(parts) > 1 {
		mkt = parts[1]
	}
	return
}

// detectConfigRepo reads the git remote URL from syncDir and returns a short form.
func detectConfigRepo(syncDir string) string {
	cmd := exec.Command("git", "-C", syncDir, "remote", "get-url", "origin")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	url := strings.TrimSpace(string(out))
	// Try to parse as GitHub URL for short form
	if short := marketplace.ParseGitHubRepoURL(url); short != "" {
		return short
	}
	// Return the raw URL if not a GitHub URL
	return url
}

// detectCommitsBehind checks how many commits the local branch is behind its upstream.
func detectCommitsBehind(syncDir string) int {
	cmd := exec.Command("git", "-C", syncDir, "rev-list", "HEAD..@{upstream}", "--count")
	out, err := cmd.Output()
	if err != nil {
		return 0
	}
	n, err := strconv.Atoi(strings.TrimSpace(string(out)))
	if err != nil {
		return 0
	}
	return n
}

// countClaudeMDFragments counts .md files in the claude-md/ directory.
func countClaudeMDFragments(syncDir string) int {
	claudeMDDir := filepath.Join(syncDir, "claude-md")
	entries, err := os.ReadDir(claudeMDDir)
	if err != nil {
		return 0
	}
	count := 0
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") {
			count++
		}
	}
	return count
}
