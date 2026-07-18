package app

import (
	"encoding/json"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestHandleSettingsSavedIndented(t *testing.T) {
	paths.SettingsJsonFile = filepath.Join(t.TempDir(), "settings.json")
	// The UI POSTs minified, one-line JSON (JSON.stringify).
	body := `{"f_search":"x","color_ranges":{"per second":{"low":0.05,"high":0.15}},"order":[[1,"asc"]]}`
	rec := httptest.NewRecorder()
	handleSettings(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d", rec.Code)
	}

	saved, err := os.ReadFile(paths.SettingsJsonFile)
	if err != nil {
		t.Fatal(err)
	}
	// It must be written indented (2-space), not the one-line input.
	if !strings.Contains(string(saved), "\n  \"") {
		t.Errorf("settings.json should be indented, got:\n%s", saved)
	}
	// ...and still be valid JSON carrying the same data.
	var m map[string]interface{}
	if err := json.Unmarshal(saved, &m); err != nil {
		t.Fatalf("saved settings not valid JSON: %v", err)
	}
	if m["f_search"] != "x" {
		t.Errorf("data lost in re-indent: %v", m["f_search"])
	}

	// GET reads it back.
	g := httptest.NewRecorder()
	handleSettings(g, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if g.Code != http.StatusOK || !strings.Contains(g.Body.String(), "f_search") {
		t.Errorf("GET settings failed: %d %s", g.Code, g.Body.String())
	}
}

func TestHandleHealth(t *testing.T) {
	rec := httptest.NewRecorder()
	handleHealth(rec, httptest.NewRequest(http.MethodGet, "/api/health", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET status = %d", rec.Code)
	}
	var m map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &m); err != nil {
		t.Fatalf("body not JSON: %v", err)
	}
	if m["status"] != "ok" {
		t.Errorf("status field = %v, want ok", m["status"])
	}

	// Wrong method is rejected.
	rec2 := httptest.NewRecorder()
	handleHealth(rec2, httptest.NewRequest(http.MethodPost, "/api/health", nil))
	if rec2.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST status = %d, want 405", rec2.Code)
	}
}

func TestHandleUpdateStatus(t *testing.T) {
	rec := httptest.NewRecorder()
	handleUpdateStatus(rec, httptest.NewRequest(http.MethodGet, "/api/update/status", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") {
		t.Errorf("content-type = %q", ct)
	}
}

func TestHandleGetModels(t *testing.T) {
	migratedDB(t)
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "Test HF", "source": "huggingface", "hf_slug": "x/y",
		"output_modalities": []interface{}{"text"},
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}

	// No query string -> the markdown usage guide.
	guide := httptest.NewRecorder()
	handleGetModels(guide, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	if guide.Code != http.StatusOK {
		t.Fatalf("guide status = %d", guide.Code)
	}
	if ct := guide.Header().Get("Content-Type"); !strings.Contains(ct, "markdown") {
		t.Errorf("guide content-type = %q, want markdown", ct)
	}

	// With a query -> JSON array including our model.
	rec := httptest.NewRecorder()
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models?all=1", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("models status = %d, body=%s", rec.Code, rec.Body.String())
	}
	var arr []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &arr); err != nil {
		t.Fatalf("body not a JSON array: %v (%s)", err, rec.Body.String())
	}
	found := false
	for _, m := range arr {
		if shared.GetStr(m, "name") == "Test HF" {
			found = true
		}
	}
	if !found {
		t.Errorf("served list should include 'Test HF' (got %d models)", len(arr))
	}
}
