package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func home() string {
	h, _ := os.UserHomeDir()
	return h
}

// SyncDir returns ~/.claude-sync.
func SyncDir() string {
	return filepath.Join(home(), ".claude-sync")
}

// ClaudeDir returns ~/.claude.
func ClaudeDir() string {
	return filepath.Join(home(), ".claude")
}

// ConfigFile returns ~/.claude-sync/config.yaml.
func ConfigFile() string {
	return filepath.Join(SyncDir(), "config.yaml")
}

// UserPreferencesFile returns ~/.claude-sync/user-preferences.yaml.
func UserPreferencesFile() string {
	return filepath.Join(SyncDir(), "user-preferences.yaml")
}

// ForkedPluginsDir returns ~/.claude-sync/plugins.
func ForkedPluginsDir() string {
	return filepath.Join(SyncDir(), "plugins")
}

// SyncMemoryDir returns ~/.claude-sync/memory.
func SyncMemoryDir() string {
	return filepath.Join(SyncDir(), "memory")
}

// ClaudeMemoryDir returns ~/.claude/memory.
func ClaudeMemoryDir() string {
	return filepath.Join(ClaudeDir(), "memory")
}

// CCSDir returns ~/.ccs.
func CCSDir() string {
	return filepath.Join(home(), ".ccs")
}

// CCSInstances returns the list of CCS instance directory paths and whether
// CCS is installed. Each path is ~/.ccs/instances/<name>.
func CCSInstances() ([]string, bool) {
	instancesDir := filepath.Join(CCSDir(), "instances")
	entries, err := os.ReadDir(instancesDir)
	if err != nil {
		return nil, false
	}
	var dirs []string
	for _, e := range entries {
		if e.IsDir() && !strings.HasPrefix(e.Name(), ".") {
			dirs = append(dirs, filepath.Join(instancesDir, e.Name()))
		}
	}
	return dirs, len(dirs) > 0
}

// CCSInstanceMemoryDir returns the memory directory for a CCS instance.
func CCSInstanceMemoryDir(instancePath string) string {
	return filepath.Join(instancePath, "memory")
}
