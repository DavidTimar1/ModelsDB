// Finding a model by a name that is close but not exact.
//
// This is the SAME matcher modelquery.go already uses to suggest a column key
// for a misspelt parameter (closeness, best, sharesAWord, editDistance, all in
// modelquery.go), reused here for model names rather than column keys, plus one
// addition: FOLDING, which splits a name into the parts a person actually reads
// it by, so "Qwen3-32B" and "Qwen-3-32B" - punctuated differently but the same
// name - score as identical. No edit-distance function of its own lives here;
// closenessReason in modelquery.go is the only one, shared by both callers.
//
// UNICODE NORMALIZATION IS NOT APPLIED. Go's standard library carries no Unicode
// canonical-composition step; that lives in golang.org/x/text/unicode/norm, which
// is not a dependency of this project. Folding here lowercases and tokenizes
// only.

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// The parameters GET /api/models/similar takes.
const (
	similarParamName          = "name"
	similarParamLimit         = "limit"
	similarParamIncludeHidden = "include_hidden"
)

const (
	similarDefaultLimit = 10
	similarMaxLimit     = 50
)

// foldName splits a model name into the tokens a person reads it by: lowercase,
// with every one of / - _ . : and space a separator, and a boundary between a
// letter and a digit a separator too. "Qwen/Qwen3-32B-FP8" folds to
// ["qwen","qwen","3","32","b","fp","8"], which is what lets "Qwen3-32B" and
// "Qwen-3-32B" fold to the same list and score as the same name.
func foldName(name string) []string {
	lower := strings.ToLower(name)
	isSeparator := func(r rune) bool {
		switch r {
		case '/', '-', '_', '.', ':', ' ':
			return true
		}
		return false
	}
	// letterOrDigit classifies a rune for the letter/digit boundary rule; a rune
	// that is neither (punctuation this app does not treat as a separator) never
	// forces a split on its own, so it rides with whichever side it fell on.
	letterOrDigit := func(r rune) rune {
		switch {
		case unicode.IsDigit(r):
			return 'd'
		case unicode.IsLetter(r):
			return 'l'
		}
		return 0
	}

	var tokens []string
	var current strings.Builder
	var currentClass rune
	flush := func() {
		if current.Len() > 0 {
			tokens = append(tokens, current.String())
			current.Reset()
			currentClass = 0
		}
	}
	for _, r := range lower {
		if isSeparator(r) {
			flush()
			continue
		}
		class := letterOrDigit(r)
		if current.Len() > 0 && class != 0 && currentClass != 0 && class != currentClass {
			flush()
		}
		current.WriteRune(r)
		if class != 0 {
			currentClass = class
		}
	}
	flush()
	return tokens
}

// nameCloseness scores how alike two model names are, the way closenessReason
// in modelquery.go scores two words, extended to treat two differently
// punctuated spellings of the same name as identical. The score is the
// BEST of closeness on the two raw names and closeness on the two folded
// strings (each name's tokens joined by single spaces) - lower wins, and a
// folded pair that is token-for-token equal scores 0 through this same path,
// since closenessReason already scores two equal strings 0.
func nameCloseness(a, b string) (int, string) {
	rawScore, rawWhy := closenessReason(a, b)
	foldedA := strings.Join(foldName(a), " ")
	foldedB := strings.Join(foldName(b), " ")
	foldedScore, foldedWhy := closenessReason(foldedA, foldedB)
	if rawScore < 0 {
		return foldedScore, foldedWhy
	}
	if foldedScore < 0 || rawScore <= foldedScore {
		return rawScore, rawWhy
	}
	return foldedScore, foldedWhy
}

// similarModel is one row GET /api/models/similar, or the "similar" advisory
// on a successful add or import, answers about one close-but-not-identical
// model name.
type similarModel struct {
	Name      string `json:"name"`
	Score     int    `json:"score"`
	Why       string `json:"why"`
	Hidden    bool   `json:"hidden"`
	VariantOf string `json:"variant_of"`
}

// rankSimilarModels scores every row in rows against name, leaves out anything
// that does not match at all (a negative nameCloseness score) and anything
// named exclude (case-sensitive, exactly as models.name compares), and answers
// the best limit of what is left - lower score first, ties broken by name so
// the same catalog always answers in the same order.
func rankSimilarModels(name string, rows []map[string]interface{}, limit int, exclude string) []similarModel {
	out := make([]similarModel, 0, len(rows))
	for _, row := range rows {
		rowName, _ := row["name"].(string)
		if rowName == "" || rowName == exclude {
			continue
		}
		score, why := nameCloseness(name, rowName)
		if score < 0 {
			continue
		}
		hidden, _ := row["hidden"].(bool)
		variantOf, _ := row["variant_of"].(string)
		out = append(out, similarModel{Name: rowName, Score: score, Why: why, Hidden: hidden, VariantOf: variantOf})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Score != out[j].Score {
			return out[i].Score < out[j].Score
		}
		return out[i].Name < out[j].Name
	})
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// hiddenFilteredRows answers the rows processModels built, narrowed to the
// visible ones unless includeHidden asked for the rest too - the same rule
// filterAndProject applies to GET /api/models, reused here so the two routes
// agree on what "hidden" means.
func hiddenFilteredRows(rows []map[string]interface{}, includeHidden bool) []map[string]interface{} {
	if includeHidden {
		return rows
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		if hidden, _ := row["hidden"].(bool); hidden {
			continue
		}
		out = append(out, row)
	}
	return out
}

// similarQuery is one request's question to GET /api/models/similar, read out
// of its parameters before any row is looked at.
type similarQuery struct {
	name          string
	limit         int
	includeHidden bool
}

// parseSimilarQuery reads GET /api/models/similar's three parameters and
// refuses anything else, the way parseModelQuery refuses an unknown parameter
// on GET /api/models: 400, naming it, and offering the parameter names it
// comes closest to.
func parseSimilarQuery(q url.Values) (similarQuery, error) {
	names := make([]string, 0, len(q))
	for name := range q {
		names = append(names, name)
	}
	sort.Strings(names)

	var sq similarQuery
	sq.limit = similarDefaultLimit
	for _, name := range names {
		value := strings.TrimSpace(q.Get(name))
		switch name {
		case similarParamName:
			sq.name = value
		case similarParamLimit:
			n, err := strconv.Atoi(value)
			if err != nil || n < 1 {
				return similarQuery{}, fmt.Errorf("limit takes a whole number of at least 1, and %q is not one", value)
			}
			if n > similarMaxLimit {
				return similarQuery{}, fmt.Errorf("limit takes at most %d, and %q asks for more", similarMaxLimit, value)
			}
			sq.limit = n
		case similarParamIncludeHidden:
			sq.includeHidden = truthyParam(value)
		default:
			return similarQuery{}, unknownSimilarParameter(name)
		}
	}
	if sq.name == "" {
		return similarQuery{}, fmt.Errorf("name is required: GET /api/models/similar?name=<a model name> finds the models whose name is closest to it")
	}
	return sq, nil
}

// unknownSimilarParameter is the refusal a parameter GET /api/models/similar
// does not take gets, naming the three it does and the one it comes closest to.
func unknownSimilarParameter(param string) error {
	valid := []string{similarParamName, similarParamLimit, similarParamIncludeHidden}
	best := ""
	bestScore := -1
	for _, v := range valid {
		if score, _ := closenessReason(v, param); score >= 0 && (bestScore < 0 || score < bestScore) {
			bestScore, best = score, v
		}
	}
	msg := fmt.Sprintf("%q is not a parameter GET /api/models/similar takes. It takes %s, %s and %s.",
		param, similarParamName, similarParamLimit, similarParamIncludeHidden)
	if best != "" {
		msg += fmt.Sprintf(" Did you mean %s?", best)
	}
	return errors.New(msg)
}

// handleSimilarModels (GET /api/models/similar) finds the models in this
// catalog whose name is closest to the one given, best first. It never writes:
// see nameCloseness and rankSimilarModels for how "closest" is decided.
func handleSimilarModels(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	sq, err := parseSimilarQuery(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	merged, err := processModels()
	if err != nil {
		http.Error(w, fmt.Sprintf("Failed to process models: %v", err), http.StatusInternalServerError)
		return
	}
	rows := hiddenFilteredRows(merged, sq.includeHidden)
	out := rankSimilarModels(sq.name, rows, sq.limit, "")
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(out)
}

// similarToJustCreated is the "similar" advisory POST /api/models/add and
// POST /api/models/import carry on a successful write: rows already in the
// catalog - hidden and variations included, since a hint has to see the whole
// catalog to be useful - that look close to the row just created, excluding
// that row itself. It is always present in the answer, an empty array where
// nothing is close, and never turns a successful write into a failed request:
// a lookup that could not run is logged and answered as no matches rather than
// losing the row that was just written.
func similarToJustCreated(name string) []similarModel {
	merged, err := processModels()
	if err != nil {
		log.Printf("ERROR similar-name advisory for %q: %v", name, err)
		return []similarModel{}
	}
	return rankSimilarModels(name, merged, similarDefaultLimit, name)
}
