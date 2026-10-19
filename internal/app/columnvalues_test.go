package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"modelsdb/internal/store"
)

// servedValues drives GET /api/columns/values and returns the decoded answer,
// failing the test when the request was refused.
func servedValues(t *testing.T, query string) columnValuesAnswer {
	t.Helper()
	rec := httptest.NewRecorder()
	handleColumnValues(rec, httptest.NewRequest(http.MethodGet, "/api/columns/values"+query, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/columns/values%s status = %d, body %s", query, rec.Code, rec.Body.String())
	}
	var answer columnValuesAnswer
	if err := json.Unmarshal(rec.Body.Bytes(), &answer); err != nil {
		t.Fatalf("GET /api/columns/values%s body is not the answer shape: %v (%s)", query, err, rec.Body.String())
	}
	return answer
}

// valuesRefused drives the same route and returns the refusal, failing the test
// when the request was answered instead.
func valuesRefused(t *testing.T, query string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handleColumnValues(rec, httptest.NewRequest(http.MethodGet, "/api/columns/values"+query, nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("GET /api/columns/values%s answered %d, want 400: %s", query, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// TestValuesOfAListColumnAreItsMembers pins the split: a column holding
// ["text","image"] offers text and image rather than the JSON of the pair, each
// with how many models hold it, most-held first.
func TestValuesOfAListColumnAreItsMembers(t *testing.T) {
	aCatalogToQuery(t)

	answer := servedValues(t, "?key=input_modalities")
	if answer.Key != "input_modalities" {
		t.Errorf("key = %q", answer.Key)
	}
	if answer.DistinctTotal != 2 || answer.Truncated {
		t.Errorf("distinct_total = %d truncated = %v, want 2 and false", answer.DistinctTotal, answer.Truncated)
	}
	want := []columnValue{{Value: "text", Models: 2}, {Value: "image", Models: 1}}
	if len(answer.Values) != len(want) || answer.Values[0] != want[0] || answer.Values[1] != want[1] {
		t.Errorf("values = %v, want %v (members split, most-held first)", answer.Values, want)
	}
}

// TestValuesOfAChoiceColumnKeepTheDeclaredOrder pins the one ordering that is a
// scale: a choice column answers its own options, in their declared order, even
// where no model holds one.
func TestValuesOfAChoiceColumnKeepTheDeclaredOrder(t *testing.T) {
	aCatalogToQuery(t)
	if _, err := store.CreateColumn(store.ColumnSpec{
		Key: "verdict", Label: "Verdict", ValueType: store.ValueTypeChoice,
		Options: []string{"great", "fine", "poor"},
	}, store.SourceHand); err != nil {
		t.Fatalf("create the choice column: %v", err)
	}
	if err := store.WriteCells(store.SourceHand, "Gemini Flash",
		map[string]string{"verdict": "fine"}); err != nil {
		t.Fatalf("write a verdict: %v", err)
	}

	answer := servedValues(t, "?key=verdict")
	if answer.DistinctTotal != 3 {
		t.Fatalf("distinct_total = %d, want all 3 declared options", answer.DistinctTotal)
	}
	want := []columnValue{{Value: "great", Models: 0}, {Value: "fine", Models: 1}, {Value: "poor", Models: 0}}
	for i := range want {
		if answer.Values[i] != want[i] {
			t.Errorf("values[%d] = %v, want %v (declared order, zero counts kept)", i, answer.Values[i], want[i])
		}
	}
}

// TestValuesAreCountedOverTheWholeCatalog pins that hiding a model changes
// nothing here: a list that shrank as you filtered would make the control
// unusable, so the counts always cover every stored row.
func TestValuesAreCountedOverTheWholeCatalog(t *testing.T) {
	aCatalogToQuery(t)
	before := servedValues(t, "?key=input_modalities")
	if err := store.SetHidden("Flux Image", true); err != nil {
		t.Fatalf("hide a model: %v", err)
	}
	after := servedValues(t, "?key=input_modalities")
	if after.DistinctTotal != before.DistinctTotal || len(after.Values) != len(before.Values) {
		t.Errorf("hiding a model changed the answer: before %v, after %v", before.Values, after.Values)
	}
}

// TestValuesContainsAndLimitNarrowHonestly pins the two narrowing controls:
// contains= filters the values, and limit= cuts the list while distinct_total
// and truncated still tell the whole story.
func TestValuesContainsAndLimitNarrowHonestly(t *testing.T) {
	aCatalogToQuery(t)

	narrowed := servedValues(t, "?key=input_modalities&contains=IMA")
	if narrowed.DistinctTotal != 1 || len(narrowed.Values) != 1 || narrowed.Values[0].Value != "image" {
		t.Errorf("contains=IMA answered %v (total %d), want image alone, capitals ignored",
			narrowed.Values, narrowed.DistinctTotal)
	}

	cut := servedValues(t, "?key=input_modalities&limit=1")
	if len(cut.Values) != 1 || !cut.Truncated || cut.DistinctTotal != 2 {
		t.Errorf("limit=1 answered %d values, truncated=%v, total=%d; want 1, true, 2",
			len(cut.Values), cut.Truncated, cut.DistinctTotal)
	}
}

// TestValuesRouteRefusalsNameWhatToDo pins the refusals: an unknown key offers
// the nearest columns, a missing key says what to pass, a bad limit names the
// default, and any verb but GET is not allowed.
func TestValuesRouteRefusalsNameWhatToDo(t *testing.T) {
	aCatalogToQuery(t)

	if body := valuesRefused(t, "?key=input_modality"); !strings.Contains(body, "input_modalities") {
		t.Errorf("an unknown key was not offered the real one: %s", body)
	}
	if body := valuesRefused(t, ""); !strings.Contains(body, "?key=") {
		t.Errorf("a missing key does not say what to pass: %s", body)
	}
	if body := valuesRefused(t, "?key=notes&limit=zero"); !strings.Contains(body, "200") {
		t.Errorf("a bad limit does not name the default: %s", body)
	}
	rec := httptest.NewRecorder()
	handleColumnValues(rec, httptest.NewRequest(http.MethodPost, "/api/columns/values?key=notes", nil))
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST answered %d, want 405", rec.Code)
	}
}

// TestAnyMatchesAnyListedValue is the include half of the value-list grammar:
// the union over one column's listed values, on a list column and on a flag.
func TestAnyMatchesAnyListedValue(t *testing.T) {
	aCatalogToQuery(t)

	if got := queried(t, "?input_modalities=any:image,audio"); !eqStr(got, []string{"Flux Image"}) {
		t.Errorf("any:image,audio served %v, want Flux Image", got)
	}
	if got := queried(t, "?input_modalities=any:text,image"); !eqStr(got, []string{"Claude Opus", "Flux Image"}) {
		t.Errorf("any:text,image served %v, want the union", got)
	}
	// A flag reads either spelling, exactly as an exact match does.
	if got := queried(t, "?favorite=any:true"); !eqStr(got, []string{"Gemini Flash"}) {
		t.Errorf("any:true on a flag served %v, want Gemini Flash", got)
	}
	// An empty member is the empty match, so any: alone asks "no value at all".
	if got := queried(t, "?notes=any:"); !eqStr(got, []string{"Claude Opus", "Flux Image"}) {
		t.Errorf("any: alone served %v, want the models with no note", got)
	}
}

// TestNotExcludesEveryListedValue is the exclude half: the complement, with a
// model holding nothing matching too (it equals none of the listed values),
// and not: alone keeping only the models that state something.
func TestNotExcludesEveryListedValue(t *testing.T) {
	aCatalogToQuery(t)

	if got := queried(t, "?input_modalities=not:image"); !eqStr(got, []string{"Claude Opus", "Gemini Flash"}) {
		t.Errorf("not:image served %v, want everything not taking an image (the model stating nothing included)", got)
	}
	if got := queried(t, "?notes=not:"); !eqStr(got, []string{"Gemini Flash"}) {
		t.Errorf("not: alone served %v, want only the models that state something", got)
	}
}

// TestARepeatedParameterMeansAndAlso pins the fix to the silent drop: two
// values of one parameter are two conditions, so an include list and an
// exclude list can ride on one column in one address.
func TestARepeatedParameterMeansAndAlso(t *testing.T) {
	aCatalogToQuery(t)

	if got := queried(t, "?input_modalities=text&input_modalities=image"); !eqStr(got, []string{"Flux Image"}) {
		t.Errorf("two exact matches on one column served %v, want their AND (Flux Image)", got)
	}
	if got := queried(t, "?input_modalities=any:text,image&input_modalities=not:audio"); !eqStr(got, []string{"Claude Opus", "Flux Image"}) {
		t.Errorf("any: plus not: on one column served %v, want both applied", got)
	}
}

// TestAnEscapedCommaIsALiteralComma pins the \, escape, without which a value
// holding a comma could never be listed.
func TestAnEscapedCommaIsALiteralComma(t *testing.T) {
	aCatalogToQuery(t)
	if _, err := store.CreateOwnColumn(store.ColumnSpec{Label: "Tags"}); err != nil {
		t.Fatalf("make a column of his own: %v", err)
	}
	if err := store.WriteCells(store.SourceHand, "Claude Opus",
		map[string]string{"tags": "long, careful"}); err != nil {
		t.Fatalf("write a value holding a comma: %v", err)
	}

	// %5C is the backslash; unescaped, the same comma would split the value in two.
	if got := queried(t, "?tags=any:long%5C%2C%20careful"); !eqStr(got, []string{"Claude Opus"}) {
		t.Errorf("any: with an escaped comma served %v, want Claude Opus", got)
	}
	if got := queried(t, "?tags=any:long,careful"); len(got) != 0 {
		t.Errorf("any: with an unescaped comma served %v, want nothing (two members, neither held)", got)
	}
}

// TestAnyAndNotAreRefusedOnANumberColumn pins the one kind the two operators do
// not cover, with the refusal naming what a number column does take.
func TestAnyAndNotAreRefusedOnANumberColumn(t *testing.T) {
	aCatalogToQuery(t)

	for _, query := range []string{"?context_length=any:1,2", "?context_length=not:1"} {
		body := refused(t, query)
		if !strings.Contains(body, "min: and max:") {
			t.Errorf("GET /api/models%s was refused without offering min: and max:: %s", query, body)
		}
	}
}
