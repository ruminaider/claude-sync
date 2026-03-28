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

func TestRemoteURL(t *testing.T) {
	src := initTestRepo(t)
	os.WriteFile(filepath.Join(src, "test.txt"), []byte("hello"), 0644)
	exec.Command("git", "-C", src, "add", ".").Run()
	exec.Command("git", "-C", src, "commit", "-m", "initial").Run()

	dst := filepath.Join(t.TempDir(), "clone")
	err := git.Clone(src, dst)
	require.NoError(t, err)

	url, err := git.RemoteURL(dst, "origin")
	require.NoError(t, err)
	assert.Equal(t, src, url)
}

func TestRemoteURL_NoRemote(t *testing.T) {
	dir := initTestRepo(t)
	_, err := git.RemoteURL(dir, "nonexistent")
	assert.Error(t, err)
}

// mustExec runs a command and fails the test if it errors.
func mustExec(t *testing.T, name string, args ...string) {
	t.Helper()
	out, err := exec.Command(name, args...).CombinedOutput()
	require.NoError(t, err, "%s %v failed: %s", name, args, string(out))
}

func TestIsLocalPath(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"/home/user/repo", true},
		{"./relative/path", true},
		{"../parent/path", true},
		{"repo-name", true},
		{"git://github.com/org/repo", false},
		{"ssh://github.com/org/repo", false},
		{"https://github.com/org/repo", false},
		{"http://github.com/org/repo", false},
		{"git@github.com:org/repo.git", false},
		{"", true},                              // empty string is not a remote URL
		{"file:///path/to/repo", false},          // file:// scheme is a git transport
		{"deploy@host:repo.git", false},          // SCP-style with non-git username
		{"/home/user@name/repo", true},           // @ in local path, no colon after host
	}
	for _, tt := range tests {
		got := git.IsLocalPath(tt.input)
		assert.Equal(t, tt.want, got, "IsLocalPath(%q)", tt.input)
	}
}

func TestSetRemoteURL(t *testing.T) {
	src := initTestRepo(t)
	os.WriteFile(filepath.Join(src, "test.txt"), []byte("hello"), 0644)
	mustExec(t, "git", "-C", src, "add", ".")
	mustExec(t, "git", "-C", src, "commit", "-m", "initial")

	dst := filepath.Join(t.TempDir(), "clone")
	err := git.Clone(src, dst)
	require.NoError(t, err)

	newURL := "https://github.com/org/repo.git"
	err = git.SetRemoteURL(dst, "origin", newURL)
	require.NoError(t, err)

	got, err := git.RemoteURL(dst, "origin")
	require.NoError(t, err)
	assert.Equal(t, newURL, got)
}

func TestResolveUpstreamURL(t *testing.T) {
	// Create a repo with an origin remote.
	src := initTestRepo(t)
	os.WriteFile(filepath.Join(src, "test.txt"), []byte("hello"), 0644)
	mustExec(t, "git", "-C", src, "add", ".")
	mustExec(t, "git", "-C", src, "commit", "-m", "initial")

	// Clone it so the clone has origin pointing to src.
	clone := filepath.Join(t.TempDir(), "clone")
	err := git.Clone(src, clone)
	require.NoError(t, err)

	got, err := git.ResolveUpstreamURL(clone)
	require.NoError(t, err)
	assert.Equal(t, src, got)
}

func TestResolveUpstreamURL_NoRemote(t *testing.T) {
	dir := initTestRepo(t)
	_, err := git.ResolveUpstreamURL(dir)
	assert.Error(t, err)
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

// initRepoWithUpstream creates a "bare" upstream repo and a clone that tracks it.
// Returns (cloneDir, upstreamDir).
func initRepoWithUpstream(t *testing.T) (string, string) {
	t.Helper()

	// Create a repo to act as the upstream (bare).
	upstream := filepath.Join(t.TempDir(), "upstream.git")
	mustExec(t, "git", "init", "--bare", upstream)

	// Clone it so we get a tracking branch.
	clone := filepath.Join(t.TempDir(), "clone")
	mustExec(t, "git", "clone", upstream, clone)
	mustExec(t, "git", "-C", clone, "config", "user.email", "test@test.com")
	mustExec(t, "git", "-C", clone, "config", "user.name", "Test")

	// Need at least one commit so HEAD exists.
	os.WriteFile(filepath.Join(clone, "init.txt"), []byte("init"), 0644)
	mustExec(t, "git", "-C", clone, "add", ".")
	mustExec(t, "git", "-C", clone, "commit", "-m", "initial")
	mustExec(t, "git", "-C", clone, "push")

	return clone, upstream
}

func TestCommitsBehind(t *testing.T) {
	t.Run("non-git directory", func(t *testing.T) {
		dir := t.TempDir()
		assert.Equal(t, -1, git.CommitsBehind(dir))
	})

	t.Run("at same commit as upstream", func(t *testing.T) {
		clone, _ := initRepoWithUpstream(t)
		assert.Equal(t, 0, git.CommitsBehind(clone))
	})

	t.Run("behind upstream", func(t *testing.T) {
		clone, upstream := initRepoWithUpstream(t)

		// Push two more commits from a second clone so upstream moves ahead.
		second := filepath.Join(t.TempDir(), "second")
		mustExec(t, "git", "clone", upstream, second)
		mustExec(t, "git", "-C", second, "config", "user.email", "test@test.com")
		mustExec(t, "git", "-C", second, "config", "user.name", "Test")

		os.WriteFile(filepath.Join(second, "a.txt"), []byte("a"), 0644)
		mustExec(t, "git", "-C", second, "add", ".")
		mustExec(t, "git", "-C", second, "commit", "-m", "commit a")

		os.WriteFile(filepath.Join(second, "b.txt"), []byte("b"), 0644)
		mustExec(t, "git", "-C", second, "add", ".")
		mustExec(t, "git", "-C", second, "commit", "-m", "commit b")
		mustExec(t, "git", "-C", second, "push")

		// Fetch in original clone so it sees the new upstream commits.
		mustExec(t, "git", "-C", clone, "fetch")

		assert.Equal(t, 2, git.CommitsBehind(clone))
	})
}

func TestCommitsAhead(t *testing.T) {
	t.Run("non-git directory", func(t *testing.T) {
		dir := t.TempDir()
		assert.Equal(t, -1, git.CommitsAhead(dir))
	})

	t.Run("at same commit as upstream", func(t *testing.T) {
		clone, _ := initRepoWithUpstream(t)
		assert.Equal(t, 0, git.CommitsAhead(clone))
	})

	t.Run("ahead of upstream", func(t *testing.T) {
		clone, _ := initRepoWithUpstream(t)

		// Create local commits without pushing.
		os.WriteFile(filepath.Join(clone, "local1.txt"), []byte("local1"), 0644)
		mustExec(t, "git", "-C", clone, "add", ".")
		mustExec(t, "git", "-C", clone, "commit", "-m", "local commit 1")

		os.WriteFile(filepath.Join(clone, "local2.txt"), []byte("local2"), 0644)
		mustExec(t, "git", "-C", clone, "add", ".")
		mustExec(t, "git", "-C", clone, "commit", "-m", "local commit 2")

		os.WriteFile(filepath.Join(clone, "local3.txt"), []byte("local3"), 0644)
		mustExec(t, "git", "-C", clone, "add", ".")
		mustExec(t, "git", "-C", clone, "commit", "-m", "local commit 3")

		assert.Equal(t, 3, git.CommitsAhead(clone))
	})
}

func TestDiffNameOnly(t *testing.T) {
	t.Run("invalid refs", func(t *testing.T) {
		dir := initTestRepo(t)
		_, err := git.DiffNameOnly(dir, "nonexistent-ref-a", "nonexistent-ref-b")
		assert.Error(t, err)
	})

	t.Run("identical refs", func(t *testing.T) {
		dir := initTestRepo(t)
		os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
		mustExec(t, "git", "-C", dir, "add", ".")
		mustExec(t, "git", "-C", dir, "commit", "-m", "initial")

		files, err := git.DiffNameOnly(dir, "HEAD", "HEAD")
		require.NoError(t, err)
		assert.Nil(t, files)
	})

	t.Run("known diff", func(t *testing.T) {
		dir := initTestRepo(t)
		os.WriteFile(filepath.Join(dir, "test.txt"), []byte("hello"), 0644)
		mustExec(t, "git", "-C", dir, "add", ".")
		mustExec(t, "git", "-C", dir, "commit", "-m", "initial")

		sha1, err := git.RevParse(dir, "HEAD")
		require.NoError(t, err)

		os.WriteFile(filepath.Join(dir, "added.txt"), []byte("new file"), 0644)
		os.WriteFile(filepath.Join(dir, "test.txt"), []byte("modified"), 0644)
		mustExec(t, "git", "-C", dir, "add", ".")
		mustExec(t, "git", "-C", dir, "commit", "-m", "second")

		sha2, err := git.RevParse(dir, "HEAD")
		require.NoError(t, err)

		files, err := git.DiffNameOnly(dir, sha1, sha2)
		require.NoError(t, err)
		assert.ElementsMatch(t, []string{"added.txt", "test.txt"}, files)
	})

	t.Run("filters empty strings", func(t *testing.T) {
		// Verify that the implementation does not include empty strings.
		// When git output has trailing newlines, Split produces empty entries.
		// The function should filter them out. We verify this indirectly by
		// checking that a real diff produces no empty entries.
		dir := initTestRepo(t)
		os.WriteFile(filepath.Join(dir, "a.txt"), []byte("a"), 0644)
		mustExec(t, "git", "-C", dir, "add", ".")
		mustExec(t, "git", "-C", dir, "commit", "-m", "first")

		sha1, err := git.RevParse(dir, "HEAD")
		require.NoError(t, err)

		os.WriteFile(filepath.Join(dir, "b.txt"), []byte("b"), 0644)
		mustExec(t, "git", "-C", dir, "add", ".")
		mustExec(t, "git", "-C", dir, "commit", "-m", "second")

		sha2, err := git.RevParse(dir, "HEAD")
		require.NoError(t, err)

		files, err := git.DiffNameOnly(dir, sha1, sha2)
		require.NoError(t, err)
		for _, f := range files {
			assert.NotEmpty(t, f, "DiffNameOnly should not return empty strings")
		}
	})
}

func TestMergeFFOnly(t *testing.T) {
	t.Run("fast-forward succeeds", func(t *testing.T) {
		clone, upstream := initRepoWithUpstream(t)

		// Push a commit from a second clone so upstream moves ahead.
		second := filepath.Join(t.TempDir(), "second")
		mustExec(t, "git", "clone", upstream, second)
		mustExec(t, "git", "-C", second, "config", "user.email", "test@test.com")
		mustExec(t, "git", "-C", second, "config", "user.name", "Test")

		os.WriteFile(filepath.Join(second, "new.txt"), []byte("new"), 0644)
		mustExec(t, "git", "-C", second, "add", ".")
		mustExec(t, "git", "-C", second, "commit", "-m", "new commit")
		mustExec(t, "git", "-C", second, "push")

		// Fetch in original clone, then fast-forward merge.
		mustExec(t, "git", "-C", clone, "fetch")

		err := git.MergeFFOnly(clone)
		require.NoError(t, err)

		// Verify the file from the upstream commit is now present.
		data, err := os.ReadFile(filepath.Join(clone, "new.txt"))
		require.NoError(t, err)
		assert.Equal(t, "new", string(data))
	})

	t.Run("non-fast-forward fails", func(t *testing.T) {
		clone, upstream := initRepoWithUpstream(t)

		// Push a commit from a second clone so upstream diverges.
		second := filepath.Join(t.TempDir(), "second")
		mustExec(t, "git", "clone", upstream, second)
		mustExec(t, "git", "-C", second, "config", "user.email", "test@test.com")
		mustExec(t, "git", "-C", second, "config", "user.name", "Test")

		os.WriteFile(filepath.Join(second, "upstream.txt"), []byte("upstream"), 0644)
		mustExec(t, "git", "-C", second, "add", ".")
		mustExec(t, "git", "-C", second, "commit", "-m", "upstream commit")
		mustExec(t, "git", "-C", second, "push")

		// Create a local divergent commit in original clone.
		os.WriteFile(filepath.Join(clone, "local.txt"), []byte("local"), 0644)
		mustExec(t, "git", "-C", clone, "add", ".")
		mustExec(t, "git", "-C", clone, "commit", "-m", "local commit")

		// Fetch so clone sees the upstream commit, creating divergence.
		mustExec(t, "git", "-C", clone, "fetch")

		err := git.MergeFFOnly(clone)
		assert.Error(t, err)
	})
}
