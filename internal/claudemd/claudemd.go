package claudemd

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"go.yaml.in/yaml/v3"
)

// claudeMdSubdir is the subdirectory within a sync dir that holds fragments.
const claudeMdSubdir = "claude-md"

// Section represents a section of a CLAUDE.md file, split on ## headers.
type Section struct {
	Header  string // "" for preamble (content before first ## header)
	Content string // the full content of the section including the ## header line
}

// Split splits markdown content on "## " headers.
// Content before the first "## " becomes a preamble section with empty Header.
// Empty/whitespace-only input returns nil.
func Split(content string) []Section {
	if strings.TrimSpace(content) == "" {
		return nil
	}

	var sections []Section
	lines := strings.Split(content, "\n")
	var current []string
	currentHeader := ""
	inPreamble := true

	for _, line := range lines {
		if strings.HasPrefix(line, "## ") {
			// Flush previous section
			if inPreamble {
				if len(current) > 0 {
					sections = append(sections, Section{
						Header:  "",
						Content: strings.Join(current, "\n"),
					})
				}
				inPreamble = false
			} else {
				sections = append(sections, Section{
					Header:  currentHeader,
					Content: strings.Join(current, "\n"),
				})
			}
			currentHeader = strings.TrimPrefix(line, "## ")
			current = []string{line}
		} else {
			current = append(current, line)
		}
	}

	// Flush last section
	if len(current) > 0 {
		if inPreamble {
			sections = append(sections, Section{
				Header:  "",
				Content: strings.Join(current, "\n"),
			})
		} else {
			sections = append(sections, Section{
				Header:  currentHeader,
				Content: strings.Join(current, "\n"),
			})
		}
	}

	return sections
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

// FragmentMeta holds metadata about a single fragment.
type FragmentMeta struct {
	Header      string `yaml:"header"`
	ContentHash string `yaml:"content_hash"`
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
func ReadFragment(claudeMdDir, name string) (string, error) {
	data, err := os.ReadFile(filepath.Join(claudeMdDir, name+".md"))
	if err != nil {
		return "", err
	}
	return string(data), nil
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
		name := HeaderToFragmentName(sec.Header)
		names = append(names, name)
		manifest.Order = append(manifest.Order, name)
		manifest.Fragments[name] = FragmentMeta{
			Header:      sec.Header,
			ContentHash: ContentHash(sec.Content),
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
		if strings.HasPrefix(content, "## ") {
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
