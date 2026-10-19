package dbcore

import "testing"

// seedTwoNamespaceColumns creates two columns for the same bare attribute, in
// two different namespaces and positions, mirroring one of the
// real merge groups before v40 runs. lowerPosition holds the lower
// position (the survivor) and higherPosition the one v40
// is expected to absorb and drop.
func seedTwoNamespaceColumns(t *testing.T, attribute string) (lowerPosition, higherPosition int64) {
	t.Helper()
	insert := func(namespace string, position int) int64 {
		res, err := DB.Exec(`INSERT INTO columns
			(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
			VALUES (?, ?, ?, 'text', '', '', ?, 0, 0, ?, '')`,
			namespace+"."+attribute, namespace, namespace+"."+attribute, position, NowStamp())
		if err != nil {
			t.Fatalf("insert column %s.%s: %v", namespace, attribute, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	lowerPosition = insert("openrouter", 10)
	higherPosition = insert("huggingface", 11)
	return
}

// seedMergeGroup creates one column per namespace given, all stating the same
// bare attribute, at ascending positions in the order given - so the first
// namespace listed is always the survivor. It mirrors a
// merge group with no mine.* member, the shape that would otherwise get no
// hand row at all.
func seedMergeGroup(t *testing.T, attribute string, namespaces ...string) map[string]int64 {
	t.Helper()
	ids := map[string]int64{}
	for i, namespace := range namespaces {
		res, err := DB.Exec(`INSERT INTO columns
			(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
			VALUES (?, ?, ?, 'text', '', '', ?, 0, 0, ?, '')`,
			namespace+"."+attribute, namespace, namespace+"."+attribute, i, NowStamp())
		if err != nil {
			t.Fatalf("insert column %s.%s: %v", namespace, attribute, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		ids[namespace] = id
	}
	return ids
}

// stampAtV38 rolls a fresh (floor-schema) database back to just before v39, so
// migrateThrough(t, target) for a target at or past v39 applies v39 itself -
// which is what adds cells.source, column_sources and sources - rather than
// finding the database already claiming to have them while the floor schema
// underneath still lacks the columns.
func stampAtV38(t *testing.T) {
	t.Helper()
	if _, err := DB.Exec("PRAGMA user_version=38"); err != nil {
		t.Fatal(err)
	}
}

// TestCollapseMergesOntoLowestPositionSurvivorAndMovesItsCell covers the core
// of v40: two columns stating one attribute become one, the lower-position
// column survives, and the value the other one held is not lost - it moves
// onto the survivor rather than being dropped. v39's backfill (run as part of
// migrateThrough(t, 40), since the stamp below is at v38) is what stamps the
// pre-existing cell's source from its column's namespace before v40 reads it.
func TestCollapseMergesOntoLowestPositionSurvivorAndMovesItsCell(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	survivor, absorbed := seedTwoNamespaceColumns(t, "widget_count")
	putModel(t, "M")
	putTextCell(t, absorbed, "M", "7")

	migrateThrough(t, 40)

	var stillExists int
	if err := DB.QueryRow("SELECT COUNT(*) FROM columns WHERE id=?", absorbed).Scan(&stillExists); err != nil {
		t.Fatal(err)
	}
	if stillExists != 0 {
		t.Fatalf("the absorbed column %d still exists after the collapse, want it dropped", absorbed)
	}

	var value, source string
	if err := DB.QueryRow("SELECT value_text, source FROM cells WHERE column_id=? AND model_name='M'", survivor).Scan(&value, &source); err != nil {
		t.Fatalf("the absorbed column's cell did not land on the survivor: %v", err)
	}
	if value != "7" || source != "huggingface" {
		t.Fatalf("survivor's cell = (%q, %q), want (\"7\", \"huggingface\") - the moved value and its original source", value, source)
	}
}

// TestCollapseMakesAMergeTargetColumnVisible checks that a column
// named in mergeTargetColumns becomes visible once the collapse runs, even
// though every namespace column feeding it was hidden.
func TestCollapseMakesAMergeTargetColumnVisible(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	survivor, _ := seedTwoNamespaceColumns(t, "price_unit")

	migrateThrough(t, 40)

	var visible int
	if err := DB.QueryRow("SELECT visible FROM columns WHERE id=?", survivor).Scan(&visible); err != nil {
		t.Fatal(err)
	}
	if visible != 1 {
		t.Errorf("price_unit's survivor visible = %d, want 1 (a merge-target column)", visible)
	}
}

// TestCollapseFillsColumnSourcesForAnUntouchedColumn covers
// fillColumnSourcesForSingletons: a column the collapse never merges still
// ends up with a hand row at rank 0 and its pre-pivot source at rank 1,
// because v41's write-guard trigger refuses any column with no hand row
// and refuses any column with no column_sources row at all.
func TestCollapseFillsColumnSourcesForAnUntouchedColumn(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	lone := insertColumn(t, "openrouter.context_length", "openrouter")

	migrateThrough(t, 40)

	rows, err := DB.Query("SELECT source, rank FROM column_sources WHERE column_id=? ORDER BY rank", lone)
	if err != nil {
		t.Fatalf("no column_sources rows for the untouched column: %v", err)
	}
	defer rows.Close()
	var got [][2]any
	for rows.Next() {
		var source string
		var rank int
		if err := rows.Scan(&source, &rank); err != nil {
			t.Fatal(err)
		}
		got = append(got, [2]any{source, rank})
	}
	want := [][2]any{{"hand", 0}, {"openrouter", 1}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("column_sources for the untouched column = %v, want %v", got, want)
	}
}

// TestCollapseGivesAMineOnlySingletonJustTheHandRow covers
// fillColumnSourcesForSingletons for a mine-namespace column that is not
// merged with anything (matching the personal columns that hold data): it takes the hand row at rank 0 and nothing else,
// because its own pre-pivot source already IS hand.
func TestCollapseGivesAMineOnlySingletonJustTheHandRow(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	col := insertColumn(t, "mine.notes", "mine")

	migrateThrough(t, 40)

	rows, err := DB.Query("SELECT source, rank FROM column_sources WHERE column_id=?", col)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		n++
		var source string
		var rank int
		if err := rows.Scan(&source, &rank); err != nil {
			t.Fatal(err)
		}
		if source != "hand" || rank != 0 {
			t.Errorf("mine.notes column_sources row = (%q, %d), want (\"hand\", 0)", source, rank)
		}
	}
	if n != 1 {
		t.Errorf("mine.notes has %d column_sources row(s), want exactly 1", n)
	}
}

// TestCollapseReassignsActiveParametersFromHuggingfaceToHand covers criterion
// C13: active_parameters' cells move to the hand source, so a later refresh
// can never overwrite a reader's own correction again.
func TestCollapseReassignsActiveParametersFromHuggingfaceToHand(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	col := insertColumn(t, "huggingface.active_parameters", "huggingface")
	putModel(t, "M")
	putTextCell(t, col, "M", "7000000000")

	migrateThrough(t, 40)

	var source string
	if err := DB.QueryRow("SELECT source FROM cells WHERE column_id=? AND model_name='M'", col).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "hand" {
		t.Errorf("active_parameters' cell source after v40 = %q, want \"hand\"", source)
	}
}

// TestCollapseStampsAddedWithTheAppSource checks that mine.added is
// not a fact any source states, so its cells are stamped app rather than
// hand (which is what v39's backfill would otherwise give a mine-namespace
// cell).
func TestCollapseStampsAddedWithTheAppSource(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	col := insertColumn(t, "mine.added", "mine")
	putModel(t, "M")
	putTextCell(t, col, "M", "2026-08-01")

	migrateThrough(t, 40)

	var source string
	if err := DB.QueryRow("SELECT source FROM cells WHERE column_id=? AND model_name='M'", col).Scan(&source); err != nil {
		t.Fatal(err)
	}
	if source != "app" {
		t.Errorf("added's cell source after v40 = %q, want \"app\"", source)
	}
}

// TestCollapseSetsUnionResolveOnTheThreeListColumns covers the resolve='union'
// side of v40: input_modalities, output_modalities and collections take union
// resolution, unlike every other column's default first_stated.
func TestCollapseSetsUnionResolveOnTheThreeListColumns(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	insertColumn(t, "openrouter.input_modalities", "openrouter")
	insertColumn(t, "openrouter.output_modalities", "openrouter")
	insertColumn(t, "collection.collections", "collection")
	other := insertColumn(t, "openrouter.context_length", "openrouter")

	migrateThrough(t, 40)

	for _, bare := range []string{"input_modalities", "output_modalities", "collections"} {
		var resolve string
		if err := DB.QueryRow("SELECT resolve FROM columns WHERE column_key LIKE ?", "%"+bare).Scan(&resolve); err != nil {
			t.Fatalf("read resolve for %s: %v", bare, err)
		}
		if resolve != "union" {
			t.Errorf("%s resolve = %q, want \"union\"", bare, resolve)
		}
	}
	var resolve string
	if err := DB.QueryRow("SELECT resolve FROM columns WHERE id=?", other).Scan(&resolve); err != nil {
		t.Fatal(err)
	}
	if resolve != "first_stated" {
		t.Errorf("context_length resolve = %q, want the untouched default \"first_stated\"", resolve)
	}
}

// TestCollapseRestampsAnUnsourcedCellBeforeMergingSoTwoOfThemDoNotCollide
// reproduces the failure the owner hit on his real catalog: a pre-pivot
// binary, run after v39 added cells.source, does not know that column exists
// and writes every cell it inserts with the column default, an empty string. Two such
// cells, one on each member of a merge group, share that empty string as a
// source, and widenCellsPrimaryKey's own PRIMARY KEY (column_id, model_name,
// source) refuses the second the instant collapseMergedColumns tries to move
// it onto the survivor - exactly the "UNIQUE constraint failed" error the
// owner saw. Without the restamp this migration fails; with it, both cells
// survive under the sources their own columns' namespaces imply. It calls
// theColumnsCollapseOntoOnePerAttribute directly, the same pattern
// TestCollapseRefusesToMergeColumnsWhoseUnitOrTypeDisagree uses below, rather
// than a second migrateThrough call: migrateThrough truncates the shared
// migrations slice and restores it only in t.Cleanup, so a second call in the
// same test reads that truncation as its own starting point and silently
// drops every migration after the first call's target.
func TestCollapseRestampsAnUnsourcedCellBeforeMergingSoTwoOfThemDoNotCollide(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	migrateThrough(t, 39)

	survivor, absorbed := seedTwoNamespaceColumns(t, "widget_count")
	putModel(t, "M")
	putTextCell(t, survivor, "M", "5") // written with no source, like the 9.3.0 binary would
	putTextCell(t, absorbed, "M", "7") // same: no source

	// theColumnsCollapseOntoOnePerAttribute's own widenCellsPrimaryKey step
	// drops and recreates cells, which every other cell-store table
	// references - the same reason migrate() runs v40 with
	// disableForeignKeys, done here by hand since this test calls the
	// migration step directly rather than through migrate().
	if _, err := DB.Exec("PRAGMA foreign_keys=OFF"); err != nil {
		t.Fatal(err)
	}
	defer DB.Exec("PRAGMA foreign_keys=ON")

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := theColumnsCollapseOntoOnePerAttribute(tx); err != nil {
		tx.Rollback()
		t.Fatalf("v40 failed to collapse two unsourced cells onto one survivor: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	rows, err := DB.Query("SELECT source, value_text FROM cells WHERE column_id=? AND model_name='M' ORDER BY source", survivor)
	if err != nil {
		t.Fatalf("read the survivor's cells after the collapse: %v", err)
	}
	defer rows.Close()
	got := map[string]string{}
	for rows.Next() {
		var source, value string
		if err := rows.Scan(&source, &value); err != nil {
			t.Fatal(err)
		}
		got[source] = value
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"openrouter": "5", "huggingface": "7"}
	if len(got) != len(want) || got["openrouter"] != want["openrouter"] || got["huggingface"] != want["huggingface"] {
		t.Fatalf("survivor's cells for M after the collapse = %v, want %v - both answers, each under its own restamped source", got, want)
	}
}

// TestCollapseWidensCellsPrimaryKeySoTwoSourcesCanShareAModelAndColumn covers
// widenCellsPrimaryKey directly: once v40 has run, two cells for the same
// column and model but different sources can coexist, which the old
// (column_id, model_name) key would have refused as a duplicate.
func TestCollapseWidensCellsPrimaryKeySoTwoSourcesCanShareAModelAndColumn(t *testing.T) {
	setupTestDB(t)
	migrateThrough(t, 40)

	col := insertColumn(t, "openrouter.description", "openrouter")
	if _, err := DB.Exec(`INSERT INTO column_sources (column_id, source, rank) VALUES (?,'openrouter',0),(?,'collection',1)`, col, col); err != nil {
		t.Fatal(err)
	}
	putModel(t, "M")
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','openrouter','a','t')`, col); err != nil {
		t.Fatal(err)
	}
	if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','collection','b','t')`, col); err != nil {
		t.Fatalf("second source's cell for the same column and model was refused: %v", err)
	}

	var n int
	if err := DB.QueryRow("SELECT COUNT(*) FROM cells WHERE column_id=? AND model_name='M'", col).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Errorf("cells for one column and model after v40 = %d, want 2 (one per source)", n)
	}
}

// TestCollapseAcceptsAHandWriteOnEveryGroupThatHadNoMineMember covers the
// merge groups that never contained a mine.* column (zdr, repository,
// description, created_at, input_modalities, output_modalities,
// price_image_output, price_unit). Their survivor must still take a hand row,
// or v41's cells_source_is_declared trigger refuses every hand write to them.
// Each subtest mirrors one group's real namespaces and positions, migrates a
// database that goes through the collapse, and writes a hand cell on the
// survivor, which must be accepted.
func TestCollapseAcceptsAHandWriteOnEveryGroupThatHadNoMineMember(t *testing.T) {
	groups := []struct {
		attribute  string
		namespaces []string // survivor first, in position order
	}{
		{"zdr", []string{"derived", "openrouter"}},
		{"repository", []string{"openrouter", "huggingface"}},
		{"description", []string{"openrouter", "huggingface"}},
		{"created_at", []string{"openrouter", "huggingface"}},
		{"input_modalities", []string{"openrouter", "huggingface", "collection"}},
		{"output_modalities", []string{"openrouter", "huggingface", "collection"}},
		{"price_image_output", []string{"openrouter", "collection"}},
		{"price_unit", []string{"collection", "openrouter"}},
	}

	for _, g := range groups {
		t.Run(g.attribute, func(t *testing.T) {
			setupTestDB(t)
			stampAtV38(t)
			seedMergeGroup(t, g.attribute, g.namespaces...)

			migrateThrough(t, 41)

			var survivorID int64
			if err := DB.QueryRow("SELECT id FROM columns WHERE column_key=?", g.attribute).Scan(&survivorID); err != nil {
				t.Fatalf("no surviving column named %s after the collapse: %v", g.attribute, err)
			}
			putModel(t, "M")
			if _, err := DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','hand','x','t')`, survivorID); err != nil {
				t.Errorf("a hand write to %s was refused after the collapse: %v", g.attribute, err)
			}
		})
	}
}

// TestCollapseAddedTakesNoHandRowAndRefusesAHandWrite covers the added column:
// added's only source is the app itself, so it gets no hand row at rank 0
// (unlike every other column), and v41's
// cells_source_is_declared trigger refuses a hand write to it.
func TestCollapseAddedTakesNoHandRowAndRefusesAHandWrite(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	col := insertColumn(t, "mine.added", "mine")

	migrateThrough(t, 41)

	rows, err := DB.Query("SELECT source, rank FROM column_sources WHERE column_id=?", col)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var n int
	for rows.Next() {
		n++
		var source string
		var rank int
		if err := rows.Scan(&source, &rank); err != nil {
			t.Fatal(err)
		}
		if source != "app" || rank != 0 {
			t.Errorf("added's column_sources row = (%q, %d), want (\"app\", 0)", source, rank)
		}
	}
	if n != 1 {
		t.Errorf("added has %d column_sources row(s), want exactly 1 (no hand row)", n)
	}

	putModel(t, "M")
	_, err = DB.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, written_at) VALUES (?,'M','hand','x','t')`, col)
	if err == nil {
		t.Error("expected a hand write to added to be refused, got no error")
	}
}

// TestCollapsePreservesEveryHandColumnsCellCount checks that the
// ten hand-bearing columns - the nine mine.* columns that hold data, plus
// huggingface.active_parameters, reassigned to hand - keep every cell
// they held before the collapse, and the total cell count does not fall.
func TestCollapsePreservesEveryHandColumnsCellCount(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	putModel(t, "M1")
	putModel(t, "M2")

	mineOnly := []string{"notes", "author", "architecture_basis", "build_size_gb",
		"rating", "sizing_notes", "favorite", "pricing_note", "context_length_build"}
	ids := map[string]int64{}
	for _, attribute := range mineOnly {
		id := insertColumn(t, "mine."+attribute, "mine")
		putTextCell(t, id, "M1", "v1")
		putTextCell(t, id, "M2", "v2")
		ids[attribute] = id
	}
	added := insertColumn(t, "mine.added", "mine")
	putTextCell(t, added, "M1", "2026-01-01")
	activeParameters := insertColumn(t, "huggingface.active_parameters", "huggingface")
	putTextCell(t, activeParameters, "M1", "7000000000")

	var totalBefore int
	if err := DB.QueryRow("SELECT COUNT(*) FROM cells").Scan(&totalBefore); err != nil {
		t.Fatal(err)
	}

	migrateThrough(t, 41)

	var totalAfter int
	if err := DB.QueryRow("SELECT COUNT(*) FROM cells").Scan(&totalAfter); err != nil {
		t.Fatal(err)
	}
	if totalAfter < totalBefore {
		t.Errorf("total cells fell from %d to %d across v40 and v41", totalBefore, totalAfter)
	}

	for _, attribute := range mineOnly {
		var n int
		if err := DB.QueryRow("SELECT COUNT(*) FROM cells WHERE column_id=? AND source='hand'", ids[attribute]).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 2 {
			t.Errorf("%s has %d hand cell(s) after the collapse, want 2 (unchanged)", attribute, n)
		}
	}
	var addedCount int
	if err := DB.QueryRow("SELECT COUNT(*) FROM cells WHERE column_id=? AND source='app'", added).Scan(&addedCount); err != nil {
		t.Fatal(err)
	}
	if addedCount != 1 {
		t.Errorf("added has %d app cell(s), want 1", addedCount)
	}
	var activeParametersCount int
	if err := DB.QueryRow("SELECT COUNT(*) FROM cells WHERE column_id=? AND source='hand'", activeParameters).Scan(&activeParametersCount); err != nil {
		t.Fatal(err)
	}
	if activeParametersCount != 1 {
		t.Errorf("active_parameters has %d hand cell(s) after reassignment, want 1", activeParametersCount)
	}
}

// seedMergeGroupTyped creates one column per namespace given, all stating the
// same bare attribute in the same value type and unit, at ascending
// positions in the order given - so the first namespace listed is always the
// survivor. It is seedMergeGroup with the value type and
// unit stated explicitly, needed here because the merge guard now checks
// both against the survivor before it moves a single cell.
func seedMergeGroupTyped(t *testing.T, attribute, valueType, unit string, namespaces ...string) map[string]int64 {
	t.Helper()
	ids := map[string]int64{}
	for i, namespace := range namespaces {
		res, err := DB.Exec(`INSERT INTO columns
			(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
			VALUES (?, ?, ?, ?, ?, '', ?, 0, 0, ?, '')`,
			namespace+"."+attribute, namespace, namespace+"."+attribute, valueType, unit, i, NowStamp())
		if err != nil {
			t.Fatalf("insert column %s.%s: %v", namespace, attribute, err)
		}
		id, err := res.LastInsertId()
		if err != nil {
			t.Fatal(err)
		}
		ids[namespace] = id
	}
	return ids
}

// TestCollapseRefusesToMergeColumnsWhoseUnitOrTypeDisagree is the successor to
// the deleted TestEveryComputedColumnMergesOneFactInOneUnit
// (internal/store/shipped_names_test.go, removed in stage B2's commit
// 3785c3b with no replacement). That test proved a computed column only ever
// read inputs sharing its own fact stem, unit and value type - the guard
// against the fault that made "author" mean the vendor on one row and the
// republisher on the next. Post-pivot that same fault would be a merge group
// whose members disagree on unit or value type: collapseMergedColumns groups
// purely by the bare attribute name in column_key, so nothing stopped it
// silently combining two unlike figures into one column before this test's
// subject (the check added to collapseMergedColumns) existed. This calls the
// migration step directly, not through migrate(), because a real failure
// there is fatal (failMigration calls shared.Fatalf) and would kill the test
// binary rather than report a failure.
func TestCollapseRefusesToMergeColumnsWhoseUnitOrTypeDisagree(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	migrateThrough(t, 39)

	seedMergeGroupTyped(t, "widget_count", "number", "widgets", "openrouter")
	if _, err := DB.Exec(`INSERT INTO columns
		(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES ('huggingface.widget_count', 'huggingface', 'huggingface.widget_count', 'text', '', '', 1, 0, 0, ?, '')`,
		NowStamp()); err != nil {
		t.Fatal(err)
	}

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()

	err = collapseMergedColumns(tx)
	if err == nil {
		t.Fatal("collapseMergedColumns merged two widget_count columns holding different value types, want it refused")
	}
}

// mergeGroupFixture is one of the real merge groups,
// with the label, value type and unit its shipped columns actually carry
// (internal/store/shipped.go, read 2026-08-28) and the namespaces stated for
// it, survivor first in position order.
type mergeGroupFixture struct {
	attribute  string
	valueType  string
	unit       string
	namespaces []string
}

// theFifteenRealMergeGroups pins which namespaces state each attribute, and internal/store/shipped.go's own
// account of each survivor's value type and unit. If a future edit to either
// document introduces a member that disagrees, this fixture and
// TestCollapseAcceptsAllFifteenRealMergeGroups drift out of step with it and
// the test that reproduces it (immediately above) is what catches the drift
// once the mismatched member is real: a column stating one of these fifteen
// attributes with a different value type or unit is refused by
// collapseMergedColumns rather than silently merged.
var theFifteenRealMergeGroups = []mergeGroupFixture{
	{"model_type", "text", "", []string{"derived", "mine", "collection", "openrouter", "huggingface"}},
	{"context_length", "number", "tokens", []string{"derived", "mine", "openrouter", "huggingface"}},
	{"price_prompt", "number", "USD per token", []string{"derived", "mine", "openrouter", "collection"}},
	{"price_completion", "number", "USD per token", []string{"derived", "mine", "openrouter", "collection"}},
	{"supports_tools", "flag", "", []string{"derived", "mine", "openrouter"}},
	{"supports_reasoning", "flag", "", []string{"derived", "mine", "openrouter"}},
	{"zdr", "flag", "", []string{"derived", "openrouter"}},
	{"moe", "flag", "", []string{"derived", "mine", "huggingface", "openrouter"}},
	{"repository", "text", "", []string{"openrouter", "huggingface"}},
	{"description", "text", "", []string{"openrouter", "huggingface"}},
	{"created_at", "text", "", []string{"openrouter", "huggingface"}},
	{"input_modalities", "list", "", []string{"openrouter", "huggingface", "collection"}},
	{"output_modalities", "list", "", []string{"openrouter", "huggingface", "collection"}},
	{"price_image_output", "number", "USD per image", []string{"openrouter", "collection"}},
	{"price_unit", "text", "", []string{"collection", "openrouter"}},
}

// TestCollapseAcceptsAllFifteenRealMergeGroups seeds a column set shaped like
// the real merge groups, each member carrying the value
// type and unit its shipped column actually holds, and proves
// collapseMergedColumns merges every one of them into a single survivor
// without the value-type-or-unit guard refusing any - the fifteen groups
// really are semantically sound, which is the fact the deleted
// TestEveryComputedColumnMergesOneFactInOneUnit used to establish about their
// pre-pivot equivalent (computed columns' recipe inputs).
func TestCollapseAcceptsAllFifteenRealMergeGroups(t *testing.T) {
	setupTestDB(t)
	stampAtV38(t)
	migrateThrough(t, 39)

	for _, group := range theFifteenRealMergeGroups {
		seedMergeGroupTyped(t, group.attribute, group.valueType, group.unit, group.namespaces...)
	}

	tx, err := DB.Begin()
	if err != nil {
		t.Fatal(err)
	}
	if err := collapseMergedColumns(tx); err != nil {
		tx.Rollback()
		t.Fatalf("collapseMergedColumns refused one of the fifteen real merge groups: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}

	for _, group := range theFifteenRealMergeGroups {
		var n int
		if err := DB.QueryRow("SELECT COUNT(*) FROM columns WHERE column_key LIKE ?", "%."+group.attribute).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("%s has %d column(s) after the collapse, want 1 (one survivor, the rest absorbed)", group.attribute, n)
		}
	}
}
