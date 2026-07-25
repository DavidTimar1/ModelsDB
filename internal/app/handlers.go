// Package app handles handlers logic and functionalities.
package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/paths"
	"modelsdb/internal/selfupdate"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":  "ok",
		"port":    paths.Port,
		"data":    paths.DataDir,
		"version": shared.Version,
	})
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
			"database":      paths.DbFile,
			"curated.json":  paths.CuratedJsonFile,
			"config.jsonc":  paths.ConfigFile,
			"settings.json": paths.SettingsJsonFile,
			"backups":       paths.BackupsDir,
			"logs":          paths.LogsDir,
			"pid":           paths.PidFile,
			"update-check":  paths.UpdateCacheFile,
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
		log.Printf("ERROR [%s %s] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if !refresh.start() {
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
		log.Printf("ERROR [%s %s] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(refresh.snapshot())
}

func handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		b, err := os.ReadFile(paths.SettingsJsonFile)
		if err != nil {
			b = []byte(`{}`)
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write(b)
	case http.MethodPost:
		b, err := io.ReadAll(r.Body)
		if err != nil {
			log.Printf("ERROR [%s %s] failed to read body: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusBadRequest)
			return
		}
		// Store settings.json human-readable (indented, not one line). json.Indent
		// preserves the data and key order; if the body is not valid JSON (it always
		// is, from the UI) fall back to writing it verbatim.
		var pretty bytes.Buffer
		if json.Indent(&pretty, b, "", "  ") == nil {
			b = append(pretty.Bytes(), '\n')
		}
		os.MkdirAll(filepath.Dir(paths.SettingsJsonFile), 0755)
		if err := os.WriteFile(paths.SettingsJsonFile, b, 0644); err != nil {
			log.Printf("ERROR [%s %s] failed to write settings.json: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Failed to write settings: %v", err), http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Write([]byte(`{"ok": true}`))
	default:
		log.Printf("ERROR [%s %s] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleCustomColumns (GET/POST/DELETE /api/custom-columns) is the CRUD surface
// for user-defined personal columns: GET lists every column definition (with its
// dropdown options, if any); POST creates one from {name, type, options}; DELETE
// (?id=<id>) removes one, which cascades to wipe every model's stored value for
// it (enforced by the DB's ON DELETE CASCADE, not by code here). Per-model
// VALUES are not served here - they ride embedded in each row of
// GET /api/models (custom_values), and are written back alongside the curated
// fields in a normal /api/save batch (see handleSaveModels below), so the
// existing autosave/retry/debounce machinery covers custom columns too without
// a second save pipeline.
func handleCustomColumns(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	switch r.Method {
	case http.MethodGet:
		cols, err := store.ListCustomColumns()
		if err != nil {
			log.Printf("ERROR [%s %s] list custom columns: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Failed to list custom columns: %v", err), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(cols)

	case http.MethodPost:
		var body struct {
			Name    string   `json:"name"`
			Type    string   `json:"type"`
			Options []string `json:"options"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			log.Printf("ERROR [%s %s] JSON decode failed: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
			return
		}
		id, err := store.CreateCustomColumn(body.Name, body.Type, body.Options)
		if err != nil {
			log.Printf("ERROR [%s %s] create custom column: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Failed to create custom column: %v", err), http.StatusBadRequest)
			return
		}
		cols, err := store.ListCustomColumns()
		if err != nil {
			log.Printf("ERROR [%s %s] list after create: %v", r.Method, r.URL.Path, err)
			http.Error(w, fmt.Sprintf("Column created but failed to list it back: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("ok /api/custom-columns created id=%d name=%q type=%q", id, body.Name, body.Type)
		for _, c := range cols {
			if c.ID == id {
				json.NewEncoder(w).Encode(c)
				return
			}
		}
		json.NewEncoder(w).Encode(map[string]interface{}{"id": id})

	case http.MethodDelete:
		id, err := strconv.ParseInt(r.URL.Query().Get("id"), 10, 64)
		if err != nil {
			http.Error(w, "Invalid or missing id", http.StatusBadRequest)
			return
		}
		if err := store.DeleteCustomColumn(id); err != nil {
			log.Printf("ERROR [%s %s] delete custom column %d: %v", r.Method, r.URL.Path, id, err)
			http.Error(w, fmt.Sprintf("Failed to delete custom column: %v", err), http.StatusInternalServerError)
			return
		}
		log.Printf("ok /api/custom-columns deleted id=%d", id)
		json.NewEncoder(w).Encode(map[string]interface{}{"ok": true})

	default:
		log.Printf("ERROR [%s %s] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

func handleGetModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("ERROR [%s %s] unsupported method", r.Method, r.URL.Path)
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
		log.Printf("ERROR [%s %s] processModels failed: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to process models: %v", err), http.StatusInternalServerError)
		return
	}

	out := filterAndProject(merged, r.URL.Query())
	log.Printf("  ok Served %d/%d models (query: %s)", len(out), len(merged), r.URL.RawQuery)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(out)
}

// handleSaveModels (POST /api/save) applies a batch of partial curated updates.
// Each item is keyed by model name and may carry any of the fixed curated
// fields (applied via store.SaveCurated's read-modify-write merge) plus an
// optional `custom_values` object - {"<column id>": "<value>"} - for the
// user-defined personal columns. Unlike the fixed fields, a custom value needs
// no merge step (each column is already an independent, sparse row), so it is
// applied directly via store.SetCustomValue, one call per entry.
func handleSaveModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %s] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var updates []map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&updates); err != nil {
		log.Printf("ERROR [%s %s] JSON decode failed: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), http.StatusBadRequest)
		return
	}

	log.Printf("ok /api/save called with %d updates", len(updates))

	saved := 0
	for _, u := range updates {
		name := shared.GetStr(u, "name")
		if name == "" {
			continue
		}
		if err := store.SaveCurated(name, u); err != nil {
			log.Printf("ERROR saving %q: %v", name, err)
			http.Error(w, fmt.Sprintf("Failed to save %q: %v", name, err), http.StatusInternalServerError)
			return
		}
		if cv, ok := u["custom_values"].(map[string]interface{}); ok {
			for k, v := range cv {
				colID, err := strconv.ParseInt(k, 10, 64)
				if err != nil {
					continue
				}
				s, _ := v.(string)
				if err := store.SetCustomValue(name, colID, s); err != nil {
					log.Printf("ERROR saving custom value (model=%q column=%d): %v", name, colID, err)
					http.Error(w, fmt.Sprintf("Failed to save custom value for %q: %v", name, err), http.StatusInternalServerError)
					return
				}
			}
		}
		saved++
	}

	if saved > 0 {
		if err := store.ExportData(); err != nil {
			log.Printf("Warning: export data failed: %v", err)
		}
		log.Printf("  ok Saved %d updates to DB", saved)
	} else {
		log.Printf("  ok No updates to save")
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "records": saved})
}
