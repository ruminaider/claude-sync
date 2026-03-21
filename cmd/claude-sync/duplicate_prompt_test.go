package main

import (
	"testing"

	"github.com/ruminaider/claude-sync/internal/plugins"
	"github.com/stretchr/testify/assert"
)

func TestIsForkDuplicate_ForkVsMarketplace(t *testing.T) {
	d := plugins.Duplicate{
		Name:    "bash-validator",
		Sources: []string{"bash-validator@bash-validator-marketplace", "bash-validator@claude-sync-forks"},
	}
	forkSrc, mktSrc, ok := isForkDuplicate(d)
	assert.True(t, ok)
	assert.Equal(t, "bash-validator@claude-sync-forks", forkSrc)
	assert.Equal(t, "bash-validator@bash-validator-marketplace", mktSrc)
}

func TestIsForkDuplicate_TwoMarketplaces(t *testing.T) {
	d := plugins.Duplicate{
		Name:    "superpowers",
		Sources: []string{"superpowers@marketplace-a", "superpowers@marketplace-b"},
	}
	_, _, ok := isForkDuplicate(d)
	assert.False(t, ok, "two marketplaces is not a fork duplicate")
}

func TestIsForkDuplicate_MoreThanTwoSources(t *testing.T) {
	d := plugins.Duplicate{
		Name: "plugin",
		Sources: []string{
			"plugin@marketplace-a",
			"plugin@marketplace-b",
			"plugin@claude-sync-forks",
		},
	}
	_, _, ok := isForkDuplicate(d)
	assert.False(t, ok, "3+ sources should not be treated as simple fork duplicate")
}
