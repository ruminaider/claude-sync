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
	assert.Equal(t, "/path/CLAUDE.md::_preamble", result[0].FragmentKey)
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

func TestPreviewRebuildRows_SingleSource(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble", Source: "~/.claude/CLAUDE.md"},
		{Header: "Git Commits", FragmentKey: "git-commits", Source: "~/.claude/CLAUDE.md"},
	}
	p := NewPreview(sections)

	// Should have 1 header + 2 section rows.
	require.Len(t, p.rows, 3)
	assert.True(t, p.rows[0].isHeader)
	assert.Equal(t, "~/.claude/CLAUDE.md", p.rows[0].source)
	assert.Equal(t, 2, p.rows[0].count)
	assert.False(t, p.rows[1].isHeader)
	assert.Equal(t, 0, p.rows[1].sectionIdx)
	assert.False(t, p.rows[2].isHeader)
	assert.Equal(t, 1, p.rows[2].sectionIdx)
}

func TestPreviewRebuildRows_MultipleSources(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble", Source: "~/.claude/CLAUDE.md"},
		{Header: "Git Commits", FragmentKey: "git-commits", Source: "~/.claude/CLAUDE.md"},
		{Header: "API Keys", FragmentKey: "api-keys", Source: "~/project/CLAUDE.md"},
	}
	p := NewPreview(sections)

	// Should have 2 headers + 3 section rows = 5 rows.
	require.Len(t, p.rows, 5)

	// First group header.
	assert.True(t, p.rows[0].isHeader)
	assert.Equal(t, "~/.claude/CLAUDE.md", p.rows[0].source)
	assert.Equal(t, 2, p.rows[0].count)
	// First group sections.
	assert.Equal(t, 0, p.rows[1].sectionIdx)
	assert.Equal(t, 1, p.rows[2].sectionIdx)
	// Second group header.
	assert.True(t, p.rows[3].isHeader)
	assert.Equal(t, "~/project/CLAUDE.md", p.rows[3].source)
	assert.Equal(t, 1, p.rows[3].count)
	// Second group section.
	assert.Equal(t, 2, p.rows[4].sectionIdx)
}

func TestPreviewCollapse(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", Source: "a.md"},
		{Header: "Git Commits", Source: "a.md"},
		{Header: "API Keys", Source: "b.md"},
	}
	p := NewPreview(sections)
	require.Len(t, p.rows, 5) // 2 headers + 3 sections

	// Collapse first group.
	p.toggleCollapse("a.md")

	// Should have 2 headers + 1 section (from b.md) = 3 rows.
	require.Len(t, p.rows, 3)
	assert.True(t, p.rows[0].isHeader)
	assert.Equal(t, "a.md", p.rows[0].source)
	assert.Equal(t, 2, p.rows[0].count) // count still 2 even when collapsed
	assert.True(t, p.rows[1].isHeader)
	assert.Equal(t, "b.md", p.rows[1].source)
	assert.Equal(t, 2, p.rows[2].sectionIdx) // API Keys

	// Expand again.
	p.toggleCollapse("a.md")
	require.Len(t, p.rows, 5)
}

func TestPreviewAddSections_RebuildRows(t *testing.T) {
	initial := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble", Source: "a.md"},
	}
	p := NewPreview(initial)
	require.Len(t, p.rows, 2) // 1 header + 1 section

	p.AddSections([]PreviewSection{
		{Header: "New", FragmentKey: "new", Source: "b.md"},
	})

	// Now 2 headers + 2 sections = 4 rows.
	require.Len(t, p.rows, 4)
	assert.Equal(t, "a.md", p.rows[0].source)
	assert.Equal(t, "b.md", p.rows[2].source)
}

func TestPreviewSelectAllNone(t *testing.T) {
	sections := []PreviewSection{
		{Header: "A", FragmentKey: "a", Source: "a.md"},
		{Header: "B", FragmentKey: "b", Source: "b.md"},
	}
	p := NewPreview(sections)

	// Collapse one group then select none — should still deselect all.
	p.toggleCollapse("a.md")

	for i := range p.sections {
		p.selected[i] = false
	}
	assert.Equal(t, 0, p.SelectedCount())

	// Select all — should select collapsed sections too.
	for i := range p.sections {
		p.selected[i] = true
	}
	assert.Equal(t, 2, p.SelectedCount())
}

func TestGlobalSelectedFragmentKeys(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "_preamble"},                              // global
		{Header: "Git Commits", FragmentKey: "git-commits"},                           // global
		{Header: "(preamble)", FragmentKey: "~/project/CLAUDE.md::_preamble"},         // project
		{Header: "API Keys", FragmentKey: "~/project/CLAUDE.md::api-keys"},            // project
	}
	p := NewPreview(sections)

	globalKeys := p.GlobalSelectedFragmentKeys()
	assert.Equal(t, []string{"_preamble", "git-commits"}, globalKeys)
	assert.Equal(t, 2, p.GlobalTotalCount())
}

func TestGlobalSelectedFragmentKeys_NoneGlobal(t *testing.T) {
	sections := []PreviewSection{
		{Header: "(preamble)", FragmentKey: "~/project/CLAUDE.md::_preamble"},
	}
	p := NewPreview(sections)

	assert.Nil(t, p.GlobalSelectedFragmentKeys())
	assert.Equal(t, 0, p.GlobalTotalCount())
}

func TestClaudeMDPreviewSections_GlobalVsProject(t *testing.T) {
	sections := []claudemd.Section{
		{Header: "", Content: "preamble content"},
		{Header: "Section A", Content: "## Section A\ncontent"},
	}

	// Global source: keys are unqualified.
	globalResult := ClaudeMDPreviewSections(sections, "~/.claude/CLAUDE.md")
	assert.Equal(t, "_preamble", globalResult[0].FragmentKey)
	assert.Equal(t, "section-a", globalResult[1].FragmentKey)

	// Project source: keys are source-qualified.
	projResult := ClaudeMDPreviewSections(sections, "~/project/CLAUDE.md")
	assert.Equal(t, "~/project/CLAUDE.md::_preamble", projResult[0].FragmentKey)
	assert.Equal(t, "~/project/CLAUDE.md::section-a", projResult[1].FragmentKey)
}

func TestPreviewRowCursorOnSearchAction(t *testing.T) {
	sections := []PreviewSection{
		{Header: "A", FragmentKey: "a", Source: "a.md"},
	}
	p := NewPreview(sections)
	require.Len(t, p.rows, 2) // 1 header + 1 section

	// rowCursor starts at 0 (header). Move to search action = len(rows) = 2.
	p.rowCursor = len(p.rows)
	p.syncViewport() // should not panic
}
