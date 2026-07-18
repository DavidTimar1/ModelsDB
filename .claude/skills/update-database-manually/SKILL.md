---
name: update-database-manually
description: 'Manually fill in the model data the OpenRouter API does not provide for ModelsDB (the data/modelsdb.db SQLite catalog). Use whenever the user wants to "update the database" / "update the models" - which means BOTH the programmatic refresh AND a manual backfill pass - and specifically to: fill missing parameters/active_parameters/moe/context_length/modalities for recent models (looked up on HuggingFace), price collection-added models the API omits (image/video/embedding/rerank/tts/stt), add HuggingFace-direct models with no OpenRouter listing, fix model capabilities, add a model, or record a pricing caveat. Invoke this even when the user does not name the skill.'
---

# update-database-manually

Updates are done by you (the AI), not by app code - but ONLY for the data the OpenRouter API does not give us.

## What "update the database" means (do BOTH phases)

When the user says "update the database" / "update the models", it is a TWO-phase job. Phase 2 is not optional - skipping it is the bug this skill exists to prevent.

**Phase 1 - programmatic refresh (factual, automatic).** Trigger the built-in refresh: the header **Update models** button (`POST /api/update`), or the CLI sequence `update` -> `collections` -> `pricing`. This pulls the OpenRouter catalog, re-derives `zdr`, scrapes collections, and prices API-priced models. It does NOT fill parameter counts or anything the API never returns.

**Phase 2 - manual backfill pass (the part that keeps getting missed).** The OpenRouter API NEVER returns parameter counts, so every model relies on you to fill `parameters` (and `active_parameters`/`moe`) by hand. Do a quick pass over RECENT models (created in roughly the **last 3 months**) that are missing basic data and look it up online, **HuggingFace first**:

```bash
# Recent models missing basic param data (the Phase-2 worklist). Any model with
# an hf_slug counts - source 'openrouter' OR 'huggingface'. Adjust the date.
sqlite3 data/modelsdb.db "SELECT name, source, hf_slug, created_at FROM models WHERE hf_slug!='' AND (parameters IS NULL OR parameters='' OR parameters=0) AND created_at >= date('now','-3 months') ORDER BY created_at DESC;"
```

"Basic data" = `parameters` (total, in billions), `active_parameters` (for MoE), `moe`, and - if also missing - `context_length`, `input_modalities`, `output_modalities`. For each row: open `https://huggingface.co/<hf_slug>` (and its `config.json`), READ it, and write the values (see the field-map + SQL below). `parameters` is a CURATED column, so it is preserved across refreshes and exported to `curated.json` even for `source='openrouter'` models - it is safe and correct to set it on them. Many slugs encode the answer (`...-550B-A55B` -> total 550, active 55, MoE; `...-35B-A3B` -> 35 / 3 / MoE), but CONFIRM on the page rather than guessing when the slug is not explicit.

After the pass, persist with `update` (or the UI save) so the DB is written. To publish the changes, run `modelsdb export internal/seedcatalog/catalog.json` and commit that file (the app never runs git).

## Scope - read this first

The app's startup fetch (`/api/v1/models`) already provides full pricing and metadata for every model OpenRouter exposes through the API, and upserts it automatically. You must NOT manually scrape or re-price those models.

This skill is ONLY for the two kinds of model the API does not cover:

1. **Collection-added OpenRouter models** (`source='collection'`) the API does not price: image, video, embedding, rerank, TTS, STT, some image-gen. Scrape their OpenRouter page `https://openrouter.ai/<id>` and read the pricing, understanding the (non-token) units.
2. **HuggingFace-direct models** (`source='huggingface'`) that have NO OpenRouter listing/link. Scrape their HuggingFace page `https://huggingface.co/<hf_slug>` for details (parameters, context length, modalities, etc.); they have no OpenRouter pricing at all.

For both, manual judgement is the point: automated scraping cannot tell per-image from per-second from per-1M-tokens, and HuggingFace pages carry no standard pricing.

## The database

- `data/modelsdb.db` - SQLite, the source of truth. Edit with the `sqlite3` CLI: `sqlite3 data/modelsdb.db "<SQL>"`. That path is the **dev** (`go run`) database; a shipped/installed binary keeps its DB in the OS data dir instead (e.g. `~/.local/share/modelsdb/modelsdb.db` on Linux) - point `sqlite3` at whichever DB the running instance uses.
- The running server reads the DB on every request, so a direct SQL edit is visible on the next page refresh - no restart needed.
- `curated.json` - the human-readable OBJECTIVE export, written into the DB's data dir (git-ignored). It is re-exported whenever the app saves or runs `update`. Personal fields (`notes, speed, rating, favorite, ocr_quality`) and any model flagged `unlisted` are NEVER exported - they live only in `modelsdb.db` (the source of truth). Back up personal data by copying the DB.
- **Publishing the public catalog:** the shipped catalog is embedded in the binary at `internal/seedcatalog/catalog.json`. To publish, run `modelsdb export internal/seedcatalog/catalog.json` (writes the objective catalog, excluding personal + `unlisted`), then commit that file and build the release. `build.sh` auto-refreshes it from the live DB when one is present.

## Table `models` (primary key = `name`) - key columns

- Identity: `name`, `id` (e.g. `openai/gpt-5-image`), `slug`, `hf_slug`, `author`, `source` (`openrouter` | `huggingface` | `collection` | `manual`), `model_type` (`chat`|`image`|`video`|`embedding`|`rerank`|`tts`|`stt`).
- Capabilities: `input_modalities`, `output_modalities` (JSON-array text, e.g. `["text","image"]`), `supports_reasoning`, `context_length`.
- Pricing: `price_prompt`, `price_completion`, `price_image`, `price_audio`, `price_internal_reasoning` (raw per-unit numbers as strings), `price_display` (human summary), `pricing_url`.
- Curated (always preserved + exported): `notes`, `speed`, `rating`, `favorite`, `ocr_quality`, `tool`, `moe`, `parameters`, `active_parameters`, `disk_size_gb`.
- `disk_size_gb` (REAL, objective/curated/exported): total size in GB of the model's NATIVE-precision weight files (safetensors / pytorch `.bin`) on the **main** revision of the **canonical** HuggingFace repo - EXCLUDING quantized/GGUF mirrors and non-weight files. Manually researched from the repo's "Files" tab; never auto-filled by the OpenRouter refresh. Stored as a decimal number of GB (e.g. `16.1`), shown read-only (rounded up to a whole number) in the UI "Size (GB)" column.
- `zdr` (Zero Data Retention, INTEGER 0/1): auto-derived on every catalog refresh for OpenRouter models (any serving provider with `retainsPrompts == false` -> 1); always 1 for non-OpenRouter models. Do NOT hand-edit it for OpenRouter models - the refresh recomputes it.

## Find what needs updating

```bash
# OpenRouter models with no pricing yet (the usual targets):
sqlite3 data/modelsdb.db "SELECT name,id,model_type FROM models WHERE id!='' AND price_prompt='' AND price_completion='' AND price_image='';"
# Everything the public API does not return (added from collections):
sqlite3 data/modelsdb.db "SELECT name,id,model_type FROM models WHERE source='collection';"
# Models with a HuggingFace link but no parameter count (Phase-2 backfill; drop
# the date clause to see ALL of them, keep it for just the recent worklist):
sqlite3 data/modelsdb.db "SELECT name,source,hf_slug,created_at FROM models WHERE hf_slug!='' AND (parameters IS NULL OR parameters='' OR parameters=0) AND created_at >= date('now','-3 months') ORDER BY created_at DESC;"
# Models with a HuggingFace link but no native on-disk size (disk_size_gb):
sqlite3 data/modelsdb.db "SELECT name,source,hf_slug FROM models WHERE hf_slug!='' AND (disk_size_gb IS NULL OR disk_size_gb=0) ORDER BY created_at DESC;"
```

## How to update a model (the accurate, manual way)

> Read and understand each page YOURSELF. Do NOT write a regex/parser/script to
> bulk-extract pricing - the formats and units vary per model and need your
> judgement. Real example: one "transcribe" (STT) model is priced per *token*
> while another is per *minute of audio*; only reading the page tells you which.
> Fetch the page, look at it, decide the right values and units by hand.

1. Take the model's `id` and fetch its page: `https://openrouter.ai/<id>` (HuggingFace models: the huggingface.co page). Fetch with your internal engine first (curl / WebFetch); fall back to the `brightdata` CLI/skill only if that fails. On OpenRouter the pricing is shown in the page JSON under `"pricing"` / `"display_pricing"` (`sku_label`, `price`, `unitLabel`, `displayMultiplier`) - read it, do not auto-parse it across models.
2. UNDERSTAND THE UNITS - the point of doing this by hand:
   - chat/text: per 1M tokens (prompt = input, completion = output)
   - image: per image OR per megapixel
   - video: per second
   - tts: per 1M characters; stt/transcribe: per minute / hour / second of audio
   - embedding: per 1M tokens (input only) The human number = `price * displayMultiplier`.
3. Write the real values into the right columns. **The In/Out columns (`price_prompt`/`price_completion`) carry the price for EVERY model, whatever the unit** - the `measurement` column states the unit, so the price is a real number in a column and is NEVER duplicated as prose. Follow these rules:

   - **Pick the column by what is MEASURED (the data flow), not by the unit:**
     - Output media (video output, image output) -> `price_completion` (Out).
     - What the model CONSUMES (STT audio per hour/minute/second, TTS text per character, rerank per search, image input) -> `price_prompt` (In).
     - chat/text/embedding: input -> In, output -> Out, as usual.
     A model may fill BOTH columns when it has an input price and an output price in the same unit (e.g. an image model with `Image Input: $0.01/image` and `Image Output: $0.05/image` -> In `0.01`, Out `0.05`).

   - **Set `measurement` to the unit** (`per 1M tokens`, `per 1M characters`, `per second`, `per minute`, `per hour`, `per image`, `per megapixel`, `per search`, ...). It is static, derived at this step and never edited in the app. The UI shows it read-only as a short abbreviation under each In $/Out $ price (`per 1M tokens` -> `tkn`, `per 1M characters` -> `chr`, `per image` -> `img`, `per second` -> `sec`, `per minute` -> `min`, `per hour` -> `hr`, `per megapixel` -> `mpx`, `per search` -> `srch`), with the full text on hover, and filters it from the header **Unit** dropdown. A unit that is not in that list is shown verbatim, so a new one still displays - but add it to `UNIT_ABBREV` in `internal/app/static/app.js` so it reads like the rest. Do NOT restate the unit anywhere else.

   - **How you SCALE the stored number depends on the unit:**
     - "per 1M ..." units (`per 1M tokens`, `per 1M characters`): store the price PER SINGLE UNIT (the per-1M figure / 1,000,000); the UI multiplies by 1M for display. $0.13/1M tokens -> `0.00000013`; $7/1M characters -> `0.000007`.
     - every per-single-unit measurement (`per second/minute/hour/image/megapixel/search`): store the LITERAL per-unit dollar amount; the UI shows it as-is. $0.112/second -> `0.112`; $0.04/image -> `0.04`.

   - **`pricing_note` holds ONLY the leftover SKUs that do not fit In/Out.** When you convert a price segment into In or Out, REMOVE that exact segment from `pricing_note`. A single-segment note becomes empty. A multi-segment note keeps only the variants you could NOT capture - a different unit, or a third/fourth variant of the same unit. `pricing_note` is curated/objective and public; NEVER put pricing in `notes` (personal/private). Examples:
     - `Video Output: $0.112/second` -> Out `0.112`, `measurement='per second'`, `pricing_note=''`.
     - `Video (with audio): $0.1512/second; Video (no audio): $0.1512/second; Video Tokens (with audio): $7/M tokens; Video Tokens (no audio): $7/M tokens` -> Out `0.1512`, `measurement='per second'`, and `pricing_note='Video (no audio): $0.1512/second; Video Tokens (with audio): $7/M tokens; Video Tokens (no audio): $7/M tokens'` (only the converted `Video (with audio)` segment is removed; the rest stays because two video-second variants cannot both fit one Out column and the token SKUs are a different unit).

```bash
# Per-token embedding model: per-token numbers in In/Out, unit in measurement.
sqlite3 data/modelsdb.db "UPDATE models SET price_prompt='0.00000013', price_completion='0', measurement='per 1M tokens', pricing_url='https://openrouter.ai/openai/text-embedding-3-large' WHERE name='Openai: Text Embedding 3 Large';"

# Image model priced per megapixel: literal price in Out, unit in measurement.
sqlite3 data/modelsdb.db "UPDATE models SET price_completion='0.07', measurement='per megapixel', pricing_url='https://openrouter.ai/black-forest-labs/flux.2-max' WHERE name='Black Forest Labs: Flux.2 Max';"

# Video per second, with a non-fitting variant kept in pricing_note:
sqlite3 data/modelsdb.db "UPDATE models SET price_completion='0.1', measurement='per second', pricing_note='Video (no audio): \$0.08/second', pricing_url='https://openrouter.ai/google/veo-3.1-fast' WHERE name='Google: Veo 3.1 Fast';"
```

4. Fix capabilities when wrong (modalities are JSON arrays):

```bash
sqlite3 data/modelsdb.db "UPDATE models SET input_modalities='[\"text\"]', output_modalities='[\"embedding\"]', model_type='embedding' WHERE name='OpenAI: Text Embedding 3 Small';"
```

5. Persist: after your edits run `go run ./cmd/modelsdb update` (refreshes the catalog and re-exports the data-dir `curated.json`). To publish, run `modelsdb export internal/seedcatalog/catalog.json` and commit that file (the app does not run git). Objective curated fields and the pricing of non-API (`source != 'openrouter'`) models are included in the export and ship in the embedded catalog; personal fields (`notes` etc.) and `unlisted` models are never exported - they live only in `modelsdb.db` (back it up by copying the DB file).
   - Do NOT manually set price columns on `source='openrouter'` models: the API refresh overwrites them. For those, record any objective pricing caveat in `pricing_note` and the unit in `measurement` (both curated/public) - NOT in `notes`, which is your private overlay.

## HuggingFace-direct models (no OpenRouter link)

These exist only on HuggingFace - the OpenRouter API never returns them, so you add and maintain them by hand. This section is self-contained: it covers scraping the page, mapping every field, and the exact insert/update SQL.

> The `vibe-huggingface` MCP plugin is just an HF connector and is optional - you
> do NOT need it. Plain page fetching (below) is enough.

### Step 1 - fetch and READ the model page

Fetch `https://huggingface.co/<hf_slug>` (e.g. `https://huggingface.co/Qwen/Qwen3.6-27B`). Use your internal engine first (WebFetch / curl); fall back to the `brightdata` CLI/skill only if that fails. Read it yourself - do not write a bulk parser. The page's model card, the "Model tree"/config, and `config.json` (at `https://huggingface.co/<hf_slug>/blob/main/config.json`) carry the facts you need.

### Step 2 - map HF facts to columns

| Column                | Where on the HF page                                              |
|-----------------------|-------------------------------------------------------------------|
| `name`                | Display name you choose (PK). Convention: `Author: Model Name`. Append ` HF` when an OpenRouter twin exists, to keep them distinct. |
| `hf_slug`             | The `<author>/<repo>` id from the URL, e.g. `Qwen/Qwen3.6-27B`.   |
| `author`              | The org/user (left of the `/`).                                   |
| `model_type`          | `chat` for text LLMs; else `image`/`video`/`embedding`/`rerank`/`tts`/`stt` per what it does. |
| `description`         | One- or two-line summary from the model card.                     |
| `context_length`      | `max_position_embeddings` (config.json) or the card's stated context window. |
| `input_modalities` / `output_modalities` | JSON arrays, e.g. `["text"]`, or `["text","image"]` for vision. |
| `parameters`          | Total params in BILLIONS, integer (27B -> `27`). Read the card / repo name. |
| `active_parameters`   | For MoE: active params per forward pass in billions (e.g. A3B -> `3`). Else leave NULL. |
| `disk_size_gb`        | Total GB of the NATIVE-precision weight files (safetensors / `.bin`) on the repo's **main** revision. Open the "Files and versions" tab, sum the sizes of the weight shards (`*.safetensors` / `pytorch_model*.bin`) - EXCLUDE quantized/GGUF mirrors and non-weight files. Decimal GB, e.g. `16.1`. Leave NULL if unknown. |
| `moe`                 | `yes` if Mixture-of-Experts (config has `num_experts` / card says MoE), else `no`. |
| `supports_reasoning`  | `1` if it is a reasoning/thinking model, else `0`.                |
| price columns         | Leave empty - HF-direct models have no hosted price. (Optionally note "HF-only, no hosted price".) |

`source` is always `huggingface` for these. Modalities are stored as JSON-array text; everything else as shown.

### Step 3 - insert or update (PK is `name`)

```bash
sqlite3 data/modelsdb.db "INSERT INTO models (name, source, model_type, hf_slug, author, description, context_length, input_modalities, output_modalities, supports_reasoning, parameters, active_parameters, disk_size_gb, moe, notes) VALUES ('Qwen: Qwen3.6-35B-A3B HF','huggingface','chat','Qwen/Qwen3.6-35B-A3B','Qwen','MoE text model, 35B total / 3B active.',262144,'[\"text\"]','[\"text\"]',1,35,3,70.6,'yes','HF-only, no hosted price') ON CONFLICT(name) DO UPDATE SET hf_slug=excluded.hf_slug, author=excluded.author, description=excluded.description, context_length=excluded.context_length, input_modalities=excluded.input_modalities, output_modalities=excluded.output_modalities, supports_reasoning=excluded.supports_reasoning, parameters=excluded.parameters, active_parameters=excluded.active_parameters, disk_size_gb=excluded.disk_size_gb, moe=excluded.moe, notes=excluded.notes;"
```

The `zdr` column defaults to 1 for these (non-OpenRouter models are always Zero Data Retention by rule) - you do not set it.

### Step 4 - persist

Run `go run ./cmd/modelsdb update` (or save via the UI) to write the DB, then `modelsdb export internal/seedcatalog/catalog.json` and commit that file to publish (the app does not run git). HF-direct models are non-API, so their full identity/metadata (and `zdr`) are written to the exported catalog and ship embedded in the binary.

## Reliable bulk helpers (factual data only)

- `go run ./cmd/modelsdb update` - refresh the OpenRouter catalog (one API call).
- `go run ./cmd/modelsdb collections` - re-scrape the 6 type collections (image/video/STT/TTS/embedding/rerank): sets `model_type`/modalities and adds the models the API omits. Membership is factual, so this is safe to re-run.
- `go run ./cmd/modelsdb pricing` exists but only bulk-scrapes the first price block and cannot judge units or weighting - VERIFY anything it writes, or prefer the manual method above for pricing.

## Notes vs pricing_note convention

Two different columns - do not mix them:

- **`pricing_note`** (curated, objective, PUBLIC; surfaced in the UI as a bold orange note-document icon badge beside the unit under BOTH the In $ and the Out $ price - there is one note per model, so either badge hovers to read it and clicks to edit the same note) holds only the pricing SKUs that the In/Out columns cannot capture, after you have converted every fitting segment into In/Out (see rule 3). Never the unit itself (that is `measurement`), and never a price already in In/Out.
- **`notes`** (personal, subjective, stored only in `modelsdb.db` - never exported; surfaced in the UI on the third line of the model's **Name** cell, opened by the speech-bubble icon beside the name) is your private data: quality observations, OCR remarks, reminders. NEVER put pricing or unit information in `notes` - that is objective data and belongs in In/Out + `measurement` + `pricing_note`.
