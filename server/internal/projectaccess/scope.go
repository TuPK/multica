// Package projectaccess decides which projects, and so which issues, a viewer
// sees (spec 7.1). Every read path asks the Scope computed here; no other code
// compares project.access or project_member.
package projectaccess

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ScopeMarker is the comment line every query that applies Fragment carries,
// so the read-path inventory test can find it.
const ScopeMarker = "-- projectaccess:scope"

// Visibility modes, passed to queries as the scope_mode parameter.
const (
	ModeAll    = "all"
	ModeMember = "member"
	ModeGuest  = "guest"
)

// ErrNotMember means the viewer has no member row in the workspace.
var ErrNotMember = errors.New("projectaccess: viewer is not a workspace member")

// Querier is the part of *db.Queries that ForViewer reads.
type Querier interface {
	GetViewerMember(ctx context.Context, arg db.GetViewerMemberParams) (db.GetViewerMemberRow, error)
	ListViewerProjectAccess(ctx context.Context, arg db.ListViewerProjectAccessParams) ([]db.ListViewerProjectAccessRow, error)
}

// Scope is what one viewer may see in one workspace. A caller that fails to
// compute one must fail the request; it must not continue without it.
type Scope struct {
	mode   string
	userID pgtype.UUID
	// restricted and granted list only the projects that differ from the
	// workspace default, so a workspace of plain projects costs no rows.
	restricted map[pgtype.UUID]bool
	granted    map[pgtype.UUID]bool
}

// System is the scope of a background job, which has no viewer and sees every
// project. A request handler must not use it for a viewer's read or write.
func System() Scope { return Scope{mode: ModeAll} }

// ForViewer computes the viewer's Scope. Owners and admins see everything and
// need no project list.
func ForViewer(ctx context.Context, q Querier, workspaceID, userID pgtype.UUID) (Scope, error) {
	m, err := q.GetViewerMember(ctx, db.GetViewerMemberParams{WorkspaceID: workspaceID, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Scope{}, ErrNotMember
	}
	if err != nil {
		return Scope{}, err
	}
	s := Scope{mode: ModeMember, userID: userID}
	switch {
	case m.Role == "owner" || m.Role == "admin":
		s.mode = ModeAll
		return s, nil
	case m.IsGuest:
		s.mode = ModeGuest
	}
	rows, err := q.ListViewerProjectAccess(ctx, db.ListViewerProjectAccessParams{WorkspaceID: workspaceID, UserID: userID})
	if err != nil {
		return Scope{}, err
	}
	s.restricted = map[pgtype.UUID]bool{}
	s.granted = map[pgtype.UUID]bool{}
	for _, r := range rows {
		if r.Restricted {
			s.restricted[r.ID] = true
		}
		if r.Granted {
			s.granted[r.ID] = true
		}
	}
	return s, nil
}

// Allows reports whether the viewer sees projectID. An invalid (NULL) id is an
// issue that belongs to no project.
func (s Scope) Allows(projectID pgtype.UUID) bool {
	switch {
	case s.mode == ModeAll:
		return true
	case !projectID.Valid:
		return s.mode != ModeGuest
	case s.granted[projectID]:
		return true
	case s.mode == ModeGuest:
		return false
	}
	return !s.restricted[projectID]
}

// Mode returns the scope_mode query parameter.
func (s Scope) Mode() string { return s.mode }

// UserID returns the scope_user_id query parameter.
func (s Scope) UserID() pgtype.UUID { return s.userID }

// CanManage reports whether the viewer is an owner or admin.
func (s Scope) CanManage() bool { return s.mode == ModeAll }

// fragment is the standard predicate. It is written for a query that already
// has a WHERE clause. "= 'workspace'" is exact on purpose: any other value of
// project.access is restricted.
const fragment = ScopeMarker + `
AND (sqlc.arg(scope_mode)::text = 'all'
    OR (COL IS NULL AND sqlc.arg(scope_mode)::text <> 'guest')
    OR EXISTS (SELECT 1 FROM project_member scope_pm
               WHERE scope_pm.project_id = COL AND scope_pm.user_id = sqlc.arg(scope_user_id)::uuid)
    OR (sqlc.arg(scope_mode)::text = 'member'
        AND EXISTS (SELECT 1 FROM project scope_p WHERE scope_p.id = COL AND scope_p.access = 'workspace')))`

// Fragment returns the standard SQL predicate for the project id expression
// column (for example "issue.project_id"). Paste it, marker line included,
// after the WHERE conditions of a query; it takes the sqlc parameters
// scope_mode (Scope.Mode) and scope_user_id (Scope.UserID).
func Fragment(column string) string {
	return strings.ReplaceAll(fragment, "COL", column)
}
