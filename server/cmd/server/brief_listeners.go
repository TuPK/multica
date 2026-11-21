package main

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/inbox"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// notifyBriefReady raises a brief_ready inbox item (severity attention) for
// each workspace owner. Admins can publish and decide, but the brief is the
// boss's, so only owners are announced to. The publisher is not skipped: an
// owner who publishes by hand still gets the item.
func notifyBriefReady(ctx context.Context, queries *db.Queries, bus *events.Bus, e events.Event) {
	payload, ok := e.Payload.(map[string]any)
	if !ok {
		return
	}
	brief, ok := payload["brief"].(handler.BriefResponse)
	if !ok {
		return
	}
	owners, err := queries.ListWorkspaceOwnerUserIDs(ctx, parseUUID(e.WorkspaceID))
	if err != nil {
		slog.Error("brief:created notification: failed to list owners", "workspace_id", e.WorkspaceID, "error", err)
		return
	}
	ownerIDs := make([]string, len(owners))
	for i, id := range owners {
		ownerIDs[i] = util.UUIDToString(id)
	}
	details, _ := json.Marshal(map[string]string{"brief_id": brief.ID, "kind": brief.Kind})

	for _, ownerID := range ownerIDs {
		// Deliver drops the item when the owner muted agent activity.
		if _, _, err := inbox.Deliver(ctx, queries, bus, inbox.Notification{Params: db.CreateInboxItemParams{
			ID:            dbid.NewV7(),
			WorkspaceID:   parseUUID(e.WorkspaceID),
			RecipientType: "member",
			RecipientID:   parseUUID(ownerID),
			Type:          "brief_ready",
			Severity:      "attention",
			IssueID:       pgtype.UUID{},
			Title:         brief.Title,
			Body:          util.StrToText(service.BriefHeadline(brief.Body)),
			ActorType:     util.StrToText(e.ActorType),
			ActorID:       optionalUUID(e.ActorID),
			Details:       details,
		}}); err != nil {
			slog.Error("brief:created notification: inbox item failed", "brief_id", brief.ID, "recipient_id", ownerID, "error", err)
		}
	}
}
