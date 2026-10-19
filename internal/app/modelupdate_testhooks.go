//go:build testhooks

// The test-only settings of the daily model update, compiled ONLY into a binary
// built with `-tags testhooks`. A release build (`-tags minified`, or none) does
// not contain this file, so no environment variable, request or page can reach
// what it sets: the wake and due intervals, the per-model passes, and where the
// feed and its sibling listings are read from. The browser exercise of the
// daily update uses it to force "due" in seconds and to make the feed fail.
//
// None of it is a setting: it is read once at start from the environment of the
// process that was started with it.

package app

import (
	"modelsdb/internal/catalog"
	"os"
	"strings"
	"time"
)

func init() {
	if d, err := time.ParseDuration(os.Getenv("MODELSDB_TEST_WAKE")); err == nil && d > 0 {
		modelUpdateWakeInterval = d
	}
	if d, err := time.ParseDuration(os.Getenv("MODELSDB_TEST_DUE_AFTER")); err == nil && d > 0 {
		modelUpdateInterval = d
	}
	if os.Getenv("MODELSDB_TEST_SKIP_PER_MODEL") == "1" {
		scheduledRunOptions = catalog.Options{SkipPerModelPasses: true}
	}
	// MODELSDB_TEST_FEED_BASE points the feed, the EU list, the video and image
	// listings, the provider-policy map and the two per-model endpoints at one local server.
	if base := strings.TrimRight(os.Getenv("MODELSDB_TEST_FEED_BASE"), "/"); base != "" {
		catalog.ApiUrl = base + "/models"
		catalog.EURoutingURL = base + "/eu"
		catalog.VideoModelsURL = base + "/videos"
		catalog.ImageModelsURL = base + "/images"
		catalog.AllProvidersURL = base + "/all-providers"
		catalog.ModelEndpointsBase = base + "/ep"
		catalog.StatsEndpointBase = base + "/stats"
	}
}
