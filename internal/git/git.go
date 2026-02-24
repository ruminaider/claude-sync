package git

import (
	"fmt"
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
	if _, err := Run(dir, "init", "-b", "main"); err != nil {
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

// NonFastForwardError is returned when a push is rejected because the remote
// has commits not in the local repo.
type NonFastForwardError struct {
	Output string
}

func (e *NonFastForwardError) Error() string {
	return e.Output
}

// Push runs git push. Returns NonFastForwardError if rejected due to diverged history.
func Push(dir string) error {
	out, err := Run(dir, "push")
	if err != nil {
		if isNonFastForward(out) {
			return &NonFastForwardError{Output: out}
		}
		return fmt.Errorf("%s", out)
	}
	return nil
}

// ForcePush runs git push --force-with-lease.
func ForcePush(dir string) error {
	out, err := Run(dir, "push", "--force-with-lease")
	if err != nil {
		return fmt.Errorf("%s", out)
	}
	return nil
}

// PushWithUpstream pushes and sets the upstream tracking branch.
// Returns NonFastForwardError if rejected due to diverged history.
func PushWithUpstream(dir, remote, branch string) error {
	out, err := Run(dir, "push", "-u", remote, branch)
	if err != nil {
		if isNonFastForward(out) {
			return &NonFastForwardError{Output: out}
		}
		return fmt.Errorf("%s", out)
	}
	return nil
}

// ForcePushWithUpstream pushes with --force-with-lease and sets upstream.
func ForcePushWithUpstream(dir, remote, branch string) error {
	out, err := Run(dir, "push", "--force-with-lease", "-u", remote, branch)
	if err != nil {
		return fmt.Errorf("%s", out)
	}
	return nil
}

// isNonFastForward detects push rejections due to diverged histories.
func isNonFastForward(output string) bool {
	return strings.Contains(output, "non-fast-forward") ||
		strings.Contains(output, "fetch first")
}

// PullRebase pulls with rebase, allowing unrelated histories (e.g. remote
// has a README, local has config — no common ancestor).
func PullRebase(dir string) error {
	out, err := Run(dir, "pull", "--rebase", "--allow-unrelated-histories")
	if err != nil {
		return fmt.Errorf("%s", out)
	}
	return nil
}

// PullRebaseFrom pulls from a specific remote/branch with rebase.
func PullRebaseFrom(dir, remote, branch string) error {
	out, err := Run(dir, "pull", "--rebase", "--allow-unrelated-histories", remote, branch)
	if err != nil {
		return fmt.Errorf("%s", out)
	}
	return nil
}

// ResetSoftHead undoes the last commit, keeping changes staged.
func ResetSoftHead(dir string) error {
	_, err := Run(dir, "reset", "--soft", "HEAD~1")
	return err
}

// Stash stashes uncommitted changes. Returns true if anything was stashed.
func Stash(dir string) bool {
	out, err := Run(dir, "stash")
	return err == nil && !strings.Contains(out, "No local changes")
}

// StashPop restores stashed changes.
func StashPop(dir string) {
	Run(dir, "stash", "pop")
}

// RemoteLog returns the last n commit summaries from a remote branch.
func RemoteLog(dir, remote, branch string, n int) (string, error) {
	ref := fmt.Sprintf("%s/%s", remote, branch)
	return Run(dir, "log", "--oneline", ref, fmt.Sprintf("-%d", n))
}

// Fetch runs git fetch --quiet.
func Fetch(dir string) error {
	_, err := Run(dir, "fetch", "--quiet")
	return err
}

// FetchPrune fetches and removes stale remote-tracking refs.
func FetchPrune(dir string) {
	Run(dir, "fetch", "--quiet", "--prune")
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
