package handler

import (
	"context"
	"log/slog"
	"net/http"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
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

func logAttentionFailure(subject string, id pgtype.UUID, err error) {
	slog.Warn("attention ledger: question not admitted", "subject_type", subject, "subject_id", util.UUIDToString(id), "error", err)
}
