package commands

import (
	"os"
	"path/filepath"
)

// DefaultProjectSearchDirs is a function variable returning the directories to
// scan for managed projects. It can be overridden in tests.
var DefaultProjectSearchDirs = defaultProjectSearchDirs

func defaultProjectSearchDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Work"),
		filepath.Join(home, "Projects"),
		filepath.Join(home, "Repositories"),
		filepath.Join(home, "repos"),
		filepath.Join(home, "src"),
	}
}
