package claudemd

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"go.yaml.in/yaml/v3"
)

// claudeMdSubdir is the subdirectory within a sync dir that holds fragments.
const claudeMdSubdir = "claude-md"

// Section represents a section of a CLAUDE.md file, split on ## headers.
type Section struct {
	Header  string // "" for preamble (content before first ## header)
	Content string // the full content of the section including the ## header line
	Group   string // parent fragment name for ### sub-sections; empty for top-level
	Source  string // source file path (e.g., "~/Work/evvy/CLAUDE.md"); empty for global
}

// Split splits markdown content on "## " and "### " headers.
// Content before the first "## " becomes a preamble section with empty Header.
// A "### " header within a "## " section creates a sub-section whose Group is
// set to HeaderToFragmentName(parentHeader). The parent section's content is
// truncated at the first "### " boundary.
// If "### " appears before any "## ", it is treated as top-level (Group = "").
// Empty/whitespace-only input returns nil.
func Split(content string) []Section {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	var sections []Section
	lines := strings.Split(content, "\n")
	var current []string
	currentHeader := ""
	currentGroup := ""
	parentHeader := "" // tracks the active ## header for grouping ### sections
	inPreamble := true

	flush := func() {
		if len(current) > 0 {
			sections = append(sections, Section{
				Header:  currentHeader,
				Content: strings.TrimRight(strings.Join(current, "\n"), "\n"),
				Group:   currentGroup,
			})
		}
	}

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") && !strings.HasPrefix(line, "### ") {
			// Flush previous section
			flush()
			inPreamble = false
			currentHeader = strings.TrimPrefix(line, "## ")
			parentHeader = currentHeader
			currentGroup = ""
			current = []string{line}
		} else if strings.HasPrefix(line, "### ") {
			// Flush previous section
			flush()
			currentHeader = strings.TrimPrefix(line, "### ")
			if inPreamble {
				currentGroup = ""
			} else {
				currentGroup = HeaderToFragmentName(parentHeader)
			}
			current = []string{line}
		} else {
			current = append(current, line)
		}
	}

	// Flush last section
	flush()

	return sections
}

// ChildFragmentName creates a fragment name for a ### sub-section.
func ChildFragmentName(parentName, childHeader string) string {
	return parentName + "--" + HeaderToFragmentName(childHeader)
}

// Assemble concatenates sections with a single newline separator between them.
func Assemble(sections []Section) string {
	if len(sections) == 0 {
		return ""
	}
	parts := make([]string, len(sections))
	for i, s := range sections {
		parts[i] = s.Content
	}
	return strings.Join(parts, "\n")
}

var nonAlphanumHyphen = regexp.MustCompile(`[^a-z0-9-]`)
var multiHyphen = regexp.MustCompile(`-{2,}`)

// HeaderToFragmentName converts a section header to a filename-safe fragment name.
func HeaderToFragmentName(header string) string {
	if header == "" {
		return "_preamble"
	}
	s := strings.ToLower(header)
	s = strings.ReplaceAll(s, " ", "-")
	s = nonAlphanumHyphen.ReplaceAllString(s, "")
	s = multiHyphen.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	return s
}

// ContentHash returns the first 16 hex characters of the SHA-256 hash of content.
func ContentHash(content string) string {
	h := sha256.Sum256([]byte(content))
	return hex.EncodeToString(h[:])[:16]
}

// DirContentHash computes a combined hash of all files in a directory.
// Files are sorted by relative path for determinism. Directories named
// .git, node_modules, and __pycache__ are skipped, as are .DS_Store files.
func DirContentHash(dir string) (string, error) {
	var files []string
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		name := d.Name()
		if d.IsDir() {
			if name == ".git" || name == "node_modules" || name == "__pycache__" {
				return filepath.SkipDir
			}
			return nil
		}
		if name == ".DS_Store" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		files = append(files, rel)
		return nil
	})
	if err != nil {
		return "", err
	}

	sort.Strings(files)

	h := sha256.New()
	for _, rel := range files {
		h.Write([]byte(rel))
		h.Write([]byte{0})
		data, err := os.ReadFile(filepath.Join(dir, rel))
		if err != nil {
			return "", err
		}
		h.Write(data)
		h.Write([]byte{0})
	}

	return hex.EncodeToString(h.Sum(nil))[:16], nil
}

// FragmentMeta holds metadata about a single fragment.
type FragmentMeta struct {
	Header       string `yaml:"header"`
	ContentHash  string `yaml:"content_hash"`
	Group        string `yaml:"group,omitempty"`
	Source       string `yaml:"source,omitempty"`        // source file path for project fragments
	QualifiedKey string `yaml:"qualified_key,omitempty"` // original qualified key for project fragments
}

// Manifest tracks all fragments and their order.
type Manifest struct {
	Fragments map[string]FragmentMeta `yaml:"fragments"`
	Order     []string               `yaml:"order"`
}

// WriteManifest writes the manifest to claudeMdDir/manifest.yaml.
func WriteManifest(claudeMdDir string, m Manifest) error {
	data, err := yaml.Marshal(m)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(claudeMdDir, "manifest.yaml"), data, 0644)
}

// ReadManifest reads the manifest from claudeMdDir/manifest.yaml.
// Returns an empty manifest with initialized Fragments map if the file doesn't exist.
func ReadManifest(claudeMdDir string) (Manifest, error) {
	data, err := os.ReadFile(filepath.Join(claudeMdDir, "manifest.yaml"))
	if os.IsNotExist(err) {
		return Manifest{Fragments: make(map[string]FragmentMeta)}, nil
	}
	if err != nil {
		return Manifest{}, err
	}
	var m Manifest
	if err := yaml.Unmarshal(data, &m); err != nil {
		return Manifest{}, err
	}
	if m.Fragments == nil {
		m.Fragments = make(map[string]FragmentMeta)
	}
	return m, nil
}

// WriteFragment writes content to claudeMdDir/name.md.
func WriteFragment(claudeMdDir, name, content string) error {
	return os.WriteFile(filepath.Join(claudeMdDir, name+".md"), []byte(content), 0644)
}

// ReadFragment reads content from claudeMdDir/name.md.
// For qualified keys (containing "::"), it resolves the project fragment filename.
func ReadFragment(claudeMdDir, name string) (string, error) {
	filename := name
	if _, _, isProj := ParseQualifiedKey(name); isProj {
		filename = ProjectFragmentFilename(name)
	}
	data, err := os.ReadFile(filepath.Join(claudeMdDir, filename+".md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
}

// ParseQualifiedKey splits a fragment key on "::" into source and fragment name.
// For unqualified keys (no "::"), isProject is false and fragment is the key itself.
func ParseQualifiedKey(key string) (source, fragment string, isProject bool) {
	if idx := strings.Index(key, "::"); idx >= 0 {
		return key[:idx], key[idx+2:], true
	}
	return "", key, false
}

// ProjectFragmentFilename returns the storage filename (without .md extension)
// for a project fragment. The format is proj--<8-char source hash>--<fragment>.
func ProjectFragmentFilename(qualifiedKey string) string {
	source, fragment, _ := ParseQualifiedKey(qualifiedKey)
	h := sha256.Sum256([]byte(source))
	prefix := hex.EncodeToString(h[:])[:8]
	return "proj--" + prefix + "--" + fragment
}

// ReadProjectFragment reads a project fragment from the claude-md directory.
func ReadProjectFragment(claudeMdDir, qualifiedKey string) (string, error) {
	filename := ProjectFragmentFilename(qualifiedKey)
	return ReadFragment(claudeMdDir, filename)
}

// WriteProjectFragment writes a project fragment to the claude-md directory.
func WriteProjectFragment(claudeMdDir, qualifiedKey, content string) error {
	filename := ProjectFragmentFilename(qualifiedKey)
	return WriteFragment(claudeMdDir, filename, content)
}

// ImportProjectFragments reads a project CLAUDE.md file and exports selected
// sections to the sync repo's claude-md/ directory. It updates the manifest
// with metadata for each exported fragment.
func ImportProjectFragments(syncDir, sourcePath, content string, selectedKeys []string) error {
	claudeMdDir := filepath.Join(syncDir, claudeMdSubdir)
	if err := os.MkdirAll(claudeMdDir, 0755); err != nil {
		return err
	}

	// Build a set of selected fragment names for this source.
	selectedFragments := make(map[string]bool)
	for _, key := range selectedKeys {
		src, frag, isProj := ParseQualifiedKey(key)
		if isProj && src == sourcePath {
			selectedFragments[frag] = true
		}
	}

	if len(selectedFragments) == 0 {
		return nil
	}

	sections := Split(content)
	manifest, err := ReadManifest(claudeMdDir)
	if err != nil {
		return err
	}

	for _, sec := range sections {
		fragName := HeaderToFragmentName(sec.Header)
		if sec.Group != "" {
			fragName = ChildFragmentName(sec.Group, sec.Header)
		}
		if !selectedFragments[fragName] {
			continue
		}

		qualifiedKey := sourcePath + "::" + fragName
		filename := ProjectFragmentFilename(qualifiedKey)

		manifest.Fragments[filename] = FragmentMeta{
			Header:      sec.Header,
			ContentHash: ContentHash(sec.Content),
			Group:       sec.Group,
			Source:      sourcePath,
			QualifiedKey: qualifiedKey,
		}
		if err := WriteFragment(claudeMdDir, filename, sec.Content); err != nil {
			return err
		}
	}

	return WriteManifest(claudeMdDir, manifest)
}

// ImportResult holds the result of importing a CLAUDE.md file.
type ImportResult struct {
	FragmentNames []string
}

// ImportClaudeMD splits content into sections and writes fragment files + manifest
// into syncDir/claude-md/.
func ImportClaudeMD(syncDir, content string) (*ImportResult, error) {
	claudeMdDir := filepath.Join(syncDir, claudeMdSubdir)
	if err := os.MkdirAll(claudeMdDir, 0755); err != nil {
		return nil, err
	}

	sections := Split(content)
	manifest := Manifest{
		Fragments: make(map[string]FragmentMeta),
	}
	var names []string

	for _, sec := range sections {
		var name string
		if sec.Group != "" {
			name = ChildFragmentName(sec.Group, sec.Header)
		} else {
			name = HeaderToFragmentName(sec.Header)
		}
		names = append(names, name)
		manifest.Order = append(manifest.Order, name)
		manifest.Fragments[name] = FragmentMeta{
			Header:      sec.Header,
			ContentHash: ContentHash(sec.Content),
			Group:       sec.Group,
		}
		if err := WriteFragment(claudeMdDir, name, sec.Content); err != nil {
			return nil, err
		}
	}

	if err := WriteManifest(claudeMdDir, manifest); err != nil {
		return nil, err
	}

	return &ImportResult{FragmentNames: names}, nil
}

// AssembleFromDir reads fragment files listed in include from syncDir/claude-md/
// and concatenates them using Assemble.
func AssembleFromDir(syncDir string, include []string) (string, error) {
	claudeMdDir := filepath.Join(syncDir, claudeMdSubdir)
	var sections []Section

	for _, name := range include {
		content, err := ReadFragment(claudeMdDir, name)
		if err != nil {
			return "", err
		}
		// Recover the header from the content to build a proper Section
		sec := Section{Content: content}
		if strings.HasPrefix(content, "### ") {
			if idx := strings.Index(content, "\n"); idx != -1 {
				sec.Header = strings.TrimPrefix(content[:idx], "### ")
			} else {
				sec.Header = strings.TrimPrefix(content, "### ")
			}
		} else if strings.HasPrefix(content, "## ") {
			if idx := strings.Index(content, "\n"); idx != -1 {
				sec.Header = strings.TrimPrefix(content[:idx], "## ")
			} else {
				sec.Header = strings.TrimPrefix(content, "## ")
			}
		}
		sections = append(sections, sec)
	}

	return Assemble(sections), nil
}

// ContentSimilarity computes Jaccard similarity on lowercased word sets.
func ContentSimilarity(a, b string) float64 {
	setA := wordSet(a)
	setB := wordSet(b)

	if len(setA) == 0 && len(setB) == 0 {
		return 1.0
	}

	intersection := 0
	for w := range setA {
		if setB[w] {
			intersection++
		}
	}

	union := len(setA)
	for w := range setB {
		if !setA[w] {
			union++
		}
	}

	if union == 0 {
		return 1.0
	}

	return float64(intersection) / float64(union)
}

func wordSet(s string) map[string]bool {
	words := strings.Fields(strings.ToLower(s))
	set := make(map[string]bool, len(words))
	for _, w := range words {
		set[w] = true
	}
	return set
}
