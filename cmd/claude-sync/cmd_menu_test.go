package main

import (
	"testing"

	"github.com/ruminaider/claude-sync/cmd/claude-sync/tui"
	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
)

func TestNewAppModel_SetsFields(t *testing.T) {
	state := commands.MenuState{ConfigExists: true}
	model := tui.NewAppModel(state)
	model.SetVersion("1.2.3")
	model.SetClaudeDir("/mock/claude")
	model.SetSyncDir("/mock/sync")

	// Verify model can be cast and has no launch signal by default
	assert.False(t, model.LaunchConfigEditor)
}

func TestNewAppModel_FreshInstall(t *testing.T) {
	state := commands.MenuState{ConfigExists: false}
	model := tui.NewAppModel(state)
	model.SetVersion("0.1.0")

	// Should render without panicking
	view := model.View()
	assert.NotEmpty(t, view)
}
