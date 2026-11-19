// Package memberprovision turns pending platform_member_request rows of kind
// add into a member of the workspace. The platform writes the request; this job
// owns its state, user_id, member_id, user_created, error_code and attempts.
package memberprovision

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/planlimit"
	"github.com/multica-ai/multica/server/internal/scheduler"
	"github.com/multica-ai/multica/server/internal/seatcapacity"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// JobName is the scheduler job name, stable across releases.
const JobName = "member_provisioning"

// TxStarter opens transactions; *pgxpool.Pool satisfies it.
type TxStarter interface {
	Begin(ctx context.Context) (pgx.Tx, error)
}

// Publisher is the event bus. *events.Bus satisfies it.
type Publisher interface {
	Publish(events.Event)
}

// Deps are the collaborators of a Runner.
type Deps struct {
	Pool                TxStarter
	Queries             *db.Queries
	BlockedEmailDomains []string
	// Seats is the seat protocol client. Nil means seats are not managed.
	Seats seatcapacity.Executor
	// Bus receives the member:added event. May be nil.
	Bus Publisher
	// MemberPayload builds the "member" value of the member:added event. The
	// default is a plain map with the fields of the handler's member response.
	MemberPayload func(db.Member, db.User) any
	// Inject is a test hook called inside the transaction at stage "audit",
	// just before the audit row is written. A non-nil error is transient.
	Inject func(stage string) error
}

// Runner provisions pending member requests.
type Runner struct{ d Deps }

// New returns a Runner.
func New(d Deps) *Runner { return &Runner{d: d} }

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

const (
	batchSize   = 20
	maxAttempts = 3

	errAlreadyMember       = "already_member"
	errAccountNotActive    = "account_not_active"
	errEmailDomainBlocked  = "email_domain_blocked"
	errCapacityFull        = "capacity_full"
	errCapacityOvercommit  = "capacity_overcommitted"
	errSeatUnavailable     = "seat_capacity_unavailable"
	errWorkspaceNotActive  = "workspace_not_active"
	errInternal            = "internal"
	confirmAttempts        = 3
	accountStatusActive    = "active"
	workspaceStatusActive  = "active"
	targetTypeMember       = "member"
	targetTypeMemberReq    = "member_request"
	uniqueViolationSQLCode = "23505"
)

// permanentError is a failure no retry can fix; Code goes to error_code.
type permanentError struct{ Code string }

func (e *permanentError) Error() string { return "member request failed: " + e.Code }

// seatTransportError is a failed call to the seat service. It is retried, and
// on the last attempt the request fails as seat_capacity_unavailable.
type seatTransportError struct{ err error }

func (e *seatTransportError) Error() string { return "seat service: " + e.err.Error() }
func (e *seatTransportError) Unwrap() error { return e.err }

// attemptState is what one attempt did outside the database transaction.
type attemptState struct {
	// seatHeld is true once the seat service accepted the claim, so a failure
	// must release it.
	seatHeld bool
	// seatManaged is false when the workspace has no seat limit.
	seatManaged bool
	// seatCalled is true when the seat service was asked at all.
	seatCalled bool

	user    db.User
	member  db.Member
	created bool
	wsName  string
}

// RunOnce processes up to 20 pending requests and returns how many were
// completed. A request that fails does not stop the others.
func (r *Runner) RunOnce(ctx context.Context) (int, error) {
	ids, err := r.d.Queries.ListPendingMemberRequestIDs(ctx, batchSize)
	if err != nil {
		return 0, err
	}
	done := 0
	for _, id := range ids {
		ok, err := r.process(ctx, id)
		if err != nil {
			slog.Error("member provisioning: request failed", "request_id", uuid.UUID(id.Bytes).String(), "error", err)
		}
		if ok {
			done++
		}
	}
	return done, nil
}

// process runs one request in its own transaction. It returns true when the
// request was completed.
func (r *Runner) process(ctx context.Context, id pgtype.UUID) (bool, error) {
	tx, err := r.d.Pool.Begin(ctx)
	if err != nil {
		return false, err
	}
	defer tx.Rollback(ctx)
	qtx := r.d.Queries.WithTx(tx)

	req, err := qtx.LockPendingMemberRequest(ctx, id)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil // another run has it, or it is no longer pending
	}
	if err != nil {
		return false, err
	}

	var st attemptState
	err = r.provision(ctx, qtx, req, &st)
	if err == nil {
		err = tx.Commit(ctx)
	}
	if err != nil {
		_ = tx.Rollback(ctx)
		return false, errors.Join(err, r.recordFailure(ctx, req, err, &st))
	}

	r.afterCommit(ctx, req, &st)
	return true, nil
}

func (r *Runner) provision(ctx context.Context, qtx *db.Queries, req db.PlatformMemberRequest, st *attemptState) error {
	if req.Role != "admin" && req.Role != "member" && req.Role != "viewer" {
		return &permanentError{Code: errInternal}
	}
	email := strings.ToLower(strings.TrimSpace(req.Email))

	ws, err := qtx.GetWorkspace(ctx, req.WorkspaceID)
	if errors.Is(err, pgx.ErrNoRows) {
		return &permanentError{Code: errWorkspaceNotActive}
	}
	if err != nil {
		return err
	}
	st.wsName = ws.Name
	switch state, err := qtx.WorkspaceState(ctx, ws.ID); {
	case errors.Is(err, pgx.ErrNoRows): // no row means active
	case err != nil:
		return err
	case state.Status != workspaceStatusActive:
		return &permanentError{Code: errWorkspaceNotActive}
	}

	if auth.IsTemporarilyDisabledUserEmail(email) {
		return &permanentError{Code: errAccountNotActive}
	}
	user, err := qtx.GetUserByEmail(ctx, email)
	switch {
	case err == nil:
		if err := r.checkExistingUser(ctx, qtx, user, ws.ID); err != nil {
			return err
		}
	case errors.Is(err, pgx.ErrNoRows):
		if planlimit.EmailDomainBlocked(email, r.d.BlockedEmailDomains) {
			return &permanentError{Code: errEmailDomainBlocked}
		}
	default:
		return err
	}

	if err := r.claimSeat(ctx, req, st); err != nil {
		return err
	}

	if !user.ID.Valid {
		// Spec R6 section 4.2, rule 6: the name is the part before @, as the
		// sign-in flow names a new user, and ALLOW_SIGNUP is not consulted.
		name := email
		if at := strings.IndexByte(email, '@'); at > 0 {
			name = email[:at]
		}
		if user, err = qtx.CreateUser(ctx, db.CreateUserParams{Name: name, Email: email}); err != nil {
			return err
		}
		st.created = true
	}
	// Marked onboarded like AcceptInvitation does, so the person lands in the
	// workspace instead of the onboarding questionnaire. Idempotent.
	if user, err = qtx.MarkUserOnboarded(ctx, user.ID); err != nil {
		return err
	}
	st.user = user

	member, err := qtx.CreateMember(ctx, db.CreateMemberParams{WorkspaceID: ws.ID, UserID: user.ID, Role: req.Role})
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == uniqueViolationSQLCode {
			return &permanentError{Code: errAlreadyMember}
		}
		return err
	}
	st.member = member

	if r.d.Inject != nil {
		if err := r.d.Inject("audit"); err != nil {
			return err
		}
	}
	tenantID := ""
	if t, err := qtx.GetTenantIDByWorkspace(ctx, ws.ID); err == nil {
		tenantID = uuid.UUID(t.Bytes).String()
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return err
	}
	if err := audit.Record(ctx, auditstore.New(qtx), audit.Entry{
		WorkspaceID: uuid.UUID(ws.ID.Bytes).String(),
		TenantID:    tenantID,
		ActorType:   audit.ActorStaff,
		ActorID:     uuid.UUID(req.RequestedBy.Bytes).String(),
		ActorLabel:  req.RequestedByLabel,
		Action:      audit.ActionMemberStaffAdded,
		TargetType:  targetTypeMember,
		TargetID:    uuid.UUID(member.ID.Bytes).String(),
		TargetLabel: email,
		Detail: map[string]any{
			"role":         req.Role,
			"request_id":   uuid.UUID(req.ID.Bytes).String(),
			"user_created": st.created,
		},
	}); err != nil {
		return err
	}
	return qtx.MarkMemberRequestCompleted(ctx, db.MarkMemberRequestCompletedParams{
		ID: req.ID, UserID: user.ID, MemberID: member.ID, UserCreated: st.created,
	})
}

// checkExistingUser refuses a user whose account is not active and a user who is
// a member already.
func (r *Runner) checkExistingUser(ctx context.Context, qtx *db.Queries, user db.User, workspaceID pgtype.UUID) error {
	switch acct, err := qtx.AccountState(ctx, user.ID); {
	case errors.Is(err, pgx.ErrNoRows): // no row means active
	case err != nil:
		return err
	case acct.Status != accountStatusActive:
		return &permanentError{Code: errAccountNotActive}
	}
	_, err := qtx.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: user.ID, WorkspaceID: workspaceID})
	switch {
	case err == nil:
		return &permanentError{Code: errAlreadyMember}
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	default:
		return err
	}
}

// claimSeat takes the seat for the request, with the request id as the token.
// The call is idempotent by token, so a retry takes the same seat.
func (r *Runner) claimSeat(ctx context.Context, req db.PlatformMemberRequest, st *attemptState) error {
	if r.d.Seats == nil {
		return nil
	}
	st.seatCalled = true
	d, err := r.d.Seats.ClaimShareJoin(ctx, uuid.UUID(req.WorkspaceID.Bytes), uuid.UUID(req.ID.Bytes))
	if err != nil {
		if seatcapacity.IsCapacityOvercommitted(err) {
			return &permanentError{Code: errCapacityOvercommit}
		}
		return &seatTransportError{err: err}
	}
	st.seatManaged = d.Managed
	if !d.Managed {
		return nil
	}
	st.seatHeld = true
	if !d.Allowed {
		return &permanentError{Code: errCapacityFull}
	}
	return nil
}

// recordFailure stores the outcome of a rolled-back attempt in its own
// transaction: permanent errors fail the request, transient ones count an
// attempt and fail it on the third. A failed request writes its audit row and
// frees the seat it may hold.
func (r *Runner) recordFailure(ctx context.Context, req db.PlatformMemberRequest, cause error, st *attemptState) error {
	code, final := failureCode(cause)
	tx, err := r.d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := r.d.Queries.WithTx(tx)

	if !final {
		attempts, err := q.IncrementMemberRequestAttempts(ctx, req.ID)
		if err != nil {
			return err
		}
		if attempts < maxAttempts {
			return tx.Commit(ctx)
		}
	}
	if err := q.MarkMemberRequestFailed(ctx, db.MarkMemberRequestFailedParams{ID: req.ID, ErrorCode: code}); err != nil {
		return err
	}
	tenantID := ""
	if t, err := q.GetTenantIDByWorkspace(ctx, req.WorkspaceID); err == nil {
		tenantID = uuid.UUID(t.Bytes).String()
	}
	if err := audit.Record(ctx, auditstore.New(q), audit.Entry{
		WorkspaceID: uuid.UUID(req.WorkspaceID.Bytes).String(),
		TenantID:    tenantID,
		ActorType:   audit.ActorSystem,
		Action:      audit.ActionMemberStaffAddFailed,
		TargetType:  targetTypeMemberReq,
		TargetID:    uuid.UUID(req.ID.Bytes).String(),
		TargetLabel: req.Email,
		Detail:      map[string]any{"error_code": code},
	}); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	var te *seatTransportError
	if st.seatHeld || errors.As(cause, &te) {
		r.releaseSeat(ctx, req)
	}
	return nil
}

// failureCode maps a cause to the error_code of a failed request. final is true
// for a permanent cause, which fails the request at once.
func failureCode(cause error) (code string, final bool) {
	var perm *permanentError
	if errors.As(cause, &perm) {
		return perm.Code, true
	}
	var te *seatTransportError
	if errors.As(cause, &te) {
		return errSeatUnavailable, false
	}
	return errInternal, false
}

func (r *Runner) releaseSeat(ctx context.Context, req db.PlatformMemberRequest) {
	if r.d.Seats == nil {
		return
	}
	if _, err := r.d.Seats.Release(ctx, uuid.UUID(req.WorkspaceID.Bytes), uuid.UUID(req.ID.Bytes)); err != nil {
		slog.Warn("member provisioning: release seat failed", "request_id", uuid.UUID(req.ID.Bytes).String(), "error", err)
	}
}

// afterCommit confirms the seat and tells the workspace. The member exists, so
// a seat that cannot be confirmed does not undo it.
func (r *Runner) afterCommit(ctx context.Context, req db.PlatformMemberRequest, st *attemptState) {
	if r.d.Seats != nil && st.seatManaged {
		var err error
		for i := 0; i < confirmAttempts; i++ {
			var d seatcapacity.Decision
			d, err = r.d.Seats.Confirm(ctx, uuid.UUID(req.WorkspaceID.Bytes), uuid.UUID(req.ID.Bytes), uuid.UUID(st.member.ID.Bytes))
			if err == nil && (!d.Managed || d.Allowed) {
				err = nil
				break
			}
			if err == nil {
				err = fmt.Errorf("seat service refused to confirm: %s", d.Reason)
			}
		}
		if err != nil {
			slog.Error("member provisioning: confirm seat failed", "request_id", uuid.UUID(req.ID.Bytes).String(), "error", err)
		}
	}
	if r.d.Bus == nil {
		return
	}
	var member any
	if r.d.MemberPayload != nil {
		member = r.d.MemberPayload(st.member, st.user)
	} else {
		member = defaultMemberPayload(st.member, st.user)
	}
	r.d.Bus.Publish(events.Event{
		Type:        protocol.EventMemberAdded,
		WorkspaceID: uuid.UUID(req.WorkspaceID.Bytes).String(),
		ActorType:   "system",
		Payload:     map[string]any{"member": member, "workspace_name": st.wsName},
	})
}

// defaultMemberPayload has the fields of the handler's member response.
func defaultMemberPayload(m db.Member, u db.User) map[string]any {
	var avatar any
	if u.AvatarUrl.Valid {
		avatar = u.AvatarUrl.String
	}
	return map[string]any{
		"id":           uuid.UUID(m.ID.Bytes).String(),
		"workspace_id": uuid.UUID(m.WorkspaceID.Bytes).String(),
		"user_id":      uuid.UUID(m.UserID.Bytes).String(),
		"role":         m.Role,
		"created_at":   m.CreatedAt.Time.UTC().Format(time.RFC3339),
		"name":         u.Name,
		"email":        u.Email,
		"avatar_url":   avatar,
		"is_guest":     m.IsGuest,
	}
}
