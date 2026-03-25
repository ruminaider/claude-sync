package memory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFullMemorySyncFlow(t *testing.T) {
	// 1. Set up a source directory with memory files.
	sourceDir := t.TempDir()
	syncMemDir := t.TempDir()
	targetDir := t.TempDir()

	// Write two memory files.
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "pref.md"),
		[]byte("---\nname: User Preference\ndescription: Likes terse\ntype: user\n---\n\nTerse please."), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "feedback.md"),
		[]byte("---\nname: No Mocks\ndescription: Use real DB\ntype: feedback\n---\n\nReal DB."), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "MEMORY.md"),
		[]byte("# Index\n"), 0o644)) // should be skipped

	// 2. Import into sync repo.
	importResult, err := memory.ImportFromDir(sourceDir, syncMemDir)
	require.NoError(t, err)
	require.Len(t, importResult.Imported, 2)

	// 3. Verify manifest.
	m, err := memory.ReadManifest(syncMemDir)
	require.NoError(t, err)
	assert.Len(t, m.Fragments, 2)

	// 4. Write fragments to target (simulates pull).
	for _, name := range importResult.Imported {
		content, err := memory.ReadFragment(syncMemDir, name)
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(targetDir, 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(targetDir, name+".md"), []byte(content), 0o644))
	}

	// 5. Add a local-only file.
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, "local-experiment.md"),
		[]byte("---\nname: Local Experiment\ntype: project\n---\n\nMy experiment."), 0o644))

	// 6. Regenerate index.
	err = memory.RegenerateIndex(targetDir)
	require.NoError(t, err)

	// 7. Verify MEMORY.md includes all files.
	index, err := os.ReadFile(filepath.Join(targetDir, "MEMORY.md"))
	require.NoError(t, err)
	assert.Contains(t, string(index), "User Preference")
	assert.Contains(t, string(index), "No Mocks")
	assert.Contains(t, string(index), "Local Experiment")

	// 8. Modify a synced file locally (simulates user edit).
	// Use the known slug for "User Preference" to avoid order-dependent indexing.
	prefSlug := memory.SlugifyName("User Preference")
	modifiedContent := "---\nname: User Preference\ndescription: Updated\ntype: user\n---\n\nUpdated preference."
	require.NoError(t, os.WriteFile(filepath.Join(targetDir, prefSlug+".md"), []byte(modifiedContent), 0o644))

	// 9. Reconcile (push direction).
	reconcileResult, err := memory.Reconcile(targetDir, syncMemDir)
	require.NoError(t, err)
	assert.NotEmpty(t, reconcileResult.Updated, "should detect the content edit")
	assert.NotEmpty(t, reconcileResult.New, "local-experiment should be detected as new")
}

func TestReconcileDeletedFragment(t *testing.T) {
	sourceDir := t.TempDir()
	syncMemDir := t.TempDir()

	// Set up a synced fragment.
	content := "---\nname: Old Memory\ntype: user\n---\n\nOld."
	require.NoError(t, memory.WriteFragment(syncMemDir, "old-memory", content))
	require.NoError(t, memory.WriteManifest(syncMemDir, memory.Manifest{
		Fragments: map[string]memory.FragmentMeta{
			"old-memory": {Name: "Old Memory", Type: "user", Level: "user", ContentHash: memory.ContentHash(content)},
		},
		Order: []string{"old-memory"},
	}))

	// Source dir is empty (the memory was deleted locally).
	// Reconcile should detect it as deleted.
	result, err := memory.Reconcile(sourceDir, syncMemDir)
	require.NoError(t, err)
	assert.Contains(t, result.Deleted, "old-memory")
}

func TestLocalOnlyFilesPreservedDuringRegeneration(t *testing.T) {
	dir := t.TempDir()

	// Write synced + local files.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "synced.md"),
		[]byte("---\nname: Synced\ntype: user\n---\n\nSynced."), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "local-only.md"),
		[]byte("---\nname: Local Only\ntype: feedback\n---\n\nLocal."), 0o644))

	// Regenerate index.
	err := memory.RegenerateIndex(dir)
	require.NoError(t, err)

	// Both should be in MEMORY.md.
	index, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	require.NoError(t, err)
	assert.Contains(t, string(index), "Synced")
	assert.Contains(t, string(index), "Local Only")

	// Local file should still exist on disk.
	_, err = os.Stat(filepath.Join(dir, "local-only.md"))
	assert.NoError(t, err)
}
