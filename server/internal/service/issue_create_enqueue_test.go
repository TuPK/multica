package service

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The client can give up after the issue commits. The issue is kept, so its
// assigned run must be queued anyway; before the fix the run's enqueue used the
// cancelled request context, and the issue was left with no run and no error.
func TestCreateAgentIssueQueuesRunWhenRequestCancelsAfterCommit(t *testing.T) {
	pool := newResolveOriginatorPool(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	q := db.New(pool)
	workspaceID, userID, agentID, _ := seedAttributionFixture(t, pool)
	workspaceUUID := util.MustParseUUID(workspaceID)
	userUUID := util.MustParseUUID(userID)
	agentUUID := util.MustParseUUID(agentID)

	bus := events.New()
	taskService := &TaskService{Queries: q, TxStarter: pool, Bus: bus}
	txStarter := &afterCommitTxStarter{pool: pool, afterCommit: cancel}
	issueService := NewIssueService(q, txStarter, bus, nil, taskService)

	result, err := issueService.Create(ctx, IssueCreateParams{
		WorkspaceID:  workspaceUUID,
		Title:        "enqueue after the request is cancelled",
		Status:       "todo",
		Priority:     "none",
		AssigneeType: pgtype.Text{String: "agent", Valid: true},
		AssigneeID:   agentUUID,
		CreatorType:  "member",
		CreatorID:    userUUID,
	}, IssueCreateOpts{})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if ctx.Err() == nil {
		t.Fatal("test setup: the request context was not cancelled after the commit")
	}
	if !result.AssignedTaskID.Valid {
		t.Fatal("the assigned run was not queued after the request was cancelled")
	}
	var queued int
	if err := pool.QueryRow(context.Background(),
		`SELECT count(*) FROM agent_task_queue WHERE id = $1 AND issue_id = $2`,
		result.AssignedTaskID, result.Issue.ID,
	).Scan(&queued); err != nil {
		t.Fatalf("count queued run: %v", err)
	}
	if queued != 1 {
		t.Fatalf("queued runs for the issue = %d, want 1", queued)
	}
}
