package store

import "testing"

// The type a fact is listed under is the kind of value its column holds, for
// every source that states it.
func TestEveryFactIsListedUnderTheKindItsColumnHolds(t *testing.T) {
	seedInto(t)
	for _, fact := range Facts {
		for _, source := range fact.Sources {
			key := sourceCellKeyOf(fact.Key, source)
			if key == "" {
				continue
			}
			column, err := ColumnByKey(key)
			if err != nil {
				t.Errorf("%s: its column %s: %v", fact.Key, key, err)
				continue
			}
			if column.ValueType != fact.Type {
				t.Errorf("%s is listed as %s, but %s holds %s", fact.Key, fact.Type, key, column.ValueType)
			}
		}
	}
}

// RecordFacts names only what was really stored: a value the column refuses is
// not claimed as kept.
func TestRecordFactsDoesNotClaimAValueTheColumnRefused(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Repo model")

	kept, err := RecordFacts("Repo model", SourceHuggingFace, map[string]string{
		"parameters":      "7.25",
		"weights_size_gb": "not a number",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(kept) != 1 || kept[0] != "parameters" {
		t.Errorf("kept = %v, want only parameters", kept)
	}
	if got := cellsOf(t, "Repo model")["weights_size_gb"]; got != "" {
		t.Errorf("a refused value was stored: %q", got)
	}
}
