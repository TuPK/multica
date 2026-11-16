package workspaceexport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// pageSize is how many rows one read fetches.
const pageSize = 500

// jsonColumns are jsonb columns the queries cast to text; the source parses
// them back so the archive holds JSON values.
var jsonColumns = map[string]bool{"acceptance_criteria": true, "properties": true}

// DBSource reads the rows of a workspace through the queries in
// workspace_export.sql. Each query selects only allowlisted columns and filters
// by the workspace id.
type DBSource struct{ q *db.Queries }

// NewDBSource returns the database source.
func NewDBSource(q *db.Queries) *DBSource { return &DBSource{q: q} }

// toRow turns a sqlc row into a Row through its JSON tags.
func toRow(v any) (Row, error) {
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var row Row
	if err := dec.Decode(&row); err != nil {
		return nil, err
	}
	for col := range jsonColumns {
		s, ok := row[col].(string)
		if !ok {
			continue
		}
		var parsed any
		if err := json.Unmarshal([]byte(s), &parsed); err == nil {
			row[col] = parsed
		}
	}
	return row, nil
}

// page walks a keyset-paged query: fetch returns one page after the given id
// and the id of its last row, and the walk stops at a short page.
func page[T any](after pgtype.UUID, fetch func(after pgtype.UUID) ([]T, error), id func(T) pgtype.UUID, emit func(Row) error) error {
	for {
		rows, err := fetch(after)
		if err != nil {
			return err
		}
		for _, r := range rows {
			row, err := toRow(r)
			if err != nil {
				return err
			}
			if err := emit(row); err != nil {
				return err
			}
		}
		if len(rows) < pageSize {
			return nil
		}
		after = id(rows[len(rows)-1])
	}
}

// Rows implements Source.
func (s *DBSource) Rows(ctx context.Context, table, workspaceID string, emit func(Row) error) error {
	ws, err := util.ParseUUID(workspaceID)
	if err != nil {
		return fmt.Errorf("parse workspace id: %w", err)
	}
	start := pgtype.UUID{Valid: true} // the nil uuid sorts before every id
	limit := int32(pageSize)

	switch table {
	case TableWorkspace:
		r, err := s.q.GetWorkspaceForExport(ctx, ws)
		if err != nil {
			return err
		}
		row, err := toRow(r)
		if err != nil {
			return err
		}
		return emit(row)
	case TableMembers:
		return page(start, func(a pgtype.UUID) ([]db.ListMembersForExportRow, error) {
			return s.q.ListMembersForExport(ctx, db.ListMembersForExportParams{WorkspaceID: ws, After: a, RowLimit: limit})
		}, func(r db.ListMembersForExportRow) pgtype.UUID { return r.ID }, emit)
	case TableProjects:
		return page(start, func(a pgtype.UUID) ([]db.ListProjectsForExportRow, error) {
			return s.q.ListProjectsForExport(ctx, db.ListProjectsForExportParams{WorkspaceID: ws, After: a, RowLimit: limit})
		}, func(r db.ListProjectsForExportRow) pgtype.UUID { return r.ID }, emit)
	case TableIssues:
		return page(start, func(a pgtype.UUID) ([]db.ListIssuesForExportRow, error) {
			return s.q.ListIssuesForExport(ctx, db.ListIssuesForExportParams{WorkspaceID: ws, After: a, RowLimit: limit})
		}, func(r db.ListIssuesForExportRow) pgtype.UUID { return r.ID }, emit)
	case TableComments:
		return page(start, func(a pgtype.UUID) ([]db.ListCommentsForExportRow, error) {
			return s.q.ListCommentsForExport(ctx, db.ListCommentsForExportParams{WorkspaceID: ws, After: a, RowLimit: limit})
		}, func(r db.ListCommentsForExportRow) pgtype.UUID { return r.ID }, emit)
	case TableLabels:
		return page(start, func(a pgtype.UUID) ([]db.ListLabelsForExportRow, error) {
			return s.q.ListLabelsForExport(ctx, db.ListLabelsForExportParams{WorkspaceID: ws, After: a, RowLimit: limit})
		}, func(r db.ListLabelsForExportRow) pgtype.UUID { return r.ID }, emit)
	case TableStatuses:
		return page(start, func(a pgtype.UUID) ([]db.ListStatusesForExportRow, error) {
			return s.q.ListStatusesForExport(ctx, db.ListStatusesForExportParams{WorkspaceID: ws, After: a, RowLimit: limit})
		}, func(r db.ListStatusesForExportRow) pgtype.UUID { return r.ID }, emit)
	case TableAttachments:
		return page(start, func(a pgtype.UUID) ([]db.ListAttachmentsForExportRow, error) {
			return s.q.ListAttachmentsForExport(ctx, db.ListAttachmentsForExportParams{WorkspaceID: ws, After: a, RowLimit: limit})
		}, func(r db.ListAttachmentsForExportRow) pgtype.UUID { return r.ID }, emit)
	}
	return fmt.Errorf("unknown export table %q", table)
}
