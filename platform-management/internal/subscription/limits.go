package subscription

import (
	"context"
	"net/http"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Tenant limit overrides (spec R6 section 3b.4, rule 4, D97). One gate is set on
// every workspace of a tenant, or on a subset, in one transaction: a failure on
// any workspace changes none. The override is the same limit_overrides field
// that putSubscription writes for one workspace.

// Error codes of spec R6 section 3b.4.
const (
	CodeLimitGateInvalid     = "limit_gate_invalid"
	CodeLimitValueInvalid    = "limit_value_invalid"
	CodeReasonRequired       = "reason_required"
	CodeWorkspaceNotInTenant = "workspace_not_in_tenant"
)

// LimitOverride sets one gate. A nil Value releases the gate: the plan's limit applies again.
type LimitOverride struct {
	Gate  string
	Value *int64
}

// OverrideActor is the staff member who sets the override, for the audit row.
type OverrideActor struct {
	ID    string
	Label string
}

// checkOverride applies the input rules: a known gate, a non-negative value, and a reason.
func checkOverride(o LimitOverride, reason string) (string, *Error) {
	if !slices.Contains(plan.LimitGates, o.Gate) {
		return "", &Error{Status: http.StatusBadRequest, Code: CodeLimitGateInvalid, Message: "gate is not a plan limit"}
	}
	if o.Value != nil && *o.Value < 0 {
		return "", &Error{Status: http.StatusBadRequest, Code: CodeLimitValueInvalid, Message: "value must not be negative, or null to release the gate"}
	}
	reason = strings.TrimSpace(reason)
	if reason == "" || len([]rune(reason)) > 500 {
		return "", &Error{Status: http.StatusBadRequest, Code: CodeReasonRequired, Message: "a reason of 1 to 500 characters is required"}
	}
	return reason, nil
}

// withOverride returns a copy of limits with the gate set, or removed when the
// value is nil.
func withOverride(limits plan.Limits, o LimitOverride) plan.Limits {
	out := plan.Limits{}
	for k, v := range limits {
		out[k] = v
	}
	if o.Value == nil {
		delete(out, o.Gate)
	} else {
		v := *o.Value
		out[o.Gate] = &v
	}
	return out
}

// overrideValue is the gate's override value for the audit row; nil when absent or released.
func overrideValue(limits plan.Limits, gate string) any {
	if v, ok := limits[gate]; ok && v != nil {
		return *v
	}
	return nil
}

// SetLimitOverride sets o on each workspace in workspaceIDs, which belong to
// tenantID, in one transaction: one audit row per workspace with the tenant id,
// and the cached policy of the changed workspaces dropped after the commit. A
// workspace with no subscription row gets a free one first (rule 4).
func (s *Service) SetLimitOverride(ctx context.Context, tenantID string, workspaceIDs []string, o LimitOverride, reason string, by OverrideActor) error {
	reason, rerr := checkOverride(o, reason)
	if rerr != nil {
		return rerr
	}
	var changed []string
	err := s.runner.InTx(ctx, func(tx Tx) error {
		var err error
		changed, err = applyLimitOverrides(ctx, tx, tenantID, workspaceIDs, o, reason, by)
		return err
	})
	if err != nil {
		return err
	}
	for _, ws := range changed {
		s.cache.Invalidate(ws)
	}
	return nil
}

// SetLimitOverrideIn applies one workspace's override inside the caller's pgx
// transaction, so a quota approval writes its request row and the override
// together (spec R6 section 3c.5, rule 3). It returns the workspace changed; the
// caller drops the cached policy after the commit.
func SetLimitOverrideIn(ctx context.Context, tx pgx.Tx, tenantID, workspaceID string, o LimitOverride, reason string, by OverrideActor) error {
	reason, rerr := checkOverride(o, reason)
	if rerr != nil {
		return rerr
	}
	_, err := applyLimitOverrides(ctx, &pgxTx{q: db.New(tx)}, tenantID, []string{workspaceID}, o, reason, by)
	return err
}

// applyLimitOverrides writes o on each workspace inside tx and returns the
// workspaces written. It does not commit and does not drop caches.
func applyLimitOverrides(ctx context.Context, tx Tx, tenantID string, workspaceIDs []string, o LimitOverride, reason string, by OverrideActor) ([]string, error) {
	var changed []string
	{
		for _, ws := range workspaceIDs {
			name, found, err := tx.WorkspaceName(ctx, ws)
			if err != nil {
				return nil, err
			}
			if !found {
				return nil, NotFound()
			}
			current, exists, err := tx.Get(ctx, ws)
			if err != nil {
				return nil, err
			}
			before := current
			if !exists {
				before = baseline(ws)
			}
			next := before
			next.LimitOverrides = withOverride(before.LimitOverrides, o)
			var written Subscription
			if exists {
				written, err = tx.Update(ctx, next)
				if err != nil {
					return nil, err
				}
			} else {
				var ok bool
				written, ok, err = tx.Insert(ctx, next)
				if err != nil {
					return nil, err
				}
				if !ok {
					return nil, &Error{Status: http.StatusConflict, Code: "concurrent_change",
						Message: "the subscription was changed by someone else; read it again and retry"}
				}
			}
			if err := audit.Record(ctx, tx, audit.Entry{
				WorkspaceID: ws, TenantID: tenantID,
				ActorType: audit.ActorStaff, ActorID: by.ID, ActorLabel: by.Label,
				Action: audit.ActionSubscriptionLimitOverrideChanged, TargetType: targetWorkspace, TargetID: ws, TargetLabel: name,
				Detail: map[string]any{
					"gate": o.Gate, "from": overrideValue(before.LimitOverrides, o.Gate), "to": overrideValue(next.LimitOverrides, o.Gate),
					"reason": reason, "version": written.Version,
				},
			}); err != nil {
				return nil, err
			}
			changed = append(changed, ws)
		}
	}
	return changed, nil
}
