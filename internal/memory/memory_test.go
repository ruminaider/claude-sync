package memory_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ruminaider/claude-sync/internal/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWriteAndReadManifest(t *testing.T) {
	dir := t.TempDir()

	m := memory.Manifest{
		Fragments: map[string]memory.FragmentMeta{
			"coding-style": {
				Name:        "Coding Style",
				Description: "My preferred coding style",
				Type:        "user",
				Level:       "profile",
				ContentHash: "abc123",
			},
		},
		Order: []string{"coding-style"},
	}

	err := memory.WriteManifest(dir, m)
	require.NoError(t, err)

	got, err := memory.ReadManifest(dir)
	require.NoError(t, err)
	assert.Equal(t, m.Order, got.Order)
	assert.Equal(t, m.Fragments["coding-style"].Name, got.Fragments["coding-style"].Name)
	assert.Equal(t, m.Fragments["coding-style"].ContentHash, got.Fragments["coding-style"].ContentHash)
}

func TestReadManifestMissing(t *testing.T) {
	dir := t.TempDir()

	m, err := memory.ReadManifest(dir)
	require.NoError(t, err)
	assert.NotNil(t, m.Fragments)
	assert.Empty(t, m.Fragments)
	assert.Empty(t, m.Order)
}

func TestWriteAndReadFragment(t *testing.T) {
	dir := t.TempDir()

	content := "---\nname: Test Fragment\n---\n\nSome content here."
	err := memory.WriteFragment(dir, "test-fragment", content)
	require.NoError(t, err)

	// Verify the file exists at the expected path
	_, err = os.Stat(filepath.Join(dir, "test-fragment.md"))
	require.NoError(t, err)

	got, err := memory.ReadFragment(dir, "test-fragment")
	require.NoError(t, err)
	assert.Equal(t, content, got)
}

func TestContentHash(t *testing.T) {
	hash := memory.ContentHash("hello world")
	assert.Len(t, hash, 16)

	// Same input produces same hash
	assert.Equal(t, hash, memory.ContentHash("hello world"))

	// Different input produces different hash
	assert.NotEqual(t, hash, memory.ContentHash("hello world!"))
}

func TestParseFrontmatter(t *testing.T) {
	t.Run("valid frontmatter", func(t *testing.T) {
		content := "---\nname: My Fragment\ndescription: A test fragment\ntype: user\n---\n\nBody content here."
		fm, err := memory.ParseFrontmatter(content)
		require.NoError(t, err)
		assert.Equal(t, "My Fragment", fm.Name)
		assert.Equal(t, "A test fragment", fm.Description)
		assert.Equal(t, "user", fm.Type)
	})

	t.Run("no frontmatter", func(t *testing.T) {
		_, err := memory.ParseFrontmatter("Just some text without frontmatter")
		assert.Error(t, err)
	})

	t.Run("missing closing delimiter", func(t *testing.T) {
		_, err := memory.ParseFrontmatter("---\nname: Incomplete\nno closing")
		assert.Error(t, err)
	})
}

func TestRegenerateIndex(t *testing.T) {
	dir := t.TempDir()

	// Write a synced fragment (has frontmatter)
	syncedContent := "---\nname: Coding Style\ndescription: My preferred coding style\ntype: user\n---\n\nI prefer short functions."
	require.NoError(t, os.WriteFile(filepath.Join(dir, "coding-style.md"), []byte(syncedContent), 0o644))

	// Write a local-only fragment (has frontmatter with different type)
	localContent := "---\nname: Project Architecture\ndescription: How the project is structured\ntype: project\n---\n\nThe project uses hexagonal architecture."
	require.NoError(t, os.WriteFile(filepath.Join(dir, "project-architecture.md"), []byte(localContent), 0o644))

	err := memory.RegenerateIndex(dir)
	require.NoError(t, err)

	// Read the generated MEMORY.md
	data, err := os.ReadFile(filepath.Join(dir, "MEMORY.md"))
	require.NoError(t, err)
	content := string(data)

	// Verify header
	assert.Contains(t, content, "# Memory\n")

	// Verify both fragments appear
	assert.Contains(t, content, "[Coding Style](coding-style.md): My preferred coding style")
	assert.Contains(t, content, "[Project Architecture](project-architecture.md): How the project is structured")

	// Verify type sections exist
	assert.Contains(t, content, "## User\n")
	assert.Contains(t, content, "## Project\n")

	// Verify User section comes before Project section
	userIdx := strings.Index(content, "## User")
	projectIdx := strings.Index(content, "## Project")
	assert.Greater(t, projectIdx, userIdx, "User section should come before Project section")
}

func TestSlugifyName(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"Coding Style", "coding-style"},
		{"My Cool Fragment!", "my-cool-fragment"},
		{"already-slugified", "already-slugified"},
		{"UPPER CASE", "upper-case"},
		{"special@chars#here", "specialcharshere"},
		{"multiple   spaces", "multiple-spaces"},
		{"@@@", "unnamed"},
		{"", "unnamed"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"---dashes---", "dashes"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			assert.Equal(t, tt.expected, memory.SlugifyName(tt.input))
		})
	}
}
