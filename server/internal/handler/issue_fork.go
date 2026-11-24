package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/projectaccess"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// projectMoveKey marks a write context in which the issue changes project, so
// the write in the same transaction also drops the assignee and watchers who
// cannot see the destination (spec 7.1 rule 7).
type projectMoveKey struct{}

type projectMove struct{ workspaceID pgtype.UUID }

func withProjectMove(ctx context.Context, workspaceID pgtype.UUID) context.Context {
	return context.WithValue(ctx, projectMoveKey{}, projectMove{workspaceID: workspaceID})
}

// removeSubscribersWhoCannotSee unsubscribes the member watchers of issue who
// cannot see its project.
func removeSubscribersWhoCannotSee(ctx context.Context, q *db.Queries, workspaceID pgtype.UUID, issue db.Issue) error {
	subscribers, err := q.ListIssueSubscribers(ctx, issue.ID)
	if err != nil {
		return fmt.Errorf("list issue subscribers: %w", err)
	}
	for _, sub := range subscribers {
		if sub.UserType != "member" {
			continue
		}
		sees, err := memberSeesProject(ctx, q, workspaceID, sub.UserID, issue.ProjectID)
		if err != nil {
			return fmt.Errorf("check watcher project visibility: %w", err)
		}
		if sees {
			continue
		}
		if err := q.RemoveIssueSubscriber(ctx, db.RemoveIssueSubscriberParams{
			IssueID: issue.ID, UserType: sub.UserType, UserID: sub.UserID,
		}); err != nil {
			return fmt.Errorf("remove issue subscriber: %w", err)
		}
	}
	return nil
}

// authorizeDirectiveCreate verifies a directive create (origin_type =
// "directive") and writes the refusal itself when it fails. Only the workspace
// Secretary may create one, from a live run whose trigger is a chat message
// from an owner or admin (see service.AuthorizeDirective).
func (h *Handler) authorizeDirectiveCreate(w http.ResponseWriter, r *http.Request, workspaceID, creatorType, creatorID string, requestedOriginID *string, parentIssueID pgtype.UUID) (service.DirectiveGrant, bool) {
	if creatorType != "agent" || !h.isWorkspaceSecretary(r, workspaceID, creatorID) {
		writeError(w, http.StatusForbidden, "only the workspace's secretary agent may create a directive")
		return service.DirectiveGrant{}, false
	}
	taskHeader := r.Header.Get("X-Task-ID")
	taskID, err := util.ParseUUID(taskHeader)
	if taskHeader == "" || err != nil {
		writeError(w, http.StatusForbidden, "a directive must be created from a live run triggered by a boss chat message")
		return service.DirectiveGrant{}, false
	}
	wsUUID, err := util.ParseUUID(workspaceID)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid workspace id")
		return service.DirectiveGrant{}, false
	}
	agentUUID, err := util.ParseUUID(creatorID)
	if err != nil {
		writeError(w, http.StatusForbidden, "only the workspace's secretary agent may create a directive")
		return service.DirectiveGrant{}, false
	}
	in := service.DirectiveAuthorityInput{
		WorkspaceID:   wsUUID,
		AgentID:       agentUUID,
		TaskID:        taskID,
		ParentIssueID: parentIssueID,
	}
	if requestedOriginID != nil {
		oid, ok := parseUUIDOrBadRequest(w, *requestedOriginID, "origin_id")
		if !ok {
			return service.DirectiveGrant{}, false
		}
		in.RequestedOriginID = oid
	}

	grant, err := service.AuthorizeDirective(r.Context(), h.Queries, in)
	if err == nil {
		return grant, true
	}
	var guard *service.GuardrailError
	switch {
	case errors.As(err, &guard):
		writeError(w, http.StatusTooManyRequests, err.Error())
	case errors.Is(err, service.ErrDirectiveMessageMismatch), errors.Is(err, service.ErrDirectiveTargetNotFound):
		writeError(w, http.StatusBadRequest, err.Error())
	case errors.Is(err, service.ErrDirectiveProposal), errors.Is(err, service.ErrDirectiveStageGate):
		writeError(w, http.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, service.ErrDirectiveRunNotLive), errors.Is(err, service.ErrDirectiveNotChatTriggered),
		errors.Is(err, service.ErrDirectiveSessionNotLive), errors.Is(err, service.ErrDirectiveOriginatorNotAdmin),
		errors.Is(err, service.ErrDirectiveNotOriginatorMsg):
		writeError(w, http.StatusForbidden, err.Error())
	default:
		slog.Error("authorize directive", "error", err, "task_id", taskHeader)
		writeError(w, http.StatusInternalServerError, "failed to check the directive")
	}
	return service.DirectiveGrant{}, false
}

// squadLeaderAssigneeGate limits an agent that leads a squad to the members of
// the squads it leads when it assigns an issue to an agent. The squad leader's
// briefing lists the roster, but the handler is the trust boundary: without
// this check a leader can assign to any agent in the workspace. An agent that
// leads no squad, an assignment to itself, and a handoff the workflow engine
// makes pass unchanged.
func (h *Handler) squadLeaderAssigneeGate(ctx context.Context, r *http.Request, wsUUID, assigneeID pgtype.UUID) (int, string) {
	if isWorkflowHandoff(ctx) {
		return 0, ""
	}
	actorType, actorID := h.resolveActor(r, requestUserID(r), util.UUIDToString(wsUUID))
	if actorType != "agent" {
		return 0, ""
	}
	actorUUID, err := util.ParseUUID(actorID)
	if err != nil || actorUUID == assigneeID {
		return 0, ""
	}
	ledSquads, err := h.Queries.ListSquadsLedByAgent(ctx, db.ListSquadsLedByAgentParams{
		WorkspaceID: wsUUID,
		AgentID:     actorUUID,
	})
	if err != nil {
		return http.StatusInternalServerError, "failed to check squad membership"
	}
	if len(ledSquads) == 0 {
		return 0, ""
	}
	for _, squadID := range ledSquads {
		isMember, err := h.Queries.IsSquadMember(ctx, db.IsSquadMemberParams{
			SquadID:    squadID,
			MemberType: "agent",
			MemberID:   assigneeID,
		})
		if err != nil {
			return http.StatusInternalServerError, "failed to check squad membership"
		}
		if isMember {
			return 0, ""
		}
	}
	return http.StatusForbidden, "a squad leader can assign work only to agents in its squad"
}

// chooseCreateFlow resolves the workflow version a person picked for a new request, and
// checks that an agent may open a sub-issue of a request a workflow runs. It writes the
// refusal itself and reports ok=false when the create must stop. Nothing is written yet.
func (h *Handler) chooseCreateFlow(w http.ResponseWriter, r *http.Request, req CreateIssueRequest, creatorType, creatorID, workspaceID string, wsUUID, parentIssueID, assigneeID pgtype.UUID, assigneeType pgtype.Text) (chosenFlow *db.IssueWorkflow, flowTrial bool, ok bool) {
	// A person may choose which version of the squad's workflows the request runs on.
	// It is checked before anything is written, so a bad choice creates nothing.
	chosenID := ""
	if req.FlowWorkflowID != nil {
		chosenID = *req.FlowWorkflowID
	}
	if (chosenID != "" || req.FlowTrial) && creatorType != "agent" {
		if !featureflags.FlowAuthoringEnabled(r.Context(), h.FeatureFlags) {
			writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled")
			return nil, false, false
		}
		var id pgtype.UUID
		if chosenID != "" {
			var ok bool
			if id, ok = parseUUIDOrBadRequest(w, chosenID, "flow_workflow_id"); !ok {
				return nil, false, false
			}
		}
		if parentIssueID.Valid {
			writeErrorCode(w, http.StatusUnprocessableEntity, "flow_workflow_invalid", "a sub-issue does not run on a flow of its own")
			return nil, false, false
		}
		if req.FlowTrial {
			if !h.canRehearseSquad(r.Context(), creatorID, workspaceID, assigneeType, assigneeID) {
				writeErrorCode(w, http.StatusForbidden, "flow_forbidden", "only the members who manage the squad can rehearse a workflow")
				return nil, false, false
			}
			flowTrial = true
		}
		if !id.Valid && assigneeType.String == "squad" && assigneeID.Valid {
			// A rehearsal without a named version runs on the squad's active one.
			if squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: assigneeID, WorkspaceID: wsUUID}); err == nil {
				id = squad.ActiveWorkflowID
			}
		}
		row, err := service.ResolveFlowChoice(r.Context(), h.Queries, wsUUID, assigneeType, assigneeID, id, flowTrial)
		var choice *service.FlowChoiceError
		if errors.As(err, &choice) {
			writeErrorCode(w, http.StatusUnprocessableEntity, "flow_workflow_invalid", choice.Message)
			return nil, false, false
		}
		if err != nil {
			slog.Warn("create issue: resolve flow workflow", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to resolve the workflow")
			return nil, false, false
		}
		chosenFlow = &row
	}

	// A workflow opens the sub-issues of a request it runs; an agent does not.
	if parentIssueID.Valid && creatorType == "agent" {
		if err := h.checkFlowChildCreate(r.Context(), wsUUID, parentIssueID); err != nil {
			if writeFlowRejection(w, err) {
				return nil, false, false
			}
			writeError(w, http.StatusInternalServerError, "failed to check flow rules")
			return nil, false, false
		}
	}
	return chosenFlow, flowTrial, true
}

// checkBatchFlow runs the workflow rules over a whole batch before anything is written: a
// batch cannot close a Secretary request with an unanswered child, close a gate without its
// verdict, hand a task to someone else or leave half of itself done. It writes the refusal
// itself and reports ok=false when the batch must stop.
func (h *Handler) checkBatchFlow(w http.ResponseWriter, r *http.Request, req BatchUpdateIssuesRequest, rawUpdates map[string]json.RawMessage, hidden projectaccess.Hidden, wsUUID pgtype.UUID, workspaceID, userID, batchStatusKey string) (batchOutcome string, batchActor flowActor, ok bool) {
	// A batch that closes a child of a Secretary request is refused as a whole when
	// any child has no answer yet, the same rule as a single close.
	if batchStatusKey == "done" {
		for _, issueID := range req.IssueIDs {
			issueUUID, err := util.ParseUUID(issueID)
			if err != nil {
				continue
			}
			prevIssue, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: issueUUID, WorkspaceID: wsUUID})
			if err != nil || !hidden.CanSeeProject(prevIssue.ProjectID) || prevIssue.Status == "done" {
				continue
			}
			if err := service.CheckSecretaryRequestAnswered(r.Context(), h.Queries, prevIssue); err != nil {
				writeError(w, http.StatusUnprocessableEntity, err.Error())
				return "", flowActor{}, false
			}
		}
	}
	// The outcome of the update is the outcome of every item that changes status.
	if req.Updates.Outcome != nil {
		if !flowOutcomeKey.MatchString(*req.Updates.Outcome) {
			writeErrorCode(w, http.StatusBadRequest, "invalid_outcome", "outcome must be approved, rejected, blocked or done")
			return "", flowActor{}, false
		}
		batchOutcome = *req.Updates.Outcome
	}
	// The flow rules refuse the whole batch before anything is written, so a
	// batch cannot close a gate without its verdict, hand a task to someone else
	// or leave half of itself done.
	batchActor = h.flowActorFor(r, userID, workspaceID)
	_, batchTouchesAssignee := rawUpdates["assignee_type"]
	if _, ok := rawUpdates["assignee_id"]; ok {
		batchTouchesAssignee = true
	}
	if batchStatusKey != "" || batchTouchesAssignee {
		for _, issueID := range req.IssueIDs {
			issueUUID, err := util.ParseUUID(issueID)
			if err != nil {
				continue
			}
			prevIssue, err := h.Queries.GetIssueInWorkspace(r.Context(), db.GetIssueInWorkspaceParams{ID: issueUUID, WorkspaceID: wsUUID})
			if err != nil || !hidden.CanSeeProject(prevIssue.ProjectID) {
				continue
			}
			if batchOutcome != "" && !h.outcomeKeyAllowed(r.Context(), prevIssue, batchOutcome) {
				writeErrorCode(w, http.StatusBadRequest, "invalid_outcome", "outcome must be approved, rejected, blocked or done")
				return "", flowActor{}, false
			}
			check := db.UpdateIssueParams{AssigneeType: prevIssue.AssigneeType, AssigneeID: prevIssue.AssigneeID}
			if batchStatusKey != "" {
				check.Status = pgtype.Text{String: batchStatusKey, Valid: true}
			}
			if _, ok := rawUpdates["assignee_type"]; ok {
				check.AssigneeType = pgtype.Text{}
				if req.Updates.AssigneeType != nil {
					check.AssigneeType = pgtype.Text{String: *req.Updates.AssigneeType, Valid: true}
				}
			}
			if _, ok := rawUpdates["assignee_id"]; ok {
				check.AssigneeID = pgtype.UUID{}
				if req.Updates.AssigneeID != nil {
					if assigneeUUID, err := util.ParseUUID(*req.Updates.AssigneeID); err == nil {
						check.AssigneeID = assigneeUUID
					}
				}
			}
			if batchTouchesAssignee {
				check.AssigneeType, check.AssigneeID, err = h.leaderAssigneeAsSquad(r.Context(), wsUUID, check.AssigneeType, check.AssigneeID)
				if err != nil {
					writeError(w, http.StatusInternalServerError, "failed to check flow rules")
					return "", flowActor{}, false
				}
			}
			if err := h.checkFlowWrite(r.Context(), prevIssue, check, batchOutcome, batchActor); err != nil {
				if writeFlowRejection(w, err) {
					return "", flowActor{}, false
				}
				slog.Error("batch update issues: check flow rules", append(logger.RequestAttrs(r), "error", err)...)
				writeError(w, http.StatusInternalServerError, "failed to check flow rules")
				return "", flowActor{}, false
			}
		}
	}
	return batchOutcome, batchActor, true
}
