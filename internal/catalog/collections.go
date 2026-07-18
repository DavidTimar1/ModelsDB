// Package catalog - import OpenRouter "collections" to enrich capabilities and add
// models the public API does not return (video, embeddings, rerank, TTS, STT,
// some image-gen). Runs on demand via the `collections` subcommand; never on
// startup. Capability data lands in the DB and is preserved across refreshes.
package catalog

import (
	"fmt"
	"io"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"net/http"
	"regexp"
	"strings"
)

// collectionDef maps a collection to the capability it implies.
type collectionDef struct {
	slug      string
	modelType string   // sets model_type when not already specialized
	addIn     []string // input modalities to union in
	addOut    []string // output modalities to union in
	setTool   bool     // mark tool-calling support
}

// collectionDefs lists ONLY the collections that surface model TYPES the public
// API (/api/v1/models) does not return: image-gen, video, speech-to-text,
// text-to-speech, embeddings, rerank. A measurement against the live catalog
// (2026-06-25) confirmed these six supply 100% of the models the API omits
// (48 of 48 net-new), while every other OpenRouter collection added zero unique
// models and was dropped:
//   - vision / audio / tool-calling only re-asserted modality + tool flags the
//     raw API already provides (architecture.{input,output}_modalities and
//     supported_parameters, applied in NormalizeModel/UpsertAPIModel);
//   - free / distillable / programming / roleplay / openclaw are pure curation
//     with no capability mapping (they only tagged the unused `collections`
//     column).
//
// Dropping them halves the scrape and removes the most fragile, duplicative pages.
var collectionDefs = []collectionDef{
	{"image-models", "image", nil, []string{"image"}, false},
	{"video-models", "video", nil, []string{"video"}, false},
	{"speech-to-text-models", "stt", []string{"audio"}, []string{"text"}, false},
	{"text-to-speech-models", "tts", []string{"text"}, []string{"audio"}, false},
	{"embedding-models", "embedding", []string{"text"}, []string{"embedding"}, false},
	{"rerank-models", "rerank", []string{"text"}, []string{"ranking"}, false},
}

var hrefModelRe = regexp.MustCompile(`href="/([a-z0-9._-]+)/([a-z0-9._:-]+)"`)

var denyFirstSeg = map[string]bool{
	"docs": true, "api": true, "legal": true, "policies": true, "terms": true,
	"static": true, "dist": true, "en": true, "images": true, "company": true,
	"u": true, "blog": true, "enterprise": true, "careers": true, "pricing": true,
	"settings": true, "account": true, "rankings": true, "models": true,
	"collections": true, "chat": true, "login": true, "signup": true,
	"privacy": true, "about": true, "multimodal": true, "gtag": true, "cdn": true,
	"_next": true, "favicon": true, "sitemap": true, "assets": true, "help": true,
	"status": true, "changelog": true, "use": true,
}

func httpGetBrowser(url string) (string, error) {
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("status %s", resp.Status)
	}
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

// scrapeCollection returns the model ids (author/model) listed on a collection.
func scrapeCollection(slug string) ([]string, error) {
	html, err := httpGetBrowser("https://openrouter.ai/collections/" + slug)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	var ids []string
	for _, m := range hrefModelRe.FindAllStringSubmatch(html, -1) {
		if denyFirstSeg[m[1]] {
			continue
		}
		id := m[1] + "/" + m[2]
		if !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func titleize(s string) string {
	s = strings.NewReplacer("-", " ", "_", " ").Replace(s)
	words := strings.Fields(s)
	for i, w := range words {
		if w != "" {
			words[i] = strings.ToUpper(w[:1]) + w[1:]
		}
	}
	return strings.Join(words, " ")
}

// deriveName turns an "author/model" id into a display name like
// "Black Forest Labs: Flux.2 Pro" for models the API does not provide a name for.
func deriveName(id string) string {
	parts := strings.SplitN(id, "/", 2)
	if len(parts) == 2 {
		return titleize(parts[0]) + ": " + titleize(parts[1])
	}
	return titleize(id)
}

// findNameByIDOrSlug locates an existing model row by API id or slug.
func findNameByIDOrSlug(id string) (string, bool) {
	if name, ok := store.FindNameByID(id); ok {
		return name, true
	}
	var name string
	err := dbcore.DB.QueryRow("SELECT name FROM models WHERE slug=? OR slug LIKE ? LIMIT 1", id, id+"-%").Scan(&name)
	if err != nil {
		return "", false
	}
	return name, true
}

// RunCollections scrapes every collection and applies capability data, adding
// models the API omits as new rows.
func RunCollections() error {
	enriched, added := 0, 0
	for _, cd := range collectionDefs {
		ids, err := scrapeCollection(cd.slug)
		if err != nil {
			log.Printf("Warning: collection %q scrape failed: %v", cd.slug, err)
			continue
		}
		newInThis := 0
		for _, rawID := range ids {
			// Strip ":variant" suffixes (e.g. ":free") so variants map to the base model.
			id := strings.SplitN(rawID, ":", 2)[0]
			name, ok := findNameByIDOrSlug(id)
			if !ok {
				name = deriveName(id)
				rec := map[string]interface{}{
					"name": name, "id": id, "slug": id,
					"input_modalities":  cd.addIn,
					"output_modalities": cd.addOut,
				}
				if err := store.ImportFullRecord(rec, "collection", cd.modelType); err != nil {
					log.Printf("Warning: add %q failed: %v", name, err)
					continue
				}
				added++
				newInThis++
			} else {
				enriched++
			}
			if err := store.EnrichCapability(name, cd.modelType, cd.addIn, cd.addOut, cd.slug); err != nil {
				log.Printf("Warning: enrich %q failed: %v", name, err)
			}
			if cd.setTool {
				dbcore.DB.Exec("UPDATE models SET tool='yes' WHERE name=?", name)
			}
		}
		log.Printf("  %-22s %3d models (%d new to DB)", cd.slug, len(ids), newInThis)
	}
	log.Printf("Collections import: %d models enriched, %d added", enriched, added)

	if err := store.ExportData(); err != nil {
		log.Printf("Warning: export data failed: %v", err)
	}
	return nil
}
