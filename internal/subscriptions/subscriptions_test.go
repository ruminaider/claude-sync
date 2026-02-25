package subscriptions

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadState_FileNotExist(t *testing.T) {
	syncDir := t.TempDir()

	state, err := ReadState(syncDir)

	require.NoError(t, err)
	assert.NotNil(t, state.Subscriptions)
	assert.Len(t, state.Subscriptions, 0)
}

func TestWriteAndReadState(t *testing.T) {
	syncDir := t.TempDir()

	now := time.Now().Truncate(time.Second)
	state := SubscriptionState{
		Subscriptions: map[string]SubState{
			"team-backend": {
				LastFetched: now,
				CommitSHA:   "abc1234",
				AcceptedItems: map[string][]string{
					"mcp":     {"sentry", "grafana"},
					"plugins": {"datadog-plugin"},
				},
			},
		},
	}

	err := WriteState(syncDir, state)
	require.NoError(t, err)

	// Verify file exists.
	_, err = os.Stat(StatePath(syncDir))
	require.NoError(t, err)

	// Read back.
	got, err := ReadState(syncDir)
	require.NoError(t, err)

	assert.Equal(t, "abc1234", got.Subscriptions["team-backend"].CommitSHA)
	assert.Equal(t, now.UTC(), got.Subscriptions["team-backend"].LastFetched.UTC())
	assert.Equal(t, []string{"sentry", "grafana"}, got.Subscriptions["team-backend"].AcceptedItems["mcp"])
}

func TestReadState_InvalidYAML(t *testing.T) {
	syncDir := t.TempDir()
	err := os.WriteFile(StatePath(syncDir), []byte(":::invalid yaml"), 0644)
	require.NoError(t, err)

	_, err = ReadState(syncDir)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "parsing subscription state")
}

func TestSubDir(t *testing.T) {
	dir := SubDir("/home/user/.claude-sync", "team-backend")
	assert.Equal(t, filepath.Join("/home/user/.claude-sync", "subscriptions", "team-backend"), dir)
}

func TestStatePath(t *testing.T) {
	p := StatePath("/home/user/.claude-sync")
	assert.Equal(t, filepath.Join("/home/user/.claude-sync", "subscription-state.yaml"), p)
}

func TestConflict_String(t *testing.T) {
	c := Conflict{
		Category: "mcp",
		ItemName: "sentry",
		SourceA:  "team-backend",
		SourceB:  "team-infra",
	}
	s := c.String()
	assert.Contains(t, s, "mcp")
	assert.Contains(t, s, "sentry")
	assert.Contains(t, s, "team-backend")
	assert.Contains(t, s, "team-infra")
}

func TestSubscription_EffectiveRef(t *testing.T) {
	tests := []struct {
		name     string
		ref      string
		expected string
	}{
		{"empty ref defaults to main", "", "main"},
		{"explicit ref used", "develop", "develop"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := Subscription{Ref: tt.ref}
			assert.Equal(t, tt.expected, s.EffectiveRef())
		})
	}
}

func TestReadState_NilSubscriptionsInitialized(t *testing.T) {
	syncDir := t.TempDir()
	// Write a state file with no subscriptions key.
	err := os.WriteFile(StatePath(syncDir), []byte("# empty state\n"), 0644)
	require.NoError(t, err)

	state, err := ReadState(syncDir)
	require.NoError(t, err)
	assert.NotNil(t, state.Subscriptions)
}
