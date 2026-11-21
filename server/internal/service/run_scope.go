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
// person, or whose agent's owner, cannot see the project of the task's issue.
// The task stays queued, as for the other refusals (D25).
const VerdictReasonProjectNotVisible = "project_not_visible"

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

// runRefusedForVisibility reports whether a queued run must wait because the
// person who started it, or its agent's owner, cannot see the project of the
// task's issue. The person is the task's originator and, for an autopilot run
// without one, the autopilot's creator; a run with no person sees no private
// project. A task without an issue, or an issue without a project, has nothing
// to hide and is not checked.
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
	person := task.OriginatorUserID
	if !person.Valid && task.AutopilotRunID.Valid {
		if person, err = autopilotCreatorOfRun(ctx, q, ws, task.AutopilotRunID); err != nil {
			return false, err
		}
	}
	principals := []pgtype.UUID{person}
	if agent, err := q.GetAgent(ctx, task.AgentID); err == nil {
		principals = append(principals, agent.OwnerID)
	}
	hidden, err := projectaccess.ForPrincipals(ctx, q, ws, principals...)
	if err != nil {
		return false, fmt.Errorf("compute run access: %w", err)
	}
	return !hidden.CanSeeProject(issue.ProjectID), nil
}
