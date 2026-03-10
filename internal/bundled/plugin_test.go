package bundled

import (
	"os"
	"path/filepath"
	"testing"
)

func TestExtractPlugin(t *testing.T) {
	dstDir := filepath.Join(t.TempDir(), "claude-sync")

	if err := ExtractPlugin(dstDir); err != nil {
		t.Fatalf("ExtractPlugin failed: %v", err)
	}

	expectedFiles := []string{
		".claude-plugin/plugin.json",
		"skills/using-claude-sync/SKILL.md",
		"hooks/hooks.json",
		"commands/sync.md",
	}

	for _, f := range expectedFiles {
		path := filepath.Join(dstDir, f)
		if _, err := os.Stat(path); os.IsNotExist(err) {
			t.Errorf("expected file %s not found after extraction", f)
		}
	}
}

func TestExtractPlugin_Idempotent(t *testing.T) {
	dstDir := filepath.Join(t.TempDir(), "claude-sync")

	if err := ExtractPlugin(dstDir); err != nil {
		t.Fatalf("first ExtractPlugin failed: %v", err)
	}
	if err := ExtractPlugin(dstDir); err != nil {
		t.Fatalf("second ExtractPlugin failed: %v", err)
	}
}
