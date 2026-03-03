package plugins

import (
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

const pluginSourcesFile = "plugin-sources.yaml"

// PluginSources tracks duplicate plugin resolutions and active-dev relationships.
type PluginSources struct {
	Plugins map[string]PluginSourceEntry `yaml:"plugins"`
}

// PluginSourceEntry tracks a single plugin's source decision.
type PluginSourceEntry struct {
	ActiveSource                 string     `yaml:"active_source"`
	Suppressed                   string     `yaml:"suppressed"`
	Relationship                 string     `yaml:"relationship"`
	LocalRepo                    string     `yaml:"local_repo,omitempty"`
	DecidedAt                    time.Time  `yaml:"decided_at"`
	MarketplaceVersionAtDecision string     `yaml:"marketplace_version_at_decision,omitempty"`
	LastLocalCommitAtDecision    time.Time  `yaml:"last_local_commit_at_decision,omitempty"`
	SnoozeUntil                  *time.Time `yaml:"snooze_until,omitempty"`
}

// ReadPluginSources reads plugin-sources.yaml from syncDir.
// Returns an initialized empty struct if the file doesn't exist.
func ReadPluginSources(syncDir string) (PluginSources, error) {
	p := filepath.Join(syncDir, pluginSourcesFile)
	data, err := os.ReadFile(p)
	if os.IsNotExist(err) {
		return PluginSources{Plugins: make(map[string]PluginSourceEntry)}, nil
	}
	if err != nil {
		return PluginSources{}, err
	}

	var sources PluginSources
	if err := yaml.Unmarshal(data, &sources); err != nil {
		return PluginSources{}, err
	}
	if sources.Plugins == nil {
		sources.Plugins = make(map[string]PluginSourceEntry)
	}
	return sources, nil
}

// WritePluginSources writes plugin-sources.yaml to syncDir.
func WritePluginSources(syncDir string, sources PluginSources) error {
	data, err := yaml.Marshal(sources)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(syncDir, pluginSourcesFile), data, 0644)
}
