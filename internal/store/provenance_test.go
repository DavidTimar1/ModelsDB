package store

import (
	"testing"
)

// TestAnswersForRanksTheAutomaticSourceAboveResearch confirms an automatic
// source outranks a researched value that was there first, that the
// researched value is still readable rather than overwritten, and that every
// answer carries the moment it was written.
func TestAnswersForRanksTheAutomaticSourceAboveResearch(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Test: Model")
	if err := ensureShippedColumns(); err != nil {
		t.Fatalf("ensureShippedColumns: %v", err)
	}
	if err := WriteCells(SourceResearch, "Test: Model", map[string]string{"context_length": "8192"}); err != nil {
		t.Fatalf("write research's context_length: %v", err)
	}
	if err := WriteCells(SourceOpenRouter, "Test: Model", map[string]string{"context_length": "40960"}); err != nil {
		t.Fatalf("write openrouter's context_length: %v", err)
	}

	answers, err := AnswersFor("Test: Model", "context_length")
	if err != nil {
		t.Fatalf("AnswersFor: %v", err)
	}
	if answers.Winner != string(SourceOpenRouter) {
		t.Errorf("winner = %q, want openrouter", answers.Winner)
	}
	var order []string
	for _, a := range answers.Answers {
		order = append(order, a.Source)
		if a.Source == string(SourceResearch) {
			if a.Value == nil || *a.Value != "8192" {
				t.Errorf("research's own value = %v, want \"8192\"", a.Value)
			}
			if a.WrittenAt == "" {
				t.Errorf("research's answer carries no written_at")
			}
		}
		if a.Source == string(SourceHuggingFace) && a.WrittenAt != "" {
			t.Errorf("a source that has not answered carries written_at %q", a.WrittenAt)
		}
	}
	if len(order) != 3 || order[0] != "openrouter" || order[1] != "huggingface" || order[2] != "research" {
		t.Errorf("answers are in the order %v, want openrouter, huggingface, research", order)
	}
	if answers.Answers[0].Rank != 0 || answers.Answers[2].Rank != 2 {
		t.Errorf("ranks = %d..%d, want the declared ranks 0..2", answers.Answers[0].Rank, answers.Answers[2].Rank)
	}
}

// TestADisputeIsOpenRouterAgainstHuggingFaceOnly: the red mark means the two
// automatic sources disagree, and nothing else.
func TestADisputeIsOpenRouterAgainstHuggingFaceOnly(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Test: Model")
	if err := ensureShippedColumns(); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceOpenRouter, "Test: Model", map[string]string{"context_length": "40960", "repository": "acme/one", "description": "Said one way."}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceHuggingFace, "Test: Model", map[string]string{"context_length": "131072", "repository": "acme/one", "description": "Said another way."}); err != nil {
		t.Fatal(err)
	}
	answeredBy, disputed, _, err := AllProvenance()
	if err != nil {
		t.Fatal(err)
	}
	if disputed["Test: Model"]["description"] {
		t.Errorf("two descriptions in different words is not a dispute: %v", disputed)
	}
	if !disputed["Test: Model"]["context_length"] {
		t.Errorf("two automatic sources holding different text is not a dispute: %v", disputed)
	}
	if disputed["Test: Model"]["repository"] {
		t.Errorf("two automatic sources agreeing is a dispute: %v", disputed)
	}
	if answeredBy["Test: Model"]["context_length"] != string(SourceOpenRouter) {
		t.Errorf("context_length answered by %q, want openrouter, the preferred source", answeredBy["Test: Model"]["context_length"])
	}
}

// TestAResearchCellNeverDisputes: a researched value an automatic source has
// overtaken is simply outranked, and a hand cell shares no column with an
// automatic one, so neither ever lights the mark.
func TestAResearchCellNeverDisputes(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Test: Model")
	if err := ensureShippedColumns(); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceResearch, "Test: Model", map[string]string{"context_length": "8192", "parameters": "70"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceOpenRouter, "Test: Model", map[string]string{"context_length": "40960"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceHand, "Test: Model", map[string]string{"notes": "mine"}); err != nil {
		t.Fatal(err)
	}
	answeredBy, disputed, _, err := AllProvenance()
	if err != nil {
		t.Fatal(err)
	}
	if len(disputed["Test: Model"]) != 0 {
		t.Errorf("research beside an automatic source lit a dispute: %v", disputed)
	}
	for key, want := range map[string]string{
		"context_length": string(SourceOpenRouter),
		"parameters":     string(SourceResearch),
		"notes":          string(SourceHand),
	} {
		if answeredBy["Test: Model"][key] != want {
			t.Errorf("%s answered by %q, want %s", key, answeredBy["Test: Model"][key], want)
		}
	}
}

// TestAUnionColumnAnswersMergedWhoeverStatedIt: a union column merges every
// source's list, research included, and answered_by says so.
func TestAUnionColumnAnswersMergedWhoeverStatedIt(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Test: Model")
	if err := ensureShippedColumns(); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceResearch, "Test: Model", map[string]string{"input_modalities": `["audio"]`}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceOpenRouter, "Test: Model", map[string]string{"input_modalities": `["text","image"]`}); err != nil {
		t.Fatal(err)
	}
	answeredBy, _, _, err := AllProvenance()
	if err != nil {
		t.Fatal(err)
	}
	if answeredBy["Test: Model"]["input_modalities"] != "merged" {
		t.Errorf("a union column is answered by %q, want merged", answeredBy["Test: Model"]["input_modalities"])
	}
	// Research alone on a union column is that one source's answer, not a
	// merge: the page reads "merged" as "an automatic source contributed".
	addModelRow(t, "Alone")
	if err := WriteCells(SourceResearch, "Alone", map[string]string{"input_modalities": `["audio"]`}); err != nil {
		t.Fatal(err)
	}
	if answeredBy, _, _, err = AllProvenance(); err != nil {
		t.Fatal(err)
	} else if answeredBy["Alone"]["input_modalities"] != string(SourceResearch) {
		t.Errorf("a union column research alone fills is answered by %q, want research", answeredBy["Alone"]["input_modalities"])
	}
	cells, err := ModelCells("Test: Model")
	if err != nil {
		t.Fatal(err)
	}
	if cells["input_modalities"] != `["text","image","audio"]` {
		t.Errorf("input_modalities = %q, want every source's list merged", cells["input_modalities"])
	}
}

// TestAllCellsAnswersWhatModelCellsAnswersForEveryModel: the batch read merges a
// union column's lists the same way the per-model read does, for a model every
// source states, a model research alone fills, and a model with none.
func TestAllCellsAnswersWhatModelCellsAnswersForEveryModel(t *testing.T) {
	cellStoreDB(t)
	if err := ensureShippedColumns(); err != nil {
		t.Fatal(err)
	}
	addModelRow(t, "Both")
	addModelRow(t, "Alone")
	addModelRow(t, "Bare")
	if err := WriteCells(SourceResearch, "Both", map[string]string{"input_modalities": `["audio"]`}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceOpenRouter, "Both", map[string]string{"input_modalities": `["text","image"]`, "context_length": "40960"}); err != nil {
		t.Fatal(err)
	}
	if err := WriteCells(SourceResearch, "Alone", map[string]string{"input_modalities": `["audio"]`}); err != nil {
		t.Fatal(err)
	}

	all, err := AllCells()
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"Both", "Alone", "Bare"} {
		one, err := ModelCells(name)
		if err != nil {
			t.Fatal(err)
		}
		if len(one) != len(all[name]) {
			t.Errorf("%s: AllCells holds %d cell(s), ModelCells %d", name, len(all[name]), len(one))
		}
		for key, want := range one {
			if all[name][key] != want {
				t.Errorf("%s %s: AllCells = %q, ModelCells = %q", name, key, all[name][key], want)
			}
		}
	}
	if all["Both"]["input_modalities"] != `["text","image","audio"]` {
		t.Errorf("Both input_modalities = %q, want every source's list merged", all["Both"]["input_modalities"])
	}
}
