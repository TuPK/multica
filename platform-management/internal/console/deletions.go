package console

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/deletionrequest"
	"github.com/multica-ai/multica/platform-management/internal/staff"
)

// DeletionRequests is the immediate deletion service (spec R6 section 6).
// *deletionrequest.Service is one.
type DeletionRequests interface {
	RequestWorkspace(ctx context.Context, by deletionrequest.Actor, workspaceID string, in deletionrequest.Input) (deletionrequest.Request, bool, error)
	RequestTenant(ctx context.Context, by deletionrequest.Actor, tenantID string, in deletionrequest.Input) (deletionrequest.Request, bool, error)
	RequestRuntime(ctx context.Context, by deletionrequest.Actor, runtimeID string, in deletionrequest.Input) (deletionrequest.Request, bool, error)
	RequestAccount(ctx context.Context, by deletionrequest.Actor, userID string, in deletionrequest.AccountInput) (deletionrequest.Request, bool, error)
	PreviewAccount(ctx context.Context, by deletionrequest.Actor, userID string) (deletionrequest.AccountPreview, error)
	Cancel(ctx context.Context, by deletionrequest.Actor, id string) (deletionrequest.Request, error)
	Retry(ctx context.Context, by deletionrequest.Actor, id string) (deletionrequest.Request, error)
	List(ctx context.Context, f deletionrequest.Filter) ([]deletionrequest.Request, error)
	Get(ctx context.Context, id string) (deletionrequest.Request, bool, error)
}

// deletionRequestJSON is a request as the console shows it.
type deletionRequestJSON struct {
	ID               string `json:"id"`
	Kind             string `json:"kind"`
	TargetID         string `json:"target_id"`
	TargetLabel      string `json:"target_label"`
	Reason           string `json:"reason"`
	State            string `json:"state"`
	ErrorCode        string `json:"error_code"`
	Attempts         int32  `json:"attempts"`
	TenantID         string `json:"tenant_id,omitempty"`
	RequestedBy      string `json:"requested_by"`
	RequestedByLabel string `json:"requested_by_label"`
	CreatedAt        string `json:"created_at"`
	UpdatedAt        string `json:"updated_at"`
}

func toDeletionRequestJSON(r deletionrequest.Request) deletionRequestJSON {
	return deletionRequestJSON{
		ID: r.ID, Kind: r.Kind, TargetID: r.TargetID, TargetLabel: r.TargetLabel, Reason: r.Reason,
		State: r.State, ErrorCode: r.ErrorCode, Attempts: r.Attempts, TenantID: r.TenantID,
		RequestedBy: r.RequestedBy, RequestedByLabel: r.RequestedByLabel,
		CreatedAt: r.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: r.UpdatedAt.UTC().Format(time.RFC3339),
	}
}

// failDeletion answers a service refusal with its status and code, an unknown
// id with the 404 of an unknown id, and anything else as fail does.
func (h *Handler) failDeletion(w http.ResponseWriter, r *http.Request, op string, err error) {
	var de *deletionrequest.Error
	switch {
	case errors.As(err, &de):
		body := map[string]any{"error": de.Message, "code": de.Code}
		if de.State != "" {
			body["state"] = de.State
		}
		for k, v := range de.Extra {
			body[k] = v
		}
		if de.Code == deletionrequest.CodeLegalHold {
			a, aerr := h.currentActor(r)
			if aerr != nil {
				delete(body, "legal_hold_id")
			} else {
				hideHoldID(body, a.Role)
			}
		}
		writeJSON(w, de.Status, body)
	case errors.Is(err, deletionrequest.ErrNotFound):
		writeError(w, notFound())
	default:
		h.fail(w, r, op, err)
	}
}

func (h *Handler) deletionActor(r *http.Request) (deletionrequest.Actor, error) {
	a, err := h.currentActor(r)
	if err != nil {
		return deletionrequest.Actor{}, err
	}
	return deletionrequest.Actor{ID: a.ID, Label: a.Email}, nil
}

// deleteAnswer is the body of a 202: the request id and its state (spec R6 section 6.5).
type deleteAnswer struct {
	RequestID string `json:"request_id"`
	State     string `json:"state"`
}

// deleteWorkspaceNow requests the immediate deletion of a workspace. It answers
// 202 for a new request and 200 with the live request when one exists.
func (h *Handler) deleteWorkspaceNow(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionWorkspaceDeleteNow)
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
	var in deletionrequest.Input
	if perr := decodeBody(w, r, &in); perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, created, err := h.Deletions.RequestWorkspace(r.Context(), by, id, in)
	if err != nil {
		h.failDeletion(w, r, "request workspace deletion", err)
		return
	}
	writeJSON(w, deleteStatus(created), deleteAnswer{RequestID: req.ID, State: req.State})
}

// deleteTenantNow requests the immediate deletion of a tenant. Only system_admin
// has the route; any other scope is the flat 404.
func (h *Handler) deleteTenantNow(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionTenantDeleteNow)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !scope.All {
		writeError(w, notFound())
		return
	}
	var in deletionrequest.Input
	if perr := decodeBody(w, r, &in); perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, created, err := h.Deletions.RequestTenant(r.Context(), by, id, in)
	if err != nil {
		h.failDeletion(w, r, "request tenant deletion", err)
		return
	}
	writeJSON(w, deleteStatus(created), deleteAnswer{RequestID: req.ID, State: req.State})
}

// deleteAccountNow requests the immediate deletion of a user's account (spec R6
// section 7). Only system_admin has the route: any other scope is the flat 404,
// even for a user who belongs only to the admin's tenant (M25, D86).
func (h *Handler) deleteAccountNow(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionAccountDeleteNow)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !scope.All {
		writeError(w, notFound())
		return
	}
	var in deletionrequest.AccountInput
	if perr := decodeBody(w, r, &in); perr != nil {
		writeError(w, perr)
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	req, created, err := h.Deletions.RequestAccount(r.Context(), by, id, in)
	if err != nil {
		h.failDeletion(w, r, "request account deletion", err)
		return
	}
	writeJSON(w, deleteStatus(created), deleteAnswer{RequestID: req.ID, State: req.State})
}

// accountPreviewJSON is the preview the console builds the form from.
type accountPreviewJSON struct {
	Email       string                             `json:"email"`
	NeedChoice  []deletionrequest.AccountWorkspace `json:"need_choice"`
	DeletedWith []deletionrequest.AccountWorkspace `json:"deleted_with"`
}

// previewAccountDeleteNow returns the workspaces a deletion needs and writes nothing.
func (h *Handler) previewAccountDeleteNow(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionAccountDeleteNow)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	if !scope.All {
		writeError(w, notFound())
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	p, err := h.Deletions.PreviewAccount(r.Context(), by, id)
	if err != nil {
		h.failDeletion(w, r, "preview account deletion", err)
		return
	}
	writeJSON(w, http.StatusOK, accountPreviewJSON{Email: p.Email, NeedChoice: nonNilWS(p.NeedChoice), DeletedWith: nonNilWS(p.DeletedWith)})
}

func nonNilWS(in []deletionrequest.AccountWorkspace) []deletionrequest.AccountWorkspace {
	if in == nil {
		return []deletionrequest.AccountWorkspace{}
	}
	return in
}

func deleteStatus(created bool) int {
	if created {
		return http.StatusAccepted
	}
	return http.StatusOK
}

// listDeletionRequests returns the newest requests, filtered by kind and state.
// An admin sees only the workspace and tenant requests of its tenant.
func (h *Handler) listDeletionRequests(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionDeletionRequestsRead)
	if !ok {
		return
	}
	f := deletionrequest.Filter{Kind: r.URL.Query().Get("kind"), State: r.URL.Query().Get("state")}
	if !validDeletionKind(f.Kind) || !validDeletionState(f.State) {
		writeError(w, invalidFilter("kind or state is not a known value"))
		return
	}
	if !scope.All {
		f.TenantID = scope.TenantID
	}
	rows, err := h.Deletions.List(r.Context(), f)
	if err != nil {
		h.failDeletion(w, r, "list deletion requests", err)
		return
	}
	out := make([]deletionRequestJSON, 0, len(rows))
	for _, row := range rows {
		out = append(out, toDeletionRequestJSON(row))
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": out})
}

func validDeletionKind(k string) bool {
	return k == "" || k == deletionrequest.KindWorkspace || k == deletionrequest.KindTenant
}

func validDeletionState(s string) bool {
	switch s {
	case "", deletionrequest.StatePending, deletionrequest.StateRunning, deletionrequest.StateCompleted,
		deletionrequest.StateFailed, deletionrequest.StateCanceled:
		return true
	}
	return false
}

// visibleDeletion reads the request and answers the 404 of an unknown id, and
// returns false, when it does not exist or it is outside the scope. An admin's
// scope is its tenant and only the workspace and tenant kinds (spec R6 section 6.5).
func (h *Handler) visibleDeletion(w http.ResponseWriter, r *http.Request, scope Scope, id string) (deletionrequest.Request, bool) {
	req, found, err := h.Deletions.Get(r.Context(), id)
	if err != nil {
		h.fail(w, r, "get deletion request", err)
		return deletionrequest.Request{}, false
	}
	if !found || (!scope.All && (req.TenantID != scope.TenantID || req.TenantID == "" ||
		(req.Kind != deletionrequest.KindWorkspace && req.Kind != deletionrequest.KindTenant))) {
		writeError(w, notFound())
		return deletionrequest.Request{}, false
	}
	return req, true
}

func (h *Handler) getDeletionRequest(w http.ResponseWriter, r *http.Request) {
	scope, ok := h.scopeFor(w, r, staff.ActionDeletionRequestsRead)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	req, ok := h.visibleDeletion(w, r, scope, id)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, toDeletionRequestJSON(req))
}

// cancelDeletionRequest cancels a pending or failed request. An admin cancels
// only the requests it made (spec R6 section 6.2, rule 6).
func (h *Handler) cancelDeletionRequest(w http.ResponseWriter, r *http.Request) {
	h.actOnDeletion(w, r, func(by deletionrequest.Actor, id string) (deletionrequest.Request, error) {
		return h.Deletions.Cancel(r.Context(), by, id)
	}, "cancel deletion request")
}

// retryDeletionRequest sets a failed request back to pending. The same rule as cancel applies.
func (h *Handler) retryDeletionRequest(w http.ResponseWriter, r *http.Request) {
	h.actOnDeletion(w, r, func(by deletionrequest.Actor, id string) (deletionrequest.Request, error) {
		return h.Deletions.Retry(r.Context(), by, id)
	}, "retry deletion request")
}

func (h *Handler) actOnDeletion(w http.ResponseWriter, r *http.Request,
	act func(deletionrequest.Actor, string) (deletionrequest.Request, error), op string) {
	scope, ok := h.scopeFor(w, r, staff.ActionDeletionRequestsAct)
	if !ok {
		return
	}
	id, perr := pathID(r, "id")
	if perr != nil {
		writeError(w, perr)
		return
	}
	req, ok := h.visibleDeletion(w, r, scope, id)
	if !ok {
		return
	}
	by, err := h.deletionActor(r)
	if err != nil {
		h.fail(w, r, "resolve staff actor", err)
		return
	}
	if !scope.All && req.RequestedBy != by.ID {
		writeError(w, notFound())
		return
	}
	out, err := act(by, id)
	if err != nil {
		h.failDeletion(w, r, op, err)
		return
	}
	writeJSON(w, http.StatusOK, toDeletionRequestJSON(out))
}
