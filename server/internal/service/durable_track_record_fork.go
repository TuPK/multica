package service

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// The track record of an (agent, work class) pair (durable spec 3.2) and the verification
// mode that goes with it (spec 3.3). A settlement recomputes its pair at once, so a high
// fail takes effect with the verdict. The nightly job recomputes every pair with a
// settlement in the last 90 days.

// trackWindowDays are the two windows of the track record, 30 and 90 days.
var trackWindowDays = [2]int32{30, 90}

// trackModeWindow is the window the verification mode reads and is stored on.
const trackModeWindow int32 = 30

// trackHorizon is the longest window: the nightly job covers pairs settled within it.
const trackHorizon = 90 * 24 * time.Hour

// ErrTrackModeInvalid: a pinned mode is not full, sampled or spot.
var ErrTrackModeInvalid = errors.New("verification mode must be full, sampled or spot")

// ErrTrackAgentNotFound: the agent is not in this workspace.
var ErrTrackAgentNotFound = errors.New("agent not found")

// TrackRecordService reads and recomputes track records, and pins verification modes.
// Recompute writes nothing new while durable_outcomes is off (RecomputeAll checks it).
type TrackRecordService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Flags     *featureflag.Service
	// Now is the clock for the windows. Nil means time.Now.
	Now func() time.Time
}

func (s *TrackRecordService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// ModeFor returns the verification mode for the next outcome of the pair. A pair with no
// track record starts at full (spec 3.3).
func (s *TrackRecordService) ModeFor(ctx context.Context, ws, agentID pgtype.UUID, workClass string) (string, error) {
	row, err := s.Queries.GetTrackRecord(ctx, db.GetTrackRecordParams{
		WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, WindowDays: trackModeWindow,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return VerificationModeFull, nil
	}
	if err != nil {
		return "", fmt.Errorf("read track record: %w", err)
	}
	return row.VerificationMode, nil
}

// Recompute rewrites the windows of one pair and moves its mode if the rules say so, in
// one transaction.
func (s *TrackRecordService) Recompute(ctx context.Context, ws, agentID pgtype.UUID, workClass string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		return recordTrack(ctx, q, ws, agentID, workClass, s.now())
	})
}

// RecomputeAll is the nightly job. It recomputes every pair with an outcome settled in the
// last 90 days, and returns how many it recomputed. It writes nothing while
// durable_outcomes is off.
func (s *TrackRecordService) RecomputeAll(ctx context.Context) (int, error) {
	if !featureflags.DurableOutcomesEnabled(ctx, s.Flags) {
		return 0, nil
	}
	pairs, err := s.Queries.ListSettledTrackPairsSince(ctx, pgtype.Timestamptz{Time: s.now().Add(-trackHorizon), Valid: true})
	if err != nil {
		return 0, fmt.Errorf("list track pairs: %w", err)
	}
	for i, p := range pairs {
		if err := s.Recompute(ctx, p.WorkspaceID, p.AgentID, p.WorkClass); err != nil {
			return i, err
		}
	}
	return len(pairs), nil
}

// Pin sets or clears the pinned mode of one pair (spec 3.3). A mode pins the pair there,
// and recomputes stop moving it. A nil mode unpins it: the mode stays until the next
// recompute moves it. A change of the mode is one governance event by the actor. It
// returns the 30-day row after the change.
func (s *TrackRecordService) Pin(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, agentID pgtype.UUID, workClass string, mode *string) (db.TrackRecord, error) {
	if mode != nil && !validVerificationMode(*mode) {
		return db.TrackRecord{}, ErrTrackModeInvalid
	}
	var out db.TrackRecord
	err := s.inTx(ctx, func(q *db.Queries) error {
		if _, err := q.GetAgentRuntimeModel(ctx, db.GetAgentRuntimeModelParams{ID: agentID, WorkspaceID: ws}); err != nil {
			return mapNoRows(err, ErrTrackAgentNotFound, "load agent")
		}
		if err := q.EnsureTrackRecordRow(ctx, db.EnsureTrackRecordRowParams{
			WorkspaceID: ws, AgentID: agentID, WorkClass: workClass,
		}); err != nil {
			return fmt.Errorf("ensure track record: %w", err)
		}
		pinned := mode != nil
		if err := q.SetTrackRecordPinned(ctx, db.SetTrackRecordPinnedParams{
			WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, ModePinned: pinned,
		}); err != nil {
			return fmt.Errorf("set pin: %w", err)
		}
		cur, err := q.GetTrackRecord(ctx, db.GetTrackRecordParams{
			WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, WindowDays: trackModeWindow,
		})
		if err != nil {
			return fmt.Errorf("read track record: %w", err)
		}
		if pinned && cur.VerificationMode != *mode {
			if err := q.SetTrackRecordMode(ctx, db.SetTrackRecordModeParams{
				WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, VerificationMode: *mode,
			}); err != nil {
				return fmt.Errorf("set mode: %w", err)
			}
			if err := recordModeChange(ctx, q, ws, agentID, workClass, cur.VerificationMode, *mode, actor, map[string]any{"pinned": true}); err != nil {
				return err
			}
		}
		out, err = q.GetTrackRecord(ctx, db.GetTrackRecordParams{
			WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, WindowDays: trackModeWindow,
		})
		if err != nil {
			return fmt.Errorf("read track record: %w", err)
		}
		return nil
	})
	return out, err
}

func (s *TrackRecordService) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin track record: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit track record: %w", err)
	}
	return nil
}

// recordTrack recomputes both windows of one pair from its settled outcomes, then moves
// its mode by spec 3.3 unless it is pinned. It runs through q, so a caller's transaction
// carries it. An outcome with no agent has no pair and nothing to record.
func recordTrack(ctx context.Context, q *db.Queries, ws, agentID pgtype.UUID, workClass string, now time.Time) error {
	if !agentID.Valid {
		return nil
	}
	var rows [2]TrackRow
	for i, days := range trackWindowDays {
		c, err := q.CountSettledOutcomesForTrack(ctx, db.CountSettledOutcomesForTrackParams{
			WorkspaceID: ws, AgentID: agentID, WorkClass: workClass,
			WindowStart: pgtype.Timestamptz{Time: now.Add(-time.Duration(days) * 24 * time.Hour), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("count track window %d: %w", days, err)
		}
		r := TrackRow{NPass: int(c.NPass), NPartial: int(c.NPartial), NFail: int(c.NFail), NHighFail: int(c.NHighFail)}
		r.N = r.NPass + r.NPartial + r.NFail
		r.ScoreLower = WilsonLower(r.NPass, r.NPartial, r.NFail)
		if c.LastHighFailAt.Valid {
			r.LastHighFailAt = c.LastHighFailAt.Time
		}
		rows[i] = r
		if err := q.UpsertTrackRecordStats(ctx, db.UpsertTrackRecordStatsParams{
			WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, WindowDays: days,
			NPass: int32(c.NPass), NPartial: int32(c.NPartial), NFail: int32(c.NFail),
			NUnverified: int32(c.NUnverified), NHighFail: int32(c.NHighFail),
			ScoreLower: r.ScoreLower, LastHighFailAt: c.LastHighFailAt,
		}); err != nil {
			return fmt.Errorf("write track window %d: %w", days, err)
		}
	}

	cur, err := q.GetTrackRecord(ctx, db.GetTrackRecordParams{
		WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, WindowDays: trackModeWindow,
	})
	if err != nil {
		return fmt.Errorf("read track record: %w", err)
	}
	if cur.ModePinned {
		return nil
	}
	next := NextVerificationMode(cur.VerificationMode, rows[0], rows[1], cur.ModeChangedAt.Time, now)
	if next == cur.VerificationMode {
		return nil
	}
	if err := q.SetTrackRecordMode(ctx, db.SetTrackRecordModeParams{
		WorkspaceID: ws, AgentID: agentID, WorkClass: workClass, VerificationMode: next,
	}); err != nil {
		return fmt.Errorf("set mode: %w", err)
	}
	return recordModeChange(ctx, q, ws, agentID, workClass, cur.VerificationMode, next,
		GovernanceActor{Type: GovernanceActorTrackRecord},
		map[string]any{"score_lower": rows[0].ScoreLower, "n": rows[0].N, "window_days": trackModeWindow})
}

// recordModeChange writes the one governance event a mode change makes (spec 3.3).
func recordModeChange(ctx context.Context, q *db.Queries, ws, agentID pgtype.UUID, workClass, from, to string, actor GovernanceActor, details map[string]any) error {
	details["work_class"] = workClass
	details["from"] = from
	details["to"] = to
	if err := NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: ws,
		Kind:        GovernanceKindVerificationMode,
		ActorType:   actor.Type,
		ActorID:     actor.ID,
		SubjectType: "agent",
		SubjectID:   agentID,
		Summary:     fmt.Sprintf("Verification for %s work changed from %s to %s", workClass, from, to),
		Details:     details,
	}); err != nil {
		return fmt.Errorf("record verification mode change: %w", err)
	}
	return nil
}

func validVerificationMode(m string) bool {
	switch m {
	case VerificationModeFull, VerificationModeSampled, VerificationModeSpot:
		return true
	}
	return false
}
