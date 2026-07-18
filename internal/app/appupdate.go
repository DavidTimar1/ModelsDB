// Package app - in-app binary self-update controller.
//
// This is the APP self-update (replace the running binary), distinct from the
// catalog refresh on /api/update (refresh model DATA). It drives the
// internal/selfupdate package: download + verify + swap a newer signed release,
// then re-exec on an explicit user restart. The staging never restarts the
// process on its own - a restart is always a deliberate click.
//
// A single shared status behind a mutex mirrors updatecheck.go's style. Only one
// staging runs at a time; concurrent triggers (the daily auto-check and a manual
// click) collapse onto the same run.
package app

import (
	"encoding/json"
	"log"
	"modelsdb/internal/paths"
	"modelsdb/internal/selfupdate"
	"modelsdb/internal/shared"
	"os"
	"sync"
)

// appUpdateStatus is the served state of the self-update. State is one of
// idle | downloading | verifying | staged | failed. Supported and Updatable are
// filled at read time: Supported reports the platform can self-update at all;
// Updatable additionally requires a real release signing key and a non-dev
// build, so the UI only offers "Update now" when it can actually succeed.
type appUpdateStatus struct {
	State     string `json:"state"`
	Error     string `json:"error,omitempty"`
	Latest    string `json:"latest,omitempty"`
	Supported bool   `json:"supported"`
	Updatable bool   `json:"updatable"`
}

const (
	appUpdateIdle        = "idle"
	appUpdateDownloading = "downloading"
	appUpdateVerifying   = "verifying"
	appUpdateStaged      = "staged"
	appUpdateFailed      = "failed"
)

var (
	appUpdateMu    sync.Mutex
	appUpdateState = appUpdateStatus{State: appUpdateIdle}
)

func setAppUpdateState(state, errMsg, latest string) {
	appUpdateMu.Lock()
	appUpdateState.State = state
	appUpdateState.Error = errMsg
	if latest != "" {
		appUpdateState.Latest = latest
	}
	appUpdateMu.Unlock()
}

func currentAppUpdateStatus() appUpdateStatus {
	appUpdateMu.Lock()
	defer appUpdateMu.Unlock()
	s := appUpdateState
	s.Supported = selfupdate.Supported()
	s.Updatable = selfUpdatePossible()
	return s
}

// selfUpdatePossible reports whether an in-app self-update can actually run to
// completion here: a supported platform, a real embedded signing key, and a
// non-dev build. When false, the UI falls back to the plain download link and
// the auto-stage path is skipped, so no attempt is made that would only fail.
func selfUpdatePossible() bool {
	return selfupdate.Supported() && selfupdate.ReleaseKeyConfigured() && !shared.IsDevVersion()
}

// startAppUpdate stages the given latest release in the background. It is a
// no-op when a staging is already in progress or already complete, so repeated
// triggers (auto-check + manual click) do not re-download. It returns whether a
// new staging was started.
func startAppUpdate(latest string) bool {
	appUpdateMu.Lock()
	switch appUpdateState.State {
	case appUpdateDownloading, appUpdateVerifying, appUpdateStaged:
		appUpdateMu.Unlock()
		return false
	}
	appUpdateState = appUpdateStatus{State: appUpdateDownloading, Latest: latest}
	appUpdateMu.Unlock()

	go func() {
		staged, err := selfupdate.Stage(latest, func(phase string) {
			setAppUpdateState(phase, "", latest)
		})
		if err != nil {
			log.Printf("App self-update failed: %v", err)
			setAppUpdateState(appUpdateFailed, err.Error(), latest)
			return
		}
		if !staged {
			setAppUpdateState(appUpdateFailed, "update was not staged", latest)
			return
		}
		setAppUpdateState(appUpdateStaged, "", latest)
	}()
	return true
}

// autoUpdateEnabled reports whether the user opted into automatic downloading of
// updates. It reads the "auto_update" boolean from settings.json (plain JSON
// written by the UI); a missing file or key means off.
func autoUpdateEnabled() bool {
	b, err := os.ReadFile(paths.SettingsJsonFile)
	if err != nil {
		return false
	}
	var s struct {
		AutoUpdate bool `json:"auto_update"`
	}
	if json.Unmarshal(b, &s) != nil {
		return false
	}
	return s.AutoUpdate
}
