package store

import (
	"errors"
	"modelsdb/internal/dbcore"
	"testing"
)

// A build whose display name an ordinary model already carries is refused: filing
// it would hide that model and make it a child of another.
func TestABuildNeverTakesOverAnOrdinaryModelOfTheSameName(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Parent")
	addModelRow(t, "Parent (Q4)")

	err := ImportVariation(Variation{Parent: "Parent", Name: "Parent (Q4)", HFSlug: "x/y-GGUF"})
	if !errors.Is(err, ErrModelExists) {
		t.Fatalf("err = %v, want ErrModelExists", err)
	}
	identity, _, _ := IdentityOf("Parent (Q4)")
	if identity.VariantOf != "" || identity.Hidden {
		t.Errorf("the ordinary model was changed: %+v", identity)
	}

	// A repeat import of a real build still refreshes it.
	if err := ImportVariation(Variation{Parent: "Parent", Name: "Parent (Q8)", HFSlug: "x/y-GGUF"}); err != nil {
		t.Fatal(err)
	}
	if err := ImportVariation(Variation{Parent: "Parent", Name: "Parent (Q8)", HFSlug: "x/y-GGUF"}); err != nil {
		t.Errorf("a repeat import of a build was refused: %v", err)
	}
}

func TestIsUniqueViolation(t *testing.T) {
	seedInto(t)
	addModelRow(t, "Twice")
	_, err := dbcore.DB.Exec("INSERT INTO models (name, source, updated_at) VALUES ('Twice','manual','t')")
	if !isUniqueViolation(err) {
		t.Errorf("a duplicate primary key was not recognised: %v", err)
	}
	if isUniqueViolation(nil) || isUniqueViolation(errors.New("some other failure")) {
		t.Error("something that is not a duplicate key was recognised as one")
	}
}
