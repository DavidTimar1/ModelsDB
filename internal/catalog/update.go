// The refresh: reading the OpenRouter feed into the store, and the one place its record shape is understood.

package catalog

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/http"
	"strings"
	"time"
)

// ApiUrl is the OpenRouter models endpoint FetchModels reads. It is a
// package-level var, not a const, so a test can point it at a local
// httptest.Server and run the whole refresh path with no real request to
// openrouter.ai for the catalog itself.
//
// The query string matters: OpenRouter's default GET /models answer is
// filtered to output_modalities=text and hides every embeddings, image,
// video, transcription, speech, rerank and decisions model. Reading
// ?output_modalities=all instead brings all of them, in the identical record
// shape NormalizeModel already reads, through this one URL and no second
// fetch.
var ApiUrl = "https://openrouter.ai/api/v1/models?output_modalities=all"

const (
	// feedHTTPTimeout bounds the request for the OpenRouter feed itself, the
	// same way hfHTTPTimeout bounds a HuggingFace request (huggingface.go).
	// http.Get has no default timeout, so a hung upstream would otherwise
	// block the caller forever.
	feedHTTPTimeout = 25 * time.Second
	// feedMaxBodyBytes bounds the feed body read. The OpenRouter feed is a few
	// MB today; 32 MB leaves headroom for the catalog to grow without letting
	// an unbounded or malicious response exhaust memory.
	feedMaxBodyBytes = 32 << 20
)

// NormalizeModel converts a model from the OpenRouter public API
// (/api/v1/models) into the internal shape the data layer expects. All
// knowledge of the upstream schema is isolated here:
//   - pricing nested under "endpoint.pricing"
//   - tool support derives from supported_parameters -> endpoint.supports_tool_parameters
//   - id kept as-is (collection join key), canonical_slug -> slug, hugging_face_id -> hf_slug
//   - architecture.{input,output}_modalities -> top-level
//   - created (unix seconds) -> created_at (ISO-8601)
func NormalizeModel(m map[string]interface{}) map[string]interface{} {
	out := map[string]interface{}{
		"name":        shared.GetStr(m, "name"),
		"description": shared.GetStr(m, "description"),
		"id":          shared.GetStr(m, "id"),
	}

	if s := shared.GetStr(m, "canonical_slug"); s != "" {
		out["slug"] = s
	} else {
		out["slug"] = shared.GetStr(m, "id")
	}

	if hf := shared.GetStr(m, "hugging_face_id"); hf != "" {
		out["hf_slug"] = hf
	} else {
		out["hf_slug"] = nil
	}

	if c, ok := m["created"].(float64); ok && c > 0 {
		out["created_at"] = time.Unix(int64(c), 0).UTC().Format(time.RFC3339)
	}

	out["context_length"] = m["context_length"]

	if arch, ok := m["architecture"].(map[string]interface{}); ok {
		out["input_modalities"] = arch["input_modalities"]
		out["output_modalities"] = arch["output_modalities"]
	}

	supportsTool := false
	supportsReasoning := false
	if sp, ok := m["supported_parameters"].([]interface{}); ok {
		for _, p := range sp {
			switch fmt.Sprintf("%v", p) {
			case "tools", "tool_choice":
				supportsTool = true
			case "reasoning", "include_reasoning":
				supportsReasoning = true
			}
		}
	}
	out["supports_reasoning"] = supportsReasoning

	endpoint := map[string]interface{}{
		"supports_tool_parameters": supportsTool,
	}
	if pr, ok := m["pricing"].(map[string]interface{}); ok {
		endpoint["pricing"] = pr
	}
	out["endpoint"] = endpoint

	return out
}

// OpenRouterFacts reads, out of ONE raw upstream record, every fact the
// catalogue records beyond what NormalizeModel carries - keyed by the fact keys
// GET /api/import-fields lists.
//
// It sits beside NormalizeModel because both are upstream-schema knowledge, and
// this package is the only one that is allowed to hold any. It is separate from
// NormalizeModel because what it produces goes somewhere else: a normalized
// model feeds UpsertAPIModel, while these facts are recorded one by one.
//
// A fact the record does not state is left OUT of the map rather than written as
// an empty string, because an absent key creates no column and an empty one
// would create a column of blank cells.
func OpenRouterFacts(m map[string]interface{}) map[string]string {
	facts := map[string]string{}
	// The author is not a field upstream sends: an OpenRouter id is
	// "<author>/<model>", so the part before the slash is who publishes it. That
	// is the same fact a HuggingFace import reads off the repository owner, so it
	// lands in the same column.
	if id := shared.GetStr(m, "id"); id != "" {
		if i := strings.Index(id, "/"); i > 0 {
			factText(facts, "author", id[:i])
		}
	}
	if arch, ok := m["architecture"].(map[string]interface{}); ok {
		factText(facts, "tokenizer", shared.GetStr(arch, "tokenizer"))
		factText(facts, "instruct_type", shared.GetStr(arch, "instruct_type"))
	}
	if params, ok := m["supported_parameters"].([]interface{}); ok && len(params) > 0 {
		words := make([]string, 0, len(params))
		for _, p := range params {
			words = append(words, fmt.Sprintf("%v", p))
		}
		factText(facts, "supported_parameters", strings.Join(words, ", "))
	}
	if tp, ok := m["top_provider"].(map[string]interface{}); ok {
		factNumber(facts, "max_completion_tokens", tp["max_completion_tokens"])
		factYesNo(facts, "moderated", tp["is_moderated"])
	}
	factText(facts, "knowledge_cutoff", shared.GetStr(m, "knowledge_cutoff"))
	factText(facts, "expiration_date", shared.GetStr(m, "expiration_date"))
	if reasoning, ok := m["reasoning"].(map[string]interface{}); ok {
		factYesNo(facts, "reasoning_mandatory", reasoning["mandatory"])
	}
	if alias, ok := m["alias_target"].(map[string]interface{}); ok {
		factText(facts, "alias_target", shared.GetStr(alias, "slug"))
	}
	if pricing, ok := m["pricing"].(map[string]interface{}); ok {
		for key, upstream := range extraPriceKeys {
			factNumber(facts, key, pricing[upstream])
		}
	}
	return facts
}

// extraPriceKeys are the prices upstream quotes that no column of models holds,
// keyed by the fact each one is recorded under. The five prices models DOES hold
// - the input, output, image, audio and internal-reasoning figures - are absent
// here, because a fact and a fixed column for the same figure would disagree the
// first time one of them was corrected.
var extraPriceKeys = map[string]string{
	"price_cache_read":       "input_cache_read",
	"price_cache_write":      "input_cache_write",
	"price_cache_write_1h":   "input_cache_write_1h",
	"price_audio_cache_read": "input_audio_cache",
	"price_image_output":     "image_output",
	"price_audio_output":     "audio_output",
	"price_web_search":       "web_search",
}

// factText records a text fact, leaving an empty one out.
func factText(facts map[string]string, key, value string) {
	if v := strings.TrimSpace(value); v != "" {
		facts[key] = v
	}
}

// factNumber records a figure, leaving out anything that is not a number and
// anything that is zero or below - upstream sends "0" for a price it does not
// charge and a negative figure for a model it will not quote at all, and neither
// of those is a figure to put in a column.
func factNumber(facts map[string]string, key string, value interface{}) {
	if value == nil {
		return
	}
	text := strings.TrimSpace(fmt.Sprintf("%v", value))
	f, err := shared.ParseNumber(text)
	if err != nil || f <= 0 {
		return
	}
	facts[key] = text
}

// factYesNo records a true/false answer in the words the rest of this catalog
// uses for one, so a person reading the cell sees the same vocabulary as the
// tool and moe columns.
func factYesNo(facts map[string]string, key string, value interface{}) {
	said, isBool := value.(bool)
	if !isBool {
		return
	}
	if said {
		facts[key] = "yes"
		return
	}
	facts[key] = "no"
}

// fetchFeedFrom reads a models listing LIVE and answers each record
// exactly as upstream sent it, twice over: parsed into the map every other
// reader of a record uses, and as the same record's own raw JSON bytes,
// same length and same order. The raw bytes exist for exactly one reason -
// catalog.objectArrayMembers reads a price window or leaderboard row's own
// key order out of them, which a Go map cannot hold. It holds no cache of its
// own: the catalog refresh (FetchModelsWith below) reads the general listing
// through it, and every column made from the feed is filled from the records
// that same call fetched. The video and image listings (dedicatedlistings.go)
// read theirs the same way - same timeout, same size cap, same twice-over
// answer - without a second copy of the fetch. pc paces and retries it like
// every other request of the run.
func fetchFeedFrom(url string, pc *pacer) (data []map[string]interface{}, raw []json.RawMessage, err error) {
	client := shared.NewHTTPClient(feedHTTPTimeout)
	resp, err := getWithRetry(client, url, pc)
	if err != nil {
		return nil, nil, fmt.Errorf("fetch models: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("fetch models: unexpected status %s from %s", resp.Status, url)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, feedMaxBodyBytes+1))
	if err != nil {
		return nil, nil, fmt.Errorf("read response body: %w", err)
	}
	if len(body) > feedMaxBodyBytes {
		return nil, nil, fmt.Errorf("fetch models: feed from %s is too large (over %d bytes)", url, feedMaxBodyBytes)
	}

	var doc struct {
		Data []json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, nil, fmt.Errorf("parse models JSON (got %d bytes, content-type %q): %w", len(body), resp.Header.Get("Content-Type"), err)
	}
	data = make([]map[string]interface{}, len(doc.Data))
	for i, r := range doc.Data {
		if err := json.Unmarshal(r, &data[i]); err != nil {
			return nil, nil, fmt.Errorf("parse record %d of the models feed: %w", i, err)
		}
	}
	return data, doc.Data, nil
}

// FetchModels pulls the OpenRouter catalog and UPSERTs it into the DB: what
// OpenRouter states is refreshed in OpenRouter's own cells, and what the reader
// or an assistant wrote is left alone. It also RECORDS EVERY LEAF PATH the feed carried this time
// (see observeFeedFields) and MAKES A COLUMN FOR EVERY PATH NOTHING READS YET
// (see feedColumnCandidates and store.CreateFeedFieldColumn), filled in this
// same pass from the records already in memory, so a field OpenRouter adds
// reaches the table with nobody asked to import it. newFields and
// vanishedFields are what changed since the previous refresh, for the caller
// to report as a warning (see app.refreshState.addWarning). created is how
// many rows this refresh brought into the library that were not there when it
// started, which is what the page reports back to the reader.
//
// It is the whole refresh with no choices made and no report kept: the entry
// point of the command line and the tests. The server runs FetchModelsWith,
// which also answers which passes failed.
func FetchModels() (created int, newFields, vanishedFields []string, err error) {
	report, err := FetchModelsWith(Options{})
	return report.Created, report.NewFields, report.VanishedFields, err
}

// Options chooses which passes of a refresh run. The zero value runs them all.
type Options struct {
	// SkipPerModelPasses leaves out the zero-data-retention and throughput
	// passes, which each send one request per model (about 1,400 in all) to
	// OpenRouter's frontend endpoints. A skipped pass is not a failed one.
	SkipPerModelPasses bool
	// Paced sends the run's requests one at a time with a pause of about a
	// second between them (see pacer), for the silent daily run that nobody
	// waits on. The zero value keeps the per-model passes at 8 requests at a
	// time. A 429 or 503 slows every run down either way.
	Paced bool
}

// pacer is the pacer this run's requests go through: none when it is not paced.
func (o Options) pacer() *pacer {
	if o.Paced {
		return newPacer()
	}
	return nil
}

// Report is what one refresh did, beyond the error that ends it.
type Report struct {
	// Created is how many rows the refresh brought into the library.
	Created int
	// NewFields and VanishedFields are the feed paths that appeared or dropped
	// since the previous refresh.
	NewFields, VanishedFields []string
	// FailedParts names, in the words a reader sees, each fail-soft pass that did
	// not do its job: the refresh went on without it, and every value it would
	// have written stays as it was.
	FailedParts []string
}

// The names FailedParts carries. They are shown to the reader as they are.
const (
	PartZDR      = "zero data retention"
	PartEU       = "EU routing"
	PartSpeed    = "throughput"
	PartListings = "video and image lists"
)

// FetchModelsWith is FetchModels with the choice of passes and the report.
func FetchModelsWith(opts Options) (Report, error) {
	log.Printf("=== Fetching models from OpenRouter ===")
	pc := opts.pacer()
	data, raw, err := fetchFeedFrom(ApiUrl, pc)
	if err != nil {
		return Report{}, err
	}

	// Snapshot every name held before this refresh. It decides two things: which
	// feed records are NEW, which is the number the reader is told, and which
	// models a column made from the feed may be filled for at the end of this
	// function - so a name created below is added to it as it goes.
	//
	// THE ERROR IS FATAL TO THE REFRESH, not shrugged off. An empty map here
	// would make every feed record look new, which would report the whole feed
	// as added while stamping nothing wrong into the database.
	existing, err := heldNames()
	if err != nil {
		return Report{}, fmt.Errorf("read the names already held: %w", err)
	}

	// EVERY FEED RECORD IS STORED. A record naming a model this database does
	// not hold is created, visible, by the same upsert that refreshes a held
	// one. The one refusal is a record with no name, which is a row nothing
	// could address afterwards.
	//
	// idToName is the join key the video and image listings use below: OpenRouter's
	// own id, mapped to the row THIS general listing gave it. A dedicated listing
	// never creates a row of its own - it only writes onto a name found here.
	count, nameless, recorded, renamed, merged := 0, 0, 0, 0, 0
	var report Report
	snapshot := &renameSnapshot{}
	idToName := map[string]string{}
	for _, m := range data {
		norm := NormalizeModel(m)
		name := shared.GetStr(norm, "name")
		// A record upstream sends with no name is the ONE record this refresh
		// cannot store: models.name is the primary key and the handle every route
		// addresses a model by, so the row would exist and be unreachable. It is
		// counted and reported rather than passed over in silence.
		if strings.TrimSpace(name) == "" {
			nameless++
			continue
		}
		// A model OpenRouter renamed arrives under a name this catalog does not
		// hold, with an id it does: the row holding the id takes the new name (or
		// is folded into the row that already bears it) before the upsert, so the
		// upsert refreshes that row and no second one appears.
		if id := shared.GetStr(m, "id"); id != "" {
			done, err := store.ReconcileRenamedModel(id, name, snapshot.take)
			if err != nil {
				return Report{}, fmt.Errorf("follow the rename of %q: %w", name, err)
			}
			if done.Renamed != "" {
				delete(existing, done.Renamed)
				existing[name] = true
				renamed++
				log.Printf("  renamed %q to %q (OpenRouter id %s)", done.Renamed, name, id)
			}
			for _, gone := range done.Merged {
				delete(existing, gone)
				merged++
				log.Printf("  merged %q into %q (OpenRouter id %s)", gone, name, id)
			}
		}
		isNew := !existing[name]
		stored, err := store.UpsertAPIModel(norm)
		if err != nil {
			return Report{}, fmt.Errorf("upsert %q: %w", name, err)
		}
		if !stored {
			continue
		}
		// Counted where the row was actually written, and only once per name: the
		// number has to be rows this refresh created, not feed records it believed
		// were missing, so a name the feed lists twice cannot inflate it.
		if isNew {
			report.Created++
			existing[name] = true
		}
		if id := shared.GetStr(m, "id"); id != "" {
			idToName[id] = name
		}
		// The facts go on only where a row exists, and after it: they are cells,
		// which key on the model's name. Nobody is watching this pass, so it writes
		// only OpenRouter's own columns and can reach nothing the reader wrote -
		// see store.RecordFacts.
		written, err := store.RecordFacts(name, store.SourceOpenRouter, OpenRouterFacts(m))
		if err != nil {
			return Report{}, fmt.Errorf("record what OpenRouter states about %q: %w", name, err)
		}
		recorded += len(written)
		count++
	}
	log.Printf("  ok %d model(s) stored from the OpenRouter feed", count)
	log.Printf("  ok %d model(s) added to the library by this refresh", report.Created)
	log.Printf("  ok %d value(s) written into the columns you keep", recorded)
	if renamed > 0 || merged > 0 {
		log.Printf("  ok %d renamed model(s) followed and %d duplicate row(s) merged; snapshot %s", renamed, merged, snapshot.path)
	}
	if nameless > 0 {
		log.Printf("  %d record(s) arrived with no name and were not stored", nameless)
	}

	// The video and image listings: read AFTER the general listing, joined onto
	// the rows it just made, by id. Neither ever creates a row, and a listing
	// that cannot be read costs only itself - see readDedicatedListings.
	extraData, extraRaw, listingsErr := readDedicatedListings(idToName, pc)
	if listingsErr != nil {
		report.FailedParts = append(report.FailedParts, PartListings)
	}

	// Record every leaf path this refresh saw, over EVERY feed record - a
	// nameless one included, and every joined video and image record too,
	// because the three listings are read as one feed: a path is seen if any
	// of them carried it this refresh, so the general listing's own paths are
	// never reported vanished for want of the other two.
	allData := data
	allRaw := raw
	if len(extraData) > 0 {
		allData = append(append(make([]map[string]interface{}, 0, len(data)+len(extraData)), data...), extraData...)
		allRaw = append(append(make([]json.RawMessage, 0, len(raw)+len(extraRaw)), raw...), extraRaw...)
	}
	observed := observeFeedFields(allData)
	newFields, vanishedFields, err := store.RecordFeedFields(observed)
	if err != nil {
		log.Printf("Warning: recording feed fields failed: %v", err)
	}
	if len(newFields) > 0 {
		log.Printf("  new field(s) in the feed: %s", strings.Join(newFields, ", "))
	}
	if len(vanishedFields) > 0 {
		log.Printf("  field(s) that dropped out of the feed: %s", strings.Join(vanishedFields, ", "))
	}

	// Make a column for every recorded path nothing reads yet, and seal every
	// column tied to a path. One path that cannot be made costs only itself,
	// the same as one unusable value: nobody is watching this pass, and the
	// other paths and the rest of the refresh still go through. Recording
	// having failed above, there is nothing to tie a column to, so nothing is
	// made this time.
	if err == nil {
		makeFeedFieldColumns(observed)
	}

	// Fill every column tied to a feed path - the ones just made included - the
	// same way an ordinary OpenRouter column is kept current. existing is
	// passed because it now names the library this refresh leaves behind, the
	// rows it created included.
	tied, err := store.ImportedFeedFieldColumns()
	if err != nil {
		log.Printf("Warning: reading the columns tied to feed fields failed: %v", err)
	}
	for columnKey, path := range tied {
		column, err := store.ColumnByKey(columnKey)
		if err != nil {
			log.Printf("Warning: %s is tied to %s but its column could not be read: %v", columnKey, path, err)
			continue
		}
		filled, err := FillFeedFieldColumn(allData, allRaw, path, columnKey, column.ValueType, existing)
		if err != nil {
			log.Printf("Warning: filling %s from %s was partly refused (%d value(s) written): %v", columnKey, path, filled, err)
			continue
		}
		log.Printf("  ok %d value(s) written into %s (from the feed's %s)", filled, columnKey, path)
	}

	// A row the feed no longer lists under its stored name is never visited by
	// the loop above, so it would keep a type derived before the app learned to
	// type its kind. Retyping every OpenRouter row here types them all alike.
	if retyped, err := store.RederiveOpenRouterTypes(); err != nil {
		log.Printf("Warning: retyping the OpenRouter rows failed: %v", err)
	} else if retyped > 0 {
		log.Printf("  ok %d OpenRouter row(s) retyped from the modalities they hold", retyped)
	}

	// Derive Zero Data Retention from OpenRouter's frontend provider policies.
	// Fail-soft: a failure here leaves existing zdr values unchanged (handled
	// inside DeriveZDR) and never aborts the refresh.
	listedIDs := make(map[string]bool, len(idToName))
	for id := range idToName {
		listedIDs[id] = true
	}
	// The two per-model passes (this one and throughput) are the ones a caller may
	// leave out; see Options.SkipPerModelPasses.
	if !opts.SkipPerModelPasses {
		if err := DeriveZDR(listedIDs, pc); err != nil {
			log.Printf("Warning: ZDR derivation failed: %v", err)
			report.FailedParts = append(report.FailedParts, PartZDR)
		}
	}

	// Which models OpenRouter's EU address serves comes from one request. It
	// fails soft and never aborts the refresh (handled inside DeriveEURouting).
	if err := DeriveEURouting(listedIDs, pc); err != nil {
		report.FailedParts = append(report.FailedParts, PartEU)
	}

	// Throughput comes from OpenRouter's frontend stats feed, one call per model.
	// Fail-soft the same way: it never aborts the refresh.
	if !opts.SkipPerModelPasses {
		if err := DeriveThroughput(data, idToName, pc); err != nil {
			log.Printf("Warning: throughput derivation failed: %v", err)
			report.FailedParts = append(report.FailedParts, PartSpeed)
		}
	}

	report.NewFields, report.VanishedFields = newFields, vanishedFields
	return report, nil
}

// makeFeedFieldColumns creates a column for every observed path
// feedColumnCandidates names, then seals every column tied to a feed path,
// however it was made: a refresh would make such a column again, so a delete
// would be a lie, and sealing is what refuses it (store.DeleteColumn). Each
// failure is logged and costs only its own path.
func makeFeedFieldColumns(observed map[string]store.FieldObservation) {
	tied, err := store.ImportedFeedFieldColumns()
	if err != nil {
		log.Printf("Warning: reading the columns tied to feed fields failed, so no column was made this refresh: %v", err)
		return
	}
	tiedPaths := make(map[string]bool, len(tied))
	for _, path := range tied {
		tiedPaths[path] = true
	}
	var made []string
	for _, path := range limitNewFeedColumns(feedColumnCandidates(observed, tiedPaths)) {
		column, err := store.CreateFeedFieldColumn(path, observed[path].Kind)
		if err != nil {
			log.Printf("Warning: no column was made for the feed field %s: %v", path, err)
			continue
		}
		made = append(made, column.Key)
	}
	if len(made) > 0 {
		log.Printf("  ok %d column(s) made from the feed: %s", len(made), strings.Join(made, ", "))
	}
	sealed, err := store.SealFeedFieldColumns()
	if err != nil {
		log.Printf("Warning: sealing the columns tied to feed fields failed: %v", err)
	} else if sealed > 0 {
		log.Printf("  ok %d column(s) tied to the feed sealed", sealed)
	}
}

// maxNewFeedColumns caps how many columns one refresh makes from the feed. A
// feed change that put model ids or SKUs into object keys would otherwise make
// thousands of sealed columns, which nothing can delete, in a single pass.
const maxNewFeedColumns = 200

// limitNewFeedColumns keeps the first maxNewFeedColumns candidates and logs how
// many wait for a later refresh.
func limitNewFeedColumns(candidates []string) []string {
	if len(candidates) <= maxNewFeedColumns {
		return candidates
	}
	log.Printf("Warning: the feed offers %d new field(s); %d column(s) are made this refresh and the rest wait for the next", len(candidates), maxNewFeedColumns)
	return candidates[:maxNewFeedColumns]
}

// heldNames answers every name the models table holds right now.
func heldNames() (map[string]bool, error) {
	rows, err := dbcore.DB.Query("SELECT name FROM models")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	held := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			return nil, err
		}
		held[name] = true
	}
	return held, rows.Err()
}

// renameSnapshot takes the one snapshot a refresh owes before it first renames
// or merges a row, and never a second.
type renameSnapshot struct {
	path string
}

// take writes the snapshot unless this refresh already wrote it. It is the
// callback store.ReconcileRenamedModel runs before its first write.
func (s *renameSnapshot) take() error {
	if s.path != "" {
		return nil
	}
	path, err := dbcore.SnapshotBeforeRewrite()
	if err != nil {
		return err
	}
	s.path = path
	log.Printf("  snapshot written before the first rename or merge: %s", path)
	return nil
}
