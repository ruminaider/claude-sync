package commands

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/project"
)

// ProjectRemove removes claude-sync management from a project.
// It deletes .claude/.claude-sync.yaml but leaves settings.local.json as-is.
func ProjectRemove(projectDir string) error {
	configPath := filepath.Join(projectDir, ".claude", project.ConfigFileName)
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		return fmt.Errorf("project is not managed by claude-sync (no %s found)", project.ConfigFileName)
	}
	return os.Remove(configPath)
}
