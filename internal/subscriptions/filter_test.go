package subscriptions

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestResolveItems_AllMode(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			MCP: CategoryAll,
		},
	}
	allItems := []string{"sentry", "grafana", "datadog", "prometheus"}

	result := ResolveItems(sub, "mcp", allItems)

	assert.Len(t, result, 4)
	assert.True(t, result["sentry"])
	assert.True(t, result["grafana"])
	assert.True(t, result["datadog"])
	assert.True(t, result["prometheus"])
}

func TestResolveItems_AllMode_WithExcludes(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			MCP: CategoryAll,
		},
		Exclude: map[string][]string{
			"mcp": {"datadog", "prometheus"},
		},
	}
	allItems := []string{"sentry", "grafana", "datadog", "prometheus"}

	result := ResolveItems(sub, "mcp", allItems)

	assert.Len(t, result, 2)
	assert.True(t, result["sentry"])
	assert.True(t, result["grafana"])
	assert.False(t, result["datadog"])
	assert.False(t, result["prometheus"])
}

func TestResolveItems_NoneMode(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			MCP: CategoryNone,
		},
	}
	allItems := []string{"sentry", "grafana", "datadog"}

	result := ResolveItems(sub, "mcp", allItems)

	assert.Len(t, result, 0)
}

func TestResolveItems_NoneMode_WithIncludes(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			MCP: CategoryNone,
		},
		Include: map[string][]string{
			"mcp": {"sentry", "grafana"},
		},
	}
	allItems := []string{"sentry", "grafana", "datadog", "prometheus"}

	result := ResolveItems(sub, "mcp", allItems)

	assert.Len(t, result, 2)
	assert.True(t, result["sentry"])
	assert.True(t, result["grafana"])
	assert.False(t, result["datadog"])
}

func TestResolveItems_NoneMode_IncludeNonexistent(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			MCP: CategoryNone,
		},
		Include: map[string][]string{
			"mcp": {"sentry", "nonexistent"},
		},
	}
	allItems := []string{"sentry", "grafana"}

	result := ResolveItems(sub, "mcp", allItems)

	// "nonexistent" is in include but not in allItems, so it's ignored.
	assert.Len(t, result, 1)
	assert.True(t, result["sentry"])
}

func TestResolveItems_EmptyMode_DefaultsToNone(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			// MCP not set — defaults to CategoryNone
		},
	}
	allItems := []string{"sentry", "grafana"}

	result := ResolveItems(sub, "mcp", allItems)

	assert.Len(t, result, 0)
}

func TestResolveItems_DifferentCategories(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			MCP:      CategoryAll,
			Plugins:  CategoryNone,
			Settings: CategoryAll,
		},
		Include: map[string][]string{
			"plugins": {"figma"},
		},
		Exclude: map[string][]string{
			"settings": {"dangerous_setting"},
		},
	}

	// MCP: all
	mcp := ResolveItems(sub, "mcp", []string{"sentry", "grafana"})
	assert.Len(t, mcp, 2)

	// Plugins: none + include figma
	plugins := ResolveItems(sub, "plugins", []string{"figma", "datadog"})
	assert.Len(t, plugins, 1)
	assert.True(t, plugins["figma"])

	// Settings: all - exclude dangerous_setting
	settings := ResolveItems(sub, "settings", []string{"theme", "dangerous_setting", "font"})
	assert.Len(t, settings, 2)
	assert.True(t, settings["theme"])
	assert.True(t, settings["font"])
	assert.False(t, settings["dangerous_setting"])
}

func TestIsPreferred(t *testing.T) {
	sub := Subscription{
		Prefer: map[string][]string{
			"mcp": {"sentry", "grafana"},
		},
	}

	assert.True(t, IsPreferred(sub, "mcp", "sentry"))
	assert.True(t, IsPreferred(sub, "mcp", "grafana"))
	assert.False(t, IsPreferred(sub, "mcp", "datadog"))
	assert.False(t, IsPreferred(sub, "plugins", "sentry")) // wrong category
}

func TestIsPreferred_NoPrefer(t *testing.T) {
	sub := Subscription{} // no prefer directives

	assert.False(t, IsPreferred(sub, "mcp", "sentry"))
}

func TestCategoryMode_Commands(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			Commands: &SubscriptionCommandsMode{
				Mode: CategoryAll,
			},
		},
	}

	result := ResolveItems(sub, "commands", []string{"cmd1", "cmd2"})
	assert.Len(t, result, 2)
}

func TestCategoryMode_CommandsNil(t *testing.T) {
	sub := Subscription{
		Categories: SubscriptionCategories{
			// Commands is nil
		},
	}

	result := ResolveItems(sub, "commands", []string{"cmd1", "cmd2"})
	assert.Len(t, result, 0) // nil Commands → none
}
