package workspaceguard

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/sso"
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
	// TokenWorkspaceID is the workspace a personal access token is bound to,
	// or empty for any other credential and for an unbound token.
	TokenWorkspaceID string
	// WorkspaceID is the workspace the request targets, or empty when the route
	// has no workspace context.
	WorkspaceID string
	// AllowNoWorkspace admits a bound token on a route with no workspace
	// context. Only GET /api/me is such a route.
	AllowNoWorkspace bool
	// IsOwner marks the owner of the workspace, who require_sso does not bind.
	IsOwner bool
	// SSOConnectionID is the active SSO connection of the workspace, or empty
	// when it has none. Only read when the policy requires SSO.
	SSOConnectionID string
	// SSOCovered reports that the caller's email is on a verified domain of the
	// workspace. Only read when the policy requires SSO.
	SSOCovered bool
}

// CheckWorkspaceAccess applies the session policies of one workspace to a
// request's session and returns the refusal code, or "" when the request may
// proceed. A personal access token bound to a workspace is refused on any other
// workspace first, whatever kind of credential it is. require_sso refuses a
// member of a verified domain whose session did not come through the
// workspace's SSO connection; the owner, a member outside the verified domains,
// and a workspace with no active connection are not refused.
func CheckWorkspaceAccess(p workspacepolicy.Policy, s Session, now time.Time) string {
	if s.TokenWorkspaceID != "" {
		switch {
		case s.WorkspaceID == "" && !s.AllowNoWorkspace:
			return CodeTokenWorkspaceMismatch
		case s.WorkspaceID != "" && !strings.EqualFold(s.WorkspaceID, s.TokenWorkspaceID):
			return CodeTokenWorkspaceMismatch
		}
	}
	if s.Machine {
		return ""
	}
	if p.RequireSSO && s.SSOCovered && s.SSOConnectionID != "" && !s.IsOwner && !sso.HasAMR(s.AMR, s.SSOConnectionID) {
		return CodeSSORequired
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

// SSOAccess is what require_sso needs to know about one member of a workspace.
type SSOAccess struct {
	// ConnectionID is the workspace's active SSO connection, or empty when it
	// has none: with no way to sign in through SSO, requiring it would lock
	// everyone out.
	ConnectionID string
	// Covered reports that the member's email is on a verified domain of the
	// workspace. Guests and contractors outside the domains follow the invite
	// policy instead (spec 6.3).
	Covered bool
}

// SSOAccess reads the SSO facts of userID in a workspace.
func (s *PolicyStore) SSOAccess(ctx context.Context, workspaceID, userID string) (SSOAccess, error) {
	ws, err := util.ParseUUID(workspaceID)
	if err != nil {
		return SSOAccess{}, err
	}
	uid, err := util.ParseUUID(userID)
	if err != nil {
		return SSOAccess{}, err
	}
	conn, err := s.q.GetSSOConnectionByWorkspace(ctx, ws)
	if errors.Is(err, pgx.ErrNoRows) {
		return SSOAccess{}, nil
	}
	if err != nil {
		return SSOAccess{}, err
	}
	if conn.Status != "active" {
		return SSOAccess{}, nil
	}
	user, err := s.q.GetUser(ctx, uid)
	if err != nil {
		return SSOAccess{}, err
	}
	domain := sso.EmailDomain(user.Email)
	if domain == "" {
		return SSOAccess{ConnectionID: util.UUIDToString(conn.ID)}, nil
	}
	covered, err := s.q.HasVerifiedSSODomain(ctx, db.HasVerifiedSSODomainParams{WorkspaceID: ws, Domain: domain})
	if err != nil {
		return SSOAccess{}, err
	}
	return SSOAccess{ConnectionID: util.UUIDToString(conn.ID), Covered: covered}, nil
}
