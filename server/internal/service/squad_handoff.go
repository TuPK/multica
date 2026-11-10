package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
)

// Handoff between squads: when a squad has finished a request (the planning squad
// with its documents), it hands the work to another squad (the SDLC squad). The
// rule says to which squad and how: off (the boss hands off by hand), ask (the
// boss approves each handoff) or auto (it runs at once). The squad's lead asks for
// the handoff when its work is done; the platform decides from the rule, so the
// outcome never depends on the agent remembering a flag.

// Handoff modes, as the rule stores them.
const (
	HandoffModeOff  = "off"
	HandoffModeAsk  = "ask"
	HandoffModeAuto = "auto"
)

// Handoff statuses. HandoffStatusOff is the answer when no handoff applies; no row has it.
const (
	HandoffStatusOff       = "off"
	HandoffStatusPending   = "pending"
	HandoffStatusHandedOff = "handed_off"
	HandoffStatusDeclined  = "declined"
)

// The packet a planning lead posts on its request before it asks for the handoff
// (the planning-squad-lead skill, "Planning handoff packet"). An automatic handoff
// needs it, so the receiving squad starts from what the planning squad verified.
const (
	handoffPacketHeading = "## Planning handoff"
	handoffPacketVerdict = "Readiness verdict: PASS"
)

var (
	// ErrHandoffModeInvalid: the mode is not off, ask or auto.
	ErrHandoffModeInvalid = errors.New("mode must be off, ask or auto")
	// ErrHandoffTargetInvalid: the target squad is missing, archived, the source itself, or hands back to the source.
	ErrHandoffTargetInvalid = errors.New("the target must be another live squad that does not hand back to this one")
	// ErrHandoffSquadNotFound: the source squad is missing or archived.
	ErrHandoffSquadNotFound = errors.New("squad not found")
	// ErrHandoffNoRule: the request's squad has no handoff rule, so there is nobody to hand to.
	ErrHandoffNoRule = errors.New("this squad has no handoff target")
	// ErrHandoffNotAssigned: the request is not held by the squad the rule belongs to.
	ErrHandoffNotAssigned = errors.New("the request is not assigned to a squad")
	// ErrHandoffPacketMissing: an automatic or requested handoff needs the lead's handoff packet with a PASS verdict.
	ErrHandoffPacketMissing = errors.New("the request has no planning handoff packet with a PASS readiness verdict; post it first")
	// ErrHandoffNoProject: the target squad has no open project to receive the request in.
	ErrHandoffNoProject = errors.New("the target squad has no open project; assign one to it first")
	// ErrHandoffChained: the request itself came from a handoff, and a handoff does not hand on again.
	ErrHandoffChained = errors.New("this request came from a handoff; it is not handed on again")
	// ErrHandoffNothingPending: there is no pending handoff to decline.
	ErrHandoffNothingPending = errors.New("this request has no pending handoff")
	// ErrHandoffDone: the handoff has already run.
	ErrHandoffDone = errors.New("this request was already handed off")
)

// HandoffActor is who asks for or decides a handoff: a member or an agent.
type HandoffActor struct {
	Type string
	ID   pgtype.UUID
}

// HandoffResult is the answer to a request for a handoff.
type HandoffResult struct {
	// Status is off, pending, handed_off or declined.
	Status string
	// Reason says why a handoff is pending.
	Reason      string
	Handoff     *db.SquadHandoff
	TargetIssue *db.Issue
	TargetSquad *db.Squad
}

// SquadHandoffService reads and changes handoff rules and runs handoffs.
type SquadHandoffService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Issues    *IssueService
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

func (s *SquadHandoffService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// GetRule returns the squad's rule, or nil when it has none.
func (s *SquadHandoffService) GetRule(ctx context.Context, ws, squad pgtype.UUID) (*db.SquadHandoffRule, error) {
	rule, err := s.Queries.GetSquadHandoffRule(ctx, db.GetSquadHandoffRuleParams{WorkspaceID: ws, SourceSquadID: squad})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load handoff rule: %w", err)
	}
	return &rule, nil
}

// SetRule saves the squad's rule. The target must be another live squad that does not
// hand back to the source. Only the boss or an admin calls it; an agent never does.
func (s *SquadHandoffService) SetRule(ctx context.Context, ws, source, target pgtype.UUID, mode string, actor GovernanceActor) (db.SquadHandoffRule, error) {
	if mode != HandoffModeOff && mode != HandoffModeAsk && mode != HandoffModeAuto {
		return db.SquadHandoffRule{}, ErrHandoffModeInvalid
	}
	if actor.Type != GovernanceActorMember {
		return db.SquadHandoffRule{}, ErrGovernanceActorNotBoss
	}
	src, err := s.liveSquad(ctx, ws, source)
	if err != nil {
		return db.SquadHandoffRule{}, ErrHandoffSquadNotFound
	}
	if util.UUIDToString(source) == util.UUIDToString(target) {
		return db.SquadHandoffRule{}, ErrHandoffTargetInvalid
	}
	dst, err := s.liveSquad(ctx, ws, target)
	if err != nil {
		return db.SquadHandoffRule{}, ErrHandoffTargetInvalid
	}
	if back, err := s.GetRule(ctx, ws, target); err != nil {
		return db.SquadHandoffRule{}, err
	} else if back != nil && util.UUIDToString(back.TargetSquadID) == util.UUIDToString(source) {
		return db.SquadHandoffRule{}, ErrHandoffTargetInvalid
	}

	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return db.SquadHandoffRule{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	rule, err := q.UpsertSquadHandoffRule(ctx, db.UpsertSquadHandoffRuleParams{
		WorkspaceID: ws, SourceSquadID: source, TargetSquadID: target, Mode: mode, UpdatedBy: actor.ID,
	})
	if err != nil {
		return db.SquadHandoffRule{}, fmt.Errorf("save handoff rule: %w", err)
	}
	if err := NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: ws,
		Kind:        GovernanceKindSquadHandoff,
		ActorType:   actor.Type,
		ActorID:     actor.ID,
		SubjectType: "squad",
		SubjectID:   source,
		Summary:     fmt.Sprintf("%s handoff to %s set to %s", src.Name, dst.Name, mode),
		Details:     map[string]any{"event": "rule_set", "mode": mode, "target_squad_id": util.UUIDToString(target)},
	}); err != nil {
		return db.SquadHandoffRule{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.SquadHandoffRule{}, fmt.Errorf("commit: %w", err)
	}
	return rule, nil
}

// ClearRule removes the squad's rule.
func (s *SquadHandoffService) ClearRule(ctx context.Context, ws, source pgtype.UUID, actor GovernanceActor) error {
	if actor.Type != GovernanceActorMember {
		return ErrGovernanceActorNotBoss
	}
	rule, err := s.GetRule(ctx, ws, source)
	if err != nil || rule == nil {
		return err
	}
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	if err := q.DeleteSquadHandoffRule(ctx, db.DeleteSquadHandoffRuleParams{WorkspaceID: ws, SourceSquadID: source}); err != nil {
		return fmt.Errorf("delete handoff rule: %w", err)
	}
	if err := NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: ws, Kind: GovernanceKindSquadHandoff, ActorType: actor.Type, ActorID: actor.ID,
		SubjectType: "squad", SubjectID: source,
		Summary: "Handoff target removed", Details: map[string]any{"event": "rule_cleared"},
	}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// Get returns the handoff of a request, or nil when there is none.
func (s *SquadHandoffService) Get(ctx context.Context, ws, issue pgtype.UUID) (*db.SquadHandoff, error) {
	row, err := s.Queries.GetSquadHandoffForIssue(ctx, db.GetSquadHandoffForIssueParams{WorkspaceID: ws, SourceIssueID: issue})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load handoff: %w", err)
	}
	return &row, nil
}

// Request is what the squad's lead asks for when its work is done. It follows the
// rule: off answers off, ask files a pending handoff for the boss, and auto hands
// off at once unless the company is paused, when it stays pending. A request that
// was already handled gets its earlier answer back, so asking twice is harmless.
func (s *SquadHandoffService) Request(ctx context.Context, ws pgtype.UUID, issue db.Issue, actor HandoffActor) (HandoffResult, error) {
	source, err := assignedSquad(issue)
	if err != nil {
		return HandoffResult{}, err
	}
	rule, err := s.GetRule(ctx, ws, source)
	if err != nil {
		return HandoffResult{}, err
	}
	if rule == nil || rule.Mode == HandoffModeOff {
		return HandoffResult{Status: HandoffStatusOff}, nil
	}
	if existing, err := s.Get(ctx, ws, issue.ID); err != nil {
		return HandoffResult{}, err
	} else if existing != nil {
		return s.resultOf(ctx, ws, *existing, "")
	}
	if err := s.checkNotChained(ctx, ws, issue); err != nil {
		return HandoffResult{}, err
	}
	if _, err := s.packetOf(ctx, ws, issue); err != nil {
		return HandoffResult{}, err
	}
	row, filed, err := s.fileHandoff(ctx, ws, issue, source, *rule, actor)
	if err != nil {
		return HandoffResult{}, err
	}
	if !filed {
		// Another caller filed it first and answers for it.
		return s.resultOf(ctx, ws, row, "")
	}
	if rule.Mode == HandoffModeAsk {
		return s.resultOf(ctx, ws, row, "waiting for the boss to approve")
	}
	verdict, err := CompanyVerdict(ctx, s.Queries, ws, RunClassDelegated, s.now())
	if err != nil {
		return HandoffResult{}, err
	}
	if !verdict.Admit {
		return s.resultOf(ctx, ws, row, "the company is paused; approve the handoff when it runs again")
	}
	res, err := s.perform(ctx, ws, issue, row, actor, pgtype.UUID{})
	if err != nil {
		// The automatic handoff could not run (no project, or the create failed): withdraw the row
		// so the lead's next request tries again, instead of finding a pending handoff nobody decides.
		_ = s.Queries.DeletePendingSquadHandoff(ctx, db.DeletePendingSquadHandoffParams{WorkspaceID: ws, ID: row.ID})
		return HandoffResult{}, err
	}
	return res, nil
}

// HandOff runs the handoff now, as the boss's decision: it approves a pending
// handoff, or hands a request off by hand when the rule is off. It needs a rule
// for the target, and it does not need the packet.
func (s *SquadHandoffService) HandOff(ctx context.Context, ws pgtype.UUID, issue db.Issue, actor HandoffActor) (HandoffResult, error) {
	if actor.Type != GovernanceActorMember {
		return HandoffResult{}, ErrGovernanceActorNotBoss
	}
	source, err := assignedSquad(issue)
	if err != nil {
		return HandoffResult{}, err
	}
	rule, err := s.GetRule(ctx, ws, source)
	if err != nil {
		return HandoffResult{}, err
	}
	if rule == nil {
		return HandoffResult{}, ErrHandoffNoRule
	}
	existing, err := s.Get(ctx, ws, issue.ID)
	if err != nil {
		return HandoffResult{}, err
	}
	if existing != nil && existing.Status == HandoffStatusHandedOff {
		return s.resultOf(ctx, ws, *existing, "")
	}
	if err := s.checkNotChained(ctx, ws, issue); err != nil {
		return HandoffResult{}, err
	}
	var row db.SquadHandoff
	if existing != nil {
		row, err = s.Queries.RetargetPendingSquadHandoff(ctx, db.RetargetPendingSquadHandoffParams{
			WorkspaceID: ws, ID: existing.ID, TargetSquadID: rule.TargetSquadID, Mode: rule.Mode,
		})
		if err != nil {
			return HandoffResult{}, fmt.Errorf("reopen handoff: %w", err)
		}
	} else if row, _, err = s.fileHandoff(ctx, ws, issue, source, *rule, actor); err != nil {
		return HandoffResult{}, err
	}
	return s.perform(ctx, ws, issue, row, actor, actor.ID)
}

// Decline closes a pending handoff without running it, as the boss's decision.
func (s *SquadHandoffService) Decline(ctx context.Context, ws pgtype.UUID, issue db.Issue, actor HandoffActor, note string) (HandoffResult, error) {
	if actor.Type != GovernanceActorMember {
		return HandoffResult{}, ErrGovernanceActorNotBoss
	}
	existing, err := s.Get(ctx, ws, issue.ID)
	if err != nil {
		return HandoffResult{}, err
	}
	if existing == nil || existing.Status != HandoffStatusPending {
		return HandoffResult{}, ErrHandoffNothingPending
	}
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	row, err := q.SettleSquadHandoff(ctx, db.SettleSquadHandoffParams{
		WorkspaceID: ws, ID: existing.ID, Status: HandoffStatusDeclined, DecidedBy: actor.ID, Note: strings.TrimSpace(note),
	})
	if err != nil {
		return HandoffResult{}, fmt.Errorf("decline handoff: %w", err)
	}
	if err := NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: ws, Kind: GovernanceKindSquadHandoff, ActorType: actor.Type, ActorID: actor.ID,
		SubjectType: "squad", SubjectID: row.SourceSquadID,
		Summary: fmt.Sprintf("Handoff of %q declined", issue.Title),
		Details: map[string]any{"event": "declined", "source_issue_id": util.UUIDToString(issue.ID)},
	}); err != nil {
		return HandoffResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return HandoffResult{}, fmt.Errorf("commit: %w", err)
	}
	return s.resultOf(ctx, ws, row, "")
}

// fileHandoff inserts the request's pending row and reports true. A request filed at the
// same time by another caller returns that caller's row and false.
func (s *SquadHandoffService) fileHandoff(ctx context.Context, ws pgtype.UUID, issue db.Issue, source pgtype.UUID, rule db.SquadHandoffRule, actor HandoffActor) (db.SquadHandoff, bool, error) {
	row, err := s.Queries.InsertSquadHandoff(ctx, db.InsertSquadHandoffParams{
		WorkspaceID: ws, SourceSquadID: source, TargetSquadID: rule.TargetSquadID, SourceIssueID: issue.ID,
		Mode: rule.Mode, RequestedByType: actor.Type, RequestedByID: actor.ID,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		existing, gerr := s.Get(ctx, ws, issue.ID)
		if gerr != nil || existing == nil {
			return db.SquadHandoff{}, false, fmt.Errorf("load handoff: %w", errors.Join(gerr, err))
		}
		return *existing, false, nil
	}
	if err != nil {
		return db.SquadHandoff{}, false, fmt.Errorf("file handoff: %w", err)
	}
	return row, true, nil
}

// perform creates the request in the target squad's project and settles the row.
func (s *SquadHandoffService) perform(ctx context.Context, ws pgtype.UUID, source db.Issue, row db.SquadHandoff, actor HandoffActor, decidedBy pgtype.UUID) (HandoffResult, error) {
	if row.Status == HandoffStatusHandedOff {
		return s.resultOf(ctx, ws, row, "")
	}
	target, err := s.liveSquad(ctx, ws, row.TargetSquadID)
	if err != nil {
		return HandoffResult{}, ErrHandoffTargetInvalid
	}
	project, err := s.Queries.GetSquadOpenProject(ctx, db.GetSquadOpenProjectParams{WorkspaceID: ws, SquadID: target.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return HandoffResult{}, ErrHandoffNoProject
	}
	if err != nil {
		return HandoffResult{}, fmt.Errorf("load the target squad's project: %w", err)
	}
	packet, _ := s.packetOf(ctx, ws, source)

	creatorType := "member"
	if actor.Type == GovernanceActorAgent {
		creatorType = "agent"
	}
	created, err := s.Issues.Create(ctx, IssueCreateParams{
		WorkspaceID:    ws,
		Title:          "[Planning handoff] " + source.Title,
		Description:    pgtype.Text{String: handoffDescription(source, packet), Valid: true},
		Status:         "todo",
		Priority:       source.Priority,
		AssigneeType:   pgtype.Text{String: "squad", Valid: true},
		AssigneeID:     target.ID,
		CreatorType:    creatorType,
		CreatorID:      actor.ID,
		ProjectID:      project.ID,
		OriginType:     pgtype.Text{String: "squad_request", Valid: true},
		OriginID:       source.ID,
		AllowDuplicate: true,
	}, IssueCreateOpts{ActorID: util.UUIDToString(actor.ID)})
	if err != nil {
		return HandoffResult{}, fmt.Errorf("create the target squad's request: %w", err)
	}

	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return HandoffResult{}, fmt.Errorf("begin tx: %w", err)
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	settled, err := q.SettleSquadHandoff(ctx, db.SettleSquadHandoffParams{
		WorkspaceID: ws, ID: row.ID, Status: HandoffStatusHandedOff, TargetIssueID: created.Issue.ID, DecidedBy: decidedBy,
	})
	if err != nil {
		return HandoffResult{}, fmt.Errorf("settle handoff: %w", err)
	}
	if _, err := q.CreateComment(ctx, db.CreateCommentParams{
		ID: dbid.NewV7(), IssueID: source.ID, WorkspaceID: ws, AuthorType: actor.Type, AuthorID: actor.ID,
		Content: fmt.Sprintf("Handed off to %s: [%s](mention://issue/%s).", target.Name, created.Issue.Title, util.UUIDToString(created.Issue.ID)),
		Type:    "comment",
	}); err != nil {
		return HandoffResult{}, fmt.Errorf("post the handoff note: %w", err)
	}
	if err := NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: ws, Kind: GovernanceKindSquadHandoff, ActorType: actor.Type, ActorID: actor.ID,
		SubjectType: "squad", SubjectID: row.SourceSquadID,
		Summary: fmt.Sprintf("Handed %q to %s (%s)", source.Title, target.Name, row.Mode),
		Details: map[string]any{
			"event": "handed_off", "mode": row.Mode,
			"source_issue_id": util.UUIDToString(source.ID), "target_issue_id": util.UUIDToString(created.Issue.ID),
			"target_squad_id": util.UUIDToString(target.ID),
		},
	}); err != nil {
		return HandoffResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return HandoffResult{}, fmt.Errorf("commit: %w", err)
	}
	issue := created.Issue
	return HandoffResult{Status: HandoffStatusHandedOff, Handoff: &settled, TargetIssue: &issue, TargetSquad: &target}, nil
}

// resultOf reads a row as a result, with the target issue and squad when it has them.
func (s *SquadHandoffService) resultOf(ctx context.Context, ws pgtype.UUID, row db.SquadHandoff, reason string) (HandoffResult, error) {
	out := HandoffResult{Status: row.Status, Reason: reason, Handoff: &row}
	if squad, err := s.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: row.TargetSquadID, WorkspaceID: ws}); err == nil {
		out.TargetSquad = &squad
	}
	if row.TargetIssueID.Valid {
		if issue, err := s.Queries.GetIssueInWorkspace(ctx, db.GetIssueInWorkspaceParams{ID: row.TargetIssueID, WorkspaceID: ws}); err == nil {
			out.TargetIssue = &issue
		}
	}
	return out, nil
}

func (s *SquadHandoffService) liveSquad(ctx context.Context, ws, id pgtype.UUID) (db.Squad, error) {
	squad, err := s.Queries.GetSquadInWorkspace(ctx, db.GetSquadInWorkspaceParams{ID: id, WorkspaceID: ws})
	if err != nil {
		return db.Squad{}, err
	}
	if squad.ArchivedAt.Valid {
		return db.Squad{}, pgx.ErrNoRows
	}
	return squad, nil
}

// checkNotChained refuses a request that a handoff created.
func (s *SquadHandoffService) checkNotChained(ctx context.Context, ws pgtype.UUID, issue db.Issue) error {
	_, err := s.Queries.GetSquadHandoffByTargetIssue(ctx, db.GetSquadHandoffByTargetIssueParams{WorkspaceID: ws, TargetIssueID: issue.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("check handoff chain: %w", err)
	}
	return ErrHandoffChained
}

// packetOf returns the latest comment that carries the handoff packet with a PASS verdict.
func (s *SquadHandoffService) packetOf(ctx context.Context, ws pgtype.UUID, issue db.Issue) (string, error) {
	comments, err := s.Queries.ListCommentsForIssue(ctx, db.ListCommentsForIssueParams{IssueID: issue.ID, WorkspaceID: ws, Limit: 200})
	if err != nil {
		return "", fmt.Errorf("list comments: %w", err)
	}
	for i := len(comments) - 1; i >= 0; i-- {
		c := comments[i].Content
		if strings.Contains(c, handoffPacketHeading) && strings.Contains(c, handoffPacketVerdict) {
			return c, nil
		}
	}
	return "", ErrHandoffPacketMissing
}

// assignedSquad returns the squad a request is assigned to.
func assignedSquad(issue db.Issue) (pgtype.UUID, error) {
	if !issue.AssigneeType.Valid || issue.AssigneeType.String != "squad" || !issue.AssigneeID.Valid {
		return pgtype.UUID{}, ErrHandoffNotAssigned
	}
	return issue.AssigneeID, nil
}

// handoffDescription is the receiving squad's request: where it came from and the packet.
func handoffDescription(source db.Issue, packet string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Planning handoff from [%s](mention://issue/%s).\n\n", source.Title, util.UUIDToString(source.ID))
	b.WriteString("The planning squad finished this request, and its independent readiness review passed. Plan the build from its documents and queue the work as you always do. This run is unattended. Do not wait for approval; take the recommended default and record it as an assumption.\n\n")
	if strings.TrimSpace(packet) == "" {
		b.WriteString("No handoff packet was posted. Read the request's comments and the planning documents in the repository.\n")
	} else {
		b.WriteString(packet)
		b.WriteString("\n")
	}
	return b.String()
}
