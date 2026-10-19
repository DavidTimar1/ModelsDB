package store

import (
	"modelsdb/internal/dbcore"
	"testing"
)

// cellsOf reads back every value stored for one model, keyed by column key, and
// fails the test if it cannot.
func cellsOf(t *testing.T, model string) map[string]string {
	t.Helper()
	stored, err := ModelCells(model)
	if err != nil {
		t.Fatalf("read the cells of %q: %v", model, err)
	}
	return stored
}

// wantCell fails unless the model's value for that column is exactly text.
func wantCell(t *testing.T, stored map[string]string, key, text string) {
	t.Helper()
	if stored[key] != text {
		t.Fatalf("%s holds %q, expected %q", key, stored[key], text)
	}
}

// TestARefreshNeverOverwritesAResearchedAnswer holds the write-guard's whole
// promise against one refresh: OpenRouter's answer and a researched answer to
// the same column both land as their own cell, OpenRouter's is the one a read
// answers with, and the researched one is still there beneath it, refresh
// after refresh.
func TestARefreshNeverOverwritesAResearchedAnswer(t *testing.T) {
	cellStoreDB(t)
	if _, err := SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}

	norm := map[string]interface{}{
		"name": "Vendor: A Model", "id": "vendor/a-model", "slug": "vendor/a-model",
		"description": "a model", "created_at": "2026-01-01T00:00:00Z",
		"context_length":     float64(131072),
		"input_modalities":   []interface{}{"text"},
		"output_modalities":  []interface{}{"text"},
		"supports_reasoning": true,
		"endpoint": map[string]interface{}{
			"supports_tool_parameters": true,
			"pricing":                  map[string]interface{}{"prompt": "0.0000004", "completion": "0.0000016"},
		},
	}
	if stored, err := UpsertAPIModel(norm); err != nil || !stored {
		t.Fatalf("UpsertAPIModel: stored=%v err=%v", stored, err)
	}
	if err := WriteCells(SourceResearch, "Vendor: A Model",
		map[string]string{"context_length": "200000"}); err != nil {
		t.Fatalf("write the researched answer: %v", err)
	}
	if _, err := UpsertAPIModel(norm); err != nil {
		t.Fatalf("second UpsertAPIModel: %v", err)
	}

	stored := cellsOf(t, "Vendor: A Model")
	wantCell(t, stored, "id", "vendor/a-model")
	wantCell(t, stored, "slug", "vendor/a-model")
	wantCell(t, stored, "description", "a model")
	wantCell(t, stored, "created_at", "2026-01-01T00:00:00Z")
	// OpenRouter's answer outranks the researched one, and a refresh does not
	// move either.
	wantCell(t, stored, "context_length", "131072")
	if answers, err := AnswersFor("Vendor: A Model", "context_length"); err != nil {
		t.Fatal(err)
	} else {
		var research string
		for _, a := range answers.Answers {
			if a.Source == string(SourceResearch) && a.Value != nil {
				research = *a.Value
			}
		}
		if research != "200000" {
			t.Errorf("the researched context_length = %q after a refresh, want 200000 kept beneath OpenRouter's", research)
		}
	}
	wantCell(t, stored, "input_modalities", `["text"]`)
	wantCell(t, stored, "supports_tools", "yes")
	wantCell(t, stored, "supports_reasoning", "yes")
	wantCell(t, stored, "vendor", "vendor")
	wantCell(t, stored, "price_prompt", "0.0000004")
	wantCell(t, stored, "price_completion", "0.0000016")

	var raw string
	if err := dbcore.DB.QueryRow(`SELECT raw_json FROM source_records WHERE model_name=? AND source=?`,
		"Vendor: A Model", "openrouter").Scan(&raw); err != nil {
		t.Fatalf("read the record OpenRouter sent: %v", err)
	}
	if raw == "" {
		t.Fatalf("the record OpenRouter sent was not stored")
	}
}

// TestASaveByHandWritesAHandCell covers the by-hand write path every editor
// and every agent reaches: POST /api/save and POST /api/models/add both end in
// SaveCells, which writes the cells the request names as a hand answer,
// straight into the same columns a refresh answers through.
func TestASaveByHandWritesAHandCell(t *testing.T) {
	cellStoreDB(t)
	if _, err := SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}
	addModelRow(t, "Typed: Model")

	if err := SaveCells("Typed: Model", map[string]string{
		"notes":          "a note of my own",
		"favorite":       "yes",
		"context_length": "65536",
		"supports_tools": "yes",
		"model_type":     "image",
	}); err != nil {
		t.Fatalf("SaveCells: %v", err)
	}

	stored := cellsOf(t, "Typed: Model")
	wantCell(t, stored, "notes", "a note of my own")
	wantCell(t, stored, "favorite", "yes")
	wantCell(t, stored, "context_length", "65536")
	wantCell(t, stored, "supports_tools", "yes")
	wantCell(t, stored, "model_type", "image")
}

// TestAnImportWritesHuggingFaceCells covers the repository importer: the record
// it builds and the facts it measured both land as HuggingFace-sourced cells.
func TestAnImportWritesHuggingFaceCells(t *testing.T) {
	cellStoreDB(t)
	if _, err := SeedShippedColumns(); err != nil {
		t.Fatalf("seed the shipped columns: %v", err)
	}

	record := map[string]interface{}{
		"name": "Qwen: Qwen3-32B", "hf_slug": "Qwen/Qwen3-32B",
		"description":       "a repository",
		"context_length":    float64(40960),
		"input_modalities":  []string{"text"},
		"output_modalities": []string{"text"},
		"moe":               "no",
	}
	if err := ImportFullRecord(record, "huggingface", "chat"); err != nil {
		t.Fatalf("ImportFullRecord: %v", err)
	}
	if _, err := RecordFacts("Qwen: Qwen3-32B", SourceHuggingFace,
		map[string]string{"parameters": "32.8", "weights_size_gb": "65.52", "base_model": "Qwen/Qwen3-32B-Base"}); err != nil {
		t.Fatalf("RecordFacts: %v", err)
	}

	stored := cellsOf(t, "Qwen: Qwen3-32B")
	wantCell(t, stored, "repository", "Qwen/Qwen3-32B")
	wantCell(t, stored, "context_length", "40960")
	wantCell(t, stored, "output_modalities", `["text"]`)
	wantCell(t, stored, "moe", "no")
	wantCell(t, stored, "parameters", "32.8")
	// A fact the reader keeps no custom column for still lands in the shipped
	// column: what a source states is not a choice, only which columns are drawn.
	wantCell(t, stored, "weights_size_gb", "65.52")
	wantCell(t, stored, "base_model", "Qwen/Qwen3-32B-Base")
}

// TestSeedingTheShippedColumnsAgainChangesNothing is what makes seeding safe to
// run on every start: a column somebody hid stays hidden and nothing is created
// twice.
func TestSeedingTheShippedColumnsAgainChangesNothing(t *testing.T) {
	cellStoreDB(t)
	// The migration chain leaves a database holding the reader's own columns
	// before anything is seeded, so the count to measure against is what was
	// already there plus what the seed made.
	before, err := ListColumns()
	if err != nil {
		t.Fatalf("list the columns: %v", err)
	}
	first, err := SeedShippedColumns()
	if err != nil {
		t.Fatalf("first seed: %v", err)
	}
	if len(first) != len(shippedColumns) {
		t.Fatalf("the first seed created %d column(s), expected %d", len(first), len(shippedColumns))
	}
	if err := SetColumnVisible("tokenizer", true); err != nil {
		t.Fatalf("show a column: %v", err)
	}

	second, err := SeedShippedColumns()
	if err != nil {
		t.Fatalf("second seed: %v", err)
	}
	if len(second) != 0 {
		t.Fatalf("the second seed created %v, expected nothing", second)
	}
	after, err := ListColumns()
	if err != nil {
		t.Fatalf("list the columns: %v", err)
	}
	if len(after) != len(before)+len(shippedColumns) {
		t.Fatalf("the catalog holds %d column(s), expected %d", len(after), len(before)+len(shippedColumns))
	}
	shown, err := ColumnByKey("tokenizer")
	if err != nil {
		t.Fatalf("read the column back: %v", err)
	}
	if !shown.Visible {
		t.Fatalf("a column shown on purpose was hidden again by the seed")
	}
}

// TestEveryMirroredFactLandsInAShippedColumn holds the fact pairing against the
// columns the app really ships, so a fact can never be written towards a column
// that does not exist.
func TestEveryMirroredFactLandsInAShippedColumn(t *testing.T) {
	shipped := map[string]string{}
	for _, c := range shippedColumns {
		shipped[c.Key] = c.ValueType
	}
	for fact, key := range factCellKeys {
		if _, known := shipped[key]; !known {
			t.Fatalf("the fact %q is written into %q, which the app does not ship", fact, key)
		}
		if _, isFact := factByKey[fact]; !isFact {
			t.Fatalf("%q is paired with a column but is not a fact this app reads", fact)
		}
	}
	for _, inherited := range inheritedFacts {
		if _, known := shipped[inherited.into]; !known {
			t.Fatalf("a build inherits %q, which the app does not ship", inherited.into)
		}
		if _, known := shipped[inherited.from]; !known {
			t.Fatalf("a build reads %q from its parent, which the app does not ship", inherited.from)
		}
	}
}
