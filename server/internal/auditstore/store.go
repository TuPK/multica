// Package auditstore adapts the sqlc queries to audit.Writer.
package auditstore

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Store writes audit events through a set of queries, which may be bound to a
// transaction so the event commits or rolls back with the change it records.
type Store struct{ q *db.Queries }

var _ audit.Writer = (*Store)(nil)

// New returns a Store over q.
func New(q *db.Queries) *Store { return &Store{q: q} }

// NewTx returns a Store bound to tx.
func NewTx(q *db.Queries, tx pgx.Tx) *Store { return &Store{q: q.WithTx(tx)} }

// InsertAuditEvent implements audit.Writer.
func (s *Store) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	var ws pgtype.UUID
	if e.WorkspaceID != "" {
		u, err := util.ParseUUID(e.WorkspaceID)
		if err != nil {
			return err
		}
		ws = u
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return s.q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		WorkspaceID: ws,
		ActorType:   e.ActorType,
		ActorID:     pgtype.Text{String: e.ActorID, Valid: e.ActorID != ""},
		ActorLabel:  e.ActorLabel,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		TargetLabel: e.TargetLabel,
		Detail:      raw,
		Ip:          e.IP,
		UserAgent:   e.UserAgent,
		RequestID:   e.RequestID,
	})
}
