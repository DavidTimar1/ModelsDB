// GET /api/columns/answers: how many models have answered each
// column, and how many models the catalog holds.
//
// It exists so the browser can mark a filter control, and print a figure on
// every row of the Columns dialog, without reading the catalog: the answer is
// one small number per column, where the catalog itself is a full read of every
// cell and every source record.

package app

import (
	"encoding/json"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
)

// columnAnswersReply is the whole answer: how many models the catalog holds at
// all, how many of them the table draws as rows of their own (a variation is
// drawn under its parent, so it is not one), and the per-column counts keyed by
// column key.
//
// The model count is here because "no column has an answer" and "there are no
// models" are different states and read differently: the first is a catalog
// nobody has filled in, the second is a catalog nothing has been fetched into.
type columnAnswersReply struct {
	Models   int            `json:"models"`
	TopLevel int            `json:"top_level"`
	Answers  map[string]int `json:"answers"`
}

// handleColumnAnswers (GET /api/columns/answers) answers the whole catalog's
// per-column answer counts. It takes no parameters and only reads.
func handleColumnAnswers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	answers, err := store.AnswerCounts()
	if err != nil {
		log.Printf("ERROR [%s %q] answer counts: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Failed to count the answers: "+err.Error(), http.StatusInternalServerError)
		return
	}
	models, err := dbcore.CountModels()
	if err != nil {
		log.Printf("ERROR [%s %q] count models: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Failed to count the models: "+err.Error(), http.StatusInternalServerError)
		return
	}
	topLevel, err := dbcore.CountTopLevelModels()
	if err != nil {
		log.Printf("ERROR [%s %q] count top-level models: %v", r.Method, r.URL.Path, err)
		http.Error(w, "Failed to count the models: "+err.Error(), http.StatusInternalServerError)
		return
	}
	log.Printf("ok /api/columns/answers: %d columns over %d models", len(answers), models)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(columnAnswersReply{Models: models, TopLevel: topLevel, Answers: answers})
}
