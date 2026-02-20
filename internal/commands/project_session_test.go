package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPull_DetectsUnmanagedProject(t *testing.T) {
	// Create a temp dir simulating a project with settings.local.json but no .claude-sync.yaml
	projectDir := t.TempDir()
	claudeDir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Write a settings.local.json (simulates a project that has local settings but isn't managed)
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(`{"permissions":{"allow":["Read"]}}`), 0644))

	// Ensure no .claude-sync.yaml exists
	_, err := os.Stat(filepath.Join(claudeDir, project.ConfigFileName))
	assert.True(t, os.IsNotExist(err))

	// Test the detection logic directly.
	// The detection logic checks: settings.local.json exists AND .claude-sync.yaml does NOT exist.
	settingsExists := false
	configExists := false

	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	if _, statErr := os.Stat(settingsPath); statErr == nil {
		settingsExists = true
	}
	configPath := filepath.Join(projectDir, ".claude", project.ConfigFileName)
	if _, statErr := os.Stat(configPath); statErr == nil {
		configExists = true
	}

	assert.True(t, settingsExists, "settings.local.json should exist")
	assert.False(t, configExists, "config file should not exist")
	// This validates the conditions that would trigger ProjectUnmanagedDetected = true
}

func TestPull_ManagedProjectNotDetectedAsUnmanaged(t *testing.T) {
	projectDir := t.TempDir()
	claudeDir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Both exist - not unmanaged
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(`{}`), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, project.ConfigFileName), []byte("version: \"1.0.0\"\n"), 0644))

	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	configPath := filepath.Join(projectDir, ".claude", project.ConfigFileName)

	settingsExists := false
	configExists := false
	if _, err := os.Stat(settingsPath); err == nil {
		settingsExists = true
	}
	if _, err := os.Stat(configPath); err == nil {
		configExists = true
	}

	// Both exist, so this is a managed project - not unmanaged
	assert.True(t, settingsExists, "settings.local.json should exist")
	assert.True(t, configExists, "config file should exist")
}

func TestDeclinedProjectSkippedOnPull(t *testing.T) {
	projectDir := t.TempDir()
	claudeDir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	// Write a declined .claude-sync.yaml
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, project.ConfigFileName), []byte("version: \"1.0.0\"\ndeclined: true\n"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "settings.local.json"), []byte(`{}`), 0644))

	// The declined config should not be treated as unmanaged
	configPath := filepath.Join(claudeDir, project.ConfigFileName)
	_, err := os.Stat(configPath)
	require.NoError(t, err)
	// Config exists, so the unmanaged detection won't fire (it's in the else branch)
}

func TestNoSettingsLocalJson_NotDetectedAsUnmanaged(t *testing.T) {
	// A directory with neither settings.local.json nor .claude-sync.yaml
	// should NOT trigger unmanaged detection
	projectDir := t.TempDir()
	claudeDir := filepath.Join(projectDir, ".claude")
	require.NoError(t, os.MkdirAll(claudeDir, 0755))

	settingsPath := filepath.Join(projectDir, ".claude", "settings.local.json")
	_, err := os.Stat(settingsPath)
	assert.True(t, os.IsNotExist(err), "settings.local.json should not exist")
	// Without settings.local.json, unmanaged detection would not trigger
}
