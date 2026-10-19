// The SQLite data layer: opening the database, the floor schema a brand-new
// database starts from, and the small value helpers the stores share. Each model
// exists once, keyed by name, and every fact about it is a cell (see the store
// package).

package dbcore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// DB is the one open database connection, set by OpenDB. The pool holds a single
// connection, so every statement runs one at a time.
var DB *sql.DB

// schemaSQL is the table set a brand-new database starts from, before migrate()
// brings it to the current schema version.
//
// IT IS THE FLOOR SCHEMA: exactly what a database at schema version 33 holds. No
// database below 33 is opened (see the floor check in OpenDB), so only the shape
// that version has needs declaring here. It is regenerated from the frozen dump
// in floorschema_test.go if the floor ever rises: see schemadump_gen_test.go.
//
// It is written in two pieces so that the cell store has ONE definition:
// cellStoreSQL is also what a database carried forward from before the cell store
// existed was given, and a second copy of that DDL is a second thing to keep in
// step.
const schemaSQL = coreSchemaSQL + cellStoreSQL

const coreSchemaSQL = `
-- One model, and its IDENTITY only: the handle every stored value hangs off,
-- which route created the row, whether the reader has hidden it, which model
-- it is filed under, and when it was last written. Every other fact about a
-- model is a cell (see cells below).
CREATE TABLE IF NOT EXISTS models (
  -- The handle every route addresses a model by, and the key every stored value
  -- hangs off (cells.model_name REFERENCES models(name)). SQLite permits a
  -- missing value in a TEXT PRIMARY KEY column, and such a row could never be
  -- read back, written to, hidden or deleted - so NOT NULL is what makes "every
  -- stored row is reachable" true in the file rather than only in the Go that
  -- writes it.
  name                     TEXT PRIMARY KEY NOT NULL,
  -- Which route created this row: the OpenRouter pull, a HuggingFace import, or a
  -- person typing it. It is not derived from which sources hold cells, because a
  -- model OpenRouter drops must still be recognisable as one of theirs after its
  -- openrouter cells have gone.
  source                   TEXT    DEFAULT '',
  -- What marks a row as a VARIATION: one quantized build of another model - a
  -- single file on a HuggingFace repository - stored as a models row of its own
  -- and grouped under the model it was built from. variant_of is the grouping
  -- itself rather than a fact about the model, which is why it stays a column
  -- while every figure the build importer measures is a cell.
  variant_of               TEXT    DEFAULT '',  -- the parent model's name; empty on a parent row. NOT a foreign key: one there would make deleting a parent either cascade its builds away or refuse the delete
  updated_at               TEXT    DEFAULT '',  -- last DB write (bookkeeping)
  -- 1 = hidden. The row and everything attached to it stay exactly where they
  -- are; what changes is who sees it: the table draws it only while "Show
  -- hidden" is ticked, and GET /api/models withholds it unless the caller
  -- passes include_hidden=1.
  hidden                   INTEGER DEFAULT 0
);

-- Part of the floor shape, dropped by migration v44 (ignoredropped.go). It stays
-- here so this text keeps matching floorSchemaSQL (floorschema_test.go): a fresh
-- install creates the table and migrate() removes it, the same as a database
-- carried forward.
CREATE TABLE IF NOT EXISTS ignored_models (
  kind       TEXT NOT NULL CHECK (kind IN ('openrouter', 'huggingface', 'name')),
  model_key  TEXT NOT NULL,
  name       TEXT NOT NULL DEFAULT '',
  ignored_at TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (kind, model_key)
);

CREATE INDEX IF NOT EXISTS idx_ignored_models_name ON ignored_models(name);
`

// cellStoreSQL is the cell store, and the whole of it: the tables that hold
// one fact about one model as one ROW. It is the second half of schemaSQL, and
// what a database carried forward from before the cell store was given, so both
// paths end at the same shape from one piece of text.
const cellStoreSQL = `
--
-- Tables that hold one fact about one model as one ROW rather than as a column of
-- models. Each index below is over a column its own CREATE TABLE just made, so
-- there is no state of any database in which an index names a column that is not
-- there.
--
-- A column's PERMANENT identity is its key, e.g. openrouter.context_length. The
-- part before the dot is its namespace: who the column belongs to. The CHECK on
-- column_key holds the two in step, so a key can never say one namespace while
-- the row says another. (Migrations v39 to v41 move that distinction onto the
-- cell, as its source.)
--
-- The value types mirror store.ValueTypeText and its neighbours
-- (internal/store/cells.go) - dbcore sits below store in the import DAG (paths
-- -> shared -> dbcore -> store), so it cannot import those constants; change both
-- together.
CREATE TABLE IF NOT EXISTS columns (
  id         INTEGER PRIMARY KEY AUTOINCREMENT,
  column_key TEXT    NOT NULL UNIQUE,
  namespace  TEXT    NOT NULL CHECK (namespace IN ('openrouter','huggingface','collection','mine','derived')),
  label      TEXT    NOT NULL,
  value_type TEXT    NOT NULL CHECK (value_type IN ('text','number','flag','choice','list')),
  unit       TEXT    NOT NULL DEFAULT '',
  about      TEXT    NOT NULL DEFAULT '',
  position   INTEGER NOT NULL DEFAULT 0,
  visible    INTEGER NOT NULL DEFAULT 1,
  sealed     INTEGER NOT NULL DEFAULT 0,
  created_at TEXT    NOT NULL DEFAULT '',
  -- What colour this column's cells are painted, by the value they hold, as the
  -- JSON of one rule: ordered thresholds on a number column, a colour for yes
  -- and one for no on a flag, a colour per label on a choice. Empty means the
  -- column is not coloured, which is what a text or list column always holds.
  -- store.SetColumnColorRules is what writes it, and it refuses a rule the
  -- column's kind cannot mean. Added to a database carried forward by migration
  -- v33.
  color_rules TEXT   NOT NULL DEFAULT '',
  CHECK (column_key LIKE namespace || '.%')
);

-- No CREATE INDEX IF NOT EXISTS on columns.namespace here: this block runs on
-- EVERY openConn call, including one against a database already migrated past
-- v41, where columns.namespace no longer exists (see pivotcontract.go). An
-- unconditional index on a column that may not exist would fail that open, and
-- namespace is dropped for good once a database reaches v41, along with
-- whatever indexed it.

-- The allowed labels of a choice column, in the order they were written. A
-- column of any other type has no row here. position is also what a cell in that
-- column SORTS by, so that fast/avg/slow stays a scale instead of becoming an
-- alphabet.
CREATE TABLE IF NOT EXISTS column_options (
  column_id INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  position  INTEGER NOT NULL,
  label     TEXT    NOT NULL,
  PRIMARY KEY (column_id, position),
  UNIQUE (column_id, label)
);

-- One model's value in one column, and the whole of the data. A model with no
-- value for a column has NO row here rather than a row holding a blank, which is
-- what the CHECK enforces.
--
-- value_text is the truth: what was written, character for character, and what
-- is shown and handed back. value_number is a SORT KEY worked out from it by the
-- column's type - the figure for a number, 1 or 0 for a flag, the option's
-- POSITION for a choice, and nothing at all for text or a list. Only
-- store.WriteCells and store.SaveCells write this table, and each fills both fields in one statement,
-- so the two can never disagree.
--
-- The primary key puts column_id FIRST on purpose: sorting the table by one
-- column and filtering on one column both read one column across every model, so
-- those rows sit together and the answer is one range of the index.
CREATE TABLE IF NOT EXISTS cells (
  column_id    INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  model_name   TEXT    NOT NULL REFERENCES models(name) ON DELETE CASCADE,
  value_text   TEXT    NOT NULL,
  value_number REAL,
  written_at   TEXT    NOT NULL DEFAULT '',
  PRIMARY KEY (column_id, model_name),
  CHECK (value_text <> '')
);

CREATE INDEX IF NOT EXISTS idx_cells_model  ON cells(model_name);
CREATE INDEX IF NOT EXISTS idx_cells_number ON cells(column_id, value_number) WHERE value_number IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_cells_text   ON cells(column_id, value_text COLLATE NOCASE);

-- A computed column's definition: which named transformation works its value
-- out, and anything that transformation needs beyond its inputs.
CREATE TABLE IF NOT EXISTS derived_columns (
  column_id  INTEGER PRIMARY KEY REFERENCES columns(id) ON DELETE CASCADE,
  transform  TEXT    NOT NULL,
  params     TEXT    NOT NULL DEFAULT '',
  rebuilt_at TEXT    NOT NULL DEFAULT ''
);

-- Which columns a computed column reads, in order: position 0 is the preferred
-- answer, because a transformation that takes the first value there is one reads
-- them in this order.
--
-- input_column_id is the ONE foreign key in this schema that RESTRICTS rather
-- than cascades. Cascading would quietly remove a column from a computed
-- column's input list and change what a figure on screen means with nothing to
-- say so; restricting refuses the delete instead, and
-- idx_derived_inputs_source is what makes naming the computed columns that read
-- it cheap.
CREATE TABLE IF NOT EXISTS derived_inputs (
  column_id       INTEGER NOT NULL REFERENCES derived_columns(column_id) ON DELETE CASCADE,
  position        INTEGER NOT NULL,
  input_column_id INTEGER NOT NULL REFERENCES columns(id) ON DELETE RESTRICT,
  PRIMARY KEY (column_id, position)
);

CREATE INDEX IF NOT EXISTS idx_derived_inputs_source ON derived_inputs(input_column_id);

-- The verbatim record each source last sent about each model, kept beside the
-- cells rather than in one. It is a copy of a conversation rather than a fact
-- about the model, it is large, and it is per SOURCE - so putting it in cells
-- would carry a page of JSON through every sort and filter and into three
-- indexes.
CREATE TABLE IF NOT EXISTS source_records (
  model_name TEXT NOT NULL REFERENCES models(name) ON DELETE CASCADE,
  namespace  TEXT NOT NULL,
  raw_json   TEXT NOT NULL DEFAULT '',
  read_at    TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (model_name, namespace)
);
`

// OpenDB opens (creating if needed) the SQLite database, ensures the base
// schema, then runs any pending schema migrations (see migrate.go), which take
// a versioned, backed-up path rather than ad-hoc column patching.
//
// checkFloor runs FIRST, before openConn touches anything: a database below
// schemaFloor is refused unmodified rather than opened read-write to find out
// (see floorcheck.go).
func OpenDB(path string) error {
	checkFloor(path)
	if err := openConn(path); err != nil {
		return err
	}
	return migrate()
}

// openConn opens the connection and applies the base schema + pragmas, WITHOUT
// running migrations. Migration auto-restore reuses it to reopen a restored file
// without recursing back into migrate().
func openConn(path string) error {
	// The pragmas travel in the DSN, so EVERY connection the pool ever opens has
	// them. foreign_keys in particular is per connection, and the cascade rules the
	// migrations' cell guard relies on depend on it. The driver reads the query
	// off the first '?', so a path holding one cannot be opened.
	if strings.Contains(path, "?") {
		return fmt.Errorf("open db: the path %q contains a question mark, which the database driver reads as the start of options", path)
	}
	var err error
	DB, err = sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)")
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	DB.SetMaxOpenConns(1) // SQLite single-writer; serialize to avoid lock churn
	// schemaSQL is the FLOOR shape, which still holds tables later migrations drop,
	// so it is applied only to a file the migrations will run on. A file already
	// at the current version, or beyond it, has everything it needs, and applying
	// the floor again would bring the dropped tables back and would take the write
	// lock a running server may be holding.
	have, err := currentSchemaVersion()
	if err != nil {
		return fmt.Errorf("read schema version: %w", err)
	}
	if have >= schemaVersion {
		return nil
	}
	if _, err := DB.Exec(schemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

// CountModels counts every stored model row, hidden rows and variations included.
func CountModels() (int, error) {
	var n int
	err := DB.QueryRow("SELECT COUNT(*) FROM models").Scan(&n)
	return n, err
}

// CountTopLevelModels counts the rows the table draws as rows of their own:
// every model except a variation whose parent row is here and is not itself a
// variation. A variation whose parent is gone is drawn as an ordinary row, so
// it counts.
func CountTopLevelModels() (int, error) {
	var n int
	err := DB.QueryRow(`SELECT COUNT(*) FROM models m LEFT JOIN models p ON p.name = m.variant_of
		WHERE COALESCE(m.variant_of, '') = '' OR p.name IS NULL OR COALESCE(p.variant_of, '') != ''`).Scan(&n)
	return n, err
}

// ---- value helpers ----------------------------------------------------------

// ToJSONArr stores a modality/list value as a JSON array string.
func ToJSONArr(v interface{}) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		s := strings.TrimSpace(x)
		if s == "" {
			return ""
		}
		if strings.HasPrefix(s, "[") {
			return s // already JSON
		}
		parts := SplitList(s)
		b, _ := json.Marshal(parts)
		return string(b)
	case []string:
		b, _ := json.Marshal(x)
		return string(b)
	case []interface{}:
		b, _ := json.Marshal(x)
		return string(b)
	}
	return ""
}

// SplitList splits comma-separated text into its trimmed, non-empty parts.
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// ParseArr turns a stored JSON-array string into a []string.
func ParseArr(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var arr []interface{}
	if err := json.Unmarshal([]byte(s), &arr); err == nil {
		out := make([]string, 0, len(arr))
		for _, v := range arr {
			out = append(out, fmt.Sprintf("%v", v))
		}
		return out
	}
	return SplitList(s)
}

// MergeArr unions two modality lists, preserving order and de-duping.
func MergeArr(a, b []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, list := range [][]string{a, b} {
		for _, v := range list {
			v = strings.TrimSpace(v)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

// NullInt maps a JSON-ish numeric (or nil) to an int64 or nil for SQL.
func NullInt(v interface{}) interface{} {
	switch x := v.(type) {
	case float64:
		return int64(x)
	case int:
		return int64(x)
	case int64:
		return x
	case json.Number:
		n, _ := x.Int64()
		return n
	}
	return nil
}

// NowStamp is the current time as the UTC RFC 3339 text every timestamp is stored in.
func NowStamp() string { return time.Now().UTC().Format(time.RFC3339) }
