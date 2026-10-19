// The HTTP surface for a model's variations.
//
// A model listed here is a hosted service. A BUILD is a file you can download and
// run yourself: the same model with each weight stored in fewer bits, published
// on HuggingFace by whoever made it. This is where a user picks one and the app
// records it, as a variation row under the model it was built from (see
// store.ImportVariation).
//
// Two routes, three steps, and the order is the point:
//
//	GET  /api/models/variations?name=<model>   which repositories publish builds of it
//	GET  /api/models/variations/files?repo=..  what builds that one repository holds
//	POST /api/models/variations                record the build the user picked
//
// The two reads exist because NEITHER step can be decided by a program. The
// repositories come from many owners and not all of them are really builds of
// this model; the files inside one are a mix of quantizations, shard sets and
// vision projectors. A person judges both, which is why the app lists and does
// not choose.
//
// THE POST TRUSTS NOTHING THE CLIENT SENDS ABOUT THE BUILD. It takes a repository
// and a build id, re-reads the repository itself, and records the size, format
// and bit width IT read. A caller cannot post a made-up size into the catalog,
// and the values in bit and format are therefore always measurements.
//
// Two failures are expected rather than exceptional, and both leave the database
// untouched and answer with a sentence a person can act on: a machine with no
// network, and a GATED repository - one whose owner requires an account, which
// answers the metadata call in full and refuses the file itself. Recording a
// build nobody can download would be worse than refusing to.

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

// handleVariations (GET/POST /api/models/variations) lists the repositories that
// publish builds of a model, and records the build the user picked.
func handleVariations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		listVariationRepos(w, r)
	case http.MethodPost:
		importVariation(w, r)
	default:
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// listVariationRepos answers the first step of the picker: which HuggingFace
// repositories publish builds of this model.
//
// A model with no HuggingFace repository recorded gets a 400 saying so. That is
// not a dead end the user can walk into - the control that calls this is absent
// on such a row - but the endpoint has to answer honestly to a caller that asks
// anyway.
func listVariationRepos(w http.ResponseWriter, r *http.Request) {
	name := r.URL.Query().Get("name")
	if noModelName(name) {
		http.Error(w, "A model name is required", http.StatusBadRequest)
		return
	}
	slug, exists, err := store.HFSlugOf(name)
	if err != nil {
		log.Printf("ERROR [%s %q] read %q: %v", r.Method, r.URL.Path, name, err)
		http.Error(w, fmt.Sprintf("Failed to read %q: %v", name, err), http.StatusInternalServerError)
		return
	}
	if !exists {
		http.Error(w, fmt.Sprintf("No model named %q", name), http.StatusNotFound)
		return
	}
	if slug == "" {
		http.Error(w, fmt.Sprintf("%q has no HuggingFace repository recorded, so there is nothing to look up", name), http.StatusBadRequest)
		return
	}

	repos, truncated, err := catalog.HFQuantizedRepos(slug)
	if err != nil {
		log.Printf("ERROR [%s %q] list builds of %q: %v", r.Method, r.URL.Path, slug, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if repos == nil {
		repos = []catalog.HFRepo{}
	}
	log.Printf("ok /api/models/variations: %d repositories publish builds of %s", len(repos), slug)
	writeJSON(w, map[string]interface{}{
		"name":      name,
		"hf_slug":   slug,
		"repos":     repos,
		"truncated": truncated,
	})
}

// handleVariationFiles (GET /api/models/variations/files) answers the second step
// of the picker: what one repository holds, with each build's exact size.
func handleVariationFiles(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	repo := strings.TrimSpace(r.URL.Query().Get("repo"))
	if repo == "" {
		http.Error(w, "A HuggingFace repository is required", http.StatusBadRequest)
		return
	}
	detail, err := catalog.HFRepoBuilds(repo)
	if err != nil {
		log.Printf("ERROR [%s %q] read %q: %v", r.Method, r.URL.Path, repo, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	if detail.Builds == nil {
		detail.Builds = []catalog.HFBuild{}
	}
	log.Printf("ok /api/models/variations/files: %s holds %d build(s)", repo, len(detail.Builds))
	writeJSON(w, detail)
}

// importVariation records one build. The body names the model, the repository and
// the build; everything stored about the build is read here rather than taken
// from the request.
func importVariation(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name  string `json:"name"`
		Repo  string `json:"repo"`
		Build string `json:"build"`
	}
	if !decodeModelRequest(w, r, &body) {
		return
	}
	parent := body.Name
	repo := strings.TrimSpace(body.Repo)
	if noModelName(parent) || repo == "" {
		http.Error(w, "A model name and a HuggingFace repository are required", http.StatusBadRequest)
		return
	}
	if !store.ModelExists(parent) {
		http.Error(w, fmt.Sprintf("No model named %q", parent), http.StatusNotFound)
		return
	}

	detail, err := catalog.HFRepoBuilds(repo)
	if err != nil {
		log.Printf("ERROR [%s %q] read %q: %v", r.Method, r.URL.Path, repo, err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	// A gated repository answers this call in full and refuses the file itself, so
	// the metadata succeeding proves nothing about being able to download the
	// build. Recording one the user cannot fetch would put a row in the catalog
	// that looks available and is not, so it is refused here, before anything is
	// written.
	if detail.Gated {
		http.Error(w, fmt.Sprintf("%s requires an account on HuggingFace, so its files cannot be read here and nothing was recorded", repo), http.StatusBadRequest)
		return
	}
	build, ok := detail.FindBuild(strings.TrimSpace(body.Build))
	if !ok {
		http.Error(w, fmt.Sprintf("%s does not hold a build called %q", repo, body.Build), http.StatusBadRequest)
		return
	}

	format := buildFormat(build.Format, build.Label)
	name := variationName(parent, repo, build)

	err = store.ImportVariation(store.Variation{
		Parent:      parent,
		Name:        name,
		HFSlug:      repo,
		Description: variationDescription(parent, repo, format, build),
		Facts:       variationFacts(detail, build, format),
	})
	// A relation the table cannot draw is the CALLER's to correct, so it answers
	// 4xx and says what to do. Everything else here really is a failure.
	if errors.Is(err, store.ErrNotAParent) {
		log.Printf("ok [%s %q] refused: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Nothing was recorded: %v. Record the build under the model it was built from instead.", err), http.StatusBadRequest)
		return
	}
	if errors.Is(err, store.ErrModelExists) {
		log.Printf("ok [%s %q] refused: %v", r.Method, r.URL.Path, err)
		http.Error(w, fmt.Sprintf("Nothing was recorded: %v.", err), http.StatusConflict)
		return
	}
	if err != nil {
		log.Printf("ERROR [%s %q] record %q: %v", r.Method, r.URL.Path, name, err)
		http.Error(w, fmt.Sprintf("Failed to record the build: %v", err), http.StatusInternalServerError)
		return
	}
	log.Printf("ok /api/models/variations: recorded %q (%s, %d bit, %d bytes)", name, format, build.Bit, build.Bytes)
	writeJSON(w, map[string]interface{}{
		"ok":         true,
		"name":       name,
		"variant_of": parent,
		"format":     format,
		"bit":        build.Bit,
		"bytes":      build.Bytes,
	})
}

// handleFileAsVariation (POST /api/models/variations/existing) files a model that
// is ALREADY in the catalog under another one, as a variation of it.
//
// It is the counterpart of the importer above, for the case that importer cannot
// serve: a build somebody already added as a model of its own - by hand or from a
// HuggingFace address - which belongs under the model it was made from. Nothing
// is fetched, because there is nothing to measure: the row keeps its note, its
// favourite star, every value in a column of the reader's own and every figure it
// holds, and gains one fact.
//
// THE ROW BECOMES HIDDEN, like every variation, so it leaves the model list and
// the default GET /api/models answer. The reply says so in as many words, because
// a row that vanishes from the table without explanation reads as a delete.
func handleFileAsVariation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name   string `json:"name"`
		Parent string `json:"parent"`
	}
	if !decodeModelRequest(w, r, &body) {
		return
	}
	name := body.Name
	parent := body.Parent
	if noModelName(name) || noModelName(parent) {
		http.Error(w, "Both name (the model to file) and parent (the model to file it under) are required", http.StatusBadRequest)
		return
	}

	err := store.FileAsVariation(name, parent)
	switch {
	case errors.Is(err, store.ErrNoSuchModel):
		log.Printf("ok /api/models/variations/existing refused: %v", err)
		http.Error(w, fmt.Sprintf("Nothing was changed: %v. Both models have to be in this catalog already.", err), http.StatusNotFound)
		return
	case err != nil:
		// Every remaining refusal is one of the four relations that cannot be
		// drawn - a row under itself, under its own variation, under a variation,
		// or a row that has variations of its own - each of which is the caller's
		// to correct.
		log.Printf("ok /api/models/variations/existing refused: %v", err)
		http.Error(w, fmt.Sprintf("Nothing was changed: %v.", err), http.StatusBadRequest)
		return
	}
	log.Printf("ok /api/models/variations/existing: %q is now a variation of %q, and hidden", name, parent)
	writeJSON(w, map[string]interface{}{
		"ok":         true,
		"name":       name,
		"variant_of": parent,
		"hidden":     true,
	})
}

// handleUnfileVariation (POST /api/models/variations/unfile) takes one row back
// out from under the model it is filed under: {"name": "..."}.
//
// IT IS THE WAY BACK, and the reason it has to exist. Filing a row is one press,
// nothing else in this app clears variant_of, and a row the OpenRouter feed
// lists cannot be deleted - so without this route a misfiled OpenRouter model is
// a hidden child of another model for ever.
//
// Hiding is not touched here, and undoing the hide is the row's own control.
func handleUnfileVariation(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		log.Printf("ERROR [%s %q] unsupported method", r.Method, r.URL.Path)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}
	var body struct {
		Name string `json:"name"`
	}
	if !decodeModelRequest(w, r, &body) {
		return
	}
	name := body.Name
	if noModelName(name) {
		http.Error(w, "A model name is required", http.StatusBadRequest)
		return
	}

	err := store.UnfileVariation(name)
	switch {
	case errors.Is(err, store.ErrNoSuchModel):
		log.Printf("ok /api/models/variations/unfile refused: %v", err)
		http.Error(w, fmt.Sprintf("Nothing was changed: %v. The model has to be in this catalog already.", err), http.StatusNotFound)
		return
	case err != nil:
		// A row that is filed under nothing, and a row with variations of its
		// own, are the two refusals, and each is the caller's to correct.
		log.Printf("ok /api/models/variations/unfile refused: %v", err)
		http.Error(w, fmt.Sprintf("Nothing was changed: %v.", err), http.StatusBadRequest)
		return
	}
	log.Printf("ok /api/models/variations/unfile: %q is a model of its own again", name)
	writeJSON(w, map[string]interface{}{
		"ok":         true,
		"name":       name,
		"variant_of": "",
	})
}

// buildFormat composes the stored Build format cell out of the format word and
// the quantization label, without stating the format word twice.
//
// A GGUF label is the quantization alone (Q4_K_M), read straight off the file
// name, so it is appended after the format as it always was. Every other
// repository's label is built in internal/catalog/huggingface.go as "<format>
// <bit>-bit" or as the format alone, so it ALREADY carries the format word -
// prefixing it again is what produced "FP8 FP8 8-bit". The label is kept as
// given whenever it already begins with the format, ignoring case, and an
// empty format stores the label alone with no leading space.
func buildFormat(format, label string) string {
	if label == "" {
		return format
	}
	if format != "" && !strings.HasPrefix(strings.ToLower(label), strings.ToLower(format)) {
		return format + " " + label
	}
	return label
}

// variationName is the build's display name, and it is the models primary key, so
// it has to be different for every build anyone could record.
//
// It carries the repository as well as the label because two owners publish the
// same quantization of the same model all the time - one Q4_K_M from each - and a
// name built from the label alone would let the second silently overwrite the
// first.
func variationName(parent, repo string, build catalog.HFBuild) string {
	label := build.Label
	if label == "" {
		label = build.Format
	}
	return fmt.Sprintf("%s (%s %s)", parent, repo, label)
}

// variationDescription is the one line the table shows about a build.
func variationDescription(parent, repo, format string, build catalog.HFBuild) string {
	size := ""
	if build.Bytes > 0 {
		size = fmt.Sprintf(", %.1f GB", gigabytes(build.Bytes))
	}
	if build.Projector {
		return fmt.Sprintf("Vision projector for %s, from the HuggingFace repository %s%s. It is loaded alongside a quantization rather than being one.", parent, repo, size)
	}
	return fmt.Sprintf("%s build of %s, from the HuggingFace repository %s%s. A file you download and run yourself, not a hosted model.", format, parent, repo, size)
}

// gigabytes converts a byte count to decimal GB, the same unit disk_size_gb
// already holds and the same one HuggingFace shows on its own files tab.
func gigabytes(b int64) float64 {
	if b <= 0 {
		return 0
	}
	return float64(b) / 1e9
}

// writeJSON sends one JSON object with the header every handler here sets.
func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	json.NewEncoder(w).Encode(v)
}
