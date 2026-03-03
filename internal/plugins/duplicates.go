package plugins

import (
	"encoding/json"
	"sort"
	"strings"

	"github.com/ruminaider/claude-sync/internal/claudecode"
)

// Duplicate represents a plugin name that appears with multiple enabled sources.
type Duplicate struct {
	Name    string   // plugin name (before @)
	Sources []string // full keys, e.g. ["bash-validator@marketplace", "bash-validator@forks"]
}

// DetectDuplicates reads enabledPlugins from settings.json and returns any plugin names
// that appear with multiple enabled sources.
func DetectDuplicates(claudeDir string) ([]Duplicate, error) {
	settings, err := claudecode.ReadSettings(claudeDir)
	if err != nil {
		return nil, err
	}

	raw, ok := settings["enabledPlugins"]
	if !ok {
		return nil, nil
	}

	var enabled map[string]bool
	if err := json.Unmarshal(raw, &enabled); err != nil {
		return nil, err
	}

	// Group enabled plugins by name (part before @)
	byName := make(map[string][]string)
	for key, isEnabled := range enabled {
		if !isEnabled {
			continue
		}
		name := pluginName(key)
		byName[name] = append(byName[name], key)
	}

	var dupes []Duplicate
	for name, sources := range byName {
		if len(sources) > 1 {
			sort.Strings(sources)
			dupes = append(dupes, Duplicate{Name: name, Sources: sources})
		}
	}
	sort.Slice(dupes, func(i, j int) bool {
		return dupes[i].Name < dupes[j].Name
	})
	return dupes, nil
}

// pluginName extracts the name portion from a "name@source" plugin key.
func pluginName(key string) string {
	if idx := strings.LastIndex(key, "@"); idx > 0 {
		return key[:idx]
	}
	return key
}
