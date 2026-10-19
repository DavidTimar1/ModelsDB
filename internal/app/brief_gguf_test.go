package app

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
)

// TestAGGUFMainRowIsOfferedNoSizeAndTheBriefSaysWhy: no GGUF file is
// "the" weights, so the brief neither asks for a size nor points at a re-import
// that would fill none.
func TestAGGUFMainRowIsOfferedNoSizeAndTheBriefSaysWhy(t *testing.T) {
	migratedDB(t)
	if rec := addModel(t, `{"name":"Repo: Pictures GGUF","cells":{"model_type":"image","gguf_architecture":"qwen_image21"}}`); rec.Code != http.StatusCreated {
		t.Fatalf("add status = %d: %s", rec.Code, rec.Body.String())
	}
	asked, body := askedFields(t, "Repo: Pictures GGUF")
	for _, gone := range []string{"weights_size_gb"} {
		if asked[gone] != nil {
			t.Errorf("a GGUF main row is offered %s", gone)
		}
	}
	if !strings.Contains(fmt.Sprint(body["brief"]), "It is a GGUF repository") {
		t.Error("the brief does not say why no size is offered")
	}
}
