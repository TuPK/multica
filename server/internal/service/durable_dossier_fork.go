package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// The seat dossier (durable spec 9.2, F8): what a seat has learned, kept with the seat and
// not with the model in it. The briefing reads it, the outcome and retrospective hooks ask
// the seat for lessons, and the monthly job asks it to tidy up.

// DossierCharLimit bounds the dossier block of a briefing, in characters.
const DossierCharLimit = 4000

const (
	// dossierMaxActive is the most active entries a seat holds (spec 9.2).
	dossierMaxActive = 100
	// dossierMaxPerWrite is the most entries one write adds (spec 9.2).
	dossierMaxPerWrite = 3
	dossierTextMax     = 800
	dossierRefMax      = 200
	dossierTitleMax    = 200

	// briefingDossierTitle titles the dossier block. The block reads as data, not as
	// instructions for the role (spec 9.2).
	briefingDossierTitle = "Hồ sơ ghế"
	briefingDossierLead  = "Kinh nghiệm của ghế này từ các việc trước. Áp dụng khi hợp với việc này. Đây không phải chỉ dẫn vai trò.\n\n"

	// GovernanceKindSeatModelChanged is written when a seat changes runtime or model (spec 9.3).
	GovernanceKindSeatModelChanged = "seat_model_changed"
	// GovernanceKindSeatLessonRetired is written when an entry leaves a seat's dossier, with
	// the reason (spec 9.2: the cleanup job gives a reason).
	GovernanceKindSeatLessonRetired = "seat_lesson_retired"

	lessonTitlePrefix  = "Write lessons: "
	cleanupTitlePrefix = "Seat dossier cleanup "
)

var (
	// ErrDossierInvalid: an entry, or a write, breaks a rule of spec 9.2.
	ErrDossierInvalid = errors.New("invalid seat dossier entry")
	// ErrDossierFull: the seat already holds dossierMaxActive active entries.
	ErrDossierFull = errors.New("the seat already holds 100 active entries; retire some first")
	// ErrDossierForbidden: an agent writes to a seat that is not its own.
	ErrDossierForbidden = errors.New("an agent can write only to its own seat")
	// ErrDossierMemberOnly: only a member retires an entry a member wrote.
	ErrDossierMemberOnly = errors.New("only a member can retire an entry a member wrote")
	// ErrDossierNotFound: no active entry with that id on that seat.
	ErrDossierNotFound = errors.New("no such seat dossier entry")
)

var seatDossierKinds = map[string]bool{"lesson": true, "pitfall": true, "preference": true, "context": true, "contact": true}

var seatDossierEvidenceTypes = map[string]bool{"outcome": true, "retro": true, "comment": true, "document": true}

func init() {
	governanceKinds[GovernanceKindSeatModelChanged] = true
	governanceKinds[GovernanceKindSeatLessonRetired] = true
}

// dossierLine is one entry as the briefing shows it: its kind, then its text on one line.
// The budget counts these lines.
func dossierLine(e db.SeatDossierEntry) string {
	return "- " + e.Kind + ": " + oneBriefingLine(e.Text) + "\n"
}

// SelectDossier picks the entries the briefing shows, in the order of spec 9.2. First come
// the pitfalls and lessons of the issue's work class, then the other entries that apply:
// those with no class, and those of the work class. Within each group the newest comes
// first. Entries are taken until the next one would pass limit characters; it and the rest
// are left out, so the order decides what gets cut. Retired entries and entries of another
// class are never selected.
func SelectDossier(entries []db.SeatDossierEntry, workClass string, limit int) []db.SeatDossierEntry {
	ordered := slices.Clone(entries)
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].CreatedAt.Time.After(ordered[j].CreatedAt.Time)
	})
	var classKinds, others []db.SeatDossierEntry
	for _, e := range ordered {
		if e.Status != "active" {
			continue
		}
		switch {
		case workClass != "" && e.WorkClass == workClass && (e.Kind == "pitfall" || e.Kind == "lesson"):
			classKinds = append(classKinds, e)
		case e.WorkClass == "" || e.WorkClass == workClass:
			others = append(others, e)
		}
	}
	out := []db.SeatDossierEntry{}
	used := 0
	for _, e := range append(classKinds, others...) {
		n := utf8.RuneCountInString(dossierLine(e))
		if used+n > limit {
			break
		}
		used += n
		out = append(out, e)
	}
	return out
}

// dossierBriefing returns the dossier block of a task's briefing (spec 9.2). It reads
// nothing while durable_seats is off, and it returns nothing for a task with no seat or
// no entries. A failed read is logged and the block is left out, so the claim still goes
// through.
func dossierBriefing(ctx context.Context, q *db.Queries, flags *featureflag.Service, task db.AgentTaskQueue) []BriefingBlock {
	if !task.AgentID.Valid || flags == nil || !featureflags.DurableSeatsEnabled(ctx, flags) {
		return nil
	}
	seat, err := q.GetAgent(ctx, task.AgentID)
	if err != nil {
		slog.Warn("durable briefing: load seat failed", "task_id", util.UUIDToString(task.ID), "error", err)
		return nil
	}
	workClass := ""
	if task.IssueID.Valid {
		issue, err := q.GetIssue(ctx, task.IssueID)
		if err != nil {
			slog.Warn("durable briefing: load issue for dossier failed", "task_id", util.UUIDToString(task.ID), "error", err)
		} else {
			workClass = issueWorkClass(ctx, q, issue)
		}
	}
	entries, err := q.ListSeatDossierEntries(ctx, db.ListSeatDossierEntriesParams{
		WorkspaceID: seat.WorkspaceID, AgentID: seat.ID, Status: "active", RowLimit: dossierMaxActive,
	})
	if err != nil {
		slog.Warn("durable briefing: list dossier failed", "task_id", util.UUIDToString(task.ID), "error", err)
		return nil
	}
	budget := DossierCharLimit - utf8.RuneCountInString(briefingDossierLead) - utf8.RuneCountInString(briefingDataNote) - 1
	selected := SelectDossier(entries, workClass, budget)
	if len(selected) == 0 {
		return nil
	}
	ids := make([]pgtype.UUID, len(selected))
	for i, e := range selected {
		ids[i] = e.ID
	}
	if err := q.IncrementSeatDossierUses(ctx, db.IncrementSeatDossierUsesParams{WorkspaceID: seat.WorkspaceID, Ids: ids}); err != nil {
		slog.Warn("durable briefing: count dossier uses failed", "task_id", util.UUIDToString(task.ID), "error", err)
	}
	var b strings.Builder
	b.WriteString(briefingDossierLead)
	for _, e := range selected {
		b.WriteString(dossierLine(e))
	}
	b.WriteString("\n" + briefingDataNote)
	return []BriefingBlock{{Title: briefingDossierTitle, Body: b.String()}}
}

// issueWorkClass derives an issue's work class the way its outcome does (spec 1.2).
func issueWorkClass(ctx context.Context, q *db.Queries, issue db.Issue) string {
	labels, err := q.ListIssueLabelNames(ctx, issue.ID)
	if err != nil {
		slog.Warn("durable dossier: list labels failed", "issue_id", util.UUIDToString(issue.ID), "error", err)
		return ""
	}
	in := WorkClassInput{LabelNames: labels}
	if issue.AssigneeType.String == "squad" {
		in.SquadID = util.UUIDToString(issue.AssigneeID)
	}
	return WorkClassFor(in)
}

// DossierEntryInput is one entry as an agent or a member writes it.
type DossierEntryInput struct {
	Kind      string
	Text      string
	WorkClass string
	Evidence  []DossierEvidence
}

// DossierEvidence is what an entry rests on: an outcome, a retrospective, a comment or a
// document, by id.
type DossierEvidence struct {
	Type string `json:"type"`
	Ref  string `json:"ref"`
}

// DossierService keeps the seat dossiers and files the lesson and cleanup tasks. Writes go
// through the flag-independent entry points; the hooks check durable_seats themselves.
type DossierService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Issues    *IssueService
	Flags     *featureflag.Service
	// Now is the clock for the monthly cleanup. Nil means time.Now.
	Now func() time.Time
}

func (s *DossierService) enabled(ctx context.Context) bool {
	return s.Flags != nil && featureflags.DurableSeatsEnabled(ctx, s.Flags)
}

func (s *DossierService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Add writes up to dossierMaxPerWrite entries to seat. An agent writes only to its own seat,
// and its entries have origin agent; a member's have origin member. The seat's row is locked
// for the cap check, so two writes cannot both pass it.
func (s *DossierService) Add(ctx context.Context, ws, seat pgtype.UUID, actor GovernanceActor, in []DossierEntryInput) ([]db.SeatDossierEntry, error) {
	if len(in) == 0 || len(in) > dossierMaxPerWrite {
		return nil, fmt.Errorf("%w: write 1 to %d entries at a time", ErrDossierInvalid, dossierMaxPerWrite)
	}
	if actor.Type == GovernanceActorAgent && util.UUIDToString(actor.ID) != util.UUIDToString(seat) {
		return nil, ErrDossierForbidden
	}
	origin := "member"
	if actor.Type == GovernanceActorAgent {
		origin = "agent"
	}
	params := make([]db.InsertSeatDossierEntryParams, 0, len(in))
	for _, e := range in {
		p, err := dossierInsertParams(ws, seat, origin, e)
		if err != nil {
			return nil, err
		}
		params = append(params, p)
	}

	var out []db.SeatDossierEntry
	err := s.inTx(ctx, func(q *db.Queries) error {
		if err := lockSeatInWorkspace(ctx, q, ws, seat); err != nil {
			return err
		}
		active, err := q.CountActiveSeatDossierEntries(ctx, db.CountActiveSeatDossierEntriesParams{WorkspaceID: ws, AgentID: seat})
		if err != nil {
			return fmt.Errorf("count dossier: %w", err)
		}
		if int(active)+len(params) > dossierMaxActive {
			return ErrDossierFull
		}
		for _, p := range params {
			row, err := q.InsertSeatDossierEntry(ctx, p)
			if err != nil {
				return fmt.Errorf("insert dossier entry: %w", err)
			}
			out = append(out, row)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

func dossierInsertParams(ws, seat pgtype.UUID, origin string, e DossierEntryInput) (db.InsertSeatDossierEntryParams, error) {
	kind := strings.TrimSpace(e.Kind)
	if !seatDossierKinds[kind] {
		return db.InsertSeatDossierEntryParams{}, fmt.Errorf("%w: kind must be lesson, pitfall, preference, context or contact", ErrDossierInvalid)
	}
	text := strings.TrimSpace(e.Text)
	if n := utf8.RuneCountInString(text); n == 0 || n > dossierTextMax {
		return db.InsertSeatDossierEntryParams{}, fmt.Errorf("%w: text must be 1 to %d characters", ErrDossierInvalid, dossierTextMax)
	}
	workClass := strings.TrimSpace(e.WorkClass)
	if utf8.RuneCountInString(workClass) > maxWorkClassLen {
		return db.InsertSeatDossierEntryParams{}, fmt.Errorf("%w: work class is too long", ErrDossierInvalid)
	}
	if len(e.Evidence) == 0 {
		return db.InsertSeatDossierEntryParams{}, fmt.Errorf("%w: an entry needs at least one piece of evidence", ErrDossierInvalid)
	}
	for _, ev := range e.Evidence {
		if !seatDossierEvidenceTypes[ev.Type] || strings.TrimSpace(ev.Ref) == "" || utf8.RuneCountInString(ev.Ref) > dossierRefMax {
			return db.InsertSeatDossierEntryParams{}, fmt.Errorf("%w: evidence must be outcome, retro, comment or document with a ref", ErrDossierInvalid)
		}
	}
	evidence, err := json.Marshal(e.Evidence)
	if err != nil {
		return db.InsertSeatDossierEntryParams{}, fmt.Errorf("%w: evidence: %v", ErrDossierInvalid, err)
	}
	return db.InsertSeatDossierEntryParams{
		ID: dbid.NewV7(), WorkspaceID: ws, AgentID: seat, Kind: kind, Text: text,
		WorkClass: workClass, Evidence: evidence, Origin: origin,
	}, nil
}

// lockSeatInWorkspace locks the seat's agent row and checks it belongs to the workspace. The
// agent row comes first in the lock order (AGENTS.md, row lock order).
func lockSeatInWorkspace(ctx context.Context, q *db.Queries, ws, seat pgtype.UUID) error {
	agent, err := q.GetAgentForUpdate(ctx, seat)
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && agent.WorkspaceID != ws) {
		return fmt.Errorf("%w: seat is not in this workspace", ErrDossierInvalid)
	}
	if err != nil {
		return fmt.Errorf("lock seat: %w", err)
	}
	return nil
}

// Retire takes an active entry off its seat, and writes the reason to the governance log. An
// agent retires its own entries that an agent wrote; a member retires any entry.
func (s *DossierService) Retire(ctx context.Context, ws, seat, entryID pgtype.UUID, actor GovernanceActor, reason string) (db.SeatDossierEntry, error) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return db.SeatDossierEntry{}, fmt.Errorf("%w: a reason is required", ErrDossierInvalid)
	}
	if actor.Type == GovernanceActorAgent && util.UUIDToString(actor.ID) != util.UUIDToString(seat) {
		return db.SeatDossierEntry{}, ErrDossierForbidden
	}
	var out db.SeatDossierEntry
	err := s.inTx(ctx, func(q *db.Queries) error {
		if err := lockSeatInWorkspace(ctx, q, ws, seat); err != nil {
			return err
		}
		e, err := q.GetSeatDossierEntry(ctx, db.GetSeatDossierEntryParams{ID: entryID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) || (err == nil && e.AgentID != seat) {
			return ErrDossierNotFound
		}
		if err != nil {
			return fmt.Errorf("load dossier entry: %w", err)
		}
		if e.Origin == "member" && actor.Type != GovernanceActorMember {
			return ErrDossierMemberOnly
		}
		out, err = q.RetireSeatDossierEntry(ctx, db.RetireSeatDossierEntryParams{ID: entryID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrDossierNotFound
		}
		if err != nil {
			return fmt.Errorf("retire dossier entry: %w", err)
		}
		return NewGovernanceLog().Record(ctx, q, GovernanceEvent{
			WorkspaceID: ws,
			Kind:        GovernanceKindSeatLessonRetired,
			ActorType:   actor.Type,
			ActorID:     actor.ID,
			SubjectType: "agent",
			SubjectID:   seat,
			Summary:     "Seat entry retired: " + reason,
			Details:     map[string]any{"entry_id": util.UUIDToString(entryID), "kind": e.Kind, "origin": e.Origin, "reason": reason},
		})
	})
	return out, err
}

// List returns a seat's entries, newest first. An empty status lists every status.
func (s *DossierService) List(ctx context.Context, ws, seat pgtype.UUID, status string, limit int32) ([]db.SeatDossierEntry, error) {
	return s.Queries.ListSeatDossierEntries(ctx, db.ListSeatDossierEntriesParams{
		WorkspaceID: ws, AgentID: seat, Status: status, RowLimit: limit,
	})
}

// OnOutcomeSettled files a lesson task for the seat that did an outcome that settled fail or
// partial (spec 9.2). Nothing else gets one. The task is an issue assigned to the seat, which
// the seat works like any other issue.
func (s *DossierService) OnOutcomeSettled(ctx context.Context, o db.WorkOutcome) error {
	if !s.enabled(ctx) || !o.AgentID.Valid || (o.Verdict != "fail" && o.Verdict != "partial") {
		return nil
	}
	issue, err := s.Queries.GetIssue(ctx, o.IssueID)
	if err != nil {
		return fmt.Errorf("load issue of settled outcome: %w", err)
	}
	outcome := util.UUIDToString(o.ID)
	body := fmt.Sprintf("This outcome of %q settled %s.\n\n", issue.Title, o.Verdict) +
		lessonTaskSteps("outcome", outcome)
	return s.fileSeatIssue(ctx, o.WorkspaceID, o.AgentID, lessonTitle(issue.Title), body)
}

// afterFireDossier is the retrospective hook: the seat that takes a retrospective gets a
// lesson task with it (spec 9.2). A failure is logged, so the retrospective stands.
func (s *RetroService) afterFireDossier(ctx context.Context, ws, agent, retroIssue pgtype.UUID, name string) {
	d := &DossierService{Queries: s.Queries, Issues: s.Issues, Flags: s.Flags}
	if !d.enabled(ctx) || !agent.Valid {
		return
	}
	body := fmt.Sprintf("This retrospective of %q is open.\n\n", name) +
		lessonTaskSteps("retro", util.UUIDToString(retroIssue))
	if err := d.fileSeatIssue(ctx, ws, agent, lessonTitle(name), body); err != nil {
		slog.Warn("retro: lesson task failed", "workspace_id", util.UUIDToString(ws), "error", err)
	}
}

// lessonTaskSteps tells the seat what to record and how, with the command the CLI runs.
func lessonTaskSteps(source, ref string) string {
	return "Record what this seat learned, at most three entries, each with the evidence it rests on:\n\n" +
		"    multica seat lesson add --kind pitfall --text \"<one line>\" --evidence " + source + ":" + ref + "\n\n" +
		"Kinds: pitfall (avoid this), lesson (this worked), context, preference, contact. " +
		"Add --work-class when the entry is for one kind of work. Add nothing when nothing new was learned. " +
		"Then close this issue.\n"
}

func lessonTitle(name string) string {
	return dossierTruncate(lessonTitlePrefix+name, dossierTitleMax)
}

// CleanupMonthly files, once a month, one cleanup issue for each seat that holds at least two
// active entries, asking it to retire the duplicates and the outdated ones with a reason
// (spec 9.2). It starts on the second day of the month, as the monthly review does, and a
// seat gets its issue once. It returns how many it filed.
func (s *DossierService) CleanupMonthly(ctx context.Context) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	now := s.now().UTC()
	if now.Day() < 2 {
		return 0, nil
	}
	month := now.Format("2006-01")
	candidates, err := s.Queries.ListSeatDossierCleanupCandidates(ctx)
	if err != nil {
		return 0, fmt.Errorf("list cleanup candidates: %w", err)
	}
	filed := 0
	for _, c := range candidates {
		title := dossierTruncate(cleanupTitlePrefix+month+": "+c.Name, dossierTitleMax)
		existing, err := s.Queries.CountIssuesWithTitle(ctx, db.CountIssuesWithTitleParams{WorkspaceID: c.WorkspaceID, Title: title})
		if err != nil || existing > 0 {
			if err != nil {
				slog.Warn("seat dossier cleanup: count issues failed", "workspace_id", util.UUIDToString(c.WorkspaceID), "error", err)
			}
			continue
		}
		body := fmt.Sprintf("Tidy the dossier of this seat for %s. Read the entries, then retire the duplicates and the entries that no longer hold:\n\n", month) +
			"    multica seat lesson list --json\n" +
			"    multica seat lesson retire <entry-id> --reason \"<why>\"\n\n" +
			"Give a reason for each retirement. Keep the rest. Entries a member wrote can only be retired by a member: leave them. Then close this issue.\n"
		if err := s.fileSeatIssue(ctx, c.WorkspaceID, c.AgentID, title, body); err != nil {
			slog.Warn("seat dossier cleanup: could not file the issue", "workspace_id", util.UUIDToString(c.WorkspaceID), "error", err)
			continue
		}
		filed++
	}
	return filed, nil
}

// fileSeatIssue opens an issue for the seat, filed on behalf of the workspace's oldest owner,
// as the retrospective and the monthly review do.
func (s *DossierService) fileSeatIssue(ctx context.Context, ws, seat pgtype.UUID, title, body string) error {
	if s.Issues == nil {
		return errors.New("seat dossier has no issue service")
	}
	owner, err := s.Queries.GetOldestWorkspaceOwnerMember(ctx, ws)
	if err != nil {
		return fmt.Errorf("workspace has no owner to file the seat issue: %w", err)
	}
	_, err = s.Issues.Create(ctx, IssueCreateParams{
		WorkspaceID:    ws,
		Title:          title,
		Description:    pgtype.Text{String: body, Valid: true},
		Status:         "todo",
		Priority:       "none",
		AssigneeType:   pgtype.Text{String: "agent", Valid: true},
		AssigneeID:     seat,
		CreatorType:    "member",
		CreatorID:      owner.UserID,
		AllowDuplicate: true,
	}, IssueCreateOpts{})
	return err
}

func (s *DossierService) inTx(ctx context.Context, fn func(q *db.Queries) error) error {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin dossier tx: %w", err)
	}
	defer tx.Rollback(ctx)
	if err := fn(s.Queries.WithTx(tx)); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// RecordSeatModelChange writes seat_model_changed when a save moves an agent to another
// runtime or model (spec 9.3). It runs through q, so the event commits with the save. It
// writes nothing while durable_seats is off, and nothing when the runtime and model are the
// same as before.
func RecordSeatModelChange(ctx context.Context, q *db.Queries, flags *featureflag.Service, actor GovernanceActor, before, after db.Agent) error {
	if flags == nil || !featureflags.DurableSeatsEnabled(ctx, flags) {
		return nil
	}
	if before.RuntimeID == after.RuntimeID && before.Model == after.Model {
		return nil
	}
	fromProvider, err := runtimeProvider(ctx, q, before.RuntimeID)
	if err != nil {
		return err
	}
	toProvider, err := runtimeProvider(ctx, q, after.RuntimeID)
	if err != nil {
		return err
	}
	details := map[string]any{
		"from_provider": fromProvider, "from_model": before.Model.String,
		"to_provider": toProvider, "to_model": after.Model.String,
	}
	return NewGovernanceLog().Record(ctx, q, GovernanceEvent{
		WorkspaceID: after.WorkspaceID,
		Kind:        GovernanceKindSeatModelChanged,
		ActorType:   actor.Type,
		ActorID:     actor.ID,
		SubjectType: "agent",
		SubjectID:   after.ID,
		Summary:     fmt.Sprintf("Seat %s changed model from %s to %s", after.Name, providerModel(fromProvider, before.Model.String), providerModel(toProvider, after.Model.String)),
		Details:     details,
	})
}

// runtimeProvider is the provider of a runtime, or "" when the agent has none or the runtime
// no longer exists.
func runtimeProvider(ctx context.Context, q *db.Queries, runtimeID pgtype.UUID) (string, error) {
	if !runtimeID.Valid {
		return "", nil
	}
	rt, err := q.GetAgentRuntime(ctx, runtimeID)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("load runtime: %w", err)
	}
	return rt.Provider, nil
}

func providerModel(provider, model string) string {
	if model == "" {
		return provider
	}
	return provider + "/" + model
}

// dossierTruncate cuts s to at most n characters.
func dossierTruncate(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}
