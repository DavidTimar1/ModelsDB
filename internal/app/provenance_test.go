package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"modelsdb/internal/store"
)

// servedModelAnswers drives GET /api/models/{name}/answers/{column} directly
// against the handler and decodes the reply.
func servedModelAnswers(t *testing.T, name, column string) (store.ColumnAnswers, int) {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/models/"+url.PathEscape(name)+"/answers/"+column, nil)
	req.SetPathValue("name", name)
	req.SetPathValue("column", column)
	rec := httptest.NewRecorder()
	handleModelAnswers(rec, req)
	if rec.Code != http.StatusOK {
		return store.ColumnAnswers{}, rec.Code
	}
	var out store.ColumnAnswers
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("GET .../answers/%s body is not a ColumnAnswers: %v (%s)", column, err, rec.Body.String())
	}
	return out, rec.Code
}

// TestModelAnswersRanksEverySourceAndPicksAWinner seeds a column two sources
// state and confirms the automatic source outranks a researched value: both
// answers are carried, in the column's fixed order, each with the moment it
// was written, and "winner" names OpenRouter.
func TestModelAnswersRanksEverySourceAndPicksAWinner(t *testing.T) {
	migratedDB(t)

	if err := store.AddModelWithCells("Qwen: Qwen3-32B", map[string]string{}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if err := store.WriteCells(store.SourceResearch, "Qwen: Qwen3-32B", map[string]string{"context_length": "8192"}); err != nil {
		t.Fatalf("write research's context_length: %v", err)
	}
	if err := store.WriteCells(store.SourceOpenRouter, "Qwen: Qwen3-32B", map[string]string{"context_length": "40960"}); err != nil {
		t.Fatalf("write openrouter's context_length: %v", err)
	}

	answers, status := servedModelAnswers(t, "Qwen: Qwen3-32B", "context_length")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	if answers.Winner != string(store.SourceOpenRouter) {
		t.Errorf("winner = %q, want %q", answers.Winner, store.SourceOpenRouter)
	}
	if len(answers.Answers) != 3 {
		t.Fatalf("answers = %v, want openrouter, huggingface and research", answers.Answers)
	}
	if answers.Answers[0].Source != string(store.SourceOpenRouter) || answers.Answers[2].Source != string(store.SourceResearch) {
		t.Errorf("answers are ordered %q..%q, want openrouter first and research last", answers.Answers[0].Source, answers.Answers[2].Source)
	}
	if answers.Answers[0].Value == nil || *answers.Answers[0].Value != "40960" || answers.Answers[0].WrittenAt == "" {
		t.Errorf("openrouter's answer = %+v, want \"40960\" with a written_at", answers.Answers[0])
	}
	if answers.Answers[2].Value == nil || *answers.Answers[2].Value != "8192" || answers.Answers[2].WrittenAt == "" {
		t.Errorf("research's answer = %+v, want \"8192\" with a written_at, untouched by the refresh", answers.Answers[2])
	}
	if answers.Answers[1].Value != nil || answers.Answers[1].WrittenAt != "" {
		t.Errorf("huggingface, which has not answered, = %+v, want a null value and no date", answers.Answers[1])
	}
}

// TestTheAnswersPanelCarriesWrittenAt: every answer the panel draws is dated
// by the cell it came from, in the stamp the store writes.
func TestTheAnswersPanelCarriesWrittenAt(t *testing.T) {
	migratedDB(t)
	if err := store.AddModelWithCells("Qwen: Qwen3-32B", map[string]string{"parameters": "32.76"}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	answers, status := servedModelAnswers(t, "Qwen: Qwen3-32B", "parameters")
	if status != http.StatusOK {
		t.Fatalf("status = %d, want 200", status)
	}
	for _, a := range answers.Answers {
		if a.Source == string(store.SourceResearch) {
			if _, err := time.Parse(time.RFC3339, a.WrittenAt); err != nil {
				t.Errorf("research's written_at = %q, want an RFC3339 stamp: %v", a.WrittenAt, err)
			}
		} else if a.WrittenAt != "" {
			t.Errorf("%s has not answered and carries written_at %q", a.Source, a.WrittenAt)
		}
	}
}

// TestModelAnswersRefusesAnUnknownModelOrColumn pins the two 4xx refusals: a
// column nothing declares, and a model this catalog has never seen.
func TestModelAnswersRefusesAnUnknownModelOrColumn(t *testing.T) {
	migratedDB(t)

	if err := store.AddModelWithCells("Qwen: Qwen3-32B", map[string]string{}); err != nil {
		t.Fatalf("AddModel: %v", err)
	}

	if _, status := servedModelAnswers(t, "Qwen: Qwen3-32B", "not_a_column"); status < 400 {
		t.Errorf("status for an unknown column = %d, want a 4xx refusal", status)
	}
	if _, status := servedModelAnswers(t, "Nobody Here", "context_length"); status < 400 {
		t.Errorf("status for an unknown model = %d, want a 4xx refusal", status)
	}
}
