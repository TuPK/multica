package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
	"sort"
)

// JurorCandidate is an agent that may sit on a jury, with the runtime provider
// and model it runs on. Diversity is judged on these fields, never on the agent.
type JurorCandidate struct {
	AgentID  string
	Provider string
	Model    string
}

// PickJurors chooses up to size jurors to check work done by doer. doer is never
// picked. Candidates are taken in AgentID order, in three passes: first a provider
// other than doer's and not yet used by the panel, then a model other than doer's
// and not yet used, then whoever is left. So the same pool always gives the same
// panel, however it is ordered.
//
// diversity is the weakest level on the panel: "provider" when no juror shares
// doer's provider, "model_only" when some juror shares the provider but none shares
// the model, "none" when a juror runs doer's exact model or the panel is empty.
func PickJurors(pool []JurorCandidate, doer JurorCandidate, size int) ([]JurorCandidate, string) {
	candidates := make([]JurorCandidate, 0, len(pool))
	for _, c := range pool {
		if c.AgentID != doer.AgentID {
			candidates = append(candidates, c)
		}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		return candidates[i].AgentID < candidates[j].AgentID
	})

	jurors := []JurorCandidate{}
	chosen := map[string]bool{}
	usedProviders := map[string]bool{}
	usedModels := map[string]bool{}
	passes := []func(JurorCandidate) bool{
		func(c JurorCandidate) bool { return c.Provider != doer.Provider && !usedProviders[c.Provider] },
		func(c JurorCandidate) bool { return c.Model != doer.Model && !usedModels[c.Model] },
		func(JurorCandidate) bool { return true },
	}
	for _, eligible := range passes {
		for _, c := range candidates {
			if len(jurors) >= size {
				break
			}
			if chosen[c.AgentID] || !eligible(c) {
				continue
			}
			chosen[c.AgentID] = true
			usedProviders[c.Provider] = true
			usedModels[c.Model] = true
			jurors = append(jurors, c)
		}
	}
	sort.SliceStable(jurors, func(i, j int) bool {
		return jurors[i].AgentID < jurors[j].AgentID
	})

	if len(jurors) == 0 {
		return jurors, "none"
	}
	diversity := "provider"
	for _, j := range jurors {
		if j.Provider != doer.Provider {
			continue
		}
		if j.Model == doer.Model {
			return jurors, "none"
		}
		diversity = "model_only"
	}
	return jurors, diversity
}

// A jury checks an agent_judgment criterion with two or three agents from other runtimes
// than the doer's, each blind to the others (durable spec 8.2 and 8.3). A panel is one
// criterion of one outcome. Its jurors' checks are outcome_verification rows with
// verifier_type agent; the decided verdict is a jury check (unanimous) or a person's
// check (after a split or an escalation).

const (
	// juryTimeout is how long a panel waits for its votes before the criterion is unable.
	juryTimeout = 24 * time.Hour
	// juryPanelSize and juryUrgentPanelSize are the jurors of a panel; an urgent issue
	// gets three.
	juryPanelSize       = 2
	juryUrgentPanelSize = 3
	// juryAskReason is the attention reason of the question a split or escalated panel
	// puts to the person (spec 7.3).
	juryAskReason = "verification"
	// juryTrackPrefix names a juror's track record by the work class it checked
	// (spec 8.3): verify:<work class of the work>.
	juryTrackPrefix  = "verify:"
	juryTimeoutBatch = 500

	juryStateOpen      = "open"
	juryStateUnanimous = "unanimous"
	juryStateSplit     = "split"
	juryStateEscalated = "escalated"
	juryStateResolved  = "resolved"
	juryStateTimedOut  = "timed_out"
	verifierTypeJury   = "jury"
	juryUnableResult   = "unable"
)

var (
	ErrJuryPanelNotFound = errors.New("jury_panel_not_found")
	ErrJuryNotDecidable  = errors.New("jury_not_decidable")
)

// JuryAsker puts a panel that needs the person's decision to them, in the transaction
// that moves the panel. It is the attention ledger's question (spec 7.3), so it is nil
// until that ledger is wired; a nil asker leaves a split or escalated panel waiting.
type JuryAsker interface {
	Ask(ctx context.Context, q *db.Queries, panel db.JuryPanel, reason string) error
}

// JuryService opens the panels, takes the votes, times them out and records what a
// person decides about them.
type JuryService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Flags     *featureflag.Service
	// Tasks gives each juror its check. It is needed to open a panel with jurors.
	Tasks *TaskService
	Asker JuryAsker
	// Now is the clock for the timeout. Nil means time.Now.
	Now func() time.Time
}

// NewJuryService is the jury the server and the handlers share: it reads the pool from
// the workspace settings and gives jurors their checks through tasks. Its asker is
// wired with the attention ledger (spec 7.3) when that lands.
func NewJuryService(q *db.Queries, tx TxStarter, tasks *TaskService) *JuryService {
	return &JuryService{Queries: q, TxStarter: tx, Tasks: tasks}
}

func (s *JuryService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

type juryWorkspaceSettings struct {
	JuryPool        []string `json:"jury_pool"`
	JuryWorkClasses []string `json:"jury_work_classes"`
}

func (s *JuryService) settings(ctx context.Context, ws pgtype.UUID) (juryWorkspaceSettings, error) {
	var out juryWorkspaceSettings
	w, err := s.Queries.GetWorkspace(ctx, ws)
	if err != nil {
		return out, fmt.Errorf("read workspace: %w", err)
	}
	if len(w.Settings) > 0 {
		if err := json.Unmarshal(w.Settings, &out); err != nil {
			slog.Warn("durable jury: workspace settings are not an object", "workspace_id", util.UUIDToString(ws), "error", err)
			return juryWorkspaceSettings{}, nil
		}
	}
	return out, nil
}

// Eligible reports whether an agent_judgment criterion is checked by a jury rather than
// one verifier (spec 8.2). It needs the outcome checked in full or the criterion asking
// for it, a workspace with a jury pool, and important work: a work class the workspace
// lists, or an issue that is urgent or high.
func (s *JuryService) Eligible(ctx context.Context, o db.WorkOutcome, c db.AcceptanceCriterion) (bool, error) {
	if c.CheckKind != CheckKindAgentJudgment {
		return false, nil
	}
	if o.VerificationMode != VerificationModeFull && !juryRequested(c.CheckSpec) {
		return false, nil
	}
	settings, err := s.settings(ctx, o.WorkspaceID)
	if err != nil {
		return false, err
	}
	if len(settings.JuryPool) == 0 {
		return false, nil
	}
	for _, prefix := range settings.JuryWorkClasses {
		if prefix != "" && strings.HasPrefix(o.WorkClass, prefix) {
			return true, nil
		}
	}
	issue, err := s.Queries.GetIssue(ctx, o.IssueID)
	if err != nil {
		return false, fmt.Errorf("load issue: %w", err)
	}
	return issue.Priority == "urgent" || issue.Priority == "high", nil
}

// juryRequested reads check_spec.jury, which asks for a jury on one criterion.
func juryRequested(spec []byte) bool {
	var s struct {
		Jury bool `json:"jury"`
	}
	return len(spec) > 0 && json.Unmarshal(spec, &s) == nil && s.Jury
}

// Open gives a criterion its panel: the jurors PickJurors chooses from the pool, and a
// verification task for each. A pool with fewer than two jurors for the doer escalates
// the panel at once, to the person. Opening an existing panel does nothing.
func (s *JuryService) Open(ctx context.Context, o db.WorkOutcome, c db.AcceptanceCriterion) error {
	if _, err := s.Queries.GetJuryPanelForCriterion(ctx, db.GetJuryPanelForCriterionParams{
		WorkspaceID: o.WorkspaceID, OutcomeID: o.ID, CriterionID: c.ID,
	}); err == nil {
		return nil
	} else if !errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("load panel: %w", err)
	}
	issue, err := s.Queries.GetIssue(ctx, o.IssueID)
	if err != nil {
		return fmt.Errorf("load issue: %w", err)
	}
	size := juryPanelSize
	if issue.Priority == "urgent" {
		size = juryUrgentPanelSize
	}
	pool, err := s.candidates(ctx, o.WorkspaceID, issue, nil)
	if err != nil {
		return err
	}
	doer := JurorCandidate{AgentID: util.UUIDToString(o.AgentID), Provider: o.RuntimeProvider, Model: o.Model}
	picked, diversity := PickJurors(pool, doer, size)
	state := juryStateOpen
	if len(picked) < 2 {
		state = juryStateEscalated
	}
	jurors, err := juryIDs(picked)
	if err != nil {
		return err
	}
	panel := db.JuryPanel{
		ID: dbid.NewV7(), WorkspaceID: o.WorkspaceID, OutcomeID: o.ID, CriterionID: c.ID,
		JurorAgentIds: jurors, Diversity: diversity, State: state,
	}
	inserted := int64(0)
	err = s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.InsertJuryPanel(ctx, db.InsertJuryPanelParams{
			ID: panel.ID, WorkspaceID: panel.WorkspaceID, OutcomeID: panel.OutcomeID, CriterionID: panel.CriterionID,
			JurorAgentIds: jurors, Diversity: diversity, State: state,
		})
		if err != nil {
			return fmt.Errorf("insert panel: %w", err)
		}
		inserted = n
		if n == 1 && state == juryStateEscalated {
			return s.ask(ctx, q, panel)
		}
		return nil
	})
	if err != nil || inserted == 0 || state == juryStateEscalated {
		return err
	}
	for _, j := range jurors {
		if err := s.enqueueJuror(ctx, issue, o, panel.ID, j); err != nil {
			return err
		}
	}
	return nil
}

// OnVote takes one juror's check on a jury criterion. An unable check is replaced once,
// by the next eligible agent; a second unable, or no one left to replace the first,
// escalates the panel. When every juror still on the panel has voted, the panel is
// unanimous or split. A check on a criterion with no open panel is not a juror's and
// does nothing.
func (s *JuryService) OnVote(ctx context.Context, v db.OutcomeVerification) error {
	panel, err := s.Queries.GetJuryPanelForCriterion(ctx, db.GetJuryPanelForCriterionParams{
		WorkspaceID: v.WorkspaceID, OutcomeID: v.OutcomeID, CriterionID: v.CriterionID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load panel: %w", err)
	}
	if panel.State != juryStateOpen {
		return nil
	}
	votes, err := s.Queries.ListJuryVotesForPanel(ctx, db.ListJuryVotesForPanelParams{
		WorkspaceID: panel.WorkspaceID, OutcomeID: panel.OutcomeID, CriterionID: v.CriterionID,
	})
	if err != nil {
		return fmt.Errorf("list votes: %w", err)
	}
	if v.Result == juryUnableResult {
		unable := 0
		for _, vote := range votes {
			if vote.Result == juryUnableResult {
				unable++
			}
		}
		if unable == 1 {
			replaced, err := s.replaceJuror(ctx, panel)
			if err != nil {
				return err
			}
			if replaced {
				return nil
			}
		}
		return s.close(ctx, panel, juryStateEscalated)
	}
	return s.decide(ctx, panel, votes)
}

// replaceJuror adds the best eligible agent to a panel, if there is one. It reports
// false when no agent is left to take the place.
func (s *JuryService) replaceJuror(ctx context.Context, panel db.JuryPanel) (bool, error) {
	outcome, err := s.Queries.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: panel.OutcomeID, WorkspaceID: panel.WorkspaceID})
	if err != nil {
		return false, fmt.Errorf("load outcome: %w", err)
	}
	issue, err := s.Queries.GetIssue(ctx, outcome.IssueID)
	if err != nil {
		return false, fmt.Errorf("load issue: %w", err)
	}
	seated := make(map[string]bool, len(panel.JurorAgentIds))
	for _, j := range panel.JurorAgentIds {
		seated[util.UUIDToString(j)] = true
	}
	pool, err := s.candidates(ctx, panel.WorkspaceID, issue, seated)
	if err != nil {
		return false, err
	}
	doer := JurorCandidate{AgentID: util.UUIDToString(outcome.AgentID), Provider: outcome.RuntimeProvider, Model: outcome.Model}
	picked, _ := PickJurors(pool, doer, 1)
	if len(picked) == 0 {
		return false, nil
	}
	next, err := util.ParseUUID(picked[0].AgentID)
	if err != nil {
		return false, fmt.Errorf("replacement juror: %w", err)
	}
	n, err := s.Queries.AddJuryJuror(ctx, db.AddJuryJurorParams{ID: panel.ID, AgentID: next})
	if err != nil {
		return false, fmt.Errorf("add juror: %w", err)
	}
	if n == 0 {
		return false, nil
	}
	if err := s.enqueueJuror(ctx, issue, outcome, panel.ID, next); err != nil {
		return false, err
	}
	return true, nil
}

// decide settles a panel once every juror still on it has voted: unanimous writes the
// jury's check, a disagreement splits it and asks the person.
func (s *JuryService) decide(ctx context.Context, panel db.JuryPanel, votes []db.OutcomeVerification) error {
	gone := map[string]bool{}
	for _, v := range votes {
		if v.Result == juryUnableResult {
			gone[util.UUIDToString(v.VerifierID)] = true
		}
	}
	active := 0
	for _, j := range panel.JurorAgentIds {
		if !gone[util.UUIDToString(j)] {
			active++
		}
	}
	var counted []db.OutcomeVerification
	for _, v := range votes {
		if v.Result != juryUnableResult {
			counted = append(counted, v)
		}
	}
	if len(counted) == 0 || len(counted) < active {
		return nil
	}
	unanimous := true
	for _, v := range counted[1:] {
		if v.Result != counted[0].Result {
			unanimous = false
		}
	}
	if !unanimous {
		return s.close(ctx, panel, juryStateSplit)
	}
	return s.finishUnanimous(ctx, panel, counted[0].Result, len(counted))
}

// finishUnanimous moves an open panel to unanimous and writes the jury's check.
func (s *JuryService) finishUnanimous(ctx context.Context, panel db.JuryPanel, result string, jurors int) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.SetJuryPanelState(ctx, s.stateParams(panel, juryStateUnanimous, pgtype.Text{}, pgtype.Text{}, s.now()))
		if err != nil {
			return fmt.Errorf("set panel state: %w", err)
		}
		if n == 0 {
			return nil
		}
		_, err = q.InsertOutcomeVerification(ctx, verificationParams(panel.WorkspaceID, panel.OutcomeID, VerificationInput{
			CriterionID: panel.CriterionID, Result: result,
			Note: fmt.Sprintf("The jury of %d agreed.", jurors),
		}, verifierTypeJury, pgtype.UUID{}, pgtype.UUID{}))
		if err != nil {
			return fmt.Errorf("record jury check: %w", err)
		}
		return s.recordSplitRate(ctx, q, panel, false)
	})
}

// close moves an open panel to state. A split or an escalated panel asks the person in
// the same transaction; a timed-out one asks nobody, its criterion is unable.
func (s *JuryService) close(ctx context.Context, panel db.JuryPanel, state string) error {
	return s.inTx(ctx, func(q *db.Queries) error {
		n, err := q.SetJuryPanelState(ctx, s.stateParams(panel, state, pgtype.Text{}, pgtype.Text{}, s.now()))
		if err != nil {
			return fmt.Errorf("set panel state: %w", err)
		}
		if n == 0 {
			return nil
		}
		if state == juryStateSplit || state == juryStateEscalated {
			if err := s.ask(ctx, q, panel); err != nil {
				return err
			}
		}
		if state == juryStateSplit {
			return s.recordSplitRate(ctx, q, panel, true)
		}
		return nil
	})
}

func (s *JuryService) stateParams(panel db.JuryPanel, state string, verdict, resolvedBy pgtype.Text, at time.Time) db.SetJuryPanelStateParams {
	return db.SetJuryPanelStateParams{
		ID: panel.ID, State: state, Verdict: verdict, ResolvedByType: resolvedBy,
		ClosedAt:   pgtype.Timestamptz{Time: at, Valid: true},
		FromStates: []string{juryStateOpen},
	}
}

func (s *JuryService) ask(ctx context.Context, q *db.Queries, panel db.JuryPanel) error {
	if s.Asker == nil {
		return nil
	}
	if err := s.Asker.Ask(ctx, q, panel, juryAskReason); err != nil {
		return fmt.Errorf("ask the person about panel %s: %w", util.UUIDToString(panel.ID), err)
	}
	return nil
}

// recordSplitRate logs whether a panel split, for the jury.split_rate metric (spec 8.3),
// with the work class it checked. The rate is read from these logs per class.
func (s *JuryService) recordSplitRate(ctx context.Context, q *db.Queries, panel db.JuryPanel, split bool) error {
	outcome, err := q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: panel.OutcomeID, WorkspaceID: panel.WorkspaceID})
	if err != nil {
		return fmt.Errorf("load outcome: %w", err)
	}
	slog.Info("durable metric", "metric", "jury.split_rate", "work_class", outcome.WorkClass, "split", split)
	return nil
}

// Timeout times out the open panels older than a day (spec 8.3). Their criteria become
// unable; nobody is asked.
func (s *JuryService) Timeout(ctx context.Context) error {
	if s.Flags == nil || !featureflags.DurableSeatsEnabled(ctx, s.Flags) {
		return nil
	}
	panels, err := s.Queries.ListStaleOpenJuryPanels(ctx, db.ListStaleOpenJuryPanelsParams{
		Cutoff:   pgtype.Timestamptz{Time: s.now().Add(-juryTimeout), Valid: true},
		RowLimit: juryTimeoutBatch,
	})
	if err != nil {
		return fmt.Errorf("list stale panels: %w", err)
	}
	for _, p := range panels {
		if err := s.close(ctx, p, juryStateTimedOut); err != nil {
			slog.Error("durable jury: time out panel failed", "panel_id", util.UUIDToString(p.ID), "error", err)
		}
	}
	return nil
}

// Resolve records a person's verdict on a split or escalated panel. It is the check the
// criterion settles on, and it moves the track record of each juror who was overruled
// (spec 8.3): the juror's record for the work class it checked, verify:<work class>.
func (s *JuryService) Resolve(ctx context.Context, ws pgtype.UUID, actor GovernanceActor, panelID pgtype.UUID, verdict string) error {
	if verdict != "pass" && verdict != "partial" && verdict != "fail" {
		return ErrVerificationInvalid
	}
	return s.inTx(ctx, func(q *db.Queries) error {
		panel, err := q.GetJuryPanel(ctx, db.GetJuryPanelParams{ID: panelID, WorkspaceID: ws})
		if err != nil {
			return mapNoRows(err, ErrJuryPanelNotFound, "load panel")
		}
		outcome, err := q.GetWorkOutcomeInWorkspace(ctx, db.GetWorkOutcomeInWorkspaceParams{ID: panel.OutcomeID, WorkspaceID: ws})
		if err != nil {
			return mapNoRows(err, ErrOutcomeNotFound, "load outcome")
		}
		if outcome.SettledAt.Valid {
			return ErrOutcomeSettled
		}
		n, err := q.SetJuryPanelState(ctx, db.SetJuryPanelStateParams{
			ID: panel.ID, State: juryStateResolved, Verdict: pgtype.Text{String: verdict, Valid: true},
			ResolvedByType: pgtype.Text{String: actor.Type, Valid: true},
			ClosedAt:       pgtype.Timestamptz{Time: s.now(), Valid: true},
			FromStates:     []string{juryStateSplit, juryStateEscalated},
		})
		if err != nil {
			return fmt.Errorf("set panel state: %w", err)
		}
		if n == 0 {
			return ErrJuryNotDecidable
		}
		if _, err := q.InsertOutcomeVerification(ctx, verificationParams(ws, panel.OutcomeID, VerificationInput{
			CriterionID: panel.CriterionID, Result: verdict, Note: "Decided by a person after the jury.",
		}, verifierTypeMember, actor.ID, pgtype.UUID{})); err != nil {
			return fmt.Errorf("record decision: %w", err)
		}
		for _, juror := range panel.JurorAgentIds {
			if err := s.recordJurorTrack(ctx, q, ws, juror, outcome.WorkClass); err != nil {
				return err
			}
		}
		return nil
	})
}

// recordJurorTrack rewrites a juror's track record for verify:<work class> from the
// resolved panels it sat on, in both windows.
func (s *JuryService) recordJurorTrack(ctx context.Context, q *db.Queries, ws, juror pgtype.UUID, workClass string) error {
	now := s.now()
	for _, days := range trackWindowDays {
		c, err := q.CountJuryVotesForTrack(ctx, db.CountJuryVotesForTrackParams{
			WorkspaceID: ws, JurorAgentID: juror, WorkClass: workClass,
			Since: pgtype.Timestamptz{Time: now.Add(-time.Duration(days) * 24 * time.Hour), Valid: true},
		})
		if err != nil {
			return fmt.Errorf("count juror votes: %w", err)
		}
		if err := q.UpsertTrackRecordStats(ctx, db.UpsertTrackRecordStatsParams{
			WorkspaceID: ws, AgentID: juror, WorkClass: juryTrackPrefix + workClass, WindowDays: days,
			NPass: c.NPass, NFail: c.NFail,
			ScoreLower: WilsonLower(int(c.NPass), 0, int(c.NFail)),
		}); err != nil {
			return fmt.Errorf("write juror track: %w", err)
		}
	}
	return nil
}

// candidates reads the workspace's jury pool as the agents PickJurors chooses from. An
// agent is left out when it is archived, has no runtime, is already on the panel
// (skip), or has a pending task on the issue: it could not take the check now.
func (s *JuryService) candidates(ctx context.Context, ws pgtype.UUID, issue db.Issue, skip map[string]bool) ([]JurorCandidate, error) {
	settings, err := s.settings(ctx, ws)
	if err != nil {
		return nil, err
	}
	var out []JurorCandidate
	for _, raw := range settings.JuryPool {
		id, ok := parseAgentRef(raw)
		if !ok || skip[util.UUIDToString(id)] {
			continue
		}
		agent, err := s.Queries.GetAgent(ctx, id)
		if err != nil || agent.WorkspaceID != ws || agent.ArchivedAt.Valid || !agent.RuntimeID.Valid {
			continue
		}
		_, err = s.Queries.GetPendingAgentTaskOnIssue(ctx, db.GetPendingAgentTaskOnIssueParams{IssueID: issue.ID, AgentID: id})
		if err == nil {
			continue
		}
		if !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("check pending task: %w", err)
		}
		rt, err := s.Queries.GetAgentRuntime(ctx, agent.RuntimeID)
		if err != nil {
			continue
		}
		out = append(out, JurorCandidate{AgentID: util.UUIDToString(id), Provider: rt.Provider, Model: agent.Model.String})
	}
	return out, nil
}

// enqueueJuror gives one juror its verification task and marks it with the panel, so its
// check is accepted for that panel and no other.
func (s *JuryService) enqueueJuror(ctx context.Context, issue db.Issue, o db.WorkOutcome, panelID, juror pgtype.UUID) error {
	if s.Tasks == nil {
		return errors.New("durable jury: no task service to give a juror its check")
	}
	task, err := s.Tasks.EnqueueVerificationTask(ctx, issue, juror)
	if err != nil {
		return fmt.Errorf("enqueue juror task: %w", err)
	}
	n, err := s.Queries.SetJurorTaskContext(ctx, db.SetJurorTaskContextParams{
		ID: task.ID, OutcomeID: util.UUIDToString(o.ID), JuryPanelID: util.UUIDToString(panelID),
	})
	if err != nil {
		return fmt.Errorf("mark juror task: %w", err)
	}
	if n == 0 {
		return fmt.Errorf("juror task %s was claimed before it was marked", util.UUIDToString(task.ID))
	}
	return nil
}

func (s *JuryService) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
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

func juryIDs(picked []JurorCandidate) ([]pgtype.UUID, error) {
	out := make([]pgtype.UUID, 0, len(picked))
	for _, j := range picked {
		id, err := util.ParseUUID(j.AgentID)
		if err != nil {
			return nil, fmt.Errorf("juror id: %w", err)
		}
		out = append(out, id)
	}
	return out, nil
}

// juryPanelIDOf reads the panel a juror's task was given, "" for any other task.
func juryPanelIDOf(taskContext []byte) string {
	var m struct {
		JuryPanelID string `json:"jury_panel_id"`
	}
	if len(taskContext) == 0 || json.Unmarshal(taskContext, &m) != nil {
		return ""
	}
	return m.JuryPanelID
}

// authorizeJuror lets a juror check a criterion of its panel, once, while the panel is
// open and the juror is on it.
func authorizeJuror(ctx context.Context, q *db.Queries, ws pgtype.UUID, o db.WorkOutcome, criterion pgtype.UUID, actor GovernanceActor, panelID string) error {
	id, err := util.ParseUUID(panelID)
	if err != nil {
		return ErrNotVerificationTask
	}
	panel, err := q.GetJuryPanel(ctx, db.GetJuryPanelParams{ID: id, WorkspaceID: ws})
	if err != nil {
		return mapNoRows(err, ErrNotVerificationTask, "load panel")
	}
	if panel.OutcomeID != o.ID || panel.CriterionID != criterion || panel.State != juryStateOpen {
		return ErrNotVerificationTask
	}
	if !slices.Contains(panel.JurorAgentIds, actor.ID) {
		return ErrNotVerificationTask
	}
	votes, err := q.ListJuryVotesForPanel(ctx, db.ListJuryVotesForPanelParams{WorkspaceID: ws, OutcomeID: o.ID, CriterionID: criterion})
	if err != nil {
		return fmt.Errorf("list votes: %w", err)
	}
	for _, v := range votes {
		if v.VerifierID == actor.ID {
			return ErrCriterionChecked
		}
	}
	return nil
}
