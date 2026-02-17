package claudecode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// InstalledPlugins represents ~/.claude/plugins/installed_plugins.json.
type InstalledPlugins struct {
	Version int                             `json:"version"`
	Plugins map[string][]PluginInstallation `json:"plugins"`
}

// PluginInstallation represents a single plugin installation entry.
type PluginInstallation struct {
	Scope        string `json:"scope"`
	InstallPath  string `json:"installPath"`
	ProjectPath  string `json:"projectPath,omitempty"`
	Version      string `json:"version"`
	InstalledAt  string `json:"installedAt"`
	LastUpdated  string `json:"lastUpdated"`
	GitCommitSha string `json:"gitCommitSha,omitempty"`
}

// PluginKeys returns a list of plugin keys (e.g., "beads@beads-marketplace").
func (ip *InstalledPlugins) PluginKeys() []string {
	keys := make([]string, 0, len(ip.Plugins))
	for k := range ip.Plugins {
		keys = append(keys, k)
	}
	return keys
}

// DefaultClaudeDir returns the default ~/.claude path.
func DefaultClaudeDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude")
}

// DirExists returns true if the Claude Code directory exists.
func DirExists(claudeDir string) bool {
	info, err := os.Stat(claudeDir)
	return err == nil && info.IsDir()
}

// ReadInstalledPlugins reads installed_plugins.json.
func ReadInstalledPlugins(claudeDir string) (*InstalledPlugins, error) {
	path := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading installed plugins: %w", err)
	}
	var plugins InstalledPlugins
	if err := json.Unmarshal(data, &plugins); err != nil {
		return nil, fmt.Errorf("parsing installed plugins: %w", err)
	}
	return &plugins, nil
}

// ReadSettings reads settings.json as a generic map.
func ReadSettings(claudeDir string) (map[string]json.RawMessage, error) {
	path := filepath.Join(claudeDir, "settings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading settings: %w", err)
	}
	var settings map[string]json.RawMessage
	if err := json.Unmarshal(data, &settings); err != nil {
		return nil, fmt.Errorf("parsing settings: %w", err)
	}
	return settings, nil
}

// WriteSettings writes settings.json.
func WriteSettings(claudeDir string, settings map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(settings, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling settings: %w", err)
	}
	path := filepath.Join(claudeDir, "settings.json")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// ReadMarketplaces reads known_marketplaces.json.
func ReadMarketplaces(claudeDir string) (map[string]json.RawMessage, error) {
	path := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading marketplaces: %w", err)
	}
	var mkts map[string]json.RawMessage
	if err := json.Unmarshal(data, &mkts); err != nil {
		return nil, fmt.Errorf("parsing marketplaces: %w", err)
	}
	return mkts, nil
}

// WriteMarketplaces writes known_marketplaces.json.
func WriteMarketplaces(claudeDir string, mkts map[string]json.RawMessage) error {
	data, err := json.MarshalIndent(mkts, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling marketplaces: %w", err)
	}
	path := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// WriteInstalledPlugins writes installed_plugins.json.
func WriteInstalledPlugins(claudeDir string, ip *InstalledPlugins) error {
	data, err := json.MarshalIndent(ip, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling installed plugins: %w", err)
	}
	path := filepath.Join(claudeDir, "plugins", "installed_plugins.json")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// ReadMCPConfigFile reads an .mcp.json file at the given path.
// Returns empty map if file doesn't exist.
func ReadMCPConfigFile(path string) (map[string]json.RawMessage, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]json.RawMessage{}, nil
		}
		return nil, fmt.Errorf("reading MCP config: %w", err)
	}

	// .mcp.json has a top-level "mcpServers" key
	var wrapper struct {
		MCPServers map[string]json.RawMessage `json:"mcpServers"`
	}
	if err := json.Unmarshal(data, &wrapper); err != nil {
		// Try direct map format as fallback
		var direct map[string]json.RawMessage
		if err2 := json.Unmarshal(data, &direct); err2 != nil {
			return nil, fmt.Errorf("parsing MCP config: %w", err)
		}
		return direct, nil
	}
	if wrapper.MCPServers == nil {
		return map[string]json.RawMessage{}, nil
	}
	return wrapper.MCPServers, nil
}

// WriteMCPConfigFile writes MCP server configs to the given path.
func WriteMCPConfigFile(path string, mcp map[string]json.RawMessage) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return fmt.Errorf("creating directory for MCP config: %w", err)
	}
	wrapper := map[string]any{
		"mcpServers": mcp,
	}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling MCP config: %w", err)
	}
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// ReadMCPConfig reads ~/.claude/.mcp.json as a map of server configs.
// Returns empty map if file doesn't exist.
func ReadMCPConfig(claudeDir string) (map[string]json.RawMessage, error) {
	return ReadMCPConfigFile(filepath.Join(claudeDir, ".mcp.json"))
}

// WriteMCPConfig writes the MCP server configs to ~/.claude/.mcp.json.
func WriteMCPConfig(claudeDir string, mcp map[string]json.RawMessage) error {
	return WriteMCPConfigFile(filepath.Join(claudeDir, ".mcp.json"), mcp)
}

// ReadKeybindings reads ~/.claude/keybindings.json.
// Returns empty map if file doesn't exist.
func ReadKeybindings(claudeDir string) (map[string]any, error) {
	path := filepath.Join(claudeDir, "keybindings.json")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]any{}, nil
		}
		return nil, fmt.Errorf("reading keybindings: %w", err)
	}
	var kb map[string]any
	if err := json.Unmarshal(data, &kb); err != nil {
		return nil, fmt.Errorf("parsing keybindings: %w", err)
	}
	return kb, nil
}

// WriteKeybindings writes keybindings to ~/.claude/keybindings.json.
func WriteKeybindings(claudeDir string, kb map[string]any) error {
	data, err := json.MarshalIndent(kb, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling keybindings: %w", err)
	}
	path := filepath.Join(claudeDir, "keybindings.json")
	return os.WriteFile(path, append(data, '\n'), 0644)
}

// Bootstrap creates minimal Claude Code directory structure for fresh machines.
func Bootstrap(claudeDir string) error {
	pluginDir := filepath.Join(claudeDir, "plugins")
	if err := os.MkdirAll(pluginDir, 0755); err != nil {
		return fmt.Errorf("creating plugin directory: %w", err)
	}

	pluginsFile := filepath.Join(pluginDir, "installed_plugins.json")
	if _, err := os.Stat(pluginsFile); os.IsNotExist(err) {
		data := []byte("{\"version\": 2, \"plugins\": {}}\n")
		if err := os.WriteFile(pluginsFile, data, 0644); err != nil {
			return err
		}
	}

	mktsFile := filepath.Join(pluginDir, "known_marketplaces.json")
	if _, err := os.Stat(mktsFile); os.IsNotExist(err) {
		if err := os.WriteFile(mktsFile, []byte("{}\n"), 0644); err != nil {
			return err
		}
	}

	return nil
}
