package catalog

import (
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/store"
	"os"
	"testing"
)

// holdRow makes a row from a route, holding an OpenRouter id.
func holdRow(t *testing.T, name, route, id string) {
	t.Helper()
	if _, err := dbcore.DB.Exec("INSERT INTO models (name, source, updated_at) VALUES (?,?,?)", name, route, dbcore.NowStamp()); err != nil {
		t.Fatal(err)
	}
	if err := store.WriteCells(store.SourceOpenRouter, name, map[string]string{"id": id}); err != nil {
		t.Fatal(err)
	}
}

func snapshotsWritten(t *testing.T) int {
	t.Helper()
	entries, err := os.ReadDir(paths.BackupsDir)
	if os.IsNotExist(err) {
		return 0
	}
	if err != nil {
		t.Fatal(err)
	}
	return len(entries)
}

func renameFeed(t *testing.T) {
	t.Helper()
	dedicatedListingServer(t, `{"data":[
		{"name":"Vendor: Renamed","canonical_slug":"vendor/renamed","id":"vendor/renamed","description":"d"},
		{"name":"Vendor: Settled","canonical_slug":"vendor/settled","id":"vendor/settled","description":"d"},
		{"name":"Vendor: Brand new","canonical_slug":"vendor/new","id":"vendor/new","description":"d"}
	]}`, `{"data":[]}`, `{"data":[]}`)
}

// A renamed model keeps its row, a stale duplicate folds into the named row, a
// brand-new record still counts as created, and the snapshot is written once,
// by the refresh that first changes something.
func TestARefreshFollowsARenameAndMergesADuplicateAfterOneSnapshot(t *testing.T) {
	catalogTestDB(t)
	holdRow(t, "Vendor: Old name", "openrouter", "vendor/renamed")
	if err := store.WriteCells(store.SourceHand, "Vendor: Old name", map[string]string{"notes": "my note"}); err != nil {
		t.Fatal(err)
	}
	holdRow(t, "Vendor: Settled", "openrouter", "vendor/settled")
	holdRow(t, "Vendor: Settled (stale)", "openrouter", "vendor/settled")
	if err := store.WriteCells(store.SourceHand, "Vendor: Settled (stale)", map[string]string{"notes": "stale note"}); err != nil {
		t.Fatal(err)
	}
	holdRow(t, "Vendor: Collection twin", "collection", "vendor/settled")
	renameFeed(t)

	created, _, _, err := FetchModels()
	if err != nil {
		t.Fatalf("FetchModels: %v", err)
	}
	if created != 1 {
		t.Errorf("created = %d, want 1: the renamed and merged rows are updated, only the brand-new record is created", created)
	}
	held, _ := heldNames()
	for name, want := range map[string]bool{
		"Vendor: Old name": false, "Vendor: Renamed": true, "Vendor: Settled (stale)": false,
		"Vendor: Settled": true, "Vendor: Collection twin": true, "Vendor: Brand new": true,
	} {
		if held[name] != want {
			t.Errorf("row %q held = %v, want %v", name, held[name], want)
		}
	}
	if cells, _ := store.ModelCells("Vendor: Renamed"); cells["notes"] != "my note" {
		t.Errorf("renamed row's note = %q, want it kept", cells["notes"])
	}
	if cells, _ := store.ModelCells("Vendor: Settled"); cells["notes"] != "stale note" {
		t.Errorf("merged row's note = %q, want the stale row's kept", cells["notes"])
	}
	if got := snapshotsWritten(t); got != 1 {
		t.Fatalf("snapshots after the first refresh = %d, want exactly 1", got)
	}

	// Nothing is left to follow, so the next refresh changes nothing and writes no snapshot.
	created, _, _, err = FetchModels()
	if err != nil {
		t.Fatal(err)
	}
	if created != 0 {
		t.Errorf("second refresh created %d, want 0", created)
	}
	if got := snapshotsWritten(t); got != 1 {
		t.Errorf("snapshots after the second refresh = %d, want still 1", got)
	}
	after, _ := heldNames()
	if len(after) != len(held) {
		t.Errorf("the second refresh changed the row count from %d to %d", len(held), len(after))
	}
}

// A refresh with nothing to rename or merge writes no snapshot at all.
func TestARefreshWithNothingToFollowWritesNoSnapshot(t *testing.T) {
	catalogTestDB(t)
	renameFeed(t)
	if _, _, _, err := FetchModels(); err != nil {
		t.Fatal(err)
	}
	if got := snapshotsWritten(t); got != 0 {
		t.Errorf("snapshots = %d, want none", got)
	}
}
