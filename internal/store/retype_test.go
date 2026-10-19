package store

import (
	"modelsdb/internal/dbcore"
	"testing"
)

// addOpenRouterRow makes a row OpenRouter created and gives it the cells a refresh
// would have stored in an earlier release.
func addOpenRouterRow(t *testing.T, name string, cells map[string]string) {
	t.Helper()
	if _, err := dbcore.DB.Exec("INSERT INTO models (name, source, updated_at) VALUES (?,?,?)",
		name, "openrouter", dbcore.NowStamp()); err != nil {
		t.Fatal(err)
	}
	if len(cells) > 0 {
		if err := WriteCells(SourceOpenRouter, name, cells); err != nil {
			t.Fatal(err)
		}
	}
}

// A row the feed no longer lists is typed by the same rule as a listed one.
func TestRederiveOpenRouterTypesRetypesRowsTheFeedNoLongerLists(t *testing.T) {
	seedInto(t)
	addOpenRouterRow(t, "Decider old", map[string]string{"output_modalities": `["decisions"]`, "model_type": "chat"})
	addOpenRouterRow(t, "Flux TTS", map[string]string{"output_modalities": `["audio"]`, "description": "Speaks text aloud.", "model_type": "chat"})
	addOpenRouterRow(t, "ASR old", map[string]string{"output_modalities": `["transcription"]`, "model_type": "chat"})
	addOpenRouterRow(t, "Already right", map[string]string{"output_modalities": `["image"]`, "model_type": "image"})
	addOpenRouterRow(t, "Never typed", map[string]string{"description": "No modalities and no type stated."})
	addOpenRouterRow(t, "No cells at all", nil)
	addModelRow(t, "Hand added") // source manual

	n, err := RederiveOpenRouterTypes()
	if err != nil {
		t.Fatal(err)
	}
	if n != 3 {
		t.Errorf("retyped %d rows, want 3", n)
	}
	for name, want := range map[string]string{
		"Decider old": "decisions", "Flux TTS": "tts", "ASR old": "stt", "Already right": "image",
	} {
		if got := cellsOf(t, name)["model_type"]; got != want {
			t.Errorf("%s: model_type = %q, want %q", name, got, want)
		}
	}
	if got := cellsOf(t, "Never typed")["model_type"]; got != "" {
		t.Errorf("a row that states nothing about its kind was typed %q", got)
	}
	if got := cellsOf(t, "No cells at all")["model_type"]; got != "" {
		t.Errorf("a row with no cells was typed %q", got)
	}
	if got := cellsOf(t, "Hand added")["model_type"]; got != "" {
		t.Errorf("a row OpenRouter did not create was typed %q", got)
	}

	// A second pass finds nothing left to change.
	if n, err := RederiveOpenRouterTypes(); err != nil || n != 0 {
		t.Errorf("second pass: retyped %d, err %v; want 0 and none", n, err)
	}
}
