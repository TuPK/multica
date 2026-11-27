package service

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// ShadowRunView is one shadow run of a seat with its report (durable spec 9.4).
type ShadowRunView struct {
	Config db.ShadowConfig
	Report ShadowReport
}

// ListSeatShadowRuns returns a seat's most recent shadow runs, newest first, each with the
// report of its compared pairs. It reads only.
func (s *ShadowService) ListSeatShadowRuns(ctx context.Context, ws, seat pgtype.UUID, limit int32) ([]ShadowRunView, error) {
	configs, err := s.Queries.ListShadowConfigsForAgent(ctx, db.ListShadowConfigsForAgentParams{
		WorkspaceID: ws, AgentID: seat, RowLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list shadow runs: %w", err)
	}
	out := make([]ShadowRunView, 0, len(configs))
	for _, c := range configs {
		report, err := s.Report(ctx, ws, c.ID)
		if err != nil {
			return nil, err
		}
		out = append(out, ShadowRunView{Config: c, Report: report})
	}
	return out, nil
}
