package seat

import (
	"context"
	"testing"
)

// A staff add-member request uses its id as the seat token (spec R6 section
// 4.2, rule 7): claiming twice with it holds one seat, and releasing frees it.
func TestStaffAddClaimIsIdempotentByRequestToken(t *testing.T) {
	f := newDBFixture(t, 3, 1)
	ctx := context.Background()
	request := newUUID(t)

	first, err := f.svc.Claim(ctx, f.ws, request, request)
	if err != nil || !first.Managed || !first.Allowed {
		t.Fatalf("first claim = %+v, %v", first, err)
	}
	again, err := f.svc.Claim(ctx, f.ws, request, request)
	if err != nil || !again.Allowed {
		t.Fatalf("second claim = %+v, %v", again, err)
	}
	if again.Capacity == nil || again.Capacity.ReservedSeats != 1 {
		t.Fatalf("reserved seats after two claims = %+v, want 1", again.Capacity)
	}
	if again.Operation == nil || again.Operation.State != StateConsuming {
		t.Errorf("operation = %+v, want consuming", again.Operation)
	}

	// Another request takes a different seat; the workspace still has one free.
	other := newUUID(t)
	if d, err := f.svc.Claim(ctx, f.ws, other, other); err != nil || !d.Allowed {
		t.Fatalf("claim of another request = %+v, %v", d, err)
	}
	third := newUUID(t)
	if d, err := f.svc.Claim(ctx, f.ws, third, third); err != nil || d.Allowed || d.Reason != ReasonCapacityFull {
		t.Fatalf("third claim = %+v, %v, want capacity_full: 1 member + 2 holds on 3 seats", d, err)
	}

	// Release frees the seat once; a second release is the answer of Appendix B rule 2.
	rel, err := f.svc.Release(ctx, f.ws, request)
	if err != nil || !rel.Allowed {
		t.Fatalf("release = %+v, %v", rel, err)
	}
	if rel.Capacity == nil || rel.Capacity.ReservedSeats != 1 {
		t.Errorf("reserved seats after the release = %+v, want 1", rel.Capacity)
	}
	if again, err := f.svc.Release(ctx, f.ws, request); err != nil || again.Allowed || again.Reason != ReasonReleased {
		t.Errorf("second release = %+v, %v, want not allowed with reason released", again, err)
	}
	if d, err := f.svc.Claim(ctx, f.ws, third, third); err != nil || !d.Allowed {
		t.Errorf("claim after the release = %+v, %v, want the freed seat", d, err)
	}
}
