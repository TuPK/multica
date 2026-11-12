package service

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// cancelAfterTaskInsertDB cancels the request context as soon as the quick-create
// task row is inserted, the way a client that disconnects right after the insert
// would.
type cancelAfterTaskInsertDB struct {
	db.DBTX
	cancel context.CancelFunc
}

func (d cancelAfterTaskInsertDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := d.DBTX.QueryRow(ctx, sql, args...)
	if strings.Contains(sql, "INSERT INTO agent_task_queue") {
		return cancelAfterScanRow{Row: row, cancel: d.cancel}
	}
	return row
}

type cancelAfterScanRow struct {
	pgx.Row
	cancel context.CancelFunc
}

func (r cancelAfterScanRow) Scan(dest ...any) error {
	err := r.Row.Scan(dest...)
	r.cancel()
	return err
}

// The client can give up after the quick-create task is inserted. The task is
// kept, so the work that follows the insert (the member's delegation ledger and
// the task's queued analytics) must not depend on the request's context. Before
// the fix both used the cancelled request context, so the ledger row was lost
// without any error.
func TestEnqueueQuickCreateTaskRecordsDelegationWhenRequestCancelsAfterInsert(t *testing.T) {
	pool := newResolveOriginatorPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	workspaceID, userID, agentID, _ := seedAttributionFixture(t, pool)
	workspaceUUID := util.MustParseUUID(workspaceID)
	userUUID := util.MustParseUUID(userID)
	agentUUID := util.MustParseUUID(agentID)

	taskService := &TaskService{
		Queries:              db.New(cancelAfterTaskInsertDB{DBTX: pool, cancel: cancel}),
		TxStarter:            pool,
		Bus:                  events.New(),
		RecordMemberActivity: true,
	}

	task, err := taskService.EnqueueQuickCreateTask(ctx, workspaceUUID, userUUID, agentUUID,
		pgtype.UUID{}, "enqueue after the request is cancelled", "", "", pgtype.UUID{}, pgtype.UUID{}, nil)
	if err != nil {
		t.Fatalf("EnqueueQuickCreateTask: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("test setup: the request context was not cancelled after the insert")
	}

	var queued int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_task_queue WHERE id = $1 AND agent_id = $2`,
		task.ID, agentUUID,
	).Scan(&queued); err != nil {
		t.Fatalf("count queued task: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued quick-create tasks = %d, want 1", queued)
	}

	var delegationDays int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM member_delegation_day WHERE user_id = $1`, userUUID,
	).Scan(&delegationDays); err != nil {
		t.Fatalf("count delegation days: %v", err)
	}
	if delegationDays != 1 {
		t.Fatalf("member_delegation_day rows = %d, want 1 (the delegation was dropped with the request context)", delegationDays)
	}
}
