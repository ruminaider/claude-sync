package config

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// Permissions holds tool-permission allow/deny lists.
type Permissions struct {
	Allow []string `yaml:"allow,omitempty"`
	Deny  []string `yaml:"deny,omitempty"`
}

// ClaudeMDConfig holds CLAUDE.md fragment include paths.
type ClaudeMDConfig struct {
	Include []string `yaml:"include,omitempty"`
}

// MCPServerMeta stores metadata about an imported MCP server.
type MCPServerMeta struct {
	SourceProject string `yaml:"source_project,omitempty"`
}

// MarketplaceSource declares how a custom marketplace should be resolved.
// Well-known marketplaces (claude-plugins-official, etc.) don't need entries.
type MarketplaceSource struct {
	Source string `yaml:"source"` // "github" or "git"
	Repo   string `yaml:"repo,omitempty"`   // "org/repo" for github
	URL    string `yaml:"url,omitempty"`    // full URL for git
}

// ConfigV2 represents ~/.claude-sync/config.yaml with categorized plugins.
type ConfigV2 struct {
	Version     string                     `yaml:"version"`
	Upstream    []string                   `yaml:"-"` // parsed from plugins.upstream (or flat list in v1)
	Pinned      map[string]string          `yaml:"-"` // parsed from plugins.pinned (key -> version)
	Forked      []string                   `yaml:"-"` // parsed from plugins.forked
	Excluded    []string                   `yaml:"-"` // plugins user excluded during init
	Settings    map[string]any             `yaml:"settings,omitempty"`
	Hooks       map[string]json.RawMessage `yaml:"-"`
	Permissions Permissions                `yaml:"-"`
	ClaudeMD    ClaudeMDConfig             `yaml:"-"`
	MCP         map[string]json.RawMessage `yaml:"-"`
	MCPMeta     map[string]MCPServerMeta   `yaml:"-"`
	Keybindings  map[string]any              `yaml:"-"`
	Commands     []string                   `yaml:"-"`
	Skills       []string                   `yaml:"-"`
	Marketplaces map[string]MarketplaceSource `yaml:"-"`
}

// Config is a type alias for ConfigV2 to maintain backward compatibility.
type Config = ConfigV2

// AllPluginKeys returns all plugin keys across all categories (upstream, pinned, forked).
// The result is sorted for deterministic output.
func (c *ConfigV2) AllPluginKeys() []string {
	seen := make(map[string]bool)
	var keys []string

	for _, k := range c.Upstream {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for k := range c.Pinned {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}
	for _, k := range c.Forked {
		if !seen[k] {
			seen[k] = true
			keys = append(keys, k)
		}
	}

	sort.Strings(keys)
	return keys
}

// Parse parses config.yaml bytes into a Config, auto-detecting v1 (flat list) vs v2 (categorized).
func Parse(data []byte) (Config, error) {
	// First, parse the raw document to inspect the plugins node type.
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}

	// Empty document
	if doc.Kind == 0 || len(doc.Content) == 0 {
		return Config{}, fmt.Errorf("parsing config: empty document")
	}

	// doc is a DocumentNode; its first child is the top-level mapping.
	root := doc.Content[0]
	if root.Kind != yaml.MappingNode {
		return Config{}, fmt.Errorf("parsing config: expected mapping at top level")
	}

	var cfg Config
	cfg.Pinned = make(map[string]string)

	// Parse the mapping manually to handle plugins specially.
	for i := 0; i < len(root.Content)-1; i += 2 {
		keyNode := root.Content[i]
		valNode := root.Content[i+1]

		switch keyNode.Value {
		case "version":
			cfg.Version = valNode.Value
		case "plugins":
			if err := parsePluginsNode(valNode, &cfg); err != nil {
				return Config{}, fmt.Errorf("parsing config plugins: %w", err)
			}
		case "settings":
			var settings map[string]any
			if err := valNode.Decode(&settings); err != nil {
				return Config{}, fmt.Errorf("parsing config settings: %w", err)
			}
			cfg.Settings = settings
		case "hooks":
			var hooks map[string]string
			if err := valNode.Decode(&hooks); err != nil {
				return Config{}, fmt.Errorf("parsing config hooks: %w", err)
			}
			cfg.Hooks = make(map[string]json.RawMessage, len(hooks))
			for k, v := range hooks {
				if strings.HasPrefix(v, "[") && json.Valid([]byte(v)) {
					cfg.Hooks[k] = json.RawMessage(v)
				} else {
					cfg.Hooks[k] = ExpandHookCommand(v)
				}
			}
		case "permissions":
			var perms Permissions
			if err := valNode.Decode(&perms); err != nil {
				return Config{}, fmt.Errorf("parsing config permissions: %w", err)
			}
			cfg.Permissions = perms
		case "claude_md":
			var cmd ClaudeMDConfig
			if err := valNode.Decode(&cmd); err != nil {
				return Config{}, fmt.Errorf("parsing config claude_md: %w", err)
			}
			cfg.ClaudeMD = cmd
		case "mcp":
			var mcpRaw map[string]any
			if err := valNode.Decode(&mcpRaw); err != nil {
				return Config{}, fmt.Errorf("parsing config mcp: %w", err)
			}
			cfg.MCP = make(map[string]json.RawMessage, len(mcpRaw))
			for k, v := range mcpRaw {
				data, err := json.Marshal(v)
				if err != nil {
					return Config{}, fmt.Errorf("encoding mcp entry %q: %w", k, err)
				}
				cfg.MCP[k] = json.RawMessage(data)
			}
		case "mcp_metadata":
			var meta map[string]MCPServerMeta
			if err := valNode.Decode(&meta); err != nil {
				return Config{}, fmt.Errorf("parsing config mcp_metadata: %w", err)
			}
			cfg.MCPMeta = meta
		case "keybindings":
			var kb map[string]any
			if err := valNode.Decode(&kb); err != nil {
				return Config{}, fmt.Errorf("parsing config keybindings: %w", err)
			}
			cfg.Keybindings = kb
		case "commands":
			var commands []string
			if err := valNode.Decode(&commands); err != nil {
				return Config{}, fmt.Errorf("parsing config commands: %w", err)
			}
			cfg.Commands = commands
		case "skills":
			var skills []string
			if err := valNode.Decode(&skills); err != nil {
				return Config{}, fmt.Errorf("parsing config skills: %w", err)
			}
			cfg.Skills = skills
		case "marketplaces":
			var mkts map[string]MarketplaceSource
			if err := valNode.Decode(&mkts); err != nil {
				return Config{}, fmt.Errorf("parsing config marketplaces: %w", err)
			}
			cfg.Marketplaces = mkts
		}
	}

	return cfg, nil
}

// parsePluginsNode parses the plugins mapping node with upstream/pinned/forked/excluded.
func parsePluginsNode(node *yaml.Node, cfg *Config) error {
	switch node.Kind {
	case yaml.MappingNode:
		for i := 0; i < len(node.Content)-1; i += 2 {
			keyNode := node.Content[i]
			valNode := node.Content[i+1]

			switch keyNode.Value {
			case "upstream":
				var upstream []string
				if err := valNode.Decode(&upstream); err != nil {
					return fmt.Errorf("decoding upstream plugins: %w", err)
				}
				cfg.Upstream = upstream
			case "pinned":
				if err := parsePinnedNode(valNode, cfg); err != nil {
					return err
				}
			case "forked":
				var forked []string
				if err := valNode.Decode(&forked); err != nil {
					return fmt.Errorf("decoding forked plugins: %w", err)
				}
				cfg.Forked = forked
			case "excluded":
				var excluded []string
				if err := valNode.Decode(&excluded); err != nil {
					return fmt.Errorf("decoding excluded plugins: %w", err)
				}
				cfg.Excluded = excluded
			}
		}
		return nil

	case yaml.ScalarNode:
		// Handle empty plugins: (null/empty scalar)
		return nil

	default:
		return fmt.Errorf("unexpected plugins node kind: %d", node.Kind)
	}
}

// parsePinnedNode parses the pinned plugins section.
// Each entry is a sequence of single-key mappings: - key: "version"
func parsePinnedNode(node *yaml.Node, cfg *Config) error {
	if node.Kind != yaml.SequenceNode {
		return fmt.Errorf("expected sequence for pinned plugins, got kind %d", node.Kind)
	}

	for _, item := range node.Content {
		if item.Kind != yaml.MappingNode {
			return fmt.Errorf("expected mapping in pinned entry, got kind %d", item.Kind)
		}
		if len(item.Content) < 2 {
			continue
		}
		key := item.Content[0].Value
		version := item.Content[1].Value
		cfg.Pinned[key] = version
	}
	return nil
}

// MarshalV2 serializes a Config to V2 YAML format with categorized plugins.
func MarshalV2(cfg Config) ([]byte, error) {
	// Build the YAML document manually for proper structure.
	doc := &yaml.Node{
		Kind: yaml.DocumentNode,
	}

	root := &yaml.Node{
		Kind: yaml.MappingNode,
	}
	doc.Content = append(doc.Content, root)

	// version
	root.Content = append(root.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Value: "version", Tag: "!!str"},
		&yaml.Node{Kind: yaml.ScalarNode, Value: cfg.Version, Tag: "!!str", Style: yaml.DoubleQuotedStyle},
	)

	// plugins (mapping with upstream/pinned/forked)
	pluginsMap := &yaml.Node{Kind: yaml.MappingNode}

	// upstream
	if len(cfg.Upstream) > 0 {
		upstreamSeq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, u := range cfg.Upstream {
			upstreamSeq.Content = append(upstreamSeq.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: u, Tag: "!!str"},
			)
		}
		pluginsMap.Content = append(pluginsMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "upstream", Tag: "!!str"},
			upstreamSeq,
		)
	}

	// pinned
	if len(cfg.Pinned) > 0 {
		pinnedSeq := &yaml.Node{Kind: yaml.SequenceNode}
		// Sort keys for deterministic output
		pinnedKeys := make([]string, 0, len(cfg.Pinned))
		for k := range cfg.Pinned {
			pinnedKeys = append(pinnedKeys, k)
		}
		sort.Strings(pinnedKeys)

		for _, k := range pinnedKeys {
			v := cfg.Pinned[k]
			entry := &yaml.Node{
				Kind: yaml.MappingNode,
			}
			entry.Content = append(entry.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: k, Tag: "!!str"},
				&yaml.Node{Kind: yaml.ScalarNode, Value: v, Tag: "!!str", Style: yaml.DoubleQuotedStyle},
			)
			pinnedSeq.Content = append(pinnedSeq.Content, entry)
		}
		pluginsMap.Content = append(pluginsMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "pinned", Tag: "!!str"},
			pinnedSeq,
		)
	}

	// forked
	if len(cfg.Forked) > 0 {
		forkedSeq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, f := range cfg.Forked {
			forkedSeq.Content = append(forkedSeq.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: f, Tag: "!!str"},
			)
		}
		pluginsMap.Content = append(pluginsMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "forked", Tag: "!!str"},
			forkedSeq,
		)
	}

	// excluded
	if len(cfg.Excluded) > 0 {
		excludedSeq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, e := range cfg.Excluded {
			excludedSeq.Content = append(excludedSeq.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: e, Tag: "!!str"},
			)
		}
		pluginsMap.Content = append(pluginsMap.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "excluded", Tag: "!!str"},
			excludedSeq,
		)
	}

	if len(pluginsMap.Content) > 0 {
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "plugins", Tag: "!!str"},
			pluginsMap,
		)
	}

	// settings
	if len(cfg.Settings) > 0 {
		var settingsNode yaml.Node
		if err := settingsNode.Encode(cfg.Settings); err != nil {
			return nil, fmt.Errorf("encoding settings: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "settings", Tag: "!!str"},
			&settingsNode,
		)
	}

	// hooks — store each value as its JSON string representation
	if len(cfg.Hooks) > 0 {
		hooksStrMap := make(map[string]string, len(cfg.Hooks))
		for k, v := range cfg.Hooks {
			hooksStrMap[k] = string(v)
		}
		var hooksNode yaml.Node
		if err := hooksNode.Encode(hooksStrMap); err != nil {
			return nil, fmt.Errorf("encoding hooks: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "hooks", Tag: "!!str"},
			&hooksNode,
		)
	}

	// permissions
	if len(cfg.Permissions.Allow) > 0 || len(cfg.Permissions.Deny) > 0 {
		var permsNode yaml.Node
		if err := permsNode.Encode(cfg.Permissions); err != nil {
			return nil, fmt.Errorf("encoding permissions: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "permissions", Tag: "!!str"},
			&permsNode,
		)
	}

	// claude_md
	if len(cfg.ClaudeMD.Include) > 0 {
		var claudeMDNode yaml.Node
		if err := claudeMDNode.Encode(cfg.ClaudeMD); err != nil {
			return nil, fmt.Errorf("encoding claude_md: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "claude_md", Tag: "!!str"},
			&claudeMDNode,
		)
	}

	// mcp
	if len(cfg.MCP) > 0 {
		mcpMap := make(map[string]any, len(cfg.MCP))
		for k, v := range cfg.MCP {
			var val any
			json.Unmarshal(v, &val)
			mcpMap[k] = val
		}
		var mcpNode yaml.Node
		if err := mcpNode.Encode(mcpMap); err != nil {
			return nil, fmt.Errorf("encoding mcp: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "mcp", Tag: "!!str"},
			&mcpNode,
		)
	}

	// mcp_metadata
	if len(cfg.MCPMeta) > 0 {
		var metaNode yaml.Node
		if err := metaNode.Encode(cfg.MCPMeta); err != nil {
			return nil, fmt.Errorf("encoding mcp_metadata: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "mcp_metadata", Tag: "!!str"},
			&metaNode,
		)
	}

	// keybindings
	if len(cfg.Keybindings) > 0 {
		var kbNode yaml.Node
		if err := kbNode.Encode(cfg.Keybindings); err != nil {
			return nil, fmt.Errorf("encoding keybindings: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "keybindings", Tag: "!!str"},
			&kbNode,
		)
	}

	// commands
	if len(cfg.Commands) > 0 {
		cmdSeq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, c := range cfg.Commands {
			cmdSeq.Content = append(cmdSeq.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: c, Tag: "!!str"},
			)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "commands", Tag: "!!str"},
			cmdSeq,
		)
	}

	// skills
	if len(cfg.Skills) > 0 {
		skillSeq := &yaml.Node{Kind: yaml.SequenceNode}
		for _, s := range cfg.Skills {
			skillSeq.Content = append(skillSeq.Content,
				&yaml.Node{Kind: yaml.ScalarNode, Value: s, Tag: "!!str"},
			)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "skills", Tag: "!!str"},
			skillSeq,
		)
	}

	// marketplaces
	if len(cfg.Marketplaces) > 0 {
		var mktsNode yaml.Node
		if err := mktsNode.Encode(cfg.Marketplaces); err != nil {
			return nil, fmt.Errorf("encoding marketplaces: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "marketplaces", Tag: "!!str"},
			&mktsNode,
		)
	}

	return yaml.Marshal(doc)
}

// Marshal serializes a Config to YAML bytes.
func Marshal(cfg Config) ([]byte, error) {
	return MarshalV2(cfg)
}

// ExpandHookCommand converts a simple command string to the full hook JSON format.
func ExpandHookCommand(command string) json.RawMessage {
	hook := []map[string]any{
		{
			"matcher": "",
			"hooks": []map[string]string{
				{
					"type":    "command",
					"command": command,
				},
			},
		},
	}
	data, _ := json.Marshal(hook)
	return json.RawMessage(data)
}

// SyncCategory identifies a category of config data that can be synced.
type SyncCategory string

const (
	CategorySettings    SyncCategory = "settings"
	CategoryHooks       SyncCategory = "hooks"
	CategoryPermissions SyncCategory = "permissions"
	CategoryMCP         SyncCategory = "mcp"
)

// SyncPrefs holds per-machine sync opt-out preferences.
type SyncPrefs struct {
	Skip []string `yaml:"skip,omitempty"`
}

// UserPreferences represents ~/.claude-sync/user-preferences.yaml.
type UserPreferences struct {
	SyncMode string            `yaml:"sync_mode"`
	Settings map[string]any    `yaml:"settings,omitempty"`
	Plugins  UserPluginPrefs   `yaml:"plugins,omitempty"`
	Pins     map[string]string `yaml:"pins,omitempty"`
	Sync     SyncPrefs         `yaml:"sync,omitempty"`
}

// UserPluginPrefs holds plugin override preferences.
type UserPluginPrefs struct {
	Unsubscribe []string `yaml:"unsubscribe,omitempty"`
	Personal    []string `yaml:"personal,omitempty"`
}

// ParseUserPreferences parses user-preferences.yaml.
func ParseUserPreferences(data []byte) (UserPreferences, error) {
	var prefs UserPreferences
	if err := yaml.Unmarshal(data, &prefs); err != nil {
		return UserPreferences{}, fmt.Errorf("parsing user preferences: %w", err)
	}
	if prefs.SyncMode == "" {
		prefs.SyncMode = "union"
	}
	return prefs, nil
}

// ShouldSkip returns true if the given category is in the skip list.
func (p *UserPreferences) ShouldSkip(cat SyncCategory) bool {
	for _, s := range p.Sync.Skip {
		if s == string(cat) {
			return true
		}
	}
	return false
}

// DefaultUserPreferences returns preferences with default values.
func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		SyncMode: "union",
		Pins:     map[string]string{},
	}
}

// MarshalUserPreferences serializes UserPreferences to YAML bytes.
func MarshalUserPreferences(prefs UserPreferences) ([]byte, error) {
	return yaml.Marshal(prefs)
}
