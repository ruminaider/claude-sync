package paths

import (
	"os"
	"path/filepath"
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
