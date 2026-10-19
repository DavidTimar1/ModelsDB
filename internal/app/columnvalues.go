// GET /api/columns/values: the distinct values one column really
// holds, with how many models hold each.
//
// It exists for the browser's value-list filters: a filter over a column nobody
// can see into is a box you type letters into, and this route is what lets the
// page offer the values that are actually there. The values are computed over
// the WHOLE catalog - hidden rows and variations included - never over the rows
// a filter is currently showing, because a list that shrank as you filtered
// would make the control unusable.

package app

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"sort"
	"strconv"
	"strings"
)

// valuesLimitDefault caps how many values come back when the request names no
// limit of its own, so a column of six hundred one-model values is never
// serialised in full unless it is asked for. The browser's threshold for
// drawing a value list at all is the same figure, read from distinct_total.
const valuesLimitDefault = 200

// columnValue is one distinct value and how many models hold it.
type columnValue struct {
	Value  string `json:"value"`
	Models int    `json:"models"`
}

// columnValuesAnswer is the whole reply: which column, how many distinct values
// it holds in all (after any contains= narrowing, before the limit), whether
// the list below came back short, how many models hold no value in the column
// at all, and the values themselves. Empty is counted over the whole catalog and
// is never narrowed by contains=, because the empty choice is a row of its own
// that no search word describes.
type columnValuesAnswer struct {
	Key           string        `json:"key"`
	DistinctTotal int           `json:"distinct_total"`
	Empty         int           `json:"empty"`
	Truncated     bool          `json:"truncated"`
	Values        []columnValue `json:"values"`
}

// handleColumnValues (GET /api/columns/values) answers ?key=<column key>, with
// ?contains=<letters> narrowing the values (capitals ignored) and ?limit=<n>
// bounding how many come back.
//
// A LIST column is split into its members first, so a column holding
// ["text","image"] offers text and image rather than the JSON of the pair. A
// CHOICE column answers its declared options in their declared order, even
// where no model holds one, because the order of a choice column is a scale.
// Every other column answers its stored values, most-held first.
func handleColumnValues(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	q := r.URL.Query()
	key := strings.TrimSpace(q.Get("key"))
	if key == "" {
		http.Error(w, `Which column? Pass ?key=<column key> - GET /api/columns lists every one.`, http.StatusBadRequest)
		return
	}
	column, err := store.ColumnByKey(key)
	if err != nil {
		columns, lerr := store.ListColumns()
		if lerr != nil {
			log.Printf("ERROR [%s %q] list columns: %v", r.Method, r.URL.Path, lerr)
			http.Error(w, fmt.Sprintf("Failed to list the columns: %v", lerr), http.StatusInternalServerError)
			return
		}
		log.Printf("ok /api/columns/values refused: no column %q", key)
		http.Error(w, fmt.Sprintf("%q is not a column this catalog holds. GET /api/columns lists every one.%s",
			key, suggestions(key, columns)), http.StatusBadRequest)
		return
	}

	limit := valuesLimitDefault
	if raw := strings.TrimSpace(q.Get("limit")); raw != "" {
		n, err := strconv.Atoi(raw)
		if err != nil || n < 1 {
			http.Error(w, fmt.Sprintf("limit takes a positive whole number, and %q is not one. Leave it out for the default of %d.",
				raw, valuesLimitDefault), http.StatusBadRequest)
			return
		}
		limit = n
	}
	contains := parseSearch(q.Get("contains"))

	values, err := distinctColumnValues(column, contains)
	if err != nil {
		log.Printf("ERROR [%s %q] values of %s: %v", r.Method, r.URL.Path, column.Key, err)
		http.Error(w, fmt.Sprintf("Failed to read the values of %s: %v", column.Key, err), http.StatusInternalServerError)
		return
	}

	empty, err := modelsWithoutValue(column)
	if err != nil {
		log.Printf("ERROR [%s %q] empty count of %s: %v", r.Method, r.URL.Path, column.Key, err)
		http.Error(w, fmt.Sprintf("Failed to count the models with no value in %s: %v", column.Key, err), http.StatusInternalServerError)
		return
	}

	answer := columnValuesAnswer{Key: column.Key, DistinctTotal: len(values), Empty: empty, Values: values}
	if len(values) > limit {
		answer.Values = values[:limit]
		answer.Truncated = true
	}
	if answer.Values == nil {
		answer.Values = []columnValue{}
	}
	log.Printf("ok /api/columns/values: served %d/%d values of %s (contains=%q limit=%d)",
		len(answer.Values), answer.DistinctTotal, column.Key, q.Get("contains"), limit)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(answer)
}

// modelsWithoutValue counts the models, hidden rows and variations included,
// that hold no cell in the column: the models a filter asking for the empty
// value returns. It reads the cells table directly, as distinctColumnValues
// does, so a model answered by two sources is still counted once.
func modelsWithoutValue(column store.Column) (int, error) {
	var n int
	err := dbcore.DB.QueryRow(`SELECT COUNT(*) FROM models m WHERE NOT EXISTS (
		SELECT 1 FROM cells c WHERE c.column_id = ? AND c.model_name = m.name AND TRIM(c.value_text) <> '')`,
		column.ID).Scan(&n)
	return n, err
}

// distinctColumnValues reads one column's stored values over the whole catalog
// and groups them, narrowed by contains. A list column is
// split into members, with a model counted once per member it holds; a choice
// column answers its declared options in order, counting zero where no model
// holds one; everything else is grouped as stored, most-held first.
func distinctColumnValues(column store.Column, contains searchExpr) ([]columnValue, error) {
	rows, err := dbcore.DB.Query("SELECT value_text, COUNT(*) FROM cells WHERE column_id=? GROUP BY value_text", column.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	counts := map[string]int{}
	var order []string
	note := func(value string, n int) {
		if _, seen := counts[value]; !seen {
			order = append(order, value)
		}
		counts[value] += n
	}
	for rows.Next() {
		var value string
		var n int
		if err := rows.Scan(&value, &n); err != nil {
			return nil, err
		}
		if column.ValueType == store.ValueTypeList {
			for _, member := range dbcore.ParseArr(value) {
				if member = strings.TrimSpace(member); member != "" {
					note(member, n)
				}
			}
			continue
		}
		note(value, n)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	if column.ValueType == store.ValueTypeChoice {
		out := make([]columnValue, 0, len(column.Options))
		for _, option := range column.Options {
			if !contains.matches([]string{option}) {
				continue
			}
			out = append(out, columnValue{Value: option, Models: counts[option]})
		}
		return out, nil
	}

	out := make([]columnValue, 0, len(order))
	for _, value := range order {
		if !contains.matches([]string{value}) {
			continue
		}
		out = append(out, columnValue{Value: value, Models: counts[value]})
	}
	sort.Slice(out, func(a, b int) bool {
		if out[a].Models != out[b].Models {
			return out[a].Models > out[b].Models
		}
		return strings.ToLower(out[a].Value) < strings.ToLower(out[b].Value)
	})
	return out, nil
}
