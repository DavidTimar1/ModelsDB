package dbcore

import "testing"

// insertUnitColumn adds one number column with the unit and position given, the
// shape a schema 33 to 36 database holds for a huggingface column and for the
// reader's empty twin of it.
func insertUnitColumn(t *testing.T, namespace, attribute, unit string, position int) int64 {
	t.Helper()
	res, err := DB.Exec(`INSERT INTO columns
		(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES (?, ?, ?, 'number', ?, '', ?, 0, 0, ?, '')`,
		namespace+"."+attribute, namespace, namespace+"."+attribute, unit, position, NowStamp())
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// insertTypedColumn adds one column of the given type and unit with its own label.
func insertTypedColumn(t *testing.T, namespace, attribute, label, valueType, unit string, position int) int64 {
	t.Helper()
	res, err := DB.Exec(`INSERT INTO columns
		(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES (?, ?, ?, ?, ?, '', ?, 0, 0, ?, '')`,
		namespace+"."+attribute, namespace, label, valueType, unit, position, NowStamp())
	if err != nil {
		t.Fatal(err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestCollapseAbsorbsAnEmptyMineTwinWhoseUnitIsBlank reproduces the databases
// from schema 33 to 36: mine.parameters has no unit where huggingface.parameters
// says billions, and holds no cell. v40 must merge them, and the huggingface
// figure must reach the survivor untouched.
func TestCollapseAbsorbsAnEmptyMineTwinWhoseUnitIsBlank(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	survivor := insertUnitColumn(t, "huggingface", "parameters", "billions", 57)
	twin := insertUnitColumn(t, "mine", "parameters", "", 1000)
	putModel(t, "M")
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, value_text, value_number, written_at)
		VALUES (?, 'M', '7', 7, ?)`, survivor, NowStamp()); err != nil {
		t.Fatal(err)
	}

	migrateThrough(t, 40)

	var twins int
	if err := DB.QueryRow("SELECT COUNT(*) FROM columns WHERE id=?", twin).Scan(&twins); err != nil {
		t.Fatal(err)
	}
	if twins != 0 {
		t.Errorf("the empty mine twin still exists after the collapse")
	}
	var value, unit string
	if err := DB.QueryRow("SELECT value_text FROM cells WHERE column_id=? AND model_name='M'", survivor).Scan(&value); err != nil {
		t.Fatalf("the survivor lost its cell: %v", err)
	}
	if value != "7" {
		t.Errorf("survivor's value = %q, want \"7\"", value)
	}
	if err := DB.QueryRow("SELECT unit FROM columns WHERE id=?", survivor).Scan(&unit); err != nil {
		t.Fatal(err)
	}
	if unit != "billions" {
		t.Errorf("survivor's unit = %q, want billions", unit)
	}
}

// TestCollapseKeepsAnOwnColumnThatHoldsValuesTheShippedTwinCannotTake covers the
// three names a v1.30.1 reader can have made as custom columns. Each holds a
// typed value and disagrees with the shipped column of that name on type or
// unit. v40 must complete, the shipped column must keep its own figures, and
// the reader's column must survive with every value under a name that says
// it is theirs.
func TestCollapseKeepsAnOwnColumnThatHoldsValuesTheShippedTwinCannotTake(t *testing.T) {
	cases := []struct {
		attribute, label, theirType, theirValue string
		shippedType, shippedUnit, shippedValue  string
	}{
		{"parameters", "Parameters (B)", "text", "about seven", "number", "billions", "7"},
		{"author", "Author", "choice", "Acme", "text", "", "acme-labs"},
		{"weights_size_gb", "Weights size (GB)", "text", "a lot", "number", "GB", "14"},
	}
	for _, c := range cases {
		t.Run(c.attribute, func(t *testing.T) {
			setupTestDB(t)
			stampAtV38(t)
			shipped := insertTypedColumn(t, "huggingface", c.attribute, "huggingface."+c.attribute, c.shippedType, c.shippedUnit, 57)
			own := insertTypedColumn(t, "mine", c.attribute, c.label, c.theirType, "", 1000)
			putModel(t, "M")
			if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, value_text, value_number, written_at)
				VALUES (?, 'M', ?, NULL, ?)`, shipped, c.shippedValue, NowStamp()); err != nil {
				t.Fatal(err)
			}
			if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, value_text, value_number, written_at)
				VALUES (?, 'M', ?, NULL, ?)`, own, c.theirValue, NowStamp()); err != nil {
				t.Fatal(err)
			}

			migrateThrough(t, 41)

			var key, label, valueType string
			if err := DB.QueryRow("SELECT column_key, label, value_type FROM columns WHERE id=?", own).Scan(&key, &label, &valueType); err != nil {
				t.Fatalf("the reader's column is gone: %v", err)
			}
			if key == c.attribute {
				t.Errorf("the reader's column kept the shipped key %q", key)
			}
			if label == c.label {
				t.Errorf("the reader's column kept the label %q, want one that says it is theirs", label)
			}
			if valueType != c.theirType {
				t.Errorf("the reader's column type = %q, want %q", valueType, c.theirType)
			}
			var got string
			if err := DB.QueryRow("SELECT value_text FROM cells WHERE column_id=? AND model_name='M' AND source='hand'", own).Scan(&got); err != nil {
				t.Fatalf("the reader's value is gone: %v", err)
			}
			if got != c.theirValue {
				t.Errorf("the reader's value = %q, want %q", got, c.theirValue)
			}
			var sourced int
			if err := DB.QueryRow("SELECT COUNT(*) FROM column_sources WHERE column_id=? AND source='hand'", own).Scan(&sourced); err != nil {
				t.Fatal(err)
			}
			if sourced != 1 {
				t.Errorf("the reader's column has %d hand source row(s), want 1", sourced)
			}
			var shippedKey, shippedGot string
			if err := DB.QueryRow("SELECT column_key FROM columns WHERE id=?", shipped).Scan(&shippedKey); err != nil {
				t.Fatal(err)
			}
			if shippedKey != c.attribute {
				t.Errorf("the shipped column key = %q, want %q", shippedKey, c.attribute)
			}
			if err := DB.QueryRow("SELECT value_text FROM cells WHERE column_id=? AND model_name='M'", shipped).Scan(&shippedGot); err != nil {
				t.Fatal(err)
			}
			if shippedGot != c.shippedValue {
				t.Errorf("the shipped column's value = %q, want %q", shippedGot, c.shippedValue)
			}
		})
	}
}

// TestCollapseGivesTheOwnColumnAFreeNameWhenTheFirstChoiceIsTaken pins that the
// detached column never collides with a column the reader already named the
// same way.
func TestCollapseGivesTheOwnColumnAFreeNameWhenTheFirstChoiceIsTaken(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	insertTypedColumn(t, "huggingface", "parameters", "huggingface.parameters", "number", "billions", 57)
	own := insertTypedColumn(t, "mine", "parameters", "Parameters (B)", "text", "", 1000)
	insertTypedColumn(t, "mine", "parameters_mine", "Parameters (B) (mine)", "text", "", 1001)
	putModel(t, "M")
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, value_text, written_at)
		VALUES (?, 'M', 'x', ?)`, own, NowStamp()); err != nil {
		t.Fatal(err)
	}

	migrateThrough(t, 41)

	var key, label string
	if err := DB.QueryRow("SELECT column_key, label FROM columns WHERE id=?", own).Scan(&key, &label); err != nil {
		t.Fatal(err)
	}
	if key != "parameters_mine_2" || label != "Parameters (B) (mine 2)" {
		t.Errorf("renamed to key %q, label %q, want parameters_mine_2 and \"Parameters (B) (mine 2)\"", key, label)
	}
}
