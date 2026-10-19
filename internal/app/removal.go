// The HTTP surface for removing a model from view or from the
// catalog.
//
// Two verbs:
//
//   - POST /api/models/delete permanently removes the row and everything
//     attached to it. It is REFUSED on a row whose models.source is
//     openrouter, because the refresh pulls the whole feed and would create
//     that row again on its next run - see handleDeleteModel.
//   - POST /api/models/hide, the non-destructive alternative: the row stays,
//     with everything written about it, and is only taken out of view. It is
//     the only way an OpenRouter model leaves the table.
//
// WHY DELETE CARRIES A CONFIRMATION FIELD. There is no authentication: a page on
// another address cannot reach a write route (see originAllowed and hostAllowed),
// but anything else that can reach the port can, which is why the server refuses
// to bind a public address (see StartServer). Delete destroys data no other file
// holds, so the request itself has to say so: a body carrying confirm_delete:
// true. It is a field in the
// BODY, never a query parameter, because a URL is the part that gets linked,
// prefetched, logged and pasted. A dialog in the browser cannot provide this
// guarantee - it is not on the wire.

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/store"
	"net/http"
	"strings"
)

// decodeModelRequest reads the JSON body shared by the model verbs. It
// writes the error response itself and reports whether the caller may continue.
func decodeModelRequest(w http.ResponseWriter, r *http.Request, body interface{}) bool {
	decoder := json.NewDecoder(r.Body)
	// A key the request shape does not carry is REFUSED rather than ignored, so a
	// body written against an older shape fails loudly instead of writing nothing.
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(body); err != nil {
		log.Printf("ERROR [%s %q] JSON decode failed: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Invalid JSON: %v", err), decodeFailureStatus(err))
		return false
	}
	return true
}

// noModelName reports whether a request supplied no model name at all, and is
// the ONLY place an incoming name is looked at past its whitespace.
//
// A NAME THAT ADDRESSES A ROW ALREADY HERE IS A LOOKUP KEY, NEVER TEXT TO TIDY.
// models.name is the primary key, and the OpenRouter catalog refresh stores each
// name exactly as upstream sends it - which includes names ending in a space,
// such as "Google: Gemma 4 26B A4B ". Trimming one before the lookup makes those
// rows unreachable through every verb here, so nothing does it: the raw name
// goes to the store, exactly as POST /api/save has always used it. Tidying a
// name is the business of store.NormalizeModelName, and that runs only where a
// row is CREATED.
func noModelName(name string) bool { return strings.TrimSpace(name) == "" }

// handleDeleteModel (POST /api/models/delete) permanently removes a model row:
// {"name": "...", "confirm_delete": true}.
//
// The confirmation field is REQUIRED and the request is refused without it (see
// this file's package comment for why it lives in the body). What the delete
// destroys exists nowhere else: the model's note, its favorite flag, and its
// value in every column of the reader's own all cascade away with the row. Only
// a copy of modelsdb.db brings them back.
//
// THE ORDER OF THE TWO REFUSALS IS DELIBERATE. The row is read first, so a model
// that can never be deleted says so on the first request rather than asking for
// a confirmation and then refusing it anyway.
//
// A ROW WHOSE models.source IS openrouter IS REFUSED. The refresh stores every
// record the feed carries, so such a row is written again the next time it
// runs: deleting it would destroy the reader's own answers about the model and
// hand back a row minutes later. Hiding is what takes it out of view, and the
// refusal says so. The test is that one word exactly, because models.source is
// free text with a legal empty state and every other value it takes names a row
// no refresh creates.
func handleDeleteModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name          string `json:"name"`
		ConfirmDelete bool   `json:"confirm_delete"`
	}
	if !decodeModelRequest(w, r, &body) {
		return
	}
	name := body.Name
	if noModelName(name) {
		http.Error(w, "A model name is required", http.StatusBadRequest)
		return
	}
	// THE ROW IS READ BEFORE THE CONFIRMATION IS ASKED FOR. A refusal no
	// confirmation can lift comes first: asking a caller to send confirm_delete
	// and then refusing the row anyway sends them round twice to learn that this
	// model was never deletable.
	identity, held, err := store.IdentityOf(name)
	if err != nil {
		log.Printf("ERROR [%s %q] read %q: %v", r.Method, r.URL.Path, name, err)
		http.Error(w, fmt.Sprintf("Failed to read %q: %v", name, err), http.StatusInternalServerError)
		return
	}
	if !held {
		http.Error(w, fmt.Sprintf("No model named %q", name), http.StatusNotFound)
		return
	}
	if identity.Source == string(store.SourceOpenRouter) {
		log.Printf("ERROR [%s %q] refused: %q is listed by OpenRouter", r.Method, r.URL.Path, name)
		http.Error(w, fmt.Sprintf("Refused: OpenRouter lists %q, so an update would add it straight back. Hide it instead - the row and everything you wrote about it stay, and it leaves the table.", name), http.StatusConflict)
		return
	}
	if !body.ConfirmDelete {
		log.Printf("ERROR [%s %q] refused: delete of %q carried no confirm_delete field", r.Method, r.URL.Path, name)
		http.Error(w, `Refused: deleting a model destroys its note, its favorite and its value in every column of your own. Send "confirm_delete": true in the request body to go ahead.`, http.StatusBadRequest)
		return
	}
	if err := store.DeleteModelPermanently(name); err != nil {
		if errors.Is(err, store.ErrListedByOpenRouter) {
			// A refresh created the row between the read above and this delete.
			http.Error(w, fmt.Sprintf("Refused: OpenRouter lists %q, so an update would add it straight back. Hide it instead.", name), http.StatusConflict)
			return
		}
		log.Printf("ERROR [%s %q] delete %q: %v", r.Method, r.URL.Path, name, err)
		http.Error(w, fmt.Sprintf("Failed to delete %q: %v", name, err), http.StatusInternalServerError)
		return
	}
	log.Printf("ok /api/models/delete: %q and everything attached to it are gone", name)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "name": name})
}

// handleHideModel (POST /api/models/hide) hides a model from the default list,
// or shows it again: {"name": "...", "hidden": true|false}. The row and
// everything written about it stay in this database; only who sees it changes.
// A hidden model is withheld from GET /api/models unless that request passes
// include_hidden=1.
func handleHideModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Hidden bool   `json:"hidden"`
	}
	if !decodeModelRequest(w, r, &body) {
		return
	}
	name := body.Name
	if noModelName(name) {
		http.Error(w, "A model name is required", http.StatusBadRequest)
		return
	}
	if err := store.SetHidden(name, body.Hidden); err != nil {
		log.Printf("ERROR [%s %q] hide %q: %v", r.Method, r.URL.Path, name, err)
		status := http.StatusInternalServerError
		if errors.Is(err, store.ErrNoSuchModel) {
			status = http.StatusNotFound
		}
		http.Error(w, err.Error(), status)
		return
	}
	log.Printf("ok /api/models/hide: %q hidden=%v", name, body.Hidden)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "name": name, "hidden": body.Hidden})
}
