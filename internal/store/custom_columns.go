// Package store - user-defined "custom columns": personal, DB-only columns the
// user creates (text, or a single-select dropdown of text or number labels) and
// deletes at will. A custom column and its values are ALWAYS personal-only: they
// live solely in custom_columns/custom_column_options/custom_values (see
// internal/dbcore/db.go), and are never read by ExportCuratedJSON/CuratedBytes,
// so they never reach curated.json or internal/seedcatalog/catalog.json and are
// never shared across installs - the same guarantee the fixed personal fields
// (notes/speed/rating/favorite/ocr_quality) carry.
package store

import (
	"database/sql"
	"fmt"
	"modelsdb/internal/dbcore"
	"strings"
)

// The three predefined custom-column types: text-only, and single-select
// dropdowns of text or number labels (no multi-select, no open/arbitrary type
// system). A number-dropdown value is a plain label, sorted and filtered as an
// enum, never as a numeric range.
//
// These exact string values are duplicated in the CHECK constraint on
// custom_columns.type (internal/dbcore/db.go's schemaSQL and the matching DDL
// in internal/dbcore/migrate.go's addCustomColumnsAndMigratePersonalFields) -
// dbcore sits below store in the import DAG (paths -> shared -> dbcore ->
// store), so it cannot import this constant. Change all three together.
const (
	ColumnTypeText           = "text"
	ColumnTypeDropdownText   = "dropdown_text"
	ColumnTypeDropdownNumber = "dropdown_number"
)

// ValidColumnType reports whether t is one of the three predefined column types.
func ValidColumnType(t string) bool {
	switch t {
	case ColumnTypeText, ColumnTypeDropdownText, ColumnTypeDropdownNumber:
		return true
	}
	return false
}

// isDropdownType reports whether a column type carries an option set.
func isDropdownType(t string) bool {
	return t == ColumnTypeDropdownText || t == ColumnTypeDropdownNumber
}

// CustomColumn is one user-defined column definition: its type, and - for a
// dropdown type - its allowed values in display order (the order the user
// entered them in, one per line; that order is fixed at creation, since a
// column has no separate reorder control).
type CustomColumn struct {
	ID        int64    `json:"id"`
	Name      string   `json:"name"`
	Type      string   `json:"type"`
	Options   []string `json:"options,omitempty"`
	CreatedAt string   `json:"created_at"`
}

// ListCustomColumns returns every custom column definition, oldest first (so
// the three columns migrated from the legacy Speed/Rating/OCR fields lead),
// each with its dropdown options in display order (nil for a text column).
func ListCustomColumns() ([]CustomColumn, error) {
	rows, err := dbcore.DB.Query("SELECT id, name, type, created_at FROM custom_columns ORDER BY id ASC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var cols []CustomColumn
	for rows.Next() {
		var c CustomColumn
		if err := rows.Scan(&c.ID, &c.Name, &c.Type, &c.CreatedAt); err != nil {
			return nil, err
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for i := range cols {
		opts, err := customColumnOptions(cols[i].ID)
		if err != nil {
			return nil, err
		}
		cols[i].Options = opts
	}
	return cols, nil
}

// customColumnOptions returns a dropdown column's allowed values in display
// order (nil for a text column, or a dropdown with no options recorded).
func customColumnOptions(columnID int64) ([]string, error) {
	rows, err := dbcore.DB.Query("SELECT value FROM custom_column_options WHERE column_id=? ORDER BY position ASC", columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var opts []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		opts = append(opts, v)
	}
	return opts, rows.Err()
}

// customColumnType looks up a column's type by id, returning an error if the
// column does not exist.
func customColumnType(columnID int64) (string, error) {
	var t string
	err := dbcore.DB.QueryRow("SELECT type FROM custom_columns WHERE id=?", columnID).Scan(&t)
	if err == sql.ErrNoRows {
		return "", fmt.Errorf("custom column %d does not exist", columnID)
	}
	return t, err
}

// CreateCustomColumn inserts a new column definition and, for a dropdown type,
// its ordered option list (one row per line of the textarea the UI collects,
// blank lines dropped), in one transaction. name must be non-empty and unique;
// a dropdown type needs at least one non-blank option; a text column takes no
// options (any passed in are ignored).
func CreateCustomColumn(name, colType string, options []string) (int64, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return 0, fmt.Errorf("custom column name is required")
	}
	if !ValidColumnType(colType) {
		return 0, fmt.Errorf("invalid custom column type %q", colType)
	}

	var cleanOpts []string
	if isDropdownType(colType) {
		for _, o := range options {
			if o = strings.TrimSpace(o); o != "" {
				cleanOpts = append(cleanOpts, o)
			}
		}
		if len(cleanOpts) == 0 {
			return 0, fmt.Errorf("a dropdown column needs at least one value")
		}
	}

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	res, err := tx.Exec("INSERT INTO custom_columns (name, type, created_at) VALUES (?,?,?)", name, colType, dbcore.NowStamp())
	if err != nil {
		return 0, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, opt := range cleanOpts {
		if _, err := tx.Exec("INSERT INTO custom_column_options (column_id, position, value) VALUES (?,?,?)", id, i, opt); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// DeleteCustomColumn permanently removes a column definition. Every model's
// stored value for it, and its dropdown option list, cascade-delete with it
// (ON DELETE CASCADE on custom_column_options.column_id and
// custom_values.column_id) - deleting a column always wipes its data, with no
// way to recover it short of restoring a DB backup.
func DeleteCustomColumn(id int64) error {
	_, err := dbcore.DB.Exec("DELETE FROM custom_columns WHERE id=?", id)
	return err
}

// GetCustomValue returns a model's stored value for a custom column. ok is
// false when no value is stored (never set, or cleared back to unset).
func GetCustomValue(modelName string, columnID int64) (value string, ok bool, err error) {
	err = dbcore.DB.QueryRow("SELECT value FROM custom_values WHERE model_name=? AND column_id=?", modelName, columnID).Scan(&value)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return value, true, nil
}

// SetCustomValue stores a model's value for a custom column. An empty (or
// whitespace-only) value CLEARS it: the row is deleted rather than storing an
// empty string, so a model with no value for a column has no row at all
// (sparse storage), matching GetCustomValue/ListModelCustomValues. For a
// dropdown column, a non-empty value must be one of the column's defined
// options - this predefined-type system has no free-text dropdown entry.
func SetCustomValue(modelName string, columnID int64, value string) error {
	if modelName == "" {
		return fmt.Errorf("model name is required")
	}
	value = strings.TrimSpace(value)
	if value == "" {
		_, err := dbcore.DB.Exec("DELETE FROM custom_values WHERE model_name=? AND column_id=?", modelName, columnID)
		return err
	}

	colType, err := customColumnType(columnID)
	if err != nil {
		return err
	}
	if isDropdownType(colType) {
		opts, err := customColumnOptions(columnID)
		if err != nil {
			return err
		}
		valid := false
		for _, o := range opts {
			if o == value {
				valid = true
				break
			}
		}
		if !valid {
			return fmt.Errorf("value %q is not one of this column's allowed options", value)
		}
	}

	_, err = dbcore.DB.Exec(`INSERT INTO custom_values (column_id, model_name, value) VALUES (?,?,?)
		ON CONFLICT(column_id, model_name) DO UPDATE SET value=excluded.value`, columnID, modelName, value)
	return err
}

// ListModelCustomValues returns every custom column value stored for a model,
// keyed by column id. A column the model has no value for is simply absent
// from the map.
func ListModelCustomValues(modelName string) (map[int64]string, error) {
	rows, err := dbcore.DB.Query("SELECT column_id, value FROM custom_values WHERE model_name=?", modelName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[int64]string{}
	for rows.Next() {
		var colID int64
		var v string
		if err := rows.Scan(&colID, &v); err != nil {
			return nil, err
		}
		out[colID] = v
	}
	return out, rows.Err()
}

// AllCustomValues returns every model's custom-column values in one query,
// keyed by model name then column id. This is what hydrates the main models
// table in one shot (one query for every row, instead of one query per row via
// ListModelCustomValues, which stays the single-model helper used by the
// migration's data carry and by tests). A model with no custom values at all is
// simply absent from the outer map.
func AllCustomValues() (map[string]map[int64]string, error) {
	rows, err := dbcore.DB.Query("SELECT model_name, column_id, value FROM custom_values")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := map[string]map[int64]string{}
	for rows.Next() {
		var name string
		var colID int64
		var v string
		if err := rows.Scan(&name, &colID, &v); err != nil {
			return nil, err
		}
		if out[name] == nil {
			out[name] = map[int64]string{}
		}
		out[name][colID] = v
	}
	return out, rows.Err()
}
