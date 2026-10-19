// The HTTP surface for adding a model by hand.
//
// Nearly every row in this catalog arrives on its own: the OpenRouter refresh
// writes the upstream catalog on every startup, and the build importer records
// a variation of a model. This route is for the model none of them offers - one
// you run yourself, or one listed nowhere this app reads.
//
// IT IS THE ONLY WAY TO CREATE A MODEL BY HAND, and that is why it is a route of
// its own rather than a side effect of writing a field. POST /api/save writes
// fields onto a model that already exists and answers 404 for a name this
// database has never seen, so a typo in a save fails instead of minting a model
// carrying whatever was sent. Adding a model is a deliberate act, and the
// request says which act it is.
//
// ONE refusal, 409: this catalog already holds that model, which /api/save is
// the endpoint for. Already holds it means the row is recognised by its
// repository, by its upstream id or by its name (store.FindExistingModel), so
// a second row cannot be minted for a model that is here by typing a
// different name for it.

package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/store"
	"net/http"
)

// handleAddModel (POST /api/models/add) creates one model row: {"name": "...",
// plus any of the fields POST /api/save accepts}. It answers 201 with the name
// it created, 409 when that name is already here, and 400 without a name or
// with a field this API does not write as sent. A refused field creates no row
// at all: the fields are read before the insert, not between it and the write
// that follows.
//
// EVERY FIELD CAN BE FILLED HERE, in the request that adds the model: the row
// it creates carries no listing and no repository, so nothing automatic has
// stated anything about it. What is typed into a fetched column lands as
// research and is replaced if the model ever appears on OpenRouter or its
// repository is read; what is typed into a column of the reader's own lands as
// hand.
//
// It takes ONE object rather than the array /api/save takes, because adding a
// model is one decision at a time and a partial failure halfway through a batch
// of new models would leave the caller working out which of them exist.
//
// The name is trimmed and has runs of whitespace collapsed
// (store.NormalizeModelName). That tidying happens where a row is CREATED and
// nowhere else: no stored name is ever rewritten, because renaming a row would
// destroy its personal values (see NormalizeModelName for why).
func handleAddModel(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body saveRequest
	if !decodeModelRequest(w, r, &body) {
		return
	}
	// The name is normalized before anything reads it, so the row that gets
	// created and every message below all name the same string.
	name := store.NormalizeModelName(body.Name)
	if name == "" {
		http.Error(w, "A model name is required, as {\"name\":\"...\",\"cells\":{...}}", http.StatusBadRequest)
		return
	}

	err := store.AddModelWithCells(name, body.Cells)
	switch {
	case errors.Is(err, store.ErrModelExists):
		log.Printf("ok /api/models/add refused %q: %v", name, err)
		http.Error(w, fmt.Sprintf("%q was not added: %v. POST /api/save writes cells onto a model that is already here.", name, err), http.StatusConflict)
		return
	case err != nil:
		status := statusOfWriteRefusal(err)
		if status == http.StatusInternalServerError {
			log.Printf("ERROR [%s %q] add %q: %v", r.Method, r.URL.Path, name, err)
		} else {
			log.Printf("ok /api/models/add refused for %q: %v", name, err)
		}
		http.Error(w, fmt.Sprintf("%q was not added: %v.", name, err), status)
		return
	}

	log.Printf("ok /api/models/add: %q added", name)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(map[string]interface{}{"ok": true, "name": name, "similar": similarToJustCreated(name)})
}
