package store

import (
	"testing"
)

// TestEveryFactsColumnDeclaresItsSourcesAndResearchLast is the guard on the
// write side of the two kinds: ONE column answers a fact for every source that
// can state it, so a source's reading lands where Facts says, and research is
// declared last on every such column so a gap can be filled in until the
// source states it - never the other way round.
func TestEveryFactsColumnDeclaresItsSourcesAndResearchLast(t *testing.T) {
	seedInto(t)
	declared, err := ListColumnSources()
	if err != nil {
		t.Fatal(err)
	}
	for _, fact := range Facts {
		for _, source := range fact.Sources {
			key := sourceCellKeyOf(fact.Key, source)
			if key == "" {
				continue
			}
			sources := declared[key]
			if !containsSource(sources, source) {
				t.Errorf("%s: %s may not write %s, which is where Facts says it lands", fact.Key, source, key)
			}
			if len(sources) == 0 || sources[len(sources)-1] != SourceResearch {
				t.Errorf("%s: %s declares %v, and research must be last so a gap can be filled in", fact.Key, key, sources)
			}
		}
	}
}

// TestEveryFactNamesASourceThatCanStateIt catches a fact added with no source,
// which would be a column nothing could ever fill and a line in the chooser
// offering the reader a choice with no effect.
func TestEveryFactNamesASourceThatCanStateIt(t *testing.T) {
	for _, fact := range Facts {
		if len(fact.Sources) == 0 {
			t.Errorf("%s names no source, so nothing can ever write %q", fact.Key, fact.Column)
		}
		for _, source := range fact.Sources {
			if source != SourceOpenRouter && source != SourceHuggingFace {
				t.Errorf("%s names the unknown source %q", fact.Key, source)
			}
		}
	}
}

// TestOneFactKeyAndOneColumnPerFact catches the copy-paste that gives two facts
// the same key, which would make a saved choice ambiguous, or the same column,
// which would make two facts fight over one cell.
func TestOneFactKeyAndOneColumnPerFact(t *testing.T) {
	keys := map[string]bool{}
	columns := map[string]bool{}
	for _, fact := range Facts {
		if keys[fact.Key] {
			t.Errorf("two facts share the key %q", fact.Key)
		}
		keys[fact.Key] = true
		if columns[fact.Column] {
			t.Errorf("two facts fill the column %q", fact.Column)
		}
		columns[fact.Column] = true
	}
}

// TestEveryFactIsFoundFromTheColumnKeyAlone is what replaced the pairing by
// NAME. A column's KEY is its identity and never moves, so the fact is read
// straight off the key: this asserts that factOfColumnKey agrees with both
// pairings that feed it, and that no key claims a fact this app does not
// record.
func TestEveryFactIsFoundFromTheColumnKeyAlone(t *testing.T) {
	for fact, key := range factCellKeys {
		if found := factOfColumnKey(key); found != fact {
			t.Errorf("the column %q reads as the fact %q, want %q", key, found, fact)
		}
	}
	for fact, perSource := range factCellKeysBySource {
		for _, key := range perSource {
			if found := factOfColumnKey(key); found != fact {
				t.Errorf("the column %q reads as the fact %q, want %q", key, found, fact)
			}
		}
	}
	if factOfColumnKey("notes") != "" {
		t.Error(`"notes" is one of the app's own static columns, holding no catalogue fact, and reads as one anyway`)
	}
}
