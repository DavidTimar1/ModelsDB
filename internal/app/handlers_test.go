package app

import (
	"encoding/json"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestHandleSettingsRoundTrip pins the contract: what a write stores is what the
// next GET answers, with each value's type intact; a key the write does not name
// is left alone; and naming it in "delete" is what removes it.
func TestHandleSettingsRoundTrip(t *testing.T) {
	migratedDB(t)
	// The UI POSTs minified, one-line JSON (JSON.stringify).
	body := `{"set":{"f_search":"x","col_hidden":["rating"],"order":[[1,"asc"]],"page_length":50,"auto_update":true},"delete":[]}`
	rec := httptest.NewRecorder()
	handleSettings(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("POST status = %d, body=%s", rec.Code, rec.Body.String())
	}

	g := httptest.NewRecorder()
	handleSettings(g, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if g.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body=%s", g.Code, g.Body.String())
	}
	var got map[string]interface{}
	if err := json.Unmarshal(g.Body.Bytes(), &got); err != nil {
		t.Fatalf("GET body not valid JSON: %v (%s)", err, g.Body.String())
	}
	if got["f_search"] != "x" {
		t.Errorf("f_search = %v, want x", got["f_search"])
	}
	if got["page_length"] != float64(50) {
		t.Errorf("page_length = %#v, want the number 50", got["page_length"])
	}
	if got["auto_update"] != true {
		t.Errorf("auto_update = %#v, want true", got["auto_update"])
	}
	if hidden, ok := got["col_hidden"].([]interface{}); !ok || len(hidden) != 1 || hidden[0] != "rating" {
		t.Errorf("col_hidden = %#v, want [mine.rating]", got["col_hidden"])
	}

	// A second write naming only page_length leaves f_search exactly as it was.
	// This is what stops a page that loaded before a preference existed from
	// destroying it by not carrying it.
	rec2 := httptest.NewRecorder()
	handleSettings(rec2, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"set":{"page_length":25}}`)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("second POST status = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	g2 := httptest.NewRecorder()
	handleSettings(g2, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	var after map[string]interface{}
	if err := json.Unmarshal(g2.Body.Bytes(), &after); err != nil {
		t.Fatal(err)
	}
	if after["f_search"] != "x" {
		t.Errorf("f_search = %#v after a write that did not name it, want x", after["f_search"])
	}
	if after["page_length"] != float64(25) {
		t.Errorf("page_length = %#v, want the number 25", after["page_length"])
	}

	// Naming it in "delete" is what removes it.
	rec3 := httptest.NewRecorder()
	handleSettings(rec3, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"delete":["f_search"]}`)))
	if rec3.Code != http.StatusOK {
		t.Fatalf("delete POST status = %d, body=%s", rec3.Code, rec3.Body.String())
	}
	g3 := httptest.NewRecorder()
	handleSettings(g3, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	var afterDelete map[string]interface{}
	if err := json.Unmarshal(g3.Body.Bytes(), &afterDelete); err != nil {
		t.Fatal(err)
	}
	if _, still := afterDelete["f_search"]; still {
		t.Errorf("f_search should be gone after a delete naming it: %#v", afterDelete)
	}
	if afterDelete["page_length"] != float64(25) {
		t.Errorf("page_length = %#v after a delete of another key, want the number 25", afterDelete["page_length"])
	}
}

// TestHandleSettingsRefusesTheFlatBody pins the refusal of the whole-set shape
// versions before 8.2.0 posted. It has to be a refusal rather than a no-op:
// a body accepted quietly would store nothing and look like it had worked.
func TestHandleSettingsRefusesTheFlatBody(t *testing.T) {
	migratedDB(t)
	rec := httptest.NewRecorder()
	handleSettings(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(`{"page_length":50}`)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	g := httptest.NewRecorder()
	handleSettings(g, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if strings.Contains(g.Body.String(), "page_length") {
		t.Errorf("a refused POST must store nothing, got: %s", g.Body.String())
	}
}

// TestHandleSettingsRefusesAKeyInBothHalves pins that a key named in "set" and
// in "delete" is a 400 and writes nothing. Which half wins is not something to
// guess at: guessing is how a preference goes missing.
func TestHandleSettingsRefusesAKeyInBothHalves(t *testing.T) {
	migratedDB(t)
	rec := httptest.NewRecorder()
	body := `{"set":{"page_length":50,"f_search":"x"},"delete":["page_length"]}`
	handleSettings(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body)))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("POST status = %d, want 400 (body=%s)", rec.Code, rec.Body.String())
	}
	g := httptest.NewRecorder()
	handleSettings(g, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
	if strings.Contains(g.Body.String(), "f_search") {
		t.Errorf("a refused POST must store nothing, got: %s", g.Body.String())
	}
}

// TestHandleSettingsRefusesAServerKey pins the reserved namespace: the page must
// not be able to create, change or clear a key the server keeps for itself, from
// either half of a write, and the refusal stores nothing at all.
func TestHandleSettingsRefusesAServerKey(t *testing.T) {
	migratedDB(t)
	for _, body := range []string{
		`{"set":{"f_search":"x","server.notice.settings_reset":false}}`,
		`{"set":{"f_search":"x"},"delete":["server.notice.settings_reset"]}`,
	} {
		rec := httptest.NewRecorder()
		handleSettings(rec, httptest.NewRequest(http.MethodPost, "/api/settings", strings.NewReader(body)))
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("POST %s status = %d, want 400 (body=%s)", body, rec.Code, rec.Body.String())
		}
		g := httptest.NewRecorder()
		handleSettings(g, httptest.NewRequest(http.MethodGet, "/api/settings", nil))
		if strings.Contains(g.Body.String(), "f_search") {
			t.Errorf("a refused POST must store nothing, got: %s", g.Body.String())
		}
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
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models?include_hidden=0", nil))
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
			cells, ok := m["cells"].(map[string]interface{})
			if !ok {
				t.Errorf("cells should be a JSON object, got %T", m["cells"])
			} else if cells["repository"] != "x/y" {
				t.Errorf("the row's cells = %v, want the repository it was imported from", cells)
			}
		}
	}
	if !found {
		t.Errorf("served list should include 'Test HF' (got %d models)", len(arr))
	}
}

// TestHandleSaveModelsCustomValues pins that a column of the reader's own is
// written by the same request, under the same "cells" object, as every other
// value: there is one kind of column and one write path, so a heading somebody
// invented needs no second pipeline.
func TestHandleSaveModelsCustomValues(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("M", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	colID, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}

	body := `[{"name":"M","cells":{"notes":"hi",` + asJSON(colID) + `:"gold"}}]`
	rec := httptest.NewRecorder()
	handleSaveModels(rec, httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec.Code, rec.Body.String())
	}

	if got := colStr(t, "M", "notes"); got != "hi" {
		t.Errorf("curated notes = %q, want hi", got)
	}
	v, ok, err := ownValueOf("M", colID)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || v != "gold" {
		t.Errorf("custom value = %q, ok=%v, want \"gold\"", v, ok)
	}

	// Clearing (empty string) removes the stored value, as it does in any column.
	clearBody := `[{"name":"M","cells":{` + asJSON(colID) + `:""}}]`
	rec2 := httptest.NewRecorder()
	handleSaveModels(rec2, httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(clearBody)))
	if rec2.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rec2.Code, rec2.Body.String())
	}
	if _, ok, err := ownValueOf("M", colID); err != nil || ok {
		t.Errorf("value should be cleared, ok=%v err=%v", ok, err)
	}
}
