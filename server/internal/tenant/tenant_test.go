package tenant

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

var (
	wsID     = pgtype.UUID{Bytes: [16]byte{1}, Valid: true}
	tenantID = pgtype.UUID{Bytes: [16]byte{2}, Valid: true}
)

// fakeQueries records the calls in order and plays a tenant with `links`
// workspaces.
type fakeQueries struct {
	calls     []string
	locked    db.LockPlanGateParams
	linked    bool
	links     int64
	lockErr   error
	deleteErr error
}

func (f *fakeQueries) LockPlanGate(_ context.Context, arg db.LockPlanGateParams) error {
	f.calls = append(f.calls, "lock")
	f.locked = arg
	return f.lockErr
}

func (f *fakeQueries) GetTenantIDByWorkspace(context.Context, pgtype.UUID) (pgtype.UUID, error) {
	f.calls = append(f.calls, "get")
	if !f.linked {
		return pgtype.UUID{}, pgx.ErrNoRows
	}
	return tenantID, nil
}

func (f *fakeQueries) DeleteTenantWorkspaceLink(context.Context, pgtype.UUID) error {
	f.calls = append(f.calls, "delete link")
	f.links--
	return f.deleteErr
}

func (f *fakeQueries) CountTenantWorkspaceLinks(context.Context, pgtype.UUID) (int64, error) {
	f.calls = append(f.calls, "count")
	return f.links, nil
}

func (f *fakeQueries) DeleteTenant(context.Context, pgtype.UUID) error {
	f.calls = append(f.calls, "delete tenant")
	return nil
}

func TestGateIsMaxWorkspaces(t *testing.T) {
	if Gate != "max_workspaces" {
		t.Fatalf("Gate = %q", Gate)
	}
}

func TestLockScopesTheAdvisoryLockByTenantID(t *testing.T) {
	f := &fakeQueries{}
	if err := Lock(context.Background(), f, tenantID); err != nil {
		t.Fatal(err)
	}
	if f.locked.Scope != "02000000-0000-0000-0000-000000000000" || f.locked.Gate != "max_workspaces" {
		t.Fatalf("lock = %+v, want the tenant id as scope and the max_workspaces gate", f.locked)
	}
}

func TestDetachLocksTheTenantBeforeChangingAnything(t *testing.T) {
	f := &fakeQueries{linked: true, links: 2}
	if err := Detach(context.Background(), f, wsID); err != nil {
		t.Fatal(err)
	}
	want := []string{"get", "lock", "delete link", "count"}
	if len(f.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", f.calls, want)
	}
	for i := range want {
		if f.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", f.calls, want)
		}
	}
}

func TestDetachDeletesTheTenantOnlyWhenItsLastWorkspaceGoes(t *testing.T) {
	last := &fakeQueries{linked: true, links: 1}
	if err := Detach(context.Background(), last, wsID); err != nil {
		t.Fatal(err)
	}
	if got := last.calls[len(last.calls)-1]; got != "delete tenant" {
		t.Fatalf("calls = %v, want the tenant deleted after the last link", last.calls)
	}
}

func TestDetachOfAWorkspaceWithoutATenantDoesNothing(t *testing.T) {
	f := &fakeQueries{}
	if err := Detach(context.Background(), f, wsID); err != nil {
		t.Fatal(err)
	}
	if len(f.calls) != 1 || f.calls[0] != "get" {
		t.Fatalf("calls = %v, want one read and no lock or delete", f.calls)
	}
}

func TestDetachReturnsTheErrorOfAFailedStep(t *testing.T) {
	boom := errors.New("boom")
	if err := Detach(context.Background(), &fakeQueries{linked: true, links: 1, lockErr: boom}, wsID); !errors.Is(err, boom) {
		t.Fatalf("lock failure = %v", err)
	}
	f := &fakeQueries{linked: true, links: 1, deleteErr: boom}
	if err := Detach(context.Background(), f, wsID); !errors.Is(err, boom) {
		t.Fatalf("delete failure = %v", err)
	}
	for _, c := range f.calls {
		if c == "delete tenant" {
			t.Fatal("the tenant was deleted after the link delete failed")
		}
	}
}
