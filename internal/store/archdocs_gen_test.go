package store

// The generator behind the database reference: a document produced ON DEMAND,
// read, and thrown away. It is stored nowhere, and it is never written back
// into this repository.
//
// It is a test rather than a command so that it can read the package's own
// unexported values, which a program outside the package could only guess at
// from the source text. A test compiled INTO the package reads the real values,
// so a document it prints cannot disagree with the code it was printed from.
//
// It does nothing at all unless MODELSDB_ARCHDOCS_DIR names the folder to write
// into, so an ordinary `go test ./...` neither writes a file nor opens a
// database.
//
// Name a scratch folder OUTSIDE this repository, read what lands there, and
// delete it:
//
//	MODELSDB_ARCHDOCS_DIR=/tmp/archdocs \
//	  go test ./internal/store -run TestGenerateArchitectureDocs -count=1
//
// The path must be ABSOLUTE. Go runs a test with the package's own directory as
// the working directory, so a relative one would land under internal/store.
//
// The database it inspects is a brand-new file in the test's own temporary
// directory, migrated from nothing to the current schema version. No database
// belonging to anybody is opened, read or written.

import (
	"database/sql"
	"fmt"
	"modelsdb/internal/dbcore"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

// TestGenerateArchitectureDocs writes 04_database.md into the scratch folder
// MODELSDB_ARCHDOCS_DIR names, and is skipped when that variable is unset.
func TestGenerateArchitectureDocs(t *testing.T) {
	outDir := os.Getenv("MODELSDB_ARCHDOCS_DIR")
	if outDir == "" {
		t.Skip("MODELSDB_ARCHDOCS_DIR is not set, so there is nowhere to write the documents")
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		t.Fatalf("make the output folder %s: %v", outDir, err)
	}

	dbPath := filepath.Join(t.TempDir(), "throwaway.db")
	if err := dbcore.OpenDB(dbPath); err != nil {
		t.Fatalf("open and migrate a throwaway database at %s: %v", dbPath, err)
	}
	defer dbcore.DB.Close()

	schema, err := readSchema()
	if err != nil {
		t.Fatalf("read the schema back out of SQLite: %v", err)
	}

	write := func(name, body string) {
		path := filepath.Join(outDir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", path, err)
		}
		t.Logf("wrote %s (%d bytes)", path, len(body))
	}
	write("04_database.md", databaseMarkdown(schema))
}

// ---------------------------------------------------------------------------
// Reading the schema back out of SQLite
// ---------------------------------------------------------------------------

type columnInfo struct {
	Position     int
	Name         string
	DeclaredType string
	NotNull      bool
	Default      string
	PrimaryKey   int
}

type foreignKey struct {
	FromColumn string
	ToTable    string
	ToColumn   string
	OnDelete   string
	OnUpdate   string
}

type indexInfo struct {
	Name    string
	Unique  bool
	Origin  string // "c" a CREATE INDEX, "u" a UNIQUE constraint, "pk" a primary key
	Columns []string
}

type tableInfo struct {
	Name        string
	CreateSQL   string
	Columns     []columnInfo
	ForeignKeys []foreignKey
	Indexes     []indexInfo
}

type schemaInfo struct {
	Version int
	Tables  []tableInfo
}

func readSchema() (schemaInfo, error) {
	var s schemaInfo
	if err := dbcore.DB.QueryRow("PRAGMA user_version").Scan(&s.Version); err != nil {
		return s, fmt.Errorf("read user_version: %w", err)
	}

	names, createSQL, err := tableNames()
	if err != nil {
		return s, err
	}
	for _, name := range names {
		table := tableInfo{Name: name, CreateSQL: createSQL[name]}
		if table.Columns, err = tableColumns(name); err != nil {
			return s, err
		}
		if table.ForeignKeys, err = tableForeignKeys(name); err != nil {
			return s, err
		}
		if table.Indexes, err = tableIndexes(name); err != nil {
			return s, err
		}
		s.Tables = append(s.Tables, table)
	}
	return s, nil
}

func tableNames() ([]string, map[string]string, error) {
	rows, err := dbcore.DB.Query(`SELECT name, COALESCE(sql,'') FROM sqlite_master
		WHERE type='table' AND name NOT LIKE 'sqlite_%' ORDER BY name`)
	if err != nil {
		return nil, nil, fmt.Errorf("list tables: %w", err)
	}
	defer rows.Close()
	var names []string
	create := map[string]string{}
	for rows.Next() {
		var name, sqlText string
		if err := rows.Scan(&name, &sqlText); err != nil {
			return nil, nil, err
		}
		names = append(names, name)
		create[name] = sqlText
	}
	return names, create, rows.Err()
}

func tableColumns(table string) ([]columnInfo, error) {
	rows, err := dbcore.DB.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("table_info(%s): %w", table, err)
	}
	defer rows.Close()
	var out []columnInfo
	for rows.Next() {
		var (
			c        columnInfo
			notNull  int
			defValue sql.NullString
		)
		if err := rows.Scan(&c.Position, &c.Name, &c.DeclaredType, &notNull, &defValue, &c.PrimaryKey); err != nil {
			return nil, err
		}
		c.NotNull = notNull == 1
		if defValue.Valid {
			c.Default = defValue.String
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func tableForeignKeys(table string) ([]foreignKey, error) {
	rows, err := dbcore.DB.Query(fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("foreign_key_list(%s): %w", table, err)
	}
	defer rows.Close()
	var out []foreignKey
	for rows.Next() {
		var (
			id, seq int
			fk      foreignKey
			toCol   sql.NullString
			match   string
		)
		if err := rows.Scan(&id, &seq, &fk.ToTable, &fk.FromColumn, &toCol, &fk.OnUpdate, &fk.OnDelete, &match); err != nil {
			return nil, err
		}
		if toCol.Valid {
			fk.ToColumn = toCol.String
		}
		out = append(out, fk)
	}
	return out, rows.Err()
}

func tableIndexes(table string) ([]indexInfo, error) {
	rows, err := dbcore.DB.Query(fmt.Sprintf("PRAGMA index_list(%q)", table))
	if err != nil {
		return nil, fmt.Errorf("index_list(%s): %w", table, err)
	}
	var out []indexInfo
	for rows.Next() {
		var (
			seq     int
			idx     indexInfo
			unique  int
			partial int
		)
		if err := rows.Scan(&seq, &idx.Name, &unique, &idx.Origin, &partial); err != nil {
			rows.Close()
			return nil, err
		}
		idx.Unique = unique == 1
		out = append(out, idx)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		cols, err := indexColumns(out[i].Name)
		if err != nil {
			return nil, err
		}
		out[i].Columns = cols
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

func indexColumns(index string) ([]string, error) {
	rows, err := dbcore.DB.Query(fmt.Sprintf("PRAGMA index_info(%q)", index))
	if err != nil {
		return nil, fmt.Errorf("index_info(%s): %w", index, err)
	}
	defer rows.Close()
	var cols []string
	for rows.Next() {
		var (
			seq, cid int
			name     sql.NullString
		)
		if err := rows.Scan(&seq, &cid, &name); err != nil {
			return nil, err
		}
		if name.Valid {
			cols = append(cols, name.String)
		} else {
			cols = append(cols, "(an expression)")
		}
	}
	return cols, rows.Err()
}

// ---------------------------------------------------------------------------
// 04_database.md
// ---------------------------------------------------------------------------

// tableSummaries is the one hand-written part of the database document: a plain
// sentence saying what each table holds. It is keyed by table name, and a table
// with no sentence here is printed with a placeholder that says so, so a new
// table cannot be added without the omission showing.
var tableSummaries = map[string]string{
	"models":         "One row per model, keyed by its display name. It holds the row's IDENTITY and nothing else: the name, the route that created it, when it was last written, whether it is hidden, and which model it is filed under.",
	"columns":        "One row per column that exists: its permanent key, which is the plain name of the fact it answers, the label shown on screen, what kind of value it holds, and how several sources' answers to it are resolved.",
	"column_options": "The allowed labels of a choice column, in the order they were written. That order is also what a cell in the column sorts by, so `fast, avg, slow` stays a scale.",
	"cells":          "One model's value in one column. A model with no value for a column has no row here at all, rather than a row holding a blank.",
	"sources":        "One row per source that can state a fact: the catalog fetch, the collection pages, a model repository, the reader, and the app itself. `kind` is what separates a source that fetches from the reader's own hand.",
	"column_sources": "Which sources may answer a column, and in what order. The lowest rank wins when more than one has answered, and a source with no row here cannot write that column at all.",
	"source_records": "The verbatim record each source last sent about each model, and when it was read. One row per model per source.",
}

func databaseMarkdown(s schemaInfo) string {
	var b strings.Builder
	p := func(format string, args ...interface{}) {
		fmt.Fprintf(&b, format+"\n", args...)
	}

	p("# The database, table by table")
	p("")
	p("**GENERATED, AND A THROWAWAY COPY. Do not edit it by hand, and do not store")
	p("it anywhere - least of all back inside the repository.** Every table, column,")
	p("default, index and cascade below was read out of a real SQLite database with")
	p("`PRAGMA table_info`, `PRAGMA foreign_key_list`, `PRAGMA index_list` and")
	p("`PRAGMA index_info`. If something here is wrong, the generator is wrong.")
	p("")
	p("Produce it again whenever you need it, into a scratch folder OUTSIDE the")
	p("repository that you delete afterwards:")
	p("")
	p("```bash")
	p("MODELSDB_ARCHDOCS_DIR=/tmp/archdocs \\")
	p("  go test ./internal/store -run TestGenerateArchitectureDocs -count=1")
	p("```")
	p("")
	p("Run that command from the ROOT of the repository, because `./internal/store`")
	p("is a relative package path and Go resolves it only from inside the module. The")
	p("OUTPUT path must be ABSOLUTE, and outside the repository: Go runs a test with")
	p("the package's own directory as the working directory, so a relative one lands")
	p("under `internal/store`.")
	p("")
	p("The generator is `internal/store/archdocs_gen_test.go`. It creates a")
	p("BRAND-NEW database in its own temporary folder, lets the app migrate it from")
	p("nothing to the current schema version, and then asks SQLite what is there.")
	p("It opens nobody's real database, and it writes this file and nothing else.")
	p("")
	p("**Schema version %d** `[OBS] PRAGMA user_version` on that throwaway database.", s.Version)
	p("")
	p("**One part of this file IS hand-written**, and it is marked here rather than")
	p("left to be guessed at: the plain sentence under each table, the note under some")
	p("of them, and the \"What it is for\" cell of each column. Those live in the maps")
	p("`tableSummaries`, `tableNotes` and `columnPurpose` inside the generator, so they")
	p("are printed in place but written by a person. Everything else - the tables, the")
	p("columns, the types, the defaults, the keys, the cascades, the indexes and the")
	p("SQL - comes from the database.")
	p("")

	p("## Why this file exists, and why it is not the schema in the source")
	p("")
	p("`internal/dbcore/db.go` holds a block of SQL called `schemaSQL`. It is what a")
	p("brand-new database STARTS from, not what a database ends up with: %d", s.Version)
	p("migrations run on top of it, and several of them delete a column that block")
	p("creates. Reading `schemaSQL` to learn which columns exist therefore gives the")
	p("wrong answer, which is why this file is generated from a migrated database")
	p("instead.")
	p("")

	p("## Vocabulary, defined once")
	p("")
	p("- **Table.** One kind of thing the database stores, drawn as a grid: the rows")
	p("  are the things, the columns are what is known about each of them.")
	p("- **Column.** One fact per row. Every column has a declared TYPE, which is what")
	p("  kind of value it holds.")
	p("- **Declared type.** `TEXT` is words, `INTEGER` is a whole number, `REAL` is a")
	p("  number with decimals. A column with a blank type in the tables below holds")
	p("  whatever it is given, which SQLite permits.")
	p("- **Default.** What the column holds when a row is created without saying.")
	p("- **Primary key.** The column, or columns, that identify a row uniquely. No two")
	p("  rows can carry the same one.")
	p("- **Foreign key.** A column whose value must match a row in another table.")
	p("  `ON DELETE CASCADE` means that when the row it points at is deleted, this row")
	p("  is deleted too, by the database itself, with no undo and nothing to confirm.")
	p("- **Index.** A lookup aid the database keeps so a search on that column does not")
	p("  read every row. It changes speed, never meaning.")
	p("- **Source.** Who stated a value: `openrouter`, `collection`, `huggingface`,")
	p("  `hand` (the reader) or `app`. It is recorded on the CELL, never on the")
	p("  column, so one column holds every source's answer at once.")
	p("- **Cell.** One model's value in one column, from one source, stored as a ROW")
	p("  of `cells`. Every fact about a model is one, whoever stated it.")
	p("- **Rank.** Which source's answer is shown when more than one has answered.")
	p("  It is per column, in `column_sources`, and `hand` is always first.")
	p("- **Column the reader owns.** A row of `columns` the app does not ship, which")
	p("  they can rename, re-type and delete. It is a row like any other column, not")
	p("  a second mechanism beside them.")
	p("")

	p("## How the tables relate")
	p("")
	p("```mermaid")
	p("erDiagram")
	p("    MODELS ||--o| MODELS : \"variant_of files a build under its parent\"")
	p("    MODELS ||--o{ CELLS : \"every fact about it, deleted with it\"")
	p("    COLUMNS ||--o{ CELLS : \"gives each cell its column, deleted with it\"")
	p("    COLUMNS ||--o{ COLUMN_OPTIONS : \"lists a choice column's labels, deleted with it\"")
	p("    SOURCES ||--o{ CELLS : \"stamps each value with who stated it\"")
	p("    COLUMNS ||--o{ COLUMN_SOURCES : \"which sources may answer it, in order\"")
	p("    SOURCES ||--o{ COLUMN_SOURCES : \"is allowed to answer\"")
	p("    MODELS ||--o{ SOURCE_RECORDS : \"the verbatim copy each source sent\"")
	p("```")
	p("")
	p("Some of those lines are worth reading twice.")
	p("")
	p("- **`models.variant_of` points at another `models` row by name and is NOT a")
	p("  foreign key either.** One would force a choice between deleting a model's")
	p("  builds with it and refusing to delete it at all.")
	p("- **`cells` hangs off both `models` and `columns`, and both links cascade.**")
	p("  Deleting a model, or deleting a column, permanently deletes every value on the")
	p("  other side of it. Those values are personal and this database file is their")
	p("  only copy.")
	p("- **`cells.source` points at `sources` and nothing cascades from it.** A source")
	p("  is never deleted; the five are what this app can read the world through.")
	p("- **A trigger refuses a cell whose source has no `column_sources` row for that")
	p("  column**, and a second refuses any UPDATE that moves a stored value onto")
	p("  another column or another source. Together they are why a refresh cannot")
	p("  reach an answer the reader typed.")
	p("")

	p("## The tables")
	p("")
	p("| Table | Columns | What it holds |")
	p("|---|---:|---|")
	for _, tb := range s.Tables {
		p("| `%s` | %d | %s |", tb.Name, len(tb.Columns), summaryOf(tb.Name))
	}
	p("")

	for _, tb := range s.Tables {
		p("---")
		p("")
		p("### `%s`", tb.Name)
		p("")
		p("%s", summaryOf(tb.Name))
		p("")
		if note, ok := tableNotes[tb.Name]; ok {
			p("%s", note)
			p("")
		}
		p("**Columns: %d.**", len(tb.Columns))
		p("")
		p("| # | Column | Declared type | Default | Can it be empty? | Part of the primary key | What it is for |")
		p("|---:|---|---|---|---|---|---|")
		for _, c := range tb.Columns {
			p("| %d | `%s` | %s | %s | %s | %s | %s |",
				c.Position, c.Name, typeText(c.DeclaredType), defaultText(c.Default),
				emptyText(c.NotNull), keyText(c.PrimaryKey), purposeOf(tb.Name, c.Name))
		}
		p("")

		p("**Foreign keys.**")
		p("")
		if len(tb.ForeignKeys) == 0 {
			p("None. No column of this table points at a row of another table.")
		} else {
			p("| Column here | Points at | On delete | On update |")
			p("|---|---|---|---|")
			for _, fk := range tb.ForeignKeys {
				p("| `%s` | `%s.%s` | %s | %s |", fk.FromColumn, fk.ToTable, fk.ToColumn, fk.OnDelete, fk.OnUpdate)
			}
			p("")
			for _, fk := range tb.ForeignKeys {
				if strings.EqualFold(fk.OnDelete, "CASCADE") {
					p("- Deleting a row of `%s` DELETES every row of `%s` whose `%s` matched it. The database does this itself, and nothing can undo it.",
						fk.ToTable, tb.Name, fk.FromColumn)
				}
			}
		}
		p("")

		p("**Indexes.**")
		p("")
		if len(tb.Indexes) == 0 {
			p("None beyond the primary key itself.")
		} else {
			p("| Index | Columns | Unique | Where it came from |")
			p("|---|---|---|---|")
			for _, idx := range tb.Indexes {
				p("| `%s` | %s | %s | %s |", idx.Name, backtickList(idx.Columns), yesNo(idx.Unique), originText(idx.Origin))
			}
		}
		p("")

		p("<details><summary>The exact SQL this table was created from</summary>")
		p("")
		p("```sql")
		p("%s", strings.TrimSpace(tb.CreateSQL))
		p("```")
		p("")
		p("</details>")
		p("")
	}

	p("---")
	p("")
	p("Generated on %s.", time.Now().UTC().Format("2006-01-02"))
	return b.String()
}

// tableNotes is the second hand-written part: anything about a table that the
// pragmas alone do not say. A table with nothing to add has no entry.
var tableNotes = map[string]string{
	"models": "**A model with no name is refused twice over.** SQLite accepts a missing value in " +
		"a `TEXT PRIMARY KEY` column, so `name` carries an explicit `NOT NULL` and the database " +
		"itself turns such a row away - the \"Can it be empty?\" answer for `name` below reads No. " +
		"Above it, `store.hasNoName` (`internal/store/store.go`) refuses a nameless record on every " +
		"path that creates one, and refuses a name of nothing but spaces too, which the database " +
		"cannot see. Both exist because `name` is the handle every route addresses a model by, and " +
		"the key every personal value is stored under, so a row without one could never be read " +
		"back, written to, hidden or deleted.",
	"columns": "`value_type` is checked by the database itself against exactly five words: " +
		"`text`, `number`, `flag`, `choice`, `list`, and `resolve` against two, `first_stated` and " +
		"`union`. A third CHECK, `instr(column_key,'.') = 0`, keeps a key a plain word: a prefixed key " +
		"would say who answered, which is recorded on the cell instead. `label` carries a unique index " +
		"under NOCASE, so two columns can never be told apart by their capitals alone. The words are " +
		"spelled out in Go as `store.ValueTypeText` and its neighbours (`internal/store/cells.go`); " +
		"`dbcore` sits below `store` and cannot import them, so the two copies are changed together.",
	"cells": "**The value is stored twice on purpose.** `value_text` is the truth - what was written, " +
		"character for character, and what is shown and handed back. `value_number` is a SORT KEY " +
		"worked out from that text by the column's kind: the figure for a number, 1 or 0 for a flag, " +
		"the option's POSITION for a choice, and nothing at all for text or a list. One function fills " +
		"both in one statement (`store.WriteCells`), and it is the only writer of this table, which is " +
		"what stops the two disagreeing. A value the column's kind will not take is a REFUSED write, " +
		"never a stored value with no sort key.",
	"column_sources": "**This table is a permission as well as an order.** A trigger refuses a cell " +
		"whose source has no row here for that column, so \"a refresh cannot reach an answer you " +
		"typed\" is enforced by the database rather than remembered by the code. `hand` is rank 0 " +
		"on every column but `added`, which is why your own answer always wins.",
}

// columnPurpose is the third hand-written part: what one column is FOR, keyed
// "table.column". A column with no entry is printed with a placeholder saying
// so, so a column added later cannot slip in undescribed.
var columnPurpose = map[string]string{
	"models.name":                     "The model's display name, and the handle every route addresses it by. It is never rewritten: renaming a row is not something this app can do.",
	"models.source":                   "Which kind of thing put this row here: `openrouter`, `collection`, `huggingface` or `manual`. It is what decides which refresh phase owns which column on this row.",
	"models.model_type":               "What the model does: `chat`, `image`, `video`, `tts`, `stt`, `music`, `embedding`, `rerank`.",
	"models.id":                       "The OpenRouter model id, e.g. `openai/gpt-5`. It is the key a collection page and the pricing scrape find the row by.",
	"models.slug":                     "OpenRouter's canonical slug for the model.",
	"models.hf_slug":                  "The HuggingFace repository the model lives at, e.g. `Qwen/Qwen3-32B`.",
	"models.description":              "The model's blurb, as upstream states it.",
	"models.context_length":           "How many tokens the model takes. One of the four MERGED columns: the higher figure wins.",
	"models.input_modalities":         "What the model accepts, as a JSON list, e.g. `[\"text\",\"image\"]`. A refresh unions it with what is stored.",
	"models.output_modalities":        "What the model produces, as a JSON list. A refresh unions it with what is stored.",
	"models.supports_reasoning":       "1 when any source has seen reasoning support. One of the four MERGED columns: it is raised, never lowered.",
	"models.price_prompt":             "The input price, as upstream or the scrape stated it.",
	"models.price_completion":         "The output price.",
	"models.price_image":              "The image price.",
	"models.price_audio":              "The audio price.",
	"models.price_internal_reasoning": "The price of the model's own internal reasoning tokens.",
	"models.price_display":            "The pricing summary the model's page shows in words, e.g. `Output Image: $0.014/megapixel`. It is what a per-megapixel or per-second price is kept in, since the two price columns above are per token.",
	"models.pricing_url":              "The page a scraped price was read off.",
	"models.measurement":              "What unit the two price columns are in: `per 1M tokens`, `per second`, `per image`.",
	"models.collections":              "Which OpenRouter collection pages listed this model, as a JSON list of slugs. Unioned, never replaced.",
	"models.notes":                    "The reader's own note. Personal, and in this file only.",
	"models.favorite":                 "The reader's own star. Personal, and in this file only.",
	"models.tool":                     "Whether the model can call tools: `yes`, `no`, `unknown` or empty for nobody has said. One of the four MERGED columns.",
	"models.moe":                      "Whether the model is a mixture of experts, in the same four words. One of the four MERGED columns.",
	"models.zdr":                      "Zero Data Retention: 1 when some serving provider keeps no prompts. Rewritten on every row by every completed catalog phase.",
	"models.variant_of":               "The name of the model this row is a quantized build OF, or empty on an ordinary row. It is how one row is filed under another, not a fact about the model.",
	"models.updated_at":               "When this row was last written, by anything.",
	"models.hidden":                   "1 when the reader has hidden the row. The row and everything attached to it stay exactly where they are; what changes is who sees it.",

	"columns.id":          "The column's own number, and what a cell points at, because a number is small and a link over it is cheap. Nothing on the network names a column by it: every request names a column by its key.",
	"columns.column_key":  "The column's PERMANENT identity, and the plain name of the fact it answers, e.g. `context_length`. It is what every request names, it is never rewritten, and it carries no prefix: who answered is recorded on the cell.",
	"columns.label":       "What the column is called on screen. Editable at any time, and UNIQUE ignoring case: one fact is one column, so two columns reading \"Context length\" would be two answers to one question.",
	"columns.value_type":  "What kind of value this column holds: `text`, `number`, `flag`, `choice` or `list`. It is read by the one cell writer, which is what makes a column the app ships and one the user invented the same kind of thing.",
	"columns.unit":        "What the figure is IN, for display: `GB`, `tokens`, `USD per token`. It carries no meaning to the code; it is what makes two columns of the same name readable side by side.",
	"columns.about":       "One line saying what the column means, shown when choosing columns.",
	"columns.position":    "Where the column sits, left to right.",
	"columns.visible":     "Whether the table draws it. Hiding a column leaves the definition and every value exactly where they are, so unhiding one costs nothing.",
	"columns.sealed":      "1 for a column the app ships. It can be relabelled, moved and hidden, and its kind can never be changed.",
	"columns.created_at":  "When the column came into being.",
	"columns.resolve":     "How several sources' answers are turned into the one value shown: `first_stated` shows the best-ranked answer, `union` merges them all. Only the two modality lists and `collections` are `union`.",
	"columns.color_rules": "What colour this column's cells are painted, by the value they hold, as the JSON of one rule: ordered thresholds on a number column, a colour for yes and one for no on a yes-or-no, a colour per label on a one-of-a-list. Empty means the column is not coloured, which is what a text or list column always holds - neither has an order to shade along or a fixed set of values to colour one by one.",

	"column_options.column_id": "Which choice column this label belongs to.",
	"column_options.position":  "Where the label sits in the list, counting from 0. This is also what a cell in that column SORTS by.",
	"column_options.label":     "The label itself, which is both what is stored in the cell and what is shown.",

	"cells.column_id":    "Which column this value is in.",
	"cells.model_name":   "Which model it is about.",
	"cells.source":       "Who stated this value. It is part of the primary key, so one column can hold one answer per source per model, and a source writing its own answer can never land on another's.",
	"cells.value_text":   "The value itself, exactly as it was written, and the thing that is shown, returned and written back. Never empty: an empty cell has no row.",
	"cells.value_number": "The same value as a number, for sorting and for range filters, and nothing else. It is worked out from the text by the column's kind, so the two can never disagree. Empty for a kind that has no number.",
	"cells.written_at":   "When this cell was last written. It is not a permission and nothing branches on it.",

	"sources.name":     "The source's permanent identity, and what a cell is stamped with: `openrouter`, `collection`, `huggingface`, `hand`, `app`.",
	"sources.label":    "What the source is called on screen, e.g. \"You\" for `hand`.",
	"sources.kind":     "`fetched` for a source that reads the outside world again on every refresh, `hand` for the reader, `static` for the app stating something once. It is what the code asks when it needs to know whether a value can be re-read.",
	"sources.position": "The order sources are listed in, on screen and in the answers panel.",

	"column_sources.column_id": "Which column the source may answer.",
	"column_sources.source":    "Which source may answer it. A source with no row here cannot write this column at all.",
	"column_sources.rank":      "Which answer wins when more than one source has one: the lowest rank. `hand` is 0 wherever it appears.",

	"source_records.model_name": "Which model the record is about.",
	"source_records.source":     "Which source sent it.",
	"source_records.raw_json":   "What it sent, untouched. It is kept beside the cells rather than in one, because it is a copy of a conversation rather than a fact about the model, and it is large.",
	"source_records.read_at":    "When it was read.",
}

func purposeOf(table, column string) string {
	if p, ok := columnPurpose[table+"."+column]; ok {
		return p
	}
	return "**Not described.** Add `" + table + "." + column + "` to `columnPurpose` in `internal/store/archdocs_gen_test.go`."
}

func summaryOf(table string) string {
	if s, ok := tableSummaries[table]; ok {
		return s
	}
	return "**No plain-language summary is recorded for this table.** Add one to `tableSummaries` in `internal/store/archdocs_gen_test.go`."
}

func typeText(declared string) string {
	if strings.TrimSpace(declared) == "" {
		return "*(none declared)*"
	}
	return "`" + declared + "`"
}

func defaultText(def string) string {
	if def == "" {
		return "*(none: the column starts empty of any value at all)*"
	}
	return "`" + def + "`"
}

func emptyText(notNull bool) string {
	if notNull {
		return "No, a value is required"
	}
	return "Yes"
}

func keyText(pk int) string {
	if pk == 0 {
		return "No"
	}
	return fmt.Sprintf("Yes, part %d", pk)
}

func yesNo(b bool) string {
	if b {
		return "Yes"
	}
	return "No"
}

func originText(origin string) string {
	switch origin {
	case "c":
		return "a `CREATE INDEX` of its own"
	case "u":
		return "a `UNIQUE` constraint on the column"
	case "pk":
		return "the primary key"
	}
	return origin
}

func backtickList(items []string) string {
	out := make([]string, 0, len(items))
	for _, i := range items {
		out = append(out, "`"+i+"`")
	}
	return strings.Join(out, ", ")
}
