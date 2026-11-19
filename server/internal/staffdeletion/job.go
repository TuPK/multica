// Package staffdeletion carries out the immediate deletions that platform
// management queues (spec R6 sections 6.3 and 7.4, phase 2): one workspace, a
// tenant with every workspace in it, or an account. The platform writes the request in phase 1; this
// job owns its state, attempts, error_code, updated_at and completed_at.
package staffdeletion

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/scheduler"
	"github.com/multica-ai/multica/server/internal/tenant"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// JobName is the scheduler job name, stable across releases.
const JobName = "staff_deletion"

const (
	batchSize   = 20
	maxAttempts = 3
	// errDeleteFailed is the error_code of a request whose teardown failed. The
	// workspace data is intact: the teardown is one transaction.
	errDeleteFailed = "delete_failed"

	kindWorkspace = "workspace"
	kindTenant    = "tenant"
	kindAccount   = "account"
)

// TxStarter opens transactions; *pgxpool.Pool satisfies it.
type TxStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Eraser runs the teardowns a staff request needs. The handler implements it,
// through the same teardowns the user's own deletions run.
type Eraser interface {
	DeleteWorkspaceForStaff(ctx context.Context, workspaceID string, origin handler.DeletionOrigin, inject func(stage string) error) (bool, error)
	DeleteAccountForOrigin(ctx context.Context, userID string, now time.Time, origin handler.DeletionOrigin) (bool, error)
}

// Deps are the collaborators of a Runner.
type Deps struct {
	Pool    TxStarter
	Queries *db.Queries
	Eraser  Eraser
	// Inject is a test hook passed to every teardown. A non-nil error from
	// stage "audit" fails the attempt, as a transient failure would.
	Inject func(stage string) error
}

// Runner executes pending platform deletion requests.
type Runner struct{ d Deps }

// New returns a Runner.
func New(d Deps) *Runner { return &Runner{d: d} }

// Job returns the scheduler job that runs the Runner every minute. A tenant
// deletion can take a while, so a run may last up to RunTimeout; a run that
// stops leaves its request running, and it is claimed again after the stale
// window of the claim query.
func Job(r *Runner) scheduler.JobSpec {
	return scheduler.JobSpec{
		Name:              JobName,
		Cadence:           time.Minute,
		CatchUpMode:       scheduler.CatchUpLatestOnly,
		CatchUpWindow:     time.Minute,
		RunTimeout:        25 * time.Minute,
		StaleTimeout:      30 * time.Minute,
		HeartbeatInterval: 30 * time.Second,
		AllowStaleReentry: true,
		MaxAttempts:       3,
		RetryBackoff:      []time.Duration{time.Minute, 5 * time.Minute},
		Scopes:            scheduler.StaticScopes(scheduler.ScopeGlobal),
		Handler: func(ctx context.Context, _ scheduler.HandlerInput) (scheduler.HandlerResult, error) {
			n, err := r.RunOnce(ctx)
			return scheduler.HandlerResult{RowsAffected: int64(n)}, err
		},
	}
}

// RunOnce processes up to batchSize requests and returns how many completed. A
// request that fails does not stop the others.
func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	ids, err := r.d.Queries.ListPendingStaffDeletionIDs(ctx, batchSize)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, id := range ids {
		ok, err := r.process(ctx, id)
		if err != nil {
			slog.Error("staff deletion: request failed", "request_id", util.UUIDToString(id), "error", err)
		}
		if ok {
			done++
		}
	}
	return done, nil
}

// process claims one request, runs it and records the outcome. It returns true
// when the request completed. A claim another run holds is not an error.
func (r *Runner) process(ctx context.Context, id pgtype.UUID) (bool, error) {
	req, err := r.d.Queries.ClaimStaffDeletionRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	err = r.run(ctx, req)
	var perm *permanentError
	switch {
	case errors.Is(err, errWaitingForChildren):
		return false, r.d.Queries.ReleaseStaffDeletionClaim(ctx, id)
	case errors.As(err, &perm):
		markErr := r.d.Queries.FailStaffDeletionNow(ctx, db.FailStaffDeletionNowParams{ID: id, ErrorCode: perm.Code})
		return false, errors.Join(err, markErr)
	case err != nil:
		_, markErr := r.d.Queries.FailStaffDeletionAttempt(ctx, db.FailStaffDeletionAttemptParams{
			ID:          id,
			ErrorCode:   errDeleteFailed,
			MaxAttempts: maxAttempts,
		})
		return false, errors.Join(err, markErr)
	}
	if err := r.d.Queries.CompleteStaffDeletionRequest(ctx, id); err != nil {
		return false, err
	}
	return true, nil
}

// run carries out a claimed request. Runtime requests are not handled here;
// the claim query leaves them alone.
func (r *Runner) run(ctx context.Context, req db.PlatformDeletionRequest) error {
	origin := handler.DeletionOrigin{
		Kind:       handler.DeletionOriginStaff,
		RequestID:  util.UUIDToString(req.ID),
		StaffID:    util.UUIDToString(req.RequestedBy),
		StaffLabel: req.RequestedByLabel,
	}
	switch req.Kind {
	case kindWorkspace:
		_, err := r.d.Eraser.DeleteWorkspaceForStaff(ctx, util.UUIDToString(req.TargetID), origin, r.d.Inject)
		return err
	case kindTenant:
		return r.runTenant(ctx, req, origin)
	case kindAccount:
		return r.runAccount(ctx, req, origin)
	}
	return fmt.Errorf("staff deletion: unsupported kind %q", req.Kind)
}

// runTenant deletes each workspace of the tenant through the workspace teardown,
// then the tenant. A workspace that fails stops the run; the next run lists only
// the workspaces still linked, so the ones already deleted stay deleted.
func (r *Runner) runTenant(ctx context.Context, req db.PlatformDeletionRequest, origin handler.DeletionOrigin) error {
	ids, err := r.d.Queries.ListTenantWorkspaceIDs(ctx, req.TargetID)
	if err != nil {
		return fmt.Errorf("list tenant workspaces: %w", err)
	}
	for _, id := range ids {
		if _, err := r.d.Eraser.DeleteWorkspaceForStaff(ctx, util.UUIDToString(id), origin, r.d.Inject); err != nil {
			return fmt.Errorf("workspace %s: %w", util.UUIDToString(id), err)
		}
	}
	return r.finishTenant(ctx, req, origin)
}

// finishTenant removes the tenant row and writes tenant.deleted in one
// transaction. The tenant lock makes it queue behind a workspace creation that
// is counting the tenant's workspaces.
func (r *Runner) finishTenant(ctx context.Context, req db.PlatformDeletionRequest, origin handler.DeletionOrigin) error {
	tx, err := r.d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := r.d.Queries.WithTx(tx)
	if err := tenant.Lock(ctx, qtx, req.TargetID); err != nil {
		return fmt.Errorf("lock tenant: %w", err)
	}
	left, err := qtx.CountTenantWorkspaceLinks(ctx, req.TargetID)
	if err != nil {
		return fmt.Errorf("count workspaces: %w", err)
	}
	if left > 0 {
		return fmt.Errorf("tenant still has %d workspaces", left)
	}
	if err := qtx.DeleteTenant(ctx, req.TargetID); err != nil {
		return fmt.Errorf("delete tenant: %w", err)
	}
	tenantID := util.UUIDToString(req.TargetID)
	if err := audit.Record(ctx, auditstore.NewTx(r.d.Queries, tx), audit.Entry{
		TenantID:    tenantID,
		ActorType:   audit.ActorStaff,
		ActorID:     origin.StaffID,
		ActorLabel:  origin.StaffLabel,
		Action:      audit.ActionTenantDeleted,
		TargetType:  "tenant",
		TargetID:    tenantID,
		TargetLabel: req.TargetLabel,
		Detail:      map[string]any{"reason": "staff_immediate_deletion", "origin": "staff", "request_id": origin.RequestID},
	}); err != nil {
		return fmt.Errorf("audit: %w", err)
	}
	return tx.Commit(ctx)
}
