package memory

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// NewFragment represents a fragment file found locally that is not in the manifest.
type NewFragment struct {
	SlugName string
	FilePath string
	Content  string
}

// ReconcileResult holds the outcome of a Reconcile operation.
type ReconcileResult struct {
	Updated []string      // slugified names of fragments whose content changed
	New     []NewFragment // fragments found locally but not in manifest
	Deleted []string      // slugified names in manifest but missing locally
}

// Reconcile compares local .md files in sourceDir against the manifest in syncMemDir.
// It detects updated, new, and deleted fragments.
// Updated fragments have their content written to syncMemDir.
func Reconcile(sourceDir, syncMemDir string) (*ReconcileResult, error) {
	manifest, err := ReadManifest(syncMemDir)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("read source dir %s: %w", sourceDir, err)
	}

	// Build a map of slug -> content from local files
	localFragments := make(map[string]struct {
		filePath string
		content  string
	})

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".md") {
			continue
		}
		if strings.EqualFold(name, "MEMORY.md") {
			continue
		}

		filePath := filepath.Join(sourceDir, name)
		content, err := os.ReadFile(filePath)
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", name, err)
		}

		fm, err := ParseFrontmatter(string(content))
		if err != nil {
			fm.Name = strings.TrimSuffix(name, ".md")
		}

		slug := SlugifyName(fm.Name)
		localFragments[slug] = struct {
			filePath string
			content  string
		}{filePath: filePath, content: string(content)}
	}

	result := &ReconcileResult{}

	// Check manifest entries against local files
	seen := make(map[string]bool)
	for slug, meta := range manifest.Fragments {
		seen[slug] = true
		local, exists := localFragments[slug]
		if !exists {
			result.Deleted = append(result.Deleted, slug)
			continue
		}
		// Check if content changed
		if ContentHash(local.content) != meta.ContentHash {
			result.Updated = append(result.Updated, slug)
			// Write updated content to syncMemDir
			if err := WriteFragment(syncMemDir, slug, local.content); err != nil {
				return nil, fmt.Errorf("write updated fragment %s: %w", slug, err)
			}
			// Update the manifest entry with the new hash
			meta.ContentHash = ContentHash(local.content)
			manifest.Fragments[slug] = meta
		}
	}

	// Check for new fragments not in manifest
	for slug, local := range localFragments {
		if !seen[slug] {
			result.New = append(result.New, NewFragment{
				SlugName: slug,
				FilePath: local.filePath,
				Content:  local.content,
			})
		}
	}

	// Persist updated manifest so content hashes stay current
	if len(result.Updated) > 0 {
		if err := WriteManifest(syncMemDir, manifest); err != nil {
			return nil, fmt.Errorf("writing updated manifest: %w", err)
		}
	}

	return result, nil
}
