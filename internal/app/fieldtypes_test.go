package app

import (
	"fmt"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

// saveModels drives POST /api/save with the given body and hands back what the
// real handler answered.
func saveModels(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleSaveModels(rec, httptest.NewRequest(http.MethodPost, "/api/save", strings.NewReader(body)))
	return rec
}

// nullableInt reads a numeric field and says whether anybody has stated it at
// all, which colInt cannot: nothing stated and a stated 0 both read back as 0.
func nullableInt(t *testing.T, name, col string) (int64, bool) {
	t.Helper()
	figure, stated := nullableFloat(t, name, col)
	return int64(figure), stated
}

func nullableFloat(t *testing.T, name, col string) (float64, bool) {
	t.Helper()
	text := strings.TrimSpace(colStr(t, name, col))
	if text == "" {
		return 0, false
	}
	figure, err := strconv.ParseFloat(text, 64)
	if err != nil {
		return 0, false
	}
	return figure, true
}

// TestSaveRefusesAWrongTypeAndKeepsTheStoredValue is the regression this check
// exists for, and it is about a SAVE rather than an add: a save is an overlay
// onto values already in the row, so a number the statement could not read used
// to empty the column and answer 200. The count in it was researched by hand and
// exists in no other file, so that answer destroyed data while reporting
// success.
func TestSaveRefusesAWrongTypeAndKeepsTheStoredValue(t *testing.T) {
	migratedDB(t)

	if rec := addModel(t, `{"name":"Qwen: Qwen3 HF","cells":{"notes":"researched by hand","context_length":"262144"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	// The exact body shape that used to answer 200 and clear the value: a value
	// the column's kind will not take. Every cell is sent as text, so what is
	// checked is what that text READS as - and "quite long" is not a figure.
	rec := saveModels(t, `[{"name":"Qwen: Qwen3 HF","cells":{"context_length":"quite long"}}]`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a quoted number status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "context_length") {
		t.Errorf("the refusal does not name the field: %s", body)
	}

	// Every stored value is exactly as it was, including the ones the same
	// request did not mention.
	if v, ok := nullableInt(t, "Qwen: Qwen3 HF", "context_length"); !ok || v != 262144 {
		t.Errorf("context_length = %d (present=%v), want the stored 262144 untouched", v, ok)
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "notes"); got != "researched by hand" {
		t.Errorf("notes = %q, want the stored note untouched", got)
	}

	// The same shape on each of the other kinds of column.
	for _, c := range []struct{ what, body string }{
		{"a number sent unquoted, which is not the text a cell holds", `[{"name":"Qwen: Qwen3 HF","cells":{"notes":42}}]`},
		{"a word for a flag column", `[{"name":"Qwen: Qwen3 HF","cells":{"favorite":"maybe"}}]`},
		{"a word for a number column", `[{"name":"Qwen: Qwen3 HF","cells":{"price_prompt":"cheap"}}]`},
		{"the one column no hand write can ever state", `[{"name":"Qwen: Qwen3 HF","cells":{"added":"2026-01-01"}}]`},
	} {
		if r := saveModels(t, c.body); r.Code != http.StatusBadRequest {
			t.Errorf("%s status = %d, want 400; body %s", c.what, r.Code, r.Body.String())
		}
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "notes"); got != "researched by hand" {
		t.Errorf("notes = %q after four refusals, want it untouched", got)
	}
	if got := colInt(t, "Qwen: Qwen3 HF", "favorite"); got != 0 {
		t.Errorf("favorite = %d after a refused flag, want it untouched", got)
	}
}

// TestSaveRefusesAWrongCustomValueAndKeepsTheStoredOne is the same regression
// one table over, and the destruction was quieter there because no column on the
// models table is involved. A value that did not arrive as text was read as an
// empty string, and an empty string is how a custom value is CLEARED - so
// sending 4 for a rating whose options are the labels "1" to "4" deleted the
// rating and answered 200. Nothing upstream re-creates a custom value.
func TestSaveRefusesAWrongCustomValueAndKeepsTheStoredOne(t *testing.T) {
	migratedDB(t)

	if rec := addModel(t, `{"name":"Qwen: Qwen3 HF","cells":{"notes":"mine"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}
	// A number dropdown, the exact shape this defect was worst on: its options
	// are the labels "1" to "4", so a bare 4 sent for it looks like a valid
	// option and is what used to slip through as an empty string.
	col, err := ownColumnKey(t, "Rating", store.ValueTypeChoice, []string{"1", "2", "3", "4"})
	if err != nil {
		t.Fatalf("create the Rating column: %v", err)
	}
	if err := setOwnValue("Qwen: Qwen3 HF", col, "4"); err != nil {
		t.Fatalf("seed value: %v", err)
	}

	stored := func() string {
		t.Helper()
		vals, err := ownValuesOf("Qwen: Qwen3 HF")
		if err != nil {
			t.Fatalf("read custom values: %v", err)
		}
		return vals[col]
	}
	if got := stored(); got != "4" {
		t.Fatalf("seeded value = %q, want %q", got, "4")
	}

	// The exact body that used to answer 200 and delete the rating: the label
	// sent as the number it looks like.
	body := fmt.Sprintf(`[{"name":"Qwen: Qwen3 HF","cells":{%q:"9"}}]`, col)
	rec := saveModels(t, body)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a number for a custom value status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if got := rec.Body.String(); !strings.Contains(got, fmt.Sprint(col)) {
		t.Errorf("the refusal does not name the column: %s", got)
	}
	if got := stored(); got != "4" {
		t.Errorf("custom value = %q, want the stored %q untouched", got, "4")
	}

	// A refused custom value must not write the rest of the same object either,
	// which is what "writes NOTHING" means everywhere else in this API.
	both := fmt.Sprintf(`[{"name":"Qwen: Qwen3 HF","cells":{"notes":"overwritten",%q:"gold"}}]`, col)
	if r := saveModels(t, both); r.Code != http.StatusBadRequest {
		t.Fatalf("a boolean status = %d, want 400; body %s", r.Code, r.Body.String())
	}
	if got := colStr(t, "Qwen: Qwen3 HF", "notes"); got != "mine" {
		t.Errorf("notes = %q, want the stored note untouched by a refused request", got)
	}

	// Clearing on purpose still works, or the refusal would have taken away the
	// only way to empty the column.
	clear := fmt.Sprintf(`[{"name":"Qwen: Qwen3 HF","cells":{%q:""}}]`, col)
	if r := saveModels(t, clear); r.Code != http.StatusOK {
		t.Fatalf("clearing status = %d, want 200; body %s", r.Code, r.Body.String())
	}
	if got := stored(); got != "" {
		t.Errorf("custom value = %q after an intentional clear, want it gone", got)
	}
}

// TestAddRefusesAWrongTypeAndCreatesNoRow covers the other writer. The add is
// two statements - the row, then the curated values - so the refusal has to
// land before the first of them or it leaves an empty row behind under a name
// the caller was told was not added.
func TestAddRefusesAWrongTypeAndCreatesNoRow(t *testing.T) {
	migratedDB(t)

	rec := addModel(t, `{"name":"My Local Build","cells":{"notes":"mine","context_length":"twenty-seven"}}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("a value the column will not hold: status = %d, want 400; body %s", rec.Code, rec.Body.String())
	}
	if body := rec.Body.String(); !strings.Contains(body, "context_length") {
		t.Errorf("the refusal does not name the field: %s", body)
	}
	if store.ModelExists("My Local Build") {
		t.Error("a refused add created the row")
	}
	if n, err := dbcore.CountModels(); err != nil || n != 0 {
		t.Errorf("the database holds %d models (err %v), want none", n, err)
	}

	// The same body with a real number is accepted, so the refusal is about the
	// type and nothing else.
	ok := addModel(t, `{"name":"My Local Build","cells":{"notes":"mine","context_length":"8192"}}`)
	if ok.Code != http.StatusCreated {
		t.Fatalf("status = %d, want 201; body %s", ok.Code, ok.Body.String())
	}
	if v, present := nullableInt(t, "My Local Build", "context_length"); !present || v != 8192 {
		t.Errorf("context_length = %d (present=%v), want 8192", v, present)
	}
}

// TestAFractionalCountKeepsItsDecimal is the regression for a loss no type check
// could see: the value was the right KIND, so nothing was refused - the figure
// simply came out as a different number. A parameter count is in BILLIONS, and a
// whole-number column turned 3.3 into 3 and 0.6 into 0 while the save answered
// success.
//
// A count is stored at ONE DECIMAL PLACE, which is what the three figures here
// are chosen to pin. 3.3 is the active count of a 30B-A3B Mixture-of-Experts
// model and survives as it was sent. 0.6 is a whole model - Qwen3-0.6B, and
// every small embedding model - and must never become 0, a value the page and
// the guide both read as "nobody has filled this in". 32.762123264 is what a
// repository's own element count works out to, and two decimals are all of it
// anybody reads.
func TestAFractionalCountKeepsItsDecimal(t *testing.T) {
	migratedDB(t)

	if rec := addModel(t, `{"name":"Qwen: Qwen3-0.6B","cells":{"notes":"small"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}
	// "Parameters (B)" and "Active parameters (B)" are shipped columns (their
	// bare keys are "parameters" and "active_parameters"), seeded the moment the
	// model above was added - nothing here mints a column of its own.
	if rec := saveModels(t, `[{"name":"Qwen: Qwen3-0.6B","cells":{"parameters":"0.6","active_parameters":"3.3"}}]`); rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct{ column, want string }{
		{"Parameters (B)", "0.6"},
		{"Active parameters (B)", "3.3"},
	} {
		if got := customValue(t, "Qwen: Qwen3-0.6B", c.column); got != c.want {
			t.Errorf("%s = %q, want %q", c.column, got, c.want)
		}
	}

	// A figure carrying more precision than anybody reads is stored rounded, not
	// stored long and shown short - so a value read out of the column and written
	// back into it is the value that was already there.
	if rec := saveModels(t, `[{"name":"Qwen: Qwen3-0.6B","cells":{"parameters":"32.762123264"}}]`); rec.Code != http.StatusOK {
		t.Fatalf("save status = %d, body %s", rec.Code, rec.Body.String())
	}
	if got := customValue(t, "Qwen: Qwen3-0.6B", "Parameters (B)"); got != "32.76" {
		t.Errorf("Parameters (B) = %q, want %q", got, "32.76")
	}

	// And what an outside consumer receives is that same stored figure.
	rows := servedModels(t, "?name=qwen3-0.6b")
	if len(rows) != 1 {
		t.Fatalf("GET /api/models served %v, want the one model", modelNames(rows))
	}
	cells, _ := rows[0]["cells"].(map[string]interface{})
	found := false
	for _, v := range cells {
		if v == "32.76" {
			found = true
		}
	}
	if !found {
		t.Errorf("the served cells = %v, want the stored 32.76", cells)
	}
}

// TestClearingAFieldOnPurposeStillWorks pins the behaviour an over-strict check
// would ruin. Emptying a field is a real instruction and each column has its own
// way of saying it, which the guide documents: null for a number, "" for the
// text fields that take it, and - the exception - "" IGNORED by tool and moe, so
// that "this model has no tool support" is said with "no" rather than by leaving
// the field blank.
func TestClearingAFieldOnPurposeStillWorks(t *testing.T) {
	migratedDB(t)

	if rec := addModel(t, `{"name":"Filled In","cells":{"notes":"a note","supports_tools":"yes","moe":"yes","price_unit":"per 1M tokens","favorite":"yes","context_length":"8192"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}

	// null empties the numeric column.
	if rec := saveModels(t, `[{"name":"Filled In","cells":{"context_length":""}}]`); rec.Code != http.StatusOK {
		t.Fatalf("clearing with null status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if v, present := nullableInt(t, "Filled In", "context_length"); present {
		t.Errorf("context_length = %d, want it cleared by null", v)
	}

	// "" empties the three text columns that take it, and false lowers the flag.
	if rec := saveModels(t, `[{"name":"Filled In","cells":{"notes":"","price_unit":"","favorite":"no"}}]`); rec.Code != http.StatusOK {
		t.Fatalf("clearing with \"\" status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	for _, col := range []string{"notes", "measurement"} {
		if got := colStr(t, "Filled In", col); got != "" {
			t.Errorf("%s = %q, want it cleared by \"\"", col, got)
		}
	}
	if got := colInt(t, "Filled In", "favorite"); got != 0 {
		t.Errorf("favorite = %d, want false to have lowered it", got)
	}

	// THE TWO CAPABILITY ANSWERS ARE CLEARED LIKE ANY OTHER CELL. They used to
	// treat "" as "no answer" and leave the stored word alone, because the column
	// they shared with every source had no way to say "nobody has said". Each
	// answer is a column of its own now and an empty cell IS that state, so
	// clearing one is saying nothing - and the next refresh may answer it again.
	if rec := saveModels(t, `[{"name":"Filled In","cells":{"supports_tools":"","moe":""}}]`); rec.Code != http.StatusOK {
		t.Fatalf("empty tool/moe status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	for _, key := range []string{"supports_tools", "moe"} {
		if got := cellValue(t, "Filled In", key); got != "" {
			t.Errorf("%s = %q, want an empty value to have cleared it", key, got)
		}
	}

	// The flag's other accepted spellings still write.
	for _, c := range []struct {
		body string
		want int64
	}{
		{`[{"name":"Filled In","cells":{"favorite":"1"}}]`, 1},
		{`[{"name":"Filled In","cells":{"favorite":"0"}}]`, 0},
		{`[{"name":"Filled In","cells":{"favorite":"true"}}]`, 1},
		{`[{"name":"Filled In","cells":{"favorite":"false"}}]`, 0},
	} {
		if rec := saveModels(t, c.body); rec.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body %s", c.body, rec.Code, rec.Body.String())
		}
		if got := colInt(t, "Filled In", "favorite"); got != c.want {
			t.Errorf("%s left favorite = %d, want %d", c.body, got, c.want)
		}
	}
}
