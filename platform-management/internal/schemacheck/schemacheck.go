// Package schemacheck stops the platform service at start when the database
// lacks a migration the service needs. The service runs no migrations itself;
// the Multica server's migrate command owns the schema.
package schemacheck

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5"
)

// Required lists the migration versions, as recorded in schema_migrations,
// that create or change a table the platform reads or writes. A step that adds
// a migration for such a table adds its version here in the same commit; the
// scan test in this package fails when one is missing. Keep the list sorted.
var Required = []string{
	"651_platform_account",                  // platform_account: table
	"652_platform_account_user_idx",         // platform_account: unique index on user_id
	"653_platform_account_status_idx",       // platform_account: partial index on status
	"654_audit_event",                       // audit_event: table
	"655_audit_event_id_idx",                // audit_event: unique index on id
	"656_audit_event_workspace_created_idx", // audit_event: index on workspace_id, created_at
	"657_audit_event_actor_created_idx",     // audit_event: index on actor_id, created_at
	"658_user_consent",                      // user_consent: table
	"659_user_consent_uidx",                 // user_consent: unique index on user_id, document, version
	"660_audit_event_created_idx",           // audit_event: index on created_at
	"663_platform_staff",                    // platform_staff: table
	"664_platform_staff_user_idx",           // platform_staff: partial unique index on user_id
	"665_platform_workspace",                // platform_workspace: table
	"666_platform_workspace_uidx",           // platform_workspace: unique index on workspace_id
	"667_platform_workspace_status_idx",     // platform_workspace: partial index on status
	"668_platform_note",                     // platform_note: table
	"669_platform_note_workspace_idx",       // platform_note: index on workspace_id, created_at
	"670_plan",                              // plan: table
	"671_plan_key_uidx",                     // plan: unique index on key
	"672_workspace_subscription",            // workspace_subscription: table
	"673_workspace_subscription_uidx",       // workspace_subscription: unique index on workspace_id
	"674_platform_seat_operation",           // platform_seat_operation: table
	"675_platform_seat_operation_uidx",      // platform_seat_operation: unique index on workspace_id, operation_token
	"676_platform_seat_operation_state_idx", // platform_seat_operation: index on workspace_id, state
	"677_billing_order",                     // billing_order: table
	"678_billing_order_id_uidx",             // billing_order: unique index on id
	"679_billing_order_idem_uidx",           // billing_order: partial unique index on workspace_id, idempotency_key
	"680_billing_order_transfer_uidx",       // billing_order: unique index on transfer_code
	"681_billing_order_status_idx",          // billing_order: index on status, created_at
	"682_billing_order_workspace_idx",       // billing_order: index on workspace_id, created_at
	"683_billing_notice",                    // billing_notice: table
	"684_billing_notice_key_uidx",           // billing_notice: unique index on workspace_id, notice_key
	"685_workspace_export",                  // workspace_export: table
	"686_workspace_subscription_status_idx", // workspace_subscription: index on status, current_period_end
	"687_workspace_export_running_uidx",     // workspace_export: partial unique index on workspace_id
	"688_platform_note_id_uidx",             // platform_note: unique index on id
	"689_platform_usage_daily",              // platform_usage_daily: table
	"690_platform_usage_daily_uidx",         // platform_usage_daily: unique index on workspace_id, day
	"953_plan_subscription_r5",              // plan, workspace_subscription: R5 columns
}

// Queryer is the part of a pgx pool, connection or transaction that Check uses.
type Queryer interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// MissingError lists the required versions that schema_migrations lacks.
type MissingError struct {
	Versions []string
}

func (e *MissingError) Error() string {
	return fmt.Sprintf("database schema is missing required migrations: %s (run the Multica server's migrate command)", strings.Join(e.Versions, ", "))
}

// Check returns a *MissingError when any of required is not recorded in
// schema_migrations.
func Check(ctx context.Context, q Queryer, required []string) error {
	rows, err := q.Query(ctx, `SELECT version FROM schema_migrations WHERE version = ANY($1)`, required)
	if err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	defer rows.Close()
	var applied []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return fmt.Errorf("read schema_migrations: %w", err)
		}
		applied = append(applied, v)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("read schema_migrations: %w", err)
	}
	if m := missing(required, applied); len(m) > 0 {
		return &MissingError{Versions: m}
	}
	return nil
}

func missing(required, applied []string) []string {
	have := make(map[string]bool, len(applied))
	for _, v := range applied {
		have[v] = true
	}
	var out []string
	for _, v := range required {
		if !have[v] {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
