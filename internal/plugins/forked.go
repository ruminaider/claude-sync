package plugins

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/ruminaider/claude-sync/internal/claudecode"
)

// MarketplaceName is the identifier for the claude-sync local marketplace.
const MarketplaceName = "claude-sync-forks"

// marketplaceEntry represents a known_marketplaces.json entry for a directory-based marketplace.
type marketplaceEntry struct {
	Source          marketplaceSource `json:"source"`
	InstallLocation string           `json:"installLocation"`
	LastUpdated     string           `json:"lastUpdated"`
}

// marketplaceSource represents the source field of a marketplace entry.
type marketplaceSource struct {
	Source string `json:"source"`
	Path   string `json:"path"`
}

// RegisterLocalMarketplace adds a claude-sync-forks entry to known_marketplaces.json
// and generates a marketplace.json manifest listing all forked plugins.
// It preserves existing marketplace entries. If the marketplaces file does not exist,
// it bootstraps the Claude directory first.
func RegisterLocalMarketplace(claudeDir, syncDir string) error {
	mkts, err := claudecode.ReadMarketplaces(claudeDir)
	if err != nil {
		// If the file doesn't exist, bootstrap and retry.
		if errors.Is(err, os.ErrNotExist) {
			if bErr := claudecode.Bootstrap(claudeDir); bErr != nil {
				return fmt.Errorf("bootstrapping claude dir: %w", bErr)
			}
			mkts, err = claudecode.ReadMarketplaces(claudeDir)
			if err != nil {
				return fmt.Errorf("reading marketplaces after bootstrap: %w", err)
			}
		} else {
			return fmt.Errorf("reading marketplaces: %w", err)
		}
	}

	pluginsDir := filepath.Join(syncDir, "plugins")

	entry := marketplaceEntry{
		Source: marketplaceSource{
			Source: "directory",
			Path:   pluginsDir,
		},
		InstallLocation: pluginsDir,
		LastUpdated:     time.Now().UTC().Format(time.RFC3339),
	}

	raw, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshaling marketplace entry: %w", err)
	}
	mkts[MarketplaceName] = json.RawMessage(raw)

	if err := claudecode.WriteMarketplaces(claudeDir, mkts); err != nil {
		return fmt.Errorf("writing marketplaces: %w", err)
	}

	// Generate the marketplace manifest so Claude can discover the plugins.
	if err := generateMarketplaceManifest(pluginsDir); err != nil {
		return fmt.Errorf("generating marketplace manifest: %w", err)
	}

	return nil
}

// marketplaceManifest represents the .claude-plugin/marketplace.json file.
type marketplaceManifest struct {
	Schema  string                   `json:"$schema"`
	Name    string                   `json:"name"`
	Owner   marketplaceOwner         `json:"owner"`
	Plugins []marketplacePluginEntry `json:"plugins"`
}

// marketplaceOwner satisfies the required "owner" field in the marketplace schema.
type marketplaceOwner struct {
	Name string `json:"name"`
}

// marketplacePluginEntry represents a single plugin in the marketplace manifest.
type marketplacePluginEntry struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Version     string `json:"version,omitempty"`
	Source      string `json:"source"`
}

// pluginManifest represents the minimal fields read from a plugin's plugin.json.
type pluginManifest struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Version     string `json:"version"`
}

// generateMarketplaceManifest scans the plugins directory and writes a
// .claude-plugin/marketplace.json listing all valid plugins.
func generateMarketplaceManifest(pluginsDir string) error {
	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("reading plugins directory: %w", err)
	}

	var plugins []marketplacePluginEntry
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginsDir, entry.Name(), ".claude-plugin", "plugin.json")
		data, err := os.ReadFile(manifestPath)
		if err != nil {
			continue // skip directories without a valid plugin.json
		}
		var pm pluginManifest
		if json.Unmarshal(data, &pm) != nil {
			continue
		}

		// Remove any nested marketplace.json copied from the original installation.
		// These interfere with Claude's plugin discovery by making it treat the
		// plugin directory as a marketplace instead of a plain plugin.
		nestedMkt := filepath.Join(pluginsDir, entry.Name(), ".claude-plugin", "marketplace.json")
		os.Remove(nestedMkt) // best-effort, ignore errors

		plugins = append(plugins, marketplacePluginEntry{
			Name:        entry.Name(),
			Description: pm.Description,
			Version:     pm.Version,
			Source:      "./" + entry.Name(),
		})
	}

	manifest := marketplaceManifest{
		Schema:  "https://anthropic.com/claude-code/marketplace.schema.json",
		Name:    MarketplaceName,
		Owner:   marketplaceOwner{Name: "claude-sync"},
		Plugins: plugins,
	}

	manifestDir := filepath.Join(pluginsDir, ".claude-plugin")
	if err := os.MkdirAll(manifestDir, 0755); err != nil {
		return fmt.Errorf("creating manifest directory: %w", err)
	}

	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling manifest: %w", err)
	}

	return os.WriteFile(filepath.Join(manifestDir, "marketplace.json"), append(data, '\n'), 0644)
}

// ListForkedPlugins scans <syncDir>/plugins/ for directories containing a valid
// plugin manifest at .claude-plugin/plugin.json. It returns the directory names
// of valid plugins. Returns an empty slice (not an error) if the plugins directory
// does not exist.
func ListForkedPlugins(syncDir string) ([]string, error) {
	pluginsDir := filepath.Join(syncDir, "plugins")

	entries, err := os.ReadDir(pluginsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}
		return nil, fmt.Errorf("reading plugins directory: %w", err)
	}

	var plugins []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		manifestPath := filepath.Join(pluginsDir, entry.Name(), ".claude-plugin", "plugin.json")
		if _, err := os.Stat(manifestPath); err == nil {
			plugins = append(plugins, entry.Name())
		}
	}

	if plugins == nil {
		plugins = []string{}
	}
	return plugins, nil
}

// ForkedPluginKey returns the marketplace-qualified plugin key for a forked plugin.
// The format is "name@claude-sync-forks".
func ForkedPluginKey(name string) string {
	return name + "@" + MarketplaceName
}

