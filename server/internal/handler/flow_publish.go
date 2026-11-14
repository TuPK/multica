package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"sort"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// Publishing a draft (spec 8.1): one transaction that checks everything and writes
// a locked version of the lineage.

type publishFlowDraftRequest struct {
	Revision *int32   `json:"revision"`
	Ack      []string `json:"ack"`
	// MakeActive is the client's convenience: it sets the version active with the
	// route that already does that. The server does not.
	MakeActive bool `json:"make_active"`
}

// ackKinds maps the names a client sends to the kinds the validator knows, and
// says who may give each: admins give the two that let work leave unseen.
var ackKinds = map[string]struct {
	kind  string
	admin bool
}{
	"allow_unattended_release":         {issueworkflow.AckUnattendedRelease, true},
	issueworkflow.AckUnattendedRelease: {issueworkflow.AckUnattendedRelease, true},
	"lead_may_work":                    {issueworkflow.AckLeadMayWork, true},
	issueworkflow.AckNoGate:            {issueworkflow.AckNoGate, false},
	issueworkflow.AckProposalReviewed:  {issueworkflow.AckProposalReviewed, false},
}

func flowRejectJSON(w http.ResponseWriter, status int, code, message string, extra map[string]any) {
	body := map[string]any{"error": message, "code": code}
	for k, v := range extra {
		body[k] = v
	}
	writeJSON(w, status, body)
}

// PublishFlowDraft publishes a draft as the next version of its lineage.
func (h *Handler) PublishFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_publish_forbidden")
	if !ok {
		return
	}
	ctx := r.Context()
	if !featureflags.FlowGraphV2Enabled(ctx, h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_graph_v2_disabled", "writing a flow document of schema 2 is not enabled")
		return
	}
	var req publishFlowDraftRequest
	if !decodeFlowBody(w, r, &req) {
		return
	}
	if req.Revision == nil {
		writeErrorCode(w, http.StatusBadRequest, "flow_draft_revision_required", "revision is required")
		return
	}
	if a.squad.ArchivedAt.Valid {
		writeError(w, http.StatusConflict, "squad is archived")
		return
	}
	draftID, ok := a.draftID(w, r)
	if !ok {
		return
	}
	admin := isWorkspaceAdmin(a.member)
	var acks []string
	for _, name := range req.Ack {
		spec, known := ackKinds[name]
		if !known {
			writeErrorCode(w, http.StatusBadRequest, "flow_ack_unknown", fmt.Sprintf("%q is not an acknowledgement", name))
			return
		}
		if spec.admin && !admin {
			flowRejectJSON(w, http.StatusForbidden, "flow_ack_forbidden", "only a workspace admin can acknowledge this", map[string]any{"required": spec.kind})
			return
		}
		acks = append(acks, spec.kind)
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := qtx.LockIssueStatusCatalog(ctx, a.wsUUID); err != nil {
		writeFlowError(w, r, err)
		return
	}
	if _, err := qtx.LockSquadForWorkflowChange(ctx, db.LockSquadForWorkflowChangeParams{ID: a.squad.ID, WorkspaceID: a.wsUUID}); err != nil {
		writeFlowError(w, r, err)
		return
	}
	draft, err := qtx.GetFlowDraft(ctx, db.GetFlowDraftParams{ID: draftID, WorkspaceID: a.wsUUID, SquadID: a.squad.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "flow_draft_not_found", "draft not found")
		return
	}
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	if draft.Revision != *req.Revision {
		flowRejectJSON(w, http.StatusConflict, "flow_draft_conflict", "the draft was saved by someone else: read it again", map[string]any{"revision": draft.Revision})
		return
	}
	version := int32(1)
	lineage := draft.LineageID
	if lineage.Valid {
		latest, err := qtx.LatestFlowLineageVersion(ctx, db.LatestFlowLineageVersionParams{WorkspaceID: a.wsUUID, LineageID: lineage})
		if err == nil {
			if latest.ID != draft.BaseWorkflowID {
				flowRejectJSON(w, http.StatusConflict, "flow_draft_stale", "a newer version was published: make a draft from it",
					map[string]any{"latest_workflow_id": uuidToString(latest.ID)})
				return
			}
			version = latest.Version.Int32 + 1
		} else if !errors.Is(err, pgx.ErrNoRows) {
			writeFlowError(w, r, err)
			return
		}
		count, err := qtx.CountFlowLineageVersions(ctx, db.CountFlowLineageVersionsParams{WorkspaceID: a.wsUUID, LineageID: lineage})
		if err != nil {
			writeFlowError(w, r, err)
			return
		}
		if count >= issueworkflow.MaxVersionsPerLineage {
			flowRejectJSON(w, http.StatusConflict, "flow_limit_reached", "the workflow has the most versions it may have",
				map[string]any{"limit": "versions", "max": issueworkflow.MaxVersionsPerLineage})
			return
		}
	} else {
		lineage = pgtype.UUID{Bytes: dbid.NewV7().Bytes, Valid: true}
	}

	doc, parseIssues := issueworkflow.ParseFlowDoc(draft.Doc)
	if len(parseIssues) > 0 {
		flowRejectJSON(w, http.StatusUnprocessableEntity, "flow_draft_invalid", "the draft has errors", map[string]any{"issues": parseIssues})
		return
	}
	// The acknowledgements are the publisher's, not the document's: whatever the draft says is dropped.
	doc.Policy.Acks = nil
	now := time.Now().UTC().Format(time.RFC3339)
	for _, kind := range acks {
		doc.Policy.Acks = append(doc.Policy.Acks, issueworkflow.Ack{Kind: kind, By: uuidToString(a.member.UserID), At: now})
		switch kind {
		case issueworkflow.AckUnattendedRelease:
			doc.Policy.AllowUnattendedRelease = true
		case issueworkflow.AckLeadMayWork:
			doc.Policy.LeadMayWork = true
		}
	}

	// A status named by a step is found or, for an admin, made. Doing it first makes a missing
	// right the answer, and the check below sees the statuses the document will have.
	createdStatuses, status, code, msg := h.resolveDocStatuses(r, qtx, a, &doc, admin)
	if status != 0 {
		writeErrorCode(w, status, code, msg)
		return
	}

	vctx, err := h.flowValidateContext(ctx, qtx, a, true)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	issues := issueworkflow.ValidateFlowDoc(doc, vctx)
	required := map[string]bool{}
	blocking := false
	for _, issue := range issues {
		switch {
		case issue.Code == "ack_required":
			if kind, ok := issue.Params["ack"].(string); ok {
				required[kind] = true
			}
		case issue.Severity == issueworkflow.SeverityError:
			blocking = true
		case issue.Ack != "":
			required[issue.Ack] = true
		}
	}
	// A workflow an agent proposed is published only by a manager who says they have read it.
	if draft.Origin == "proposal" && !slices.Contains(acks, issueworkflow.AckProposalReviewed) {
		required[issueworkflow.AckProposalReviewed] = true
	}
	if issues == nil {
		issues = []issueworkflow.Issue{}
	}
	if blocking {
		flowRejectJSON(w, http.StatusUnprocessableEntity, "flow_draft_invalid", "the draft has errors", map[string]any{"issues": issues})
		return
	}
	if len(required) > 0 {
		kinds := make([]string, 0, len(required))
		for k := range required {
			kinds = append(kinds, k)
		}
		sort.Strings(kinds)
		flowRejectJSON(w, http.StatusUnprocessableEntity, "flow_ack_required", "the draft needs an acknowledgement to be published", map[string]any{"required": kinds, "issues": issues})
		return
	}

	// Each agent that fills a role is one the publisher may wire to a squad.
	for _, role := range doc.Roles {
		if role.Bind == nil {
			continue
		}
		for _, agentRef := range role.Bind.AgentIDs {
			agentID, err := parseFlowUUID(agentRef)
			if err != nil {
				continue
			}
			agent, err := qtx.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: a.wsUUID})
			if err != nil || !h.memberCanWireAgent(ctx, a.member, agent, a.workspaceID) {
				flowRejectJSON(w, http.StatusForbidden, "flow_agent_forbidden", "you cannot wire this agent to a squad", map[string]any{"agent_id": agentRef})
				return
			}
		}
	}

	name := fmt.Sprintf("%s · v%d", draft.Name, version)
	if runes := []rune(name); len(runes) > issueworkflow.MaxNameRunes {
		suffix := fmt.Sprintf(" · v%d", version)
		keep := issueworkflow.MaxNameRunes - utf8.RuneCountInString(suffix)
		name = string([]rune(draft.Name)[:keep]) + suffix
	}
	flowConfig, err := json.Marshal(doc)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	steps, err := docStatusSteps(doc)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	row, err := qtx.CreateFlowWorkflowVersion(ctx, db.CreateFlowWorkflowVersionParams{
		WorkspaceID: a.wsUUID, Name: name, Description: "", InitialStatusKey: issueworkflow.StartStatus, Steps: steps,
		OwnerID: a.member.UserID, SquadID: a.squad.ID, FlowConfig: flowConfig,
		LineageID: lineage, Version: version, PublishedBy: a.member.UserID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeErrorCode(w, http.StatusConflict, "workflow_name_taken", "a workflow with this name already exists")
			return
		}
		slog.Warn("publish flow draft failed", append(logger.RequestAttrs(r), "error", err)...)
		writeFlowError(w, r, err)
		return
	}
	if _, err := qtx.SetFlowDraftBase(ctx, db.SetFlowDraftBaseParams{
		ID: draft.ID, WorkspaceID: a.wsUUID, SquadID: a.squad.ID, LineageID: lineage, BaseWorkflowID: row.ID, ActorID: a.member.UserID,
	}); err != nil {
		writeFlowError(w, r, err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeFlowError(w, r, err)
		return
	}
	if len(createdStatuses) > 0 {
		h.publishIssueStatusChanged(a.workspaceID, a.member, "created")
	}
	h.publishIssueWorkflowChanged(a.workspaceID, a.member, "published")
	h.publishFlowDraftUpdated(a, draft.ID)
	h.writeIssueWorkflow(w, r, http.StatusCreated, row)
}

// resolveDocStatuses gives every step that names a new status the key of the status
// it found or made. It reports a status the publisher may not create.
func (h *Handler) resolveDocStatuses(r *http.Request, qtx *db.Queries, a flowAuthoring, doc *issueworkflow.FlowDoc, admin bool) (created []db.IssueStatus, status int, code, msg string) {
	for i, step := range doc.Steps {
		if step.Status.Key != "" || step.Status.Name == "" {
			continue
		}
		entry, isNew, message, err := createWorkflowStatus(r.Context(), qtx, a.wsUUID, NewWorkflowStatus{Name: step.Status.Name, Category: step.Status.Category}, admin)
		switch {
		case errors.Is(err, errStatusCreateForbidden):
			return nil, http.StatusForbidden, "status_create_forbidden", err.Error()
		case message != "":
			return nil, http.StatusUnprocessableEntity, "flow_draft_invalid", message
		case err != nil:
			return nil, http.StatusInternalServerError, "internal", "failed to resolve the statuses"
		}
		if isNew {
			created = append(created, entry)
		}
		doc.Steps[i].Status = issueworkflow.StepStatus{Key: entry.Key}
	}
	return created, 0, "", ""
}

// docStatusSteps is the list of statuses a row of a schema 2 flow keeps in steps
// (spec 2.1): every status the document uses, in the order it first uses it, with
// the three that every workflow has.
func docStatusSteps(doc issueworkflow.FlowDoc) ([]byte, error) {
	seen := map[string]bool{}
	var keys []string
	add := func(key string) {
		if key != "" && !seen[key] {
			seen[key] = true
			keys = append(keys, key)
		}
	}
	add(issueworkflow.StartStatus)
	for _, s := range doc.Steps {
		add(s.Status.Key)
	}
	for _, t := range doc.Terminals {
		add(t.Status)
	}
	for _, s := range doc.Stops {
		add(s.Status)
	}
	add("blocked")
	add("done")
	add("cancelled")
	return issueworkflow.EncodeKeys(keys)
}
