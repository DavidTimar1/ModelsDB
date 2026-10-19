package dbcore

import "testing"

// TestRuleDeathDeletesARuleCellThatHasANonRuleSibling covers the ordinary
// case: a rule cell the collapse (v40) carried onto a column that also holds
// a cell from a real source is gone once v42 has run, and the sibling cell is
// untouched.
func TestRuleDeathDeletesARuleCellThatHasANonRuleSibling(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	// openrouter listed first survives the collapse at the lower position;
	// derived is absorbed onto it, carrying its cell's source (backfilled to
	// 'rule' by v39) along for the ride.
	cols := seedMergeGroup(t, "model_type", "openrouter", "derived")
	putModel(t, "M")
	putTextCell(t, cols["openrouter"], "M", "chat")
	putTextCell(t, cols["derived"], "M", "chat")

	migrateThrough(t, 42)

	var ruleLeft int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM cells WHERE source='rule'`).Scan(&ruleLeft); err != nil {
		t.Fatal(err)
	}
	if ruleLeft != 0 {
		t.Errorf("rule cells remaining after v42 = %d, want 0", ruleLeft)
	}

	var siblingLeft int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM cells WHERE source='openrouter' AND model_name='M'`).Scan(&siblingLeft); err != nil {
		t.Fatal(err)
	}
	if siblingLeft != 1 {
		t.Errorf("the non-rule sibling cell = %d row(s) after v42, want 1 (untouched)", siblingLeft)
	}
}

// TestRuleDeathAbortsWhenARuleCellHasNoNonRuleSibling covers the guard: a
// rule cell that is the ONLY cell on record for its column and model is the
// answers view's served value today, so deleting it would take that answer
// away rather than merely stop computing on read what the view already
// computes. The migration must refuse rather than delete it.
//
// This calls theDeadRuleCellsAreDeleted directly, inside a transaction this
// test owns and rolls back, rather than through migrate(): a real failure
// there ends in shared.Fatalf, which exits the whole process (see
// pivotcontract_test.go's TestContractFailsLoudlyWhenTheCollapseLeftADuplicateLabel
// for the same pattern).
func TestRuleDeathAbortsWhenARuleCellHasNoNonRuleSibling(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	derivedCol := insertColumn(t, "derived.lonely_attr", "derived")
	putModel(t, "M")
	putTextCell(t, derivedCol, "M", "x")

	migrateThrough(t, 41) // carries the rule cell forward without deleting it

	var before int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM cells WHERE source='rule'`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	if before != 1 {
		t.Fatalf("setup: rule cells before v42 = %d, want 1", before)
	}

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err = theDeadRuleCellsAreDeleted(tx)
	if err == nil {
		t.Fatal("expected theDeadRuleCellsAreDeleted to refuse an orphaned rule cell, got no error")
	}
	tx.Rollback()

	var after int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM cells WHERE source='rule'`).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != before {
		t.Errorf("rule cells after the refused, rolled-back migration = %d, want %d (nothing lost)", after, before)
	}
}
