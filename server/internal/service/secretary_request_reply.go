package service

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// isSecretaryRequest reports whether an issue is a request the workspace's
// Secretary filed in the Secretary desk: assigned to the Secretary, in that
// project. Only such a request's runs may answer in a chat.
func isSecretaryRequest(ctx context.Context, q *db.Queries, ws db.Workspace, issue db.Issue) (bool, error) {
	if !ws.SecretaryAgentID.Valid || issue.AssigneeType.String != "agent" || !issue.ProjectID.Valid ||
		util.UUIDToString(issue.AssigneeID) != util.UUIDToString(ws.SecretaryAgentID) {
		return false, nil
	}
	project, err := q.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{ID: issue.ProjectID, WorkspaceID: issue.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return project.Title == SecretaryDeskProjectTitle, nil
}

// requestReplySession returns the chat session a request's runs may reply into:
// the session the request names in its chat_session_id metadata, when the request
// is a Secretary request and that session is the Secretary's chat with the run's
// originator. Anything else gives an invalid id, so the run replies nowhere.
func requestReplySession(ctx context.Context, q *db.Queries, ws db.Workspace, issue db.Issue, originator pgtype.UUID) (pgtype.UUID, error) {
	none := pgtype.UUID{}
	if !originator.Valid {
		return none, nil
	}
	ok, err := isSecretaryRequest(ctx, q, ws, issue)
	if err != nil || !ok {
		return none, err
	}
	var meta map[string]any
	if err := json.Unmarshal(issue.Metadata, &meta); err != nil {
		return none, nil
	}
	raw, _ := meta["chat_session_id"].(string)
	sessionID, err := util.ParseUUID(raw)
	if err != nil {
		return none, nil
	}
	session, err := q.GetChatSessionInWorkspace(ctx, db.GetChatSessionInWorkspaceParams{ID: sessionID, WorkspaceID: issue.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return none, nil
	}
	if err != nil {
		return none, err
	}
	if session.AgentID != ws.SecretaryAgentID || session.CreatorID != originator || session.Status == "archived" {
		return none, nil
	}
	return session.ID, nil
}

// linkRequestReplySession binds a run that a request's wakeup started to the chat
// the request names, so the Secretary can reply into that chat from the run. The
// run's chat_session_id stays empty: a chat-bound run would write its output into
// the chat as the turn's outcome.
func linkRequestReplySession(ctx context.Context, q *db.Queries, issue db.Issue, task db.AgentTaskQueue, originator pgtype.UUID) error {
	ws, err := q.GetWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return err
	}
	sessionID, err := requestReplySession(ctx, q, ws, issue, originator)
	if err != nil || !sessionID.Valid {
		return err
	}
	return q.SetAgentTaskReplyChatSession(ctx, db.SetAgentTaskReplyChatSessionParams{ID: task.ID, ReplyChatSessionID: sessionID})
}

// ErrSecretaryRequestAnswerRequired refuses closing a child of a Secretary request
// before an agent has answered on it.
var ErrSecretaryRequestAnswerRequired = errors.New("this issue is part of a Secretary request: answer it in a comment before you close it, so the Secretary can read the answer")

// CheckSecretaryRequestAnswered refuses closing a child of a Secretary request until
// an agent has commented on it. That comment is the answer the Secretary reads; a
// close without it leaves the Secretary with nothing to assess.
func CheckSecretaryRequestAnswered(ctx context.Context, q *db.Queries, issue db.Issue) error {
	if !issue.ParentIssueID.Valid {
		return nil
	}
	parent, err := q.GetIssue(ctx, issue.ParentIssueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	ws, err := q.GetWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return err
	}
	ok, err := isSecretaryRequest(ctx, q, ws, parent)
	if err != nil || !ok {
		return err
	}
	answers, err := q.CountAgentAnswersOnIssue(ctx, issue.ID)
	if err != nil {
		return err
	}
	if answers == 0 {
		return ErrSecretaryRequestAnswerRequired
	}
	return nil
}

// IssueInSecretaryTree reports whether an issue is part of the Secretary's own
// work: a request in the Secretary desk, or a child of one. A Secretary run that
// reads a request may read this much of the board and no more.
func IssueInSecretaryTree(ctx context.Context, q *db.Queries, issue db.Issue) (bool, error) {
	if issue.ProjectID.Valid {
		project, err := q.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{ID: issue.ProjectID, WorkspaceID: issue.WorkspaceID})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return false, err
		}
		if err == nil && project.Title == SecretaryDeskProjectTitle {
			return true, nil
		}
	}
	if !issue.ParentIssueID.Valid {
		return false, nil
	}
	parent, err := q.GetIssue(ctx, issue.ParentIssueID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	ws, err := q.GetWorkspace(ctx, issue.WorkspaceID)
	if err != nil {
		return false, err
	}
	return isSecretaryRequest(ctx, q, ws, parent)
}

// SecretaryDeskID returns the Secretary desk project's id, or an invalid id when
// the workspace has none.
func SecretaryDeskID(ctx context.Context, q *db.Queries, workspaceID pgtype.UUID) (pgtype.UUID, error) {
	return findSecretaryDesk(ctx, q, workspaceID)
}
