package handler

import (
	"context"
	"errors"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/service"
)

// flowTaskMayOpen applies the external effects hold to a task the engine is about to open
// (durable spec 5.4): an external task opens only once a person has approved its intent for
// this round. A rejected or expired intent stalls the flow instead of holding forever.
// Any other task may open. Both the graph and the schema 1 opening paths call it.
func (h *Handler) flowTaskMayOpen(ctx context.Context, run *flowRun, key, effects string) (bool, *advanceOutcome, error) {
	if effects != issueworkflow.EffectsExternal {
		return true, nil, nil
	}
	if h.durable.Intents == nil {
		return false, nil, errors.New("external flow task: the intent gate is not configured")
	}
	ok, err := h.durable.Intents.HoldFlowTask(ctx, run.issue.WorkspaceID, run.issue.ID, run.state.Step, key, run.round())
	if errors.Is(err, service.ErrFlowTaskIntentTerminal) {
		out, stallErr := h.stallFlow(ctx, run, "external_intent_closed", err.Error(), "")
		if stallErr != nil {
			return false, nil, stallErr
		}
		out.action.Tasks = []string{key}
		return false, &out, nil
	}
	return ok, nil, err
}
