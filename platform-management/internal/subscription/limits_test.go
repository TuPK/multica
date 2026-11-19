package subscription

import (
	"context"
	"errors"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/plan"
)

// Spec R6 section 3b.4 (QT4, D97): a tenant-wide limit override writes every
// workspace in one transaction, one audit row each with the tenant id.

const (
	limTenant = "5c000000-0000-0000-0000-000000000001"
	limWSTwo  = "5b000000-0000-0000-0000-000000000002"
)

func ptr(v int64) *int64 { return &v }

func TestLimitOverrideSetsEveryWorkspaceWithOneAuditEach(t *testing.T) {
	svc, store, cache := newService(t)
	store.workspaces[limWSTwo] = "Second"
	store.subs[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "pro", Status: StatusActive, Version: 3, LimitOverrides: plan.Limits{}}

	err := svc.SetLimitOverride(context.Background(), limTenant, []string{wsID, limWSTwo},
		LimitOverride{Gate: plan.GateMaxWorkspaces, Value: ptr(7)}, "contract raised", OverrideActor{ID: "staff-1", Label: "root@example.test"})
	if err != nil {
		t.Fatalf("SetLimitOverride: %v", err)
	}
	for _, ws := range []string{wsID, limWSTwo} {
		if v := store.subs[ws].LimitOverrides[plan.GateMaxWorkspaces]; v == nil || *v != 7 {
			t.Fatalf("%s override = %v, want 7", ws, v)
		}
	}
	if store.subs[wsID].Version != 4 || store.subs[limWSTwo].Version != 1 {
		t.Fatalf("versions = %d and %d, want 4 and 1 (new row starts at 1)", store.subs[wsID].Version, store.subs[limWSTwo].Version)
	}
	rows := 0
	for _, a := range store.audits {
		if a.Action == "subscription.limit_override_changed" {
			rows++
			if a.TenantID != limTenant || a.ActorType != "staff" {
				t.Fatalf("audit row = %+v", a)
			}
		}
	}
	if rows != 2 {
		t.Fatalf("audit rows = %d, want one per workspace", rows)
	}
	if len(cache.workspaces) != 2 {
		t.Fatalf("cache dropped %v, want both workspaces", cache.workspaces)
	}
}

func TestLimitOverrideIsAllOrNothing(t *testing.T) {
	svc, store, _ := newService(t)
	store.subs[wsID] = Subscription{WorkspaceID: wsID, PlanKey: "pro", Status: StatusActive, Version: 3, LimitOverrides: plan.Limits{}}
	const unknown = "5b000000-0000-0000-0000-0000000000ff"

	err := svc.SetLimitOverride(context.Background(), limTenant, []string{wsID, unknown},
		LimitOverride{Gate: plan.GateMaxWorkspaces, Value: ptr(7)}, "contract raised", OverrideActor{ID: "staff-1"})
	var serr *Error
	if err == nil || !errors.As(err, &serr) && !errors.Is(err, NotFound()) {
		t.Fatalf("err = %v, want not found", err)
	}
	if _, ok := store.subs[wsID].LimitOverrides[plan.GateMaxWorkspaces]; ok || store.subs[wsID].Version != 3 {
		t.Fatalf("the first workspace changed although the second failed: %+v", store.subs[wsID])
	}
	for _, a := range store.audits {
		if a.Action == "subscription.limit_override_changed" {
			t.Fatal("an audit row was written for a rolled-back change")
		}
	}
}

func TestLimitOverrideValidationAndRelease(t *testing.T) {
	svc, store, _ := newService(t)
	ctx := context.Background()
	by := OverrideActor{ID: "staff-1"}

	for _, c := range []struct {
		name string
		o    LimitOverride
		code string
	}{
		{"unknown gate", LimitOverride{Gate: "nope", Value: ptr(1)}, CodeLimitGateInvalid},
		{"negative value", LimitOverride{Gate: plan.GateMaxWorkspaces, Value: ptr(-1)}, CodeLimitValueInvalid},
	} {
		err := svc.SetLimitOverride(ctx, limTenant, []string{wsID}, c.o, "reason", by)
		var serr *Error
		if !errors.As(err, &serr) || serr.Code != c.code {
			t.Fatalf("%s: err = %v, want %s", c.name, err, c.code)
		}
	}
	if err := svc.SetLimitOverride(ctx, limTenant, []string{wsID}, LimitOverride{Gate: plan.GateMaxWorkspaces, Value: ptr(1)}, "  ", by); err == nil {
		t.Fatal("a blank reason must be refused")
	}
	if len(store.audits) != 0 {
		t.Fatal("refused input wrote audit rows")
	}

	if err := svc.SetLimitOverride(ctx, limTenant, []string{wsID}, LimitOverride{Gate: plan.GateMaxWorkspaces, Value: ptr(5)}, "raise", by); err != nil {
		t.Fatalf("set: %v", err)
	}
	if err := svc.SetLimitOverride(ctx, limTenant, []string{wsID}, LimitOverride{Gate: plan.GateMaxWorkspaces, Value: nil}, "release", by); err != nil {
		t.Fatalf("release: %v", err)
	}
	if _, ok := store.subs[wsID].LimitOverrides[plan.GateMaxWorkspaces]; ok {
		t.Fatal("a null value must release the override so the plan limit applies")
	}
}
