package dbcore

import (
	"fmt"
	"testing"
)

// seedTypeColumns creates the five columns migration v37 touches, as they
// would already exist on a carried-forward database, and answers their ids:
// derived, mine, collection, openrouter, huggingface.
func seedTypeColumns(t *testing.T) (derived, mine, collection, openrouter, huggingface int64) {
	t.Helper()
	insert := func(key, namespace string, position int) int64 {
		res, err := DB.Exec(`INSERT INTO columns
			(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
			VALUES (?, ?, 'Type', 'text', '', '', ?, 1, 1, ?, '')`,
			key, namespace, position, NowStamp())
		if err != nil {
			t.Fatalf("insert column %s: %v", key, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	// derived carries the lowest position, matching every real installed
	// database: a computed column is always the survivor once columns collapse onto one
	// per attribute (migration v40), never an input merged into something else.
	derived = insert("derived.model_type", "derived", 0)
	mine = insert("mine.model_type", "mine", 1)
	collection = insert("collection.model_type", "collection", 2)
	openrouter = insert("openrouter.model_type", "openrouter", 3)
	huggingface = insert("huggingface.model_type", "huggingface", 4)
	return
}

// putDerivedRecipe stores derived.model_type's recipe as first_non_empty over
// the given inputs, in order - mirroring what store.DefineDerivedColumn would
// have written, without depending on the store package.
func putDerivedRecipe(t *testing.T, derivedID int64, inputIDs []int64) {
	t.Helper()
	if _, err := DB.Exec("INSERT INTO derived_columns (column_id, transform, params, rebuilt_at) VALUES (?,?,?,?)",
		derivedID, "first_non_empty", "", NowStamp()); err != nil {
		t.Fatal(err)
	}
	for position, id := range inputIDs {
		if _, err := DB.Exec("INSERT INTO derived_inputs (column_id, position, input_column_id) VALUES (?,?,?)",
			derivedID, position, id); err != nil {
			t.Fatal(err)
		}
	}
}

func putModel(t *testing.T, name string) {
	t.Helper()
	if _, err := DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES (?,?,?)`,
		name, "openrouter", NowStamp()); err != nil {
		t.Fatal(err)
	}
}

func putTextCell(t *testing.T, columnID int64, modelName, value string) {
	t.Helper()
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, value_text, value_number, written_at)
		VALUES (?,?,?,NULL,?)`, columnID, modelName, value, NowStamp()); err != nil {
		t.Fatal(err)
	}
}

// readDerivedInputs answers one computed column's stored input ids, in order.
func readDerivedInputs(t *testing.T, derivedID int64) []int64 {
	t.Helper()
	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	ids, err := orderedDerivedInputs(tx, derivedID)
	if err != nil {
		t.Fatal(err)
	}
	return ids
}

func readCellText(t *testing.T, columnID int64, modelName string) string {
	t.Helper()
	var value string
	err := DB.QueryRow("SELECT value_text FROM cells WHERE column_id=? AND model_name=?", columnID, modelName).Scan(&value)
	if err != nil {
		return ""
	}
	return value
}

func idsEqual(got []int64, want ...int64) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// stampAndMigrateThrough stamps user_version at from, then runs only the
// registered migrations up to and including through. Used by a test that
// asserts on state a migration AFTER through - v40's collapse, v41's contract
// - goes on to change or remove, so the test can see what "through" itself
// produced rather than what the full chain leaves behind.
func stampAndMigrateThrough(t *testing.T, from, through int) {
	t.Helper()
	if _, err := DB.Exec(fmt.Sprintf("PRAGMA user_version=%d", from)); err != nil {
		t.Fatal(err)
	}
	migrateThrough(t, through)
}

// TestUntouchedTypeRecipeIsRepairedByMigration covers criterion 2's first
// half: a database still holding the ORIGINAL two-input recipe is repaired to
// read all four sources, and a model whose only stated type is OpenRouter's
// now shows it (criterion 3 and 6, at the migration layer).
func TestUntouchedTypeRecipeIsRepairedByMigration(t *testing.T) {
	setupTestDB(t)
	derived, mine, collection, openrouter, huggingface := seedTypeColumns(t)
	putDerivedRecipe(t, derived, []int64{mine, collection}) // the original, untouched recipe

	putModel(t, "Only OpenRouter States It")
	putTextCell(t, openrouter, "Only OpenRouter States It", "image")

	stampAndMigrateThrough(t, 36, 37)

	got := readDerivedInputs(t, derived)
	if !idsEqual(got, mine, collection, openrouter, huggingface) {
		t.Fatalf("derived.model_type inputs after migration = %v, want [mine collection openrouter huggingface] ids %v",
			got, []int64{mine, collection, openrouter, huggingface})
	}
	if value := readCellText(t, derived, "Only OpenRouter States It"); value != "image" {
		t.Fatalf("derived.model_type for the model = %q, want %q (recomputed from openrouter.model_type)", value, "image")
	}

	var rebuiltAt string
	if err := DB.QueryRow("SELECT rebuilt_at FROM derived_columns WHERE column_id=?", derived).Scan(&rebuiltAt); err != nil {
		t.Fatal(err)
	}
	if rebuiltAt == "" {
		t.Fatal("derived_columns.rebuilt_at was not stamped")
	}
}

// TestChangedTypeRecipeIsLeftAloneByMigration covers criterion 2's second
// half: a recipe the reader has changed - here, down to one input - is the
// reader's own and is never touched, whatever it says.
func TestChangedTypeRecipeIsLeftAloneByMigration(t *testing.T) {
	setupTestDB(t)
	derived, mine, _, _, _ := seedTypeColumns(t)
	putDerivedRecipe(t, derived, []int64{mine}) // the reader trimmed it to one input, on purpose

	putModel(t, "Reader Owns This Recipe")
	putTextCell(t, mine, "Reader Owns This Recipe", "chat")

	stampAndMigrateThrough(t, 36, 37)

	got := readDerivedInputs(t, derived)
	if !idsEqual(got, mine) {
		t.Fatalf("derived.model_type inputs after migration = %v, want the reader's own [mine] id %v", got, []int64{mine})
	}
}
