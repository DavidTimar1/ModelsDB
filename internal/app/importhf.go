// The HTTP surface for adding a model from a HuggingFace address.
//
// POST /api/models/import takes ONE thing, a HuggingFace model URL, and creates
// the whole row from what the SERVER reads there. The caller supplies no value
// at all beyond the address, so nothing in the created row is somebody's
// recollection: the repository id, the author, the context length, the
// modalities and the on-disk size are each read off HuggingFace at the moment
// of the import. The architecture is not: it is one line a person or assistant
// writes into the Architecture column afterwards (procedure section 6a).
//
// It is the same shape POST /api/models/variations already ships, for the same
// reason: a request that cannot state a measurement cannot get one wrong.
// POST /api/models/add is the other half of the pair - it creates a row from
// what a person types, for a model that is on no site this app reads.
//
// A MODEL ALREADY IN THE CATALOG CAN BE READ AGAIN, on request: send
// "reimport": true and the repository is re-read and what the importer MEASURES
// is rewritten. It is not the default, so an accidental repeat still answers
// 409 rather than quietly overwriting a row.
//
// ALREADY IN THE CATALOG IS DECIDED BY store.FindExistingModel, which asks about
// the repository and the upstream id before it asks about the name. The row that
// holds a repository is usually stored under somebody else's name for the model:
// OpenRouter lists "inclusionAI: Ling 3.0 Tiny (free)" and this repository
// address would invent "inclusionAI: Ling-3.0-tiny" for the same model. A
// re-import therefore writes onto the row the lookup FOUND, whatever it is
// called, because writing under the invented name is what puts one model's note,
// star and column values on two rows that never mention each other.
//
// WHY RE-READING EXISTS. The parameter count, the size on disk, the modalities
// and the context length are measurements, and the update procedure forbids
// anybody to type one. They can only come from an import, so a model added
// before they were measured can be filled in only by reading its repository
// again.
//
// WHAT A RE-IMPORT TOUCHES, and what it must never touch. It rewrites what the
// repository states and nothing else:
//
//   - the huggingface cells for the facts the repository stated
//     (store.RecordFacts writes each into the column that states it);
//   - the row's repository, its description when it has none, its modality lists,
//     and its context length (store.ImportFullRecord, which cannot replace a
//     stored value with an empty one or lower a merged one).
//
// PERSONAL DATA IS UNTOUCHED BY CONSTRUCTION rather than by a guard added here.
// The import writes the repository's own cells and names neither notes nor
// favorite nor hidden - so a column the reader created themselves, and a value
// they typed into one of the importer's columns, are equally out of its reach.
//
// The answers, and each of them is deliberate:
//
//	201  the row was created; the body says what was read and what is still empty
//	200  the model was already here and was re-read ("reimport": true); "name"
//	     says which row was written, which is the row that already held it
//	409  this catalog already holds that model and nobody asked to re-read it
//	400  the address cannot be used, or the repository is gated
//	502  HuggingFace could not be reached, or answered something unusable
//
// THE ALREADY-HERE CHECK BELONGS TO THIS HANDLER. store.ImportFullRecord
// underneath it upserts on a name conflict, because that is right for an
// unattended pass where nobody is watching. Here a person or an agent is
// waiting for an answer, so the case is answered explicitly - the
// same 409 contract POST /api/models/add already keeps, with the re-import as
// the way past it.
//
// A GATED repository is refused with 400 and nothing is recorded, exactly as
// POST /api/models/variations refuses one. Its metadata answers in full while
// its files refuse, so the row would carry no size and no context length while
// looking like a complete import.

package app

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/catalog"
	"modelsdb/internal/store"
	"net/http"
	"strings"
)

// handleImportHF (POST /api/models/import) creates one model row from a
// HuggingFace model URL: {"url": "https://huggingface.co/Qwen/Qwen3-32B"}.
func handleImportHF(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		URL string `json:"url"`
		// Read the repository again for a model this catalog already holds, and
		// rewrite what the importer MEASURES. Without it a name already here is
		// refused.
		Reimport bool `json:"reimport"`
	}
	if !decodeModelRequest(w, r, &body) {
		return
	}
	repo, err := catalog.NormalizeHFRepo(body.URL)
	if err != nil {
		http.Error(w, fmt.Sprintf("%v. Send {\"url\": \"https://huggingface.co/<owner>/<model>\"}, or the bare <owner>/<model>.", err), http.StatusBadRequest)
		return
	}
	name := store.NormalizeModelName(hfDisplayName(repo))

	// THE ALREADY-HERE CHECK COMES BEFORE THE NETWORK READ: reading a repository
	// whose row cannot be written is wasted.
	//
	// A MODEL ALREADY HERE IS ONLY A REFUSAL WHILE NOBODY ASKED TO RE-READ IT.
	// The size and the counts are measurements the procedure forbids anybody to
	// type, and they can only come from an import - so a model added before those
	// columns existed could be filled in NO way at all while this route refused
	// it. The answer names the request that does it, rather than leaving the
	// reader to find out that one exists.
	//
	// ALREADY HERE IS A QUESTION ABOUT THE REPOSITORY, not about the name this
	// import would invent. The row that holds this repository is very often stored
	// under a different name - OpenRouter's name for the model rather than the
	// repository's - and importing onto a second row for it is how one model's
	// note, star and column values end up split between two rows.
	match, existed, err := store.FindExistingModel(name, "", repo)
	if err != nil {
		log.Printf("ERROR [%s %q] look %q up in the catalog: %v", r.Method, r.URL.Path, repo, err)
		http.Error(w, fmt.Sprintf("Failed to look %s up in this catalog: %v", repo, err), http.StatusInternalServerError)
		return
	}
	if existed && !body.Reimport {
		log.Printf("ok /api/models/import refused: %s is already held by %q (matched on %s)", repo, match.Name, match.By)
		http.Error(w, fmt.Sprintf("This catalog already holds that model, as %q, which carries %s, so nothing was imported. "+
			"Send the same request with \"reimport\": true to read %s again ONTO %q and rewrite what the importer measures - "+
			"your notes, your favourite star and every column you fill in yourself are left untouched. "+
			"POST /api/save writes fields onto a model that is already here.", match.Name, match.Matched(), repo, match.Name), http.StatusConflict)
		return
	}
	// The row a re-import writes is the row that already holds this model,
	// whatever it is called. Writing under the invented name instead would create
	// the second row this whole check exists to prevent.
	target := name
	if existed {
		target = match.Name
	}

	model, err := catalog.HFModelRecord(repo)
	if err != nil {
		log.Printf("ERROR [%s %q] read %q: %v", r.Method, r.URL.Path, repo, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if model.Gated {
		log.Printf("ok /api/models/import refused: %q is gated", repo)
		http.Error(w, fmt.Sprintf("%s requires an account on HuggingFace, so its files cannot be read here and nothing was imported.", repo), http.StatusBadRequest)
		return
	}

	// A repository that states no task, or one that maps to nothing, leaves the
	// type to the build's parent when the row is filed under one; only a row with
	// no parent falls back to chat.
	rowType := model.ModelType
	if rowType == "" {
		rowType = store.ParentModelType(target)
	}
	record := hfImportRecord(target, model)
	if err := store.ImportFullRecord(record, "huggingface", rowType); err != nil {
		log.Printf("ERROR [%s %q] import %q: %v", r.Method, r.URL.Path, target, err)
		http.Error(w, fmt.Sprintf("Failed to import %q: %v", target, err), http.StatusInternalServerError)
		return
	}
	// The row is read back rather than assumed: every insert path in the store
	// can decline silently, and an import that says 201 must mean a row exists.
	if !store.ModelExists(target) {
		log.Printf("ERROR [%s %q] %q was not created by the import", r.Method, r.URL.Path, target)
		http.Error(w, fmt.Sprintf("%q was not recorded. Nothing was written.", target), http.StatusInternalServerError)
		return
	}
	// The facts go on last, each into the repository's own column. The row already
	// exists at this point, so a fact that cannot be recorded is reported without
	// pretending the import failed.
	facts := hfModelFacts(model)
	recorded, err := store.RecordFacts(target, store.SourceHuggingFace, facts)
	if err != nil {
		log.Printf("ERROR [%s %q] record what %q states: %v", r.Method, r.URL.Path, repo, err)
		http.Error(w, fmt.Sprintf("%q was added, but what its repository states could not be recorded: %v", target, err), http.StatusInternalServerError)
		return
	}

	// What is still empty is read back off the row that was just written, so it
	// is the row's own state rather than a list of the fields this importer
	// happens to know it cannot fill.
	missing, err := store.Gaps(target)
	if err != nil {
		log.Printf("ERROR [%s %q] work out what %q is still missing: %v", r.Method, r.URL.Path, target, err)
		http.Error(w, fmt.Sprintf("%q was imported, but what it is still missing could not be read: %v", target, err), http.StatusInternalServerError)
		return
	}

	// A re-import ANSWERS 200 and a first import 201, because the two did
	// different things: one rewrote a row that was already here and the other
	// created one. A caller that has to know which is which - a script deciding
	// whether to report a new model - reads the status, not the body.
	status := http.StatusCreated
	if existed {
		status = http.StatusOK
	}
	log.Printf("ok /api/models/import: %q imported from %s (%d bytes of weights, %s, reimport=%v)",
		target, repo, model.WeightBytes, model.WeightBasis, existed)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"ok":             true,
		"reimported":     existed,
		"name":           target,
		"hf_slug":        model.Repo,
		"model_type":     modelTypeOfRow(rowType),
		"revision":       model.Revision,
		"size_basis":     model.WeightBasis,
		"context_length": model.ContextLength,
		// What the repository states, keyed by the fact keys
		// GET /api/import-fields lists, and the ones of them that really became
		// columns here. A caller can tell at a glance what was read and what was
		// recorded.
		"read":    facts,
		"kept":    recorded,
		"missing": missing,
		"similar": similarToJustCreated(target),
	})
}

// hfDisplayName is the name the imported row is stored under: the repository's
// owner and model name, in the "Author: Model" form the rest of this catalog
// uses. It is derived rather than supplied, so two people importing the same
// repository land on the same row instead of two rows for one model.
//
// It is only the name for a row this import CREATES. A repository this catalog
// already holds under another name is written onto that row instead, so the
// invented name is never used to mint a second row for a model already here.
func hfDisplayName(repo string) string {
	owner, model := repo, ""
	if i := strings.Index(repo, "/"); i > 0 {
		owner, model = repo[:i], repo[i+1:]
	}
	if model == "" {
		return owner
	}
	return owner + ": " + model
}

// hfImportRecord turns what was read into the record store.ImportFullRecord
// writes: the identity-level fields. Every key here holds a value read off
// HuggingFace; a fact the repository did not state is simply absent, which
// leaves that column empty for a person or an agent to fill in afterwards.
//
// Everything else the repository states - who published it, how many parameters
// it has, how much disk it takes - is written separately by
// store.RecordFacts from hfModelFacts, into the huggingface cell of the column
// that states it.
func hfImportRecord(name string, m *catalog.HFModel) map[string]interface{} {
	record := map[string]interface{}{
		"name":        name,
		"source":      "huggingface",
		"hf_slug":     m.Repo,
		"description": m.Description,
		"created_at":  m.CreatedAt,
	}
	if m.ContextLength > 0 {
		record["context_length"] = float64(m.ContextLength)
	}
	if len(m.InputModalities) > 0 {
		record["input_modalities"] = m.InputModalities
	}
	if len(m.OutputModalities) > 0 {
		record["output_modalities"] = m.OutputModalities
	}
	if m.MoE != "" {
		record["moe"] = m.MoE
	}
	return record
}

// modelTypeOfRow is the type the row ends up with. An unrecognized HuggingFace
// task leaves the type empty here and store.ImportFullRecord derives one from
// the output modalities, which with none of those is "chat" - so the answer says
// chat rather than an empty string the caller would have to interpret.
func modelTypeOfRow(modelType string) string {
	if modelType != "" {
		return modelType
	}
	return "chat"
}
