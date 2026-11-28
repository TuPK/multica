package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ErrFlowTaskIntentTerminal reports that an external flow task's approval intent
// reached a terminal closed state (rejected or expired) and the flow step must stop.
var ErrFlowTaskIntentTerminal = errors.New("flow task intent")

// HoldFlowTask decides whether an external task of a flow step may open (durable spec 5.4).
// The first time a round asks, it records a declared intent that names the step and the task,
// and the task waits. The task opens once a person has approved that intent. Approval itself
// runs nothing: the platform does not run declared work, and the request's next advance opens
// the task. A new round of the same task holds again. While durable_effects is off the mark
// does nothing and the task opens as an ordinary one.
func (s *IntentService) HoldFlowTask(ctx context.Context, ws, issueID pgtype.UUID, step, task string, round int) (bool, error) {
	if !featureflags.DurableEffectsEnabled(ctx, s.Flags) {
		return true, nil
	}
	target := flowTaskTarget(step, task, round)
	latest, err := s.Queries.GetLatestEffectIntentForTarget(ctx, db.GetLatestEffectIntentForTargetParams{WorkspaceID: ws, IssueID: issueID, Target: target})
	if errors.Is(err, pgx.ErrNoRows) {
		_, err := s.Rehearse(ctx, pgtype.UUID{}, connectedapps.EffectRequest{
			WS:            ws,
			IssueID:       issueID,
			Source:        connectedapps.SourceDeclared,
			Class:         connectedapps.EffectWrite,
			Reversibility: connectedapps.ReversibilityCompensable,
			Target:        target,
			Summary:       fmt.Sprintf("flow step %s, task %s", step, task),
		}, connectedapps.CanonicalCall{}, "")
		return false, err
	}
	if err != nil {
		return false, fmt.Errorf("load flow task intent: %w", err)
	}
	if latest.State == "rejected" || latest.State == "expired" {
		return false, fmt.Errorf("%w %s", ErrFlowTaskIntentTerminal, latest.State)
	}
	return latest.State == "approved", nil
}

// flowTaskTarget names the intent of one round of a flow task: one intent per round, so a
// rework round that opens the task again asks again.
func flowTaskTarget(step, task string, round int) string {
	return fmt.Sprintf("flow/%s.%s#%d", step, task, round)
}
