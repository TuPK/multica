package handler

import (
	"context"
	"errors"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// flowTaskMayOpen applies the external effects hold to a task the engine is about to open
// (durable spec 5.4): an external task opens only once a person has approved its intent for
// this round. Any other task may open. Both the graph and the schema 1 opening paths call it.
func (h *Handler) flowTaskMayOpen(ctx context.Context, run *flowRun, key, effects string) (bool, error) {
	if effects != issueworkflow.EffectsExternal {
		return true, nil
	}
	if h.Intents == nil {
		return false, errors.New("external flow task: the intent gate is not configured")
	}
	return h.Intents.HoldFlowTask(ctx, run.issue.WorkspaceID, run.issue.ID, run.state.Step, key, run.round())
}
