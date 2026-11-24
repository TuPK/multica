package main

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/inbox"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// The fork's notification listeners and recipient filters. notification_listeners.go
// is upstream's and reaches this file through one call each.

// registerForkNotificationListeners subscribes the fork's own events.
func registerForkNotificationListeners(ctx context.Context, bus *events.Bus, queries *db.Queries) {
	// brief:created — one brief_ready item per workspace owner
	bus.Subscribe(protocol.EventBriefCreated, func(e events.Event) {
		notifyBriefReady(ctx, queries, bus, e)
	})
}

// dropUnseeableRecipients removes the members who cannot see the issue. A
// mention reaches only the members who see the issue, @all and @squad included.
func dropUnseeableRecipients(queries *db.Queries, workspaceID, issueID string, recipientIDs map[string]bool) {
	audience := newIssueAudience(context.Background(), queries, workspaceID, issueID)
	for id := range recipientIDs {
		if !audience.sees(id) {
			delete(recipientIDs, id)
		}
	}
}

// issueAudience is inbox.IssueAudience for the string ids the listeners carry
// (spec 7.1 rule 4).
type issueAudience struct{ a *inbox.IssueAudience }

func newIssueAudience(ctx context.Context, queries *db.Queries, workspaceID, issueID string) issueAudience {
	ws, wsErr := util.ParseUUID(workspaceID)
	issue, issueErr := util.ParseUUID(issueID)
	if wsErr != nil || issueErr != nil {
		return issueAudience{inbox.NewIssueAudience(ctx, queries, pgtype.UUID{}, pgtype.UUID{})}
	}
	return issueAudience{inbox.NewIssueAudience(ctx, queries, ws, issue)}
}

// sees reports whether the member sees the issue.
func (a issueAudience) sees(userID string) bool {
	id, err := util.ParseUUID(userID)
	return err == nil && a.a.Sees(id)
}
