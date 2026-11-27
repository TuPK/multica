package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// attentionService is the attention ledger over the handler's queries and flags.
func (h *Handler) attentionService() *service.AttentionService {
	return &service.AttentionService{Queries: h.Queries, Flags: h.FeatureFlags}
}

// allowAgentAsk writes the 422 an agent's question gets without its reason, why_not_decide or
// searched sources (durable spec 7.4), and reports whether the request may go on. With
// durable_principal off it lets every request through, as it did before.
func (h *Handler) allowAgentAsk(w http.ResponseWriter, r *http.Request, reason, whyNotDecide string, searched []string) bool {
	if !featureflags.DurablePrincipalEnabled(r.Context(), h.FeatureFlags) {
		return true
	}
	err := service.CheckAgentAsk(reason, whyNotDecide, searched)
	if err == nil {
		return true
	}
	writeErrorCode(w, http.StatusUnprocessableEntity, service.AttentionErrorCode(err), err.Error())
	return false
}

// GetAttentionBudget returns the workspace's attention budget and this week's spend. Any
// workspace member may read it.
//
//	GET /api/company/attention
func (h *Handler) GetAttentionBudget(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	out, err := h.attentionService().Budget(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read the attention budget")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// PrincipalQueuedAsk is one question queued for a person to decide (durable spec 7.5).
type PrincipalQueuedAsk struct {
	SubjectType      string  `json:"subject_type"`
	SubjectID        string  `json:"subject_id"`
	Reason           string  `json:"reason"`
	EstimatedMinutes float64 `json:"estimated_minutes"`
	CreatedAt        string  `json:"created_at"`
}

// ListQueuedAttention lists the questions queued for the caller, oldest first (GET
// /api/company/attention/queued). An owner or admin also sees the questions addressed to no one.
// Only a person reads it, and only while durable_principal is on.
func (h *Handler) ListQueuedAttention(w http.ResponseWriter, r *http.Request) {
	c, ok := h.principalCallerFor(w, r, true)
	if !ok {
		return
	}
	if !requirePrincipalPerson(w, c) {
		return
	}
	rows, err := h.Queries.ListQueuedAttentionForMember(r.Context(), db.ListQueuedAttentionForMemberParams{
		WorkspaceID: c.ws, MemberID: c.actor.ID, IncludeUnassigned: c.owner, RowLimit: service.AttentionQueueMax,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list the queued questions")
		return
	}
	asks := make([]PrincipalQueuedAsk, 0, len(rows))
	for _, row := range rows {
		asks = append(asks, PrincipalQueuedAsk{
			SubjectType: row.SubjectType, SubjectID: util.UUIDToString(row.SubjectID), Reason: row.Reason,
			EstimatedMinutes: row.EstimatedMinutes, CreatedAt: timestampToString(row.CreatedAt),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"asks": asks})
}

// SetAttentionBudget saves the weekly budget and its split. Owners and admins only.
//
//	PUT /api/company/attention {minutes_per_week, split}
func (h *Handler) SetAttentionBudget(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.companyActorFor(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req struct {
		MinutesPerWeek int32  `json:"minutes_per_week"`
		Split          string `json:"split"`
	}
	if !decodeInitiativeBody(w, r, &req) {
		return
	}
	out, err := h.attentionService().SetBudget(r.Context(), wsUUID, req.MinutesPerWeek, req.Split)
	switch {
	case errors.Is(err, service.ErrPrincipalDisabled):
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	case errors.Is(err, service.ErrAttentionBudgetRange), errors.Is(err, service.ErrAttentionSplit):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to save the attention budget")
		return
	}
	writeJSON(w, http.StatusOK, out)
}

// admitProposal puts a filed proposal in the attention ledger: the filing is the question.
// A standing approval that decided it at once is delegated and spends no minutes. A failure
// is logged; the filing already stands.
func (h *Handler) admitProposal(ctx context.Context, ws, squad pgtype.UUID, actorType string, req CreateProposalRequest, res service.ProposalCreateResult) {
	kind := "proposal"
	if req.Kind == proposalkinds.KeyOrgChange {
		kind = "org_change"
	}
	_, err := h.attentionService().Admit(ctx, service.AskInput{
		WorkspaceID:  ws,
		SquadID:      squad,
		SubjectType:  "proposal",
		SubjectID:    res.Proposal.ID,
		EstimateKind: kind,
		Words:        len(strings.Fields(res.Issue.Description.String)),
		Reason:       req.Reason,
		WhyNotDecide: req.WhyNotDecide,
		Searched:     req.Searched,
		ByAgent:      actorType == "agent",
		Urgent:       req.Urgent,
		Delegated:    res.AutoDecision != nil,
	})
	if err != nil {
		logAttentionFailure("proposal", res.Proposal.ID, err)
	}
}

// admitInquiry puts the Secretary's question to a squad leader in the attention ledger.
func (h *Handler) admitInquiry(ctx context.Context, ws, squad pgtype.UUID, req AskBriefInquiryRequest, inquiryID pgtype.UUID) {
	_, err := h.attentionService().Admit(ctx, service.AskInput{
		WorkspaceID:  ws,
		SquadID:      squad,
		SubjectType:  "inquiry",
		SubjectID:    inquiryID,
		Reason:       req.Reason,
		WhyNotDecide: req.WhyNotDecide,
		Searched:     req.Searched,
		ByAgent:      true,
	})
	if err != nil {
		logAttentionFailure("inquiry", inquiryID, err)
	}
}

// answerAttention records that a person decided a question. A failure is logged; the decision
// stands. It records no measured time: the decision handlers do not know when the person opened
// the question.
func (h *Handler) answerAttention(ctx context.Context, subject string, id pgtype.UUID) {
	if err := h.attentionService().Answered(ctx, subject, id, nil); err != nil {
		slog.Warn("attention ledger: answer not recorded", "subject_type", subject, "subject_id", util.UUIDToString(id), "error", err)
	}
}

func logAttentionFailure(subject string, id pgtype.UUID, err error) {
	slog.Warn("attention ledger: question not admitted", "subject_type", subject, "subject_id", util.UUIDToString(id), "error", err)
}
