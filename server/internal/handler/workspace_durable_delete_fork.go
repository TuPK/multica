package handler

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
)

// workspaceDurableTables are the durable tables whose rows belong to one workspace (each has a
// workspace_id). durable_outcome_applied is handled in deleteWorkspaceDurableData. No foreign key
// covers them (project rule), so the teardown deletes them by workspace_id. Each one is marked
// "delete" in workspaceDeletionManifest.
var workspaceDurableTables = []string{
	"acceptance_criterion",
	"attention_ledger",
	"effect_event",
	"effect_intent",
	"effect_policy_override",
	"issue_status_transition",
	"jury_panel",
	"necessity_test",
	"outcome_check_task",
	"outcome_verification",
	"reach_snapshot",
	"seat_offer",
	"shadow_config",
	"shadow_pair",
	"track_record",
	"work_outcome",
}

// deleteWorkspaceDurableData removes the durable rows of a workspace inside the teardown transaction.
func deleteWorkspaceDurableData(ctx context.Context, tx pgx.Tx, wsID pgtype.UUID) error {
	// durable_outcome_applied has no workspace_id: it follows the workspace's status transitions.
	if _, err := tx.Exec(ctx, `DELETE FROM durable_outcome_applied WHERE transition_id IN (SELECT id FROM issue_status_transition WHERE workspace_id = $1)`, wsID); err != nil {
		return err
	}
	for _, table := range workspaceDurableTables {
		if _, err := tx.Exec(ctx, "DELETE FROM "+table+" WHERE workspace_id = $1", wsID); err != nil {
			return err
		}
	}
	return nil
}
