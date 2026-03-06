package commands_test

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/ruminaider/claude-sync/internal/commands"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAppliedHashes_LoadSave(t *testing.T) {
	dir := t.TempDir()

	h, err := commands.LoadAppliedHashes(dir)
	require.NoError(t, err)
	h.Set("settings", "some content")
	require.NoError(t, h.Save())

	h2, err := commands.LoadAppliedHashes(dir)
	require.NoError(t, err)
	assert.Equal(t, h.Hashes["settings"], h2.Hashes["settings"])
}

func TestAppliedHashes_IsLocallyModified_NoStoredHash(t *testing.T) {
	dir := t.TempDir()
	h, err := commands.LoadAppliedHashes(dir)
	require.NoError(t, err)

	// Create a file but don't store a hash for it.
	filePath := filepath.Join(dir, "test.json")
	require.NoError(t, os.WriteFile(filePath, []byte("content"), 0644))

	// No stored hash = first pull, should allow overwrite.
	assert.False(t, h.IsLocallyModified("test", filePath))
}

func TestAppliedHashes_IsLocallyModified_Unchanged(t *testing.T) {
	dir := t.TempDir()
	h, err := commands.LoadAppliedHashes(dir)
	require.NoError(t, err)

	content := "original content"
	filePath := filepath.Join(dir, "test.json")
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))

	// Store hash of original content.
	h.Set("test", content)

	// File hasn't changed — not locally modified.
	assert.False(t, h.IsLocallyModified("test", filePath))
}

func TestAppliedHashes_IsLocallyModified_Changed(t *testing.T) {
	dir := t.TempDir()
	h, err := commands.LoadAppliedHashes(dir)
	require.NoError(t, err)

	// Store hash of original content.
	h.Set("test", "original content")

	// Write different content to file.
	filePath := filepath.Join(dir, "test.json")
	require.NoError(t, os.WriteFile(filePath, []byte("modified content"), 0644))

	// File differs from stored hash — locally modified.
	assert.True(t, h.IsLocallyModified("test", filePath))
}

func TestAppliedHashes_IsLocallyModified_FileMissing(t *testing.T) {
	dir := t.TempDir()
	h, err := commands.LoadAppliedHashes(dir)
	require.NoError(t, err)

	h.Set("test", "some content")

	// File doesn't exist — allow overwrite.
	assert.False(t, h.IsLocallyModified("test", filepath.Join(dir, "nonexistent.json")))
}

func TestAppliedHashes_LoadCorruptJSON(t *testing.T) {
	dir := t.TempDir()

	// Write invalid JSON to the hash file.
	hashPath := filepath.Join(dir, ".applied-hashes.json")
	require.NoError(t, os.WriteFile(hashPath, []byte("{corrupt json!!!"), 0644))

	h, err := commands.LoadAppliedHashes(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing applied hashes")

	// Should still return a usable (empty) hash map.
	assert.NotNil(t, h)
	assert.NotNil(t, h.Hashes)
	assert.Empty(t, h.Hashes)
}

func TestAppliedHashes_LoadIOError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based test not reliable on Windows")
	}

	dir := t.TempDir()

	// Write valid JSON then make it unreadable.
	hashPath := filepath.Join(dir, ".applied-hashes.json")
	require.NoError(t, os.WriteFile(hashPath, []byte(`{"hashes":{}}`), 0644))
	require.NoError(t, os.Chmod(hashPath, 0000))
	t.Cleanup(func() { os.Chmod(hashPath, 0644) })

	h, err := commands.LoadAppliedHashes(dir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "reading applied hashes")

	// Should still return a usable (empty) hash map.
	assert.NotNil(t, h)
	assert.NotNil(t, h.Hashes)
}

func TestAppliedHashes_IsLocallyModified_PermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission-based test not reliable on Windows")
	}

	dir := t.TempDir()
	h, err := commands.LoadAppliedHashes(dir)
	require.NoError(t, err)

	// Create file, store hash, then make unreadable.
	filePath := filepath.Join(dir, "test.json")
	content := "original content"
	require.NoError(t, os.WriteFile(filePath, []byte(content), 0644))
	h.Set("test", content)
	require.NoError(t, os.Chmod(filePath, 0000))
	t.Cleanup(func() { os.Chmod(filePath, 0644) })

	// Can't read file — conservative: assume modified.
	assert.True(t, h.IsLocallyModified("test", filePath))
}

func TestAppliedHashes_SaveError(t *testing.T) {
	// Point hash path to an unwritable location.
	h, err := commands.LoadAppliedHashes("/nonexistent/directory/that/does/not/exist")
	require.NoError(t, err) // file doesn't exist = first run, no error

	h.Set("test", "content")
	assert.Error(t, h.Save())
}
