package planlimit

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// OwnerLimitEnv names the variable that caps how many workspaces one user may
// own while each of them is on the free plan.
const OwnerLimitEnv = "FREE_PLAN_OWNED_WORKSPACE_LIMIT"

// FreePlanKey is the plan of a workspace that has no subscription row, and the
// key of the free plan in the catalogue.
const FreePlanKey = "free"

// ParseOwnerLimit reads the variable. Empty and 0 mean no limit.
func ParseOwnerLimit(raw string) (int, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return 0, nil
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("%s must be a whole number of 0 or more, got %q", OwnerLimitEnv, raw)
	}
	return n, nil
}

// OwnerLimitReached counts the free workspaces among those a user owns and
// reports whether creating one more is refused. ownedPlanKeys has one entry per
// owned workspace: its subscription's plan key, or "" when it has no
// subscription row. A workspace on any other plan is paid and is not counted.
// A limit of 0 or less is no limit.
func OwnerLimitReached(limit int, ownedPlanKeys []string) (used int, reached bool) {
	for _, key := range ownedPlanKeys {
		if key == "" || key == FreePlanKey {
			used++
		}
	}
	return used, limit > 0 && used >= limit
}

// OwnerGate names the advisory lock that serialises one user's workspace
// creations. Its scope is the user, so users never wait on each other.
const OwnerGate = "free_owned_workspaces"

// OwnerStore is what CheckOwnerLimit reads. It must be bound to the
// transaction that creates the workspace. *db.Queries satisfies it.
type OwnerStore interface {
	Locker
	ListOwnedWorkspacePlanKeys(ctx context.Context, userID pgtype.UUID) ([]string, error)
}

// OwnerLimitError reports that the user owns the most free workspaces allowed.
type OwnerLimitError struct {
	Limit int
	Used  int
}

func (e *OwnerLimitError) Error() string {
	return fmt.Sprintf("free workspace limit reached: owns %d of %d", e.Used, e.Limit)
}

// CheckOwnerLimit refuses a new workspace for a user who owns `limit` free
// ones. It takes the user's advisory lock first, so two simultaneous creations
// by one user count one after the other; the caller creates the workspace on
// the same transaction before it commits. A limit of 0 or less runs nothing.
func CheckOwnerLimit(ctx context.Context, q OwnerStore, userID pgtype.UUID, limit int) (used int, err error) {
	if limit <= 0 {
		return 0, nil
	}
	if err := q.LockPlanGate(ctx, db.LockPlanGateParams{
		Scope: uuid.UUID(userID.Bytes).String(), Gate: OwnerGate,
	}); err != nil {
		return 0, fmt.Errorf("lock owner workspace creation: %w", err)
	}
	keys, err := q.ListOwnedWorkspacePlanKeys(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("list owned workspaces: %w", err)
	}
	used, reached := OwnerLimitReached(limit, keys)
	if reached {
		return used, &OwnerLimitError{Limit: limit, Used: used}
	}
	return used, nil
}
