// Package connectaudit writes the integration.connected audit row of a channel
// installation (Slack, Lark, Telegram, WeCom, DingTalk). The five install paths
// call it with the queries of their install transaction, so the row commits or
// rolls back with the installation.
package connectaudit

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Connected names an installation that was just written.
type Connected struct {
	// Provider is the integration: slack, lark, telegram, wecom or dingtalk.
	Provider       string
	WorkspaceID    pgtype.UUID
	InstallationID pgtype.UUID
	// InstallerUserID is the member who connected it. When it is not set the
	// row is attributed to the system.
	InstallerUserID pgtype.UUID
}

// Entry builds the audit entry for c. actorLabel is the installer's email. The
// detail names the provider and nothing from the installation's config: no
// token, no secret and no app id.
func Entry(c Connected, actorLabel string) audit.Entry {
	e := audit.Entry{
		WorkspaceID: util.UUIDToString(c.WorkspaceID),
		ActorType:   audit.ActorSystem,
		ActorLabel:  "system: " + c.Provider + " install",
		Action:      audit.ActionIntegrationConnected,
		TargetType:  "integration",
		TargetID:    util.UUIDToString(c.InstallationID),
		Detail:      map[string]any{"provider": c.Provider},
	}
	if c.InstallerUserID.Valid {
		e.ActorType = audit.ActorMember
		e.ActorID = util.UUIDToString(c.InstallerUserID)
		e.ActorLabel = actorLabel
	}
	return e
}

// Record writes integration.connected through q, which must be bound to the
// install transaction. An error means the row was not written and the caller
// must roll the installation back.
func Record(ctx context.Context, q *db.Queries, c Connected) error {
	var label string
	if c.InstallerUserID.Valid {
		user, err := q.GetUser(ctx, c.InstallerUserID)
		if err != nil {
			return fmt.Errorf("audit %s connect: read installer: %w", c.Provider, err)
		}
		label = user.Email
	}
	if err := audit.Record(ctx, auditstore.New(q), Entry(c, label)); err != nil {
		return fmt.Errorf("audit %s connect: %w", c.Provider, err)
	}
	return nil
}
