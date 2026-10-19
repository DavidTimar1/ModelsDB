// What a HuggingFace import READ, turned into the facts the store records.
//
// Nothing here decides what is kept. Each builder answers "this is what the
// repository says", keyed by the fact keys in internal/store/facts.go, and
// store.RecordFacts writes each into the huggingface cell of the column that
// states it. A fact the repository does not state is simply absent from the map,
// which is what keeps a blank value from being written for it.

package app

import (
	"fmt"
	"log"
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"strconv"
	"strings"
)

// handleImportFields (GET /api/import-fields) lists every fact this app can
// record about a model, with the column that answers it, the sources that can
// state it, and whether the reader has a column of their own for it. It is what
// the import dialog draws its list from and what an agent reads to learn the
// column names the procedure names.
func handleImportFields(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	choices, err := store.FactChoices()
	if err != nil {
		log.Printf("ERROR [%s %q] list the import facts: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to list what an import can read: %v", err), http.StatusInternalServerError)
		return
	}
	writeJSON(w, choices)
}

// numberFact renders a whole-number fact, leaving a zero or negative figure out
// entirely - "the repository does not say" and "it is zero" are different
// answers, and only one of them belongs in a column.
func numberFact(facts map[string]string, key string, value int) {
	if value > 0 {
		facts[key] = strconv.Itoa(value)
	}
}

// number64Fact is numberFact for a byte count.
func number64Fact(facts map[string]string, key string, value int64) {
	if value > 0 {
		facts[key] = strconv.FormatInt(value, 10)
	}
}

// decimalFact renders a fact that carries decimals, keeping every digit it was
// derived with - the column it goes into is the only copy of the figure.
func decimalFact(facts map[string]string, key string, value float64) {
	if value > 0 {
		facts[key] = strconv.FormatFloat(value, 'f', -1, 64)
	}
}

// textFact records a text fact, leaving an empty one out.
func textFact(facts map[string]string, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		facts[key] = v
	}
}

// hfModelFacts is everything an imported MODEL row carries beyond the fixed
// columns: who published it and how big it is. The architecture is not read
// here: a person or assistant writes the Architecture line afterwards.
func hfModelFacts(m *catalog.HFModel) map[string]string {
	facts := map[string]string{}
	textFact(facts, "author", m.Author)
	decimalFact(facts, "parameters", m.Parameters)
	decimalFact(facts, "weights_size_gb", m.DiskSizeGB())
	textFact(facts, "base_model", m.BaseModel)
	textFact(facts, "gguf_architecture", m.GGUFArchitecture)
	return facts
}

// variationFacts is everything a recorded BUILD carries: which file it is, what
// it was made from, and exactly how big it is.
func variationFacts(detail *catalog.HFRepoDetail, build catalog.HFBuild, format string) map[string]string {
	facts := map[string]string{}
	textFact(facts, "author", detail.Author)
	textFact(facts, "build_revision", detail.Revision)
	textFact(facts, "base_model", detail.BaseModel)
	textFact(facts, "build_format", format)
	if len(build.Files) > 0 {
		facts["build_files"] = strings.Join(build.Files, ", ")
	}
	number64Fact(facts, "build_size_bytes", build.Bytes)
	numberFact(facts, "bits_per_weight", build.Bit)
	textFact(facts, "gguf_architecture", detail.GGUF["architecture"])
	return facts
}
