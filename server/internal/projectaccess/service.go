package projectaccess

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Access values a client may set. The database accepts any text, and the
// predicate treats everything but AccessWorkspace as restricted.
const (
	AccessWorkspace  = "workspace"
	AccessRestricted = "restricted"
)

// Audit actions written by this package.
const (
	actionAccessChanged = "project.access_changed"
	actionMemberAdded   = "project.member_added"
	actionMemberRemoved = "project.member_removed"
)

var ErrProjectNotFound = errors.New("projectaccess: project not found")

// MemberInvalidError means a user id in the request is not a workspace member.
type MemberInvalidError struct{ Field string }

func (e *MemberInvalidError) Error() string { return "projectaccess: invalid " + e.Field }

// Service holds the rules for changing a project's access and members. Every
// method runs in one transaction that includes its audit rows.
type Service struct {
	Queries *db.Queries
	Tx      TxStarter
	// AuditWriter returns the writer bound to tx. Nil means the audit_event
	// table; tests replace it to fail the write.
	AuditWriter func(tx pgx.Tx) audit.Writer
}

// Actor is the person who makes a change, with the request fields the audit
// row records.
type Actor struct {
	UserID    pgtype.UUID
	Email     string
	IP        string
	UserAgent string
	RequestID string
}

// SwitchResult reports what a switch did.
type SwitchResult struct {
	Access    string
	AutoAdded int
	// Changed is false when the project already had this access value.
	Changed bool
	// PreviousViewers are the people who could see the project before the
	// switch. They are the only recipients of project:access_changed.
	PreviousViewers []string
}

// SwitchAccess sets the project's access and grants memberIDs. Switching to
// restricted also grants the creator, the lead and the assignees of the
// project's issues, so nobody loses the place they work in. No issue, comment
// or file changes, and no grant is removed in either direction.
func (s *Service) SwitchAccess(ctx context.Context, a Actor, workspaceID, projectID pgtype.UUID, access string, memberIDs []pgtype.UUID) (SwitchResult, error) {
	res := SwitchResult{Access: access}
	err := inTx(ctx, s.Tx, s.Queries, func(q *db.Queries, tx pgx.Tx) error {
		// The row lock serializes concurrent switches on this project.
		p, err := q.LockProjectAccess(ctx, db.LockProjectAccessParams{ID: projectID, WorkspaceID: workspaceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		}
		if err != nil {
			return err
		}
		if err := requireMembers(ctx, q, workspaceID, memberIDs, "member_ids"); err != nil {
			return err
		}
		res.Changed = p.Access != access
		if res.Changed {
			if res.PreviousViewers, err = AudienceUsers(ctx, q, workspaceID, projectID); err != nil {
				return err
			}
		}
		auto := int64(0)
		if access == AccessRestricted {
			if auto, err = q.GrantProjectMembersFromProject(ctx, db.GrantProjectMembersFromProjectParams{
				ProjectID: projectID, WorkspaceID: workspaceID, AddedBy: a.UserID,
			}); err != nil {
				return err
			}
		}
		added, err := q.GrantProjectMembers(ctx, db.GrantProjectMembersParams{
			ProjectID: projectID, WorkspaceID: workspaceID, AddedBy: a.UserID, UserIds: memberIDs,
		})
		if err != nil {
			return err
		}
		if err := q.SetProjectAccess(ctx, db.SetProjectAccessParams{ID: projectID, WorkspaceID: workspaceID, Access: access}); err != nil {
			return err
		}
		issues, err := q.CountProjectIssues(ctx, db.CountProjectIssuesParams{ProjectID: projectID, WorkspaceID: workspaceID})
		if err != nil {
			return err
		}
		res.AutoAdded = int(auto)
		return s.record(ctx, tx, a, workspaceID, audit.Entry{
			Action:      actionAccessChanged,
			TargetType:  "project",
			TargetID:    util.UUIDToString(projectID),
			TargetLabel: p.Title,
			Detail: map[string]any{
				"from":       p.Access,
				"to":         access,
				"auto_added": auto,
				"added":      len(added),
				"issues":     issues,
			},
		})
	})
	return res, err
}

// Grant adds userIDs to the project and returns the ones that were not
// members of it yet. Adding an existing member does nothing.
func (s *Service) Grant(ctx context.Context, a Actor, workspaceID, projectID pgtype.UUID, userIDs []pgtype.UUID) ([]pgtype.UUID, error) {
	var added []pgtype.UUID
	err := inTx(ctx, s.Tx, s.Queries, func(q *db.Queries, tx pgx.Tx) error {
		if _, err := q.LockProjectAccess(ctx, db.LockProjectAccessParams{ID: projectID, WorkspaceID: workspaceID}); errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		} else if err != nil {
			return err
		}
		if err := requireMembers(ctx, q, workspaceID, userIDs, "user_ids"); err != nil {
			return err
		}
		var err error
		if added, err = q.GrantProjectMembers(ctx, db.GrantProjectMembersParams{
			ProjectID: projectID, WorkspaceID: workspaceID, AddedBy: a.UserID, UserIds: userIDs,
		}); err != nil {
			return err
		}
		for _, id := range added {
			if err := s.record(ctx, tx, a, workspaceID, audit.Entry{
				Action:     actionMemberAdded,
				TargetType: "member",
				TargetID:   util.UUIDToString(id),
				Detail:     map[string]any{"project_id": util.UUIDToString(projectID)},
			}); err != nil {
				return err
			}
		}
		return nil
	})
	return added, err
}

// Revoke removes userID from the project. Removing someone who is not a
// member does nothing.
func (s *Service) Revoke(ctx context.Context, a Actor, workspaceID, projectID, userID pgtype.UUID) error {
	return inTx(ctx, s.Tx, s.Queries, func(q *db.Queries, tx pgx.Tx) error {
		if _, err := q.LockProjectAccess(ctx, db.LockProjectAccessParams{ID: projectID, WorkspaceID: workspaceID}); errors.Is(err, pgx.ErrNoRows) {
			return ErrProjectNotFound
		} else if err != nil {
			return err
		}
		n, err := q.RevokeProjectMember(ctx, db.RevokeProjectMemberParams{ProjectID: projectID, UserID: userID})
		if err != nil || n == 0 {
			return err
		}
		return s.record(ctx, tx, a, workspaceID, audit.Entry{
			Action:     actionMemberRemoved,
			TargetType: "member",
			TargetID:   util.UUIDToString(userID),
			Detail:     map[string]any{"project_id": util.UUIDToString(projectID)},
		})
	})
}

// requireMembers fails with MemberInvalidError{field} unless every id is a
// member of the workspace.
func requireMembers(ctx context.Context, q *db.Queries, workspaceID pgtype.UUID, ids []pgtype.UUID, field string) error {
	if len(ids) == 0 {
		return nil
	}
	found, err := q.FilterWorkspaceMemberUserIDs(ctx, db.FilterWorkspaceMemberUserIDsParams{WorkspaceID: workspaceID, UserIds: ids})
	if err != nil {
		return err
	}
	have := make(map[pgtype.UUID]bool, len(found))
	for _, id := range found {
		have[id] = true
	}
	for _, id := range ids {
		if !have[id] {
			return &MemberInvalidError{Field: field}
		}
	}
	return nil
}

func (s *Service) record(ctx context.Context, tx pgx.Tx, a Actor, workspaceID pgtype.UUID, e audit.Entry) error {
	var w audit.Writer = auditstore.NewTx(s.Queries, tx)
	if s.AuditWriter != nil {
		w = s.AuditWriter(tx)
	}
	e.WorkspaceID = util.UUIDToString(workspaceID)
	e.ActorType = audit.ActorMember
	e.ActorID = util.UUIDToString(a.UserID)
	e.ActorLabel = a.Email
	e.IP, e.UserAgent, e.RequestID = a.IP, a.UserAgent, a.RequestID
	return audit.Record(ctx, w, e)
}
