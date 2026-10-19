package app

import (
	"modelsdb/internal/store"
	"reflect"
	"regexp"
	"strings"
	"testing"
)

// jsonTagsOf lists the JSON member names a struct serializes under.
func jsonTagsOf(v interface{}) map[string]bool {
	tags := map[string]bool{}
	typ := reflect.TypeOf(v)
	for i := 0; i < typ.NumField(); i++ {
		name := strings.Split(typ.Field(i).Tag.Get("json"), ",")[0]
		if name != "" && name != "-" {
			tags[name] = true
		}
	}
	return tags
}

// Every example object in the served guide that starts with "field" is a gap or a
// held entry; its members must be ones the real structs serialize, so the guide
// cannot describe a key the server never sends.
func TestGuideExamplesOfGapsAndHeldUseOnlyRealMembers(t *testing.T) {
	allowed := jsonTagsOf(store.Gap{})
	for key := range jsonTagsOf(store.Held{}) {
		allowed[key] = true
	}
	objectRe := regexp.MustCompile(`(?s)\{"field":.*?\}`)
	keyRe := regexp.MustCompile(`"([a-z_]+)":`)

	objects := objectRe.FindAllString(apiGuide, -1)
	if len(objects) == 0 {
		t.Fatal("found no example gap or held object in the guide")
	}
	for _, object := range objects {
		for _, m := range keyRe.FindAllStringSubmatch(object, -1) {
			if !allowed[m[1]] {
				t.Errorf("the guide shows %q in a gap or held example, which the server never sends:\n%s", m[1], object)
			}
		}
	}
}

// The brief example shows the "held" list the paragraph below it describes.
func TestGuideBriefExampleShowsHeld(t *testing.T) {
	section := apiGuide[strings.Index(apiGuide, "### GET /api/models/brief"):]
	section = section[:strings.Index(section[4:], "### ")+4]
	if !strings.Contains(section, `"held":[`) {
		t.Error(`the GET /api/models/brief example does not show "held"`)
	}
}
