// Following an OpenRouter rename: the refresh's one place that changes a row's
// name, and the one place that folds two rows holding one OpenRouter id into one.
//
// A row is STORED under its display name, and that name is the primary key, but
// the OpenRouter id is what a listing keeps when upstream renames it. So when a
// feed record names a model whose id this catalog already holds under another
// name, the row that holds the id is the same model: it takes the new name
// (a rename), or, when the new name already has a row of its own, it is folded
// into that row (a merge). Neither ever loses something the reader wrote.
//
// models.name is a foreign key target (cells and source_records cascade on it,
// with no ON UPDATE), so a plain UPDATE of the name is refused. A rename is
// therefore: insert the row under the new name, point everything at it, delete
// the old row.
//
// ONLY ROWS WHOSE models.source IS openrouter ARE EVER LOOKED AT. A row that
// came from another route (the old collection rows, a hand-typed or imported
// one) is never renamed, merged into or merged away, even when it holds the
// same id.
//
// This file moves and deletes cells rows, which WriteCells and SaveCells never
// do: a cell is carried to another model name here, and the cells_source_never_moves
// trigger does not object because its column and source stay the same.

package store

import (
	"database/sql"
	"fmt"
	"log"
	"modelsdb/internal/dbcore"
	"strings"
)

// Reconciliation says what ReconcileRenamedModel did with one feed record.
type Reconciliation struct {
	// Renamed is the old name of the row that now bears the feed's name; empty
	// when no row was renamed.
	Renamed string
	// Merged lists the names of the rows folded into the feed's row and gone.
	Merged []string
}

// Changed reports whether any row was renamed or merged away.
func (r Reconciliation) Changed() bool { return r.Renamed != "" || len(r.Merged) > 0 }

// sqlRunner is what *sql.DB and *sql.Tx both offer, so a plan is made the same
// way before and inside the transaction that carries it out.
type sqlRunner interface {
	Query(query string, args ...interface{}) (*sql.Rows, error)
	Exec(query string, args ...interface{}) (sql.Result, error)
}

// reconcilePlan is the work one feed record asks for.
type reconcilePlan struct {
	// rename is the old name when exactly one row holds the id and the feed's
	// name is free.
	rename string
	// merge lists the rows to fold into the feed's row.
	merge []string
}

func (p reconcilePlan) empty() bool { return p.rename == "" && len(p.merge) == 0 }

// ReconcileRenamedModel makes the catalog agree with one feed record's id and
// name before the record is upserted.
//
// beforeFirstChange is called once the record is known to need a rename or a
// merge and before anything is written; a refresh uses it to take a snapshot, and
// an error from it stops the change. It may be nil.
//
// THE CASES, all among rows whose models.source is openrouter:
//   - no other row holds the id: nothing to do;
//   - the feed's name is free and exactly one row holds the id: that row is
//     renamed;
//   - the feed's name is borne by a row holding the same id: every other row
//     holding the id is merged into it, unless two of them hold different hand
//     answers for one column, in which case none is merged and a warning names
//     the id and the column;
//   - the feed's name is borne by a row holding another id: nothing changes and
//     a warning says so;
//   - the feed's name is free but two or more rows hold the id: nothing changes
//     here; the upsert creates the named row and the next refresh merges.
func ReconcileRenamedModel(id, name string, beforeFirstChange func() error) (Reconciliation, error) {
	id = strings.TrimSpace(id)
	if id == "" || hasNoName(name) {
		return Reconciliation{}, nil
	}
	plan, err := planReconcile(dbcore.DB, id, name, true)
	if err != nil || plan.empty() {
		return Reconciliation{}, err
	}
	if beforeFirstChange != nil {
		if err := beforeFirstChange(); err != nil {
			return Reconciliation{}, fmt.Errorf("no snapshot was written, so %q was not reconciled: %w", name, err)
		}
	}

	tx, err := dbcore.DB.Begin()
	if err != nil {
		return Reconciliation{}, err
	}
	defer tx.Rollback()

	// Planned again inside the transaction: the plan outside it only decides
	// whether a snapshot is owed, and a write between the two must not be acted on
	// from a stale reading. The warnings were logged by the first reading.
	plan, err = planReconcile(tx, id, name, false)
	if err != nil || plan.empty() {
		return Reconciliation{}, err
	}
	var result Reconciliation
	if plan.rename != "" {
		if err := renameRow(tx, plan.rename, name); err != nil {
			return Reconciliation{}, err
		}
		result.Renamed = plan.rename
	}
	for _, stale := range plan.merge {
		if err := mergeRowInto(tx, stale, name); err != nil {
			return Reconciliation{}, err
		}
		result.Merged = append(result.Merged, stale)
	}
	if err := foreignKeysHold(tx); err != nil {
		return Reconciliation{}, err
	}
	if err := tx.Commit(); err != nil {
		return Reconciliation{}, err
	}
	return result, nil
}

// planReconcile reads which of the cases ReconcileRenamedModel lists applies. It
// writes nothing; it logs the two warnings when warn is set.
func planReconcile(q sqlRunner, id, name string, warn bool) (reconcilePlan, error) {
	holders, err := firstColumn(q, `SELECT m.name FROM models m
		JOIN cells v ON v.model_name=m.name AND v.source=?
		JOIN columns c ON c.id=v.column_id AND c.column_key=?
		WHERE m.source=? AND v.value_text=? ORDER BY m.name`,
		string(SourceOpenRouter), upstreamIDColumns[0], string(SourceOpenRouter), id)
	if err != nil {
		return reconcilePlan{}, err
	}
	var others []string
	namedHoldsID := false
	for _, h := range holders {
		if h == name {
			namedHoldsID = true
			continue
		}
		others = append(others, h)
	}
	if len(others) == 0 {
		return reconcilePlan{}, nil
	}

	bearers, err := firstColumn(q, "SELECT source FROM models WHERE name=?", name)
	if err != nil {
		return reconcilePlan{}, err
	}
	if len(bearers) == 0 {
		if len(others) == 1 {
			return reconcilePlan{rename: others[0]}, nil
		}
		return reconcilePlan{}, nil
	}
	if bearers[0] != string(SourceOpenRouter) {
		// The name is taken by a row from another route, which is never touched.
		return reconcilePlan{}, nil
	}
	if !namedHoldsID {
		if warn {
			log.Printf("Warning: OpenRouter id %q is held by %s, but the name %q the feed gives it belongs to a row with another id, so nothing was renamed or merged",
				id, quotedList(others), name)
		}
		return reconcilePlan{}, nil
	}
	args := append([]interface{}{string(SourceHand)}, stringArgs(holders)...)
	conflict, err := firstColumn(q, `SELECT col.column_key FROM cells c JOIN columns col ON col.id=c.column_id
		WHERE c.source=? AND c.model_name IN (`+placeholders(len(holders))+`)
		GROUP BY c.column_id HAVING COUNT(DISTINCT c.value_text)>1 ORDER BY col.column_key`, args...)
	if err != nil {
		return reconcilePlan{}, err
	}
	if len(conflict) > 0 {
		if warn {
			log.Printf("Warning: OpenRouter id %q is held by %s, which were not merged: they hold different answers of yours for %s",
				id, quotedList(holders), quotedList(conflict))
		}
		return reconcilePlan{}, nil
	}
	return reconcilePlan{merge: others}, nil
}

// renameRow gives the row old the name next: the new row is made first, then
// everything is pointed at it, then the old row goes.
func renameRow(tx sqlRunner, old, next string) error {
	steps := []struct {
		what, query string
		args        []interface{}
	}{
		{"make the row", `INSERT INTO models (name, source, variant_of, updated_at, hidden)
			SELECT ?, source, variant_of, ?, hidden FROM models WHERE name=?`, []interface{}{next, dbcore.NowStamp(), old}},
		{"move its cells", "UPDATE cells SET model_name=? WHERE model_name=?", []interface{}{next, old}},
		{"move its source records", "UPDATE source_records SET model_name=? WHERE model_name=?", []interface{}{next, old}},
		{"move its variations", "UPDATE models SET variant_of=? WHERE variant_of=?", []interface{}{next, old}},
		{"remove the old row", "DELETE FROM models WHERE name=?", []interface{}{old}},
	}
	return runSteps(tx, fmt.Sprintf("rename %q to %q", old, next), steps)
}

// mergeRowInto folds the row stale into the row survivor and deletes it.
//
//   - Stale's OpenRouter cells and record go: the survivor's own come from the
//     listing the refresh is about to write.
//   - Every other cell and record moves where the survivor has none for that
//     column and source; where it has one, the survivor's stays. The caller has
//     already established that no two hand answers differ.
//   - added keeps the older date, hidden stays set only if both were hidden, and
//     rows filed under stale are filed under the survivor.
func mergeRowInto(tx sqlRunner, stale, survivor string) error {
	steps := []struct {
		what, query string
		args        []interface{}
	}{
		{"keep the older added date", `UPDATE cells SET value_text=(
				SELECT o.value_text FROM cells o WHERE o.column_id=cells.column_id AND o.source=cells.source AND o.model_name=?)
			WHERE model_name=? AND source=? AND column_id=(SELECT id FROM columns WHERE column_key=?)
			AND EXISTS (SELECT 1 FROM cells o WHERE o.column_id=cells.column_id AND o.source=cells.source
				AND o.model_name=? AND o.value_text<cells.value_text)`,
			[]interface{}{stale, survivor, string(SourceApp), AddedColumnKey, stale}},
		{"move its cells", `UPDATE cells SET model_name=? WHERE model_name=? AND source<>?
			AND NOT EXISTS (SELECT 1 FROM cells o WHERE o.column_id=cells.column_id AND o.source=cells.source AND o.model_name=?)`,
			[]interface{}{survivor, stale, string(SourceOpenRouter), survivor}},
		{"move its source records", `UPDATE source_records SET model_name=? WHERE model_name=? AND source<>?
			AND NOT EXISTS (SELECT 1 FROM source_records o WHERE o.model_name=? AND o.source=source_records.source)`,
			[]interface{}{survivor, stale, string(SourceOpenRouter), survivor}},
		{"settle hidden", `UPDATE models SET hidden=MIN(hidden,(SELECT hidden FROM models WHERE name=?)), updated_at=? WHERE name=?`,
			[]interface{}{stale, dbcore.NowStamp(), survivor}},
		{"move its variations", "UPDATE models SET variant_of=? WHERE variant_of=?", []interface{}{survivor, stale}},
		{"remove the stale row", "DELETE FROM models WHERE name=?", []interface{}{stale}},
	}
	return runSteps(tx, fmt.Sprintf("merge %q into %q", stale, survivor), steps)
}

// runSteps executes each statement in order, naming the change and the step in
// the error of the first that fails.
func runSteps(tx sqlRunner, change string, steps []struct {
	what, query string
	args        []interface{}
}) error {
	for _, s := range steps {
		if _, err := tx.Exec(s.query, s.args...); err != nil {
			return fmt.Errorf("%s, %s: %w", change, s.what, err)
		}
	}
	return nil
}

// foreignKeysHold refuses to commit a change that left a row pointing at a model
// that is not there.
func foreignKeysHold(tx sqlRunner) error {
	rows, err := tx.Query("PRAGMA foreign_key_check")
	if err != nil {
		return err
	}
	defer rows.Close()
	broken := 0
	for rows.Next() {
		broken++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if broken > 0 {
		return fmt.Errorf("the change left %d row(s) pointing at a model that is not there, so it was not kept", broken)
	}
	return nil
}

// firstColumn answers the first column of every row a query returns, as text.
func firstColumn(q sqlRunner, query string, args ...interface{}) ([]string, error) {
	rows, err := q.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func stringArgs(values []string) []interface{} {
	out := make([]interface{}, len(values))
	for i, v := range values {
		out[i] = v
	}
	return out
}
