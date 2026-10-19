package app

import (
	"encoding/json"
	"modelsdb/internal/catalog"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// The one build every test here imports, and the repository it lives on.
const (
	testParent    = "Vendor: Base 32B"
	testParentHF  = "vendor/base-32b"
	testBuildRepo = "owner/base-32b-GGUF"
	testBuildFile = "base-32b-Q4_K_M.gguf"
	testBuildName = testParent + " (" + testBuildRepo + " Q4_K_M)"
)

// The one repository POST /api/models/import reads, the name that import creates,
// and the two figures that make the size test mean something: what the shard
// index names, against what every weight file in the repository adds up to.
const (
	testImportRepo      = "vendor/import-7b"
	testImportName      = "vendor: import-7b"
	testImportGatedRepo = "closed/import-7b"
	testImportShardSum  = 14_496_080_928 // the two shards the index names
	testImportAllFiles  = 28_992_161_856 // those two plus the repository's second copy of the same weights
)

// hfCatalogServer stands in for huggingface.co for the whole app package. Every
// test in this file is fully offline: catalog's two base URLs are pointed at a
// local server and restored afterwards, so no test ever reaches the real API.
//
// The canned repository is the shape that actually turns up: a GGUF repository
// with no config.json of its own, naming in its tags the model it was built
// from - which is the repository that does have one.
func hfCatalogServer(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/api/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`[{"id":"` + testBuildRepo + `","likes":12,"downloads":900,"gated":false,"tags":["gguf"]}]`))
	})
	mux.HandleFunc("/api/models/"+testBuildRepo, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"` + testBuildRepo + `","author":"owner","sha":"c0ffee","gated":false,
			"tags":["gguf","base_model:` + testParentHF + `","base_model:quantized:` + testParentHF + `"],
			"gguf":{"architecture":"vendorarch","context_length":40960},
			"siblings":[{"rfilename":"` + testBuildFile + `","size":19762138368},
			            {"rfilename":"README.md","size":900}]}`))
	})
	// The gated twin: its metadata reads in full, which is exactly why the refusal
	// has to be a decision rather than a failed request.
	mux.HandleFunc("/api/models/closed/base-32b-GGUF", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"closed/base-32b-GGUF","author":"closed","sha":"d00d","gated":"manual",
			"tags":["gguf"],
			"siblings":[{"rfilename":"base-32b-Q4_K_M.gguf","size":19762138368}]}`))
	})
	mux.HandleFunc("/"+testBuildRepo+"/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Entry not found", http.StatusNotFound)
	})
	mux.HandleFunc("/"+testParentHF+"/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{"num_hidden_layers":64,"hidden_size":5120}`))
	})

	// The repository POST /api/models/import reads. It is the shape that makes the
	// size question real: two shards the index names, plus a SECOND COPY of the
	// same weights in one consolidated file, which a repository really does ship
	// (mistralai/Mistral-7B-Instruct-v0.3 is the live example). Summing every
	// weight file here gives exactly twice the right answer.
	mux.HandleFunc("/api/models/"+testImportRepo, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"` + testImportRepo + `","author":"vendor","sha":"5eaf00d","gated":false,
			"createdAt":"2026-01-04T09:00:00.000Z","pipeline_tag":"text-generation",
			"tags":["safetensors","text-generation"],
			"safetensors":{"parameters":{"BF16":7248023552},"total":7248023552},
			"siblings":[{"rfilename":"model-00001-of-00002.safetensors","size":9000000000},
			            {"rfilename":"model-00002-of-00002.safetensors","size":5496080928},
			            {"rfilename":"consolidated.safetensors","size":14496080928},
			            {"rfilename":"model.safetensors.index.json","size":25000},
			            {"rfilename":"config.json","size":700},
			            {"rfilename":"README.md","size":4917}]}`))
	})
	mux.HandleFunc("/"+testImportRepo+"/resolve/main/model.safetensors.index.json", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"metadata":{"total_size":14496047104},
			"weight_map":{"model.embed_tokens.weight":"model-00001-of-00002.safetensors",
			              "model.layers.0.self_attn.q_proj.weight":"model-00001-of-00002.safetensors",
			              "lm_head.weight":"model-00002-of-00002.safetensors"}}`))
	})
	mux.HandleFunc("/"+testImportRepo+"/resolve/main/config.json", func(w http.ResponseWriter, r *http.Request) {
		// The shape fields are the ones a GPU-sizing tool reads: 32 layers of
		// grouped-query attention, 8 key-value heads, a 128-wide head, in
		// bfloat16 - so one token of context costs 2 x 32 x 8 x 128 x 2 bytes.
		w.Write([]byte(`{"architectures":["VendorForCausalLM"],"model_type":"vendor",
			"max_position_embeddings":32768,"num_local_experts":8,
			"hidden_size":4096,"num_hidden_layers":32,
			"num_attention_heads":32,"num_key_value_heads":8,
			"torch_dtype":"bfloat16"}`))
	})
	// The gated twin of that repository: its metadata answers in full and its
	// files refuse, which is why the refusal is a decision rather than a failure.
	mux.HandleFunc("/api/models/"+testImportGatedRepo, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"` + testImportGatedRepo + `","author":"closed","sha":"d00d","gated":"manual",
			"pipeline_tag":"text-generation","siblings":[{"rfilename":"model.safetensors","size":14000000000}]}`))
	})

	server := httptest.NewServer(mux)
	origAPI, origResolve := catalog.HFAPIBaseURL, catalog.HFResolveBaseURL
	catalog.HFAPIBaseURL = server.URL + "/api"
	catalog.HFResolveBaseURL = server.URL
	t.Cleanup(func() {
		catalog.HFAPIBaseURL = origAPI
		catalog.HFResolveBaseURL = origResolve
		server.Close()
	})
}

// seedParent creates the model a build is recorded under.
func seedParent(t *testing.T) {
	t.Helper()
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": testParent, "source": "huggingface", "hf_slug": testParentHF,
		"context_length":   float64(40960),
		"input_modalities": []interface{}{"text"}, "output_modalities": []interface{}{"text"},
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
}

// importBuild drives the real POST handler and returns the recorder.
func importBuild(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleVariations(rec, httptest.NewRequest(http.MethodPost, "/api/models/variations", strings.NewReader(body)))
	return rec
}

// TestImportedVariationRowStartsHidden is the phase's central regression test,
// and it pins a DEFAULT rather than an invariant: the row the IMPORTER creates is
// hidden, so it is left out of the default row set the moment it exists. It is
// deliberately NOT written as "a row with variant_of set is never served", which
// is a different claim and one this project has decided is false - the owner of
// the database may un-hide any row, a build included.
//
// So the row is built by running the importer, and what is asserted is that
// row's starting state.
func TestImportedVariationRowStartsHidden(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedParent(t)

	rec := importBuild(t, `{"name":"`+testParent+`","repo":"`+testBuildRepo+`","build":"`+testBuildFile+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST /api/models/variations status = %d, body %s", rec.Code, rec.Body.String())
	}

	// The default row set: the model, and not the build.
	shown := servedModels(t, "?include_hidden=0")
	if len(shown) != 1 || shared.GetStr(shown[0], "name") != testParent {
		t.Fatalf("GET /api/models?include_hidden=0 served %v, want only the model itself", modelNames(shown))
	}

	// The opt-in row set: both, with the build carrying every field that makes it
	// one.
	all := servedModels(t, "?include_hidden=1")
	if len(all) != 2 {
		t.Fatalf("GET /api/models?include_hidden=1 served %v, want the model and its build", modelNames(all))
	}
	byName := map[string]map[string]interface{}{}
	for _, r := range all {
		byName[shared.GetStr(r, "name")] = r
	}
	build, ok := byName[testBuildName]
	if !ok {
		t.Fatalf("the build was not recorded under %q; got %v", testBuildName, modelNames(all))
	}
	if got := shared.GetStr(build, "variant_of"); got != testParent {
		t.Errorf("variant_of = %q, want the model it was built from", got)
	}
	if got := build["hidden"]; got != true {
		t.Errorf("an imported build's hidden flag = %v, want true", got)
	}
	// What the importer MEASURED is in the repository's own columns, whether or
	// not the reader keeps a column of their own beside them.
	for _, key := range []string{"build_format", "build_revision",
		"build_size_bytes", "build_bits_per_weight"} {
		if cellValue(t, shared.GetStr(build, "name"), key) == "" {
			t.Errorf("the build carries no %s; what the importer measured must be there", key)
		}
	}
	// A parent row is told apart by variant_of being empty, so that has to hold
	// for the ordinary row too.
	if got := shared.GetStr(byName[testParent], "variant_of"); got != "" {
		t.Errorf("the model's own variant_of = %q, want empty", got)
	}

	// What the importer measured is a column of its own, so a table can sort it,
	// a filter can read it, and a person can see it in a cell.
	for _, want := range []struct{ column, value string }{
		{"Build format", "GGUF Q4_K_M"},
		{"Bits per weight", "4"},
		{"Build revision", "c0ffee"},
	} {
		if got := customValue(t, testBuildName, want.column); got != want.value {
			t.Errorf("%s = %q, want %q", want.column, got, want.value)
		}
	}
	// The architecture is written by hand afterwards, so a fresh build has none.
	if got := customValue(t, testBuildName, "Architecture"); got != "" {
		t.Errorf("the importer wrote an Architecture line: %q", got)
	}
}

// TestUnhidingABuildIsAllowed pins the decision that the regression test above
// deliberately does not pin: hidden is a starting state, and the shipped un-hide
// control is not special-cased for a build. One mechanism, one meaning, and which
// rows are hidden stays the user's choice.
func TestUnhidingABuildIsAllowed(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedParent(t)
	if rec := importBuild(t, `{"name":"`+testParent+`","repo":"`+testBuildRepo+`","build":"`+testBuildFile+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("import: %d %s", rec.Code, rec.Body.String())
	}

	rec := httptest.NewRecorder()
	handleHideModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/hide",
		strings.NewReader(`{"name":`+quote(testBuildName)+`,"hidden":false}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("un-hiding a build answered %d: %s", rec.Code, rec.Body.String())
	}
	names := modelNames(servedModels(t, "?include_hidden=0"))
	if len(names) != 2 {
		t.Fatalf("after un-hiding, the default row set is %v, want the model and the build", names)
	}
}

// TestReimportingABuildKeepsTheUsersChoices pins what a second read of the same
// repository may and may not touch: the columns the importer owns are refreshed,
// and everything the user decided about that row is left alone - including a
// hidden flag they cleared, which a blind re-import would silently set again.
func TestReimportingABuildKeepsTheUsersChoices(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedParent(t)
	body := `{"name":"` + testParent + `","repo":"` + testBuildRepo + `","build":"` + testBuildFile + `"}`
	if rec := importBuild(t, body); rec.Code != http.StatusOK {
		t.Fatalf("first import: %d %s", rec.Code, rec.Body.String())
	}
	if err := store.SetHidden(testBuildName, false); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCells(testBuildName, map[string]string{"notes": "runs at 40 tokens a second here"}); err != nil {
		t.Fatal(err)
	}

	if rec := importBuild(t, body); rec.Code != http.StatusOK {
		t.Fatalf("second import: %d %s", rec.Code, rec.Body.String())
	}
	if got := colInt(t, testBuildName, "hidden"); got != 0 {
		t.Errorf("hidden = %d after a re-import; the user's choice to show it must stand", got)
	}
	if got := colStr(t, testBuildName, "notes"); got != "runs at 40 tokens a second here" {
		t.Errorf("notes = %q after a re-import, want the note kept", got)
	}
	if got := customValue(t, testBuildName, "Build format"); got != "GGUF Q4_K_M" {
		t.Errorf("Build format = %q after a re-import, want it refreshed from the repository", got)
	}
}

// TestGatedRepositoryRecordsNothing pins one of the two expected failures: a
// repository whose owner requires an account answers the metadata call in full
// and refuses the file, so a row for it would look available while being
// impossible to download. The answer is a sentence, and the database is untouched.
func TestGatedRepositoryRecordsNothing(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedParent(t)

	before, _ := dbcore.CountModels()
	rec := importBuild(t, `{"name":"`+testParent+`","repo":"closed/base-32b-GGUF","build":"base-32b-Q4_K_M.gguf"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a gated repository answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "requires an account") {
		t.Errorf("the refusal reads %q, want it to say why", strings.TrimSpace(rec.Body.String()))
	}
	after, _ := dbcore.CountModels()
	if after != before {
		t.Errorf("the model count went %d -> %d; a refused import must write nothing", before, after)
	}
	assertOnlyTheDatabaseInDataDir(t, "a refused import")
}

// TestHuggingFaceUnreachableRecordsNothing pins the other expected failure: an
// offline machine. It must read as a message about the network, not as a broken
// app, and it must leave no half-written row behind.
func TestHuggingFaceUnreachableRecordsNothing(t *testing.T) {
	migratedDB(t)
	seedParent(t)
	// A server that is not there: a closed listener's address refuses at once.
	dead := httptest.NewServer(http.NewServeMux())
	origAPI, origResolve := catalog.HFAPIBaseURL, catalog.HFResolveBaseURL
	catalog.HFAPIBaseURL = dead.URL + "/api"
	catalog.HFResolveBaseURL = dead.URL
	dead.Close()
	defer func() { catalog.HFAPIBaseURL, catalog.HFResolveBaseURL = origAPI, origResolve }()

	before, _ := dbcore.CountModels()
	rec := importBuild(t, `{"name":"`+testParent+`","repo":"`+testBuildRepo+`","build":"`+testBuildFile+`"}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("an unreachable HuggingFace answered %d, want 502: %s", rec.Code, rec.Body.String())
	}
	if after, _ := dbcore.CountModels(); after != before {
		t.Errorf("the model count went %d -> %d; a failed import must write nothing", before, after)
	}

	list := httptest.NewRecorder()
	handleVariations(list, httptest.NewRequest(http.MethodGet, "/api/models/variations?name="+quoteQuery(testParent), nil))
	if list.Code != http.StatusBadGateway {
		t.Errorf("listing against an unreachable HuggingFace answered %d, want 502", list.Code)
	}
}

// TestBuildLookupNeedsAModelWithARepository pins what the first step of the
// picker answers, and it is why the control is ABSENT rather than present and
// failing on a row with no HuggingFace repository: there is nothing to look up.
func TestBuildLookupNeedsAModelWithARepository(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedParent(t)
	if err := store.AddModelWithCells("Hand Added", map[string]string{}); err != nil {
		t.Fatal(err)
	}

	for _, c := range []struct {
		query string
		want  int
	}{
		{"?name=" + quoteQuery(testParent), http.StatusOK},
		{"?name=Hand+Added", http.StatusBadRequest},
		{"?name=Nothing+Like+This", http.StatusNotFound},
		{"", http.StatusBadRequest},
	} {
		rec := httptest.NewRecorder()
		handleVariations(rec, httptest.NewRequest(http.MethodGet, "/api/models/variations"+c.query, nil))
		if rec.Code != c.want {
			t.Errorf("GET /api/models/variations%s answered %d, want %d (%s)", c.query, rec.Code, c.want, strings.TrimSpace(rec.Body.String()))
		}
	}

	// The successful one names the repositories that publish builds, and says
	// whether it read them all.
	rec := httptest.NewRecorder()
	handleVariations(rec, httptest.NewRequest(http.MethodGet, "/api/models/variations?name="+quoteQuery(testParent), nil))
	var body struct {
		Repos     []map[string]interface{} `json:"repos"`
		Truncated bool                     `json:"truncated"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v (%s)", err, rec.Body.String())
	}
	if len(body.Repos) != 1 || body.Repos[0]["repo"] != testBuildRepo {
		t.Errorf("repos = %v, want the one repository publishing builds", body.Repos)
	}
	if body.Truncated {
		t.Error("one page was the whole list, so truncated must be false")
	}
}

// TestBuildImportRefusesAnUnknownBuild pins that the server decides what was
// imported. It re-reads the repository and looks the build up there, so a caller
// cannot name a build the repository does not hold - nor post a size or a format
// of its own.
func TestBuildImportRefusesAnUnknownBuild(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedParent(t)

	rec := importBuild(t, `{"name":"`+testParent+`","repo":"`+testBuildRepo+`","build":"invented-Q9_K_M.gguf"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an invented build answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if n, _ := dbcore.CountModels(); n != 1 {
		t.Errorf("the database holds %d models, want only the model itself", n)
	}
}

// TestVariationEndpointsRejectWrongMethod pins the method surface, the same check
// every other endpoint here makes.
func TestVariationEndpointsRejectWrongMethod(t *testing.T) {
	migratedDB(t)
	rec := httptest.NewRecorder()
	handleVariations(rec, httptest.NewRequest(http.MethodDelete, "/api/models/variations", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("DELETE /api/models/variations answered %d, want 405", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleVariationFiles(rec, httptest.NewRequest(http.MethodPost, "/api/models/variations/files", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /api/models/variations/files answered %d, want 405", rec.Code)
	}
	rec = httptest.NewRecorder()
	handleUnfileVariation(rec, httptest.NewRequest(http.MethodGet, "/api/models/variations/unfile", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET /api/models/variations/unfile answered %d, want 405", rec.Code)
	}
}

// TestAPIGuideNamesTheBuildFields covers the half of the decision that is not
// about behaviour: a field a consumer cannot discover may as well not exist, and
// the served guide is the only place they can discover one.
func TestAPIGuideNamesTheBuildFields(t *testing.T) {
	migratedDB(t)
	rec := httptest.NewRecorder()
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	body := rec.Body.String()
	// What a build is made of is a column per fact now, so the guide names those
	// keys rather than the one blob that used to hold them all.
	for _, want := range []string{"variant_of", "build_format", "/api/models/variations"} {
		if !strings.Contains(body, want) {
			t.Errorf("the served guide never mentions %q", want)
		}
	}
}

// TestBuildFormatDoesNotRepeatTheFormatWord pins the four cases in
// buildFormat: a non-GGUF label already carries the format word (built in
// internal/catalog/huggingface.go as "<format> <bit>-bit"), so prefixing it
// again used to store "FP8 FP8 8-bit" instead of "FP8 8-bit".
func TestBuildFormatDoesNotRepeatTheFormatWord(t *testing.T) {
	cases := []struct {
		name   string
		format string
		label  string
		want   string
	}{
		{"a non-GGUF label already carries the format", "FP8", "FP8 8-bit", "FP8 8-bit"},
		{"a GGUF label is the quantization alone", "GGUF", "Q4_K_M", "GGUF Q4_K_M"},
		{"an empty format stores the label alone, no leading space", "", "FP8 8-bit", "FP8 8-bit"},
		{"an empty label stores the format alone", "FP8", "", "FP8"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := buildFormat(c.format, c.label); got != c.want {
				t.Errorf("buildFormat(%q, %q) = %q, want %q", c.format, c.label, got, c.want)
			}
		})
	}
}

// quote renders a string as a JSON string literal, so a model name carrying a
// bracket or a quote can be put in a test request body safely.
func quote(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// quoteQuery escapes a string for a query parameter.
func quoteQuery(s string) string {
	return strings.ReplaceAll(strings.ReplaceAll(s, " ", "+"), ":", "%3A")
}

// fileAsVariation drives the real POST handler and returns the recorder.
func fileAsVariation(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleFileAsVariation(rec, httptest.NewRequest(http.MethodPost, "/api/models/variations/existing", strings.NewReader(body)))
	return rec
}

// seedPlainModel adds an ordinary row, the shape a model that turns out to be
// a build of another one actually has. It is written the way the refresh
// writes a row - the name stored exactly as given, a trailing space included -
// because the add route would tidy the name and one test here needs it kept.
func seedPlainModel(t *testing.T, name string) {
	t.Helper()
	if err := store.ImportFullRecord(map[string]interface{}{"name": name}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
}

// TestFilingAnExistingModelKeepsEverythingPersonalOnIt is the central promise of
// this route: the row MOVES, it is not re-created, so the note, the favourite
// star and every custom-column value ride along with it. Those exist in no other
// file, so losing them here would be unrecoverable.
func TestFilingAnExistingModelKeepsEverythingPersonalOnIt(t *testing.T) {
	migratedDB(t)
	seedParent(t)
	seedPlainModel(t, "Base 32B Q4 GGUF")

	colID, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCells("Base 32B Q4 GGUF", map[string]string{"notes": "runs on my own machine", "favorite": "yes"}); err != nil {
		t.Fatal(err)
	}
	// "Weights size (GB)" is the shipped column "weights_size_gb", already seeded
	// by seedPlainModel's model creation above - nothing here mints a column of
	// its own.
	sizeID := "weights_size_gb"
	if err := setOwnValue("Base 32B Q4 GGUF", sizeID, "19.7"); err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("Base 32B Q4 GGUF", colID, "gold"); err != nil {
		t.Fatal(err)
	}

	rec := fileAsVariation(t, `{"name":"Base 32B Q4 GGUF","parent":"`+testParent+`"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}

	variantOf := colStr(t, "Base 32B Q4 GGUF", "variant_of")
	notes := colStr(t, "Base 32B Q4 GGUF", "notes")
	favorite := colInt(t, "Base 32B Q4 GGUF", "favorite")
	hidden := colInt(t, "Base 32B Q4 GGUF", "hidden")
	size := customValue(t, "Base 32B Q4 GGUF", "Weights size (GB)")
	if variantOf != testParent {
		t.Errorf("variant_of = %q, want %q", variantOf, testParent)
	}
	if hidden != 1 {
		t.Errorf("hidden = %d, want 1 - a variation starts out of the list", hidden)
	}
	if notes != "runs on my own machine" || favorite != 1 || size != "19.7" {
		t.Errorf("the row came back notes=%q favorite=%d size=%v; filing moves a row, it does not re-create one", notes, favorite, size)
	}
	if v, ok, _ := ownValueOf("Base 32B Q4 GGUF", colID); !ok || v != "gold" {
		t.Errorf("the custom-column value came back %q (present=%v), want \"gold\"", v, ok)
	}
	// It is drawn under its parent, which is what VariationsOf answers.
	own, err := store.VariationsOf(testParent)
	if err != nil || len(own) != 1 || own[0] != "Base 32B Q4 GGUF" {
		t.Errorf("VariationsOf(parent) = %v (err %v), want the row just filed", own, err)
	}
}

// storedCell reads one column's winning value straight off the cells, which is
// the only way to see a column no source but the app states.
func storedCell(t *testing.T, name, key string) string {
	t.Helper()
	cells, err := store.ModelCells(name)
	if err != nil {
		t.Fatalf("read the cells of %q: %v", name, err)
	}
	return cells[key]
}

// unfileVariation drives POST /api/models/variations/unfile.
func unfileVariation(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleUnfileVariation(rec, httptest.NewRequest(http.MethodPost, "/api/models/variations/unfile", strings.NewReader(body)))
	return rec
}

// TestABuildCannotBeRecordedUnderAVariation is the guard on the other route in.
// A variation is drawn as a CHILD ROW of its parent, and a child row has no
// children of its own, so a build recorded under a row that is itself filed
// would sit in the database and be drawn nowhere at all. The state is forced
// through the real routes, and both rows are read back afterwards to prove the
// refusal wrote nothing.
func TestABuildCannotBeRecordedUnderAVariation(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	seedParent(t)
	seedPlainModel(t, "Middle Row")
	if rec := fileAsVariation(t, `{"name":"Middle Row","parent":"`+testParent+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("filing the middle row: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := colStr(t, "Middle Row", "variant_of"); got != testParent {
		t.Fatalf("the middle row is filed under %q; this test proves nothing otherwise", got)
	}
	before, err := dbcore.CountModels()
	if err != nil {
		t.Fatal(err)
	}

	rec := importBuild(t, `{"name":"Middle Row","repo":"`+testBuildRepo+`","build":"`+testBuildFile+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("recording a build under a variation status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "itself a variation") {
		t.Errorf("the refusal must say why, got %q", body)
	}
	if !strings.Contains(body, "Nothing was recorded") {
		t.Errorf("the refusal must say nothing was written, got %q", body)
	}

	// Nothing was written: no row, and neither side moved.
	after, err := dbcore.CountModels()
	if err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("the catalog holds %d rows, want %d - a refused import must create nothing", after, before)
	}
	if got := colStr(t, "Middle Row", "variant_of"); got != testParent {
		t.Errorf("the refused import left the middle row filed under %q, want %q", got, testParent)
	}
	own, err := store.VariationsOf("Middle Row")
	if err != nil || len(own) != 0 {
		t.Errorf("VariationsOf(\"Middle Row\") = %v (err %v), want nothing filed under it", own, err)
	}
	if own, err := store.VariationsOf(testParent); err != nil || len(own) != 1 || own[0] != "Middle Row" {
		t.Errorf("VariationsOf(parent) = %v (err %v), want only the middle row", own, err)
	}
}

// TestAFiledRowCanBeUnfiledAndKeepsEverything is the way back, and the reason it
// has to exist: nothing else in this app clears variant_of, and a row the
// OpenRouter feed lists cannot be deleted, so a misfiled row would otherwise be
// a hidden child of another model for ever. The round trip has to leave the row
// carrying everything it carried before, the date it was added included.
func TestAFiledRowCanBeUnfiledAndKeepsEverything(t *testing.T) {
	migratedDB(t)
	seedParent(t)
	seedPlainModel(t, "Base 32B Q4 GGUF")
	if err := store.SaveCells("Base 32B Q4 GGUF", map[string]string{"notes": "runs on my own machine", "favorite": "yes"}); err != nil {
		t.Fatal(err)
	}
	// Read off the cells rather than through ModelValues: added is stated by the
	// app source, which the row-shaped read does not carry.
	addedBefore := storedCell(t, "Base 32B Q4 GGUF", store.AddedColumnKey)
	if addedBefore == "" {
		t.Fatal("the seeded row carries no added stamp, so this test could not tell whether one survived")
	}

	if rec := fileAsVariation(t, `{"name":"Base 32B Q4 GGUF","parent":"`+testParent+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("filing it: status = %d, body %s", rec.Code, rec.Body.String())
	}
	rec := unfileVariation(t, `{"name":"Base 32B Q4 GGUF"}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("unfiling it: status = %d, body %s", rec.Code, rec.Body.String())
	}

	if got := colStr(t, "Base 32B Q4 GGUF", "variant_of"); got != "" {
		t.Errorf("variant_of = %q, want empty - the row is a model of its own again", got)
	}
	own, err := store.VariationsOf(testParent)
	if err != nil || len(own) != 0 {
		t.Errorf("VariationsOf(parent) = %v (err %v), want nothing left under it", own, err)
	}
	// Hiding is its own decision with its own control, so unfiling must not undo
	// it: a row that comes back out still hidden is shown again by the verb that
	// shows a row.
	if got := colInt(t, "Base 32B Q4 GGUF", "hidden"); got != 1 {
		t.Errorf("hidden = %d, want 1 - unfiling changes what a row is filed under and nothing else", got)
	}
	if got := colStr(t, "Base 32B Q4 GGUF", "notes"); got != "runs on my own machine" {
		t.Errorf("notes = %q, want the note kept", got)
	}
	if got := colInt(t, "Base 32B Q4 GGUF", "favorite"); got != 1 {
		t.Errorf("favorite = %d, want it kept", got)
	}
	if got := storedCell(t, "Base 32B Q4 GGUF", store.AddedColumnKey); got != addedBefore {
		t.Errorf("added = %q, want %q - the date a model entered this catalog cannot be rewritten", got, addedBefore)
	}
}

// TestUnfilingRefusesWhatItCannotUnfile covers the three refusals. Each state is
// forced first and read back, so a case that never arose cannot pass by silence.
func TestUnfilingRefusesWhatItCannotUnfile(t *testing.T) {
	migratedDB(t)
	seedParent(t)
	seedPlainModel(t, "Ordinary Row")

	unknown := unfileVariation(t, `{"name":"Nothing Here"}`)
	if unknown.Code != http.StatusNotFound {
		t.Errorf("unfiling an unknown model status = %d, want 404; body %s", unknown.Code, unknown.Body.String())
	}

	if got := colStr(t, "Ordinary Row", "variant_of"); got != "" {
		t.Fatalf("the seeded row is filed under %q; this case proves nothing otherwise", got)
	}
	notFiled := unfileVariation(t, `{"name":"Ordinary Row"}`)
	if notFiled.Code != http.StatusBadRequest {
		t.Errorf("unfiling a row that is filed under nothing status = %d, want 400; body %s", notFiled.Code, notFiled.Body.String())
	}
	if !strings.Contains(notFiled.Body.String(), "not filed under any model") {
		t.Errorf("the refusal must say the row is filed under nothing, got %q", notFiled.Body.String())
	}

	// A row that is itself filed AND has a row filed under it: unfiling it would
	// leave that child hanging off a row that is no longer anybody's parent, so
	// the child comes out first.
	//
	// THE STATE IS WRITTEN STRAIGHT INTO THE TABLE, because no route produces it
	// any more: both ways in refuse a parent that is itself filed. A database
	// filled before that guard existed can still hold one, and this is the test
	// that such a row is not stranded.
	seedPlainModel(t, "Middle Row")
	seedPlainModel(t, "Grandchild Row")
	if rec := fileAsVariation(t, `{"name":"Middle Row","parent":"`+testParent+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("filing the middle row: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if _, err := dbcore.DB.Exec("UPDATE models SET variant_of='Middle Row', hidden=1 WHERE name='Grandchild Row'"); err != nil {
		t.Fatalf("write the grandchild state this test is about: %v", err)
	}
	if got := colStr(t, "Grandchild Row", "variant_of"); got != "Middle Row" {
		t.Fatalf("the grandchild is filed under %q; this case proves nothing otherwise", got)
	}
	hasOwn := unfileVariation(t, `{"name":"Middle Row"}`)
	if hasOwn.Code != http.StatusBadRequest {
		t.Errorf("unfiling a row with variations of its own status = %d, want 400; body %s", hasOwn.Code, hasOwn.Body.String())
	}
	if got := colStr(t, "Middle Row", "variant_of"); got != testParent {
		t.Errorf("a refused unfile left variant_of = %q, want it unchanged at %q", got, testParent)
	}
	// The way through is to unfile the child first, which is this same route.
	if rec := unfileVariation(t, `{"name":"Grandchild Row"}`); rec.Code != http.StatusOK {
		t.Fatalf("unfiling the grandchild: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := unfileVariation(t, `{"name":"Middle Row"}`); rec.Code != http.StatusOK {
		t.Fatalf("unfiling the middle row once nothing hangs off it: status = %d, body %s", rec.Code, rec.Body.String())
	}
}

// TestAnUnfiledOpenRouterRowIsStillNotDeletable keeps the two rules apart. Being
// filed under another model has nothing to do with whether the feed lists a row,
// so coming back out from under a parent must not turn a model OpenRouter lists
// into one that can be deleted.
func TestAnUnfiledOpenRouterRowIsStillNotDeletable(t *testing.T) {
	migratedDB(t)
	seedParent(t)
	if _, err := store.UpsertAPIModel(map[string]interface{}{"name": "Feed Listed"}); err != nil {
		t.Fatal(err)
	}
	if rec := fileAsVariation(t, `{"name":"Feed Listed","parent":"`+testParent+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("filing it: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if rec := unfileVariation(t, `{"name":"Feed Listed"}`); rec.Code != http.StatusOK {
		t.Fatalf("unfiling it: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := colStr(t, "Feed Listed", "source"); got != string(store.SourceOpenRouter) {
		t.Fatalf("the row is sourced %q, want openrouter - this test proves nothing otherwise", got)
	}

	del := httptest.NewRecorder()
	handleDeleteModel(del, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Feed Listed","confirm_delete":true}`)))
	if del.Code != http.StatusConflict {
		t.Errorf("deleting the unfiled OpenRouter row status = %d, want 409; body %s", del.Code, del.Body.String())
	}
	if !store.ModelExists("Feed Listed") {
		t.Fatal("the row was deleted; unfiling must not make a feed-listed model deletable")
	}
}

// TestFilingRefusesEveryRelationTheTableCannotDraw covers the four guards, each
// with the row it would have broken. A variation is a child row of its parent, so
// the relation has to be one level deep and acyclic.
func TestFilingRefusesEveryRelationTheTableCannotDraw(t *testing.T) {
	migratedDB(t)
	seedParent(t)
	seedPlainModel(t, "Build A")
	seedPlainModel(t, "Build B")
	seedPlainModel(t, "Other Model")

	// Build A becomes a variation of the parent, which is the legitimate case and
	// the setup for two of the four refusals.
	if rec := fileAsVariation(t, `{"name":"Build A","parent":"`+testParent+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("the legitimate filing failed: %d %s", rec.Code, rec.Body.String())
	}

	cases := []struct {
		what   string
		body   string
		status int
	}{
		{"a model under itself", `{"name":"Build B","parent":"Build B"}`, http.StatusBadRequest},
		{"a model under its own variation", `{"name":"` + testParent + `","parent":"Build A"}`, http.StatusBadRequest},
		{"a model under a row that is already a variation", `{"name":"Build B","parent":"Build A"}`, http.StatusBadRequest},
		{"a model that has variations of its own", `{"name":"` + testParent + `","parent":"Other Model"}`, http.StatusBadRequest},
		{"a model that is not here", `{"name":"Nothing At All","parent":"` + testParent + `"}`, http.StatusNotFound},
		{"a parent that is not here", `{"name":"Build B","parent":"Nothing At All"}`, http.StatusNotFound},
		{"a missing parent name", `{"name":"Build B"}`, http.StatusBadRequest},
	}
	for _, c := range cases {
		rec := fileAsVariation(t, c.body)
		if rec.Code != c.status {
			t.Errorf("%s: status = %d, want %d (body %s)", c.what, rec.Code, c.status, rec.Body.String())
		}
	}

	// Nothing was written by any of them: Build B is still an ordinary row and the
	// parent is still nobody's variation.
	var b, p string
	dbcore.DB.QueryRow("SELECT variant_of FROM models WHERE name='Build B'").Scan(&b)
	dbcore.DB.QueryRow("SELECT variant_of FROM models WHERE name=?", testParent).Scan(&p)
	if b != "" || p != "" {
		t.Errorf("a refused request wrote something: Build B variant_of=%q, parent variant_of=%q", b, p)
	}
}

// TestAFiledRowCanBeMovedToAnotherParent pins the one relation that is allowed to
// change: a row filed under the wrong model is corrected by filing it under the
// right one, rather than by having to unpick anything first.
func TestAFiledRowCanBeMovedToAnotherParent(t *testing.T) {
	migratedDB(t)
	seedParent(t)
	seedPlainModel(t, "Second Base")
	seedPlainModel(t, "Build A")

	if rec := fileAsVariation(t, `{"name":"Build A","parent":"`+testParent+`"}`); rec.Code != http.StatusOK {
		t.Fatalf("first filing: %d %s", rec.Code, rec.Body.String())
	}
	if rec := fileAsVariation(t, `{"name":"Build A","parent":"Second Base"}`); rec.Code != http.StatusOK {
		t.Fatalf("moving it: %d %s", rec.Code, rec.Body.String())
	}
	var variantOf string
	dbcore.DB.QueryRow("SELECT variant_of FROM models WHERE name='Build A'").Scan(&variantOf)
	if variantOf != "Second Base" {
		t.Errorf("variant_of = %q, want \"Second Base\"", variantOf)
	}
}

// TestAModelWhoseStoredNameEndsInASpaceCanStillBeFiled covers the whole class of
// row the OpenRouter catalog produces and nothing tidies: upstream sends a name
// with a space on the end, the refresh stores it exactly as sent, and the name is
// then the primary key with that space in it. A verb that trimmed the name it was
// handed would look for a row that does not exist, so such a model could be
// neither filed under another nor made a parent - through the API or from the
// page, which sends the stored name verbatim. Both directions are checked here.
func TestAModelWhoseStoredNameEndsInASpaceCanStillBeFiled(t *testing.T) {
	migratedDB(t)
	const spaced = "Google: Gemma 4 26B A4B "
	seedPlainModel(t, spaced)
	seedPlainModel(t, "Gemma 4 26B Q8 GGUF")
	seedPlainModel(t, "Some Other Model")

	if rec := fileAsVariation(t, `{"name":`+quote("Gemma 4 26B Q8 GGUF")+`,"parent":`+quote(spaced)+`}`); rec.Code != http.StatusOK {
		t.Fatalf("filing under a parent whose name ends in a space: %d %s", rec.Code, rec.Body.String())
	}
	var variantOf string
	dbcore.DB.QueryRow("SELECT variant_of FROM models WHERE name='Gemma 4 26B Q8 GGUF'").Scan(&variantOf)
	if variantOf != spaced {
		t.Errorf("variant_of = %q, want %q", variantOf, spaced)
	}

	const spacedChild = "Gemma 4 26B FP8 "
	seedPlainModel(t, spacedChild)
	if rec := fileAsVariation(t, `{"name":`+quote(spacedChild)+`,"parent":`+quote("Some Other Model")+`}`); rec.Code != http.StatusOK {
		t.Fatalf("filing a model whose own name ends in a space: %d %s", rec.Code, rec.Body.String())
	}
	dbcore.DB.QueryRow("SELECT variant_of FROM models WHERE name=?", spacedChild).Scan(&variantOf)
	if variantOf != "Some Other Model" {
		t.Errorf("variant_of = %q, want \"Some Other Model\"", variantOf)
	}
}

// TestTheThreeModelVerbsAddressANameEndingInASpace is the same defect at the
// routes that remove or hide a row. Each one is given a stored name with a
// space on the end and must find the row rather than answer that no model has
// that name.
func TestTheThreeModelVerbsAddressANameEndingInASpace(t *testing.T) {
	migratedDB(t)
	const spaced = "Google: Gemma 4 31B "

	seedPlainModel(t, spaced)
	rec := httptest.NewRecorder()
	handleHideModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/hide",
		strings.NewReader(`{"name":`+quote(spaced)+`,"hidden":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("hide: %d %s", rec.Code, rec.Body.String())
	}
	var hidden int
	dbcore.DB.QueryRow("SELECT hidden FROM models WHERE name=?", spaced).Scan(&hidden)
	if hidden != 1 {
		t.Errorf("hidden = %d, want 1", hidden)
	}

	rec = httptest.NewRecorder()
	handleDeleteModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":`+quote(spaced)+`,"confirm_delete":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if store.ModelExists(spaced) {
		t.Error("the row is still there after a delete that answered 200")
	}
}
