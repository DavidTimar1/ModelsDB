package store

import (
	"database/sql"
	"errors"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// cellStoreDB gives one test its own fully-migrated temporary database, so
// nothing here reads or writes the real data directory.
func cellStoreDB(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	paths.DataDir = dir
	paths.DbFile = filepath.Join(dir, "modelsdb.db")
	paths.BackupsDir = filepath.Join(dir, ".backups")
	if err := dbcore.OpenDB(paths.DbFile); err != nil {
		t.Fatalf("OpenDB: %v", err)
	}
	t.Cleanup(func() {
		if dbcore.DB != nil {
			dbcore.DB.Close()
		}
	})
}

// addModelRow writes one model row, which a cell has to hang off (cells.model_name
// REFERENCES models(name)).
func addModelRow(t *testing.T, name string) {
	t.Helper()
	if _, err := dbcore.DB.Exec("INSERT INTO models (name, source, updated_at) VALUES (?,?,?)",
		name, "manual", dbcore.NowStamp()); err != nil {
		t.Fatalf("insert the model %q: %v", name, err)
	}
}

// makeColumn creates one column, declaring SourceHand as its one source unless
// the caller names others, and fails the test if it cannot.
func makeColumn(t *testing.T, spec ColumnSpec, sources ...Source) Column {
	t.Helper()
	if len(sources) == 0 {
		sources = []Source{SourceHand}
	}
	c, err := CreateColumn(spec, sources...)
	if err != nil {
		t.Fatalf("create the column %s: %v", spec.Key, err)
	}
	return c
}

// sortKeyOf reads the number a cell sorts by straight out of the database, which
// is the half of a cell the API never shows.
func sortKeyOf(t *testing.T, columnKey, modelName string) (float64, bool) {
	t.Helper()
	var key sql.NullFloat64
	err := dbcore.DB.QueryRow(`SELECT v.value_number FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.column_key=? AND v.model_name=?`, columnKey, modelName).Scan(&key)
	if err == sql.ErrNoRows {
		t.Fatalf("no cell stored for %s on %q", columnKey, modelName)
	}
	if err != nil {
		t.Fatalf("read the sort key of %s on %q: %v", columnKey, modelName, err)
	}
	return key.Float64, key.Valid
}

// TestEachKindOfValueRoundTripsThroughTheCellStore writes one cell of every kind
// a column can be and reads back both halves: the text, which is the truth, and
// the number it sorts by, which is worked out from that text by the column's
// kind. A value stored twice can only stay honest if the one writer fills both.
func TestEachKindOfValueRoundTripsThroughTheCellStore(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")

	makeColumn(t, ColumnSpec{Key: "note", Label: "Note", ValueType: ValueTypeText})
	makeColumn(t, ColumnSpec{Key: "context", Label: "Context", ValueType: ValueTypeNumber})
	makeColumn(t, ColumnSpec{Key: "favourite", Label: "Favourite", ValueType: ValueTypeFlag})
	makeColumn(t, ColumnSpec{Key: "speed", Label: "Speed", ValueType: ValueTypeChoice,
		Options: []string{"fast", "avg", "slow"}})
	makeColumn(t, ColumnSpec{Key: "modalities", Label: "Modalities", ValueType: ValueTypeList})

	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{
		"note":       "reads code well",
		"context":    "131072",
		"favourite":  "yes",
		"speed":      "avg",
		"modalities": `["text","image"]`,
	}); err != nil {
		t.Fatalf("write the five cells: %v", err)
	}

	stored, err := ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatalf("read the cells back: %v", err)
	}
	for key, want := range map[string]string{
		"note":       "reads code well",
		"context":    "131072",
		"favourite":  "yes",
		"speed":      "avg",
		"modalities": `["text","image"]`,
	} {
		if stored[key] != want {
			t.Errorf("%s came back as %q, want %q", key, stored[key], want)
		}
	}

	for _, c := range []struct {
		key      string
		want     float64
		hasASort bool
	}{
		{"note", 0, false},
		{"context", 131072, true},
		{"favourite", 1, true},
		{"speed", 1, true}, // "avg" is the second option, counting from 0
		{"modalities", 0, false},
	} {
		got, present := sortKeyOf(t, c.key, "Qwen3 32B")
		if present != c.hasASort {
			t.Errorf("%s: a sort key is present=%v, want %v", c.key, present, c.hasASort)
			continue
		}
		if present && got != c.want {
			t.Errorf("%s sorts by %v, want %v", c.key, got, c.want)
		}
	}
}

// TestAChoiceColumnSortsByItsOptionPositionAndNotItsLetters is the case the
// whole two-column design exists for: read as words, "fast, avg, slow" sorts
// into "avg, fast, slow", which is nonsense, because it is a scale.
func TestAChoiceColumnSortsByItsOptionPositionAndNotItsLetters(t *testing.T) {
	cellStoreDB(t)
	makeColumn(t, ColumnSpec{Key: "speed", Label: "Speed", ValueType: ValueTypeChoice,
		Options: []string{"fast", "avg", "slow"}})
	for model, speed := range map[string]string{"A": "slow", "B": "fast", "C": "avg"} {
		addModelRow(t, model)
		if err := WriteCells(SourceHand, model, map[string]string{"speed": speed}); err != nil {
			t.Fatalf("write %s for %q: %v", speed, model, err)
		}
	}

	rows, err := dbcore.DB.Query(`SELECT v.value_text FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.column_key='speed' ORDER BY v.value_number ASC`)
	if err != nil {
		t.Fatalf("sort the column: %v", err)
	}
	defer rows.Close()
	var order []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			t.Fatal(err)
		}
		order = append(order, v)
	}
	if strings.Join(order, ",") != "fast,avg,slow" {
		t.Errorf("the column sorted as %v, want fast,avg,slow - sorting it by its letters would give avg,fast,slow", order)
	}
}

// TestAValueTheColumnWillNotHoldIsRefusedAndSaysWhy covers the rule that a value
// which cannot be converted is a REFUSED write rather than a cell with no sort
// key. A value that fails quietly does not fail: an empty value clears a cell,
// so it would DELETE a figure somebody researched.
func TestAValueTheColumnWillNotHoldIsRefusedAndSaysWhy(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")
	makeColumn(t, ColumnSpec{Key: "context", Label: "Context", ValueType: ValueTypeNumber})
	makeColumn(t, ColumnSpec{Key: "speed", Label: "Speed", ValueType: ValueTypeChoice,
		Options: []string{"fast", "avg", "slow"}})
	makeColumn(t, ColumnSpec{Key: "tool", Label: "Tool", ValueType: ValueTypeFlag})

	for _, c := range []struct{ what, key, value, mustName string }{
		{"a number column given words", "context", "about 32 billion", "about 32 billion"},
		{"a choice column given a label it does not offer", "speed", "quick", "quick"},
		{"a flag column given something that is not yes or no", "tool", "maybe", "maybe"},
	} {
		err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{c.key: c.value})
		if !errors.Is(err, ErrCellValue) {
			t.Errorf("%s: err = %v, want ErrCellValue", c.what, err)
			continue
		}
		if !strings.Contains(err.Error(), c.key) || !strings.Contains(err.Error(), c.mustName) {
			t.Errorf("%s: the refusal must name the column and the value, got %q", c.what, err)
		}
	}

	stored, err := ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Errorf("a refused write stored something after all: %v", stored)
	}
}

// TestARefusedWriteStoresNoneOfTheOtherValuesEither pins the whole call being one
// transaction: a good value beside a bad one is not written.
func TestARefusedWriteStoresNoneOfTheOtherValuesEither(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")
	makeColumn(t, ColumnSpec{Key: "aaa", Label: "First", ValueType: ValueTypeText})
	makeColumn(t, ColumnSpec{Key: "zzz", Label: "Second", ValueType: ValueTypeNumber})

	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{
		"aaa": "this one is fine",
		"zzz": "not a number",
	}); !errors.Is(err, ErrCellValue) {
		t.Fatalf("err = %v, want ErrCellValue", err)
	}
	stored, err := ModelCells("Qwen3 32B")
	if err != nil {
		t.Fatal(err)
	}
	if len(stored) != 0 {
		t.Errorf("the good value was kept even though the write was refused: %v", stored)
	}
}

// TestAnEmptyValueClearsTheCellRatherThanStoringABlank: a model with no value
// for a column has no row at all, which is what makes "nobody has said" and
// "somebody said nothing" one answer.
func TestAnEmptyValueClearsTheCellRatherThanStoringABlank(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")
	makeColumn(t, ColumnSpec{Key: "note", Label: "Note", ValueType: ValueTypeText})

	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{"note": "something"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{"note": "   "}); err != nil {
		t.Fatalf("clear the cell: %v", err)
	}

	var rows int
	if err := dbcore.DB.QueryRow("SELECT COUNT(*) FROM cells").Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d cell row(s) survived being cleared, want none", rows)
	}
}

// TestAWriterIsRefusedAColumnItDoesNotState is what makes a source unable to
// reach a column it never declared. It is the Go half; the database's own
// triggers (cells_source_is_declared, cells_source_never_moves) are the other.
func TestAWriterIsRefusedAColumnItDoesNotState(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")
	makeColumn(t, ColumnSpec{Key: "author", Label: "Author", ValueType: ValueTypeText}, SourceHuggingFace)

	err := WriteCells(SourceOpenRouter, "Qwen3 32B", map[string]string{"author": "Qwen"})
	if !errors.Is(err, ErrColumnKey) {
		t.Fatalf("err = %v, want ErrColumnKey", err)
	}
	if !strings.Contains(err.Error(), "author") {
		t.Errorf("the refusal must name the key, got %q", err)
	}
}

// TestTheDatabaseRefusesAColumnKeyCarryingADot pins the CHECK on columns, so
// the rule survives a writer that never went through this package: a column
// key carries no source of its own any more, so a dot in it can never mean
// anything.
func TestTheDatabaseRefusesAColumnKeyCarryingADot(t *testing.T) {
	cellStoreDB(t)
	_, err := dbcore.DB.Exec(`INSERT INTO columns (column_key, label, value_type)
		VALUES ('huggingface.author','Author','text')`)
	if err == nil {
		t.Fatal("the database stored a column key carrying a dot")
	}
}

// TestASourceColumnRefusesDeletionAndAHandOnlyColumnAllowsIt is answer A1: an
// automatic source states its columns again on every read, so one is hidden
// and never deleted, and research beside it changes nothing; the reader's own
// hand-only column is theirs to remove.
func TestASourceColumnRefusesDeletionAndAHandOnlyColumnAllowsIt(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")
	makeColumn(t, ColumnSpec{Key: "author", Label: "Author", ValueType: ValueTypeText, Sealed: true},
		SourceOpenRouter, SourceHuggingFace)
	makeColumn(t, ColumnSpec{Key: "model_type", Label: "Model type", ValueType: ValueTypeText, Sealed: true},
		SourceOpenRouter, SourceResearch)
	for _, key := range []string{"author", "model_type"} {
		err := DeleteColumn(key)
		if !errors.Is(err, ErrColumnIsASource) {
			t.Errorf("deleting %s: err = %v, want ErrColumnIsASource", key, err)
			continue
		}
		if !strings.Contains(err.Error(), "hidden") {
			t.Errorf("deleting %s must offer hiding instead, got %q", key, err)
		}
		if err := SetColumnVisible(key, false); err != nil {
			t.Errorf("hide %s: %v", key, err)
		}
	}

	makeColumn(t, ColumnSpec{Key: "sizing_notes", Label: "Sizing notes", ValueType: ValueTypeText})
	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{"sizing_notes": "big"}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteColumn("sizing_notes"); err != nil {
		t.Fatalf("delete a hand-only column: %v", err)
	}
	if _, err := ColumnByKey("sizing_notes"); !errors.Is(err, ErrNoSuchColumn) {
		t.Errorf("the deleted column is still there: %v", err)
	}
	var cells int
	if err := dbcore.DB.QueryRow("SELECT COUNT(*) FROM cells").Scan(&cells); err != nil {
		t.Fatal(err)
	}
	if cells != 0 {
		t.Errorf("%d value(s) survived their column being deleted, want none", cells)
	}
}

// TestAnUnknownKeyIsRefusedNotMinted: a key naming no column is refused by
// every writer alike. Making a column is the reader's own act or the
// refresh's, never a side effect of a write, so a typo cannot become a column.
func TestAnUnknownKeyIsRefusedNotMinted(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")

	for _, src := range []Source{SourceHand, SourceResearch, SourceOpenRouter} {
		err := WriteCells(src, "Qwen3 32B", map[string]string{"invented": "x"})
		if !errors.Is(err, ErrNoSuchColumn) {
			t.Errorf("%s writing a key with no column: err = %v, want ErrNoSuchColumn", src, err)
		}
	}
	if _, err := ColumnByKey("invented"); !errors.Is(err, ErrNoSuchColumn) {
		t.Errorf("a column was made on the way past: %v", err)
	}
}

// TestDeclareColumnSourcesRanksByListPosition pins the rank rule: the rank is
// the position in the list, from 0, and no source gets a special number.
func TestDeclareColumnSourcesRanksByListPosition(t *testing.T) {
	cellStoreDB(t)
	column := makeColumn(t, ColumnSpec{Key: "moe", Label: "MoE", ValueType: ValueTypeFlag},
		SourceHuggingFace, SourceOpenRouter, SourceResearch)
	rows, err := dbcore.DB.Query("SELECT source, rank FROM column_sources WHERE column_id=? ORDER BY rank ASC", column.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var got []string
	for rows.Next() {
		var src string
		var rank int
		if err := rows.Scan(&src, &rank); err != nil {
			t.Fatal(err)
		}
		got = append(got, src+"="+strconv.Itoa(rank))
	}
	if strings.Join(got, ",") != "huggingface=0,openrouter=1,research=2" {
		t.Errorf("ranks = %v, want huggingface=0,openrouter=1,research=2", got)
	}
}

// TestDeclareColumnSourcesRefusesHandBesideAFetchedSource: a column of the
// reader's own is stated by nobody else, and the app's stamp likewise, so hand
// and app are declared alone or not at all.
func TestDeclareColumnSourcesRefusesHandBesideAFetchedSource(t *testing.T) {
	cellStoreDB(t)
	for _, sources := range [][]Source{
		{SourceOpenRouter, SourceHand},
		{SourceHand, SourceResearch},
		{SourceApp, SourceOpenRouter},
	} {
		_, err := CreateColumn(ColumnSpec{Key: "x", Label: "X", ValueType: ValueTypeText}, sources...)
		if !errors.Is(err, ErrColumnKey) {
			t.Errorf("declaring %v: err = %v, want ErrColumnKey", sources, err)
		}
	}
	if _, err := ColumnByKey("x"); !errors.Is(err, ErrNoSuchColumn) {
		t.Errorf("a refused declaration left the column behind: %v", err)
	}
}

// TestDeclareColumnSourcesRefusesResearchAnywhereButLast: research fills a gap
// only until an automatic source states the fact, so it can never outrank one.
// A source named twice is refused for the same reason a rank is unique.
func TestDeclareColumnSourcesRefusesResearchAnywhereButLast(t *testing.T) {
	cellStoreDB(t)
	for _, sources := range [][]Source{
		{SourceResearch, SourceOpenRouter},
		{SourceOpenRouter, SourceResearch, SourceHuggingFace},
		{SourceOpenRouter, SourceOpenRouter},
	} {
		_, err := CreateColumn(ColumnSpec{Key: "x", Label: "X", ValueType: ValueTypeText}, sources...)
		if !errors.Is(err, ErrColumnKey) {
			t.Errorf("declaring %v: err = %v, want ErrColumnKey", sources, err)
		}
	}
	if _, err := CreateColumn(ColumnSpec{Key: "x", Label: "X", ValueType: ValueTypeText}, SourceOpenRouter, SourceResearch); err != nil {
		t.Errorf("research last was refused: %v", err)
	}
}

// TestNoStoredCellDisagreesWithItsColumnsKind is the invariant across the whole
// table, rather than one write at a time: no cell of a number or flag column may
// be missing its sort key, and no cell of a choice column may sort by anything
// that is not one of that column's option positions.
func TestNoStoredCellDisagreesWithItsColumnsKind(t *testing.T) {
	cellStoreDB(t)
	makeColumn(t, ColumnSpec{Key: "size", Label: "Size", ValueType: ValueTypeNumber})
	makeColumn(t, ColumnSpec{Key: "tool", Label: "Tool", ValueType: ValueTypeFlag})
	makeColumn(t, ColumnSpec{Key: "speed", Label: "Speed", ValueType: ValueTypeChoice,
		Options: []string{"fast", "avg", "slow"}})
	makeColumn(t, ColumnSpec{Key: "note", Label: "Note", ValueType: ValueTypeText})
	for _, model := range []string{"A", "B"} {
		addModelRow(t, model)
		if err := WriteCells(SourceHand, model, map[string]string{
			"size":  "32.5",
			"tool":  "no",
			"speed": "slow",
			"note":  "a note",
		}); err != nil {
			t.Fatal(err)
		}
	}

	var missing int
	if err := dbcore.DB.QueryRow(`SELECT COUNT(*) FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.value_type IN ('number','flag','choice') AND v.value_number IS NULL`).Scan(&missing); err != nil {
		t.Fatal(err)
	}
	if missing != 0 {
		t.Errorf("%d cell(s) of a sorted column have no sort key", missing)
	}

	var spare int
	if err := dbcore.DB.QueryRow(`SELECT COUNT(*) FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.value_type='text' AND v.value_number IS NOT NULL`).Scan(&spare); err != nil {
		t.Fatal(err)
	}
	if spare != 0 {
		t.Errorf("%d text cell(s) carry a sort key, which nothing would order by", spare)
	}

	var stray int
	if err := dbcore.DB.QueryRow(`SELECT COUNT(*) FROM cells v JOIN columns c ON c.id=v.column_id
		WHERE c.value_type='choice' AND v.value_number NOT IN
		  (SELECT position FROM column_options o WHERE o.column_id=c.id)`).Scan(&stray); err != nil {
		t.Fatal(err)
	}
	if stray != 0 {
		t.Errorf("%d choice cell(s) sort by something that is not one of the column's option positions", stray)
	}
}

// TestWritingACellNeedsAModelThatExists: a cell names its model by name, so a
// name nothing holds is a write with nothing to attach to.
func TestWritingACellNeedsAModelThatExists(t *testing.T) {
	cellStoreDB(t)
	makeColumn(t, ColumnSpec{Key: "note", Label: "Note", ValueType: ValueTypeText})
	if err := WriteCells(SourceHand, "not here", map[string]string{"note": "x"}); !errors.Is(err, ErrNoSuchModel) {
		t.Fatalf("err = %v, want ErrNoSuchModel", err)
	}
}

// TestACellDiesWithItsModel: cells.model_name REFERENCES models(name) ON DELETE
// CASCADE, so deleting a model takes every value of it away and nothing undoes
// that.
func TestACellDiesWithItsModel(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Qwen3 32B")
	makeColumn(t, ColumnSpec{Key: "note", Label: "Note", ValueType: ValueTypeText})
	if err := WriteCells(SourceHand, "Qwen3 32B", map[string]string{"note": "x"}); err != nil {
		t.Fatal(err)
	}
	if _, err := dbcore.DB.Exec("DELETE FROM models WHERE name=?", "Qwen3 32B"); err != nil {
		t.Fatal(err)
	}
	var cells int
	if err := dbcore.DB.QueryRow("SELECT COUNT(*) FROM cells").Scan(&cells); err != nil {
		t.Fatal(err)
	}
	if cells != 0 {
		t.Errorf("%d cell(s) outlived their model", cells)
	}
}

// TestACreatedColumnKeepsWhatItWasGiven covers the definition itself, including
// the two shapes that cannot mean anything: a choice column with nothing to
// choose from, and an option list on a column that offers no choice.
func TestACreatedColumnKeepsWhatItWasGiven(t *testing.T) {
	cellStoreDB(t)
	column := makeColumn(t, ColumnSpec{
		Key: "context_length", Label: "Context length", ValueType: ValueTypeNumber,
		Unit: "tokens", About: "How many tokens the model takes", Position: 3, Hidden: true, Sealed: true,
	}, SourceOpenRouter)
	if column.Unit != "tokens" || column.Position != 3 || column.Visible || !column.Sealed {
		t.Errorf("the column came back as %+v", column)
	}

	if _, err := CreateColumn(ColumnSpec{Key: "speed", Label: "Speed", ValueType: ValueTypeChoice}, SourceHand); err == nil {
		t.Error("a choice column with nothing to choose from was created")
	}
	if _, err := CreateColumn(ColumnSpec{Key: "note", Label: "Note", ValueType: ValueTypeText,
		Options: []string{"a", "b"}}, SourceHand); err == nil {
		t.Error("a text column was given options")
	}
	if _, err := CreateColumn(ColumnSpec{Key: "a.author", Label: "Author", ValueType: ValueTypeText}, SourceHand); !errors.Is(err, ErrColumnKey) {
		t.Error("a key carrying a dot was accepted")
	}
	if _, err := CreateColumn(ColumnSpec{Key: "x", Label: "X", ValueType: "colour"}, SourceHand); err == nil {
		t.Error("a column was created with a kind of value this app does not store")
	}
	if _, err := CreateColumn(ColumnSpec{Key: "no_source", Label: "No source", ValueType: ValueTypeText}); err == nil {
		t.Error("a column was created with no declared source")
	}

	columns, err := ListColumns()
	if err != nil {
		t.Fatal(err)
	}
	created := 0
	for _, c := range columns {
		if c.Sealed {
			created++
		}
	}
	if created != 1 {
		t.Errorf("%d column(s) were created, want the one that was", created)
	}
}
