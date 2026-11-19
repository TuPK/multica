package memberrequest

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/multica-ai/multica/platform-management/audit"
)

// Deps are the Service's dependencies.
type Deps struct {
	Runner Runner
	Reader Reader
	Seats  SeatReleaser
	Log    *slog.Logger
}

// Service creates, cancels and reads member requests.
type Service struct{ Deps }

// New builds the Service.
func New(d Deps) *Service {
	if d.Log == nil {
		d.Log = slog.Default()
	}
	return &Service{d}
}

const (
	targetMemberRequest = "member_request"
	defaultListLimit    = 50
)

// Create checks in, then stores a pending request and its audit row in one
// transaction. created is false when a live request already holds the workspace
// and email: the answer is then that request and nothing is written. It returns
// ErrNotFound for an unknown workspace.
func (s *Service) Create(ctx context.Context, by Actor, workspaceID string, in Input) (r Request, created bool, err error) {
	in, verr := ValidateInput(in)
	if verr != nil {
		return Request{}, false, verr
	}
	err = s.Runner.InTx(ctx, func(tx Tx) error {
		r, created = Request{}, false
		ws, found, err := tx.Workspace(ctx, workspaceID)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if ws.Status != WorkspaceActive {
			return &Error{Status: 409, Code: CodeWorkspaceNotActive, Message: "the workspace is not active", State: ws.Status}
		}
		if staff, err := tx.IsActiveStaff(ctx, in.Email); err != nil {
			return err
		} else if staff {
			return &Error{Status: 409, Code: CodeStaffCannotBeAdded, Message: "a staff member cannot be added to a workspace"}
		}
		if member, err := tx.IsMember(ctx, ws.ID, in.Email); err != nil {
			return err
		} else if member {
			return &Error{Status: 409, Code: CodeAlreadyMember, Message: "this person is already a member of the workspace"}
		}
		if live, ok, err := tx.Live(ctx, ws.ID, KindAdd, in.Email); err != nil {
			return err
		} else if ok {
			r = live
			return nil
		}
		row, inserted, err := tx.Insert(ctx, ws.ID, in.Email, in.Role, by)
		if err != nil {
			return err
		}
		if !inserted {
			// The unique index on the live key: another request committed first.
			winner, ok, err := tx.Live(ctx, ws.ID, KindAdd, in.Email)
			if err != nil {
				return err
			}
			if !ok {
				return fmt.Errorf("member request for %s conflicted but no live request is visible", in.Email)
			}
			r = winner
			return nil
		}
		r, created = row, true
		return audit.Record(ctx, tx, by.entry(ws, audit.Entry{
			Action: audit.ActionMemberStaffAddRequested, TargetType: targetMemberRequest, TargetID: row.ID, TargetLabel: row.Email,
			Detail: map[string]any{"role": row.Role},
		}))
	})
	if err != nil {
		return Request{}, false, err
	}
	return r, created, nil
}

// Cancel moves a pending request to canceled, with its audit row, then frees
// the seat the request may hold. It returns ErrNotFound for an unknown id and a
// 409 member_request_not_cancelable for any other state. A seat that cannot be
// released is logged: the cancel stands and the recovery worker retires the claim.
func (s *Service) Cancel(ctx context.Context, by Actor, id string) (Request, error) {
	var out Request
	err := s.Runner.InTx(ctx, func(tx Tx) error {
		cur, found, err := tx.Lock(ctx, id)
		if err != nil {
			return err
		}
		if !found {
			return ErrNotFound
		}
		if cur.State != StatePending {
			return &Error{Status: 409, Code: CodeNotCancelable, Message: "only a pending request can be canceled", State: cur.State}
		}
		ws, _, err := tx.Workspace(ctx, cur.WorkspaceID)
		if err != nil {
			return err
		}
		ws.ID = cur.WorkspaceID
		if out, err = tx.Cancel(ctx, id); err != nil {
			return err
		}
		return audit.Record(ctx, tx, by.entry(ws, audit.Entry{
			Action: audit.ActionMemberStaffAddCanceled, TargetType: targetMemberRequest, TargetID: id, TargetLabel: cur.Email,
			Detail: map[string]any{"role": cur.Role},
		}))
	})
	if err != nil {
		return Request{}, err
	}
	if s.Seats != nil {
		if err := s.Seats.Release(ctx, out.WorkspaceID, out.ID); err != nil {
			s.Log.Warn("release the seat of a canceled member request", "request", out.ID, "workspace", out.WorkspaceID, "error", err)
		}
	}
	return out, nil
}

// List returns the newest requests of a workspace, or ErrNotFound for an
// unknown workspace.
func (s *Service) List(ctx context.Context, workspaceID string) ([]Request, error) {
	if _, found, err := s.Reader.Workspace(ctx, workspaceID); err != nil {
		return nil, err
	} else if !found {
		return nil, ErrNotFound
	}
	return s.Reader.List(ctx, workspaceID, defaultListLimit)
}

// Get returns found == false for an unknown id.
func (s *Service) Get(ctx context.Context, id string) (Request, bool, error) {
	return s.Reader.Get(ctx, id)
}

func (a Actor) entry(ws Workspace, e audit.Entry) audit.Entry {
	e.ActorType, e.ActorID, e.ActorLabel = audit.ActorStaff, a.ID, a.Label
	e.WorkspaceID, e.TenantID = ws.ID, ws.TenantID
	return e
}
