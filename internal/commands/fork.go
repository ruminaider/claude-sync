package commands

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
)

// Fork copies a plugin from the Claude Code cache into the sync repo's plugins/
// directory, then updates config to move it from upstream/pinned to forked.
func Fork(claudeDir, syncDir, pluginKey string) error {
	// Split pluginKey on "@" to get name and marketplace.
	parts := strings.SplitN(pluginKey, "@", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid plugin key %q: expected format name@marketplace", pluginKey)
	}
	name := parts[0]

	// Read installed_plugins.json to find the plugin's installPath.
	installed, err := claudecode.ReadInstalledPlugins(claudeDir)
	if err != nil {
		return fmt.Errorf("reading installed plugins: %w", err)
	}

	installations, ok := installed.Plugins[pluginKey]
	if !ok || len(installations) == 0 {
		return fmt.Errorf("plugin %q not found in installed_plugins.json", pluginKey)
	}

	srcDir := installations[0].InstallPath
	if srcDir == "" {
		return fmt.Errorf("plugin %q has no installPath", pluginKey)
	}

	// Copy the entire plugin directory to <syncDir>/plugins/<pluginName>/.
	dstDir := filepath.Join(syncDir, "plugins", name)
	if err := copyDir(srcDir, dstDir); err != nil {
		return fmt.Errorf("copying plugin directory: %w", err)
	}

	// Read and update config.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	cfg, err := config.Parse(cfgData)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	// Remove from upstream.
	cfg.Upstream = removeFromSlice(cfg.Upstream, pluginKey)

	// Remove from pinned.
	delete(cfg.Pinned, pluginKey)

	// Add pluginName to forked (if not already present).
	if !containsString(cfg.Forked, name) {
		cfg.Forked = append(cfg.Forked, name)
	}

	// Write updated config.
	newCfgData, err := config.MarshalV2(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), newCfgData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Stage all changes and commit.
	if err := git.Add(syncDir, "."); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := git.Commit(syncDir, fmt.Sprintf("Fork %s for customization", name)); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}

// Unfork removes a forked plugin directory and moves it back to upstream.
func Unfork(syncDir, pluginName, marketplace string) error {
	// Remove <syncDir>/plugins/<pluginName>/ directory.
	pluginDir := filepath.Join(syncDir, "plugins", pluginName)
	if err := os.RemoveAll(pluginDir); err != nil {
		return fmt.Errorf("removing forked plugin directory: %w", err)
	}

	// Read and update config.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	if err != nil {
		return fmt.Errorf("reading config: %w", err)
	}

	cfg, err := config.Parse(cfgData)
	if err != nil {
		return fmt.Errorf("parsing config: %w", err)
	}

	// Remove from forked.
	cfg.Forked = removeFromSlice(cfg.Forked, pluginName)

	// Add <name>@<marketplace> back to upstream.
	upstreamKey := pluginName + "@" + marketplace
	if !containsString(cfg.Upstream, upstreamKey) {
		cfg.Upstream = append(cfg.Upstream, upstreamKey)
	}

	// Write updated config.
	newCfgData, err := config.MarshalV2(cfg)
	if err != nil {
		return fmt.Errorf("marshaling config: %w", err)
	}
	if err := os.WriteFile(filepath.Join(syncDir, "config.yaml"), newCfgData, 0644); err != nil {
		return fmt.Errorf("writing config: %w", err)
	}

	// Stage and commit.
	if err := git.Add(syncDir, "."); err != nil {
		return fmt.Errorf("staging changes: %w", err)
	}
	if err := git.Commit(syncDir, fmt.Sprintf("Unfork %s — returning to upstream", pluginName)); err != nil {
		return fmt.Errorf("committing: %w", err)
	}

	return nil
}

// copyDir recursively copies a directory tree from src to dst.
func copyDir(src, dst string) error {
	return filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}

		// Compute the relative path and the corresponding destination path.
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)

		if d.IsDir() {
			return os.MkdirAll(dstPath, 0755)
		}

		return copyFile(path, dstPath)
	})
}

// copyFile copies a single file from src to dst.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}

	return out.Close()
}

// removeFromSlice returns a new slice with all occurrences of val removed.
func removeFromSlice(slice []string, val string) []string {
	result := make([]string, 0, len(slice))
	for _, s := range slice {
		if s != val {
			result = append(result, s)
		}
	}
	return result
}

// containsString returns true if the slice contains the given string.
func containsString(slice []string, val string) bool {
	for _, s := range slice {
		if s == val {
			return true
		}
	}
	return false
}
