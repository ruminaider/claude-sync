package bundled

import (
	"encoding/json"
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

	// Verify plugin.json contains valid non-empty JSON.
	data, err := os.ReadFile(filepath.Join(dstDir, ".claude-plugin", "plugin.json"))
	if err != nil {
		t.Fatalf("reading plugin.json: %v", err)
	}
	if len(data) == 0 {
		t.Fatal("plugin.json is empty")
	}
	var parsed map[string]any
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("plugin.json is not valid JSON: %v", err)
	}
	if _, ok := parsed["name"]; !ok {
		t.Error("plugin.json missing 'name' field")
	}
}

func TestExtractPlugin_Idempotent(t *testing.T) {
	dstDir := filepath.Join(t.TempDir(), "claude-sync")

	if err := ExtractPlugin(dstDir); err != nil {
		t.Fatalf("first ExtractPlugin failed: %v", err)
	}

	// Read original content of a known file.
	skillPath := filepath.Join(dstDir, "skills", "using-claude-sync", "SKILL.md")
	original, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("reading skill file: %v", err)
	}

	// Modify the file.
	os.WriteFile(skillPath, []byte("corrupted"), 0644)

	// Second extraction should restore it.
	if err := ExtractPlugin(dstDir); err != nil {
		t.Fatalf("second ExtractPlugin failed: %v", err)
	}

	restored, err := os.ReadFile(skillPath)
	if err != nil {
		t.Fatalf("reading restored skill file: %v", err)
	}
	if string(restored) != string(original) {
		t.Error("second extraction did not restore modified file")
	}
}
