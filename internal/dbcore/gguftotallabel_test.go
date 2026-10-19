package dbcore

import "testing"

// columnLabelAndUnit reads the two fields v49 changes.
func columnLabelAndUnit(t *testing.T, key string) (label, unit string) {
	t.Helper()
	if err := DB.QueryRow(`SELECT label, unit FROM columns WHERE column_key=?`, key).Scan(&label, &unit); err != nil {
		t.Fatalf("read %s: %v", key, err)
	}
	return label, unit
}

// TestTheGGUFTotalColumnIsRelabelledOnADatabaseCarriedForward opens a schema-48
// database whose column still carries the shipped bytes wording, migrates it,
// and checks the label and unit changed and the cell it holds did not.
func TestTheGGUFTotalColumnIsRelabelledOnADatabaseCarriedForward(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 48)

	id := v45Column(t, "gguf_total_bytes", freshLayout, false, "huggingface", "research")
	if _, err := DB.Exec(`UPDATE columns SET label=?, unit='bytes' WHERE id=?`, oldGGUFTotalLabel, id); err != nil {
		t.Fatal(err)
	}
	putModel(t, "Vendor: Some GGUF")
	putSourcedCell(t, id, "Vendor: Some GGUF", "huggingface", "7115124736", "2026-09-18T00:00:00Z")

	migrateUpTo(t, 49)

	label, unit := columnLabelAndUnit(t, "gguf_total_bytes")
	if label != newGGUFTotalLabel || unit != "" {
		t.Errorf("after v49 the column reads %q / %q, want %q and no unit", label, unit, newGGUFTotalLabel)
	}
	if n := countWhere(t, "column_id=? AND model_name='Vendor: Some GGUF' AND value_text='7115124736'", id); n != 1 {
		t.Errorf("the stored cell = %d row(s), want 1 untouched", n)
	}
	if v, _ := currentSchemaVersion(); v != 49 {
		t.Errorf("user_version = %d, want 49", v)
	}
}

// TestARelabelledGGUFTotalColumnIsLeftAlone: a label the owner chose is theirs.
func TestARelabelledGGUFTotalColumnIsLeftAlone(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 48)

	id := v45Column(t, "gguf_total_bytes", freshLayout, false, "huggingface", "research")
	if _, err := DB.Exec(`UPDATE columns SET label='My GGUF size', unit='bytes' WHERE id=?`, id); err != nil {
		t.Fatal(err)
	}

	migrateUpTo(t, 49)

	if label, unit := columnLabelAndUnit(t, "gguf_total_bytes"); label != "My GGUF size" || unit != "bytes" {
		t.Errorf("the owner's label became %q / %q, want it untouched", label, unit)
	}
}

// TestTheRelabelIsANoOpWhereTheColumnNeverExisted: a database without the
// column migrates cleanly.
func TestTheRelabelIsANoOpWhereTheColumnNeverExisted(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 49)
	if v, _ := currentSchemaVersion(); v != 49 {
		t.Errorf("user_version = %d, want 49", v)
	}
}
