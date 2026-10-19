package store

import (
	"bytes"
	"database/sql"
	"errors"
	"log"
	"modelsdb/internal/dbcore"
	"os"
	"strings"
	"testing"
)

// captureReconcileLog collects what the package logs for the length of a test.
func captureReconcileLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(os.Stderr) })
	return &buf
}

// orRow makes an OpenRouter row holding an id, with an `added` stamp, as the
// refresh leaves it.
func orRow(t *testing.T, name, id string) {
	t.Helper()
	addOpenRouterRow(t, name, map[string]string{"id": id, "price_prompt": "1"})
	if err := StampAdded(name); err != nil {
		t.Fatal(err)
	}
}

// putCells writes cells as src on a row that exists.
func putCells(t *testing.T, src Source, name string, cells map[string]string) {
	t.Helper()
	if err := WriteCells(src, name, cells); err != nil {
		t.Fatalf("write %v onto %q: %v", cells, name, err)
	}
}

// cellOn reads one cell straight out of the table, empty when there is none.
func cellOn(t *testing.T, model, key string, src Source) string {
	t.Helper()
	var v string
	err := dbcore.DB.QueryRow(`SELECT c.value_text FROM cells c JOIN columns col ON col.id=c.column_id
		WHERE c.model_name=? AND col.column_key=? AND c.source=?`, model, key, string(src)).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return v
}

func rowHeld(t *testing.T, name string) bool {
	t.Helper()
	_, held, err := IdentityOf(name)
	if err != nil {
		t.Fatal(err)
	}
	return held
}

func setAdded(t *testing.T, name, stamp string) {
	t.Helper()
	if _, err := dbcore.DB.Exec(`UPDATE cells SET value_text=? WHERE model_name=? AND source='app'
		AND column_id=(SELECT id FROM columns WHERE column_key='added')`, stamp, name); err != nil {
		t.Fatal(err)
	}
}

func setHidden(t *testing.T, name string, hidden bool) {
	t.Helper()
	if err := SetHidden(name, hidden); err != nil {
		t.Fatal(err)
	}
}

func setRecord(t *testing.T, name string, src Source, raw string) {
	t.Helper()
	storeSourceRecord(src, name, raw)
}

func recordOn(t *testing.T, name string, src Source) string {
	t.Helper()
	var raw string
	err := dbcore.DB.QueryRow("SELECT raw_json FROM source_records WHERE model_name=? AND source=?", name, string(src)).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return ""
	}
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

// databaseIsSound runs the two checks a rewrite must leave passing, through the
// same driver the app uses.
func databaseIsSound(t *testing.T) {
	t.Helper()
	var verdict string
	if err := dbcore.DB.QueryRow("PRAGMA integrity_check").Scan(&verdict); err != nil || verdict != "ok" {
		t.Fatalf("integrity_check = %q, %v", verdict, err)
	}
	rows, err := dbcore.DB.Query("PRAGMA foreign_key_check")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	if rows.Next() {
		t.Fatal("foreign_key_check reports a row pointing at a model that is not there")
	}
}

func TestARenameGivesTheRowItsNewNameAndKeepsEverythingOnIt(t *testing.T) {
	seedInto(t)
	orRow(t, "Old name", "vendor/m1")
	putCells(t, SourceHand, "Old name", map[string]string{"notes": "my note", "favorite": "yes"})
	putCells(t, SourceHuggingFace, "Old name", map[string]string{"parameters": "30"})
	setRecord(t, "Old name", SourceHuggingFace, `{"hf":1}`)
	setRecord(t, "Old name", SourceOpenRouter, `{"or":1}`)
	setHidden(t, "Old name", true)
	setAdded(t, "Old name", "2026-01-01T00:00:00Z")
	if _, err := dbcore.DB.Exec(`INSERT INTO models (name, source, variant_of, hidden, updated_at) VALUES ('Old name q4','huggingface','Old name',1,'t')`); err != nil {
		t.Fatal(err)
	}

	got, err := ReconcileRenamedModel("vendor/m1", "New name", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Renamed != "Old name" || len(got.Merged) != 0 {
		t.Fatalf("result = %+v, want Old name renamed", got)
	}
	if rowHeld(t, "Old name") {
		t.Error("the old name still answers")
	}
	identity, held, _ := IdentityOf("New name")
	if !held || !identity.Hidden || identity.Source != "openrouter" {
		t.Errorf("new row = %+v held=%v, want a hidden openrouter row", identity, held)
	}
	for key, want := range map[string]string{"notes": "my note", "favorite": "yes"} {
		if got := cellOn(t, "New name", key, SourceHand); got != want {
			t.Errorf("hand %s = %q, want %q", key, got, want)
		}
	}
	if got := cellOn(t, "New name", "parameters", SourceHuggingFace); got != "30" {
		t.Errorf("huggingface parameters = %q, want 30", got)
	}
	if got := cellOn(t, "New name", "added", SourceApp); got != "2026-01-01T00:00:00Z" {
		t.Errorf("added = %q, want the original date", got)
	}
	if recordOn(t, "New name", SourceHuggingFace) != `{"hf":1}` || recordOn(t, "New name", SourceOpenRouter) != `{"or":1}` {
		t.Error("a source record did not follow the rename")
	}
	if child, _, _ := IdentityOf("Old name q4"); child.VariantOf != "New name" {
		t.Errorf("variation filed under %q, want New name", child.VariantOf)
	}
	databaseIsSound(t)
}

func TestAMergeFoldsTheStaleRowIntoTheNamedOneWithoutLosingAnAnswer(t *testing.T) {
	seedInto(t)
	orRow(t, "Stale", "vendor/m2")
	orRow(t, "Named", "vendor/m2")
	putCells(t, SourceOpenRouter, "Stale", map[string]string{"price_prompt": "9", "zdr": "yes"})
	putCells(t, SourceHand, "Stale", map[string]string{"notes": "keep me"})
	putCells(t, SourceHuggingFace, "Stale", map[string]string{"parameters": "30", "weights_size_gb": "60"})
	putCells(t, SourceHuggingFace, "Named", map[string]string{"parameters": "31"})
	putCells(t, SourceHand, "Named", map[string]string{"favorite": "yes"})
	setRecord(t, "Stale", SourceHuggingFace, `{"stale":1}`)
	setRecord(t, "Stale", SourceOpenRouter, `{"stale-or":1}`)
	setRecord(t, "Named", SourceOpenRouter, `{"named-or":1}`)
	setAdded(t, "Stale", "2026-01-01T00:00:00Z")
	setAdded(t, "Named", "2026-06-01T00:00:00Z")
	setHidden(t, "Stale", true)
	if _, err := dbcore.DB.Exec(`INSERT INTO models (name, source, variant_of, hidden, updated_at) VALUES ('Stale q4','huggingface','Stale',1,'t')`); err != nil {
		t.Fatal(err)
	}

	got, err := ReconcileRenamedModel("vendor/m2", "Named", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Renamed != "" || len(got.Merged) != 1 || got.Merged[0] != "Stale" {
		t.Fatalf("result = %+v, want Stale merged", got)
	}
	if rowHeld(t, "Stale") {
		t.Error("the stale row is still there")
	}
	if got := cellOn(t, "Named", "notes", SourceHand); got != "keep me" {
		t.Errorf("the stale row's note = %q, want it on the survivor", got)
	}
	if got := cellOn(t, "Named", "favorite", SourceHand); got != "yes" {
		t.Errorf("the survivor's favourite = %q, want it kept", got)
	}
	if got := cellOn(t, "Named", "weights_size_gb", SourceHuggingFace); got != "60" {
		t.Errorf("weights_size_gb = %q, want the stale row's 60 moved over", got)
	}
	if got := cellOn(t, "Named", "parameters", SourceHuggingFace); got != "31" {
		t.Errorf("parameters = %q, want the survivor's own 31 to stand", got)
	}
	if got := cellOn(t, "Named", "price_prompt", SourceOpenRouter); got != "1" {
		t.Errorf("openrouter price = %q, want the survivor's, not the stale 9", got)
	}
	if got := cellOn(t, "Named", "zdr", SourceOpenRouter); got != "" {
		t.Errorf("openrouter zdr = %q, want the stale row's openrouter cell dropped", got)
	}
	if got := cellOn(t, "Named", "added", SourceApp); got != "2026-01-01T00:00:00Z" {
		t.Errorf("added = %q, want the older date", got)
	}
	if recordOn(t, "Named", SourceHuggingFace) != `{"stale":1}` {
		t.Error("the stale row's huggingface record did not move")
	}
	if recordOn(t, "Named", SourceOpenRouter) != `{"named-or":1}` {
		t.Errorf("openrouter record = %q, want the survivor's", recordOn(t, "Named", SourceOpenRouter))
	}
	if identity, _, _ := IdentityOf("Named"); identity.Hidden {
		t.Error("the survivor is hidden although only the stale row was")
	}
	if child, _, _ := IdentityOf("Stale q4"); child.VariantOf != "Named" {
		t.Errorf("variation filed under %q, want Named", child.VariantOf)
	}
	databaseIsSound(t)
}

func TestAMergedRowStaysHiddenOnlyWhenEveryRowWas(t *testing.T) {
	seedInto(t)
	orRow(t, "Stale", "vendor/m3")
	orRow(t, "Named", "vendor/m3")
	setHidden(t, "Stale", true)
	setHidden(t, "Named", true)
	if _, err := ReconcileRenamedModel("vendor/m3", "Named", nil); err != nil {
		t.Fatal(err)
	}
	if identity, _, _ := IdentityOf("Named"); !identity.Hidden {
		t.Error("both rows were hidden, but the survivor is shown")
	}
}

func TestEqualHandAnswersMergeAndDifferingOnesRefuseTheWholeGroup(t *testing.T) {
	seedInto(t)
	orRow(t, "Stale", "vendor/m4")
	orRow(t, "Named", "vendor/m4")
	putCells(t, SourceHand, "Stale", map[string]string{"notes": "same"})
	putCells(t, SourceHand, "Named", map[string]string{"notes": "same"})
	if got, err := ReconcileRenamedModel("vendor/m4", "Named", nil); err != nil || len(got.Merged) != 1 {
		t.Fatalf("equal hand answers: %+v, %v, want a merge", got, err)
	}

	orRow(t, "Stale 2", "vendor/m5")
	orRow(t, "Named 2", "vendor/m5")
	putCells(t, SourceHand, "Stale 2", map[string]string{"notes": "one"})
	putCells(t, SourceHand, "Named 2", map[string]string{"notes": "two"})
	buf := captureReconcileLog(t)
	snapshots := 0
	got, err := ReconcileRenamedModel("vendor/m5", "Named 2", func() error { snapshots++; return nil })
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed() || snapshots != 0 {
		t.Errorf("result = %+v, snapshots = %d, want nothing merged and no snapshot", got, snapshots)
	}
	if !rowHeld(t, "Stale 2") || cellOn(t, "Stale 2", "notes", SourceHand) != "one" || cellOn(t, "Named 2", "notes", SourceHand) != "two" {
		t.Error("a group with differing hand answers was changed")
	}
	if log := buf.String(); !strings.Contains(log, "vendor/m5") || !strings.Contains(log, "notes") {
		t.Errorf("warning = %q, want it to name the id and the column", log)
	}
}

func TestAFeedNameBelongingToAnotherIdChangesNothingAndWarns(t *testing.T) {
	seedInto(t)
	orRow(t, "Old", "vendor/a")
	orRow(t, "Taken", "vendor/b")
	buf := captureReconcileLog(t)
	got, err := ReconcileRenamedModel("vendor/a", "Taken", nil)
	if err != nil {
		t.Fatal(err)
	}
	if got.Changed() || !rowHeld(t, "Old") || !rowHeld(t, "Taken") {
		t.Errorf("result = %+v, want both rows untouched", got)
	}
	if cellOn(t, "Taken", "id", SourceOpenRouter) != "vendor/b" {
		t.Error("the other row's id was disturbed")
	}
	if !strings.Contains(buf.String(), "vendor/a") {
		t.Errorf("warning = %q, want it to name the id", buf.String())
	}
}

func TestARowFromAnotherRouteIsNeverRenamedMergedOrMergedInto(t *testing.T) {
	seedInto(t)
	// A collection row holds the id; the feed's name is free.
	addModelRow(t, "Collection row")
	if _, err := dbcore.DB.Exec("UPDATE models SET source='collection' WHERE name='Collection row'"); err != nil {
		t.Fatal(err)
	}
	putCells(t, SourceOpenRouter, "Collection row", map[string]string{"id": "vendor/c"})
	if got, err := ReconcileRenamedModel("vendor/c", "Fresh name", nil); err != nil || got.Changed() {
		t.Fatalf("collection row holding the id: %+v, %v, want nothing", got, err)
	}
	if !rowHeld(t, "Collection row") || rowHeld(t, "Fresh name") {
		t.Error("the collection row was renamed")
	}

	// An openrouter row holds the id; the feed's name belongs to a collection row.
	orRow(t, "Stale", "vendor/d")
	addModelRow(t, "Collection named")
	if _, err := dbcore.DB.Exec("UPDATE models SET source='collection' WHERE name='Collection named'"); err != nil {
		t.Fatal(err)
	}
	putCells(t, SourceOpenRouter, "Collection named", map[string]string{"id": "vendor/d"})
	if got, err := ReconcileRenamedModel("vendor/d", "Collection named", nil); err != nil || got.Changed() {
		t.Fatalf("collection row bearing the name: %+v, %v, want nothing", got, err)
	}
	if !rowHeld(t, "Stale") || !rowHeld(t, "Collection named") {
		t.Error("a row was merged into or away from a collection row")
	}
}

func TestTwoRowsHoldingTheIdWithTheNameFreeWaitForTheNamedRow(t *testing.T) {
	seedInto(t)
	orRow(t, "One", "vendor/e")
	orRow(t, "Two", "vendor/e")
	if got, err := ReconcileRenamedModel("vendor/e", "Free", nil); err != nil || got.Changed() {
		t.Fatalf("result = %+v, %v, want nothing yet", got, err)
	}
	orRow(t, "Free", "vendor/e")
	got, err := ReconcileRenamedModel("vendor/e", "Free", nil)
	if err != nil || len(got.Merged) != 2 {
		t.Fatalf("result = %+v, %v, want both merged once the named row exists", got, err)
	}
	databaseIsSound(t)
}

func TestTheSnapshotCallbackRunsOnceBeforeAChangeAndNeverWithoutOne(t *testing.T) {
	seedInto(t)
	orRow(t, "Old", "vendor/f")
	calls := 0
	if _, err := ReconcileRenamedModel("vendor/none", "Whatever", func() error { calls++; return nil }); err != nil || calls != 0 {
		t.Fatalf("nothing to follow: calls = %d, err = %v, want no call", calls, err)
	}
	if _, err := ReconcileRenamedModel("vendor/f", "Old", func() error { calls++; return nil }); err != nil || calls != 0 {
		t.Fatalf("the row already bears the name: calls = %d, err = %v, want no call", calls, err)
	}
	refused := errors.New("disk full")
	if _, err := ReconcileRenamedModel("vendor/f", "New", func() error { return refused }); !errors.Is(err, refused) {
		t.Fatalf("err = %v, want the snapshot failure", err)
	}
	if !rowHeld(t, "Old") || rowHeld(t, "New") {
		t.Error("a row was renamed although the snapshot failed")
	}
	if _, err := ReconcileRenamedModel("vendor/f", "New", func() error { calls++; return nil }); err != nil || calls != 1 {
		t.Fatalf("calls = %d, err = %v, want one call before the rename", calls, err)
	}
}

func TestAFollowedRenameIsIdempotent(t *testing.T) {
	seedInto(t)
	orRow(t, "Stale", "vendor/g")
	orRow(t, "Named", "vendor/g")
	if got, err := ReconcileRenamedModel("vendor/g", "Named", nil); err != nil || !got.Changed() {
		t.Fatalf("first: %+v, %v", got, err)
	}
	if got, err := ReconcileRenamedModel("vendor/g", "Named", nil); err != nil || got.Changed() {
		t.Fatalf("second: %+v, %v, want nothing to do", got, err)
	}
	databaseIsSound(t)
}
