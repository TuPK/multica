package workspaceguard

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	"github.com/multica-ai/multica/server/internal/util"
)

// StateReader reads a workspace's state. *workspacestate.Guard is one.
type StateReader interface {
	State(ctx context.Context, workspaceID string) (workspacestate.Record, error)
}

// ClaimGate answers the task claim's question: may an agent start a new task in
// this workspace? It implements service.WorkspaceGate. Every status other than
// active refuses, including one this build does not know.
type ClaimGate struct{ state StateReader }

// NewClaimGate returns the claim gate over a state reader.
func NewClaimGate(state StateReader) *ClaimGate { return &ClaimGate{state: state} }

// Refusal returns the refusal code of the workspace, or "" when it is active.
func (g *ClaimGate) Refusal(ctx context.Context, workspaceID pgtype.UUID) (string, error) {
	if !workspaceID.Valid {
		return "", errors.New("workspace gate: invalid workspace id")
	}
	rec, err := g.state.State(ctx, util.UUIDToString(workspaceID))
	if err != nil {
		return "", err
	}
	return CodeFor(string(rec.Status)), nil
}
