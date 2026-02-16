package claudemd

import (
	"os"
	"path/filepath"
)

// RenamedFragment describes a fragment whose header changed but content is similar.
type RenamedFragment struct {
	OldName   string
	NewHeader string
}

// ReconcileResult describes what changed between the stored fragments and
// the current CLAUDE.md content.
type ReconcileResult struct {
	Updated []string          // fragment names with updated content
	New     []Section         // sections not matching any fragment
	Deleted []string          // fragment names not found in current content
	Renamed []RenamedFragment // sections with similar content but different header
}

// Reconcile compares currentContent against stored fragments and returns
// what changed. This is the push-direction function.
func Reconcile(syncDir, currentContent string) (*ReconcileResult, error) {
	claudeMdDir := filepath.Join(syncDir, claudeMdSubdir)

	manifest, err := ReadManifest(claudeMdDir)
	if err != nil {
		return nil, err
	}

	sections := Split(currentContent)

	result := &ReconcileResult{}

	// Track which manifest fragments and sections have been matched.
	matchedFragments := make(map[string]bool)
	matchedSections := make(map[int]bool)

	// Pass 1: exact header match.
	for i, sec := range sections {
		name := HeaderToFragmentName(sec.Header)
		meta, exists := manifest.Fragments[name]
		if !exists {
			continue
		}
		matchedFragments[name] = true
		matchedSections[i] = true

		// Check if content changed.
		newHash := ContentHash(sec.Content)
		if newHash != meta.ContentHash {
			result.Updated = append(result.Updated, name)
			// Update fragment file and manifest entry.
			if err := WriteFragment(claudeMdDir, name, sec.Content); err != nil {
				return nil, err
			}
			manifest.Fragments[name] = FragmentMeta{
				Header:      sec.Header,
				ContentHash: newHash,
			}
		}
	}

	// Collect unmatched sections and fragments.
	var unmatchedSections []int
	for i := range sections {
		if !matchedSections[i] {
			unmatchedSections = append(unmatchedSections, i)
		}
	}

	var unmatchedFragments []string
	for name := range manifest.Fragments {
		if !matchedFragments[name] {
			unmatchedFragments = append(unmatchedFragments, name)
		}
	}

	// Pass 2: rename detection via content similarity.
	usedFragments := make(map[string]bool)
	usedSections := make(map[int]bool)

	for _, si := range unmatchedSections {
		sec := sections[si]
		bestSim := 0.0
		bestFrag := ""

		for _, fname := range unmatchedFragments {
			if usedFragments[fname] {
				continue
			}
			fragContent, err := ReadFragment(claudeMdDir, fname)
			if err != nil {
				continue
			}
			sim := ContentSimilarity(sec.Content, fragContent)
			if sim > bestSim {
				bestSim = sim
				bestFrag = fname
			}
		}

		if bestSim > 0.8 && bestFrag != "" {
			usedFragments[bestFrag] = true
			usedSections[si] = true
			result.Renamed = append(result.Renamed, RenamedFragment{
				OldName:   bestFrag,
				NewHeader: sec.Header,
			})

			// Update the fragment: write new content under old name, update manifest.
			if err := WriteFragment(claudeMdDir, bestFrag, sec.Content); err != nil {
				return nil, err
			}
			manifest.Fragments[bestFrag] = FragmentMeta{
				Header:      sec.Header,
				ContentHash: ContentHash(sec.Content),
			}
		}
	}

	// Pass 3: new and deleted.
	for _, si := range unmatchedSections {
		if usedSections[si] {
			continue
		}
		result.New = append(result.New, sections[si])
	}

	for _, fname := range unmatchedFragments {
		if usedFragments[fname] {
			continue
		}
		result.Deleted = append(result.Deleted, fname)
	}

	// Write updated manifest, creating directory if needed.
	if err := os.MkdirAll(claudeMdDir, 0755); err != nil {
		return nil, err
	}
	if err := WriteManifest(claudeMdDir, manifest); err != nil {
		return nil, err
	}

	return result, nil
}
