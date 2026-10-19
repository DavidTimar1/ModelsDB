package store

import (
	"errors"
	"modelsdb/internal/dbcore"
	"testing"
)

// TestRecordFeedFieldsReportsNothingOnTheFirstRun asserts the "first ever run"
// carve-out in RecordFeedFields: a database with no prior rows has nothing to
// compare against, so calling every path "new" would be noise, not news.
func TestRecordFeedFieldsReportsNothingOnTheFirstRun(t *testing.T) {
	cellStoreDB(t)

	newP, vanished, err := RecordFeedFields(map[string]FieldObservation{
		"id":             {Kind: ValueTypeText, Models: 3, Sample: "a/b"},
		"pricing.prompt": {Kind: ValueTypeNumber, Models: 3, Sample: "0.1"},
	})
	if err != nil {
		t.Fatalf("RecordFeedFields: %v", err)
	}
	if len(newP) != 0 || len(vanished) != 0 {
		t.Fatalf("first run should report nothing new/vanished, got new=%v vanished=%v", newP, vanished)
	}

	fields, err := ListFeedFields()
	if err != nil {
		t.Fatalf("ListFeedFields: %v", err)
	}
	if len(fields) != 2 {
		t.Fatalf("expected 2 recorded fields, got %d", len(fields))
	}
}

// TestRecordFeedFieldsReportsNewAndVanished exercises the comparison a SECOND
// refresh makes: a path absent from the first run is new, and a path present in
// the first run but missing from the second has vanished.
func TestRecordFeedFieldsReportsNewAndVanished(t *testing.T) {
	cellStoreDB(t)

	if _, _, err := RecordFeedFields(map[string]FieldObservation{
		"id":      {Kind: ValueTypeText, Models: 3, Sample: "a/b"},
		"context": {Kind: ValueTypeNumber, Models: 3, Sample: "8192"},
	}); err != nil {
		t.Fatalf("first RecordFeedFields: %v", err)
	}

	newP, vanished, err := RecordFeedFields(map[string]FieldObservation{
		"id":               {Kind: ValueTypeText, Models: 3, Sample: "a/b"},
		"pricing.internal": {Kind: ValueTypeNumber, Models: 2, Sample: "0.2"},
	})
	if err != nil {
		t.Fatalf("second RecordFeedFields: %v", err)
	}
	if len(newP) != 1 || newP[0] != "pricing.internal" {
		t.Fatalf("expected new=[pricing.internal], got %v", newP)
	}
	if len(vanished) != 1 || vanished[0] != "context" {
		t.Fatalf("expected vanished=[context], got %v", vanished)
	}

	fields, err := ListFeedFields()
	if err != nil {
		t.Fatalf("ListFeedFields: %v", err)
	}
	byPath := map[string]FeedField{}
	for _, f := range fields {
		byPath[f.Path] = f
	}
	if !byPath["context"].Vanished {
		t.Errorf("context should read as vanished")
	}
	if byPath["id"].Vanished {
		t.Errorf("id should not read as vanished")
	}
	if byPath["pricing.internal"].Vanished {
		t.Errorf("pricing.internal should not read as vanished")
	}
}

// recordPath records one feed path as a refresh would, so a column can be
// made from it.
func recordPath(t *testing.T, path, kind string) {
	t.Helper()
	if _, _, err := RecordFeedFields(map[string]FieldObservation{
		path: {Kind: kind, Models: 1, Sample: "x"},
	}); err != nil {
		t.Fatalf("RecordFeedFields(%s): %v", path, err)
	}
}

// TestCreateFeedFieldColumnIsKeyedByTheWholePathHiddenAndSealed covers what a
// column made from the feed IS: its key is the whole path with "[]" removed
// and dots as underscores, its label those words capitalised, it arrives
// hidden and sealed, sourced openrouter then hand, sits where a reader's own
// column sits, and the path is tied to it.
func TestCreateFeedFieldColumnIsKeyedByTheWholePathHiddenAndSealed(t *testing.T) {
	cellStoreDB(t)
	recordPath(t, "benchmarks.design_arena[].elo", ValueTypeNumber)

	col, err := CreateFeedFieldColumn("benchmarks.design_arena[].elo", ValueTypeNumber)
	if err != nil {
		t.Fatalf("CreateFeedFieldColumn: %v", err)
	}
	if col.Key != "benchmarks_design_arena_elo" {
		t.Errorf("key = %q, want benchmarks_design_arena_elo", col.Key)
	}
	if col.Label != "Benchmarks design arena elo" {
		t.Errorf("label = %q, want \"Benchmarks design arena elo\"", col.Label)
	}
	if col.ValueType != ValueTypeNumber {
		t.Errorf("value type = %q, want number", col.ValueType)
	}
	if col.Visible {
		t.Errorf("a column made from the feed must arrive hidden")
	}
	if !col.Sealed {
		t.Errorf("a column made from the feed must be sealed")
	}
	if col.Position != ownColumnPosition {
		t.Errorf("position = %d, want %d, where a reader's own column sits", col.Position, ownColumnPosition)
	}

	rows, err := dbcore.DB.Query("SELECT source FROM column_sources WHERE column_id=? ORDER BY rank ASC", col.ID)
	if err != nil {
		t.Fatal(err)
	}
	var sources []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			t.Fatal(err)
		}
		sources = append(sources, src)
	}
	rows.Close()
	if len(sources) != 2 || sources[0] != string(SourceOpenRouter) || sources[1] != string(SourceResearch) {
		t.Errorf("sources by rank = %v, want [openrouter research]", sources)
	}

	tied, err := ImportedFeedFieldColumns()
	if err != nil {
		t.Fatalf("ImportedFeedFieldColumns: %v", err)
	}
	if tied["benchmarks_design_arena_elo"] != "benchmarks.design_arena[].elo" {
		t.Errorf("ImportedFeedFieldColumns[benchmarks_design_arena_elo] = %q, want the path", tied["benchmarks_design_arena_elo"])
	}
}

// TestCreateFeedFieldColumnRefusesAnUnrecordedPath asserts a path never seen
// in a refresh gets no column - there is nothing to tie it to yet.
func TestCreateFeedFieldColumnRefusesAnUnrecordedPath(t *testing.T) {
	cellStoreDB(t)
	if _, err := CreateFeedFieldColumn("never.seen", ValueTypeText); err == nil {
		t.Fatalf("expected an error making a column for a path never recorded by a refresh")
	}
}

// TestATiedPathKeepsItsColumnAndBecomesSealed: a column an earlier release
// let the reader make from the feed by hand, under a last-segment key, keeps
// that key, that label and that type - the path is never made twice - and
// SealFeedFieldColumns seals it like one the refresh made.
func TestATiedPathKeepsItsColumnAndBecomesSealed(t *testing.T) {
	cellStoreDB(t)
	recordPath(t, "benchmarks.artificial_analysis.coding_index", ValueTypeNumber)
	old := makeColumn(t, ColumnSpec{Key: "coding_index", Label: "Coding index", ValueType: ValueTypeText, Position: ownColumnPosition}, SourceOpenRouter, SourceResearch)
	if _, err := dbcore.DB.Exec("UPDATE feed_fields SET column_id=? WHERE path=?", old.ID, "benchmarks.artificial_analysis.coding_index"); err != nil {
		t.Fatalf("tie the old column: %v", err)
	}

	again, err := CreateFeedFieldColumn("benchmarks.artificial_analysis.coding_index", ValueTypeNumber)
	if err != nil {
		t.Fatalf("CreateFeedFieldColumn on a tied path: %v", err)
	}
	if again.Key != "coding_index" || again.Label != "Coding index" || again.ValueType != ValueTypeText {
		t.Errorf("a tied path answered %+v, want the old column unchanged", again)
	}
	if _, err := ColumnByKey("benchmarks_artificial_analysis_coding_index"); err == nil {
		t.Errorf("a second column was made for a path already tied to one")
	}

	sealed, err := SealFeedFieldColumns()
	if err != nil {
		t.Fatalf("SealFeedFieldColumns: %v", err)
	}
	if sealed != 1 {
		t.Errorf("SealFeedFieldColumns sealed %d, want 1", sealed)
	}
	col, err := ColumnByKey("coding_index")
	if err != nil {
		t.Fatal(err)
	}
	if !col.Sealed {
		t.Errorf("the old column is not sealed after SealFeedFieldColumns")
	}
	if sealed, err := SealFeedFieldColumns(); err != nil || sealed != 0 {
		t.Errorf("a second SealFeedFieldColumns sealed %d (err %v), want 0", sealed, err)
	}
}

// TestAColumnMadeFromTheFeedCannotBeDeleted: the next refresh would make it
// again, so DeleteColumn refuses it and its cells stay.
func TestAColumnMadeFromTheFeedCannotBeDeleted(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")
	recordPath(t, "top_provider.context_length", ValueTypeNumber)
	col, err := CreateFeedFieldColumn("top_provider.context_length", ValueTypeNumber)
	if err != nil {
		t.Fatalf("CreateFeedFieldColumn: %v", err)
	}
	if err := WriteCells(SourceOpenRouter, "Qwen3 32B", map[string]string{col.Key: "32768"}); err != nil {
		t.Fatalf("WriteCells: %v", err)
	}

	if err := DeleteColumn(col.Key); !errors.Is(err, ErrColumnIsASource) {
		t.Fatalf("DeleteColumn on a column made from the feed = %v, want ErrColumnIsASource", err)
	}
	cells, err := ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatal(err)
	}
	if cells[col.Key] != "32768" {
		t.Errorf("%s holds %q after the refused delete, want 32768", col.Key, cells[col.Key])
	}
}

// TestFeedColumnLabelGainsThePathWhenTheLabelIsTaken: a label another column
// carries would fail the insert on the unique index over labels, so the
// column is made under the label with its path in brackets instead.
func TestFeedColumnLabelGainsThePathWhenTheLabelIsTaken(t *testing.T) {
	cellStoreDB(t)
	makeColumn(t, ColumnSpec{Key: "elo", Label: "Arena elo", ValueType: ValueTypeNumber})
	recordPath(t, "arena.elo", ValueTypeNumber)

	col, err := CreateFeedFieldColumn("arena.elo", ValueTypeNumber)
	if err != nil {
		t.Fatalf("CreateFeedFieldColumn: %v", err)
	}
	if col.Key != "arena_elo" {
		t.Errorf("key = %q, want arena_elo", col.Key)
	}
	if col.Label != "Arena elo (arena.elo)" {
		t.Errorf("label = %q, want \"Arena elo (arena.elo)\"", col.Label)
	}
}

// TestCreateFeedFieldColumnRefusesAKeyAnotherColumnHolds: the full-path key
// can still be taken, by a column of the reader's own; the path then makes
// nothing rather than a column under a different key, and the refresh logs it.
func TestCreateFeedFieldColumnRefusesAKeyAnotherColumnHolds(t *testing.T) {
	cellStoreDB(t)
	makeColumn(t, ColumnSpec{Key: "arena_elo", Label: "My elo", ValueType: ValueTypeNumber})
	recordPath(t, "arena.elo", ValueTypeNumber)
	if _, err := CreateFeedFieldColumn("arena.elo", ValueTypeNumber); err == nil {
		t.Fatalf("expected a refusal when arena_elo is already a column")
	}
}
