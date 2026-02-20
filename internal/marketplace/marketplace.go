package marketplace

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// PluginVersionInfo holds version data for a plugin in a marketplace.
type PluginVersionInfo struct {
	Name      string
	Version   string // from plugin.json
	CommitSHA string // git commit SHA for the plugin directory
}

// knownMarketplaces maps well-known marketplace IDs to their GitHub org.
var knownMarketplaces = map[string]string{
	"claude-plugins-official":  "anthropics",
	"superpowers-marketplace":  "anthropics",
	"beads-marketplace":        "anthropics",
	"claude-sync-marketplace":  "ruminaider",
}

// IsPortableMarketplace returns true if the marketplace ID is a well-known,
// publicly available marketplace that can be installed on any machine.
// Non-portable marketplaces (local/custom) should be auto-forked during init.
//
// Prefer IsPortable(), which checks known_marketplaces.json first and falls
// back to this hardcoded list.
func IsPortableMarketplace(id string) bool {
	_, ok := knownMarketplaces[id]
	return ok
}

// knownMarketplacesEntry represents a single entry in known_marketplaces.json.
type knownMarketplacesEntry struct {
	Source struct {
		Source string `json:"source"` // "github", "git", or "directory"
	} `json:"source"`
}

// IsPortableFromKnownMarketplaces checks known_marketplaces.json for a marketplace.
// Returns (isPortable, found). If found is false, the caller should fall back
// to the hardcoded list.
func IsPortableFromKnownMarketplaces(claudeDir, id string) (isPortable bool, found bool) {
	path := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	data, err := os.ReadFile(path)
	if err != nil {
		return false, false
	}

	var entries map[string]knownMarketplacesEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return false, false
	}

	entry, ok := entries[id]
	if !ok {
		return false, false
	}

	src := entry.Source.Source
	return src == "github" || src == "git", true
}

// IsPortable checks whether a marketplace is portable (publicly installable).
// It reads known_marketplaces.json first, then falls back to the hardcoded list.
func IsPortable(claudeDir, id string) bool {
	if portable, found := IsPortableFromKnownMarketplaces(claudeDir, id); found {
		return portable
	}
	return IsPortableMarketplace(id)
}

// ParseMarketplaceSource extracts the org and repo from a marketplace identifier.
//
// Well-known marketplace IDs are mapped to their canonical org:
//
//	"claude-plugins-official" → ("anthropics", "claude-plugins-official")
//	"superpowers-marketplace" → ("anthropics", "superpowers-marketplace")
//
// IDs containing a "/" are split into org/repo:
//
//	"myorg/my-marketplace" → ("myorg", "my-marketplace")
//
// Unknown single-word IDs return the ID as both org and repo:
//
//	"unknown" → ("unknown", "unknown")
func ParseMarketplaceSource(marketplace string) (org, repo string) {
	// Check well-known marketplaces first.
	if known, ok := knownMarketplaces[marketplace]; ok {
		return known, marketplace
	}

	// If it contains a slash, split into org/repo.
	if parts := strings.SplitN(marketplace, "/", 2); len(parts) == 2 {
		return parts[0], parts[1]
	}

	// Unknown single-word marketplace — use as both org and repo.
	return marketplace, marketplace
}

// pluginJSON represents the minimal fields we read from a plugin's plugin.json.
type pluginJSON struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// QueryPluginVersion reads version information for a plugin from a local
// marketplace git repository. It reads the plugin's plugin.json for the
// version string, and uses git log to get the latest commit SHA touching
// the plugin's directory.
func QueryPluginVersion(repoPath, pluginName string) (*PluginVersionInfo, error) {
	// Read plugin.json from the plugin subdirectory.
	pluginDir := filepath.Join(repoPath, pluginName)
	jsonPath := filepath.Join(pluginDir, "plugin.json")

	data, err := os.ReadFile(jsonPath)
	if err != nil {
		return nil, fmt.Errorf("reading plugin.json for %s: %w", pluginName, err)
	}

	var pj pluginJSON
	if err := json.Unmarshal(data, &pj); err != nil {
		return nil, fmt.Errorf("parsing plugin.json for %s: %w", pluginName, err)
	}

	// Get the latest commit SHA for the plugin directory.
	sha, err := gitLogLastCommit(repoPath, pluginName)
	if err != nil {
		return nil, fmt.Errorf("getting commit SHA for %s: %w", pluginName, err)
	}

	return &PluginVersionInfo{
		Name:      pj.Name,
		Version:   pj.Version,
		CommitSHA: sha,
	}, nil
}

// HasUpdate returns true if the current and latest versions differ.
// This handles both semantic version strings and SHA-based versions.
func HasUpdate(current, latest string) bool {
	if current == "" || latest == "" {
		return false
	}
	return strings.TrimSpace(current) != strings.TrimSpace(latest)
}

// QueryRemoteVersion uses git ls-remote to get the HEAD SHA of a remote
// repository without cloning it. This is useful for checking if a remote
// marketplace has been updated.
func QueryRemoteVersion(marketplaceURL, pluginName string) (string, error) {
	cmd := exec.Command("git", "ls-remote", marketplaceURL, "HEAD")
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git ls-remote %s: %w: %s", marketplaceURL, err, string(out))
	}

	output := strings.TrimSpace(string(out))
	if output == "" {
		return "", fmt.Errorf("no HEAD ref found for %s", marketplaceURL)
	}

	// Output format: "<sha>\tHEAD"
	fields := strings.Fields(output)
	if len(fields) < 1 {
		return "", fmt.Errorf("unexpected ls-remote output for %s: %s", marketplaceURL, output)
	}

	return fields[0], nil
}

// marketplaceJSON represents the minimal fields we read from a marketplace.json.
type marketplaceJSON struct {
	Plugins []struct {
		Name    string `json:"name"`
		Source  string `json:"source"`
		Version string `json:"version"`
	} `json:"plugins"`
}

// knownMarketplacesVersionEntry represents a marketplace entry with installLocation.
type knownMarketplacesVersionEntry struct {
	InstallLocation string `json:"installLocation"`
}

// ReadMarketplacePluginVersion reads the current version for a plugin from its
// marketplace source on disk. It parses the pluginKey ("name@marketplace") to
// find the marketplace's installLocation in known_marketplaces.json, then reads
// the marketplace.json and plugin.json files to determine the version.
//
// Handles both single-plugin (source="./") and multi-plugin (source="./plugins/foo")
// marketplaces. Returns an error if the marketplace or plugin cannot be found
// locally; callers should skip gracefully on error.
func ReadMarketplacePluginVersion(claudeDir, pluginKey string) (string, error) {
	parts := strings.SplitN(pluginKey, "@", 2)
	if len(parts) != 2 {
		return "", fmt.Errorf("invalid plugin key: %s", pluginKey)
	}
	pluginName := parts[0]
	marketplaceName := parts[1]

	// Read known_marketplaces.json to get installLocation.
	kmPath := filepath.Join(claudeDir, "plugins", "known_marketplaces.json")
	kmData, err := os.ReadFile(kmPath)
	if err != nil {
		return "", fmt.Errorf("reading known_marketplaces.json: %w", err)
	}

	var entries map[string]knownMarketplacesVersionEntry
	if err := json.Unmarshal(kmData, &entries); err != nil {
		return "", fmt.Errorf("parsing known_marketplaces.json: %w", err)
	}

	entry, ok := entries[marketplaceName]
	if !ok {
		return "", fmt.Errorf("marketplace %s not found", marketplaceName)
	}
	if entry.InstallLocation == "" {
		return "", fmt.Errorf("no installLocation for marketplace %s", marketplaceName)
	}

	// Read marketplace.json to find the plugin's source path.
	mkplPath := filepath.Join(entry.InstallLocation, ".claude-plugin", "marketplace.json")
	mkplData, err := os.ReadFile(mkplPath)
	if err != nil {
		return "", fmt.Errorf("reading marketplace.json for %s: %w", marketplaceName, err)
	}

	var mkpl marketplaceJSON
	if err := json.Unmarshal(mkplData, &mkpl); err != nil {
		return "", fmt.Errorf("parsing marketplace.json for %s: %w", marketplaceName, err)
	}

	// Find the plugin in the marketplace's plugins array.
	var sourcePath, mkplVersion string
	for _, p := range mkpl.Plugins {
		if p.Name == pluginName {
			sourcePath = p.Source
			mkplVersion = p.Version
			break
		}
	}
	if sourcePath == "" {
		return "", fmt.Errorf("plugin %s not found in marketplace %s", pluginName, marketplaceName)
	}

	// Try reading plugin.json at the source path for the authoritative version.
	pjPath := filepath.Join(entry.InstallLocation, sourcePath, ".claude-plugin", "plugin.json")
	pjData, err := os.ReadFile(pjPath)
	if err == nil {
		var pj pluginJSON
		if json.Unmarshal(pjData, &pj) == nil && pj.Version != "" {
			return pj.Version, nil
		}
	}

	// Fall back to the version in marketplace.json.
	if mkplVersion != "" {
		return mkplVersion, nil
	}

	return "", fmt.Errorf("no version found for plugin %s in marketplace %s", pluginName, marketplaceName)
}

// gitLogLastCommit returns the latest commit SHA that touched the given path
// within the repository at repoPath.
func gitLogLastCommit(repoPath, path string) (string, error) {
	cmd := exec.Command("git", "log", "-1", "--format=%H", "--", path)
	cmd.Dir = repoPath
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git log: %w: %s", err, string(out))
	}

	sha := strings.TrimSpace(string(out))
	if sha == "" {
		return "", fmt.Errorf("no commits found for path %s", path)
	}

	return sha, nil
}
