package app

import (
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// createColumnOverHTTP asks the server to create a column of the reader's own,
// exactly as an agent or the page would, and answers what it said.
func createColumnOverHTTP(t *testing.T, body string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	handleColumns(rec, httptest.NewRequest(http.MethodPost, "/api/columns", strings.NewReader(body)))
	return rec
}

// TestCreateListDeleteAColumnOfYourOwn exercises the whole column surface over
// HTTP: creating a text column and a choice column, listing them back with their
// options in entry order, then deleting one and confirming it (and only it)
// disappears. Every freshly migrated database already carries the two columns a
// fresh install starts with ("Rating" and "OCR"), so this asserts on the two
// columns it adds itself rather than on a total.
func TestCreateListDeleteAColumnOfYourOwn(t *testing.T) {
	migratedDB(t)
	baseline, err := ownColumnsList()
	if err != nil {
		t.Fatalf("baseline list: %v", err)
	}

	if rec := createColumnOverHTTP(t, `{"label":"Vendor","value_type":"text"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create text column: %d %s", rec.Code, rec.Body.String())
	}
	if rec := createColumnOverHTTP(t, `{"label":"Tier","value_type":"choice","options":["gold","silver","bronze"]}`); rec.Code != http.StatusCreated {
		t.Fatalf("create choice column: %d %s", rec.Code, rec.Body.String())
	}

	cols, err := ownColumnsList()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cols) != len(baseline)+2 {
		t.Fatalf("got %d columns of the reader's own, want %d (baseline + 2)", len(cols), len(baseline)+2)
	}
	vendor, held := columnCalled(t, "Vendor")
	if !held || vendor.Key != "vendor" || vendor.ValueType != store.ValueTypeText || vendor.Options != nil {
		t.Errorf("text column = %+v", vendor)
	}
	tier, held := columnCalled(t, "Tier")
	if !held || tier.Key != "tier" {
		t.Errorf("choice column = %+v", tier)
	}
	if want := []string{"gold", "silver", "bronze"}; len(tier.Options) != len(want) || tier.Options[0] != want[0] || tier.Options[2] != want[2] {
		t.Errorf("choice options = %v, want %v (in entry order)", tier.Options, want)
	}

	if rec := deleteColumnOverHTTP(t, vendor.Key, true); rec.Code != http.StatusOK {
		t.Fatalf("delete: %d %s", rec.Code, rec.Body.String())
	}
	if _, held := columnCalled(t, "Vendor"); held {
		t.Error("the deleted column is still in the list")
	}
	if _, held := columnCalled(t, "Tier"); !held {
		t.Error("the column that was not deleted went with it")
	}
}

// TestDeletingAColumnNeedsTheConfirmationInTheBody. One request destroys a value
// on every model at once and this database is their only copy, so the
// confirmation is where the caller has to write it - the same rule
// POST /api/models/delete follows.
func TestDeletingAColumnNeedsTheConfirmationInTheBody(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("M", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	key, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("M", key, "gold"); err != nil {
		t.Fatal(err)
	}

	rec := deleteColumnOverHTTP(t, key, false)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("an unconfirmed delete answered %d, want 400: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "confirm") {
		t.Errorf("the refusal must say how to confirm, got %q", rec.Body.String())
	}
	if _, held, _ := ownValueOf("M", key); !held {
		t.Error("the unconfirmed delete removed the value anyway")
	}
	if _, held := columnCalled(t, "Tier"); !held {
		t.Error("the unconfirmed delete removed the column anyway")
	}

	if rec := deleteColumnOverHTTP(t, key, true); rec.Code != http.StatusOK {
		t.Fatalf("the confirmed delete answered %d: %s", rec.Code, rec.Body.String())
	}
	if _, held := columnCalled(t, "Tier"); held {
		t.Error("the confirmed delete left the column behind")
	}
}

// TestCreatingAColumnRefusesWhatCannotMeanAnything pins the rules a definition
// has to keep: a kind of value this app does not store, a choice column with no
// options to choose from, no heading at all, and a heading another column holds.
func TestCreatingAColumnRefusesWhatCannotMeanAnything(t *testing.T) {
	migratedDB(t)

	for _, c := range []struct{ what, body string }{
		{"a kind of value nothing stores", `{"label":"Bad","value_type":"enum"}`},
		{"a choice column with no options", `{"label":"Empty Choice","value_type":"choice","options":["  ",""]}`},
		{"no heading at all", `{"label":"","value_type":"text"}`},
		{"a dotted key, which carries no namespace any more", `{"key":"mine.sneaky","label":"Sneaky","value_type":"text"}`},
	} {
		if rec := createColumnOverHTTP(t, c.body); rec.Code == http.StatusCreated {
			t.Errorf("%s was accepted", c.what)
		}
	}

	if rec := createColumnOverHTTP(t, `{"label":"Plain Text","value_type":"text","options":["ignored"]}`); rec.Code == http.StatusCreated {
		t.Error("options on a column that offers no choice were accepted")
	}
	if rec := createColumnOverHTTP(t, `{"label":"Plain Text","value_type":"text"}`); rec.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	if rec := createColumnOverHTTP(t, `{"label":"plain text","value_type":"text"}`); rec.Code == http.StatusCreated {
		t.Error("a heading another column already holds was accepted")
	}
}

// TestAColumnTheAppShipsCannotBeDeleted. Every writer of that source binds
// through its key and its kind, and a source states its columns again on every
// read - so it can be relabelled and hidden, and never removed.
func TestAColumnTheAppShipsCannotBeDeleted(t *testing.T) {
	migratedDB(t)
	if _, err := store.SeedShippedColumns(); err != nil {
		t.Fatalf("seed the columns the app ships: %v", err)
	}
	for _, key := range []string{"notes", "context_length", "context_length"} {
		if rec := deleteColumnOverHTTP(t, key, true); rec.Code != http.StatusBadRequest {
			t.Errorf("deleting %s answered %d, want 400: %s", key, rec.Code, rec.Body.String())
		}
		if _, err := store.ColumnByKey(key); err != nil {
			t.Errorf("%s was deleted after all: %v", key, err)
		}
	}
}

// TestDeleteCustomColumnCascadesValues pins the delete-wipes-everything rule:
// removing a column definition permanently deletes every model's stored value
// for it, via ON DELETE CASCADE on cells.column_id.
func TestDeleteCustomColumnCascadesValues(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("M", map[string]string{}); err != nil {
		t.Fatal(err)
	}
	key, err := ownColumnKey(t, "Tier", store.ValueTypeChoice, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("M", key, "gold"); err != nil {
		t.Fatalf("set value: %v", err)
	}
	column, err := store.ColumnByKey(key)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok, err := ownValueOf("M", key); err != nil || !ok {
		t.Fatalf("value should be stored before delete: ok=%v err=%v", ok, err)
	}

	if err := store.DeleteColumn(key); err != nil {
		t.Fatalf("delete column: %v", err)
	}

	var n int
	if err := dbcore.DB.QueryRow("SELECT COUNT(*) FROM cells WHERE column_id=?", column.ID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("%d stored value(s) outlived the column they belonged to", n)
	}
	if _, ok, err := ownValueOf("M", key); err != nil || ok {
		t.Errorf("the value reads back as present after the delete: ok=%v err=%v", ok, err)
	}
}

// TestSetGetCustomValueLifecycle covers writing and reading a value for the
// kinds of column a person makes: text round-trips verbatim, a choice column
// takes one of its own options and nothing else, and clearing a value removes
// the cell entirely rather than storing "".
func TestSetGetCustomValueLifecycle(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("M", map[string]string{}); err != nil {
		t.Fatal(err)
	}

	textKey, err := ownColumnKey(t, "Vendor Note", store.ValueTypeText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("M", textKey, "Acme Corp"); err != nil {
		t.Fatalf("set text value: %v", err)
	}
	if v, ok, err := ownValueOf("M", textKey); err != nil || !ok || v != "Acme Corp" {
		t.Fatalf("get text value: v=%q ok=%v err=%v", v, ok, err)
	}

	numKey, err := ownColumnKey(t, "Priority", store.ValueTypeChoice, []string{"1", "2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if err := setOwnValue("M", numKey, "2"); err != nil {
		t.Fatalf("set a choice value: %v", err)
	}
	if err := setOwnValue("M", numKey, "99"); err == nil {
		t.Error("a value outside the column's options should be refused")
	}

	// Clearing a value (empty string) removes it entirely.
	if err := setOwnValue("M", textKey, ""); err != nil {
		t.Fatalf("clear value: %v", err)
	}
	if _, ok, err := ownValueOf("M", textKey); err != nil || ok {
		t.Errorf("cleared value should be absent, ok=%v err=%v", ok, err)
	}

	vals, err := ownValuesOf("M")
	if err != nil {
		t.Fatalf("list model values: %v", err)
	}
	if len(vals) != 1 || vals[numKey] != "2" {
		t.Errorf("the model's own values = %v, want only {%q: \"2\"}", vals, numKey)
	}
}

// TestCustomColumnDataStaysInDBNeverExported is the custom-column equivalent of
// TestPersonalDataStaysInDBNeverExported (durability_test.go): a column of the
// reader's own and its per-model values are ALWAYS personal-only, so the
// heading, its options and every stored value stay in the database and reach no
// file - the database is their only copy.
func TestCustomColumnDataStaysInDBNeverExported(t *testing.T) {
	migratedDB(t)

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "M", "source": "huggingface", "hf_slug": "x/m",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	key, err := ownColumnKey(t, "Secret Tag", store.ValueTypeChoice, []string{"alpha-leak", "beta-leak"})
	if err != nil {
		t.Fatalf("create the column: %v", err)
	}
	if err := setOwnValue("M", key, "alpha-leak"); err != nil {
		t.Fatalf("set the value: %v", err)
	}

	// A save writes real data. Nothing may appear in the data dir but the
	// database and the sidecars SQLite keeps beside it.
	save := httptest.NewRecorder()
	handleSaveModels(save, httptest.NewRequest(http.MethodPost, "/api/save",
		strings.NewReader(`[{"name":"M","cells":{"notes":"n"}}]`)))
	if save.Code != http.StatusOK {
		t.Fatalf("POST /api/save status = %d, body %s", save.Code, save.Body.String())
	}
	assertOnlyTheDatabaseInDataDir(t, "saving with a value in a column of the reader's own")

	// The value is still there, which is the other half of the promise: private,
	// not lost.
	if v, ok, _ := ownValueOf("M", key); !ok || v != "alpha-leak" {
		t.Errorf("the value should still be in the database, got %q (present=%v)", v, ok)
	}
}

// TestANumberColumnStoresOnlyNumbers covers the number kind end to end: it is
// created like any other column, it holds a figure exactly as sent (decimals
// included), and it refuses anything that is not a number - because a column the
// page sorts as a quantity cannot hold a word.
func TestANumberColumnStoresOnlyNumbers(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("Local Build", map[string]string{}); err != nil {
		t.Fatal(err)
	}

	key, err := ownColumnKey(t, "KV cache per token", store.ValueTypeNumber, nil)
	if err != nil {
		t.Fatalf("create number column: %v", err)
	}
	column, held := columnCalled(t, "KV cache per token")
	if !held || column.Key != key || column.ValueType != store.ValueTypeNumber || column.Options != nil {
		t.Errorf("number column = %+v, want kind %q and no options", column, store.ValueTypeNumber)
	}

	// The digits are stored as sent: a figure a person researched must not be
	// rounded on the way in.
	if err := setOwnValue("Local Build", key, "131.072"); err != nil {
		t.Fatalf("store a decimal: %v", err)
	}
	got, ok, err := ownValueOf("Local Build", key)
	if err != nil || !ok || got != "131.072" {
		t.Errorf("stored value = %q ok=%v err=%v, want \"131.072\"", got, ok, err)
	}

	// Anything that is not a number is refused, and refusing writes nothing.
	if err := setOwnValue("Local Build", key, "quite big"); err == nil {
		t.Error("a number column accepted a word; it must refuse one")
	}
	got, _, _ = ownValueOf("Local Build", key)
	if got != "131.072" {
		t.Errorf("a refused write changed the stored value to %q, want the 131.072 it held", got)
	}

	// An empty value clears it, exactly as it does for every other kind.
	if err := setOwnValue("Local Build", key, ""); err != nil {
		t.Fatalf("clear: %v", err)
	}
	if _, ok, _ := ownValueOf("Local Build", key); ok {
		t.Error("an empty value should have cleared the stored number")
	}
}

// TestTheMeasuredFiguresAreStoredAtTwoDecimals pins the rounding rule where it is
// applied - on the way into the database - for the three figures it covers, and
// pins that it stops there.
//
// Storing the rounded figure rather than showing a long one short is the whole
// point: a value read out of one of these columns and written back into it has to
// be the value that was already there, whichever of the write paths carried it.
func TestTheMeasuredFiguresAreStoredAtTwoDecimals(t *testing.T) {
	migratedDB(t)

	if rec := addModel(t, `{"name":"Local Build","cells":{}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d, body %s", rec.Code, rec.Body.String())
	}
	for _, c := range []struct{ column, sent, want string }{
		{"Parameters (B)", "32.762123264", "32.76"},
		{"Parameters (B)", "0.6", "0.6"},
		{"Parameters (B)", "27.0", "27"},
		// The figure two decimals exist for: a 350-million-parameter model. One
		// decimal place made this 0.4, a number nothing states about it.
		{"Parameters (B)", "0.35", "0.35"},
		{"Parameters (B)", "33.10", "33.1"},
		{"Active parameters (B)", "3.31", "3.31"},
		{"Weights size (GB)", "65.52", "65.52"},

		// An exact byte count is NOT one of the rounded figures: only its GB
		// rendering in the browser is shortened, and the count stays as the
		// importer measured it.
		{"Build size (bytes)", "17666268832", "17666268832"},
	} {
		// Three of these four labels are shipped columns now (parameters,
		// active_parameters, weights_size_gb) - sealed, so ownColumnKey would
		// only ever try to mint a colliding one of its own. Read the sealed
		// column's own key where one already carries this label, and mint a
		// column of the reader's own only for the one label nothing ships.
		var key string
		if c2, held := columnCalled(t, c.column); held {
			key = c2.Key
		} else {
			var err error
			key, err = ownColumnKey(t, c.column, store.ValueTypeNumber, nil)
			if err != nil {
				t.Fatalf("create %s: %v", c.column, err)
			}
		}
		if err := setOwnValue("Local Build", key, c.sent); err != nil {
			t.Fatalf("store %s: %v", c.column, err)
		}
		got, _, err := ownValueOf("Local Build", key)
		if err != nil {
			t.Fatal(err)
		}
		if got != c.want {
			t.Errorf("%s sent as %q is stored as %q, want %q", c.column, c.sent, got, c.want)
		}
	}

	// Rounding happens after the number check, so a word is still refused rather
	// than quietly stored.
	col, held := columnCalled(t, "Parameters (B)")
	if !held {
		t.Fatal("Parameters (B) should already be shipped")
	}
	if err := setOwnValue("Local Build", col.Key, "about thirty"); err == nil {
		t.Error("a word was accepted into Parameters (B); it must be refused")
	}
}
