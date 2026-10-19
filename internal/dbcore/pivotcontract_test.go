package dbcore

import "testing"

// TestContractStripsTheNamespacePrefixAndDropsTheColumn covers
// rebuildColumnsWithoutNamespace: every column's key loses its "namespace."
// prefix, and the namespace column itself is gone, once v41 has run.
func TestContractStripsTheNamespacePrefixAndDropsTheColumn(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	insertColumn(t, "openrouter.context_length", "openrouter")

	migrateThrough(t, 41)

	var key string
	if err := DB.QueryRow(`SELECT column_key FROM columns WHERE column_key='context_length'`).Scan(&key); err != nil {
		t.Fatalf("context_length's key after v41 = error %v, want the bare key with no namespace prefix", err)
	}

	var namespaceStillThere int
	err := DB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('columns') WHERE name='namespace'`).Scan(&namespaceStillThere)
	if err != nil {
		t.Fatal(err)
	}
	if namespaceStillThere != 0 {
		t.Error("columns still has a namespace column after v41, want it dropped")
	}
}

// TestContractRejectsADottedColumnKey proves the CHECK on columns.column_key
// added by v41 actually constrains what can be stored, not only what the
// migration itself writes.
func TestContractRejectsADottedColumnKey(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 41)

	_, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('openrouter.bogus','Bogus','text',?)`, NowStamp())
	if err == nil {
		t.Fatal("expected the dot-free CHECK on column_key to reject a dotted key, got no error")
	}
}

// TestContractRejectsACaseInsensitiveDuplicateLabel proves the unique index
// on label COLLATE NOCASE added by v41 actually constrains what can be
// stored: two columns whose labels differ only by case are refused.
func TestContractRejectsACaseInsensitiveDuplicateLabel(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 41)

	if _, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('first_of_two','Shared Label','text',?)`, NowStamp()); err != nil {
		t.Fatalf("insert the first column: %v", err)
	}
	_, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('second_of_two','shared label','text',?)`, NowStamp())
	if err == nil {
		t.Fatal("expected the case-insensitive unique index on label to reject 'shared label', got no error")
	}
}

// TestContractFailsLoudlyWhenTheCollapseLeftADuplicateLabel covers
// duplicateLabelsCaseInsensitive's own guard: if two columns somehow reach
// v41 sharing a label, the migration refuses to build an index it could not
// enforce, rather than silently dropping one column's data.
//
// This calls rebuildColumnsWithoutNamespace directly, in a transaction of its
// own, rather than through migrate(): a migration failure there ends by
// calling shared.Fatalf, which exits the whole process (see migrate.go) - the
// right behaviour for a real corrupted install, but fatal to the rest of this
// test binary if triggered from inside it.
func TestContractFailsLoudlyWhenTheCollapseLeftADuplicateLabel(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 40) // builds a real v40 schema

	// Two columns sharing a label - the collapse's own guarantee (the real merge
	// groups leave none) does not hold here on purpose, so the guard must catch it itself.
	if _, err := DB.Exec(`INSERT INTO columns (column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES ('openrouter.dup_a','openrouter','Duplicate','text','','',0,1,0,?,'')`, NowStamp()); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO columns (column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES ('huggingface.dup_b','huggingface','duplicate','text','','',0,1,0,?,'')`, NowStamp()); err != nil {
		t.Fatal(err)
	}

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err = rebuildColumnsWithoutNamespace(tx)
	if err == nil {
		t.Fatal("expected rebuildColumnsWithoutNamespace to refuse a case-insensitive duplicate label, got no error")
	}
}

// TestContractCellsSourceIsDeclaredTriggerRefusesAnUndeclaredSource covers
// the first of v41's two write-guard triggers directly: a cell whose source
// has no matching column_sources row for its column is refused, and one that
// does is accepted.
func TestContractCellsSourceIsDeclaredTriggerRefusesAnUndeclaredSource(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 41)

	res, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('guarded','Guarded','text',?)`, NowStamp())
	if err != nil {
		t.Fatal(err)
	}
	col, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	putModel(t, "M")

	_, err = DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','openrouter','x','t')`, col)
	if err == nil {
		t.Fatal("expected cells_source_is_declared to refuse a source with no column_sources row, got no error")
	}

	if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'openrouter',0)`, col); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','openrouter','x','t')`, col); err != nil {
		t.Fatalf("a declared source was refused: %v", err)
	}
}

// TestContractCellsSourceNeverMovesTriggerRefusesReassignment covers v41's
// second write-guard trigger: an UPDATE that would move a stored cell onto a
// different column or a different source is refused - the row is fixed once
// written, and a caller wanting a different answer must delete and re-insert.
func TestContractCellsSourceNeverMovesTriggerRefusesReassignment(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 41)

	res, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('fixed_cell','Fixed cell','text',?)`, NowStamp())
	if err != nil {
		t.Fatal(err)
	}
	col, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'openrouter',0),(?,'hand',1)`, col, col); err != nil {
		t.Fatal(err)
	}
	putModel(t, "M")
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','openrouter','x','t')`, col); err != nil {
		t.Fatal(err)
	}

	_, err = DB.Exec(`UPDATE cells SET source='hand' WHERE column_id=? AND model_name='M' AND source='openrouter'`, col)
	if err == nil {
		t.Fatal("expected cells_source_never_moves to refuse changing a written cell's source, got no error")
	}

	// A field the trigger does not guard is still writable on the same row.
	if _, err := DB.Exec(`UPDATE cells SET value_text='y' WHERE column_id=? AND model_name='M' AND source='openrouter'`, col); err != nil {
		t.Fatalf("updating value_text on the same row was refused: %v", err)
	}
}

// TestContractAnswersViewPicksTheLowestRankedSource covers the answers view:
// with two sources on record for one column and model, the view returns the
// one column_sources ranks first, not whichever was written last.
func TestContractAnswersViewPicksTheLowestRankedSource(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 41)

	res, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('ranked','Ranked','text',?)`, NowStamp())
	if err != nil {
		t.Fatal(err)
	}
	col, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'openrouter',0),(?,'collection',1)`, col, col); err != nil {
		t.Fatal(err)
	}
	putModel(t, "M")
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','collection','low priority','t')`, col); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','openrouter','high priority','t')`, col); err != nil {
		t.Fatal(err)
	}

	var value, source string
	if err := DB.QueryRow(`SELECT value_text, source FROM answers WHERE column_id=? AND model_name='M'`, col).Scan(&value, &source); err != nil {
		t.Fatal(err)
	}
	if value != "high priority" || source != "openrouter" {
		t.Errorf("answers gave (%q, %q), want the rank-0 source's (\"high priority\", \"openrouter\")", value, source)
	}

	var n int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM answers WHERE column_id=? AND model_name='M'`, col).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Errorf("answers returned %d row(s) for one column and model, want exactly 1 (the winner)", n)
	}
}

// TestContractDropsTheDerivedColumnSubsystem covers dropDerivedColumnSubsystem:
// derived_columns and derived_inputs no longer exist once v41 has run, because
// v40 already turned every recipe they held into column_sources rows.
func TestContractDropsTheDerivedColumnSubsystem(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 41)

	for _, table := range []string{"derived_columns", "derived_inputs"} {
		var n int
		err := DB.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&n)
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s still exists after v41, want it dropped", table)
		}
	}
}

// TestContractRenamesSourceRecordsNamespaceToSource covers
// renameSourceRecordsNamespace: source_records is read and written by its
// source column, not its old namespace column, once v41 has run.
func TestContractRenamesSourceRecordsNamespaceToSource(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 41)

	putModel(t, "M")
	if _, err := DB.Exec(`INSERT INTO source_records (model_name, source, raw_json, read_at) VALUES ('M','openrouter','{}','t')`); err != nil {
		t.Fatalf("source_records.source was refused after v41: %v", err)
	}

	var namespaceStillThere int
	err := DB.QueryRow(`SELECT COUNT(*) FROM pragma_table_info('source_records') WHERE name='namespace'`).Scan(&namespaceStillThere)
	if err != nil {
		t.Fatal(err)
	}
	if namespaceStillThere != 0 {
		t.Error("source_records still has a namespace column after v41, want it renamed to source")
	}
}
