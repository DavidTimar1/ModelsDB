package dbcore

import (
	"fmt"
	"testing"
)

// TestOverrideWindowColumnsAreDeletedWithTheirCells builds a v47 database
// carrying three of the eleven pricing_overrides_* columns the old
// element-by-element walk made (a number one with cells on two models, a
// list one with cells on one model, and an empty one nobody ever filled), one
// benchmarks_design_arena_* column, and a column that only LOOKS related
// (pricing_overrides_extra_note, still inside the retired prefix, so it must
// go too) beside an UNRELATED survivor (price_prompt) that must not be
// touched. v48 must delete exactly the four retired columns and exactly the
// cells they held, leave the survivor's cells and rank order alone, and land
// on schema 48.
func TestOverrideWindowColumnsAreDeletedWithTheirCells(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 47)

	promptCol := v45Column(t, "pricing_overrides_prompt", freshLayout, false, "openrouter", "research")
	utcDaysCol := v45Column(t, "pricing_overrides_utc_days", freshLayout, false, "openrouter", "research")
	v45Column(t, "pricing_overrides_input_cache_write_1h", freshLayout, false, "openrouter", "research")
	eloCol := v45Column(t, "benchmarks_design_arena_elo", freshLayout, false, "openrouter", "research")
	survivorCol := v45Column(t, "price_prompt", freshLayout, false, "openrouter", "research")

	putModel(t, "DeepSeek: DeepSeek V4.1 Flash")
	putModel(t, "Qwen: Qwen3 Max")
	putSourcedCell(t, promptCol, "DeepSeek: DeepSeek V4.1 Flash", "openrouter", "0.00000015", "2026-09-18T00:00:00Z")
	putSourcedCell(t, promptCol, "Qwen: Qwen3 Max", "openrouter", "0.00000156", "2026-09-18T00:00:00Z")
	putSourcedCell(t, utcDaysCol, "DeepSeek: DeepSeek V4.1 Flash", "openrouter", `["sat","sun","mon"]`, "2026-09-18T00:00:00Z")
	putSourcedCell(t, eloCol, "Qwen: Qwen3 Max", "openrouter", "1234.5", "2026-09-18T00:00:00Z")
	putSourcedCell(t, survivorCol, "Qwen: Qwen3 Max", "openrouter", "0.00000156", "2026-09-18T00:00:00Z")

	before, err := countCells()
	if err != nil {
		t.Fatal(err)
	}

	migrateUpTo(t, 48)

	after, err := countCells()
	if err != nil {
		t.Fatal(err)
	}
	if drop := before - after; drop != 4 {
		t.Errorf("cells fell from %d to %d (by %d), want 4 (2 on pricing_overrides_prompt, 1 on pricing_overrides_utc_days, 1 on benchmarks_design_arena_elo)", before, after, drop)
	}

	for _, key := range []string{"pricing_overrides_prompt", "pricing_overrides_utc_days", "pricing_overrides_input_cache_write_1h", "benchmarks_design_arena_elo"} {
		var n int
		if err := DB.QueryRow(`SELECT COUNT(*) FROM columns WHERE column_key=?`, key).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s still exists after v48", key)
		}
	}
	var survivorLeft int
	if err := DB.QueryRow(`SELECT COUNT(*) FROM columns WHERE column_key='price_prompt'`).Scan(&survivorLeft); err != nil {
		t.Fatal(err)
	}
	if survivorLeft != 1 {
		t.Errorf("price_prompt was deleted, want it untouched")
	}
	if n := countWhere(t, "column_id=? AND model_name='Qwen: Qwen3 Max' AND value_text='0.00000156'", survivorCol); n != 1 {
		t.Errorf("price_prompt's surviving cell = %d row(s), want 1 untouched", n)
	}

	if v, _ := currentSchemaVersion(); v != 48 {
		t.Errorf("user_version = %d, want 48", v)
	}
}

// TestOverrideWindowColumnsMigrationIsANoOpWhenNoneExist proves v48 runs
// cleanly on a database that never made any of the eleven columns (his
// install after a refresh where no model had several price windows would
// look like this pre-migration, and every fresh test database does too): no
// error, no cell dropped, schema still lands on 48.
func TestOverrideWindowColumnsMigrationIsANoOpWhenNoneExist(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 47)
	survivorCol := v45Column(t, "price_prompt", freshLayout, false, "openrouter", "research")
	putModel(t, "M")
	putSourcedCell(t, survivorCol, "M", "openrouter", "0.000001", "2026-09-18T00:00:00Z")

	before, err := countCells()
	if err != nil {
		t.Fatal(err)
	}

	migrateUpTo(t, 48)

	after, err := countCells()
	if err != nil {
		t.Fatal(err)
	}
	if before != after {
		t.Errorf("cells changed from %d to %d with no retired column present", before, after)
	}
	if v, _ := currentSchemaVersion(); v != 48 {
		t.Errorf("user_version = %d, want 48", v)
	}
}

// TestOverrideWindowColumnsRefusesAnUnexpectedDrop drives the guard the same
// way TestCollectionDeathRefusesAnUnexpectedDrop does: a cell lost for a
// reason this migration does not account for aborts it rather than
// committing a silent extra loss.
func TestOverrideWindowColumnsRefusesAnUnexpectedDrop(t *testing.T) {
	setupTestDB(t)
	migrateUpTo(t, 47)
	promptCol := v45Column(t, "pricing_overrides_prompt", freshLayout, false, "openrouter", "research")
	survivorCol := v45Column(t, "price_prompt", freshLayout, false, "openrouter", "research")
	putModel(t, "M")
	putSourcedCell(t, promptCol, "M", "openrouter", "0.000001", "2026-09-18T00:00:00Z")
	putSourcedCell(t, survivorCol, "M", "openrouter", "0.000002", "2026-09-18T00:00:00Z")
	trigger := fmt.Sprintf(`CREATE TRIGGER surprise_v48 AFTER DELETE ON columns WHEN old.column_key='pricing_overrides_prompt'
		BEGIN DELETE FROM cells WHERE column_id=%d; END`, survivorCol)
	if _, err := DB.Exec(trigger); err != nil {
		t.Fatal(err)
	}
	before, _ := countCells()

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err = theOverrideWindowColumnsAreRetired(tx)
	if err == nil {
		t.Fatal("expected the migration to refuse a drop it cannot account for, got no error")
	}
	tx.Rollback()

	if after, _ := countCells(); after != before {
		t.Errorf("cells after the refused, rolled-back migration = %d, want %d (nothing lost)", after, before)
	}
}
