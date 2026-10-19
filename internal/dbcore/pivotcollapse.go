package dbcore

import (
	"database/sql"
	"fmt"
	"log"
	"sort"
)

// This file is migration v40: the collapse of the columns that state one
// attribute onto a single column. It merges mechanically; nothing here is a
// judgement call the migration makes on its own.
//
// WHAT IT TOUCHES.
//   - cells: any cell still carrying the empty string v39 could not have
//     backfilled - because a binary that predates cells.source wrote it after
//     v39, and so it took the column default instead of a real source - is
//     restamped with the source its own column's namespace implies, the same
//     mapping v39 used. Then cells is rebuilt onto the wide
//     primary key (column_id, model_name, source), because a merged column's
//     cells can otherwise collide - two sources' answers to one attribute
//     become two rows sharing the old (column_id, model_name) key the moment
//     they land on one column_id. Without the restamp, two members of one
//     merge group each holding an unsourced cell for the same model collide
//     on the empty string itself, which is the failure this step exists to
//     stop.
//   - columns: every attribute stated by more than one column keeps only the
//     lowest-position column - the survivor - moves every other column's cells
//     onto it and drops the rest. The survivor KEEPS its namespaced key here: the floor schema's
//     own CHECK (column_key LIKE namespace || '.%') is still live at this
//     point in the chain, so a bare key would fail it. v41's columns rebuild,
//     which drops that CHECK, is what de-prefixes every column - survivors
//     included - in one pass. A reader's own column that holds cells the
//     survivor cannot take (another type or unit) is not merged: it is renamed
//     apart and kept whole (detachOwnColumnsThatDisagree).
//   - column_sources: filled for every column, not only the merged ones - a
//     column with no row there admits no source at all once v41's write-guard
//     trigger is in place, so every attribute the collapse never touched
//     still needs its own source order recorded too. Every column, merged or
//     singleton, gets a hand row at rank 0 and its original source after it at
//     rank 1, where that source is a real one distinct from hand. added is the
//     one exception: its only source is the app itself, at rank 0, no hand row.
//   - The merged columns price_unit, input_modalities, output_modalities,
//     description, created_at and repository become visible;
//     price_image_output stays hidden.
//   - The active_parameters cells move from the huggingface source to hand, so
//     the importer can never overwrite a reader's own figure.
//   - added's cells are stamped with the app source.
//   - input_modalities, output_modalities and collections take resolve='union'.
//
// No cell is dropped: cellsMayFall is not set, and migrate() checks that.
func theColumnsCollapseOntoOnePerAttribute(tx *sql.Tx) error {
	held, err := tableExists(tx, "columns")
	if err != nil {
		return err
	}
	if !held {
		return nil // no cell store yet, nothing to collapse
	}

	if err := restampUnsourcedCells(tx); err != nil {
		return err
	}
	if err := widenCellsPrimaryKey(tx); err != nil {
		return err
	}
	if err := collapseMergedColumns(tx); err != nil {
		return err
	}
	if err := fillColumnSourcesForSingletons(tx); err != nil {
		return err
	}
	if err := reassignActiveParametersSource(tx); err != nil {
		return err
	}
	if err := stampAddedSource(tx); err != nil {
		return err
	}
	if err := setUnionResolve(tx); err != nil {
		return err
	}
	return nil
}

// restampUnsourcedCells corrects a cell that reaches this migration with an
// empty source. v39's own backfill (backfillCellSources, pivotexpand.go) only
// ran once, against whatever cells already existed; a binary that predates
// cells.source and runs afterwards does not know it exists, so every cell it
// writes takes the column's default, an empty string. Left alone, two such cells
// belonging to different members of one merge group collide on that shared empty
// string the moment widenCellsPrimaryKey below makes source part of the key -
// which is the exact failure this restamp exists to stop. It reuses
// sourceForNamespace (pivotexpand.go), the same mapping v39's own backfill
// used, so an empty cell ends up sourced exactly as v39 would have sourced it
// had it existed when v39 ran.
func restampUnsourcedCells(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT DISTINCT namespace FROM columns
		WHERE id IN (SELECT column_id FROM cells WHERE source = '')`)
	if err != nil {
		return fmt.Errorf("find which namespaces hold an unsourced cell: %w", err)
	}
	var namespaces []string
	for rows.Next() {
		var ns string
		if err := rows.Scan(&ns); err != nil {
			rows.Close()
			return err
		}
		namespaces = append(namespaces, ns)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	var restamped int64
	for _, ns := range namespaces {
		res, err := tx.Exec(`UPDATE cells SET source = ? WHERE source = '' AND column_id IN (
			SELECT id FROM columns WHERE namespace = ?)`, sourceForNamespace(ns), ns)
		if err != nil {
			return fmt.Errorf("restamp unsourced %s cell(s): %w", ns, err)
		}
		n, err := res.RowsAffected()
		if err != nil {
			return err
		}
		restamped += n
	}
	if restamped > 0 {
		log.Printf("Migration v40: restamped %d cell(s) that reached this migration with no source", restamped)
	}
	return nil
}

// widenCellsPrimaryKey rebuilds cells from (column_id, model_name) to
// (column_id, model_name, source), by create-new/copy/drop-old/rename-new -
// SQLite cannot ALTER a primary key. This migration runs with
// disableForeignKeys set (see migrate.go), because cells is DROPped mid-way
// and every other table's foreign key points at columns, not at cells, so
// nothing but cells itself is at risk here - but the drop still needs foreign
// keys off for the same reason any DROP TABLE does under this schema.
func widenCellsPrimaryKey(tx *sql.Tx) error {
	if _, err := tx.Exec(`CREATE TABLE cells_wide (
		column_id    INTEGER NOT NULL REFERENCES columns(id) ON DELETE CASCADE,
		model_name   TEXT    NOT NULL REFERENCES models(name) ON DELETE CASCADE,
		source       TEXT    NOT NULL DEFAULT '',
		value_text   TEXT    NOT NULL,
		value_number REAL,
		written_at   TEXT    NOT NULL DEFAULT '',
		PRIMARY KEY (column_id, model_name, source),
		CHECK (value_text <> '')
	)`); err != nil {
		return fmt.Errorf("create the wide-key cells table: %w", err)
	}
	if _, err := tx.Exec(`INSERT INTO cells_wide (column_id, model_name, source, value_text, value_number, written_at)
		SELECT column_id, model_name, source, value_text, value_number, written_at FROM cells`); err != nil {
		return fmt.Errorf("copy cells onto the wide key: %w", err)
	}
	if _, err := tx.Exec(`DROP TABLE cells`); err != nil {
		return fmt.Errorf("drop the narrow-key cells table: %w", err)
	}
	if _, err := tx.Exec(`ALTER TABLE cells_wide RENAME TO cells`); err != nil {
		return fmt.Errorf("rename the wide-key table into place: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX idx_cells_model  ON cells(model_name)`); err != nil {
		return fmt.Errorf("recreate idx_cells_model: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX idx_cells_number ON cells(column_id, value_number) WHERE value_number IS NOT NULL`); err != nil {
		return fmt.Errorf("recreate idx_cells_number: %w", err)
	}
	if _, err := tx.Exec(`CREATE INDEX idx_cells_text   ON cells(column_id, value_text COLLATE NOCASE)`); err != nil {
		return fmt.Errorf("recreate idx_cells_text: %w", err)
	}
	return nil
}

// mergeTargetColumns are the attributes that had no shown column before the
// merge; the survivor in each becomes visible once its cells are backed by every
// source that held its own column.
// price_image_output is deliberately absent: it stays hidden, like every
// other price column that is not price_prompt or price_completion.
var mergeTargetColumns = map[string]bool{
	"price_unit":        true,
	"input_modalities":  true,
	"output_modalities": true,
	"description":       true,
	"created_at":        true,
	"repository":        true,
}

// mergeColumn is one row of columns, read wholesale so the collapse can group
// and sort them in Go rather than in a correlated SQL query.
type mergeColumn struct {
	id        int64
	key       string
	namespace string
	position  int
	valueType string
	unit      string
}

// bareAttribute strips a column's namespace prefix - the CHECK on columns
// guarantees every key still starts with "namespace." at this point in the
// migration, because v41, not this one, is what removes the CHECK.
func bareAttribute(key, namespace string) string {
	return key[len(namespace)+1:]
}

// collapseMergedColumns groups every column by its bare attribute name, and
// for every group of two or more, keeps the lowest-position member (the
// survivor), moves every other member's cells onto it, records the source order
// (ascending position), makes it visible where mergeTargetColumns says to, and
// drops the columns it absorbed. The survivor's key is left
// namespaced; v41's rebuild de-prefixes it along with every other column.
func collapseMergedColumns(tx *sql.Tx) error {
	rows, err := tx.Query("SELECT id, column_key, namespace, position, value_type, unit FROM columns ORDER BY position ASC")
	if err != nil {
		return err
	}
	var all []mergeColumn
	for rows.Next() {
		var c mergeColumn
		if err := rows.Scan(&c.id, &c.key, &c.namespace, &c.position, &c.valueType, &c.unit); err != nil {
			rows.Close()
			return err
		}
		all = append(all, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	groups := map[string][]mergeColumn{}
	var order []string
	for _, c := range all {
		bare := bareAttribute(c.key, c.namespace)
		if _, seen := groups[bare]; !seen {
			order = append(order, bare)
		}
		groups[bare] = append(groups[bare], c)
	}

	merged := 0
	for _, bare := range order {
		members := groups[bare]
		if len(members) < 2 {
			continue
		}
		sort.Slice(members, func(i, j int) bool { return members[i].position < members[j].position })
		members, err = detachOwnColumnsThatDisagree(tx, members)
		if err != nil {
			return err
		}
		if len(members) < 2 {
			continue
		}
		survivor := members[0]
		others := members[1:]

		// The silent-merge guard (the fault it prevents is "author" meaning the
		// vendor on one row and the repository's republisher on the next): every
		// member of a group must state the SAME fact in the SAME unit,
		// holding the SAME kind of value, or moving one member's cells onto
		// another turns the survivor into two incomparable figures wearing
		// one column. Label is exempt - price_image_output's two members
		// disagree there ('Image output price' vs 'Image output $') and the
		// survivor's label already wins by construction.
		//
		// A mine member that holds no cell is exempt. Databases from schema 33 to
		// 36 carry the reader's own twin of a huggingface column (mine.parameters
		// and nine more) with an empty unit where the huggingface column states
		// one, and no cell was ever typed into any of them. With nothing stored
		// there is no figure to be incomparable, and the member is absorbed like
		// any other. A mine member that does hold a cell still has to agree,
		// because moving it would relabel the reader's own number.
		for _, other := range others {
			if other.valueType == survivor.valueType && other.unit == survivor.unit {
				continue
			}
			if other.namespace == "mine" {
				var held int
				if err := tx.QueryRow("SELECT COUNT(*) FROM cells WHERE column_id=?", other.id).Scan(&held); err != nil {
					return fmt.Errorf("count the cells %s holds: %w", other.key, err)
				}
				if held == 0 {
					continue
				}
			}
			return fmt.Errorf("cannot merge %s into %s: %s holds %s (%q) and %s holds %s (%q) - two columns stating one attribute must agree on value type and unit",
				other.key, bare, survivor.key, survivor.valueType, survivor.unit, other.key, other.valueType, other.unit)
		}

		// A clean slate: v39 may already have recorded a source order for a
		// derived survivor from its stored recipe, in the recipe's own order -
		// which does not always match ascending position (price_prompt and
		// price_completion are the two cases where it does not). Rebuilding in
		// ascending position makes every group agree regardless of what a recipe
		// happened to say.
		if _, err := tx.Exec("DELETE FROM column_sources WHERE column_id=?", survivor.id); err != nil {
			return fmt.Errorf("clear the stored source order for %s before rebuilding it: %w", bare, err)
		}

		// Every column ends v40 with a hand row at rank 0: a hand answer beats
		// every fetched answer as a property of the data, not an accident of
		// whether a mine.* column happened to be a member of this group. No merge
		// group survives on a mine column (every mine member holds zero cells), so
		// this never collides with a real value.
		if _, err := tx.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,'hand',0)", survivor.id); err != nil {
			return fmt.Errorf("record %s's hand source at rank 0: %w", bare, err)
		}

		rank := 1
		if survivor.namespace != "mine" && survivor.namespace != "derived" {
			// A derived survivor's own cells are stamped 'rule', which is never a
			// source a column may be read FROM (see pivotexpand.go) - only its
			// merged-in inputs are registered as sources for it. A mine survivor's
			// own source is hand, already recorded above.
			if _, err := tx.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,?)",
				survivor.id, sourceForNamespace(survivor.namespace), rank); err != nil {
				return fmt.Errorf("record %s's own source: %w", bare, err)
			}
			rank++
		}

		for _, other := range others {
			if _, err := tx.Exec("UPDATE cells SET column_id=? WHERE column_id=?", survivor.id, other.id); err != nil {
				return fmt.Errorf("move %s's cells onto %s: %w", other.key, bare, err)
			}
			// A merged-in derived column's cells were computed, not read from a
			// source column_sources can name - 'rule' is deliberately never
			// seeded into sources (see pivotexpand.go). No merge group puts a
			// derived column anywhere but the survivor slot, so this branch is
			// not expected to run against real data; it is here so a group that
			// did would drop the value's PLACE in the order rather than fail the
			// whole migration on an unseedable source name. A merged-in mine column's own source is hand,
			// already recorded above, so it takes no rank of its own either.
			if other.namespace != "derived" && other.namespace != "mine" {
				if _, err := tx.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,?)",
					survivor.id, sourceForNamespace(other.namespace), rank); err != nil {
					return fmt.Errorf("record %s's place in %s's source order: %w", other.key, bare, err)
				}
				rank++
			}

			// derived_inputs, derived_columns, column_options and column_sources
			// all reference a column being dropped - the first as one of its
			// inputs, the rest as facts about the column itself (a dropped column
			// can be a computed one whose own recipe and v39-backfilled source
			// order this deletes along with it). None of their DELETE ON CASCADE /
			// DELETE RESTRICT fires here: this migration runs with foreign keys
			// off, so nothing but an explicit delete removes them, and PRAGMA
			// foreign_key_check (run once this migration commits) would otherwise
			// catch the columns row as still referenced.
			if _, err := tx.Exec("DELETE FROM derived_inputs WHERE input_column_id=? OR column_id=?", other.id, other.id); err != nil {
				return fmt.Errorf("clear %s from any recipe, and any recipe of its own: %w", other.key, err)
			}
			if _, err := tx.Exec("DELETE FROM derived_columns WHERE column_id=?", other.id); err != nil {
				return fmt.Errorf("clear %s's own recipe metadata: %w", other.key, err)
			}
			if _, err := tx.Exec("DELETE FROM column_options WHERE column_id=?", other.id); err != nil {
				return fmt.Errorf("clear %s's own options: %w", other.key, err)
			}
			if _, err := tx.Exec("DELETE FROM column_sources WHERE column_id=?", other.id); err != nil {
				return fmt.Errorf("clear %s's own v39-backfilled source order: %w", other.key, err)
			}
			if _, err := tx.Exec("DELETE FROM cells WHERE column_id=?", other.id); err != nil {
				return fmt.Errorf("clear anything left behind in %s: %w", other.key, err)
			}
			if _, err := tx.Exec("DELETE FROM columns WHERE id=?", other.id); err != nil {
				return fmt.Errorf("drop the merged column %s: %w", other.key, err)
			}
		}

		if mergeTargetColumns[bare] {
			if _, err := tx.Exec("UPDATE columns SET visible=1 WHERE id=?", survivor.id); err != nil {
				return fmt.Errorf("make %s visible: %w", bare, err)
			}
		}
		merged++
	}
	if merged > 0 {
		log.Printf("Migration v40: collapsed %d merge group(s) onto their survivor columns", merged)
	}
	return nil
}

// detachOwnColumnsThatDisagree keeps a reader's own column apart from the
// shipped column it shares a name with, when it holds a value the shipped
// column cannot take. A v1.29 to v1.30 catalog can carry a custom column named
// like a shipped one ("Parameters (B)", "Author") with text typed into it;
// moving that text onto a number column, or onto a column stating another
// unit, would relabel the reader's own figure, and refusing the whole
// migration would leave the reader with no way forward. The column stays a
// column of its own, with every cell it holds, under a key and a label that
// say it is the reader's. members is sorted by position; the columns that
// remain to be merged are returned. An own column that holds no cell is not
// detached: the merge absorbs it as it always has.
func detachOwnColumnsThatDisagree(tx *sql.Tx, members []mergeColumn) ([]mergeColumn, error) {
	var reference *mergeColumn
	for i := range members {
		if members[i].namespace != "mine" {
			reference = &members[i]
			break
		}
	}
	if reference == nil {
		return members, nil
	}
	kept := make([]mergeColumn, 0, len(members))
	for _, m := range members {
		if m.namespace != "mine" || (m.valueType == reference.valueType && m.unit == reference.unit) {
			kept = append(kept, m)
			continue
		}
		var held int
		if err := tx.QueryRow("SELECT COUNT(*) FROM cells WHERE column_id=?", m.id).Scan(&held); err != nil {
			return nil, fmt.Errorf("count the cells %s holds: %w", m.key, err)
		}
		if held == 0 {
			kept = append(kept, m)
			continue
		}
		if err := renameOwnColumnApart(tx, m); err != nil {
			return nil, err
		}
	}
	return kept, nil
}

// renameOwnColumnApart gives the reader's own column a key and a label no
// other column bears, so it stands beside the shipped column of the same name.
// The key keeps the "mine." prefix the schema still demands at this point in
// the chain; v41 strips it with every other.
func renameOwnColumnApart(tx *sql.Tx, m mergeColumn) error {
	var label string
	if err := tx.QueryRow("SELECT label FROM columns WHERE id=?", m.id).Scan(&label); err != nil {
		return fmt.Errorf("read the label of %s: %w", m.key, err)
	}
	newKey, newLabel := "", ""
	for n := 1; ; n++ {
		keySuffix, labelSuffix := "_mine", " (mine)"
		if n > 1 {
			keySuffix, labelSuffix = fmt.Sprintf("_mine_%d", n), fmt.Sprintf(" (mine %d)", n)
		}
		var clash int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM columns WHERE column_key=? OR label=? COLLATE NOCASE`,
			m.key+keySuffix, label+labelSuffix).Scan(&clash); err != nil {
			return fmt.Errorf("check the new name of %s is free: %w", m.key, err)
		}
		if clash == 0 {
			newKey, newLabel = m.key+keySuffix, label+labelSuffix
			break
		}
	}
	if _, err := tx.Exec("UPDATE columns SET column_key=?, label=? WHERE id=?", newKey, newLabel, m.id); err != nil {
		return fmt.Errorf("rename %s apart from the shipped column: %w", m.key, err)
	}
	log.Printf("Migration v40: kept your column %q apart from the shipped %s column, as %q, because it holds values that column cannot take", label, bareAttribute(m.key, m.namespace), newLabel)
	return nil
}

// fillColumnSourcesForSingletons gives every column the collapse never
// touched its own source order. Every one of them takes a hand row at rank 0,
// same as a merged survivor does in collapseMergedColumns, with its original
// source following at rank 1 where that source is a real one distinct from
// hand. The one exception is added: its only source is the app itself, and it
// takes no hand row at all.
func fillColumnSourcesForSingletons(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT id, column_key, namespace FROM columns
		WHERE id NOT IN (SELECT column_id FROM column_sources)`)
	if err != nil {
		return err
	}
	type col struct {
		id             int64
		key, namespace string
	}
	var cols []col
	for rows.Next() {
		var c col
		if err := rows.Scan(&c.id, &c.key, &c.namespace); err != nil {
			rows.Close()
			return err
		}
		cols = append(cols, c)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	rows.Close()

	for _, c := range cols {
		if c.key == "mine.added" {
			if _, err := tx.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,'app',0)", c.id); err != nil {
				return fmt.Errorf("record added's app source: %w", err)
			}
			continue
		}

		if _, err := tx.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,'hand',0)", c.id); err != nil {
			return fmt.Errorf("record column %d's hand source at rank 0: %w", c.id, err)
		}

		// A mine column's own source is hand, already recorded above. A
		// standalone derived column (not part of any merge group) has no source
		// of its own to add either: its cells are stamped 'rule', which is
		// deliberately never seeded into sources (pivotexpand.go), so it becomes
		// an ordinary hand column - hand at rank 0 and nothing else, matching
		// what collapseMergedColumns does for a merged-in mine or derived
		// member.
		if c.namespace == "mine" || c.namespace == "derived" {
			continue
		}
		if _, err := tx.Exec("INSERT INTO column_sources (column_id, source, rank) VALUES (?,?,1)",
			c.id, sourceForNamespace(c.namespace)); err != nil {
			return fmt.Errorf("record column %d's own source: %w", c.id, err)
		}
	}
	return nil
}

// reassignActiveParametersSource moves the active_parameters cells from the
// huggingface source to hand: a build's active-parameter count is read once by
// the variation importer and then belongs to whoever corrects it, never to a
// refresh that runs again later.
func reassignActiveParametersSource(tx *sql.Tx) error {
	id, ok, err := columnIDByKey(tx, "huggingface.active_parameters")
	if err != nil || !ok {
		return err // no such column: nothing to reassign
	}
	if _, err := tx.Exec("UPDATE cells SET source='hand' WHERE column_id=?", id); err != nil {
		return fmt.Errorf("reassign active_parameters' cells to the hand source: %w", err)
	}
	return nil
}

// stampAddedSource marks every added cell with the app source: the date a model
// entered this catalog is not a fact any source states, so its only source is
// the app itself.
func stampAddedSource(tx *sql.Tx) error {
	id, ok, err := columnIDByKey(tx, "mine.added")
	if err != nil || !ok {
		return err
	}
	if _, err := tx.Exec("UPDATE cells SET source='app' WHERE column_id=?", id); err != nil {
		return fmt.Errorf("stamp added's cells with the app source: %w", err)
	}
	return nil
}

// setUnionResolve marks input_modalities, output_modalities and collections
// as union columns: every source's answer contributes, rather than only the
// best-ranked one. All three still carry their namespace prefix at this
// point in the chain (collapseMergedColumns leaves it in place; see the file
// doc comment), so this matches on the bare attribute name either on its own
// or after a "namespace." prefix.
func setUnionResolve(tx *sql.Tx) error {
	for _, bare := range []string{"input_modalities", "output_modalities", "collections"} {
		if _, err := tx.Exec("UPDATE columns SET resolve='union' WHERE column_key=? OR column_key LIKE ?",
			bare, "%."+bare); err != nil {
			return fmt.Errorf("set %s to union resolve: %w", bare, err)
		}
	}
	return nil
}
