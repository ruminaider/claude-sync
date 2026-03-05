package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudemd"
)

// Hash key constants for managed surfaces.
const (
	HashKeySettings    = "settings"
	HashKeyClaudeMD    = "claude-md"
	HashKeyMCP         = "mcp"
	HashKeyKeybindings = "keybindings"
)

// AppliedHashes tracks content hashes of files last written by pull.
// Used to detect local modifications before overwriting.
type AppliedHashes struct {
	Hashes map[string]string `json:"hashes"`
	path   string
}

// LoadAppliedHashes reads .applied-hashes.json from syncDir.
// Returns a usable (possibly empty) AppliedHashes even on error, so callers
// can continue with degraded protection rather than aborting the pull.
func LoadAppliedHashes(syncDir string) (*AppliedHashes, error) {
	h := &AppliedHashes{
		Hashes: make(map[string]string),
		path:   filepath.Join(syncDir, ".applied-hashes.json"),
	}
	data, err := os.ReadFile(h.path)
	if err != nil {
		if os.IsNotExist(err) {
			return h, nil // first run — no hashes yet
		}
		return h, fmt.Errorf("reading applied hashes: %w", err)
	}
	if err := json.Unmarshal(data, h); err != nil {
		h.Hashes = make(map[string]string)
		return h, fmt.Errorf("parsing applied hashes (file may be corrupt): %w", err)
	}
	if h.Hashes == nil {
		h.Hashes = make(map[string]string)
	}
	return h, nil
}

// Save persists the hashes to disk.
func (h *AppliedHashes) Save() error {
	data, err := json.MarshalIndent(h, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(h.path, data, 0644)
}

// Set records the hash of the given content under key.
func (h *AppliedHashes) Set(key, content string) {
	h.Hashes[key] = claudemd.ContentHash(content)
}

// IsLocallyModified returns true if the file at filePath has been modified
// since pull last wrote it. Returns false if no hash is stored (first pull)
// or the file doesn't exist. Returns true on read errors (conservative: assume
// modified rather than risk overwriting user changes).
func (h *AppliedHashes) IsLocallyModified(key, filePath string) bool {
	storedHash, ok := h.Hashes[key]
	if !ok {
		return false // first pull — allow overwrite
	}
	currentData, err := os.ReadFile(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return false // file deleted — allow overwrite
		}
		return true // can't read file — assume modified (conservative)
	}
	return claudemd.ContentHash(string(currentData)) != storedHash
}
