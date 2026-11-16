// Package accountprovision turns pending platform_account_request rows into a
// user and a workspace. The platform writes the request; this job owns its
// state, user_id, workspace_id, error_code and attempts.
package accountprovision

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auditstore"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/planlimit"
	"github.com/multica-ai/multica/server/internal/scheduler"
	"github.com/multica-ai/multica/server/internal/workspaceprovision"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// JobName is the scheduler job name, stable across releases.
const JobName = "account_provisioning"

const (
	batchSize        = 20
	maxAttempts      = 3
	maxSlugCandidate = 5

	errEmailExists       = "email_exists"
	errEmailDomainBlock  = "email_domain_blocked"
	errSlugUnavailable   = "slug_unavailable"
	errInternal          = "internal"
	actionAccountCreated = "account.provisioned"
)

// TxStarter opens transactions; *pgxpool.Pool satisfies it.
type TxStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Knowledge enables the company knowledge base for a new workspace.
type Knowledge interface {
	AutoEnable(workspaceID, actorID pgtype.UUID)
}

// Deps are the collaborators of a Runner.
type Deps struct {
	Pool                TxStarter
	Queries             *db.Queries
	BlockedEmailDomains []string
	Analytics           analytics.Client
	Metrics             *obsmetrics.BusinessMetrics
	// Knowledge may be nil.
	Knowledge Knowledge
	// Suffix returns the slug suffix for a collision retry. Default: 4 random
	// characters from [a-z0-9].
	Suffix func() (string, error)
	// Inject is a test hook called inside the transaction at stage "create"
	// just before the workspace is created. A non-nil error is transient.
	Inject func(stage string) error
}

// Runner provisions pending account requests.
type Runner struct{ d Deps }

// New returns a Runner.
func New(d Deps) *Runner {
	if d.Suffix == nil {
		d.Suffix = randomSuffix
	}
	return &Runner{d: d}
}

// Job returns the scheduler job that runs the Runner every minute.
func Job(r *Runner) scheduler.JobSpec {
	return scheduler.JobSpec{
		Name:              JobName,
		Cadence:           time.Minute,
		CatchUpMode:       scheduler.CatchUpLatestOnly,
		CatchUpWindow:     time.Minute,
		RunTimeout:        5 * time.Minute,
		StaleTimeout:      10 * time.Minute,
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

// permanentError is a failure no retry can fix; Code goes to error_code.
type permanentError struct{ Code string }

func (e *permanentError) Error() string { return "account request failed: " + e.Code }

// RunOnce processes up to 20 pending requests and returns how many were
// provisioned. A request that fails does not stop the others.
func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	ids, err := r.d.Queries.ListPendingAccountRequestIDs(ctx, batchSize)
	if err != nil {
		return 0, err
	}
	provisioned := 0
	for _, id := range ids {
		ok, err := r.process(ctx, id)
		if err != nil {
			slog.Error("account provisioning: request failed", "request_id", uuidString(id), "error", err)
		}
		if ok {
			provisioned++
		}
	}
	return provisioned, nil
}

// process runs one request in its own transaction. It returns true when the
// request was provisioned.
func (r *Runner) process(ctx context.Context, id pgtype.UUID) (bool, error) {
	tx, err := r.d.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	qtx := r.d.Queries.WithTx(tx)

	req, err := qtx.LockPendingAccountRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // another run has it, or it is no longer pending
	}
	if err != nil {
		return false, err
	}

	user, ws, err := r.provision(ctx, qtx, req)
	if err == nil {
		err = qtx.MarkAccountRequestProvisioned(ctx, db.MarkAccountRequestProvisionedParams{
			ID: id, UserID: user.ID, WorkspaceID: ws.ID,
		})
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return false, errors.Join(err, r.recordFailure(ctx, id, err))
	}

	obsmetrics.RecordEvent(r.d.Analytics, r.d.Metrics, analytics.WorkspaceCreated(uuidString(user.ID), uuidString(ws.ID)))
	if r.d.Knowledge != nil {
		go r.d.Knowledge.AutoEnable(ws.ID, user.ID)
	}
	return true, nil
}

// recordFailure stores the outcome of a rolled-back attempt in its own
// transaction: permanent errors fail the request, transient ones count an
// attempt and fail it on the third.
func (r *Runner) recordFailure(ctx context.Context, id pgtype.UUID, cause error) error {
	tx, err := r.d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := r.d.Queries.WithTx(tx)

	var perm *permanentError
	if errors.As(cause, &perm) {
		if err := q.MarkAccountRequestFailed(ctx, db.MarkAccountRequestFailedParams{ID: id, ErrorCode: perm.Code}); err != nil {
			return err
		}
		return tx.Commit(ctx)
	}
	attempts, err := q.IncrementAccountRequestAttempts(ctx, id)
	if err != nil {
		return err
	}
	if attempts >= maxAttempts {
		if err := q.MarkAccountRequestFailed(ctx, db.MarkAccountRequestFailedParams{ID: id, ErrorCode: errInternal}); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

func (r *Runner) provision(ctx context.Context, qtx *db.Queries, req db.PlatformAccountRequest) (db.User, db.Workspace, error) {
	email := strings.ToLower(strings.TrimSpace(req.Email))
	if _, err := qtx.GetUserByEmail(ctx, email); err == nil {
		return db.User{}, db.Workspace{}, &permanentError{Code: errEmailExists}
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.User{}, db.Workspace{}, err
	}
	if planlimit.EmailDomainBlocked(email, r.d.BlockedEmailDomains) {
		return db.User{}, db.Workspace{}, &permanentError{Code: errEmailDomainBlock}
	}

	slug, err := r.pickSlug(ctx, qtx, req)
	if err != nil {
		return db.User{}, db.Workspace{}, err
	}

	user, err := qtx.CreateUser(ctx, db.CreateUserParams{Name: req.DisplayName, Email: email})
	if err != nil {
		return db.User{}, db.Workspace{}, err
	}
	if user, err = qtx.MarkUserOnboarded(ctx, user.ID); err != nil {
		return db.User{}, db.Workspace{}, err
	}

	if r.d.Inject != nil {
		if err := r.d.Inject("create"); err != nil {
			return db.User{}, db.Workspace{}, err
		}
	}
	store := auditstore.New(qtx)
	ws, err := workspaceprovision.Create(ctx, qtx, workspaceprovision.Params{
		Name:        req.WorkspaceName,
		Slug:        slug,
		IssuePrefix: workspaceprovision.DefaultIssuePrefix(slug),
		OwnerID:     user.ID,
		Record: func(e audit.Entry) error {
			e.ActorType = audit.ActorSystem
			return audit.Record(ctx, store, e)
		},
	})
	if err != nil {
		return db.User{}, db.Workspace{}, err
	}

	wsID := uuidString(ws.ID)
	if err := audit.Record(ctx, store, audit.Entry{
		WorkspaceID: wsID,
		ActorType:   audit.ActorSystem,
		Action:      actionAccountCreated,
		TargetType:  "workspace",
		TargetID:    wsID,
		TargetLabel: ws.Name,
		// "plan", not "plan_key": audit.Redact drops any key containing "key".
		Detail: map[string]any{
			"staff": uuidString(req.CreatedBy),
			"kind":  req.Kind,
			"plan":  req.PlanKey,
		},
	}); err != nil {
		return db.User{}, db.Workspace{}, err
	}
	return user, ws, nil
}

// pickSlug returns the workspace slug or a permanent slug_unavailable error.
func (r *Runner) pickSlug(ctx context.Context, qtx *db.Queries, req db.PlatformAccountRequest) (string, error) {
	unavailable := &permanentError{Code: errSlugUnavailable}
	usable := func(slug string) (bool, error) {
		if !workspaceprovision.SlugPattern.MatchString(slug) || workspaceprovision.IsReservedSlug(slug) {
			return false, nil
		}
		_, err := qtx.GetWorkspaceBySlug(ctx, slug)
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}

	if given := strings.ToLower(strings.TrimSpace(req.WorkspaceSlug)); given != "" {
		ok, err := usable(given)
		if err != nil {
			return "", err
		}
		if !ok {
			return "", unavailable
		}
		return given, nil
	}

	base := workspaceprovision.DeriveSlug(req.WorkspaceName)
	for i := 0; i < maxSlugCandidate; i++ {
		candidate := base
		if i > 0 {
			suffix, err := r.d.Suffix()
			if err != nil {
				return "", err
			}
			candidate = strings.Trim(base+"-"+suffix, "-")
		}
		ok, err := usable(candidate)
		if err != nil {
			return "", err
		}
		if ok {
			return candidate, nil
		}
	}
	return "", unavailable
}

const suffixAlphabet = "abcdefghijklmnopqrstuvwxyz0123456789"

func randomSuffix() (string, error) {
	out := make([]byte, 4)
	for i := range out {
		n, err := rand.Int(rand.Reader, big.NewInt(int64(len(suffixAlphabet))))
		if err != nil {
			return "", fmt.Errorf("slug suffix: %w", err)
		}
		out[i] = suffixAlphabet[n.Int64()]
	}
	return string(out), nil
}

func uuidString(id pgtype.UUID) string { return uuid.UUID(id.Bytes).String() }
