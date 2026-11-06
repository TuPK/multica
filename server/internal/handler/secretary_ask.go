package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// `multica secretary ask` in one request. The Secretary files a request and its
// children, sets the request's metadata, and sets the deadline, all in one call,
// so a request is never left half-filed. It accepts only from the Secretary's
// own turn in the boss's chat, and the chat must be the one that turn belongs to.

const (
	secretaryAskQuestionLimit = 2000
	secretaryAskAddresseeMax  = 20
	secretaryAskWakeupInstr   = "The deadline has passed: read the answers and assess what is in."
)

type SecretaryAskAddressee struct {
	SquadID     string `json:"squad_id"`
	AgentID     string `json:"agent_id"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

type SecretaryAskRequest struct {
	Question      string                  `json:"question"`
	ChatSessionID string                  `json:"chat_session_id"`
	DueDate       string                  `json:"due_date"`
	Deadline      string                  `json:"deadline"`
	Addressees    []SecretaryAskAddressee `json:"addressees"`
}

type SecretaryAskResponse struct {
	RequestID     string   `json:"request_id"`
	ChildIDs      []string `json:"child_ids"`
	WakeupID      string   `json:"wakeup_id"`
	ChatSessionID string   `json:"chat_session_id"`
}

// SecretaryAsk files a Secretary request for a judgment question.
func (h *Handler) SecretaryAsk(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req SecretaryAskRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	run, secretaryID, ok := h.secretaryTurnForAsk(w, r, wsUUID)
	if !ok {
		return
	}
	sessionID, err := util.ParseUUID(req.ChatSessionID)
	if err != nil || !run.ChatSessionID.Valid || util.UUIDToString(run.ChatSessionID) != util.UUIDToString(sessionID) {
		writeError(w, http.StatusForbidden, "a Secretary request is filed from the turn in the chat it answers")
		return
	}

	question := strings.TrimSpace(req.Question)
	if question == "" {
		writeError(w, http.StatusBadRequest, "question is required")
		return
	}
	if utf8.RuneCountInString(question) > secretaryAskQuestionLimit {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("question must be at most %d characters", secretaryAskQuestionLimit))
		return
	}
	dueDate, err := time.Parse("2006-01-02", req.DueDate)
	if err != nil {
		writeError(w, http.StatusBadRequest, "due_date must be a calendar day, YYYY-MM-DD")
		return
	}
	deadline, err := time.Parse(time.RFC3339, req.Deadline)
	if err != nil || !deadline.After(time.Now()) {
		writeError(w, http.StatusBadRequest, "deadline must be a future RFC3339 time")
		return
	}
	if len(req.Addressees) == 0 || len(req.Addressees) > secretaryAskAddresseeMax {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("addressees must be between 1 and %d", secretaryAskAddresseeMax))
		return
	}
	for i, a := range req.Addressees {
		if (a.SquadID == "") == (a.AgentID == "") {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("addressee %d needs exactly one of squad_id or agent_id", i+1))
			return
		}
		if strings.TrimSpace(a.Title) == "" || strings.TrimSpace(a.Description) == "" {
			writeError(w, http.StatusBadRequest, fmt.Sprintf("addressee %d needs a title and a description", i+1))
			return
		}
	}

	desk, err := service.SecretaryDeskID(r.Context(), h.Queries, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to find the Secretary desk")
		return
	}
	if !desk.Valid {
		writeError(w, http.StatusConflict, "the Secretary desk does not exist; company setup creates it")
		return
	}
	if err := service.CheckSecretaryRequestCaps(r.Context(), h.Queries, wsUUID, desk, pgtype.UUID{}, true); err != nil {
		writeError(w, http.StatusConflict, err.Error())
		return
	}

	// Each step runs the handler that owns it, as the Secretary's run, so the rules
	// of issue creation, assignment, caps and wakeups apply unchanged. A failed step
	// cancels what this call already filed.
	var created []string
	fail := func(status int, msg string) {
		for _, id := range created {
			h.cancelIssueAsSecretary(r, wsUUID, id)
		}
		writeError(w, status, msg)
	}

	status, body := h.callAsSecretary(r, h.CreateIssue, workspaceID, "POST", "/api/issues", map[string]any{
		"title":         truncateRunes(question, 200),
		"description":   question,
		"status":        "todo",
		"project_id":    util.UUIDToString(desk),
		"assignee_type": "agent",
		"assignee_id":   util.UUIDToString(secretaryID),
		"due_date":      dueDate.Format("2006-01-02"),
	}, nil)
	if status != http.StatusCreated {
		fail(status, "file the request: "+body)
		return
	}
	var parentIssue IssueResponse
	if err := json.Unmarshal([]byte(body), &parentIssue); err != nil || parentIssue.ID == "" {
		fail(http.StatusInternalServerError, "file the request: no id in the response")
		return
	}
	created = append(created, parentIssue.ID)

	for _, key := range []struct {
		name  string
		value any
	}{
		{"chat_session_id", util.UUIDToString(sessionID)},
		{"asked_by", util.UUIDToString(run.OriginatorUserID)},
		{"deadline", deadline.UTC().Format(time.RFC3339)},
	} {
		raw, _ := json.Marshal(key.value)
		id, _ := util.ParseUUID(parentIssue.ID)
		if _, err := h.Queries.SetIssueMetadataKey(r.Context(), db.SetIssueMetadataKeyParams{
			Key: key.name, Value: raw, ID: id, WorkspaceID: wsUUID,
		}); err != nil {
			fail(http.StatusInternalServerError, "set request metadata: "+err.Error())
			return
		}
	}

	var childIDs []string
	for _, a := range req.Addressees {
		payload := map[string]any{
			"title":           truncateRunes(a.Title, 200),
			"description":     a.Description,
			"parent_issue_id": parentIssue.ID,
		}
		if a.SquadID != "" {
			payload["assignee_type"], payload["assignee_id"] = "squad", a.SquadID
		} else {
			payload["assignee_type"], payload["assignee_id"] = "agent", a.AgentID
		}
		status, child := h.callAsSecretary(r, h.CreateIssue, workspaceID, "POST", "/api/issues", payload, nil)
		if status != http.StatusCreated {
			fail(status, "file a child for an addressee: "+child)
			return
		}
		var childIssue IssueResponse
		if err := json.Unmarshal([]byte(child), &childIssue); err != nil || childIssue.ID == "" {
			fail(http.StatusInternalServerError, "file a child: no id in the response")
			return
		}
		created = append(created, childIssue.ID)
		childIDs = append(childIDs, childIssue.ID)
	}

	status, wake := h.callAsSecretary(r, h.CreateIssueWakeup, workspaceID, "POST", "/api/issues/"+parentIssue.ID+"/wakeups", map[string]any{
		"kind":        "at",
		"at":          deadline.UTC().Format(time.RFC3339),
		"mode":        "once",
		"instruction": secretaryAskWakeupInstr,
	}, map[string]string{"id": parentIssue.ID})
	if status != http.StatusCreated {
		fail(status, "set the deadline: "+wake)
		return
	}
	var wakeup struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal([]byte(wake), &wakeup)

	writeJSON(w, http.StatusCreated, SecretaryAskResponse{
		RequestID:     parentIssue.ID,
		ChildIDs:      childIDs,
		WakeupID:      wakeup.ID,
		ChatSessionID: util.UUIDToString(sessionID),
	})
}

// secretaryTurnForAsk authorizes the call: the caller is the workspace's Secretary,
// running a live chat turn whose originator owns that chat.
func (h *Handler) secretaryTurnForAsk(w http.ResponseWriter, r *http.Request, wsUUID pgtype.UUID) (db.AgentTaskQueue, pgtype.UUID, bool) {
	actorType, actorID := h.resolveActor(r, requestUserID(r), util.UUIDToString(wsUUID))
	if actorType != "agent" {
		writeError(w, http.StatusForbidden, "only the workspace's Secretary files a Secretary request")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	ws, err := h.Queries.GetWorkspace(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load workspace")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	secretaryID, err := util.ParseUUID(actorID)
	if err != nil || !ws.SecretaryAgentID.Valid || ws.SecretaryAgentID != secretaryID {
		writeError(w, http.StatusForbidden, "only the workspace's Secretary files a Secretary request")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	taskID, err := util.ParseUUID(r.Header.Get("X-Task-ID"))
	if err != nil {
		writeError(w, http.StatusForbidden, "a Secretary request comes from a live run")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	run, err := h.Queries.GetAgentTask(r.Context(), taskID)
	if err != nil || run.AgentID != secretaryID {
		writeError(w, http.StatusForbidden, "a Secretary request comes from a live run")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	if run.Status == "completed" || run.Status == "failed" || run.Status == "cancelled" {
		writeError(w, http.StatusForbidden, "a Secretary request comes from a live run")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	if !run.ChatSessionID.Valid {
		writeError(w, http.StatusForbidden, "a Secretary request is filed from the Secretary's chat turn")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	session, err := h.Queries.GetChatSessionInWorkspace(r.Context(), db.GetChatSessionInWorkspaceParams{ID: run.ChatSessionID, WorkspaceID: wsUUID})
	if err != nil || session.AgentID != secretaryID || session.CreatorID != run.OriginatorUserID || session.Status == "archived" {
		writeError(w, http.StatusForbidden, "the chat is not the boss's chat with the Secretary")
		return db.AgentTaskQueue{}, pgtype.UUID{}, false
	}
	return run, secretaryID, true
}

// callAsSecretary runs one of the handler's own endpoints for the Secretary's
// run: the same context and the same agent and task headers, with the given path
// and body. It returns the status and the response body as text.
func (h *Handler) callAsSecretary(r *http.Request, handler http.HandlerFunc, workspaceID, method, path string, body map[string]any, params map[string]string) (int, string) {
	raw, _ := json.Marshal(body)
	sep := "?"
	if strings.Contains(path, "?") {
		sep = "&"
	}
	req := httptest.NewRequest(method, path+sep+"workspace_id="+workspaceID, bytes.NewReader(raw)).WithContext(r.Context())
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Workspace-ID", workspaceID)
	req.Header.Set("X-Agent-ID", r.Header.Get("X-Agent-ID"))
	req.Header.Set("X-Task-ID", r.Header.Get("X-Task-ID"))
	// The auth middleware stamped these on the original request; the handlers read them.
	req.Header.Set("X-User-ID", r.Header.Get("X-User-ID"))
	if src := r.Header.Get("X-Actor-Source"); src != "" {
		req.Header.Set("X-Actor-Source", src)
	}
	if len(params) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range params {
			rctx.URLParams.Add(k, v)
		}
		req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	}
	rec := httptest.NewRecorder()
	handler(rec, req)
	return rec.Code, rec.Body.String()
}

// cancelIssueAsSecretary cancels an issue this call filed when a later step fails,
// through the same update path the Secretary's own runs use.
func (h *Handler) cancelIssueAsSecretary(r *http.Request, workspaceID pgtype.UUID, issueID string) {
	ws := util.UUIDToString(workspaceID)
	h.callAsSecretary(r, h.UpdateIssue, ws, http.MethodPut, "/api/issues/"+issueID, map[string]any{"status": "cancelled"}, map[string]string{"id": issueID})
}

func truncateRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
