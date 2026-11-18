package invoice

import (
	"context"
	"errors"
	"testing"
)

type unsupportedReplace struct {
	*Fake
}

func (unsupportedReplace) ReplaceSupported() bool { return false }

// replaceSpy fails the test if Replace reaches the store; the embedded nil
// Store would panic on any other call.
type replaceSpy struct {
	Store
	replaced int
}

func (s *replaceSpy) ReplaceIssued(context.Context, string, int, []byte, string, string) (Record, error) {
	s.replaced++
	return Record{}, nil
}

func TestReplaceUnsupportedReturnsNoLocalCancel(t *testing.T) {
	buyer := Buyer{Name: "A", TaxCode: "1", Address: "x", Email: "a@example.com"}

	store := &replaceSpy{}
	svc := NewService(store, unsupportedReplace{NewFake()}, Config{})
	if _, err := svc.Replace(context.Background(), "order", 1, buyer, "actor", "reason"); !errors.Is(err, ErrReplaceUnsupported) {
		t.Errorf("err = %v, want ErrReplaceUnsupported", err)
	}
	if store.replaced != 0 {
		t.Errorf("the record was cancelled locally")
	}

	// The fake does not implement ReplaceSupported, so it stays supported.
	store = &replaceSpy{}
	svc = NewService(store, NewFake(), Config{})
	if _, err := svc.Replace(context.Background(), "order", 1, buyer, "actor", "reason"); err != nil {
		t.Errorf("fake provider: %v", err)
	}
	if store.replaced != 1 {
		t.Errorf("fake provider replace reached the store %d times, want 1", store.replaced)
	}
}
