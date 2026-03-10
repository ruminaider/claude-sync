package bundled

import (
	"embed"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:plugin
var pluginFS embed.FS

// PluginName is the directory name used for the bundled plugin.
const PluginName = "claude-sync"

// ExtractPlugin writes the embedded plugin files to dstDir.
// It is idempotent — overwrites existing files with the bundled version.
func ExtractPlugin(dstDir string) error {
	return fs.WalkDir(pluginFS, "plugin", func(path string, d fs.DirEntry, err error) error {
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

		target := filepath.Join(dstDir, rel)

		if d.IsDir() {
			return os.MkdirAll(target, 0755)
		}

		data, err := pluginFS.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading embedded %s: %w", path, err)
		}

		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}

		return os.WriteFile(target, data, 0644)
	})
}
