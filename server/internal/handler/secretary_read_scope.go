package handler

import (
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A Secretary run in a chat turn, or a run a Secretary request starts, reads its
// requests and the answers on them. It does not read the company's board: the
// departments' work is what the leaders answer for. The scope is enforced here,
// not by the instructions, so every model is held to it alike.

// secretaryRunReadsTree reports whether the request is such a run: the
// Secretary's own run, bound to a chat turn or to a request's reply session.
func (h *Handler) secretaryRunReadsTree(r *http.Request, workspaceID pgtype.UUID) (bool, error) {
	actorType, actorID := h.resolveActor(r, requestUserID(r), util.UUIDToString(workspaceID))
	if actorType != "agent" {
		return false, nil
	}
	ws, err := h.Queries.GetWorkspace(r.Context(), workspaceID)
	if err != nil {
		return false, err
	}
	if !ws.SecretaryAgentID.Valid || util.UUIDToString(ws.SecretaryAgentID) != actorID {
		return false, nil
	}
	taskID, err := util.ParseUUID(r.Header.Get("X-Task-ID"))
	if err != nil {
		return false, nil
	}
	task, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil {
		return false, nil
	}
	return task.ChatSessionID.Valid || task.ReplyChatSessionID.Valid, nil
}

// refuseOutsideSecretaryTree writes the refusal and returns true when a
// Secretary run reads an issue outside its requests. It returns false otherwise,
// and also when the check cannot be made. The check fails closed: a database error
// writes a server error, not a read.
func (h *Handler) refuseOutsideSecretaryTree(w http.ResponseWriter, r *http.Request, issue db.Issue) bool {
	restricted, err := h.secretaryRunReadsTree(r, issue.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check the Secretary's read scope")
		return true
	}
	if !restricted {
		return false
	}
	inTree, err := service.IssueInSecretaryTree(r.Context(), h.Queries, issue)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check the Secretary's read scope")
		return true
	}
	if inTree {
		return false
	}
	writeError(w, http.StatusForbidden, "in this run the Secretary reads its requests and their answers only; ask the leader of the squad for anything else")
	return true
}

// refuseBoardListForSecretary limits a Secretary run's issue list to the Secretary
// desk, and writes the refusal when the list asks for more. It returns true when
// the list was refused.
func (h *Handler) refuseBoardListForSecretary(w http.ResponseWriter, r *http.Request, workspaceID pgtype.UUID) bool {
	restricted, err := h.secretaryRunReadsTree(r, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check the Secretary's read scope")
		return true
	}
	if !restricted {
		return false
	}
	desk, err := service.SecretaryDeskID(r.Context(), h.Queries, workspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check the Secretary's read scope")
		return true
	}
	if desk.Valid && r.URL.Query().Get("project_id") == util.UUIDToString(desk) && r.URL.Query().Get("project_ids") == "" {
		return false
	}
	writeError(w, http.StatusForbidden, "in this run the Secretary lists its desk only; ask the leader of the squad for anything else")
	return true
}
