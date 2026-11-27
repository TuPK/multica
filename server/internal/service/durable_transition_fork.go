package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// builtinLifecycleCategory maps the seven built-in status keys to their lifecycle
// category, as migration 469 assigns them. It applies only when issue_status has
// no row for the key: a workspace created before migration 339, or one with no
// seed row. Catalog rows always win (D32).
var builtinLifecycleCategory = map[string]string{
	"backlog":     "unstarted",
	"todo":        "unstarted",
	"in_progress": "started",
	"in_review":   "started",
	"blocked":     "started",
	"done":        "done",
	"cancelled":   "closed",
}

// LifecycleCategoryOf returns the lifecycle category of a status key. A catalog
// row decides; without one a built-in key maps by table; any other key is
// unstarted, the same default the trigger path used before categories moved to
// read time.
func LifecycleCategoryOf(catalogCategory pgtype.Text, key string) string {
	if catalogCategory.Valid {
		return catalogCategory.String
	}
	if c, ok := builtinLifecycleCategory[key]; ok {
		return c
	}
	return "unstarted"
}

// IssueStatusTransitionRecord is one row of issue_status_transition with its
// categories resolved for reading. FromStatusKey and FromCategory are empty when
// the row has no "from" status.
type IssueStatusTransitionRecord struct {
	ID            pgtype.UUID
	FromStatusKey string
	ToStatusKey   string
	FromCategory  string
	ToCategory    string
	ActorType     string
	ActorID       pgtype.UUID
	TaskID        pgtype.UUID
	Source        string
	CreatedAt     pgtype.Timestamptz
}

// ListIssueStatusTransitionRecords returns the transitions of one issue, oldest
// first, with lifecycle categories resolved.
func ListIssueStatusTransitionRecords(ctx context.Context, q *db.Queries, workspaceID, issueID pgtype.UUID) ([]IssueStatusTransitionRecord, error) {
	rows, err := q.ListIssueStatusTransitions(ctx, db.ListIssueStatusTransitionsParams{
		WorkspaceID: workspaceID,
		IssueID:     issueID,
	})
	if err != nil {
		return nil, fmt.Errorf("list issue status transitions: %w", err)
	}
	out := make([]IssueStatusTransitionRecord, 0, len(rows))
	for _, r := range rows {
		rec := IssueStatusTransitionRecord{
			ID:          r.ID,
			ToStatusKey: r.ToStatusKey,
			ToCategory:  LifecycleCategoryOf(r.ToCatalogCategory, r.ToStatusKey),
			ActorType:   r.ActorType,
			ActorID:     r.ActorID,
			TaskID:      r.TaskID,
			Source:      r.Source,
			CreatedAt:   r.CreatedAt,
		}
		if r.FromStatusKey.Valid {
			rec.FromStatusKey = r.FromStatusKey.String
			rec.FromCategory = LifecycleCategoryOf(r.FromCatalogCategory, r.FromStatusKey.String)
		}
		out = append(out, rec)
	}
	return out, nil
}

// IssueEnteredDone is the transition event emitted when an issue moves into
// the done lifecycle category from a non-done category (durable spec 2.2, A1).
type IssueEnteredDone struct {
	Transition IssueStatusTransitionRecord
}

// IssueLeftDone is the transition event emitted when an issue leaves the done
// lifecycle category. Reopened is true for moves back to an active/unstarted
// category and false for moves into closed (cancelled, spec 2.2).
type IssueLeftDone struct {
	Transition IssueStatusTransitionRecord
	Reopened   bool
}

// ClassifyIssueTransition classifies one resolved transition record into an
// IssueEnteredDone or IssueLeftDone event, or (nil, nil) if the transition
// neither enters nor leaves the done lifecycle category.
func ClassifyIssueTransition(rec IssueStatusTransitionRecord) (*IssueEnteredDone, *IssueLeftDone) {
	switch {
	case rec.ToCategory == "done" && rec.FromCategory != "done":
		return &IssueEnteredDone{Transition: rec}, nil
	case rec.FromCategory == "done" && rec.ToCategory != "done":
		return nil, &IssueLeftDone{
			Transition: rec,
			Reopened:   rec.ToCategory != "closed",
		}
	default:
		return nil, nil
	}
}
