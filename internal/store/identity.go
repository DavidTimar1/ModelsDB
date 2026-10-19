// Deciding whether a model arriving from somewhere is one this
// catalog already holds.
//
// A row is STORED under its display name, and that name is the primary key, but
// it is not what a model IS. The same model reaches this app under two different
// names all the time: OpenRouter calls it "inclusionAI: Ling 3.0 Tiny (free)"
// and its repository address turns into "inclusionAI: Ling-3.0-tiny". Asking
// only whether the incoming NAME is taken therefore answers a different question
// from the one every creating path means to ask, and answers it wrong: it lets a
// second row be created for a model already here, after which a note, a
// favourite star and a column value can be split across the two with nothing
// saying so.
//
// So every path that creates a row asks THIS question instead, and there is one
// implementation of it for all of them.

package store

import (
	"database/sql"
	"errors"
	"fmt"
	"modelsdb/internal/dbcore"
	"strings"
)

// The three identifiers a row can be recognised by, strongest first. They are
// the words the refusals and the import answer use for the identifier that
// matched. The repository is spelled hf_slug there, which the page and the API
// read, although the column it is matched in is called repository.
const (
	MatchByRepository = "hf_slug"
	MatchByUpstreamID = "id"
	MatchByName       = "name"
)

// ModelMatch is the existing row an incoming model turns out to be: the name it
// is stored under, which identifier recognised it, and the value that matched.
type ModelMatch struct {
	Name string `json:"name"`
	By   string `json:"by"`
	Key  string `json:"key"`
}

// Matched words the identifier this row was recognised by, for a refusal that
// has to say WHY two differently-named things are one model.
func (m ModelMatch) Matched() string {
	switch m.By {
	case MatchByRepository:
		return fmt.Sprintf("the HuggingFace repository %q", m.Key)
	case MatchByUpstreamID:
		return fmt.Sprintf("the OpenRouter id %q", m.Key)
	}
	return fmt.Sprintf("the name %q", m.Key)
}

// FindExistingModel names the row this catalog already holds for an incoming
// model, or reports that it holds none.
//
// THE ORDER IS THE POINT. A repository address and an upstream id survive a
// rename upstream and a display name does not, so they are asked first: a model
// whose OpenRouter listing is renamed tomorrow is still recognised by the
// repository it was read from. The name is the last resort, and it is compared
// exactly, character for character, because that is how the primary key compares
// it - a lookup that matched more loosely there would answer with a row a write
// could not then address.
//
// The repository is compared IGNORING CAPITALS. One repository is written in
// several ways by the people and the sources that name it, and the two answers
// this can get wrong are not equal: recognising two spellings as one repository
// costs a refusal the reader can see and read, while failing to recognise them
// creates the silent second row this function exists to prevent.
//
// AN EMPTY IDENTIFIER NEVER MATCHES. Most rows carry no repository and many
// carry no id, so a blank probe would otherwise match the first of them and
// report an unrelated model as the same one.
//
// The order among rows sharing an identifier is fixed rather than left to
// SQLite, so the same catalog always answers with the same row: a variation - one
// downloadable build, filed under a model - sorts after the models, and then by
// name.
func FindExistingModel(name, id, hfSlug string) (ModelMatch, bool, error) {
	probes := []struct {
		by, key          string
		columns          []string
		ignoringCapitals bool
	}{
		{MatchByRepository, strings.TrimSpace(hfSlug), repositoryColumns, true},
		{MatchByUpstreamID, strings.TrimSpace(id), upstreamIDColumns, false},
	}
	for _, probe := range probes {
		found, held, err := FindNameByCell(probe.columns, probe.key, probe.ignoringCapitals)
		if err != nil {
			return ModelMatch{}, false, err
		}
		if held {
			return ModelMatch{Name: found, By: probe.by, Key: probe.key}, true, nil
		}
	}

	// The name is the last resort, and it is the row's own key rather than a cell:
	// it is what every value hangs off, so it is compared exactly, character for
	// character, the way the primary key compares it.
	if name != "" {
		var found string
		err := dbcore.DB.QueryRow(`SELECT name FROM models WHERE name=?
			ORDER BY (variant_of<>''), name LIMIT 1`, name).Scan(&found)
		if err == nil {
			return ModelMatch{Name: found, By: MatchByName, Key: name}, true, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return ModelMatch{}, false, err
		}
	}
	return ModelMatch{}, false, nil
}
