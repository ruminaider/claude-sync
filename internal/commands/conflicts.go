package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"go.yaml.in/yaml/v3"
)

// PendingConflict describes a single unresolved merge conflict.
type PendingConflict struct {
	Timestamp   string          `yaml:"timestamp"`
	Key         string          `yaml:"key"`
	LocalValue  json.RawMessage `yaml:"local_value,omitempty"`
	RemoteValue json.RawMessage `yaml:"remote_value,omitempty"`
}

// PendingConflictFile holds a list of conflicts persisted to disk.
type PendingConflictFile struct {
	Conflicts []PendingConflict `yaml:"conflicts"`
}

const conflictsDir = "conflicts"

// SaveConflicts writes pending conflicts to the sync dir.
func SaveConflicts(syncDir string, conflicts []PendingConflict) error {
	dir := filepath.Join(syncDir, conflictsDir)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}
	ts := time.Now().UTC().Format("20060102T150405Z")
	file := PendingConflictFile{Conflicts: conflicts}
	data, err := yaml.Marshal(file)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, ts+".yaml"), data, 0644)
}

// HasPendingConflicts checks if there are unresolved conflicts.
func HasPendingConflicts(syncDir string) bool {
	dir := filepath.Join(syncDir, conflictsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			return true
		}
	}
	return false
}

// ListPendingConflicts reads all pending conflict files.
func ListPendingConflicts(syncDir string) ([]PendingConflict, error) {
	dir := filepath.Join(syncDir, conflictsDir)
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var all []PendingConflict
	var fileNames []string
	for _, e := range entries {
		if !e.IsDir() && filepath.Ext(e.Name()) == ".yaml" {
			fileNames = append(fileNames, e.Name())
		}
	}
	sort.Strings(fileNames)

	for _, name := range fileNames {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			continue
		}
		var f PendingConflictFile
		if yaml.Unmarshal(data, &f) == nil {
			all = append(all, f.Conflicts...)
		}
	}
	return all, nil
}

// DiscardConflicts removes all pending conflict files.
func DiscardConflicts(syncDir string) error {
	dir := filepath.Join(syncDir, conflictsDir)
	return os.RemoveAll(dir)
}

// ResolveConflict applies a resolution choice and removes the conflict file.
// choice: "local" keeps local value, "remote" keeps remote value.
func ResolveConflict(syncDir string, index int, choice string) error {
	conflicts, err := ListPendingConflicts(syncDir)
	if err != nil {
		return err
	}
	if index < 0 || index >= len(conflicts) {
		return fmt.Errorf("conflict index %d out of range (0-%d)", index, len(conflicts)-1)
	}

	// Remove the resolved conflict and re-save remaining
	remaining := append(conflicts[:index], conflicts[index+1:]...)

	// Clear all files and re-save if there are remaining conflicts
	if err := DiscardConflicts(syncDir); err != nil {
		return err
	}
	if len(remaining) > 0 {
		return SaveConflicts(syncDir, remaining)
	}
	return nil
}
