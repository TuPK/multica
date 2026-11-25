package service

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// The autonomy envelope (durable spec 3.4). The table test is the spec's rows; the DB
// tests drive the nightly job, the settlement path and the manual change.

var envelopeNow = time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)

func envelopeDaysAgo(d int) time.Time {
	return envelopeNow.Add(-time.Duration(d) * 24 * time.Hour)
}

// TestNextSquadLevelTable: each row of spec 3.4, and the limits of the envelope.
func TestNextSquadLevelTable(t *testing.T) {
	wide := Envelope{Floor: 0, Ceiling: 3}
	// Every condition of the L1 to L2 and L2 to L3 rows is met.
	solid := SquadStats{N: 30, ScoreLower: 0.95, AcceptanceRate: 0.80, HasAcceptance: true, LevelSince: envelopeDaysAgo(70)}

	cases := []struct {
		name     string
		cur      int
		env      Envelope
		stats    SquadStats
		lastAuto time.Time
		want     int
	}{
		{"L0 to L1 at a lower bound of 0.60", 0, wide, SquadStats{N: 20, ScoreLower: 0.60}, time.Time{}, 1},
		{"L0 holds under 0.60", 0, wide, SquadStats{N: 20, ScoreLower: 0.59}, time.Time{}, 0},
		{"L1 to L2 with every condition met", 1, wide, solid, time.Time{}, 2},
		{"L1 holds under a lower bound of 0.80", 1, wide, SquadStats{N: 30, ScoreLower: 0.79, AcceptanceRate: 0.9, HasAcceptance: true}, time.Time{}, 1},
		{"L1 holds with a high fail in 30 days", 1, wide, SquadStats{N: 30, ScoreLower: 0.95, HighFail30: true, AcceptanceRate: 0.9, HasAcceptance: true}, time.Time{}, 1},
		{"L1 holds when acceptance is under 60 percent", 1, wide, SquadStats{N: 30, ScoreLower: 0.95, AcceptanceRate: 0.59, HasAcceptance: true}, time.Time{}, 1},
		{"L1 holds with no acceptance sample", 1, wide, SquadStats{N: 30, ScoreLower: 0.95}, time.Time{}, 1},
		{"L2 to L3 with every condition met", 2, wide, solid, time.Time{}, 3},
		{"L2 holds before 60 days at L2", 2, wide, SquadStats{N: 30, ScoreLower: 0.95, LevelSince: envelopeDaysAgo(59)}, time.Time{}, 2},
		{"L2 holds with a high fail in 90 days", 2, wide, SquadStats{N: 30, ScoreLower: 0.95, HighFail90: true, LevelSince: envelopeDaysAgo(70)}, time.Time{}, 2},
		{"L2 holds under a lower bound of 0.90", 2, wide, SquadStats{N: 30, ScoreLower: 0.89, LevelSince: envelopeDaysAgo(70)}, time.Time{}, 2},
		{"an envelope capped at 2 never reaches L3", 2, Envelope{Floor: 1, Ceiling: 2}, solid, time.Time{}, 2},
		{"an envelope capped at 0 never reaches L1", 0, Envelope{Floor: 0, Ceiling: 0}, SquadStats{N: 30, ScoreLower: 0.99}, time.Time{}, 0},
		{"L1 falls to L0 under 0.60 with a floor of 0", 1, wide, SquadStats{N: 30, ScoreLower: 0.59}, time.Time{}, 0},
		{"L1 never falls below a floor of 1", 1, Envelope{Floor: 1, Ceiling: 2}, SquadStats{N: 30, ScoreLower: 0.10}, time.Time{}, 1},
		{"L2 falls to L1 under 0.80", 2, wide, SquadStats{N: 30, ScoreLower: 0.79, LevelSince: envelopeDaysAgo(70)}, time.Time{}, 1},
		{"L3 falls to L2 under 0.90", 3, wide, SquadStats{N: 30, ScoreLower: 0.89}, time.Time{}, 2},
		{"a floor of 2 stops the fall at L2", 2, Envelope{Floor: 2, Ceiling: 3}, SquadStats{N: 30, ScoreLower: 0.10}, time.Time{}, 2},
		{"one step per calculation", 0, wide, SquadStats{N: 30, ScoreLower: 0.99}, time.Time{}, 1},
		{"the 14-day rule holds a second rise at 13 days", 1, wide, solid, envelopeDaysAgo(13), 1},
		{"the 14-day rule allows a rise at 14 days", 1, wide, solid, envelopeDaysAgo(14), 2},
		{"n under 20 holds a rise", 1, wide, SquadStats{N: 19, ScoreLower: 0.99, AcceptanceRate: 0.9, HasAcceptance: true}, time.Time{}, 1},
		{"n under 20 holds a fall", 2, wide, SquadStats{N: 19, ScoreLower: 0.10}, time.Time{}, 2},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := NextSquadLevel(c.cur, c.env, c.stats, c.lastAuto, envelopeNow)
			if got != c.want {
				t.Fatalf("NextSquadLevel(cur=%d, env=%+v) = %d, want %d", c.cur, c.env, got, c.want)
			}
		})
	}
}

// envelopeService is the envelope service over the fixture, with the outcomes flag on
// or off.
func envelopeService(t *testing.T, f principalFixture, flagOn bool) *EnvelopeService {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: flagOn})
	return &EnvelopeService{
		Queries:   f.q,
		TxStarter: f.Pool,
		Flags:     featureflag.NewService(provider),
		Now:       time.Now,
	}
}

// squadFixture is a squad with a leader agent, at the given level, with cleanups.
func squadFixture(t *testing.T, vf verificationFixture, level int) string {
	t.Helper()
	squad := vf.Squad(t, "envelope squad", vf.doer)
	vf.Exec(t, `UPDATE squad SET autonomy_level = $2 WHERE id = $1`, squad, level)
	vf.Cleanup(t, `DELETE FROM governance_event WHERE subject_id = $1`, squad)
	vf.Cleanup(t, `DELETE FROM squad_attention WHERE squad_id = $1`, squad)
	vf.Cleanup(t, `DELETE FROM work_outcome WHERE squad_id = $1`, squad)
	return squad
}

// setEnvelope writes a squad's envelope as the boss would.
func setEnvelope(t *testing.T, f principalFixture, squad string, floor, ceiling int) {
	t.Helper()
	f.Exec(t, `INSERT INTO squad_autonomy_envelope (workspace_id, squad_id, floor_level, ceiling_level, set_by, reason)
		VALUES ($1, $2, $3, $4, $5, 'envelope test')`, f.WorkspaceID, squad, floor, ceiling, f.UserID)
	f.Cleanup(t, `DELETE FROM squad_autonomy_envelope WHERE squad_id = $1`, squad)
}

// seedSquadOutcomes settles n outcomes of a squad, all with the same verdict, settled
// daysAgo days ago.
func seedSquadOutcomes(t *testing.T, f principalFixture, squad, agent string, n int, verdict, severity string, daysAgo int) {
	t.Helper()
	f.Exec(t, `INSERT INTO work_outcome (workspace_id, issue_id, done_transition_id, agent_id, squad_id, work_class,
		verification_mode, sampled, verdict, severity, settle_at, settled_at)
		SELECT $1, gen_random_uuid(), gen_random_uuid(), $2, $3, 'general', 'full', true, $4, $5,
			now() - make_interval(days => $6::int), now() - make_interval(days => $6::int)
		FROM generate_series(1, $7::int)`, f.WorkspaceID, agent, squad, verdict, severity, daysAgo, n)
}

// seedAcceptance writes the squad's attention row, as the nightly recompute would.
func seedAcceptance(t *testing.T, f principalFixture, squad string, rate float64, sample int) {
	t.Helper()
	f.Exec(t, `INSERT INTO squad_attention (squad_id, workspace_id, pending_cap, weekly_cap, acceptance_rate, sample_size, computed_at)
		VALUES ($1, $2, 3, 5, $3::numeric, $4, now())
		ON CONFLICT (squad_id) DO UPDATE SET acceptance_rate = EXCLUDED.acceptance_rate, sample_size = EXCLUDED.sample_size`,
		squad, f.WorkspaceID, rate, sample)
}

// autoChanges counts the autonomy changes the machine wrote for a squad.
func autoChanges(t *testing.T, f principalFixture, squad string) int {
	t.Helper()
	return f.Count(t, `SELECT count(*) FROM governance_event WHERE workspace_id = $1 AND kind = 'autonomy_changed'
		AND subject_id = $2 AND actor_type = 'track_record'`, f.WorkspaceID, squad)
}

func squadLevel(t *testing.T, f principalFixture, squad string) int {
	t.Helper()
	var level int
	f.QueryRow(t, `SELECT autonomy_level FROM squad WHERE id = $1`, squad).Scan(&level)
	return level
}

// TestNoEnvelopeNoAutoChange: a squad with no envelope never changes its level by
// itself, not in the nightly job and not on a high fail.
func TestNoEnvelopeNoAutoChange(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	squad := squadFixture(t, vf, 1)
	seedSquadOutcomes(t, vf.principalFixture, squad, vf.doer, 25, "pass", "none", 1)
	seedAcceptance(t, vf.principalFixture, squad, 0.90, 20)

	if _, err := envelopeService(t, vf.principalFixture, true).Nightly(ctx); err != nil {
		t.Fatalf("nightly: %v", err)
	}
	seedSquadOutcomes(t, vf.principalFixture, squad, vf.doer, 1, "fail", "high", 0)
	high := db.WorkOutcome{WorkspaceID: vf.ws, SquadID: util.MustParseUUID(squad), Verdict: "fail", Severity: "high"}
	if err := (&EnvelopeService{Now: time.Now}).OnHighFail(ctx, vf.q, high); err != nil {
		t.Fatalf("on high fail: %v", err)
	}

	if got := squadLevel(t, vf.principalFixture, squad); got != 1 {
		t.Fatalf("squad without an envelope moved to L%d, want L1", got)
	}
	if n := autoChanges(t, vf.principalFixture, squad); n != 0 {
		t.Fatalf("automatic changes without an envelope = %d, want none", n)
	}
}

// TestHighFailDropsSquadNow: a settled high fail drops its squad one level in the same
// settlement, without waiting for the nightly job, and never below the floor.
func TestHighFailDropsSquadNow(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	squad := squadFixture(t, vf, 2)
	setEnvelope(t, vf.principalFixture, squad, 0, 3)

	issue := vf.doneIssue(t, vf.doer)
	c := vf.addCriterion(t, issue, "The report has a header.", CheckKindManual, map[string]any{})
	o := vf.currentOutcome(t, issue)
	vf.Exec(t, `UPDATE work_outcome SET sampled = true, squad_id = $2, settle_at = now() - interval '1 minute' WHERE id = $1`,
		util.UUIDToString(o.ID), squad)
	if _, err := vf.checks.Record(ctx, vf.ws, vf.member, o.ID, VerificationInput{
		CriterionID: c.ID, Result: "fail", Severity: "high", Note: "missing header",
	}); err != nil {
		t.Fatalf("record check: %v", err)
	}

	if _, err := vf.outcomes.Settle(ctx, time.Now()); err != nil {
		t.Fatalf("settle: %v", err)
	}

	if got := squadLevel(t, vf.principalFixture, squad); got != 1 {
		t.Fatalf("after a settled high fail the squad is L%d, want L1", got)
	}
	if n := autoChanges(t, vf.principalFixture, squad); n != 1 {
		t.Fatalf("automatic changes after the high fail = %d, want one", n)
	}
}

// TestHighFailNeverBelowFloor: a squad already at its floor is not dropped by a high fail.
func TestHighFailNeverBelowFloor(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	squad := squadFixture(t, vf, 1)
	setEnvelope(t, vf.principalFixture, squad, 1, 3)
	seedSquadOutcomes(t, vf.principalFixture, squad, vf.doer, 1, "fail", "high", 0)
	high := db.WorkOutcome{WorkspaceID: vf.ws, SquadID: util.MustParseUUID(squad), Verdict: "fail", Severity: "high"}

	if err := (&EnvelopeService{Now: time.Now}).OnHighFail(ctx, vf.q, high); err != nil {
		t.Fatalf("on high fail: %v", err)
	}
	if got := squadLevel(t, vf.principalFixture, squad); got != 1 {
		t.Fatalf("squad at its floor moved to L%d on a high fail, want L1", got)
	}
}

// TestAutoChangeLogsOneEvent: a rise by the nightly job writes exactly one governance
// event, a second run in the same window writes none, and the change is in the digest
// the next brief reads.
func TestAutoChangeLogsOneEvent(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	squad := squadFixture(t, vf, 1)
	setEnvelope(t, vf.principalFixture, squad, 0, 2)
	seedSquadOutcomes(t, vf.principalFixture, squad, vf.doer, 20, "pass", "none", 1)
	seedAcceptance(t, vf.principalFixture, squad, 0.70, 20)
	svc := envelopeService(t, vf.principalFixture, true)

	if _, err := svc.Nightly(ctx); err != nil {
		t.Fatalf("nightly: %v", err)
	}
	if got := squadLevel(t, vf.principalFixture, squad); got != 2 {
		t.Fatalf("squad after the nightly job is L%d, want L2", got)
	}
	if _, err := svc.Nightly(ctx); err != nil {
		t.Fatalf("second nightly: %v", err)
	}
	if n := autoChanges(t, vf.principalFixture, squad); n != 1 {
		t.Fatalf("automatic governance events = %d, want exactly one", n)
	}
	var from, to int
	vf.QueryRow(t, `SELECT (details->>'from')::int, (details->>'to')::int FROM governance_event
		WHERE subject_id = $1 AND actor_type = 'track_record' AND kind = 'autonomy_changed'`, squad).Scan(&from, &to)
	if from != 1 || to != 2 {
		t.Fatalf("event details from=%d to=%d, want 1 and 2", from, to)
	}

	digest, err := (&DigestService{Queries: vf.q, Now: time.Now}).Compute(ctx, vf.ws, time.Now().Add(-time.Hour), "")
	if err != nil {
		t.Fatalf("digest: %v", err)
	}
	if digest.AutonomyAutoChanges.Count != 1 {
		t.Fatalf("digest autonomy changes = %d, want one", digest.AutonomyAutoChanges.Count)
	}
}

// TestManualChangeOutsideEnvelopeWidensIt: a boss's level change outside the envelope
// widens the envelope to take it in; a change inside leaves the envelope alone.
func TestManualChangeOutsideEnvelopeWidensIt(t *testing.T) {
	vf := newVerificationFixture(t)
	ctx := context.Background()
	squad := squadFixture(t, vf, 1)
	setEnvelope(t, vf.principalFixture, squad, 1, 2)
	svc := &AutonomyService{Queries: vf.q, TxStarter: vf.Pool}

	if _, err := svc.Set(ctx, vf.ws, util.MustParseUUID(squad), 2, vf.member, "inside the envelope"); err != nil {
		t.Fatalf("set inside: %v", err)
	}
	if _, err := svc.Set(ctx, vf.ws, util.MustParseUUID(squad), 3, vf.member, "outside the envelope"); err != nil {
		t.Fatalf("set outside: %v", err)
	}

	var floor, ceiling int
	vf.QueryRow(t, `SELECT floor_level, ceiling_level FROM squad_autonomy_envelope WHERE squad_id = $1`, squad).Scan(&floor, &ceiling)
	if floor != 1 || ceiling != 3 {
		t.Fatalf("envelope after the boss set L3 is [%d, %d], want [1, 3]", floor, ceiling)
	}
}
