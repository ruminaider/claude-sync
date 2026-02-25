package subscriptions

import (
	"fmt"
	"os"
	"time"

	"github.com/ruminaider/claude-sync/internal/config"
	gitpkg "github.com/ruminaider/claude-sync/internal/git"
)

// FetchResult holds the result of fetching one subscription.
type FetchResult struct {
	Name       string
	CommitSHA  string
	Changed    bool  // true if the SHA differs from the last-fetched state
	Error      error
}

// FetchAll fetches all subscriptions, creating shallow clones if needed.
// Returns a result per subscription.
func FetchAll(syncDir string, subs map[string]config.SubscriptionEntry, state SubscriptionState) []FetchResult {
	var results []FetchResult
	for name, sub := range subs {
		result := FetchOne(syncDir, name, sub, state)
		results = append(results, result)
	}
	return results
}

// FetchOne fetches a single subscription. If the clone doesn't exist yet,
// performs a shallow clone. Otherwise, fetches and resets.
func FetchOne(syncDir, name string, sub config.SubscriptionEntry, state SubscriptionState) FetchResult {
	dir := SubDir(syncDir, name)
	ref := sub.Ref
	if ref == "" {
		ref = "main"
	}

	if _, err := os.Stat(dir); os.IsNotExist(err) {
		// First fetch — shallow clone.
		if err := gitpkg.ShallowClone(sub.URL, dir, ref); err != nil {
			return FetchResult{Name: name, Error: fmt.Errorf("cloning %q: %w", name, err)}
		}
	} else {
		// Existing clone — fetch and reset.
		if err := gitpkg.FetchShallow(dir); err != nil {
			return FetchResult{Name: name, Error: fmt.Errorf("fetching %q: %w", name, err)}
		}
		if err := gitpkg.ResetToFetchHead(dir); err != nil {
			return FetchResult{Name: name, Error: fmt.Errorf("resetting %q: %w", name, err)}
		}
	}

	sha, err := gitpkg.HeadSHA(dir)
	if err != nil {
		return FetchResult{Name: name, Error: fmt.Errorf("reading SHA for %q: %w", name, err)}
	}

	prevState, hasPrev := state.Subscriptions[name]
	changed := !hasPrev || prevState.CommitSHA != sha

	return FetchResult{
		Name:      name,
		CommitSHA: sha,
		Changed:   changed,
	}
}

// UpdateState updates the subscription state after a successful fetch.
func UpdateState(state *SubscriptionState, name, sha string, acceptedItems map[string][]string) {
	if state.Subscriptions == nil {
		state.Subscriptions = make(map[string]SubState)
	}
	prev, ok := state.Subscriptions[name]
	if !ok {
		prev = SubState{}
	}
	prev.LastFetched = time.Now().UTC()
	prev.CommitSHA = sha
	if acceptedItems != nil {
		prev.AcceptedItems = acceptedItems
	} else if prev.AcceptedItems == nil {
		prev.AcceptedItems = make(map[string][]string)
	}
	state.Subscriptions[name] = prev
}

// RemoveSubscription cleans up a subscription's local clone and state.
func RemoveSubscription(syncDir, name string, state *SubscriptionState) error {
	dir := SubDir(syncDir, name)
	if err := os.RemoveAll(dir); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("removing subscription clone %q: %w", name, err)
	}
	delete(state.Subscriptions, name)
	return nil
}
