package claudemd_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ruminaider/claude-sync/internal/claudemd"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSplit(t *testing.T) {
	t.Run("with preamble", func(t *testing.T) {
		content := "Some preamble text\nMore preamble\n## Section One\nContent one\n## Section Two\nContent two"
		sections := claudemd.Split(content)
		require.Len(t, sections, 3)

		assert.Equal(t, "", sections[0].Header)
		assert.Equal(t, "Some preamble text\nMore preamble", sections[0].Content)

		assert.Equal(t, "Section One", sections[1].Header)
		assert.Equal(t, "## Section One\nContent one", sections[1].Content)

		assert.Equal(t, "Section Two", sections[2].Header)
		assert.Equal(t, "## Section Two\nContent two", sections[2].Content)
	})

	t.Run("no preamble", func(t *testing.T) {
		content := "## First\nContent A\n## Second\nContent B"
		sections := claudemd.Split(content)
		require.Len(t, sections, 2)

		assert.Equal(t, "First", sections[0].Header)
		assert.Equal(t, "## First\nContent A", sections[0].Content)

		assert.Equal(t, "Second", sections[1].Header)
		assert.Equal(t, "## Second\nContent B", sections[1].Content)
	})

	t.Run("empty input", func(t *testing.T) {
		assert.Nil(t, claudemd.Split(""))
		assert.Nil(t, claudemd.Split("   \n  \n  "))
	})

	t.Run("single section", func(t *testing.T) {
		content := "## Only Section\nSome content here"
		sections := claudemd.Split(content)
		require.Len(t, sections, 1)
		assert.Equal(t, "Only Section", sections[0].Header)
		assert.Equal(t, "## Only Section\nSome content here", sections[0].Content)
	})

	t.Run("multiple sections", func(t *testing.T) {
		content := "## A\nOne\n## B\nTwo\n## C\nThree"
		sections := claudemd.Split(content)
		require.Len(t, sections, 3)
		assert.Equal(t, "A", sections[0].Header)
		assert.Equal(t, "B", sections[1].Header)
		assert.Equal(t, "C", sections[2].Header)
	})

	t.Run("splits on ### within ## but not ##word", func(t *testing.T) {
		content := "## Real Section\nContent\n### Subsection\nMore\n##notsplit\nEnd"
		sections := claudemd.Split(content)
		require.Len(t, sections, 2)

		assert.Equal(t, "Real Section", sections[0].Header)
		assert.Equal(t, "## Real Section\nContent", sections[0].Content)
		assert.Equal(t, "", sections[0].Group)

		assert.Equal(t, "Subsection", sections[1].Header)
		assert.Equal(t, "### Subsection\nMore\n##notsplit\nEnd", sections[1].Content)
		assert.Equal(t, "real-section", sections[1].Group)
	})
}

func TestAssemble(t *testing.T) {
	t.Run("basic assemble", func(t *testing.T) {
		sections := []claudemd.Section{
			{Header: "", Content: "Preamble"},
			{Header: "A", Content: "## A\nContent A"},
		}
		result := claudemd.Assemble(sections)
		assert.Equal(t, "Preamble\n## A\nContent A", result)
	})

	t.Run("empty sections", func(t *testing.T) {
		assert.Equal(t, "", claudemd.Assemble(nil))
	})
}

func TestSplitAssembleRoundTrip(t *testing.T) {
	content := "Preamble text\n## Section One\nContent one\n## Section Two\nContent two"
	sections := claudemd.Split(content)
	reassembled := claudemd.Assemble(sections)
	sections2 := claudemd.Split(reassembled)
	require.Equal(t, len(sections), len(sections2))
	for i := range sections {
		assert.Equal(t, sections[i].Header, sections2[i].Header)
		assert.Equal(t, sections[i].Content, sections2[i].Content)
	}
}

func TestHeaderToFragmentName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Git Conventions", "git-conventions"},
		{"README Writing", "readme-writing"},
		{"", "_preamble"},
		{"Tool Authentication Requirements (CRITICAL)", "tool-authentication-requirements-critical"},
		{"UPPER CASE", "upper-case"},
		{"special!@#chars", "specialchars"},
		{"  spaces  around  ", "spaces-around"},
		{"multiple---hyphens", "multiple-hyphens"},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, claudemd.HeaderToFragmentName(tt.input))
		})
	}
}

func TestContentHash(t *testing.T) {
	t.Run("determinism", func(t *testing.T) {
		h1 := claudemd.ContentHash("hello world")
		h2 := claudemd.ContentHash("hello world")
		assert.Equal(t, h1, h2)
	})

	t.Run("length", func(t *testing.T) {
		h := claudemd.ContentHash("test content")
		assert.Len(t, h, 16)
	})

	t.Run("uniqueness", func(t *testing.T) {
		h1 := claudemd.ContentHash("content A")
		h2 := claudemd.ContentHash("content B")
		assert.NotEqual(t, h1, h2)
	})
}

func TestManifestIO(t *testing.T) {
	t.Run("write and read round-trip", func(t *testing.T) {
		dir := t.TempDir()
		m := claudemd.Manifest{
			Fragments: map[string]claudemd.FragmentMeta{
				"section-one": {Header: "Section One", ContentHash: "abc123"},
				"section-two": {Header: "Section Two", ContentHash: "def456"},
			},
			Order: []string{"section-one", "section-two"},
		}

		err := claudemd.WriteManifest(dir, m)
		require.NoError(t, err)

		got, err := claudemd.ReadManifest(dir)
		require.NoError(t, err)
		assert.Equal(t, m.Order, got.Order)
		assert.Equal(t, m.Fragments["section-one"], got.Fragments["section-one"])
		assert.Equal(t, m.Fragments["section-two"], got.Fragments["section-two"])
	})

	t.Run("missing file returns empty manifest", func(t *testing.T) {
		dir := t.TempDir()
		m, err := claudemd.ReadManifest(dir)
		require.NoError(t, err)
		assert.NotNil(t, m.Fragments)
		assert.Empty(t, m.Fragments)
		assert.Empty(t, m.Order)
	})
}

func TestFragmentIO(t *testing.T) {
	dir := t.TempDir()
	content := "## Test Section\nSome content here"

	err := claudemd.WriteFragment(dir, "test-section", content)
	require.NoError(t, err)

	got, err := claudemd.ReadFragment(dir, "test-section")
	require.NoError(t, err)
	assert.Equal(t, content, got)

	// Verify file exists at expected path.
	_, err = os.Stat(filepath.Join(dir, "test-section.md"))
	assert.NoError(t, err)
}

func TestImportClaudeMD(t *testing.T) {
	t.Run("import multiple sections", func(t *testing.T) {
		syncDir := t.TempDir()
		content := "Preamble text\n## Git Conventions\nUse conventional commits\n## README Writing\nStructure READMEs well"

		result, err := claudemd.ImportClaudeMD(syncDir, content)
		require.NoError(t, err)

		assert.Equal(t, []string{"_preamble", "git-conventions", "readme-writing"}, result.FragmentNames)

		// Verify fragment files exist.
		claudeMdDir := filepath.Join(syncDir, "claude-md")
		for _, name := range result.FragmentNames {
			_, err := os.Stat(filepath.Join(claudeMdDir, name+".md"))
			assert.NoError(t, err, "fragment file %s should exist", name)
		}

		// Verify manifest.
		manifest, err := claudemd.ReadManifest(claudeMdDir)
		require.NoError(t, err)
		assert.Equal(t, []string{"_preamble", "git-conventions", "readme-writing"}, manifest.Order)
		assert.Len(t, manifest.Fragments, 3)
		assert.Equal(t, "Git Conventions", manifest.Fragments["git-conventions"].Header)
	})
}

func TestAssembleFromDir(t *testing.T) {
	t.Run("assemble after import", func(t *testing.T) {
		syncDir := t.TempDir()
		content := "Preamble\n## Alpha\nAlpha content\n## Beta\nBeta content"

		_, err := claudemd.ImportClaudeMD(syncDir, content)
		require.NoError(t, err)

		assembled, err := claudemd.AssembleFromDir(syncDir, []string{"_preamble", "alpha", "beta"})
		require.NoError(t, err)
		assert.Equal(t, content, assembled)
	})

	t.Run("partial include list", func(t *testing.T) {
		syncDir := t.TempDir()
		content := "## Alpha\nAlpha content\n## Beta\nBeta content\n## Gamma\nGamma content"

		_, err := claudemd.ImportClaudeMD(syncDir, content)
		require.NoError(t, err)

		assembled, err := claudemd.AssembleFromDir(syncDir, []string{"alpha", "gamma"})
		require.NoError(t, err)
		assert.Equal(t, "## Alpha\nAlpha content\n## Gamma\nGamma content", assembled)
	})

	t.Run("missing fragment error", func(t *testing.T) {
		syncDir := t.TempDir()
		content := "## Only\nContent"
		_, err := claudemd.ImportClaudeMD(syncDir, content)
		require.NoError(t, err)

		_, err = claudemd.AssembleFromDir(syncDir, []string{"nonexistent"})
		assert.Error(t, err)
	})
}

func TestContentSimilarity(t *testing.T) {
	t.Run("identical", func(t *testing.T) {
		assert.Equal(t, 1.0, claudemd.ContentSimilarity("hello world foo", "hello world foo"))
	})

	t.Run("similar", func(t *testing.T) {
		a := "## Git Conventions\nUse conventional commits for all changes in this project"
		b := "## Git Rules\nUse conventional commits for all changes in this project"
		sim := claudemd.ContentSimilarity(a, b)
		assert.Greater(t, sim, 0.8)
	})

	t.Run("different", func(t *testing.T) {
		a := "The quick brown fox jumps over the lazy dog"
		b := "A completely different sentence with unique words and meaning"
		sim := claudemd.ContentSimilarity(a, b)
		assert.Less(t, sim, 0.5)
	})

	t.Run("both empty", func(t *testing.T) {
		assert.Equal(t, 1.0, claudemd.ContentSimilarity("", ""))
	})
}

func TestSplitWithSubSections(t *testing.T) {
	content := "## Work Style\n\nIntro text.\n\n### Git Commits\n\nDo NOT include Co-Authored-By.\n\n### Verification\n\nDiff behavior."
	sections := claudemd.Split(content)
	require.Len(t, sections, 3)

	assert.Equal(t, "Work Style", sections[0].Header)
	assert.Equal(t, "## Work Style\n\nIntro text.", sections[0].Content)
	assert.Equal(t, "", sections[0].Group)

	assert.Equal(t, "Git Commits", sections[1].Header)
	assert.Equal(t, "### Git Commits\n\nDo NOT include Co-Authored-By.", sections[1].Content)
	assert.Equal(t, "work-style", sections[1].Group)

	assert.Equal(t, "Verification", sections[2].Header)
	assert.Equal(t, "### Verification\n\nDiff behavior.", sections[2].Content)
	assert.Equal(t, "work-style", sections[2].Group)
}

func TestSplitPreservesExistingBehavior(t *testing.T) {
	// No ### headers, should work exactly as before
	content := "## First\nContent A\n## Second\nContent B"
	sections := claudemd.Split(content)
	require.Len(t, sections, 2)
	assert.Equal(t, "", sections[0].Group)
	assert.Equal(t, "", sections[1].Group)
}

func TestSplitSubSectionBeforeAnyH2(t *testing.T) {
	// ### before any ## should be treated as top-level
	content := "### Orphan Sub\nOrphan content\n## Real Section\nContent"
	sections := claudemd.Split(content)
	require.Len(t, sections, 2)

	assert.Equal(t, "Orphan Sub", sections[0].Header)
	assert.Equal(t, "", sections[0].Group)

	assert.Equal(t, "Real Section", sections[1].Header)
	assert.Equal(t, "", sections[1].Group)
}

func TestChildFragmentName(t *testing.T) {
	assert.Equal(t, "work-style--git-commits",
		claudemd.ChildFragmentName("work-style", "Git Commits"))
}

func TestSectionFragmentName(t *testing.T) {
	assert.Equal(t, "git-commits",
		claudemd.SectionFragmentName(claudemd.Section{Header: "Git Commits"}))
	assert.Equal(t, "work-style--git-commits",
		claudemd.SectionFragmentName(claudemd.Section{Header: "Git Commits", Group: "work-style"}))
	assert.Equal(t, "_preamble",
		claudemd.SectionFragmentName(claudemd.Section{Header: ""}))
}

func TestParseQualifiedKey(t *testing.T) {
	t.Run("global key", func(t *testing.T) {
		source, frag, isProj := claudemd.ParseQualifiedKey("git-commits")
		assert.Equal(t, "", source)
		assert.Equal(t, "git-commits", frag)
		assert.False(t, isProj)
	})

	t.Run("project key", func(t *testing.T) {
		source, frag, isProj := claudemd.ParseQualifiedKey("~/Work/evvy/CLAUDE.md::beads-issue-tracking")
		assert.Equal(t, "~/Work/evvy/CLAUDE.md", source)
		assert.Equal(t, "beads-issue-tracking", frag)
		assert.True(t, isProj)
	})
}

func TestProjectFragmentFilename(t *testing.T) {
	name := claudemd.ProjectFragmentFilename("~/Work/evvy/CLAUDE.md::beads-issue-tracking")
	assert.Contains(t, name, "proj--")
	assert.Contains(t, name, "--beads-issue-tracking")
	assert.Len(t, name, len("proj--12345678--beads-issue-tracking"))

	// Deterministic: same input produces same output.
	name2 := claudemd.ProjectFragmentFilename("~/Work/evvy/CLAUDE.md::beads-issue-tracking")
	assert.Equal(t, name, name2)

	// Different source produces different prefix.
	name3 := claudemd.ProjectFragmentFilename("~/other/project/CLAUDE.md::beads-issue-tracking")
	assert.NotEqual(t, name, name3)
}

func TestReadFragmentWithQualifiedKey(t *testing.T) {
	dir := t.TempDir()
	key := "~/Work/evvy/CLAUDE.md::testing-rules"
	content := "## Testing Rules\n\nAlways write tests."

	// Write using the resolved filename, then read back via qualified key.
	filename := claudemd.ProjectFragmentFilename(key)
	require.NoError(t, claudemd.WriteFragment(dir, filename, content))

	got, err := claudemd.ReadFragment(dir, key)
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestDirContentHash(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.md"), []byte("hello"), 0644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("world"), 0644))

	hash1, err := claudemd.DirContentHash(dir)
	require.NoError(t, err)
	assert.Len(t, hash1, 16)

	// Same content produces same hash.
	hash2, err := claudemd.DirContentHash(dir)
	require.NoError(t, err)
	assert.Equal(t, hash1, hash2)

	// Modifying a file changes the hash.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.md"), []byte("changed"), 0644))
	hash3, err := claudemd.DirContentHash(dir)
	require.NoError(t, err)
	assert.NotEqual(t, hash1, hash3)
}

func TestImportProjectFragments(t *testing.T) {
	syncDir := t.TempDir()
	source := "~/Work/evvy/CLAUDE.md"
	content := "## Testing Rules\n\nAlways write tests.\n\n## Docker Setup\n\nUse compose.\n"
	selectedKeys := []string{
		source + "::testing-rules",
	}

	err := claudemd.ImportProjectFragments(syncDir, source, content, selectedKeys)
	require.NoError(t, err)

	// The selected fragment should be readable.
	key := source + "::testing-rules"
	got, err := claudemd.ReadFragment(filepath.Join(syncDir, "claude-md"), key)
	require.NoError(t, err)
	assert.Contains(t, got, "Testing Rules")

	// The non-selected fragment should not exist.
	key2 := source + "::docker-setup"
	_, err = claudemd.ReadFragment(filepath.Join(syncDir, "claude-md"), key2)
	assert.Error(t, err)

	// Manifest should have the fragment metadata.
	manifest, err := claudemd.ReadManifest(filepath.Join(syncDir, "claude-md"))
	require.NoError(t, err)
	filename := claudemd.ProjectFragmentFilename(key)
	meta, ok := manifest.Fragments[filename]
	assert.True(t, ok)
	assert.Equal(t, "Testing Rules", meta.Header)
	assert.Equal(t, source, meta.Source)
}
