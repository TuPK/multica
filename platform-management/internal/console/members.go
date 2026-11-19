package console

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/memberrequest"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// MemberRequests is the staff add-member service (spec R6 section 4).
// *memberrequest.Service is one.
type MemberRequests interface {
	Create(ctx context.Context, by memberrequest.Actor, workspaceID string, in memberrequest.Input) (r memberrequest.Request, created bool, err error)
	Cancel(ctx context.Context, by memberrequest.Actor, id string) (memberrequest.Request, error)
	List(ctx context.Context, workspaceID string) ([]memberrequest.Request, error)
	Get(ctx context.Context, id string) (r memberrequest.Request, found bool, err error)
}

// memberRequestJSON is a request as the console shows it.
type memberRequestJSON struct {
	ID               string  `json:"id"`
	WorkspaceID      string  `json:"workspace_id"`
	Kind             string  `json:"kind"`
	Email            string  `json:"email"`
	Role             string  `json:"role"`
	State            string  `json:"state"`
	ErrorCode        string  `json:"error_code"`
	Attempts         int     `json:"attempts"`
	UserID           string  `json:"user_id,omitempty"`
	MemberID         string  `json:"member_id,omitempty"`
	UserCreated      bool    `json:"user_created"`
	RequestedBy      string  `json:"requested_by"`
	RequestedByLabel string  `json:"requested_by_label"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	CompletedAt      *string `json:"completed_at"`
}

func toMemberRequestJSON(r memberrequest.Request) memberRequestJSON {
	out := memberRequestJSON{
		ID: r.ID, WorkspaceID: r.WorkspaceID, Kind: r.Kind, Email: r.Email, Role: r.Role, State: r.State,
		ErrorCode: r.ErrorCode, Attempts: r.Attempts, UserID: r.UserID, MemberID: r.MemberID, UserCreated: r.UserCreated,
		RequestedBy: r.RequestedBy, RequestedByLabel: r.RequestedByLabel,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if r.CompletedAt != nil {
		s := r.CompletedAt.UTC().Format(time.RFC3339)
		out.CompletedAt = &s
	}
	return out
}

// failMember answers a service refusal with its status and code, an unknown
// workspace or request with the 404 of an unknown id, and anything else as fail
// does.
func (h *Handler) failMember(w http.ResponseWriter, r *http.Request, op string, err error) {
	var me *memberrequest.Error
	switch {
	case errors.As(err, &me):
		body := map[string]any{"error": me.Message, "code": me.Code}
		if len(me.Fields) > 0 {
			body["fields"] = me.Fields
		}
		if me.State != "" {
			key := "state"
			if me.Code == memberrequest.CodeWorkspaceNotActive {
				key = "status"
			}
			body[key] = me.State
		}
		writeJSON(w, me.Status, body)
	case errors.Is(err, memberrequest.ErrNotFound):
		writeError(w, notFound())
	default:
		h.fail(w, r, op, err)
	}
}

func (h *Handler) memberActor(r *http.Request) (memberrequest.Actor, error) {
	a, err := h.currentActor(r)
	if err != nil {
		return memberrequest.Actor{}, err
	}
	return memberrequest.Actor{ID: a.ID, Label: a.Email}, nil
}

// visibleWorkspace answers the 404 of an unknown id, and returns false, when the
// workspace is outside the scope. It runs before any other check, so a workspace
// of another tenant reads as one that does not exist (spec R6 section 4.2,
// rule 1).
func (h *Handler) visibleWorkspace(w http.ResponseWriter, r *http.Request, scope Scope, workspaceID string) bool {
	ok, err := h.workspaceVisible(r.Context(), scope, workspaceID)
	if err != nil {
		h.fail(w, r, "check workspace scope", err)
		return false
	}
	if !ok {
		writeError(w, notFound())
		return false
	}
	return true
}

// addMember stores a pending add-member request. It answers 202 with the
// request, or 200 with the live request that already holds the email.
func (h *Handler) addMember(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionMembersAdd)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !h.visibleWorkspace(w, r, scope, id) {
		return
	}
	var in memberrequest.Input
	if perr := decodeBody(w, r, &in); perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.memberActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, created, err := h.Members.Create(r.Context(), by, id, in)
	if err != nil {
		h.failMember(w, r, "create member request", err)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusAccepted
	}
	writeJSON(w, status, toMemberRequestJSON(req))
}

func (h *Handler) listMemberRequests(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionMemberRequests)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !h.visibleWorkspace(w, r, scope, id) {
		return
	}
	rows, err := h.Members.List(r.Context(), id)
	if err != nil {
		h.failMember(w, r, "list member requests", err)
		return
	}
	out := make([]memberRequestJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, toMemberRequestJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": out})
}

// visibleMemberRequest reads the request and answers the 404 of an unknown id,
// and returns false, when it does not exist or its workspace is outside the scope.
func (h *Handler) visibleMemberRequest(w http.ResponseWriter, r *http.Request, scope Scope, id string) (memberrequest.Request, bool) {
	req, found, err := h.Members.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get member request", err)
		return memberrequest.Request{}, false
	}
	if !found {
		writeError(w, notFound())
		return memberrequest.Request{}, false
	}
	if !h.visibleWorkspace(w, r, scope, req.WorkspaceID) {
		return memberrequest.Request{}, false
	}
	return req, true
}

func (h *Handler) getMemberRequest(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionMemberRequests)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	req, ok := h.visibleMemberRequest(w, r, scope, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toMemberRequestJSON(req))
}

func (h *Handler) cancelMemberRequest(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionMemberRequests)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if _, ok := h.visibleMemberRequest(w, r, scope, id); !ok {
		return
	}
	by, err := h.memberActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, err := h.Members.Cancel(r.Context(), by, id)
	if err != nil {
		h.failMember(w, r, "cancel member request", err)
		return
	}
	writeJSON(w, http.StatusOK, toMemberRequestJSON(req))
}
