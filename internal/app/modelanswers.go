// What one model's disagreement panel draws for one column.
//
// GET /api/models/{name}/answers/{column} is the read behind the panel a
// reader opens when a value looks contested: every source the column
// declares, whether or not it has answered, in the order the column prefers
// them. Nothing here writes anything.

package app

import (
	"encoding/json"
	"log"
	"modelsdb/internal/store"
	"net/http"
)

// handleModelAnswers answers GET /api/models/{name}/answers/{column}.
func handleModelAnswers(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	name := r.PathValue("name")
	column := r.PathValue("column")

	answers, err := store.AnswersFor(name, column)
	if err != nil {
		log.Printf("ok /api/models/%q/answers/%q refused: %v", name, column, err)
		http.Error(w, err.Error(), statusOfWriteRefusal(err))
		return
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(answers)
}
