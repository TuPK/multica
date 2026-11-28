package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// EffectInput is one action the gate decides on.
type EffectInput = connectedapps.EffectRequest

// EffectDecision is the gate's answer for one action.
type EffectDecision = connectedapps.EffectRecord

// ErrEffectSettled: Finish found no awaiting event, so it was already settled.
var ErrEffectSettled = errors.New("effect event is not awaiting")

// ErrEffectsOff: a write that needs durable_effects was asked while the flag is off.
var ErrEffectsOff = errors.New("durable effects is off")

const (
	// effectUndoWindow is how long an executed effect can be undone (spec 4.6).
	effectUndoWindow = 24 * time.Hour
	// effectPolicyCacheTTL bounds how stale the autonomy level and the overrides
	// the gate reads can be (spec 4.3 and 3.4: 60 seconds).
	effectPolicyCacheTTL = 60 * time.Second
	effectTargetMax      = 300
	effectSummaryMax     = 500
	effectResultMaxBytes = 64 << 10
)

// EffectService decides and logs effects (durable spec 4.3 and 4.4). The gateway and
// the other sources call Gate before the action and Finish after it. Nothing is
// written while durable_effects is off.
type EffectService struct {
	Queries *db.Queries
	Flags   *featureflag.Service
	// Now is the clock. Nil means time.Now.
	Now func() time.Time

	mu        sync.Mutex
	levels    map[string]cachedValue[int]
	overrides map[string]cachedValue[[]Override]
}

type cachedValue[T any] struct {
	value T
	until time.Time
}

// Gate decides one action and writes its event. A blocked action is written as
// blocked. Anything else is written as awaiting until Finish settles it. The
// decision is the stricter of the caller's Prior and the effect policy.
// Task run class (shadow/verification/human) and workspace pause state are
// evaluated inline in InsertGatedEffectEvent so warm calls issue zero SELECTs
// (spec 14.7) and never observe stale shadow or pause state.
func (s *EffectService) Gate(ctx context.Context, in EffectInput) (EffectDecision, error) {
	if !featureflags.DurableEffectsEnabled(ctx, s.Flags) {
		return EffectDecision{Decision: in.Prior}, nil
	}
	rev := in.Reversibility
	if rev == "" {
		rev = connectedapps.DefaultReversibility(in.Class)
	}
	level, err := s.autonomyLevel(ctx, in.WS, in.AgentID)
	if err != nil {
		return EffectDecision{}, err
	}
	overrides, err := s.overridesFor(ctx, in.WS)
	if err != nil {
		return EffectDecision{}, err
	}
	sends := 0
	if in.Class == connectedapps.EffectSend && level == 3 && in.AgentID.Valid {
		sends, err = s.sendsToday(ctx, in.WS, in.AgentID)
		if err != nil {
			return EffectDecision{}, err
		}
	}
	policy := DecideEffect(level, in.Class, rev, overrides, "", "", sends)
	baseDecision := connectedapps.Stricter(in.Prior, policy)

	row, err := s.Queries.InsertGatedEffectEvent(ctx, db.InsertGatedEffectEventParams{
		WorkspaceID:   in.WS,
		TaskID:        in.TaskID,
		AgentID:       in.AgentID,
		MemberID:      in.MemberID,
		Source:        in.Source,
		Target:        effectTruncate(in.Target, effectTargetMax),
		EffectClass:   in.Class,
		Reversibility: rev,
		Summary:       effectTruncate(in.Summary, effectSummaryMax),
		AppToolCallID: in.AppToolCallID,
		BaseDecision:  baseDecision,
	})
	if err != nil {
		return EffectDecision{}, fmt.Errorf("record effect event: %w", err)
	}
	return EffectDecision{Decision: row.Decision, EventID: row.ID}, nil
}

// Finish settles an awaiting event as executed, failed or blocked. When it is
// executed, undo (if any) is stored with a 24-hour window. An event that is no
// longer awaiting returns ErrEffectSettled. An invalid event id, which the gate
// returns while the flag is off, is a no-op.
func (s *EffectService) Finish(ctx context.Context, workspaceID, eventID pgtype.UUID, outcome string, undo *connectedapps.UndoSpec) error {
	return s.FinishWithResult(ctx, workspaceID, eventID, outcome, undo, nil)
}

// FinishWithResult settles an awaiting event and, on executed, also stores the
// provider result payload on the effect event.
func (s *EffectService) FinishWithResult(ctx context.Context, workspaceID, eventID pgtype.UUID, outcome string, undo *connectedapps.UndoSpec, result map[string]any) error {
	if !eventID.Valid {
		return nil
	}
	switch outcome {
	case "executed", "failed", "blocked":
	default:
		return fmt.Errorf("effect outcome %q is not executed, failed or blocked", outcome)
	}
	params := db.FinishEffectEventParams{
		Status:      outcome,
		ID:          eventID,
		WorkspaceID: workspaceID,
	}
	if undo != nil && outcome == "executed" {
		raw, err := json.Marshal(undo)
		if err != nil {
			return fmt.Errorf("encode undo: %w", err)
		}
		params.Undo = raw
		params.UndoUntil = pgtype.Timestamptz{Time: s.now().Add(effectUndoWindow), Valid: true}
	}
	if result != nil && outcome == "executed" {
		raw, err := json.Marshal(result)
		if err != nil || len(raw) > effectResultMaxBytes {
			raw = []byte(`{"truncated":true}`)
		}
		params.Result = raw
	}
	n, err := s.Queries.FinishEffectEvent(ctx, params)
	if err != nil {
		return err
	}
	if n == 0 {
		return ErrEffectSettled
	}
	return nil
}

func (s *EffectService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// autonomyLevel is the squad's level for the agent. An agent in no squad runs at L1
// (spec 4.3). An action with no agent is a person's own: a person is not held to an
// agent's level, so it is L3. The task-class and paused rules still apply to it.
func (s *EffectService) autonomyLevel(ctx context.Context, ws, agent pgtype.UUID) (int, error) {
	if !agent.Valid {
		return 3, nil
	}
	key := util.UUIDToString(agent)
	if v, ok := s.cachedLevel(key); ok {
		return v, nil
	}
	squadID, err := s.Queries.GetAgentSquadID(ctx, agent)
	if errors.Is(err, pgx.ErrNoRows) {
		s.storeLevel(key, 1)
		return 1, nil
	}
	if err != nil {
		return 0, err
	}
	level, err := s.Queries.GetSquadAutonomyLevel(ctx, db.GetSquadAutonomyLevelParams{ID: squadID, WorkspaceID: ws})
	if err != nil {
		return 0, err
	}
	s.storeLevel(key, int(level))
	return int(level), nil
}

func (s *EffectService) overridesFor(ctx context.Context, ws pgtype.UUID) ([]Override, error) {
	key := util.UUIDToString(ws)
	s.mu.Lock()
	hit, ok := s.overrides[key]
	s.mu.Unlock()
	if ok && s.now().Before(hit.until) {
		return hit.value, nil
	}
	store := &EffectPolicyStore{Queries: s.Queries}
	out, err := store.Overrides(ctx, ws)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	if s.overrides == nil {
		s.overrides = map[string]cachedValue[[]Override]{}
	}
	s.overrides[key] = cachedValue[[]Override]{value: out, until: s.now().Add(effectPolicyCacheTTL)}
	s.mu.Unlock()
	return out, nil
}

func (s *EffectService) cachedLevel(key string) (int, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	hit, ok := s.levels[key]
	if !ok || !s.now().Before(hit.until) {
		return 0, false
	}
	return hit.value, true
}

func (s *EffectService) storeLevel(key string, level int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.levels == nil {
		s.levels = map[string]cachedValue[int]{}
	}
	s.levels[key] = cachedValue[int]{value: level, until: s.now().Add(effectPolicyCacheTTL)}
}

// sendsToday counts an agent's executed sends since the start of the UTC day.
func (s *EffectService) sendsToday(ctx context.Context, ws, agent pgtype.UUID) (int, error) {
	now := s.now().UTC()
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC)
	n, err := s.Queries.CountAgentSendsSince(ctx, db.CountAgentSendsSinceParams{
		WorkspaceID: ws,
		AgentID:     agent,
		Since:       pgtype.Timestamptz{Time: start, Valid: true},
	})
	return int(n), err
}

// effectTruncate cuts s to at most n runes, so a long summary never fails the insert.
func effectTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
