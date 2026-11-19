package planlimit

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/tenant"
)

type tenantCounter struct {
	events *[]string
	n      int64
}

func (c tenantCounter) CountTenantWorkspaceLinks(_ context.Context, id pgtype.UUID) (int64, error) {
	*c.events = append(*c.events, "count:"+uuid.UUID(id.Bytes).String())
	return c.n, nil
}

// The lock of the max_workspaces gate is scoped by the tenant id, not by a
// workspace id, so every creation and deletion in the tenant queues on it.
func TestAdmitScopesLockByTenantID(t *testing.T) {
	var events []string
	tenantID := uuid.New()
	d := limitDecision(entitlement.ActionEnforce, 3)
	p, _ := resolved(t, entitlement.GateMaxWorkspaces, &d)
	err := p.Admit(context.Background(), &fakeStore{events: &events}, pg(tenantID), 1,
		CountTenantWorkspaces(tenantCounter{events: &events, n: 2}, pg(tenantID)))
	if err != nil {
		t.Fatalf("Admit at 2 of 3: %v", err)
	}
	want := []string{"lock:" + tenantID.String() + "|" + tenant.Gate, "count:" + tenantID.String()}
	if len(events) != 2 || events[0] != want[0] || events[1] != want[1] {
		t.Fatalf("events = %v, want %v", events, want)
	}

	events = nil
	err = p.Admit(context.Background(), &fakeStore{events: &events}, pg(tenantID), 1,
		CountTenantWorkspaces(tenantCounter{events: &events, n: 3}, pg(tenantID)))
	var reached *ReachedError
	if !errors.As(err, &reached) || reached.Gate != entitlement.GateMaxWorkspaces || reached.Limit != 3 || reached.Used != 3 {
		t.Fatalf("Admit at 3 of 3 = %v, want a ReachedError for max_workspaces, limit 3, used 3", err)
	}
}
