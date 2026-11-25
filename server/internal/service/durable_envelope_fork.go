package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// The autonomy envelope (durable spec 3.4). The boss sets a floor and a ceiling for a
// squad's autonomy level; the machine moves the level inside them on evidence. A squad
// with no envelope never changes level by itself. Every level the machine writes is one
// autonomy_changed governance entry with actor track_record.

const (
	// envelopeMinSample: a squad with fewer settled outcomes keeps its level.
	envelopeMinSample = 20
	// envelopeStepGap: a rise is at most one level per this gap.
	envelopeStepGap = 14 * 24 * time.Hour
	// envelopeL2Hold: a rise from L2 needs this long at L2.
	envelopeL2Hold = 60 * 24 * time.Hour
	// envelopeWindow is the squad's reputation window; envelopeHighWindow the short one
	// for high fails.
	envelopeWindow     = 90 * 24 * time.Hour
	envelopeHighWindow = 30 * 24 * time.Hour
	// envelopeAcceptanceMin is the accepted share of proposals a rise from L1 needs.
	envelopeAcceptanceMin = 0.60
	// envelopeWindowDays is the window the autonomy_changed details name.
	envelopeWindowDays = 90
)

// Envelope is a squad's floor and ceiling, both autonomy levels.
type Envelope struct {
	Floor   int
	Ceiling int
}

// SquadStats is the reputation of a squad, as the envelope reads it.
type SquadStats struct {
	// N is the settled verified outcomes in the window: pass, partial and fail.
	N          int
	ScoreLower float64
	HighFail30 bool
	HighFail90 bool
	// AcceptanceRate and HasAcceptance come from the squad's attention row: a rate is
	// there only once the sample is large enough.
	AcceptanceRate float64
	HasAcceptance  bool
	// LevelSince is when the squad entered its current level. Zero when unknown.
	LevelSince time.Time
}

// envelopeHoldBar is the lower bound a squad at level must keep to stay there. It is the
// bar of the rise into that level (spec 3.4).
func envelopeHoldBar(level int) float64 {
	switch level {
	case AutonomyLevelWork:
		return 0.60
	case AutonomyLevelTrusted:
		return 0.80
	case AutonomyLevelAutonomous:
		return 0.90
	}
	return 0
}

// envelopeMayRise reports whether the evidence meets the rule for the next level up.
func envelopeMayRise(cur int, s SquadStats, now time.Time) bool {
	switch cur {
	case AutonomyLevelSuggest:
		return s.ScoreLower >= 0.60
	case AutonomyLevelWork:
		return s.ScoreLower >= 0.80 && !s.HighFail30 && s.HasAcceptance && s.AcceptanceRate >= envelopeAcceptanceMin
	case AutonomyLevelTrusted:
		return s.ScoreLower >= 0.90 && !s.HighFail90 && !s.LevelSince.IsZero() &&
			!now.Before(s.LevelSince.Add(envelopeL2Hold))
	}
	return false
}

// NextSquadLevel returns the level a squad moves to on this calculation (spec 3.4). It
// moves at most one level. It never leaves the envelope: a fall stops at the floor and a
// rise stops at the ceiling. A squad with fewer than envelopeMinSample outcomes keeps its
// level. A high fail is not decided here: it drops the squad at once (OnHighFail).
func NextSquadLevel(cur int, env Envelope, s SquadStats, lastAuto time.Time, now time.Time) int {
	if s.N < envelopeMinSample {
		return cur
	}
	if cur > env.Floor && s.ScoreLower < envelopeHoldBar(cur) {
		return cur - 1
	}
	stepped := lastAuto.IsZero() || !now.Before(lastAuto.Add(envelopeStepGap))
	if cur < env.Ceiling && stepped && envelopeMayRise(cur, s, now) {
		return cur + 1
	}
	return cur
}

// EnvelopeService runs the envelope: the nightly job moves squads on evidence, a settled
// high fail drops its squad inside the settling transaction, and the boss sets and clears
// the envelope. Now is the clock; nil means time.Now.
type EnvelopeService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Flags     *featureflag.Service
	Now       func() time.Time
}

func (s *EnvelopeService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// squadEnvelopeStats reads a squad's reputation through q, so a caller's transaction
// carries it.
func squadEnvelopeStats(ctx context.Context, q *db.Queries, ws, squadID pgtype.UUID, now time.Time) (SquadStats, error) {
	c, err := q.SquadEnvelopeOutcomeCounts(ctx, db.SquadEnvelopeOutcomeCountsParams{
		WorkspaceID: ws,
		SquadID:     squadID,
		WindowStart: pgtype.Timestamptz{Time: now.Add(-envelopeWindow), Valid: true},
		Since30:     pgtype.Timestamptz{Time: now.Add(-envelopeHighWindow), Valid: true},
	})
	if err != nil {
		return SquadStats{}, fmt.Errorf("count squad outcomes: %w", err)
	}
	pass, partial, fail := int(c.NPass), int(c.NPartial), int(c.NFail)
	s := SquadStats{
		N:          pass + partial + fail,
		ScoreLower: WilsonLower(pass, partial, fail),
		HighFail30: c.HighFail30 > 0,
		HighFail90: c.HighFail90 > 0,
	}

	att, err := q.GetSquadAttention(ctx, db.GetSquadAttentionParams{SquadID: squadID, WorkspaceID: ws})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return SquadStats{}, fmt.Errorf("read squad attention: %w", err)
	default:
		if rate := numericRate(att.AcceptanceRate); rate != nil && att.SampleSize >= AttentionMinSample {
			s.AcceptanceRate, s.HasAcceptance = *rate, true
		}
	}

	last, err := q.GetLastAutonomyChange(ctx, db.GetLastAutonomyChangeParams{WorkspaceID: ws, SubjectID: squadID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return SquadStats{}, fmt.Errorf("read last autonomy change: %w", err)
	default:
		s.LevelSince = last.CreatedAt.Time
	}
	return s, nil
}

// applyEnvelopeLevel moves a squad to level to, in the caller's transaction: it sets the
// level, stamps the envelope's last automatic change, and writes the one governance entry
// the change makes. trigger is the outcome that caused the move, or invalid for the
// nightly job.
func applyEnvelopeLevel(ctx context.Context, q *db.Queries, squad db.Squad, to int, stats SquadStats, now time.Time, trigger pgtype.UUID) error {
	from := int(squad.AutonomyLevel)
	if _, err := q.SetSquadAutonomyLevel(ctx, db.SetSquadAutonomyLevelParams{
		ID: squad.ID, WorkspaceID: squad.WorkspaceID, AutonomyLevel: int32(to),
	}); err != nil {
		return fmt.Errorf("set autonomy level: %w", err)
	}
	if err := q.StampSquadEnvelopeAutoChange(ctx, db.StampSquadEnvelopeAutoChangeParams{
		WorkspaceID: squad.WorkspaceID, SquadID: squad.ID, At: pgtype.Timestamptz{Time: now, Valid: true},
	}); err != nil {
		return fmt.Errorf("stamp envelope: %w", err)
	}
	details := map[string]any{
		"from": from, "to": to, "score_lower": stats.ScoreLower, "n": stats.N, "window_days": envelopeWindowDays,
	}
	if trigger.Valid {
		details["trigger_outcome_id"] = util.UUIDToString(trigger)
	}
	return NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: squad.WorkspaceID,
		Kind:        GovernanceKindAutonomyChanged,
		ActorType:   GovernanceActorTrackRecord,
		SubjectType: "squad",
		SubjectID:   squad.ID,
		Summary: fmt.Sprintf("%s moved from L%d %s to L%d %s on evidence: %d settled outcomes, lower bound %.2f",
			squad.Name, from, AutonomyLevelName(from), to, AutonomyLevelName(to), stats.N, stats.ScoreLower),
		Details: details,
	})
}

// Nightly moves every squad with an envelope by NextSquadLevel and returns how many it
// moved. Each squad is its own transaction. It writes nothing while durable_outcomes is off.
func (s *EnvelopeService) Nightly(ctx context.Context) (int, error) {
	if !featureflags.DurableOutcomesEnabled(ctx, s.Flags) {
		return 0, nil
	}
	envs, err := s.Queries.ListSquadEnvelopes(ctx)
	if err != nil {
		return 0, fmt.Errorf("list squad envelopes: %w", err)
	}
	moved := 0
	for _, e := range envs {
		ok, err := s.evaluate(ctx, e.WorkspaceID, e.SquadID)
		if err != nil {
			return moved, err
		}
		if ok {
			moved++
		}
	}
	return moved, nil
}

// evaluate applies the rules to one squad in its own transaction. It locks the squad, then
// the envelope, the order OnHighFail uses too.
func (s *EnvelopeService) evaluate(ctx context.Context, ws, squadID pgtype.UUID) (bool, error) {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("begin envelope: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)

	squad, err := q.LockSquadForAutonomy(ctx, db.LockSquadForAutonomyParams{ID: squadID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && squad.ArchivedAt.Valid) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock squad: %w", err)
	}
	env, err := q.LockSquadEnvelope(ctx, db.LockSquadEnvelopeParams{WorkspaceID: ws, SquadID: squadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("lock envelope: %w", err)
	}

	now := s.now()
	stats, err := squadEnvelopeStats(ctx, q, ws, squadID, now)
	if err != nil {
		return false, err
	}
	cur := int(squad.AutonomyLevel)
	next := NextSquadLevel(cur, Envelope{Floor: int(env.FloorLevel), Ceiling: int(env.CeilingLevel)},
		stats, env.LastAutoChangeAt.Time, now)
	if next == cur {
		return false, nil
	}
	if err := applyEnvelopeLevel(ctx, q, squad, next, stats, now, pgtype.UUID{}); err != nil {
		return false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit envelope: %w", err)
	}
	return true, nil
}

// OnHighFail drops the squad of a settled high fail one level, never below its floor
// (spec 3.4). It runs through q, the settling transaction, so the drop and the verdict
// commit together. The caller passes the outcome with its settled verdict and severity.
// An outcome with no squad, or a squad with no envelope, changes nothing.
func (s *EnvelopeService) OnHighFail(ctx context.Context, q *db.Queries, o db.WorkOutcome) error {
	if o.Verdict != "fail" || o.Severity != "high" || !o.SquadID.Valid {
		return nil
	}
	squad, err := q.LockSquadForAutonomy(ctx, db.LockSquadForAutonomyParams{ID: o.SquadID, WorkspaceID: o.WorkspaceID})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && squad.ArchivedAt.Valid) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock squad: %w", err)
	}
	env, err := q.LockSquadEnvelope(ctx, db.LockSquadEnvelopeParams{WorkspaceID: o.WorkspaceID, SquadID: o.SquadID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("lock envelope: %w", err)
	}
	cur := int(squad.AutonomyLevel)
	if cur <= int(env.FloorLevel) {
		return nil
	}
	now := s.now()
	stats, err := squadEnvelopeStats(ctx, q, o.WorkspaceID, o.SquadID, now)
	if err != nil {
		return err
	}
	return applyEnvelopeLevel(ctx, q, squad, cur-1, stats, now, o.ID)
}

// widenEnvelopeForLevel widens a squad's envelope to take in a level the boss set outside
// it (spec 3.4). A level inside the envelope, or a squad with none, changes nothing.
func widenEnvelopeForLevel(ctx context.Context, q *db.Queries, ws, squadID pgtype.UUID, level int) error {
	if _, err := q.WidenSquadEnvelope(ctx, db.WidenSquadEnvelopeParams{
		Level: int32(level), WorkspaceID: ws, SquadID: squadID,
	}); err != nil {
		return fmt.Errorf("widen squad envelope: %w", err)
	}
	return nil
}

// EnvelopeView is a squad's envelope, when it has one, and the reputation the envelope
// reads.
type EnvelopeView struct {
	Envelope *db.SquadAutonomyEnvelope
	Stats    SquadStats
}

// Envelope errors.
var (
	// ErrEnvelopeInvalid: a bound is not 0 to 3, or the floor is above the ceiling.
	ErrEnvelopeInvalid = errors.New("floor and ceiling must be 0 to 3, with the floor at or below the ceiling")
	// ErrEnvelopeNotFound: the squad has no envelope to clear.
	ErrEnvelopeNotFound = errors.New("the squad has no autonomy envelope")
)

// Get returns a live squad's envelope and reputation. A squad with no envelope gets a nil
// Envelope.
func (s *EnvelopeService) Get(ctx context.Context, ws, squadID pgtype.UUID) (EnvelopeView, error) {
	squad, err := s.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && squad.ArchivedAt.Valid) {
		return EnvelopeView{}, ErrAutonomySquadNotFound
	}
	if err != nil {
		return EnvelopeView{}, fmt.Errorf("load squad: %w", err)
	}
	var out EnvelopeView
	env, err := s.Queries.GetSquadEnvelope(ctx, db.GetSquadEnvelopeParams{WorkspaceID: ws, SquadID: squadID})
	switch {
	case errors.Is(err, pgx.ErrNoRows):
	case err != nil:
		return EnvelopeView{}, fmt.Errorf("load envelope: %w", err)
	default:
		out.Envelope = &env
	}
	out.Stats, err = squadEnvelopeStats(ctx, s.Queries, ws, squadID, s.now())
	if err != nil {
		return EnvelopeView{}, err
	}
	return out, nil
}

// Set writes a squad's envelope. Only an owner or an admin may (the caller checks it, and
// the actor is recorded). A level the squad holds outside the new envelope is moved to the
// nearest bound in the same transaction, as the boss's own change.
func (s *EnvelopeService) Set(ctx context.Context, ws, squadID pgtype.UUID, floor, ceiling int, reason string, actor GovernanceActor) (EnvelopeView, error) {
	reason = strings.TrimSpace(reason)
	if floor < 0 || ceiling > 3 || floor > ceiling {
		return EnvelopeView{}, ErrEnvelopeInvalid
	}
	if reason == "" || len([]rune(reason)) > AutonomyReasonMax {
		return EnvelopeView{}, ErrAutonomyReasonRequired
	}
	err := s.inTx(ctx, func(q *db.Queries) error {
		if err := requireGovernanceBoss(ctx, q, ws, actor); err != nil {
			return err
		}
		squad, err := q.LockSquadForAutonomy(ctx, db.LockSquadForAutonomyParams{ID: squadID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && squad.ArchivedAt.Valid) {
			return ErrAutonomySquadNotFound
		}
		if err != nil {
			return fmt.Errorf("lock squad: %w", err)
		}
		if _, err := q.UpsertSquadEnvelope(ctx, db.UpsertSquadEnvelopeParams{
			WorkspaceID: ws, SquadID: squadID, FloorLevel: int32(floor), CeilingLevel: int32(ceiling),
			SetBy: actor.ID, Reason: reason,
		}); err != nil {
			return fmt.Errorf("write envelope: %w", err)
		}
		level := int(squad.AutonomyLevel)
		if level >= floor && level <= ceiling {
			return nil
		}
		clamped := min(max(level, floor), ceiling)
		_, err = ApplyAutonomyChange(ctx, q, ws, squadID, clamped, actor, reason)
		return err
	})
	if err != nil {
		return EnvelopeView{}, err
	}
	return s.Get(ctx, ws, squadID)
}

// Clear removes a squad's envelope. The squad keeps its level and no longer moves by
// itself.
func (s *EnvelopeService) Clear(ctx context.Context, ws, squadID pgtype.UUID, actor GovernanceActor) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		if err := requireGovernanceBoss(ctx, q, ws, actor); err != nil {
			return err
		}
		n, err := q.DeleteSquadEnvelope(ctx, db.DeleteSquadEnvelopeParams{WorkspaceID: ws, SquadID: squadID})
		if err != nil {
			return fmt.Errorf("delete envelope: %w", err)
		}
		if n == 0 {
			return ErrEnvelopeNotFound
		}
		return nil
	})
}

func (s *EnvelopeService) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin envelope: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit envelope: %w", err)
	}
	return nil
}

// DigestAutonomyAutoChangesMax bounds the automatic changes the brief lists, newest first.
const DigestAutonomyAutoChangesMax = 20

// DigestAutonomyAutoChange is one level the envelope moved since the brief's window opened.
type DigestAutonomyAutoChange struct {
	SquadID   string `json:"squad_id"`
	Summary   string `json:"summary"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	ChangedAt string `json:"changed_at"`
}

// DigestAutonomyAutoChanges is the brief's FYI of the envelope's own changes. Count is the
// total; Items is capped at DigestAutonomyAutoChangesMax.
type DigestAutonomyAutoChanges struct {
	Count int                        `json:"count"`
	Items []DigestAutonomyAutoChange `json:"items"`
}

// computeAutonomyAutoChanges lists the envelope's changes since the time (spec 3.4: they go
// into the FYI of the next brief).
func (s *DigestService) computeAutonomyAutoChanges(ctx context.Context, ws pgtype.UUID, since time.Time) (DigestAutonomyAutoChanges, error) {
	rows, err := s.Queries.ListAutoAutonomyChangesSince(ctx, db.ListAutoAutonomyChangesSinceParams{
		WorkspaceID: ws,
		Since:       pgtype.Timestamptz{Time: since, Valid: true},
		RowLimit:    DigestAutonomyAutoChangesMax,
	})
	if err != nil {
		return DigestAutonomyAutoChanges{}, fmt.Errorf("digest autonomy changes: %w", err)
	}
	out := DigestAutonomyAutoChanges{Items: []DigestAutonomyAutoChange{}}
	for i, r := range rows {
		if i == 0 {
			out.Count = int(r.TotalCount)
		}
		var d struct {
			From int `json:"from"`
			To   int `json:"to"`
		}
		if err := json.Unmarshal(r.Details, &d); err != nil {
			return DigestAutonomyAutoChanges{}, fmt.Errorf("read autonomy change details: %w", err)
		}
		out.Items = append(out.Items, DigestAutonomyAutoChange{
			SquadID:   util.UUIDToString(r.SubjectID),
			Summary:   r.Summary,
			From:      d.From,
			To:        d.To,
			ChangedAt: r.CreatedAt.Time.UTC().Format(time.RFC3339),
		})
	}
	return out, nil
}
