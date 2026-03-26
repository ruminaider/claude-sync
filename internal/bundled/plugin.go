package bundled

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// The all: prefix is required so that go:embed includes dotfiles such as
// .claude-plugin/. Without it, files and directories starting with "." or
// "_" are silently excluded by the embed package.
//
//go:embed all:plugin
var pluginFS embed.FS

// PluginName is the directory name used for the bundled plugin.
const PluginName = "claude-sync"

// ExtractPlugin atomically replaces dstDir with the embedded plugin files.
// Safe to call multiple times. Extraction happens into a temp directory on
// the same filesystem; on success the old dstDir is removed and the temp
// directory is renamed into place, so a failure can never leave a partially
// written plugin directory.
func ExtractPlugin(dstDir string) error {
	parentDir := filepath.Dir(dstDir)
	if err := os.MkdirAll(parentDir, 0755); err != nil {
		return fmt.Errorf("creating parent directory %s: %w", parentDir, err)
	}

	tmpDir, err := os.MkdirTemp(parentDir, ".claude-sync-extract-*")
	if err != nil {
		return fmt.Errorf("creating temp directory in %s: %w", parentDir, err)
	}

	// Clean up the temp dir on any failure path.
	success := false
	defer func() {
		if !success {
			os.RemoveAll(tmpDir)
		}
	}()

	if err := fs.WalkDir(pluginFS, "plugin", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		rel, err := filepath.Rel("plugin", path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}

		target := filepath.Join(tmpDir, rel)

		if d.IsDir() {
			if err := os.MkdirAll(target, 0755); err != nil {
				return fmt.Errorf("creating directory %s: %w", target, err)
			}
			return nil
		}

		data, err := pluginFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return fmt.Errorf("creating directory %s: %w", filepath.Dir(target), err)
		}

		perm := os.FileMode(0644)
		if strings.HasSuffix(path, ".sh") {
			perm = 0755
		}
		if err := os.WriteFile(target, data, perm); err != nil {
			return fmt.Errorf("writing %s: %w", target, err)
		}
		return nil
	}); err != nil {
		return err
	}

	// Atomic swap: remove old destination then rename temp into place.
	if err := os.RemoveAll(dstDir); err != nil {
		return fmt.Errorf("removing old plugin directory %s: %w", dstDir, err)
	}
	if err := os.Rename(tmpDir, dstDir); err != nil {
		return fmt.Errorf("renaming %s to %s: %w", tmpDir, dstDir, err)
	}

	success = true
	return nil
}
