package service

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
)

// Database tests of the workspace claim gate. They are written against the real
// claim path and the real gate adapter (no Redis, so a state change shows at
// once). They run only when PLATFORM_TEST_DATABASE_URL names the same database
// as DATABASE_URL, the convention of the platform tests.

func requirePlatformTestDB(t *testing.T) {
	t.Helper()
	dsn := os.Getenv("PLATFORM_TEST_DATABASE_URL")
	if dsn == "" || dsn != os.Getenv("DATABASE_URL") {
		t.Skip("PLATFORM_TEST_DATABASE_URL not set or differs from DATABASE_URL")
	}
}

// newGatedFixture is the budget fixture with the workspace gate on its task service.
func newGatedFixture(t *testing.T) budgetFixture {
	t.Helper()
	requirePlatformTestDB(t)
	f := newBudgetFixture(t)
	f.tasks.WorkspaceGate = workspaceguard.NewClaimGate(workspaceguard.New(f.q, nil))
	t.Cleanup(func() {
		f.Pool.Exec(context.Background(), `DELETE FROM platform_workspace WHERE workspace_id = $1`, f.wsText)
	})
	return f
}

func (f budgetFixture) setWorkspaceState(t *testing.T, status string) {
	t.Helper()
	f.Exec(t, `
		INSERT INTO platform_workspace (workspace_id, status) VALUES ($1, $2)
		ON CONFLICT (workspace_id) DO UPDATE SET status = EXCLUDED.status`, f.wsText, status)
}

// claimKindCols are the three run kinds the claim classifies. Every task is
// bound to the fixture runtime so the runtime-scoped entry points see it.
func (f budgetFixture) claimKindCols() map[string]testutil.Cols {
	return map[string]testutil.Cols{
		"human":     {"runtime_id": f.rt, "originator_source": "direct_human"},
		"routine":   {"runtime_id": f.rt},
		"delegated": {"runtime_id": f.rt, "originator_source": "delegation"},
	}
}

func TestClaimTaskIsRefusedInANonActiveWorkspaceForEveryRunKind(t *testing.T) {
	ctx := context.Background()
	for _, status := range []string{"read_only", "suspended", "pending_deletion"} {
		for _, kind := range []string{"human", "routine", "delegated"} {
			t.Run(status+"/"+kind, func(t *testing.T) {
				f := newGatedFixture(t)
				task := f.queued(t, f.claimKindCols()[kind])
				f.setWorkspaceState(t, status)

				got, err := f.tasks.ClaimTask(ctx, util.MustParseUUID(f.agent))
				if err != nil || got != nil {
					t.Fatalf("claim in a %s workspace: task %v err %v; want no task", status, got, err)
				}
				if st := f.taskStatus(t, task); st != "queued" {
					t.Fatalf("the task is %q, want queued", st)
				}
				if n := f.holdsFor(t, task); n != 0 {
					t.Fatalf("a workspace refusal wrote %d budget holds, want none", n)
				}

				// Set back to active: the same task is claimable at once.
				f.setWorkspaceState(t, "active")
				got, err = f.tasks.ClaimTask(ctx, util.MustParseUUID(f.agent))
				if err != nil || got == nil || util.UUIDToString(got.ID) != task {
					t.Fatalf("after the workspace is active again: task %v err %v; want %s", got, err, task)
				}
			})
		}
	}
}

func TestRuntimeClaimPathsRefuseANonActiveWorkspace(t *testing.T) {
	ctx := context.Background()
	f := newGatedFixture(t)
	task := f.queued(t, f.claimKindCols()["routine"])
	f.setWorkspaceState(t, "suspended")

	if got, err := f.tasks.ClaimTaskForRuntime(ctx, util.MustParseUUID(f.rt)); err != nil || got != nil {
		t.Fatalf("ClaimTaskForRuntime: task %v err %v; want no task", got, err)
	}
	if got, err := f.tasks.ClaimTasksForRuntimes(ctx, []pgtype.UUID{util.MustParseUUID(f.rt)}, 5); err != nil || len(got) != 0 {
		t.Fatalf("ClaimTasksForRuntimes: %d tasks err %v; want none", len(got), err)
	}
	if st := f.taskStatus(t, task); st != "queued" {
		t.Fatalf("the task is %q, want queued", st)
	}

	f.setWorkspaceState(t, "active")
	got, err := f.tasks.ClaimTasksForRuntimes(ctx, []pgtype.UUID{util.MustParseUUID(f.rt)}, 5)
	if err != nil || len(got) != 1 || util.UUIDToString(got[0].ID) != task {
		t.Fatalf("after the workspace is active again: %d tasks err %v; want %s", len(got), err, task)
	}
}

// Running tasks finish: completion does not pass the claim gate, so a task that
// started before the workspace was suspended can still report its result.
func TestARunningTaskCompletesInANonActiveWorkspace(t *testing.T) {
	ctx := context.Background()
	f := newGatedFixture(t)
	task := f.queued(t, testutil.Cols{
		"runtime_id": f.rt, "status": "running",
		"dispatched_at": testutil.Raw("now()"), "started_at": testutil.Raw("now()"),
	})
	f.setWorkspaceState(t, "suspended")

	if _, err := f.tasks.CompleteTask(ctx, util.MustParseUUID(task), []byte(`{"ok":true}`), "", "", "", false, "", ""); err != nil {
		t.Fatalf("complete a running task in a suspended workspace: %v", err)
	}
	if st := f.taskStatus(t, task); st != "completed" {
		t.Fatalf("the task is %q, want completed", st)
	}
}

// A task service with no gate claims exactly as before, whatever the state row says.
func TestNilGateClaimsAsBefore(t *testing.T) {
	ctx := context.Background()
	requirePlatformTestDB(t)
	f := newBudgetFixture(t)
	t.Cleanup(func() {
		f.Pool.Exec(context.Background(), `DELETE FROM platform_workspace WHERE workspace_id = $1`, f.wsText)
	})
	if f.tasks.WorkspaceGate != nil {
		t.Fatal("NewTaskService set a gate")
	}
	task := f.queued(t, f.claimKindCols()["routine"])
	f.setWorkspaceState(t, "suspended")

	got, err := f.tasks.ClaimTask(ctx, util.MustParseUUID(f.agent))
	if err != nil || got == nil || util.UUIDToString(got.ID) != task {
		t.Fatalf("a nil gate must claim: task %v err %v", got, err)
	}
}
