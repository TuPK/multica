package handler

import (
	"context"
	"fmt"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// flowTaskCreateFork holds fork-specific fields of a flow task being opened.
type flowTaskCreateFork struct {
	Effects string
}

func (h *Handler) shouldHoldExternalFlowTask(ctx context.Context, c flowTaskCreate) bool {
	return c.Effects == issueworkflow.EffectsExternal &&
		h.durable.Intents != nil &&
		featureflags.DurableEffectsEnabled(ctx, h.FeatureFlags)
}

func (h *Handler) prepareFlowTaskCreateOpts(ctx context.Context, c flowTaskCreate, opts *service.IssueCreateOpts) {
	if h.shouldHoldExternalFlowTask(ctx, c) {
		opts.SkipAutoEnqueue = true
	}
}

func (h *Handler) maybeHoldExternalFlowTask(ctx context.Context, run *flowRun, c flowTaskCreate, issue db.Issue) error {
	if !h.shouldHoldExternalFlowTask(ctx, c) {
		return nil
	}
	if _, err := h.durable.Intents.HoldFlowTask(ctx, run.issue.WorkspaceID, issue.ID, c.Step, c.Key, c.Round); err != nil {
		return fmt.Errorf("hold external flow task %s: %w", c.Key, err)
	}
	return nil
}
