package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// WorkspaceGate is the claim gate's view of the workspace state. The platform
// owns the state; the service takes it through this one method so it does not
// depend on the platform packages.
type WorkspaceGate interface {
	// Refusal returns the code that refuses a new agent task in the workspace,
	// or "" when the workspace is active. An error means the state could not be
	// read; the claim treats it as a refusal and the task stays queued.
	Refusal(ctx context.Context, workspaceID pgtype.UUID) (string, error)
}
