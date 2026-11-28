package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
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

// flagsOpenAndOutcomes turns on the Reach Map and the outcome writes a gap criterion needs.
func flagsOpenAndOutcomes() *featureflag.Service {
	p := featureflag.NewStaticProvider()
	p.Set(featureflags.DurableOpen, featureflag.Rule{Default: true})
	p.Set(featureflags.DurableOutcomes, featureflag.Rule{Default: true})
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
			Verification: &VerificationService{Queries: e.q, TxStarter: pool, Flags: flagsOpenAndOutcomes()},
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

func TestSnapshotDeclaredAgentEffect(t *testing.T) {
	e := newReachEnv(t, true)
	e.effect(t, "declared", "https://example.com/x", "allow", true)
	if err := e.reach.Snapshot(context.Background(), e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	row := e.snapshotRows(t, reachFixedNow)["external:example.com"]
	if row.State != "declared_only" {
		t.Errorf("state = %q, want declared_only", row.State)
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

// Closing a gap opens one issue for the squad, with a metric criterion that the gap halves
// within 30 days (spec 11.3).
func TestCloseGapCreatesIssueWithMetricCriterion(t *testing.T) {
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
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM acceptance_criterion
		WHERE issue_id = $1 AND check_kind = 'metric'
		  AND check_spec->>'metric_key' = 'reach.gap_asks_30d'
		  AND check_spec->>'op' = '<='
		  AND check_spec->>'value' = '1'
		  AND check_spec->>'within_days' = '30'
		  AND check_spec->>'scope' = 'company'
		  AND check_spec->>'surface' = 'app:jira'
		  AND check_spec->>'baseline' = '2'`, util.UUIDToString(issue.ID)); n != 1 {
		t.Errorf("gap metric criteria with baseline 2 halving to 1 = %d, want 1", n)
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

// Q4: ReachAskCounts maps non-effect_intent attention asks (app_action_request, proposal, inquiry)
// onto their surfaces alongside effect_intent.
func TestSnapshotIncludesAllAttentionAskTypes(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	askedAt := reachFixedNow.AddDate(0, 0, -2)

	// 1. app_action_request ask (missing_access on toolkit "linear") -> app:linear human_only.
	reqID := dbid.NewV7()
	e.fx.Exec(t, `INSERT INTO app_action_request
		(id, workspace_id, kind, status, task_id, agent_id, approver_user_id, account_scope, toolkit_slug, tool_slug, action_class, expires_at)
		VALUES ($1, $2, 'connect', 'pending', $3, $4, $5, 'team', 'linear', '', 'write', $6)`,
		util.UUIDToString(reqID), util.UUIDToString(e.ws), util.UUIDToString(dbid.NewV7()),
		util.UUIDToString(e.agent), util.UUIDToString(e.owner), reachFixedNow.AddDate(0, 0, 1))
	e.fx.Exec(t, `INSERT INTO attention_ledger
		(workspace_id, squad_id, subject_type, subject_id, reason, estimated_minutes, state, week_start, created_at)
		VALUES ($1, $2, 'app_action_request', $3, 'missing_access', 5, 'delivered', '2026-11-16'::date, $4)`,
		util.UUIDToString(e.ws), util.UUIDToString(e.squad), util.UUIDToString(reqID), askedAt)

	// 2. proposal ask (missing_info) -> area:squad:<squad_id>.
	prop := e.file(e.squad, "", "{}", false).Proposal
	e.fx.Exec(t, `INSERT INTO attention_ledger
		(workspace_id, squad_id, subject_type, subject_id, reason, estimated_minutes, state, week_start, created_at)
		VALUES ($1, $2, 'proposal', $3, 'missing_info', 4, 'delivered', '2026-11-16'::date, $4)`,
		util.UUIDToString(e.ws), util.UUIDToString(e.squad), util.UUIDToString(prop.ID), askedAt)

	// 3. inquiry ask (missing_info) -> area:squad:<squad_id>.
	inqID := dbid.NewV7()
	e.fx.Exec(t, `INSERT INTO brief_inquiry
		(id, workspace_id, asked_by_agent_id, squad_id, question, asked_on, created_at)
		VALUES ($1, $2, $3, $4, 'Which pricing tier?', '2026-11-18'::date, $5)`,
		util.UUIDToString(inqID), util.UUIDToString(e.ws), util.UUIDToString(e.agent), util.UUIDToString(e.squad), askedAt)
	e.fx.Exec(t, `INSERT INTO attention_ledger
		(workspace_id, squad_id, subject_type, subject_id, reason, estimated_minutes, state, week_start, created_at)
		VALUES ($1, $2, 'inquiry', $3, 'missing_info', 3, 'delivered', '2026-11-16'::date, $4)`,
		util.UUIDToString(e.ws), util.UUIDToString(e.squad), util.UUIDToString(inqID), askedAt)

	if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	rows := e.snapshotRows(t, reachFixedNow)
	if got := rows["app:linear"]; got.State != "human_only" || got.GapAsks30d != 1 {
		t.Errorf("app:linear = (state=%q, gap_asks=%d), want (human_only, 1)", got.State, got.GapAsks30d)
	}
	squadSurface := "area:squad:" + util.UUIDToString(e.squad)
	if got := rows[squadSurface]; got.GapAsks30d != 2 {
		t.Errorf("%s gap_asks_30d = %d, want 2 (proposal + inquiry)", squadSurface, got.GapAsks30d)
	}
}

// Q3: reach.gap_asks_30d reads from reach_snapshot (not metric_reading) and keeps the outcome
// open across the 30-day metric window rather than settling prematurely after 7 days.
func TestReachGapMetricReadsReachSnapshotWith30DayWindow(t *testing.T) {
	e := newReachEnv(t, true)
	ctx := context.Background()
	for i := 0; i < 4; i++ {
		e.ask(t, "connected_app", "jira_create", "missing_access")
	}
	if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	issue, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:jira", e.squad)
	if err != nil {
		t.Fatalf("close gap: %v", err)
	}

	// Move gap issue to done and create its work_outcome.
	e.fx.Exec(t, `UPDATE issue SET status = 'done' WHERE id = $1`, util.UUIDToString(issue.ID))
	outcomes := &OutcomeService{
		Queries:      e.q,
		TxStarter:    e.fx.Pool,
		Flags:        flagsOpenAndOutcomes(),
		Verification: e.reach.Verification,
	}
	if _, err := outcomes.ProcessTransitions(ctx); err != nil {
		t.Fatalf("process transitions: %v", err)
	}
	outcome, err := e.q.GetCurrentWorkOutcomeForIssue(ctx, db.GetCurrentWorkOutcomeForIssueParams{
		WorkspaceID: e.ws, IssueID: issue.ID,
	})
	if err != nil {
		t.Fatalf("load gap outcome: %v", err)
	}
	// After 10 days (> default 7-day settle window, < 30-day metric window), outcome must stay open.
	if _, err := outcomes.Settle(ctx, time.Now().Add(10*24*time.Hour)); err != nil {
		t.Fatalf("settle at day 10: %v", err)
	}
	reloaded, err := e.q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: outcome.ID, WorkspaceID: e.ws})
	if err != nil {
		t.Fatalf("reload gap outcome: %v", err)
	}
	if reloaded.SettledAt.Valid {
		t.Fatalf("gap outcome settled prematurely at day 10 (settle_at=%v)", reloaded.SettleAt.Time)
	}

	// Write a newer reach_snapshot row with gap_asks_30d = 1 (<= baseline/2 = 2) and no metric_reading.
	var zero pgtype.Numeric
	_ = zero.Scan("0")
	day15 := time.Now().Add(15 * 24 * time.Hour)
	if err := e.q.UpsertReachSnapshotRow(ctx, db.UpsertReachSnapshotRowParams{
		WorkspaceID: e.ws, SnapshotDate: pgtype.Date{Time: reachDay(day15), Valid: true},
		Surface: "app:jira", State: "agent_with_confirm", GapAsks30d: 1,
		HumanMinutes30d: zero, TopAskIds: []pgtype.UUID{},
		ComputedAt: pgtype.Timestamptz{Time: day15, Valid: true},
	}); err != nil {
		t.Fatalf("upsert day 15 snapshot: %v", err)
	}
	if _, err := e.reach.Verification.CheckMetrics(ctx, day15); err != nil {
		t.Fatalf("CheckMetrics: %v", err)
	}
	if n := e.fx.Count(t, `SELECT COUNT(*) FROM outcome_verification
		WHERE outcome_id = $1 AND verifier_type = 'metric' AND result = 'pass'`, util.UUIDToString(outcome.ID)); n != 1 {
		t.Fatalf("passed reach metric verifications = %d, want 1", n)
	}
}

// Q5: CloseGap is atomic (concurrent calls create one issue; failures roll back without orphan issues).
func TestCloseGapAtomicAndConcurrent(t *testing.T) {
	t.Run("concurrent close gap creates exactly one issue", func(t *testing.T) {
		e := newReachEnv(t, true)
		ctx := context.Background()
		e.ask(t, "connected_app", "jira_create", "missing_access")
		if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
			t.Fatalf("snapshot: %v", err)
		}

		const workers = 5
		var wg sync.WaitGroup
		ids := make([]string, workers)
		errs := make([]error, workers)
		start := make(chan struct{})
		for i := 0; i < workers; i++ {
			wg.Add(1)
			go func(idx int) {
				defer wg.Done()
				<-start
				iss, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:jira", e.squad)
				if err != nil {
					errs[idx] = err
					return
				}
				ids[idx] = util.UUIDToString(iss.ID)
			}(i)
		}
		close(start)
		wg.Wait()

		first := ""
		for i := 0; i < workers; i++ {
			if errs[i] != nil {
				t.Fatalf("worker %d failed: %v", i, errs[i])
			}
			if first == "" {
				first = ids[i]
			} else if ids[i] != first {
				t.Fatalf("worker %d returned issue %s, want %s", i, ids[i], first)
			}
		}
		if n := e.fx.Count(t, `SELECT COUNT(*) FROM issue WHERE workspace_id = $1 AND title = 'Lấp khoảng trống: app:jira'`, util.UUIDToString(e.ws)); n != 1 {
			t.Fatalf("gap issues created = %d, want 1", n)
		}
	})

	t.Run("criterion failure leaves no orphan issue", func(t *testing.T) {
		e := newReachEnv(t, true)
		ctx := context.Background()
		e.ask(t, "connected_app", "jira_create", "missing_access")
		if err := e.reach.Snapshot(ctx, e.ws, reachFixedNow); err != nil {
			t.Fatalf("snapshot: %v", err)
		}
		// Turn durable_outcomes off so adding the metric criterion fails.
		e.reach.Verification.Flags = flagsOpen(true)
		if _, err := e.reach.CloseGap(ctx, e.ws, e.boss(), "app:jira", e.squad); !errors.Is(err, ErrOutcomesOff) {
			t.Fatalf("CloseGap err = %v, want ErrOutcomesOff", err)
		}
		if n := e.fx.Count(t, `SELECT COUNT(*) FROM issue WHERE workspace_id = $1 AND title = 'Lấp khoảng trống: app:jira'`, util.UUIDToString(e.ws)); n != 0 {
			t.Fatalf("orphan gap issues = %d, want 0", n)
		}
	})
}
