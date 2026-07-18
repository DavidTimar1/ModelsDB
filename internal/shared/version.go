// Package shared - build/version metadata and semantic-version helpers.
package shared

import (
	"strconv"
	"strings"
)

// Version is the application's semantic version, injected at build time from the
// repo-root VERSION file via
//
//	-ldflags "-X 'modelsdb/internal/shared.Version=<value>'"
//
// A release build (build.sh) injects the bare version, e.g. "1.26.1". The dev
// wrappers (start_dev.sh, start_dev_agent.sh) inject it with a "-dev" suffix, e.g.
// "1.26.1-dev", so a dev server states which version it is built from AND that it is
// not a release - both matter when a dev and a production server run side by side.
// The bare "dev" fallback below is what a plain `go run ./cmd/modelsdb` with no
// ldflags reports.
//
// Code that gates on the version - the curated.json min_app_version check, the
// update notifier, the self-updater - treats any dev build as the newest possible,
// so development runs are never blocked from importing data nor nagged to update.
// See IsDevVersion.
var Version = "dev"

// Curated catalog schema metadata, written into curated.json on export.
const (
	// CuratedSchemaVersion is the on-disk shape version of curated.json.
	CuratedSchemaVersion = 1

	// CuratedMinAppVersion is the minimum app version able to import the curated
	// catalog this build produces. A binary older than the value found in a
	// fetched curated.json refuses the import and tells the user to update.
	CuratedMinAppVersion = "0.1.0"
)

// IsDevVersion reports whether this is a development build, in which case version
// gates and the update check are skipped. True for the bare "dev" fallback and for
// any version carrying the "-dev" suffix the dev wrappers inject (e.g. "1.26.1-dev"),
// which names its version without claiming to be that release.
func IsDevVersion() bool {
	return Version == "dev" || Version == "" || strings.HasSuffix(Version, "-dev")
}

// parseSemver parses "x.y.z" (optionally "vX.Y.Z" with a -prerelease/+build
// suffix, which is ignored) into a comparable [major, minor, patch] triple.
// Missing or non-numeric parts are treated as 0.
func parseSemver(s string) [3]int {
	s = strings.TrimPrefix(strings.TrimSpace(s), "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	var out [3]int
	for i, p := range strings.SplitN(s, ".", 3) {
		if i >= 3 {
			break
		}
		out[i], _ = strconv.Atoi(strings.TrimSpace(p))
	}
	return out
}

// CompareSemver returns -1 if a < b, 0 if equal, 1 if a > b (by major.minor.patch).
func CompareSemver(a, b string) int {
	pa, pb := parseSemver(a), parseSemver(b)
	for i := 0; i < 3; i++ {
		switch {
		case pa[i] < pb[i]:
			return -1
		case pa[i] > pb[i]:
			return 1
		}
	}
	return 0
}
