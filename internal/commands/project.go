package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/ruminaider/claude-sync/internal/project"
)

// ProjectInitOptions configures project initialization.
type ProjectInitOptions struct {
	ProjectDir    string
	SyncDir       string
	Profile       string
	ProjectedKeys []string
	Yes           bool // non-interactive mode
}

// ProjectInitResult describes what happened during project init.
type ProjectInitResult struct {
	Created             bool
	Profile             string
	ProjectedKeys       []string
	ImportedPermissions int
	ImportedHooks       int
}

// ProjectInit initializes a project's settings.local.json from a claude-sync profile.
func ProjectInit(opts ProjectInitOptions) (*ProjectInitResult, error) {
	// 1. Verify global config exists
	cfgPath := filepath.Join(opts.SyncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, fmt.Errorf("claude-sync not initialized — run 'claude-sync config create' first")
	}
	cfg, err := config.Parse(cfgData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// 2. Resolve base config + profile
	resolved := ResolveWithProfile(cfg, opts.SyncDir, opts.Profile)

	// 3. Import existing settings.local.json
	var overrides project.ProjectOverrides
	var importedPerms, importedHooks int
	settingsPath := filepath.Join(opts.ProjectDir, ".claude", "settings.local.json")
	if data, readErr := os.ReadFile(settingsPath); readErr == nil {
		var existing map[string]json.RawMessage
		if json.Unmarshal(data, &existing) == nil {
			importedPerms = importPermissionOverrides(&overrides, existing, resolved.Permissions)
			importedHooks = importHookOverrides(&overrides, existing, resolved.Hooks)
		}
	}

	// 4. Write .claude-sync.yaml
	pcfg := project.ProjectConfig{
		Version:       "1.0.0",
		Profile:       opts.Profile,
		Initialized:   time.Now().UTC().Format(time.RFC3339),
		ProjectedKeys: opts.ProjectedKeys,
		Overrides:     overrides,
	}
	if err := project.WriteProjectConfig(opts.ProjectDir, pcfg); err != nil {
		return nil, fmt.Errorf("failed to write project config: %w", err)
	}

	// 5. Apply projected keys to settings.local.json
	if err := ApplyProjectSettings(opts.ProjectDir, resolved, pcfg); err != nil {
		return nil, fmt.Errorf("failed to apply project settings: %w", err)
	}

	// 6. Ensure .claude-sync.yaml is in .gitignore
	ensureGitignore(opts.ProjectDir, ".claude/"+project.ConfigFileName)

	return &ProjectInitResult{
		Created:             true,
		Profile:             opts.Profile,
		ProjectedKeys:       opts.ProjectedKeys,
		ImportedPermissions: importedPerms,
		ImportedHooks:       importedHooks,
	}, nil
}

// ResolvedConfig holds the fully merged base + profile result.
// Exported so pull.go and push.go can reuse it.
type ResolvedConfig struct {
	Hooks       map[string]json.RawMessage
	Permissions config.Permissions
	Settings    map[string]any
	ClaudeMD    []string
	MCP         map[string]json.RawMessage
}

// ResolveWithProfile merges base config with the specified profile (or active profile if empty).
func ResolveWithProfile(cfg config.Config, syncDir, profileName string) ResolvedConfig {
	rc := ResolvedConfig{
		Hooks:       copyHooks(cfg.Hooks),
		Permissions: cfg.Permissions,
		Settings:    cfg.Settings,
		ClaudeMD:    cfg.ClaudeMD.Include,
		MCP:         copyHooks(cfg.MCP), // same type: map[string]json.RawMessage
	}

	if profileName == "" {
		profileName, _ = profiles.ReadActiveProfile(syncDir)
	}

	if profileName != "" {
		if p, err := profiles.ReadProfile(syncDir, profileName); err == nil {
			rc.Hooks = profiles.MergeHooks(rc.Hooks, p)
			rc.Permissions = profiles.MergePermissions(rc.Permissions, p)
			rc.Settings = profiles.MergeSettings(rc.Settings, p)
			rc.ClaudeMD = profiles.MergeClaudeMD(rc.ClaudeMD, p)
			rc.MCP = profiles.MergeMCP(rc.MCP, p)
		}
	}

	return rc
}

// copyHooks makes a shallow copy of a map[string]json.RawMessage.
func copyHooks(m map[string]json.RawMessage) map[string]json.RawMessage {
	if m == nil {
		return make(map[string]json.RawMessage)
	}
	result := make(map[string]json.RawMessage, len(m))
	for k, v := range m {
		result[k] = v
	}
	return result
}

func importPermissionOverrides(overrides *project.ProjectOverrides, existing map[string]json.RawMessage, basePerms config.Permissions) int {
	permRaw, ok := existing["permissions"]
	if !ok {
		return 0
	}
	var perms struct {
		Allow []string `json:"allow"`
	}
	if json.Unmarshal(permRaw, &perms) != nil {
		return 0
	}

	baseSet := make(map[string]bool, len(basePerms.Allow))
	for _, p := range basePerms.Allow {
		baseSet[p] = true
	}

	var added int
	for _, p := range perms.Allow {
		if !baseSet[p] {
			overrides.Permissions.AddAllow = append(overrides.Permissions.AddAllow, p)
			added++
		}
	}
	return added
}

func importHookOverrides(overrides *project.ProjectOverrides, existing map[string]json.RawMessage, baseHooks map[string]json.RawMessage) int {
	hooksRaw, ok := existing["hooks"]
	if !ok {
		return 0
	}
	var hooks map[string]json.RawMessage
	if json.Unmarshal(hooksRaw, &hooks) != nil {
		return 0
	}

	var added int
	for name, raw := range hooks {
		if _, inBase := baseHooks[name]; !inBase {
			if overrides.Hooks.Add == nil {
				overrides.Hooks.Add = make(map[string]json.RawMessage)
			}
			overrides.Hooks.Add[name] = raw
			added++
		}
	}
	return added
}

// ApplyProjectSettings writes managed keys to settings.local.json.
// Unmanaged keys are preserved.
func ApplyProjectSettings(projectDir string, resolved ResolvedConfig, pcfg project.ProjectConfig) error {
	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")

	// Read existing settings (preserve unmanaged keys)
	var settings map[string]json.RawMessage
	if data, err := os.ReadFile(settingsPath); err == nil {
		json.Unmarshal(data, &settings)
	}
	if settings == nil {
		settings = make(map[string]json.RawMessage)
	}

	// Apply project overrides on top of resolved config
	finalPerms := config.Permissions{
		Allow: append(append([]string{}, resolved.Permissions.Allow...), pcfg.Overrides.Permissions.AddAllow...),
		Deny:  append(append([]string{}, resolved.Permissions.Deny...), pcfg.Overrides.Permissions.AddDeny...),
	}

	finalHooks := copyHooks(resolved.Hooks)
	for name, raw := range pcfg.Overrides.Hooks.Add {
		finalHooks[name] = raw
	}
	for _, name := range pcfg.Overrides.Hooks.Remove {
		delete(finalHooks, name)
	}

	// Write projected keys
	for _, key := range pcfg.ProjectedKeys {
		switch key {
		case "hooks":
			data, _ := json.Marshal(finalHooks)
			settings["hooks"] = data
		case "permissions":
			p := map[string]any{"allow": finalPerms.Allow}
			if len(finalPerms.Deny) > 0 {
				p["deny"] = finalPerms.Deny
			}
			data, _ := json.Marshal(p)
			settings["permissions"] = data
		}
	}

	// Ensure .claude dir exists
	if err := os.MkdirAll(filepath.Join(projectDir, ".claude"), 0755); err != nil {
		return err
	}

	// Write back
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(settingsPath, append(data, '\n'), 0644)
}

func ensureGitignore(projectDir, entry string) {
	gitignorePath := filepath.Join(projectDir, ".gitignore")
	content, _ := os.ReadFile(gitignorePath)
	if containsLine(string(content), entry) {
		return
	}
	f, err := os.OpenFile(gitignorePath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return
	}
	defer f.Close()
	if len(content) > 0 && content[len(content)-1] != '\n' {
		f.WriteString("\n")
	}
	f.WriteString(entry + "\n")
}

func containsLine(s, target string) bool {
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) == target {
			return true
		}
	}
	return false
}
