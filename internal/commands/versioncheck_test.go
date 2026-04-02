package commands

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIsNewer(t *testing.T) {
	tests := []struct {
		latest, current string
		want            bool
	}{
		{"0.12.0", "0.11.0", true},
		{"0.11.1", "0.11.0", true},
		{"1.0.0", "0.11.0", true},
		{"0.11.0", "0.11.0", false},
		{"0.10.0", "0.11.0", false},
		{"0.11.0", "0.12.0", false},
	}
	for _, tt := range tests {
		t.Run(tt.latest+"_vs_"+tt.current, func(t *testing.T) {
			got := isNewer(tt.latest, tt.current)
			if got != tt.want {
				t.Errorf("isNewer(%q, %q) = %v, want %v", tt.latest, tt.current, got, tt.want)
			}
		})
	}
}

func TestWriteAndReadVersionCache(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "latest-version")

	if err := writeVersionCache(cachePath, "0.12.0"); err != nil {
		t.Fatalf("writeVersionCache: %v", err)
	}

	ver, ts, err := readVersionCache(cachePath)
	if err != nil {
		t.Fatalf("readVersionCache: %v", err)
	}
	if ver != "0.12.0" {
		t.Errorf("version = %q, want %q", ver, "0.12.0")
	}
	if time.Since(ts) > 5*time.Second {
		t.Errorf("timestamp too old: %v", ts)
	}
}

func TestReadVersionCache_Missing(t *testing.T) {
	_, _, err := readVersionCache("/nonexistent/path")
	if err == nil {
		t.Error("expected error for missing cache")
	}
}

func TestReadVersionCache_Malformed(t *testing.T) {
	dir := t.TempDir()
	cachePath := filepath.Join(dir, "latest-version")
	if err := os.WriteFile(cachePath, []byte("garbage"), 0644); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, _, err := readVersionCache(cachePath)
	if err == nil {
		t.Error("expected error for malformed cache")
	}
}

func TestIsCacheStale(t *testing.T) {
	fresh := time.Now().Add(-1 * time.Hour)
	stale := time.Now().Add(-25 * time.Hour)

	if isCacheStale(fresh) {
		t.Error("1-hour-old cache should not be stale")
	}
	if !isCacheStale(stale) {
		t.Error("25-hour-old cache should be stale")
	}
}
