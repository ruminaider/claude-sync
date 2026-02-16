package profiles

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/config"
	"go.yaml.in/yaml/v3"
)

// Profile represents a named profile that layers on top of base config.
type Profile struct {
	Plugins     ProfilePlugins     `yaml:"plugins,omitempty"`
	Settings    map[string]any     `yaml:"settings,omitempty"`
	Hooks       ProfileHooks       `yaml:"hooks,omitempty"`
	Permissions ProfilePermissions `yaml:"permissions,omitempty"`
	ClaudeMD    ProfileClaudeMD    `yaml:"claude_md,omitempty"`
	MCP         ProfileMCP         `yaml:"mcp,omitempty"`
	Keybindings ProfileKeybindings `yaml:"keybindings,omitempty"`
}

// ProfilePlugins holds plugin add/remove directives for a profile.
type ProfilePlugins struct {
	Add    []string `yaml:"add,omitempty"`
	Remove []string `yaml:"remove,omitempty"`
}

// ProfileHooks holds hook add/remove directives for a profile.
type ProfileHooks struct {
	Add    map[string]json.RawMessage `yaml:"add,omitempty"`
	Remove []string                   `yaml:"remove,omitempty"`
}

// ProfilePermissions holds permission add directives for a profile.
type ProfilePermissions struct {
	AddAllow []string `yaml:"add_allow,omitempty"`
	AddDeny  []string `yaml:"add_deny,omitempty"`
}

// ProfileClaudeMD holds CLAUDE.md fragment add/remove directives for a profile.
type ProfileClaudeMD struct {
	Add    []string `yaml:"add,omitempty"`
	Remove []string `yaml:"remove,omitempty"`
}

// ProfileMCP holds MCP server add/remove directives for a profile.
type ProfileMCP struct {
	Add    map[string]json.RawMessage `yaml:"-"`
	Remove []string                   `yaml:"remove,omitempty"`
}

// ProfileKeybindings holds keybinding override directives for a profile.
type ProfileKeybindings struct {
	Override map[string]any `yaml:"override,omitempty"`
}

// ParseProfile parses a profile YAML file into a Profile struct.
// Hook values in hooks.add are stored as JSON strings in YAML (same pattern
// as config.go). If the string starts with '[' and is valid JSON, it's used
// as-is; otherwise config.ExpandHookCommand converts it.
func ParseProfile(data []byte) (Profile, error) {
	// Use yaml.Node for hooks parsing, same pattern as config.Parse.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Profile{}, fmt.Errorf("parsing profile: %w", err)
	}

	// Empty document is a valid empty profile.
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return Profile{}, nil
	}

	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return Profile{}, fmt.Errorf("parsing profile: expected mapping at top level")
	}

	var p Profile

	for i := 0; i < len(root.Content)-1; i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]

		switch keyNode.Value {
		case "plugins":
			var plugins ProfilePlugins
			if err := valNode.Decode(&plugins); err != nil {
				return Profile{}, fmt.Errorf("parsing profile plugins: %w", err)
			}
			p.Plugins = plugins

		case "settings":
			var settings map[string]any
			if err := valNode.Decode(&settings); err != nil {
				return Profile{}, fmt.Errorf("parsing profile settings: %w", err)
			}
			p.Settings = settings

		case "hooks":
			if err := parseProfileHooks(valNode, &p); err != nil {
				return Profile{}, err
			}

		case "permissions":
			var perms ProfilePermissions
			if err := valNode.Decode(&perms); err != nil {
				return Profile{}, fmt.Errorf("parsing profile permissions: %w", err)
			}
			p.Permissions = perms

		case "claude_md":
			var cmd ProfileClaudeMD
			if err := valNode.Decode(&cmd); err != nil {
				return Profile{}, fmt.Errorf("parsing profile claude_md: %w", err)
			}
			p.ClaudeMD = cmd

		case "mcp":
			if err := parseProfileMCP(valNode, &p); err != nil {
				return Profile{}, err
			}

		case "keybindings":
			var kb ProfileKeybindings
			if err := valNode.Decode(&kb); err != nil {
				return Profile{}, fmt.Errorf("parsing profile keybindings: %w", err)
			}
			p.Keybindings = kb
		}
	}

	return p, nil
}

// parseProfileHooks parses the hooks section of a profile YAML.
func parseProfileHooks(node *yaml.Node, p *Profile) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("parsing profile hooks: expected mapping")
	}

	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		switch keyNode.Value {
		case "add":
			var hooksMap map[string]string
			if err := valNode.Decode(&hooksMap); err != nil {
				return fmt.Errorf("parsing profile hooks.add: %w", err)
			}
			p.Hooks.Add = make(map[string]json.RawMessage, len(hooksMap))
			for k, v := range hooksMap {
				if strings.HasPrefix(v, "[") && json.Valid([]byte(v)) {
					p.Hooks.Add[k] = json.RawMessage(v)
				} else {
					p.Hooks.Add[k] = config.ExpandHookCommand(v)
				}
			}
		case "remove":
			var remove []string
			if err := valNode.Decode(&remove); err != nil {
				return fmt.Errorf("parsing profile hooks.remove: %w", err)
			}
			p.Hooks.Remove = remove
		}
	}

	return nil
}

// parseProfileMCP parses the mcp section of a profile YAML.
func parseProfileMCP(node *yaml.Node, p *Profile) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("parsing profile mcp: expected mapping")
	}

	for i := 0; i < len(node.Content)-1; i += 2 {
		keyNode := node.Content[i]
		valNode := node.Content[i+1]

		switch keyNode.Value {
		case "add":
			var mcpRaw map[string]any
			if err := valNode.Decode(&mcpRaw); err != nil {
				return fmt.Errorf("parsing profile mcp.add: %w", err)
			}
			p.MCP.Add = make(map[string]json.RawMessage, len(mcpRaw))
			for k, v := range mcpRaw {
				data, err := json.Marshal(v)
				if err != nil {
					return fmt.Errorf("encoding profile mcp entry %q: %w", k, err)
				}
				p.MCP.Add[k] = json.RawMessage(data)
			}
		case "remove":
			var remove []string
			if err := valNode.Decode(&remove); err != nil {
				return fmt.Errorf("parsing profile mcp.remove: %w", err)
			}
			p.MCP.Remove = remove
		}
	}

	return nil
}

// MarshalProfile serializes a Profile to YAML bytes.
// For hooks.add, json.RawMessage values are stored as their string
// representation in YAML (same pattern as config.go MarshalV2).
func MarshalProfile(p Profile) ([]byte, error) {
	doc := &yaml.Node{Kind: yaml.DocumentNode}
	root := &yaml.Node{Kind: yaml.MappingNode}
	doc.Content = append(doc.Content, root)

	// plugins
	if len(p.Plugins.Add) > 0 || len(p.Plugins.Remove) > 0 {
		var pluginsNode yaml.Node
		if err := pluginsNode.Encode(p.Plugins); err != nil {
			return nil, fmt.Errorf("encoding profile plugins: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "plugins", Tag: "!!str"},
			&pluginsNode,
		)
	}

	// settings
	if len(p.Settings) > 0 {
		var settingsNode yaml.Node
		if err := settingsNode.Encode(p.Settings); err != nil {
			return nil, fmt.Errorf("encoding profile settings: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "settings", Tag: "!!str"},
			&settingsNode,
		)
	}

	// hooks
	if len(p.Hooks.Add) > 0 || len(p.Hooks.Remove) > 0 {
		hooksMap := &yaml.Node{Kind: yaml.MappingNode}

		if len(p.Hooks.Add) > 0 {
			// Store JSON as strings, same pattern as config.go
			hooksStrMap := make(map[string]string, len(p.Hooks.Add))
			for k, v := range p.Hooks.Add {
				hooksStrMap[k] = string(v)
			}
			var addNode yaml.Node
			if err := addNode.Encode(hooksStrMap); err != nil {
				return nil, fmt.Errorf("encoding profile hooks.add: %w", err)
			}
			hooksMap.Content = append(hooksMap.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "add", Tag: "!!str"},
				&addNode,
			)
		}

		if len(p.Hooks.Remove) > 0 {
			var removeNode yaml.Node
			if err := removeNode.Encode(p.Hooks.Remove); err != nil {
				return nil, fmt.Errorf("encoding profile hooks.remove: %w", err)
			}
			hooksMap.Content = append(hooksMap.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "remove", Tag: "!!str"},
				&removeNode,
			)
		}

		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "hooks", Tag: "!!str"},
			hooksMap,
		)
	}

	// permissions
	if len(p.Permissions.AddAllow) > 0 || len(p.Permissions.AddDeny) > 0 {
		var permsNode yaml.Node
		if err := permsNode.Encode(p.Permissions); err != nil {
			return nil, fmt.Errorf("encoding profile permissions: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "permissions", Tag: "!!str"},
			&permsNode,
		)
	}

	// claude_md
	if len(p.ClaudeMD.Add) > 0 || len(p.ClaudeMD.Remove) > 0 {
		var claudeMDNode yaml.Node
		if err := claudeMDNode.Encode(p.ClaudeMD); err != nil {
			return nil, fmt.Errorf("encoding profile claude_md: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "claude_md", Tag: "!!str"},
			&claudeMDNode,
		)
	}

	// mcp
	if len(p.MCP.Add) > 0 || len(p.MCP.Remove) > 0 {
		mcpMap := &yaml.Node{Kind: yaml.MappingNode}

		if len(p.MCP.Add) > 0 {
			addMap := make(map[string]any, len(p.MCP.Add))
			for k, v := range p.MCP.Add {
				var val any
				json.Unmarshal(v, &val)
				addMap[k] = val
			}
			var addNode yaml.Node
			if err := addNode.Encode(addMap); err != nil {
				return nil, fmt.Errorf("encoding profile mcp.add: %w", err)
			}
			mcpMap.Content = append(mcpMap.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "add", Tag: "!!str"},
				&addNode,
			)
		}

		if len(p.MCP.Remove) > 0 {
			var removeNode yaml.Node
			if err := removeNode.Encode(p.MCP.Remove); err != nil {
				return nil, fmt.Errorf("encoding profile mcp.remove: %w", err)
			}
			mcpMap.Content = append(mcpMap.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: "remove", Tag: "!!str"},
				&removeNode,
			)
		}

		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "mcp", Tag: "!!str"},
			mcpMap,
		)
	}

	// keybindings
	if len(p.Keybindings.Override) > 0 {
		var kbNode yaml.Node
		if err := kbNode.Encode(p.Keybindings); err != nil {
			return nil, fmt.Errorf("encoding profile keybindings: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "keybindings", Tag: "!!str"},
			&kbNode,
		)
	}

	return yaml.Marshal(doc)
}

// ListProfiles returns sorted profile names by scanning profiles/*.yaml
// in the syncDir. Returns empty slice (not error) if profiles/ dir doesn't exist.
func ListProfiles(syncDir string) ([]string, error) {
	profilesDir := filepath.Join(syncDir, "profiles")

	entries, err := os.ReadDir(profilesDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("listing profiles: %w", err)
	}

	var names []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasSuffix(name, ".yaml") {
			names = append(names, strings.TrimSuffix(name, ".yaml"))
		}
	}

	sort.Strings(names)
	return names, nil
}

// ReadProfile reads and parses profiles/<name>.yaml from syncDir.
func ReadProfile(syncDir, name string) (Profile, error) {
	path := filepath.Join(syncDir, "profiles", name+".yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		return Profile{}, fmt.Errorf("reading profile %q: %w", name, err)
	}
	return ParseProfile(data)
}

// ReadActiveProfile reads the active-profile file from syncDir.
// Returns "" and nil error if the file doesn't exist.
func ReadActiveProfile(syncDir string) (string, error) {
	path := filepath.Join(syncDir, "active-profile")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return "", nil
		}
		return "", fmt.Errorf("reading active profile: %w", err)
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteActiveProfile writes the profile name to the active-profile file in syncDir.
func WriteActiveProfile(syncDir, name string) error {
	path := filepath.Join(syncDir, "active-profile")
	if err := os.WriteFile(path, []byte(name+"\n"), 0644); err != nil {
		return fmt.Errorf("writing active profile: %w", err)
	}
	return nil
}

// DeleteActiveProfile removes the active-profile file. No error if it doesn't exist.
func DeleteActiveProfile(syncDir string) error {
	path := filepath.Join(syncDir, "active-profile")
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("deleting active profile: %w", err)
	}
	return nil
}

// MergePlugins starts with base, adds profile.Plugins.Add (no duplicates),
// then removes profile.Plugins.Remove. Maintains order (base first, then adds).
func MergePlugins(base []string, profile Profile) []string {
	seen := make(map[string]bool, len(base))
	result := make([]string, 0, len(base)+len(profile.Plugins.Add))

	for _, p := range base {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}

	for _, p := range profile.Plugins.Add {
		if !seen[p] {
			seen[p] = true
			result = append(result, p)
		}
	}

	if len(profile.Plugins.Remove) > 0 {
		removeSet := make(map[string]bool, len(profile.Plugins.Remove))
		for _, p := range profile.Plugins.Remove {
			removeSet[p] = true
		}
		filtered := result[:0]
		for _, p := range result {
			if !removeSet[p] {
				filtered = append(filtered, p)
			}
		}
		result = filtered
	}

	return result
}

// MergeSettings copies base map, then overlays profile.Settings on top.
// If base is nil, returns a copy of profile.Settings.
// If profile.Settings is nil, returns a copy of base.
func MergeSettings(base map[string]any, profile Profile) map[string]any {
	if base == nil && profile.Settings == nil {
		return nil
	}

	result := make(map[string]any)

	for k, v := range base {
		result[k] = v
	}

	for k, v := range profile.Settings {
		result[k] = v
	}

	return result
}

// MergeHooks copies base, adds profile.Hooks.Add entries, then removes
// profile.Hooks.Remove entries.
func MergeHooks(base map[string]json.RawMessage, profile Profile) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage)

	for k, v := range base {
		result[k] = v
	}

	for k, v := range profile.Hooks.Add {
		result[k] = v
	}

	for _, k := range profile.Hooks.Remove {
		delete(result, k)
	}

	return result
}

// MergePermissions copies base, then appends profile permission additions
// without duplicates.
func MergePermissions(base config.Permissions, profile Profile) config.Permissions {
	return config.Permissions{
		Allow: appendUnique(base.Allow, profile.Permissions.AddAllow),
		Deny:  appendUnique(base.Deny, profile.Permissions.AddDeny),
	}
}

// appendUnique appends items from add to base, skipping duplicates.
func appendUnique(base, add []string) []string {
	if len(add) == 0 {
		return base
	}
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

// MergeClaudeMD starts with base includes, adds profile.ClaudeMD.Add (no
// duplicates), then removes profile.ClaudeMD.Remove. Same pattern as
// MergePlugins.
func MergeClaudeMD(base []string, profile Profile) []string {
	seen := make(map[string]bool, len(base))
	result := make([]string, 0, len(base)+len(profile.ClaudeMD.Add))

	for _, s := range base {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	for _, s := range profile.ClaudeMD.Add {
		if !seen[s] {
			seen[s] = true
			result = append(result, s)
		}
	}

	if len(profile.ClaudeMD.Remove) > 0 {
		removeSet := make(map[string]bool, len(profile.ClaudeMD.Remove))
		for _, s := range profile.ClaudeMD.Remove {
			removeSet[s] = true
		}
		filtered := result[:0]
		for _, s := range result {
			if !removeSet[s] {
				filtered = append(filtered, s)
			}
		}
		result = filtered
	}

	return result
}

// MergeMCP copies base, adds profile.MCP.Add entries, then removes
// profile.MCP.Remove entries. Same pattern as MergeHooks.
func MergeMCP(base map[string]json.RawMessage, profile Profile) map[string]json.RawMessage {
	result := make(map[string]json.RawMessage)

	for k, v := range base {
		result[k] = v
	}

	for k, v := range profile.MCP.Add {
		result[k] = v
	}

	for _, k := range profile.MCP.Remove {
		delete(result, k)
	}

	return result
}

// MergeKeybindings copies base, then overlays profile.Keybindings.Override
// on top. Same pattern as MergeSettings.
func MergeKeybindings(base map[string]any, profile Profile) map[string]any {
	if base == nil && len(profile.Keybindings.Override) == 0 {
		return nil
	}

	result := make(map[string]any)

	for k, v := range base {
		result[k] = v
	}

	for k, v := range profile.Keybindings.Override {
		result[k] = v
	}

	return result
}

// ProfileSummary returns a human-readable summary of profile changes.
// Format: "+N plugin(s), -N plugin(s), key -> value, +N hook(s), -N hook(s)"
// Returns "no changes" if the profile has no directives.
func ProfileSummary(p Profile) string {
	var parts []string

	if n := len(p.Plugins.Add); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", n, pluralize("plugin", n)))
	}
	if n := len(p.Plugins.Remove); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d %s", n, pluralize("plugin", n)))
	}

	if len(p.Settings) > 0 {
		// Sort keys for deterministic output.
		keys := make([]string, 0, len(p.Settings))
		for k := range p.Settings {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%s \u2192 %v", k, p.Settings[k]))
		}
	}

	if n := len(p.Hooks.Add); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", n, pluralize("hook", n)))
	}
	if n := len(p.Hooks.Remove); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d %s", n, pluralize("hook", n)))
	}

	if n := len(p.Permissions.AddAllow); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", n, pluralize("allow permission", n)))
	}
	if n := len(p.Permissions.AddDeny); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", n, pluralize("deny permission", n)))
	}

	if n := len(p.ClaudeMD.Add); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", n, pluralize("claude_md include", n)))
	}
	if n := len(p.ClaudeMD.Remove); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d %s", n, pluralize("claude_md include", n)))
	}

	if n := len(p.MCP.Add); n > 0 {
		parts = append(parts, fmt.Sprintf("+%d %s", n, pluralize("mcp server", n)))
	}
	if n := len(p.MCP.Remove); n > 0 {
		parts = append(parts, fmt.Sprintf("-%d %s", n, pluralize("mcp server", n)))
	}

	if n := len(p.Keybindings.Override); n > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", n, pluralize("keybinding override", n)))
	}

	if len(parts) == 0 {
		return "no changes"
	}
	return strings.Join(parts, ", ")
}

// pluralize returns the singular or plural form depending on count.
func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}
