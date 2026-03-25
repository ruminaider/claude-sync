package memory_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportFromDir(t *testing.T) {
	sourceDir := t.TempDir()
	syncMemDir := t.TempDir()

	// Create source files
	frag1 := "---\nname: Coding Style\ndescription: My coding style\ntype: user\n---\n\nI prefer short functions."
	frag2 := "---\nname: Project Notes\ndescription: Notes about the project\ntype: project\n---\n\nImportant notes here."
	memoryMd := "# Memory\n\nThis should be skipped."

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "coding-style.md"), []byte(frag1), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "project-notes.md"), []byte(frag2), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "MEMORY.md"), []byte(memoryMd), 0o644))
	// Create a non-.md file that should be skipped
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "readme.txt"), []byte("skip me"), 0o644))
	// Create a directory that should be skipped
	require.NoError(t, os.MkdirAll(filepath.Join(sourceDir, "subdir"), 0o755))

	result, err := memory.ImportFromDir(sourceDir, syncMemDir)
	require.NoError(t, err)
	require.Len(t, result.Imported, 2)

	// Verify fragments were written
	for _, slug := range result.Imported {
		_, err := memory.ReadFragment(syncMemDir, slug)
		require.NoError(t, err, "fragment %s should exist", slug)
	}

	// Verify manifest was written
	manifest, err := memory.ReadManifest(syncMemDir)
	require.NoError(t, err)
	assert.Len(t, manifest.Fragments, 2)
	assert.Len(t, manifest.Order, 2)

	// Verify MEMORY.md was not imported
	assert.NotContains(t, manifest.Fragments, "memory")
}

func TestImportFromDir_Collision(t *testing.T) {
	sourceDir := t.TempDir()
	syncMemDir := t.TempDir()

	// Pre-populate manifest with existing fragment
	manifest := memory.Manifest{
		Fragments: map[string]memory.FragmentMeta{
			"my-fragment": {Name: "My Fragment", ContentHash: "existing"},
		},
		Order: []string{"my-fragment"},
	}
	require.NoError(t, memory.WriteManifest(syncMemDir, manifest))

	// Create a source file that will collide
	content := "---\nname: My Fragment\ndescription: Colliding name\ntype: user\n---\n\nNew content."
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "my-fragment.md"), []byte(content), 0o644))

	result, err := memory.ImportFromDir(sourceDir, syncMemDir)
	require.NoError(t, err)
	require.Len(t, result.Imported, 1)
	assert.Equal(t, "my-fragment-2", result.Imported[0])

	// Verify both exist in manifest
	m, err := memory.ReadManifest(syncMemDir)
	require.NoError(t, err)
	assert.Contains(t, m.Fragments, "my-fragment")
	assert.Contains(t, m.Fragments, "my-fragment-2")
}

func TestReconcile(t *testing.T) {
	sourceDir := t.TempDir()
	syncMemDir := t.TempDir()

	// Set up initial state: two fragments in manifest
	origContent := "---\nname: Coding Style\ndescription: My coding style\ntype: user\n---\n\nOriginal content."
	unchangedContent := "---\nname: Project Notes\ndescription: Notes\ntype: project\n---\n\nUnchanged."

	manifest := memory.Manifest{
		Fragments: map[string]memory.FragmentMeta{
			"coding-style": {
				Name:        "Coding Style",
				Description: "My coding style",
				Type:        "user",
				ContentHash: memory.ContentHash(origContent),
			},
			"project-notes": {
				Name:        "Project Notes",
				Description: "Notes",
				Type:        "project",
				ContentHash: memory.ContentHash(unchangedContent),
			},
			"deleted-fragment": {
				Name:        "Deleted Fragment",
				Description: "Will be deleted",
				Type:        "user",
				ContentHash: "somehash",
			},
		},
		Order: []string{"coding-style", "project-notes", "deleted-fragment"},
	}
	require.NoError(t, memory.WriteManifest(syncMemDir, manifest))

	// In sourceDir: updated coding-style, unchanged project-notes, new fragment, deleted-fragment missing
	updatedContent := "---\nname: Coding Style\ndescription: My coding style\ntype: user\n---\n\nUpdated content here."
	newContent := "---\nname: New Ideas\ndescription: Fresh ideas\ntype: feedback\n---\n\nBrand new."

	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "coding-style.md"), []byte(updatedContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "project-notes.md"), []byte(unchangedContent), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "new-ideas.md"), []byte(newContent), 0o644))

	result, err := memory.Reconcile(sourceDir, syncMemDir)
	require.NoError(t, err)

	// Check updated
	assert.Contains(t, result.Updated, "coding-style")
	assert.NotContains(t, result.Updated, "project-notes")

	// Check new
	require.Len(t, result.New, 1)
	assert.Equal(t, "new-ideas", result.New[0].SlugName)
	assert.Equal(t, newContent, result.New[0].Content)

	// Check deleted
	assert.Contains(t, result.Deleted, "deleted-fragment")

	// Verify updated content was written to syncMemDir
	got, err := memory.ReadFragment(syncMemDir, "coding-style")
	require.NoError(t, err)
	assert.Equal(t, updatedContent, got)

	// Verify manifest was updated with new content hash
	updatedManifest, err := memory.ReadManifest(syncMemDir)
	require.NoError(t, err)
	assert.Equal(t, memory.ContentHash(updatedContent), updatedManifest.Fragments["coding-style"].ContentHash,
		"manifest should have updated content hash after reconcile")
	// Unchanged fragment should retain original hash
	assert.Equal(t, memory.ContentHash(unchangedContent), updatedManifest.Fragments["project-notes"].ContentHash,
		"unchanged fragment hash should remain the same")
}
