package commands

import (
	"fmt"

	"github.com/ruminaider/claude-sync/internal/approval"
)

// Reject clears pending changes without applying them.
func Reject(syncDir string) error {
	pending, err := approval.ReadPending(syncDir)
	if err != nil {
		return fmt.Errorf("reading pending changes: %w", err)
	}

	if pending.IsEmpty() {
		return fmt.Errorf("no pending changes to reject")
	}

	if err := approval.ClearPending(syncDir); err != nil {
		return fmt.Errorf("clearing pending: %w", err)
	}

	return nil
}
