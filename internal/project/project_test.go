package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadProjectConfig(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	yamlContent := `version: "1.0.0"
profile: work
initialized: "2026-02-19T10:30:00Z"
projected_keys:
  - hooks
  - permissions
overrides:
  permissions:
    add_allow:
      - "mcp__evvy_db__query"
      - "Bash(docker compose:*)"
`
	os.WriteFile(filepath.Join(claudeDir, ".claude-sync.yaml"), []byte(yamlContent), 0644)

	cfg, err := ReadProjectConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, "1.0.0", cfg.Version)
	assert.Equal(t, "work", cfg.Profile)
	assert.Equal(t, []string{"hooks", "permissions"}, cfg.ProjectedKeys)
	assert.Equal(t, []string{"mcp__evvy_db__query", "Bash(docker compose:*)"}, cfg.Overrides.Permissions.AddAllow)
}

func TestReadProjectConfig_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := ReadProjectConfig(dir)
	assert.ErrorIs(t, err, ErrNoProjectConfig)
}

func TestReadProjectConfig_Declined(t *testing.T) {
	dir := t.TempDir()
	claudeDir := filepath.Join(dir, ".claude")
	os.MkdirAll(claudeDir, 0755)

	yamlContent := `version: "1.0.0"
declined: true
`
	os.WriteFile(filepath.Join(claudeDir, ".claude-sync.yaml"), []byte(yamlContent), 0644)

	cfg, err := ReadProjectConfig(dir)
	require.NoError(t, err)
	assert.True(t, cfg.Declined)
}

func TestWriteProjectConfig(t *testing.T) {
	dir := t.TempDir()

	cfg := ProjectConfig{
		Version:       "1.0.0",
		Profile:       "work",
		Initialized:   "2026-02-19T10:30:00Z",
		ProjectedKeys: []string{"hooks", "permissions"},
	}

	err := WriteProjectConfig(dir, cfg)
	require.NoError(t, err)

	// Read back and verify
	got, err := ReadProjectConfig(dir)
	require.NoError(t, err)
	assert.Equal(t, cfg.Version, got.Version)
	assert.Equal(t, cfg.Profile, got.Profile)
	assert.Equal(t, cfg.ProjectedKeys, got.ProjectedKeys)
}

func TestWriteProjectConfig_CreatesClaudeDir(t *testing.T) {
	dir := t.TempDir()

	cfg := ProjectConfig{
		Version: "1.0.0",
		Profile: "test",
	}

	err := WriteProjectConfig(dir, cfg)
	require.NoError(t, err)

	// Verify .claude dir was created
	_, err = os.Stat(filepath.Join(dir, ".claude"))
	assert.NoError(t, err)
}

func TestFindProjectRoot(t *testing.T) {
	root := t.TempDir()
	claudeDir := filepath.Join(root, ".claude")
	os.MkdirAll(claudeDir, 0755)
	os.WriteFile(filepath.Join(claudeDir, ".claude-sync.yaml"), []byte("version: \"1.0.0\"\nprofile: work\n"), 0644)

	nested := filepath.Join(root, "src", "pkg")
	os.MkdirAll(nested, 0755)

	found, err := FindProjectRoot(nested)
	require.NoError(t, err)
	assert.Equal(t, root, found)
}

func TestFindProjectRoot_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := FindProjectRoot(dir)
	assert.ErrorIs(t, err, ErrNoProjectConfig)
}
