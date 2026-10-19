// The column list, which is what makes every other answer
// readable.
//
// GET /api/columns names every column this catalog holds: its key, which is how
// a value is written and read and filtered; its label, which is what a person
// sees; what kind of column it is and who writes it; and what kind of value it
// takes. A caller that has this list needs nothing else to read a model row,
// because a row answers under these same keys, and needs nothing else to know
// which columns a write may reach.
//
// TWO KINDS OF COLUMN, SAID ON EVERY ROW OF THE LIST. "kind" is one word:
// fetched, measured or derived for a column an automatic source states (which
// research may fill until it does), stamped for the app's own arrival date,
// yours for a column only the reader writes. "sources" is who may state it,
// in the order a read prefers them, and it is code the server reapplies at
// every start, never something a request changes.
//
// A COLUMN ALSO SAYS WHAT COLOUR ITS CELLS ARE PAINTED. "color_rules" is that
// rule, and its shape follows the kind of value the column holds: ordered
// thresholds on a number, one colour for yes and one for no on a flag, one
// colour per label on a choice. A text and a list column take none, so the
// browser offers none for them.

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"strings"
)

// columnAnswer is one column as this route answers it: the stored definition,
// what kind of column it is, and who may state it.
type columnAnswer struct {
	store.Column
	// One word for what the column is, computed on the server because the
	// page cannot tell a derived column from a fetched one by its sources: the
	// refresh writes both as openrouter.
	Kind string `json:"kind"`
	// Who may state it, in rank order. The first automatic one is preferred;
	// research present means an empty cell may be filled in.
	Sources []store.Source `json:"sources"`
	// What a derived column is worked out from, in the words the page shows.
	DerivedFrom string `json:"derived_from,omitempty"`
	// Whether the column holds a date, which is what makes its filter a range
	// of days: the server answers from: and to: on these columns alone.
	Date bool `json:"date,omitempty"`
	// The feed's own names for what fills this column, so a column can be found
	// by the feed's word for it as well as by its heading: "hugging_face_id" is
	// read into Repository, and "Repository" is not a word anyone types when
	// looking for it. Absent on a column no feed path fills.
	FeedPaths []string `json:"feed_paths,omitempty"`
}

// handleColumns (GET/POST/PATCH/DELETE /api/columns) is the whole column
// surface: GET lists them, POST creates one of your own, PATCH changes what one
// is called or what colour it paints its cells, and DELETE removes one of yours
// together with every model's value for it.
func handleColumns(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		answerColumnList(w, r)
	case http.MethodPost:
		createColumn(w, r)
	case http.MethodPatch:
		changeColumn(w, r)
	case http.MethodDelete:
		deleteColumn(w, r)
	default:
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// answerColumnList answers every column, each with its kind and its sources.
func answerColumnList(w http.ResponseWriter, r *http.Request) {
	columns, err := store.ListColumns()
	if err != nil {
		log.Printf("ERROR [%s %q] list columns: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to list the columns: %v", err), http.StatusInternalServerError)
		return
	}
	sources, err := store.ListColumnSources()
	if err != nil {
		log.Printf("ERROR [%s %q] list the columns' sources: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to list the columns: %v", err), http.StatusInternalServerError)
		return
	}

	// Two tables say what feeds a column: the shipped mapping the refresh reads
	// through, and the columns the refresh made from the feed's other paths.
	feedPaths := catalog.FeedPathsByColumn()
	tied, err := store.ImportedFeedFieldColumns()
	if err != nil {
		log.Printf("ERROR [%s %q] list the columns tied to feed fields: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Failed to list the columns: %v", err), http.StatusInternalServerError)
		return
	}
	for key, path := range tied {
		feedPaths[key] = append(feedPaths[key], path)
	}

	out := make([]columnAnswer, 0, len(columns))
	for _, column := range columns {
		declared := sources[column.Key]
		if declared == nil {
			declared = []store.Source{}
		}
		out = append(out, columnAnswer{
			Column:      column,
			Kind:        store.ColumnKind(column, declared),
			Sources:     declared,
			DerivedFrom: store.DerivedFrom(column.Key),
			Date:        store.IsDateColumn(column.Key),
			FeedPaths:   feedPaths[column.Key],
		})
	}

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(out)
}

// columnRequest is what the three writing methods take. A create needs a label
// and takes a kind of value and, for a choice column, its options in the order
// they sort in; a relabel and a delete name the column by its KEY, because that
// is the one thing about a column that never changes.
//
// Confirm is what a DELETE requires, in the BODY, matching POST /api/models/delete:
// deleting a column destroys every model's value for it at once, and this
// database is their only copy.
type columnRequest struct {
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	ValueType string   `json:"value_type"`
	Unit      string   `json:"unit"`
	About     string   `json:"about"`
	Options   []string `json:"options"`
	Confirm   bool     `json:"confirm"`

	// ColorRules is what colour the column paints its cells. It is a raw message
	// so that a PATCH can tell "leave the colours alone" from "clear them": a
	// request that carries the key at all replaces what is stored, and an empty
	// object clears it.
	ColorRules json.RawMessage `json:"color_rules"`
}

// colorsAsked reads the colour rules out of a request, saying whether it carried
// any at all.
func (c columnRequest) colorsAsked() (store.ColorRules, bool, error) {
	raw := strings.TrimSpace(string(c.ColorRules))
	if raw == "" || raw == "null" {
		return store.ColorRules{}, false, nil
	}
	var rules store.ColorRules
	if err := json.Unmarshal(c.ColorRules, &rules); err != nil {
		return store.ColorRules{}, true, fmt.Errorf("\"color_rules\" was not read: %v. A number column takes {\"thresholds\":[{\"below\":1,\"color\":\"#22c55e\"}],\"otherwise\":\"#ef4444\"}, a yes-or-no column {\"yes\":\"#22c55e\",\"no\":\"#ef4444\"}, and a one-of-a-list column {\"options\":{\"good\":\"#22c55e\"}}", err)
	}
	return rules, true, nil
}

// createColumn adds a column of the reader's own. The key is worked out from the
// label where the request gives none, and the answer carries the column as it
// was stored - key included, because that is what every later request names it
// by.
func createColumn(w http.ResponseWriter, r *http.Request) {
	var body columnRequest
	if !decodeModelRequest(w, r, &body) {
		return
	}
	rules, asked, err := body.colorsAsked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	spec := store.ColumnSpec{
		Key: body.Key, Label: body.Label, ValueType: body.ValueType,
		Unit: body.Unit, About: body.About, Options: body.Options,
	}
	if asked {
		spec.Colors = &rules
	}
	column, err := store.CreateOwnColumn(spec)
	if err != nil {
		log.Printf("ok /api/columns refused a create: %v", err)
		http.Error(w, fmt.Sprintf("The column was not created: %v.", err), http.StatusBadRequest)
		return
	}
	log.Printf("ok /api/columns created %s (%s)", column.Key, column.ValueType)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(column)
}

// changeColumn changes what a column is CALLED, what colour it paints its cells,
// or both. Every stored value is untouched either way: a value names its column
// by key, the key does not move, and a colour says how a value is drawn rather
// than what it is.
//
// A COLUMN THAT TAKES NO COLOURS REFUSES THEM rather than storing a rule nothing
// can read: a text and a list column have no order to shade along and no fixed
// set of values to colour one by one.
func changeColumn(w http.ResponseWriter, r *http.Request) {
	var body columnRequest
	if !decodeModelRequest(w, r, &body) {
		return
	}
	if body.Key == "" {
		http.Error(w, `Which column? Send {"key":"rating","label":"New heading"}, or {"key":"rating","color_rules":{...}}.`, http.StatusBadRequest)
		return
	}
	rules, colorsAsked, err := body.colorsAsked()
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	if body.Label == "" && !colorsAsked {
		http.Error(w, fmt.Sprintf(`%s was not changed: send a "label" to rename it, or "color_rules" to say what colour its cells are painted.`, body.Key), http.StatusBadRequest)
		return
	}
	if body.Label != "" {
		if err := store.RelabelColumn(body.Key, body.Label); err != nil {
			log.Printf("ok /api/columns refused a relabel of %s: %v", body.Key, err)
			http.Error(w, fmt.Sprintf("%s was not relabelled: %v.", body.Key, err), statusOfColumnRefusal(err))
			return
		}
	}
	if colorsAsked {
		if err := store.SetColumnColorRules(body.Key, rules); err != nil {
			log.Printf("ok /api/columns refused the colours of %s: %v", body.Key, err)
			http.Error(w, fmt.Sprintf("The colours of %s were not changed: %v.", body.Key, err), statusOfColumnRefusal(err))
			return
		}
	}
	column, err := store.ColumnByKey(body.Key)
	if err != nil {
		log.Printf("ERROR [%s %q] read %s back: %v", r.Method, r.URL.Path, body.Key, err)
		http.Error(w, fmt.Sprintf("%s was changed but could not be read back: %v", body.Key, err), http.StatusInternalServerError)
		return
	}
	log.Printf("ok /api/columns changed %s (label %q, colours %v)", column.Key, column.Label, column.Colors != nil)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(column)
}

// deleteColumn removes one column of the reader's own, and with it every model's
// value for it.
//
// IT REQUIRES {"confirm":true} IN THE BODY. One request destroys a value on every
// model at once, this database is their only copy, and nothing upstream re-creates
// them - so the confirmation is in the body where the caller has to write it,
// exactly as POST /api/models/delete requires one.
func deleteColumn(w http.ResponseWriter, r *http.Request) {
	var body columnRequest
	if !decodeModelRequest(w, r, &body) {
		return
	}
	if body.Key == "" {
		http.Error(w, `Which column? Send {"key":"rating","confirm":true}.`, http.StatusBadRequest)
		return
	}
	if !body.Confirm {
		log.Printf("ok /api/columns refused an unconfirmed delete of %q", body.Key)
		http.Error(w, fmt.Sprintf("%s was NOT deleted. Deleting a column permanently deletes every model's value for it, and this database is the only copy - send {\"key\":%q,\"confirm\":true} to go ahead.", body.Key, body.Key), http.StatusBadRequest)
		return
	}
	if err := store.DeleteColumn(body.Key); err != nil {
		log.Printf("ok /api/columns refused the delete of %s: %v", body.Key, err)
		http.Error(w, fmt.Sprintf("%s was not deleted: %v.", body.Key, err), statusOfColumnRefusal(err))
		return
	}
	log.Printf("ok /api/columns deleted %q and every value stored in it", body.Key)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "key": body.Key})
}

// statusOfColumnRefusal says which kind of refusal a column request met: a key
// naming no column at all is 404, and everything else is the caller's to correct.
func statusOfColumnRefusal(err error) int {
	if errors.Is(err, store.ErrNoSuchColumn) {
		return http.StatusNotFound
	}
	return http.StatusBadRequest
}
