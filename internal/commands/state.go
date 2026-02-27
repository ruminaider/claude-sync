package commands

import (
	"os"

	"github.com/ruminaider/claude-sync/internal/approval"
	"github.com/ruminaider/claude-sync/internal/profiles"
)

// MenuState holds the detected state used to build the TUI menu.
type MenuState struct {
	ConfigExists  bool
	HasPending    bool
	HasConflicts  bool
	Profiles      []string
	ActiveProfile string
}

// DetectMenuState checks the current claude-sync state for menu rendering.
// It is designed to be fast and never error — unknown state defaults to false/empty.
func DetectMenuState(claudeDir, syncDir string) MenuState {
	var state MenuState

	// Check if sync dir exists (i.e., claude-sync is initialized)
	if _, err := os.Stat(syncDir); os.IsNotExist(err) {
		return state
	}
	state.ConfigExists = true

	// Check for pending approval changes
	pending, err := approval.ReadPending(syncDir)
	if err == nil && !pending.IsEmpty() {
		state.HasPending = true
	}

	// Check for merge conflicts
	state.HasConflicts = HasPendingConflicts(syncDir)

	// Check for profiles
	profileList, err := profiles.ListProfiles(syncDir)
	if err == nil {
		state.Profiles = profileList
	}

	// Check active profile
	active, err := profiles.ReadActiveProfile(syncDir)
	if err == nil {
		state.ActiveProfile = active
	}

	return state
}
