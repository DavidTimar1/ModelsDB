// The cell store: one fact about one model, stored as one row.
//
// A CELL is a model's value in a column, stamped with the SOURCE that stated
// it (source.go). A COLUMN is a row of its own, carrying a permanent key
// (context_length), the label shown on screen, the kind of value it holds,
// and how a read resolves several sources' cells into one answer (Resolve).
// One column holds every source's answer to the same question as separate
// rows, keyed by (column_id, model_name, source), so a column is not duplicated
// once per source that can state it.
//
// EVERYTHING IN THIS FILE IS PERSONAL-ONLY IN THE SAME SENSE AS THE REST OF
// THE PACKAGE: a column, an option list and a cell live in modelsdb.db and are
// written to no other file, so this database is their only copy.
//
// writeOneCell is the ONE statement that writes a cell, and it is reached from
// two entries: WriteCells, for the automatic sources and the app's own stamp, and
// SaveCells (savecells.go) with the writers beside it, for what a person or an
// assistant sends over the network. Nothing else may insert, update or delete a
// row of cells (a migration aside), because that one statement is what holds a
// cell's sort key in step with its text - see sortKeyFor.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"sort"
	"strconv"
	"strings"
)

// The ways a read resolves several sources' cells for one column into one
// answer.
//
// These exact strings are duplicated in the CHECK constraint on
// columns.resolve (internal/dbcore/pivotexpand.go) - dbcore sits below store
// in the import DAG (paths -> shared -> dbcore -> store), so it cannot import
// these constants. Change them together.
const (
	ResolveFirstStated = "first_stated"
	ResolveUnion       = "union"
)

// The kinds of value a column can hold.
//
// ValueTypeNumber and ValueTypeChoice are different things, and the
// difference decides how a cell sorts. A NUMBER column holds one figure and
// sorts as a quantity, so 9 comes below 10. A CHOICE column holds one label
// out of a fixed list and sorts by that label's POSITION in the list, so
// "fast, avg, slow" stays a scale instead of being reordered into an
// alphabet.
//
// These exact strings are duplicated in the CHECK constraint on
// columns.value_type (internal/dbcore/db.go, cellStoreSQL); change them
// together.
const (
	ValueTypeText   = "text"
	ValueTypeNumber = "number"
	ValueTypeFlag   = "flag"
	ValueTypeChoice = "choice"
	ValueTypeList   = "list"
)

// The answers a caller gets when the cell store will not do what was asked.
// Each one is a different reply over HTTP, so each is its own value rather than
// a message a handler has to read.
var (
	ErrNoSuchColumn    = errors.New("no column with that key")
	ErrColumnKey       = errors.New("that is not a usable column key")
	ErrColumnIsASource = errors.New("that column belongs to a source")
	ErrCellValue       = errors.New("that column will not hold that value")
)

// Column is one column definition: what it is called, what kind of value it
// holds, how a read resolves several sources' cells for it, and where it
// sits.
//
// Key is the identity and Label is the presentation, and they are
// deliberately separate: the key never changes and never appears on screen,
// so relabelling a column costs nothing.
type Column struct {
	ID        int64    `json:"-"`
	Key       string   `json:"key"`
	Label     string   `json:"label"`
	ValueType string   `json:"value_type"`
	Resolve   string   `json:"resolve"`
	Unit      string   `json:"unit,omitempty"`
	About     string   `json:"about,omitempty"`
	Position  int      `json:"position"`
	Visible   bool     `json:"visible"`
	Sealed    bool     `json:"sealed"`
	CreatedAt string   `json:"created_at"`
	Options   []string `json:"options,omitempty"`
	// What colour this column's cells are painted, by the value they hold. Only a
	// number, a flag and a choice column can carry one; see colors.go.
	Colors *ColorRules `json:"color_rules,omitempty"`
}

// ColumnSpec is what creating a column takes. Only Key, Label and ValueType are
// required; a choice column also needs its Options, in the order they are to be
// offered and sorted in. Resolve is left empty for the ordinary case (the
// first cell by source priority wins); set it to ResolveUnion for a column
// whose answer is every source's list merged together.
type ColumnSpec struct {
	Key       string
	Label     string
	ValueType string
	Resolve   string
	Unit      string
	About     string
	Position  int
	Hidden    bool // a column created out of sight, which the column chooser can show
	Sealed    bool // a column the app ships: it can never be deleted or re-typed
	Options   []string
	Colors    *ColorRules // what colour its cells are painted, where it takes colours
}

// ValidValueType reports whether t is one of the kinds of value a column holds.
func ValidValueType(t string) bool {
	switch t {
	case ValueTypeText, ValueTypeNumber, ValueTypeFlag, ValueTypeChoice, ValueTypeList:
		return true
	}
	return false
}

// reservedColumnKeys are the words a request already gives a meaning besides a
// column key: the query parameters that are not columns and the parts of a
// row that are not cells. A column under one of them could never be filtered by,
// or fields= would answer the identity field instead of the column.
var reservedColumnKeys = map[string]bool{
	"name": true, "include_hidden": true, "hide_free": true, "links": true, "fields": true,
	"source": true, "hidden": true, "variant_of": true, "_raw": true,
}

// checkColumnKeyNotReserved refuses a key a request reads as something else.
func checkColumnKeyNotReserved(key string) error {
	if reservedColumnKeys[key] {
		return fmt.Errorf("%w: %q is a word the query already uses for something else, so no column can be called that; choose another label", ErrColumnKey, key)
	}
	return nil
}

// IsReservedColumnKey reports whether key is a word no column may carry. It is
// exported so the query package's test can hold the words it reads before any
// column against this list.
func IsReservedColumnKey(key string) bool { return reservedColumnKeys[key] }

// CreateColumn adds a column definition, its ordered option list for a choice
// column, and the sources allowed to state it, all in one transaction. It
// answers the column as it was stored.
//
// Every column needs at least one declared source: that list is the second
// layer of the write guard (source.go), so a column with none could never be
// written to at all.
func CreateColumn(spec ColumnSpec, sources ...Source) (Column, error) {
	spec.Key = strings.TrimSpace(spec.Key)
	spec.Label = strings.TrimSpace(spec.Label)

	if spec.Key == "" {
		return Column{}, fmt.Errorf("%w: a column needs a key", ErrColumnKey)
	}
	if strings.Contains(spec.Key, ".") {
		return Column{}, fmt.Errorf("%w: %q contains a dot, and a column key carries no dot - who states it is recorded on the cell, not in the key", ErrColumnKey, spec.Key)
	}
	if err := checkColumnKeyNotReserved(spec.Key); err != nil {
		return Column{}, err
	}
	if spec.Label == "" {
		return Column{}, fmt.Errorf("a column needs a label")
	}
	if !ValidValueType(spec.ValueType) {
		return Column{}, fmt.Errorf("%q is not a kind of value a column holds: %s",
			spec.ValueType, quotedList([]string{ValueTypeText, ValueTypeNumber, ValueTypeFlag, ValueTypeChoice, ValueTypeList}))
	}
	if spec.Resolve != "" && spec.Resolve != ResolveFirstStated && spec.Resolve != ResolveUnion {
		return Column{}, fmt.Errorf("%q is not a way a column resolves several sources' answers: %s",
			spec.Resolve, quotedList([]string{ResolveFirstStated, ResolveUnion}))
	}
	if len(sources) == 0 {
		return Column{}, fmt.Errorf("%w: %s needs at least one source that can state it", ErrColumnKey, spec.Key)
	}
	for _, src := range sources {
		if err := validSource(src); err != nil {
			return Column{}, err
		}
	}
	options, err := cleanOptions(spec.ValueType, spec.Options)
	if err != nil {
		return Column{}, err
	}
	if spec.Colors != nil {
		if err := checkColorRules(Column{Key: spec.Key, ValueType: spec.ValueType, Options: options}, *spec.Colors); err != nil {
			return Column{}, err
		}
	}

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return Column{}, err
	}
	defer tx.Rollback()

	id, err := insertColumn(tx, spec, options)
	if err != nil {
		return Column{}, err
	}
	if err := declareColumnSources(tx, id, sources); err != nil {
		return Column{}, err
	}
	if err := tx.Commit(); err != nil {
		return Column{}, err
	}
	return ColumnByKey(spec.Key)
}

// insertColumn writes one column definition and its options inside a
// transaction, and answers the row's id. It is shared by CreateColumn and by
// the column a refresh makes from a feed path (feedfields.go).
func insertColumn(tx *sql.Tx, spec ColumnSpec, options []string) (int64, error) {
	visible := 1
	if spec.Hidden {
		visible = 0
	}
	sealed := 0
	if spec.Sealed {
		sealed = 1
	}
	colors := ""
	if spec.Colors != nil {
		text, err := colorRulesText(*spec.Colors)
		if err != nil {
			return 0, err
		}
		colors = text
	}
	resolve := spec.Resolve
	if resolve == "" {
		resolve = ResolveFirstStated
	}
	res, err := tx.Exec(`INSERT INTO columns
		(column_key, label, value_type, unit, about, position, visible, sealed, created_at, color_rules, resolve)
		VALUES (?,?,?,?,?,?,?,?,?,?,?)`,
		spec.Key, spec.Label, spec.ValueType, spec.Unit, spec.About,
		spec.Position, visible, sealed, dbcore.NowStamp(), colors, resolve)
	if err != nil {
		return 0, fmt.Errorf("create the column %s: %w", spec.Key, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return 0, err
	}
	for i, label := range options {
		if _, err := tx.Exec("INSERT INTO column_options (column_id, position, label) VALUES (?,?,?)", id, i, label); err != nil {
			return 0, fmt.Errorf("record the options of %s: %w", spec.Key, err)
		}
	}
	return id, nil
}

// declareColumnSources records which sources may state a column, and the rank
// a read prefers them in: THE RANK IS THE POSITION IN THE LIST, from 0, and no
// source gets a special number. The list is refused before a row is written
// when it breaks the shape the two kinds of column have: hand and app are
// declared alone (a column of the reader's own, or the app's stamp, is stated
// by nobody else), research only last (it fills a gap until an automatic
// source states the fact, so it can never outrank one), and no source twice.
//
// Every caller writes into a column that has no rows yet (CreateColumn,
// CreateFeedFieldColumn) or deletes the column's rows first in the same
// transaction (DeclareShippedSources), which is what keeps
// UNIQUE(column_id, rank) from colliding.
func declareColumnSources(tx *sql.Tx, columnID int64, sources []Source) error {
	if err := checkSourceList(sources); err != nil {
		return err
	}
	for rank, src := range sources {
		if _, err := tx.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,?)", columnID, string(src), rank); err != nil {
			return fmt.Errorf("declare %s as a source of column %d: %w", src, columnID, err)
		}
	}
	return nil
}

// checkSourceList is the shape rule declareColumnSources refuses on, worded
// for the person who wrote the list.
func checkSourceList(sources []Source) error {
	if len(sources) == 0 {
		return fmt.Errorf("%w: a column needs at least one source that can state it", ErrColumnKey)
	}
	seen := map[Source]bool{}
	for i, src := range sources {
		if err := validSource(src); err != nil {
			return err
		}
		if seen[src] {
			return fmt.Errorf("%w: %s is declared twice", ErrColumnKey, src)
		}
		seen[src] = true
		if (src == SourceHand || src == SourceApp) && len(sources) > 1 {
			return fmt.Errorf("%w: %s is declared alone or not at all, and this list names %d sources", ErrColumnKey, src, len(sources))
		}
		if src == SourceResearch && i != len(sources)-1 {
			return fmt.Errorf("%w: research is declared last, because it fills a gap only until an automatic source states the fact", ErrColumnKey)
		}
	}
	return nil
}

// declaredSources lists the sources a column may be stated by.
func declaredSources(tx *sql.Tx, columnID int64) ([]Source, error) {
	rows, err := tx.Query("SELECT source FROM column_sources WHERE column_id=?", columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Source
	for rows.Next() {
		var src string
		if err := rows.Scan(&src); err != nil {
			return nil, err
		}
		out = append(out, Source(src))
	}
	return out, rows.Err()
}

// ListColumnSources answers every column's declared sources in rank order,
// keyed by column key, from one query. GET /api/columns carries it so the page
// learns who declares a column in the same request that lists the columns,
// and the query grammar's by: filter checks a source name against it.
func ListColumnSources() (map[string][]Source, error) {
	rows, err := dbcore.DB.Query(`SELECT c.column_key, cs.source FROM column_sources cs
		JOIN columns c ON c.id = cs.column_id ORDER BY c.column_key ASC, cs.rank ASC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string][]Source{}
	for rows.Next() {
		var key, src string
		if err := rows.Scan(&key, &src); err != nil {
			return nil, err
		}
		out[key] = append(out[key], Source(src))
	}
	return out, rows.Err()
}

// sourceDeclaredFor reports whether src may state that column.
func sourceDeclaredFor(tx *sql.Tx, columnID int64, src Source) (bool, error) {
	var n int
	if err := tx.QueryRow("SELECT COUNT(*) FROM column_sources WHERE column_id=? AND source=?", columnID, string(src)).Scan(&n); err != nil {
		return false, err
	}
	return n > 0, nil
}

// cleanOptions drops blank option lines and refuses the two shapes that cannot
// mean anything: a choice column with no options to choose from, and an option
// list on a column that offers no choice.
func cleanOptions(valueType string, options []string) ([]string, error) {
	var clean []string
	for _, o := range options {
		if o = strings.TrimSpace(o); o != "" {
			clean = append(clean, o)
		}
	}
	if valueType == ValueTypeChoice {
		if len(clean) == 0 {
			return nil, fmt.Errorf("a choice column needs at least one option, and the order they are written in is the order it sorts in")
		}
		seen := map[string]bool{}
		for _, o := range clean {
			if seen[o] {
				return nil, fmt.Errorf("the option %q is offered twice, and a cell holding it could not say which one it meant", o)
			}
			seen[o] = true
		}
		return clean, nil
	}
	if len(clean) > 0 {
		return nil, fmt.Errorf("only a choice column has options, and this one is %s", valueType)
	}
	return nil, nil
}

// ColumnByKey returns one column definition, with its options in order.
func ColumnByKey(key string) (Column, error) {
	tx, err := dbcore.DB.Begin()
	if err != nil {
		return Column{}, err
	}
	defer tx.Rollback()
	return columnByKey(tx, key)
}

// columnByKey is the one lookup, inside a transaction so that a write reads the
// definition it is about to write against.
func columnByKey(tx *sql.Tx, key string) (Column, error) {
	var c Column
	var visible, sealed int
	var colors string
	err := tx.QueryRow(`SELECT id, column_key, label, value_type, unit, about, position, visible, sealed, created_at, color_rules, resolve
		FROM columns WHERE column_key=?`, key).
		Scan(&c.ID, &c.Key, &c.Label, &c.ValueType, &c.Unit, &c.About, &c.Position, &visible, &sealed, &c.CreatedAt, &colors, &c.Resolve)
	if errors.Is(err, sql.ErrNoRows) {
		return Column{}, fmt.Errorf("%w: %q", ErrNoSuchColumn, key)
	}
	if err != nil {
		return Column{}, err
	}
	c.Visible, c.Sealed = visible == 1, sealed == 1
	c.Colors = colorRulesFrom(colors)
	if c.Options, err = columnOptions(tx, c.ID); err != nil {
		return Column{}, err
	}
	return c, nil
}

// columnOptions returns a choice column's labels in the order they were
// written, which is the order it sorts in. Any other kind of column has none.
func columnOptions(tx *sql.Tx, columnID int64) ([]string, error) {
	rows, err := tx.Query("SELECT label FROM column_options WHERE column_id=? ORDER BY position ASC", columnID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var label string
		if err := rows.Scan(&label); err != nil {
			return nil, err
		}
		out = append(out, label)
	}
	return out, rows.Err()
}

// ListColumns returns every column definition, in the order the table draws
// them: by position, then by key so two columns at the same position always come
// back the same way round.
func ListColumns() ([]Column, error) {
	tx, err := dbcore.DB.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT column_key FROM columns ORDER BY position ASC, column_key ASC`)
	if err != nil {
		return nil, err
	}
	var keys []string
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		keys = append(keys, key)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]Column, 0, len(keys))
	for _, key := range keys {
		c, err := columnByKey(tx, key)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

// DeleteColumn permanently removes a column nothing but hand states, together
// with its option list and every model's value for it (ON DELETE CASCADE on
// column_options.column_id, cells.column_id and column_sources.column_id).
// This database is the only copy of those values and nothing brings them
// back.
//
// A COLUMN AN AUTOMATIC SOURCE CAN STATE IS REFUSED, and hidden instead.
// OpenRouter and a HuggingFace repository state their columns again every
// time they are read, so a deletion would either come
// straight back or have to be remembered forever in a record of its own - and
// this app now keeps neither. Hiding says the same thing to the person
// looking at the table and keeps the values, so unhiding costs nothing.
func DeleteColumn(key string) error {
	tx, err := dbcore.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	column, err := columnByKey(tx, key)
	if err != nil {
		return err
	}
	if column.Sealed {
		return fmt.Errorf("%w: %s is a column this app ships, so it can be relabelled and hidden but never deleted. Hiding it keeps every value where it is",
			ErrColumnIsASource, column.Key)
	}
	sources, err := declaredSources(tx, column.ID)
	if err != nil {
		return err
	}
	for _, src := range sources {
		if fetchedSources[src] {
			return fmt.Errorf("%w: %s can be stated by %s, which states it again on every read, so it can be hidden but never deleted. Hide it instead, and its values stay where they are",
				ErrColumnIsASource, column.Key, src)
		}
	}
	if _, err := tx.Exec("DELETE FROM columns WHERE id=?", column.ID); err != nil {
		return err
	}
	return tx.Commit()
}

// storedCell is the source and the text of one stored cell, as automaticAnswer
// reads it back.
type storedCell struct{ source, value string }

// WriteCells is the entry through which the automatic sources and the app's own
// stamp write cells; SaveCells is the other, for what arrives over the network.
// Both end in writeOneCell, the only place a cell's sort key is worked out.
//
// It takes the SOURCE the caller is writing AS. That source must be declared
// in column_sources for every key it writes (source.go's second write-guard
// layer); a source outside a column's declared list is refused before
// anything is stored. cells' own primary key (column_id, model_name, source)
// is the first layer, and the cells_source_is_declared trigger
// (internal/dbcore/pivotcontract.go) repeats this same check inside the
// database as the third, so a write cannot land on a column its source was
// never given.
//
// AN EMPTY VALUE CLEARS THE CELL. The row is deleted rather than storing a
// blank, so a model with no value for a column has no row at all - which is what
// makes "nobody has said" and "somebody said nothing" the same answer, and it is
// how personal values already behave.
//
// A VALUE THE COLUMN'S TYPE WILL NOT TAKE IS A REFUSED WRITE, never a stored
// value with no sort key. The reason is that a value which fails quietly does
// not fail, it DELETES: an empty value means clear this cell, so text arriving
// where a number was meant would empty a figure somebody researched.
//
// The whole call is one transaction, so a refusal on the last key leaves the
// first ones unwritten too. A key naming no column is refused: nothing makes a
// column on the way past, because making columns is the reader's own act
// (CreateOwnColumn) or the refresh's (CreateFeedFieldColumn).
func WriteCells(src Source, modelName string, values map[string]string) error {
	if err := validSource(src); err != nil {
		return err
	}
	if hasNoName(modelName) {
		return ErrNoModelName
	}

	// Sorted, so a call carrying two values this refuses always names the same
	// one - a map is walked in a different order every time.
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if err := modelMustExist(tx, modelName); err != nil {
		return err
	}

	stamp := dbcore.NowStamp()
	for _, key := range keys {
		column, err := columnByKey(tx, key)
		if err != nil {
			return err
		}
		declared, err := sourceDeclaredFor(tx, column.ID, src)
		if err != nil {
			return err
		}
		if !declared {
			sources, err := declaredSources(tx, column.ID)
			if err != nil {
				return err
			}
			names := make([]string, 0, len(sources))
			for _, s := range sources {
				names = append(names, string(s))
			}
			return fmt.Errorf("%w: %s does not state %s. It is stated by %s. Nothing was written",
				ErrColumnKey, src, key, quotedList(names))
		}
		if err := writeOneCell(tx, column, src, modelName, values[key], stamp); err != nil {
			return err
		}
	}

	if err := stampModelWritten(tx, modelName, stamp); err != nil {
		return err
	}
	return tx.Commit()
}

// modelMustExist refuses a write onto a name this catalog holds no row for: a
// cell names its model by name, so there would be nothing to attach it to.
func modelMustExist(tx *sql.Tx, modelName string) error {
	var exists int
	if err := tx.QueryRow("SELECT COUNT(*) FROM models WHERE name=?", modelName).Scan(&exists); err != nil {
		return err
	}
	if exists == 0 {
		return fmt.Errorf("%w: %q", ErrNoSuchModel, modelName)
	}
	return nil
}

// stampModelWritten records on the row that something wrote to it, which every
// write path stamps.
func stampModelWritten(tx *sql.Tx, modelName, stamp string) error {
	_, err := tx.Exec("UPDATE models SET updated_at=? WHERE name=?", stamp, modelName)
	return err
}

// writeOneCell stores one value, or clears the cell when the value is empty. It
// is the only statement in this program that writes the cells table; WriteCells
// and SaveCells are its two callers.
func writeOneCell(tx *sql.Tx, column Column, src Source, modelName, value, stamp string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		_, err := tx.Exec("DELETE FROM cells WHERE column_id=? AND model_name=? AND source=?", column.ID, modelName, string(src))
		return err
	}
	sortKey, err := sortKeyFor(column, value)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, value_number, written_at)
		VALUES (?,?,?,?,?,?)
		ON CONFLICT(column_id, model_name, source) DO UPDATE SET
		  value_text=excluded.value_text, value_number=excluded.value_number, written_at=excluded.written_at`,
		column.ID, modelName, string(src), value, sortKey, stamp)
	return err
}

// sortKeyFor works out the number a cell SORTS by, from the text it holds and
// the kind of value its column declares. It is the whole of the typing rule, in
// one place, for a column the app ships and a column the reader invented
// alike.
//
// Text and a list have no sort key at all: text is ordered by its own letters,
// ignoring capitals, and a list is not ordered, it is searched for a member.
// A number sorts by the figure, a flag by 1 or 0, and a CHOICE by the option's
// POSITION in its list - which is what keeps "fast, avg, slow" a scale.
//
// A value it cannot convert is an error, never a missing sort key, because a
// cell with no sort key in a number column would sort as though nobody had
// answered.
func sortKeyFor(column Column, value string) (interface{}, error) {
	switch column.ValueType {
	case ValueTypeText, ValueTypeList:
		return nil, nil
	case ValueTypeNumber:
		figure, err := shared.ParseNumber(value)
		if err != nil {
			return nil, fmt.Errorf("%w: %s holds a number, and %q is not one", ErrCellValue, column.Key, value)
		}
		return figure, nil
	case ValueTypeFlag:
		switch strings.ToLower(value) {
		case "yes", "true", "1":
			return float64(1), nil
		case "no", "false", "0":
			return float64(0), nil
		}
		return nil, fmt.Errorf("%w: %s holds yes or no, and %q is neither", ErrCellValue, column.Key, value)
	case ValueTypeChoice:
		for position, label := range column.Options {
			if label == value {
				return float64(position), nil
			}
		}
		return nil, fmt.Errorf("%w: %s holds one of %s, and %q is not one of them",
			ErrCellValue, column.Key, quotedList(column.Options), value)
	}
	return nil, fmt.Errorf("%w: %s is declared as %q, which is not a kind of value this app stores",
		ErrCellValue, column.Key, column.ValueType)
}

// labelFromKey words a key as something to show at the top of a column:
// "context_length" becomes "Context length".
func labelFromKey(key string) string {
	name := strings.ReplaceAll(key, "_", " ")
	if name == "" {
		return key
	}
	return strings.ToUpper(name[:1]) + name[1:]
}

// ModelCells returns every column's WINNING value for one model, keyed by
// column key. A column the model has no value for at all is simply absent.
//
// THE WINNER IS THE answers VIEW'S ROW (lowest column_sources.rank), so this
// function never has to know which source a column prefers - EXCEPT for a
// union-resolve column, whose true answer is every source's list merged, not
// only the lowest-ranked source's list. A research cell on a union column
// ranks last, so it is either merged in or the only cell, and the merge of one
// cell is itself.
func ModelCells(modelName string) (map[string]string, error) {
	rows, err := dbcore.DB.Query(`SELECT c.id, c.column_key, c.resolve, a.source, a.value_text
		FROM answers a JOIN columns c ON c.id=a.column_id WHERE a.model_name=?`, modelName)
	if err != nil {
		return nil, err
	}
	type winner struct {
		id                          int64
		key, resolve, source, value string
	}
	var winners []winner
	for rows.Next() {
		var w winner
		if err := rows.Scan(&w.id, &w.key, &w.resolve, &w.source, &w.value); err != nil {
			rows.Close()
			return nil, err
		}
		winners = append(winners, w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := map[string]string{}
	for _, w := range winners {
		if w.resolve != ResolveUnion {
			out[w.key] = w.value
			continue
		}
		merged, err := unionOfColumnCells(w.id, modelName)
		if err != nil {
			return nil, err
		}
		out[w.key] = merged
	}
	return out, nil
}

// unionOfColumnCells merges every source's list for one column and model into
// one JSON array, duplicates removed and order kept.
func unionOfColumnCells(columnID int64, modelName string) (string, error) {
	rows, err := dbcore.DB.Query("SELECT value_text FROM cells WHERE column_id=? AND model_name=?", columnID, modelName)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var all []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return "", err
		}
		all = dbcore.MergeArr(all, dbcore.ParseArr(v))
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(all) == 0 {
		return "", nil
	}
	return dbcore.ToJSONArr(all), nil
}

// quotedList words a set of allowed words for a refusal a person reads.
func quotedList(items []string) string {
	quoted := make([]string, 0, len(items))
	for _, i := range items {
		quoted = append(quoted, strconv.Quote(i))
	}
	return strings.Join(quoted, ", ")
}
