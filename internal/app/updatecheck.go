// Package app - notify-only update checking.
//
// On startup and then once a day, the server asks the upstream repo's GitHub
// Releases API for the latest release tag and compares it to this build. If a
// newer version exists, the UI shows a dismissible "download" banner. We never
// download or replace the binary - updating is the user's deliberate action.
//
// The last result is cached to the cache dir so a quick restart does not re-hit
// GitHub. Like all GitHub calls here, requests send only Go's default
// User-Agent (no app/owner identity). Dev builds skip the check entirely.
package app

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"os"
	"strings"
	"sync"
	"time"
)

const releasesCheckInterval = 24 * time.Hour

// updateInfo is the cached/served update-check result.
type updateInfo struct {
	Current   string `json:"current"`    // this build's version
	Latest    string `json:"latest"`     // latest release tag, normalized (no leading v)
	URL       string `json:"url"`        // release page to download from
	Available bool   `json:"available"`  // Latest > Current
	CheckedAt string `json:"checked_at"` // RFC3339 of the last successful check
}

// githubRelease is the subset of the GitHub "latest release" payload we read.
type githubRelease struct {
	TagName string `json:"tag_name"`
	HTMLURL string `json:"html_url"`
}

var (
	updateMu   sync.RWMutex
	updateLast updateInfo
)

func releasesURL() string {
	return fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", paths.ActiveUpstream.Owner, paths.ActiveUpstream.Repo)
}

// startUpdateChecker launches the background checker. It is a no-op for dev
// builds (which have no real version to compare).
func startUpdateChecker() {
	if shared.IsDevVersion() {
		return
	}
	loadUpdateCache()
	go func() {
		for {
			if !recentlyChecked() {
				checkForUpdate()
			}
			time.Sleep(releasesCheckInterval)
		}
	}()
}

func recentlyChecked() bool {
	updateMu.RLock()
	defer updateMu.RUnlock()
	if updateLast.CheckedAt == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, updateLast.CheckedAt)
	return err == nil && time.Since(t) < releasesCheckInterval
}

func checkForUpdate() {
	var rel githubRelease
	if err := catalog.HTTPGetJSON(releasesURL(), &rel); err != nil {
		log.Printf("Update check skipped (%s): %v", releasesURL(), err)
		return
	}
	latest := strings.TrimPrefix(strings.TrimSpace(rel.TagName), "v")
	info := updateInfo{
		Current:   shared.Version,
		Latest:    latest,
		URL:       rel.HTMLURL,
		Available: latest != "" && shared.CompareSemver(shared.Version, latest) < 0,
		CheckedAt: dbcore.NowStamp(),
	}
	updateMu.Lock()
	updateLast = info
	updateMu.Unlock()
	saveUpdateCache(info)
	if info.Available {
		log.Printf("Update available: v%s (you have v%s) - %s", info.Latest, info.Current, info.URL)
		// Auto opt-in: when the user enabled automatic downloads and self-update can
		// actually run here (supported platform, real signing key, non-dev build),
		// stage the new binary in the background now. The swap is applied only on an
		// explicit restart, never unattended.
		if selfUpdatePossible() && autoUpdateEnabled() {
			log.Printf("Auto-update enabled: staging v%s in the background", info.Latest)
			startAppUpdate(info.Latest)
		}
	}
}

func currentUpdateInfo() updateInfo {
	updateMu.RLock()
	defer updateMu.RUnlock()
	info := updateLast
	// Current and Available ALWAYS reflect the running binary, never the cache: the
	// cached values may have been written by an earlier build (the check is
	// throttled and an in-place update swaps the binary without re-checking), so
	// serving them would show a stale version in the UI. Only Latest/URL/CheckedAt
	// come from the (possibly cached) remote result.
	info.Current = shared.Version
	info.Available = info.Latest != "" && shared.CompareSemver(shared.Version, info.Latest) < 0
	return info
}

func loadUpdateCache() {
	b, err := os.ReadFile(paths.UpdateCacheFile)
	if err != nil {
		return
	}
	var info updateInfo
	if json.Unmarshal(b, &info) == nil {
		updateMu.Lock()
		updateLast = info
		updateMu.Unlock()
	}
}

func saveUpdateCache(info updateInfo) {
	b, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return
	}
	os.MkdirAll(paths.CacheDir, 0755)
	if err := os.WriteFile(paths.UpdateCacheFile, b, 0644); err != nil {
		log.Printf("Warning: could not write update cache: %v", err)
	}
}
