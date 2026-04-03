package commands

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const versionCacheFile = "latest-version"
const cacheTTL = 24 * time.Hour
const githubReleasesURL = "https://api.github.com/repos/ruminaider/claude-sync/releases/latest"

// isNewer returns true if latest is a newer semver than current.
// Both must be in "major.minor.patch" format (no "v" prefix).
func isNewer(latest, current string) bool {
	parse := func(v string) (int, int, int) {
		v = strings.TrimPrefix(v, "v")
		parts := strings.SplitN(v, ".", 3)
		if len(parts) != 3 {
			return 0, 0, 0
		}
		major, _ := strconv.Atoi(parts[0])
		minor, _ := strconv.Atoi(parts[1])
		patch, _ := strconv.Atoi(parts[2])
		return major, minor, patch
	}
	lMaj, lMin, lPat := parse(latest)
	cMaj, cMin, cPat := parse(current)
	if lMaj != cMaj {
		return lMaj > cMaj
	}
	if lMin != cMin {
		return lMin > cMin
	}
	return lPat > cPat
}

func versionCachePath(syncDir string) string {
	return filepath.Join(syncDir, versionCacheFile)
}

func writeVersionCache(path, version string) error {
	content := version + "\n" + time.Now().UTC().Format(time.RFC3339) + "\n"
	return os.WriteFile(path, []byte(content), 0644)
}

func readVersionCache(path string) (version string, ts time.Time, err error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", time.Time{}, err
	}
	lines := strings.SplitN(strings.TrimSpace(string(data)), "\n", 2)
	if len(lines) != 2 {
		return "", time.Time{}, fmt.Errorf("malformed cache: expected 2 lines, got %d", len(lines))
	}
	ts, err = time.Parse(time.RFC3339, lines[1])
	if err != nil {
		return "", time.Time{}, fmt.Errorf("malformed cache timestamp: %w", err)
	}
	return lines[0], ts, nil
}

func isCacheStale(ts time.Time) bool {
	return time.Since(ts) > cacheTTL
}

type githubRelease struct {
	TagName string `json:"tag_name"`
}

// fetchLatestVersion queries GitHub for the latest release tag.
// Returns the version string without the "v" prefix.
func fetchLatestVersion() (string, error) {
	client := &http.Client{Timeout: 3 * time.Second}
	resp, err := client.Get(githubReleasesURL)
	if err != nil {
		return "", fmt.Errorf("failed to query GitHub releases: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("GitHub API returned status %d", resp.StatusCode)
	}

	var release githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return "", fmt.Errorf("failed to parse GitHub response: %w", err)
	}

	return strings.TrimPrefix(release.TagName, "v"), nil
}

// VersionCheckResult holds the result of a version check.
type VersionCheckResult struct {
	UpdateAvailable bool
	LatestVersion   string
	CurrentVersion  string
}

// CheckForUpdate checks whether a newer version of claude-sync is available.
// Uses a cache file in syncDir to avoid hitting the GitHub API on every call.
func CheckForUpdate(syncDir, currentVersion string) VersionCheckResult {
	cachePath := versionCachePath(syncDir)
	result := VersionCheckResult{CurrentVersion: currentVersion}

	// Try reading cache first.
	ver, ts, err := readVersionCache(cachePath)
	if err == nil && !isCacheStale(ts) {
		result.LatestVersion = ver
		result.UpdateAvailable = isNewer(ver, currentVersion)
		return result
	}

	// Cache miss or stale: fetch from GitHub.
	latest, err := fetchLatestVersion()
	if err != nil {
		// Network failure is not fatal; return "no update" rather than erroring.
		return result
	}

	_ = writeVersionCache(cachePath, latest)
	result.LatestVersion = latest
	result.UpdateAvailable = isNewer(latest, currentVersion)
	return result
}
