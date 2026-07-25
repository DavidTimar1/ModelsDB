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
	"strconv"
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
			cv, ok := m["custom_values"].(map[string]interface{})
			if !ok {
				t.Errorf("custom_values should be a JSON object, got %T", m["custom_values"])
			} else if len(cv) != 0 {
				t.Errorf("a model with no custom values should get an empty custom_values object, got %v", cv)
			}
		}
	}
	if !found {
		t.Errorf("served list should include 'Test HF' (got %d models)", len(arr))
	}
}

// TestHandleCustomColumnsCRUD exercises the HTTP surface end to end: GET lists
// (the migration's Speed/Rating/OCR columns are already there on any migrated
// DB), POST creates a new one, and DELETE removes it and cascades away its
// values - mirroring what the "Custom columns" management UI drives.
func TestHandleCustomColumnsCRUD(t *testing.T) {
	migratedDB(t)

	baseline := httptest.NewRecorder()
	handleCustomColumns(baseline, httptest.NewRequest(http.MethodGet, "/api/custom-columns", nil))
	if baseline.Code != http.StatusOK {
		t.Fatalf("GET status = %d", baseline.Code)
	}
	var before []store.CustomColumn
	if err := json.Unmarshal(baseline.Body.Bytes(), &before); err != nil {
		t.Fatalf("GET body not JSON: %v", err)
	}

	createBody := `{"name":"Vendor","type":"dropdown_text","options":["acme","globex"]}`
	create := httptest.NewRecorder()
	handleCustomColumns(create, httptest.NewRequest(http.MethodPost, "/api/custom-columns", strings.NewReader(createBody)))
	if create.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body=%s", create.Code, create.Body.String())
	}
	var created store.CustomColumn
	if err := json.Unmarshal(create.Body.Bytes(), &created); err != nil {
		t.Fatalf("POST body not JSON: %v (%s)", err, create.Body.String())
	}
	if created.Name != "Vendor" || created.Type != store.ColumnTypeDropdownText || len(created.Options) != 2 {
		t.Errorf("created column = %+v", created)
	}

	after := httptest.NewRecorder()
	handleCustomColumns(after, httptest.NewRequest(http.MethodGet, "/api/custom-columns", nil))
	var list []store.CustomColumn
	if err := json.Unmarshal(after.Body.Bytes(), &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != len(before)+1 {
		t.Fatalf("column count after create = %d, want %d", len(list), len(before)+1)
	}

	del := httptest.NewRecorder()
	handleCustomColumns(del, httptest.NewRequest(http.MethodDelete, "/api/custom-columns?id="+strconv.FormatInt(created.ID, 10), nil))
	if del.Code != http.StatusOK {
		t.Fatalf("DELETE status = %d, body=%s", del.Code, del.Body.String())
	}

	final := httptest.NewRecorder()
	handleCustomColumns(final, httptest.NewRequest(http.MethodGet, "/api/custom-columns", nil))
	var listFinal []store.CustomColumn
	if err := json.Unmarshal(final.Body.Bytes(), &listFinal); err != nil {
		t.Fatal(err)
	}
	if len(listFinal) != len(before) {
		t.Fatalf("column count after delete = %d, want back to %d", len(listFinal), len(before))
	}

	// A missing/invalid id is rejected, not silently accepted.
	bad := httptest.NewRecorder()
	handleCustomColumns(bad, httptest.NewRequest(http.MethodDelete, "/api/custom-columns", nil))
	if bad.Code != http.StatusBadRequest {
		t.Errorf("DELETE with no id status = %d, want 400", bad.Code)
	}
}

// TestHandleSaveModelsCustomValues pins the /api/save extension: a batch item's
// optional custom_values object is applied via store.SetCustomValue alongside
// the fixed curated fields, in the same request - the whole point of folding
// custom-column saves into the existing autosave/retry pipeline instead of
// adding a second one.
func TestHandleSaveModelsCustomValues(t *testing.T) {
	migratedDB(t)
	if err := store.ImportFullRecord(map[string]interface{}{"name": "M", "source": "manual"}, "manual", "chat"); err != nil {
		t.Fatal(err)
	}
	colID, err := store.CreateCustomColumn("Tier", store.ColumnTypeDropdownText, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}

	body := `[{"name":"M","notes":"hi","custom_values":{"` + strconv.FormatInt(colID, 10) + `":"gold"}}]`
	rec := httptest.NewRecorder()
	handleSaveModels(rec, httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	if got := colStr(t, "M", "notes"); got != "hi" {
		t.Errorf("curated notes = %q, want hi", got)
	}
	v, ok, err := store.GetCustomValue("M", colID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || v != "gold" {
		t.Errorf("custom value = %q, ok=%v, want \"gold\"", v, ok)
	}

	// Clearing (empty string) removes the stored value, same as a direct
	// SetCustomValue call.
	clearBody := `[{"name":"M","custom_values":{"` + strconv.FormatInt(colID, 10) + `":""}}]`
	rec2 := httptest.NewRecorder()
	handleSaveModels(rec2, httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(clearBody)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	if _, ok, err := store.GetCustomValue("M", colID); err != nil || ok {
		t.Errorf("value should be cleared, ok=%v err=%v", ok, err)
	}
}
