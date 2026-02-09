package config

import (
	"fmt"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// ConfigV2 represents ~/.claude-sync/config.yaml with categorized plugins.
// Version "1.0.0" uses a flat plugin list (all Upstream).
// Version "2.0.0" uses categorized plugins: upstream, pinned, forked.
type ConfigV2 struct {
	Version  string            `yaml:"version"`
	Upstream []string          `yaml:"-"` // parsed from plugins.upstream (or flat list in v1)
	Pinned   map[string]string `yaml:"-"` // parsed from plugins.pinned (key -> version)
	Forked   []string          `yaml:"-"` // parsed from plugins.forked
	Settings map[string]any    `yaml:"settings,omitempty"`
	Hooks    map[string]string `yaml:"hooks,omitempty"`
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
			cfg.Hooks = hooks
		}
	}

	return cfg, nil
}

// parsePluginsNode parses the plugins node, handling both v1 (sequence) and v2 (mapping) formats.
func parsePluginsNode(node *yaml.Node, cfg *Config) error {
	switch node.Kind {
	case yaml.SequenceNode:
		// V1 format: flat list -> all go to Upstream
		var plugins []string
		if err := node.Decode(&plugins); err != nil {
			return fmt.Errorf("decoding plugin list: %w", err)
		}
		cfg.Upstream = plugins
		return nil

	case yaml.MappingNode:
		// V2 format: categorized mapping with upstream/pinned/forked
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

	// hooks
	if len(cfg.Hooks) > 0 {
		var hooksNode yaml.Node
		if err := hooksNode.Encode(cfg.Hooks); err != nil {
			return nil, fmt.Errorf("encoding hooks: %w", err)
		}
		root.Content = append(root.Content,
			&yaml.Node{Kind: yaml.ScalarNode, Value: "hooks", Tag: "!!str"},
			&hooksNode,
		)
	}

	return yaml.Marshal(doc)
}

// marshalV1 serializes a Config to V1 YAML format with flat plugin list.
func marshalV1(cfg Config) ([]byte, error) {
	// Build a v1 compatible structure for marshaling.
	v1 := struct {
		Version  string            `yaml:"version"`
		Plugins  []string          `yaml:"plugins"`
		Settings map[string]any    `yaml:"settings,omitempty"`
		Hooks    map[string]string `yaml:"hooks,omitempty"`
	}{
		Version:  cfg.Version,
		Plugins:  cfg.Upstream,
		Settings: cfg.Settings,
		Hooks:    cfg.Hooks,
	}
	return yaml.Marshal(v1)
}

// Marshal serializes a Config to YAML bytes. It auto-selects v1 or v2
// format based on the version string.
func Marshal(cfg Config) ([]byte, error) {
	if isV2(cfg.Version) {
		return MarshalV2(cfg)
	}
	return marshalV1(cfg)
}

// isV2 returns true if the version string indicates a v2 config.
func isV2(version string) bool {
	return strings.HasPrefix(version, "2.")
}

// SyncCategory identifies a category of config data that can be synced.
type SyncCategory string

const (
	CategorySettings SyncCategory = "settings"
	CategoryHooks    SyncCategory = "hooks"
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
