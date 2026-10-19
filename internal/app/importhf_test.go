package app

import (
	"encoding/json"
	"fmt"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// importModel drives the real POST handler and returns the recorder.
func importModel(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleImportHF(rec, httptest.NewRequest(http.MethodPost, "/api/models/import", strings.NewReader(body)))
	return rec
}

// importedBody decodes a successful import's answer.
func importedBody(t *testing.T, rec *httptest.ResponseRecorder) map[string]interface{} {
	t.Helper()
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v (%s)", err, rec.Body.String())
	}
	return body
}

// TestImportBuildsTheWholeRowFromTheRepository is the route's central test: one
// address goes in, and every column comes out of what the server read on
// HuggingFace rather than out of the request.
func TestImportBuildsTheWholeRowFromTheRepository(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("POST /api/models/import status = %d, want 201: %s", rec.Code, rec.Body.String())
	}
	body := importedBody(t, rec)
	if body["name"] != testImportName {
		t.Errorf("the row was created as %v, want %q", body["name"], testImportName)
	}
	if !store.ModelExists(testImportName) {
		t.Fatalf("no row called %q exists after a 201", testImportName)
	}

	for _, c := range []struct{ col, want string }{
		{"source", "huggingface"},
		{"hf_slug", testImportRepo},
		{"model_type", "chat"},
		{"input_modalities", `["text"]`},
		{"output_modalities", `["text"]`},
		{"moe", "yes"},
		{"created_at", "2026-01-04T09:00:00.000Z"},
	} {
		if got := colStr(t, testImportName, c.col); got != c.want {
			t.Errorf("%s = %q, want %q", c.col, got, c.want)
		}
	}
	if got := colInt(t, testImportName, "context_length"); got != 32768 {
		t.Errorf("context_length = %d, want 32768 from the config's max_position_embeddings", got)
	}
	// 7,248,023,552 stored elements, every one of them a BF16 parameter - so the
	// count is 7.248023552 billion, and the column holds it at the two decimal
	// place this app stores a parameter count at.
	if got := customValue(t, testImportName, "Parameters (B)"); got != "7.25" {
		t.Errorf("Parameters (B) = %q, want 7.25 - 7,248,023,552 parameters at two decimal places", got)
	}
	if got := customValue(t, testImportName, "Author"); got != "vendor" {
		t.Errorf("Author = %q, want vendor", got)
	}
	if got := colStr(t, testImportName, "description"); !strings.Contains(got, testImportRepo) {
		t.Errorf("description = %q, want it to name the repository it was read from", got)
	}
	// Nothing was priced, because a HuggingFace repository quotes no price.
	if got := colStr(t, testImportName, "price_prompt"); got != "" {
		t.Errorf("price_prompt = %q, want it empty", got)
	}
	// The answer tells the caller what is still to research, which is the whole
	// point of the handover this route sits in the middle of.
	missing, ok := body["missing"].([]interface{})
	if !ok || len(missing) == 0 {
		t.Fatalf("missing = %#v, want the fields this row still has no answer for", body["missing"])
	}
	named := map[string]map[string]interface{}{}
	for _, m := range missing {
		entry, isObject := m.(map[string]interface{})
		if !isObject {
			t.Fatalf("missing carries %#v, want one object per field", m)
		}
		named[fmt.Sprint(entry["field"])] = entry
	}
	// No repository states either of these, and both are writable on this row, so
	// both are what the handover is for.
	for _, want := range []string{"supports_reasoning", "supports_tools"} {
		if named[want] == nil {
			t.Errorf("%q is not in missing (%v), and no repository states it", want, missing)
		}
	}
	// A fact the repository DID state is not a gap, however it was worded before:
	// the row holds the parameter count, so nothing asks for it again.
	for _, filled := range []string{"parameters", "author", "context_length"} {
		if named[filled] != nil {
			t.Errorf("%q is named as missing, but the import wrote it", filled)
		}
	}
}

// TestImportedSizeComesFromTheShardIndex is the measurement test, and it is
// written against a repository where the two answers DIFFER: the index names two
// shards, and the repository also ships a consolidated copy of the same weights.
// A sum over every weight file is exactly twice the truth here and looks entirely
// plausible in a column this app presents as a measurement.
func TestImportedSizeComesFromTheShardIndex(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	rec := importModel(t, `{"url":"`+testImportRepo+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("import status = %d: %s", rec.Code, rec.Body.String())
	}

	// 14,496,080,928 bytes over the two shards, which is 14.5 GB at the one
	// decimal place the size is stored at. Summing every weight file instead
	// gives 29 GB, so the two answers are still told apart after rounding.
	const wantGB, everyFile = "14.5", "29"
	if got := customValue(t, testImportName, "Weights size (GB)"); got != wantGB {
		t.Errorf("Weights size (GB) = %v, want %v - the shards model.safetensors.index.json names", got, wantGB)
	}
	if got := customValue(t, testImportName, "Weights size (GB)"); got == everyFile {
		t.Errorf("Weights size (GB) = %v, which is every weight file summed; the repository's second copy of its own weights must not be counted", got)
	}
	// The answer says which files that number came from, so nobody has to guess
	// later whether it was measured or estimated.
	if got, _ := importedBody(t, rec)["size_basis"].(string); !strings.Contains(got, "model.safetensors.index.json") {
		t.Errorf("size_basis = %q, want it to name the index the size was read from", got)
	}
}

// TestImportRefusesANameAlreadyHere pins the contract POST /api/models/add
// already keeps and store.ImportFullRecord on its own does not: an existing name
// is a 409, and the row that is already there is left exactly as it was rather
// than being upserted over.
func TestImportRefusesANameAlreadyHere(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	if err := store.AddModelWithCells(testImportName, map[string]string{"description": "written by hand", "notes": "mine"}); err != nil {
		t.Fatal(err)
	}

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("importing onto an existing name answered %d, want 409: %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct{ col, want string }{
		{"source", "manual"},
		{"description", "written by hand"},
		{"notes", "mine"},
		{"hf_slug", ""},
	} {
		if got := colStr(t, testImportName, c.col); got != c.want {
			t.Errorf("%s = %q after a refused import, want the stored %q untouched", c.col, got, c.want)
		}
	}
	// The refusal names the way past it, or the reader has to guess that one
	// exists - which is how the architecture columns became unfillable.
	if said := rec.Body.String(); !strings.Contains(said, "reimport") {
		t.Errorf("the refusal does not name the re-import: %s", said)
	}
}

// TestReimportRereadsTheRepositoryAndKeepsEveryPersonalValue is the fix for a
// model that could be filled in NO way at all: the architecture columns are
// measurements the procedure forbids anybody to type, so they can only come from
// an import, and the import refused the name.
//
// The half that matters more is what a re-import must NOT do. Notes, the
// favourite star, a column the reader made themselves, and a value they typed
// into one of the importer's own columns are all theirs, and a re-read of a
// repository is no reason to touch any of them.
func TestReimportRereadsTheRepositoryAndKeepsEveryPersonalValue(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	// A model added by hand: a row with a note, a star, a column of the
	// reader's own, and a parameter count they typed in themselves, which
	// lands as research because no repository has stated one.
	if rec := addModel(t, `{"name":"`+testImportName+`","cells":{"notes":"my own note","favorite":"yes",`+
		`"parameters":"999"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}
	mine, err := ownColumnKey(t, "My own column", store.ValueTypeText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue(testImportName, mine, "mine alone"); err != nil {
		t.Fatal(err)
	}

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`","reimport":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("a re-import answered %d, want 200: %s", rec.Code, rec.Body.String())
	}
	if body := importedBody(t, rec); body["reimported"] != true {
		t.Errorf(`the answer says reimported = %v, want true`, body["reimported"])
	}

	// The figure the reader typed was research filling a gap; the repository
	// has now stated the fact, so the measurement is what shows and the typed
	// figure sits beneath it, unmoved.
	if got := customValue(t, testImportName, "Parameters (B)"); got != "7.25" {
		t.Errorf("Parameters (B) = %q, want the repository's 7.25 now that it has been read", got)
	}
	if answers, err := store.AnswersFor(testImportName, "parameters"); err != nil {
		t.Fatal(err)
	} else if !answerHolds(answers, store.SourceResearch, "999") {
		t.Errorf("the typed 999 is no longer beneath the measurement: %+v", answers.Answers)
	}
	// What the repository states was read into every cell nobody had filled.
	if got := customValue(t, testImportName, "Weights size (GB)"); got == "" {
		t.Error("the size was not filled by the re-import, which is what it exists for")
	}
	if got := colStr(t, testImportName, "hf_slug"); got != testImportRepo {
		t.Errorf("hf_slug = %q, want %q", got, testImportRepo)
	}

	// And every personal value is exactly as it was.
	for _, c := range []struct{ col, want string }{
		{"notes", "my own note"},
		{"favorite", "1"},
	} {
		if got := colStr(t, testImportName, c.col); got != c.want {
			t.Errorf("%s = %q after a re-import, want the stored %q untouched", c.col, got, c.want)
		}
	}
	if got := customValue(t, testImportName, "My own column"); got != "mine alone" {
		t.Errorf("My own column = %q after a re-import, want it untouched", got)
	}
}

// The catalog's own listing of the repository the import reads: an OpenRouter
// row, under OpenRouter's name for the model rather than the repository's, and
// carrying that repository in hf_slug. It is the shape the duplicate defect was
// found in - a catalog holding "inclusionAI: Ling 3.0 Tiny (free)" with
// hf_slug inclusionAI/Ling-3.0-tiny, imported again as a second row.
const testUpstreamName = "Vendor: Import 7B (free)"

// seedUpstreamListing writes that row, with a note, a star and a figure the
// reader typed into one of the importer's own columns.
func seedUpstreamListing(t *testing.T) {
	t.Helper()
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": testUpstreamName, "id": "vendor/import-7b:free",
		"hf_slug": testImportRepo, "description": "as OpenRouter describes it",
	}, "openrouter", "chat"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCells(testUpstreamName, map[string]string{"notes": "my own note", "favorite": "yes"}); err != nil {
		t.Fatal(err)
	}
	mine, err := ownColumnKey(t, "My own column", store.ValueTypeText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue(testUpstreamName, mine, "mine alone"); err != nil {
		t.Fatal(err)
	}
	// "Parameters (B)" is the shipped column "parameters", seeded by the
	// ImportFullRecord call above - nothing here mints a column of its own.
	if err := setOwnValue(testUpstreamName, "parameters", "999"); err != nil {
		t.Fatal(err)
	}
}

// TestImportRefusesARepositoryHeldUnderAnotherName is the defect this identity
// lookup exists for, in the exact shape it was proven in: the catalog already
// holds the model under OpenRouter's name for it, the same repository is
// imported, and the old check - is this INVENTED name taken? - said no and made
// a second row for one model.
func TestImportRefusesARepositoryHeldUnderAnotherName(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedUpstreamListing(t)

	before, _ := dbcore.CountModels()
	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("importing a repository this catalog already holds answered %d, want 409: %s", rec.Code, rec.Body.String())
	}
	if after, _ := dbcore.CountModels(); after != before {
		t.Errorf("the model count went %d -> %d; a refused import must create no row", before, after)
	}
	if store.ModelExists(testImportName) {
		t.Errorf("a second row %q was created for a model already in the catalog", testImportName)
	}
	// The refusal has to name the row that holds it, or the reader cannot tell
	// which model this is a duplicate of, and the way past it.
	said := rec.Body.String()
	for _, want := range []string{testUpstreamName, testImportRepo, "reimport"} {
		if !strings.Contains(said, want) {
			t.Errorf("the refusal never mentions %q: %s", want, strings.TrimSpace(said))
		}
	}
}

// TestReimportWritesOntoTheRowThatAlreadyHoldsTheRepository is the way past that
// refusal, and it must land on the row that already holds the model rather than
// on a new one named after the repository. Everything personal on that row is
// left exactly as it was.
func TestReimportWritesOntoTheRowThatAlreadyHoldsTheRepository(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedUpstreamListing(t)

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`","reimport":true}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("a re-import answered %d, want 200: %s", rec.Code, rec.Body.String())
	}
	body := importedBody(t, rec)
	if body["name"] != testUpstreamName {
		t.Errorf("the answer names %v as the row written, want %q", body["name"], testUpstreamName)
	}
	if body["reimported"] != true {
		t.Errorf("reimported = %v, want true", body["reimported"])
	}
	if n, _ := dbcore.CountModels(); n != 1 {
		t.Errorf("the catalog holds %d rows, want the one model it started with", n)
	}
	if store.ModelExists(testImportName) {
		t.Fatalf("a second row %q was created by the re-import", testImportName)
	}

	// What the repository MEASURES landed on the row that was already here.
	for _, c := range []struct{ column, want string }{
		{"Weights size (GB)", "14.5"},
	} {
		if got := customValue(t, testUpstreamName, c.column); got != c.want {
			t.Errorf("%s = %q on the row that was re-imported, want %q", c.column, got, c.want)
		}
	}
	// And nothing personal moved: the note, the star, the reader's own column and
	// the figure they typed into one of the importer's columns.
	for _, c := range []struct{ col, want string }{
		{"notes", "my own note"},
		{"favorite", "1"},
	} {
		if got := colStr(t, testUpstreamName, c.col); got != c.want {
			t.Errorf("%s = %q after a re-import, want the stored %q untouched", c.col, got, c.want)
		}
	}
	if got := customValue(t, testUpstreamName, "My own column"); got != "mine alone" {
		t.Errorf("My own column = %q after a re-import, want it untouched", got)
	}
	if got := customValue(t, testUpstreamName, "Parameters (B)"); got != "7.25" {
		t.Errorf("Parameters (B) = %q, want the repository's 7.25 over the researched 999", got)
	}
	if answers, err := store.AnswersFor(testUpstreamName, "parameters"); err != nil {
		t.Fatal(err)
	} else if !answerHolds(answers, store.SourceResearch, "999") {
		t.Errorf("the typed 999 is no longer beneath the measurement: %+v", answers.Answers)
	}
}

// answerHolds reports whether one source's answer in the panel's data is the
// given value.
func answerHolds(answers store.ColumnAnswers, src store.Source, want string) bool {
	for _, a := range answers.Answers {
		if a.Source == string(src) {
			return a.Value != nil && *a.Value == want
		}
	}
	return false
}

// TestImportOfARepositoryNobodyHoldsStillCreatesTheRow is the other side of the
// same check: recognising a repository must not make the route refuse a model
// that genuinely is not here.
func TestImportOfARepositoryNobodyHoldsStillCreatesTheRow(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	// An unrelated row carrying a repository of its own, so the lookup has
	// something to compare against and still has to answer "not this one".
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "Someone Else: Other Model", "id": "someone/other", "hf_slug": "someone/other",
	}, "openrouter", "chat"); err != nil {
		t.Fatal(err)
	}

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("importing a repository nobody holds answered %d, want 201: %s", rec.Code, rec.Body.String())
	}
	if !store.ModelExists(testImportName) {
		t.Errorf("no row called %q exists after a 201", testImportName)
	}
	if n, _ := dbcore.CountModels(); n != 2 {
		t.Errorf("the catalog holds %d rows, want the unrelated one plus the imported one", n)
	}
}

// TestImportRefusesAGatedRepository pins the same decision
// POST /api/models/variations makes: a repository whose owner requires an account
// answers its metadata call in full and refuses its files, so importing it would
// create a row with no size, no context length and no architecture while looking
// like a complete model.
func TestImportRefusesAGatedRepository(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	before, _ := dbcore.CountModels()
	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportGatedRepo+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a gated repository answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "requires an account") {
		t.Errorf("the refusal reads %q, want it to say why", strings.TrimSpace(rec.Body.String()))
	}
	if after, _ := dbcore.CountModels(); after != before {
		t.Errorf("the model count went %d -> %d; a refused import must write nothing", before, after)
	}
	assertOnlyTheDatabaseInDataDir(t, "a refused import")
}

// TestImportWithHuggingFaceUnreachableRecordsNothing pins the other expected
// failure: a machine with no network reads as a message about the network, and
// leaves no half-written row behind.
func TestImportWithHuggingFaceUnreachableRecordsNothing(t *testing.T) {
	migratedDB(t)
	dead := httptest.NewServer(http.NewServeMux())
	origAPI, origResolve := catalog.HFAPIBaseURL, catalog.HFResolveBaseURL
	catalog.HFAPIBaseURL = dead.URL + "/api"
	catalog.HFResolveBaseURL = dead.URL
	dead.Close()
	defer func() { catalog.HFAPIBaseURL, catalog.HFResolveBaseURL = origAPI, origResolve }()

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("an unreachable HuggingFace answered %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if n, _ := dbcore.CountModels(); n != 0 {
		t.Errorf("the database holds %d models after a failed import, want none", n)
	}
}

// TestImportRefusesAnAddressItCannotUse pins the addresses a person actually
// pastes, on both sides: the four forms that name a model, and the four that name
// something else. Nothing is created by any of the refusals.
func TestImportRefusesAnAddressItCannotUse(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	// The first form creates the row; the other three name the SAME repository,
	// so each one lands on the name that already exists. That 409 is the proof
	// they normalized to one repository id rather than to three.
	if rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("the plain URL answered %d, want 201: %s", rec.Code, rec.Body.String())
	}
	for _, url := range []string{
		"https://huggingface.co/" + testImportRepo + "/tree/main",
		"huggingface.co/" + testImportRepo + "?library=transformers",
		testImportRepo,
	} {
		rec := importModel(t, `{"url":`+quote(url)+`}`)
		if rec.Code != http.StatusConflict {
			t.Errorf("importing %q answered %d, want 409 - it names the repository already imported: %s",
				url, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
	if n, _ := dbcore.CountModels(); n != 1 {
		t.Fatalf("the database holds %d models, want the one repository imported once", n)
	}

	for _, url := range []string{
		"https://huggingface.co/datasets/vendor/some-data",
		"https://example.test/vendor/import-7b",
		"vendor",
		"",
	} {
		rec := importModel(t, `{"url":`+quote(url)+`}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("importing %q answered %d, want 400: %s", url, rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
	if n, _ := dbcore.CountModels(); n != 1 {
		t.Errorf("the database holds %d models after refused addresses, want the one already imported", n)
	}
}

// TestImportRejectsWrongMethod pins the method surface, the same check every
// other write endpoint here makes.
func TestImportRejectsWrongMethod(t *testing.T) {
	migratedDB(t)
	rec := httptest.NewRecorder()
	handleImportHF(rec, httptest.NewRequest(http.MethodGet, "/api/models/import", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/models/import answered %d, want 405", rec.Code)
	}
}

// TestAPIGuideDocumentsTheImportRoute covers the half of this that is not
// behaviour: an agent reaching this server learns what to call from the served
// guide alone, so the route and the shape of its body have to be in it.
func TestAPIGuideDocumentsTheImportRoute(t *testing.T) {
	migratedDB(t)
	rec := httptest.NewRecorder()
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	body := rec.Body.String()
	for _, want := range []string{"/api/models/import", `{"url":"https://huggingface.co/`, "model.safetensors.index.json"} {
		if !strings.Contains(body, want) {
			t.Errorf("the served guide never mentions %q", want)
		}
	}
}

// TestImportRecordsNoArchitecture pins that an import leaves the Architecture
// line alone and records none of the figures that line now holds: a person or
// assistant writes it afterwards, from the repository's config.json.
func TestImportRecordsNoArchitecture(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := cellValue(t, testImportName, "architecture_basis"); got != "" {
		t.Errorf("the import wrote the Architecture line %q", got)
	}
	for _, key := range []string{"kv_bytes_per_token", "attention_layers", "weight_dtype", "weight_file_format", "weight_bytes"} {
		if _, err := store.ColumnByKey(key); err == nil {
			t.Errorf("the column %s still exists", key)
		}
	}
	if got := cellValue(t, testImportName, "weights_size_gb"); got == "" {
		t.Error("the size on disk was not recorded")
	}
}

// TestAnImportRecordsEveryFactTheRepositoryStates is what replaced choosing
// which columns an import creates. A source owns its own columns, so everything
// a repository states is recorded into them - the reader never loses a fact
// because they had not ticked it - and what they choose is which columns their
// table draws.
func TestAnImportRecordsEveryFactTheRepositoryStates(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)

	rec := importModel(t, `{"url":"https://huggingface.co/`+testImportRepo+`"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{
		"parameters", "repo_owner", "weights_size_gb",
	} {
		if cellValue(t, testImportName, key) == "" {
			t.Errorf("the repository states %q and the import did not record it", key)
		}
	}
	// It creates none of the reader's own columns: an import fills what the
	// repository owns, and a column of theirs is theirs to make.
	cols, err := ownColumnsList()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cols {
		if c.Label != "Rating" && c.Label != "OCR" {
			t.Errorf("the import created the column %q, which is the reader's to make", c.Label)
		}
	}
}

// TestTheImportListSaysWhichFactsYouHaveAColumnOfYourOwnFor is what is left of
// the list an import used to tick. Every fact a source states is recorded now,
// and under the pivot the reader types into the very column the source states
// it in rather than a column of their own beside it - so a hand answer is
// already possible for a fact the moment its shipped column is seeded, which
// happens on a fresh install with no models and no columns of the reader's own
// at all.
func TestTheImportListSaysWhichFactsYouHaveAColumnOfYourOwnFor(t *testing.T) {
	migratedDB(t)

	choices, err := store.FactChoices()
	if err != nil {
		t.Fatal(err)
	}
	if len(choices) == 0 {
		t.Fatal("FactChoices reports no facts at all")
	}
	for _, c := range choices {
		if !c.Exists {
			t.Errorf("%s reports exists=false, want true: its shipped column already takes a hand answer", c.Column)
		}
	}
}
