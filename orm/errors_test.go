package orm

import (
	"errors"
	"testing"
)

func TestErrorSentinels(t *testing.T) {
	if ErrDoesNotExist == nil {
		t.Fatal("ErrDoesNotExist must not be nil")
	}
	if ErrMultipleObjectsReturned == nil {
		t.Fatal("ErrMultipleObjectsReturned must not be nil")
	}
	if errors.Is(ErrDoesNotExist, ErrMultipleObjectsReturned) {
		t.Fatal("ErrDoesNotExist and ErrMultipleObjectsReturned must be distinct")
	}
	if !errors.Is(ErrDoesNotExist, ErrDoesNotExist) {
		t.Fatal("errors.Is(ErrDoesNotExist, ErrDoesNotExist) must be true")
	}
	if !errors.Is(ErrMultipleObjectsReturned, ErrMultipleObjectsReturned) {
		t.Fatal("errors.Is(ErrMultipleObjectsReturned, ErrMultipleObjectsReturned) must be true")
	}
}
