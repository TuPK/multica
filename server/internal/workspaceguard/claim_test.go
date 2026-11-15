package workspaceguard

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/workspacestate"
)

type fakeState struct {
	rec  workspacestate.Record
	err  error
	asks []string
}

func (f *fakeState) State(_ context.Context, id string) (workspacestate.Record, error) {
	f.asks = append(f.asks, id)
	return f.rec, f.err
}

func testUUID() pgtype.UUID {
	return pgtype.UUID{Bytes: [16]byte{0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77, 0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff, 0x00}, Valid: true}
}

const testUUIDString = "11223344-5566-7788-99aa-bbccddeeff00"

func TestClaimGateRefusalCodePerStatus(t *testing.T) {
	for status, want := range map[workspacestate.Status]string{
		workspacestate.Active:          "",
		workspacestate.ReadOnly:        "workspace_read_only",
		workspacestate.Suspended:       "workspace_suspended",
		workspacestate.PendingDeletion: "workspace_pending_deletion",
		"something_new":                "workspace_suspended",
	} {
		st := &fakeState{rec: workspacestate.Record{Status: status}}
		got, err := NewClaimGate(st).Refusal(context.Background(), testUUID())
		if err != nil || got != want {
			t.Errorf("status %q: Refusal = (%q, %v), want (%q, nil)", status, got, err, want)
		}
		if len(st.asks) != 1 || st.asks[0] != testUUIDString {
			t.Errorf("status %q: state asked for %v, want the canonical workspace id", status, st.asks)
		}
	}
}

func TestClaimGateStateErrorIsReturned(t *testing.T) {
	boom := errors.New("state unavailable")
	code, err := NewClaimGate(&fakeState{err: boom}).Refusal(context.Background(), testUUID())
	if !errors.Is(err, boom) || code != "" {
		t.Fatalf("Refusal = (%q, %v), want the error", code, err)
	}
}

func TestClaimGateRefusesAnInvalidWorkspaceID(t *testing.T) {
	st := &fakeState{}
	if _, err := NewClaimGate(st).Refusal(context.Background(), pgtype.UUID{}); err == nil {
		t.Fatal("an invalid workspace id was admitted")
	}
	if len(st.asks) != 0 {
		t.Fatalf("state read for an invalid id: %v", st.asks)
	}
}
