// The query grammar GET /api/models answers to.
//
// ONE PARAMETER PER COLUMN, AND A REPEATED PARAMETER MEANS "AND ALSO". A filter
// names a column by its KEY, which is the same key the answer returns that
// value under, so nothing has to be looked up or translated between reading a
// row and narrowing the list. Naming the same column twice adds a second
// condition on it, ANDed with the first, exactly as two different columns are.
// Eight prefixes widen what a filter can ask: min: and max: on a column of
// figures, contains: (words, phrases, ! and AND/OR, see searchsyntax.go) on one
// holding text or a list, any: and not: - a
// comma-separated value list to match or to exclude - on every kind but a
// number, by: - a source the column declares, or not_ followed by one,
// with you meaning hand - asking not what the cell holds but who answered it,
// and from: and to: - the two inclusive ends of a range of days - on the
// columns that hold a date.
//
// Only name, include_hidden, hide_free, links and fields are not columns: name
// searches a model's display name, include_hidden decides whether the
// hidden rows come back, hide_free drops OpenRouter's free variants and the
// models whose price in a named column is 0, links keeps the models that have
// (or lack) a HuggingFace or an OpenRouter link, and fields= names what a row
// should carry.
//
// AN UNKNOWN PARAMETER IS REFUSED, never ignored. A filter this server silently
// dropped would answer with the whole catalog and look like a narrow answer,
// which is the failure a caller cannot see, so a parameter naming no column
// answers 400 - and the refusal offers the columns whose key, and whose label,
// come closest to the word that was used.

package app

import (
	"fmt"
	"modelsdb/internal/dbcore"
	"modelsdb/internal/shared"
	"modelsdb/internal/store"
	"net/url"
	"sort"
	"strings"
	"time"
)

// The parameters that are not column keys.
const (
	paramName          = "name"
	paramIncludeHidden = "include_hidden"
	paramHideFree      = "hide_free"
	paramLinks         = "links"
	paramFields        = "fields"
)

// The two links a model can have, named as links= names them. A model has the
// HuggingFace link when its repository cell holds a value and the OpenRouter
// link when its id cell does: the same test the page uses to draw each icon.
const (
	linkHuggingFace = "huggingface"
	linkOpenRouter  = "openrouter"
)

// linkColumns maps each link to the column whose value is that link.
var linkColumns = map[string]string{
	linkHuggingFace: "repository",
	linkOpenRouter:  "id",
}

// identityFields are the parts of a row that are not cells: what the model is
// called, the route that created it, and the reader's own two decisions about
// their table. fields= can project any of them; only name filters.
var identityFields = []string{"name", "source", "hidden", "variant_of", "_raw"}

// The ways a filter can ask about a cell.
const (
	testIs       = "is"
	testMin      = "min:"
	testMax      = "max:"
	testContains = "contains:"
	testAny      = "any:"
	testNot      = "not:"
	testBy       = "by:"
	testFrom     = "from:"
	testTo       = "to:"
)

// dateLayout is the one shape a day is written in, in a from: or to: and in the
// first ten characters of a stored date.
const dateLayout = "2006-01-02"

// freeIDSuffix and freeNameSuffix are how OpenRouter marks a free variant: the
// id of the listing ends :free, and its display name ends (free). A price of 0
// is NOT the mark, because a model charged per image or per second carries no
// per-token price at all.
const (
	freeIDSuffix   = ":free"
	freeNameSuffix = "(free)"
)

// by: takes a source name after its colon, or one after this prefix to ask
// for every model that source did NOT answer; byYou is the reader's word for
// hand, the source their own columns are written as.
const (
	byNot = "not_"
	byYou = "you"
)

// cellTest is one column filter: which column, how it is being asked, and what
// it is being asked for.
type cellTest struct {
	column  store.Column
	kind    string
	want    string     // lower case, for the text comparisons
	search  searchExpr // the parsed box text, for contains:
	figure  float64    // the figure, for min: and max:
	members []string   // the listed values, for any: and not:
}

// modelQuery is one request's whole question, read out of its parameters before
// a single row is looked at - so a parameter this server does not take is
// refused before any answer is built.
type modelQuery struct {
	includeHidden bool
	hideFree      bool
	hideZero      []store.Column
	links         []linkTest
	name          searchExpr
	tests         []cellTest
	identity      []string
	cells         []string
	projecting    bool
}

// truthyParam reports whether a query parameter is asking for something to be
// switched on. It accepts the three spellings this API already uses elsewhere
// (1, true, yes) so a caller does not have to guess which one a given parameter
// wants.
func truthyParam(v string) bool {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "1", "true", "yes":
		return true
	}
	return false
}

// flagSays reads the word a flag column holds. Every spelling a cell can hold is
// accepted, so a filter written as yes matches a cell stored as true.
func flagSays(value string) (bool, bool) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "yes", "true", "1":
		return true, true
	case "no", "false", "0":
		return false, true
	}
	return false, false
}

// parseModelQuery reads a request's parameters against the columns this catalog
// holds, and refuses anything it cannot carry out.
func parseModelQuery(q url.Values, columns []store.Column) (modelQuery, error) {
	byKey := make(map[string]store.Column, len(columns))
	for _, c := range columns {
		byKey[c.Key] = c
	}
	identity := map[string]bool{}
	for _, f := range identityFields {
		identity[f] = true
	}

	// Sorted, so a request carrying two parameters this refuses always names the
	// same one - a map is walked in a different order every time.
	names := make([]string, 0, len(q))
	for name := range q {
		names = append(names, name)
	}
	sort.Strings(names)

	// Fetched at most once per request, and only when a by: filter actually
	// asks for it - most requests carry none, and every other parameter here
	// is answered from the columns already in hand.
	var declared map[string][]store.Source
	declaredSources := func() (map[string][]store.Source, error) {
		if declared != nil {
			return declared, nil
		}
		var err error
		declared, err = store.ListColumnSources()
		return declared, err
	}

	var mq modelQuery
	for _, name := range names {
		value := q.Get(name)
		switch name {
		case paramIncludeHidden:
			mq.includeHidden = truthyParam(value)
		case paramHideFree:
			free, zero, err := readHideFree(value, byKey)
			if err != nil {
				return modelQuery{}, err
			}
			mq.hideFree, mq.hideZero = free, zero
		case paramLinks:
			// Every value of a repeated links is a test of its own, ANDed, like a
			// repeated column parameter.
			for _, one := range q[name] {
				test, err := readLinkTest(one)
				if err != nil {
					return modelQuery{}, err
				}
				mq.links = append(mq.links, test)
			}
		case paramName:
			mq.name = parseSearch(value)
		case paramFields:
			mq.projecting = true
			for _, field := range dbcore.SplitList(value) {
				switch {
				case identity[field]:
					mq.identity = append(mq.identity, field)
				case byKey[field].Key != "":
					mq.cells = append(mq.cells, field)
				default:
					return modelQuery{}, unknownField(field, columns)
				}
			}
		default:
			column, held := byKey[name]
			if !held {
				return modelQuery{}, unknownParameter(name, columns)
			}
			// EVERY value of a repeated parameter becomes a test of its own, ANDed
			// with the rest. Reading only the first would silently drop the others,
			// and a dropped filter answers a wider set than it appears to - the one
			// failure a caller cannot see.
			for _, one := range q[name] {
				test, err := readCellTest(column, one, declaredSources)
				if err != nil {
					return modelQuery{}, err
				}
				mq.tests = append(mq.tests, test)
			}
		}
	}
	return mq, nil
}

// hideFreeColumns are the columns hide_free=1 stands for: the two prices the
// page draws a Hide 0 tick under.
var hideFreeColumns = []string{"price_prompt", "price_completion"}

// readHideFree reads hide_free. A switched-on word (1, true, yes) means both
// price columns, which is what the parameter meant before it took columns, so a
// saved link keeps working. Otherwise the value is a comma-separated list of
// columns of figures, each of which hides the models whose figure is 0 there.
// Any list also drops OpenRouter's free variants. An off word or an empty value
// leaves the models alone; a word naming no column of figures is refused, never
// ignored.
func readHideFree(value string, byKey map[string]store.Column) (bool, []store.Column, error) {
	value = strings.TrimSpace(value)
	switch strings.ToLower(value) {
	case "", "0", "false", "no":
		return false, nil, nil
	}
	keys := dbcore.SplitList(value)
	if truthyParam(value) {
		keys = hideFreeColumns
	}
	var columns []store.Column
	for _, key := range keys {
		column, held := byKey[key]
		if !held || column.ValueType != store.ValueTypeNumber {
			return false, nil, fmt.Errorf("%s takes 1, true or yes for both prices, or a comma-separated list of columns of figures, and %q is neither", paramHideFree, key)
		}
		columns = append(columns, column)
	}
	return true, columns, nil
}

// linkTest is one links= condition: the models that have at least one of the
// named links (any:), or that have none of them (not:).
type linkTest struct {
	kind  string // testAny or testNot
	names []string
}

// readLinkTest reads one links= value: any: or not: followed by a
// comma-separated list of huggingface and openrouter. Anything else is refused,
// because a link filter silently dropped would answer the whole catalog.
func readLinkTest(value string) (linkTest, error) {
	value = strings.TrimSpace(value)
	accepted := fmt.Sprintf("%s takes %s or %s followed by a comma-separated list of %s and %s, such as %s=%s%s,%s",
		paramLinks, testAny, testNot, linkHuggingFace, linkOpenRouter, paramLinks, testAny, linkHuggingFace, linkOpenRouter)
	var kind string
	switch {
	case strings.HasPrefix(value, testAny):
		kind = testAny
	case strings.HasPrefix(value, testNot):
		kind = testNot
	default:
		return linkTest{}, fmt.Errorf("%q is not a links value. %s", value, accepted)
	}
	var names []string
	for _, member := range splitMembers(strings.TrimPrefix(value, kind)) {
		member = strings.ToLower(member)
		if _, known := linkColumns[member]; !known {
			return linkTest{}, fmt.Errorf("%q is not a link this catalog knows. %s", member, accepted)
		}
		names = append(names, member)
	}
	return linkTest{kind: kind, names: names}, nil
}

// matches answers whether one model's cells satisfy this condition.
func (t linkTest) matches(cells map[string]string) bool {
	has := false
	for _, name := range t.names {
		if strings.TrimSpace(cells[linkColumns[name]]) != "" {
			has = true
			break
		}
	}
	return has == (t.kind == testAny)
}

// readCellTest turns one parameter's value into the question it asks, and
// refuses a question the column cannot answer: a figure comparison on a column
// that does not hold figures, a substring search on one that holds neither
// text nor a list, and a by: naming a source the column does not declare.
// declaredSources is fetched at most once per request, and only when a by:
// value is actually seen.
func readCellTest(column store.Column, value string, declaredSources func() (map[string][]store.Source, error)) (cellTest, error) {
	value = strings.TrimSpace(value)
	switch {
	case strings.HasPrefix(value, testBy):
		state := strings.TrimSpace(strings.TrimPrefix(value, testBy))
		negated := strings.HasPrefix(state, byNot)
		name := strings.TrimPrefix(state, byNot)
		if name == byYou {
			name = string(store.SourceHand)
		}
		declared, err := declaredSources()
		if err != nil {
			return cellTest{}, err
		}
		names := make([]string, 0, len(declared[column.Key]))
		for _, src := range declared[column.Key] {
			names = append(names, string(src))
		}
		if !contains(names, name) {
			return cellTest{}, fmt.Errorf("%s%s asks who answered %s, and %q does not state it. %s is stated by %s; by: takes one of those, or %s followed by one, and %s means %s",
				testBy, state, column.Key, name, column.Key, strings.Join(names, ", "), byNot, byYou, store.SourceHand)
		}
		want := name
		if negated {
			want = byNot + name
		}
		return cellTest{column: column, kind: testBy, want: want}, nil

	case strings.HasPrefix(value, testMin), strings.HasPrefix(value, testMax):
		kind := testMin
		if strings.HasPrefix(value, testMax) {
			kind = testMax
		}
		if column.ValueType != store.ValueTypeNumber {
			return cellTest{}, fmt.Errorf("%s%s asks for a comparison of figures, and %s holds %s. %s takes a value to match, or contains: to search inside it",
				kind, strings.TrimPrefix(value, kind), column.Key, column.ValueType, column.Key)
		}
		figure, err := shared.ParseNumber(strings.TrimSpace(strings.TrimPrefix(value, kind)))
		if err != nil {
			return cellTest{}, fmt.Errorf("%s takes a figure after %s, and %q is not one", column.Key, kind, strings.TrimPrefix(value, kind))
		}
		return cellTest{column: column, kind: kind, figure: figure}, nil

	case strings.HasPrefix(value, testFrom), strings.HasPrefix(value, testTo):
		kind := testFrom
		if strings.HasPrefix(value, testTo) {
			kind = testTo
		}
		day := strings.TrimSpace(strings.TrimPrefix(value, kind))
		if !store.IsDateColumn(column.Key) {
			return cellTest{}, fmt.Errorf("%s%s asks for a range of days, and %s does not hold a date. %s takes a value to match%s",
				kind, day, column.Key, column.Key, minMaxHint(column))
		}
		if _, err := time.Parse(dateLayout, day); err != nil || len(day) != len(dateLayout) {
			return cellTest{}, fmt.Errorf("%s takes a day written YYYY-MM-DD after %s, and %q is not one", column.Key, kind, day)
		}
		return cellTest{column: column, kind: kind, want: day}, nil

	case strings.HasPrefix(value, testContains):
		if column.ValueType != store.ValueTypeText && column.ValueType != store.ValueTypeList {
			return cellTest{}, fmt.Errorf("contains: searches inside text, and %s holds %s. %s takes a value to match%s",
				column.Key, column.ValueType, column.Key, minMaxHint(column))
		}
		return cellTest{column: column, kind: testContains,
			search: parseSearch(strings.TrimPrefix(value, testContains))}, nil

	case strings.HasPrefix(value, testAny), strings.HasPrefix(value, testNot):
		kind := testAny
		if strings.HasPrefix(value, testNot) {
			kind = testNot
		}
		if column.ValueType == store.ValueTypeNumber {
			return cellTest{}, fmt.Errorf("%s lists values to match or exclude, and %s holds figures. %s takes a value to match%s",
				kind, column.Key, column.Key, minMaxHint(column))
		}
		return cellTest{column: column, kind: kind,
			members: splitMembers(strings.TrimPrefix(value, kind))}, nil
	}
	return cellTest{column: column, kind: testIs, want: strings.ToLower(value)}, nil
}

// splitMembers reads the value list after any: or not:. A comma separates, a
// literal comma in a value is written \, - and an EMPTY member is kept, because
// it means "no value at all", the same thing an empty parameter asks on its own.
func splitMembers(list string) []string {
	var members []string
	var current strings.Builder
	for i := 0; i < len(list); i++ {
		switch {
		case list[i] == '\\' && i+1 < len(list) && list[i+1] == ',':
			current.WriteByte(',')
			i++
		case list[i] == ',':
			members = append(members, strings.TrimSpace(current.String()))
			current.Reset()
		default:
			current.WriteByte(list[i])
		}
	}
	members = append(members, strings.TrimSpace(current.String()))
	return members
}

// dayOf reads the day a stored date falls on: the first ten characters, which
// is the whole of a plain date and the date part of a timestamp in either shape
// created_at is stored in. A value that does not begin with a real day answers
// false, so a model with no date, or a malformed one, never matches a range.
func dayOf(value string) (string, bool) {
	if len(value) < len(dateLayout) {
		return "", false
	}
	if len(value) > len(dateLayout) && value[len(dateLayout)] != 'T' && value[len(dateLayout)] != ' ' {
		return "", false
	}
	day := value[:len(dateLayout)]
	if _, err := time.Parse(dateLayout, day); err != nil {
		return "", false
	}
	return day, true
}

// isZeroIn reports whether a model's figure in one column is 0. A model with no
// figure there is not zero, so it is kept.
func isZeroIn(column store.Column, cells map[string]string) bool {
	value := strings.TrimSpace(cells[column.Key])
	if value == "" {
		return false
	}
	figure, err := shared.ParseNumber(value)
	return err == nil && figure == 0
}

// isFreeListing reports whether a model is one of OpenRouter's free variants,
// by the id its listing carries or by its display name.
func isFreeListing(name string, cells map[string]string) bool {
	id := strings.ToLower(strings.TrimSpace(cells["id"]))
	shown := strings.ToLower(strings.TrimSpace(name))
	return strings.HasSuffix(id, freeIDSuffix) || strings.HasSuffix(shown, freeNameSuffix)
}

// minMaxHint offers the comparison a column of figures does take, in the
// refusal a caller reads.
func minMaxHint(column store.Column) string {
	if column.ValueType == store.ValueTypeNumber {
		return ", or min: and max: to compare figures"
	}
	return ""
}

// holdsValue answers whether one cell holds one wanted value, the way an exact
// match reads it: any one member of a list column, either spelling of a flag,
// and the value itself (capitals ignored) everywhere else. An empty want
// matches exactly the models holding nothing, which is also what an empty
// member of any: or not: asks.
func holdsValue(column store.Column, value, want string) bool {
	if want == "" || value == "" {
		return want == "" && value == ""
	}
	switch column.ValueType {
	case store.ValueTypeList:
		for _, member := range dbcore.ParseArr(value) {
			if strings.EqualFold(strings.TrimSpace(member), want) {
				return true
			}
		}
		return false
	case store.ValueTypeFlag:
		said, read := flagSays(value)
		wanted, asked := flagSays(want)
		if read && asked {
			return said == wanted
		}
	}
	return strings.EqualFold(value, want)
}

// matches answers whether one model's cells satisfy this filter. A model with no
// value in the column matches only a filter asking for an empty one - and not:
// reads the same way from the other side: a model holding nothing equals none
// of the listed values, so it matches unless the list names the empty value.
// answeredBy is the same row's answered_by map, read only by by: - every other
// kind asks about the cell's own text and never touches it.
func (t cellTest) matches(cells map[string]string, answeredBy map[string]string) bool {
	value := strings.TrimSpace(cells[t.column.Key])
	switch t.kind {
	case testBy:
		if strings.HasPrefix(t.want, byNot) {
			return answeredBy[t.column.Key] != strings.TrimPrefix(t.want, byNot)
		}
		return answeredBy[t.column.Key] == t.want

	case testIs:
		return holdsValue(t.column, value, t.want)

	case testAny:
		for _, member := range t.members {
			if holdsValue(t.column, value, member) {
				return true
			}
		}
		return false

	case testNot:
		for _, member := range t.members {
			if holdsValue(t.column, value, member) {
				return false
			}
		}
		return true

	case testContains:
		if t.column.ValueType == store.ValueTypeList {
			return t.search.matches(dbcore.ParseArr(value))
		}
		return t.search.matches([]string{value})

	case testFrom, testTo:
		day, ok := dayOf(value)
		if !ok {
			return false
		}
		if t.kind == testFrom {
			return day >= t.want
		}
		return day <= t.want

	case testMin, testMax:
		figure, err := shared.ParseNumber(value)
		if err != nil {
			return false
		}
		if t.kind == testMin {
			return figure >= t.figure
		}
		return figure <= t.figure
	}
	return false
}

// filterAndProject applies the query's filters (AND-combined, capitals ignored)
// and its fields= projection to the rows processModels built.
//
// IT IS THE ONLY PLACE A STORED ROW IS EVER WITHHELD, and every rule in it is
// conditional on what THIS request asked for. processModels hands over one row
// per stored model and reads no request parameters at all, so a row a caller
// does not receive is always a row that caller filtered out - including the
// hidden ones, which a request gets back the moment it passes include_hidden=1.
// Nothing anywhere excludes a model for what its name says.
func filterAndProject(rows []map[string]interface{}, q url.Values, columns []store.Column) ([]map[string]interface{}, error) {
	mq, err := parseModelQuery(q, columns)
	if err != nil {
		return nil, err
	}

	out := make([]map[string]interface{}, 0, len(rows))
	for _, row := range rows {
		cells, _ := row["cells"].(map[string]string)
		answeredBy, _ := row["answered_by"].(map[string]string)

		// Hidden rows are withheld unless this request asked for them by name. The
		// flag exists so a model can be KEPT without being listed, so the default
		// has to be the quiet one - a caller that never heard of the parameter must
		// not receive them.
		if !mq.includeHidden {
			if hidden, _ := row["hidden"].(bool); hidden {
				continue
			}
		}
		if mq.hideFree {
			shown, _ := row["name"].(string)
			if isFreeListing(shown, cells) {
				continue
			}
			zero := false
			for _, column := range mq.hideZero {
				if isZeroIn(column, cells) {
					zero = true
					break
				}
			}
			if zero {
				continue
			}
		}
		linked := true
		for _, test := range mq.links {
			if !test.matches(cells) {
				linked = false
				break
			}
		}
		if !linked {
			continue
		}
		shown, _ := row["name"].(string)
		if !mq.name.matches([]string{shown}) {
			continue
		}
		matched := true
		for _, test := range mq.tests {
			if !test.matches(cells, answeredBy) {
				matched = false
				break
			}
		}
		if !matched {
			continue
		}
		out = append(out, mq.project(row, cells))
	}
	return out, nil
}

// project narrows one row to what fields= named. Without a projection the whole
// row goes back. With one, the row carries the identity fields it named, and a
// cells object holding only the column keys it named - which is absent
// altogether when it named none.
func (mq modelQuery) project(row map[string]interface{}, cells map[string]string) map[string]interface{} {
	if !mq.projecting {
		return row
	}
	out := make(map[string]interface{}, len(mq.identity)+1)
	for _, field := range mq.identity {
		if value, held := row[field]; held {
			out[field] = value
		}
	}
	if len(mq.cells) > 0 {
		kept := make(map[string]string, len(mq.cells))
		for _, key := range mq.cells {
			if value, held := cells[key]; held {
				kept[key] = value
			}
		}
		out["cells"] = kept
	}
	return out
}

// unknownParameter is the refusal a parameter naming no column gets, with the
// columns whose key and whose label come closest to what was written.
func unknownParameter(param string, columns []store.Column) error {
	return fmt.Errorf("%q is not a parameter this query takes. A filter names a column by its key (GET /api/columns lists every one), and %q, %q, %q, %q and %q are the five that are not columns.%s",
		param, paramName, paramIncludeHidden, paramHideFree, paramLinks, paramFields, suggestions(param, columns))
}

// unknownField is the same refusal for a name inside fields=.
func unknownField(field string, columns []store.Column) error {
	return fmt.Errorf("fields= names %q, which is neither a column key (GET /api/columns lists every one) nor one of the identity fields %s.%s",
		field, strings.Join(identityFields, ", "), suggestions(field, columns))
}

// suggestions words the near matches, by column key and by column label, for a
// refusal a person or an agent reads. Both are offered because a caller may know
// the column by either: an old query parameter reads like a key, and what
// somebody sees on screen is a label.
func suggestions(word string, columns []store.Column) string {
	byKey, byLabel := closeColumns(word, columns)
	text := ""
	if len(byKey) > 0 {
		text += " Columns whose key is close: " + strings.Join(byKey, ", ") + "."
	}
	if len(byLabel) > 0 {
		text += " Columns whose label is close: " + strings.Join(byLabel, ", ") + "."
	}
	return text
}

// closeColumns ranks every column against the word that was written, once by its
// key and once by its label, and answers the best few of each.
func closeColumns(word string, columns []store.Column) (byKey, byLabel []string) {
	type match struct {
		score int
		key   string
		text  string
	}
	var keyed, labelled []match
	for _, c := range columns {
		if score := closeness(c.Key, word); score >= 0 {
			keyed = append(keyed, match{score, c.Key, c.Key})
		}
		if score := closeness(c.Label, word); score >= 0 {
			labelled = append(labelled, match{score, c.Key, fmt.Sprintf("%q (%s)", c.Label, c.Key)})
		}
	}
	pick := func(all []match) []string {
		sort.Slice(all, func(a, b int) bool {
			if all[a].score != all[b].score {
				return all[a].score < all[b].score
			}
			return all[a].key < all[b].key
		})
		out := make([]string, 0, 5)
		for _, m := range all {
			if len(out) == 5 {
				break
			}
			out = append(out, m.text)
		}
		return out
	}
	return pick(keyed), pick(labelled)
}

// closeness scores how alike two words are: 0 for the same word, 1 where one
// holds the other, and 2 upwards for a word within three single-letter changes.
// A negative answer means they are not alike at all. It is the score half of
// closenessReason below, for the callers - the column-key suggestions - that
// have no use for the reason.
func closeness(candidate, word string) int {
	score, _ := closenessReason(candidate, word)
	return score
}

// closenessReason is the one implementation of "how alike are two words",
// shared by the column-key suggestions (closeColumns) and the model-name
// matcher (nameCloseness, in similarnames.go): 0 for the same word, 1 where one
// holds the other, 2 upwards for a word within three single-letter changes, 5
// for a shared word, and a negative score for no match at all. The reason is a
// short plain phrase naming which rule matched, empty where nothing did.
func closenessReason(candidate, word string) (int, string) {
	candidate, word = strings.ToLower(strings.TrimSpace(candidate)), strings.ToLower(strings.TrimSpace(word))
	if candidate == "" || word == "" {
		return -1, ""
	}
	if candidate == word {
		return 0, "the same name written differently"
	}
	if strings.Contains(candidate, word) || strings.Contains(word, candidate) {
		return 1, "one name contains the other"
	}
	if changes := editDistance(candidate, word); changes <= 3 && changes < len([]rune(word)) {
		return 1 + changes, fmt.Sprintf("within %d letter changes", changes)
	}
	// A WORD THEY SHARE IS ENOUGH, which is what carries a caller from a
	// parameter built out of two words to the column one of them names:
	// min_context finds context_length.
	if shared, ok := sharesAWord(candidate, word); ok {
		return 5, fmt.Sprintf("shares the word %q", shared)
	}
	return -1, ""
}

// sharesAWord reports whether the two names have a word in common, splitting
// each on the punctuation this app builds keys and parameters out of, and which
// word that is. A word of one or two letters is passed over, because "in" and
// "id" are in everything. Where several words are shared, the alphabetically
// first is answered, so the same pair always names the same word.
func sharesAWord(candidate, word string) (string, bool) {
	split := func(s string) map[string]bool {
		out := map[string]bool{}
		for _, part := range strings.FieldsFunc(s, func(r rune) bool {
			return r == '.' || r == '_' || r == ' ' || r == '-'
		}) {
			if len(part) > 2 {
				out[part] = true
			}
		}
		return out
	}
	theirs := split(word)
	var shared []string
	for part := range split(candidate) {
		if theirs[part] {
			shared = append(shared, part)
		}
	}
	if len(shared) == 0 {
		return "", false
	}
	sort.Strings(shared)
	return shared[0], true
}

// editDistance counts the single-character insertions, deletions and
// replacements that turn one word into the other, which is what makes a
// misspelt column key suggest the right one, and a misspelt model name find
// the model it means. It compares RUNES, not bytes, so a multi-byte character
// counts as one change rather than several; an ASCII-only pair scores exactly
// as a byte-wise comparison would, since every ASCII byte is its own rune.
func editDistance(a, b string) int {
	ar, br := []rune(a), []rune(b)
	previous := make([]int, len(br)+1)
	current := make([]int, len(br)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(ar); i++ {
		current[0] = i
		for j := 1; j <= len(br); j++ {
			cost := 1
			if ar[i-1] == br[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, min(current[j-1]+1, previous[j-1]+cost))
		}
		previous, current = current, previous
	}
	return previous[len(br)]
}
