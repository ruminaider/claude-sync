package subscriptions

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.yaml.in/yaml/v3"
)

// CategoryMode controls how items are selected from a subscription.
type CategoryMode string

const (
	CategoryAll  CategoryMode = "all"
	CategoryNone CategoryMode = "none"
)

// Subscription represents a single subscription to another team's config repo.
type Subscription struct {
	URL        string                    `yaml:"url"`
	Ref        string                    `yaml:"ref,omitempty"` // default: "main"
	Categories SubscriptionCategories    `yaml:"categories"`
	Exclude    map[string][]string       `yaml:"exclude,omitempty"` // category -> item names to exclude
	Include    map[string][]string       `yaml:"include,omitempty"` // category -> item names to include (overrides "none")
	Prefer     map[string][]string       `yaml:"prefer,omitempty"` // category -> item names this sub wins for
}

// SubscriptionCategories controls per-category selection mode.
type SubscriptionCategories struct {
	MCP         CategoryMode              `yaml:"mcp,omitempty"`
	Plugins     CategoryMode              `yaml:"plugins,omitempty"`
	Settings    CategoryMode              `yaml:"settings,omitempty"`
	Hooks       CategoryMode              `yaml:"hooks,omitempty"`
	Permissions CategoryMode              `yaml:"permissions,omitempty"`
	ClaudeMD    CategoryMode              `yaml:"claude_md,omitempty"`
	Commands    *SubscriptionCommandsMode `yaml:"commands,omitempty"`
	Skills      CategoryMode              `yaml:"skills,omitempty"`
}

// SubscriptionCommandsMode supports both simple mode and granular include.
type SubscriptionCommandsMode struct {
	Mode    CategoryMode `yaml:"mode,omitempty"`
	Include []string     `yaml:"include,omitempty"`
}

// EffectiveRef returns the git ref to track, defaulting to "main".
func (s *Subscription) EffectiveRef() string {
	if s.Ref != "" {
		return s.Ref
	}
	return "main"
}

// SubscriptionState tracks per-subscription fetch state (gitignored).
type SubscriptionState struct {
	Subscriptions map[string]SubState `yaml:"subscriptions"`
}

// SubState tracks the last-fetched state for one subscription.
type SubState struct {
	LastFetched   time.Time         `yaml:"last_fetched"`
	CommitSHA     string            `yaml:"commit_sha"`
	AcceptedItems map[string][]string `yaml:"accepted_items"` // category -> item names
}

// Conflict represents an unresolved subscription-to-subscription conflict.
type Conflict struct {
	Category string // "mcp", "plugins", "settings", etc.
	ItemName string // the conflicting item's name
	SourceA  string // subscription name A
	SourceB  string // subscription name B
}

func (c Conflict) String() string {
	return fmt.Sprintf("%s %q: conflict between subscriptions %q and %q", c.Category, c.ItemName, c.SourceA, c.SourceB)
}

// ProvenanceItem tracks which subscription provided an item.
type ProvenanceItem struct {
	Source string          // subscription name
	Value  json.RawMessage // the item's raw value
}

// StatePath returns the path to subscription-state.yaml in the sync dir.
func StatePath(syncDir string) string {
	return filepath.Join(syncDir, "subscription-state.yaml")
}

// ReadState reads subscription-state.yaml from the sync dir.
func ReadState(syncDir string) (SubscriptionState, error) {
	path := StatePath(syncDir)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return SubscriptionState{
				Subscriptions: make(map[string]SubState),
			}, nil
		}
		return SubscriptionState{}, fmt.Errorf("reading subscription state: %w", err)
	}
	var state SubscriptionState
	if err := yaml.Unmarshal(data, &state); err != nil {
		return SubscriptionState{}, fmt.Errorf("parsing subscription state: %w", err)
	}
	if state.Subscriptions == nil {
		state.Subscriptions = make(map[string]SubState)
	}
	return state, nil
}

// WriteState writes subscription-state.yaml to the sync dir.
func WriteState(syncDir string, state SubscriptionState) error {
	data, err := yaml.Marshal(state)
	if err != nil {
		return fmt.Errorf("marshaling subscription state: %w", err)
	}
	return os.WriteFile(StatePath(syncDir), data, 0644)
}

// SubDir returns the path to a subscription's local clone directory.
func SubDir(syncDir, name string) string {
	return filepath.Join(syncDir, "subscriptions", name)
}
