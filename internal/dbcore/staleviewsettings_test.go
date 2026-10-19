package dbcore

import "testing"

// TestStaleViewSettingsRewritesDottedKeysAndDropsAWithdrawnOne covers v42's
// four settings end to end: a dotted key naming a column that still exists is
// rewritten to its bare form, and a dotted key naming a column that no longer
// exists (merged away or renamed) is dropped rather than left dangling.
func TestStaleViewSettingsRewritesDottedKeysAndDropsAWithdrawnOne(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 42) // gets columns and app_settings into their v41+ shape

	if _, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('context_length','Context length','number',?)`, NowStamp()); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO app_settings (key, value) VALUES
		('view.v1.col_hidden', '["openrouter.context_length","openrouter.withdrawn_attr"]')`); err != nil {
		t.Fatal(err)
	}

	// theStaleViewSettingsAreRewritten (v43) is called directly, inside a
	// transaction this test owns, rather than through another migrateThrough:
	// that helper narrows the package-level migrations slice for the rest of
	// the test, and a second narrowing call cannot widen it back out to reach
	// a version past the first call's target.
	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := theStaleViewSettingsAreRewritten(tx); err != nil {
		tx.Rollback()
		t.Fatalf("theStaleViewSettingsAreRewritten: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	var value string
	if err := DB.QueryRow(`SELECT value FROM app_settings WHERE key='view.v1.col_hidden'`).Scan(&value); err != nil {
		t.Fatal(err)
	}
	if value != `["context_length"]` {
		t.Errorf("view.v1.col_hidden after v43 = %q, want [\"context_length\"] (bare, withdrawn column dropped)", value)
	}
}

// TestStaleViewSettingsCollapsesTwoOldKeysOntoOneBareKey covers criterion 2:
// two dotted keys that strip down to the same bare column key are
// deduplicated, keeping the FIRST occurrence, since these lists are ordered
// (col_order and order both carry meaning in their position).
func TestStaleViewSettingsCollapsesTwoOldKeysOntoOneBareKey(t *testing.T) {
	validColumns := map[string]bool{"model_type": true}
	raw := `["openrouter.model_type","mine.model_type"]`

	got, changed, err := rewriteFlatKeyList(raw, validColumns)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected rewriteFlatKeyList to report a change, got none")
	}
	if got != `["model_type"]` {
		t.Errorf("rewriteFlatKeyList(%q) = %q, want [\"model_type\"] (deduplicated, first occurrence kept)", raw, got)
	}
}

// TestStaleViewSettingsLeavesAMalformedBlobUntouched covers criterion 4: a
// value that is not the JSON shape this migration expects (here, a JSON
// object where an array of strings belongs) is left byte-for-byte as it was,
// and reading it must not error the migration.
func TestStaleViewSettingsLeavesAMalformedBlobUntouched(t *testing.T) {
	validColumns := map[string]bool{"context_length": true}
	raw := `{"not":"a list"}`

	got, changed, err := rewriteFlatKeyList(raw, validColumns)
	if err != nil {
		t.Fatalf("rewriteFlatKeyList on a malformed blob returned an error, want it tolerated: %v", err)
	}
	if changed {
		t.Error("rewriteFlatKeyList reported a change on a malformed blob, want it left alone")
	}
	if got != raw {
		t.Errorf("rewriteFlatKeyList(%q) = %q, want the input returned unchanged", raw, got)
	}
}

// TestStaleViewSettingsOrderPairsKeepTheirDirection covers the "order"
// setting's distinct shape: rewriting the key element of a [key, direction]
// pair must not disturb the direction element traveling with it.
func TestStaleViewSettingsOrderPairsKeepTheirDirection(t *testing.T) {
	validColumns := map[string]bool{"context_length": true}
	raw := `[["openrouter.context_length","desc"],["_name","asc"]]`

	got, changed, err := rewriteOrderPairs(raw, validColumns)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected rewriteOrderPairs to report a change, got none")
	}
	want := `[["context_length","desc"],["_name","asc"]]`
	if got != want {
		t.Errorf("rewriteOrderPairs(%q) = %q, want %q (direction preserved, _name untouched)", raw, got, want)
	}
}

// TestStaleViewSettingsAnAbsentBlobIsNoOp covers criterion 4's other edge: a
// view id with no col_known row at all is simply not among the candidates
// the migration reads, so running it does nothing and errors nothing.
func TestStaleViewSettingsAnAbsentBlobIsNoOp(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 43) // no app_settings rows exist at all; must not error
}
