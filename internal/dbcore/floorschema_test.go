package dbcore

// floorSchemaSQL is the honest dump this package's floor rests on: the literal
// CREATE TABLE and CREATE INDEX statements SQLite itself produced after running
// the full v1-v33 migration chain against the pre-floor schema, captured by
// internal/dbcore/schemadump_gen_test.go and never edited since. coreSchemaSQL
// in db.go is a hand-formatted, commented restatement of the same shape; this
// constant exists only so TestFloorSchemaMatchesCurrentSchema can prove the two
// agree structurally, without trusting a human transcription.
const floorSchemaSQL = `
CREATE TABLE cells (
  column_id    INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  model_name   TEXT    NOT NULL REFERENCES models(name) ON DELETE CASCADE,
  value_text   TEXT    NOT NULL,
  value_number REAL,
  written_at   TEXT    NOT NULL DEFAULT '',
  PRIMARY KEY (column_id, model_name),
  CHECK (value_text <> '')
);

CREATE TABLE column_options (
  column_id INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
  position  INTEGER NOT NULL,
  label     TEXT    NOT NULL,
  PRIMARY KEY (column_id, position),
  UNIQUE (column_id, label)
);

CREATE TABLE columns (
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
  color_rules TEXT   NOT NULL DEFAULT '',
  CHECK (column_key LIKE namespace || '.%')
);

CREATE TABLE derived_columns (
  column_id  INTEGER PRIMARY KEY REFERENCES columns(id) ON DELETE CASCADE,
  transform  TEXT    NOT NULL,
  params     TEXT    NOT NULL DEFAULT '',
  rebuilt_at TEXT    NOT NULL DEFAULT ''
);

CREATE TABLE derived_inputs (
  column_id       INTEGER NOT NULL REFERENCES derived_columns(column_id) ON DELETE CASCADE,
  position        INTEGER NOT NULL,
  input_column_id INTEGER NOT NULL REFERENCES columns(id) ON DELETE RESTRICT,
  PRIMARY KEY (column_id, position)
);

CREATE TABLE "ignored_models" (
			kind       TEXT NOT NULL CHECK (kind IN ('openrouter', 'huggingface', 'name')),
			model_key  TEXT NOT NULL,
			name       TEXT NOT NULL DEFAULT '',
			ignored_at TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (kind, model_key)
		);

CREATE TABLE models (
  name                     TEXT PRIMARY KEY NOT NULL,
  source                   TEXT    DEFAULT '',
  variant_of               TEXT    DEFAULT '',
  updated_at               TEXT    DEFAULT ''
, hidden INTEGER DEFAULT 0);

CREATE TABLE source_records (
  model_name TEXT NOT NULL REFERENCES models(name) ON DELETE CASCADE,
  namespace  TEXT NOT NULL,
  raw_json   TEXT NOT NULL DEFAULT '',
  read_at    TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (model_name, namespace)
);

CREATE INDEX idx_cells_model  ON cells(model_name);

CREATE INDEX idx_cells_number ON cells(column_id, value_number) WHERE value_number IS NOT NULL;

CREATE INDEX idx_cells_text   ON cells(column_id, value_text COLLATE NOCASE);

CREATE INDEX idx_columns_namespace ON columns(namespace);

CREATE INDEX idx_derived_inputs_source ON derived_inputs(input_column_id);

CREATE INDEX idx_ignored_models_name ON ignored_models(name);
`
