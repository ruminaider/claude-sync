package git

import (
	"os/exec"
	"strings"
)

// Run executes a git command in the given directory and returns trimmed stdout.
func Run(dir string, args ...string) (string, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		return strings.TrimSpace(string(out)), err
	}
	return strings.TrimSpace(string(out)), nil
}

// IsRepo returns true if dir is a git repository.
func IsRepo(dir string) bool {
	_, err := Run(dir, "rev-parse", "--git-dir")
	return err == nil
}

// IsClean returns true if the working tree has no uncommitted changes.
func IsClean(dir string) (bool, error) {
	out, err := Run(dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	return out == "", nil
}

// RevParse returns the resolved SHA for a ref.
func RevParse(dir, ref string) (string, error) {
	return Run(dir, "rev-parse", ref)
}

// Init initializes a new git repo in dir with local user config
// so commits work regardless of global git configuration.
func Init(dir string) error {
	if _, err := Run(dir, "init"); err != nil {
		return err
	}
	if _, err := Run(dir, "config", "user.name", "claude-sync"); err != nil {
		return err
	}
	_, err := Run(dir, "config", "user.email", "claude-sync@localhost")
	return err
}

// Clone clones src into dst.
func Clone(src, dst string) error {
	cmd := exec.Command("git", "clone", src, dst)
	_, err := cmd.CombinedOutput()
	return err
}

// Add stages a file.
func Add(dir string, paths ...string) error {
	args := append([]string{"add"}, paths...)
	_, err := Run(dir, args...)
	return err
}

// Commit creates a commit with the given message.
func Commit(dir, message string) error {
	_, err := Run(dir, "commit", "-m", message)
	return err
}

// Pull runs git pull --ff-only.
func Pull(dir string) error {
	_, err := Run(dir, "pull", "--ff-only")
	return err
}

// Push runs git push.
func Push(dir string) error {
	_, err := Run(dir, "push")
	return err
}

// PushWithUpstream pushes and sets the upstream tracking branch.
func PushWithUpstream(dir, remote, branch string) error {
	_, err := Run(dir, "push", "-u", remote, branch)
	return err
}

// Fetch runs git fetch --quiet.
func Fetch(dir string) error {
	_, err := Run(dir, "fetch", "--quiet")
	return err
}

// RemoteAdd adds a remote.
func RemoteAdd(dir, name, url string) error {
	_, err := Run(dir, "remote", "add", name, url)
	return err
}

// CurrentBranch returns the name of the current branch.
func CurrentBranch(dir string) (string, error) {
	return Run(dir, "rev-parse", "--abbrev-ref", "HEAD")
}

// HasRemote returns true if the named remote exists.
func HasRemote(dir, name string) bool {
	_, err := Run(dir, "remote", "get-url", name)
	return err == nil
}

// HasUpstream returns true if the current branch has an upstream tracking branch.
func HasUpstream(dir string) bool {
	_, err := Run(dir, "rev-parse", "--abbrev-ref", "@{u}")
	return err == nil
}

// HasUnpushedCommits returns true if there are local commits not yet pushed
// to the remote. Returns true if no upstream is configured (all commits are unpushed).
func HasUnpushedCommits(dir string) bool {
	if !HasUpstream(dir) {
		// No upstream tracking — any local commits count as unpushed.
		out, err := Run(dir, "rev-list", "HEAD")
		return err == nil && out != ""
	}
	out, err := Run(dir, "rev-list", "@{u}..HEAD")
	return err == nil && out != ""
}
