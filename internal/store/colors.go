// What colour a cell is painted, decided per column.
//
// A COLOUR RULE IS A PROPERTY OF THE COLUMN, stored beside its label and whether
// it is shown, because that is where everything else about a column lives. Any
// column can carry one, so a column somebody invented shades exactly as a column
// the app ships does.
//
// WHAT A RULE LOOKS LIKE DEPENDS ON THE KIND OF VALUE, and on nothing else:
//
//   - a NUMBER column carries ordered thresholds, each with a colour, and one
//     colour for everything above the last of them, so a price or a context
//     length shades by size;
//   - a FLAG column carries one colour for yes and one for no;
//   - a CHOICE column carries a colour per option label;
//   - a TEXT and a LIST column carry none: there is no order to shade along and
//     no fixed set of values to colour one by one.
//
// A THRESHOLD IS READ AGAINST THE FIGURE THE CELL SHOWS, which is the figure the
// column holds everywhere except a price - stored per token and shown per
// million, as every price list quotes one. Colour follows the eye rather than
// the storage, so the numbers somebody types into the editor are the numbers
// they read in the table.
//
// EVERY COLOUR IS A HEX CODE AND NOTHING ELSE. The browser puts the value
// straight into a cell's background, so a rule that could carry any text at all
// would be a way to write into the page.

package store

import (
	"encoding/json"
	"fmt"
	"modelsdb/internal/dbcore"
	"regexp"
	"sort"
	"strings"
)

// ErrColorRules is the one refusal a colour rule makes, whatever is wrong with
// it, so a handler answers it with one status.
var ErrColorRules = fmt.Errorf("that column will not take those colours")

// hexColor is the whole of what a colour may be: #rgb or #rrggbb.
var hexColor = regexp.MustCompile(`^#([0-9a-fA-F]{3}|[0-9a-fA-F]{6})$`)

// ColorThreshold is one band of a number column: every value BELOW this figure
// takes this colour, unless an earlier band already claimed it.
type ColorThreshold struct {
	Below float64 `json:"below"`
	Color string  `json:"color"`
}

// ColorRules is one column's whole colour rule. Only the fields its kind of
// value uses are ever filled, which is what checkColorRules holds it to.
type ColorRules struct {
	Thresholds []ColorThreshold  `json:"thresholds,omitempty"` // number
	Otherwise  string            `json:"otherwise,omitempty"`  // number: above the last threshold
	Yes        string            `json:"yes,omitempty"`        // flag
	No         string            `json:"no,omitempty"`         // flag
	Options    map[string]string `json:"options,omitempty"`    // choice: one colour per label
}

// IsEmpty reports whether these rules would paint nothing at all, which is what
// a column with no colouring holds.
func (r ColorRules) IsEmpty() bool {
	return len(r.Thresholds) == 0 && r.Otherwise == "" && r.Yes == "" && r.No == "" && len(r.Options) == 0
}

// ColumnTakesColors reports whether a column of this kind can be coloured. Text
// and a list cannot, and the interface must not offer it for them.
func ColumnTakesColors(valueType string) bool {
	switch valueType {
	case ValueTypeNumber, ValueTypeFlag, ValueTypeChoice:
		return true
	}
	return false
}

// checkColorRules refuses everything a column of this kind cannot mean: a rule
// on a column that takes none, a field belonging to another kind, a colour that
// is not a hex code, thresholds out of order, and an option no such option
// exists.
func checkColorRules(column Column, rules ColorRules) error {
	if rules.IsEmpty() {
		return nil // clearing a column's colours is always allowed
	}
	if !ColumnTakesColors(column.ValueType) {
		return fmt.Errorf("%w: %s holds %s, which has no order to shade along and no fixed set of values to colour one by one. A number, a yes-or-no and a one-of-a-list column take colours",
			ErrColorRules, column.Key, column.ValueType)
	}

	switch column.ValueType {
	case ValueTypeNumber:
		if err := onlyTheseFields(column, rules, "thresholds", "otherwise"); err != nil {
			return err
		}
		if len(rules.Thresholds) == 0 {
			return fmt.Errorf("%w: %s is a number, so its colours are thresholds - a figure and the colour every value below it takes",
				ErrColorRules, column.Key)
		}
		last := rules.Thresholds[0].Below
		for i, t := range rules.Thresholds {
			if err := checkColor(t.Color); err != nil {
				return err
			}
			if i > 0 && t.Below <= last {
				return fmt.Errorf("%w: the thresholds of %s run smallest first, and %v comes after %v",
					ErrColorRules, column.Key, t.Below, last)
			}
			last = t.Below
		}
		return checkColor(rules.Otherwise)

	case ValueTypeFlag:
		if err := onlyTheseFields(column, rules, "yes", "no"); err != nil {
			return err
		}
		if rules.Yes == "" && rules.No == "" {
			return fmt.Errorf("%w: %s is a yes or no, so its colours are one for yes and one for no", ErrColorRules, column.Key)
		}
		if err := checkColor(rules.Yes); err != nil {
			return err
		}
		return checkColor(rules.No)

	case ValueTypeChoice:
		if err := onlyTheseFields(column, rules, "options"); err != nil {
			return err
		}
		offered := map[string]bool{}
		for _, o := range column.Options {
			offered[o] = true
		}
		for _, label := range sortedKeys(rules.Options) {
			if !offered[label] {
				return fmt.Errorf("%w: %s offers %s, and %q is not one of them",
					ErrColorRules, column.Key, quotedList(column.Options), label)
			}
			if err := checkColor(rules.Options[label]); err != nil {
				return err
			}
		}
	}
	return nil
}

// onlyTheseFields refuses a rule carrying a field that belongs to another kind
// of column, naming the kind it does belong to rather than ignoring it.
func onlyTheseFields(column Column, rules ColorRules, allowed ...string) error {
	held := map[string]bool{}
	if len(rules.Thresholds) > 0 || rules.Otherwise != "" {
		held["thresholds"], held["otherwise"] = true, true
	}
	if rules.Yes != "" || rules.No != "" {
		held["yes"], held["no"] = true, true
	}
	if len(rules.Options) > 0 {
		held["options"] = true
	}
	for _, name := range allowed {
		delete(held, name)
	}
	if len(held) == 0 {
		return nil
	}
	return fmt.Errorf("%w: %s holds %s, so it takes %s and nothing else",
		ErrColorRules, column.Key, column.ValueType, strings.Join(allowed, " and "))
}

// checkColor refuses anything that is not a hex code. An empty colour means
// nothing is painted there, which is a real answer rather than a mistake.
func checkColor(color string) error {
	if color == "" || hexColor.MatchString(color) {
		return nil
	}
	return fmt.Errorf("%w: %q is not a colour. A colour is a hex code, e.g. #22c55e", ErrColorRules, color)
}

// sortedKeys names a map's keys in one order, so a refusal over several of them
// always names the same one.
func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// SetColumnColorRules stores what colours a column paints its cells, or clears
// them when the rules are empty. Every value the column holds is untouched:
// colouring says how a value is drawn, never what it is.
func SetColumnColorRules(key string, rules ColorRules) error {
	column, err := ColumnByKey(key)
	if err != nil {
		return err
	}
	if err := checkColorRules(column, rules); err != nil {
		return err
	}
	stored, err := colorRulesText(rules)
	if err != nil {
		return err
	}
	_, err = dbcore.DB.Exec("UPDATE columns SET color_rules=? WHERE id=?", stored, column.ID)
	return err
}

// colorRulesText is how a rule is stored: the JSON of it, or nothing at all when
// it paints nothing, so "no colours" has one spelling in the file.
func colorRulesText(rules ColorRules) (string, error) {
	if rules.IsEmpty() {
		return "", nil
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// colorRulesFrom reads a column's stored rule back. Text that cannot be read as
// a rule answers none rather than failing the whole column list: the values in
// the column are unaffected by how they are painted.
func colorRulesFrom(stored string) *ColorRules {
	if strings.TrimSpace(stored) == "" {
		return nil
	}
	var rules ColorRules
	if err := json.Unmarshal([]byte(stored), &rules); err != nil {
		return nil
	}
	if rules.IsEmpty() {
		return nil
	}
	return &rules
}
