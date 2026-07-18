// Package dbcore - SQLite data layer. A single `models` table is the source of
// truth; each model exists once, keyed by name. API metadata is refreshed on
// startup; curated fields and collection-derived capabilities are preserved.
package dbcore

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

var DB *sql.DB

// curatedCols are user/manual fields the OpenRouter refresh must NEVER clobber.
var curatedCols = []string{"notes", "speed", "rating", "favorite", "ocr_quality", "tool", "moe", "parameters", "active_parameters", "disk_size_gb", "measurement", "pricing_note", "unlisted"}

const schemaSQL = `
CREATE TABLE IF NOT EXISTS models (
  name                     TEXT PRIMARY KEY,
  source                   TEXT    DEFAULT '',
  model_type               TEXT    DEFAULT 'chat',
  id                       TEXT    DEFAULT '',
  slug                     TEXT    DEFAULT '',
  hf_slug                  TEXT    DEFAULT '',
  author                   TEXT    DEFAULT '',
  description              TEXT    DEFAULT '',
  context_length           INTEGER,
  input_modalities         TEXT    DEFAULT '',  -- JSON array
  output_modalities        TEXT    DEFAULT '',  -- JSON array
  supports_reasoning       INTEGER DEFAULT 0,
  hidden                   INTEGER DEFAULT 0,
  price_prompt             TEXT    DEFAULT '',
  price_completion         TEXT    DEFAULT '',
  price_image              TEXT    DEFAULT '',
  price_audio              TEXT    DEFAULT '',
  price_internal_reasoning TEXT    DEFAULT '',
  price_display            TEXT    DEFAULT '',  -- human pricing summary (per-megapixel/second/etc.)
  pricing_url              TEXT    DEFAULT '',  -- source page for scraped pricing
  measurement              TEXT    DEFAULT '',  -- pricing unit for In/Out $ (e.g. 'per 1M tokens', 'per second', 'per image')
  pricing_note             TEXT    DEFAULT '',  -- objective pricing caveats (e.g. extra SKUs the In/Out columns omit)
  collections              TEXT    DEFAULT '',  -- JSON array of collection slugs
  notes                    TEXT    DEFAULT '',
  speed                    TEXT    DEFAULT '',
  rating                   INTEGER DEFAULT 0,
  favorite                 INTEGER DEFAULT 0,
  ocr_quality              TEXT    DEFAULT '',
  unlisted                 INTEGER DEFAULT 0,  -- 1 = keep in this DB but EXCLUDE from the public curated.json export
  tool                     TEXT    DEFAULT '',
  moe                      TEXT    DEFAULT '',
  parameters               INTEGER,
  active_parameters        INTEGER,
  disk_size_gb             REAL,               -- native-precision on-disk size (GB) of the safetensors/.bin weight files on the canonical HF repo's main revision (excludes quantized/GGUF mirrors); curated, never auto-refreshed
  zdr                      INTEGER DEFAULT 1,  -- Zero Data Retention: any serving provider keeps no prompts (non-OpenRouter models are always 1)
  raw_json                 TEXT    DEFAULT '',
  created_at               TEXT    DEFAULT '',  -- model creation date (from API)
  updated_at               TEXT    DEFAULT ''   -- last DB write (bookkeeping)
);
`

// OpenDB opens (creating if needed) the SQLite database, ensures the base
// schema, then runs any pending schema migrations (see migrate.go), which take
// a versioned, backed-up path rather than ad-hoc column patching.
func OpenDB(path string) error {
	if err := openConn(path); err != nil {
		return err
	}
	return migrate()
}

// openConn opens the connection and applies the base schema + pragmas, WITHOUT
// running migrations. Migration auto-restore reuses it to reopen a restored file
// without recursing back into migrate().
func openConn(path string) error {
	var err error
	DB, err = sql.Open("sqlite", path)
	if err != nil {
		return fmt.Errorf("open db: %w", err)
	}
	DB.SetMaxOpenConns(1) // SQLite single-writer; serialize to avoid lock churn
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err := DB.Exec(p); err != nil {
			return fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	if _, err := DB.Exec(schemaSQL); err != nil {
		return fmt.Errorf("create schema: %w", err)
	}
	return nil
}

func CountModels() (int, error) {
	var n int
	err := DB.QueryRow("SELECT COUNT(*) FROM models").Scan(&n)
	return n, err
}

// rowsToMaps scans an arbitrary result set into a slice of column->value maps,
// normalizing []byte to string.
func rowsToMaps(rows *sql.Rows) ([]map[string]interface{}, error) {
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	var out []map[string]interface{}
	for rows.Next() {
		vals := make([]interface{}, len(cols))
		ptrs := make([]interface{}, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]interface{}, len(cols))
		for i, c := range cols {
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			m[c] = v
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func GetAllModels() ([]map[string]interface{}, error) {
	rows, err := DB.Query("SELECT * FROM models")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return rowsToMaps(rows)
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

// NullFloat maps a JSON-ish numeric (or nil) to a float64 or nil for SQL. It is
// the float counterpart of NullInt, used for REAL columns like disk_size_gb.
func NullFloat(v interface{}) interface{} {
	switch x := v.(type) {
	case float64:
		return x
	case float32:
		return float64(x)
	case int:
		return float64(x)
	case int64:
		return float64(x)
	case json.Number:
		f, _ := x.Float64()
		return f
	}
	return nil
}

func NowStamp() string { return time.Now().UTC().Format(time.RFC3339) }
