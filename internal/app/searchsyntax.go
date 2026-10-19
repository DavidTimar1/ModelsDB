package app

// The search syntax every text box understands: words, "quoted phrases", a
// leading ! to negate a term, AND and OR between terms. It lives in one place
// so name=, contains: and /api/columns/values?contains= agree, and its
// JavaScript twin in static/app.js (between the SEARCH-SYNTAX markers) is held
// to the same cases in testdata/search_cases.json. Parsing never fails: a box
// is read on every keystroke, so half-typed text must mean something.

import "strings"

// searchTerm is one word or phrase to look for, lower case.
type searchTerm struct {
	text    string
	negated bool
}

// searchExpr is an OR of groups, each group an AND of terms. No groups means
// the box said nothing, which matches everything.
type searchExpr struct {
	groups [][]searchTerm
}

// searchBlank is the white space that separates words; kept to ASCII so the
// JavaScript twin splits identically.
func searchBlank(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r'
}

// parseSearch reads a box's text leniently: an unclosed quote runs to the end,
// a stray operator, a lone ! and empty quotes are dropped, and ! negates only
// at the start of a term. Only uppercase AND and OR standing alone are
// operators; everything inside quotes is literal.
func parseSearch(text string) searchExpr {
	var expr searchExpr
	var group []searchTerm
	closeGroup := func() {
		if len(group) > 0 {
			expr.groups = append(expr.groups, group)
			group = nil
		}
	}
	i, n := 0, len(text)
	for i < n {
		if searchBlank(text[i]) {
			i++
			continue
		}
		negated := false
		if text[i] == '!' {
			negated = true
			i++
		}
		var term string
		quoted := false
		if i < n && text[i] == '"' {
			quoted = true
			end := strings.IndexByte(text[i+1:], '"')
			if end < 0 {
				term = text[i+1:]
				i = n
			} else {
				term = text[i+1 : i+1+end]
				i += end + 2
			}
		} else {
			start := i
			for i < n && !searchBlank(text[i]) {
				i++
			}
			term = text[start:i]
		}
		if !negated && !quoted {
			if term == "AND" {
				continue
			}
			if term == "OR" {
				closeGroup()
				continue
			}
		}
		if term == "" {
			continue
		}
		group = append(group, searchTerm{text: strings.ToLower(term), negated: negated})
	}
	closeGroup()
	return expr
}

// matches answers whether a cell satisfies the expression. members are the
// texts the cell holds: one for a text cell, one per member for a list cell,
// none for an empty list. A positive term is true when any member holds it, so
// different terms may be met by different members; a negated term is true
// when no member holds it, which an empty cell satisfies.
func (e searchExpr) matches(members []string) bool {
	if len(e.groups) == 0 {
		return true
	}
	lowered := make([]string, len(members))
	for i, m := range members {
		lowered[i] = strings.ToLower(m)
	}
	for _, group := range e.groups {
		met := true
		for _, term := range group {
			found := false
			for _, m := range lowered {
				if strings.Contains(m, term.text) {
					found = true
					break
				}
			}
			if found == term.negated {
				met = false
				break
			}
		}
		if met {
			return true
		}
	}
	return false
}
