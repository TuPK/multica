package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
)

// HoldFlowTask holds an external task of a flow step until a person approves it
// (durable spec 5.4). It records a declared intent for the task, with the step and
// the task named in its preview, before the task runs. The platform cannot run a
// declared intent: approval returns the decision to the agent, so the approval
// starts the task. The task's effect is not known to the platform, so the intent
// is a compensable write.
func (s *IntentService) HoldFlowTask(ctx context.Context, in connectedapps.EffectRequest, step, task string) (pgtype.UUID, error) {
	in.Source = connectedapps.SourceDeclared
	in.Class = connectedapps.EffectWrite
	in.Reversibility = connectedapps.ReversibilityCompensable
	in.Target = "flow/" + step + "." + task
	in.Summary = "flow step " + step + ", task " + task
	return s.Rehearse(ctx, pgtype.UUID{}, in, connectedapps.CanonicalCall{}, "")
}
