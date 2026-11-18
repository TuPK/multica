package service

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/projectaccess"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// VerdictReasonProjectNotVisible is the claim refusal for a run whose acting
// person cannot see the project of the task's issue (spec 7.1 rule 12, D55).
// The task stays queued, as for the other refusals (D25).
const VerdictReasonProjectNotVisible = "project_not_visible"

// RunScope is what a run may see in workspace ws: the Scope of its acting
// person, computed by projectaccess.ForViewer like every other reader.
//
// The acting person is agent_task_queue.originator_user_id. When that is NULL
// and the task belongs to an autopilot run, it is the autopilot's creator
// (O-R4-3). Any other NULL-originator task has no acting person and sees no
// restricted project. A person who is no longer a workspace member yields
// projectaccess.ErrNotMember; the caller treats that as seeing nothing.
//
// RunScope only reads. It never writes the creator into originator_user_id,
// which decides the connected-app overlay (migration 129).
func RunScope(ctx context.Context, q *db.Queries, ws pgtype.UUID, task db.AgentTaskQueue) (projectaccess.Scope, error) {
	person := task.OriginatorUserID
	if !person.Valid && task.AutopilotRunID.Valid {
		creator, err := autopilotCreatorOfRun(ctx, q, ws, task.AutopilotRunID)
		if err != nil {
			return projectaccess.Scope{}, err
		}
		person = creator
	}
	if !person.Valid {
		return projectaccess.ForViewer(ctx, noPersonQuerier{q}, ws, person)
	}
	return projectaccess.ForViewer(ctx, q, ws, person)
}

// autopilotCreatorOfRun returns the member who created the run's autopilot, or
// an invalid UUID when the run, its autopilot or a member creator is missing
// (an agent creator is no person).
func autopilotCreatorOfRun(ctx context.Context, q *db.Queries, ws, runID pgtype.UUID) (pgtype.UUID, error) {
	run, err := q.GetAutopilotRun(ctx, runID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("load autopilot run for scope: %w", err)
	}
	ap, err := q.GetAutopilot(ctx, run.AutopilotID)
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("load autopilot for scope: %w", err)
	}
	if ap.WorkspaceID != ws || ap.CreatedByType != "member" {
		return pgtype.UUID{}, nil
	}
	return ap.CreatedByID, nil
}

// noPersonQuerier answers ForViewer for a run without an acting person: a plain
// member who holds no grant. The Scope it yields is the one ForViewer computes
// for any ungranted member, so no visibility rule is written here.
type noPersonQuerier struct{ q *db.Queries }

func (n noPersonQuerier) GetViewerMember(context.Context, db.GetViewerMemberParams) (db.GetViewerMemberRow, error) {
	return db.GetViewerMemberRow{Role: "member"}, nil
}

func (n noPersonQuerier) ListViewerProjectAccess(ctx context.Context, arg db.ListViewerProjectAccessParams) ([]db.ListViewerProjectAccessRow, error) {
	return n.q.ListViewerProjectAccess(ctx, arg)
}

// runRefusedForVisibility reports whether the run's acting person cannot see
// the project of the task's issue. A task without an issue, or an issue
// without a project, has nothing to hide and is not checked (no query is run
// for the first).
func runRefusedForVisibility(ctx context.Context, q *db.Queries, ws pgtype.UUID, task db.AgentTaskQueue) (bool, error) {
	if !task.IssueID.Valid {
		return false, nil
	}
	issue, err := q.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: task.IssueID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load issue for run visibility: %w", err)
	}
	if !issue.ProjectID.Valid {
		return false, nil
	}
	scope, err := RunScope(ctx, q, ws, task)
	if errors.Is(err, projectaccess.ErrNotMember) {
		return true, nil
	}
	if err != nil {
		return false, fmt.Errorf("compute run scope: %w", err)
	}
	return !scope.Allows(issue.ProjectID), nil
}
