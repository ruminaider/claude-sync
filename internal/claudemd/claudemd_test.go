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

	t.Run("does not split on ### or ##word", func(t *testing.T) {
		content := "## Real Section\nContent\n### Subsection\nMore\n##notsplit\nEnd"
		sections := claudemd.Split(content)
		require.Len(t, sections, 1)
		assert.Equal(t, "Real Section", sections[0].Header)
		assert.Contains(t, sections[0].Content, "### Subsection")
		assert.Contains(t, sections[0].Content, "##notsplit")
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
