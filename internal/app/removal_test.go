package app

import (
	"encoding/json"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// servedModels drives GET /api/models with the given query string and decodes
// what it served. It is deliberately the whole handler rather than
// filterAndProject alone: what an outside consumer receives is the thing under
// test, and the parameter that decides it is read from the request.
func servedModels(t *testing.T, query string) []map[string]interface{} {
	t.Helper()
	rec := httptest.NewRecorder()
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/models%s status = %d, body %s", query, rec.Code, rec.Body.String())
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &rows); err != nil {
		t.Fatalf("GET /api/models%s body is not a JSON array: %v (%s)", query, err, rec.Body.String())
	}
	return rows
}

// modelNames lists the served rows by name, so a failure says which models came
// back rather than only how many.
func modelNames(rows []map[string]interface{}) []string {
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, shared.GetStr(r, "name"))
	}
	return out
}

// TestHandleDeleteModelNeedsConfirmationInTheBody is the refusal case. The
// server, not the browser dialog, is what makes a delete deliberate: the write
// endpoints carry no authentication, so an unconfirmed request must be turned
// away, and the flag must count only when it arrives in the BODY.
func TestHandleDeleteModelNeedsConfirmationInTheBody(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("Hand Added", map[string]string{"notes": "mine"}); err != nil {
		t.Fatal(err)
	}

	bare := httptest.NewRecorder()
	handleDeleteModel(bare, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Hand Added"}`)))
	if bare.Code != http.StatusBadRequest {
		t.Errorf("delete with no confirmation status = %d, want 400", bare.Code)
	}
	if !store.ModelExists("Hand Added") {
		t.Fatal("a refused delete must leave the row alone")
	}

	// The flag is read from the body only: a query parameter saying the same
	// thing must not stand in for it.
	viaQuery := httptest.NewRecorder()
	handleDeleteModel(viaQuery, httptest.NewRequest(http.MethodPost, "/api/models/delete?confirm_delete=true",
		strings.NewReader(`{"name":"Hand Added"}`)))
	if viaQuery.Code != http.StatusBadRequest {
		t.Errorf("delete confirmed only by query parameter status = %d, want 400", viaQuery.Code)
	}
	if !store.ModelExists("Hand Added") {
		t.Fatal("a query parameter must not authorize a delete")
	}

	// Explicitly false is still a refusal, not a default-on.
	explicit := httptest.NewRecorder()
	handleDeleteModel(explicit, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Hand Added","confirm_delete":false}`)))
	if explicit.Code != http.StatusBadRequest {
		t.Errorf("delete with confirm_delete:false status = %d, want 400", explicit.Code)
	}
	if !store.ModelExists("Hand Added") {
		t.Fatal("confirm_delete:false must leave the row alone")
	}
}

// TestHandleDeleteModelWithConfirmation covers the accepted case, including the
// cascade that is the whole reason the confirmation exists.
func TestHandleDeleteModelWithConfirmation(t *testing.T) {
	migratedDB(t)
	colID, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddModelWithCells("Hand Added", map[string]string{"notes": "mine"}); err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("Hand Added", colID, "gold"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleDeleteModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Hand Added","confirm_delete":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if store.ModelExists("Hand Added") {
		t.Error("a confirmed delete must remove the row")
	}
	if _, ok, _ := ownValueOf("Hand Added", colID); ok {
		t.Error("the deleted model's custom-column value should have cascaded away")
	}

	// A model nobody has heard of is a 404, even with the confirmation flag.
	missing := httptest.NewRecorder()
	handleDeleteModel(missing, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Nothing Here","confirm_delete":true}`)))
	if missing.Code != http.StatusNotFound {
		t.Errorf("deleting an unknown model status = %d, want 404", missing.Code)
	}
}

// TestAnOpenRouterRowCannotBeDeleted is the rule that makes hiding the only way
// an OpenRouter model leaves the table. A refresh stores every model the feed
// lists, so a delete would destroy the reader's own answers about it and hand
// the row back on the next run. The state is FORCED here through the same
// upsert the refresh uses rather than reasoned about.
func TestAnOpenRouterRowCannotBeDeleted(t *testing.T) {
	migratedDB(t)
	if _, err := store.UpsertAPIModel(map[string]interface{}{"name": "Feed Listed"}); err != nil {
		t.Fatal(err)
	}
	identity, held, err := store.IdentityOf("Feed Listed")
	if err != nil || !held {
		t.Fatalf("read the seeded row: held=%v err=%v", held, err)
	}
	if identity.Source != string(store.SourceOpenRouter) {
		t.Fatalf("the seeded row is sourced %q, want openrouter - this test proves nothing otherwise", identity.Source)
	}

	rec := httptest.NewRecorder()
	handleDeleteModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Feed Listed","confirm_delete":true}`)))
	if rec.Code != http.StatusConflict {
		t.Errorf("deleting an OpenRouter row status = %d, want 409; body %s", rec.Code, rec.Body.String())
	}
	if !store.ModelExists("Feed Listed") {
		t.Fatal("a refused delete must leave the row alone")
	}
	body := rec.Body.String()
	if !strings.Contains(body, "OpenRouter") {
		t.Errorf("the refusal must say who lists the model, got %q", body)
	}
	if !strings.Contains(body, "Hide") {
		t.Errorf("the refusal must name hiding as what removes it from view, got %q", body)
	}

	// Hiding it is not refused: it is the way out the refusal points at.
	hide := httptest.NewRecorder()
	handleHideModel(hide, httptest.NewRequest(http.MethodPost, "/api/models/hide",
		strings.NewReader(`{"name":"Feed Listed","hidden":true}`)))
	if hide.Code != http.StatusOK {
		t.Fatalf("hiding an OpenRouter row status = %d, body %s", hide.Code, hide.Body.String())
	}
	if !store.ModelExists("Feed Listed") {
		t.Error("hiding must keep the row")
	}
}

// TestTheRefusalThatCannotBeLiftedComesFirst pins the ORDER of the two refusals
// in handleDeleteModel, which no other test here can see: every one of them
// sends confirm_delete, so the 409 could slide back behind the missing-field 400
// and they would all still pass. It did exactly that once, and a caller learned
// only on its second request that the model was never deletable.
func TestTheRefusalThatCannotBeLiftedComesFirst(t *testing.T) {
	migratedDB(t)
	if _, err := store.UpsertAPIModel(map[string]interface{}{"name": "Feed Listed"}); err != nil {
		t.Fatal(err)
	}

	// No confirm_delete field at all: the request the old order answered 400.
	rec := httptest.NewRecorder()
	handleDeleteModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Feed Listed"}`)))
	if rec.Code != http.StatusConflict {
		t.Errorf("deleting an OpenRouter row with no confirmation status = %d, want 409 on the FIRST request; body %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "OpenRouter") {
		t.Errorf("the first answer must be the refusal that cannot be lifted, not a request for confirmation, got %q", body)
	}
	if !store.ModelExists("Feed Listed") {
		t.Fatal("a refused delete must leave the row alone")
	}

	// A row that IS deletable still has to ask for the confirmation, so the
	// reordering did not simply drop that guard.
	if err := store.AddModelWithCells("Typed By Hand", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	ask := httptest.NewRecorder()
	handleDeleteModel(ask, httptest.NewRequest(http.MethodPost, "/api/models/delete",
		strings.NewReader(`{"name":"Typed By Hand"}`)))
	if ask.Code != http.StatusBadRequest {
		t.Errorf("deleting a deletable row with no confirmation status = %d, want 400; body %s", ask.Code, ask.Body.String())
	}
	if !store.ModelExists("Typed By Hand") {
		t.Fatal("a delete with no confirmation must leave the row alone")
	}
}

// TestEveryOtherSourceStaysDeletable is the other half of the rule: the test is
// for the one word openrouter and nothing else, and models.source is free text
// with a legal empty state. Each row here is forced to the source under test and
// read back before the delete is attempted.
func TestEveryOtherSourceStaysDeletable(t *testing.T) {
	migratedDB(t)
	for _, source := range []string{"manual", "huggingface", "collection", ""} {
		name := "Row Sourced " + source
		if source == "" {
			name = "Row Sourced Nothing"
		}
		// models.source records which route made the row; it is free text, and
		// a row an old scrape made still reads collection there, so the row is
		// written directly at the source under test.
		if _, err := dbcore.DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES (?,?,?)`, name, source, dbcore.NowStamp()); err != nil {
			t.Fatalf("seed %q: %v", name, err)
		}
		identity, held, err := store.IdentityOf(name)
		if err != nil || !held {
			t.Fatalf("read %q: held=%v err=%v", name, held, err)
		}
		if identity.Source != source {
			t.Fatalf("%q is sourced %q, want %q - this case proves nothing otherwise", name, identity.Source, source)
		}

		rec := httptest.NewRecorder()
		handleDeleteModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/delete",
			strings.NewReader(`{"name":`+strconv.Quote(name)+`,"confirm_delete":true}`)))
		if rec.Code != http.StatusOK {
			t.Errorf("deleting the %q row status = %d, want 200; body %s", source, rec.Code, rec.Body.String())
		}
		if store.ModelExists(name) {
			t.Errorf("the %q row survived a confirmed delete", source)
		}
	}
}

// TestHandleHideModel pins the non-destructive alternative the delete dialog
// offers: hiding a model changes who sees it and destroys nothing. The row, the
// note and every custom-column value are all still there afterwards, and the
// same verb puts the model back in the list.
func TestHandleHideModel(t *testing.T) {
	migratedDB(t)
	colID, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "Private Build", "source": "huggingface", "hf_slug": "me/private",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	if err := store.SaveCells("Private Build", map[string]string{"notes": "mine"}); err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("Private Build", colID, "gold"); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handleHideModel(rec, httptest.NewRequest(http.MethodPost, "/api/models/hide",
		strings.NewReader(`{"name":"Private Build","hidden":true}`)))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	if !store.ModelExists("Private Build") {
		t.Fatal("hiding a model must keep the row")
	}
	if v, ok, _ := ownValueOf("Private Build", colID); !ok || v != "gold" {
		t.Error("hiding a model must keep its custom-column values")
	}
	if got := colStr(t, "Private Build", "notes"); got != "mine" {
		t.Errorf("hiding a model must keep its note, got %q", got)
	}
	if got := colInt(t, "Private Build", "hidden"); got != 1 {
		t.Errorf("hidden flag = %d, want 1", got)
	}

	// It is out of the default response and reachable through the opt-in.
	if got := servedModels(t, "?include_hidden=0"); len(got) != 0 {
		t.Errorf("GET /api/models?include_hidden=0 served %v, want no rows", modelNames(got))
	}
	if got := servedModels(t, "?include_hidden=1"); len(got) != 1 {
		t.Errorf("GET /api/models?include_hidden=1 served %v, want the hidden model", modelNames(got))
	}

	// ...and the same verb puts it back in the list.
	back := httptest.NewRecorder()
	handleHideModel(back, httptest.NewRequest(http.MethodPost, "/api/models/hide",
		strings.NewReader(`{"name":"Private Build","hidden":false}`)))
	if back.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", back.Code, back.Body.String())
	}
	if got := colInt(t, "Private Build", "hidden"); got != 0 {
		t.Errorf("hidden flag after showing it again = %d, want 0", got)
	}
	if got := servedModels(t, "?include_hidden=0"); len(got) != 1 {
		t.Errorf("GET /api/models?include_hidden=0 after showing it again served %v, want the model back", modelNames(got))
	}
}

// TestGetModelsWithholdsHiddenRowsByDefault is the server-side half of the
// hidden flag, and the half that holds for consumers this app does not control -
// the VRAM calculator, a script, an agent reading the guide. The browser hides
// rows itself too, but that is one caller's user interface: delete every line of
// it and this must still pass.
func TestGetModelsWithholdsHiddenRowsByDefault(t *testing.T) {
	migratedDB(t)
	for _, name := range []string{"Shown Model", "Hidden Model"} {
		if err := store.ImportFullRecord(map[string]interface{}{
			"name": name, "source": "huggingface", "hf_slug": "x/" + name,
		}, "huggingface", "chat"); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.SetHidden("Hidden Model", true); err != nil {
		t.Fatal(err)
	}

	// ?include_hidden=0 is first on purpose: it is the spelling the served guide
	// has told every consumer to use for the whole catalog, so it is precisely the
	// one that must NOT double as the escape hatch. Every other shape of query -
	// a name, a column filter, a projection - withholds a hidden row just the
	// same.
	for _, q := range []string{"?include_hidden=0", "?name=model", "?repository=contains:x/", "?fields=name"} {
		rows := servedModels(t, q)
		if len(rows) != 1 {
			t.Errorf("GET /api/models%s served %v, want only \"Shown Model\"", q, modelNames(rows))
			continue
		}
		if got := shared.GetStr(rows[0], "name"); got != "Shown Model" {
			t.Errorf("GET /api/models%s served %q, want \"Shown Model\"", q, got)
		}
	}

	// The opt-in brings them back, and each row still carries the flag - hiding a
	// row from a caller that did not ask does not mean hiding the flag from one
	// that did.
	rows := servedModels(t, "?include_hidden=1")
	if len(rows) != 2 {
		t.Fatalf("GET /api/models?include_hidden=1 served %v, want both models", modelNames(rows))
	}
	byName := map[string]map[string]interface{}{}
	for _, r := range rows {
		byName[shared.GetStr(r, "name")] = r
	}
	if got := byName["Hidden Model"]["hidden"]; got != true {
		t.Errorf("the hidden model's hidden field = %v, want true", got)
	}
	if got := byName["Shown Model"]["hidden"]; got != false {
		t.Errorf("the shown model's hidden field = %v, want false", got)
	}
}

// TestAPIGuideNamesBothParameters covers the two things a caller cannot work out
// for itself. Passing any parameter is what gets JSON instead of this guide, and
// include_hidden=1 is what gets the models left out of the default list. Half of
// that sentence used to be wrong; both halves have to be there.
func TestAPIGuideNamesBothParameters(t *testing.T) {
	migratedDB(t)
	rec := httptest.NewRecorder()
	handleGetModels(rec, httptest.NewRequest(http.MethodGet, "/api/models", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/models with no query status = %d", rec.Code)
	}
	body := rec.Body.String()
	for _, want := range []string{"?include_hidden=0", "include_hidden=1"} {
		if !strings.Contains(body, want) {
			t.Errorf("the served guide never mentions %q", want)
		}
	}
	if !strings.Contains(body, "/api/models/hide") {
		t.Error("the served guide should name the hide endpoint")
	}
}

// TestCurationEndpointsRejectWrongMethod pins that every write verb here is
// POST-only, the same check every other write endpoint makes.
func TestCurationEndpointsRejectWrongMethod(t *testing.T) {
	migratedDB(t)
	for path, h := range map[string]http.HandlerFunc{
		"/api/models/add":    handleAddModel,
		"/api/models/delete": handleDeleteModel,
		"/api/models/hide":   handleHideModel,
	} {
		rec := httptest.NewRecorder()
		h(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusMethodNotAllowed {
			t.Errorf("GET %s status = %d, want 405", path, rec.Code)
		}
	}
}

// TestHiddenFlagDefaultsOffAndRidesEveryRow pins what models.hidden holds. The
// question "does that column exist" answers nothing on its own: a column of this
// name was dropped as dead weight by migration v7 and given back a real meaning
// by v9, so what it CARRIES is the test.
//
// It also pins where the exclusion lives. processModels emits every row,
// flagged; withholding is filterAndProject's job, because that is the only
// function that reads the request's include_hidden parameter.
func TestHiddenFlagDefaultsOffAndRidesEveryRow(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("M", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	if got := colInt(t, "M", "hidden"); got != 0 {
		t.Errorf("a newly imported model's hidden flag = %d, want 0", got)
	}
	rows, err := processModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("processModels returned %d rows, want 1", len(rows))
	}
	if got := rows[0]["hidden"]; got != false {
		t.Errorf("row hidden = %v, want false", got)
	}

	if err := store.SetHidden("M", true); err != nil {
		t.Fatal(err)
	}
	if got := colInt(t, "M", "hidden"); got != 1 {
		t.Errorf("hidden flag after hiding = %d, want 1", got)
	}
	rows, err = processModels()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("processModels dropped a hidden row; it returned %d rows, want 1", len(rows))
	}
	if got := rows[0]["hidden"]; got != true {
		t.Errorf("row hidden after hiding = %v, want true", got)
	}
}
