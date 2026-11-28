package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
)

// durableBriefBody places the period's outcome facts (durable spec 2.7, while durable_outcomes
// is on) and the flow-stage merge or drop candidates (durable spec 10.4, while durable_seats is on)
// under a company brief's Progress section. Floor briefs keep the body the publisher wrote. A
// failed read is logged and that section is skipped, so the brief still publishes.
func (h *Handler) durableBriefBody(ctx context.Context, ws pgtype.UUID, scopeType, body string, start, end time.Time) string {
	if scopeType != briefScopeCompany {
		return body
	}
	var sections string
	if featureflags.DurableOutcomesEnabled(ctx, h.FeatureFlags) {
		facts, err := service.BriefFactsFor(ctx, h.Queries, ws, start, end)
		if err != nil {
			slog.Warn("brief outcome facts failed", "error", err)
		} else {
			sections += service.BriefFactsMarkdown(facts)
		}
	}
	stages, err := service.FlowStageBriefFor(ctx, h.Queries, h.FeatureFlags, ws)
	if err != nil {
		slog.Warn("brief flow stage candidates failed", "error", err)
	} else {
		sections += stages
	}
	return service.BriefWithFacts(body, sections)
}
