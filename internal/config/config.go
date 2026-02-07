package config

import (
	"fmt"

	"go.yaml.in/yaml/v3"
)

// Config represents ~/.claude-sync/config.yaml (Phase 1 format).
type Config struct {
	Version  string            `yaml:"version"`
	Plugins  []string          `yaml:"plugins"`
	Settings map[string]any    `yaml:"settings,omitempty"`
	Hooks    map[string]string `yaml:"hooks,omitempty"`
}

// UserPreferences represents ~/.claude-sync/user-preferences.yaml.
type UserPreferences struct {
	SyncMode string            `yaml:"sync_mode"`
	Settings map[string]any    `yaml:"settings,omitempty"`
	Plugins  UserPluginPrefs   `yaml:"plugins,omitempty"`
	Pins     map[string]string `yaml:"pins,omitempty"`
}

// UserPluginPrefs holds plugin override preferences.
type UserPluginPrefs struct {
	Unsubscribe []string `yaml:"unsubscribe,omitempty"`
	Personal    []string `yaml:"personal,omitempty"`
}

// Parse parses config.yaml bytes into a Config.
func Parse(data []byte) (Config, error) {
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parsing config: %w", err)
	}
	return cfg, nil
}

// Marshal serializes a Config to YAML bytes.
func Marshal(cfg Config) ([]byte, error) {
	return yaml.Marshal(cfg)
}

// ParseUserPreferences parses user-preferences.yaml.
func ParseUserPreferences(data []byte) (UserPreferences, error) {
	var prefs UserPreferences
	if err := yaml.Unmarshal(data, &prefs); err != nil {
		return UserPreferences{}, fmt.Errorf("parsing user preferences: %w", err)
	}
	if prefs.SyncMode == "" {
		prefs.SyncMode = "union"
	}
	return prefs, nil
}

// DefaultUserPreferences returns preferences with default values.
func DefaultUserPreferences() UserPreferences {
	return UserPreferences{
		SyncMode: "union",
		Pins:     map[string]string{},
	}
}
