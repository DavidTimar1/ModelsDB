package dbcore

import "testing"

// insertColumn is a one-line helper for creating a column in one namespace,
// with the fields the tests here need and nothing derived.model_type's or
// derived.moe's own test files did not already need.
func insertColumn(t *testing.T, key, namespace string) int64 {
	t.Helper()
	// label is the key itself, not a fixed literal: v41 adds a unique index on
	// label COLLATE NOCASE, and column_key is already unique across every call
	// this helper makes, so reusing it here keeps that guarantee for free.
	res, err := DB.Exec(`INSERT INTO columns
		(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES (?, ?, ?, 'text', '', '', 0, 1, 0, ?, '')`,
		key, namespace, key, NowStamp())
	if err != nil {
		t.Fatalf("insert column %s: %v", key, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

// TestTheChainSeedsEverySource covers the seeding half of v39 and what v46
// and v47 do to it: a fresh database ends up with exactly the sources the app
// knows, each carrying the kind it was assigned, and collection is not one.
func TestTheChainSeedsEverySource(t *testing.T) {
	setupTestDB(t)
	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	rows, err := DB.Query("SELECT name, kind FROM sources ORDER BY position")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	want := map[string]string{
		"openrouter":  "fetched",
		"huggingface": "fetched",
		"research":    "research",
		"hand":        "hand",
		"app":         "static",
	}
	got := map[string]string{}
	for rows.Next() {
		var name, kind string
		if err := rows.Scan(&name, &kind); err != nil {
			t.Fatal(err)
		}
		got[name] = kind
	}
	if len(got) != len(want) {
		t.Fatalf("sources after migration = %v, want %v", got, want)
	}
	for name, kind := range want {
		if got[name] != kind {
			t.Errorf("source %s has kind %q, want %q", name, got[name], kind)
		}
	}
}

// TestPivotBackfillsCellSourceFromNamespace covers the backfill half of v39:
// a cell in an ordinary namespace ends up stamped with that namespace's name
// as its source, mine becomes hand, and derived becomes rule.
func TestPivotBackfillsCellSourceFromNamespace(t *testing.T) {
	setupTestDB(t)
	// Stamp the database as already at v38, WITHOUT migrating: setSchemaVersionThenMigrate
	// would run v39 immediately, before there is anything in cells to backfill.
	if _, err := DB.Exec("PRAGMA user_version=38"); err != nil {
		t.Fatal(err)
	}

	openrouterCol := insertColumn(t, "openrouter.context_length", "openrouter")
	mineCol := insertColumn(t, "mine.rating_of_mine", "mine")
	derivedCol := insertColumn(t, "derived.model_type", "derived")
	putModel(t, "M")
	putTextCell(t, openrouterCol, "M", "128000")
	putTextCell(t, mineCol, "M", "5")
	putTextCell(t, derivedCol, "M", "chat")

	// migrateThrough(t, 39), not a full migrate(): this test covers v39's own
	// backfill, and the derived cell it plants has no sibling from a real
	// source, which v42 (ruledeath.go) refuses to delete for exactly that
	// reason - a full migrate() run would abort here rather than reach v43.
	migrateThrough(t, 39)

	cases := []struct {
		columnID int64
		want     string
	}{
		{openrouterCol, "openrouter"},
		{mineCol, "hand"},
		{derivedCol, "rule"},
	}
	for _, c := range cases {
		var source string
		if err := DB.QueryRow("SELECT source FROM cells WHERE column_id=? AND model_name='M'", c.columnID).Scan(&source); err != nil {
			t.Fatalf("read source for column %d: %v", c.columnID, err)
		}
		if source != c.want {
			t.Errorf("cell source for column %d = %q, want %q", c.columnID, source, c.want)
		}
	}
}

// TestPivotColumnsDefaultToFirstStatedResolve covers columns.resolve: a
// column that already existed before v39, and one created after it, both
// read back as first_stated.
func TestPivotColumnsDefaultToFirstStatedResolve(t *testing.T) {
	setupTestDB(t)
	// Stamp the database as already at v38, WITHOUT migrating, so this column is
	// in place before v39's ALTER TABLE runs.
	if _, err := DB.Exec("PRAGMA user_version=38"); err != nil {
		t.Fatal(err)
	}
	existing := insertColumn(t, "openrouter.context_length", "openrouter")

	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	var resolve string
	if err := DB.QueryRow("SELECT resolve FROM columns WHERE id=?", existing).Scan(&resolve); err != nil {
		t.Fatal(err)
	}
	if resolve != "first_stated" {
		t.Errorf("resolve for a column that pre-dates v39 = %q, want first_stated", resolve)
	}

	// This column is created AFTER migrate() has already reached v41, where
	// columns has no namespace column and a dotted key fails its own CHECK -
	// insertColumn assumes the pre-v41 shape, so this one is a raw insert
	// matching the current schema instead.
	res, err := DB.Exec(`INSERT INTO columns (column_key, label, value_type, created_at)
		VALUES ('after_migration','After migration','text',?)`, NowStamp())
	if err != nil {
		t.Fatal(err)
	}
	fresh, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if err := DB.QueryRow("SELECT resolve FROM columns WHERE id=?", fresh).Scan(&resolve); err != nil {
		t.Fatal(err)
	}
	if resolve != "first_stated" {
		t.Errorf("resolve for a column created after v39 = %q, want first_stated", resolve)
	}
}

// TestPivotRecordsColumnSourcesFromDerivedRecipe covers
// backfillColumnSourcesFromDerivedInputs: a computed column's stored recipe
// becomes a column_sources row per distinct source, ranked by the order the
// recipe names them.
func TestPivotRecordsColumnSourcesFromDerivedRecipe(t *testing.T) {
	setupTestDB(t)
	// Stamp the database as already at v36, WITHOUT migrating, so v37 still runs
	// and repairs the untouched recipe this test writes to four inputs before
	// v39 reads it - setSchemaVersionThenMigrate would run v37-v39 immediately
	// instead, before there is a recipe in place for v37 to repair.
	if _, err := DB.Exec("PRAGMA user_version=36"); err != nil {
		t.Fatal(err)
	}
	derived, mine, collection, openrouter, huggingface := seedTypeColumns(t)
	putDerivedRecipe(t, derived, []int64{mine, collection})
	_, _ = openrouter, huggingface

	// Through v45 only: v46 and v47 rewrite these very rows (research replaces
	// hand, collection goes), and what is asserted here is what v39 recorded.
	migrateUpTo(t, 45)

	rows, err := DB.Query("SELECT source, rank FROM column_sources WHERE column_id=? ORDER BY rank", derived)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var source string
		var rank int
		if err := rows.Scan(&source, &rank); err != nil {
			t.Fatal(err)
		}
		got = append(got, source)
	}
	// v38 rewrites the untouched recipe to [mine, collection, openrouter,
	// huggingface] before v39 reads it, so column_sources reflects that order:
	// hand (from mine), then collection, then openrouter, then huggingface.
	want := []string{"hand", "collection", "openrouter", "huggingface"}
	if len(got) != len(want) {
		t.Fatalf("column_sources for derived.model_type = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("column_sources rank %d = %q, want %q", i, got[i], want[i])
		}
	}
}

// TestPivotResolveRejectsAnUnknownValue proves the CHECK on columns.resolve
// added by v39 actually constrains what can be stored, not only what the
// migration itself writes.
func TestPivotResolveRejectsAnUnknownValue(t *testing.T) {
	setupTestDB(t)
	if err := migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	_, err := DB.Exec(`INSERT INTO columns
		(column_key, label, value_type, unit, about, position, visible, sealed, created_at, color_rules, resolve)
		VALUES ('bogus','Bogus','text','','',0,1,0,?,'', 'bogus')`, NowStamp())
	if err == nil {
		t.Fatal("expected the resolve CHECK to reject 'bogus', got no error")
	}
}
