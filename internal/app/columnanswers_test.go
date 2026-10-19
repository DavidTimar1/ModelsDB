package app

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"modelsdb/internal/store"
)

// servedAnswers drives GET /api/columns/answers and returns the decoded reply,
// failing the test when the request was refused.
func servedAnswers(t *testing.T) columnAnswersReply {
	t.Helper()
	rec := httptest.NewRecorder()
	handleColumnAnswers(rec, httptest.NewRequest(http.MethodGet, "/api/columns/answers", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/columns/answers status = %d, body %s", rec.Code, rec.Body.String())
	}
	var reply columnAnswersReply
	if err := json.Unmarshal(rec.Body.Bytes(), &reply); err != nil {
		t.Fatalf("GET /api/columns/answers body is not the reply shape: %v (%s)", err, rec.Body.String())
	}
	return reply
}

// TestAnswerCountsAreOneNumberPerColumn seeds three models over three answered
// columns and pins the arithmetic the browser prints: a count per column, a
// column nobody answered reported as zero rather than left out, and the model
// count the same reply carries.
func TestAnswerCountsAreOneNumberPerColumn(t *testing.T) {
	migratedDB(t)

	write := func(name string, src store.Source, values map[string]string) {
		t.Helper()
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil && !strings.Contains(err.Error(), "already") {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
		if err := store.WriteCells(src, name, values); err != nil {
			t.Fatalf("write %s cells on %q: %v", src, name, err)
		}
	}

	// A column of the reader's own, so the test names a column no seeding
	// touches and no shipped default can fill.
	if _, err := store.CreateColumn(store.ColumnSpec{
		Key: "architecture_class", Label: "Architecture class", ValueType: store.ValueTypeText,
	}, store.SourceHand); err != nil {
		t.Fatalf("create the unanswered column: %v", err)
	}

	write("Claude Opus", store.SourceOpenRouter, map[string]string{
		"context_length": "200000",
		"price_prompt":   "0.000003",
	})
	write("Gemini Flash", store.SourceOpenRouter, map[string]string{
		"context_length": "1000000",
	})
	write("Gemini Flash", store.SourceHand, map[string]string{
		"notes": "fast enough for a batch job",
	})
	write("Flux Image", store.SourceHand, map[string]string{
		"notes": "prices per image",
	})

	reply := servedAnswers(t)

	if reply.Models != 3 {
		t.Errorf("models = %d, want the 3 seeded models", reply.Models)
	}
	want := map[string]int{
		"context_length":     2,
		"price_prompt":       1,
		"notes":              2,
		"architecture_class": 0,
	}
	for key, n := range want {
		got, present := reply.Answers[key]
		if !present {
			t.Errorf("answers has no entry for %q; every column is reported, a zero one included", key)
			continue
		}
		if got != n {
			t.Errorf("answers[%q] = %d, want %d", key, got, n)
		}
	}

	// The reply covers every defined column, not only the answered ones.
	columns, err := store.ListColumns()
	if err != nil {
		t.Fatalf("read the columns: %v", err)
	}
	if len(reply.Answers) != len(columns) {
		t.Errorf("answers holds %d columns, want all %d defined ones", len(reply.Answers), len(columns))
	}
}

// TestAnAnswerCountIsCellsNotModelsWithText pins the premise the counting rests
// on: a cell cleared to the empty string is deleted rather than stored, so the
// count falls without AnswerCounts testing any value.
func TestAnAnswerCountIsCellsNotModelsWithText(t *testing.T) {
	migratedDB(t)

	for _, name := range []string{"Claude Opus", "Gemini Flash"} {
		if err := store.AddModelWithCells(name, map[string]string{}); err != nil {
			t.Fatalf("AddModel(%q): %v", name, err)
		}
		if err := store.WriteCells(store.SourceHand, name, map[string]string{"notes": "a note"}); err != nil {
			t.Fatalf("write a note on %q: %v", name, err)
		}
	}
	if got := servedAnswers(t).Answers["notes"]; got != 2 {
		t.Fatalf("answers[notes] = %d after two notes, want 2", got)
	}

	if err := store.WriteCells(store.SourceHand, "Gemini Flash", map[string]string{"notes": "   "}); err != nil {
		t.Fatalf("clear the note: %v", err)
	}
	if got := servedAnswers(t).Answers["notes"]; got != 1 {
		t.Errorf("answers[notes] = %d after one note was cleared, want 1", got)
	}
}
