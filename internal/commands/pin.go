package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// Pin moves a plugin from upstream to pinned at a specific version.
// If the plugin is already pinned, its version is updated.
// If the plugin is not found in upstream or pinned, an error is returned.
func Pin(syncDir, pluginKey, version string) error {
	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	cfg, err := config.Parse(cfgData)
	if err != nil {
		return err
	}

	// Check if already pinned — update the version.
	if _, ok := cfg.Pinned[pluginKey]; ok {
		cfg.Pinned[pluginKey] = version
	} else {
		// Check if in upstream — remove and add to pinned.
		found := false
		newUpstream := make([]string, 0, len(cfg.Upstream))
		for _, u := range cfg.Upstream {
			if u == pluginKey {
				found = true
				continue
			}
			newUpstream = append(newUpstream, u)
		}
		if !found {
			return fmt.Errorf("plugin %q not found in config", pluginKey)
		}
		cfg.Upstream = newUpstream
		if cfg.Pinned == nil {
			cfg.Pinned = make(map[string]string)
		}
		cfg.Pinned[pluginKey] = version
	}

	cfg.Version = "2.0.0"

	newData, err := config.MarshalV2(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	if err := git.Add(syncDir, "config.yaml"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := git.Commit(syncDir, fmt.Sprintf("Pin %s to %s", pluginKey, version)); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}

// Unpin moves a plugin from pinned back to upstream.
// If the plugin is not pinned, an error is returned.
func Unpin(syncDir, pluginKey string) error {
	cfgPath := filepath.Join(syncDir, "config.yaml")
	cfgData, err := os.ReadFile(cfgPath)
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	cfg, err := config.Parse(cfgData)
	if err != nil {
		return err
	}

	if _, ok := cfg.Pinned[pluginKey]; !ok {
		return fmt.Errorf("plugin %q is not pinned", pluginKey)
	}

	delete(cfg.Pinned, pluginKey)
	cfg.Upstream = append(cfg.Upstream, pluginKey)

	cfg.Version = "2.0.0"

	newData, err := config.MarshalV2(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(cfgPath, newData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	if err := git.Add(syncDir, "config.yaml"); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := git.Commit(syncDir, fmt.Sprintf("Unpin %s", pluginKey)); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}
