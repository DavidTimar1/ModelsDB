package app

import (
	"database/sql"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// customValue reads a model's stored value for a column, by the NAME that column
// carries on screen. It is how every fact that used to be a column of its own is
// read back now, so a test says which column it means in the words the user
// sees, whichever key the value really lives under.
//
// The reader's own answer comes first and the source's after it, which is the
// order every read in the app prefers them in.
func customValue(t *testing.T, model, column string) string {
	t.Helper()
	cells, err := store.ModelCells(model)
	if err != nil {
		t.Fatalf("read the cells of %q: %v", model, err)
	}
	for _, key := range cellKeysOfColumnName(column) {
		if v := strings.TrimSpace(cells[key]); v != "" {
			return v
		}
	}
	return ""
}

// cellKeysOfColumnName names every cell key one heading's value can be found
// in. One heading usually names one cell key; "Author" is the one exception,
// where what OpenRouter attributes the model to and who published the
// repository are two different facts sharing a heading, each in its own
// column (see store.factCellKeysBySource).
func cellKeysOfColumnName(column string) []string {
	if strings.EqualFold(column, "Author") {
		return []string{"vendor", "repo_owner"}
	}
	// "Active parameters (B)" is a shipped column, not a Fact - store.Facts only
	// carries "Parameters (B)" - so it stems wrong through the generic fallback
	// below and needs naming directly, the same way store.namedColumnKeys does.
	if strings.EqualFold(column, "Active parameters (B)") {
		return []string{"active_parameters"}
	}
	facts := store.FactCellKeys()
	for _, f := range store.Facts {
		if !strings.EqualFold(f.Column, column) {
			continue
		}
		if key, mapped := facts[f.Key]; mapped {
			return []string{key}
		}
		return []string{f.Key}
	}
	return []string{columnKeyStem(column)}
}

// columnKeyStem turns a heading into the word a column key carries after its
// namespace, the same way the app does: lower case, every run of anything else
// one underscore.
func columnKeyStem(name string) string {
	var b strings.Builder
	lastWasBreak := true
	for _, r := range strings.ToLower(strings.TrimSpace(name)) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastWasBreak = false
			continue
		}
		if !lastWasBreak {
			b.WriteByte('_')
			lastWasBreak = true
		}
	}
	return strings.Trim(b.String(), "_")
}

// dataDirFiles lists the plain filenames in the test's data dir.
func dataDirFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(paths.DataDir)
	if err != nil {
		t.Fatalf("read the data dir %s: %v", paths.DataDir, err)
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names
}

// assertOnlyTheDatabaseInDataDir fails if the data dir holds anything but
// modelsdb.db and the sidecars SQLite keeps beside it. It names no file it is
// watching for, so nothing can be renamed around it.
func assertOnlyTheDatabaseInDataDir(t *testing.T, after string) {
	t.Helper()
	for _, name := range dataDirFiles(t) {
		if !strings.HasPrefix(name, "modelsdb.db") {
			t.Errorf("after %s the data dir holds %q; the database and its sidecars are the only files there", after, name)
		}
	}
}

// TestNoFileArtefactIsWrittenByAnyWritePath pins the model in one place: the
// database is the only thing this app writes model data to. Each verb below
// changes real data, and none of them may leave a file behind - not a catalog,
// not a personal file, not anything.
func TestNoFileArtefactIsWrittenByAnyWritePath(t *testing.T) {
	migratedDB(t)

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "Kept HF", "source": "huggingface", "hf_slug": "x/kept",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	assertOnlyTheDatabaseInDataDir(t, "adding a model from an upstream shape")

	// Adding a model by hand, through the real HTTP handler. It is the write path
	// that creates a row from nothing but what somebody typed.
	add := httptest.NewRecorder()
	handleAddModel(add, httptest.NewRequest(http.MethodPost, "/api/models/add",
		strings.NewReader(`{"name":"Hand Added","cells":{"notes":"typed here"}}`)))
	if add.Code != http.StatusCreated {
		t.Fatalf("POST /api/models/add status = %d, body %s", add.Code, add.Body.String())
	}
	assertOnlyTheDatabaseInDataDir(t, "adding a model by hand")

	// Importing a model from a HuggingFace address, through the real HTTP
	// handler. It is the write path that brings the most read-in facts into one
	// row at once - the repository, the author, the modalities, the context
	// length and the measured on-disk size - so it is exactly the shape that
	// would tempt somebody to keep a copy of what was fetched.
	hfCatalogServer(t)
	imported := httptest.NewRecorder()
	handleImportHF(imported, httptest.NewRequest(http.MethodPost, "/api/models/import",
		strings.NewReader(`{"url":"https://huggingface.co/`+testImportRepo+`"}`)))
	if imported.Code != http.StatusCreated {
		t.Fatalf("POST /api/models/import status = %d, body %s", imported.Code, imported.Body.String())
	}
	if got := colStr(t, testImportName, "hf_slug"); got != testImportRepo {
		t.Fatalf("the imported row did not reach the database: hf_slug = %q", got)
	}
	assertOnlyTheDatabaseInDataDir(t, "importing a model from HuggingFace")

	// A save, through the real HTTP handler, into a note and a column of the
	// reader's own.
	if _, err := store.CreateOwnColumn(store.ColumnSpec{Label: "Rating"}); err != nil {
		t.Fatal(err)
	}
	save := httptest.NewRecorder()
	handleSaveModels(save, httptest.NewRequest(http.MethodPost, "/api/save",
		strings.NewReader(`[{"name":"Kept HF","cells":{"notes":"my note","rating":"3"}}]`)))
	if save.Code != http.StatusOK {
		t.Fatalf("POST /api/save status = %d, body %s", save.Code, save.Body.String())
	}
	assertOnlyTheDatabaseInDataDir(t, "a save")

	// Hiding.
	hide := httptest.NewRecorder()
	handleHideModel(hide, httptest.NewRequest(http.MethodPost, "/api/models/hide",
		strings.NewReader(`{"name":"Kept HF","hidden":true}`)))
	if hide.Code != http.StatusOK {
		t.Fatalf("POST /api/models/hide status = %d, body %s", hide.Code, hide.Body.String())
	}
	assertOnlyTheDatabaseInDataDir(t, "hiding a model")

	// Recording a variation of a model. It is the write path that brings the most
	// facts into the database at once - the repository, the revision, the file
	// list and the exact byte count - so it is exactly the
	// shape that would tempt somebody to write a file: it must land in the
	// database and nowhere else.
	hfCatalogServer(t)
	seedParent(t)
	variation := httptest.NewRecorder()
	handleVariations(variation, httptest.NewRequest(http.MethodPost, "/api/models/variations",
		strings.NewReader(`{"name":"`+testParent+`","repo":"`+testBuildRepo+`","build":"`+testBuildFile+`"}`)))
	if variation.Code != http.StatusOK {
		t.Fatalf("POST /api/models/variations status = %d, body %s", variation.Code, variation.Body.String())
	}
	if got := customValue(t, testBuildName, "Build format"); got == "" {
		t.Fatalf("what the importer measured did not reach the database: build format = %q", got)
	}
	assertOnlyTheDatabaseInDataDir(t, "recording a variation")

	// Deleting a model nothing re-creates.
	del := httptest.NewRecorder()
	handleDeleteModel(del, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Kept HF","confirm_delete":true}`)))
	if del.Code != http.StatusOK {
		t.Fatalf("POST /api/models/delete status = %d, body %s", del.Code, del.Body.String())
	}
	assertOnlyTheDatabaseInDataDir(t, "a delete")
}

// TestPersonalDataStaysInDBNeverExported pins the same model from the other
// side: the personal things a user records - a note, a favourite - are readable
// in the database afterwards and appear in no file at all.
func TestPersonalDataStaysInDBNeverExported(t *testing.T) {
	migratedDB(t)

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "M", "source": "huggingface", "hf_slug": "x/m",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	// Set personal data the way the UI save path does (the database only).
	if err := store.SaveCells("M", map[string]string{
		"notes": "my note", "favorite": "yes",
	}); err != nil {
		t.Fatal(err)
	}
	if got := colStr(t, "M", "notes"); got != "my note" {
		t.Fatalf("the note is not in the database: %q", got)
	}
	if got := colInt(t, "M", "favorite"); got != 1 {
		t.Fatalf("the favourite flag is not in the database: %d", got)
	}
	assertOnlyTheDatabaseInDataDir(t, "recording personal data")
}

// TestFreshInstallStartsWithAnEmptyCatalog pins what a fresh database is: empty,
// and empty again however many times it is opened. The app carries no catalog of
// its own and reads none from disk - every model arrives from OpenRouter, which
// is a network call this test deliberately never makes.
func TestFreshInstallStartsWithAnEmptyCatalog(t *testing.T) {
	migratedDB(t)

	// Two starts, because a start that populated the database on the first would
	// leave the second with a populated one and so look correct for the wrong
	// reason.
	for run := 1; run <= 2; run++ {
		n, err := dbcore.CountModels()
		if err != nil {
			t.Fatalf("run %d: count models: %v", run, err)
		}
		if n != 0 {
			t.Fatalf("run %d: a fresh install must start with 0 models, got %d", run, n)
		}
		if err := dbcore.DB.Close(); err != nil {
			t.Fatalf("run %d: close the database: %v", run, err)
		}
		if err := dbcore.OpenDB(paths.DbFile); err != nil {
			t.Fatalf("run %d: reopen the database: %v", run, err)
		}
	}
	assertOnlyTheDatabaseInDataDir(t, "starting twice on an empty database")
}

// TestACustomColumnValueSurvivesARefresh pins the researched size, which is a
// custom column now: the OpenRouter refresh writes the models table and touches
// custom_values not at all, so a figure somebody looked up by hand cannot be
// clobbered by a startup.
func TestACustomColumnValueSurvivesARefresh(t *testing.T) {
	migratedDB(t)

	norm := map[string]interface{}{"name": "OR Sized", "id": "x/or-sized"}
	if _, err := store.UpsertAPIModel(norm); err != nil {
		t.Fatal(err)
	}
	// Weights size (GB) is a shipped column now, sealed by the fetch that just
	// created the row above, so this reads its own key rather than minting a
	// colliding one of the reader's own.
	col, held := columnCalled(t, "Weights size (GB)")
	if !held {
		t.Fatal("Weights size (GB) should already be shipped")
	}
	id := col.Key
	if err := setOwnValue("OR Sized", id, "12.5"); err != nil {
		t.Fatal(err)
	}
	if got := customValue(t, "OR Sized", "Weights size (GB)"); got != "12.5" {
		t.Fatalf("the size was not stored: %q", got)
	}
	if _, err := store.UpsertAPIModel(norm); err != nil {
		t.Fatal(err)
	}
	if got := customValue(t, "OR Sized", "Weights size (GB)"); got != "12.5" {
		t.Fatalf("a refresh clobbered the size: %q", got)
	}
}

// TestBackupCarriesPersonalDataOutOfTheDatabase is the restore side of the same
// model. Personal data lives only in the database, so a copy of the database is
// the only backup that has it - and this drives the engine the `backup` verb
// calls to prove that copy really does.
func TestBackupCarriesPersonalDataOutOfTheDatabase(t *testing.T) {
	migratedDB(t)

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "Kept HF", "source": "huggingface", "hf_slug": "x/kept",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCells("Kept HF", map[string]string{"notes": "my note"}); err != nil {
		t.Fatal(err)
	}
	colID, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold"})
	if err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("Kept HF", colID, "gold"); err != nil {
		t.Fatal(err)
	}

	dest := t.TempDir()
	path, err := dbcore.BackupTo(dest, dbcore.TimestampedBackup)
	if err != nil {
		t.Fatalf("BackupTo: %v", err)
	}
	entries, err := os.ReadDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("a backup should write exactly one file, found %d", len(entries))
	}
	if filepath.Ext(path) != ".db" {
		t.Errorf("the backup file is %q, want a .db file", path)
	}

	copyDB, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open the backup: %v", err)
	}
	defer copyDB.Close()

	// The note is a cell, so it is read out of the backup where it really lives.
	var notes string
	if err := copyDB.QueryRow(`SELECT v.value_text FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.column_key='notes' AND v.model_name=?`, "Kept HF").Scan(&notes); err != nil {
		t.Fatalf("read the note back out of the backup: %v", err)
	}
	if notes != "my note" {
		t.Errorf("the backup holds notes = %q, want \"my note\"", notes)
	}
	var value string
	if err := copyDB.QueryRow(`SELECT v.value_text FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE v.model_name=? AND c.column_key=?`, "Kept HF", colID).Scan(&value); err != nil {
		t.Fatalf("read the custom-column value back out of the backup: %v", err)
	}
	if value != "gold" {
		t.Errorf("the backup holds the custom-column value %q, want \"gold\"", value)
	}
}

// TestFetchModelsWritesNoFileArtefact extends the no-file-artefact guarantee to
// the catalog REFRESH path, which TestNoFileArtefactIsWrittenByAnyWritePath
// above does not reach because it goes over the network. catalog.ApiUrl and
// catalog.AllProvidersURL are both package-level vars for exactly this reason:
// this test points them at a local httptest.Server serving a canned
// OpenRouter-shaped payload and an empty provider-policy map, and runs
// FetchModels end to end with no real request to openrouter.ai.
//
// The canned model deliberately carries NO "id" field. deriveZDR's per-model
// endpoint fetch (modelEndpointTags) is the one request in the refresh that
// still builds its URL inline from each model's id rather than from a named
// var, so a model WITH an id would reach the real network there. An id-less
// model is excluded by deriveZDR's own query
// (`WHERE source='openrouter' AND id != ”`), so its ZDR job list is empty and
// that remaining real-network call is never reached at all, keeping this test
// fully offline while still exercising the write path (store.UpsertAPIModel)
// that a user's "Update models" click and every startup actually run.
//
// The other two refresh phases have a test of their own directly below, on the
// same injectable-URL footing.
func TestFetchModelsWritesNoFileArtefact(t *testing.T) {
	migratedDB(t)

	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"name":"Test: Refresh Model","canonical_slug":"test-vendor/refresh-model","context_length":8192,"architecture":{"input_modalities":["text"],"output_modalities":["text"]},"supported_parameters":["tools"],"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
	})
	mux.HandleFunc("/all-providers", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/videos", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/images", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	mux.HandleFunc("/eu", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[]}`))
	})
	server := httptest.NewServer(mux)
	defer server.Close()

	origAPI, origProviders := catalog.ApiUrl, catalog.AllProvidersURL
	origVideo, origImage := catalog.VideoModelsURL, catalog.ImageModelsURL
	catalog.ApiUrl = server.URL + "/models"
	origEU := catalog.EURoutingURL
	t.Cleanup(func() { catalog.EURoutingURL = origEU })
	catalog.EURoutingURL = server.URL + "/eu"
	catalog.AllProvidersURL = server.URL + "/all-providers"
	catalog.VideoModelsURL = server.URL + "/videos"
	catalog.ImageModelsURL = server.URL + "/images"
	defer func() {
		catalog.ApiUrl = origAPI
		catalog.AllProvidersURL = origProviders
		catalog.VideoModelsURL = origVideo
		catalog.ImageModelsURL = origImage
	}()

	// Seeded first so the refresh MEETS a row rather than only creating one:
	// the file rule has to hold on the update path as well as the insert.
	if _, err := store.UpsertAPIModel(catalog.NormalizeModel(map[string]interface{}{
		"name": "Test: Refresh Model", "canonical_slug": "test-vendor/refresh-model",
	})); err != nil {
		t.Fatal(err)
	}

	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if n, err := dbcore.CountModels(); err != nil || n != 1 {
		t.Fatalf("expected the canned model to be upserted, count=%d err=%v", n, err)
	}
	assertOnlyTheDatabaseInDataDir(t, "a catalog refresh (the update verb / startup fetch)")
}
