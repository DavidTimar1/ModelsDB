// The handlers for health, paths, settings, reading the catalog and saving cells.

package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/paths"
	"modelsdb/internal/selfupdate"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"os"
	"time"
)

// instanceInfo is what this process says about itself: which application it is,
// which process, which port, which data dir, and every address it answers on.
// GET /api/health returns it with status "ok", and a request that arrives before
// startup finishes returns the same block with status "starting" (see
// writeStarting in server.go).
//
// The "app" and "pid" fields are what make the reply usable as PROOF of who
// holds the port: `stop` asks the port itself who is serving there and acts on
// the answer, which is right even when the PID file is stale, missing, or was
// never written. The marker is required before anything acts on that pid.
//
// "addresses" holds the base URLs whose listeners really bound (see
// servedaddrs.go). It belongs here because this block is already this instance's
// identity, and because the page has no other way to learn it: a browser knows
// only the address its reader typed, so the research brief it writes for an AI
// assistant takes the full list from here.
//
// "allowed_origins" and "allowed_origins_dropped" report config.jsonc
// "allowed_origins" as this process actually resolved it at startup - the
// normalized form actually compared against a request's Origin, and how many
// entries were dropped as malformed - so a hand edit that did not take effect
// is visible here without reading the log.
func instanceInfo(status string) map[string]interface{} {
	exe, _ := os.Executable()
	return map[string]interface{}{
		"app":                     shared.HealthAppMarker,
		"status":                  status,
		"pid":                     os.Getpid(),
		"exe":                     exe,
		"port":                    paths.Port,
		"data":                    paths.DataDir,
		"version":                 shared.Version,
		"addresses":               servedAddresses(),
		"allowed_origins":         paths.AllowedOrigins,
		"allowed_origins_dropped": paths.AllowedOriginsDropped,
	}
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(instanceInfo("ok"))
}

// handlePaths (GET /api/paths) reports the resolved on-disk locations so the UI
// can show the user exactly where ModelsDB keeps its data, config, and cache
// (the dirs plus the key files within them). Read-only; no secrets.
func handlePaths(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"executable": paths.ExePath,
		"port":       paths.Port,
		"version":    shared.Version,
		"dirs": map[string]string{
			"config": paths.ConfigDir,
			"data":   paths.DataDir,
			"cache":  paths.CacheDir,
		},
		"files": map[string]string{
			"database":     paths.DbFile,
			"config.jsonc": paths.ConfigFile,
			"backups":      paths.BackupsDir,
			"logs":         paths.LogsDir,
			"pid":          paths.PidFile,
			"update-check": paths.UpdateCacheFile,
		},
	})
}

// handleUpdateCheck (GET /api/update-check) returns the cached update-check
// result so the UI can show a "new version available" banner + the version label.
func handleUpdateCheck(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	// ?force=1 is a user-initiated check from the UI: hit GitHub now, bypassing the
	// once-a-day throttle, so the returned result reflects this instant. It runs
	// synchronously so the response carries the fresh values.
	if r.URL.Query().Get("force") == "1" {
		checkForUpdate()
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(currentUpdateInfo())
}

// handleAppUpdateApply (POST /api/app-update/apply) begins staging the newer
// binary in the background (download + verify + in-place swap) and returns 202.
// This REPLACES THE BINARY - it is separate from /api/update, which refreshes
// model data. It refuses when self-update is unsupported on this platform or no
// newer release is available. Progress is observable via
// /api/app-update/status; the swap is applied only on an explicit restart.
func handleAppUpdateApply(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !selfupdate.Supported() {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"started": false, "error": "self-update is not supported on this platform"})
		return
	}
	info := currentUpdateInfo()
	if !info.Available || info.Latest == "" {
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]interface{}{"started": false, "error": "no update available"})
		return
	}
	started := startAppUpdate(info.Latest)
	log.Printf("ok /api/app-update/apply: staging v%s (started=%v)", info.Latest, started)
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"started": started, "latest": info.Latest})
}

// handleAppUpdateStatus (GET /api/app-update/status) returns the self-update
// staging state so the UI can poll for completion.
func handleAppUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(currentAppUpdateStatus())
}

// handleAppUpdateRestart (POST /api/app-update/restart) re-launches the process
// so a staged binary takes effect. It refuses unless an update is staged. The
// response is flushed FIRST, then the restart fires after a short delay, because
// the re-exec replaces this process and the reply would otherwise never reach
// the browser. The UI then polls /api/health until the new process answers.
func handleAppUpdateRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if currentAppUpdateStatus().State != appUpdateStaged {
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{"restarting": false, "error": "no update is staged"})
		return
	}
	json.NewEncoder(w).Encode(map[string]interface{}{"restarting": true})
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
	log.Printf("ok /api/app-update/restart: re-launching to apply staged update")
	go func() {
		time.Sleep(400 * time.Millisecond) // let the response reach the browser
		if err := selfupdate.Restart(); err != nil {
			log.Printf("ERROR self-update restart failed: %v", err)
			setAppUpdateState(appUpdateFailed, "restart failed: "+err.Error(), "")
		}
	}()
}

// handleUpdate (POST /api/update) launches a full DB refresh in the background
// and returns immediately. The single-flight guard rejects a concurrent start
// with HTTP 409; the running refresh is otherwise observable via
// /api/update/status.
func handleUpdate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !refresh.start(false) {
		log.Printf("ok /api/update rejected: refresh already running")
		w.WriteHeader(http.StatusConflict)
		json.NewEncoder(w).Encode(map[string]interface{}{"started": false, "running": true})
		return
	}
	log.Printf("ok /api/update accepted: full refresh started")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]interface{}{"started": true})
}

// handleUpdateStatus (GET /api/update/status) returns the current refresh
// status so the frontend can poll progress and detect completion.
func handleUpdateStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(refresh.snapshot())
}

// backupSettingKeys are the three keys of a settings write that are NOT the
// reader's view: they say whether a daily copy of the database is taken and
// where it is written, which is a fact about this machine and belongs in
// config.jsonc. The page sends and receives them beside every other key and
// knows nothing of the split.
var backupSettingKeys = []string{"backup_enabled", "backup_dir", "backup_overwrite"}

// isBackupSettingKey reports whether key is one config.jsonc holds.
func isBackupSettingKey(key string) bool {
	for _, backupKey := range backupSettingKeys {
		if key == backupKey {
			return true
		}
	}
	return false
}

// maxSettingsBody is the largest POST /api/settings body this route reads.
//
// The route is unauthenticated and it writes to disk, and this server does not
// always answer on loopback alone: configured with a LAN or Tailscale address it
// binds that too. A view naming every column the app can draw is a few
// kilobytes, so the ceiling is out of a reader's reach and takes away an
// unbounded write.
const maxSettingsBody = 4 << 20

// settingsWrite is a POST /api/settings body that has passed every check: the
// keys to store, the keys to remove, and the backup settings peeled off into the
// shape config.jsonc takes.
type settingsWrite struct {
	set           map[string]json.RawMessage
	del           []string
	backup        backupScheduleConfig
	backupChanged bool
}

// handleSettings (GET, POST /api/settings) is the reader's view and the app's
// preferences: every stored key on the way out, and the keys to store or remove
// on the way in.
//
// A WRITE NAMES WHAT IT REMOVES. The body is {"set": {...}, "delete": [...]},
// and a key in neither half is left exactly as it was. A page that loaded before
// a preference existed does not carry it and so cannot destroy it, which is what
// a whole-set write could not promise.
//
// THE BODY IS ONE THING TO THE BROWSER AND TWO THINGS HERE. The reader's view
// goes into the database, one row per key, so a restored backup brings back the
// view that describes those models. The three backup keys go into config.jsonc,
// so a restored backup can never repoint where the next copy is written.
//
// EVERY CHECK RUNS BEFORE THE FIRST BYTE IS WRITTEN, because those two stores
// cannot be made atomic with each other: a body that is going to be refused has
// to be refused while neither of them has been touched.
//
// A FAILED READ IS AN ERROR, never an empty object. The page suppresses every
// save until a read succeeds, and it can only do that if a failure looks like
// one: answering {} would have it draw defaults and then save them over a view
// that was fine.
func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		stored, err := store.ListSettings()
		if err != nil {
			log.Printf("ERROR [%s %q] failed to read the stored settings: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Failed to read settings: %v", err), http.StatusInternalServerError)
			return
		}
		out := make(map[string]interface{}, len(stored)+len(backupSettingKeys))
		for key, value := range stored {
			out[key] = value
		}
		cfg := readBackupScheduleConfig()
		out["backup_enabled"] = cfg.Enabled
		out["backup_dir"] = cfg.Dir
		out["backup_overwrite"] = cfg.Overwrite
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		json.NewEncoder(w).Encode(out)
	case http.MethodPost:
		r.Body = http.MaxBytesReader(w, r.Body, maxSettingsBody)
		write, err := readSettingsWrite(r.Body)
		if err != nil {
			log.Printf("ERROR [%s %q] refused the write: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("The settings write was refused: %v", err), http.StatusBadRequest)
			return
		}
		if write.backupChanged {
			if err := paths.SaveBackupSettings(write.backup.Enabled, write.backup.Dir, write.backup.Overwrite); err != nil {
				log.Printf("ERROR [%s %q] failed to write the backup settings: %v", r.Method, r.URL.Path, err)
				http.Error(w, fmt.Sprintf("Failed to write settings: %v", err), http.StatusInternalServerError)
				return
			}
		}
		if err := store.WriteSettings(write.set, write.del); err != nil {
			log.Printf("ERROR [%s %q] failed to store the settings: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Failed to write settings: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(`{"ok": true}`))
	default:
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// readSettingsWrite reads a POST /api/settings body and answers what it asks
// for, or the one reason it is refused. Every error it returns is the caller's,
// and the route answers all of them with 400.
//
// IT WRITES NOTHING AND CHECKS EVERYTHING. A half-checked body that had already
// updated config.jsonc before a reserved key was noticed would leave the machine
// half-written behind a 400 on the wire.
//
// AN UNKNOWN TOP-LEVEL MEMBER IS REFUSED rather than ignored: accepting a flat
// whole-set body quietly would store nothing and look like it had worked.
func readSettingsWrite(body io.Reader) (settingsWrite, error) {
	var envelope struct {
		Set    json.RawMessage `json:"set"`
		Delete json.RawMessage `json:"delete"`
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&envelope); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return settingsWrite{}, fmt.Errorf("the body is larger than the %d bytes this route accepts", maxSettingsBody)
		}
		return settingsWrite{}, fmt.Errorf(`the body must be a JSON object of the shape {"set":{...},"delete":[...]} and hold nothing else: %v`, err)
	}

	out := settingsWrite{set: map[string]json.RawMessage{}}
	if len(envelope.Set) > 0 {
		if isJSONNull(envelope.Set) {
			return settingsWrite{}, errors.New(`"set" must be a JSON object of keys and values; leave it out to store nothing`)
		}
		if err := json.Unmarshal(envelope.Set, &out.set); err != nil {
			return settingsWrite{}, fmt.Errorf(`"set" must be a JSON object of keys and values: %v`, err)
		}
	}
	if len(envelope.Delete) > 0 {
		if isJSONNull(envelope.Delete) {
			return settingsWrite{}, errors.New(`"delete" must be a JSON array of setting keys; leave it out to remove nothing`)
		}
		if err := json.Unmarshal(envelope.Delete, &out.del); err != nil {
			return settingsWrite{}, fmt.Errorf(`"delete" must be a JSON array of setting keys: %v`, err)
		}
	}

	for key := range out.set {
		if err := checkPostedSettingKey(key); err != nil {
			return settingsWrite{}, err
		}
	}
	for _, key := range out.del {
		if err := checkPostedSettingKey(key); err != nil {
			return settingsWrite{}, err
		}
		if _, inBoth := out.set[key]; inBoth {
			return settingsWrite{}, fmt.Errorf("the key %q is in both halves; name it in one of them, because which half wins is not something to guess at", key)
		}
		if isBackupSettingKey(key) {
			return settingsWrite{}, fmt.Errorf("the key %q names where this machine writes its daily backup and cannot be deleted; set it instead", key)
		}
	}

	cfg := readBackupScheduleConfig()
	want, err := backupSettingsFrom(out.set, cfg)
	if err != nil {
		return settingsWrite{}, err
	}
	out.backup = want
	out.backupChanged = want != cfg
	for _, key := range backupSettingKeys {
		delete(out.set, key)
	}
	return out, nil
}

// checkPostedSettingKey answers whether the browser may name key in either half
// of a write.
func checkPostedSettingKey(key string) error {
	if key == "" {
		return errors.New("a settings key cannot be empty")
	}
	if store.IsReservedSettingKey(key) {
		return fmt.Errorf("the key %q belongs to the server and cannot be written here", key)
	}
	return nil
}

// isJSONNull reports whether raw is the JSON literal null. A member that is
// present and null is a caller's bug rather than an absent member: a caller who
// means "nothing to store" leaves the member out.
func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

// backupSettingsFrom answers what config.jsonc should hold once the backup keys
// a write carries are applied to what it holds now.
//
// AN ABSENT KEY KEEPS ITS STORED VALUE. Every other key in a write is the
// caller's to state; these three name a folder somebody typed, and a caller
// saving one preference must not be able to turn their daily backup off by not
// mentioning it.
//
// A value of the wrong type comes back as an error the caller is answerable for,
// so the route says 400 rather than the 500 a failed write earns.
func backupSettingsFrom(set map[string]json.RawMessage, cfg backupScheduleConfig) (backupScheduleConfig, error) {
	want := cfg
	if raw, ok := set["backup_enabled"]; ok {
		if err := json.Unmarshal(raw, &want.Enabled); err != nil {
			return cfg, fmt.Errorf("backup_enabled must be true or false: %v", err)
		}
	}
	if raw, ok := set["backup_dir"]; ok {
		if err := json.Unmarshal(raw, &want.Dir); err != nil {
			return cfg, fmt.Errorf("backup_dir must be a folder path: %v", err)
		}
	}
	if raw, ok := set["backup_overwrite"]; ok {
		if err := json.Unmarshal(raw, &want.Overwrite); err != nil {
			return cfg, fmt.Errorf("backup_overwrite must be true or false: %v", err)
		}
	}
	return want, nil
}

// handleSettingsAckNotice (POST /api/settings/ack-notice) dismisses the one-time
// notice that the reader's view did not come across from the settings file, by
// deleting the row the migration wrote. It is a route of its own because the
// page cannot write that key itself: the server's own keys are refused on
// /api/settings, which is what stops a filter change from clearing them.
func handleSettingsAckNotice(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := store.DeleteSetting(store.SettingsResetNotice); err != nil {
		log.Printf("ERROR [%s %q] failed to dismiss the notice: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to dismiss the notice: %v", err), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte(`{"ok": true}`))
}

func handleGetModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// No query string -> serve the markdown usage guide (for AI agents / humans).
	if r.URL.RawQuery == "" {
		handleAPIGuide(w, r)
		return
	}

	merged, err := processModels()
	if err != nil {
		log.Printf("ERROR [%s %q] processModels failed: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to process models: %v", err), http.StatusInternalServerError)
		return
	}

	// The columns are read per request because a filter names one, and the set is
	// not fixed: a column the reader invented is filterable the moment it exists.
	columns, err := store.ListColumns()
	if err != nil {
		log.Printf("ERROR [%s %q] list columns: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to list the columns: %v", err), http.StatusInternalServerError)
		return
	}

	out, err := filterAndProject(merged, r.URL.Query(), columns)
	if err != nil {
		// A parameter naming no column is refused rather than ignored: a filter
		// this server dropped would answer with the whole catalog while looking
		// like a narrow answer.
		log.Printf("ok /api/models refused: %v", err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	log.Printf("  ok Served %d/%d models", len(out), len(merged))
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(out)
}

// saveRequest is one item of a save: which model, and the cells to write on it.
// Both keys are named exactly as GET /api/models answers them, so a row read out
// of this API can be sent straight back with nothing looked up in between.
type saveRequest struct {
	Name  string            `json:"name"`
	Cells map[string]string `json:"cells"`
}

// saveResult is what the caller learns about ONE item of the batch: whether it
// was written and, where it was refused, the sentence saying what to correct.
type saveResult struct {
	Name    string `json:"name"`
	Written bool   `json:"written"`
	Error   string `json:"error,omitempty"`
}

// handleSaveModels (POST /api/save) writes cells onto models that ALREADY EXIST.
// The body is a JSON array of {"name": "...", "cells": {"<column key>": "<text>"}}.
//
// A BATCH IS HONEST, ITEM BY ITEM. Each item is written on its own, in one
// transaction, and the answer carries a result per item saying whether it was
// written and, where it was not, why. Nothing is hidden: stopping at the first
// bad item would tell the caller that nothing had happened when several models
// had changed.
//
// WHY PER ITEM AND NOT ONE TRANSACTION OVER THE WHOLE BATCH. The items are
// independent: each names a different model, and they arrive from an agent that
// researched several models in one pass. Discarding good research about six
// models because a seventh named a column that does not exist would lose work
// nothing else holds, and the caller cannot correct what it cannot see. What must
// be atomic is ONE model's write, and that it is: a refused key in an item leaves
// every cell of that model exactly as it was.
//
// THE STATUS SAYS WHICH CASE IT IS. Every item written answers 200. Some written
// and some refused answers 207, with the per-item results. NOTHING written
// answers the refusal of the first item - 404 for a model that is not here, 400
// for a key nothing typed may reach, a key naming no column, or a value the
// column will not hold - so a one-item call reads exactly as it always did.
func handleSaveModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// A key the request shape does not carry is REFUSED rather than ignored: a
	// model posted back under flat field names would otherwise be accepted and
	// write nothing at all.
	var updates []saveRequest
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&updates); err != nil {
		log.Printf("ERROR [%s %q] JSON decode failed: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Invalid JSON: %v. The body is an array of {\"name\":\"...\",\"cells\":{\"notes\":\"...\"}}.", err), decodeFailureStatus(err))
		return
	}

	log.Printf("ok /api/save called with %d updates", len(updates))

	results := make([]saveResult, 0, len(updates))
	written, firstFailure := 0, error(nil)
	for _, u := range updates {
		err := store.SaveCells(u.Name, u.Cells)
		if err != nil {
			if firstFailure == nil {
				firstFailure = err
			}
			log.Printf("ok /api/save refused %q: %v", u.Name, err)
			results = append(results, saveResult{Name: u.Name, Error: err.Error()})
			continue
		}
		written++
		results = append(results, saveResult{Name: u.Name, Written: true})
	}

	// Nothing written at all: answer the first refusal on its own terms, so a
	// caller sending one item gets the status that names what went wrong.
	if written == 0 && firstFailure != nil {
		http.Error(w, fmt.Sprintf("Nothing was written: %v.", firstFailure), statusOfWriteRefusal(firstFailure))
		return
	}
	if written > 0 && written < len(results) {
		log.Printf("  ok Saved %d of %d items; the rest are named in the answer", written, len(results))
	} else {
		log.Printf("  ok Saved %d updates to DB", written)
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if written < len(results) {
		w.WriteHeader(http.StatusMultiStatus)
	}
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok": written == len(results), "records": written, "results": results,
	})
}

// statusOfWriteRefusal turns a refusal from the write path into the HTTP status
// that says which kind it is: a model that is not here, or a request to correct.
func statusOfWriteRefusal(err error) int {
	switch {
	case errors.Is(err, store.ErrNoSuchModel):
		return http.StatusNotFound
	case errors.Is(err, store.ErrNotWritable), errors.Is(err, store.ErrCellValue),
		errors.Is(err, store.ErrColumnKey), errors.Is(err, store.ErrNoSuchColumn):
		return http.StatusBadRequest
	}
	return http.StatusInternalServerError
}
