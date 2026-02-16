package claudemd_test

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReconcile(t *testing.T) {
	t.Run("updated section", func(t *testing.T) {
		syncDir := t.TempDir()
		original := "## Git Conventions\nUse conventional commits"
		_, err := claudemd.ImportClaudeMD(syncDir, original)
		require.NoError(t, err)

		updated := "## Git Conventions\nUse conventional commits and sign all commits"
		result, err := claudemd.Reconcile(syncDir, updated)
		require.NoError(t, err)

		assert.Equal(t, []string{"git-conventions"}, result.Updated)
		assert.Empty(t, result.New)
		assert.Empty(t, result.Deleted)
		assert.Empty(t, result.Renamed)
	})

	t.Run("new section", func(t *testing.T) {
		syncDir := t.TempDir()
		original := "## Existing\nExisting content"
		_, err := claudemd.ImportClaudeMD(syncDir, original)
		require.NoError(t, err)

		current := "## Existing\nExisting content\n## Brand New\nNew content here"
		result, err := claudemd.Reconcile(syncDir, current)
		require.NoError(t, err)

		assert.Empty(t, result.Updated)
		require.Len(t, result.New, 1)
		assert.Equal(t, "Brand New", result.New[0].Header)
		assert.Empty(t, result.Deleted)
		assert.Empty(t, result.Renamed)
	})

	t.Run("deleted section", func(t *testing.T) {
		syncDir := t.TempDir()
		original := "## Keep\nKeep content\n## Remove\nRemove content"
		_, err := claudemd.ImportClaudeMD(syncDir, original)
		require.NoError(t, err)

		current := "## Keep\nKeep content"
		result, err := claudemd.Reconcile(syncDir, current)
		require.NoError(t, err)

		assert.Empty(t, result.Updated)
		assert.Empty(t, result.New)
		assert.Equal(t, []string{"remove"}, result.Deleted)
		assert.Empty(t, result.Renamed)
	})

	t.Run("renamed section", func(t *testing.T) {
		syncDir := t.TempDir()
		// Use a longer body so the header word difference is < 20% of total words.
		body := "Use conventional commits for all changes in this project repository always"
		original := "## Git Conventions\n" + body
		_, err := claudemd.ImportClaudeMD(syncDir, original)
		require.NoError(t, err)

		// Same body, different header.
		current := "## Git Rules\n" + body
		result, err := claudemd.Reconcile(syncDir, current)
		require.NoError(t, err)

		assert.Empty(t, result.Updated)
		assert.Empty(t, result.New)
		assert.Empty(t, result.Deleted)
		require.Len(t, result.Renamed, 1)
		assert.Equal(t, "git-conventions", result.Renamed[0].OldName)
		assert.Equal(t, "Git Rules", result.Renamed[0].NewHeader)
	})

	t.Run("no changes", func(t *testing.T) {
		syncDir := t.TempDir()
		content := "## Alpha\nAlpha content\n## Beta\nBeta content"
		_, err := claudemd.ImportClaudeMD(syncDir, content)
		require.NoError(t, err)

		result, err := claudemd.Reconcile(syncDir, content)
		require.NoError(t, err)

		assert.Empty(t, result.Updated)
		assert.Empty(t, result.New)
		assert.Empty(t, result.Deleted)
		assert.Empty(t, result.Renamed)
	})

	t.Run("empty manifest", func(t *testing.T) {
		syncDir := t.TempDir()
		current := "## New Section\nContent"
		result, err := claudemd.Reconcile(syncDir, current)
		require.NoError(t, err)

		assert.Empty(t, result.Updated)
		require.Len(t, result.New, 1)
		assert.Equal(t, "New Section", result.New[0].Header)
		assert.Empty(t, result.Deleted)
		assert.Empty(t, result.Renamed)
	})
}
