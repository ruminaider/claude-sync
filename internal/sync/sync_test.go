package sync_test

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/sync"
	"github.com/stretchr/testify/assert"
)

func TestComputeDiff(t *testing.T) {
	t.Run("install missing plugins", func(t *testing.T) {
		desired := []string{"context7@official", "beads@beads", "episodic-memory@super"}
		installed := []string{"context7@official"}

		diff := sync.ComputePluginDiff(desired, installed)
		assert.ElementsMatch(t, []string{"beads@beads", "episodic-memory@super"}, diff.ToInstall)
		assert.Empty(t, diff.ToRemove)
		assert.ElementsMatch(t, []string{"context7@official"}, diff.Synced)
	})

	t.Run("detect untracked plugins", func(t *testing.T) {
		desired := []string{"context7@official"}
		installed := []string{"context7@official", "local-plugin@some"}

		diff := sync.ComputePluginDiff(desired, installed)
		assert.Empty(t, diff.ToInstall)
		assert.ElementsMatch(t, []string{"local-plugin@some"}, diff.Untracked)
	})

	t.Run("union mode keeps extras", func(t *testing.T) {
		desired := []string{"context7@official"}
		installed := []string{"context7@official", "extra@local"}

		diff := sync.ComputePluginDiff(desired, installed)
		assert.Empty(t, diff.ToRemove)
		assert.Contains(t, diff.Untracked, "extra@local")
	})

	t.Run("empty desired", func(t *testing.T) {
		diff := sync.ComputePluginDiff(nil, []string{"a@b"})
		assert.Empty(t, diff.Synced)
		assert.Empty(t, diff.ToInstall)
		assert.Contains(t, diff.Untracked, "a@b")
	})

	t.Run("empty installed", func(t *testing.T) {
		diff := sync.ComputePluginDiff([]string{"a@b"}, nil)
		assert.Contains(t, diff.ToInstall, "a@b")
		assert.Empty(t, diff.Synced)
	})
}

func TestApplyUserPreferences(t *testing.T) {
	t.Run("unsubscribe removes from desired", func(t *testing.T) {
		desired := []string{"context7@official", "greptile@official", "ralph-wiggum@official"}
		unsubscribe := []string{"ralph-wiggum@official", "greptile@official"}
		personal := []string{"my-plugin@local"}

		result := sync.ApplyPluginPreferences(desired, unsubscribe, personal)
		assert.Contains(t, result, "context7@official")
		assert.Contains(t, result, "my-plugin@local")
		assert.NotContains(t, result, "ralph-wiggum@official")
		assert.NotContains(t, result, "greptile@official")
	})

	t.Run("no preferences", func(t *testing.T) {
		desired := []string{"a@b", "c@d"}
		result := sync.ApplyPluginPreferences(desired, nil, nil)
		assert.ElementsMatch(t, desired, result)
	})
}

func TestComputeSettingsDiff(t *testing.T) {
	t.Run("detect changed settings", func(t *testing.T) {
		desired := map[string]any{"model": "opus"}
		current := map[string]any{"model": "sonnet"}

		diff := sync.ComputeSettingsDiff(desired, current)
		assert.Len(t, diff.Changed, 1)
		assert.Equal(t, "opus", diff.Changed["model"].Desired)
		assert.Equal(t, "sonnet", diff.Changed["model"].Current)
	})

	t.Run("no diff when equal", func(t *testing.T) {
		desired := map[string]any{"model": "opus"}
		current := map[string]any{"model": "opus"}

		diff := sync.ComputeSettingsDiff(desired, current)
		assert.Empty(t, diff.Changed)
	})

	t.Run("detect new settings", func(t *testing.T) {
		desired := map[string]any{"model": "opus"}
		current := map[string]any{}

		diff := sync.ComputeSettingsDiff(desired, current)
		assert.Len(t, diff.Changed, 1)
	})
}
