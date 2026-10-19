package store

import (
	"modelsdb/internal/dbcore"
	"strings"
	"testing"
)

// TestTheAddedColumnShipsLastAndSealed. Two paths create this column - the seed
// on a fresh install, and migration v34 on a catalog carried forward - and they
// have to end at one column. dbcore cannot read the shipped list (it sits below
// store in the import DAG), so the migration carries its own copy of the key,
// the label and the About text, and this is what holds the two in step.
//
// It also holds the column LAST in the shipped list, because that is what the
// migration's "one past the highest position in use" agrees with.
func TestTheAddedColumnShipsLastAndSealed(t *testing.T) {
	seedInto(t)

	if last := shippedColumns[len(shippedColumns)-1].Key; last != AddedColumnKey {
		t.Errorf("the shipped list ends at %s, want %s last", last, AddedColumnKey)
	}
	// dbcore predates the pivot and still creates this column under its old,
	// namespaced key (migration v34, dbcore.AddedColumnKey = "mine.added") - the
	// pivot's own migration (v41, pivotcontract.go) strips every column's
	// namespace prefix afterwards, which is what makes a catalog carried forward
	// converge on the same bare key a fresh seed uses.
	strippedMigrationKey := dbcore.AddedColumnKey[strings.LastIndex(dbcore.AddedColumnKey, ".")+1:]
	if strippedMigrationKey != AddedColumnKey {
		t.Errorf("migration v34's key strips to %s, want %s", strippedMigrationKey, AddedColumnKey)
	}

	column, err := ColumnByKey(AddedColumnKey)
	if err != nil {
		t.Fatalf("%s is not shipped: %v", AddedColumnKey, err)
	}
	if column.Label != dbcore.AddedColumnLabel {
		t.Errorf("the seed labels it %q and the migration %q", column.Label, dbcore.AddedColumnLabel)
	}
	if column.About != dbcore.AddedColumnAbout {
		t.Errorf("the seed and the migration describe it differently:\n seed: %s\n v34:  %s", column.About, dbcore.AddedColumnAbout)
	}
	if column.ValueType != ValueTypeText {
		t.Errorf("it holds %s, and both paths store an RFC3339 stamp as text", column.ValueType)
	}
	appColumns, err := columnsAcceptingSource(SourceApp)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(appColumns, AddedColumnKey) {
		t.Errorf("SourceApp must be declared for %s, and %v is what it may state", AddedColumnKey, appColumns)
	}
	for _, other := range []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch, SourceHand} {
		otherColumns, err := columnsAcceptingSource(other)
		if err != nil {
			t.Fatal(err)
		}
		if contains(otherColumns, AddedColumnKey) {
			t.Errorf("%s may also state %s, and no source but the app may ever reach it", other, AddedColumnKey)
		}
	}
	// Sealed is protection of data nothing can restate: one Delete click would
	// permanently remove every model's arrival date, and no source, refresh or
	// import can put one back.
	if !column.Sealed {
		t.Error("it is not sealed, so a Delete click could destroy dates nothing can state again")
	}
	if column.Visible {
		t.Error("it ships drawn, and the visible set is the nine columns the table has always shown")
	}

	// The last position among the columns the app ships, which is exactly what
	// migration v34 writes on a catalog carried forward. The reader's own columns
	// sit past every shipped one and are not part of the question.
	columns, err := ListColumns()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range columns {
		if c.Sealed && c.Key != column.Key && c.Position > column.Position {
			t.Errorf("%s sits at %d, past %s at %d", c.Key, c.Position, column.Key, column.Position)
		}
	}
}

// TestEveryPathThatCreatesAModelStampsTheDateItArrived, and none of them stamps
// it twice. The date exists in no other file and nothing can work it out again,
// so a second write would quietly turn "when it arrived" into "when it was last
// refreshed" - which models.updated_at already answers.
func TestEveryPathThatCreatesAModelStampsTheDateItArrived(t *testing.T) {
	seedInto(t)

	if err := AddModelWithCells("A hand-added model", map[string]string{"notes": "typed"}); err != nil {
		t.Fatalf("add a model by hand: %v", err)
	}
	stamped := addedDate(t, "A hand-added model")
	if stamped == "" {
		t.Fatal("a model added by hand carries no arrival date")
	}
	if !strings.HasPrefix(stamped, "20") || !strings.Contains(stamped, "T") {
		t.Errorf("the date reads %q, and every path writes an RFC3339 stamp", stamped)
	}

	// A save onto the same row is a write like any other and must not move it.
	if err := SaveCells("A hand-added model", map[string]string{"notes": "typed again"}); err != nil {
		t.Fatal(err)
	}
	if again := addedDate(t, "A hand-added model"); again != stamped {
		t.Errorf("a later write moved the arrival date from %s to %s", stamped, again)
	}

	if _, err := UpsertAPIModel(map[string]interface{}{"name": "An upstream model", "id": "v/one"}); err != nil {
		t.Fatalf("store an upstream listing: %v", err)
	}
	upstream := addedDate(t, "An upstream model")
	if upstream == "" {
		t.Fatal("a model the OpenRouter refresh created carries no arrival date")
	}
	// The refresh runs on every startup, so this is the write that would move.
	if _, err := UpsertAPIModel(map[string]interface{}{"name": "An upstream model", "id": "v/one"}); err != nil {
		t.Fatal(err)
	}
	if again := addedDate(t, "An upstream model"); again != upstream {
		t.Errorf("a second refresh moved the arrival date from %s to %s", upstream, again)
	}

	if err := ImportFullRecord(map[string]interface{}{"name": "An imported model"}, "huggingface", ""); err != nil {
		t.Fatalf("import a repository: %v", err)
	}
	if addedDate(t, "An imported model") == "" {
		t.Error("an imported model carries no arrival date")
	}

	if err := ImportVariation(Variation{
		Parent: "An upstream model",
		Name:   "An upstream model (Q4)",
		HFSlug: "somebody/an-upstream-model-GGUF",
	}); err != nil {
		t.Fatalf("record a build: %v", err)
	}
	if addedDate(t, "An upstream model (Q4)") == "" {
		t.Error("a recorded build carries no arrival date")
	}
}

// TestNoRefreshCanReachTheArrivalDate. It is a mine. column, so the refusal is
// the one WriteCells already makes per call rather than a rule of its own.
func TestNoRefreshCanReachTheArrivalDate(t *testing.T) {
	seedInto(t)
	if err := AddModelWithCells("A model", nil); err != nil {
		t.Fatal(err)
	}
	stamped := addedDate(t, "A model")
	for _, src := range []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch, SourceHand} {
		if err := WriteCells(src, "A model", map[string]string{AddedColumnKey: "1999-01-01T00:00:00Z"}); err == nil {
			t.Errorf("%s wrote the arrival date, which nothing outside this catalog can state", src)
		}
	}
	if again := addedDate(t, "A model"); again != stamped {
		t.Errorf("the arrival date moved from %s to %s", stamped, again)
	}
}

func contains(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}

func addedDate(t *testing.T, model string) string {
	t.Helper()
	cells, err := ModelCells(model)
	if err != nil {
		t.Fatalf("read %q back: %v", model, err)
	}
	return cells[AddedColumnKey]
}
