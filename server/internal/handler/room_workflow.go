package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// writeRoomWorkflowError answers a write refused by the room rules with 422 and
// the machine-readable code, so the UI can open its workflow chooser and the CLI
// or an agent can report what to do. It reports whether err was such a refusal.
func writeRoomWorkflowError(w http.ResponseWriter, err error) bool {
	var rwe *issueworkflow.RoomWorkflowError
	if !errors.As(err, &rwe) {
		return false
	}
	writeJSON(w, http.StatusUnprocessableEntity, map[string]string{
		"error": rwe.Message,
		"code":  rwe.Code,
	})
	return true
}

// isRoomWorkflowRefusal reports whether err is a write refused by the room rules.
func isRoomWorkflowRefusal(err error) bool {
	var rwe *issueworkflow.RoomWorkflowError
	return errors.As(err, &rwe)
}

// roomWorkflowPolicyBody is the non-request policy as the API carries it:
// kind is no_action, default, or workflow; workflow_id is set for workflow.
type roomWorkflowPolicyBody struct {
	Kind       string `json:"kind"`
	WorkflowID string `json:"workflow_id,omitempty"`
}

type roomWorkflowPolicyEnvelope struct {
	NonRequestWorkflow roomWorkflowPolicyBody `json:"non_request_workflow"`
}

// GetRoomWorkflowPolicy returns the workspace's choice for issues that are not
// requests of a flow room. Members can read it; the popup needs it.
func (h *Handler) GetRoomWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	policy, err := issueworkflow.LoadNonRequestPolicy(r.Context(), h.Queries, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the workflow policy")
		return
	}
	writeJSON(w, http.StatusOK, roomWorkflowPolicyEnvelope{NonRequestWorkflow: roomWorkflowPolicyBodyOf(policy)})
}

// PutRoomWorkflowPolicy stores the workspace's choice. Admins only (the route
// group enforces it). A workflow choice must name a workflow in this workspace.
func (h *Handler) PutRoomWorkflowPolicy(w http.ResponseWriter, r *http.Request) {
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	var req roomWorkflowPolicyEnvelope
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body := req.NonRequestWorkflow
	var workflowID pgtype.UUID
	if body.Kind == issueworkflow.NonRequestWorkflow {
		id, ok := parseUUIDOrBadRequest(w, body.WorkflowID, "workflow_id")
		if !ok {
			return
		}
		if _, err := h.Queries.GetIssueWorkflow(r.Context(), db.GetIssueWorkflowParams{ID: id, WorkspaceID: wsUUID}); err != nil {
			writeError(w, http.StatusBadRequest, "workflow not found in this workspace")
			return
		}
		workflowID = id
	}
	policy, err := issueworkflow.NewNonRequestPolicy(body.Kind, workflowID)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	value, err := policy.SettingValue()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the workflow policy")
		return
	}
	if _, err := h.Queries.SetWorkspaceNonRequestWorkflow(r.Context(), db.SetWorkspaceNonRequestWorkflowParams{ID: wsUUID, Value: value}); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save the workflow policy")
		return
	}
	writeJSON(w, http.StatusOK, roomWorkflowPolicyEnvelope{NonRequestWorkflow: roomWorkflowPolicyBodyOf(policy)})
}

// roomWorkflowCheck is the answer to a preview: whether the write would be
// refused, and why, so the form can ask before it is sent.
type roomWorkflowCheck struct {
	NeedsWorkflow bool   `json:"needs_workflow"`
	Code          string `json:"code,omitempty"`
	Message       string `json:"message,omitempty"`
	SquadID       string `json:"squad_id,omitempty"`
}

// CheckRoomWorkflow previews the room rules for a new or moved issue. The query
// takes the same inputs the write would: project_id, assignee_type, assignee_id,
// and parent_issue_id. An issue with a parent and no project inherits the parent's.
func (h *Handler) CheckRoomWorkflow(w http.ResponseWriter, r *http.Request) {
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	q := r.URL.Query()
	in := issueworkflow.RoomIssue{WorkspaceID: wsUUID, AssigneeType: pgtype.Text{String: q.Get("assignee_type"), Valid: q.Get("assignee_type") != ""}}
	if v := q.Get("assignee_id"); v != "" {
		id, ok := parseUUIDOrBadRequest(w, v, "assignee_id")
		if !ok {
			return
		}
		in.AssigneeID = id
	}
	if v := q.Get("project_id"); v != "" {
		id, ok := parseUUIDOrBadRequest(w, v, "project_id")
		if !ok {
			return
		}
		in.ProjectID = id
	}
	if v := q.Get("parent_issue_id"); v != "" {
		id, ok := parseUUIDOrBadRequest(w, v, "parent_issue_id")
		if !ok {
			return
		}
		parent, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: id, WorkspaceID: wsUUID})
		if err != nil {
			writeError(w, http.StatusNotFound, "parent issue not found")
			return
		}
		in.ParentIssueID = id
		if !in.ProjectID.Valid {
			in.ProjectID = parent.ProjectID
		}
	}

	result := roomWorkflowCheck{}
	policy, err := issueworkflow.LoadNonRequestPolicy(r.Context(), h.Queries, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check the workflow")
		return
	}
	if err := issueworkflow.CheckRoomWrite(r.Context(), h.Queries, in, policy); err != nil {
		var rwe *issueworkflow.RoomWorkflowError
		if !errors.As(err, &rwe) {
			writeError(w, http.StatusInternalServerError, "failed to check the workflow")
			return
		}
		result = roomWorkflowCheck{NeedsWorkflow: true, Code: rwe.Code, Message: rwe.Message, SquadID: uuidToString(rwe.Room)}
	}
	writeJSON(w, http.StatusOK, result)
}

func roomWorkflowPolicyBodyOf(p issueworkflow.NonRequestPolicy) roomWorkflowPolicyBody {
	body := roomWorkflowPolicyBody{Kind: p.Kind}
	if p.Kind == issueworkflow.NonRequestWorkflow {
		body.WorkflowID = uuidToString(p.WorkflowID)
	}
	return body
}
