package app

import (
	"encoding/json"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestCreateListDeleteCustomColumn exercises the basic CRUD path: creating a
// text column and a dropdown column, listing them back with their options in
// entry order, then deleting one and confirming it (and only it) disappears.
// Every freshly migrated DB already carries the three columns the v4 migration
// seeds ("Speed", "Rating", "OCR" - see migrate.go), so this test only asserts
// on the two columns it adds itself, not the total count.
func TestCreateListDeleteCustomColumn(t *testing.T) {
	migratedDB(t)
	baseline, err := store.ListCustomColumns()
	if err != nil {
		t.Fatalf("baseline list: %v", err)
	}

	textID, err := store.CreateCustomColumn("Vendor", store.ColumnTypeText, nil)
	if err != nil {
		t.Fatalf("create text column: %v", err)
	}
	ddID, err := store.CreateCustomColumn("Tier", store.ColumnTypeDropdownText, []string{"gold", "silver", "bronze"})
	if err != nil {
		t.Fatalf("create dropdown column: %v", err)
	}

	cols, err := store.ListCustomColumns()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(cols) != len(baseline)+2 {
		t.Fatalf("got %d columns, want %d (baseline + 2)", len(cols), len(baseline)+2)
	}
	textCol, ddCol := cols[len(cols)-2], cols[len(cols)-1]
	if textCol.ID != textID || textCol.Name != "Vendor" || textCol.Type != store.ColumnTypeText || textCol.Options != nil {
		t.Errorf("text column = %+v", textCol)
	}
	if ddCol.ID != ddID || ddCol.Name != "Tier" {
		t.Errorf("dropdown column = %+v", ddCol)
	}
	if want := []string{"gold", "silver", "bronze"}; len(ddCol.Options) != len(want) || ddCol.Options[0] != want[0] || ddCol.Options[2] != want[2] {
		t.Errorf("dropdown options = %v, want %v (in entry order)", ddCol.Options, want)
	}

	if err := store.DeleteCustomColumn(textID); err != nil {
		t.Fatalf("delete: %v", err)
	}
	cols, err = store.ListCustomColumns()
	if err != nil {
		t.Fatal(err)
	}
	if len(cols) != len(baseline)+1 || cols[len(cols)-1].ID != ddID {
		t.Fatalf("after delete, columns = %+v, want baseline + only the dropdown column", cols)
	}
}

// TestCreateCustomColumnValidation pins the predefined-type-system rules: an
// unknown type is refused, a dropdown column needs at least one non-blank
// option, and a text column ignores any options it is passed.
func TestCreateCustomColumnValidation(t *testing.T) {
	migratedDB(t)

	if _, err := store.CreateCustomColumn("Bad", "enum", nil); err == nil {
		t.Error("an unrecognized column type should be refused")
	}
	if _, err := store.CreateCustomColumn("Empty Dropdown", store.ColumnTypeDropdownText, []string{"  ", ""}); err == nil {
		t.Error("a dropdown column with no non-blank options should be refused")
	}
	if _, err := store.CreateCustomColumn("", store.ColumnTypeText, nil); err == nil {
		t.Error("an empty column name should be refused")
	}

	id, err := store.CreateCustomColumn("Plain Text", store.ColumnTypeText, []string{"ignored"})
	if err != nil {
		t.Fatalf("create text column: %v", err)
	}
	cols, _ := store.ListCustomColumns()
	for _, c := range cols {
		if c.ID == id && len(c.Options) != 0 {
			t.Errorf("a text column must not store options, got %v", c.Options)
		}
	}
}

// TestDeleteCustomColumnCascadesValues pins the delete-wipes-everything rule:
// removing a column definition permanently deletes every model's stored value
// for it, via ON DELETE CASCADE on custom_values.column_id.
func TestDeleteCustomColumnCascadesValues(t *testing.T) {
	migratedDB(t)
	if err := store.ImportFullRecord(map[string]interface{}{"name": "M", "source": "manual"}, "manual", "chat"); err != nil {
		t.Fatal(err)
	}
	id, err := store.CreateCustomColumn("Tier", store.ColumnTypeDropdownText, []string{"gold", "silver"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCustomValue("M", id, "gold"); err != nil {
		t.Fatalf("set value: %v", err)
	}
	if _, ok, err := store.GetCustomValue("M", id); err != nil || !ok {
		t.Fatalf("value should be stored before delete: ok=%v err=%v", ok, err)
	}

	if err := store.DeleteCustomColumn(id); err != nil {
		t.Fatalf("delete column: %v", err)
	}

	var n int
	if err := dbcore.DB.QueryRow("SELECT COUNT(*) FROM custom_values WHERE column_id=?", id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("custom_values still has %d row(s) for a deleted column", n)
	}
	if _, ok, err := store.GetCustomValue("M", id); err != nil || ok {
		t.Errorf("GetCustomValue after delete: ok=%v err=%v, want ok=false", ok, err)
	}
}

// TestSetGetCustomValueLifecycle covers get/set for all three predefined types:
// a text value round-trips verbatim, a dropdown value must be one of the
// column's options, and clearing a value (empty string) removes its row
// entirely rather than storing "" (sparse storage).
func TestSetGetCustomValueLifecycle(t *testing.T) {
	migratedDB(t)
	if err := store.ImportFullRecord(map[string]interface{}{"name": "M", "source": "manual"}, "manual", "chat"); err != nil {
		t.Fatal(err)
	}

	textID, err := store.CreateCustomColumn("Vendor", store.ColumnTypeText, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCustomValue("M", textID, "Acme Corp"); err != nil {
		t.Fatalf("set text value: %v", err)
	}
	if v, ok, err := store.GetCustomValue("M", textID); err != nil || !ok || v != "Acme Corp" {
		t.Fatalf("get text value: v=%q ok=%v err=%v", v, ok, err)
	}

	numID, err := store.CreateCustomColumn("Priority", store.ColumnTypeDropdownNumber, []string{"1", "2", "3"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SetCustomValue("M", numID, "2"); err != nil {
		t.Fatalf("set dropdown_number value: %v", err)
	}
	if err := store.SetCustomValue("M", numID, "99"); err == nil {
		t.Error("a value outside the column's options should be refused")
	}

	// Clearing a value (empty string) removes it entirely.
	if err := store.SetCustomValue("M", textID, ""); err != nil {
		t.Fatalf("clear value: %v", err)
	}
	if _, ok, err := store.GetCustomValue("M", textID); err != nil || ok {
		t.Errorf("cleared value should be absent, ok=%v err=%v", ok, err)
	}

	vals, err := store.ListModelCustomValues("M")
	if err != nil {
		t.Fatalf("list model values: %v", err)
	}
	if len(vals) != 1 || vals[numID] != "2" {
		t.Errorf("ListModelCustomValues = %v, want only {%d: \"2\"}", vals, numID)
	}
}

// TestCustomColumnDataStaysInDBNeverExported is the custom-column equivalent of
// TestPersonalDataStaysInDBNeverExported/TestPopulatedDBKeepsDBAuthoritative/
// TestExportMissingCreatesCuratedOnly in seed_test.go: a custom column and its
// per-model values are ALWAYS personal-only, so an export must never surface
// the column name, its options, or any stored value, and no personal.json (or
// any other file) is ever produced for them - the only durable copy is the DB.
func TestCustomColumnDataStaysInDBNeverExported(t *testing.T) {
	migratedDB(t)
	durablePaths(t)
	orig := shared.Version
	shared.Version = "999.0.0"
	defer func() { shared.Version = orig }()

	if err := store.ImportFullRecord(map[string]interface{}{
		"name": "M", "source": "huggingface", "hf_slug": "x/m",
	}, "huggingface", "chat"); err != nil {
		t.Fatal(err)
	}
	colID, err := store.CreateCustomColumn("Secret Tag", store.ColumnTypeDropdownText, []string{"alpha-leak", "beta-leak"})
	if err != nil {
		t.Fatalf("create custom column: %v", err)
	}
	if err := store.SetCustomValue("M", colID, "alpha-leak"); err != nil {
		t.Fatalf("set custom value: %v", err)
	}

	if err := store.ExportData(); err != nil {
		t.Fatalf("export: %v", err)
	}
	if paths.FileExists(filepath.Join(paths.DataDir, "personal.json")) {
		t.Error("personal.json must never be written; personal data (including custom columns) lives only in the DB")
	}
	cur, err := os.ReadFile(paths.CuratedJsonFile)
	if err != nil {
		t.Fatalf("curated.json should exist: %v", err)
	}
	curStr := string(cur)
	for _, leaked := range []string{"Secret Tag", "alpha-leak", "beta-leak"} {
		if strings.Contains(curStr, leaked) {
			t.Errorf("curated.json must not contain custom-column data, found %q", leaked)
		}
	}

	// CuratedBytes (what every export path - ExportCuratedJSON, the `export`
	// CLI, ExportMissing - ultimately builds from) must not reference the
	// custom-column tables at all, so this holds regardless of the export
	// entry point.
	b, err := store.CuratedBytes()
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]interface{}
	if err := json.Unmarshal(b, &doc); err != nil {
		t.Fatal(err)
	}
	if _, ok := doc["custom_columns"]; ok {
		t.Error("CuratedBytes output must not carry a custom_columns key")
	}
}
