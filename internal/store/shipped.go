// The columns this app ships, and the one place they are
// described.
//
// EVERY COLUMN HERE IS SEALED: it can be relabelled, moved and hidden, and it
// can never be deleted or re-typed. A source states its columns again on every
// read, so a deletion would either come straight back or have to be remembered
// for ever, and this app remembers neither.
//
// TWO KINDS OF COLUMN, AND THE LINE BETWEEN THEM IS WHO WRITES. A column an
// automatic source states - OpenRouter, a HuggingFace repository, or the app
// working a fact out from one of them - declares those sources in the order a
// read prefers them, then research last: a value found to fill a gap, which
// shows until an automatic source states the fact and is outranked the moment
// one does. A column of the reader's own - the note, the favourite and the
// architecture line - declares hand alone, and no update ever touches it. added
// declares the app alone. Sources IS the order; declareColumnSources
// (cells.go) ranks by position and refuses any other shape.
//
// THIS LIST IS THE ONLY PLACE A COLUMN'S SOURCES ARE WRITTEN. Nothing on the
// page or over the API changes them, so DeclareShippedSources reapplies them
// to every database at every start, and a source change never needs a
// migration again.
//
// WHAT IS SHOWN IS A DECISION, NOT A CONSEQUENCE. A short set of columns is
// visible on a fresh install and every other column exists, holds its values and
// sits behind the column chooser. The visible set never changes on its own:
// seeding an install that already has these columns changes nothing at all, so a
// column somebody hid stays hidden, a column somebody relabelled keeps its label,
// and an option list somebody edited is left alone.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"sort"
	"strings"
)

// shippedColumn is one column the app ships.
type shippedColumn struct {
	Key       string
	Label     string
	ValueType string
	Unit      string
	About     string
	Visible   bool
	Resolve   string // "" means first_stated
	Sources   []Source
	Colors    *ColorRules // the colours it ships with, where it ships with any
	// Derived is set on a column the app WORKS OUT from an OpenRouter fact
	// rather than copies: what it is worked out from, in the words the page
	// shows. The refresh writes such a column as openrouter, so no source list
	// can tell it from a fetched one, which is why this is a field.
	Derived string
}

// priceColors are the thresholds the two price columns ship with, read against
// the figure the cell SHOWS - a price per million tokens: under a dollar green,
// under two yellow, above that red. Nothing else about them is special, and the
// reader may change or clear them exactly as on any other column.
func priceColors() *ColorRules {
	return &ColorRules{
		Thresholds: []ColorThreshold{{Below: 1, Color: "#22c55e"}, {Below: 2, Color: "#eab308"}},
		Otherwise:  "#ef4444",
	}
}

// shippedColumns is the whole set, in the order it is laid out left to right.
// The visible ones lead, because position is this list's own order.
//
// One column answers an attribute however many sources state it. The order
// between two automatic sources is OpenRouter first where OpenRouter serves the
// fact, HuggingFace first where a repository is read for it.
var shippedColumns = []shippedColumn{
	// What the table shows on a fresh install.
	{Key: "model_type", Label: "Type", ValueType: ValueTypeText, Visible: true,
		About:   "What kind of model this is: chat, image, embedding.",
		Sources: []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch},
		Derived: "what OpenRouter says the model gives out, and its description for audio"},
	{Key: "context_length", Label: "Context length", ValueType: ValueTypeNumber, Unit: "tokens", Visible: true,
		About:   "How much context the model takes.",
		Sources: []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch}},
	{Key: "price_prompt", Label: "In $", ValueType: ValueTypeNumber, Unit: "USD per token", Visible: true,
		About:   "What one input token costs.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Colors:  priceColors()},
	{Key: "price_completion", Label: "Out $", ValueType: ValueTypeNumber, Unit: "USD per token", Visible: true,
		About:   "What one output token costs.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Colors:  priceColors()},
	{Key: "supports_tools", Label: "Tool", ValueType: ValueTypeFlag, Visible: true,
		About:   "Whether the model calls tools.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Derived: "OpenRouter's supported parameters (tools or tool_choice)"},
	{Key: "supports_reasoning", Label: "Reasoning", ValueType: ValueTypeFlag, Visible: true,
		About:   "Whether the model reasons.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Derived: "OpenRouter's supported parameters (reasoning)"},
	{Key: "zdr", Label: "ZDR", ValueType: ValueTypeFlag, Visible: true,
		About:   "Zero Data Retention: at least one company serving this model keeps no copy of what you send it.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Derived: "OpenRouter's provider data policies"},
	{Key: "eu_routing", Label: "EU", ValueType: ValueTypeFlag, Visible: true,
		About:   "Whether OpenRouter can route this model through its EU address (In-Region Routing, which needs a Business or Enterprise plan): an endpoint for it is onboarded in the EU.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Derived: "OpenRouter's EU model list"},
	{Key: "throughput", Label: "Throughput", ValueType: ValueTypeNumber, Unit: "tokens per second", Visible: true,
		About:   "How fast the model answers, in tokens per second: the median speed of the fastest provider OpenRouter measures for it, over the last half hour. Empty where no provider is measured.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Derived: "the fastest provider's median speed on OpenRouter"},
	{Key: "notes", Label: "Note", ValueType: ValueTypeText, Visible: true,
		About:   "Anything you want to remember about this model.",
		Sources: []Source{SourceHand}},
	{Key: "favorite", Label: "Favourite", ValueType: ValueTypeFlag, Visible: true,
		About:   "Your own star.",
		Sources: []Source{SourceHand}},

	// The other columns more than one source can state, present and hidden.
	{Key: "moe", Label: "MoE", ValueType: ValueTypeFlag,
		About:   "Whether the model is a mixture of experts.",
		Sources: []Source{SourceHuggingFace, SourceOpenRouter, SourceResearch},
		Derived: "the words of the OpenRouter description"},

	// What the OpenRouter public API states, one column per attribute only it
	// can state.
	{Key: "id", Label: "OpenRouter id", ValueType: ValueTypeText, About: "The identifier OpenRouter serves this model under.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "slug", Label: "OpenRouter slug", ValueType: ValueTypeText, About: "The canonical slug of the listing.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_image", Label: "Image $", ValueType: ValueTypeNumber, Unit: "USD per image", About: "What one input image costs.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_audio", Label: "Audio $", ValueType: ValueTypeNumber, Unit: "USD per token", About: "What one audio input token costs.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_internal_reasoning", Label: "Reasoning $", ValueType: ValueTypeNumber, Unit: "USD per token", About: "What one internal reasoning token costs.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "vendor", Label: "Vendor", ValueType: ValueTypeText, About: "Who OpenRouter attributes the model to, read off the part of the id before the slash: google, mistralai, nvidia.", Sources: []Source{SourceOpenRouter, SourceResearch},
		Derived: "the part of the OpenRouter id before the slash"},
	{Key: "tokenizer", Label: "Tokenizer", ValueType: ValueTypeText, About: "Which tokenizer family the model uses, as OpenRouter classifies it.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "instruct_type", Label: "Instruct type", ValueType: ValueTypeText, About: "Which prompt template the model expects: chatml, llama3, deepseek-r1.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	// A LIST rather than text, so one member can be searched for and the column
	// does not sort by its own first letters. A sealed column can never be
	// re-typed, so this is decided here or not at all.
	{Key: "supported_parameters", Label: "Supported parameters", ValueType: ValueTypeList, About: "Which request parameters the model accepts: temperature, top_p, seed.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "max_completion_tokens", Label: "Max completion tokens", ValueType: ValueTypeNumber, Unit: "tokens", About: "The longest answer the model will produce in one call.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "moderated", Label: "Moderated", ValueType: ValueTypeFlag, About: "Whether the provider runs its own moderation over the calls.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "knowledge_cutoff", Label: "Knowledge cutoff", ValueType: ValueTypeText, About: "The date the model's training data stops at.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "expiration_date", Label: "Expiration date", ValueType: ValueTypeText, About: "The date the listing stops being served.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "reasoning_mandatory", Label: "Reasoning mandatory", ValueType: ValueTypeFlag, About: "Whether the model always reasons and cannot be asked not to.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "alias_target", Label: "Alias target", ValueType: ValueTypeText, About: "The model this listing is an alias of, where it is one.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_cache_read", Label: "Cache read price", ValueType: ValueTypeNumber, Unit: "USD per token", About: "What one cached input token costs to read back.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_cache_write", Label: "Cache write price", ValueType: ValueTypeNumber, Unit: "USD per token", About: "What one input token costs to put into the cache.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_cache_write_1h", Label: "Cache write price (1h)", ValueType: ValueTypeNumber, Unit: "USD per token", About: "The same, for a cache entry kept an hour.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_audio_cache_read", Label: "Audio cache read price", ValueType: ValueTypeNumber, Unit: "USD per token", About: "What one cached audio input token costs to read back.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_audio_output", Label: "Audio output price", ValueType: ValueTypeNumber, Unit: "USD per token", About: "What one produced audio token costs.", Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_web_search", Label: "Web search price", ValueType: ValueTypeNumber, Unit: "USD per search", About: "What one web search made on the model's behalf costs.", Sources: []Source{SourceOpenRouter, SourceResearch}},

	// The three video/image input facts: fed from the video and image listings'
	// own fields by explicit code (internal/catalog/dedicatedlistings.go), not
	// by the door's generic fill, because a plain key and a specific label are
	// wanted rather than the path-derived ones the door would give.
	// max_reference_images and reference_images also take a manual research
	// pass for a video model, since OpenRouter states the field structurally
	// only for images today.
	{Key: "frame_images", Label: "Frame images", ValueType: ValueTypeList,
		About:   "Which frame images a video model takes: first_frame, last_frame, or both.",
		Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "max_reference_images", Label: "Reference images, at most", ValueType: ValueTypeNumber,
		About:   "How many reference images the model takes at most, as its OpenRouter listing or its model card states it; empty where nobody has said.",
		Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "reference_images", Label: "Takes reference images", ValueType: ValueTypeFlag,
		About:   "Whether the model takes reference images at all.",
		Sources: []Source{SourceOpenRouter, SourceResearch}},

	// The other columns more than one source can state. The description, the
	// repository, the dates, the modalities and the pricing unit are visible;
	// price_image_output stays hidden, matching the other price columns beside
	// price_prompt and price_completion.
	{Key: "repository", Label: "Repository", ValueType: ValueTypeText, Visible: true,
		About:   "The HuggingFace repository this model's listing names.",
		Sources: []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch}},
	{Key: "description", Label: "Description", ValueType: ValueTypeText, Visible: true,
		About:   "The description published for this model.",
		Sources: []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch}},
	{Key: "created_at", Label: "Listed on", ValueType: ValueTypeText, Visible: true,
		About:   "When the listing or repository was created.",
		Sources: []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch}},
	{Key: "input_modalities", Label: "Takes in", ValueType: ValueTypeList, Visible: true, Resolve: ResolveUnion,
		About:   "What the model accepts: text, image, audio.",
		Sources: []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch}},
	{Key: "output_modalities", Label: "Gives out", ValueType: ValueTypeList, Visible: true, Resolve: ResolveUnion,
		About:   "What the model produces.",
		Sources: []Source{SourceOpenRouter, SourceHuggingFace, SourceResearch}},
	{Key: "price_image_output", Label: "Image output price", ValueType: ValueTypeNumber, Unit: "USD per image",
		About:   "What one produced image costs.",
		Sources: []Source{SourceOpenRouter, SourceResearch}},
	{Key: "price_unit", Label: "Priced per", ValueType: ValueTypeText, Visible: true,
		About:   "What the model is charged by: 1M tokens, a second, a megapixel.",
		Sources: []Source{SourceOpenRouter, SourceResearch},
		Derived: "whether OpenRouter lists an input price"},

	// What a HuggingFace repository states, one column per attribute only it
	// can state.
	{Key: "repo_owner", Label: "Repo owner", ValueType: ValueTypeText, About: "The account that published that repository, which is often somebody who republished a build rather than the model's maker: unsloth, bartowski.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "parameters", Label: "Parameters (B)", ValueType: ValueTypeNumber, Unit: "billions", About: "How many parameters the model has, in billions.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "active_parameters", Label: "Active parameters (B)", ValueType: ValueTypeNumber, Unit: "billions", About: "How many parameters a mixture-of-experts model actually runs per token, in billions. It equals the total on a model that is not one.", Sources: []Source{SourceResearch}},
	// One quantity in two units, so both labels carry the unit: a reader meeting
	// either one alone can tell which figure it is and that the other exists.
	{Key: "weights_size_gb", Label: "Weights size (GB)", ValueType: ValueTypeNumber, Unit: "GB", About: "How much disk the model's own weight files take.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	// What the repository's own quantization_config CLAIMS, as against
	// build_bits_per_weight, which is the same quantity measured off a
	// downloadable build's files. The name says which of the two this is.
	{Key: "build_bits_per_weight", Label: "Build bits per weight", ValueType: ValueTypeNumber, Unit: "bits", About: "How many bits one weight takes in THIS downloadable build, measured from its own files.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "build_format", Label: "Build format", ValueType: ValueTypeText, About: "How the build is stored, with its quantization label: GGUF Q4_K_M, AWQ 4-bit, FP8.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "build_revision", Label: "Build revision", ValueType: ValueTypeText, About: "The exact commit that repository pointed at when it was read.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "build_files", Label: "Build files", ValueType: ValueTypeText, About: "Which file or files make up the build.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "build_size_bytes", Label: "Build size (bytes)", ValueType: ValueTypeNumber, Unit: "bytes", About: "The exact size of this build's own files, which is a different figure from the model's weights size.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "base_model", Label: "Base model", ValueType: ValueTypeText, About: "The model this build was made from, as the repository states it.", Sources: []Source{SourceHuggingFace, SourceResearch}},
	{Key: "gguf_architecture", Label: "GGUF architecture", ValueType: ValueTypeText, About: "The architecture name inside a GGUF file.", Sources: []Source{SourceHuggingFace, SourceResearch}},

	// The fact line about how a model is built: layers, cache, heads, weights.
	// No source states it as one line, so it declares hand alone like the note,
	// and a person or assistant writes it through POST /api/save after reading
	// the repository's config. The key is the one the column held before it
	// shipped, kept so the lines already written and every saved view stay where
	// they are. See dbcore/architecturefold.go.
	{Key: dbcore.ArchitectureColumnKey, Label: dbcore.ArchitectureColumnLabel, ValueType: ValueTypeText,
		About:   dbcore.ArchitectureColumnAbout,
		Sources: []Source{SourceHand}},

	// LAST, AND IT HAS TO STAY LAST. The migration that adds this column to a
	// catalog that already exists puts it one past the highest position in use, so
	// a fresh install and a carried-forward one end at the same place only while
	// this is the final entry of this list. See dbcore/addedcolumn.go and
	// TestTheAddedColumnShipsLastAndSealed.
	//
	// ITS ONLY SOURCE IS THE APP ITSELF: no source can ever restate the moment a
	// row entered this catalog, nothing may type over it, and nothing can fill
	// it in, so it declares neither hand nor research.
	{Key: "added", Label: "Added", ValueType: ValueTypeText,
		About:   "When this model first entered your catalog. It is not a listing date: no source states it, and nothing can work it out again, so it is written once when the row is created and never rewritten.",
		Sources: []Source{SourceApp}},
}

// seedMarkerColumn is the LAST column SeedShippedColumns creates, so a database
// holding it has been seeded whole.
const seedMarkerColumn = "added"

// ensureShippedColumns seeds the columns the app ships into a database that has
// not got them yet. The server seeds at startup; this covers every other way the
// store is reached, so a write never lands somewhere with no column to land in.
func ensureShippedColumns() error {
	_, err := ColumnByKey(seedMarkerColumn)
	if err == nil {
		return nil
	}
	if !errors.Is(err, ErrNoSuchColumn) {
		return err
	}
	_, err = SeedShippedColumns()
	return err
}

// SeedShippedColumns puts the columns this app ships into a catalog that does
// not have them yet, and answers the keys it created.
//
// IT IS IDEMPOTENT AND ADDITIVE. A column whose key is already there is left
// exactly as it is - its label, its position, whether it is shown and its
// option list - because every one of those is something a person may have
// changed on purpose (relabelling and hiding), and a seed that corrected them
// would undo that work every time the app started. Its SOURCES are the one
// thing the seed does not own: nothing lets anyone change them, so they are
// code, and DeclareShippedSources reapplies them at every start.
func SeedShippedColumns() ([]string, error) {
	var created []string
	for position, shipped := range shippedColumns {
		made, err := seedOneColumn(shipped, position)
		if err != nil {
			return created, err
		}
		if made {
			created = append(created, shipped.Key)
		}
	}
	return created, nil
}

// seedOneColumn creates one shipped column, answering whether it made it. A key
// that is already there is left untouched.
func seedOneColumn(shipped shippedColumn, position int) (bool, error) {
	if _, err := ColumnByKey(shipped.Key); err == nil {
		return false, nil
	} else if !errors.Is(err, ErrNoSuchColumn) {
		return false, err
	}
	_, err := CreateColumn(ColumnSpec{
		Key:       shipped.Key,
		Label:     shipped.Label,
		ValueType: shipped.ValueType,
		Resolve:   shipped.Resolve,
		Unit:      shipped.Unit,
		About:     shipped.About,
		Position:  position,
		Hidden:    !shipped.Visible,
		Sealed:    true,
		Colors:    shipped.Colors,
	}, shipped.Sources...)
	if err != nil {
		return false, fmt.Errorf("seed %s: %w", shipped.Key, err)
	}
	return true, nil
}

// shippedByKey indexes shippedColumns for the lookups that do not care about
// the order.
var shippedByKey = indexShipped()

func indexShipped() map[string]shippedColumn {
	m := make(map[string]shippedColumn, len(shippedColumns))
	for _, c := range shippedColumns {
		m[c.Key] = c
	}
	return m
}

// dateColumnKeys are the shipped columns that hold a date, as a plain day or a
// timestamp beginning with one. They are the only columns a from: or to: range
// can ask about, and the page draws two date pickers for exactly these. A
// column of the reader's own is never a date: the five kinds of value have no
// date among them.
var dateColumnKeys = map[string]bool{
	"created_at": true, "added": true, "expiration_date": true, "knowledge_cutoff": true,
}

// IsDateColumn reports whether key is a column that holds a date.
func IsDateColumn(key string) bool { return dateColumnKeys[key] }

// DerivedFrom answers what a shipped column is worked out from, in the words
// the page shows, and "" for a column that is copied rather than worked out.
func DerivedFrom(key string) string {
	return shippedByKey[key].Derived
}

// The kinds of column the page draws, computed by ColumnKind.
const (
	KindFetched  = "fetched"
	KindMeasured = "measured"
	KindDerived  = "derived"
	KindStamped  = "stamped"
	KindYours    = "yours"
)

// ColumnKind is the one word that says what a column IS, read off who
// declares it: stamped when the app alone does, yours when hand alone does,
// derived when shipped.go marks it as worked out, measured when a repository
// is the first source, and fetched otherwise.
func ColumnKind(column Column, sources []Source) string {
	switch {
	case len(sources) == 1 && sources[0] == SourceApp:
		return KindStamped
	case len(sources) == 1 && sources[0] == SourceHand:
		return KindYours
	case DerivedFrom(column.Key) != "":
		return KindDerived
	case len(sources) > 0 && sources[0] == SourceHuggingFace:
		return KindMeasured
	}
	return KindFetched
}

// DeclareShippedSources rewrites every column's declared sources from what
// the code says, at every start, after SeedShippedColumns: a shipped column
// takes its Sources list here, a column tied to a feed path takes openrouter
// then research, and a column the reader made takes hand alone. This is what
// makes shipped.go the only place a column's order lives, on a database
// carried forward as much as on a fresh one. A shipped column's About sentence
// is reapplied the same way: it is the app's own description, which nothing
// lets the reader edit, and a database seeded before the sentence changed
// would otherwise serve the old one for ever.
//
// IT NEVER STOPS THE SERVER. A stored cell whose source the new list does not
// declare would be stranded by the rewrite, and removing a declaration from
// under a stored cell is a migration's job, never a seed's. So a column holding such a cell is SKIPPED: its declarations
// stay as they were, the log names the column and the stranded source, and
// the start goes on. Failure is soft everywhere else in this app, and it is
// soft here too. The answer is how many columns were rewritten.
func DeclareShippedSources() (int, error) {
	tied, err := ImportedFeedFieldColumns()
	if err != nil {
		return 0, err
	}
	columns, err := ListColumns()
	if err != nil {
		return 0, err
	}

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	rewritten := 0
	for _, column := range columns {
		if shipped, ok := shippedByKey[column.Key]; ok && column.Sealed && column.About != shipped.About {
			if _, err := tx.Exec("UPDATE columns SET about=? WHERE id=?", shipped.About, column.ID); err != nil {
				return 0, fmt.Errorf("reapply the description of %s: %w", column.Key, err)
			}
		}
		// A column made from the feed before it was described gets its sentence
		// now. Only an EMPTY description is filled, so nothing written since is
		// replaced.
		if path, fromFeed := tied[column.Key]; fromFeed && column.Sealed && column.About == "" {
			if _, err := tx.Exec("UPDATE columns SET about=? WHERE id=?", feedFieldAbout(column.Key, path), column.ID); err != nil {
				return 0, fmt.Errorf("describe %s: %w", column.Key, err)
			}
		}
		wanted, known := intendedSources(column, tied)
		if !known {
			log.Printf("Warning: %s is sealed but neither shipped nor made from the feed, so its sources were left as they are", column.Key)
			continue
		}
		stranded, err := cellSourcesOutside(tx, column.ID, wanted)
		if err != nil {
			return 0, err
		}
		if len(stranded) > 0 {
			log.Printf("ERROR: %s holds cells stated by %s, which its sources no longer declare; its declarations were left as they were. A migration is what moves those cells", column.Key, strings.Join(stranded, ", "))
			continue
		}
		if _, err := tx.Exec("DELETE FROM column_sources WHERE column_id=?", column.ID); err != nil {
			return 0, fmt.Errorf("clear the sources of %s before declaring them: %w", column.Key, err)
		}
		if err := declareColumnSources(tx, column.ID, wanted); err != nil {
			return 0, fmt.Errorf("declare the sources of %s: %w", column.Key, err)
		}
		rewritten++
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return rewritten, nil
}

// intendedSources answers the list a column should declare, and whether the
// code knows the column at all.
func intendedSources(column Column, tiedToFeed map[string]string) ([]Source, bool) {
	if !column.Sealed {
		return []Source{SourceHand}, true
	}
	if shipped, ok := shippedByKey[column.Key]; ok {
		return shipped.Sources, true
	}
	if _, ok := tiedToFeed[column.Key]; ok {
		return feedColumnSources(), true
	}
	return nil, false
}

// cellSourcesOutside names every source that has a stored cell on the column
// and is not in the list, sorted so the log reads the same way each start.
func cellSourcesOutside(tx *sql.Tx, columnID int64, wanted []Source) ([]string, error) {
	rows, err := tx.Query("SELECT DISTINCT source FROM cells WHERE column_id=?", columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	declared := map[Source]bool{}
	for _, src := range wanted {
		declared[src] = true
	}
	var out []string
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return nil, err
		}
		if !declared[Source(src)] {
			out = append(out, src)
		}
	}
	sort.Strings(out)
	return out, rows.Err()
}
