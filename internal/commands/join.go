package commands

import (
	"fmt"
	"os"

	"github.com/ruminaider/claude-sync/internal/claudecode"
	"github.com/ruminaider/claude-sync/internal/git"
)

func Join(repoURL, claudeDir, syncDir string) error {
	if _, err := os.Stat(syncDir); err == nil {
		return fmt.Errorf("%s already exists. Run 'claude-sync pull' instead", syncDir)
	}

	if !claudecode.DirExists(claudeDir) {
		if err := claudecode.Bootstrap(claudeDir); err != nil {
			return fmt.Errorf("bootstrapping Claude Code directory: %w", err)
		}
	}

	if err := git.Clone(repoURL, syncDir); err != nil {
		return fmt.Errorf("cloning config repo: %w", err)
	}

	return nil
}
