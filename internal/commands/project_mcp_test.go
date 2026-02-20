package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectInit_MCPOverrides(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	// Base config with MCP server
	cfg := config.Config{
		Version: "1.0.0",
		MCP: map[string]json.RawMessage{
			"shared-db": json.RawMessage(`{"command":"shared-db-server","args":["--port","5432"]}`),
		},
	}
	data, err := config.MarshalV2(cfg)
	require.NoError(t, err)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	result, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"mcp"},
	})
	require.NoError(t, err)
	assert.True(t, result.Created)

	// Verify .mcp.json written to project dir
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	mcp, err := claudecode.ReadMCPConfigFile(mcpPath)
	require.NoError(t, err)
	assert.Contains(t, mcp, "shared-db")
}

func TestProjectInit_MCPOverridesAddRemove(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	cfg := config.Config{
		Version: "1.0.0",
		MCP: map[string]json.RawMessage{
			"shared-db":   json.RawMessage(`{"command":"shared-db"}`),
			"shared-logs": json.RawMessage(`{"command":"shared-logs"}`),
		},
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	// Init the project first
	_, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"mcp"},
	})
	require.NoError(t, err)

	// Write overrides programmatically to .claude-sync.yaml
	pcfg := project.ProjectConfig{
		Version:       "1.0.0",
		ProjectedKeys: []string{"mcp"},
		Overrides: project.ProjectOverrides{
			MCP: project.ProjectMCPOverrides{
				Add: map[string]json.RawMessage{
					"evvy-db": json.RawMessage(`{"command":"evvy-db-server","args":["--project","evvy"]}`),
				},
				Remove: []string{"shared-logs"},
			},
		},
	}
	err = project.WriteProjectConfig(projectDir, pcfg)
	require.NoError(t, err)

	// Re-apply settings
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	parsedCfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	resolved := commands.ResolveWithProfile(parsedCfg, syncDir, "")

	projCfg, err := project.ReadProjectConfig(projectDir)
	require.NoError(t, err)

	err = commands.ApplyProjectSettings(projectDir, resolved, projCfg, syncDir)
	require.NoError(t, err)

	mcpPath := filepath.Join(projectDir, ".mcp.json")
	mcp, err := claudecode.ReadMCPConfigFile(mcpPath)
	require.NoError(t, err)
	assert.Contains(t, mcp, "shared-db")
	assert.Contains(t, mcp, "evvy-db")
	assert.NotContains(t, mcp, "shared-logs")
}

func TestProjectInit_MCPEmptyNoFile(t *testing.T) {
	projectDir, syncDir := setupProjectTestEnv(t)

	// Config with no MCP servers
	cfg := config.Config{
		Version: "1.0.0",
	}
	data, _ := config.MarshalV2(cfg)
	os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644)

	_, err := commands.ProjectInit(commands.ProjectInitOptions{
		ProjectDir:    projectDir,
		SyncDir:       syncDir,
		ProjectedKeys: []string{"mcp"},
	})
	require.NoError(t, err)

	// .mcp.json should NOT be created when there are no MCP servers
	mcpPath := filepath.Join(projectDir, ".mcp.json")
	_, err = os.Stat(mcpPath)
	assert.True(t, os.IsNotExist(err), ".mcp.json should not exist when no MCP servers are configured")
}
