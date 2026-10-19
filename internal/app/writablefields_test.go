package app

import (
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// colAny reads one field of one model as text, whatever kind of value it holds.
//
// A write under a bare cell key is read back under that key.
func colAny(t *testing.T, name, col string) string {
	t.Helper()
	return colStr(t, name, col)
}

// sourceStates reads what one named source currently holds for one column on
// one model, whether or not that source currently wins the cell - the same
// figure GET /api/models/{name}/answers/{column} shows beside the winner.
func sourceStates(t *testing.T, model, key, source string) string {
	t.Helper()
	answers, err := store.AnswersFor(model, key)
	if err != nil {
		t.Fatalf("read who answers %s on %q: %v", key, model, err)
	}
	for _, a := range answers.Answers {
		if a.Source == source {
			if a.Value == nil {
				return ""
			}
			return *a.Value
		}
	}
	t.Fatalf("%s does not declare source %s", key, source)
	return ""
}

// openedColumns is a fetched column of each kind POST /api/save accepts on an
// empty cell: what to send for it, and what the column holds afterwards. The
// reader's own columns (notes, favorite) have their own tests in
// fieldtypes_test.go, so this list is the fetched side alone.
// EVERY VALUE IS SENT AS TEXT, because a cell holds text: the column's own kind
// is what a figure, a flag or a list is checked against on the way in.
var openedColumns = []struct{ key, send, stored string }{
	{"id", `"vendor/one"`, "vendor/one"},
	{"slug", `"vendor/one-v1"`, "vendor/one-v1"},
	{"repository", `"Vendor/One"`, "Vendor/One"},
	{"description", `"a model somebody researched"`, "a model somebody researched"},
	{"created_at", `"2026-01-01T00:00:00Z"`, "2026-01-01T00:00:00Z"},
	{"model_type", `"image"`, "image"},
	{"context_length", `"262144"`, "262144"},
	{"input_modalities", `"[\"text\",\"image\"]"`, `["text","image"]`},
	{"output_modalities", `"[\"text\"]"`, `["text"]`},
	{"supports_reasoning", `"yes"`, "1"},
	{"price_prompt", `"0.000001"`, "0.000001"},
	{"price_completion", `"0.000002"`, "0.000002"},
	{"price_image", `"0.01"`, "0.01"},
	{"price_audio", `"0.02"`, "0.02"},
	{"price_internal_reasoning", `"0.03"`, "0.03"},
}

// TestSaveWritesEveryOpenedColumn drives each fetched column through the real
// handler, one request each, on a model added by hand - the row nothing
// automatic describes, so every cell is empty and every column takes research
// there. One request per column is what makes a failure say WHICH column: a
// single request carrying all of them would fail as one.
func TestSaveWritesEveryOpenedColumn(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Manual: Only Here","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	for _, c := range openedColumns {
		body := `[{"name":"Manual: Only Here","cells":{"` + c.key + `":` + c.send + `}}]`
		if rec := saveModels(t, body); rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200; body %s", c.key, rec.Code, rec.Body.String())
			continue
		}
		if got := colAny(t, "Manual: Only Here", c.key); got != c.stored {
			t.Errorf("%s = %q, want %q", c.key, got, c.stored)
		}
	}
}

// TestSaveTakesAListEitherWay pins the two shapes a modality or collection list
// accepts. The array is the shape to write; one comma-separated string is
// accepted too, because that is the form GET /api/models returns the two
// modality lists in, so a model can be read, changed and sent back.
func TestSaveTakesAListEitherWay(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Listy","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	for _, c := range []struct{ what, send, stored string }{
		{"a JSON list, as text", `"[\"text\",\"image\"]"`, `["text","image"]`},
		{"an empty value, which empties it", `""`, ``},
	} {
		if rec := saveModels(t, `[{"name":"Listy","cells":{"input_modalities":`+c.send+`}}]`); rec.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, body %s", c.what, rec.Code, rec.Body.String())
		}
		if got := colAny(t, "Listy", "input_modalities"); got != c.stored {
			t.Errorf("%s left input_modalities = %q, want %q", c.what, got, c.stored)
		}
	}
}

// TestASaveIsRefusedWhereAnAutomaticSourceHoldsTheCell is the rule the two
// kinds of column rest on, asked through the real handler on a row the feed
// describes: every column OpenRouter stated is refused with 400 naming
// OpenRouter and its value, and its cell is untouched; every column OpenRouter
// did not state takes research on that same row; and on a row nothing
// describes, every one of them is accepted.
func TestASaveIsRefusedWhereAnAutomaticSourceHoldsTheCell(t *testing.T) {
	migratedDB(t)

	// A model the OpenRouter catalog fetch writes by name on every startup, with
	// what OpenRouter itself states about it.
	if _, err := store.UpsertAPIModel(catalog.NormalizeModel(map[string]interface{}{
		"name": "Vendor: Upstream", "id": "vendor/upstream", "canonical_slug": "vendor/upstream",
		"hugging_face_id": "Vendor/Upstream", "description": "as upstream states it",
		"created": float64(1767225600),
		"pricing": map[string]interface{}{"prompt": "0.000001", "completion": "0.000002"},
	})); err != nil {
		t.Fatal(err)
	}
	// A model added by hand, which no source describes at all.
	if rec := addModel(t, `{"name":"Manual: Only Here","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	// key is what POST /api/save takes; stated is what OpenRouter said, which
	// has to be exactly where it was afterwards, and named in the refusal.
	refused := []struct{ what, key, send, label, stated string }{
		{"a price", "price_prompt", `"0.005"`, "In $", "0.000001"},
		{"the identifier", "id", `"vendor/other"`, "OpenRouter id", "vendor/upstream"},
		{"the slug", "slug", `"vendor/other"`, "OpenRouter slug", "vendor/upstream"},
		{"the repository", "repository", `"Vendor/Other"`, "Repository", "Vendor/Upstream"},
		{"the description", "description", `"typed in"`, "Description", "as upstream states it"},
		{"the type, which the app worked out", "model_type", `"image"`, "Type", "chat"},
		{"the tool answer, which the app worked out", "supports_tools", `"yes"`, "Tool", "no"},
	}
	for _, c := range refused {
		rec := saveModels(t, `[{"name":"Vendor: Upstream","cells":{"`+c.key+`":`+c.send+`}}]`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s of an OpenRouter model: status = %d, want 400; body %s", c.what, rec.Code, rec.Body.String())
			continue
		}
		want := c.label + " holds OpenRouter's answer, " + c.stated + ", which an update maintains; nothing typed replaces it"
		if !strings.Contains(rec.Body.String(), want) {
			t.Errorf("%s: the refusal %q does not carry %q", c.what, rec.Body.String(), want)
		}
		if got := sourceStates(t, "Vendor: Upstream", c.key, "openrouter"); got != c.stated {
			t.Errorf("%s: OpenRouter's own %s reads %q after the refusal, want %q untouched", c.what, c.key, got, c.stated)
		}
	}

	// The columns OpenRouter did not state on this listing take research on
	// the very same row.
	for _, c := range []struct{ what, key, send, want string }{
		{"the context length, which this listing left out", "context_length", `"8192"`, "8192"},
		{"an audio price the listing left out", "price_audio", `"0.02"`, "0.02"},
		{"the expiration date", "expiration_date", `"2027-01-01"`, "2027-01-01"},
		{"the MoE answer, which the description did not give", "moe", `"no"`, "no"},
	} {
		rec := saveModels(t, `[{"name":"Vendor: Upstream","cells":{"`+c.key+`":`+c.send+`}}]`)
		if rec.Code != http.StatusOK {
			t.Errorf("%s: status = %d, want 200; body %s", c.what, rec.Code, rec.Body.String())
			continue
		}
		if got := cellValue(t, "Vendor: Upstream", c.key); got != c.want {
			t.Errorf("%s: %s reads %q, want the researched %q", c.what, c.key, got, c.want)
		}
		if got := sourceStates(t, "Vendor: Upstream", c.key, "research"); got != c.want {
			t.Errorf("%s: %s was not written as research: %q", c.what, c.key, got)
		}
	}

	// Every refused key is accepted on a model no source describes, which is
	// what says the refusal is about what is on file, not about the column.
	for _, c := range refused {
		if rec := saveModels(t, `[{"name":"Manual: Only Here","cells":{"`+c.key+`":`+c.send+`}}]`); rec.Code != http.StatusOK {
			t.Errorf("%s on a model added by hand: status = %d, want 200; body %s", c.what, rec.Code, rec.Body.String())
		}
	}
}

// TestSaveRefusesTheColumnsNothingTypedReaches covers what a save is never
// accepted for: the one column nothing typed may reach, a key that still
// carries the old dotted shape, and a key naming no column at all.
func TestSaveRefusesTheColumnsNothingTypedReaches(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Manual: Only Here","cells":{"notes":"kept"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	for _, c := range []struct{ what, send, names string }{
		{"the one column nothing typed reaches", `"added":"2020-01-01"`, "added is stated only by \"app\""},
		{"a dotted key, which names no column", `"openrouter.id":"vendor/one"`, "no column with that key"},
		{"a key nothing holds a column for", `"trained_on":"public code"`, "POST /api/columns"},
	} {
		rec := saveModels(t, `[{"name":"Manual: Only Here","cells":{`+c.send+`}}]`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; body %s", c.what, rec.Code, rec.Body.String())
			continue
		}
		if !strings.Contains(rec.Body.String(), c.names) {
			t.Errorf("%s: %q does not mention %s", c.what, rec.Body.String(), c.names)
		}
	}
	if got := colStr(t, "Manual: Only Here", "notes"); got != "kept" {
		t.Errorf("notes = %q after the refusals, want it untouched", got)
	}
}

// refreshStandIn points the feed fetch at one local server serving a canned
// OpenRouter answer, so a test can run a refresh with no request leaving this
// machine. It returns with catalog.ApiUrl restored.
func refreshStandIn(t *testing.T) {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("/models", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"data":[{"name":"Test: Refresh Model","canonical_slug":"test-vendor/refresh-model",` +
			`"context_length":8192,"created":1700000000,` +
			`"architecture":{"input_modalities":["text"],"output_modalities":["text"]},` +
			`"supported_parameters":["tools"],"pricing":{"prompt":"0.000001","completion":"0.000002"}}]}`))
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

	api := catalog.ApiUrl
	video, image := catalog.VideoModelsURL, catalog.ImageModelsURL
	catalog.ApiUrl = server.URL + "/models"
	origEU := catalog.EURoutingURL
	t.Cleanup(func() { catalog.EURoutingURL = origEU })
	catalog.EURoutingURL = server.URL + "/eu"
	catalog.VideoModelsURL = server.URL + "/videos"
	catalog.ImageModelsURL = server.URL + "/images"
	t.Cleanup(func() {
		server.Close()
		catalog.ApiUrl = api
		catalog.VideoModelsURL = video
		catalog.ImageModelsURL = image
	})
}

// runEveryRefreshPhase runs the one phase a POST /api/update runs: the feed
// fetch.
func runEveryRefreshPhase(t *testing.T) {
	t.Helper()
	if _, _, _, err := catalog.FetchModels(); err != nil {
		t.Fatalf("the catalog fetch: %v", err)
	}
}

// TestAnAcceptedValueSurvivesARefresh is the test that proves the write rule
// is right rather than merely plausible. Everything POST /api/save accepts on
// a row is written first and a refresh is run over the top of it; anything
// that comes back changed is a cell the refresh should not have reached.
//
// It runs the real feed fetch against a local stand-in server, so the whole
// thing is offline and repeatable.
func TestAnAcceptedValueSurvivesARefresh(t *testing.T) {
	migratedDB(t)
	refreshStandIn(t)

	// Two models: one the catalog fetch describes, seeded first so the refresh
	// meets a row that already carries accepted values, and one nothing
	// automatic reaches at all.
	if _, err := store.UpsertAPIModel(map[string]interface{}{"name": "Test: Refresh Model"}); err != nil {
		t.Fatalf("seed %q: %v", "Test: Refresh Model", err)
	}
	runEveryRefreshPhase(t)
	if rec := addModel(t, `{"name":"Manual: Only Here","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	// Everything accepted on the model nothing reaches: every fetched column
	// at once, as research, plus a note.
	fields := make([]string, 0, len(openedColumns))
	for _, c := range openedColumns {
		fields = append(fields, `"`+c.key+`":`+c.send)
	}
	fields = append(fields, `"notes":"researched by hand"`)
	if rec := saveModels(t, `[{"name":"Manual: Only Here","cells":{`+strings.Join(fields, ",")+`}}]`); rec.Code != http.StatusOK {
		t.Fatalf("writing the hand-added model: status = %d, body %s", rec.Code, rec.Body.String())
	}

	// Everything accepted on the model the feed describes: its own columns,
	// and research into the cells the feed left empty.
	if rec := saveModels(t, `[{"name":"Test: Refresh Model","cells":{"price_audio":"0.02","expiration_date":"2027-01-01","notes":"my note"}}]`); rec.Code != http.StatusOK {
		t.Fatalf("writing the OpenRouter model: status = %d, body %s", rec.Code, rec.Body.String())
	}

	// The feed fetch again, over the top of every one of those values.
	runEveryRefreshPhase(t)

	for _, c := range openedColumns {
		if got := colAny(t, "Manual: Only Here", c.key); got != c.stored {
			t.Errorf("a refresh changed %s on the hand-added model: %q, want %q", c.key, got, c.stored)
		}
	}
	for _, c := range []struct{ model, column, want string }{
		{"Manual: Only Here", "notes", "researched by hand"},

		{"Test: Refresh Model", "price_audio", "0.02"},
		{"Test: Refresh Model", "expiration_date", "2027-01-01"},
		{"Test: Refresh Model", "notes", "my note"},
	} {
		if got := cellValue(t, c.model, c.column); got != c.want {
			t.Errorf("a refresh changed %s on %q: %q, want %q", c.column, c.model, got, c.want)
		}
	}
}
