package app

import (
	"encoding/json"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// makeRow puts one plain model row in, which is all a stored cell needs on the
// other side of its foreign key.
func makeRow(t *testing.T, name string) {
	t.Helper()
	if _, err := dbcore.DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES (?,'openrouter','t')`, name); err != nil {
		t.Fatalf("insert the row %q: %v", name, err)
	}
}

// recordAuthor records one author through the catalogue, exactly as an
// unattended refresh does. OpenRouter's reading of "author" lands in the
// "vendor" column (factCellKeysBySource), which is a column the reader's own
// "Author" column never shares - a fact whose answer means something different
// depending on who states it gets a column per source, never one merged column.
func recordAuthor(t *testing.T, model, author string) {
	t.Helper()
	if _, err := store.RecordFacts(model, store.SourceOpenRouter, map[string]string{"author": author}); err != nil {
		t.Fatalf("record the author of %q: %v", model, err)
	}
}

// keepAuthorColumn is the reader deciding to keep a column of their own for the
// author, which is the only thing that creates one. It is unrelated to the
// "vendor" column OpenRouter states its own reading of the author in.
func keepAuthorColumn(t *testing.T) string {
	t.Helper()
	key, err := ownColumnKey(t, "Author", store.ValueTypeText, nil)
	if err != nil {
		t.Fatalf("keep a column for the author: %v", err)
	}
	return key
}

// listColumns reads the column definitions back over HTTP, which is the one list
// a caller needs: every key, its heading, and whether it is sealed.
func listColumns(t *testing.T) []store.Column {
	t.Helper()
	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodGet, "/api/columns", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/columns answered %d: %s", rec.Code, rec.Body.String())
	}
	var cols []store.Column
	if err := json.Unmarshal(rec.Body.Bytes(), &cols); err != nil {
		t.Fatalf("read the column list: %v", err)
	}
	return cols
}

// columnCalled answers the column carrying that heading, and whether one does.
func columnCalled(t *testing.T, label string) (store.Column, bool) {
	t.Helper()
	for _, c := range listColumns(t) {
		if strings.EqualFold(c.Label, label) {
			return c, true
		}
	}
	return store.Column{}, false
}

// renameColumn asks the server to relabel a column and answers what it said. A
// column is named by its KEY, which never changes, so the heading is free to.
func renameColumn(t *testing.T, key, label string) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(`{"key":` + asJSON(key) + `,"label":` + asJSON(label) + `}`)
	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodPatch, "/api/columns", body))
	return rec
}

// deleteColumnOverHTTP asks the server to delete a column, with or without the
// confirmation the route requires in the body.
func deleteColumnOverHTTP(t *testing.T, key string, confirm bool) *httptest.ResponseRecorder {
	t.Helper()
	body := strings.NewReader(`{"key":` + asJSON(key) + `,"confirm":` + asJSON(confirm) + `}`)
	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodDelete, "/api/columns", body))
	return rec
}

// asJSON renders one value as the JSON text a request body carries it as.
func asJSON(v interface{}) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// TestARenamedColumnKeepsItsValuesAndStillReceivesThem is the whole reason a
// column's key and its heading are separate. The rename is the change a person
// makes to fix a heading they dislike, and it must cost them neither the value
// already stored nor a later hand write to the same key.
func TestARenamedColumnKeepsItsValuesAndStillReceivesThem(t *testing.T) {
	migratedDB(t)
	makeRow(t, "First")
	makeRow(t, "Second")
	key := keepAuthorColumn(t)
	if err := setOwnValue("First", key, "Qwen"); err != nil {
		t.Fatalf("type the author by hand: %v", err)
	}

	if rec := renameColumn(t, key, "Publisher"); rec.Code != http.StatusOK {
		t.Fatalf("the rename answered %d: %s", rec.Code, rec.Body.String())
	}

	value, ok, err := ownValueOf("First", key)
	if err != nil {
		t.Fatal(err)
	}
	if !ok || value != "Qwen" {
		t.Errorf("the stored value came through as %q (stored: %v), want Qwen", value, ok)
	}

	// A later hand write for another model still answers under the SAME column,
	// under its new heading, rather than building a second one beside it.
	if err := setOwnValue("Second", key, "Meta"); err != nil {
		t.Fatalf("type the author by hand: %v", err)
	}
	if value, ok, err := ownValueOf("Second", key); err != nil || !ok || value != "Meta" {
		t.Errorf("the later write reads back as %q (stored: %v, err %v), want Meta", value, ok, err)
	}
	for _, c := range listColumns(t) {
		if c.Key == key && c.Label != "Publisher" {
			t.Errorf("the column is called %q, want Publisher", c.Label)
		}
		if c.Key != key && !c.Sealed && strings.EqualFold(c.Label, "Author") {
			t.Error("a second Author column was created beside the renamed one")
		}
	}
}

// TestARenameRefusesANameAnotherColumnHolds pins the one thing a rename must
// never do: two columns of one heading leave a person unable to tell them apart.
func TestARenameRefusesANameAnotherColumnHolds(t *testing.T) {
	migratedDB(t)
	first, err := ownColumnKey(t, "Vendor tag", store.ValueTypeText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ownColumnKey(t, "Tier", store.ValueTypeText, nil); err != nil {
		t.Fatal(err)
	}
	if rec := renameColumn(t, first, "tier"); rec.Code != http.StatusBadRequest {
		t.Fatalf("renaming onto a name in use answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	for _, c := range listColumns(t) {
		if c.Key == first && c.Label != "Vendor tag" {
			t.Errorf("the refused rename still changed the heading to %q", c.Label)
		}
	}
}

// TestADeletedColumnOfTheReadersOwnStaysDeletedAcrossAnImport pins the second
// half of the identity: the refresh runs unattended on every start, and a
// source writes only its own declared columns - it can never recreate a column
// the reader deleted, because that column is not one it is allowed to state.
func TestADeletedColumnOfTheReadersOwnStaysDeletedAcrossAnImport(t *testing.T) {
	migratedDB(t)
	makeRow(t, "First")
	key := keepAuthorColumn(t)
	if err := setOwnValue("First", key, "Qwen"); err != nil {
		t.Fatalf("type the author by hand: %v", err)
	}
	if err := store.DeleteColumn(key); err != nil {
		t.Fatalf("delete the column: %v", err)
	}

	recordAuthor(t, "First", "Qwen")
	if _, held := columnCalled(t, "Author"); held {
		t.Error("an unattended refresh recreated the deleted Author column")
	}
}

// TestADeletedColumnStaysDeletedAfterARename covers the case a rename could
// hide: the source knows the column by the key, and a heading change does not
// move the key, so a refresh after a rename-then-delete has to stay as silent
// as one after a plain delete.
func TestADeletedColumnStaysDeletedAfterARename(t *testing.T) {
	migratedDB(t)
	makeRow(t, "First")
	key := keepAuthorColumn(t)
	if err := setOwnValue("First", key, "Qwen"); err != nil {
		t.Fatalf("type the author by hand: %v", err)
	}
	if rec := renameColumn(t, key, "Publisher"); rec.Code != http.StatusOK {
		t.Fatalf("the rename answered %d: %s", rec.Code, rec.Body.String())
	}
	if err := store.DeleteColumn(key); err != nil {
		t.Fatalf("delete the column: %v", err)
	}

	recordAuthor(t, "First", "Qwen")
	if _, held := columnCalled(t, "Publisher"); held {
		t.Error("an unattended refresh recreated the deleted column under its new heading")
	}
	if _, held := columnCalled(t, "Author"); held {
		t.Error("an unattended refresh recreated the deleted column under its old heading")
	}
}

// TestAColumnIsListedWithTheFeedPathsThatFillIt is what lets the Columns
// dialog find a column by the feed's own name for it. He typed "hug" looking
// for the column the feed calls hugging_face_id and found nothing, because that
// path fills the column headed Repository. Both tables feed the answer: the
// shipped mapping, and the columns the refresh made from the feed's other paths.
func TestAColumnIsListedWithTheFeedPathsThatFillIt(t *testing.T) {
	migratedDB(t)
	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}
	if _, _, err := store.RecordFeedFields(map[string]store.FieldObservation{
		"benchmarks.artificial_analysis.coding_index": {Kind: "number", Sample: "41.5", Models: 1},
	}); err != nil {
		t.Fatalf("record the feed field: %v", err)
	}
	if _, err := store.CreateFeedFieldColumn("benchmarks.artificial_analysis.coding_index", store.ValueTypeNumber); err != nil {
		t.Fatalf("CreateFeedFieldColumn: %v", err)
	}

	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodGet, "/api/columns", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/columns answered %d: %s", rec.Code, rec.Body.String())
	}
	var cols []struct {
		Key       string   `json:"key"`
		FeedPaths []string `json:"feed_paths"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &cols); err != nil {
		t.Fatalf("read the column list: %v", err)
	}
	paths := map[string][]string{}
	for _, c := range cols {
		paths[c.Key] = c.FeedPaths
	}
	if got := strings.Join(paths["repository"], ","); got != "hugging_face_id" {
		t.Errorf("repository is filled by the feed's hugging_face_id, listed feed_paths %q", got)
	}
	if got := strings.Join(paths["benchmarks_artificial_analysis_coding_index"], ","); got != "benchmarks.artificial_analysis.coding_index" {
		t.Errorf("a column made from the feed must list the path it was made from, got %q", got)
	}
	if got := paths["notes"]; len(got) != 0 {
		t.Errorf("notes is fed by no feed path and must carry none, got %q", got)
	}
}
