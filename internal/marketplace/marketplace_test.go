package marketplace_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/marketplace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// initTestRepo creates a bare git repo with user config and an initial commit.
func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	run("init")
	run("config", "user.email", "test@test.com")
	run("config", "user.name", "Test")

	return dir
}

// addPlugin writes a plugin.json inside pluginName/ and commits it.
func addPlugin(t *testing.T, repoPath, pluginName, version string) {
	t.Helper()

	pluginDir := filepath.Join(repoPath, pluginName)
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	pj := map[string]string{
		"name":    pluginName,
		"version": version,
	}
	data, err := json.MarshalIndent(pj, "", "  ")
	require.NoError(t, err)

	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "plugin.json"), data, 0644))

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = repoPath
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, "git %v failed: %s", args, string(out))
	}

	run("add", pluginName)
	run("commit", "-m", "add "+pluginName+" v"+version)
}

// ─── IsPortableMarketplace ─────────────────────────────────────────────────

func TestIsPortableMarketplace(t *testing.T) {
	tests := []struct {
		id   string
		want bool
	}{
		{"claude-plugins-official", true},
		{"superpowers-marketplace", true},
		{"beads-marketplace", true},
		{"claude-sync-marketplace", true},
		{"local-custom-plugins", false},
		{"figma-minimal-marketplace", false},
		{"context-anchor", false},
		{"myorg/my-marketplace", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			got := marketplace.IsPortableMarketplace(tt.id)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ─── IsPortableFromKnownMarketplaces ───────────────────────────────────────

func TestIsPortableFromKnownMarketplaces(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	km := `{
		"context-anchor": {"source": {"source": "github", "repo": "ruminaider/context-anchor"}},
		"every-marketplace": {"source": {"source": "git", "url": "https://github.com/EveryInc/compound.git"}},
		"claude-sync-forks": {"source": {"source": "directory", "path": "/home/user/.claude-sync/plugins"}}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte(km), 0644))

	tests := []struct {
		id         string
		wantPortable bool
		wantFound    bool
	}{
		{"context-anchor", true, true},
		{"every-marketplace", true, true},
		{"claude-sync-forks", false, true},
		{"not-in-file", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			portable, found := marketplace.IsPortableFromKnownMarketplaces(claudeDir, tt.id)
			assert.Equal(t, tt.wantFound, found, "found")
			assert.Equal(t, tt.wantPortable, portable, "portable")
		})
	}

	t.Run("missing file returns not found", func(t *testing.T) {
		portable, found := marketplace.IsPortableFromKnownMarketplaces("/nonexistent", "anything")
		assert.False(t, found)
		assert.False(t, portable)
	})

	t.Run("invalid JSON returns not found", func(t *testing.T) {
		badDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(badDir, "plugins"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(badDir, "plugins", "known_marketplaces.json"), []byte("{bad json"), 0644))
		portable, found := marketplace.IsPortableFromKnownMarketplaces(badDir, "anything")
		assert.False(t, found)
		assert.False(t, portable)
	})
}

// ─── IsPortable (combined check) ──────────────────────────────────────────

func TestIsPortable(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	km := `{
		"context-anchor": {"source": {"source": "github", "repo": "ruminaider/context-anchor"}},
		"figma-minimal-marketplace": {"source": {"source": "directory", "path": "/some/path"}}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte(km), 0644))

	t.Run("found in JSON as github → portable", func(t *testing.T) {
		assert.True(t, marketplace.IsPortable(claudeDir, "context-anchor"))
	})

	t.Run("found in JSON as directory → not portable", func(t *testing.T) {
		assert.False(t, marketplace.IsPortable(claudeDir, "figma-minimal-marketplace"))
	})

	t.Run("not in JSON, falls back to hardcoded → portable", func(t *testing.T) {
		assert.True(t, marketplace.IsPortable(claudeDir, "claude-plugins-official"))
	})

	t.Run("not in JSON, not in hardcoded → not portable", func(t *testing.T) {
		assert.False(t, marketplace.IsPortable(claudeDir, "unknown-custom"))
	})

	t.Run("JSON overrides hardcoded if present", func(t *testing.T) {
		// Write a known_marketplaces.json that marks a hardcoded marketplace as directory
		overrideDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(overrideDir, "plugins"), 0755))
		km := `{"beads-marketplace": {"source": {"source": "directory", "path": "/local"}}}`
		require.NoError(t, os.WriteFile(filepath.Join(overrideDir, "plugins", "known_marketplaces.json"), []byte(km), 0644))
		// beads-marketplace is hardcoded as portable, but JSON says directory → not portable
		assert.False(t, marketplace.IsPortable(overrideDir, "beads-marketplace"))
	})
}

// ─── ParseMarketplaceSource ────────────────────────────────────────────────

func TestParseMarketplaceSource(t *testing.T) {
	tests := []struct {
		input    string
		wantOrg  string
		wantRepo string
	}{
		{
			input:    "claude-plugins-official",
			wantOrg:  "anthropics",
			wantRepo: "claude-plugins-official",
		},
		{
			input:    "superpowers-marketplace",
			wantOrg:  "anthropics",
			wantRepo: "superpowers-marketplace",
		},
		{
			input:    "beads-marketplace",
			wantOrg:  "anthropics",
			wantRepo: "beads-marketplace",
		},
		{
			input:    "claude-sync-marketplace",
			wantOrg:  "ruminaider",
			wantRepo: "claude-sync-marketplace",
		},
		{
			input:    "myorg/my-marketplace",
			wantOrg:  "myorg",
			wantRepo: "my-marketplace",
		},
		{
			input:    "acme-corp/internal-plugins",
			wantOrg:  "acme-corp",
			wantRepo: "internal-plugins",
		},
		{
			input:    "unknown-marketplace",
			wantOrg:  "unknown-marketplace",
			wantRepo: "unknown-marketplace",
		},
		{
			input:    "solo",
			wantOrg:  "solo",
			wantRepo: "solo",
		},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			org, repo := marketplace.ParseMarketplaceSource(tt.input)
			assert.Equal(t, tt.wantOrg, org, "org")
			assert.Equal(t, tt.wantRepo, repo, "repo")
		})
	}
}

// ─── QueryPluginVersion (local repo) ───────────────────────────────────────

func TestQueryPluginVersion_LocalRepo(t *testing.T) {
	repoPath := initTestRepo(t)
	addPlugin(t, repoPath, "context7", "1.2.3")

	t.Run("reads version and commit SHA", func(t *testing.T) {
		info, err := marketplace.QueryPluginVersion(repoPath, "context7")
		require.NoError(t, err)

		assert.Equal(t, "context7", info.Name)
		assert.Equal(t, "1.2.3", info.Version)
		assert.Len(t, info.CommitSHA, 40, "commit SHA should be 40 hex characters")
	})

	t.Run("tracks per-plugin commits", func(t *testing.T) {
		// Add a second plugin; context7's SHA should not change.
		info1, err := marketplace.QueryPluginVersion(repoPath, "context7")
		require.NoError(t, err)

		addPlugin(t, repoPath, "playwright", "0.5.0")

		info1After, err := marketplace.QueryPluginVersion(repoPath, "context7")
		require.NoError(t, err)

		info2, err := marketplace.QueryPluginVersion(repoPath, "playwright")
		require.NoError(t, err)

		assert.Equal(t, info1.CommitSHA, info1After.CommitSHA,
			"context7 SHA should be unchanged after adding playwright")
		assert.NotEqual(t, info1.CommitSHA, info2.CommitSHA,
			"different plugins should have different commit SHAs")
		assert.Equal(t, "playwright", info2.Name)
		assert.Equal(t, "0.5.0", info2.Version)
	})

	t.Run("SHA updates when plugin is modified", func(t *testing.T) {
		infoBefore, err := marketplace.QueryPluginVersion(repoPath, "context7")
		require.NoError(t, err)

		// Update the plugin version.
		addPlugin(t, repoPath, "context7", "1.3.0")

		infoAfter, err := marketplace.QueryPluginVersion(repoPath, "context7")
		require.NoError(t, err)

		assert.NotEqual(t, infoBefore.CommitSHA, infoAfter.CommitSHA,
			"SHA should change after modifying plugin")
		assert.Equal(t, "1.3.0", infoAfter.Version)
	})

	t.Run("nonexistent plugin returns error", func(t *testing.T) {
		_, err := marketplace.QueryPluginVersion(repoPath, "nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "nonexistent")
	})
}

// ─── HasUpdate (version comparison) ────────────────────────────────────────

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name    string
		current string
		latest  string
		want    bool
	}{
		// Same versions — no update.
		{name: "same semver", current: "1.0.0", latest: "1.0.0", want: false},
		{name: "same SHA", current: "abc123def456", latest: "abc123def456", want: false},

		// Different versions — update available.
		{name: "patch update", current: "1.0.0", latest: "1.0.1", want: true},
		{name: "minor update", current: "1.0.0", latest: "1.1.0", want: true},
		{name: "major update", current: "1.0.0", latest: "2.0.0", want: true},
		{name: "different SHAs", current: "abc123", latest: "def456", want: true},
		{name: "semver to SHA", current: "1.0.0", latest: "abc123def456", want: true},
		{name: "custom tag update", current: "2.22.0-custom", latest: "2.25.0", want: true},

		// Edge cases.
		{name: "empty current", current: "", latest: "1.0.0", want: false},
		{name: "empty latest", current: "1.0.0", latest: "", want: false},
		{name: "both empty", current: "", latest: "", want: false},
		{name: "whitespace trimmed", current: " 1.0.0 ", latest: "1.0.0", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := marketplace.HasUpdate(tt.current, tt.latest)
			assert.Equal(t, tt.want, got)
		})
	}
}

// ─── ReadMarketplacePluginVersion ──────────────────────────────────────────

// setupMarketplaceEnv creates a claudeDir with known_marketplaces.json pointing
// to a marketplace directory containing a valid marketplace.json and plugin.json.
func setupMarketplaceEnv(t *testing.T, pluginName, version string) (claudeDir, marketplaceDir string) {
	t.Helper()

	claudeDir = t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	marketplaceDir = t.TempDir()

	// Write known_marketplaces.json.
	km := map[string]any{
		"test-marketplace": map[string]any{
			"source":          map[string]string{"source": "directory", "path": marketplaceDir},
			"installLocation": marketplaceDir,
		},
	}
	kmData, err := json.MarshalIndent(km, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), kmData, 0644))

	// Write marketplace.json.
	mkplDir := filepath.Join(marketplaceDir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(mkplDir, 0755))
	mkpl := map[string]any{
		"name": "test-marketplace",
		"plugins": []map[string]string{
			{"name": pluginName, "source": "./", "version": version},
		},
	}
	mkplData, err := json.MarshalIndent(mkpl, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mkplDir, "marketplace.json"), mkplData, 0644))

	// Write plugin.json at the source path.
	pj := map[string]string{"name": pluginName, "version": version}
	pjData, err := json.MarshalIndent(pj, "", "  ")
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(mkplDir, "plugin.json"), pjData, 0644))

	return claudeDir, marketplaceDir
}

func TestReadMarketplacePluginVersion_SinglePlugin(t *testing.T) {
	claudeDir, _ := setupMarketplaceEnv(t, "my-plugin", "1.2.3")

	ver, err := marketplace.ReadMarketplacePluginVersion(claudeDir, "my-plugin@test-marketplace")
	require.NoError(t, err)
	assert.Equal(t, "1.2.3", ver)
}

func TestReadMarketplacePluginVersion_MultiPlugin(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	marketplaceDir := t.TempDir()

	km := map[string]any{
		"multi-marketplace": map[string]any{
			"source":          map[string]string{"source": "directory", "path": marketplaceDir},
			"installLocation": marketplaceDir,
		},
	}
	kmData, _ := json.MarshalIndent(km, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), kmData, 0644))

	// Write marketplace.json with multiple plugins.
	mkplDir := filepath.Join(marketplaceDir, ".claude-plugin")
	require.NoError(t, os.MkdirAll(mkplDir, 0755))
	mkpl := map[string]any{
		"name": "multi-marketplace",
		"plugins": []map[string]string{
			{"name": "plugin-a", "source": "./plugins/plugin-a", "version": "1.0.0"},
			{"name": "plugin-b", "source": "./plugins/plugin-b", "version": "2.0.0"},
		},
	}
	mkplData, _ := json.MarshalIndent(mkpl, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(mkplDir, "marketplace.json"), mkplData, 0644))

	// Write plugin.json for plugin-b with a different version than marketplace.json.
	pbDir := filepath.Join(marketplaceDir, "plugins", "plugin-b", ".claude-plugin")
	require.NoError(t, os.MkdirAll(pbDir, 0755))
	pj := map[string]string{"name": "plugin-b", "version": "2.1.0"}
	pjData, _ := json.MarshalIndent(pj, "", "  ")
	require.NoError(t, os.WriteFile(filepath.Join(pbDir, "plugin.json"), pjData, 0644))

	t.Run("falls back to marketplace.json version", func(t *testing.T) {
		// plugin-a has no plugin.json on disk, so it uses marketplace.json version.
		ver, err := marketplace.ReadMarketplacePluginVersion(claudeDir, "plugin-a@multi-marketplace")
		require.NoError(t, err)
		assert.Equal(t, "1.0.0", ver)
	})

	t.Run("prefers plugin.json version", func(t *testing.T) {
		// plugin-b has a plugin.json with version 2.1.0, overriding marketplace.json's 2.0.0.
		ver, err := marketplace.ReadMarketplacePluginVersion(claudeDir, "plugin-b@multi-marketplace")
		require.NoError(t, err)
		assert.Equal(t, "2.1.0", ver)
	})
}

func TestReadMarketplacePluginVersion_Errors(t *testing.T) {
	t.Run("invalid plugin key", func(t *testing.T) {
		_, err := marketplace.ReadMarketplacePluginVersion("/tmp", "no-at-sign")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "invalid plugin key")
	})

	t.Run("missing known_marketplaces.json", func(t *testing.T) {
		_, err := marketplace.ReadMarketplacePluginVersion("/nonexistent", "foo@bar")
		assert.Error(t, err)
	})

	t.Run("marketplace not found", func(t *testing.T) {
		claudeDir := t.TempDir()
		require.NoError(t, os.MkdirAll(filepath.Join(claudeDir, "plugins"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(claudeDir, "plugins", "known_marketplaces.json"), []byte("{}"), 0644))

		_, err := marketplace.ReadMarketplacePluginVersion(claudeDir, "foo@nonexistent")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})

	t.Run("plugin not in marketplace", func(t *testing.T) {
		claudeDir, _ := setupMarketplaceEnv(t, "my-plugin", "1.0.0")

		_, err := marketplace.ReadMarketplacePluginVersion(claudeDir, "other-plugin@test-marketplace")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

// ─── MarketplaceSourceType ─────────────────────────────────────────────────

func TestMarketplaceSourceType(t *testing.T) {
	claudeDir := t.TempDir()
	pluginDir := filepath.Join(claudeDir, "plugins")
	require.NoError(t, os.MkdirAll(pluginDir, 0755))

	km := `{
		"dir-marketplace": {"source": {"source": "directory", "path": "/some/path"}, "installLocation": "/some/path"},
		"gh-marketplace": {"source": {"source": "github", "repo": "org/repo"}, "installLocation": "/cache/gh"}
	}`
	require.NoError(t, os.WriteFile(filepath.Join(pluginDir, "known_marketplaces.json"), []byte(km), 0644))

	assert.Equal(t, "directory", marketplace.MarketplaceSourceType(claudeDir, "dir-marketplace"))
	assert.Equal(t, "github", marketplace.MarketplaceSourceType(claudeDir, "gh-marketplace"))
	assert.Equal(t, "", marketplace.MarketplaceSourceType(claudeDir, "missing"))
	assert.Equal(t, "", marketplace.MarketplaceSourceType("/nonexistent", "anything"))
}

// ─── ResolvePluginSourceDir ────────────────────────────────────────────────

func TestResolvePluginSourceDir(t *testing.T) {
	claudeDir, marketplaceDir := setupMarketplaceEnv(t, "my-plugin", "1.0.0")

	t.Run("single plugin marketplace", func(t *testing.T) {
		dir, err := marketplace.ResolvePluginSourceDir(claudeDir, "my-plugin@test-marketplace")
		require.NoError(t, err)
		assert.Equal(t, filepath.Join(marketplaceDir, "."), dir)
	})

	t.Run("invalid key", func(t *testing.T) {
		_, err := marketplace.ResolvePluginSourceDir(claudeDir, "no-at-sign")
		assert.Error(t, err)
	})

	t.Run("missing plugin", func(t *testing.T) {
		_, err := marketplace.ResolvePluginSourceDir(claudeDir, "nonexistent@test-marketplace")
		assert.Error(t, err)
	})
}

// ─── ComputePluginContentHash ──────────────────────────────────────────────

func TestComputePluginContentHash(t *testing.T) {
	t.Run("deterministic", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0644))

		hash1, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)
		hash2, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		assert.Equal(t, hash1, hash2)
		assert.Len(t, hash1, 16, "hash should be 16 hex chars")
	})

	t.Run("detects content change", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("hello"), 0644))

		hashBefore, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "file.txt"), []byte("world"), 0644))

		hashAfter, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		assert.NotEqual(t, hashBefore, hashAfter)
	})

	t.Run("detects file addition", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644))

		hashBefore, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		require.NoError(t, os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644))

		hashAfter, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		assert.NotEqual(t, hashBefore, hashAfter)
	})

	t.Run("excludes .git and .DS_Store", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "code.py"), []byte("print('hi')"), 0644))

		hashBefore, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		// Add .git dir and .DS_Store — should not change hash.
		require.NoError(t, os.MkdirAll(filepath.Join(dir, ".git", "objects"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".git", "HEAD"), []byte("ref: refs/heads/main"), 0644))
		require.NoError(t, os.WriteFile(filepath.Join(dir, ".DS_Store"), []byte("binary junk"), 0644))

		hashAfter, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		assert.Equal(t, hashBefore, hashAfter)
	})

	t.Run("excludes node_modules", func(t *testing.T) {
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "index.js"), []byte("module.exports = {}"), 0644))

		hashBefore, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		require.NoError(t, os.MkdirAll(filepath.Join(dir, "node_modules", "dep"), 0755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "node_modules", "dep", "index.js"), []byte("exports.x = 1"), 0644))

		hashAfter, err := marketplace.ComputePluginContentHash(dir)
		require.NoError(t, err)

		assert.Equal(t, hashBefore, hashAfter)
	})
}

// ─── QueryRemoteVersion (skipped by default — requires network) ────────────

func TestQueryRemoteVersion_InvalidURL(t *testing.T) {
	// Verify that an invalid URL produces a clear error.
	_, err := marketplace.QueryRemoteVersion("https://invalid.example.com/no-such-repo.git", "test-plugin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git ls-remote")
}
