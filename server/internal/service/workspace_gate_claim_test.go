package service

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// fakeWorkspaceGate records the workspaces it was asked about and answers with
// a fixed code or error.
type fakeWorkspaceGate struct {
	code  string
	err   error
	calls []pgtype.UUID
}

func (g *fakeWorkspaceGate) Refusal(_ context.Context, ws pgtype.UUID) (string, error) {
	g.calls = append(g.calls, ws)
	return g.code, g.err
}

func gateUUID(b byte) pgtype.UUID {
	var u pgtype.UUID
	u.Valid = true
	u.Bytes[0] = b
	return u
}

// claimKinds covers every way RunClassOfTask can resolve a task. The tasks that
// an autopilot run enqueued (periodic, and the exempt brief and catch-up runs)
// cost a database read to classify. The tests below pass a nil *db.Queries, so
// a gate that ran after the classification would panic on those tasks.
func claimKinds() map[string]db.AgentTaskQueue {
	return map[string]db.AgentTaskQueue{
		"human, direct":            {OriginatorSource: pgtype.Text{String: "direct_human", Valid: true}},
		"human, chat turn":         {ChatSessionID: gateUUID(1)},
		"routine, wakeup":          {},
		"routine, trigger owner":   {OriginatorSource: pgtype.Text{String: "trigger_owner", Valid: true}},
		"delegated, delegation":    {OriginatorSource: pgtype.Text{String: "delegation", Valid: true}},
		"delegated, comment":       {OriginatorSource: pgtype.Text{String: "comment_source", Valid: true}},
		"autopilot run (periodic)": {AutopilotRunID: gateUUID(2), OriginatorSource: pgtype.Text{String: "trigger_owner", Valid: true}},
		"autopilot run (exempt)":   {AutopilotRunID: gateUUID(3)},
	}
}

func TestClaimRefusalForRefusesNonActiveWorkspaceForEveryRunKind(t *testing.T) {
	ws := gateUUID(9)
	for _, code := range []string{"workspace_read_only", "workspace_suspended", "workspace_pending_deletion"} {
		for name, task := range claimKinds() {
			gate := &fakeWorkspaceGate{code: code}
			s := &TaskService{WorkspaceGate: gate}
			// A nil *db.Queries: the gate must answer before anything is read.
			refusal, err := s.claimRefusalFor(context.Background(), nil, ws, task)
			if err != nil {
				t.Fatalf("%s / %s: %v", code, name, err)
			}
			if refusal == nil {
				t.Fatalf("%s / %s: not refused", code, name)
			}
			if refusal.Reason != code {
				t.Errorf("%s / %s: reason = %q", code, name, refusal.Reason)
			}
			if refusal.WorkspaceID != ws || refusal.Task.ID != task.ID {
				t.Errorf("%s / %s: refusal does not carry the workspace and task", code, name)
			}
			if len(gate.calls) != 1 || gate.calls[0] != ws {
				t.Errorf("%s / %s: gate asked about %v, want the claim's workspace once", code, name, gate.calls)
			}
		}
	}
}

func TestClaimRefusalForRefusalIsAClaimRefusalError(t *testing.T) {
	s := &TaskService{WorkspaceGate: &fakeWorkspaceGate{code: "workspace_suspended"}}
	// admitClaimedTask returns the refusal itself, which the claim loop matches
	// with errors.As to roll back and leave the task queued.
	err := s.admitClaimedTask(context.Background(), nil, gateUUID(9), db.AgentTaskQueue{ID: gateUUID(4)})
	var refusal *claimRefusal
	if !errors.As(err, &refusal) {
		t.Fatalf("err = %v, want a *claimRefusal", err)
	}
	if refusal.Reason != "workspace_suspended" {
		t.Errorf("reason = %q", refusal.Reason)
	}
}

func TestClaimRefusalForGateErrorFailsClosed(t *testing.T) {
	boom := errors.New("redis and postgres are down")
	for name, task := range claimKinds() {
		s := &TaskService{WorkspaceGate: &fakeWorkspaceGate{err: boom}}
		refusal, err := s.claimRefusalFor(context.Background(), nil, gateUUID(9), task)
		if !errors.Is(err, boom) {
			t.Errorf("%s: err = %v, want the gate error", name, err)
		}
		if refusal != nil {
			t.Errorf("%s: a gate error returned a refusal %+v; it must be an error so the transaction rolls back", name, refusal)
		}
		// admitClaimedTask must not admit on an error.
		if err := s.admitClaimedTask(context.Background(), nil, gateUUID(9), task); !errors.Is(err, boom) {
			t.Errorf("%s: admitClaimedTask err = %v, want the gate error", name, err)
		}
	}
}

// A human run is admitted without reading anything, so it is the one kind whose
// pre-gate behaviour can be checked here without a database.
func TestClaimRefusalForActiveWorkspaceAndNilGateBehaveAsBefore(t *testing.T) {
	human := claimKinds()["human, direct"]
	ws := gateUUID(9)

	active := &fakeWorkspaceGate{}
	for name, s := range map[string]*TaskService{
		"active gate": {WorkspaceGate: active},
		"nil gate":    {},
	} {
		refusal, err := s.claimRefusalFor(context.Background(), nil, ws, human)
		if refusal != nil || err != nil {
			t.Errorf("%s: human run = (%+v, %v), want admitted", name, refusal, err)
		}
	}
	if len(active.calls) != 1 {
		t.Errorf("active gate asked %d times, want 1", len(active.calls))
	}
}

func TestWorkspaceGateIsOptionalOnTheService(t *testing.T) {
	var s TaskService
	if s.WorkspaceGate != nil {
		t.Fatal("a zero TaskService has a gate")
	}
}
