package dbcore

import (
	"database/sql"
	"errors"
	"fmt"
	"log"
)

// This file is migration v34: every model carries the date it entered this
// catalog.
//
// WHY A MIGRATION IS THE ONLY WAY TO FILL IT. The date is written at the moment
// a row is created, and no source states it, so a row that is already here has
// nothing left to read it off. The true history was never recorded anywhere -
// models.updated_at is the last write, not the first - so a carried-forward
// catalog is given ONE date, said out loud rather than invented per row: the day
// this column arrived.
//
// WHAT IT TOUCHES. It creates one column and writes one cell per model that has
// none. It reads no other column, rewrites no value, and drops nothing, so a
// database comes through it holding everything it went in with plus one value a
// row. Re-running it writes nothing: a model that already carries the date is
// passed over, which is also what makes it safe on a catalog where the app has
// already stamped some rows itself.

// The column this migration creates, mirroring the shipped definition in
// store.shippedColumns - store sits above dbcore in the import DAG
// (paths -> shared -> dbcore -> store), so it cannot be read from here. Change
// both together; store's TestTheAddedColumnShipsLastAndSealed holds them in step
// by reading these values, which is why they are exported.
const (
	AddedColumnKey   = "mine.added"
	AddedColumnLabel = "Added"
	AddedColumnAbout = "When this model first entered your catalog. It is not a listing date: no source states it, and nothing can work it out again, so it is written once when the row is created and never rewritten."
)

// AddedBackfillDate is the date every model already in a catalog when the column
// was added is recorded as having arrived on: the day the column shipped. It is
// one date for all of them because the real one was never stored, and inventing a
// different plausible date per row would read as a record of something rather
// than as the one fact it is.
const AddedBackfillDate = "2026-08-16T00:00:00Z"

// theAddedColumnRecordsWhenAModelArrived is migration v34.
func theAddedColumnRecordsWhenAModelArrived(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store, so there is no column to add one to
	}
	var models int
	if err := tx.QueryRow("SELECT COUNT(*) FROM models").Scan(&models); err != nil {
		return fmt.Errorf("count the models: %w", err)
	}
	if models == 0 {
		// A catalog with no models has no date to record, and creating the column
		// here would be creating it in the wrong place: this runs BEFORE the seed,
		// which is what puts every shipped column at its own position, and a column
		// already there is left exactly where it was found. So an empty catalog -
		// which is what a fresh install is - lets the seed do it.
		return nil
	}

	id, err := addedColumnID(tx)
	if err != nil {
		return err
	}
	res, err := tx.Exec(`INSERT INTO cells (column_id, model_name, value_text, value_number, written_at)
		SELECT ?, m.name, ?, NULL, ?
		FROM models m
		WHERE NOT EXISTS (SELECT 1 FROM cells c WHERE c.column_id=? AND c.model_name=m.name)`,
		id, AddedBackfillDate, AddedBackfillDate, id)
	if err != nil {
		return fmt.Errorf("record when each model arrived: %w", err)
	}
	if n, err := res.RowsAffected(); err == nil && n > 0 {
		log.Printf("Migration v34: %d model(s) recorded as having arrived on %s", n, AddedBackfillDate)
	}
	return nil
}

// addedColumnID returns the id of the Added column, creating it when it is not
// there yet.
//
// IT IS CREATED LAST AMONG THE COLUMNS THE APP SHIPS, one place past the highest
// position any of them holds, because that is where a fresh install's seed puts
// it too - it is the final entry of store.shippedColumns, and a seed's position
// is that list's own order. The reader's own columns sit past all of them and
// are not counted, or this one would land after those on a catalog carried
// forward and before them on a fresh install.
//
// SEALED, and hidden. Sealed is protection rather than tidiness: deleting a
// column permanently deletes every model's value for it, and these are dates
// that no source, no refresh and no import can ever state again.
func addedColumnID(tx *sql.Tx) (int64, error) {
	var id int64
	err := tx.QueryRow("SELECT id FROM columns WHERE column_key=?", AddedColumnKey).Scan(&id)
	if err == nil {
		return id, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var last int
	if err := tx.QueryRow("SELECT COALESCE(MAX(position), -1) FROM columns WHERE sealed=1").Scan(&last); err != nil {
		return 0, fmt.Errorf("read where the shipped columns end: %w", err)
	}
	res, err := tx.Exec(`INSERT INTO columns
		(column_key, namespace, label, value_type, unit, about, position, visible, sealed, created_at, color_rules)
		VALUES (?, 'mine', ?, 'text', '', ?, ?, 0, 1, ?, '')`,
		AddedColumnKey, AddedColumnLabel, AddedColumnAbout, last+1, NowStamp())
	if err != nil {
		return 0, fmt.Errorf("create the %s column: %w", AddedColumnKey, err)
	}
	log.Printf("Migration v34: the %q column now records when a model entered your catalog", AddedColumnLabel)
	return res.LastInsertId()
}
