package store

import (
	"errors"
	"testing"
)

// The rule that an OpenRouter row is hidden and never deleted lives in the store,
// so a caller other than the route cannot bypass it.
func TestAnOpenRouterRowCannotBeDeletedFromTheStore(t *testing.T) {
	seedInto(t)
	addOpenRouterRow(t, "Listed model", nil)
	addModelRow(t, "Hand added")

	if err := DeleteModelPermanently("Listed model"); !errors.Is(err, ErrListedByOpenRouter) {
		t.Errorf("deleting an OpenRouter row: err = %v, want ErrListedByOpenRouter", err)
	}
	if _, held, _ := IdentityOf("Listed model"); !held {
		t.Error("the OpenRouter row was deleted")
	}
	if err := DeleteModelPermanently("Hand added"); err != nil {
		t.Errorf("deleting a hand-added row: %v", err)
	}
	if _, held, _ := IdentityOf("Hand added"); held {
		t.Error("the hand-added row is still there")
	}
	if err := DeleteModelPermanently("Never existed"); !errors.Is(err, ErrNoSuchModel) {
		t.Errorf("deleting a missing row: err = %v, want ErrNoSuchModel", err)
	}
}

func TestHidingAMissingModelNamesItAsMissing(t *testing.T) {
	seedInto(t)
	if err := SetHidden("Never existed", true); !errors.Is(err, ErrNoSuchModel) {
		t.Errorf("err = %v, want ErrNoSuchModel", err)
	}
}
