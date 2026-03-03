package plugins_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCheckReEvaluation_StaleLocalRepo(t *testing.T) {
	localRepo := t.TempDir()
	initGitRepo(t, localRepo)
	commitInRepo(t, localRepo, "initial", time.Now().AddDate(0, 0, -10))

	sources := plugins.PluginSources{
		Plugins: map[string]plugins.PluginSourceEntry{
			"bash-validator": {
				ActiveSource:                "claude-sync-forks",
				Suppressed:                  "bash-validator-marketplace",
				Relationship:                "active-dev",
				LocalRepo:                   localRepo,
				DecidedAt:                   time.Now().AddDate(0, 0, -14),
				MarketplaceVersionAtDecision: "1.0.0",
				LastLocalCommitAtDecision:   time.Now().AddDate(0, 0, -14),
			},
		},
	}

	signals := plugins.CheckReEvaluation(sources, 7)
	require.Len(t, signals, 1)
	assert.Equal(t, "bash-validator", signals[0].PluginName)
	assert.True(t, signals[0].LocalStale)
}

func TestCheckReEvaluation_ActiveRepo_NoSignal(t *testing.T) {
	localRepo := t.TempDir()
	initGitRepo(t, localRepo)
	commitInRepo(t, localRepo, "recent work", time.Now())

	sources := plugins.PluginSources{
		Plugins: map[string]plugins.PluginSourceEntry{
			"bash-validator": {
				ActiveSource:                "claude-sync-forks",
				Suppressed:                  "bash-validator-marketplace",
				Relationship:                "active-dev",
				LocalRepo:                   localRepo,
				DecidedAt:                   time.Now().AddDate(0, 0, -3),
				MarketplaceVersionAtDecision: "1.0.0",
				LastLocalCommitAtDecision:   time.Now().AddDate(0, 0, -3),
			},
		},
	}

	signals := plugins.CheckReEvaluation(sources, 7)
	assert.Empty(t, signals)
}

func TestCheckReEvaluation_Snoozed_NoSignal(t *testing.T) {
	localRepo := t.TempDir()
	initGitRepo(t, localRepo)
	commitInRepo(t, localRepo, "old", time.Now().AddDate(0, 0, -10))

	snoozeUntil := time.Now().Add(24 * time.Hour)
	sources := plugins.PluginSources{
		Plugins: map[string]plugins.PluginSourceEntry{
			"bash-validator": {
				ActiveSource: "forks",
				Suppressed:   "marketplace",
				Relationship: "active-dev",
				LocalRepo:    localRepo,
				SnoozeUntil:  &snoozeUntil,
			},
		},
	}

	signals := plugins.CheckReEvaluation(sources, 7)
	assert.Empty(t, signals)
}

func TestCheckReEvaluation_RepoDeleted(t *testing.T) {
	sources := plugins.PluginSources{
		Plugins: map[string]plugins.PluginSourceEntry{
			"bash-validator": {
				ActiveSource: "forks",
				Suppressed:   "marketplace",
				Relationship: "active-dev",
				LocalRepo:    "/nonexistent/path/gone",
			},
		},
	}

	signals := plugins.CheckReEvaluation(sources, 7)
	require.Len(t, signals, 1)
	assert.True(t, signals[0].RepoMissing)
}

func initGitRepo(t *testing.T, dir string) {
	t.Helper()
	run(t, dir, "git", "init")
	run(t, dir, "git", "config", "user.email", "test@test.com")
	run(t, dir, "git", "config", "user.name", "Test")
}

func commitInRepo(t *testing.T, dir, msg string, date time.Time) {
	t.Helper()
	f := filepath.Join(dir, "file.txt")
	os.WriteFile(f, []byte(msg), 0644)
	run(t, dir, "git", "add", ".")
	cmd := exec.Command("git", "commit", "-m", msg, "--date", date.Format(time.RFC3339))
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_COMMITTER_DATE="+date.Format(time.RFC3339),
	)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}

func run(t *testing.T, dir string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, string(out))
}
