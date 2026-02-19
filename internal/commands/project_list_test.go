package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/ruminaider/claude-sync/internal/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestProjectList(t *testing.T) {
	proj1 := t.TempDir()
	proj2 := t.TempDir()
	proj3 := t.TempDir() // no config

	os.MkdirAll(filepath.Join(proj1, ".claude"), 0755)
	os.MkdirAll(filepath.Join(proj2, ".claude"), 0755)

	project.WriteProjectConfig(proj1, project.ProjectConfig{Version: "1.0.0", Profile: "work"})
	project.WriteProjectConfig(proj2, project.ProjectConfig{Version: "1.0.0", Profile: "personal"})

	results, err := commands.ProjectList([]string{proj1, proj2, proj3})
	require.NoError(t, err)
	assert.Len(t, results, 2)

	profiles := make(map[string]string)
	for _, r := range results {
		profiles[r.Path] = r.Profile
	}
	assert.Equal(t, "work", profiles[proj1])
	assert.Equal(t, "personal", profiles[proj2])
}

func TestProjectList_SkipsDeclined(t *testing.T) {
	proj := t.TempDir()
	os.MkdirAll(filepath.Join(proj, ".claude"), 0755)

	project.WriteProjectConfig(proj, project.ProjectConfig{
		Version:  "1.0.0",
		Declined: true,
	})

	results, err := commands.ProjectList([]string{proj})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestProjectList_EmptyDirs(t *testing.T) {
	results, err := commands.ProjectList([]string{t.TempDir(), t.TempDir()})
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestProjectListScan(t *testing.T) {
	parent := t.TempDir()

	// Create two project subdirectories
	proj1 := filepath.Join(parent, "project-a")
	proj2 := filepath.Join(parent, "project-b")
	os.MkdirAll(filepath.Join(proj1, ".claude"), 0755)
	os.MkdirAll(filepath.Join(proj2, ".claude"), 0755)
	os.MkdirAll(filepath.Join(parent, "not-a-project"), 0755) // no config

	project.WriteProjectConfig(proj1, project.ProjectConfig{Version: "1.0.0", Profile: "work"})
	project.WriteProjectConfig(proj2, project.ProjectConfig{Version: "1.0.0", Profile: "personal"})

	results, err := commands.ProjectListScan([]string{parent})
	require.NoError(t, err)
	assert.Len(t, results, 2)
}

func TestProjectRemove(t *testing.T) {
	proj := t.TempDir()
	os.MkdirAll(filepath.Join(proj, ".claude"), 0755)

	project.WriteProjectConfig(proj, project.ProjectConfig{Version: "1.0.0", Profile: "work"})

	// Verify config exists
	_, err := project.ReadProjectConfig(proj)
	require.NoError(t, err)

	// Remove
	err = commands.ProjectRemove(proj)
	require.NoError(t, err)

	// Verify config no longer exists
	_, err = project.ReadProjectConfig(proj)
	assert.ErrorIs(t, err, project.ErrNoProjectConfig)
}

func TestProjectRemove_NotManaged(t *testing.T) {
	proj := t.TempDir()
	err := commands.ProjectRemove(proj)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "not managed")
}
