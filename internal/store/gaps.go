// What one model row still has no answer for, worked out from
// the columns this database really holds.
//
// The brief is computed from the columns, not written beside them: the list is
// the columns research may fill and nothing automatic has yet, which are exactly
// the keys a save will accept on this row, so a key the write path would refuse
// is a key nothing can ask for.

package store

import (
	"fmt"
	"modelsdb/internal/dbcore"
	"strings"
)

// Gap is one thing about a model that nothing has answered yet, worded so a
// request can be built from it without opening the source.
//
// Field is EXACTLY the key POST /api/save takes: a column research may fill,
// under the key it is stored and read under. Nothing else is ever put here,
// because a key the write path refuses is the defect this whole type exists
// to remove.
type Gap struct {
	Field    string `json:"field"`
	Label    string `json:"label"`
	Accepts  string `json:"accepts"`
	Current  string `json:"current"`
	About    string `json:"about"`
	Measured bool   `json:"measured"`

	// Placeholder is what the worked example in the brief shows in place of this
	// gap's value. It is an unquoted <...>, which is not JSON, so the example as
	// printed is refused and writes nothing. It is not part of the JSON answer.
	Placeholder string `json:"-"`
}

// notResearchable says, per column research may fill, why an empty one is not
// something to go and find out. Every other such column is a gap when nothing
// answers it, so a column added later is asked about unless a reason is
// written here.
var notResearchable = map[string]string{
	"id":   "the OpenRouter catalog's own identifier for a listing, which the refresh writes wherever the model is listed",
	"slug": "the OpenRouter catalog's own short name for a listing, written by the same refresh",
}

// closedValues names the columns that take one of a fixed set of words, in the
// exact form the write path accepts. A column's KIND says a value must be text,
// or a number, or yes or no; it cannot say that a text column takes eight
// particular words, so a closed set has to be named here to be asked for in the
// spelling the write path accepts.
var closedValues = map[string]string{
	"model_type": `one of "chat", "image", "video", "embedding", "rerank", "tts", "stt", "music", "decisions", as text in quotes`,
	// Dates are stored as YYYY-MM-DD, the last day of the month where a source
	// names only a month, so a researched one has to be written the same way or
	// it sorts and filters wrongly beside the stored ones.
	"knowledge_cutoff": `text in quotes, a date written YYYY-MM-DD ("2025-01-31"); where the source gives only a month, that month's last day`,
	"expiration_date":  `text in quotes, a date written YYYY-MM-DD ("2026-12-31")`,
}

// Held is one thing a row already holds, worded for the brief: the column, what
// it holds, and who stated it. Value is cut short when it is long, because the
// brief shows it for orientation and the row itself is one request away.
type Held struct {
	Field  string `json:"field"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Source string `json:"source"`
}

// heldValueRunes is how much of one held value the brief prints.
const heldValueRunes = 160

// RowBrief is everything the research brief says about one row: the fields that
// can apply to it and have no answer, and the ones that apply and already hold
// one.
type RowBrief struct {
	Profile RowProfile
	Gaps    []Gap
	Held    []Held
}

// Gaps lists everything this row has no answer for that can apply to it; see
// BriefOf.
func Gaps(model string) ([]Gap, error) {
	b, err := BriefOf(model)
	return b.Gaps, err
}

// BriefOf works out what the research brief says about one row, in the order a
// request would be built: the order the columns are laid out in.
//
// A COLUMN IS ANSWERED WHEN ITS MERGED VALUE IS NON-EMPTY, whoever stated it -
// ModelCells already resolves a column's context length, say, to the winning
// source's answer, so a fact any source has stated is not a question anybody
// has to research.
//
// ONLY A COLUMN RESEARCH MAY FILL IS OFFERED, because on an empty cell that is
// the whole set POST /api/save will accept; a column of the reader's own is
// theirs alone and never asked about, and a column in notResearchable is left
// out because it hands out a name nobody should be asked to fill.
//
// ONLY A COLUMN THAT CAN DESCRIBE THIS ROW IS OFFERED, as RowProfile.Applies
// decides, so an OpenRouter price is never asked of a repository, nor an image
// model's reference images of an embedding model. Every column offered carries a
// description.
func BriefOf(model string) (RowBrief, error) {
	if model == "" {
		return RowBrief{}, ErrNoModelName
	}
	identity, exists, err := IdentityOf(model)
	if err != nil {
		return RowBrief{}, err
	} else if !exists {
		return RowBrief{}, ErrNoSuchModel
	}
	cells, err := ModelCells(model)
	if err != nil {
		return RowBrief{}, err
	}
	columns, err := ListColumns()
	if err != nil {
		return RowBrief{}, err
	}
	declared, err := ListColumnSources()
	if err != nil {
		return RowBrief{}, err
	}
	fromFeed, err := ImportedFeedFieldColumns()
	if err != nil {
		return RowBrief{}, err
	}
	winners, err := winningSources(model)
	if err != nil {
		return RowBrief{}, err
	}

	brief := RowBrief{Profile: ProfileOf(identity, cells)}
	for _, column := range columns {
		sources := declared[column.Key]
		path, madeFromFeed := fromFeed[column.Key]
		if !brief.Profile.Applies(column.Key, madeFromFeed) {
			continue
		}
		if held := cells[column.Key]; held != "" {
			if !containsSource(sources, SourceHand) && !(len(sources) == 1 && sources[0] == SourceApp) {
				brief.Held = append(brief.Held, Held{
					Field:  column.Key,
					Label:  column.Label,
					Value:  shortened(held, heldValueRunes),
					Source: heldSourceWord(column, winners[column.Key]),
				})
			}
			continue
		}
		if !containsSource(sources, SourceResearch) || notResearchable[column.Key] != "" {
			continue
		}
		about := column.About
		if about == "" && madeFromFeed {
			about = feedFieldAbout(column.Key, path)
		}
		brief.Gaps = append(brief.Gaps, Gap{
			Field:    column.Key,
			Label:    column.Label,
			Accepts:  acceptedBy(column),
			Current:  "not set",
			About:    about,
			Measured: statedOnlyByRepository(sources) && !researchableWhenUnstated[column.Key],

			Placeholder: placeholderOf(column),
		})
	}
	return brief, nil
}

// winningSources answers, per column key, the source whose cell a plain read of
// this model shows.
func winningSources(model string) (map[string]string, error) {
	rows, err := dbcore.DB.Query(`SELECT c.column_key, a.source FROM answers a
		JOIN columns c ON c.id = a.column_id WHERE a.model_name=?`, model)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var key, source string
		if err := rows.Scan(&key, &source); err != nil {
			return nil, err
		}
		out[key] = source
	}
	return out, rows.Err()
}

// heldSourceWord names who stated a held value. A column that merges every
// source's list says so, because no single source owns the merged answer.
func heldSourceWord(column Column, winner string) string {
	if column.Resolve == ResolveUnion {
		return "merged from every source that stated one"
	}
	return displayWord(winner)
}

// shortened cuts text to at most n runes, marking the cut.
func shortened(text string, n int) string {
	runes := []rune(strings.Join(strings.Fields(text), " "))
	if len(runes) <= n {
		return string(runes)
	}
	return string(runes[:n]) + "..."
}

// researchableWhenUnstated names the columns a repository states only when its
// card or tags name something, so an empty one is not a measurement a re-import
// will fill: a model with no base model has none to read, and research may supply
// one the card words only in prose.
var researchableWhenUnstated = map[string]bool{"base_model": true}

// statedOnlyByRepository reports whether the only automatic source declared
// for a column is HuggingFace - which makes an empty cell a MEASUREMENT rather
// than something to research: the honest way to fill it is to read the
// repository again, not to type an answer in.
func statedOnlyByRepository(sources []Source) bool {
	automatic, repository := 0, 0
	for _, src := range sources {
		switch src {
		case SourceHuggingFace:
			automatic, repository = automatic+1, repository+1
		case SourceOpenRouter:
			automatic++
		}
	}
	return automatic > 0 && automatic == repository
}

// acceptedBy words what one column will take, in the exact form the write path
// accepts. Every cell is sent as TEXT, whatever the column shows, so the
// question a caller needs answering is what that text must read as.
func acceptedBy(column Column) string {
	if closed := closedValues[column.Key]; closed != "" {
		return closed
	}
	switch column.ValueType {
	case ValueTypeNumber:
		return `text in quotes, and it must read as a number ("32.76", not 32.76)`
	case ValueTypeFlag:
		return `one of "yes" or "no", as text in quotes`
	case ValueTypeList:
		return `a JSON list, as text in quotes: "[\"text\",\"image\"]"`
	case ValueTypeChoice:
		quoted := make([]string, 0, len(column.Options))
		for _, o := range column.Options {
			quoted = append(quoted, fmt.Sprintf("%q", o))
		}
		if len(quoted) == 0 {
			return `text in quotes, matching one of this column's options exactly`
		}
		return "one of " + strings.Join(quoted, ", ") + ", as text in quotes"
	}
	return `text in quotes`
}

// placeholderOf is the unquoted <...> the brief's worked example shows for a
// column, chosen from the column's own kind of value, so it can never drift from
// what acceptedBy says the column takes.
func placeholderOf(column Column) string {
	if closedValues[column.Key] != "" {
		return "<one of the words listed above, in quotes>"
	}
	switch column.ValueType {
	case ValueTypeNumber:
		return "<the number you found, in quotes>"
	case ValueTypeFlag:
		return `<"yes" or "no", in quotes>`
	case ValueTypeList:
		return `<the list you found as quoted text, e.g. "[\"text\",\"image\"]">`
	case ValueTypeChoice:
		return "<one of the words listed above, in quotes>"
	}
	return "<the value you found, in quotes>"
}

// RepositoryOf answers the HuggingFace repository this row records, empty for a
// row that names none.
func RepositoryOf(model string) (string, error) {
	cells, err := ModelCells(model)
	if err != nil {
		return "", err
	}
	return cells["repository"], nil
}
