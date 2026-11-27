package service

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// setProposalActorSession attributes proposal issue status changes to the
// deciding member. Standing-policy decisions have no member actor and stay system.
func setProposalActorSession(ctx context.Context, qtx *db.Queries, p ProposalDecideParams) error {
	if p.Policy != nil || !p.DeciderID.Valid {
		return nil
	}
	return WithActorSession(ctx, qtx, GovernanceActor{Type: GovernanceActorMember, ID: p.DeciderID}, pgtype.UUID{})
}

// resumeWaitingIssueStatus moves a blocked issue back to its start status,
// attributing the transition to the deciding member when a member decided.
func (s *ProposalService) resumeWaitingIssueStatus(ctx context.Context, p ProposalDecideParams, issueID pgtype.UUID, status string) (db.Issue, error) {
	if p.Policy != nil || !p.DeciderID.Valid {
		return s.Issues.Queries.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
			ID: issueID, WorkspaceID: p.WorkspaceID, Status: status,
		})
	}
	tx, err := s.Issues.TxStarter.Begin(ctx)
	if err != nil {
		return db.Issue{}, err
	}
	defer tx.Rollback(ctx)
	qtx := s.Issues.Queries.WithTx(tx)
	if err := setProposalActorSession(ctx, qtx, p); err != nil {
		return db.Issue{}, err
	}
	moved, err := qtx.UpdateIssueStatus(ctx, db.UpdateIssueStatusParams{
		ID: issueID, WorkspaceID: p.WorkspaceID, Status: status,
	})
	if err != nil {
		return db.Issue{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.Issue{}, err
	}
	return moved, nil
}
