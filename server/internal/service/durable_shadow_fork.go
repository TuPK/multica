package service

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Shadow run (durable spec 9.4, F8): a candidate runtime and model run a sampled share of
// a seat's real work, beside the seat, and the two outputs are compared blind. The
// candidate runs as a shadow agent (D27) in the shadow run class (D18), so it can only
// read; every effect it tries is blocked by the gateway. Nothing here changes the seat.

const (
	ShadowPurposeModelTrial = "model_trial"
	ShadowPurposeNecessity  = "necessity_test"

	// shadowKindMarker is the durable_kind a shadow task carries; durableRunClassOf reads
	// it to put the task in the shadow run class.
	shadowKindMarker = "shadow"

	// shadowRunningAgentIndex enforces at most one running shadow_config per (workspace_id, agent_id).
	shadowRunningAgentIndex = "shadow_config_running_agent_uidx"
)

var (
	// ErrDurableSeatsOff: a shadow run is started or judged while durable_seats is off.
	ErrDurableSeatsOff = errors.New("durable_seats is off")
	// ErrShadowAlreadyRunning: the seat already has a running shadow run.
	ErrShadowAlreadyRunning = errors.New("the seat already has a running shadow run")
	// ErrShadowInvalid: a shadow run's input is out of range or names something outside the workspace.
	ErrShadowInvalid = errors.New("invalid shadow run")
	// ErrShadowPairNotFound: no pair of that id in the workspace.
	ErrShadowPairNotFound = errors.New("shadow pair not found")
	// ErrShadowVerdict: the verdict is not A, B, tie or unable.
	ErrShadowVerdict = errors.New("verdict must be A, B, tie or unable")
)

// ShadowSelected chooses a primary task for a shadow run (spec 9.4): a deterministic
// sample of sha256(issue_id || config_id), so the same issue and run always agree.
// A rate of 1 chooses every issue.
func ShadowSelected(issueID, configID string, rate float64) bool {
	sum := sha256.Sum256([]byte(issueID + configID))
	point := float64(binary.BigEndian.Uint64(sum[:8])) / (1 << 64)
	return point < rate
}

// ShadowPrimaryIsA says whether the primary's output is labelled A for a pair. The label
// is fixed by the pair, so a juror sees the same order on a retry and the order varies
// across pairs (spec 9.4: the juror is blind to which model is which).
func ShadowPrimaryIsA(pairID string) bool {
	sum := sha256.Sum256([]byte("shadow-label|" + pairID))
	return sum[0]&1 == 1
}

// ResolveShadowVerdict maps a juror's verdict on the labelled outputs back to the pair's
// sides. A and B are the labels; tie and unable need no side.
func ResolveShadowVerdict(verdict string, primaryIsA bool) (Comparison, error) {
	switch verdict {
	case "tie":
		return ComparisonTie, nil
	case "unable":
		return ComparisonUnable, nil
	case "A", "B":
		if (verdict == "A") == primaryIsA {
			return ComparisonPrimary, nil
		}
		return ComparisonShadow, nil
	}
	return "", ErrShadowVerdict
}

// ShadowPairResult is one pair as the report reads it: its comparison and the cost each
// side spent on it, in micro-units.
type ShadowPairResult struct {
	Comparison        Comparison
	PrimaryCostMicros int64
	ShadowCostMicros  int64
}

// ShadowReport is the report of a shadow run (spec 9.4). Pairs counts the compared pairs,
// which are shadow, tie and primary; unable pairs are left out of n. NotLosingLower is the
// Wilson lower bound of (shadow + tie) / n, the share of the candidate's output that is at
// least as good. Costs are averages over the compared pairs.
type ShadowReport struct {
	Pairs                int
	Shadow               int
	Tie                  int
	Primary              int
	Unable               int
	NotLosingLower       float64
	PrimaryCostAvgMicros int64
	ShadowCostAvgMicros  int64
}

// BuildShadowReport summarises the pairs of one run.
func BuildShadowReport(results []ShadowPairResult) ShadowReport {
	var r ShadowReport
	var primaryCost, shadowCost int64
	for _, p := range results {
		switch p.Comparison {
		case ComparisonShadow:
			r.Shadow++
		case ComparisonTie:
			r.Tie++
		case ComparisonPrimary:
			r.Primary++
		case ComparisonUnable:
			r.Unable++
			continue
		default:
			continue
		}
		primaryCost += p.PrimaryCostMicros
		shadowCost += p.ShadowCostMicros
	}
	r.Pairs = r.Shadow + r.Tie + r.Primary
	if r.Pairs > 0 {
		r.NotLosingLower, _ = wilsonScore(r.Shadow+r.Tie, r.Pairs)
		r.PrimaryCostAvgMicros = primaryCost / int64(r.Pairs)
		r.ShadowCostAvgMicros = shadowCost / int64(r.Pairs)
	}
	return r
}

// ShadowStartInput is what a person asks for when a shadow run opens.
type ShadowStartInput struct {
	AgentID            pgtype.UUID
	CandidateRuntimeID pgtype.UUID
	CandidateModel     string
	Purpose            string
	NecessityTestID    pgtype.UUID
	WorkClassPrefix    string
	SampleRate         float64
	TargetPairs        int
	AutoPromote        bool
}

// ShadowService opens shadow runs, gives each sampled primary its shadow task, records the
// verdicts and reports a run. It writes nothing while durable_seats is off.
type ShadowService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	// Tasks gives the shadow agent its task and its juror their check.
	Tasks *TaskService
	// Runtime files the runtime change a finished run earns (spec 9.6). Nil files none.
	Runtime *RuntimeChangeService
	Flags   *featureflag.Service
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

func (s *ShadowService) enabled(ctx context.Context) bool {
	return s.Flags != nil && featureflags.DurableSeatsEnabled(ctx, s.Flags)
}

// Start opens a shadow run on a seat. The seat must be an ordinary agent in the workspace,
// the candidate runtime must be the workspace's, and the seat has at most one running run.
func (s *ShadowService) Start(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, in ShadowStartInput) (db.ShadowConfig, error) {
	if !s.enabled(ctx) {
		return db.ShadowConfig{}, ErrDurableSeatsOff
	}
	if in.SampleRate <= 0 || in.SampleRate > 1 || in.TargetPairs < 5 || in.TargetPairs > 200 {
		return db.ShadowConfig{}, fmt.Errorf("%w: sample rate in (0,1] and target pairs 5 to 200", ErrShadowInvalid)
	}
	if in.Purpose != ShadowPurposeModelTrial && in.Purpose != ShadowPurposeNecessity {
		return db.ShadowConfig{}, fmt.Errorf("%w: purpose", ErrShadowInvalid)
	}
	seat, err := s.Queries.GetAgent(ctx, in.AgentID)
	if err != nil || seat.WorkspaceID != ws || seat.ArchivedAt.Valid || seat.ShadowOfAgentID.Valid {
		return db.ShadowConfig{}, fmt.Errorf("%w: seat", ErrShadowInvalid)
	}
	// Auto-promotion moves a seat without a person deciding each change (spec 9.6), so
	// only an owner or an admin may turn it on.
	if in.AutoPromote {
		if err := requireGovernanceBoss(ctx, s.Queries, ws, actor); err != nil {
			return db.ShadowConfig{}, err
		}
	}
	rt, err := s.Queries.GetAgentRuntime(ctx, in.CandidateRuntimeID)
	if err != nil || rt.WorkspaceID != ws {
		return db.ShadowConfig{}, fmt.Errorf("%w: candidate runtime", ErrShadowInvalid)
	}
	if _, err := s.Queries.GetRunningShadowConfigForAgent(ctx, db.GetRunningShadowConfigForAgentParams{
		WorkspaceID: ws, AgentID: in.AgentID,
	}); err == nil {
		return db.ShadowConfig{}, ErrShadowAlreadyRunning
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return db.ShadowConfig{}, fmt.Errorf("check running shadow: %w", err)
	}
	cfg, err := s.Queries.InsertShadowConfig(ctx, db.InsertShadowConfigParams{
		ID: dbid.NewV7(), WorkspaceID: ws, AgentID: in.AgentID, CandidateRuntimeID: in.CandidateRuntimeID,
		CandidateModel: in.CandidateModel, Purpose: in.Purpose, NecessityTestID: in.NecessityTestID,
		WorkClassPrefix: in.WorkClassPrefix, SampleRate: numericOf(in.SampleRate), TargetPairs: int32(in.TargetPairs),
		AutoPromote: in.AutoPromote, CreatedBy: actor.ID,
	})
	if isShadowRunningUniqueViolation(err) {
		return db.ShadowConfig{}, ErrShadowAlreadyRunning
	}
	return cfg, err
}

func isShadowRunningUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == shadowRunningAgentIndex
}

// OnPrimaryClaimed runs when a seat's ordinary task is claimed. If the seat has a running
// shadow run, the issue matches its prefix, and the issue is sampled, it records one pair
// and gives the shadow agent a shadow task on the same issue. A second call for the same
// primary task changes nothing. Durable tasks (juror checks, shadows) are never sampled.
func (s *ShadowService) OnPrimaryClaimed(ctx context.Context, task db.AgentTaskQueue) error {
	if !s.enabled(ctx) || durableRunClassOf(task.Context) != "" || !task.IssueID.Valid {
		return nil
	}
	seat, err := s.Queries.GetAgent(ctx, task.AgentID)
	if err != nil || seat.ShadowOfAgentID.Valid {
		return nil
	}
	cfg, err := s.Queries.GetRunningShadowConfigForAgent(ctx, db.GetRunningShadowConfigForAgentParams{
		WorkspaceID: seat.WorkspaceID, AgentID: task.AgentID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load shadow run: %w", err)
	}
	issue, err := s.Queries.GetIssue(ctx, task.IssueID)
	if err != nil {
		return fmt.Errorf("load issue: %w", err)
	}
	if cfg.WorkClassPrefix != "" && !strings.HasPrefix(issueWorkClass(ctx, s.Queries, issue), cfg.WorkClassPrefix) {
		return nil
	}
	rate, err := cfg.SampleRate.Float64Value()
	if err != nil {
		return fmt.Errorf("read sample rate: %w", err)
	}
	if !ShadowSelected(util.UUIDToString(issue.ID), util.UUIDToString(cfg.ID), rate.Float64) {
		return nil
	}
	shadowTask, created, err := s.recordPair(ctx, cfg, task, issue)
	if err != nil || !created {
		return err
	}
	s.Tasks.broadcastTaskEvent(ctx, protocol.EventTaskQueued, shadowTask)
	s.Tasks.NotifyTaskEnqueued(ctx, shadowTask)
	return nil
}

// recordPair creates the pair and its marked task in the same transaction.
func (s *ShadowService) recordPair(ctx context.Context, cfg db.ShadowConfig, task db.AgentTaskQueue, issue db.Issue) (db.AgentTaskQueue, bool, error) {
	var shadowTask db.AgentTaskQueue
	created := false
	err := s.inTx(ctx, func(q *db.Queries) error {
		locked, err := q.LockShadowConfig(ctx, db.LockShadowConfigParams{ID: cfg.ID, WorkspaceID: cfg.WorkspaceID})
		if err != nil {
			return fmt.Errorf("lock shadow run: %w", err)
		}
		if locked.State != "running" {
			return nil
		}
		pair, err := q.InsertShadowPair(ctx, db.InsertShadowPairParams{
			ID: dbid.NewV7(), WorkspaceID: cfg.WorkspaceID, ConfigID: cfg.ID,
			IssueID: task.IssueID, PrimaryTaskID: task.ID,
		})
		if errors.Is(err, pgx.ErrNoRows) {
			return nil
		}
		if err != nil {
			return fmt.Errorf("insert pair: %w", err)
		}
		shadowAgent := locked.ShadowAgentID
		if !shadowAgent.Valid {
			seat, err := q.GetAgent(ctx, cfg.AgentID)
			if err != nil {
				return fmt.Errorf("load seat: %w", err)
			}
			agent, err := q.CreateShadowAgent(ctx, db.CreateShadowAgentParams{
				RuntimeID: cfg.CandidateRuntimeID, Model: pgtype.Text{String: cfg.CandidateModel, Valid: cfg.CandidateModel != ""},
				SeatID: cfg.AgentID, WorkspaceID: cfg.WorkspaceID,
				Name: fmt.Sprintf("%s (shadow %s)", seat.Name, util.UUIDToString(cfg.ID)[:8]),
			})
			if err != nil {
				return fmt.Errorf("create shadow agent: %w", err)
			}
			shadowAgent = agent.ID
			if !ShadowOmitsSkills(cfg.Purpose) {
				skills, err := q.ListEffectiveAgentSkills(ctx, cfg.AgentID)
				if err != nil {
					return fmt.Errorf("list seat skills for shadow: %w", err)
				}
				for _, sk := range skills {
					if err := q.AddAgentSkill(ctx, db.AddAgentSkillParams{AgentID: agent.ID, SkillID: sk.ID}); err != nil {
						return fmt.Errorf("copy seat skill to shadow: %w", err)
					}
				}
			}
			if _, err := q.SetShadowConfigShadowAgent(ctx, db.SetShadowConfigShadowAgentParams{ID: cfg.ID, ShadowAgentID: agent.ID}); err != nil {
				return err
			}
		}
		shadowTask, err = s.Tasks.enqueueVerificationTaskTx(ctx, q, issue, shadowAgent)
		if err != nil {
			return fmt.Errorf("enqueue shadow task: %w", err)
		}
		if _, err := q.SetShadowTaskContext(ctx, db.SetShadowTaskContextParams{
			ID: shadowTask.ID, ShadowConfigID: util.UUIDToString(cfg.ID), ShadowPairID: util.UUIDToString(pair.ID),
			ShadowPurpose: cfg.Purpose, OmitSkills: ShadowOmitsSkills(cfg.Purpose),
		}); err != nil {
			return fmt.Errorf("mark shadow task: %w", err)
		}
		if _, err := q.SetShadowPairShadowTask(ctx, db.SetShadowPairShadowTaskParams{
			ID: pair.ID, ShadowTaskID: shadowTask.ID,
		}); err != nil {
			return fmt.Errorf("link shadow task: %w", err)
		}
		shadowTask, err = q.GetAgentTask(ctx, shadowTask.ID)
		if err != nil {
			return fmt.Errorf("load shadow task: %w", err)
		}
		created = true
		return nil
	})
	return shadowTask, created, err
}

// RecordVerdict writes the blind verdict of a juror on a pair's labelled outputs, once.
// It writes nothing while durable_seats is off.
func (s *ShadowService) RecordVerdict(ctx context.Context, ws, pairID pgtype.UUID, verdict string) error {
	if !s.enabled(ctx) {
		return nil
	}
	pair, err := s.Queries.GetShadowPair(ctx, db.GetShadowPairParams{ID: pairID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrShadowPairNotFound
	}
	if err != nil {
		return fmt.Errorf("load pair: %w", err)
	}
	return s.recordVerdict(ctx, pair, verdict, "")
}

// Report summarises a run's compared pairs.
func (s *ShadowService) Report(ctx context.Context, ws, configID pgtype.UUID) (ShadowReport, error) {
	pairs, err := s.Queries.ListShadowPairsForConfig(ctx, db.ListShadowPairsForConfigParams{WorkspaceID: ws, ConfigID: configID})
	if err != nil {
		return ShadowReport{}, fmt.Errorf("list pairs: %w", err)
	}
	results := make([]ShadowPairResult, 0, len(pairs))
	for _, p := range pairs {
		if !p.Comparison.Valid {
			continue
		}
		results = append(results, ShadowPairResult{
			Comparison:        Comparison(p.Comparison.String),
			PrimaryCostMicros: p.PrimaryCostMicros,
			ShadowCostMicros:  p.ShadowCostMicros,
		})
	}
	return BuildShadowReport(results), nil
}

func (s *ShadowService) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}

// numericOf converts a rate to the numeric column's value.
func numericOf(f float64) pgtype.Numeric {
	var n pgtype.Numeric
	_ = n.Scan(fmt.Sprintf("%g", math.Round(f*1e9)/1e9))
	return n
}
