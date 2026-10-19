package dbcore

// TestFloorSchemaMatchesCurrentSchema proves coreSchemaSQL (db.go, hand-
// formatted and commented) is structurally identical to floorSchemaSQL
// (floorschema_test.go, the frozen, never-edited dump of what v1-v33 actually
// built). It builds one database from each, brings both up to the current
// version with the same migrations (v34 onward), and compares table, column,
// foreign-key and index structure - never the raw CREATE text, which differs
// in comments and whitespace by design.
//
// readSchema and its helpers are a deliberate duplicate of the pattern in
// internal/store/archdocs_gen_test.go: dbcore sits below store in the import
// DAG (paths -> shared -> dbcore -> store), so this package cannot import that
// one's helpers.

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

func TestFloorSchemaMatchesCurrentSchema(t *testing.T) {
	floorPath := filepath.Join(t.TempDir(), "floor.db")
	floorDB, err := buildFrom(floorPath, floorSchemaSQL+cellStoreSQL, 33)
	if err != nil {
		t.Fatalf("build the frozen floor database: %v", err)
	}
	DB = floorDB
	if err := migrate(); err != nil {
		floorDB.Close()
		t.Fatalf("bring the frozen floor database up to date: %v", err)
	}
	floorSchema, err := readSchema()
	floorDB.Close()
	if err != nil {
		t.Fatalf("read the frozen floor database's schema: %v", err)
	}

	currentPath := filepath.Join(t.TempDir(), "current.db")
	if err := OpenDB(currentPath); err != nil {
		t.Fatalf("open a fresh database on the current schema: %v", err)
	}
	currentSchema, err := readSchema()
	DB.Close()
	if err != nil {
		t.Fatalf("read the fresh database's schema: %v", err)
	}

	// CREATE text differs by design (comments, whitespace); only structure must
	// agree.
	for i := range floorSchema.Tables {
		floorSchema.Tables[i].CreateSQL = ""
	}
	for i := range currentSchema.Tables {
		currentSchema.Tables[i].CreateSQL = ""
	}
	floorSchema.Version = 0
	currentSchema.Version = 0

	if !reflect.DeepEqual(floorSchema, currentSchema) {
		t.Fatalf("the frozen floor schema and the current schema disagree structurally:\nfloor:   %+v\ncurrent: %+v", floorSchema, currentSchema)
	}
}

// buildFrom opens a brand-new database at path, applies rawSchema verbatim
// (no comments or formatting requirements - it is read back by
// sqlite_master, never by a person), and stamps user_version so migrate()
// picks up from exactly that point.
func buildFrom(path, rawSchema string, atVersion int) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1)
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA busy_timeout=5000", "PRAGMA foreign_keys=ON"} {
		if _, err := db.Exec(p); err != nil {
			db.Close()
			return nil, fmt.Errorf("pragma %q: %w", p, err)
		}
	}
	if _, err := db.Exec(rawSchema); err != nil {
		db.Close()
		return nil, fmt.Errorf("create schema: %w", err)
	}
	if _, err := db.Exec(fmt.Sprintf("PRAGMA user_version = %d", atVersion)); err != nil {
		db.Close()
		return nil, fmt.Errorf("set user_version: %w", err)
	}
	return db, nil
}

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
	Origin  string
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
	if err := DB.QueryRow("PRAGMA user_version").Scan(&s.Version); err != nil {
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
	rows, err := DB.Query(`SELECT name, COALESCE(sql,'') FROM sqlite_master
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
	rows, err := DB.Query(fmt.Sprintf("PRAGMA table_info(%q)", table))
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
	rows, err := DB.Query(fmt.Sprintf("PRAGMA foreign_key_list(%q)", table))
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
	sort.Slice(out, func(a, b int) bool { return out[a].FromColumn < out[b].FromColumn })
	return out, rows.Err()
}

func tableIndexes(table string) ([]indexInfo, error) {
	rows, err := DB.Query(fmt.Sprintf("PRAGMA index_list(%q)", table))
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
	rows, err := DB.Query(fmt.Sprintf("PRAGMA index_info(%q)", index))
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
		}
	}
	return cols, rows.Err()
}
