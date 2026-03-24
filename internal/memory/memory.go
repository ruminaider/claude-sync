package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// FragmentMeta holds metadata about a single memory fragment in the manifest.
type FragmentMeta struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
	Level       string `yaml:"level"`
	ContentHash string `yaml:"content_hash"`
}

// Manifest tracks all memory fragments and their ordering.
type Manifest struct {
	Fragments map[string]FragmentMeta `yaml:"fragments"`
	Order     []string               `yaml:"order"`
}

// Frontmatter represents the YAML frontmatter parsed from a memory fragment file.
type Frontmatter struct {
	Name        string `yaml:"name"`
	Description string `yaml:"description"`
	Type        string `yaml:"type"`
}

const manifestFile = "manifest.yaml"

// WriteManifest writes the manifest to dir/manifest.yaml.
func WriteManifest(dir string, m Manifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return fmt.Errorf("marshal manifest: %w", err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, manifestFile), data, 0o644)
}

// ReadManifest reads the manifest from dir/manifest.yaml.
// Returns an empty manifest if the file does not exist.
func ReadManifest(dir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, manifestFile))
	if err != nil {
		if os.IsNotExist(err) {
			return Manifest{Fragments: map[string]FragmentMeta{}}, nil
		}
		return Manifest{}, fmt.Errorf("read manifest: %w", err)
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, fmt.Errorf("unmarshal manifest: %w", err)
	}
	if m.Fragments == nil {
		m.Fragments = map[string]FragmentMeta{}
	}
	return m, nil
}

// WriteFragment writes content to dir/name.md.
func WriteFragment(dir, name, content string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create dir %s: %w", dir, err)
	}
	return os.WriteFile(filepath.Join(dir, name+".md"), []byte(content), 0o644)
}

// ReadFragment reads content from dir/name.md.
func ReadFragment(dir, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(dir, name+".md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ContentHash returns the first 16 hex characters of the SHA-256 hash of content.
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

// ParseFrontmatter extracts YAML frontmatter from content delimited by --- lines.
func ParseFrontmatter(content string) (Frontmatter, error) {
	lines := strings.Split(content, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return Frontmatter{}, fmt.Errorf("no frontmatter delimiter found")
	}

	endIdx := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			endIdx = i
			break
		}
	}
	if endIdx < 0 {
		return Frontmatter{}, fmt.Errorf("no closing frontmatter delimiter found")
	}

	yamlContent := strings.Join(lines[1:endIdx], "\n")
	var fm Frontmatter
	if err := yaml.Unmarshal([]byte(yamlContent), &fm); err != nil {
		return Frontmatter{}, fmt.Errorf("unmarshal frontmatter: %w", err)
	}
	return fm, nil
}

// ImportResult holds the outcome of an ImportFromDir operation.
type ImportResult struct {
	Imported []string // slugified fragment names that were imported
}

// ImportFromDir scans sourceDir for .md files, parses their frontmatter,
// and writes them as fragments into syncMemDir. It skips MEMORY.md and directories.
// Collisions are disambiguated with -2, -3, etc.
func ImportFromDir(sourceDir, syncMemDir string) (*ImportResult, error) {
	entries, err := os.ReadDir(sourceDir)
	if err != nil {
		return nil, fmt.Errorf("read source dir %s: %w", sourceDir, err)
	}

	manifest, err := ReadManifest(syncMemDir)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}

	var imported []string
	usedSlugs := make(map[string]bool)
	for slug := range manifest.Fragments {
		usedSlugs[slug] = true
	}

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

		content, err := os.ReadFile(filepath.Join(sourceDir, name))
		if err != nil {
			return nil, fmt.Errorf("read file %s: %w", name, err)
		}

		fm, err := ParseFrontmatter(string(content))
		if err != nil {
			// Use filename without extension as name if no frontmatter
			fm.Name = strings.TrimSuffix(name, ".md")
		}

		slug := SlugifyName(fm.Name)
		finalSlug := slug
		counter := 2
		for usedSlugs[finalSlug] {
			finalSlug = fmt.Sprintf("%s-%d", slug, counter)
			counter++
		}
		usedSlugs[finalSlug] = true

		if err := WriteFragment(syncMemDir, finalSlug, string(content)); err != nil {
			return nil, fmt.Errorf("write fragment %s: %w", finalSlug, err)
		}

		manifest.Fragments[finalSlug] = FragmentMeta{
			Name:        fm.Name,
			Description: fm.Description,
			Type:        fm.Type,
			Level:       "profile",
			ContentHash: ContentHash(string(content)),
		}
		manifest.Order = append(manifest.Order, finalSlug)
		imported = append(imported, finalSlug)
	}

	if err := WriteManifest(syncMemDir, manifest); err != nil {
		return nil, fmt.Errorf("write manifest: %w", err)
	}

	return &ImportResult{Imported: imported}, nil
}

var nonAlphanumHyphen = regexp.MustCompile(`[^a-z0-9-]`)

// SlugifyName lowercases the input, replaces spaces with hyphens,
// and removes non-alphanumeric-hyphen characters.
func SlugifyName(name string) string {
	s := strings.ToLower(name)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumHyphen.ReplaceAllString(s, "")
	return s
}
