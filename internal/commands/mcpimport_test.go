package commands_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/config"
	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/ruminaider/claude-sync/internal/profiles"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeMCPFile creates a .mcp.json file with the given servers.
func writeMCPFile(t *testing.T, path string, servers map[string]any) {
	t.Helper()
	wrapper := map[string]any{"mcpServers": servers}
	data, err := json.MarshalIndent(wrapper, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0755))
	require.NoError(t, os.WriteFile(path, data, 0644))
}

// --- Secret Detection Tests ---

func TestMCPImportScan_DetectsSecretKeys(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	writeMCPFile(t, mcpPath, map[string]any{
		"my-server": map[string]any{
			"command": "npx",
			"env": map[string]string{
				"RENDER_API_KEY": "rnd_abc123",
				"APP_TOKEN":     "tok_secret456",
				"APP_SECRET":    "mysecretvalue",
				"APP_PASSWORD":  "pass123",
				"MY_APIKEY":     "key789",
				"SAFE_VALUE":    "hello",
			},
		},
	})

	scan, err := commands.MCPImportScan(mcpPath)
	require.NoError(t, err)
	assert.Len(t, scan.Servers, 1)

	// Should detect 5 secrets (all except SAFE_VALUE).
	secretKeys := make(map[string]bool)
	for _, s := range scan.Secrets {
		secretKeys[s.EnvKey] = true
	}
	assert.True(t, secretKeys["RENDER_API_KEY"])
	assert.True(t, secretKeys["APP_TOKEN"])
	assert.True(t, secretKeys["APP_SECRET"])
	assert.True(t, secretKeys["APP_PASSWORD"])
	assert.True(t, secretKeys["MY_APIKEY"])
	assert.False(t, secretKeys["SAFE_VALUE"])
}

func TestMCPImportScan_DetectsSecretValues(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	writeMCPFile(t, mcpPath, map[string]any{
		"my-server": map[string]any{
			"command": "npx",
			"env": map[string]string{
				"OPENAI":     "sk-abc123def456",
				"RENDER":     "rnd_xyz789",
				"NEWRELIC":   "NRAK-ABCDEF",
				"SLACK_C":    "xoxc-token-here",
				"SLACK_D":    "xoxd-token-here",
				"SLACK_B":    "xoxb-token-here",
				"SLACK_P":    "xoxp-token-here",
				"SHOPIFY":    "shpat_something",
				"PLANET":     "pa-something",
				"PAYMENT":    "live_something",
				"SAFE_SHORT": "hello",
			},
		},
	})

	scan, err := commands.MCPImportScan(mcpPath)
	require.NoError(t, err)

	secretKeys := make(map[string]bool)
	for _, s := range scan.Secrets {
		secretKeys[s.EnvKey] = true
	}
	assert.True(t, secretKeys["OPENAI"], "sk- prefix")
	assert.True(t, secretKeys["RENDER"], "rnd_ prefix")
	assert.True(t, secretKeys["NEWRELIC"], "NRAK- prefix")
	assert.True(t, secretKeys["SLACK_C"], "xoxc- prefix")
	assert.True(t, secretKeys["SLACK_D"], "xoxd- prefix")
	assert.True(t, secretKeys["SLACK_B"], "xoxb- prefix")
	assert.True(t, secretKeys["SLACK_P"], "xoxp- prefix")
	assert.True(t, secretKeys["SHOPIFY"], "shpat_ prefix")
	assert.True(t, secretKeys["PLANET"], "pa- prefix")
	assert.True(t, secretKeys["PAYMENT"], "live_ prefix")
	assert.False(t, secretKeys["SAFE_SHORT"], "short safe value")
}

func TestMCPImportScan_DetectsLongAlphanumeric(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	writeMCPFile(t, mcpPath, map[string]any{
		"my-server": map[string]any{
			"command": "npx",
			"env": map[string]string{
				"LONG_CRED": "abcdefghijklmnopqrstuvwxyz0123456789", // 36 chars, 100% alnum
				"SAFE_URL":  "https://example.com/api", // short URL, under 32 chars
			},
		},
	})

	scan, err := commands.MCPImportScan(mcpPath)
	require.NoError(t, err)

	secretKeys := make(map[string]bool)
	for _, s := range scan.Secrets {
		secretKeys[s.EnvKey] = true
	}
	assert.True(t, secretKeys["LONG_CRED"], "long alphanumeric string")
	assert.False(t, secretKeys["SAFE_URL"], "URL has non-alnum chars below 80% threshold")
}

func TestMCPImportScan_SkipsAlreadyTemplated(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	writeMCPFile(t, mcpPath, map[string]any{
		"my-server": map[string]any{
			"command": "npx",
			"env": map[string]string{
				"API_KEY":   "${API_KEY}",  // already templated, should skip
				"API_TOKEN": "sk-realvalue", // not templated, should detect
			},
		},
	})

	scan, err := commands.MCPImportScan(mcpPath)
	require.NoError(t, err)

	secretKeys := make(map[string]bool)
	for _, s := range scan.Secrets {
		secretKeys[s.EnvKey] = true
	}
	assert.False(t, secretKeys["API_KEY"], "already templated values should be skipped")
	assert.True(t, secretKeys["API_TOKEN"], "real secret should be detected")
}

func TestMCPImportScan_ServerWithNoEnv(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")

	writeMCPFile(t, mcpPath, map[string]any{
		"simple-server": map[string]any{
			"command": "npx",
			"args":    []string{"-y", "my-server"},
		},
	})

	scan, err := commands.MCPImportScan(mcpPath)
	require.NoError(t, err)
	assert.Len(t, scan.Servers, 1)
	assert.Empty(t, scan.Secrets)
}

func TestMCPImportScan_FileNotFound(t *testing.T) {
	_, err := commands.MCPImportScan("/nonexistent/.mcp.json")
	assert.Error(t, err)
}

func TestMCPImportScan_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, ".mcp.json")
	writeMCPFile(t, mcpPath, map[string]any{})

	_, err := commands.MCPImportScan(mcpPath)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no MCP servers found")
}

// --- DetectMCPSecrets Tests (in-memory detection) ---

func TestDetectMCPSecrets(t *testing.T) {
	serverA, _ := json.Marshal(map[string]any{
		"command": "npx",
		"env": map[string]string{
			"API_KEY":    "sk-realkey123",
			"SAFE_VALUE": "hello",
		},
	})
	serverB, _ := json.Marshal(map[string]any{
		"command": "node",
		"env": map[string]string{
			"DB_PASSWORD": "mysecretpass",
			"APP_NAME":    "my-app",
		},
	})

	servers := map[string]json.RawMessage{
		"server-a": json.RawMessage(serverA),
		"server-b": json.RawMessage(serverB),
	}

	secrets := commands.DetectMCPSecrets(servers)

	secretMap := make(map[string]string) // "server:key" -> reason
	for _, s := range secrets {
		secretMap[s.ServerName+":"+s.EnvKey] = s.Reason
	}

	assert.Contains(t, secretMap, "server-a:API_KEY")
	assert.Contains(t, secretMap, "server-b:DB_PASSWORD")
	assert.NotContains(t, secretMap, "server-a:SAFE_VALUE")
	assert.NotContains(t, secretMap, "server-b:APP_NAME")

	// Results should be sorted by server name, then env key.
	assert.True(t, len(secrets) >= 2)
	assert.Equal(t, "server-a", secrets[0].ServerName)
}

func TestDetectMCPSecrets_Empty(t *testing.T) {
	// Empty map returns nil.
	secrets := commands.DetectMCPSecrets(nil)
	assert.Nil(t, secrets)

	// Map with no env sections returns nil.
	serverData, _ := json.Marshal(map[string]any{"command": "npx"})
	secrets = commands.DetectMCPSecrets(map[string]json.RawMessage{
		"safe": json.RawMessage(serverData),
	})
	assert.Nil(t, secrets)
}

func TestDetectMCPSecrets_AlreadyTemplated(t *testing.T) {
	serverData, _ := json.Marshal(map[string]any{
		"command": "npx",
		"env": map[string]string{
			"API_KEY": "${API_KEY}", // already templated
		},
	})
	secrets := commands.DetectMCPSecrets(map[string]json.RawMessage{
		"server": json.RawMessage(serverData),
	})
	assert.Nil(t, secrets, "already-templated values should not be flagged")
}

// --- ReplaceSecrets Tests ---

func TestReplaceSecrets_ReplacesValues(t *testing.T) {
	serverData, _ := json.Marshal(map[string]any{
		"command": "npx",
		"env": map[string]string{
			"API_KEY":  "sk-realkey123",
			"APP_NAME": "my-app",
		},
	})

	servers := map[string]json.RawMessage{
		"my-server": json.RawMessage(serverData),
	}

	secrets := []commands.DetectedSecret{
		{ServerName: "my-server", EnvKey: "API_KEY", Value: "sk-realkey123"},
	}

	result := commands.ReplaceSecrets(servers, secrets)

	var cfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(result["my-server"], &cfg))
	assert.Equal(t, "${API_KEY}", cfg.Env["API_KEY"])
	assert.Equal(t, "my-app", cfg.Env["APP_NAME"]) // untouched
}

func TestReplaceSecrets_NoSecrets(t *testing.T) {
	serverData, _ := json.Marshal(map[string]any{"command": "npx"})
	servers := map[string]json.RawMessage{"s": json.RawMessage(serverData)}

	result := commands.ReplaceSecrets(servers, nil)
	assert.Equal(t, string(servers["s"]), string(result["s"]))
}

// --- MCPImport Tests ---

func setupImportEnv(t *testing.T) string {
	t.Helper()
	syncDir := t.TempDir()

	// Create a minimal config.yaml.
	cfg := config.Config{
		Version: "1.0.0",
	}
	data, err := config.Marshal(cfg)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(syncDir, "config.yaml"), data, 0644))

	// Init git repo.
	require.NoError(t, git.Init(syncDir))
	require.NoError(t, git.Add(syncDir, "config.yaml"))
	require.NoError(t, git.Commit(syncDir, "init"))

	return syncDir
}

func TestMCPImport_BaseConfig(t *testing.T) {
	syncDir := setupImportEnv(t)

	serverData, _ := json.Marshal(map[string]any{
		"command": "npx",
		"args":    []string{"-y", "my-server"},
	})

	result, err := commands.MCPImport(commands.MCPImportOptions{
		SyncDir: syncDir,
		Servers: map[string]json.RawMessage{
			"my-server": json.RawMessage(serverData),
		},
	})
	require.NoError(t, err)
	assert.Contains(t, result.Imported, "my-server")
	assert.Empty(t, result.TargetProfile)

	// Verify config was updated.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Contains(t, cfg.MCP, "my-server")
}

func TestMCPImport_WithProjectPath(t *testing.T) {
	syncDir := setupImportEnv(t)

	serverData, _ := json.Marshal(map[string]any{"command": "npx"})

	result, err := commands.MCPImport(commands.MCPImportOptions{
		SyncDir:     syncDir,
		Servers:     map[string]json.RawMessage{"server-a": json.RawMessage(serverData)},
		ProjectPath: "/home/user/projects/myapp",
	})
	require.NoError(t, err)
	assert.Equal(t, "/home/user/projects/myapp", result.ProjectPath)

	// Verify MCPMeta was written.
	cfgData, err := os.ReadFile(filepath.Join(syncDir, "config.yaml"))
	require.NoError(t, err)
	cfg, err := config.Parse(cfgData)
	require.NoError(t, err)
	assert.Contains(t, cfg.MCPMeta, "server-a")
	assert.Equal(t, "/home/user/projects/myapp", cfg.MCPMeta["server-a"].SourceProject)
}

func TestMCPImport_MultipleServers(t *testing.T) {
	syncDir := setupImportEnv(t)

	serverA, _ := json.Marshal(map[string]any{"command": "server-a"})
	serverB, _ := json.Marshal(map[string]any{"command": "server-b"})

	result, err := commands.MCPImport(commands.MCPImportOptions{
		SyncDir: syncDir,
		Servers: map[string]json.RawMessage{
			"server-a": json.RawMessage(serverA),
			"server-b": json.RawMessage(serverB),
		},
	})
	require.NoError(t, err)
	assert.Len(t, result.Imported, 2)
	assert.Contains(t, result.Imported, "server-a")
	assert.Contains(t, result.Imported, "server-b")
}

// --- ResolveMCPEnvVars Tests ---

func TestResolveMCPEnvVars_ResolvesSetVars(t *testing.T) {
	t.Setenv("MY_API_KEY", "resolved-value")

	serverData, _ := json.Marshal(map[string]any{
		"command": "npx",
		"env": map[string]string{
			"API_KEY": "${MY_API_KEY}",
			"STATIC":  "plain-value",
		},
	})

	servers := map[string]json.RawMessage{
		"my-server": json.RawMessage(serverData),
	}

	resolved, warnings := commands.ResolveMCPEnvVars(servers)
	assert.Empty(t, warnings)

	var cfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(resolved["my-server"], &cfg))
	assert.Equal(t, "resolved-value", cfg.Env["API_KEY"])
	assert.Equal(t, "plain-value", cfg.Env["STATIC"])
}

func TestResolveMCPEnvVars_WarnsOnUnset(t *testing.T) {
	// Make sure the var doesn't exist.
	os.Unsetenv("MISSING_VAR_XYZZY")

	serverData, _ := json.Marshal(map[string]any{
		"command": "npx",
		"env": map[string]string{
			"MY_KEY": "${MISSING_VAR_XYZZY}",
		},
	})

	servers := map[string]json.RawMessage{
		"my-server": json.RawMessage(serverData),
	}

	resolved, warnings := commands.ResolveMCPEnvVars(servers)
	assert.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "MISSING_VAR_XYZZY")

	// Value should be left as-is.
	var cfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(resolved["my-server"], &cfg))
	assert.Equal(t, "${MISSING_VAR_XYZZY}", cfg.Env["MY_KEY"])
}

func TestResolveMCPEnvVars_NoEnvSection(t *testing.T) {
	serverData, _ := json.Marshal(map[string]any{"command": "npx"})
	servers := map[string]json.RawMessage{"s": json.RawMessage(serverData)}

	resolved, warnings := commands.ResolveMCPEnvVars(servers)
	assert.Empty(t, warnings)
	assert.Equal(t, string(servers["s"]), string(resolved["s"]))
}

func TestResolveMCPEnvVars_EmptyInput(t *testing.T) {
	resolved, warnings := commands.ResolveMCPEnvVars(nil)
	assert.Nil(t, resolved)
	assert.Empty(t, warnings)
}

// --- Config Round-Trip for MCPMeta ---

func TestConfigRoundTrip_MCPMeta(t *testing.T) {
	cfg := config.Config{
		Version: "1.0.0",
		MCP: map[string]json.RawMessage{
			"server-a": json.RawMessage(`{"command":"npx"}`),
		},
		MCPMeta: map[string]config.MCPServerMeta{
			"server-a": {SourceProject: "/home/user/project"},
		},
	}

	data, err := config.Marshal(cfg)
	require.NoError(t, err)

	parsed, err := config.Parse(data)
	require.NoError(t, err)

	assert.Contains(t, parsed.MCPMeta, "server-a")
	assert.Equal(t, "/home/user/project", parsed.MCPMeta["server-a"].SourceProject)

	// MCP should also round-trip.
	assert.Contains(t, parsed.MCP, "server-a")
}

// --- ReadMCPConfigFile / WriteMCPConfigFile Tests ---

func TestReadWriteMCPConfigFile(t *testing.T) {
	dir := t.TempDir()
	mcpPath := filepath.Join(dir, "project", ".mcp.json")

	servers := map[string]json.RawMessage{
		"test-server": json.RawMessage(`{"command":"npx","args":["-y","test"]}`),
	}

	require.NoError(t, claudecode.WriteMCPConfigFile(mcpPath, servers))

	// Verify file was created (including parent dirs).
	_, err := os.Stat(mcpPath)
	require.NoError(t, err)

	// Read it back.
	read, err := claudecode.ReadMCPConfigFile(mcpPath)
	require.NoError(t, err)
	assert.Contains(t, read, "test-server")
}

func TestReadMCPConfigFile_NotFound(t *testing.T) {
	result, err := claudecode.ReadMCPConfigFile("/nonexistent/.mcp.json")
	require.NoError(t, err) // returns empty map, not error
	assert.Empty(t, result)
}

func TestReadMCPConfigFile_DelegatesToExisting(t *testing.T) {
	// Verify that ReadMCPConfig still works via delegation.
	dir := t.TempDir()

	servers := map[string]json.RawMessage{
		"global-server": json.RawMessage(`{"command":"test"}`),
	}
	require.NoError(t, claudecode.WriteMCPConfig(dir, servers))

	read, err := claudecode.ReadMCPConfig(dir)
	require.NoError(t, err)
	assert.Contains(t, read, "global-server")
}

// --- MCPImport Profile Secrets Tests ---

func TestMCPImport_ProfileSecretsReplaced(t *testing.T) {
	syncDir := setupImportEnv(t)

	// Create a profile.
	profilesDir := filepath.Join(syncDir, "profiles")
	os.MkdirAll(profilesDir, 0755)
	emptyProfile, _ := profiles.MarshalProfile(profiles.Profile{})
	os.WriteFile(filepath.Join(profilesDir, "work.yaml"), emptyProfile, 0644)
	git.Add(syncDir, ".")
	git.Commit(syncDir, "add profile")

	serverData, _ := json.Marshal(map[string]any{
		"command": "npx",
		"env": map[string]string{
			"RENDER_API_KEY": "rnd_abc123xyz",
			"APP_NAME":       "my-app",
		},
	})

	result, err := commands.MCPImport(commands.MCPImportOptions{
		SyncDir: syncDir,
		Profile: "work",
		Servers: map[string]json.RawMessage{
			"render": json.RawMessage(serverData),
		},
	})
	require.NoError(t, err)
	assert.Contains(t, result.Imported, "render")

	// Read the profile and verify the secret was replaced.
	profile, err := profiles.ReadProfile(syncDir, "work")
	require.NoError(t, err)

	var cfg struct {
		Env map[string]string `json:"env"`
	}
	require.NoError(t, json.Unmarshal(profile.MCP.Add["render"], &cfg))
	assert.Equal(t, "${RENDER_API_KEY}", cfg.Env["RENDER_API_KEY"], "secret should be replaced with env var ref")
	assert.Equal(t, "my-app", cfg.Env["APP_NAME"], "non-secret value should be untouched")
}

// --- NormalizeMCPPaths / ExpandMCPPaths Tests ---

func TestNormalizeMCPPaths(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home, "HOME must be set for this test")

	serverData, _ := json.Marshal(map[string]any{
		"command": home + "/bin/my-server",
		"args":    []string{"-c", home + "/Work/evvy/.mcp.json"},
		"cwd":     home + "/Work/evvy",
	})

	servers := map[string]json.RawMessage{
		"my-server": json.RawMessage(serverData),
	}

	result := commands.NormalizeMCPPaths(servers)

	var cfg struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
	}
	require.NoError(t, json.Unmarshal(result["my-server"], &cfg))
	assert.Equal(t, "~/bin/my-server", cfg.Command)
	assert.Equal(t, "~/Work/evvy/.mcp.json", cfg.Args[1])
	assert.Equal(t, "~/Work/evvy", cfg.Cwd)
	assert.Equal(t, "-c", cfg.Args[0], "non-path arg should be untouched")
}

func TestNormalizeMCPPaths_LeavesNonHomePaths(t *testing.T) {
	serverData, _ := json.Marshal(map[string]any{
		"command": "/usr/local/bin/npx",
		"args":    []string{"-y", "@context7/mcp"},
	})

	servers := map[string]json.RawMessage{
		"context7": json.RawMessage(serverData),
	}

	result := commands.NormalizeMCPPaths(servers)

	var cfg struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
	}
	require.NoError(t, json.Unmarshal(result["context7"], &cfg))
	assert.Equal(t, "/usr/local/bin/npx", cfg.Command, "non-home path should be untouched")
	assert.Equal(t, []string{"-y", "@context7/mcp"}, cfg.Args)
}

func TestExpandMCPPaths(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home, "HOME must be set for this test")

	serverData, _ := json.Marshal(map[string]any{
		"command": "~/bin/my-server",
		"args":    []string{"-c", "~/Work/evvy/.mcp.json"},
		"cwd":     "~/Work/evvy",
	})

	servers := map[string]json.RawMessage{
		"my-server": json.RawMessage(serverData),
	}

	result := commands.ExpandMCPPaths(servers)

	var cfg struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
	}
	require.NoError(t, json.Unmarshal(result["my-server"], &cfg))
	assert.Equal(t, home+"/bin/my-server", cfg.Command)
	assert.Equal(t, home+"/Work/evvy/.mcp.json", cfg.Args[1])
	assert.Equal(t, home+"/Work/evvy", cfg.Cwd)
}

func TestNormalizeThenExpand_Roundtrip(t *testing.T) {
	home := os.Getenv("HOME")
	require.NotEmpty(t, home, "HOME must be set for this test")

	original, _ := json.Marshal(map[string]any{
		"command": home + "/bin/my-server",
		"args":    []string{"-y", home + "/Work/project/run.sh"},
		"cwd":     home + "/Work/project",
	})

	servers := map[string]json.RawMessage{
		"test": json.RawMessage(original),
	}

	normalized := commands.NormalizeMCPPaths(servers)
	expanded := commands.ExpandMCPPaths(normalized)

	// Parse both original and round-tripped.
	var origCfg, rtCfg struct {
		Command string   `json:"command"`
		Args    []string `json:"args"`
		Cwd     string   `json:"cwd"`
	}
	require.NoError(t, json.Unmarshal(servers["test"], &origCfg))
	require.NoError(t, json.Unmarshal(expanded["test"], &rtCfg))

	assert.Equal(t, origCfg.Command, rtCfg.Command, "command should round-trip")
	assert.Equal(t, origCfg.Args, rtCfg.Args, "args should round-trip")
	assert.Equal(t, origCfg.Cwd, rtCfg.Cwd, "cwd should round-trip")
}
