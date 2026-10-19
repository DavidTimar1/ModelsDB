// Reading a model's cells as a whole: every column at once (AllCells), the
// verbatim record a source sent (AllSourceRecords, RecordOf), and finding a model
// by an identifier one of its cells holds (FindNameByCell). GET /api/models
// reads AllCells and serves its bare column keys straight through
// (internal/app/process.go).

package store

import (
	"database/sql"
	"errors"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"sort"
	"strings"
)

// ModelIdentity is everything that stays on the model row: the handle every
// value hangs off, the route that created the row, and the two decisions the
// reader has made about their own table.
type ModelIdentity struct {
	Name      string
	Source    string
	Hidden    bool
	VariantOf string
}

// ListModelIdentities names every stored model, in the order the table holds
// them. It carries no fact about any of them: those are cells.
func ListModelIdentities() ([]ModelIdentity, error) {
	rows, err := dbcore.DB.Query("SELECT name, source, hidden, variant_of FROM models")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ModelIdentity
	for rows.Next() {
		var m ModelIdentity
		var hidden int
		if err := rows.Scan(&m.Name, &m.Source, &hidden, &m.VariantOf); err != nil {
			return nil, err
		}
		m.Hidden = hidden == 1
		out = append(out, m)
	}
	return out, rows.Err()
}

// IdentityOf answers one model's row, and whether there is one at all.
func IdentityOf(name string) (ModelIdentity, bool, error) {
	var m ModelIdentity
	var hidden int
	err := dbcore.DB.QueryRow("SELECT name, source, hidden, variant_of FROM models WHERE name=?", name).
		Scan(&m.Name, &m.Source, &hidden, &m.VariantOf)
	if errors.Is(err, sql.ErrNoRows) {
		return ModelIdentity{}, false, nil
	}
	if err != nil {
		return ModelIdentity{}, false, err
	}
	m.Hidden = hidden == 1
	return m, true, nil
}

// AllCells reads every stored model's WINNING cell values in one pass, keyed by
// model and then by column key. It is the batch form of ModelCells and answers
// exactly what that function would for each model in turn: the lowest-ranked
// source's value for an ordinary column, and for a union-resolve column every
// source's list merged - unless a hand answer stands, which is used whole
// rather than joining the merge. A model with no value at all is simply
// absent.
func AllCells() (map[string]map[string]string, error) {
	rows, err := dbcore.DB.Query(`SELECT a.model_name, c.id, c.column_key, c.resolve, a.source, a.value_text
		FROM answers a JOIN columns c ON c.id=a.column_id`)
	if err != nil {
		return nil, err
	}
	type winner struct {
		id                          int64
		key, resolve, source, value string
	}
	byModel := map[string][]winner{}
	for rows.Next() {
		var model string
		var w winner
		if err := rows.Scan(&model, &w.id, &w.key, &w.resolve, &w.source, &w.value); err != nil {
			rows.Close()
			return nil, err
		}
		byModel[model] = append(byModel[model], w)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Every union-resolve column's lists are read in ONE query and merged in
	// memory. A query per cell would queue for the database's single connection
	// once per cell, behind whatever the refresh is writing at that moment, which
	// stalled a read for ten seconds or more while Update models ran.
	unions, err := allUnionCells()
	if err != nil {
		return nil, err
	}

	out := make(map[string]map[string]string, len(byModel))
	for model, winners := range byModel {
		row := map[string]string{}
		for _, w := range winners {
			if w.resolve != ResolveUnion || w.source == string(SourceHand) {
				row[w.key] = w.value
				continue
			}
			row[w.key] = unions[unionCellKey{columnID: w.id, model: model}]
		}
		out[model] = row
	}
	return out, nil
}

// unionCellKey names one union-resolve cell: a column and the model it is on.
type unionCellKey struct {
	columnID int64
	model    string
}

// allUnionCells answers, for every union-resolve column and model, what
// unionOfColumnCells answers for that pair: every source's list merged into one
// JSON array, duplicates removed and order kept. The rows are read in the
// primary key's order (column, model, source), so a merge keeps the same order
// the per-cell read gives.
func allUnionCells() (map[unionCellKey]string, error) {
	rows, err := dbcore.DB.Query(`SELECT c.column_id, c.model_name, c.value_text
		FROM cells c JOIN columns col ON col.id = c.column_id
		WHERE col.resolve = ?
		ORDER BY c.column_id, c.model_name, c.source`, ResolveUnion)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	lists := map[unionCellKey][]string{}
	for rows.Next() {
		var key unionCellKey
		var value string
		if err := rows.Scan(&key.columnID, &key.model, &value); err != nil {
			return nil, err
		}
		lists[key] = dbcore.MergeArr(lists[key], dbcore.ParseArr(value))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	merged := make(map[unionCellKey]string, len(lists))
	for key, all := range lists {
		if len(all) > 0 {
			merged[key] = dbcore.ToJSONArr(all)
		}
	}
	return merged, nil
}

// AllSourceRecords reads the verbatim record every source sent, keyed by model
// and then by the source that sent it.
func AllSourceRecords() (map[string]map[string]string, error) {
	rows, err := dbcore.DB.Query("SELECT model_name, source, raw_json FROM source_records")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]map[string]string{}
	for rows.Next() {
		var model, source, raw string
		if err := rows.Scan(&model, &source, &raw); err != nil {
			return nil, err
		}
		if out[model] == nil {
			out[model] = map[string]string{}
		}
		out[model][source] = raw
	}
	return out, rows.Err()
}

// RecordOf picks the verbatim record to answer with for one model: the one sent
// by the route that created the row, and failing that whichever source has one.
func RecordOf(records map[string]string, source string) string {
	for _, src := range append([]string{source},
		string(SourceOpenRouter), string(SourceHuggingFace)) {
		if raw := records[src]; raw != "" {
			return raw
		}
	}
	return ""
}

// NamedID is one model and the upstream identifier it carries, which is what
// the ZDR check selects its work list by.
type NamedID struct{ Name, ID string }

// ModelsOfSourceWithAnID names every model created by that route which carries
// an OpenRouter identifier, in name order.
func ModelsOfSourceWithAnID(source string) ([]NamedID, error) {
	identities, err := ListModelIdentities()
	if err != nil {
		return nil, err
	}
	cells, err := AllCells()
	if err != nil {
		return nil, err
	}
	var out []NamedID
	for _, identity := range identities {
		if identity.Source != source {
			continue
		}
		if id := strings.TrimSpace(cells[identity.Name]["id"]); id != "" {
			out = append(out, NamedID{Name: identity.Name, ID: id})
		}
	}
	sort.Slice(out, func(a, b int) bool { return out[a].Name < out[b].Name })
	return out, nil
}

// wholeNumberOrNil reads a stored figure back as the whole number a caller
// compares, and answers nothing at all where nobody has stated one - which is
// what keeps "the source does not say" different from "the answer is zero".
func wholeNumberOrNil(text string) interface{} {
	figure, err := shared.ParseNumber(strings.TrimSpace(text))
	if err != nil {
		return nil
	}
	return int64(figure)
}

// stated is one column's text for this model, empty when nobody has said.
func stated(cells map[string]string, key string) string {
	return strings.TrimSpace(cells[key])
}

// FindNameByCell answers the name of the model holding that value in any of
// those columns, and whether one does.
//
// ignoringCapitals is per caller because the two identifiers are not compared
// the same way: a repository address is written in several capitalisations by
// the people and the sources that name it, and an upstream identifier is
// compared exactly.
//
// The order among rows sharing a value is fixed rather than left to SQLite, so
// the same catalog always answers with the same row: a variation - one
// downloadable build, filed under a model - sorts after the models, and then by
// name.
func FindNameByCell(keys []string, value string, ignoringCapitals bool) (string, bool, error) {
	if strings.TrimSpace(value) == "" || len(keys) == 0 {
		return "", false, nil
	}
	places := make([]string, len(keys))
	args := make([]interface{}, 0, len(keys)+1)
	for i, key := range keys {
		places[i] = "?"
		args = append(args, key)
	}
	compare := "v.value_text=?"
	if ignoringCapitals {
		compare = "v.value_text=? COLLATE NOCASE"
	}
	args = append(args, value)

	var name string
	err := dbcore.DB.QueryRow(`SELECT m.name FROM cells v
		JOIN columns c ON c.id=v.column_id
		JOIN models m ON m.name=v.model_name
		WHERE c.column_key IN (`+strings.Join(places, ",")+`) AND `+compare+`
		ORDER BY (m.variant_of<>''), m.name LIMIT 1`, args...).Scan(&name)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return name, true, nil
}

// repositoryColumns and upstreamIDColumns are the columns each identifier can be
// found in. Every source that can state one writes the same bare column, so
// each is a column of one.
var (
	repositoryColumns = []string{"repository"}
	upstreamIDColumns = []string{"id"}
)
