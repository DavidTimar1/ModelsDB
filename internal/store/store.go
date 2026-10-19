// Higher-level DB operations: upserting API models and
// importing a full record from a HuggingFace repository.
//
// EVERY FACT IS A CELL. The models table carries the row's IDENTITY and nothing
// else: the name every value hangs off, the route that created the row, the
// moment of the last write, and the reader's two decisions about their own table
// - whether the row is shown, and which row it sits under. What each source
// stated goes into that source's own columns, so two sources describing one
// model never reach for the same value and nothing has to decide which of them
// wins.
//
// The database is the ONLY place any of this lives. Nothing in this package
// writes a model, a cell, a note or a favourite out to a file,
// so there is no second copy to keep in step and nothing personal can leak into
// one. A copy of the database file is the whole backup (see dbcore.BackupTo).

package store

import (
	"encoding/json"
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"strconv"
	"strings"
)

// hasErrorPrice reports whether any price field is a sentinel/error value
// (negative, e.g. -1 or -1000000) - such models are stored with no price at all.
func hasErrorPrice(pr map[string]interface{}) bool {
	for _, k := range []string{"prompt", "completion", "image", "audio", "internal_reasoning", "request"} {
		if v, ok := pr[k]; ok && v != nil {
			if f, err := strconv.ParseFloat(fmt.Sprintf("%v", v), 64); err == nil && f < 0 {
				return true
			}
		}
	}
	return false
}

// hasNoName reports whether a record carries no name to store it under.
//
// EVERY PATH THAT CREATES A ROW CALLS THIS, and refuses. models.name is the
// primary key and the only handle every route addresses a model by, so a row
// stored under an empty name could never be read back, written to, hidden or
// deleted: it would sit in the table permanently out of reach. Refusing at the
// moment of creation is the one way to make "every stored row is reachable" true
// by construction rather than by hoping no source ever sends a nameless record.
//
// It looks past whitespace only to decide whether a name is there AT ALL. A
// stored name is never trimmed - upstream really does ship names that end in a
// space, and trimming one would address a different row - so this asks the one
// question a lookup key cannot survive being wrong about.
func hasNoName(name string) bool {
	return strings.TrimSpace(name) == ""
}

// ModelExists reports whether a row is stored under exactly that name.
func ModelExists(name string) bool {
	var n int
	_ = dbcore.DB.QueryRow("SELECT COUNT(*) FROM models WHERE name=?", name).Scan(&n)
	return n > 0
}

func toStrList(v interface{}) []string {
	switch x := v.(type) {
	case []string:
		return x
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, e := range x {
			out = append(out, fmt.Sprintf("%v", e))
		}
		return out
	case string:
		return dbcore.ParseArr(dbcore.ToJSONArr(x))
	}
	return nil
}

func pricingOf(norm map[string]interface{}) map[string]interface{} {
	if ep, ok := norm["endpoint"].(map[string]interface{}); ok {
		if pr, ok := ep["pricing"].(map[string]interface{}); ok {
			return pr
		}
	}
	return map[string]interface{}{}
}

func priceStr(pr map[string]interface{}, key string) string {
	if v, ok := pr[key]; ok && v != nil {
		return fmt.Sprintf("%v", v)
	}
	return ""
}

// outputTypes maps one output modality to the model_type a model giving it out
// is. OpenRouter spells the media "embeddings", "rerank", "speech" and
// "transcription"; a HuggingFace import spells two of them "embedding" and
// "ranking", so both spellings are listed. A kind of model this app learns to
// type from what it gives out is one more line here.
//
// "audio" is deliberately absent: it does not say WHICH audio, so deriveType
// settles it from the description.
var outputTypes = map[string]string{
	"image":         "image",
	"video":         "video",
	"embeddings":    "embedding",
	"embedding":     "embedding",
	"rerank":        "rerank",
	"ranking":       "rerank",
	"speech":        "tts",
	"transcription": "stt",
	"decisions":     "decisions",
}

// deriveType infers a model_type from a model's output modalities, and for an
// audio model from what its description says it makes. Chat is the default.
// The first modality in the list that names a kind decides it.
//
// Audio output does not say WHICH audio. A music generator and a speech model
// carry the same modalities upstream - text in, audio out - so the modality
// lists alone put both under tts, which is wrong for half of them. The
// description is the only thing that separates the two, and
// dbcore.DescribesMusicGeneration holds that test: the migration that sorts the
// rows stored without the distinction applies exactly the same one.
//
// The music answer is reachable only from the audio branch, so the heuristic can
// never touch a chat, image or video model however the description is worded.
func deriveType(out []string, description string) string {
	for _, m := range out {
		modality := strings.ToLower(strings.TrimSpace(m))
		if modality == "audio" {
			if dbcore.DescribesMusicGeneration(description) {
				return "music"
			}
			return "tts"
		}
		if kind, named := outputTypes[modality]; named {
			return kind
		}
	}
	return "chat"
}

// UpsertAPIModel records what one normalized OpenRouter listing states, in
// OpenRouter's own columns, creating the row it hangs off if none exists yet.
// It reports whether the model ended up with a row.
//
// THE REFRESH CALLS THIS FOR EVERY RECORD THE FEED CARRIES, so a name this
// catalog does not hold yet arrives here and gets a row - visible, because
// hidden is not named and takes the schema default.
//
// A record carrying NO NAME is refused with ErrNoModelName and creates nothing,
// because a row under an empty name could never be read back or written to again
// (see hasNoName). Nothing else is refused: what a model is called decides
// nothing about whether it is stored.
//
// NOTHING IS MERGED HERE AND NOTHING IS OVERRULED. Every value written is what
// OpenRouter said, in OpenRouter's own cell. Where the repository states a
// longer context or the reader has answered for themselves, each of those is a
// cell of its own, and a read picks the winner among them.
func UpsertAPIModel(norm map[string]interface{}) (bool, error) {
	name := shared.GetStr(norm, "name")
	if hasNoName(name) {
		return false, ErrNoModelName
	}

	// A negative figure is OpenRouter's placeholder for a model it will not quote
	// a price for - the model is no longer servable there. It is still a real
	// model and is stored like any other; what it loses is the price. Blanking
	// the figures here is what carries that through, because every price cell
	// below is written from this map - a new row gets no price, and a row that
	// had real ones has them cleared, since the figures it holds have stopped
	// being true.
	//
	// A row is NEVER deleted for this. The row carries the reader's own notes,
	// favourite and column values, which exist in no other file and which a
	// foreign key would cascade away with it; a model that stops being sold is
	// not a reason to destroy what somebody wrote about it.
	pr := pricingOf(norm)
	if hasErrorPrice(pr) {
		pr = map[string]interface{}{}
	}

	if _, err := dbcore.DB.Exec(
		`INSERT INTO models (name, source, updated_at) VALUES (?,'openrouter',?)
		 ON CONFLICT(name) DO UPDATE SET source='openrouter', updated_at=excluded.updated_at`,
		name, dbcore.NowStamp()); err != nil {
		return false, err
	}
	if err := StampAdded(name); err != nil {
		return false, err
	}

	// The kind of model is read off what the LISTING says it produces, together
	// with what any other source has already said, because a model a repository
	// says makes images is one whichever source named it first.
	held, err := ModelCells(name)
	if err != nil {
		return false, err
	}
	outMods := dbcore.MergeArr(dbcore.ParseArr(held["output_modalities"]), toStrList(norm["output_modalities"]))
	description := shared.GetStr(norm, "description")

	supportsTool := false
	if ep, ok := norm["endpoint"].(map[string]interface{}); ok {
		supportsTool, _ = ep["supports_tool_parameters"].(bool)
	}
	toolSaid := "no"
	if supportsTool {
		toolSaid = "yes"
	}
	// The mixture-of-experts answer is read off the WORDING of the description, so
	// it is this listing's own answer and not a second source's. An answer nobody
	// has given is an empty cell.
	moeSaid := ""
	if lower := strings.ToLower(description); strings.Contains(lower, "moe") || strings.Contains(lower, "mixture") {
		moeSaid = "yes"
	}
	// An OpenRouter listing is priced per token, which is what the unit says. It
	// is stated only where there is a price to charge.
	priceUnit := ""
	if p := priceStr(pr, "prompt"); p != "" && p != "0" {
		priceUnit = "per 1M tokens"
	}
	reasoning, _ := norm["supports_reasoning"].(bool)

	recordCells(SourceOpenRouter, name, map[string]string{
		"id":                       shared.GetStr(norm, "id"),
		"slug":                     shared.GetStr(norm, "slug"),
		"repository":               shared.GetStr(norm, "hf_slug"),
		"description":              description,
		"created_at":               shared.GetStr(norm, "created_at"),
		"context_length":           numberCellText(dbcore.NullInt(norm["context_length"])),
		"input_modalities":         listCellText(toStrList(norm["input_modalities"])),
		"output_modalities":        listCellText(toStrList(norm["output_modalities"])),
		"supports_tools":           toolSaid,
		"supports_reasoning":       yesNoText(reasoning),
		"vendor":                   vendorOf(shared.GetStr(norm, "id")),
		"price_prompt":             numberOrEmpty(pr["prompt"]),
		"price_completion":         numberOrEmpty(pr["completion"]),
		"price_image":              numberOrEmpty(pr["image"]),
		"price_audio":              numberOrEmpty(pr["audio"]),
		"price_internal_reasoning": numberOrEmpty(pr["internal_reasoning"]),
		"model_type":               deriveType(outMods, description),
		"price_unit":               priceUnit,
		"moe":                      moeSaid,
	})
	rawJSON, _ := json.Marshal(norm)
	storeSourceRecord(SourceOpenRouter, name, string(rawJSON))
	return true, nil
}

// vendorOf is who OpenRouter attributes a model to, read off the part of the id
// before the slash: "google/gemini-3-pro" is google.
func vendorOf(id string) string {
	if i := strings.Index(id, "/"); i > 0 {
		return id[:i]
	}
	return ""
}

// ImportFullRecord records what an automatic source read about a model: the
// row it hangs off, and what that source stated, as that source's own cells.
// source is "huggingface" or "openrouter", the automatic sources; anything
// else is refused, because no other source reads a record. A record with no
// name is refused with ErrNoModelName, because a row nothing can address is a
// row nothing can ever reach again.
//
// AN EMPTY VALUE IS PASSED OVER RATHER THAN CLEARING A CELL. A caller that knows
// nothing about a column must not blank what somebody else researched. A
// non-empty value still wins in its own column, because importing a full
// record is what this function is for.
func ImportFullRecord(rec map[string]interface{}, source, modelType string) error {
	name := shared.GetStr(rec, "name")
	if hasNoName(name) {
		return ErrNoModelName
	}
	var src Source
	switch source {
	case "huggingface":
		src = SourceHuggingFace
	case "openrouter":
		src = SourceOpenRouter
	default:
		return fmt.Errorf("%w: %q reads no record; only huggingface and openrouter do", ErrColumnKey, source)
	}
	if modelType == "" {
		modelType = deriveType(toStrList(rec["output_modalities"]), shared.GetStr(rec, "description"))
	}

	if _, err := dbcore.DB.Exec(
		`INSERT INTO models (name, source, updated_at) VALUES (?,?,?)
		 ON CONFLICT(name) DO UPDATE SET updated_at=excluded.updated_at`,
		name, source, dbcore.NowStamp()); err != nil {
		return err
	}
	if err := StampAdded(name); err != nil {
		return err
	}
	values := map[string]string{
		"model_type":        modelType,
		"input_modalities":  listCellText(toStrList(rec["input_modalities"])),
		"output_modalities": listCellText(toStrList(rec["output_modalities"])),
		"repository":        shared.GetStr(rec, "hf_slug"),
		"description":       shared.GetStr(rec, "description"),
		"created_at":        shared.GetStr(rec, "created_at"),
		"context_length":    numberCellText(dbcore.NullInt(rec["context_length"])),
		"moe":               flagWordOrText(shared.GetStr(rec, "moe")),
	}
	// An identifier and a slug are OPENROUTER'S whichever record carried them,
	// so they go into OpenRouter's cells rather than into a second pair beside
	// them.
	recordNonEmpty(SourceOpenRouter, name, map[string]string{
		"id":   shared.GetStr(rec, "id"),
		"slug": shared.GetStr(rec, "slug"),
	})
	recordNonEmpty(src, name, values)

	rawJSON, _ := json.Marshal(rec)
	storeSourceRecord(src, name, string(rawJSON))
	return nil
}

// The answers a caller gets when a write cannot be made. Each one is a
// different reply over HTTP, so each is its own value rather than a message a
// handler has to read: a record with no name at all creates nothing and says so,
// an unknown model is 404, a model this catalog already holds is 409, and a key
// nothing writes by hand is a 400 carrying its own sentence.
//
// A VALUE A COLUMN WILL NOT HOLD is ErrCellValue (cells.go), because the column's
// own kind is what refuses it - one rule for a column the app ships and a column
// somebody invented alike.
var (
	ErrNoModelName = errors.New("a model row needs a name")
	ErrNoSuchModel = errors.New("no model of that name")
	ErrModelExists = errors.New("this catalog already holds that model")
	ErrNotWritable = errors.New("that column is not yours to write")
	// ErrNotAParent is a relation the table cannot draw: a variation is a CHILD
	// ROW of its parent, so nothing may be filed under a row that is itself
	// filed under something. It is a sentinel so a route can answer the caller
	// 4xx rather than 500 - the request asked for something impossible, and
	// nothing failed.
	ErrNotAParent = errors.New("a variation cannot have variations of its own")
)

// NormalizeModelName is the display name a HAND-ADDED row is stored under:
// trimmed at both ends, with every run of whitespace inside it collapsed to a
// single space. "  Qwen:   Qwen3 32B " becomes "Qwen: Qwen3 32B".
//
// IT APPLIES ONLY WHERE A ROW IS BEING CREATED BY HAND, and never to a name a
// refresh wrote. A stored name is never trimmed or collapsed after the fact:
// the one thing that changes a stored row's name is ReconcileRenamedModel
// following an OpenRouter rename, and it takes the name the feed gives.
func NormalizeModelName(name string) string {
	return strings.Join(strings.Fields(name), " ")
}
