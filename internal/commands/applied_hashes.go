package commands

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/ruminaider/claude-sync/internal/claudemd"
)

// AppliedHashes tracks content hashes of files last written by pull.
// Used to detect local modifications before overwriting.
type AppliedHashes struct {
	Hashes map[string]string `json:"hashes"`
	path   string
}

// LoadAppliedHashes reads .applied-hashes.json from syncDir.
// Returns an empty hash map if the file is missing or unreadable.
func LoadAppliedHashes(syncDir string) *AppliedHashes {
	h := &AppliedHashes{
		Hashes: make(map[string]string),
		path:   filepath.Join(syncDir, ".applied-hashes.json"),
	}
	data, err := os.ReadFile(h.path)
	if err != nil {
		return h
	}
	json.Unmarshal(data, h)
	if h.Hashes == nil {
		h.Hashes = make(map[string]string)
	}
	return h
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
// or the file doesn't exist.
func (h *AppliedHashes) IsLocallyModified(key, filePath string) bool {
	storedHash, ok := h.Hashes[key]
	if !ok {
		return false // first pull — allow overwrite
	}
	currentData, err := os.ReadFile(filePath)
	if err != nil {
		return false // file doesn't exist — allow overwrite
	}
	return claudemd.ContentHash(string(currentData)) != storedHash
}
