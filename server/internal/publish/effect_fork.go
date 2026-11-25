package publish

import (
	"context"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// gateAttempt logs a post before it is sent (durable spec 4.5, source
// publish_channel). A post is a send to people outside the workspace, so it is
// irreversible.
func (s *Service) gateAttempt(ctx context.Context, attempt db.PublishAttempt) (connectedapps.EffectRecord, error) {
	if s.Effects == nil {
		return connectedapps.EffectRecord{Decision: connectedapps.PolicyAllow}, nil
	}
	return s.Effects.Gate(ctx, connectedapps.EffectRequest{
		WS:            attempt.WorkspaceID,
		Source:        connectedapps.SourcePublishChannel,
		Target:        "publish_channel",
		Class:         connectedapps.EffectSend,
		Reversibility: connectedapps.ReversibilityIrreversible,
		Summary:       "publish a post to a channel",
		Prior:         connectedapps.PolicyAllow,
	})
}

// settleAttempt records whether the post went out. It uses a context that shutdown
// does not cancel, so the outcome is written even while the service stops.
func (s *Service) settleAttempt(ctx context.Context, attempt db.PublishAttempt, gate connectedapps.EffectRecord, sendErr error) {
	if s.Effects == nil || !gate.EventID.Valid {
		return
	}
	outcome := "executed"
	if sendErr != nil {
		outcome = "failed"
	}
	settleCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := s.Effects.Finish(settleCtx, attempt.WorkspaceID, gate.EventID, outcome, nil); err != nil {
		slog.Warn("publish: settle effect", "error", err, "outcome", outcome)
	}
}
