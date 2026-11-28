package service

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// effectService returns an EffectService over the fixture and the provider that
// decides durable_effects, so a test can turn the flag on or off.
func effectService(t *testing.T, f principalFixture, flagOn bool) (*EffectService, *featureflag.StaticProvider) {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: flagOn})
	svc := &EffectService{Queries: f.q, Flags: featureflag.NewService(provider)}
	return svc, provider
}

// effectFixture is a workspace with one agent, one task on it and the event rows
// the test wrote. The rows are removed when the test ends.
type effectFixture struct {
	principalFixture
	agentID string
	taskID  string
}

func newEffectFixture(t *testing.T, taskContext string) effectFixture {
	t.Helper()
	f, _ := companyWorkspace(t)
	f.Cleanup(t, `DELETE FROM effect_event WHERE workspace_id = $1`, f.WorkspaceID)
	runtimeID := f.Runtime(t, "effect runtime")
	agentID := f.Agent(t, "effect agent", runtimeID)
	cols := testutil.Cols{"runtime_id": runtimeID}
	if taskContext != "" {
		cols["context"] = testutil.Raw("'" + taskContext + "'::jsonb")
	}
	taskID := f.Task(t, agentID, cols)
	return effectFixture{principalFixture: f, agentID: agentID, taskID: taskID}
}

func (fx effectFixture) request(source, class, rev string) EffectInput {
	return EffectInput{
		WS:            util.MustParseUUID(fx.WorkspaceID),
		TaskID:        util.MustParseUUID(fx.taskID),
		AgentID:       util.MustParseUUID(fx.agentID),
		Source:        source,
		Target:        "test/target",
		Class:         class,
		Reversibility: rev,
		Summary:       "test action",
		Prior:         connectedapps.PolicyAllow,
	}
}

func (fx effectFixture) eventCount(t *testing.T, where string, args ...any) int {
	t.Helper()
	all := append([]any{fx.WorkspaceID}, args...)
	return fx.Count(t, `SELECT count(*) FROM effect_event WHERE workspace_id = $1 AND `+where, all...)
}

// TestEachSourceLogsOneEvent: every source in spec 4.5 except declared writes exactly
// one event per action, and a blocked action is logged too.
func TestEachSourceLogsOneEvent(t *testing.T) {
	sources := []string{
		connectedapps.SourceConnectedApp, connectedapps.SourcePluginAction, connectedapps.SourceCLI,
		connectedapps.SourceChannel, connectedapps.SourceWebhook, connectedapps.SourcePublishChannel,
	}
	for _, src := range sources {
		t.Run(src, func(t *testing.T) {
			fx := newEffectFixture(t, "")
			svc, _ := effectService(t, fx.principalFixture, true)
			ctx := context.Background()

			got, err := svc.Gate(ctx, fx.request(src, connectedapps.EffectWriteInternal, connectedapps.ReversibilityReversible))
			if err != nil {
				t.Fatalf("Gate allowed action: %v", err)
			}
			if got.Decision != connectedapps.PolicyAllow || !got.EventID.Valid {
				t.Fatalf("allowed action: decision %q, event valid %v", got.Decision, got.EventID.Valid)
			}

			// A shadow task may only read, so its send is blocked (not rehearsed).
			shadow := newEffectFixture(t, `{"durable_kind":"shadow"}`)
			svc2, _ := effectService(t, shadow.principalFixture, true)
			got2, err := svc2.Gate(ctx, shadow.request(src, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible))
			if err != nil {
				t.Fatalf("Gate blocked action: %v", err)
			}
			if got2.Decision != connectedapps.PolicyBlock {
				t.Fatalf("shadow send decision %q, want block", got2.Decision)
			}

			if n := fx.eventCount(t, `source = $2`, src); n != 1 {
				t.Fatalf("allowed action logged %d events for %s, want 1", n, src)
			}
			if n := shadow.eventCount(t, `source = $2 AND status = 'blocked'`, src); n != 1 {
				t.Fatalf("blocked action logged %d blocked events for %s, want 1", n, src)
			}
		})
	}
}

// TestShadowTaskGatewayWriteBlocked: a shadow task is blocked from every write, even
// at a level that would allow it (spec 4.3, D18).
func TestShadowTaskGatewayWriteBlocked(t *testing.T) {
	fx := newEffectFixture(t, `{"durable_kind":"shadow"}`)
	svc, _ := effectService(t, fx.principalFixture, true)
	ctx := context.Background()

	for _, class := range []string{connectedapps.EffectWrite, connectedapps.EffectWriteInternal, connectedapps.EffectSend} {
		rev := connectedapps.DefaultReversibility(class)
		got, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, class, rev))
		if err != nil {
			t.Fatalf("Gate %s: %v", class, err)
		}
		if got.Decision != connectedapps.PolicyBlock {
			t.Fatalf("shadow %s decision %q, want block", class, got.Decision)
		}
	}
	got, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, connectedapps.EffectRead, ""))
	if err != nil {
		t.Fatalf("Gate read: %v", err)
	}
	if got.Decision != connectedapps.PolicyAllow {
		t.Fatalf("shadow read decision %q, want allow", got.Decision)
	}
}

// TestGateFlagOffWritesNothing: while durable_effects is off the gate returns the
// caller's own decision and writes no event.
func TestGateFlagOffWritesNothing(t *testing.T) {
	fx := newEffectFixture(t, "")
	svc, _ := effectService(t, fx.principalFixture, false)

	got, err := svc.Gate(context.Background(), fx.request(connectedapps.SourceCLI, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible))
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	if got.Decision != connectedapps.PolicyAllow || got.EventID.Valid {
		t.Fatalf("flag off: decision %q, event valid %v; want the prior allow and no event", got.Decision, got.EventID.Valid)
	}
	if n := fx.eventCount(t, `true`); n != 0 {
		t.Fatalf("flag off wrote %d events", n)
	}
}

// TestGateRehearseAndFinish: a rehearsed send waits as awaiting. An allowed write
// goes awaiting, then Finish settles it once, with its undo, and a second Finish is
// refused.
func TestGateRehearseAndFinish(t *testing.T) {
	fx := newEffectFixture(t, "")
	svc, _ := effectService(t, fx.principalFixture, true)
	ctx := context.Background()

	// At L1 the agent has no squad, so a send rehearses.
	got, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible))
	if err != nil {
		t.Fatalf("Gate send: %v", err)
	}
	if got.Decision != connectedapps.PolicyRehearse {
		t.Fatalf("send at L1 decision %q, want rehearse", got.Decision)
	}
	if n := fx.eventCount(t, `status = 'awaiting' AND effect_class = 'send'`); n != 1 {
		t.Fatalf("rehearsed send: %d awaiting rows, want 1", n)
	}

	write, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, connectedapps.EffectWrite, connectedapps.ReversibilityReversible))
	if err != nil {
		t.Fatalf("Gate write: %v", err)
	}
	if write.Decision != connectedapps.PolicyAllow {
		t.Fatalf("reversible write at L1 decision %q, want allow", write.Decision)
	}
	undo := &connectedapps.UndoSpec{Tool: "GMAIL_DELETE_DRAFT", Args: map[string]any{"draft_id": "d-1"}}
	if err := svc.Finish(ctx, util.MustParseUUID(fx.WorkspaceID), write.EventID, "executed", undo); err != nil {
		t.Fatalf("Finish executed: %v", err)
	}
	if n := fx.eventCount(t, `id = $2 AND status = 'executed' AND undo IS NOT NULL AND undo_until > now()`, util.UUIDToString(write.EventID)); n != 1 {
		t.Fatalf("finished write: %d executed rows with undo, want 1", n)
	}
	if err := svc.Finish(ctx, util.MustParseUUID(fx.WorkspaceID), write.EventID, "failed", nil); err == nil {
		t.Fatal("second Finish: want an error for an event already settled")
	}
}

// TestEffectEventNoDelete: effect_event is append-only except for the two status moves
// (awaiting -> executed|blocked|failed and executed -> undone) and its undo/intent/result
// fields (durable spec 4.4, plan B2). No SQL query in pkg/db/queries may DELETE from
// effect_event, and an event already settled cannot be re-settled.
func TestEffectEventNoDelete(t *testing.T) {
	queries, err := filepath.Glob(filepath.Join("..", "..", "pkg", "db", "queries", "*.sql"))
	if err != nil || len(queries) == 0 {
		t.Fatalf("no query files found: %v", err)
	}
	commentRe := regexp.MustCompile(`(?m)--.*$`)
	deleteRe := regexp.MustCompile(`(?is)\bDELETE\s+FROM\s+effect_event\b`)
	updateRe := regexp.MustCompile(`(?is)\bUPDATE\s+effect_event\s+SET\s+(.*?)\s+(?:WHERE|RETURNING)\b`)
	assignRe := regexp.MustCompile(`(?is)([a-z_]+)\s*=`)
	allowed := map[string]bool{"status": true, "undo": true, "undo_until": true, "intent_id": true, "result": true}

	sawUpdate := false
	for _, path := range queries {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		src := commentRe.ReplaceAll(body, nil)
		if m := deleteRe.Find(src); m != nil {
			t.Errorf("%s deletes from effect_event (%q); effect_event is append-only (spec 4.4)", filepath.Base(path), m)
		}
		for _, m := range updateRe.FindAllSubmatch(src, -1) {
			sawUpdate = true
			for _, a := range assignRe.FindAllSubmatch(m[1], -1) {
				if col := string(a[1]); !allowed[col] {
					t.Errorf("%s updates effect_event.%s; only status, undo, undo_until, intent_id and result may change", filepath.Base(path), col)
				}
			}
		}
	}
	if !sawUpdate {
		t.Fatal("no UPDATE effect_event query found; check query glob")
	}

	// Verify the two allowed status transitions on a real row: awaiting -> executed -> undone.
	fx := newEffectFixture(t, "")
	svc, _ := effectService(t, fx.principalFixture, true)
	ctx := context.Background()
	ws := util.MustParseUUID(fx.WorkspaceID)

	rec, err := svc.Gate(ctx, fx.request(connectedapps.SourceConnectedApp, connectedapps.EffectWrite, connectedapps.ReversibilityReversible))
	if err != nil {
		t.Fatalf("Gate: %v", err)
	}
	if err := svc.Finish(ctx, ws, rec.EventID, "executed", nil); err != nil {
		t.Fatalf("Finish executed: %v", err)
	}
	if err := svc.Finish(ctx, ws, rec.EventID, "blocked", nil); err == nil {
		t.Fatal("Finish on executed event must fail")
	}
	claimed, err := fx.q.ClaimEffectEventUndo(ctx, db.ClaimEffectEventUndoParams{ID: rec.EventID, WorkspaceID: ws})
	if err != nil || claimed != 1 {
		t.Fatalf("ClaimEffectEventUndo = (%d, %v), want (1, nil)", claimed, err)
	}
	undone, err := fx.q.MarkEffectEventUndone(ctx, db.MarkEffectEventUndoneParams{ID: rec.EventID, WorkspaceID: ws})
	if err != nil || undone != 1 {
		t.Fatalf("MarkEffectEventUndone = (%d, %v), want (1, nil)", undone, err)
	}
	// Once undone, claiming undo again must affect 0 rows.
	again, err := fx.q.ClaimEffectEventUndo(ctx, db.ClaimEffectEventUndoParams{ID: rec.EventID, WorkspaceID: ws})
	if err != nil || again != 0 {
		t.Fatalf("ClaimEffectEventUndo after undone = (%d, %v), want (0, nil)", again, err)
	}
}

// gateReadBudget is the maximum SELECT count allowed for one warm Gate call:
// 1) runClass -> GetAgentTask (no autopilot_run_id, so RunClassOfTask issues no extra read),
// 2) companyState -> CompanyIsPaused -> GetWorkspaceCompanyState.
// autonomyLevel and overridesFor hit their 60s caches after warmup, sendsToday is
// skipped for non-send classes, and InsertEffectEvent is an INSERT.
const gateReadBudget = 2

type gateReadTrace struct {
	selects atomic.Int64
}

type gateReadTraceKey struct{}

// isSelectSQL strips leading sqlc comment lines ("-- name: ...") before matching SELECT.
func isSelectSQL(sql string) bool {
	s := sql
	for {
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, "--") {
			break
		}
		nl := strings.IndexByte(s, '\n')
		if nl < 0 {
			return false
		}
		s = s[nl+1:]
	}
	return len(s) >= 6 && strings.EqualFold(s[:6], "SELECT")
}

func (tr *gateReadTrace) TraceQueryStart(ctx context.Context, _ *pgx.Conn, data pgx.TraceQueryStartData) context.Context {
	return context.WithValue(ctx, gateReadTraceKey{}, isSelectSQL(data.SQL))
}

func (tr *gateReadTrace) TraceQueryEnd(ctx context.Context, _ *pgx.Conn, _ pgx.TraceQueryEndData) {
	if isSelect, _ := ctx.Value(gateReadTraceKey{}).(bool); isSelect {
		tr.selects.Add(1)
	}
}

// BenchmarkGate bounds the SELECT query count of a warm (*EffectService).Gate call
// so an accidental extra read fails the benchmark.
func BenchmarkGate(b *testing.B) {
	ctx := context.Background()
	shared, err := openSharedTestPool()
	if err != nil {
		b.Skip(err)
	}
	tr := &gateReadTrace{}
	config := shared.Config()
	config.ConnConfig.Tracer = tr
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		b.Fatal(err)
	}
	b.Cleanup(pool.Close)

	fx := testutil.New(pool, "", "")
	suffix := time.Now().UnixNano()
	fx.UserID = fx.User(b, "gate bench", fmt.Sprintf("gate-bench-%d@example.test", suffix))
	fx.WorkspaceID = fx.Workspace(b, "gate bench", fmt.Sprintf("gate-bench-%d", suffix))
	fx.Member(b, fx.WorkspaceID, fx.UserID, "owner")
	fx.Cleanup(b, `DELETE FROM effect_event WHERE workspace_id = $1`, fx.WorkspaceID)
	runtimeID := fx.Runtime(b, "gate bench")
	agentID := fx.Agent(b, "gate bench", runtimeID)
	squadID := fx.Squad(b, "gate bench", agentID)
	fx.SquadMember(b, squadID, "agent", agentID)
	taskID := fx.Task(b, agentID, testutil.Cols{"runtime_id": runtimeID})

	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.DurableEffects, featureflag.Rule{Default: true})
	svc := &EffectService{Queries: db.New(pool), Flags: featureflag.NewService(provider)}

	// Use a non-send class so sendsToday does not run and the read count stays fixed.
	in := EffectInput{
		WS:            util.MustParseUUID(fx.WorkspaceID),
		TaskID:        util.MustParseUUID(taskID),
		AgentID:       util.MustParseUUID(agentID),
		Source:        connectedapps.SourceConnectedApp,
		Target:        "bench/target",
		Class:         connectedapps.EffectWrite,
		Reversibility: connectedapps.ReversibilityReversible,
		Summary:       "benchmark gate action",
		Prior:         connectedapps.PolicyAllow,
	}

	// Warm the 60s autonomy and overrides caches; this call is not counted.
	if _, err := svc.Gate(ctx, in); err != nil {
		b.Fatalf("warmup Gate: %v", err)
	}

	tr.selects.Store(0)
	if _, err := svc.Gate(ctx, in); err != nil {
		b.Fatalf("measured Gate: %v", err)
	}
	reads := tr.selects.Load()
	if reads > gateReadBudget {
		b.Fatalf("Gate issued %d SELECT queries, exceeds budget %d", reads, gateReadBudget)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := svc.Gate(ctx, in); err != nil {
			b.Fatalf("Gate: %v", err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(reads), "reads/op")
}
