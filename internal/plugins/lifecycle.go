package plugins

import (
	"os/exec"
	"strings"
	"time"
)

// ReEvalSignal describes why a tracked plugin needs user attention.
type ReEvalSignal struct {
	PluginName  string
	Entry       PluginSourceEntry
	LocalStale  bool // no local commits in staleDays
	RepoMissing bool // local_repo path doesn't exist or isn't a git repo
}

// CheckReEvaluation inspects all active-dev plugins and returns signals
// for any that may need switching back to marketplace.
func CheckReEvaluation(sources PluginSources, staleDays int) []ReEvalSignal {
	var signals []ReEvalSignal

	for name, entry := range sources.Plugins {
		if entry.Relationship != "active-dev" {
			continue
		}

		// Check snooze
		if entry.SnoozeUntil != nil && time.Now().Before(*entry.SnoozeUntil) {
			continue
		}

		signal := ReEvalSignal{
			PluginName: name,
			Entry:      entry,
		}
		triggered := false

		// Check if local repo exists and is a git repo
		lastCommit, err := lastCommitDate(entry.LocalRepo)
		if err != nil {
			signal.RepoMissing = true
			triggered = true
		} else {
			daysSince := int(time.Since(lastCommit).Hours() / 24)
			if daysSince >= staleDays {
				signal.LocalStale = true
				triggered = true
			}
		}

		if triggered {
			signals = append(signals, signal)
		}
	}

	return signals
}

// lastCommitDate returns the date of the most recent commit in a git repo.
func lastCommitDate(repoPath string) (time.Time, error) {
	cmd := exec.Command("git", "-C", repoPath, "log", "-1", "--format=%cI")
	out, err := cmd.Output()
	if err != nil {
		return time.Time{}, err
	}
	return time.Parse(time.RFC3339, strings.TrimSpace(string(out)))
}
