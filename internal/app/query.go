// The query API for other local apps/agents, and the markdown
// guide that documents the whole HTTP surface.
//
// GET /api/models?<filters>  -> JSON array of matching models
// GET /api/models  (no query) -> the markdown guide below
// GET /api                    -> the markdown guide below
//
// apiGuide is the authoritative reference for every route the server registers,
// including the write endpoints in handlers.go and removal.go. A route added
// without a heading of its own here fails TestEveryRegisteredRouteIsDocumented,
// which ranges over serverRoutes() in server.go - so the guide cannot fall
// behind the routes again without a red test.

package app

import (
	"fmt"
	"modelsdb/internal/paths"
	"net/http"
	"strings"
)

func handleAPIGuide(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/markdown; charset=utf-8")
	// The guide embeds the default port in its examples; reflect the active one.
	w.Write([]byte(strings.ReplaceAll(apiGuide, "127.0.0.1:8122", fmt.Sprintf("127.0.0.1:%d", paths.Port))))
}

const apiGuide = `# ModelsDB API

Local catalog of AI models (OpenRouter + HuggingFace + what you add), served by
the ModelsDB app running on this machine.

Base URL: http://127.0.0.1:8122

NO AUTHENTICATION OF ANY KIND. On the default loopback bind only this machine can
reach the server. A non-loopback bind (a Tailscale or LAN address configured as
"host") hands full read AND write to anyone who can route to that address -
including the endpoints that destroy model data and the one that replaces the
program.

THIS GUIDE IS THE COMPLETE REFERENCE. Every route the server registers has a
section of its own below, listing every HTTP method its handler accepts. A route
that is not in this guide does not exist.

## How to read this guide

- "Endpoints" lists every API route, one section each: the methods, the request
  shape, the response shape. One of them, GET /api/guide/update-database-manually,
  serves a second document: the procedure for researching a model's missing data
  by hand and writing it back here.
- "Writing model data" is the whole write surface: a write names COLUMN KEYS,
  exactly as a read answers them - one bare key per fact, never a dotted one.
  THERE ARE TWO KINDS OF COLUMN, AND THE LINE BETWEEN THEM IS WHO WRITES. A
  column an automatic source states - OpenRouter, a HuggingFace repository, or
  the app working a fact out from one of them - is never typed over: where it
  is EMPTY a write fills it in as RESEARCH, kept until an automatic source
  states the fact; where an automatic source has stated it the write is refused
  with 400 naming that source and its value. A column of the reader's own - the
  note, the favourite, the Architecture line, every column they made - is written
  as "hand" and no update ever touches it. A key naming NO column is refused:
  columns are made with POST /api/columns, never on the way past a write.
- "Several sources, one column" is how a fact several sources can state - a
  price, a context length - resolves to one answer, in an order fixed in the
  code, and the route that shows every source behind it.
- "Reading the catalog" is the shape GET /api/models answers with - a model's
  identity, its value in every column, and which source that value came from -
  and "The query grammar" is how a request narrows that list and picks what
  each row carries. Both speak COLUMN KEYS, which GET /api/columns lists.
- Two more paths exist and are deliberately not documented here, because no API
  caller uses them: "/" serves the browser app's page, and "/static/" serves that
  page's CSS and JavaScript.

## Endpoints

The methods named in each heading are exactly what that handler accepts; any
other method answers 405 Method Not Allowed. Requests and responses are JSON
unless a section says otherwise. A malformed JSON body answers 400.

ANY REQUEST THAT IS NOT GET OR HEAD ALSO ANSWERS 403 Forbidden, on every route,
when it carries an "Origin" header naming an address other than the one in its
own "Host" header, and other than an address listed under "allowed_origins" in
config.jsonc. GET /api/health reports how many listed addresses are currently
in effect. An "Origin" that names no address at all - the word "null", an
empty value, or anything that will not parse as a web address - is refused the
same way, and so is a request carrying more than one "Origin" header. That is
what stops a web page you opened elsewhere writing to this server through your
browser. A caller that is not a browser sends no "Origin" header and is never
refused: curl, a script and an assistant all work unchanged, so long as no
"Origin" header is added by hand. The refusal is plain text, and nothing in the
request body is read or written.

EVERY REQUEST, GET INCLUDED, ALSO ANSWERS 403 when its "Host" header names an
address this server does not answer to. The names it accepts are localhost, any
loopback address, an address a listener of this process really bound, and the
host of an entry under "allowed_origins" in config.jsonc. A web page on another
site can rebind its own name to this machine, and its requests then carry that
name in both "Host" and "Origin"; refusing the name is what stops it reading or
writing here. Reach the server by one of those names. Behind a reverse proxy, list
the public address under "allowed_origins" and restart.

A request body larger than 16 MiB is refused with 413 and nothing is written.

### GET /api

This guide, as text/markdown. This one handler does not check the method, so any
verb returns it.

### GET /api/guide/update-database-manually

The MANUAL-RESEARCH PROCEDURE, as text/markdown: how the data the OpenRouter API
never returns - parameter counts, the prices of the models it does not price, the
models that exist only on HuggingFace - is researched and written into this
catalog by hand. Read it when you have been asked to fill a model in, or when a
POST /api/models/import answer handed you a "missing" list.

It is served after a preamble this app writes, naming the five requests the pass
is built from and the two things HTTP cannot do: running one refresh phase on its
own, and taking a backup onto your own disk. Every value the procedure
researches, the prices included, is written over HTTP, on every model.

This handler does not check the method either. The text is long, which is why it
is a route of its own rather than a section here.

### GET /api/models

The catalog. With a query string it returns a JSON array of models (see "Reading
the catalog" and "The query grammar"); with an EMPTY query string it returns this
guide instead, so pass at least one parameter to get JSON at all. The shortest
request that returns every model is GET /api/models?include_hidden=0.

A PARAMETER THIS QUERY DOES NOT TAKE ANSWERS 400 AND NAMES IT, offering the
columns whose key and whose label come closest. Nothing is ever ignored, because
a filter this server dropped would answer with the whole catalog while looking
like a narrow answer.

### GET /api/models/similar

The models in this catalog whose NAME is closest to one given, best first - the
route behind the search box's "Did you mean" and behind the "similar" advisory
POST /api/models/add and POST /api/models/import answer on success. It never
writes.

  curl 'http://127.0.0.1:8122/api/models/similar?name=Qwen3-32B'
  # [{"name":"Qwen: Qwen3-32B","score":1,"why":"one name contains the other",
  #   "hidden":false,"variant_of":""}, ...]

Three parameters, and only ?name= is required:

  name             the name to find something close to. Required; empty or
                   missing answers 400.
  limit            how many rows to return, best first. Default 10, maximum
                   50; above that answers 400.
  include_hidden   also match against hidden rows and variations, same as on
                   GET /api/models. Default 0.

An unknown parameter answers 400 and names it, the way GET /api/models does.

CLOSE IS DECIDED BY THE SAME MATCHER GET /api/models' unknown-parameter refusal
already uses to suggest a column, extended with FOLDING: a name is lowercased
and split into parts on / - _ . : and space, and on a boundary between a letter
and a digit, so "Qwen3-32B" and "Qwen-3-32B" fold to the same parts and score as
the same name. The score is the best of comparing the two names as written and
comparing their folded parts - LOWER IS CLOSER:

  0   the same name written differently (identical, or identical once folded)
  1   one name contains the other
  2-4 within a few single-character changes (counted in whole characters, not
      bytes)
  5   the two names share a whole word of three letters or more

A row scoring anything else - no match at all - is left out of the answer
entirely; the ties are broken by name, so the same catalog always answers in
the same order for the same request. "why" is one of the four phrases above,
in words.

### GET /api/models/{name}/answers/{column}

Every source's own answer for ONE model in ONE column, whether or not it has
answered, when each answered, and which one a read currently shows. This is
the answers panel the browser draws behind a cell's mark.

  curl 'http://127.0.0.1:8122/api/models/OpenAI%3A%20GPT-5/answers/context_length'
  # {"column":"context_length","label":"Context length","value_type":"number",
  #  "resolve":"first_stated",
  #  "answers":[{"source":"openrouter","label":"OpenRouter","value":"400000","rank":0,"written_at":"2026-09-16T06:00:00Z"},
  #             {"source":"huggingface","label":"HuggingFace","value":"399000","rank":1,"written_at":"2026-08-30T10:12:00Z"},
  #             {"source":"research","label":"Research","value":null,"rank":2}],
  #  "winner":"openrouter"}

{name} is a model's name, exactly as GET /api/models returns it, URL-encoded;
{column} is a column key. An unknown model or column answers 404, naming which.

"answers" lists every source THIS COLUMN DECLARES, in the column's fixed rank
order (see "Several sources, one column"), whether or not that source has
actually stated a value: a source with nothing recorded still gets an entry,
with "value":null and no "written_at", so the caller can see who has not
weighed in as well as who has. "written_at" is when that source's cell was
last written. "label" on each answer is that source's own label, except
hand's, which is always "You". "winner" is the source whose value a plain
read of this column currently carries - the highest-ranked source with a
non-null value - or "" when nothing has answered at all. A UNION-RESOLVE
column (input_modalities, output_modalities) still lists every source's own
answer here; the merge itself is what GET /api/models shows, not this route.

### GET, POST, PATCH, DELETE /api/columns

Every column this catalog holds, and the three things you may do to your own:
make one, change its heading, and delete it.

  GET     -> every column, in the order the table draws them (below), each
             saying what KIND of column it is and who may state it. A column
             a feed path fills also carries "feed_paths", the feed's own names
             for it: the Repository column answers ["hugging_face_id"], and a
             column the refresh made from the feed answers the path it was
             made from, so a column can be found by the word the feed uses as
             well as by its heading or key.
  POST    -> create one of your own from
             {"label":"Sizing notes","value_type":"text"}. "value_type" is text,
             number, flag, choice or list, and a choice column also takes
             "options", in the order they sort in. The key is worked out from the
             heading unless you send one, and it carries no dot: a column key
             names no namespace, because who states a column's cells is
             recorded on the cell, not in the key. Answers 201 with the column as
             it was stored, key included, with "hand" as its one source: only
             you write it, and no update or assistant ever touches it. A
             heading another column of yours already holds, a kind of
             value this app does not store, a choice column with no options, and
             a key already belonging to another column each answer 400.
  PATCH   -> {"key":"sizing_notes","label":"Sizing"} changes the HEADING,
             and {"key":"rating","color_rules":{...}} changes what COLOUR
             the column paints its cells (below). Send either, or both. Every
             stored value follows a rename, because a value names its column by
             key and the key does not move. A heading another column of yours
             holds answers 400; a key naming no column answers 404; a request
             carrying neither a label nor colours answers 400.
  DELETE  -> {"key":"sizing_notes","confirm":true} removes it AND every
             model's value for it.

**DELETE REQUIRES {"confirm":true} IN THE BODY**, exactly as POST /api/models/delete
does. One request destroys a value on every model at once, this database is the
only copy of those values, and nothing upstream re-creates them - so without the
flag the request answers 400 and deletes nothing.

TWO COLUMNS CANNOT BE DELETED AT ALL, and both answer 400. A column the app SHIPS
is sealed: it can be relabelled and hidden, never removed, because every writer
of that source binds through its key and its kind. A column ANY AUTOMATIC
SOURCE CAN STILL STATE is refused too, naming that source: it would be
recreated, empty, on the next refresh, which would look like the delete never
happened.

A DELETED COLUMN STAYS DELETED. No refresh and no import creates a column of
yours: an automatic source writes only the columns that declare it. Create it
again whenever you want it and it comes back EMPTY.

The list is the one a caller needs: a model row answers under these same keys, a
filter names one of them, and a value is written under the key of the column it
belongs to.

  [{"key":"context_length","label":"Context length","resolve":"first_stated",
    "value_type":"number","unit":"tokens",
    "about":"How much context the model takes.","position":1,"visible":true,
    "sealed":true,"created_at":"...",
    "kind":"fetched","sources":["openrouter","huggingface","research"]},
   {"key":"supports_tools","label":"Tool","value_type":"flag", ...,
    "kind":"derived","sources":["openrouter","research"],
    "derived_from":"OpenRouter's supported parameters (tools or tool_choice)"},
   {"key":"parameters","label":"Parameters (B)","resolve":"first_stated",
    "value_type":"number","unit":"billions", ..., "kind":"measured","sources":["huggingface","research"]},
   {"key":"added","label":"Added", ..., "kind":"stamped","sources":["app"]},
   {"key":"notes","label":"Note","resolve":"first_stated","value_type":"text", ...,
    "kind":"yours","sources":["hand"]}]

- "key" is the identity, and it never changes, and it never carries a dot: one
  column now holds every source's answer to one fact, as separate cells, so a
  key never says who states it. GET /api/models/{name}/answers/{key} (below)
  is where a caller sees every source's own answer for one model.
- "kind" is one word for what the column IS: "fetched" (OpenRouter states it,
  and a repository may where OpenRouter says nothing), "measured" (a
  HuggingFace repository is read for it), "derived" (the app works it out from
  an OpenRouter fact, named in "derived_from"), "stamped" (the app writes it
  once, when the model enters the catalog: added), or "yours" (only you write
  it, in the page or through a write here).
- "sources" is who may state the column, in the order a read prefers them,
  and it is fixed in the code: no request changes it. The first automatic one
  is the preferred one; "research" last means an empty cell may be filled in
  and what is typed is kept until an automatic source states the fact. A
  "yours" column lists "hand" alone; "added" lists "app" alone.
- "label" is what a person sees, and it can be changed freely.
- "value_type" is text, number, flag, choice or list, and it decides how the
  column sorts and which filters it takes. "options" lists a choice column's
  labels, in the order they sort in.
- "resolve" says how several sources' cells for this column become the one
  answer a read carries: "first_stated" (the usual case) takes the highest-
  ranked source that has answered; "union" (input_modalities and
  output_modalities) merges every source's list together instead.
- "visible" says whether the browser's table draws it; "sealed" marks a column
  this app ships, which can be relabelled, moved and hidden but never deleted or
  re-typed.
- "color_rules" is what colour the browser paints the column's cells, and is
  absent from a column that is not coloured.

COLOURING ANY COLUMN, AND THE SHAPE PER KIND OF VALUE. The rule is a property of
the column, so a column you invented shades exactly as one this app ships. Write
it with PATCH, and clear it by sending an empty object:

  # a number: ordered thresholds, smallest first, and one colour for the rest
  curl -X PATCH http://127.0.0.1:8122/api/columns \
    -H 'Content-Type: application/json' \
    -d '{"key":"context_length","color_rules":
         {"thresholds":[{"below":32000,"color":"#ef4444"},{"below":200000,"color":"#eab308"}],
          "otherwise":"#22c55e"}}'

  # a yes or no: one colour each
  curl -X PATCH http://127.0.0.1:8122/api/columns \
    -H 'Content-Type: application/json' \
    -d '{"key":"favorite","color_rules":{"yes":"#22c55e","no":"#ef4444"}}'

  # one of a list: a colour per label
  curl -X PATCH http://127.0.0.1:8122/api/columns \
    -H 'Content-Type: application/json' \
    -d '{"key":"rating","color_rules":{"options":{"1":"#ef4444","4":"#22c55e"}}}'

  # no colours at all
  curl -X PATCH http://127.0.0.1:8122/api/columns \
    -H 'Content-Type: application/json' -d '{"key":"rating","color_rules":{}}'

- A THRESHOLD IS READ AGAINST THE FIGURE THE CELL SHOWS. That is the stored
  figure everywhere except a price, which is stored per token and shown per
  million - so the price thresholds are written per million, as the table reads
  them.
- A TEXT AND A LIST COLUMN TAKE NO COLOURS and answer 400: there is no order to
  shade along and no fixed set of values to colour one by one.
- A colour is a hex code, "#22c55e" or "#2c5", and nothing else. A field
  belonging to another kind of column, thresholds out of order, and an option no
  such column offers each answer 400 naming what is wrong.
- The two price columns SHIP with thresholds - under a dollar green, under two
  yellow, above that red. They are ordinary rules: change or clear them like
  any other.
- A COLUMN'S KIND OF VALUE IS FIXED WHEN THE COLUMN IS MADE. No route changes
  it afterwards - PATCH takes a heading and colours and nothing else - and a
  colour rule is written against that kind, so choose the kind at POST time.

### GET /api/columns/values

The distinct values one column really holds, with how many models hold each.
It is what the browser's value-list filters are built from, and it is computed
over the WHOLE catalog - hidden rows and variations included - never over the
rows a filter is currently showing.

  curl 'http://127.0.0.1:8122/api/columns/values?key=input_modalities'
  # {"key":"input_modalities","distinct_total":3,"empty":12,"truncated":false,
  #  "values":[{"value":"text","models":611},{"value":"image","models":302},
  #            {"value":"audio","models":41}]}

Three parameters, and only ?key= is required:

  key       the column, by its key. A key naming no column answers 400 and
            offers the nearest ones; a missing key answers 400 too.
  contains  keep only the values matching this text (capitals ignored). It takes
            the SEARCH SYNTAX of /api/models: words, "phrases", !not, AND, OR.
  limit     how many values to return. Without it the answer is capped at 200.

- A LIST column is split into its members first, so a column holding
  ["text","image"] offers text and image rather than the JSON of the pair, and
  a model is counted once per member it holds.
- A CHOICE column answers its declared options in their declared order, even
  where no model holds one (its count is then 0), because the order of a
  choice column is a scale and losing it would be wrong.
- Every other column answers its stored values, most-held first, ties A to Z.
- "empty" is how many models hold no value in the column at all, over the
  whole catalog like the counts beside it and never narrowed by contains=. It
  is the number of models ?<key>=any: returns, which is the empty choice of a
  filter.
- "distinct_total" is the count after any contains= narrowing and BEFORE the
  limit, and "truncated" says whether "values" came back short - which is how
  a caller can tell a short list from a short column.

### GET /api/columns/answers

How many models have answered each column, and how many models the catalog
holds at all. Like the values above it is computed over the WHOLE catalog -
hidden rows and variations included - and never over the rows a filter is
showing. "top_level" is how many of those models the page draws as rows of
their own: every model except a variation filed under a parent that is here.

  curl 'http://127.0.0.1:8122/api/columns/answers'
  # {"models":626,"top_level":620,"answers":{"context_length":611,
  #                          "notes":12,"favorite":3}}

It takes no parameters. Every column this catalog holds has an entry, including
one no model has answered, whose count is 0.

### POST /api/save

Writes cells onto models that are ALREADY HERE. The body is a JSON ARRAY of
objects, each naming one model and the cells to change:

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"OpenAI: GPT-5","cells":{"notes":"my note","favorite":"yes"}}]'

Every key inside "cells" is a COLUMN KEY, the same key a read answers under, and
every value is TEXT. A value read out of GET /api/models can therefore be posted
straight back with nothing looked up in between.

THE ANSWER SAYS WHAT HAPPENED TO EVERY ITEM:

  {"ok":true,"records":1,
   "results":[{"name":"OpenAI: GPT-5","written":true}]}

"records" counts the items written. "results" carries one entry per item, in the
order they were sent: "written" says whether that item landed, and "error"
carries the sentence saying what to correct on an item that was refused.

  200  every item was written.
  207  some were written and some refused - read "results" to see which.
  404  NOTHING was written, and the first item names a model that is not here.
  400  NOTHING was written, and the first item carries a key an automatic
       source already holds the answer for on that model, a key naming no
       column, or a value its column will not hold. See "Writing model data".

AN ITEM WITH NO CELLS IS STILL A WRITE. Send "cells" empty, or leave it out, and
the model's updated_at is stamped and the item answers "written":true with no
value changed - which is what every other path that touches a row does too. An
EMPTY ARRAY answers 200 with "records":0 and no results.

TWO ITEMS NAMING ONE MODEL ARE TWO SEPARATE WRITES, applied in the order sent,
each committed on its own. The later item's cells win, and the earlier item's
write stands even when the later one is refused.

A save NEVER creates a model: an item naming a model this database has never seen
is refused, and POST /api/models/add is how a model is added. A key the request
shape does not carry - anything beside "name" and "cells" - answers 400 naming
it, so a misspelt key fails loudly rather than writing nothing.

### POST /api/models/add

Adds one model by hand. The body is a SINGLE OBJECT - not an array - naming the
model and carrying the same "cells" object a save takes:

  curl -X POST http://127.0.0.1:8122/api/models/add \
    -H 'Content-Type: application/json' \
    -d '{"name":"My Local Build","cells":{"notes":"runs on my own machine","parameters":"27"}}'

Answers 201 {"ok":true,"name":"...","similar":[...]}. A body with no "name"
answers 400, and so does a key naming no column, a key nothing typed may reach
(added), or a value its column will not hold - and NO ROW IS CREATED in either
case, so nothing is left behind under that name.

"similar" is an ADVISORY, never a refusal: rows already in this catalog whose
name looks close to the one just created, in the GET /api/models/similar shape,
excluding the row just made. It is always present, empty where nothing is
close - a nudge to check for a near-duplicate the 409 below cannot catch,
because that check matches a repository, an id or an EXACT name and nothing
looser (a looser match there would name a row a write could not address).

EVERY FIELD CAN BE FILLED HERE. The row carries no listing and no repository,
so nothing automatic has stated anything about it: what you type into a fetched
or measured column lands as RESEARCH, and if the model ever appears on
OpenRouter or its repository is read, what they state is shown instead. What
you type into a column of your own lands as "hand".

ONE refusal, 409, and it means NOTHING was written: this catalog already holds
that model (use POST /api/save to write its cells). Already holds it is decided
by the repository in "repository" first, then the OpenRouter identifier in
"id", then the name - in that order - so giving a new display name to a
repository or an id that is already recorded does not create a second row for
it. EACH IS MATCHED HOWEVER ANY SOURCE STATED IT, not only against another row's
own researched cell: a repository is compared against every source's own cell
in that column alike. The refusal names the row that holds it and which of the
three matched.

A KEY OR VALUE THIS PATH WILL NOT TAKE IS ANSWERED FIRST: a request that both
names a model already here and carries a key naming no column answers 400,
never 409.

The row it creates is source "manual" - nothing re-creates it, so it is a row
you can delete outright whenever you want it gone.
ANY COLUMN OF YOUR OWN CAN BE FILLED IN THIS ONE REQUEST, as on any model:
everything you write is stored as your own answer, which no source and no refresh
can reach.

For a model that IS on HuggingFace, POST /api/models/import below is the better
verb: it reads those columns off the repository, so nobody has to type a number
that can be read instead.

The name is trimmed and runs of whitespace inside it are collapsed to one space,
here and at POST /api/models/import. No name already stored is ever rewritten by
a request: this API has no rename. The one thing that changes a stored name is
the refresh (POST /api/update), when OpenRouter renames a model it already lists
under another name: the row holding that OpenRouter id takes the new name, with
every value, note and rating, and the old name stops answering. A pair of
OpenRouter rows holding one id is folded into the row the feed names. Rows from
any other route are never renamed or merged.

THAT TIDYING HAPPENS ONLY WHERE A ROW IS CREATED. Every other route addresses a
model by a name that already exists, and uses it EXACTLY as you send it, space
for space: models.name is the primary key, and a name the OpenRouter catalog
stored can end in a space ("Google: Gemma 4 26B A4B " is a real one). Send the
name a read returned, unaltered, and a trailing space is not a mistake to
correct - it is part of which row you mean.

### GET /api/models/brief

What ONE model still has no answer for, and the instructions for filling it in.
?name=<model>, exactly as GET /api/models returns it; an unknown name answers 404.

  {"name":"Qwen: Qwen3-32B","hf_slug":"Qwen/Qwen3-32B",
   "held":[{"field":"context_length","label":"Context length","value":"40960",
            "source":"huggingface"}],
   "gaps":[{"field":"supports_tools","label":"Tool",
            "accepts":"one of \"yes\" or \"no\", as text in quotes",
            "current":"not set","about":"Your own answer on tool calling.",
            "measured":false},
           {"field":"parameters","label":"Parameters (B)",
            "accepts":"text in quotes, and it must read as a number (\"32.76\", not 32.76)",
            "current":"not set","about":"How many parameters the model has, in billions",
            "measured":true}],
   "example":"[{\"name\":\"Qwen: Qwen3-32B\", ...}]",
   "brief":"ModelsDB needs research on one model ..."}

"held" lists what the row already holds among the fields that can apply to it:
field, label, value (cut short when long) and source, the name of the source that
stated it. "gaps" omits every field that cannot describe this
row: an OpenRouter-only field on a row OpenRouter does not list, a field foreign
to the row's model_type, and a build or GGUF field on a row that holds none of
that family. Every gap carries its description in "about" and the
exact form it takes in "accepts" (dates as YYYY-MM-DD).

"gaps" is the whole answer in machine-readable form, and every "field" in it is
EXACTLY the key POST /api/save takes: a column of your own, sent inside "cells".
Nothing that write would refuse can appear. "label" is the heading that column is
shown under. "current" is what the row holds now, which is "not set" for a column nothing has
answered. "accepts" is the exact form that column takes, including the words a
closed set allows. "measured" marks a column whose values are READ off a
HuggingFace repository - fill those by re-importing rather than by typing.

A COLUMN IS A GAP ONLY WHERE NOTHING HAS ANSWERED THE FACT. Your own context
length, OpenRouter's and a repository's are three columns holding one question,
so a length any of them states is not something to go and research.

"brief" is that same answer as one block of text addressed to an assistant, and
"example" is the worked request inside it. EVERY VALUE IN THE EXAMPLE IS A
PLACEHOLDER of the form <...>, which is not JSON: sent as printed it is refused
with 400 and writes nothing, so running it can never record a fact nobody
established.

?origin=<base URL> names the address the caller reached this server at, which is
then the address every instruction in the text uses. Every address this instance
is really serving on is listed beside it, so a reader on another machine can pick
the one that reaches it. Without it the served addresses lead on their own.

The browser app shows this text in a model's Details dialog and after an import,
under "Copy agent brief". It writes none of it.

### POST /api/models/import

Adds one model from a HuggingFace address, with the SERVER reading every value
it stores. The body carries the address and NOTHING else:

  curl -X POST http://127.0.0.1:8122/api/models/import \
    -H 'Content-Type: application/json' \
    -d '{"url":"https://huggingface.co/Qwen/Qwen3-32B"}'

The full page URL, that URL with a trailing /tree/main or a query string, the
same address without its scheme, and the bare "Qwen/Qwen3-32B" are all accepted.
An address on another host, a /datasets/ or /spaces/ address, and anything
carrying no owner and model answer 400.

Answers 201 with what was read and what is still empty:

  {"ok":true,"name":"Qwen: Qwen3-32B","hf_slug":"Qwen/Qwen3-32B","model_type":"chat",
   "revision":"<the commit main pointed at>","context_length":40960,
   "size_basis":"the 17 file(s) named in model.safetensors.index.json",
   "read":{"author":"Qwen","parameters":"32.762123264","weights_size_gb":"65.52",
           "base_model":"...", ...},
   "kept":["repo_owner","parameters","weights_size_gb", ...],
   "missing":[{"field":"supports_tools","label":"Tool",
               "accepts":"one of \"yes\" or \"no\", as text in quotes",
               "current":"not set","about":"Your own answer on tool calling.",
               "measured":false}, ...],
   "similar":[...]}

"read" is everything the repository states, keyed by the fact keys
GET /api/import-fields lists, at the precision the repository states it - the
two parameter counts and the weights size are STORED at two decimal places, so
what the column holds afterwards is 32.76 and 65.52. "kept" is what this import
really recorded, named by COLUMN KEY (author is recorded in repo_owner), the keys
a read answers and a save takes.
"read" keeps the fact keys. A model that is a GGUF repository states its
parameter count (gguf.total) and its gguf_architecture, every import records the
base_model the card or its tags name, and an embedding model's context length is
the max_seq_length of its sentence_bert_config.json when it has one, else the
window its card states, not the position table of the model beneath it. A repository whose tags or card describe
its output as a score or probability for each option is typed decisions and gives
out ["decisions"], with "text" as well when its task outputs text. A fact this
repository did not state is read as nothing and recorded as nothing, so it is in
neither list.

"similar" is the same ADVISORY POST /api/models/add answers: rows already in
this catalog whose name looks close to the one just created or re-imported, in
the GET /api/models/similar shape, excluding that row itself. Always present,
empty where nothing is close.

THERE IS NO WAY TO ASK FOR A SUBSET, and the body has no "fields" list. EVERY
fact the repository states is recorded, in a column that belongs to HuggingFace -
one column per source, per fact, always. What you choose is which columns the
table DRAWS, not which are written: a fact you never look at costs one cell and
is there the day you want it. Nothing an import writes can reach a column of
yours, so your own answer to the same question is untouched whatever the
repository says.

THE REQUEST CANNOT SET WHAT IS STORED. There is no field for a name, a size or a
capability: the name of a row this CREATES is built from the repository id as
"Owner: Model" - a row that already holds the repository keeps the name it has - and
its author, description, context length, modalities, parameter count and on-disk
size are read from HuggingFace at that moment, and every one of them is recorded
under HuggingFace's own name for it. The row is source "huggingface", so nothing
upstream re-creates it.

"missing" is what the ROW still has no answer for once the import has written -
read back off the row itself rather than listed from what this importer knows it
cannot fill. It is the same shape GET /api/models/brief returns in "gaps", one
object per field, and every "field" in it is a key POST /api/save takes on this
model. The browser app calls this from its "Add model" control and shows that
same list on the page, where each of those fields can also be filled in as
research - so nobody needs an agent to finish a model.

The "Weights size (GB)" figure is the size of the weight files the shard index
names, never the sum of every weight file in the repository - a repository ships
copies of its own weights, and summing them all gives two or three times the
real figure.

ONE refusal, 409, and it means nothing was written: this catalog already holds
that model.

ALREADY HOLDS IT IS A QUESTION ABOUT THE REPOSITORY, not about the name this
import would invent. The row is looked up by "hf_slug" first, then by the
OpenRouter "id", then by name, and the refusal names the row that holds it:

  This catalog already holds that model, as "inclusionAI: Ling 3.0 Tiny (free)",
  which carries the HuggingFace repository "inclusionAI/Ling-3.0-tiny", so
  nothing was imported.

That row is usually stored under OpenRouter's name for the model rather than the
repository's, which is exactly why the name alone is not the question: importing
onto a second row is how one model's note, star and column values end up split
across two rows that never mention each other.

READING A MODEL THAT IS ALREADY HERE AGAIN is what the first of those refusals
points at. Send "reimport": true and the repository is read again and what the
importer MEASURES is rewritten ONTO THE ROW THAT ALREADY HOLDS IT, whatever that
row is called - the answer's "name" says which row was written. It answers 200
with "reimported": true and the same read/kept/missing body:

  curl -X POST http://127.0.0.1:8122/api/models/import \
    -H 'Content-Type: application/json' \
    -d '{"url":"Qwen/Qwen3-32B","reimport":true}'

IT IS THE ONLY WAY TO REFRESH THE MEASURED FIGURES OF A MODEL ADDED EARLIER. They
are measurements, and the update procedure forbids anybody to type one, so they
can come from an import and from nowhere else.

WHAT IT REWRITES: HuggingFace's own columns for everything the repository states
- the repository address, the description, the modality lists, the context
length, the parameter count and the size.

WHAT IT NEVER TOUCHES: anything of YOURS. Your notes, your favorite flag,
whether the row is hidden, and every column you made yourself. That is by
construction rather than by a guard: an import writes as huggingface, into that
source's own cell, never into hand's. A value researched into a column the
repository measures is replaced where the repository states one and kept where
it does not: it is its own cell beneath the measurement, outranked rather than
overwritten.

It is NOT the default, so a repeated import with no "reimport" still answers 409
rather than quietly overwriting a row.

A GATED repository - one whose owner requires an account - answers 400 and
imports NOTHING, exactly as POST /api/models/variations refuses one: its
metadata reads fine and its files do not, so the row would be an empty shell. An
unreachable HuggingFace answers 502 and writes nothing either.

THE ARCHITECTURE IS NOT RECORDED BY THE IMPORT. How a model is built - its
layers, what holds a cache, its heads, how its weights are stored - is one line
of facts in the Architecture column (key architecture_basis), which is a column of
yours that a person or assistant fills in after reading the repository's
config.json. The update procedure, section 6a, says how to write it.

AN IMPORT WRITES ONLY AS HUGGINGFACE. A column it fills can hold a researched
answer too, beneath the import's, and a plain read carries the measurement
wherever there is one - see GET /api/models/{name}/answers/{column} for both at
once. Nothing an import writes can reach your own columns.

### GET /api/import-fields

Every fact this app can record about a model, whichever source states it, and the
column of your own that answers the same question:

  [{"key":"parameters","column":"Parameters (B)","type":"number",
    "about":"How many parameters the model has, in billions",
    "sources":["huggingface"],"exists":false},
   {"key":"tokenizer","column":"Tokenizer","type":"text",
    "about":"Which tokenizer family the model uses, as OpenRouter classifies it",
    "sources":["openrouter"],"exists":true}, ...]

"key" names the fact. "column" is the heading a column of YOUR OWN carries for
it. "sources" names the places that can state it: "openrouter" for a fact the
catalog refresh reads, "huggingface" for one a repository states, both where the
two both state it - and each of those places records its answer in a column of
its own, so two sources never write one column. "exists" says whether you keep a
column of your own for that fact in this database now.

There is nothing to tick: every source records every fact it states, on every
read, whether or not you keep a column beside it. What you choose is which columns
the table DRAWS and which columns of your own exist - never which facts are
recorded.

THE REFRESH CREATES NONE OF YOUR COLUMNS. A fact OpenRouter states lands in an
openrouter column and one a repository states lands in a huggingface column, and
neither can reach a column of yours. So a column you delete stays deleted, with
nothing to untick and nothing to re-tick - see DELETE /api/columns.

### POST /api/models/delete

{"name":"...","confirm_delete":true} permanently deletes a model row.

A MODEL WHOSE "source" IS "openrouter" CANNOT BE DELETED and answers 409. A
refresh stores every model OpenRouter lists, so the row would be written again
on the next run; POST /api/models/hide is what takes it out of view. Every other
row is deletable, whatever its source.

WARNING: this destroys the model's note, its favorite flag, and its value in every
column of your own, with no undo and no backup but a copy of the database file. The
request is REFUSED with 400 unless the body carries "confirm_delete": true, and
that flag is read from the BODY only, never from the query string. An unknown name
answers 404.

### POST /api/models/hide

{"name":"...","hidden":true} hides a model from the default list;
{"hidden":false} shows it again. The row and everything recorded about it are
untouched either way - only who sees it changes. Answers
{"ok":true,"name":...,"hidden":...}; an unknown name answers 404.

This is the ONLY writer of the hidden flag. POST /api/save cannot set it.

### GET, POST /api/models/variations

A model's VARIATIONS - the quantized copies of it published on
HuggingFace, which you download and run yourself rather than call as a service.
See "Variations" below for what one is and how they are stored.

  GET  -> ?name=<model> lists the HuggingFace repositories that publish variations of
          that model: {"name","hf_slug","repos":[{repo,likes,downloads,gated,
          format,updated}],"truncated"}. "truncated" is true when more
          repositories existed than were read, so a full list is never implied.
          A model with no HuggingFace repository recorded answers 400, and an
          unknown name answers 404.
  POST <- {"name":"<model>","repo":"<owner/repo>","build":"<build id>"} records
          one build as a row of its own. The build id comes from
          GET /api/models/variations/files. Answers
          {"ok":true,"name":...,"variant_of":...,"format":...,"bit":...,"bytes":...}.
          The body takes those three keys and no others: every fact the
          repository states is recorded, in a huggingface column, always.

THE REQUEST CANNOT SET WHAT IS STORED. The server re-reads the repository and
records the size, format and bit width IT read, so those values are always
measurements. A build id the repository does not hold answers 400.

WHAT IT MEASURES FILLS SEALED, MEASURED COLUMNS: "Build format", "Bits per
weight", "Build revision", "Build files", "Build size (bytes)" and "Base model".
Each is a column, so the table sorts it and a filter reads it, and each is yours
to hide or relabel, never to delete or type over.

A GATED repository - one whose owner requires an account - answers 400 and
records NOTHING. Its metadata reads fine and its files do not, so a row for it
would look available while being impossible to download.

A MODEL THAT IS ITSELF A VARIATION answers 400 and records nothing. A variation
is drawn as a child row of its parent and a child row has no children of its
own, so a build hung off one would sit in the database and be drawn nowhere.
Record it under the model it was built from instead. POST
/api/models/variations/existing refuses the same relation.

### GET /api/models/variations/files

?repo=<owner/repo> lists what one repository holds, with each build's exact size:
{"repo","revision","gated","author","base_model","gguf",
"builds":[{id,label,format,bit,bytes,files,projector}]}.

"revision" is the commit its main branch pointed at when it was read, because
main moves. "id" is what POST /api/models/variations takes as "build". A GGUF
repository holds one file per quantization, so it offers many builds; every other
kind holds one build spread over shards, so it offers one. "projector": true
marks a vision projector, which is loaded ALONGSIDE a quantization rather than
being one.

### POST /api/models/variations/existing

Files a model ALREADY IN THIS CATALOG under another one, as a variation of it.
The other way a variation comes to exist: the route above CREATES one from a
HuggingFace repository it reads, and this one MOVES a row that is already here -
a build somebody added as a model of its own, by hand or from a HuggingFace
address, which belongs under the model it was made from.

  POST <- {"name":"<the model to file>","parent":"<the model to file it under>"}
          Answers {"ok":true,"name":...,"variant_of":...,"hidden":true}.

NOTHING IS FETCHED AND NOTHING IS MEASURED. The row keeps every value it holds -
its note, its favourite star, every value in a column of your own, its size, its
format, its prices - and gains one fact, that it is a variation of the parent.
Every column of your own on it is left exactly as it is, which is why this is a different
route from the importer above rather than another shape of its body.

THE ROW BECOMES HIDDEN, like every variation, so it leaves the model list and the
default GET /api/models answer. Pass include_hidden=1 to see it, or clear the
flag with POST /api/models/hide.

An unknown model on either side answers 404. Four relations answer 400 and change
nothing, because a variation is drawn as a child row of its parent and that has
to be one level deep and acyclic: a model filed under itself, a model filed under
one of its own variations, a model filed under a row that is itself a variation,
and a model that HAS variations of its own (they would be left with nowhere to
sit - move those out first).

### POST /api/models/variations/unfile

Takes one row back out from under the model it is filed under, so it is a model
of its own again. It is the way back from the two routes above, and it is the
only thing in this app that ever clears variant_of.

  POST <- {"name":"<the variation to unfile>"}
          Answers {"ok":true,"name":...,"variant_of":""}.

IT CHANGES ONE FACT AND NOTHING ELSE. The row keeps every cell, its note, its
favourite star, every custom-column value and the date it was added. hidden is
NOT touched: whether a row is on the list is its own decision, with its own
route (POST /api/models/hide), so a row filed by mistake comes back out still
hidden and is shown again by the verb that shows a row.

WHY IT MATTERS MORE THAN IT LOOKS: filing a row is one request, and a row the
OpenRouter feed lists cannot be deleted (see POST /api/models/delete). Without
this route a model filed under the wrong parent would be a hidden child of
another model for ever.

An unknown model answers 404. Two cases answer 400 and change nothing: a row
that is not filed under anything, and a row that HAS variations filed under it
(they would be left with nowhere to sit - unfile those first).

### GET, POST /api/settings

The browser app's own settings. It is BROWSER-OWNED and not meant for API
callers.

  GET  -> every stored setting, one JSON object, or {} on a fresh install.
  POST -> {"set": {"view.v1.page_length": 50}, "delete": ["auto_update"]}.
          Answers {"ok":true}.

A WRITE NAMES WHAT IT REMOVES. Everything in "set" is stored, everything in
"delete" is removed, and every other stored key is left exactly as it was. Both
members are optional, so {"set":{...}} alone and {"delete":[...]} alone are both
whole requests, and a caller changing one preference sends that one preference.
Deleting is the only way to remove a key: a "set" value of null stores null.

The stored keys are of two kinds. EVERYTHING ABOUT THE TABLE BELONGS TO A VIEW,
one of the reader's named arrangements, and is stored as "view.<id>.<key>" where
the id is a "v" and digits: view.v1.name and view.v1.created say which
arrangement it is, view.v1.position (a number, written for every view at once
the first time the reader moves one) says where it stands in the list, which
puts views with a position first and the rest after them, oldest first, and
view.v1.filters, view.v1.f_show_hidden,
view.v1.f_variations, view.v1.col_order, view.v1.col_hidden, view.v1.col_known,
view.v1.col_filter_off, view.v1.page_length, view.v1.hide_empty, view.v1.show_marks, view.v1.full_text,
view.v1.cards_below and view.v1.order hold the filter-bar
state, the column order, which columns are shown, the page size and the sort. A
set of view rows with no name row is not a view; the page removes it. WHICH VIEW
IS ON SCREEN IS NOT STORED HERE at all: it belongs to the browser, per tab and
per device.

The rest belong to the app rather than to any one arrangement: filters_open (is
the filter panel docked open), the "download updates automatically" preference
(auto_update), the switch that updates the model catalog from OpenRouter once a
day while the server runs (daily_model_update, bool, on until stored as false; browser-
owned like the rest, see POST /api/update), the Columns dialog's toolbar
filters (columns_dialog_filters, one object the page writes and reads back; a
value of any other shape is ignored), and the daily backup schedule the server itself reads,
backup_enabled (bool), backup_dir (string), backup_overwrite (bool).

Refused with 400, storing nothing at all:

  - A body of any other shape: a flat object of settings, a top-level member
    other than "set" and "delete", a "set" that is not an object, a "delete"
    that is not an array of strings, or a body over 4 MiB.
  - A key beginning "server.", in either member. Those belong to the server: they
    are RETURNED by GET and never written here.
  - The same key in both members, and the empty key.
  - A backup key in "delete", or a backup value of the wrong type.

The three backup keys are the one place a missing key keeps its stored value
rather than being cleared, because they name a folder on this machine and losing
your daily backup to an omission is not a recoverable mistake. They have no
delete for the same reason.

GET answers 500 when the settings cannot be read. It never answers {} to cover a
failure: the page stops saving until a read succeeds, so that an arrangement it
could not read is never overwritten with defaults.

### POST /api/settings/ack-notice

Dismisses the one-time notice that this version did not carry the old settings
file across, by deleting the "server.notice.settings_reset" key GET
/api/settings returns. No body. Answers {"ok":true}, and answers the same on a
notice that was already dismissed.

### GET /api/health

{"app":"modelsdb","status":"ok","pid":...,"exe":"...","port":...,"data":"<data
dir>","version":"...","addresses":["http://127.0.0.1:8122",...]}. The cheapest
way to prove a server is answering, and WHICH ONE: the reply names the process,
so a caller can act on it without trusting the PID file. A server that has taken
its port but has not finished starting answers the same block with
"status":"starting" and HTTP 503, so a poller waiting for it to come up reads
that as "not yet".

"addresses" lists every base URL this instance is really serving on - the
listeners that BOUND, so an address here is one that answers. Loopback comes
first; a second entry (a Tailscale or LAN address) means the server is reachable
from other machines too. Prefix any path in this guide with whichever entry fits
where you are running: the loopback one on this machine, another from elsewhere.

### GET /api/paths

Where this instance keeps its files, as resolved at startup: {"executable","port",
"version","dirs":{"config","data","cache"},"files":{"database","config.jsonc",
"backups","logs","pid","update-check"}}. Read-only; no secrets.

### POST /api/update

Starts a full DATA refresh in the background and returns at once: the OpenRouter
catalog fetch. Answers 202 {"started":true}, or 409 {"started":false,"running":
true} when one is already running. Watch it with GET /api/update/status. This
refreshes model data; it does not touch the program.

A 409 can come from the DAILY UPDATE. With the browser's daily_model_update
switch on, which is its state until it is stored as false (see POST /api/settings), the server starts this same refresh itself
once a day: it checks every hour, and a run is due a day after the last run
finished, or at the next check after a run that failed. It goes
through the same single-flight guard, so a run it started answers this route
with 409 until it finishes, and GET /api/update/status then reports
"scheduled":true. A scheduled run sends its requests one at a time with a
pause of about a second between them, so it takes around 25 minutes where this
route's run takes about two; the 409 lasts that long. A run started by this route is never "scheduled".

THIS REFRESH STORES EVERY MODEL THE FEED LISTS. A listing this catalog does not
hold is created as a row of its own, VISIBLE and not a variation, by the same
write that refreshes a model already here. It is the only way an OpenRouter
model reaches this catalog. How many rows it created is answered as "added" on
GET /api/update/status.

THE CATALOG FETCH RECORDS EVERYTHING OPENROUTER STATES - the author, the
tokenizer, the prompt template, the request parameters, the longest answer, the
moderation flag, the knowledge cutoff, the expiry, whether reasoning is
mandatory, the alias, and the cache, image, audio and web-search prices - one
column per fact, every one of them OPENROUTER'S OWN column. It creates no column
of yours and fills none: the columns it makes below are the app's, sealed like
every shipped one. GET /api/import-fields lists what it can record, and it
records all of it on every run.

THE CATALOG FETCH ALSO MAKES A COLUMN FOR EVERY LEAF PATH THE FEED CARRIES that
no shipped column reads yet, and fills it in that same refresh. Nobody imports
anything. The column's key is the whole path, lowercased, with "[]" removed and
every dot an underscore ("top_provider.context_length" is the column
top_provider_context_length); its heading is those words with the first
capitalised; its kind of value is what the feed sends there, and a path every
model quotes as a numeric string (every price is one) is a number column. A
LIST OF OBJECTS in the feed - a price window under pricing.overrides, a
leaderboard row under benchmarks.design_arena - is one such path in its own
right, never walked key by key: the whole array is ONE list column
(pricing_overrides, benchmarks_design_arena), and each element is one member,
its own keys joined as "key=value" pairs in the order the feed sent them
("prompt=0.00000156, completion=0.0000078, min_prompt_tokens=32000"), a
missing key simply absent. It arrives HIDDEN, so no view changes until it is
ticked in the Columns dialog, where it is found by its heading, its key or the
feed's own path ("feed_paths" on GET /api/columns). It is SEALED, exactly like
a shipped column: relabel, move and hide it, never delete or re-type it,
because the next refresh would make it again. It is sourced openrouter then
research, exactly like a shipped OpenRouter column: the feed fills it, and
where the feed says nothing an empty cell may be filled in as research. A path
a shipped column already reads gets no column; nor does a path with no value
on any model, until some model states one; nor does a path that is both a leaf
and a branch of other paths (only its leaves do). A value the feed stops
sending leaves the last cell standing. A path the feed adds or drops since the
previous refresh is reported back as a "warnings" entry on GET
/api/update/status.

A REFRESH NEVER REPLACES A VALUE YOU WROTE, and nothing has to enforce that. A
column belongs to whoever writes it: a refresh writes as OpenRouter and cannot
name a column of yours at all, an import writes as HuggingFace, and what you type
goes in a column of your own. Two sources stating the same thing are two columns
on purpose, and nothing reconciles them. Your answer is the one that shows,
whatever any source says; clear it (send "" for that field) and the source's
answer shows through again, which is how you ask for upstream's figure back.

### GET /api/update/status

{"running":bool,"phase":"idle|catalog","started_at":...,
"finished_at":...,"errors":[...],"warnings":[...],"added":N,"scheduled":bool,
"last_run":{...}}. A failure records an error rather than aborting, so "errors"
can be non-empty on a finished run.

"scheduled" is true while the running refresh was started by the daily update
rather than by POST /api/update.

"last_run" is the record of the last refresh that finished, however it was
started (server start, POST /api/update, the daily update), and it outlasts a
restart: {"seq":N,"finished_at":...,"outcome":"","error":"","failed_parts":[],
"scheduled":bool}. "seq" counts finished runs and rises by one for each.
"outcome" is "" before any run has finished, then "ok", "partial" (the feed was
stored but a pass that follows it did not do its job, and "failed_parts" names
each: "zero data retention", "EU routing", "throughput", "video and image
lists") or "failed" (the feed could not be stored, and "error" says why).

"added" is how many rows the refresh CREATED - models the feed lists that this
catalog did not hold. It is reset to 0 when a refresh starts and stands until
the next one.

"warnings" is not a failure: it names a feed field that appeared or vanished
since the previous refresh - see POST /api/update.

### GET /api/update-check

The cached result of the notify-only check for a newer ModelsDB release:
{"current","latest","url","available","checked_at"}. Nothing is downloaded.
Add ?force=1 to ask GitHub right now, bypassing the once-a-day throttle; that
form is synchronous, so the answer reflects this instant.

### POST /api/app-update/apply

Starts staging a newer PROGRAM binary in the background (download, verify
signature and hash, swap in place). Answers 202 {"started":bool,"latest":"..."};
400 when self-update is unsupported on this platform or no newer release exists.
Nothing is applied until an explicit restart. THIS REPLACES THE PROGRAM - it is
not POST /api/update, which refreshes model data.

### GET /api/app-update/status

{"state":"idle|downloading|verifying|staged|failed","error":"...","latest":"...",
"supported":bool,"updatable":bool}. "supported" means the platform can self-update
at all; "updatable" additionally requires a real release signing key and a
non-development build. A refused update ends in state "failed" with the reason in
"error": a bad signature, or a signed release version that is not newer than the
running one or differs from the announced "latest". The version is judged from the
signed checksums, never from the release tag.

### POST /api/app-update/restart

Re-launches the process so a staged binary takes effect. Answers
{"restarting":true}, or 409 {"restarting":false,"error":"no update is staged"}.

WARNING: this ENDS THIS SERVER PROCESS a fraction of a second after replying. Any
request in flight dies with it, and the answer to this one is flushed first
precisely because the reply would otherwise never arrive. Poll GET /api/health
until the new process answers.

### POST /api/backup/validate

{"dir":"..."} reports whether a folder can be used as a backup destination -
it exists, it is a folder, and it is really writable (proven by writing a
temporary file and removing it). Answers {"ok":true} or
{"ok":false,"error":"..."}. It persists NOTHING and changes no setting.

### POST /api/backup/run

{"dir":"...","overwrite":bool} takes a backup immediately: one self-contained
.db copy of the whole database, written into the destination folder saved in the
settings. "dir" must be that saved folder: a request naming any other folder, or
made while none is saved, answers {"ok":false,"error":"..."} and writes nothing,
so save the folder first. Answers {"ok":true,"path":"...","bytes":N}, or
{"ok":false,"error":"..."} when the destination is unusable or the copy fails.
It is not throttled. overwrite=true keeps one file that is replaced each time;
false writes a new timestamped file.

Taking a backup only READS the live database, and it writes only the one file it
was asked to write - nothing in your folder is listed, pruned, or deleted.

### GET /api/backup/status

One combined read for the daily backup: {"enabled","dir","overwrite",
"last_backup_at","last_attempt_at","last_scheduled_at","last_outcome",
"last_error","listing"}.
"last_outcome" is "" (never attempted), ok, unchanged, "invalid destination", or
error. "last_attempt_at" is when a backup was last tried by ANY route, the daily
schedule's and the Back up now button's alike; "last_scheduled_at" is when the
schedule itself last ran, which is what its 24-hour gate reads, so a backup taken
by hand never postpones the next automatic one. "listing" is
{"state","files":[{"name","size","modified"}]}, where state is none (no folder
configured), missing (configured but gone), empty, or listed.

## Writing model data

Two endpoints write model data, and they take the same shape - a "cells" object
keyed by COLUMN KEY:

  POST /api/save         a JSON ARRAY, one object per model. Writes to models
                         that already exist; refuses a name that is not here.
  POST /api/models/add   a SINGLE object. Creates the model; 409 for a name that
                         already exists.

POST /api/models/import creates a model too, and takes NO cells at all: it reads
a HuggingFace repository and stores what it read. Nothing below applies to it.

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"OpenAI: GPT-5","cells":{"notes":"my note","favorite":"yes"}}]'

"name" identifies the model and is required. A write cannot rename: writing a
different "name" writes a different model. (Only a refresh renames, when
OpenRouter renames a model; the old name then stops answering.)

### What a write may name

TWO KINDS OF COLUMN, AND ONE RULE DECIDED PER KEY. A write does not say who is
writing; the column and what is on file for that model decide, and THIS is the
rule a write here is held to - what an assistant sends. THE BROWSER PAGE HOLDS
ITSELF TO A NARROWER ONE: it never offers to type into an empty fetched,
measured or derived cell at all, only into one already holding a researched
value, or into a column of the reader's own; a brand-new row's fields, taken
through Add model, are the one page path that still lands as research. GET
/api/columns says which kind every column is ("kind") and who may state it
("sources"):

- A COLUMN OF YOUR OWN - "kind":"yours": the note, the favourite, the
  Architecture line, and every column you made - takes the write as source
  "hand". No update ever touches it. Only you write it, with one exception: the
  Architecture line (key architecture_basis) is also written by an assistant that
  has read the model's config.json, through this same route, as the update
  procedure's section 6a says.
- A FETCHED, MEASURED OR DERIVED COLUMN - one OpenRouter states, a repository
  measures, or the app works out - takes the write as source "research" WHERE
  IT IS EMPTY: a value nothing automatic has stated is not information from
  OpenRouter, so filling it in edits nothing fetched. What you write is kept,
  and shown, until OpenRouter or the repository states the fact; then theirs
  is shown and yours sits beneath it, dated, in the answers panel.
- THE SAME COLUMN WHERE AN AUTOMATIC SOURCE HAS STATED IT IS REFUSED, 400,
  naming the source and its value. An update maintains that cell, and nothing
  typed replaces it:

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"OpenAI: GPT-5","cells":{"context_length":"131072"}}]'
  # 400 Nothing was written: that column is not yours to write: Context length
  # holds OpenRouter's answer, 400000, which an update maintains; nothing
  # typed replaces it.

- "added" DECLARES NEITHER hand NOR research, so nothing typed reaches it; the
  refusal names the one source that states it.

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"OpenAI: GPT-5","cells":{"added":"2026-01-01T00:00:00Z"}}]'
  # 400 Nothing was written: that column is not yours to write: added is
  # stated only by "app", and nothing typed reaches it.

A KEY NAMING NO COLUMN IS REFUSED, 400, naming POST /api/columns: a column is
made there, by you, and never on the way past a write - so a typo cannot
become a column. A key carrying a dot names no column either, because who
states a cell is recorded on the cell itself, not in the key:
"openrouter.context_length" is never a key - "context_length" is.

A RESEARCHED VALUE REPLACES AN EARLIER ONE, and an empty value clears it, the
way a note is replaced or cleared. Send "" and the research cell
is deleted; where nothing automatic has answered the column then shows nothing
at all. AN EMPTY VALUE IS ACCEPTED EVEN WHERE AN AUTOMATIC SOURCE HAS STATED
THE FACT: it deletes the researched cell the source overtook, and nothing
else, so clearing what is no longer shown always works. There is no way to
hold a fact empty against a source that states it.

EVERY VALUE IS SENT AS TEXT, whatever the column holds. A number column takes the
figure as a quoted string ("0.00000013"), which is what keeps a small price
exactly as it was stated; a flag column takes "yes" or "no" (and accepts "true",
"false", "1" and "0"); a list column takes a JSON list inside a string
("[\"text\",\"image\"]"); a choice column takes one of its own options, exactly.

A few columns and the traps in them:

- notes is YOUR private overlay: quality observations, reminders, and the
  pricing lines the In and Out price columns cannot hold. It is never shared
  and never refreshed. Put a pricing line in it only for a price that is not
  already in those columns, and never the unit itself, which is price_unit.
- model_type is chat, image, video, embedding, rerank, tts, stt, music or decisions.
- price_prompt and price_completion are per-token figures. Research them only
  for a model no source prices - a model you run yourself, or one the API
  omits; on a listed model they are OpenRouter's and refused.
- THE PARAMETER COUNTS AND THE SIZE ARE MEASUREMENTS: parameters,
  active_parameters and weights_size_gb, both counts in BILLIONS. No repository
  states active_parameters, so it takes only a researched answer; a build is
  given its parent's. Whichever of a research save, a HuggingFace import or a
  variation writes them, they are STORED AT TWO DECIMAL PLACES -
  32.762123264 becomes 32.76, a whole number stays whole, a trailing zero goes
  (33.10 stores as 33.1), and a figure below one keeps both its decimals rather
  than becoming 0. Research one only where there is no repository to read.
- price_unit is the pricing unit shown under the In and Out prices ("per 1M
  tokens", "per second", "per image"). The app works it out from whether
  OpenRouter lists an input price; where it has not, research it.
- added is WHEN THE MODEL ENTERED THIS CATALOG, stamped once at the moment
  the row was created and never rewritten by anything automatic. Its only
  declared source is "app" itself, so no request can correct it, and once it
  is gone nothing can put it back.

### A value the column will not hold is refused

Every value is read against its column's own KIND before anything is written. A
value that kind will not take answers 400 and writes NOTHING - not that cell, and
not the other cells in the same item. The message names the column and what it
holds:

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"Qwen: Qwen3","cells":{"context_length":"quite long"}}]'
  # 400 Nothing was written: that column will not hold that value:
  # context_length holds a number, and "quite long" is not one.

THIS IS WHY THAT MATTERS: a save is an OVERLAY onto what the row already holds,
and an empty value CLEARS a cell - so a value that failed quietly would not fail,
it would delete a figure held in this database and nowhere else.

The same check runs on POST /api/models/add BEFORE the row is inserted, so a
refused add leaves nothing behind under that name.

### The row's origin decides nothing

The model's "source" in a GET /api/models answer - which route created the row
- decides nothing about what may be written, and nothing about how it is removed
either (see "Removing a model"). What decides a write is the column's kind and
what is on file for that model, cell by cell: a price on a listed model is
OpenRouter's and refused, a price on a model you added by hand is empty and
takes research, on the very same column.

### Several sources, one column

Context length, tool support, reasoning, prices and most of the rest of what
this catalog knows are facts more than one source can state. Each source's
answer is its OWN CELL in the SAME column - OpenRouter's, a repository's, a
researched one - so nothing is ever overwritten by another source's write.

A PLAIN READ RESOLVES THEM BY RANK, AND THE RANK IS FIXED IN THE CODE. Each
column declares its sources in a preferred order ("sources" on GET
/api/columns): OpenRouter first where OpenRouter serves the fact, the
repository first where it is read for the fact (parameters, MoE), and research
last on every column that accepts it. GET /api/models carries the
highest-ranked source's answer that has one - alongside two maps that say
where it came from: "answered_by" names the winning source for every column a
model has an answer in ("openrouter", "huggingface", "research", "hand",
"app", or "merged" for a union-resolve column an automatic source contributed
to; research alone there answers "research"), "disputed" flags a column where
OpenRouter and HuggingFace hold different text (never description or
created_at, which differ by nature), and "overtaken" flags a column
holding a researched value an automatic source has since stated over - still
stored, no longer shown, cleared with an empty value. Research never disputes:
it is simply outranked. See "Reading the catalog" below for the exact shape.

SO A RESEARCHED VALUE SHOWS ONLY UNTIL A SOURCE STATES THE FACT. Write 8192
where nothing has stated a context length and context_length reads 8192; the
day OpenRouter states 128000, context_length reads 128000, answered_by says
"openrouter", and your 8192 is still there, one rank down, at
GET /api/models/{name}/answers/context_length with the date you wrote it.

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"My Local Build","cells":{"context_length":"32768"}}]'
  # 200. A plain read then carries context_length "32768", answered_by
  # "research" for that column, until a source states it.

NOBODY ARRANGES THE ORDER. There is no route that changes which source a column
prefers; it is what the code says, reapplied on every start. A UNION-RESOLVE
column (input_modalities, output_modalities) has no order to speak of: a read
merges every source's list together, research included, instead of picking one.

### What is not a cell at all

Four things about a row are not facts about the model, so no write names them:

| What | Why not, and what does write it |
|---|---|
| name | The key itself. No request renames a row. A refresh renames an OpenRouter row when OpenRouter renames the model, and the old name then stops answering |
| source | The record of which route created this row, which is what decides how the row is removed. Nothing writes it by hand |
| hidden | POST /api/models/hide is its only writer, and it already has a control of its own in the page |
| variant_of | What files one row under another as a variation of it, rather than a fact about the model. POST /api/models/variations records a build read off a repository and POST /api/models/variations/existing files a model already here; POST /api/models/variations/unfile is the only thing that clears it again |

Anything else in the object - a key beside "name" and "cells" - answers 400
naming it.

### Two behaviours of a save worth knowing

1. ABSENT KEYS ARE PRESERVED, PRESENT KEYS WIN. A save overlays only the cells
   your object actually carries, so a partial save is safe: omitting notes
   keeps the note. The flip side is that a key present with an empty value is a
   real instruction to clear it.

2. A BATCH IS HONEST, ITEM BY ITEM. Each item is applied on its own, in one
   transaction, and the answer carries a result per item. So one bad item costs
   only itself: the others are written, and "results" says which. Within one
   item nothing is partial - a refused key leaves every cell of that model
   exactly as it was.

   WHY IT IS NOT ONE TRANSACTION OVER THE WHOLE BATCH: the items are independent,
   each naming a different model, and they arrive from an agent that researched
   several models in one pass. Discarding good research about six models because
   a seventh named a column that does not exist would lose work nothing else
   holds.

## Removing a model

/api/models/delete works on every model but one OpenRouter lists. It destroys
that model's personal data - its notes, its favorite flag and its value in every
column of your own - permanently, with no undo and no backup but a copy of the
database file. So it REFUSES a request whose body does not carry
"confirm_delete": true, and the flag is read from the BODY only, never from the
query string. To keep the model and everything recorded about it while taking
it out of the default list, use /api/models/hide instead.

AN OPENROUTER MODEL CANNOT BE DELETED AT ALL. A refresh stores every model the
feed lists, so the row would come straight back; the request answers 409 and
names /api/models/hide, which keeps the row and everything recorded about it and
only takes it out of the default list.

## Variations

Most rows here are hosted models you call as a service. A BUILD is different: one
quantized copy of a model, published as a file on a HuggingFace repository, that
you download and run yourself. Storing each weight in fewer bits makes the file
smaller, and which build you can run is decided by how much memory you have -
which is what the fields below are for.

A build is a MODEL ROW of its own, with three things that mark it out:

- variant_of holds the NAME of the model it was built from. It is an identity
  field rather than a column, and it is empty on every ordinary row, so it is
  also how you tell the two kinds apart.
- Its HuggingFace columns say how it is stored and where it came from:
  build_bits_per_weight (4, 8, 16), build_format
  ("GGUF Q4_K_M", "FP8"), build_revision, build_files and
  build_size_bytes. base_model names the repository it was built from.
- Its Architecture line (architecture_basis) is written by a person or assistant
  like any other row's; a GGUF repository publishes no config of its own, so it
  is read from the base model's.

The HuggingFace ones are HuggingFace cells - the importer re-reads the repository
rather than typing them - and a plain read answers each under its own bare
key, exactly like any other column.

**A build starts HIDDEN**, so it is left out of the default list exactly like any
other hidden model, and so a catalog of models does not fill up with files. That
is its starting state and not a permanent property: the reader of this database
may un-hide any row, including a build.

So there is no separate parameter for builds and there does not need to be:

  GET /api/models?include_hidden=0    models only, as always
  GET /api/models?include_hidden=1    models AND builds, each carrying variant_of

"Builds only" is the rows of that second answer whose variant_of is not empty;
"the builds of one model" is the rows whose variant_of equals its name. Both are
read off a field every row carries, which is why neither needs an endpoint of its
own.

## Reading the catalog

GET /api/models answers a JSON array. Every row is the model's IDENTITY, and its
value in every column:

  {"name":"Qwen: Qwen3-32B",
   "source":"openrouter",
   "hidden":false,
   "variant_of":"",
   "cells":{"id":"qwen/qwen3-32b",
            "context_length":"40960",
            "input_modalities":"[\"text\"]",
            "parameters":"32.76",
            "supports_tools":"yes",
            "notes":"handles long diffs well"},
   "answered_by":{"id":"openrouter",
                  "context_length":"openrouter",
                  "input_modalities":"merged",
                  "parameters":"research",
                  "supports_tools":"openrouter",
                  "notes":"hand"},
   "disputed":{"context_length":true},
   "overtaken":{"context_length":true},
   "_raw":{...}}

FOUR IDENTITY FIELDS, which are the only things about a model that are not
columns:

- name       : the display name. It is the primary key every value hangs off, and
               no request renames a row; only a refresh does, when OpenRouter renames a model
- source     : openrouter | huggingface | manual - which route created this
               row (an old row may read collection)
- hidden     : the model is hidden. The row is kept here in full and left out of
               the default answer, so you only ever see hidden:true on a request
               that passed include_hidden=1
- variant_of : the NAME of the model this row is a variation of, empty on every
               ordinary row. See "Variations"

Beside them ride "cells", below; "answered_by", "disputed" and "overtaken", the
three maps "Several sources, one column" describes; and "_raw", the untouched
record the source that created this row last sent, for anything no column
carries.

EVERY OTHER FACT IS A CELL, answered under its column's KEY, as the TEXT this
database holds. Never as a number, never as a flag, never as an array: a context
length comes back as "40960", a flag as "yes" or "no", and a list as the JSON
array it is stored as, "[\"text\",\"image\"]". So a value read is byte for byte
the value stored, and reading a column and filtering on it use one name.

A COLUMN THE MODEL HAS NO VALUE FOR IS ABSENT from the object rather than empty,
because "nobody has said" and "somebody said nothing" are one answer here.

WHICH COLUMNS EXIST IS NOT FIXED, and GET /api/columns is the list. A key
carries no namespace: "cells" holds one entry per column that has an answer,
under its bare key, and "answered_by" says which source that answer came from
- see "Several sources, one column".

model_type is one of chat, image, video, embedding, rerank, tts, stt, music or
decisions. The app works it out from what OpenRouter says the model gives out:
embeddings is embedding, rerank is rerank, speech is tts, transcription is stt,
decisions is decisions (a model whose output is a score or probability for each
allowed option, not text), and a refresh re-types every OpenRouter row, the ones
the feed no longer lists included. It is a single column several sources can
state - the app working it out from what OpenRouter says the model gives out, a
HuggingFace repository, and research where neither has - so a plain read carries
whichever of them ranks highest and has answered, OpenRouter first.

tts and music are both text-in, audio-out, and upstream data does not tell them
apart: every such model arrives stating audio, not music. What separates them
is the model's own description, read once on import (for a HuggingFace
repository, the opening of its model card) - one naming music generation is
filed as music in model_type, everything else audio as tts - so the split is a
fact stored in model_type itself, not a rule a read applies afterward. On a model neither OpenRouter nor a repository has typed, write
music into model_type yourself.

WHAT A READ ANSWERS IS WHAT A WRITE TAKES. POST /api/save and
POST /api/models/add carry the same "cells" object, keyed by the same column
keys, so a value read here can be posted straight back with nothing looked up in
between - as long as the write may reach it (see "Writing model data").

## The query grammar

Every parameter is a COLUMN KEY, except five that are not columns at all:

| Parameter      | What it does                                              |
|----------------|-----------------------------------------------------------|
| <column key>   | keep the models whose value in that column matches         |
| name           | keep the models whose display name matches this (SEARCH SYNTAX, below) |
| include_hidden | also return the models marked hidden, left out by default  |
| hide_free      | drops free variants and 0 prices (see below)               |
| links          | keep the models that have, or lack, a link (see below)     |
| fields         | comma-list: return only these identity fields and columns  |

Filters are AND-combined, and capitals are ignored throughout. A REPEATED
PARAMETER MEANS "AND ALSO": naming the same column twice adds a second
condition on it, ANDed with the first, exactly as two different columns are -
nothing is ever dropped.

NINE WAYS TO ASK ABOUT A COLUMN. The prefix on the value decides which, and a
prefix the column cannot answer is refused with 400 naming what it does take:

  ?model_type=image               the value is exactly this
  ?context_length=min:200000      at least this figure   (number columns)
  ?price_prompt=max:0.000001      at most this figure    (number columns)
  ?description=contains:agent     the text holds this    (text and list; SEARCH SYNTAX)
  ?input_modalities=any:text,image   matches ANY listed value
  ?input_modalities=not:audio        matches NONE of them
  ?notes=by:you                      YOU answered this column
  ?notes=by:not_you                  you did NOT answer this column
  ?context_length=by:research        research answered it, and no source has
  ?context_length=by:not_openrouter  OpenRouter did not answer it
  ?created_at=from:2026-01-01     on or after this day  (date columns)
  ?created_at=to:2026-06-30       on or before this day (date columns)

by: asks WHO ANSWERED the column, not what it holds: the winning source, as
answered_by names it. It takes any source the column declares - openrouter,
huggingface, research, hand, app - or not_ followed by one, and you is another
word for hand. A source the column does not declare is refused, naming the
ones it does, so ?context_length=by:you is a 400 (nothing typed wins a fetched
column) and ?added=by:you is too.

from: and to: are the two inclusive ends of a range of days, and only the
columns that hold a date take them: created_at, added, expiration_date and
knowledge_cutoff (GET /api/columns marks each with "date":true). A day is
written YYYY-MM-DD and nothing else, so ?created_at=from:2026-1-5 is a 400, and
so is from: or to: on any other column. Each end is a day, not an instant:
to:2026-10-10 keeps a model stamped 2026-10-10T23:59:59Z. The stored value is
read by its first ten characters, so both shapes created_at is stored in
(2026-06-16T17:45:30Z and 2022-03-02T23:29:04.000Z) compare the same way. A
model with no date in the column never matches from: or to:. Naming the column
twice, once with each, is the usual range:

  ?created_at=from:2026-01-01&created_at=to:2026-06-30

hide_free drops the models OpenRouter lists as a free variant: the ones whose
OpenRouter id (the id column) ends :free and the ones whose display name ends
(free). It is the one filter that looks at two things at once, the id and the
name, which is why it is a parameter of its own and not a test on a column.

hide_free also drops the models whose figure in a named column is 0, as the
Hide 0 tick under each price filter does. Name the columns in a comma-separated
list, and each one hides the models whose figure there is 0:

  ?hide_free=price_prompt                       # a 0 In price, and the free variants
  ?hide_free=price_prompt,price_completion      # a 0 in either price

hide_free=1 (or true or yes) means both prices, and 0, false, no or nothing
leaves the models as they are. A model with NO figure in a named column is
kept: a model charged per image or per second often carries no per-token price
at all, or a 0 that only means OpenRouter does not price it per token, so hiding
a 0 can hide models that are not free. Any other word is refused with 400, and
only a column of figures can be named. hide_free is ANDed with every other
filter, min: and max: on the same column included.

links keeps the models that have, or lack, one of the two links a model row can
show: huggingface (its repository column holds a value) and openrouter (its id
column holds a value). It takes any: or not: followed by a comma-separated list
of those two names, and it is a parameter of its own because "either link"
looks at two columns at once, which a column parameter cannot say:

  ?links=any:huggingface,openrouter    has at least one of the two links
  ?links=any:huggingface               has a HuggingFace link
  ?links=not:openrouter                has no OpenRouter link
  ?links=any:huggingface&links=not:openrouter   a HuggingFace link and no OpenRouter link

A repeated links ANDs, like a repeated column parameter, and links is ANDed
with every other filter. Any other word, or a value without any: or not:, is
refused with 400 saying what it takes.

any: and not: take a comma-separated value list and work on every kind of
column except a number, which keeps min: and max:. Each listed value is read
exactly as an exact match reads one: a member of a list column, either
spelling of a flag, the value itself elsewhere. A literal comma inside a value
is written \, - and an EMPTY member means "no value at all", so any: alone is
the empty match and not: alone keeps only the models that state something.
Because a repeated parameter ANDs, one column can carry an include list and an
exclude list at once:

  ?input_modalities=any:text,image&input_modalities=not:audio

SEARCH SYNTAX. name=, contains: and /api/columns/values?contains= read the
same small language, and it never answers 400 - half-typed text is read as far
as it goes:

  gpt 4o            every word must appear, in any order
  "gpt 4o"          a quoted phrase must appear exactly as written, spaces
                    included; everything inside quotes is literal
  gpt !mini         a ! at the START of a word or phrase excludes it
  claude OR gemini  either side; AND between terms is the same as a space
  a b OR c          AND binds tighter than OR: (a and b) or c

Only UPPERCASE AND and OR are operators, so "and" and "or" stay ordinary words.
There are no parentheses, and a ! inside a word is just a letter. An unclosed
quote runs to the end of the text, and a stray operator, a lone ! or empty
quotes are ignored; text that reduces to nothing matches everything. Capitals
are ignored in the terms. A double quote cannot itself be searched for.
On a LIST column a term is met when any one member holds it, so different terms
may be met by different members, and a quoted phrase must sit inside one
member. A ! term is met by a model with NO value in the column, so
?notes=contains:!draft includes every model with no note.

The four rules that are not obvious:

- an EXACT match on a LIST column matches any one member, so
  ?input_modalities=image is every model that takes an image;
- an exact match on a FLAG column reads yes, true and 1 as one word, and no,
  false and 0 as one word, so ?zdr=true and ?zdr=yes ask the same
  question;
- an EMPTY value matches the models with no value in that column at all, so
  ?notes= is every model you have written no note on;
- a model with no value in the column never matches min: or max:, and matches
  contains: only when every term is a ! term (see SEARCH SYNTAX).

GETTING JSON AT ALL. An empty query string returns this guide, so pass at least
one parameter. GET /api/models?include_hidden=0 is the whole catalog with nothing
filtered out, and include_hidden=1 adds the hidden rows - the variations, and
anything you have hidden yourself.

PROJECTION. fields= takes identity field names and column keys in one list. The
rows come back carrying the identity fields you named, and a "cells" object
holding only the column keys you named:

  curl 'http://127.0.0.1:8122/api/models?include_hidden=0&fields=name,price_prompt'
  # [{"name":"OpenAI: GPT-5","cells":{"price_prompt":"0.00000125"}}, ...]

AN UNKNOWN PARAMETER ANSWERS 400 AND NAMES IT, and so does an unknown name inside
fields=. The refusal offers the columns whose KEY comes closest to what you wrote
AND the columns whose LABEL does, both, because a column can be known either way:

  curl 'http://127.0.0.1:8122/api/models?type=image'
  # 400 "type" is not a parameter this query takes. A filter names a column by
  # its key (GET /api/columns lists every one), and "name", "include_hidden",
  # "hide_free", "links" and "fields" are the five that are not columns. Columns whose key is close:
  # model_type, instruct_type. Columns whose label
  # is close: "Type" (model_type).

A NAME IS OFFERED WHEN IT SHARES A WORD, not only when it is misspelt, which is
what carries an old two-word parameter to the column it meant: min_context
offers context_length, and max_out offers price_completion.

THERE ARE NO NAMED FILTERS, and there is no all=1. The named filters of the 1.x
releases - type, input, output, tool, reasoning, zdr, min_context, max_in,
max_out and all - do not exist, and each of them answers 400 naming the columns
that replace it:

  type=image        -> model_type=image
  input=image       -> input_modalities=image
  output=text       -> output_modalities=text
  tool=yes          -> supports_tools=yes
  reasoning=yes     -> supports_reasoning=yes
  zdr=true          -> zdr=yes
  min_context=N     -> context_length=min:N
  max_in=<per 1M>   -> price_prompt=max:<per token>
  max_out=<per 1M>  -> price_completion=max:<per token>
  all=1             -> include_hidden=0

## IMPORTANT - pricing units

The price columns hold a RAW PRICE PER TOKEN, as text: price_prompt and
price_completion. Each is a single column two sources can state - OpenRouter,
and research where OpenRouter has not - so a plain read carries OpenRouter's
wherever there is one; GET /api/models/{name}/answers/price_prompt shows every
source's own figure. Multiply by 1,000,000 for USD per 1M tokens.

For an image / video / audio / character-priced model those columns are ABSENT
unless research filled them - such a model is not priced per token at all.
price_unit says what it is charged by ("per second", "per image"), and
notes carries the SKU in words, for example:
  - "Output Image: $0.07/megapixel"
  - "Video: $0.10/second"
  - "Characters: $0.62/M characters"
  - "Audio Minutes: $0.006/minute"
Never read a per-token price as the price when price_unit is not a token unit.
A min: or max: filter on a price column therefore matches only the models that
hold a per-token figure.


## Examples

Reading:

  curl 'http://127.0.0.1:8122/api/models?model_type=image&output_modalities=image'
  curl 'http://127.0.0.1:8122/api/models?name=claude&fields=name,price_prompt,price_completion,context_length'
  curl 'http://127.0.0.1:8122/api/models?supports_tools=yes&price_prompt=max:0.000001&context_length=min:200000'
  curl 'http://127.0.0.1:8122/api/models?include_hidden=0'
  curl 'http://127.0.0.1:8122/api/models?include_hidden=1'   # hidden ones too
  curl 'http://127.0.0.1:8122/api/health'
  curl 'http://127.0.0.1:8122/api/columns'

Writing one cell, leaving every other cell alone:

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"OpenAI: GPT-5","cells":{"notes":"handles long diffs well"}}]'

Writing several models, and several cells, in one request:

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"OpenAI: GPT-5","cells":{"favorite":"yes","supports_tools":"yes"}},
         {"name":"Google: Veo 3.1","cells":{"price_unit":"per second",
          "notes":"Video (no audio): $0.08/second"}}]'

Filling in a measured column nothing has stated yet - kept as research until
the repository is read:

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"OpenAI: GPT-5","cells":{"parameters":"32.76"}}]'

Making a column of your own, which only you write:

  curl -X POST http://127.0.0.1:8122/api/columns \
    -H 'Content-Type: application/json' \
    -d '{"label":"Rating","value_type":"number"}'

Adding a model by hand - one object, not an array:

  curl -X POST http://127.0.0.1:8122/api/models/add \
    -H 'Content-Type: application/json' \
    -d '{"name":"My Local Build","cells":{"notes":"runs on my own machine",
         "parameters":"27"}}'

Writing back what you researched about a model added by hand - nothing
automatic has stated anything on it, so every field is open:

  curl -X POST http://127.0.0.1:8122/api/save \
    -H 'Content-Type: application/json' \
    -d '[{"name":"My Local Build","cells":{"model_type":"chat",
         "context_length":"40960","supports_reasoning":"yes",
         "supports_tools":"yes","moe":"no","vendor":"qwen",
         "parameters":"32.76","weights_size_gb":"65.52"}}]'

Hiding a model, and deleting one:

  curl -X POST http://127.0.0.1:8122/api/models/hide \
    -H 'Content-Type: application/json' -d '{"name":"OpenAI: GPT-5","hidden":true}'
  curl -X POST http://127.0.0.1:8122/api/models/delete \
    -H 'Content-Type: application/json' -d '{"name":"My Local Build","confirm_delete":true}'
`
