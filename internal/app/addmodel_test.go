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

// addModel drives POST /api/models/add with the given body and hands back what
// the real handler answered.
func addModel(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleAddModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/add", strings.NewReader(body)))
	return rec
}

// TestHandleAddModelCreatesTheRow is the endpoint doing its job: a name and some
// curated fields become a row, answered with 201 and the name it created.
func TestHandleAddModelCreatesTheRow(t *testing.T) {
	migratedDB(t)

	rec := addModel(t, `{"name":"My Local Build","cells":{"notes":"runs on my own machine","supports_tools":"yes","moe":"no","price_unit":"per 1M tokens","favorite":"yes"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", rec.Code, rec.Body.String())
	}
	var answer map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v (%s)", err, rec.Body.String())
	}
	if answer["ok"] != true || answer["name"] != "My Local Build" {
		t.Errorf("answer = %v, want ok and the name it created", answer)
	}

	if !store.ModelExists("My Local Build") {
		t.Fatal("the row was not created")
	}
	for _, c := range []struct{ col, want string }{
		{"source", "manual"},
		{"notes", "runs on my own machine"},
		{"tool", "yes"},
		{"moe", "no"},
		{"measurement", "per 1M tokens"},
	} {
		if got := colStr(t, "My Local Build", c.col); got != c.want {
			t.Errorf("%s = %q, want %q", c.col, got, c.want)
		}
	}
	if got := colInt(t, "My Local Build", "favorite"); got != 1 {
		t.Errorf("favorite = %d, want 1", got)
	}

	// It is a row like any other from here on: it is served, and it is not hidden.
	rows := servedModels(t, "?include_hidden=0")
	if len(rows) != 1 || modelNames(rows)[0] != "My Local Build" {
		t.Errorf("GET /api/models?include_hidden=0 served %v, want the added model", modelNames(rows))
	}
}

// TestHandleAddModelRefusesANameThatIsTaken is the guard the endpoint exists to
// carry: adding is never a way to overwrite a model that is already here, so a
// second add answers 409 and leaves every field of the first one alone.
func TestHandleAddModelRefusesANameThatIsTaken(t *testing.T) {
	migratedDB(t)

	if rec := addModel(t, `{"name":"My Local Build","cells":{"notes":"the first note"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("first add status = %d, body %s", rec.Code, rec.Body.String())
	}
	again := addModel(t, `{"name":"My Local Build","cells":{"notes":"a second note"}}`)
	if again.Code != http.StatusConflict {
		t.Errorf("adding the same name twice status = %d, want 409", again.Code)
	}
	if got := colStr(t, "My Local Build", "notes"); got != "the first note" {
		t.Errorf("notes = %q; a refused add must change nothing", got)
	}

	// A body with no name is a 400 rather than a row called "".
	noName := addModel(t, `{"cells":{"notes":"nowhere to put this"}}`)
	if noName.Code != http.StatusBadRequest {
		t.Errorf("adding with no name status = %d, want 400", noName.Code)
	}
	if n, err := dbcore.CountModels(); err != nil || n != 1 {
		t.Errorf("the database holds %d models (err %v), want only the one that was added", n, err)
	}

	// So is a body that is not JSON at all.
	garbage := addModel(t, `{"name":`)
	if garbage.Code != http.StatusBadRequest {
		t.Errorf("adding with a malformed body status = %d, want 400", garbage.Code)
	}
}

// TestHandleAddModelFillsTheWholeRow pins ALL FIELDS from the generous side:
// the row this creates carries no listing and no repository, so nothing
// automatic has stated anything about it, and the request that adds the model
// fills every column in one go - the fetched ones as research, its own as hand.
func TestHandleAddModelFillsTheWholeRow(t *testing.T) {
	migratedDB(t)

	rec := addModel(t, `{"name":"Sneaky","cells":{"notes":"mine","id":"vendor/sneaky","slug":"vendor/sneaky","repository":"vendor/sneaky","description":"typed in","price_prompt":"0.005","price_unit":"per 1M tokens","model_type":"image","context_length":"8192","supports_reasoning":"yes","input_modalities":"[\"text\", \"image\"]"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct{ col, want string }{
		{"notes", "mine"},
		{"id", "vendor/sneaky"},
		{"slug", "vendor/sneaky"},
		{"hf_slug", "vendor/sneaky"},
		{"description", "typed in"},
		{"price_prompt", "0.005"},
		{"measurement", "per 1M tokens"},
		{"model_type", "image"},
		{"input_modalities", `["text","image"]`},
	} {
		if got := colStr(t, "Sneaky", c.col); got != c.want {
			t.Errorf("%s = %q, want %q", c.col, got, c.want)
		}
	}
	answeredBy, _, _, err := store.AllProvenance()
	if err != nil {
		t.Fatal(err)
	}
	if by := answeredBy["Sneaky"]["context_length"]; by != string(store.SourceResearch) {
		t.Errorf("context_length on an added model is answered by %q, want research", by)
	}
	if by := answeredBy["Sneaky"]["notes"]; by != string(store.SourceHand) {
		t.Errorf("notes on an added model is answered by %q, want hand", by)
	}
	if got := colInt(t, "Sneaky", "context_length"); got != 8192 {
		t.Errorf("context_length = %d, want 8192", got)
	}
	if got := colInt(t, "Sneaky", "supports_reasoning"); got != 1 {
		t.Errorf("supports_reasoning = %d, want 1", got)
	}
	if got := colStr(t, "Sneaky", "source"); got != "manual" {
		t.Errorf("source = %q, want manual - the request cannot choose it", got)
	}
}

// TestHandleAddModelRefusesTheColumnsNothingTypedReaches is the same field
// set from the other side. source is the routing key the permission rule
// itself reads, hidden belongs to POST /api/models/hide, and variant_of
// belongs to the two endpoints that group one row under another - so a body
// carrying any of them is REFUSED, rather than adding the model and quietly
// dropping the value. "added" is the one column nothing typed may reach, and
// a key naming no column is refused rather than minted.
func TestHandleAddModelRefusesTheColumnsNothingTypedReaches(t *testing.T) {
	migratedDB(t)

	for _, c := range []struct{ what, body, names string }{
		{"the routing key, which is not part of the request at all",
			`{"name":"Sneaky","source":"openrouter"}`, "source"},
		{"the hidden flag, which has an endpoint of its own",
			`{"name":"Sneaky","hidden":true}`, "hidden"},
		{"a build's parent", `{"name":"Sneaky","variant_of":"Something Else"}`, "variant_of"},
		{"the one column nothing typed may reach",
			`{"name":"Sneaky","cells":{"added":"2020-01-01"}}`, "added"},
		{"a key naming no column",
			`{"name":"Sneaky","cells":{"trained_on":"public code"}}`, "POST /api/columns"},
	} {
		rec := addModel(t, c.body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status = %d, want 400; body %s", c.what, rec.Code, rec.Body.String())
		}
		if !strings.Contains(rec.Body.String(), c.names) {
			t.Errorf("%s: %q does not mention %s", c.what, rec.Body.String(), c.names)
		}
		if store.ModelExists("Sneaky") {
			t.Fatalf("%s: a refused add created the row", c.what)
		}
	}
}

// TestHandleAddModelWritesCustomValues covers a column of the reader's own being
// filled in the same request that adds the model, exactly as a save fills one:
// there is one kind of column now, so it is named by its key like every other.
func TestHandleAddModelWritesCustomValues(t *testing.T) {
	migratedDB(t)
	colID, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}

	rec := addModel(t, `{"name":"My Local Build","cells":{"`+colID+`":"gold"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if v, ok, _ := ownValueOf("My Local Build", colID); !ok || v != "gold" {
		t.Errorf("custom value = %q (present=%v), want \"gold\"", v, ok)
	}

	// A value the column does not allow is refused, and the model it was sent
	// with is NOT added: the cells are checked before the row is inserted, so a
	// refusal leaves nothing behind under that name.
	bad := addModel(t, `{"name":"Second Build","cells":{"`+colID+`":"bronze"}}`)
	if bad.Code != http.StatusBadRequest {
		t.Errorf("a value outside the column's options status = %d, want 400", bad.Code)
	}
	if store.ModelExists("Second Build") {
		t.Error("a refused value left a row behind under that name")
	}
	if _, ok, _ := ownValueOf("Second Build", colID); ok {
		t.Error("a refused value must not be stored")
	}
}

// TestAddedNameIsTrimmedAndCollapsed pins the one tidying a typed name gets, and
// where it stops. A name is trimmed and runs of whitespace inside it become one
// space, on the row being CREATED. It never reaches a row that already exists:
// models.name is the primary key and custom_values references it, so a rename
// returns that model's personal values empty and silently.
func TestAddedNameIsTrimmedAndCollapsed(t *testing.T) {
	migratedDB(t)

	rec := addModel(t, `{"name":"  Vendor:   Spaced   Out \n","cells":{"notes":"mine"}}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	var answer map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("the answer is not JSON: %v (%s)", err, rec.Body.String())
	}
	if answer["name"] != "Vendor: Spaced Out" {
		t.Errorf("the answer names %v, want the normalized \"Vendor: Spaced Out\"", answer["name"])
	}
	if !store.ModelExists("Vendor: Spaced Out") {
		t.Fatalf("no row called \"Vendor: Spaced Out\"; the database holds %v", modelNames(servedModels(t, "?include_hidden=0")))
	}

	// A name that differs only by whitespace is the SAME name, so it is refused
	// rather than becoming a second row for one model.
	again := addModel(t, `{"name":"Vendor:  Spaced Out","cells":{}}`)
	if again.Code != http.StatusConflict {
		t.Errorf("a name differing only by whitespace answered %d, want 409", again.Code)
	}

	// A name that is nothing but whitespace names no model at all.
	blank := addModel(t, `{"name":"   ","cells":{}}`)
	if blank.Code != http.StatusBadRequest {
		t.Errorf("a whitespace-only name answered %d, want 400", blank.Code)
	}
	if n, _ := dbcore.CountModels(); n != 1 {
		t.Errorf("the database holds %d models, want the one that was added", n)
	}
}

// TestSaveRefusesAModelThatIsNotHere is the other half of the split, and the
// reason the add endpoint exists: a save writes to what is already here, so a
// mistyped name fails loudly instead of creating a model nobody meant to add.
func TestSaveRefusesAModelThatIsNotHere(t *testing.T) {
	migratedDB(t)

	rec := httptest.NewRecorder()
	handleSaveModels(rec, httptest.NewRequest(http.MethodPost, "/api/save",
		strings.NewReader(`[{"name":"Vendor: Typo","cells":{"notes":"typed by hand"}}]`)))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("saving to an unknown model status = %d, want 404; body %s", rec.Code, rec.Body.String())
	}
	if store.ModelExists("Vendor: Typo") {
		t.Error("a save created a model")
	}
	if n, err := dbcore.CountModels(); err != nil || n != 0 {
		t.Errorf("the database holds %d models (err %v), want none", n, err)
	}

	// The same body works once the model has been added, so the refusal is about
	// the model being absent and nothing else.
	if add := addModel(t, `{"name":"Vendor: Typo","cells":{}}`); add.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", add.Code, add.Body.String())
	}
	ok := httptest.NewRecorder()
	handleSaveModels(ok, httptest.NewRequest(http.MethodPost, "/api/save",
		strings.NewReader(`[{"name":"Vendor: Typo","cells":{"notes":"typed by hand"}}]`)))
	if ok.Code != http.StatusOK {
		t.Fatalf("saving to the added model status = %d, body %s", ok.Code, ok.Body.String())
	}
	if got := colStr(t, "Vendor: Typo", "notes"); got != "typed by hand" {
		t.Errorf("notes = %q, want the save to have been applied", got)
	}
}
