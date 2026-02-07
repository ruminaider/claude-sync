package commands_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJoin(t *testing.T) {
	remote := t.TempDir()
	exec.Command("git", "init", remote).Run()
	exec.Command("git", "-C", remote, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", remote, "config", "user.name", "Test").Run()

	cfgContent := "version: \"1.0.0\"\nplugins:\n  - context7@claude-plugins-official\n"
	os.WriteFile(filepath.Join(remote, "config.yaml"), []byte(cfgContent), 0644)
	exec.Command("git", "-C", remote, "add", ".").Run()
	exec.Command("git", "-C", remote, "commit", "-m", "init").Run()

	claudeDir := t.TempDir()
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	_, err = os.Stat(filepath.Join(syncDir, "config.yaml"))
	assert.NoError(t, err)
}

func TestJoin_AlreadyExists(t *testing.T) {
	syncDir := t.TempDir()
	err := commands.Join("http://example.com/repo.git", "/tmp/claude", syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "already")
}

func TestJoin_BootstrapsClaudeDir(t *testing.T) {
	remote := t.TempDir()
	exec.Command("git", "init", remote).Run()
	exec.Command("git", "-C", remote, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", remote, "config", "user.name", "Test").Run()
	os.WriteFile(filepath.Join(remote, "config.yaml"), []byte("version: \"1.0.0\"\nplugins: []\n"), 0644)
	exec.Command("git", "-C", remote, "add", ".").Run()
	exec.Command("git", "-C", remote, "commit", "-m", "init").Run()

	claudeDir := filepath.Join(t.TempDir(), ".claude")
	syncDir := filepath.Join(t.TempDir(), ".claude-sync")

	err := commands.Join(remote, claudeDir, syncDir)
	require.NoError(t, err)

	// Claude dir should have been bootstrapped
	_, err = os.Stat(filepath.Join(claudeDir, "plugins", "installed_plugins.json"))
	assert.NoError(t, err)
}
