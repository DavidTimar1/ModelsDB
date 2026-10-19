// Who answered a cell.
//
// This is presentation support for GET /api/models (the answered_by and
// disputed maps) and for the panel that shows what every source said about
// one cell: GET /api/models/{name}/answers/{column}. It reads column_sources
// and sources alongside cells - nothing here writes a cell, and nothing here
// changes a column's order, which is code (shipped.go) and not stored data.

package store

import (
	"fmt"
	"modelsdb/internal/dbcore"
)

// handLabel is the reader-facing word for their own answer everywhere a
// source's label is shown outside the sources table itself: "You", never the
// DB's stored label "Typed by hand".
const handLabel = "You"

// neverDisputed names the columns where two sources differing is not a
// disagreement about the model: each source writes a description in its own
// words and dates a listing by its own calendar, so a mark there says nothing.
var neverDisputed = map[string]bool{"description": true, "created_at": true}

// AllProvenance reads every stored cell's column, source and rank in one pass
// and answers, for every model: which source WON each column (answeredBy -
// the same rule as answers/AllCells: lowest column_sources.rank, "merged" on
// a union column an automatic source contributed to, and the one source's
// own name where research alone fills a union column, because the merge of
// one cell is itself); which columns are DISPUTED, two automatic sources -
// OpenRouter and a repository - holding different text for that model
// (research, hand and app never dispute: research is simply outranked, and
// the other two share no column with an automatic source); and which columns
// hold a research cell that is OVERTAKEN, sitting beneath an automatic
// answer and no longer shown, which is what the page offers to clear.
func AllProvenance() (answeredBy map[string]map[string]string, disputed map[string]map[string]bool, overtaken map[string]map[string]bool, err error) {
	rows, err := dbcore.DB.Query(`SELECT c.model_name, col.column_key, col.resolve, c.source, c.value_text, cs.rank
		FROM cells c
		JOIN columns col ON col.id = c.column_id
		JOIN column_sources cs ON cs.column_id = c.column_id AND cs.source = c.source`)
	if err != nil {
		return nil, nil, nil, err
	}
	type cell struct {
		key, resolve, source, value string
		rank                        int
	}
	byModelColumn := map[string]map[string][]cell{}
	for rows.Next() {
		var model string
		var c cell
		if err := rows.Scan(&model, &c.key, &c.resolve, &c.source, &c.value, &c.rank); err != nil {
			rows.Close()
			return nil, nil, nil, err
		}
		if byModelColumn[model] == nil {
			byModelColumn[model] = map[string][]cell{}
		}
		byModelColumn[model][c.key] = append(byModelColumn[model][c.key], c)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, nil, err
	}

	answeredBy = map[string]map[string]string{}
	disputed = map[string]map[string]bool{}
	overtaken = map[string]map[string]bool{}
	for model, byColumn := range byModelColumn {
		for key, cells := range byColumn {
			winner := cells[0]
			for _, c := range cells[1:] {
				if c.rank < winner.rank {
					winner = c
				}
			}
			automatic := map[string]bool{}
			hasResearch := false
			for _, c := range cells {
				if Source(c.source).Kind() == "fetched" {
					automatic[c.value] = true
				}
				if c.source == string(SourceResearch) {
					hasResearch = true
				}
			}
			by := winner.source
			if winner.resolve == ResolveUnion && len(automatic) > 0 {
				by = "merged"
			}
			if answeredBy[model] == nil {
				answeredBy[model] = map[string]string{}
			}
			answeredBy[model][key] = by

			if len(automatic) >= 2 && !neverDisputed[key] {
				if disputed[model] == nil {
					disputed[model] = map[string]bool{}
				}
				disputed[model][key] = true
			}
			if hasResearch && len(automatic) > 0 {
				if overtaken[model] == nil {
					overtaken[model] = map[string]bool{}
				}
				overtaken[model][key] = true
			}
		}
	}
	return answeredBy, disputed, overtaken, nil
}

// SourceAnswer is one source's standing toward a single column on a single
// model: its rank on the column, what it has said, if anything, and when it
// said it.
type SourceAnswer struct {
	Source    string  `json:"source"`
	Label     string  `json:"label"`
	Value     *string `json:"value"`
	Rank      int     `json:"rank"`
	WrittenAt string  `json:"written_at,omitempty"`
}

// ColumnAnswers is everything the answers panel draws for one model and one
// column: every declared source, in rank order, whether or not it has
// answered, and which one wins.
type ColumnAnswers struct {
	Column    string         `json:"column"`
	Label     string         `json:"label"`
	ValueType string         `json:"value_type"`
	Resolve   string         `json:"resolve"`
	Answers   []SourceAnswer `json:"answers"`
	Winner    string         `json:"winner"`
}

// AnswersFor builds ColumnAnswers for one model and column: every source the
// column declares, in rank order, carrying "value":null where that source has
// not answered rather than an empty string, and the moment each answer was
// written. The winner is the lowest-ranked source that HAS answered - the
// same rule the answers view applies on every other read.
func AnswersFor(modelName, key string) (ColumnAnswers, error) {
	column, err := ColumnByKey(key)
	if err != nil {
		return ColumnAnswers{}, err
	}
	if _, exists, err := IdentityOf(modelName); err != nil {
		return ColumnAnswers{}, err
	} else if !exists {
		return ColumnAnswers{}, fmt.Errorf("%w: %q", ErrNoSuchModel, modelName)
	}

	rows, err := dbcore.DB.Query(`SELECT cs.source, cs.rank, s.label, c.value_text, c.written_at
		FROM column_sources cs
		JOIN sources s ON s.name = cs.source
		LEFT JOIN cells c ON c.column_id = cs.column_id AND c.source = cs.source AND c.model_name = ?
		WHERE cs.column_id = ?
		ORDER BY cs.rank ASC`, modelName, column.ID)
	if err != nil {
		return ColumnAnswers{}, err
	}
	defer rows.Close()

	out := ColumnAnswers{Column: column.Key, Label: column.Label, ValueType: column.ValueType, Resolve: column.Resolve}
	for rows.Next() {
		var source, label string
		var value, writtenAt *string
		var rank int
		if err := rows.Scan(&source, &rank, &label, &value, &writtenAt); err != nil {
			return ColumnAnswers{}, err
		}
		if source == string(SourceHand) {
			label = handLabel
		}
		answer := SourceAnswer{Source: source, Label: label, Value: value, Rank: rank}
		if writtenAt != nil {
			answer.WrittenAt = *writtenAt
		}
		out.Answers = append(out.Answers, answer)
		if value != nil && out.Winner == "" {
			out.Winner = source
		}
	}
	return out, rows.Err()
}
