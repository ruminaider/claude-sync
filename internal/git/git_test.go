package git_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/git"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func initTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init")
	cmd.Dir = dir
	require.NoError(t, cmd.Run())
	exec.Command("git", "-C", dir, "config", "user.email", "test@test.com").Run()
	exec.Command("git", "-C", dir, "config", "user.name", "Test").Run()
	return dir
}

func TestRun(t *testing.T) {
	dir := initTestRepo(t)
	out, err := git.Run(dir, "status", "--porcelain")
	require.NoError(t, err)
	assert.Empty(t, out)
}

func TestIsRepo(t *testing.T) {
	t.Run("valid repo", func(t *testing.T) {
		dir := initTestRepo(t)
		assert.True(t, git.IsRepo(dir))
	})
	t.Run("not a repo", func(t *testing.T) {
		dir := t.TempDir()
		assert.False(t, git.IsRepo(dir))
	})
	t.Run("nonexistent dir", func(t *testing.T) {
		assert.False(t, git.IsRepo("/nonexistent/path"))
	})
}

func TestIsClean(t *testing.T) {
	dir := initTestRepo(t)
	t.Run("clean repo", func(t *testing.T) {
		clean, err := git.IsClean(dir)
		require.NoError(t, err)
		assert.True(t, clean)
	})
	t.Run("dirty repo", func(t *testing.T) {
		os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
		clean, err := git.IsClean(dir)
		require.NoError(t, err)
		assert.False(t, clean)
	})
}

func TestRevParse(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
	exec.Command("git", "-C", dir, "add", ".").Run()
	exec.Command("git", "-C", dir, "commit", "-m", "initial").Run()

	sha, err := git.RevParse(dir, "HEAD")
	require.NoError(t, err)
	assert.Len(t, sha, 40)
}

func TestInit(t *testing.T) {
	dir := t.TempDir()
	err := git.Init(dir)
	require.NoError(t, err)
	assert.True(t, git.IsRepo(dir))
}

func TestClone(t *testing.T) {
	src := initTestRepo(t)
	os.WriteFile(filepath.Join(src, "test.txt"), []byte("hello"), 0644)
	exec.Command("git", "-C", src, "add", ".").Run()
	exec.Command("git", "-C", src, "commit", "-m", "initial").Run()

	dst := filepath.Join(t.TempDir(), "clone")
	err := git.Clone(src, dst)
	require.NoError(t, err)
	assert.True(t, git.IsRepo(dst))

	data, err := os.ReadFile(filepath.Join(dst, "test.txt"))
	require.NoError(t, err)
	assert.Equal(t, "hello", string(data))
}

func TestAddAndCommit(t *testing.T) {
	dir := initTestRepo(t)
	os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)

	err := git.Add(dir, "test.txt")
	require.NoError(t, err)

	err = git.Commit(dir, "test commit")
	require.NoError(t, err)

	clean, _ := git.IsClean(dir)
	assert.True(t, clean)
}
