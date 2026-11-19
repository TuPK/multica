// Package runtimeguard tells whether platform staff disabled a runtime (spec R6
// section 3b.5, D98). A disabled runtime cannot register, heartbeat or claim
// tasks. The table is read on every daemon request, so a disable takes effect
// on the next request.
package runtimeguard

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Querier is the one read the guard makes; *db.Queries satisfies it.
type Querier interface {
	GetPlatformRuntimeDisabled(ctx context.Context, runtimeID pgtype.UUID) (bool, error)
}

// Disabled reports whether staff disabled the runtime. A database error is
// returned as is: the guard fails closed, so the caller answers 503 and does
// not guess.
func Disabled(ctx context.Context, q Querier, runtimeID pgtype.UUID) (bool, error) {
	disabled, err := q.GetPlatformRuntimeDisabled(ctx, runtimeID)
	if err != nil {
		return false, fmt.Errorf("read platform runtime state: %w", err)
	}
	return disabled, nil
}

var _ Querier = (*db.Queries)(nil)
