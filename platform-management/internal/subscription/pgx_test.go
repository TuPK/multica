package subscription

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/internal/db"
)

func TestFromRowKeepsAnExplicitNullOverrideAndUTCTimes(t *testing.T) {
	var id pgtype.UUID
	if err := id.Scan(wsID); err != nil {
		t.Fatal(err)
	}
	end := time.Date(2030, 6, 1, 7, 0, 0, 0, time.FixedZone("x", 7*3600))
	s, err := fromRow(db.WorkspaceSubscription{
		WorkspaceID: id, PlanKey: "pro", Status: "active", Seats: 5, Interval: "year",
		CurrentPeriodEnd: pgtype.Timestamptz{Time: end, Valid: true},
		LimitOverrides:   []byte(`{"issue_count": null, "agents": 4}`), Version: 3,
		UpdatedAt: pgtype.Timestamptz{Time: end, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if s.WorkspaceID != wsID || s.PlanKey != "pro" || s.Seats != 5 || s.Version != 3 || s.Interval != "year" {
		t.Fatalf("subscription = %+v", s)
	}
	if v, ok := s.LimitOverrides["issue_count"]; !ok || v != nil || *s.LimitOverrides["agents"] != 4 {
		t.Fatalf("overrides = %v, want an explicit null and 4", s.LimitOverrides)
	}
	if s.CurrentPeriodEnd == nil || s.CurrentPeriodEnd.Location() != time.UTC || !s.CurrentPeriodEnd.Equal(end) {
		t.Fatalf("period end = %v", s.CurrentPeriodEnd)
	}
	if s.CurrentPeriodStart != nil || s.TrialEnd != nil || s.GraceUntil != nil {
		t.Fatalf("unset times came back set: %+v", s)
	}
}

func TestFromRowRejectsOverridesThatAreNotAnObject(t *testing.T) {
	if _, err := fromRow(db.WorkspaceSubscription{LimitOverrides: []byte(`[1]`)}); err == nil {
		t.Fatal("an overrides array was accepted")
	}
}

func TestFromRowReadsPendingChangeAndUnitPrice(t *testing.T) {
	var id pgtype.UUID
	if err := id.Scan(wsID); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2030, 6, 1, 7, 0, 0, 0, time.UTC)
	s, err := fromRow(db.WorkspaceSubscription{
		WorkspaceID: id, PlanKey: "pro", Status: "active", LimitOverrides: []byte(`{}`),
		PendingChange: []byte(`{"plan_key":"free","seats":3,"interval":"year","requested_by":"` + wsID + `","requested_at":"2030-06-01T07:00:00Z"}`),
		UnitPrice:     pgtype.Int8{Int64: 149000, Valid: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := PendingChange{PlanKey: "free", Seats: 3, Interval: IntervalYear, RequestedBy: wsID, RequestedAt: at}
	if s.PendingChange == nil || *s.PendingChange != want {
		t.Fatalf("pending change = %+v, want %+v", s.PendingChange, want)
	}
	if s.UnitPrice == nil || *s.UnitPrice != 149000 {
		t.Fatalf("unit price = %v, want 149000", s.UnitPrice)
	}
}

// A row written before migration 953 has NULL in both columns, and a JSON null
// is the same as none.
func TestFromRowReadsAnOldRowWithoutPendingChange(t *testing.T) {
	for name, raw := range map[string][]byte{"sql null": nil, "json null": []byte(`null`), "empty object": []byte(`{}`)} {
		s, err := fromRow(db.WorkspaceSubscription{LimitOverrides: []byte(`{}`), PendingChange: raw})
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if s.PendingChange != nil || s.UnitPrice != nil {
			t.Errorf("%s: pending change %+v, unit price %v; want both empty", name, s.PendingChange, s.UnitPrice)
		}
	}
	if _, err := fromRow(db.WorkspaceSubscription{LimitOverrides: []byte(`{}`), PendingChange: []byte(`[1]`)}); err == nil {
		t.Error("a pending change that is an array was accepted")
	}
}
