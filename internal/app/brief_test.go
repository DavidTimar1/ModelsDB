package app

import (
	"encoding/json"
	"fmt"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// briefFor drives GET /api/models/brief and decodes the answer.
func briefFor(t *testing.T, name string) map[string]interface{} {
	t.Helper()
	rec := httptest.NewRecorder()
	handleModelBrief(rec, httptest.NewRequest(http.MethodGet,
		"/api/models/brief?name="+strings.ReplaceAll(name, " ", "%20"), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/models/brief status = %d, want 200: %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v (%s)", err, rec.Body.String())
	}
	return body
}

// gapsOf reads the gap objects out of a brief answer.
func gapsOf(t *testing.T, body map[string]interface{}) []map[string]interface{} {
	t.Helper()
	raw, ok := body["gaps"].([]interface{})
	if !ok {
		t.Fatalf("gaps = %#v, want a list", body["gaps"])
	}
	out := make([]map[string]interface{}, 0, len(raw))
	for _, g := range raw {
		entry, isObject := g.(map[string]interface{})
		if !isObject {
			t.Fatalf("gaps carries %#v, want one object per field", g)
		}
		out = append(out, entry)
	}
	return out
}

// valueInTheShapeDescribed builds a value from the gap's OWN wording, which is
// the wording the reader gets. Following the brief literally is the whole test:
// a sentence nobody can turn into a request is as broken as a wrong field name.
func valueInTheShapeDescribed(accepts string) string {
	switch {
	case strings.Contains(accepts, "plain number"):
		return "12"
	case strings.Contains(accepts, "true or false"):
		return "true"
	case strings.Contains(accepts, "list of quoted words"):
		return `["text"]`
	case strings.Contains(accepts, "must read as a number"):
		return `"12"`
	case strings.HasPrefix(accepts, "one of "):
		// The words a closed set allows are quoted in the sentence itself.
		if first := strings.Index(accepts, `"`); first >= 0 {
			if end := strings.Index(accepts[first+1:], `"`); end >= 0 {
				return accepts[first : first+end+2]
			}
		}
		return `"x"`
	}
	return `"researched"`
}

// TestEveryFieldTheBriefAsksForIsAcceptedBySave is the acceptance test the whole
// brief exists to pass: a real model's brief is followed LITERALLY, one field at
// a time, and POST /api/save takes every one of them.
//
// The defect it closes was the reverse. The instructions were written by hand
// beside the write path rather than from it, and three of the names they handed
// out - active_parameters, parameters, disk_size_gb - are refused with 400 by the
// very endpoint they told the reader to send them to.
func TestEveryFieldTheBriefAsksForIsAcceptedBySave(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	if rec := importModel(t, `{"url":"`+testImportRepo+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("import status = %d: %s", rec.Code, rec.Body.String())
	}

	gaps := gapsOf(t, briefFor(t, testImportName))
	if len(gaps) == 0 {
		t.Fatal("the brief names no gap at all on a freshly imported model")
	}
	for _, g := range gaps {
		field := fmt.Sprint(g["field"])
		value := valueInTheShapeDescribed(fmt.Sprint(g["accepts"]))
		body := fmt.Sprintf(`[{"name":%q,"cells":{%q:%s}}]`, testImportName, field, value)
		rec := saveModels(t, body)
		if rec.Code != http.StatusOK {
			t.Errorf("POST /api/save refused %q as the brief describes it (%s): %d %s",
				field, g["accepts"], rec.Code, strings.TrimSpace(rec.Body.String()))
		}
	}
}

// TestTheBriefsOwnExampleWritesNothing pins the placeholder rule. The example
// used to carry a plausible value for the model in question, so anybody testing
// the brief by running its own command recorded a fact nobody had established.
func TestTheBriefsOwnExampleWritesNothing(t *testing.T) {
	migratedDB(t)
	hfCatalogServer(t)
	if rec := importModel(t, `{"url":"`+testImportRepo+`"}`); rec.Code != http.StatusCreated {
		t.Fatalf("import status = %d: %s", rec.Code, rec.Body.String())
	}
	body := briefFor(t, testImportName)
	example, _ := body["example"].(string)
	if example == "" {
		t.Fatal("the brief carries no worked example")
	}
	if !strings.Contains(fmt.Sprint(body["brief"]), example) {
		t.Error("the example in the text is not the example the answer names")
	}

	before := colStr(t, testImportName, "updated_at")
	rec := saveModels(t, example)
	if rec.Code != http.StatusBadRequest {
		t.Errorf("running the brief's own example answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if after := colStr(t, testImportName, "updated_at"); after != before {
		t.Errorf("running the brief's own example wrote to the row (updated_at %q -> %q)", before, after)
	}
}

// TestTheBriefSaysWhatTheRowHoldsNow covers the claim the old text made and
// could not keep: it announced that everything it named was empty. What a row
// holds is printed beside each gap, and a column that holds something is not a
// gap at all.
func TestTheBriefSaysWhatTheRowHoldsNow(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Manual: Half Filled","cells":{"context_length":"8192","moe":"no"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	for _, g := range gapsOf(t, briefFor(t, "Manual: Half Filled")) {
		switch g["field"] {
		case "supports_reasoning":
			// Nobody has answered it, and an empty cell is exactly that: an
			// unanswered question, printed as one rather than as a false.
			if g["current"] != "not set" {
				t.Errorf("mine.supports_reasoning is reported as %q, want \"not set\"", g["current"])
			}
		case "moe", "context_length":
			t.Errorf("%v is named as unanswered, and the row holds an answer for it", g["field"])
		}
	}
}

// TestTheBriefAsksForEveryEmptyFieldOnAnUpstreamRowToo is the other half of the
// acceptance test, and it is the assertion that CHANGED.
//
// WHAT IT USED TO SAY, AND WHY IT NO LONGER SAYS IT. The brief used to LEAVE OUT
// a column a refresh phase owned on this row, because POST /api/save answered
// 400 for it and asking would have sent the reader to a refusal. Nothing is
// owned now: an empty cell is a gap on every row, whichever route made the
// row, and what research finds goes in beneath whatever a source later states.
// The brief therefore ASKS, and the save ACCEPTS - which is what this now
// checks.
func TestTheBriefAsksForEveryEmptyFieldOnAnUpstreamRowToo(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Cat: Listed","cells":{"id":"cat/listed"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	if _, err := dbcore.DB.Exec(`UPDATE models SET source='openrouter' WHERE name=?`, "Cat: Listed"); err != nil {
		t.Fatalf("make the row an OpenRouter row: %v", err)
	}
	asked := map[string]bool{}
	for _, g := range gapsOf(t, briefFor(t, "Cat: Listed")) {
		if field, isText := g["field"].(string); isText {
			asked[field] = true
		}
	}
	for _, field := range []string{"price_prompt", "context_length", "model_type", "supports_tools"} {
		if !asked[field] {
			t.Errorf("the brief leaves out %q on an OpenRouter row, and nothing owns it there any more", field)
		}
	}
	if rec := saveModels(t, `[{"name":"Cat: Listed","cells":{"description":"researched by hand","price_prompt":"0.000004"}}]`); rec.Code != http.StatusOK {
		t.Errorf("saving what the brief asked for on an OpenRouter row: status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := colStr(t, "Cat: Listed", "description"); got != "researched by hand" {
		t.Errorf("description = %q, want the answer that was written", got)
	}
}

// TestTheBriefOffersOnlyResearchableColumns: the gap list is the set of keys a
// save will accept on this row, and nothing else. A column of the reader's
// own is never asked about, a column an automatic source already states on
// this model is not a gap, and the text tells an assistant to write nothing
// else and to create no column.
func TestTheBriefOffersOnlyResearchableColumns(t *testing.T) {
	migratedDB(t)
	if _, err := store.CreateOwnColumn(store.ColumnSpec{Label: "Rating"}); err != nil {
		t.Fatal(err)
	}
	if rec := addModel(t, `{"name":"Cat: Listed","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	if err := store.WriteCells(store.SourceOpenRouter, "Cat: Listed", map[string]string{"context_length": "8192"}); err != nil {
		t.Fatal(err)
	}
	body := briefFor(t, "Cat: Listed")
	asked := map[string]bool{}
	for _, g := range gapsOf(t, body) {
		if field, isText := g["field"].(string); isText {
			asked[field] = true
		}
		if _, shipped := g["shipped"]; shipped {
			t.Errorf("a gap carries a shipped flag, and no column of the reader's own can be a gap any more")
		}
	}
	for _, field := range []string{"notes", "favorite", "rating", "added", "context_length", "id", "slug"} {
		if asked[field] {
			t.Errorf("the brief asks for %q, which research may not fill on this row", field)
		}
	}
	for _, field := range []string{"price_prompt", "parameters", "model_type"} {
		if !asked[field] {
			t.Errorf("the brief leaves out %q, which nothing has stated and research may fill", field)
		}
	}
	text, _ := body["brief"].(string)
	for _, want := range []string{"kept as research", "do not create columns"} {
		if !strings.Contains(text, want) {
			t.Errorf("the brief never says %q", want)
		}
	}
	for _, gone := range []string{"Columns you made yourself", "creates the column"} {
		if strings.Contains(text, gone) {
			t.Errorf("the brief still says %q", gone)
		}
	}
}

// TestTheBriefNamesThisInstancesAddresses keeps the property the served guide
// gained when the hard-coded port came out of it: an assistant reading a brief
// may be on another machine, so the text names the address the reader arrived on
// and every address this process answers on.
func TestTheBriefNamesThisInstancesAddresses(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Manual: Addressed","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	withServedList(t, []string{"http://10.1.2.3:8199"}, func() {
		text := briefWithOrigin(t, "http://10.1.2.3:8199/")
		if !strings.Contains(text, "http://10.1.2.3:8199/api/save") {
			t.Errorf("the brief does not address the caller's own instance:\n%s", text)
		}
		if strings.Contains(text, "10.1.2.3:8199/") && strings.Contains(text, "//api") {
			t.Error("the brief doubles the slash between the address and the path")
		}
	})
}

// TestTheBriefRefusesAnAddressThisServerNeverBound is the injection this rule
// closes: the brief tells whoever reads it to POST their findings to the address
// in the text, so an address that arrived in a link and was never checked sends
// that data to whoever wrote the link.
func TestTheBriefRefusesAnAddressThisServerNeverBound(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Manual: Addressed","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	withServedList(t, []string{"http://10.1.2.3:8199"}, func() {
		text := briefWithOrigin(t, "http://attacker.example/")
		if strings.Contains(text, "attacker.example") {
			t.Errorf("the brief carries an address this server never bound:\n%s", text)
		}
		if !strings.Contains(text, "http://10.1.2.3:8199/api/save") {
			t.Errorf("the brief dropped this server's own address too:\n%s", text)
		}
	})
}

// TestTheBriefAcceptsAnAddressListedInAllowedOrigins is the reverse-proxy case
// allowed_origins exists for: the reader's browser reached this server at an
// address no listener ever bound (a proxy in front of it), so the brief must
// still address that listed origin rather than refusing it like an unknown one.
func TestTheBriefAcceptsAnAddressListedInAllowedOrigins(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Manual: Addressed","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	withAllowedOrigins(t, []string{"https://models.example.com:443"}, func() {
		withServedList(t, []string{"http://10.1.2.3:8199"}, func() {
			text := briefWithOrigin(t, "https://models.example.com/")
			if !strings.Contains(text, "https://models.example.com/api/save") {
				t.Errorf("the brief does not address a listed allowed_origins entry:\n%s", text)
			}
		})
	})
}

// briefWithOrigin is the brief text for the one model these two tests add, asked
// for as the page asks for it.
func briefWithOrigin(t *testing.T, origin string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handleModelBrief(rec, httptest.NewRequest(http.MethodGet,
		"/api/models/brief?name=Manual:%20Addressed&origin="+url.QueryEscape(origin), nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("the answer is not JSON: %v", err)
	}
	return fmt.Sprint(body["brief"])
}
