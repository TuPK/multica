package projectaccess

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Recipients are the people one project's realtime events go to (spec 7.1
// rule 13).
type Recipients struct {
	// Workspace means every non-guest member, who all sit in the workspace
	// room. It is true only for a project (or no project) with access
	// "workspace".
	Workspace bool
	// UserIDs are the people who need a delivery of their own. For a
	// restricted project this is the whole audience. When Workspace is true it
	// lists only the granted guests, because guests are not in the workspace
	// room.
	UserIDs []string
}

// AudienceQuerier is the part of *db.Queries that Audience reads.
type AudienceQuerier interface {
	GetProjectAccess(ctx context.Context, arg db.GetProjectAccessParams) (db.GetProjectAccessRow, error)
	ListProjectMembers(ctx context.Context, projectID pgtype.UUID) ([]db.ListProjectMembersRow, error)
	ListMembers(ctx context.Context, workspaceID pgtype.UUID) ([]db.Member, error)
}

// Audience returns who receives the events of projectID. An invalid
// projectID is an issue without a project: the whole workspace.
//
// Each member is judged by the same Scope.Allows that ForViewer feeds, so
// this is not a second visibility rule.
func Audience(ctx context.Context, q AudienceQuerier, workspaceID, projectID pgtype.UUID) (Recipients, error) {
	return audience(ctx, q, workspaceID, projectID, false)
}

// AudienceUsers lists every person who sees projectID, guests and the members
// of the workspace room included. Use it when two audiences must be compared
// person by person, as when an issue moves between projects.
func AudienceUsers(ctx context.Context, q AudienceQuerier, workspaceID, projectID pgtype.UUID) ([]string, error) {
	r, err := audience(ctx, q, workspaceID, projectID, true)
	return r.UserIDs, err
}

func audience(ctx context.Context, q AudienceQuerier, workspaceID, projectID pgtype.UUID, enumerate bool) (Recipients, error) {
	if !projectID.Valid && !enumerate {
		return Recipients{Workspace: true}, nil
	}
	restricted := false
	granted := map[pgtype.UUID]bool{}
	if projectID.Valid {
		p, err := q.GetProjectAccess(ctx, db.GetProjectAccessParams{ID: projectID, WorkspaceID: workspaceID})
		if errors.Is(err, pgx.ErrNoRows) {
			return Recipients{}, ErrProjectNotFound
		}
		if err != nil {
			return Recipients{}, err
		}
		restricted = p.Access != AccessWorkspace
		rows, err := q.ListProjectMembers(ctx, projectID)
		if err != nil {
			return Recipients{}, err
		}
		for _, r := range rows {
			granted[r.UserID] = true
		}
		// Nobody needs a delivery of their own: the workspace room reaches
		// every non-guest, and there is no granted guest to add.
		if !restricted && len(rows) == 0 && !enumerate {
			return Recipients{Workspace: true}, nil
		}
	}
	members, err := q.ListMembers(ctx, workspaceID)
	if err != nil {
		return Recipients{}, err
	}
	out := Recipients{Workspace: !restricted && !enumerate}
	for _, m := range members {
		s := scopeOfMember(m.Role, m.IsGuest, m.UserID)
		if s.mode != ModeAll {
			s.restricted = map[pgtype.UUID]bool{projectID: restricted}
			s.granted = map[pgtype.UUID]bool{projectID: granted[m.UserID]}
		}
		if !s.Allows(projectID) {
			continue
		}
		if out.Workspace && !m.IsGuest {
			continue
		}
		out.UserIDs = append(out.UserIDs, util.UUIDToString(m.UserID))
	}
	return out, nil
}
