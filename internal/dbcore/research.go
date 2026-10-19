package dbcore

import (
	"database/sql"
	"fmt"
	"log"
)

// This file is migration v46: the research source replaces hand on every
// column an automatic source can state.
//
// The rule the data model rests on: a value OpenRouter or a HuggingFace
// repository states is never typed over, and a value found to fill a gap - by the
// reader in the page, or by an assistant through the API - is a source of its
// own, RESEARCH, ranked below every automatic source so that the moment one
// states the fact, that is what shows. Before this migration every such column
// declared hand at the top of its order and a typed answer beat every refresh.
//
// WHAT IT TOUCHES.
//   - sources: rebuilt with 'research' allowed by the CHECK on kind, then the
//     research row inserted. SQLite cannot widen a CHECK with ALTER TABLE, so
//     this is create-new/copy/drop-old/rename-new on v41's pattern, and needs
//     disableForeignKeys because column_sources references sources(name).
//   - column_sources: research declared on every column that declares a
//     source of kind 'fetched', one rank past that column's highest, so it
//     lands last on both rank layouts in the wild (hand at -1 on a database
//     seeded fresh, hand at 0 on one v40 migrated). The hand rows on those
//     same columns are deleted once no hand cell is left under them.
//   - cells: every hand cell on one of those columns becomes a research cell
//     carrying the same text, sort key and written_at. The trigger
//     cells_source_never_moves refuses an UPDATE of cells.source, so the move
//     is an INSERT ... SELECT followed by a DELETE, and research has to be
//     declared before the insert or cells_source_is_declared refuses it.
//
// NO CELL IS LOST: cellsMayFall is not set, so the chain's own count guard
// proves the re-stamp kept every row. The active_parameters cells v40 stamped
// hand (pivotcollapse.go, reassignActiveParametersSource) come along: they are
// measurements, and ranking below a future re-import is right for a measurement.
func theResearchSourceReplacesHandOnFetchedColumns(tx *sql.Tx) error {
	held, err := tableExists(tx, "sources")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to re-stamp
	}

	if err := rebuildSourcesWithResearch(tx); err != nil {
		return err
	}
	declared, err := declareResearchOnFetchedColumns(tx)
	if err != nil {
		return err
	}
	moved, err := restampHandCellsAsResearch(tx)
	if err != nil {
		return err
	}
	dropped, err := dropHandFromFetchedColumns(tx)
	if err != nil {
		return err
	}
	if err := refuseIfAHandCellIsStranded(tx); err != nil {
		return err
	}
	log.Printf("Migration v46: research declared on %d column(s), %d hand cell(s) re-stamped as research, hand withdrawn from %d column(s)", declared, moved, dropped)
	return nil
}

// researchSource is the row v46 adds to sources: what fills a gap until an
// automatic source states the fact. Its kind is its own word, because a
// researched value is not fetched (nothing outside the app restates it), not
// static, and not the reader's own answer in a column of theirs.
var researchSource = pivotSource{"research", "Research", "research", 5}

// rebuildSourcesWithResearch widens the CHECK on sources.kind to admit
// 'research' and inserts the row.
func rebuildSourcesWithResearch(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE sources_new (
		name     TEXT    PRIMARY KEY NOT NULL,
		label    TEXT    NOT NULL,
		kind     TEXT    NOT NULL CHECK (kind IN ('fetched','hand','static','research')),
		position INTEGER NOT NULL DEFAULT 0
	)`); err != nil {
		return fmt.Errorf("create the widened sources table: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO sources_new (name, label, kind, position)
		SELECT name, label, kind, position FROM sources`); err != nil {
		return fmt.Errorf("copy the sources: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE sources`); err != nil {
		return fmt.Errorf("drop the old sources table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE sources_new RENAME TO sources`); err != nil {
		return fmt.Errorf("rename the widened sources table into place: %w", err)
	}
	if _, err := tx.Exec(`INSERT OR IGNORE INTO sources (name, label, kind, position) VALUES (?,?,?,?)`,
		researchSource.name, researchSource.label, researchSource.kind, researchSource.position); err != nil {
		return fmt.Errorf("seed the research source: %w", err)
	}
	return nil
}

// fetchedColumnsSQL selects the id of every column that declares a source of
// kind 'fetched': the columns an automatic source can state, and so the
// columns research may fill.
const fetchedColumnsSQL = `SELECT DISTINCT cs.column_id FROM column_sources cs
	JOIN sources s ON s.name = cs.source WHERE s.kind = 'fetched'`

// declareResearchOnFetchedColumns declares research one rank past each such
// column's highest declared rank, and answers how many columns it declared it
// on. MAX(rank)+1 cannot collide with any rank on either layout, and the
// start-up rewrite (store.DeclareShippedSources) renumbers from 0 afterwards.
func declareResearchOnFetchedColumns(tx *sql.Tx) (int64, error) {
	res, err := tx.Exec(`INSERT INTO column_sources (column_id, source, rank)
		SELECT cs.column_id, 'research', MAX(cs.rank) + 1
		FROM column_sources cs
		WHERE cs.column_id IN (` + fetchedColumnsSQL + `)
		  AND NOT EXISTS (SELECT 1 FROM column_sources r WHERE r.column_id = cs.column_id AND r.source = 'research')
		GROUP BY cs.column_id`)
	if err != nil {
		return 0, fmt.Errorf("declare research on the fetched columns: %w", err)
	}
	return res.RowsAffected()
}

// restampHandCellsAsResearch moves every hand cell on a column an automatic
// source can state onto the research source, text, sort key and written_at
// unchanged, and answers how many it moved. No (column, model, research) row
// exists before this, so the insert cannot collide.
func restampHandCellsAsResearch(tx *sql.Tx) (int64, error) {
	res, err := tx.Exec(`INSERT INTO cells (column_id, model_name, source, value_text, value_number, written_at)
		SELECT column_id, model_name, 'research', value_text, value_number, written_at
		FROM cells WHERE source = 'hand' AND column_id IN (` + fetchedColumnsSQL + `)`)
	if err != nil {
		return 0, fmt.Errorf("re-stamp the hand cells as research: %w", err)
	}
	moved, err := res.RowsAffected()
	if err != nil {
		return 0, err
	}
	del, err := tx.Exec(`DELETE FROM cells WHERE source = 'hand' AND column_id IN (` + fetchedColumnsSQL + `)`)
	if err != nil {
		return 0, fmt.Errorf("delete the hand cells that were re-stamped: %w", err)
	}
	deleted, err := del.RowsAffected()
	if err != nil {
		return 0, err
	}
	if deleted != moved {
		return 0, fmt.Errorf("re-stamped %d hand cell(s) as research but deleted %d, so the count would not stay level", moved, deleted)
	}
	return moved, nil
}

// dropHandFromFetchedColumns withdraws the hand declaration from every column
// an automatic source can state, and answers how many rows went. After
// restampHandCellsAsResearch no hand cell is left under any of them.
func dropHandFromFetchedColumns(tx *sql.Tx) (int64, error) {
	res, err := tx.Exec(`DELETE FROM column_sources WHERE source = 'hand' AND column_id IN (` + fetchedColumnsSQL + `)`)
	if err != nil {
		return 0, fmt.Errorf("withdraw hand from the fetched columns: %w", err)
	}
	return res.RowsAffected()
}

// refuseIfAHandCellIsStranded aborts the migration - which restores the
// pre-migration backup - if any hand cell sits on a column that no longer
// declares hand. Such a cell would rank last under the answers view and be
// unreachable from every write path, so it must not exist once this
// migration commits.
func refuseIfAHandCellIsStranded(tx *sql.Tx) error {
	var stranded int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM cells c WHERE c.source = 'hand'
		AND NOT EXISTS (SELECT 1 FROM column_sources cs WHERE cs.column_id = c.column_id AND cs.source = 'hand')`).Scan(&stranded); err != nil {
		return fmt.Errorf("count hand cells left under no declaration: %w", err)
	}
	if stranded > 0 {
		return fmt.Errorf("refusing to commit: %d hand cell(s) would be left on a column that no longer declares hand", stranded)
	}
	return nil
}
