// Package app - read-only query API for other local apps/agents.
//
// GET /api/models?<filters>  -> JSON array of matching models
// GET /api/models  (no query) -> the markdown guide below
// GET /api                    -> the markdown guide below
package app

import (
	"fmt"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/paths"
	"modelsdb/internal/shared"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

func handleAPIGuide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	// The guide embeds the default port in its examples; reflect the active one.
	w.Write([]byte(strings.ReplaceAll(apiGuide, "127.0.0.1:8122", fmt.Sprintf("127.0.0.1:%d", paths.Port))))
}

func toFloat(v interface{}) (float64, bool) {
	switch x := v.(type) {
	case float64:
		return x, true
	case int:
		return float64(x), true
	case int64:
		return float64(x), true
	case string:
		f, err := strconv.ParseFloat(strings.TrimSpace(x), 64)
		return f, err == nil
	}
	return 0, false
}

// perMillion converts a raw per-token price string to USD per 1M tokens.
func perMillion(s string) (float64, bool) {
	f, ok := toFloat(s)
	if !ok {
		return 0, false
	}
	return f * 1_000_000, true
}

func parseFloatDefault(s string, def float64) float64 {
	if s == "" {
		return def
	}
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return f
	}
	return def
}

// filterAndProject applies the query filters (AND-combined, case-insensitive)
// and optional `fields` projection to the processed model rows.
func filterAndProject(rows []map[string]interface{}, q url.Values) []map[string]interface{} {
	name := strings.ToLower(q.Get("name"))
	typ := strings.ToLower(q.Get("type"))
	input := strings.ToLower(q.Get("input"))
	output := strings.ToLower(q.Get("output"))
	wantTool := q.Get("tool") == "yes"
	wantReasoning := q.Get("reasoning") == "yes"
	zdrFilter := strings.ToLower(strings.TrimSpace(q.Get("zdr"))) // "true" | "false" | "" (all)
	minCtx := parseFloatDefault(q.Get("min_context"), -1)
	maxIn := parseFloatDefault(q.Get("max_in"), -1)
	maxOut := parseFloatDefault(q.Get("max_out"), -1)
	var fields []string
	if f := q.Get("fields"); f != "" {
		fields = dbcore.SplitList(f)
	}

	out := make([]map[string]interface{}, 0, len(rows))
	for _, m := range rows {
		if name != "" && !strings.Contains(strings.ToLower(shared.GetStr(m, "name")), name) {
			continue
		}
		if typ != "" && strings.ToLower(shared.GetStr(m, "model_type")) != typ {
			continue
		}
		if input != "" && !strings.Contains(strings.ToLower(shared.GetStr(m, "input_modalities")), input) {
			continue
		}
		if output != "" && !strings.Contains(strings.ToLower(shared.GetStr(m, "output_modalities")), output) {
			continue
		}
		if wantTool && shared.GetStr(m, "tool") != "yes" {
			continue
		}
		if wantReasoning && !shared.GetBool(m, "supports_reasoning") {
			continue
		}
		if zdrFilter == "true" && !shared.GetBool(m, "zdr") {
			continue
		}
		if zdrFilter == "false" && shared.GetBool(m, "zdr") {
			continue
		}
		if minCtx >= 0 {
			if cl, ok := toFloat(m["context_length"]); !ok || cl < minCtx {
				continue
			}
		}
		if maxIn >= 0 {
			if p, ok := perMillion(shared.GetStr(m, "prompt")); !ok || p > maxIn {
				continue
			}
		}
		if maxOut >= 0 {
			if c, ok := perMillion(shared.GetStr(m, "completion")); !ok || c > maxOut {
				continue
			}
		}
		if len(fields) > 0 {
			proj := make(map[string]interface{}, len(fields))
			for _, f := range fields {
				if v, ok := m[f]; ok {
					proj[f] = v
				}
			}
			out = append(out, proj)
		} else {
			out = append(out, m)
		}
	}
	return out
}

const apiGuide = `# ModelsDB API

Local, read-only catalog of AI models (OpenRouter + HuggingFace + curated data).
Base URL: http://127.0.0.1:8122   (default loopback bind; no auth - a non-loopback "host" opens full read/write to that network)

## Endpoints

- GET /api/models?<filters>   -> JSON array of models matching the filters
- GET /api/models             (no query string) -> this guide
- GET /api                    -> this guide
- GET /api/health             -> {"status":"ok","port":...}
- POST /api/update            -> start a full refresh (catalog,collections,pricing) in the background; 202 {"started":true}, or 409 {"started":false,"running":true} if one is already running
- GET /api/update/status      -> {"running":bool,"phase":"idle|catalog|collections|pricing","started_at":...,"finished_at":...,"errors":[...]}

To get the FULL list, pass any parameter, e.g. GET /api/models?all=1

## Query parameters (all optional, AND-combined, case-insensitive)

| Param        | Meaning                                  | Example              |
|--------------|------------------------------------------|----------------------|
| name         | substring match on model name            | name=gpt             |
| type         | model_type exact match                   | type=image           |
| input        | input modality contains                  | input=image          |
| output       | output modality contains                 | output=text          |
| tool         | tool=yes -> only tool/function-calling   | tool=yes             |
| reasoning    | reasoning=yes -> only reasoning models   | reasoning=yes        |
| zdr          | zdr=true/false -> Zero Data Retention    | zdr=true             |
| min_context  | minimum context length (max tokens)      | min_context=200000   |
| max_in       | max input price, USD per 1M tokens       | max_in=1             |
| max_out      | max output price, USD per 1M tokens      | max_out=5            |
| fields       | comma-list; return only these fields     | fields=name,prompt   |
| all          | ignored; use it to fetch everything      | all=1                |

model_type values: chat, image, video, embedding, rerank, tts, stt

## Response fields (per model)

- name              : display name (stable identifier / primary key)
- id, slug          : OpenRouter ids; hf_slug is the HuggingFace id
- model_type        : chat | image | video | embedding | rerank | tts | stt
- source            : openrouter | huggingface | collection
- description       : model description
- context_length    : max tokens (may be null for non-LLM models)
- input_modalities  : comma-joined, e.g. "text, image"
- output_modalities : comma-joined, e.g. "text"
- supports_reasoning: boolean
- zdr               : boolean - Zero Data Retention. true if any serving
                      provider keeps no prompts (mirrors OpenRouter's "Zero Data
                      Retention" filter); non-OpenRouter models are always true.
- tool              : "yes" | "no"  (tool / function calling)
- prompt            : INPUT price, RAW USD PER TOKEN (string)
- completion        : OUTPUT price, RAW USD PER TOKEN (string)
- price_display     : human pricing summary (see the pricing note below)
- notes             : curated notes (often the pricing-unit caveat)
- rating, favorite, speed, moe, parameters, active_parameters, disk_size_gb : curated fields
- disk_size_gb       : native-precision on-disk size in GB (safetensors/.bin
                      weights on the canonical HuggingFace repo; null when unset)

## IMPORTANT - pricing units

prompt and completion are PER-TOKEN prices and are set ONLY for token-priced
models. Multiply by 1,000,000 to get USD per 1M tokens.

For image / video / audio / character-priced models, prompt and completion are
EMPTY - those models are NOT priced per token. Read price_display for the real
price and unit, for example:
  - "Output Image: $0.07/megapixel"
  - "Video: $0.10/second"
  - "Characters: $0.62/M characters"
  - "Audio Minutes: $0.006/minute"
Never treat prompt/completion as the price when price_display shows a non-token
unit. The max_in / max_out filters only match token-priced models.

## Examples

  curl 'http://127.0.0.1:8122/api/models?type=image&output=image'
  curl 'http://127.0.0.1:8122/api/models?name=claude&fields=name,prompt,completion,context_length'
  curl 'http://127.0.0.1:8122/api/models?tool=yes&max_in=1&min_context=200000'
  curl 'http://127.0.0.1:8122/api/models?all=1'
`
