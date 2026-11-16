package subscription

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/plan"
)

func TestDBWorkspaceWithoutAProfileRowIsIndividual(t *testing.T) {
	f := newDBFixture(t)
	ws := f.workspace(nil)
	reader := NewPgxProfiles(f.pool)
	kind, err := AccountKind(context.Background(), reader, ws)
	if err != nil || kind != AccountIndividual {
		t.Fatalf("AccountKind = %q, %v; want individual", kind, err)
	}
	if n := f.count(`SELECT count(*) FROM billing_profile WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("a read wrote %d rows", n)
	}
}

func TestDBChangeAccountKindWritesTheRowAndTheAuditRow(t *testing.T) {
	f := newDBFixture(t)
	owner := f.user()
	ws := f.workspace(map[string]string{owner: "owner"})
	svc := NewProfileService(NewPgxProfileRunner(f.pool))
	reader := NewPgxProfiles(f.pool)
	ctx := context.Background()
	change := func(kind string, fields *ProfileFields) (KindResult, error) {
		return svc.ChangeAccountKind(ctx, KindChange{
			WorkspaceID: ws, Kind: kind, Profile: fields,
			ActorType: audit.ActorMember, ActorID: owner, ActorLabel: "owner@example.test",
		})
	}

	if _, err := change(AccountBusiness, &ProfileFields{LegalName: "Acme Co"}); asError(t, err).Code != "billing_profile_incomplete" {
		t.Fatalf("incomplete profile: %v", err)
	}
	if n := f.count(`SELECT count(*) FROM billing_profile WHERE workspace_id = $1`, ws); n != 0 {
		t.Fatalf("a refused change left %d rows", n)
	}

	fields := completeProfile()
	res, err := change(AccountBusiness, &fields)
	if err != nil || !res.Changed {
		t.Fatalf("to business = %+v, %v", res, err)
	}
	got, found, err := reader.Profile(ctx, ws)
	if err != nil || !found {
		t.Fatalf("Profile = %v, %v", found, err)
	}
	if got.AccountKind != AccountBusiness || got.LegalName != "Acme Co" || got.TaxCode != "0123456789" ||
		got.Address != "1 Main St" || got.InvoiceEmail != "ap@acme.test" || got.UpdatedBy != owner || got.UpdatedAt.IsZero() {
		t.Fatalf("stored = %+v", got)
	}

	back, err := change(AccountIndividual, nil)
	if err != nil || !back.Changed {
		t.Fatalf("to individual = %+v, %v", back, err)
	}
	got, _, err = reader.Profile(ctx, ws)
	if err != nil || got.AccountKind != AccountIndividual || got.LegalName != "Acme Co" || got.TaxCode != "0123456789" {
		t.Fatalf("profile after change back = %+v, %v", got, err)
	}
	if n := f.count(`SELECT count(*) FROM billing_profile WHERE workspace_id = $1`, ws); n != 1 {
		t.Fatalf("rows = %d, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2 AND actor_type = 'member' AND actor_id = $3
		AND detail->'changes'->'account_kind'->>'from' = 'individual' AND detail->'changes'->'account_kind'->>'to' = 'business'`,
		ws, audit.ActionWorkspaceAccountKindChanged, owner); n != 1 {
		t.Fatalf("audit rows for individual to business = %d, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND action = $2
		AND detail->'changes'->'account_kind'->>'from' = 'business' AND detail->'changes'->'account_kind'->>'to' = 'individual'`,
		ws, audit.ActionWorkspaceAccountKindChanged); n != 1 {
		t.Fatalf("audit rows for business to individual = %d, want 1", n)
	}
	if n := f.count(`SELECT count(*) FROM audit_event WHERE workspace_id = $1 AND detail::text LIKE '%0123456789%'`, ws); n != 0 {
		t.Fatalf("the tax code reached the audit log")
	}
}

func TestDBChangeBackIsRefusedOnABusinessOnlyPlan(t *testing.T) {
	f := newDBFixture(t)
	ws := f.workspace(nil)
	p := f.plan(func(p *plan.Plan) { p.Audience = plan.AudienceBusiness })
	ctx := context.Background()
	if _, err := f.service(&recordingCache{}).Apply(ctx, Change{
		WorkspaceID: ws, PlanKey: p.Key, Seats: 3, Interval: IntervalMonth, Reason: "db test", ActorID: f.user(), ActorLabel: "staff",
	}); err != nil {
		t.Fatal(err)
	}
	svc := NewProfileService(NewPgxProfileRunner(f.pool))
	fields := completeProfile()
	actor := KindChange{WorkspaceID: ws, ActorType: audit.ActorStaff, ActorLabel: "staff"}
	to := func(kind string, fl *ProfileFields) error {
		c := actor
		c.Kind, c.Profile = kind, fl
		_, err := svc.ChangeAccountKind(ctx, c)
		return err
	}
	if err := to(AccountBusiness, &fields); err != nil {
		t.Fatal(err)
	}
	e := asError(t, to(AccountIndividual, nil))
	if e.Status != http.StatusConflict || e.Code != "plan_audience_mismatch" {
		t.Fatalf("refusal = %v", e)
	}
	kind, err := AccountKind(ctx, NewPgxProfiles(f.pool), ws)
	if err != nil || kind != AccountBusiness {
		t.Fatalf("kind after the refusal = %q, %v", kind, err)
	}
	if err := CheckPlanForWorkspace(ctx, NewPgxProfiles(f.pool), ws, p); err != nil {
		t.Fatalf("a business buying a business plan: %v", err)
	}
}

// Migration 955 makes (workspace_id) unique, so two rows for one workspace
// cannot exist even when two writers get past the advisory lock.
func TestDBBillingProfileWorkspaceIDIsUnique(t *testing.T) {
	f := newDBFixture(t)
	ws := f.workspace(nil)
	ctx := context.Background()
	if n := f.count(`SELECT count(*) FROM pg_indexes WHERE indexname = 'billing_profile_uidx' AND indexdef LIKE 'CREATE UNIQUE INDEX%(workspace_id)'`); n != 1 {
		t.Fatalf("billing_profile_uidx on (workspace_id) found %d times, want 1: migration 955 is not applied", n)
	}
	t.Cleanup(func() { f.pool.Exec(context.Background(), `DELETE FROM billing_profile WHERE workspace_id = $1`, ws) })
	if _, err := f.pool.Exec(ctx, `INSERT INTO billing_profile (workspace_id) VALUES ($1)`, ws); err != nil {
		t.Fatal(err)
	}
	_, err := f.pool.Exec(ctx, `INSERT INTO billing_profile (workspace_id) VALUES ($1)`, ws)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23505" {
		t.Fatalf("second row for one workspace: err = %v, want unique_violation", err)
	}
}

func TestDBChangeAccountKindOfAnUnknownWorkspaceIs404(t *testing.T) {
	f := newDBFixture(t)
	svc := NewProfileService(NewPgxProfileRunner(f.pool))
	fields := completeProfile()
	_, err := svc.ChangeAccountKind(context.Background(), KindChange{
		WorkspaceID: f.randomUUID(), Kind: AccountBusiness, Profile: &fields, ActorType: audit.ActorStaff,
	})
	if e := asError(t, err); e.Status != http.StatusNotFound {
		t.Fatalf("refusal = %v", e)
	}
}
