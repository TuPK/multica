package handler

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
)

// durableBriefBody places the period's outcome facts under a company brief's Progress
// section while durable_outcomes is on (durable spec 2.7). Floor briefs and briefs with the
// flag off keep the body the publisher wrote. A failed read is logged and the body is kept,
// so the brief still publishes.
func (h *Handler) durableBriefBody(ctx context.Context, ws pgtype.UUID, scopeType, body string, start, end time.Time) string {
	if scopeType != briefScopeCompany || !featureflags.DurableOutcomesEnabled(ctx, h.FeatureFlags) {
		return body
	}
	facts, err := service.BriefFactsFor(ctx, h.Queries, ws, start, end)
	if err != nil {
		slog.Warn("brief outcome facts failed", "error", err)
		return body
	}
	return service.BriefWithFacts(body, service.BriefFactsMarkdown(facts))
}
