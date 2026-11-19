package runtimeguard

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

type fakeQuerier struct {
	disabled bool
	err      error
}

func (f fakeQuerier) GetPlatformRuntimeDisabled(context.Context, pgtype.UUID) (bool, error) {
	return f.disabled, f.err
}

func TestDisabledReportsStaffState(t *testing.T) {
	id := pgtype.UUID{Valid: true}
	if got, err := Disabled(context.Background(), fakeQuerier{disabled: true}, id); err != nil || !got {
		t.Fatalf("Disabled = %v, %v; want true, nil", got, err)
	}
	if got, err := Disabled(context.Background(), fakeQuerier{}, id); err != nil || got {
		t.Fatalf("Disabled = %v, %v; want false, nil", got, err)
	}
}

// A read error is returned, never read as enabled: the caller must fail closed.
func TestDisabledReturnsReadErrorInsteadOfGuessing(t *testing.T) {
	boom := errors.New("connection reset")
	got, err := Disabled(context.Background(), fakeQuerier{err: boom}, pgtype.UUID{Valid: true})
	if !errors.Is(err, boom) || got {
		t.Fatalf("Disabled = %v, %v; want false with the read error", got, err)
	}
}
