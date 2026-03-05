package commands_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppliedHashes_LoadSave(t *testing.T) {
	dir := t.TempDir()

	h := commands.LoadAppliedHashes(dir)
	h.Set("settings", "some content")
	require.NoError(t, h.Save())

	h2 := commands.LoadAppliedHashes(dir)
	assert.Equal(t, h.Hashes["settings"], h2.Hashes["settings"])
}

func TestAppliedHashes_IsLocallyModified_NoStoredHash(t *testing.T) {
	dir := t.TempDir()
	h := commands.LoadAppliedHashes(dir)

	// Create a file but don't store a hash for it.
	filePath := filepath.Join(dir, "test.json")
	os.WriteFile(filePath, []byte("content"), 0644)

	// No stored hash = first pull, should allow overwrite.
	assert.False(t, h.IsLocallyModified("test", filePath))
}

func TestAppliedHashes_IsLocallyModified_Unchanged(t *testing.T) {
	dir := t.TempDir()
	h := commands.LoadAppliedHashes(dir)

	content := "original content"
	filePath := filepath.Join(dir, "test.json")
	os.WriteFile(filePath, []byte(content), 0644)

	// Store hash of original content.
	h.Set("test", content)

	// File hasn't changed — not locally modified.
	assert.False(t, h.IsLocallyModified("test", filePath))
}

func TestAppliedHashes_IsLocallyModified_Changed(t *testing.T) {
	dir := t.TempDir()
	h := commands.LoadAppliedHashes(dir)

	// Store hash of original content.
	h.Set("test", "original content")

	// Write different content to file.
	filePath := filepath.Join(dir, "test.json")
	os.WriteFile(filePath, []byte("modified content"), 0644)

	// File differs from stored hash — locally modified.
	assert.True(t, h.IsLocallyModified("test", filePath))
}

func TestAppliedHashes_IsLocallyModified_FileMissing(t *testing.T) {
	dir := t.TempDir()
	h := commands.LoadAppliedHashes(dir)

	h.Set("test", "some content")

	// File doesn't exist — allow overwrite.
	assert.False(t, h.IsLocallyModified("test", filepath.Join(dir, "nonexistent.json")))
}
