package memberprovision

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/handler"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Kinds of member request the job runs through the handler (spec R6 D94).
const (
	kindRemove     = "remove"
	kindChangeRole = "change_role"
)

// MemberChanger runs a staff remove or role change. The handler implements it.
type MemberChanger interface {
	RemoveMemberForStaff(ctx context.Context, workspaceID, userID string, origin handler.DeletionOrigin) (bool, error)
	ChangeMemberRoleForStaff(ctx context.Context, workspaceID, userID, previousRole, role string, origin handler.DeletionOrigin) (bool, error)
}

// changeMember runs one remove or change_role request. The handler commits its
// own transaction; qtx, which holds the request's lock, marks the request
// completed in this one. A refusal from the handler is final for the request.
func (r *Runner) changeMember(ctx context.Context, qtx *db.Queries, req db.PlatformMemberRequest) error {
	if r.d.Members == nil || !req.TargetUserID.Valid {
		return &permanentError{Code: errInternal}
	}
	origin := handler.DeletionOrigin{
		Kind:       handler.DeletionOriginStaff,
		RequestID:  uuid.UUID(req.ID.Bytes).String(),
		StaffID:    uuid.UUID(req.RequestedBy.Bytes).String(),
		StaffLabel: req.RequestedByLabel,
	}
	workspaceID := uuid.UUID(req.WorkspaceID.Bytes).String()
	userID := uuid.UUID(req.TargetUserID.Bytes).String()

	var err error
	switch req.Kind {
	case kindRemove:
		_, err = r.d.Members.RemoveMemberForStaff(ctx, workspaceID, userID, origin)
	case kindChangeRole:
		_, err = r.d.Members.ChangeMemberRoleForStaff(ctx, workspaceID, userID, req.PreviousRole, req.Role, origin)
	default:
		return &permanentError{Code: errInternal}
	}
	var refused *handler.StaffMemberError
	if errors.As(err, &refused) {
		return &permanentError{Code: refused.Code}
	}
	if err != nil {
		return err
	}
	return qtx.MarkMemberRequestCompleted(ctx, db.MarkMemberRequestCompletedParams{
		ID:          req.ID,
		UserID:      req.TargetUserID,
		MemberID:    pgtype.UUID{},
		UserCreated: false,
	})
}
