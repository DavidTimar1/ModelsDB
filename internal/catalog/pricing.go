// Package catalog - scrape per-model pricing from OpenRouter model pages for models
// the public API does not price (image/video/embedding/rerank/TTS/STT, etc.).
// The page embeds the pricing as JSON (no fragile xpath needed); the URL is just
// openrouter.ai/<id>. Runs on demand via the `pricing` subcommand.
package catalog

import (
	"encoding/json"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/store"
	"strconv"
	"strings"
	"time"
)

type displayPrice struct {
	SkuLabel          string  `json:"sku_label"`
	Price             string  `json:"price"`
	UnitLabel         string  `json:"unitLabel"`
	DisplayMultiplier float64 `json:"displayMultiplier"`
}

type pagePricing struct {
	Prompt         string         `json:"prompt"`
	Completion     string         `json:"completion"`
	ImageOutput    string         `json:"image_output"`
	DisplayPricing []displayPrice `json:"display_pricing"`
}

// extractBalancedObject returns the JSON object starting at the first '{' after
// the first occurrence of key, honoring strings/escapes.
func extractBalancedObject(s, key string) (string, bool) {
	i := strings.Index(s, key)
	if i < 0 {
		return "", false
	}
	j := strings.IndexByte(s[i:], '{')
	if j < 0 {
		return "", false
	}
	start := i + j
	depth, inStr, esc := 0, false, false
	for k := start; k < len(s); k++ {
		c := s[k]
		if inStr {
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch c {
		case '"':
			inStr = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : k+1], true
			}
		}
	}
	return "", false
}

func fmtNum(s string, mult float64) string {
	f, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return s
	}
	if mult == 0 {
		mult = 1
	}
	return strconv.FormatFloat(f*mult, 'g', 6, 64)
}

// buildDisplay renders the human pricing summary (what the page shows as
// "Weighted Avg ... Price"), e.g. "Output Image: $0.014/megapixel".
func buildDisplay(pp pagePricing) string {
	var parts []string
	for _, d := range pp.DisplayPricing {
		if d.Price == "" {
			continue
		}
		parts = append(parts, fmt.Sprintf("%s: $%s%s", d.SkuLabel, fmtNum(d.Price, d.DisplayMultiplier), d.UnitLabel))
	}
	return strings.Join(parts, "; ")
}

// deriveMeasurement returns the pricing unit for the In/Out columns: "per 1M
// tokens" for token-priced models, otherwise the distinct unit label(s) the page
// uses (e.g. "per megapixel", "per second"). Empty when the page prices nothing.
func deriveMeasurement(pp pagePricing) string {
	if tokenPriced(pp) {
		return "per 1M tokens"
	}
	seen := map[string]bool{}
	var units []string
	for _, d := range pp.DisplayPricing {
		if d.Price == "" {
			continue
		}
		u := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(d.UnitLabel), "/"))
		if u == "" || seen[u] {
			continue
		}
		seen[u] = true
		units = append(units, "per "+u)
	}
	return strings.Join(units, "; ")
}

// scrapeModelPricing fetches a model page and returns its pricing.
func scrapeModelPricing(id string) (pagePricing, error) {
	html, err := httpGetBrowser("https://openrouter.ai/" + id)
	if err != nil {
		return pagePricing{}, err
	}
	un := strings.ReplaceAll(html, `\"`, `"`)
	obj, ok := extractBalancedObject(un, `"pricing":`)
	if !ok {
		return pagePricing{}, fmt.Errorf("no pricing block found")
	}
	var pp pagePricing
	if err := json.Unmarshal([]byte(obj), &pp); err != nil {
		return pagePricing{}, fmt.Errorf("parse pricing: %w", err)
	}
	return pp, nil
}

// tokenPriced reports whether a model is genuinely priced per token (so the
// per-1M-token In $/Out $ columns are meaningful). It is true only when every
// priced display entry uses a token unit; image/video/audio/character pricing
// returns false so those columns are left empty instead of showing garbage
// (e.g. a per-minute price rendered as $360000/M tokens).
func tokenPriced(pp pagePricing) bool {
	any := false
	for _, d := range pp.DisplayPricing {
		if d.Price == "" {
			continue
		}
		any = true
		if !strings.Contains(strings.ToLower(d.UnitLabel), "token") {
			return false
		}
	}
	return any
}

// RunPricing re-scrapes pricing for every collection-added OpenRouter model
// (those the public API does not price). The In $/Out $ (per-1M-token) columns
// are set ONLY for token-priced models; for image/video/audio/etc. they are
// cleared and the real price lives in price_display (and the model's notes).
func RunPricing() error {
	rows, err := dbcore.DB.Query(`SELECT name, id FROM models
		WHERE source='collection' AND id != '' ORDER BY name`)
	if err != nil {
		return err
	}
	type job struct{ name, id string }
	var jobs []job
	for rows.Next() {
		var j job
		if err := rows.Scan(&j.name, &j.id); err == nil {
			jobs = append(jobs, j)
		}
	}
	rows.Close()

	log.Printf("=== Scraping pricing for %d collection models ===", len(jobs))
	ok, fail := 0, 0
	for _, j := range jobs {
		pp, err := scrapeModelPricing(j.id)
		if err != nil {
			log.Printf("  [x] %-45s %v", j.id, err)
			fail++
			continue
		}
		display := buildDisplay(pp)
		prompt, completion := "", "" // per-token columns: only for token-priced models
		if tokenPriced(pp) {
			prompt, completion = pp.Prompt, pp.Completion
		}
		if err := store.SetPricing(j.name, prompt, completion, pp.ImageOutput, display, "https://openrouter.ai/"+j.id, deriveMeasurement(pp)); err != nil {
			log.Printf("  [x] %-45s store: %v", j.id, err)
			fail++
			continue
		}
		log.Printf("  [ok] %-45s tok=%v %s", j.id, tokenPriced(pp), display)
		ok++
		time.Sleep(150 * time.Millisecond) // be polite
	}
	log.Printf("Pricing scrape: %d updated, %d failed", ok, fail)

	if err := store.ExportData(); err != nil {
		log.Printf("Warning: export data failed: %v", err)
	}
	return nil
}
