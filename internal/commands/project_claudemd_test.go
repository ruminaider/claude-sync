package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectInit_ClaudeMDFragments(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	// Setup base config with CLAUDE.md fragments
	cfg := config.Config{
		Version: "1.0.0",
		ClaudeMD: config.ClaudeMDConfig{
			Include: []string{"coding-standards"},
		},
	}
	data, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	// Create fragment file
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	os.MkdirAll(claudeMdDir, 0755)
	os.WriteFile(filepath.Join(claudeMdDir, "coding-standards.md"), []byte("## Coding Standards\n\nAlways write tests first.\n"), 0644)
	// Write manifest.yaml so fragments are tracked
	os.WriteFile(filepath.Join(claudeMdDir, "manifest.yaml"), []byte("fragments:\n  - coding-standards\n"), 0644)

	result, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"claude_md"},
	})
	require.NoError(t, err)
	assert.True(t, result.Created)

	// Verify CLAUDE.md was written to project dir
	claudeMD, err := os.ReadFile(filepath.Join(projectDir, ".claude", "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(claudeMD), "Coding Standards")
	assert.Contains(t, string(claudeMD), "Always write tests first")
}

func TestProjectInit_ClaudeMDOverrides(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	cfg := config.Config{
		Version: "1.0.0",
		ClaudeMD: config.ClaudeMDConfig{
			Include: []string{"coding-standards", "testing-patterns"},
		},
	}
	data, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	// Create fragment files
	claudeMdDir := filepath.Join(syncDir, "claude-md")
	os.MkdirAll(claudeMdDir, 0755)
	os.WriteFile(filepath.Join(claudeMdDir, "coding-standards.md"), []byte("## Coding Standards\n\nStandards content.\n"), 0644)
	os.WriteFile(filepath.Join(claudeMdDir, "testing-patterns.md"), []byte("## Testing Patterns\n\nTesting content.\n"), 0644)
	os.WriteFile(filepath.Join(claudeMdDir, "evvy-conventions.md"), []byte("## Evvy Conventions\n\nEvvy-specific content.\n"), 0644)
	os.WriteFile(filepath.Join(claudeMdDir, "manifest.yaml"), []byte("fragments:\n  - coding-standards\n  - testing-patterns\n  - evvy-conventions\n"), 0644)

	// Init project first
	commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"claude_md"},
	})

	// Now create a project config with overrides
	// The project adds "evvy-conventions" and removes "testing-patterns"
	pcfgPath := filepath.Join(projectDir, ".claude", ".claude-sync.yaml")
	pcfgYaml := `version: "1.0.0"
projected_keys:
  - claude_md
overrides:
  claude_md:
    add:
      - evvy-conventions
    remove:
      - testing-patterns
`
	os.WriteFile(pcfgPath, []byte(pcfgYaml), 0644)

	// Re-apply via a simulated pull: read config, resolve, apply
	cfgData, _ := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	parsedCfg, _ := config.Parse(cfgData)
	resolved := commands.ResolveWithProfile(parsedCfg, syncDir, "")

	projCfg, _ := project.ReadProjectConfig(projectDir)
	err = commands.ApplyProjectSettings(projectDir, resolved, projCfg, syncDir)
	require.NoError(t, err)

	claudeMD, err := os.ReadFile(filepath.Join(projectDir, ".claude", "CLAUDE.md"))
	require.NoError(t, err)
	assert.Contains(t, string(claudeMD), "Coding Standards")
	assert.Contains(t, string(claudeMD), "Evvy Conventions")
	assert.NotContains(t, string(claudeMD), "Testing Patterns")
}
