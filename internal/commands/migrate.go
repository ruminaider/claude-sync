package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// MigrateNeeded always returns false — the legacy v1 flat-list format
// is no longer supported.
func MigrateNeeded(syncDir string) (bool, error) {
	return false, nil
}

// MigratePlugins reads the plugin list from a v1 config. In v1 format the
// flat plugin list is stored in cfg.Upstream by the parser.
func MigratePlugins(syncDir string) ([]string, error) {
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading config: %w", err)
	}

	cfg, err := config.Parse(data)
	if err != nil {
		return nil, fmt.Errorf("parsing config: %w", err)
	}

	return cfg.Upstream, nil
}

// MigrateApply writes a new v2 config based on the user's categorization
// choices. Each key in categories maps a plugin name to "upstream", "pinned",
// or "forked". Pinned plugins require a corresponding entry in the versions
// map. The updated config is committed with a migration message.
func MigrateApply(syncDir string, categories map[string]string, versions map[string]string) error {
	data, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	oldCfg, err := config.Parse(data)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	newCfg := config.ConfigV2{
		Version:  "1.0.0",
		Pinned:   make(map[string]string),
		Settings: oldCfg.Settings,
		Hooks:    oldCfg.Hooks,
	}

	for plugin, category := range categories {
		switch category {
		case "upstream":
			newCfg.Upstream = append(newCfg.Upstream, plugin)
		case "pinned":
			ver, ok := versions[plugin]
			if !ok || ver == "" {
				return fmt.Errorf("pinned plugin %q requires a version", plugin)
			}
			newCfg.Pinned[plugin] = ver
		case "forked":
			newCfg.Forked = append(newCfg.Forked, plugin)
		default:
			return fmt.Errorf("unknown category %q for plugin %q", category, plugin)
		}
	}

	cfgData, err := config.MarshalV2(newCfg)
	if err != nil {
		return fmt.Errorf("marshaling v2 config: %w", err)
	}

	cfgPath := filepath.Join(syncDir, "config.yaml")
	if err := os.WriteFile(cfgPath, cfgData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	if err := git.Add(syncDir, "config.yaml"); err != nil {
		return fmt.Errorf("staging config: %w", err)
	}
	if err := git.Commit(syncDir, "Migrate config to v2 (categorized plugins)"); err != nil {
		return fmt.Errorf("committing migration: %w", err)
	}

	return nil
}
