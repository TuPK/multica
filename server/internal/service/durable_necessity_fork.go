package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Scaffolding sunset (durable spec 10, F9): each piece of scaffolding has a necessity test, a
// shadow run of the seat without it. A verdict that the piece is not needed files one proposal to
// remove it. Nothing is removed without the owner's approval (D20).

// Comparison is the outcome of one shadow pair (spec 10.2).
type Comparison string

const (
	ComparisonPrimary Comparison = "primary"
	ComparisonShadow  Comparison = "shadow"
	ComparisonTie     Comparison = "tie"
	ComparisonUnable  Comparison = "unable"
)

// NecessityVerdict applies spec 10.2 to the pairs of one necessity test. "primary" counts
// as a loss for the skill under test (the primary won), "shadow" as a win, "tie" counts
// toward n but neither way, and "unable" is excluded from n.
func NecessityVerdict(comparisons []Comparison, margin float64, minPairs int) string {
	var n, losses, wins int
	for _, c := range comparisons {
		switch c {
		case ComparisonPrimary:
			losses++
		case ComparisonShadow:
			wins++
		case ComparisonTie:
		default:
			continue
		}
		n++
	}
	if n < minPairs {
		return "running"
	}
	lo, hi := NewcombeDiffCI(losses, wins, n)
	switch {
	case hi < margin:
		return "not_needed"
	case lo > 0:
		return "needed"
	case n >= 2*minPairs:
		return "inconclusive"
	}
	return "running"
}

const (
	necessityMargin     = 0.05 // spec 10.2
	necessityMinPairs   = 20   // spec 10.2
	necessitySampleRate = 0.3  // spec 9.4 default
	necessityMinTasks   = 40   // tasks of the seat in 30 days before a skill is tested (spec 10.3)
	necessityStaleDays  = 180  // a skill is tested again after this long (spec 10.3)
	necessitySquadCap   = 2    // running necessity tests per squad (spec 10.3)

	flowStageMinPasses  = 30   // first-round verdicts a gate needs (spec 10.4)
	flowStageMinRate    = 0.95 // first-round approval a gate needs (spec 10.4)
	flowStageWindowDays = 90
)

// ErrNecessityInvalid: a necessity test names a seat or skill that does not fit it.
var ErrNecessityInvalid = errors.New("invalid necessity test")

// ErrNecessityRunning: the skill already has a running necessity test on the seat.
var ErrNecessityRunning = errors.New("a necessity test of this skill is already running")

// NecessityService opens necessity tests, decides them and files their proposals.
type NecessityService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Shadow    *ShadowService
	Proposals *ProposalService
	Flags     *featureflag.Service
}

func (s *NecessityService) enabled(ctx context.Context) bool {
	return s.Flags != nil && featureflags.DurableSeatsEnabled(ctx, s.Flags)
}

// Start opens a necessity test on a skill a seat holds: a shadow run of the seat, without the
// skill, that runs beside the seat's work. The seat has at most one running test per skill.
func (s *NecessityService) Start(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, agentID, skillID pgtype.UUID) (db.NecessityTest, error) {
	if !s.enabled(ctx) {
		return db.NecessityTest{}, ErrDurableSeatsOff
	}
	seat, err := s.Queries.GetAgent(ctx, agentID)
	if err != nil || seat.WorkspaceID != ws || seat.ArchivedAt.Valid || seat.ShadowOfAgentID.Valid || !seat.RuntimeID.Valid {
		return db.NecessityTest{}, fmt.Errorf("%w: seat", ErrNecessityInvalid)
	}
	if _, err := s.Queries.GetSkillInWorkspace(ctx, db.GetSkillInWorkspaceParams{ID: skillID, WorkspaceID: ws}); err != nil {
		return db.NecessityTest{}, fmt.Errorf("%w: skill", ErrNecessityInvalid)
	}
	linked, err := s.Queries.CountAgentSkillLink(ctx, db.CountAgentSkillLinkParams{AgentID: agentID, SkillID: skillID})
	if err != nil || linked == 0 {
		return db.NecessityTest{}, fmt.Errorf("%w: the seat does not hold the skill", ErrNecessityInvalid)
	}
	test, err := s.Queries.InsertNecessityTest(ctx, db.InsertNecessityTestParams{
		ID: dbid.NewV7(), WorkspaceID: ws, SubjectType: "skill", SubjectRef: util.UUIDToString(skillID),
		AgentID: agentID, Margin: numericOf(necessityMargin), MinPairs: necessityMinPairs,
		CreatedByType: actor.Type, CreatedByID: actor.ID,
	})
	if err != nil {
		if isUniqueViolation(err) {
			return db.NecessityTest{}, ErrNecessityRunning
		}
		return db.NecessityTest{}, fmt.Errorf("insert necessity test: %w", err)
	}
	cfg, err := s.Shadow.Start(ctx, ws, actor, ShadowStartInput{
		AgentID: agentID, CandidateRuntimeID: seat.RuntimeID, CandidateModel: seat.Model.String,
		Purpose: ShadowPurposeNecessity, NecessityTestID: test.ID,
		SampleRate: necessitySampleRate, TargetPairs: 2 * necessityMinPairs,
	})
	if err != nil {
		// The test row stays out of the running set, so the skill can be tested again later.
		if _, stopErr := s.Queries.SetNecessityTestStopped(ctx, db.SetNecessityTestStoppedParams{ID: test.ID, WorkspaceID: ws}); stopErr != nil {
			return db.NecessityTest{}, fmt.Errorf("stop necessity test: %w", stopErr)
		}
		return db.NecessityTest{}, err
	}
	if _, err := s.Queries.SetNecessityTestShadowConfig(ctx, db.SetNecessityTestShadowConfigParams{
		ID: test.ID, WorkspaceID: ws, ShadowConfigID: cfg.ID,
	}); err != nil {
		return db.NecessityTest{}, fmt.Errorf("link necessity test: %w", err)
	}
	test.ShadowConfigID = cfg.ID
	return test, nil
}

// WeeklySchedule opens the necessity tests a workspace is due (spec 10.3): a skill of a seat
// that did at least 40 tasks in 30 days and was not tested in the last 180 days. Never-tested
// skills come first, then the least recently tested. At most two run per squad at a time.
func (s *NecessityService) WeeklySchedule(ctx context.Context, ws pgtype.UUID) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	running, err := s.Queries.CountRunningNecessityTestsBySquad(ctx, ws)
	if err != nil {
		return 0, fmt.Errorf("count running tests: %w", err)
	}
	perSquad := map[string]int64{}
	for _, r := range running {
		if r.SquadID.Valid {
			perSquad[util.UUIDToString(r.SquadID)] = r.Running
		}
	}
	candidates, err := s.Queries.ListNecessityCandidateSkills(ctx, ws)
	if err != nil {
		return 0, fmt.Errorf("list skills: %w", err)
	}
	stale := time.Now().AddDate(0, 0, -necessityStaleDays)
	var due []db.ListNecessityCandidateSkillsRow
	for _, c := range candidates {
		if c.Tasks30d < necessityMinTasks || (c.LastTestedAt.Valid && c.LastTestedAt.Time.After(stale)) {
			continue
		}
		due = append(due, c)
	}
	sort.SliceStable(due, func(i, j int) bool {
		a, b := due[i].LastTestedAt, due[j].LastTestedAt
		if a.Valid != b.Valid {
			return !a.Valid
		}
		return a.Time.Before(b.Time)
	})
	started := 0
	for _, c := range due {
		key := util.UUIDToString(c.SquadID)
		if perSquad[key] >= necessitySquadCap {
			continue
		}
		seat, err := s.Queries.GetAgent(ctx, c.AgentID)
		if err != nil || !seat.OwnerID.Valid {
			continue
		}
		actor := GovernanceActor{Type: GovernanceActorMember, ID: seat.OwnerID}
		if _, err := s.Start(ctx, ws, actor, c.AgentID, c.SkillID); err != nil {
			if errors.Is(err, ErrShadowAlreadyRunning) || errors.Is(err, ErrNecessityInvalid) || errors.Is(err, ErrNecessityRunning) {
				continue
			}
			return started, err
		}
		perSquad[key]++
		started++
	}
	return started, nil
}

// Weekly runs WeeklySchedule in every workspace (the weekly job).
func (s *NecessityService) Weekly(ctx context.Context) (int, error) {
	ids, err := s.Queries.ListAllWorkspaceIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces: %w", err)
	}
	total := 0
	for _, id := range ids {
		n, err := s.WeeklySchedule(ctx, id)
		if err != nil {
			return total, err
		}
		total += n
	}
	return total, nil
}

// OnVerdicts decides a necessity run once its comparisons decide it, or once it has its pairs
// (spec 10.2). A verdict other than running completes the run and writes the test's outcome; a
// not_needed skill files its removal as one proposal. It is called after each verdict.
func (s *NecessityService) OnVerdicts(ctx context.Context, cfg db.ShadowConfig) error {
	test, err := s.Queries.GetNecessityTestByShadowConfig(ctx, db.GetNecessityTestByShadowConfigParams{
		ShadowConfigID: cfg.ID, WorkspaceID: cfg.WorkspaceID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load necessity test: %w", err)
	}
	pairs, err := s.Queries.ListShadowPairsForConfig(ctx, db.ListShadowPairsForConfigParams{WorkspaceID: cfg.WorkspaceID, ConfigID: cfg.ID})
	if err != nil {
		return fmt.Errorf("list pairs: %w", err)
	}
	var comparisons []Comparison
	for _, p := range pairs {
		if p.Comparison.Valid {
			comparisons = append(comparisons, Comparison(p.Comparison.String))
		}
	}
	margin, err := test.Margin.Float64Value()
	if err != nil {
		return fmt.Errorf("read margin: %w", err)
	}
	verdict := NecessityVerdict(comparisons, margin.Float64, int(test.MinPairs))
	if verdict == "running" {
		return nil
	}
	completed, err := s.Shadow.completeRun(ctx, cfg)
	if err != nil || !completed {
		return err
	}
	var proposal pgtype.UUID
	if verdict == "not_needed" && test.SubjectType == "skill" {
		if proposal, err = s.fileSkillRemoval(ctx, test); err != nil {
			return err
		}
	}
	result, err := json.Marshal(necessityResultOf(comparisons, margin.Float64))
	if err != nil {
		return fmt.Errorf("encode result: %w", err)
	}
	if _, err := s.Queries.SetNecessityTestOutcome(ctx, db.SetNecessityTestOutcomeParams{
		ID: test.ID, WorkspaceID: test.WorkspaceID, State: verdict, Result: result, ProposalID: proposal,
	}); err != nil {
		return fmt.Errorf("write necessity verdict: %w", err)
	}
	return nil
}

// necessityResult is the evidence a verdict rests on: the counts and the 95% interval of d.
type necessityResult struct {
	Pairs   int     `json:"pairs"`
	Primary int     `json:"primary"`
	Shadow  int     `json:"shadow"`
	Tie     int     `json:"tie"`
	DLower  float64 `json:"d_lower"`
	DUpper  float64 `json:"d_upper"`
	Margin  float64 `json:"margin"`
}

func necessityResultOf(comparisons []Comparison, margin float64) necessityResult {
	var r necessityResult
	for _, c := range comparisons {
		switch c {
		case ComparisonPrimary:
			r.Primary++
		case ComparisonShadow:
			r.Shadow++
		case ComparisonTie:
			r.Tie++
		}
	}
	r.Pairs = r.Primary + r.Shadow + r.Tie
	r.DLower, r.DUpper = NewcombeDiffCI(r.Primary, r.Shadow, r.Pairs)
	r.Margin = margin
	return r
}

// fileSkillRemoval files the one proposal that removes the tested skill from the seat. The
// proposal waits for the owner; nothing is removed here.
func (s *NecessityService) fileSkillRemoval(ctx context.Context, test db.NecessityTest) (pgtype.UUID, error) {
	seat, err := s.Queries.GetAgent(ctx, test.AgentID)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("load seat: %w", err)
	}
	if !seat.HomeSquadID.Valid {
		return pgtype.UUID{}, errors.New("the seat has no squad to file its skill change in")
	}
	payload, err := json.Marshal(skillChangePayload{
		Type:           OrgChangeSkill,
		AgentID:        util.UUIDToString(test.AgentID),
		RemoveSkillIDs: []string{test.SubjectRef},
		Rationale:      "A necessity test ran the seat without this skill and found no loss.",
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("encode skill change: %w", err)
	}
	res, err := s.Proposals.Create(ctx, ProposalCreateParams{
		WorkspaceID: test.WorkspaceID,
		SquadID:     seat.HomeSquadID,
		Content: ProposalContent{
			Title:          "Remove a skill the seat does not need",
			Summary:        "A necessity test ran the seat beside its own work without the skill.",
			Evidence:       []string{fmt.Sprintf("Necessity test %s", util.UUIDToString(test.ID))},
			Recommendation: "Approve the removal.",
			Options:        []string{"Approve", "Keep the skill"},
			CostRisk:       "Removing the skill changes the seat's instructions, not its cost.",
			Ask:            "Remove the skill from the seat?",
		},
		CreatorType: GovernanceActorMember,
		CreatorID:   test.CreatedByID,
		Kind:        proposalkinds.KeyOrgChange,
		Payload:     payload,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("file skill removal: %w", err)
	}
	return res.Proposal.ID, nil
}

// StageStat is a gate's first-round record over the window (spec 10.4): the verdicts on round 0,
// how many approved at once, and how many sent the work back.
type StageStat struct {
	Step          string
	Task          string
	Passes        int
	ApprovedFirst int
	Resends       int
}

// Rate is the share of first-round verdicts that approved.
func (s StageStat) Rate() float64 {
	if s.Passes == 0 {
		return 0
	}
	return float64(s.ApprovedFirst) / float64(s.Passes)
}

// FlowStageCandidate reports whether a gate is a candidate to merge or drop (spec 10.4): at least
// 95% first-round approval on at least 30 first-round verdicts.
func FlowStageCandidate(s StageStat) bool {
	return s.Passes >= flowStageMinPasses && s.Rate() >= flowStageMinRate
}

// FlowStageCandidates returns the gates of a workspace that pass FlowStageCandidate over the
// window. It reads only; the brief entry for the flow's writer is not written here.
func (s *NecessityService) FlowStageCandidates(ctx context.Context, ws pgtype.UUID) ([]StageStat, error) {
	since := time.Now().AddDate(0, 0, -flowStageWindowDays)
	rows, err := s.Queries.ListFlowGateVerdictsSince(ctx, db.ListFlowGateVerdictsSinceParams{
		WorkspaceID: ws, Since: pgtype.Timestamptz{Time: since, Valid: true},
	})
	if err != nil {
		return nil, fmt.Errorf("list gate verdicts: %w", err)
	}
	var out []StageStat
	for _, r := range rows {
		stat := StageStat{
			Step: r.FlowStep.String, Task: r.FlowTask.String,
			Passes: int(r.Passes), ApprovedFirst: int(r.ApprovedFirst), Resends: int(r.Resends),
		}
		if FlowStageCandidate(stat) {
			out = append(out, stat)
		}
	}
	return out, nil
}

// MonthlyFlowStages counts the flow gate candidates of every workspace (the monthly job). It
// writes nothing while durable_seats is off.
func (s *NecessityService) MonthlyFlowStages(ctx context.Context) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	ids, err := s.Queries.ListAllWorkspaceIDs(ctx)
	if err != nil {
		return 0, fmt.Errorf("list workspaces: %w", err)
	}
	total := 0
	for _, id := range ids {
		candidates, err := s.FlowStageCandidates(ctx, id)
		if err != nil {
			return total, err
		}
		total += len(candidates)
	}
	return total, nil
}

// ListForSeat returns the seat's recent necessity tests, newest first.
func (s *NecessityService) ListForSeat(ctx context.Context, ws, agentID pgtype.UUID) ([]db.NecessityTest, error) {
	return s.Queries.ListNecessityTestsForAgent(ctx, db.ListNecessityTestsForAgentParams{WorkspaceID: ws, AgentID: agentID})
}
