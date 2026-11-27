package service

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

func TestSurfaceOf(t *testing.T) {
	cases := []struct {
		name string
		in   EffectRef
		want string
	}{
		{"connected app splits toolkit before first underscore", EffectRef{"connected_app", "GMAIL_SEND_EMAIL"}, "app:GMAIL"},
		{"connected app without underscore keeps whole target", EffectRef{"connected_app", "github"}, "app:github"},
		{"connected app with leading underscore keeps whole target", EffectRef{"connected_app", "_X"}, "app:_X"},
		{"channel uses target as type", EffectRef{"channel", "slack"}, "channel:slack"},
		{"declared URL uses host without port", EffectRef{"declared", "https://api.example.com:8443/v1/x"}, "external:api.example.com"},
		{"declared smtp host without port", EffectRef{"declared", "smtp:mail.example.com:587"}, "external:mail.example.com"},
		{"declared host prefix", EffectRef{"declared", "host:api.example.com:443"}, "external:api.example.com"},
		{"declared unparseable target", EffectRef{"declared", "not a url"}, "external:unknown"},
		{"declared empty target", EffectRef{"declared", ""}, "external:unknown"},
		{"other source keeps first two work_class segments", EffectRef{"cli", "flow:abc:stage_1"}, "area:flow:abc"},
		{"other source with one segment keeps it", EffectRef{"webhook", "general"}, "area:general"},
		{"other source with squad segment", EffectRef{"publish_channel", "squad:123"}, "area:squad:123"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SurfaceOf(tc.in); got != tc.want {
				t.Fatalf("SurfaceOf(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestReachStateTable(t *testing.T) {
	cases := []struct {
		name string
		in   SurfaceStats
		want string
	}{
		{
			"agent operable: allowed agent effects and no gap asks",
			SurfaceStats{EffectsAgent30d: 5, Effects30d: 5, AllowedAgent: true},
			"agent_operable",
		},
		{
			"gap asks block operable and fall through to confirm",
			SurfaceStats{EffectsAgent30d: 4, Effects30d: 4, AgentConfirm30d: 3, AllowedAgent: true, GapAsks30d: 2},
			"agent_with_confirm",
		},
		{
			"agent mostly through confirm",
			SurfaceStats{EffectsAgent30d: 4, Effects30d: 4, AgentConfirm30d: 3},
			"agent_with_confirm",
		},
		{
			"exactly half through confirm is not a majority",
			SurfaceStats{EffectsAgent30d: 4, Effects30d: 4, AgentConfirm30d: 2},
			"unknown",
		},
		{
			"only declared effects",
			SurfaceStats{Effects30d: 2, DeclaredEffects30d: 2},
			"declared_only",
		},
		{
			"declared only beats missing access",
			SurfaceStats{Effects30d: 2, DeclaredEffects30d: 2, MissingAccess: true},
			"declared_only",
		},
		{
			"missing access with no agent effects is human only",
			SurfaceStats{Effects30d: 3, MissingAccess: true},
			"human_only",
		},
		{
			"missing access with agent effects is not human only",
			SurfaceStats{EffectsAgent30d: 1, Effects30d: 1, MissingAccess: true},
			"unknown",
		},
		{
			"no data is unknown",
			SurfaceStats{},
			"unknown",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ReachState(tc.in); got != tc.want {
				t.Fatalf("ReachState(%+v) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

// reachFixedNow is the clock of the reach tests: the window is the 30 days before it.
var reachFixedNow = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)

// flagsOpen is a flag service with durable_open on or off.
func flagsOpen(on bool) *featureflag.Service {
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurableOpen, featureflag.Rule{Default: on})
	return featureflag.NewService(p)
}

// reachEnv is the policy workspace with a Reach Map service on a fixed clock.
type reachEnv struct {
	*policyEnv
	reach *ReachService
	squad pgtype.UUID
}

func newReachEnv(t *testing.T, on bool) reachEnv {
	t.Helper()
	e := newPolicyEnv(t)
	pool := sharedTestPool(t)
	return reachEnv{
		policyEnv: e,
		reach: &ReachService{
			Queries:      e.q,
			Issues:       NewIssueService(e.q, pool, nil, nil, nil),
			Verification: &VerificationService{Queries: e.q, TxStarter: pool, Flags: flagsOpen(true)},
			Flags:        flagsOpen(on),
			Now:          func() time.Time { return reachFixedNow },
		},
		squad: e.squad("Reach squad", AutonomyLevelWork),
	}
}

// effect records an effect in the window; byAgent makes an agent its actor.
func (e reachEnv) effect(t *testing.T, source, target, decision string, byAgent bool) {
	t.Helper()
	var agent any
	if byAgent {
		agent = util.UUIDToString(e.agent)
	}
	e.fx.Exec(t, `INSERT INTO effect_event (workspace_id, agent_id, source, target, effect_class, reversibility, decision, status, created_at)
		VALUES ($1, $2, $3, $4, 'write', 'reversible', $5, 'executed', $6)`,
		util.UUIDToString(e.ws), agent, source, target, decision, reachFixedNow.AddDate(0, 0, -2))
}

// ask puts an effect intent on the line and a question about it, asked by the reach squad.
func (e reachEnv) ask(t *testing.T, source, target, reason string) {
	t.Helper()
	var intent string
	if err := e.fx.Pool.QueryRow(context.Background(), `INSERT INTO effect_intent
		(workspace_id, agent_id, source, target, effect_class, reversibility, call, call_sha256, preview, expires_at, created_at)
		VALUES ($1, $2, $3, $4, 'write', 'reversible', '{}', 'x', '{}', $5, $6) RETURNING id::text`,
		util.UUIDToString(e.ws), util.UUIDToString(e.agent), source, target, reachFixedNow.AddDate(0, 0, 1), reachFixedNow.AddDate(0, 0, -2)).Scan(&intent); err != nil {
		t.Fatalf("insert effect intent: %v", err)
	}
	e.fx.Exec(t, `INSERT INTO attention_ledger
		(workspace_id, squad_id, subject_type, subject_id, reason, estimated_minutes, state, week_start, created_at)
		VALUES ($1, $2, 'effect_intent', $3, $4, 6, 'delivered', $5::date, $6)`,
		util.UUIDToString(e.ws), util.UUIDToString(e.squad), intent, reason, "2026-11-16", reachFixedNow.AddDate(0, 0, -2))
}

// snapshotRows returns the day's rows of the workspace, keyed by surface.
func (e reachEnv) snapshotRows(t *testing.T, day time.Time) map[string]db.ReachSnapshot {
	t.Helper()
	rows, err := e.q.ListReachSnapshotDay(context.Background(), db.ListReachSnapshotDayParams{
		WorkspaceID: e.ws, SnapshotDate: pgtype.Date{Time: reachDay(day), Valid: true},
	})
	if err != nil {
		t.Fatalf("list snapshot: %v", err)
	}
	out := make(map[string]db.ReachSnapshot, len(rows))
	for _, r := range rows {
		out[r.Surface] = r
	}
	return out
}

// Each state of the map comes out of the data that defines it (spec 11.5, 1).
func TestSnapshotStatesFromEffects(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	e.effect(t, "connected_app", "gmail_send", "allow", true)
	for i := 0; i < 3; i++ {
		e.effect(t, "connected_app", "slack_post", "confirm", true)
	}
	e.effect(t, "declared", "https://example.com/x", "allow", false)
	e.effect(t, "declared", "https://example.com/y", "allow", false)
	e.effect(t, "cli", "flow:flow_kv_pr:stage", "allow", false)
	e.ask(t, "connected_app", "jira_create", "missing_access")

	if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	rows := e.snapshotRows(t, reachFixedNow)
	want := map[string]string{
		"app:gmail":            "agent_operable",
		"app:slack":            "agent_with_confirm",
		"external:example.com": "declared_only",
		"area:flow:flow_kv_pr": "unknown",
		"app:jira":             "human_only",
	}
	if len(rows) != len(want) {
		t.Fatalf("surfaces = %d, want %d: %v", len(rows), len(want), rows)
	}
	for surface, state := range want {
		if got := rows[surface].State; got != state {
			t.Errorf("%s state = %q, want %q", surface, got, state)
		}
	}
	if got := rows["app:gmail"].AgentEffects30d; got != 1 {
		t.Errorf("app:gmail agent effects = %d, want 1", got)
	}
}

// A second run of the same day rewrites its rows and does not add new ones (spec 11.5, 3).
func TestSnapshotIdempotentPerDay(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	e.effect(t, "connected_app", "gmail_send", "allow", true)
	e.ask(t, "connected_app", "jira_create", "missing_access")

	for i := 0; i < 2; i++ {
		if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
			t.Fatalf("snapshot %d: %v", i, err)
		}
	}
	if n := len(e.snapshotRows(t, reachFixedNow)); n != 2 {
		t.Fatalf("rows after two runs = %d, want 2", n)
	}
	e.effect(t, "connected_app", "gmail_send", "allow", true)
	if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot after new effect: %v", err)
	}
	rows := e.snapshotRows(t, reachFixedNow)
	if len(rows) != 2 || rows["app:gmail"].Effects30d != 2 {
		t.Fatalf("rows = %d, app:gmail effects = %d; want 2 rows and 2 effects", len(rows), rows["app:gmail"].Effects30d)
	}
}

// Closing a gap opens one issue for the squad, with the criterion that the gap halves (spec 11.3).
func TestCloseGapCreatesGapIssueWithCriterion(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	e.ask(t, "connected_app", "jira_create", "missing_access")
	e.ask(t, "connected_app", "jira_create", "missing_info")
	if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot: %v", err)
	}

	issue, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:jira", e.squad)
	if err != nil {
		t.Fatalf("close gap: %v", err)
	}
	if issue.AssigneeType.String != "squad" || util.UUIDToString(issue.AssigneeID) != util.UUIDToString(e.squad) {
		t.Errorf("assignee = %s %s, want squad %s", issue.AssigneeType.String, util.UUIDToString(issue.AssigneeID), util.UUIDToString(e.squad))
	}
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM acceptance_criterion WHERE issue_id = $1 AND check_kind = 'manual' AND check_spec->>'baseline' = '2'`, util.UUIDToString(issue.ID)); n != 1 {
		t.Errorf("gap criteria with baseline 2 = %d, want 1", n)
	}
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM reach_snapshot WHERE workspace_id = $1 AND surface = 'app:jira' AND gap_issue_id = $2`,
		util.UUIDToString(e.ws), util.UUIDToString(issue.ID)); n != 1 {
		t.Errorf("snapshot rows that name the gap issue = %d, want 1", n)
	}
}

// A second close opens nothing new: it returns the issue already open for the surface.
func TestCloseGapTwiceOpensExisting(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	e.ask(t, "connected_app", "jira_create", "missing_access")
	if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	first, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:jira", e.squad)
	if err != nil {
		t.Fatalf("first close: %v", err)
	}
	second, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:jira", pgtype.UUID{})
	if err != nil {
		t.Fatalf("second close: %v", err)
	}
	if first.ID != second.ID {
		t.Errorf("second close opened %s, want the open issue %s", util.UUIDToString(second.ID), util.UUIDToString(first.ID))
	}
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM issue WHERE workspace_id = $1 AND title = 'Lấp khoảng trống: app:jira'`, util.UUIDToString(e.ws)); n != 1 {
		t.Errorf("gap issues = %d, want 1", n)
	}
}

// Without a snapshot row for the surface there is no gap to close, and a surface must name a kind.
func TestCloseGapRefusesUnknownSurface(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	if _, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:nothing", e.squad); err != ErrReachSurfaceUnknown {
		t.Errorf("unknown surface err = %v, want ErrReachSurfaceUnknown", err)
	}
	if _, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "gmail", e.squad); err != ErrReachSurfaceInvalid {
		t.Errorf("unnamed surface err = %v, want ErrReachSurfaceInvalid", err)
	}
}

// Snapshots older than 400 days go; the day exactly 400 days back stays (spec 11.2).
func TestReachRetention400Days(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	today := reachDay(reachFixedNow)
	var zero pgtype.Numeric
	if err := zero.Scan("0"); err != nil {
		t.Fatal(err)
	}
	for _, back := range []int{399, 400, 401} {
		day := pgtype.Date{Time: today.AddDate(0, 0, -back), Valid: true}
		if err := e.q.UpsertReachSnapshotRow(ctx, db.UpsertReachSnapshotRowParams{
			WorkspaceID: e.ws, SnapshotDate: day, Surface: "app:retention", State: "unknown",
			HumanMinutes30d: zero, TopAskIds: []pgtype.UUID{}, ComputedAt: pgtype.Timestamptz{Time: reachFixedNow, Valid: true},
		}); err != nil {
			t.Fatalf("seed %d days back: %v", back, err)
		}
	}
	if _, err := e.reach.Nightly(ctx); err != nil {
		t.Fatalf("nightly: %v", err)
	}
	for _, back := range []int{399, 400, 401} {
		n := e.fx.Count(t, `SELECT COUNT(*) FROM reach_snapshot WHERE workspace_id = $1 AND surface = 'app:retention' AND snapshot_date = $2::date`,
			util.UUIDToString(e.ws), today.AddDate(0, 0, -back).Format("2006-01-02"))
		want := 1
		if back == 401 {
			want = 0
		}
		if n != want {
			t.Errorf("%d days back: rows = %d, want %d", back, n, want)
		}
	}
}

// With durable_open off the nightly job, a snapshot and a gap close write nothing (spec 14.1).
func TestReachFlagOffWritesNothing(t *testing.T) {
	e := newReachEnv(t, false)
	ctx := context.Background()
	e.effect(t, "connected_app", "gmail_send", "allow", true)
	e.ask(t, "connected_app", "jira_create", "missing_access")
	if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, err := e.reach.Nightly(ctx); err != nil {
		t.Fatalf("nightly: %v", err)
	}
	if _, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:jira", e.squad); err != ErrReachOff {
		t.Errorf("close gap err = %v, want ErrReachOff", err)
	}
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM reach_snapshot WHERE workspace_id = $1`, util.UUIDToString(e.ws)); n != 0 {
		t.Errorf("snapshot rows with the flag off = %d, want 0", n)
	}
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM issue WHERE workspace_id = $1`, util.UUIDToString(e.ws)); n != 0 {
		t.Errorf("issues with the flag off = %d, want 0", n)
	}
}
