// Package store - higher-level DB operations: upserting API models (preserving
// curated data), importing full records (HuggingFace / manual / collection),
// updating curated fields, enriching capabilities, and exporting curated.json.
package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"os"
	"sort"
	"strconv"
	"strings"
)

// hasErrorPrice reports whether any price field is a sentinel/error value
// (negative, e.g. -1 or -1000000) - such models are dropped on import.
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

// deriveType infers a model_type from output modalities (chat is the default).
func deriveType(out []string) string {
	for _, m := range out {
		switch strings.ToLower(m) {
		case "image":
			return "image"
		case "video":
			return "video"
		case "audio":
			return "tts"
		}
	}
	return "chat"
}

func jsonArrStr(list []string) string {
	if len(list) == 0 {
		return ""
	}
	b, _ := json.Marshal(list)
	return string(b)
}

// UpsertAPIModel writes a normalized OpenRouter model: metadata columns are
// refreshed, modalities are UNIONed with whatever is already stored (so
// collection-derived capabilities are never lost), and curated columns are
// left untouched (seeded only on first insert).
func UpsertAPIModel(norm map[string]interface{}) error {
	name := shared.GetStr(norm, "name")
	if name == "" {
		return nil
	}

	// Drop models whose pricing carries an error sentinel (e.g. -1 routers).
	if hasErrorPrice(pricingOf(norm)) {
		dbcore.DB.Exec("DELETE FROM models WHERE name=?", name)
		return nil
	}

	var exIn, exOut string
	err := dbcore.DB.QueryRow("SELECT input_modalities, output_modalities FROM models WHERE name=?", name).Scan(&exIn, &exOut)
	exists := err == nil
	if err != nil && err != sql.ErrNoRows {
		return err
	}

	inMods := dbcore.MergeArr(dbcore.ParseArr(exIn), toStrList(norm["input_modalities"]))
	outMods := dbcore.MergeArr(dbcore.ParseArr(exOut), toStrList(norm["output_modalities"]))

	pr := pricingOf(norm)
	supportsTool := false
	if ep, ok := norm["endpoint"].(map[string]interface{}); ok {
		supportsTool, _ = ep["supports_tool_parameters"].(bool)
	}
	toolSeed := "no"
	if supportsTool {
		toolSeed = "yes"
	}
	moeSeed := "unknown"
	desc := strings.ToLower(shared.GetStr(norm, "description"))
	if strings.Contains(desc, "moe") || strings.Contains(desc, "mixture") {
		moeSeed = "yes"
	}
	rawJSON, _ := json.Marshal(norm)
	hidden := 0
	if b, ok := norm["hidden"].(bool); ok && b {
		hidden = 1
	}
	reasoning := 0
	if b, ok := norm["supports_reasoning"].(bool); ok && b {
		reasoning = 1
	}
	// API models are priced per token; seed the measurement unit on first insert
	// (never on conflict, so a manual edit is preserved).
	measurementSeed := ""
	if p := priceStr(pr, "prompt"); p != "" && p != "0" {
		measurementSeed = "per 1M tokens"
	}

	_, err = dbcore.DB.Exec(`
INSERT INTO models
 (name, source, model_type, id, slug, hf_slug, description, context_length,
  input_modalities, output_modalities, supports_reasoning, hidden,
  price_prompt, price_completion, price_image, price_audio, price_internal_reasoning,
  measurement, tool, moe, raw_json, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(name) DO UPDATE SET
  source='openrouter',
  model_type=CASE WHEN models.model_type IN ('','chat') THEN excluded.model_type ELSE models.model_type END,
  id=excluded.id, slug=excluded.slug, hf_slug=excluded.hf_slug,
  description=excluded.description, context_length=excluded.context_length,
  input_modalities=excluded.input_modalities, output_modalities=excluded.output_modalities,
  supports_reasoning=excluded.supports_reasoning, hidden=excluded.hidden,
  price_prompt=excluded.price_prompt, price_completion=excluded.price_completion,
  price_image=excluded.price_image, price_audio=excluded.price_audio,
  price_internal_reasoning=excluded.price_internal_reasoning,
  raw_json=excluded.raw_json, created_at=excluded.created_at, updated_at=excluded.updated_at`,
		name, "openrouter", deriveType(outMods), shared.GetStr(norm, "id"), shared.GetStr(norm, "slug"), shared.GetStr(norm, "hf_slug"),
		shared.GetStr(norm, "description"), dbcore.NullInt(norm["context_length"]),
		jsonArrStr(inMods), jsonArrStr(outMods), reasoning, hidden,
		priceStr(pr, "prompt"), priceStr(pr, "completion"), priceStr(pr, "image"), priceStr(pr, "audio"), priceStr(pr, "internal_reasoning"),
		measurementSeed, toolSeed, moeSeed, string(rawJSON), shared.GetStr(norm, "created_at"), dbcore.NowStamp())
	_ = exists
	return err
}

// ImportFullRecord inserts a non-API model (HuggingFace / manual / collection)
// with full metadata AND any curated fields present in rec. On conflict it
// refreshes metadata and unions modalities but keeps existing curated values.
func ImportFullRecord(rec map[string]interface{}, source, modelType string) error {
	name := shared.GetStr(rec, "name")
	if name == "" {
		return nil
	}
	if modelType == "" {
		modelType = deriveType(toStrList(rec["output_modalities"]))
	}

	var exIn, exOut string
	_ = dbcore.DB.QueryRow("SELECT input_modalities, output_modalities FROM models WHERE name=?", name).Scan(&exIn, &exOut)
	inMods := dbcore.MergeArr(dbcore.ParseArr(exIn), toStrList(rec["input_modalities"]))
	outMods := dbcore.MergeArr(dbcore.ParseArr(exOut), toStrList(rec["output_modalities"]))

	rawJSON, _ := json.Marshal(rec)
	reasoning := 0
	if b, ok := rec["supports_reasoning"].(bool); ok && b {
		reasoning = 1
	}
	tool := shared.GetStr(rec, "tool")
	if tool == "" {
		tool = "no"
	}
	moe := shared.GetStr(rec, "moe")
	if moe == "" {
		moe = "unknown"
	}
	fav := 0
	if shared.GetBool(rec, "favorite") {
		fav = 1
	}

	_, err := dbcore.DB.Exec(`
INSERT INTO models
 (name, source, model_type, id, slug, hf_slug, author, description, context_length,
  input_modalities, output_modalities, supports_reasoning,
  price_prompt, price_completion, price_image, price_audio, price_internal_reasoning,
  price_display, pricing_url, measurement, pricing_note,
  notes, favorite, tool, moe, parameters, active_parameters, disk_size_gb,
  raw_json, created_at, updated_at)
VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)
ON CONFLICT(name) DO UPDATE SET
  model_type=CASE WHEN models.model_type IN ('','chat') THEN excluded.model_type ELSE models.model_type END,
  hf_slug=excluded.hf_slug, author=excluded.author,
  description=CASE WHEN models.description='' THEN excluded.description ELSE models.description END,
  context_length=excluded.context_length,
  input_modalities=excluded.input_modalities, output_modalities=excluded.output_modalities,
  supports_reasoning=excluded.supports_reasoning,
  price_prompt=excluded.price_prompt, price_completion=excluded.price_completion,
  price_image=excluded.price_image, price_display=excluded.price_display,
  pricing_url=excluded.pricing_url,
  measurement=excluded.measurement, pricing_note=excluded.pricing_note,
  updated_at=excluded.updated_at`,
		name, source, modelType, shared.GetStr(rec, "id"), shared.GetStr(rec, "slug"), shared.GetStr(rec, "hf_slug"), shared.GetStr(rec, "author"),
		shared.GetStr(rec, "description"), dbcore.NullInt(rec["context_length"]),
		jsonArrStr(inMods), jsonArrStr(outMods), reasoning,
		priceFromRec(rec, "price_prompt", "prompt"), priceFromRec(rec, "price_completion", "completion"),
		priceFromRec(rec, "price_image", "image"), priceFromRec(rec, "price_audio", "audio"),
		priceFromRec(rec, "price_internal_reasoning", "internal_reasoning"),
		shared.GetStr(rec, "price_display"), shared.GetStr(rec, "pricing_url"), shared.GetStr(rec, "measurement"), shared.GetStr(rec, "pricing_note"),
		shared.GetStr(rec, "notes"), fav,
		tool, moe, dbcore.NullInt(rec["parameters"]), dbcore.NullInt(rec["active_parameters"]), dbcore.NullFloat(rec["disk_size_gb"]),
		string(rawJSON), shared.GetStr(rec, "created_at"), dbcore.NowStamp())
	return err
}

// priceFromRec reads a price preferring the flat key (curated.json export shape)
// then the nested endpoint.pricing key (API/page shape).
func priceFromRec(rec map[string]interface{}, flatKey, nestedKey string) string {
	if v := shared.GetStr(rec, flatKey); v != "" {
		return v
	}
	return priceStr(pricingOf(rec), nestedKey)
}

// SetPricing updates the scraped pricing fields for a model by name. measurement
// (the pricing unit) is derived from the page; it overwrites only when non-empty
// so a manually-set unit survives a re-scrape that could not classify it.
func SetPricing(name, prompt, completion, image, display, url, measurement string) error {
	_, err := dbcore.DB.Exec(`UPDATE models SET price_prompt=?, price_completion=?, price_image=?,
		price_display=?, pricing_url=?,
		measurement=COALESCE(NULLIF(?,''), measurement), updated_at=? WHERE name=?`,
		prompt, completion, image, display, url, measurement, dbcore.NowStamp(), name)
	return err
}

// UpdateCurated writes only curated columns for a model (insert minimal row if
// it does not exist yet, so notes are never dropped).
func UpdateCurated(name string, c map[string]interface{}) error {
	if name == "" {
		return nil
	}
	var n int
	_ = dbcore.DB.QueryRow("SELECT COUNT(*) FROM models WHERE name=?", name).Scan(&n)
	fav := 0
	if shared.GetBool(c, "favorite") {
		fav = 1
	}
	if n == 0 {
		_, err := dbcore.DB.Exec(`INSERT INTO models (name, source, updated_at) VALUES (?,?,?)`, name, "manual", dbcore.NowStamp())
		if err != nil {
			return err
		}
	}
	_, err := dbcore.DB.Exec(`UPDATE models SET notes=?, favorite=?,
		tool=COALESCE(NULLIF(?,''), tool), moe=COALESCE(NULLIF(?,''), moe),
		parameters=?, active_parameters=?, disk_size_gb=?, measurement=?, pricing_note=?, updated_at=? WHERE name=?`,
		shared.GetStr(c, "notes"), fav,
		shared.GetStr(c, "tool"), shared.GetStr(c, "moe"), dbcore.NullInt(c["parameters"]), dbcore.NullInt(c["active_parameters"]), dbcore.NullFloat(c["disk_size_gb"]),
		shared.GetStr(c, "measurement"), shared.GetStr(c, "pricing_note"), dbcore.NowStamp(), name)
	return err
}

// EnrichCapability applies collection-derived capability data to an existing
// model: sets model_type (unless already specialized), unions modalities, and
// records the collection slug.
func EnrichCapability(name, modelType string, addIn, addOut []string, collSlug string) error {
	var exIn, exOut, exColl string
	if err := dbcore.DB.QueryRow("SELECT input_modalities, output_modalities, collections FROM models WHERE name=?", name).Scan(&exIn, &exOut, &exColl); err != nil {
		return err
	}
	inMods := dbcore.MergeArr(dbcore.ParseArr(exIn), addIn)
	outMods := dbcore.MergeArr(dbcore.ParseArr(exOut), addOut)
	colls := dbcore.MergeArr(dbcore.ParseArr(exColl), []string{collSlug})
	_, err := dbcore.DB.Exec(`UPDATE models SET
		model_type=CASE WHEN model_type IN ('','chat') THEN ? ELSE model_type END,
		input_modalities=?, output_modalities=?, collections=?, updated_at=? WHERE name=?`,
		modelType, jsonArrStr(inMods), jsonArrStr(outMods), jsonArrStr(colls), dbcore.NowStamp(), name)
	return err
}

// SaveCurated applies a partial curated update from the UI, preserving any
// curated fields the payload does not include.
func SaveCurated(name string, u map[string]interface{}) error {
	if name == "" {
		return nil
	}
	cur := map[string]interface{}{}
	var notes, tool, moe, measurement, pricingNote string
	var favorite int64
	var params, aparams sql.NullInt64
	var diskSize sql.NullFloat64
	err := dbcore.DB.QueryRow(`SELECT notes, favorite, tool, moe, parameters, active_parameters, disk_size_gb, measurement, pricing_note
		FROM models WHERE name=?`, name).Scan(&notes, &favorite, &tool, &moe, &params, &aparams, &diskSize, &measurement, &pricingNote)
	if err == nil {
		cur["notes"], cur["favorite"], cur["tool"], cur["moe"] = notes, favorite, tool, moe
		cur["measurement"], cur["pricing_note"] = measurement, pricingNote
		if params.Valid {
			cur["parameters"] = params.Int64
		}
		if aparams.Valid {
			cur["active_parameters"] = aparams.Int64
		}
		if diskSize.Valid {
			cur["disk_size_gb"] = diskSize.Float64
		}
	} else if err != sql.ErrNoRows {
		return err
	}
	for _, k := range []string{"notes", "tool", "moe", "favorite", "parameters", "active_parameters", "disk_size_gb", "measurement", "pricing_note"} {
		if v, ok := u[k]; ok {
			cur[k] = v
		}
	}
	return UpdateCurated(name, cur)
}

func FindNameByID(id string) (string, bool) {
	if id == "" {
		return "", false
	}
	var name string
	err := dbcore.DB.QueryRow("SELECT name FROM models WHERE id=? LIMIT 1", id).Scan(&name)
	if err != nil {
		return "", false
	}
	return name, true
}

// ExportData writes the objective catalog snapshot to the data dir's
// curated.json. The database (modelsdb.db) is the single source of truth for ALL
// model data, objective and personal; curated.json is a one-way EXPORT of just
// the OBJECTIVE subset. Personal data (notes/favorite/custom columns/etc.) lives
// only in the DB and is deliberately never written to any file, so it is never
// published. Back up personal data by copying modelsdb.db.
func ExportData() error {
	return ExportCuratedJSON(paths.CuratedJsonFile)
}

// ExportCuratedJSON writes the objective catalog snapshot: every model's
// objective curated fields plus enough identity/metadata to rebuild non-API
// models. Personal fields (notes/favorite) and unlisted models are deliberately
// excluded so the file holds only shareable, objective data - they stay in the
// database and are never exported. User-defined custom columns are excluded even
// more strongly: they live in their own tables (see internal/store/custom_columns.go)
// that this function never queries at all, so there is nothing here to exclude.
func ExportCuratedJSON(path string) error {
	b, err := CuratedBytes()
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// CuratedBytes builds the curated.json document bytes from the current DB state
// (exactly what ExportCuratedJSON writes). It is split out so the startup
// reconcile can compare the on-disk file against the DB WITHOUT writing, and so
// detect a file the user restored out-of-band (see ReconcileDurable).
func CuratedBytes() ([]byte, error) {
	rows, err := dbcore.GetAllModels()
	if err != nil {
		return nil, err
	}
	out := make([]map[string]interface{}, 0, len(rows))
	for _, r := range rows {
		// Unlisted models stay in the DB but are withheld from the public export,
		// so a maintainer's private/experimental entries never reach the repo.
		if shared.GetNum(r, "unlisted") == 1 {
			continue
		}
		rec := map[string]interface{}{
			"name":              shared.GetStr(r, "name"),
			"source":            shared.GetStr(r, "source"),
			"model_type":        shared.GetStr(r, "model_type"),
			"tool":              shared.GetStr(r, "tool"),
			"moe":               shared.GetStr(r, "moe"),
			"parameters":        r["parameters"],
			"active_parameters": r["active_parameters"],
			"measurement":       shared.GetStr(r, "measurement"),
			"pricing_note":      shared.GetStr(r, "pricing_note"),
			"collections":       dbcore.ParseArr(shared.GetStr(r, "collections")),
			"zdr":               shared.GetNum(r, "zdr") == 1,
		}
		// disk_size_gb is emitted only when set (most models never carry it), so
		// the catalog stays free of null-valued entries for it.
		if v := r["disk_size_gb"]; v != nil {
			rec["disk_size_gb"] = v
		}
		// For non-OpenRouter models, persist identity/metadata so a fresh clone
		// (DB rebuilt from this file) keeps models the API will not return.
		if shared.GetStr(r, "source") != "openrouter" {
			rec["id"] = shared.GetStr(r, "id")
			rec["slug"] = shared.GetStr(r, "slug")
			rec["hf_slug"] = shared.GetStr(r, "hf_slug")
			rec["author"] = shared.GetStr(r, "author")
			rec["description"] = shared.GetStr(r, "description")
			rec["context_length"] = r["context_length"]
			rec["input_modalities"] = dbcore.ParseArr(shared.GetStr(r, "input_modalities"))
			rec["output_modalities"] = dbcore.ParseArr(shared.GetStr(r, "output_modalities"))
			rec["supports_reasoning"] = shared.GetNum(r, "supports_reasoning") == 1
			rec["price_prompt"] = shared.GetStr(r, "price_prompt")
			rec["price_completion"] = shared.GetStr(r, "price_completion")
			rec["price_image"] = shared.GetStr(r, "price_image")
			rec["price_display"] = shared.GetStr(r, "price_display")
			rec["pricing_url"] = shared.GetStr(r, "pricing_url")
		}
		out = append(out, rec)
	}
	sort.Slice(out, func(i, j int) bool {
		return strings.ToLower(shared.GetStr(out[i], "name")) < strings.ToLower(shared.GetStr(out[j], "name"))
	})
	// Wrap the records with a schema header so importers can gate on the minimum
	// app version required to read this catalog (see curatedDoc / curatedCompatible).
	doc := CuratedDoc{
		SchemaVersion: shared.CuratedSchemaVersion,
		MinAppVersion: shared.CuratedMinAppVersion,
		Models:        out,
	}
	return json.MarshalIndent(doc, "", "  ")
}
