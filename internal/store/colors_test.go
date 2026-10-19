package store

import (
	"encoding/json"
	"errors"
	"modelsdb/internal/dbcore"
	"strings"
	"testing"
)

// colouredColumn is one column of the reader's own, of whatever kind a test is
// asking about.
func colouredColumn(t *testing.T, key, valueType string, options []string) Column {
	t.Helper()
	return makeColumn(t, ColumnSpec{Key: key, Label: key, ValueType: valueType, Options: options})
}

// TestANumberColumnTakesOrderedThresholds is the first of the three shapes: a
// figure and the colour every value below it takes, smallest first, with one
// colour for everything left over.
func TestANumberColumnTakesOrderedThresholds(t *testing.T) {
	cellStoreDB(t)
	colouredColumn(t, "speed_score", ValueTypeNumber, nil)

	rules := ColorRules{
		Thresholds: []ColorThreshold{{Below: 10, Color: "#ef4444"}, {Below: 100, Color: "#eab308"}},
		Otherwise:  "#22c55e",
	}
	if err := SetColumnColorRules("speed_score", rules); err != nil {
		t.Fatalf("colour a number column: %v", err)
	}
	back, err := ColumnByKey("speed_score")
	if err != nil {
		t.Fatal(err)
	}
	if back.Colors == nil || len(back.Colors.Thresholds) != 2 ||
		back.Colors.Thresholds[0].Below != 10 || back.Colors.Thresholds[1].Color != "#eab308" ||
		back.Colors.Otherwise != "#22c55e" {
		t.Fatalf("the rule read back is %+v, want the two thresholds and the colour for the rest", back.Colors)
	}

	// Out of order, which would make the second band unreachable.
	err = SetColumnColorRules("speed_score", ColorRules{
		Thresholds: []ColorThreshold{{Below: 100, Color: "#eab308"}, {Below: 10, Color: "#ef4444"}},
		Otherwise:  "#22c55e",
	})
	if !errors.Is(err, ErrColorRules) || !strings.Contains(err.Error(), "smallest first") {
		t.Errorf("thresholds out of order answered %v, want a refusal naming the order", err)
	}
	// A yes-or-no's answer on a number column belongs to another kind.
	if err := SetColumnColorRules("speed_score", ColorRules{Yes: "#22c55e"}); !errors.Is(err, ErrColorRules) {
		t.Errorf("a flag's colours on a number column answered %v, want a refusal", err)
	}
}

// TestAFlagColumnTakesOneColourForYesAndOneForNo is the second shape.
func TestAFlagColumnTakesOneColourForYesAndOneForNo(t *testing.T) {
	cellStoreDB(t)
	colouredColumn(t, "tried_it", ValueTypeFlag, nil)

	if err := SetColumnColorRules("tried_it", ColorRules{Yes: "#22c55e", No: "#ef4444"}); err != nil {
		t.Fatalf("colour a flag column: %v", err)
	}
	back, err := ColumnByKey("tried_it")
	if err != nil {
		t.Fatal(err)
	}
	if back.Colors == nil || back.Colors.Yes != "#22c55e" || back.Colors.No != "#ef4444" {
		t.Fatalf("the rule read back is %+v, want a colour for each answer", back.Colors)
	}
	if err := SetColumnColorRules("tried_it", ColorRules{
		Thresholds: []ColorThreshold{{Below: 1, Color: "#22c55e"}}}); !errors.Is(err, ErrColorRules) {
		t.Errorf("thresholds on a flag column answered %v, want a refusal", err)
	}
}

// TestAChoiceColumnTakesAColourPerOption is the third shape, and the refusal
// that keeps a rule readable: a label the column does not offer.
func TestAChoiceColumnTakesAColourPerOption(t *testing.T) {
	cellStoreDB(t)
	colouredColumn(t, "grade", ValueTypeChoice, []string{"1", "2", "3", "4"})

	if err := SetColumnColorRules("grade", ColorRules{
		Options: map[string]string{"1": "#ef4444", "4": "#22c55e"}}); err != nil {
		t.Fatalf("colour a choice column: %v", err)
	}
	back, err := ColumnByKey("grade")
	if err != nil {
		t.Fatal(err)
	}
	if back.Colors == nil || back.Colors.Options["1"] != "#ef4444" || back.Colors.Options["4"] != "#22c55e" {
		t.Fatalf("the rule read back is %+v, want a colour per label", back.Colors)
	}
	err = SetColumnColorRules("grade", ColorRules{Options: map[string]string{"excellent": "#22c55e"}})
	if !errors.Is(err, ErrColorRules) || !strings.Contains(err.Error(), "excellent") {
		t.Errorf("a label the column does not offer answered %v, want a refusal naming it", err)
	}
}

// TestATextAndAListColumnTakeNoColoursAtAll. There is no order to shade along
// and no fixed set of values to colour one by one, so the interface offers
// nothing for them and the store refuses one anyway.
func TestATextAndAListColumnTakeNoColoursAtAll(t *testing.T) {
	cellStoreDB(t)
	colouredColumn(t, "sizing_notes", ValueTypeText, nil)
	colouredColumn(t, "tags", ValueTypeList, nil)

	for _, key := range []string{"sizing_notes", "tags"} {
		err := SetColumnColorRules(key, ColorRules{
			Thresholds: []ColorThreshold{{Below: 1, Color: "#22c55e"}}, Otherwise: "#ef4444"})
		if !errors.Is(err, ErrColorRules) {
			t.Errorf("%s took colours (%v), and it holds nothing to colour by", key, err)
		}
		column, err := ColumnByKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if column.Colors != nil {
			t.Errorf("%s stored %+v after a refusal, want nothing", key, column.Colors)
		}
		if ColumnTakesColors(column.ValueType) {
			t.Errorf("ColumnTakesColors says a %s column can be coloured", column.ValueType)
		}
	}
}

// TestOnlyAHexCodeIsAColour. The browser puts the value straight into a cell's
// background, so anything else would be a way to write into the page.
func TestOnlyAHexCodeIsAColour(t *testing.T) {
	cellStoreDB(t)
	colouredColumn(t, "grade", ValueTypeChoice, []string{"good"})
	colouredColumn(t, "size", ValueTypeNumber, nil)

	for _, bad := range []string{"red", "rgb(1,2,3)", "#12", "url(x)", "#22c55e; background-image:url(x)"} {
		if err := SetColumnColorRules("grade", ColorRules{
			Options: map[string]string{"good": bad}}); !errors.Is(err, ErrColorRules) {
			t.Errorf("%q was accepted as a colour", bad)
		}
	}
	// Both lengths of a real hex code are colours.
	for _, good := range []string{"#2c5", "#22c55e"} {
		if err := SetColumnColorRules("size", ColorRules{
			Thresholds: []ColorThreshold{{Below: 1, Color: good}}, Otherwise: "#ef4444"}); err != nil {
			t.Errorf("%q was refused as a colour: %v", good, err)
		}
	}
}

// TestClearingAColumnsColoursLeavesEveryValueWhereItIs. A colour says how a
// value is drawn and never what it is.
func TestClearingAColumnsColoursLeavesEveryValueWhereItIs(t *testing.T) {
	cellStoreDB(t)
	addModelRow(t, "Vendor: Model")
	colouredColumn(t, "size", ValueTypeNumber, nil)
	if err := WriteCells(SourceHand, "Vendor: Model", map[string]string{"size": "42"}); err != nil {
		t.Fatal(err)
	}
	if err := SetColumnColorRules("size", ColorRules{
		Thresholds: []ColorThreshold{{Below: 100, Color: "#22c55e"}}, Otherwise: "#ef4444"}); err != nil {
		t.Fatal(err)
	}
	if err := SetColumnColorRules("size", ColorRules{}); err != nil {
		t.Fatalf("clear the colours: %v", err)
	}
	column, err := ColumnByKey("size")
	if err != nil {
		t.Fatal(err)
	}
	if column.Colors != nil {
		t.Errorf("the column still holds %+v after being cleared", column.Colors)
	}
	cells, err := ModelCells("Vendor: Model")
	if err != nil {
		t.Fatal(err)
	}
	if cells["size"] != "42" {
		t.Errorf("the stored value is %q, want the 42 that was written", cells["size"])
	}
}

// TestThePriceColumnsShipTheColoursTheMigrationWrites checks the seeded colour
// rule against dbcore.ShippedPriceColorRules, the canonical copy of that same
// rule kept on the other side of the import DAG - the two are read together
// nowhere else, so nothing else catches them drifting apart.
func TestThePriceColumnsShipTheColoursTheMigrationWrites(t *testing.T) {
	seedInto(t)
	for _, key := range []string{"price_prompt", "price_completion"} {
		column, err := ColumnByKey(key)
		if err != nil {
			t.Fatal(err)
		}
		if column.Colors == nil {
			t.Fatalf("%s ships with no colours, and the table has always painted a price", key)
		}
		seeded, err := json.Marshal(column.Colors)
		if err != nil {
			t.Fatal(err)
		}
		if string(seeded) != dbcore.ShippedPriceColorRules {
			t.Errorf("%s seeds %s, and dbcore.ShippedPriceColorRules holds %s", key, seeded, dbcore.ShippedPriceColorRules)
		}
	}
	// Nothing else ships coloured, so a colour on screen is either these two or
	// something the reader asked for.
	columns, err := ListColumns()
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range columns {
		if c.Colors != nil && c.Key != "price_prompt" && c.Key != "price_completion" {
			t.Errorf("%s ships coloured (%+v), and only the two price columns do", c.Key, c.Colors)
		}
	}
}
