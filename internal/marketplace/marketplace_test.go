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

// ─── QueryRemoteVersion (skipped by default — requires network) ────────────

func TestQueryRemoteVersion_InvalidURL(t *testing.T) {
	// Verify that an invalid URL produces a clear error.
	_, err := marketplace.QueryRemoteVersion("https://invalid.example.com/no-such-repo.git", "test-plugin")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "git ls-remote")
}
