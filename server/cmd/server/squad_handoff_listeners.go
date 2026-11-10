package main

import (
	"context"
	"log/slog"
	"time"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

const squadHandoffCloseTimeout = 15 * time.Second

// registerSquadHandoffListeners leaves a note on a request when the request its squad
// handed off to another squad finishes, so one request shows the whole path.
func registerSquadHandoffListeners(bus *events.Bus, h *handler.Handler) {
	bus.Subscribe(protocol.EventIssueUpdated, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		if changed, _ := payload["status_changed"].(bool); !changed {
			return
		}
		issue, ok := payload["issue"].(handler.IssueResponse)
		if !ok {
			return
		}
		// Built-in keys decide here; a custom key may be done, so it goes through and CloseLoop reads its category.
		if issuestatus.IsBuiltIn(issue.Status) && issue.Status != issuestatus.Done && issue.Status != issuestatus.Cancelled {
			return
		}
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), squadHandoffCloseTimeout)
			defer cancel()
			row, err := h.Queries.GetIssue(ctx, parseUUID(issue.ID))
			if err != nil {
				return
			}
			if err := h.SquadHandoffService().CloseLoop(ctx, row); err != nil {
				slog.Warn("squad handoff: closing note failed", "error", err, "issue_id", issue.ID)
			}
		}()
	})
}
