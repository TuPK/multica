package workspaceguard

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspacepolicy"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Session is what a request proves about how its caller signed in.
type Session struct {
	// Machine marks a credential that is not a person's session: the daemon's
	// token, a personal access token or an agent's task token. Policies about
	// sessions do not apply to it.
	Machine bool
	// AMR lists the sign-in methods of the session.
	AMR []string
	// AuthTime is the Unix second of the login, valid when AuthTimeKnown.
	AuthTime      int64
	AuthTimeKnown bool
}

// CheckWorkspaceAccess applies the session policies of one workspace to a
// request's session and returns the refusal code, or "" when the request may
// proceed. require_sso is stored but refuses nothing until P36.
func CheckWorkspaceAccess(p workspacepolicy.Policy, s Session, now time.Time) string {
	if s.Machine {
		return ""
	}
	if p.RequireMFA && !workspacepolicy.HasMFA(s.AMR) {
		return CodeMFARequired
	}
	if workspacepolicy.SessionTooOld(s.AuthTime, s.AuthTimeKnown, now, p.SessionMaxAgeHours) {
		return CodeReauthRequired
	}
	return ""
}

// PolicyFeatureAvailable reports whether the plan of a workspace includes
// workspace_policies. Cloud sends `enforce` for a plan without the feature and
// `off` for one with it; a stale or missing answer counts as available, as for
// every other gate. With no provider (no MULTICA_CLOUD_URL) it is available.
func PolicyFeatureAvailable(ctx context.Context, provider entitlement.Provider, workspaceID pgtype.UUID) bool {
	if provider == nil || !workspaceID.Valid {
		return true
	}
	d := provider.Gate(ctx, uuid.UUID(workspaceID.Bytes), entitlement.GateWorkspacePolicies)
	return d.Gate.Action != entitlement.ActionEnforce
}

// PolicyFromRow converts a stored row.
func PolicyFromRow(row db.WorkspacePolicy) workspacepolicy.Policy {
	domains := row.AllowedInviteDomains
	if domains == nil {
		domains = []string{}
	}
	return workspacepolicy.Policy{
		AllowedInviteDomains: domains,
		ShareLinksEnabled:    row.ShareLinksEnabled,
		RequireSSO:           row.RequireSso,
		RequireMFA:           row.RequireMfa,
		SessionMaxAgeHours:   row.SessionMaxAgeHours,
	}
}

// PolicyStore reads the policy that is in force for a workspace: the stored
// row when the plan has the feature, and the defaults when the workspace has no
// row or the plan lacks the feature.
type PolicyStore struct {
	q            *db.Queries
	entitlements entitlement.Provider
}

// NewPolicyStore returns a store over q. entitlements may be nil.
func NewPolicyStore(q *db.Queries, entitlements entitlement.Provider) *PolicyStore {
	return &PolicyStore{q: q, entitlements: entitlements}
}

// Policy returns the policy in force for the workspace.
func (s *PolicyStore) Policy(ctx context.Context, workspaceID string) (workspacepolicy.Policy, error) {
	id, err := util.ParseUUID(workspaceID)
	if err != nil {
		return workspacepolicy.Policy{}, err
	}
	return EffectivePolicy(ctx, s.q, s.entitlements, id)
}

// EffectivePolicy is PolicyStore.Policy for a caller that holds the queries.
// The row is read first, so a workspace with no row costs no plan lookup.
func EffectivePolicy(ctx context.Context, q *db.Queries, provider entitlement.Provider, workspaceID pgtype.UUID) (workspacepolicy.Policy, error) {
	row, err := q.GetWorkspacePolicy(ctx, workspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return workspacepolicy.Default(), nil
	}
	if err != nil {
		return workspacepolicy.Policy{}, err
	}
	if !PolicyFeatureAvailable(ctx, provider, workspaceID) {
		return workspacepolicy.Default(), nil
	}
	return PolicyFromRow(row), nil
}
