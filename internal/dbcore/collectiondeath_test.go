package dbcore

import (
	"fmt"
	"strings"
	"testing"
)

// collectionFixture is a v46 database holding what a catalog scraped before
// 9.15.0 can hold: the three collection-only columns with cells, a price
// column where a scraped price differs from OpenRouter's beside it, a type
// column where the reader typed over the scrape (a research cell after v46),
// and a type where the scrape holds the very text OpenRouter states. It
// answers the column ids by key.
func collectionFixture(t *testing.T) map[string]int64 {
	t.Helper()
	setupTestDB(t)
	migrateUpTo(t, 45)
	ids := map[string]int64{}
	for _, key := range collectionOnlyColumns {
		ids[key] = v45Column(t, key, freshLayout, true, "collection")
	}
	ids["price_prompt"] = v45Column(t, "price_prompt", freshLayout, true, "collection", "openrouter")
	ids["model_type"] = v45Column(t, "model_type", freshLayout, true, "collection", "openrouter", "huggingface")
	putModel(t, "Scraped")
	putModel(t, "Typed")
	putModel(t, "Same")
	putSourcedCell(t, ids["collections"], "Scraped", "collection", `["image-models"]`, "2026-01-01T00:00:00Z")
	putSourcedCell(t, ids["price_display"], "Scraped", "collection", "$0.04 per image", "2026-01-01T00:00:00Z")
	putSourcedCell(t, ids["pricing_url"], "Scraped", "collection", "https://openrouter.ai/x", "2026-01-01T00:00:00Z")
	putSourcedCell(t, ids["price_prompt"], "Scraped", "collection", "0.04", "2026-01-01T00:00:00Z")
	putSourcedCell(t, ids["price_prompt"], "Scraped", "openrouter", "0.05", "2026-09-01T00:00:00Z")
	putSourcedCell(t, ids["model_type"], "Typed", "collection", "image", "2026-01-01T00:00:00Z")
	putSourcedCell(t, ids["model_type"], "Typed", "hand", "video", "2026-03-01T00:00:00Z")
	putSourcedCell(t, ids["model_type"], "Same", "collection", "image", "2026-01-01T00:00:00Z")
	putSourcedCell(t, ids["model_type"], "Same", "openrouter", "image", "2026-09-01T00:00:00Z")
	if _, err := DB.Exec(`INSERT INTO source_records (model_name, source, raw_json, read_at) VALUES ('Scraped','collection','{}',?)`, NowStamp()); err != nil {
		t.Fatal(err)
	}
	migrateUpTo(t, 46)
	return ids
}

// TestCollectionDeathDeletesExactlyTheThreeColumnsCells: the three columns
// and their cells are gone, the drop is exactly those plus the one collision
// and the one duplicate, and nothing else moved.
func TestCollectionDeathDeletesExactlyTheThreeColumnsCells(t *testing.T) {
	collectionFixture(t)
	before, _ := countCells()

	migrateUpTo(t, 47)

	after, _ := countCells()
	// Three cells on the three columns, the scraped type the reader typed over,
	// and the scraped type OpenRouter states word for word: five gone. The
	// scraped price, which differs from OpenRouter's, became research, so it
	// is still a cell.
	if before-after != 5 {
		t.Errorf("cells fell from %d to %d (by %d), want 5", before, after, before-after)
	}
	for _, key := range collectionOnlyColumns {
		var n int
		if err := DB.QueryRow(`SELECT COUNT(*) FROM columns WHERE column_key=?`, key).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s still exists", key)
		}
	}
	if n := countWhere(t, "source='collection'"); n != 0 {
		t.Errorf("%d collection cell(s) remain", n)
	}
	var rows int
	for _, q := range []string{
		`SELECT COUNT(*) FROM sources WHERE name='collection'`,
		`SELECT COUNT(*) FROM column_sources WHERE source='collection'`,
		`SELECT COUNT(*) FROM source_records WHERE source='collection'`,
	} {
		if err := DB.QueryRow(q).Scan(&rows); err != nil {
			t.Fatal(err)
		}
		if rows != 0 {
			t.Errorf("%s = %d, want 0", q, rows)
		}
	}
	if v, _ := currentSchemaVersion(); v != 47 {
		t.Errorf("user_version = %d, want 47", v)
	}
}

// TestCollectionDeathRestampsASurvivingCollectionCellAsResearch: a scraped
// price that DIFFERS from OpenRouter's on a surviving column becomes
// research, dated as it was, and OpenRouter's own cell beside it is
// untouched; a scraped type holding the very text OpenRouter states is
// dropped, because a duplicate of the shown value is not information.
func TestCollectionDeathRestampsASurvivingCollectionCellAsResearch(t *testing.T) {
	ids := collectionFixture(t)

	migrateUpTo(t, 47)

	if n := countWhere(t, "column_id=? AND model_name='Same'", ids["model_type"]); n != 1 {
		t.Errorf("Same's model_type holds %d cell(s), want only OpenRouter's: the duplicate scrape is dropped", n)
	}
	if n := countWhere(t, "column_id=? AND model_name='Same' AND source='openrouter' AND value_text='image'", ids["model_type"]); n != 1 {
		t.Errorf("OpenRouter's type on Same was touched")
	}

	var value, writtenAt string
	if err := DB.QueryRow(`SELECT value_text, written_at FROM cells WHERE column_id=? AND model_name='Scraped' AND source='research'`, ids["price_prompt"]).Scan(&value, &writtenAt); err != nil {
		t.Fatalf("the scraped price did not become research: %v", err)
	}
	if value != "0.04" || writtenAt != "2026-01-01T00:00:00Z" {
		t.Errorf("research cell = %q at %q, want 0.04 at 2026-01-01T00:00:00Z", value, writtenAt)
	}
	if n := countWhere(t, "column_id=? AND source='openrouter' AND value_text='0.05'", ids["price_prompt"]); n != 1 {
		t.Errorf("OpenRouter's price = %d row(s), want 1 untouched", n)
	}
	if got := fmt.Sprint(sourcesByRank(t, ids["price_prompt"])); got != "[openrouter research]" {
		t.Errorf("price_prompt declares %s, want [openrouter research]", got)
	}
}

// TestCollectionDeathDropsATypedOverCollectionCellAndCountsIt: where the
// reader typed over a scrape, what they typed (research since v46) stays and
// the scraped cell goes, counted as the one collision.
func TestCollectionDeathDropsATypedOverCollectionCellAndCountsIt(t *testing.T) {
	ids := collectionFixture(t)

	migrateUpTo(t, 47)

	rows, err := DB.Query(`SELECT source, value_text FROM cells WHERE column_id=? AND model_name='Typed'`, ids["model_type"])
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var s, v string
		if err := rows.Scan(&s, &v); err != nil {
			t.Fatal(err)
		}
		got = append(got, s+"="+v)
	}
	if strings.Join(got, ",") != "research=video" {
		t.Errorf("Typed's model_type cells = %v, want only research=video", got)
	}
}

// TestCollectionDeathRefusesAnUnexpectedDrop drives the guard: a cell that
// goes for a reason this migration does not account for aborts it, and the
// transaction the test owns rolls back with nothing lost. The extra loss is
// staged by a trigger that deletes an unrelated cell when the collection
// source row goes, which is exactly the kind of surprise the count exists to
// catch.
func TestCollectionDeathRefusesAnUnexpectedDrop(t *testing.T) {
	ids := collectionFixture(t)
	putModel(t, "Bystander")
	putSourcedCell(t, ids["price_prompt"], "Bystander", "openrouter", "1", "2026-09-01T00:00:00Z")
	if _, err := DB.Exec(`CREATE TRIGGER surprise AFTER DELETE ON sources BEGIN
		DELETE FROM cells WHERE model_name='Bystander'; END`); err != nil {
		t.Fatal(err)
	}
	before, _ := countCells()

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	err = theCollectionSourceIsGone(tx)
	if err == nil {
		t.Fatal("expected the migration to refuse a drop it cannot account for, got no error")
	}
	if !strings.Contains(err.Error(), "the cell count fell by") {
		t.Errorf("refusal = %q, want it to say it is refusing to commit", err)
	}
	tx.Rollback()

	if after, _ := countCells(); after != before {
		t.Errorf("cells after the refused, rolled-back migration = %d, want %d (nothing lost)", after, before)
	}
}
