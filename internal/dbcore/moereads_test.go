package dbcore

import "testing"

// seedMoeColumns creates the four columns migration v38 touches, as they would
// already exist on a carried-forward database, and answers their ids: derived,
// mine, huggingface, openrouter.
func seedMoeColumns(t *testing.T) (derived, mine, huggingface, openrouter int64) {
	t.Helper()
	insert := func(key, namespace string, position int) int64 {
		res, err := DB.Exec(`INSERT INTO columns
			(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
			VALUES (?, ?, 'MoE', 'flag', '', '', ?, 0, 1, ?, '')`,
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
	derived = insert("derived.moe", "derived", 0)
	mine = insert("mine.moe", "mine", 1)
	huggingface = insert("huggingface.moe", "huggingface", 2)
	openrouter = insert("openrouter.moe", "openrouter", 3)
	return
}

// TestUntouchedMoeRecipeIsRepairedByMigration covers criterion 2's first half:
// a database still holding the original two-input recipe, in the original
// order, is repaired to read openrouter.moe too, and a model whose only
// stated answer is OpenRouter's now shows it.
func TestUntouchedMoeRecipeIsRepairedByMigration(t *testing.T) {
	setupTestDB(t)
	derived, mine, huggingface, openrouter := seedMoeColumns(t)
	putDerivedRecipe(t, derived, []int64{mine, huggingface}) // the original, untouched recipe

	putModel(t, "Only OpenRouter States It")
	putTextCell(t, openrouter, "Only OpenRouter States It", "yes")

	stampAndMigrateThrough(t, 37, 38)

	got := readDerivedInputs(t, derived)
	if !idsEqual(got, mine, huggingface, openrouter) {
		t.Fatalf("derived.moe inputs after migration = %v, want [mine huggingface openrouter] ids %v",
			got, []int64{mine, huggingface, openrouter})
	}
	if value := readCellText(t, derived, "Only OpenRouter States It"); value != "yes" {
		t.Fatalf("derived.moe for the model = %q, want %q (recomputed from openrouter.moe)", value, "yes")
	}

	var rebuiltAt string
	if err := DB.QueryRow("SELECT rebuilt_at FROM derived_columns WHERE column_id=?", derived).Scan(&rebuiltAt); err != nil {
		t.Fatal(err)
	}
	if rebuiltAt == "" {
		t.Fatal("derived_columns.rebuilt_at was not stamped")
	}
}

// TestReversedMoeRecipeIsLeftAloneByMigration covers criterion 2's second
// half: the same two inputs in the OPPOSITE order is a choice the reader made,
// not the shipped recipe, and is left untouched.
func TestReversedMoeRecipeIsLeftAloneByMigration(t *testing.T) {
	setupTestDB(t)
	derived, mine, huggingface, _ := seedMoeColumns(t)
	putDerivedRecipe(t, derived, []int64{huggingface, mine}) // reversed, on purpose

	stampAndMigrateThrough(t, 37, 38)

	got := readDerivedInputs(t, derived)
	if !idsEqual(got, huggingface, mine) {
		t.Fatalf("derived.moe inputs after migration = %v, want the reader's own [huggingface mine] ids %v",
			got, []int64{huggingface, mine})
	}
}

// TestMoeRecipeWithAThirdInputIsLeftAloneByMigration covers criterion 2's
// second half again: the same two inputs plus a third the reader added is
// also the reader's own, and is left untouched.
func TestMoeRecipeWithAThirdInputIsLeftAloneByMigration(t *testing.T) {
	setupTestDB(t)
	derived, mine, huggingface, openrouter := seedMoeColumns(t)
	putDerivedRecipe(t, derived, []int64{mine, huggingface, openrouter}) // the reader already added it

	stampAndMigrateThrough(t, 37, 38)

	got := readDerivedInputs(t, derived)
	if !idsEqual(got, mine, huggingface, openrouter) {
		t.Fatalf("derived.moe inputs after migration = %v, want the reader's own [mine huggingface openrouter] ids %v",
			got, []int64{mine, huggingface, openrouter})
	}
}
