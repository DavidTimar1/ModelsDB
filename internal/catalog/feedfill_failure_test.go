package catalog

import (
	"encoding/json"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"testing"
)

// One model whose value cannot be written costs only itself: every other model
// still gets the column filled, and the failure is reported afterwards.
func TestOneRefusedFeedValueDoesNotStopTheColumnForTheOtherModels(t *testing.T) {
	catalogTestDB(t)
	if _, err := store.CreateColumn(store.ColumnSpec{Key: "feed_size", Label: "Feed size", ValueType: store.ValueTypeNumber}, store.SourceOpenRouter); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Model A", "Model C"} {
		if _, err := dbcore.DB.Exec("INSERT INTO models (name, source, updated_at) VALUES (?,?,?)", name, "openrouter", "t"); err != nil {
			t.Fatal(err)
		}
	}
	// "Model B" is in the library map but has no row, so writing its cell is refused.
	data := []map[string]interface{}{
		{"name": "Model A", "size": 1.0},
		{"name": "Model B", "size": 2.0},
		{"name": "Model C", "size": 3.0},
	}
	raw := make([]json.RawMessage, len(data))
	for i, m := range data {
		raw[i], _ = json.Marshal(m)
	}
	inLibrary := map[string]bool{"Model A": true, "Model B": true, "Model C": true}

	written, err := FillFeedFieldColumn(data, raw, "size", "feed_size", store.ValueTypeNumber, inLibrary)
	if err == nil {
		t.Error("the refused write was not reported")
	}
	if written != 2 {
		t.Errorf("written = %d, want 2: the models after the refused one were not filled", written)
	}
	for name, want := range map[string]string{"Model A": "1", "Model C": "3"} {
		cells, _ := store.ModelCells(name)
		if cells["feed_size"] != want {
			t.Errorf("%s: feed_size = %q, want %q", name, cells["feed_size"], want)
		}
	}
}
