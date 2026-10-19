---
name: update-database-manually
description: 'Manually fill in the model data the OpenRouter API does not provide for ModelsDB (the modelsdb.db SQLite catalog). Use whenever the user wants to "update the database" / "update the models" - which means BOTH the programmatic refresh AND a manual backfill pass - and specifically to: fill missing parameter counts, active parameters, mixture-of-experts answers, context lengths and modalities for recent models (looked up on HuggingFace), price the models the API omits (image/video/embedding/rerank/tts/stt), add HuggingFace-direct models with no OpenRouter listing, fix model capabilities, add a model, or record a pricing caveat. Invoke this even when the user does not name the skill.'
---

# update-database-manually

ModelsDB fills itself from OpenRouter, and OpenRouter does not state everything.
It never says how many parameters a model has. It does not state a price for
some of its image, video, speech and embedding models. It knows nothing at all
about a model that exists only as a HuggingFace repository. **This procedure is how a person or an assistant researches those
answers and writes them into the catalog.** Everything else arrives on its own
and cannot be typed: what OpenRouter or a repository states is never typed
over, and what you research is kept only until one of them states the fact.

## 1. What you need, and what you write with

**You need HTTP and nothing else.** Every step below is a `curl` command against
the running server. **The address in every example is `http://127.0.0.1:8122`**,
the default; `GET /api/health` answers with an `addresses` list of every address
whose listener bound, so use the one that reaches you.

**Two documents answer everything this one does not.** Read them rather than
guessing a request shape:

| Document | What it holds |
|---|---|
| `GET /api` | The complete HTTP reference: every route, every method, every request and answer shape, every refusal and its wording. It is authoritative, and a test fails when a route is missing from it. |
| `GET /api/columns` | Every column this catalog holds right now, with its key, its heading, what kind of value it takes and whether the app ships it. |

## 2. How this catalog stores a model

A model ROW carries only its IDENTITY, and none of it is a fact about the model:

| Identity field | What it is |
|---|---|
| `name` | The display name, and the primary key everything else hangs off. No request renames a row. A refresh renames an OpenRouter row when OpenRouter renames the model, and the old name then stops answering. |
| `source` | Whether the row is OpenRouter's: an update sets `openrouter` on every model the feed lists, whichever route created the row. The other values are `huggingface` and `manual` (an old row may still read `collection`). It decides one thing - a row sourced `openrouter` cannot be deleted, because the next update would create it again, and `POST /api/models/hide` is what takes it out of the list. |
| `hidden` | Whether the owner of this catalog has hidden the row from their own table. |
| `variant_of` | The name of the model this row is one downloadable build OF, empty on an ordinary row. `POST /api/models/variations/unfile` is the only thing that clears it. |

**Every other fact is a CELL**: one model, one column, one source, one value.
A column key is the plain name of the fact - `context_length`, `parameters`,
`notes` - and it carries no dot and no prefix. **Who stated a value is recorded
on the CELL, not in the key.** OpenRouter's context length for a model, a
repository's, and a researched one are three cells of the one `context_length`
column, stamped `openrouter`, `huggingface` and `research`. The sources:

| Source | Who writes it |
|---|---|
| `openrouter` | The catalog refresh, from OpenRouter's public model list. Also the app working a fact out from one of OpenRouter's (tool support, vendor, type, MoE, ZDR, EU routing, throughput, priced-per). |
| `huggingface` | An import, from a repository's own files. |
| `research` | **What this procedure writes.** A value found to fill a gap no automatic source has stated, by you or by an assistant, through `POST /api/save`. It ranks BELOW both sources above on every column, so it shows only until one of them states the fact. |
| `hand` | The owner of the catalog, in their own columns: the note, the favourite, the Architecture line, and every column they made. Nothing automatic ever touches them, and nothing but they writes them, with one exception: you write the Architecture line (section 6a). |
| `app` | The app itself. It states `added` and nothing else. |

**TWO KINDS OF COLUMN, and the line between them is who writes.** `GET /api/columns`
says which every column is, as `kind`:

| `kind` | What it is | What a write does there |
|---|---|---|
| `fetched`, `measured`, `derived` | OpenRouter states it, a repository is read for it, or the app works it out. An update maintains it. | Where the cell is EMPTY the write lands as `research`. Where an automatic source has stated it the write is REFUSED, naming the source and its value. |
| `yours` | Your own: `notes`, `favorite`, `architecture_basis` (heading "Architecture"), and every column they made. | Never written by this procedure, with ONE exception: `architecture_basis`, section 6a. Do not fill any other, and do not create one. |
| `stamped` | `added`, when the model entered the catalog. | Refused. |

**A plain read picks one source by RANK, and the rank is fixed in the code.**
Each column's `sources` on `GET /api/columns` is that order: OpenRouter first
where OpenRouter serves the fact, the repository first where it is read for it
(`parameters`, `moe`), `research` last. Nothing changes it. Two columns MERGE
every source's list instead: `input_modalities` and `output_modalities`.

Four consequences:

1. **Nothing you write is ever overwritten.** A refresh writes as `openrouter`,
   an import as `huggingface`, each a different cell from yours. What changes
   is what SHOWS: the day a source states the fact, its answer is shown and
   yours sits beneath it, dated, in `GET /api/models/{name}/answers/{column}`.
2. **Nothing is refused because of what a model's `source` is.** What decides a
   write is the cell: empty takes research, stated by an automatic source
   refuses.
3. **Clearing a cell is saying nothing, not saying no.** Send `""` and your
   research cell is deleted. To record that a model does NOT do something,
   write `"no"`.
4. **A research cell an automatic source has overtaken can only be cleared.**
   A non-empty write there is refused like any other; an empty value deletes
   the researched cell and nothing else. `GET /api/models` names such columns
   in `overtaken`, and the panel shows them as no longer shown.

**The one column nothing typed reaches is `added`**, when the model entered this
catalog. Its only source is `app`, and a write naming it is refused.

**Every value is sent as TEXT**, whatever the column holds: a number as a quoted
figure (`"0.00000013"`), a flag as `"yes"` or `"no"`, a list as a JSON array
inside a string (`"[\"text\",\"image\"]"`), a choice as one of its own options.
`parameters`, `active_parameters` and `weights_size_gb` are stored at two decimal
places whoever writes them.

## 3. What "update the database" means: two phases

"Update the database" or "update the models" is a TWO phase job, and phase 2 is
not optional.

**Phase 1, the programmatic refresh.** `POST /api/update` starts the OpenRouter
catalog fetch in the background. It stores EVERY model the feed lists, creating
a visible row for one this catalog does not hold yet. `GET /api/update/status`
reports on it: finished when `running` is false and `finished_at` is set,
finished CLEANLY when `errors` is also empty, and `added` is how many rows it
created. A failure records an error rather than aborting.

**Phase 2, the manual backfill.** Everything below: parameter counts, the prices
of the models the API does not price, the modalities the feed does not state,
and the models that are only on HuggingFace.

## 4. Find what needs filling in

Filters name a column by its key, they are combined with AND, and **an EMPTY
value matches the models that have no value in that column from any source**,
which is how "nobody has answered this" is asked.

```bash
# Models that carry a HuggingFace repository and that nobody has counted.
curl 'http://127.0.0.1:8122/api/models?include_hidden=0&repository=contains:/&parameters=&fields=name,repository'

# Models nobody has priced, mostly the ones the API lists on web pages only.
curl 'http://127.0.0.1:8122/api/models?include_hidden=0&price_prompt=&fields=name,source'

# Models that say nothing about what they take in.
curl 'http://127.0.0.1:8122/api/models?include_hidden=0&input_modalities=&fields=name,source'
```

**Asking who answered.** `by:<source>` keeps the models where that source's
answer is the one shown, `by:not_<source>` keeps every other model. It asks who
stated the value, never what the value is:

```bash
# The models whose context length is a researched one, still unconfirmed by a source.
curl 'http://127.0.0.1:8122/api/models?include_hidden=0&context_length=by:research&fields=name'
# The models where no source states the parameter count and nobody has researched it.
curl 'http://127.0.0.1:8122/api/models?include_hidden=0&parameters=&fields=name,repository'
```

**`source` is not a filter**, because it is an identity field rather than a
column. Ask for it in `fields=` and pick the rows yourself.

**Prefer recent models**, roughly the last three months; ask for `created_at`
in `fields=` and sort the answer yourself.

**Leave the builds out.** A row whose `variant_of` is not empty is one quantized
build, its numbers MEASURED off its repository. Do not research them; the default
answer already leaves them out, because a build starts hidden.

### What ONE model is still missing

```bash
curl 'http://127.0.0.1:8122/api/models/brief?name=Qwen:%20Qwen3.6%2027B'
```

The fastest way to work one model. `gaps` is one object per column research
may fill on this row and that can apply to it - empty, and not one of your
own - and every `field` in it is EXACTLY the key `POST /api/save`
takes. `label` is the column's heading, `about` says what it means, `current`
reads `not set`, and `accepts` is the form that column takes, including the
words a closed set allows; `measured` marks a column READ off a repository,
which you fill by re-importing rather than by typing. `held` is what the row
already holds in the columns that can apply to it, each with who stated it, so
a wrong value is reported in your answer rather than written over. `brief` is
the same answer as one block of text. Every value in `example` is a `<...>`
placeholder, refused if sent as printed.

### Where the sources disagree

```bash
curl 'http://127.0.0.1:8122/api/models/OpenAI%3A%20GPT-5/answers/context_length'
```

Every source's own answer for one model in one column, when each was written,
and which one a plain read shows. A row from `GET /api/models` also carries
`disputed`, the columns where OpenRouter and the repository disagree. Nothing
you write settles one: the column's fixed order decides, and the panel says so.

## 5. Writing what you found

One shape writes everything. The body is a JSON ARRAY, one object per model, each
naming the model and the cells to change:

```bash
curl -X POST http://127.0.0.1:8122/api/save -H 'Content-Type: application/json' \
  -d '[{"name":"Qwen: Qwen3.6 27B","cells":{"parameters":"27","moe":"no"}}]'
# {"ok":true,"records":1,"results":[{"name":"Qwen: Qwen3.6 27B","written":true}]}
```

Every key inside `cells` is a column key and every value is text. **The write
is stored as `research`**, beneath every automatic source: it is shown where
nothing automatic has stated the fact, and the day OpenRouter or the
repository states it, theirs is shown and yours is still there at
`GET /api/models/{name}/answers/context_length`, dated.

**A batch is honest, item by item.** Each object is committed on its own and the
answer carries one result per item in the order sent. `POST /api/save` never
creates a model: an unknown name is refused, and `POST /api/models/add` adds one.

**Write only the columns `GET /api/models/brief` offers, and `architecture_basis`.** The brief never lists the Architecture column, because it is one of the owner's; section 6a says when you write it. A key no column holds is refused, and this procedure never creates a column: columns are the owner's to make, in the page or with `POST /api/columns`, and the columns they made are theirs alone to fill.

**Five things are refused, and none of them writes anything:** a cell an
automatic source has already stated (the refusal names the source and its
value); `added`; a key naming no column, a dotted one such as
`openrouter.context_length` included; a value the column will not hold, such as
`"quite long"` for a number, which refuses that whole item; and a field the body
shape does not carry.

## 6. Reading a HuggingFace repository accurately

**Prefer `POST /api/models/import` (section 9)**, which stores measurements
rather than typed figures. Read one by hand only when the import cannot: a
gated repository, which answers its metadata while refusing its files, or a
figure the import does not carry.

### The three addresses you fetch

| # | Address | What it gives you |
|---|---------|-------------------|
| 1 | `https://huggingface.co/api/models/<repo>?blobs=true` | Repository metadata AND every file's exact size in bytes. |
| 2 | `https://huggingface.co/<repo>/resolve/main/model.safetensors.index.json` | Which shards a loader actually reads. |
| 3 | `https://huggingface.co/<repo>/resolve/main/config.json` | The architecture. |

**1. The listing.** `?blobs=true` adds `size` (exact bytes) to every `siblings`
entry, and an `lfs` object on a file stored in Git LFS. The same answer carries
`sha` (the commit `main` pointed at when you read it; write it down, `main`
moves), `tags` including `base_model:...`, `safetensors.parameters` and
`safetensors.total`, and a `gguf` block on a GGUF repository.

**2. The shard index.** The files a loader really reads are the DISTINCT VALUES
of `weight_map`. Sum those files' `size` from address 1: that, and only that, is
the weight size. HTTP 404 means the repository is not sharded and its single
`model.safetensors` is the whole set. Summing every weight file instead is two
or three times too big, because repositories ship second copies such as
`consolidated.safetensors` or `original/model.safetensors`. The index's own
`metadata.total_size` lands slightly under the file sizes; use the file sizes.

**3. The architecture.** `config.json` holds `architectures`, `model_type`,
`max_position_embeddings`, and the layer and head counts. On a multimodal model
these sit one level down, inside `text_config`. **`max_position_embeddings` is the
context window of a language model only.** An embedding model's window is the
`max_seq_length` in the repository's `sentence_bert_config.json` (the import
reads it first), or the length its card states; the position table of the model
underneath it is often many times longer.

**Always `/resolve/main/`, never `/raw/main/`.** For a file under Git LFS
(`tokenizer.json` by default), `/raw/main/` answers a three-line pointer with
HTTP 200 and not the file, and the pointer is not JSON.

**GGUF repositories have no `config.json`** (HTTP 404, `Entry not found`). The
`gguf` block on address 1 carries `total` (a parameter count, which the import
stores as `parameters`), `architecture` (which it stores as `gguf_architecture`)
and `context_length`; its `totalFileSize` is the size of ONE quantization file, so
take sizes from `siblings`, and its `chat_template` goes into no column. For the
architecture, take the plain `base_model:<owner>/<name>` entry from `tags`
(the qualified twin, `base_model:quantized:...` or `base_model:finetune:...`,
names the relationship) and fetch THAT repository's `config.json`, following the
chain as far as it goes. The size and file count still come from the GGUF
repository's own listing.

**Recording a quantized build** is the app's job: the Var column's download icon
searches a model's quantized repositories and records the one you pick, measuring
size, format and bit width itself.

### Cross-checks before you write a number down

**Bytes per parameter.** Roughly 2 at 16-bit, 1 at 8-bit, 0.5 at 4-bit:

| Repository | Weight bytes | Parameters | Bytes per parameter |
|---|---|---|---|
| `Qwen/Qwen3-32B` (16-bit) | 65,524,328,560 | 32,762,123,264 | 2.00 |
| `Qwen/Qwen3-32B-FP8` (8-bit) | 34,322,567,640 | 32,762,123,264 | 1.05 |
| `mlx-community/Qwen3-32B-4bit` | 18,429,850,874 | 32,762,123,264 | 0.56 |

More than about 20 percent off means you summed the wrong set of files.
- **`safetensors.total` is not the parameter count** on a quantized repository:
  it counts STORED ELEMENTS, and packed low-bit weights put several parameters
  into one element (`mlx-community/Qwen3-32B-4bit` reports `U32: 4,095,180,800`
  for a 32-billion parameter model).
- **Never compute a size from a parameter count.** `unsloth/Qwen3-32B-bnb-4bit`
  reports `U8: 32,180,951,978` while its weight files total 19,211,935,565
  bytes. The file sizes from address 1 are the authority, always.
- **A weights size is decimal GB**: the summed bytes divided by 1,000,000,000,
  matching HuggingFace's Files tab.
- **Do not derive an attention-cache figure from field names.** A config can list 64 `num_hidden_layers` while `layer_types` makes 48 of them linear attention, and a sliding-window layer does not hold a full-length cache. Write the Architecture line (section 6a) from what the config says, and say which parts you could not establish.

### Writing what the repository told you

```bash
curl -X POST http://127.0.0.1:8122/api/save -H 'Content-Type: application/json' \
  -d '[{"name":"Qwen: Qwen3.6 27B","cells":{
        "parameters":"32.762123264","active_parameters":"3.3",
        "weights_size_gb":"65.52","moe":"yes",
        "context_length":"262144","repo_owner":"Qwen"}}]'
```

A slug such as `...-35B-A3B` means 35 total and 3 active, mixture of experts,
but CONFIRM it against the repository's own files when the slug is not explicit.

### 6a. The Architecture line

The import does not record how a model is built. You write it, as ONE cell of the `architecture_basis` column (heading "Architecture"), after every import and every re-read, after adding a model by hand whose repository you can read, and whenever the cell is empty. It is the one column of the owner's that this procedure writes. It holds facts about the build of the model and nothing else, and the owner edits it freely afterwards.

**Find the models that need one:**

```bash
curl 'http://127.0.0.1:8122/api/models?include_hidden=1&architecture_basis=&repository=contains:/&fields=name,repository,variant_of'
```

**Read `config.json`** (section 6, address 3). For a GGUF repository read its base model's, following the `base_model` tag; for a multimodal model read the `text_config` level. Take `num_hidden_layers`, `layer_types` when present, `num_key_value_heads` (default `num_attention_heads`), `head_dim` (default `hidden_size / num_attention_heads`), `hidden_size`, `num_attention_heads`, `architectures[0]`, `torch_dtype` or `dtype`, and `quantization_config` with its method and bit width. The weight file format comes from the listing (address 1): `safetensors` or `pytorch_bin`.

**Count the cache the way a loader does.** Only a layer that holds a full-length cache is one that "holds a KV cache". A sliding-window layer does not, a layer that shares an earlier layer's cache does not, and a recurrent layer (linear attention, convolution, Mamba) keeps a fixed-size state instead of a cache that grows with the context. Where the config states separate global figures for the layers that hold the cache (a global head dimension, a global key-value head count), use those. Bytes per token = 2 x cache layers x KV heads x head dim x bytes per element (2 for bfloat16 and float16, 4 for float32, 1 for fp8). A figure you cannot establish is left out of the line, never estimated.

**The line has two sentences**, each part present only when you established it:

`<N> layers[ (<count> <type>, ...)], <A> hold a KV cache (or: all hold a KV cache); <R> recurrent layers (or: no recurrent state); head dim <D>, <K> KV heads (hidden size <H>, <P> attention heads); KV cache <B> bytes per token; recurrent state <S> bytes per recurrent layer.`

`Class <architectures[0]>; weights <dtype> in <format>; quantization <method>, <bits> bits per weight.` Bits per weight appear only when below 16. For a filed build, the second sentence states the build: `weights <build_format>, <build_bits_per_weight> bits per weight`.

A dense model: `40 layers, all hold a KV cache; no recurrent state; head dim 128, 8 KV heads (hidden size 5,120, 32 attention heads); KV cache 163,840 bytes per token. Class Mistral3ForConditionalGeneration; weights bfloat16 in safetensors.`

A hybrid model: `40 layers (30 linear_attention, 10 full_attention), 10 hold a KV cache; 30 recurrent layers; head dim 256, 2 KV heads (hidden size 2,048, 16 attention heads); KV cache 20,480 bytes per token. Class Qwen3_5MoeForConditionalGeneration; weights bfloat16 in safetensors.`

**Write it** with `POST /api/save` (section 5), the whole cell, one object per model:

```bash
curl -X POST http://127.0.0.1:8122/api/save -H 'Content-Type: application/json' \
  -d '[{"name":"Mistral: Ministral 3 14B 2512","cells":{"architecture_basis":"40 layers, all hold a KV cache; ..."}}]'
```

**Rules.**

- Facts only: no source names, no commentary, no remark about testing, parking or what the model is good for.
- If the cell already holds text, read it first and change only the facts that are wrong or missing. Every word the owner wrote stays.
- A repository with no `config.json` and no base model that has one: write nothing. Never write "unknown".
- A filed build (`variant_of` set): read its own `config.json` or its base model's, and when the layers and heads match the parent's, give the build the same facts as the parent's line. Copy facts, never the parent's remarks.
- Quote the repository `sha` you read in your answer to the person, not in the cell.

## 7. Pricing: read the page, decide the unit, then write

> Read and understand each page YOURSELF. Do NOT write a parser to bulk-extract
> pricing. The formats and units vary per model and need your judgement. One
> "transcribe" model is priced per TOKEN while another is per MINUTE of audio,
> and only reading the page tells you which.

**Only price what nobody has priced.** The API refresh prices every model it
lists, and a price it states cannot be typed over. This step is for the ones it
omits: the image, video, embedding, rerank and speech models whose price it does
not state, and the models you run yourself. What you write
lands as research and is replaced the day the API quotes the model.

1. **Fetch the model's page**, `https://openrouter.ai/<id>`, where the id is the
   `id` cell. The pricing is in the page's JSON under `pricing` and
   `display_pricing` (`sku_label`, `price`, `unitLabel`, `displayMultiplier`).

2. **Understand the units.** This is the point of doing it by hand:

   | Kind of model | Priced by |
   |---|---|
   | chat and text | per 1M tokens (prompt is input, completion is output) |
   | image | per image, or per megapixel |
   | video | per second |
   | text to speech | per 1M characters |
   | speech to text | per minute, hour or second of audio |
   | embedding | per 1M tokens, input only |

   The human-readable number is `price` times `displayMultiplier`.

3. **Pick the column by what is MEASURED, not by the unit.**

   - A produced IMAGE: `price_image_output`. An image the model takes IN:
     `price_image`. They are two different charges and two different columns.
   - Other output media, so video or audio out: `price_completion`.
   - What the model otherwise CONSUMES, so speech-to-text audio, text-to-speech
     characters, a rerank search: `price_prompt`.
   - Chat, text and embedding: `price_prompt` in, `price_completion` out.

   A model may fill both an input and an output column when it charges for both.

4. **Scale the stored number by the unit.** A "per 1M" unit stores the price PER
   SINGLE UNIT: $0.13 per 1M tokens is `"0.00000013"`, and $7 per 1M characters is
   `"0.000007"`. Every per-single-unit measurement stores the literal amount:
   $0.112 per second is `"0.112"`, and $0.04 per image is `"0.04"`.

5. **Record the unit in `price_unit`** (`per 1M tokens`, `per 1M characters`,
   `per second`, `per minute`, `per hour`, `per image`, `per megapixel`,
   `per search`). Do not restate the unit anywhere else.

6. **Leave the Note to the owner.** The leftover price lines the In and Out
   columns cannot hold go in `notes`, one of their own columns, and this
   procedure does not write it. Report a
   segment you could not convert - a `Video Tokens (with audio): $7/M tokens`
   beside a per-second price - so they can note it themselves.

```bash
curl -X POST http://127.0.0.1:8122/api/save -H 'Content-Type: application/json' \
  -d '[{"name":"Minimax: Hailuo 3","cells":{"price_completion":"0.1",
        "price_unit":"per second"}}]'
```

## 8. Modalities, and other capabilities that are simply absent

**A blank modality list is never left blank.** Every filter for what a model takes
in or gives out reads those columns, so a model with an empty one is invisible to
that half of the filter bar. The image and video models the API lists on web
pages only can carry no INPUT modality at all, so those rows need this pass.

**How to find the real answer**, in this order:

1. **The OpenRouter API**, when the model is in it: `curl -s
   'https://openrouter.ai/api/v1/models?output_modalities=all'` and read
   `architecture.input_modalities` and `architecture.output_modalities` for that
   id. This is the authoritative statement and needs no judgement.
2. **The model's OpenRouter page**, `https://openrouter.ai/<id>`, when the API
   does not list it. It carries the same
   fields in its embedded JSON; check that every occurrence in the page agrees,
   because a page can carry other models' data too.
3. **The vendor's own documentation**, when neither has it: a HuggingFace model
   card, the provider's API reference, the model's announcement.

Write what you read, never what the type suggests. Several image and video
models state audio and video inputs, so a guess from "it is a video model" is
wrong.

**A modality answer of yours is accepted only while NO source has stated a list
for that row.** The two modality columns merge every source's list, research
included, so once OpenRouter or the repository states a list later, it joins
yours in the merged answer and yours stays. While a source's list stands, a
write is refused, so this pass is for rows that show nothing at all. If a
stated list looks wrong or short (a repository that omits audio input, say), do
not try to write over it: report the field, the value shown and what the
source really says, in your answer to the person.

**`model_type` is `decisions` for a model whose output is a score or probability
for each allowed option rather than text.** OpenRouter states it as the output
`decisions`; the import recognises a repository by its `decision-model`,
`typed-decisions` or `calibrated-probabilities` tags or by its card describing
the output that way. A model that also answers in plain text states
`["decisions","text"]`. Where nothing has typed a row, write the word yourself.

```bash
curl -X POST http://127.0.0.1:8122/api/save -H 'Content-Type: application/json' \
  -d '[{"name":"Minimax: Hailuo 3","cells":{"input_modalities":"[\"text\",\"image\"]"}}]'
```

**Reference images are a capability of this kind.** Three columns hold them:
`reference_images` (a flag), `max_reference_images` (a number) and `frame_images`
(a list). For an image model OpenRouter states the first two itself. For a video
model it states only `frame_images`, so `reference_images` and
`max_reference_images` stay empty until you research them. Only an explicit
statement counts: the model's own OpenRouter page or its card says it takes
reference images, or gives a count. A pricing key named `reference_images` is
billing evidence, not a statement, so it is never grounds for a `"yes"`. Where
nothing is stated, leave the cell empty.

## 9. Adding a model that is only on HuggingFace

**Let the server read the repository.** One request, and no number is typed:

```bash
curl -X POST http://127.0.0.1:8122/api/models/import -H 'Content-Type: application/json' \
  -d '{"url":"https://huggingface.co/Qwen/Qwen3-32B"}'
```

The full page URL, with or without `/tree/main`, a query string or its scheme,
and the bare `Qwen/Qwen3-32B` are all accepted. The body carries the address and
NOTHING else, so every value it stores is a measurement, a `huggingface` cell.

**It refuses rather than half-adding**, and writes nothing when it does: when the
catalog already holds that model (looked up by repository, then OpenRouter id,
then name; the refusal names the row); when the address cannot be used or the
repository is GATED, because the row would be an empty shell; and when
HuggingFace could not be reached.

**Reading a model again** refreshes what the repository measures: the parameter count, the size, the description, the modalities, the context length and the creation date. It does not touch the architecture, so after an import or a re-read, write the Architecture line (section 6a):

```bash
curl -X POST http://127.0.0.1:8122/api/models/import -H 'Content-Type: application/json' \
  -d '{"url":"Qwen/Qwen3-32B","reimport":true}'
```

It writes onto the row that already holds the model, whatever it is called. It
rewrites HuggingFace's own cells and touches nothing of the owner's, because it
writes as `huggingface`. A researched value on a column the repository
measures is replaced where the repository states one and kept where it does
not.

**When the import cannot reach the model at all** (a gated repository, or a model
on no repository), add the row by hand. One object, not an array, and it creates
the row as `source=manual`:

```bash
curl -X POST http://127.0.0.1:8122/api/models/add -H 'Content-Type: application/json' \
  -d '{"name":"Qwen: Qwen3.9-35B-A3B HF","cells":{
        "repository":"Qwen/Qwen3.9-35B-A3B","model_type":"chat",
        "context_length":"262144","moe":"yes",
        "supports_reasoning":"yes","parameters":"35",
        "active_parameters":"3","weights_size_gb":"70.6",
        "repo_owner":"Qwen"}}'
```

Every fetched and measured column can be filled in that one request: the row
carries no listing and no repository read, so nothing automatic has stated
anything, and every value lands as research. It refuses on the same three
identifiers the import checks, a missing name, a key naming no column or
`added`, or a value a column will not hold, and **no row is created when it
refuses**. Name it `Author: Model Name`, with ` HF` appended when an OpenRouter
twin exists.

## 10. When a row looks like a model already here

**Say what you found, recommend one action, and leave the decision to the person.**
Do not merge two rows, delete either one, or rewrite one to match the other,
however obvious the duplicate looks: no request merges rows (the refresh folds
only OpenRouter rows holding one OpenRouter id, and only when it loses nothing the
owner typed), removing a row destroys its notes, favourite flag and researched answers with no undo, and
whether two rows are one model depends on what the owner uses them for.

Report the two display names exactly as stored; what makes them look like one
model; what each row holds that the other does not, especially notes, favourite
and researched answers; the `source` of each; and one recommended action. Then
stop.

The add routes compare the repository ignoring capitals, then the OpenRouter id,
then the display name, and refuse a match. Two rows sharing no identifier slip
past, so adding a HuggingFace model by its repository address is what reliably
prevents a duplicate.

**Check `GET /api/models/similar?name=<the name you are about to add>` BEFORE
adding a row.** It finds a close-but-not-identical name (punctuation, a
misspelling, a shared word) that the exact checks above would miss, so a
duplicate this section describes is caught before it is created rather than
reported after.

## 11. Removing a model

`POST /api/models/delete` with `"confirm_delete":true` removes a row whose
`source` is anything but `openrouter`.

**A model OpenRouter lists cannot be deleted** and the request answers 409. The
refresh (`POST /api/update`) stores every model the feed carries, so the row
would be created again on its next run. `POST /api/models/hide` is what takes
such a model out of the list, and it keeps the row and everything written about
it.

**Delete destroys that model's personal data permanently**: notes, favourite
flag, every column the owner made, and every researched answer. To keep all of
that while taking the model out of the list, use `POST /api/models/hide`
instead.

## 12. A feed field no shipped column reads

Every field the OpenRouter feed carries is a column. The refresh
(`POST /api/update`) makes a column for every leaf path no shipped column
reads, filled in that same refresh and kept current by every later one, so
nothing here is imported and nothing is typed. Such a column is keyed by the
whole path (`top_provider.context_length` is `top_provider_context_length`),
arrives hidden, and is sealed like every shipped column: relabel, move and hide
it, never delete it. Find it in the Columns dialog by its heading, its key or
the feed's own path, which `GET /api/columns` lists as `feed_paths`. `GET /api`
under `POST /api/update` states the rules in full.

## 13. Two conventions, and one backup

**`notes` holds the pricing lines too.** It is the owner's own column, and this
procedure does not write it; it is named here so a report to the owner puts the
right thing in the right place.

- `notes` is their private overlay: quality observations, reminders, and the
  pricing lines the In and Out columns cannot capture, after every fitting
  segment has been converted. Never the unit itself, which is `price_unit`, and
  never a price already in a price column.

**Back the database up before a large pass.** `modelsdb.db` is the only file any
of this data lands in, and there is no export:

```bash
curl -X POST http://127.0.0.1:8122/api/backup/run -H 'Content-Type: application/json' \
  -d '{"dir":"<the backup folder saved in the settings>","overwrite":false}'
```

Only the backup folder saved in the settings is accepted, and it is on the
SERVER's disk: a request naming any other folder, or made while none is saved,
answers `{"ok":false,...}` and writes nothing, so ask the owner to save the
folder first. Prefer it over copying the file by hand: the
database runs in write-ahead-log mode, so a hand copy of `modelsdb.db` alone
silently misses the newest writes.
