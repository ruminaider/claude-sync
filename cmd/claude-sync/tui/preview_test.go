package tui

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestClaudeMDPreviewSections(t *testing.T) {
	sections := []claudemd.Section{
		{Header: "", Content: "# Preamble\nSome content"},
		{Header: "Git Commits", Content: "## Git Commits\nDo not include Co-Authored-By"},
	}
	result := ClaudeMDPreviewSections(sections, "~/.claude/CLAUDE.md")

	require.Len(t, result, 2)

	// Preamble section
	assert.Equal(t, "(preamble)", result[0].Header)
	assert.Equal(t, "_preamble", result[0].FragmentKey)
	assert.Equal(t, "~/.claude/CLAUDE.md", result[0].Source)
	assert.Equal(t, "# Preamble\nSome content", result[0].Content)

	// Named section
	assert.Equal(t, "Git Commits", result[1].Header)
	assert.Equal(t, "git-commits", result[1].FragmentKey)
	assert.Equal(t, "~/.claude/CLAUDE.md", result[1].Source)
	assert.Equal(t, "## Git Commits\nDo not include Co-Authored-By", result[1].Content)
}

func TestClaudeMDPreviewSections_Empty(t *testing.T) {
	result := ClaudeMDPreviewSections(nil, "source.md")
	assert.Empty(t, result)
}

func TestClaudeMDPreviewSections_PreambleOnly(t *testing.T) {
	sections := []claudemd.Section{
		{Header: "", Content: "Just content without headers"},
	}
	result := ClaudeMDPreviewSections(sections, "/path/CLAUDE.md")
	require.Len(t, result, 1)
	assert.Equal(t, "(preamble)", result[0].Header)
	assert.Equal(t, "_preamble", result[0].FragmentKey)
}

func TestPreviewSelectedFragmentKeys(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble"},
		{Header: "Git Commits", FragmentKey: "git-commits"},
		{Header: "Colors", FragmentKey: "colors"},
	}
	p := NewPreview(sections)

	// All selected by default
	keys := p.SelectedFragmentKeys()
	assert.Equal(t, []string{"_preamble", "git-commits", "colors"}, keys)
	assert.Equal(t, 3, p.SelectedCount())
	assert.Equal(t, 3, p.TotalCount())
}

func TestPreviewSelectedFragmentKeys_PartialSelection(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble"},
		{Header: "Git Commits", FragmentKey: "git-commits"},
		{Header: "Colors", FragmentKey: "colors"},
	}
	p := NewPreview(sections)

	// Deselect the second one
	p.selected[1] = false

	keys := p.SelectedFragmentKeys()
	assert.Equal(t, []string{"_preamble", "colors"}, keys)
	assert.Equal(t, 2, p.SelectedCount())
	assert.Equal(t, 3, p.TotalCount())
}

func TestPreviewSelectedFragmentKeys_NoneSelected(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble"},
		{Header: "Git Commits", FragmentKey: "git-commits"},
	}
	p := NewPreview(sections)

	// Deselect all
	for i := range p.sections {
		p.selected[i] = false
	}

	keys := p.SelectedFragmentKeys()
	assert.Nil(t, keys)
	assert.Equal(t, 0, p.SelectedCount())
}

func TestPreviewAddSections(t *testing.T) {
	initial := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble", Source: "a.md"},
	}
	p := NewPreview(initial)

	assert.Equal(t, 1, p.TotalCount())

	// Add new sections
	additions := []PreviewSection{
		{Header: "New Section", FragmentKey: "new-section", Source: "b.md"},
		{Header: "Another", FragmentKey: "another", Source: "b.md"},
	}
	p.AddSections(additions)

	assert.Equal(t, 3, p.TotalCount())
	assert.Equal(t, 3, p.SelectedCount()) // new sections should be pre-selected

	keys := p.SelectedFragmentKeys()
	assert.Equal(t, []string{"_preamble", "new-section", "another"}, keys)
}

func TestPreviewAddSections_PreservesExistingSelection(t *testing.T) {
	initial := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble"},
		{Header: "Existing", FragmentKey: "existing"},
	}
	p := NewPreview(initial)

	// Deselect the first one
	p.selected[0] = false

	// Add a new section
	p.AddSections([]PreviewSection{
		{Header: "New", FragmentKey: "new"},
	})

	// First should still be deselected
	assert.False(t, p.selected[0])
	// Second should still be selected
	assert.True(t, p.selected[1])
	// New should be selected
	assert.True(t, p.selected[2])
}

func TestNewPreview_Empty(t *testing.T) {
	p := NewPreview(nil)
	assert.Equal(t, 0, p.TotalCount())
	assert.Equal(t, 0, p.SelectedCount())
	assert.Nil(t, p.SelectedFragmentKeys())
}

func TestPreviewSetSize(t *testing.T) {
	sections := []PreviewSection{
		{Header: "Test", FragmentKey: "test", Content: "content"},
	}
	p := NewPreview(sections)
	p.SetSize(100, 40)

	assert.Equal(t, 100, p.totalWidth)
	assert.Equal(t, 40, p.totalHeight)
	// Left panel gets roughly 1/3
	assert.Equal(t, 33, p.listWidth)
}

func TestPreviewSetSize_Small(t *testing.T) {
	sections := []PreviewSection{
		{Header: "Test", FragmentKey: "test", Content: "content"},
	}
	p := NewPreview(sections)
	p.SetSize(40, 10)

	// listWidth should be at least 20
	assert.GreaterOrEqual(t, p.listWidth, 20)
}
