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
