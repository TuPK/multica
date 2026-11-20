package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/legalhold"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// checkWorkspaceHeld refuses a deletion of the workspace while a live hold covers
// it or its tenant (spec R6 section 3c.7, rule 3). It is the one check of the
// workspace teardown, so every path runs it. The refusal is audited on the pool,
// not on the caller's transaction, so the row survives the rollback.
func (h *Handler) checkWorkspaceHeld(ctx context.Context, q *db.Queries, wsID pgtype.UUID) error {
	tenantID, err := q.GetTenantIDByWorkspace(ctx, wsID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read tenant: %w", err)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		tenantID = pgtype.UUID{}
	}
	holdID, err := q.ActiveLegalHoldForWorkspace(ctx, db.ActiveLegalHoldForWorkspaceParams{
		WorkspaceID: wsID,
		TenantID:    tenantID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read legal hold: %w", err)
	}
	h.recordHeldRefusal(ctx, audit.Entry{
		WorkspaceID: uuidToString(wsID),
		TenantID:    uuidToString(tenantID),
		ActorType:   audit.ActorSystem,
		Action:      audit.ActionDeletionBlockedByHold,
		TargetType:  "workspace",
		TargetID:    uuidToString(wsID),
		Detail:      map[string]any{"legal_hold_id": uuidToString(holdID), "scope": "workspace"},
	})
	return &legalhold.Blocked{HoldID: uuidToString(holdID)}
}

// checkAccountHeld refuses an account deletion while the account is held, or
// while a workspace the account is the only member of is held: deleting the
// account would delete that workspace (spec R6 section 3c.7, rule 5). It runs
// before the deletion writes anything.
func (h *Handler) checkAccountHeld(ctx context.Context, userID pgtype.UUID) error {
	holdID, err := h.Queries.ActiveLegalHoldForAccount(ctx, userID)
	if err == nil {
		h.recordHeldRefusal(ctx, audit.Entry{
			ActorType:  audit.ActorSystem,
			ActorID:    uuidToString(userID),
			Action:     audit.ActionDeletionBlockedByHold,
			TargetType: "account",
			TargetID:   uuidToString(userID),
			Detail:     map[string]any{"legal_hold_id": uuidToString(holdID), "scope": "account"},
		})
		return &legalhold.Blocked{HoldID: uuidToString(holdID)}
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("read account hold: %w", err)
	}
	rows, err := h.Queries.ListUserWorkspaceCounts(ctx, userID)
	if err != nil {
		return fmt.Errorf("list workspaces: %w", err)
	}
	for _, row := range rows {
		if row.MemberCount > 1 {
			continue
		}
		if err := h.checkWorkspaceHeld(ctx, h.Queries, row.WorkspaceID); err != nil {
			return err
		}
	}
	return nil
}

// recordHeldRefusal writes the deletion.blocked_by_hold row. A failure is logged,
// not returned: the refusal stands either way.
func (h *Handler) recordHeldRefusal(ctx context.Context, e audit.Entry) {
	if err := audit.Record(ctx, auditstore.New(h.Queries), e); err != nil {
		slog.Warn("record blocked deletion failed", "target_type", e.TargetType, "target_id", e.TargetID, "error", err)
	}
}
