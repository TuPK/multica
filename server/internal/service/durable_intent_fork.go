package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Errors the intent API answers with.
var (
	ErrIntentNotFound       = errors.New("intent not found")
	ErrIntentNotPending     = errors.New("intent is not pending")
	ErrIntentExpired        = errors.New("intent has expired")
	ErrIntentNoteRequired   = errors.New("a rejection needs a note for the agent")
	ErrIntentEditNotAllowed = errors.New("only the text of this action can be edited")
	ErrIntentEditTarget     = errors.New("an edit cannot change the tool or the account")
)

// intentTTL is how long a pending intent waits for a decision (durable spec 5.2).
const intentTTL = 72 * time.Hour

// intentEditableArgs are the arguments a person may change before approving: the text
// of a message or a post (durable spec 5.3). Other arguments are the call itself.
var intentEditableArgs = map[string]bool{
	"body": true, "body_markdown": true, "html_body": true, "text": true, "content": true, "message": true,
}

// IntentExecutor runs an approved call as the member who made it (connected apps).
type IntentExecutor interface {
	ExecuteIntent(ctx context.Context, workspaceID, memberID pgtype.UUID, toolkit string, call connectedapps.CanonicalCall) (map[string]any, error)
}

// IntentService holds rehearsed actions until a person decides on them (durable spec
// 5.2 and 5.3). Approval binds to the call's hash, runs the call once, and wakes the
// agent with the result. Nothing is written while durable_effects is off.
type IntentService struct {
	Queries   *db.Queries
	TxStarter TxStarter
	Flags     *featureflag.Service
	Apps      IntentExecutor
	// Tasks wakes the agent after a decision. Nil leaves the agent to find the answer.
	Tasks *TaskService
	// Now is the clock. Nil means time.Now.
	Now func() time.Time
}

func (s *IntentService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s *IntentService) enabled(ctx context.Context) bool {
	return featureflags.DurableEffectsEnabled(ctx, s.Flags)
}

// Rehearse stores a call the policy holds as a pending intent with its preview, and
// links the awaiting effect event to it. It implements connectedapps.IntentGate.
func (s *IntentService) Rehearse(ctx context.Context, eventID pgtype.UUID, in connectedapps.EffectRequest, call connectedapps.CanonicalCall, rationale string) (pgtype.UUID, error) {
	if !s.enabled(ctx) {
		return pgtype.UUID{}, ErrEffectsOff
	}
	sum := CallSHA256(call)
	if sum == "" {
		return pgtype.UUID{}, errors.New("the call cannot be encoded for approval")
	}
	callJSON, err := json.Marshal(call)
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("encode call: %w", err)
	}
	preview, err := json.Marshal(intentPreview(in, call))
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("encode preview: %w", err)
	}
	now := s.now()
	batch := fmt.Sprintf("%s|%s|%s|%s", util.UUIDToString(in.AgentID), in.Target, in.Class, now.UTC().Format("2006-01-02"))
	row, err := s.Queries.InsertEffectIntent(ctx, db.InsertEffectIntentParams{
		WorkspaceID:   in.WS,
		TaskID:        in.TaskID,
		AgentID:       in.AgentID,
		IssueID:       in.IssueID,
		Source:        in.Source,
		Target:        effectTruncate(in.Target, effectTargetMax),
		EffectClass:   in.Class,
		Reversibility: in.Reversibility,
		Call:          callJSON,
		CallSha256:    sum,
		Preview:       preview,
		Rationale:     effectTruncate(rationale, 2000),
		ExpiresAt:     pgtype.Timestamptz{Time: now.Add(intentTTL), Valid: true},
		BatchKey:      batch,
	})
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("insert intent: %w", err)
	}
	if eventID.Valid {
		if err := s.Queries.LinkEffectEventToIntent(ctx, db.LinkEffectEventToIntentParams{
			IntentID: row.ID, ID: eventID, WorkspaceID: in.WS,
		}); err != nil {
			return pgtype.UUID{}, fmt.Errorf("link effect to intent: %w", err)
		}
	}
	s.admitIntent(ctx, in, row.ID, call, rationale)
	return row.ID, nil
}

// Get returns one intent of the workspace.
func (s *IntentService) Get(ctx context.Context, ws, id pgtype.UUID) (db.EffectIntent, error) {
	row, err := s.Queries.GetEffectIntent(ctx, db.GetEffectIntentParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.EffectIntent{}, ErrIntentNotFound
	}
	return row, err
}

// List returns the workspace's intents, newest first, optionally of one state.
func (s *IntentService) List(ctx context.Context, ws pgtype.UUID, state string, limit int32) ([]db.EffectIntent, error) {
	params := db.ListEffectIntentsParams{WorkspaceID: ws, RowLimit: limit}
	if state != "" {
		params.State = pgtype.Text{String: state, Valid: true}
	}
	return s.Queries.ListEffectIntents(ctx, params)
}

// Approve runs a pending intent once. edited, when given, replaces the call's text
// before it runs; the hash is taken again, and the new call is what runs. A second
// approval, or one after a rejection or expiry, returns ErrIntentNotPending and runs
// nothing.
func (s *IntentService) Approve(ctx context.Context, ws, actor, id pgtype.UUID, edited *connectedapps.CanonicalCall, note string) (db.EffectIntent, error) {
	if !s.enabled(ctx) {
		return db.EffectIntent{}, ErrEffectsOff
	}
	it, call, wasEdited, err := s.claimForApproval(ctx, ws, actor, id, edited, note)
	if err != nil {
		return db.EffectIntent{}, err
	}
	if it.Source == connectedapps.SourceDeclared {
		// A declared action is not run by the platform. The decision goes back to the agent.
		s.postIntentComment(ctx, it, "approved", wasEdited, "")
		s.wake(ctx, it, actor, "approved: "+note)
		return s.Get(ctx, ws, id)
	}

	outcome := "executed"
	var resultNote string
	if s.Apps == nil {
		outcome, resultNote = "failed", "the platform cannot run approved calls"
	} else if _, err := s.Apps.ExecuteIntent(ctx, ws, s.memberOf(ctx, it), intentToolkit(it.Target), call); err != nil {
		outcome, resultNote = "failed", err.Error()
	}
	if _, err := s.Queries.SetEffectIntentOutcome(ctx, db.SetEffectIntentOutcomeParams{
		State: outcome, ID: id, WorkspaceID: ws,
	}); err != nil {
		slog.Error("intent: set outcome", "error", err, "intent_id", util.UUIDToString(id))
	}
	if _, err := s.Queries.FinishEffectEventForIntent(ctx, db.FinishEffectEventForIntentParams{
		Status: outcome, IntentID: id, WorkspaceID: ws,
	}); err != nil {
		slog.Error("intent: finish event", "error", err, "intent_id", util.UUIDToString(id))
	}
	if outcome == "executed" {
		s.postIntentComment(ctx, it, outcome, wasEdited, "")
	}
	wakeNote := "approved and run"
	if outcome == "failed" {
		wakeNote = "approved, but the call failed: " + resultNote
	}
	if note != "" {
		wakeNote += "; note: " + note
	}
	s.wake(ctx, it, actor, wakeNote)
	return s.Get(ctx, ws, id)
}

// postIntentComment leaves the system comment on the issue after an approved intent runs
// (durable spec 5.3). When the approver edited the call text, the comment says so.
func (s *IntentService) postIntentComment(ctx context.Context, it db.EffectIntent, outcome string, edited bool, extra string) {
	if !it.IssueID.Valid {
		return
	}
	content := fmt.Sprintf("Hành động %s đã được duyệt (%s).", it.Target, outcome)
	if edited {
		content = fmt.Sprintf("Hành động %s đã được duyệt (%s, đã sửa trước khi gửi).", it.Target, outcome)
	}
	if extra != "" {
		content += " " + extra
	}
	if _, err := s.Queries.CreateComment(ctx, db.CreateCommentParams{
		ID:          dbid.NewV7(),
		IssueID:     it.IssueID,
		WorkspaceID: it.WorkspaceID,
		AuthorType:  "system",
		AuthorID:    pgtype.UUID{Valid: true},
		Content:     content,
		Type:        "system",
		ParentID:    pgtype.UUID{},
	}); err != nil {
		slog.Warn("intent: post system comment", "error", err, "intent_id", util.UUIDToString(it.ID))
	}
}

// claimForApproval locks the intent, checks it is still pending and not expired,
// stores the approved call and its hash, and commits. Only one caller can claim an
// intent, so the call runs once.
func (s *IntentService) claimForApproval(ctx context.Context, ws, actor, id pgtype.UUID, edited *connectedapps.CanonicalCall, note string) (db.EffectIntent, connectedapps.CanonicalCall, bool, error) {
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	it, err := q.GetEffectIntentForUpdate(ctx, db.GetEffectIntentForUpdateParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, ErrIntentNotFound
	}
	if err != nil {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, err
	}
	if it.State != "pending" {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, ErrIntentNotPending
	}
	if !s.now().Before(it.ExpiresAt.Time) {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, ErrIntentExpired
	}
	var call connectedapps.CanonicalCall
	if err := json.Unmarshal(it.Call, &call); err != nil {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, fmt.Errorf("decode intent call: %w", err)
	}
	origSHA := it.CallSha256
	wasEdited := false
	if edited != nil {
		if err := validateEdit(call, *edited); err != nil {
			return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, err
		}
		call = *edited
		wasEdited = true
	}
	sum := CallSHA256(call)
	if sum == "" {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, errors.New("the call cannot be encoded for approval")
	}
	if wasEdited && sum == origSHA {
		wasEdited = false
	}
	callJSON, err := json.Marshal(call)
	if err != nil {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, fmt.Errorf("encode call: %w", err)
	}
	n, err := q.ApproveEffectIntent(ctx, db.ApproveEffectIntentParams{
		ID: id, WorkspaceID: ws, Call: callJSON, CallSha256: sum,
		DecidedBy: actor, DecisionNote: note,
	})
	if err != nil {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, err
	}
	if n == 0 {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, ErrIntentNotPending
	}
	if err := tx.Commit(ctx); err != nil {
		return db.EffectIntent{}, connectedapps.CanonicalCall{}, false, err
	}
	it.Call, it.CallSha256 = callJSON, sum
	return it, call, wasEdited, nil
}

// validateEdit allows the text of a message to change and nothing else: the tool, the
// account and every other argument must stay as the agent asked for them.
func validateEdit(orig, edited connectedapps.CanonicalCall) error {
	if orig.Tool != edited.Tool || orig.AccountID != edited.AccountID {
		return ErrIntentEditTarget
	}
	for k, v := range edited.Args {
		if reflect.DeepEqual(orig.Args[k], v) {
			continue
		}
		if !intentEditableArgs[k] {
			return ErrIntentEditNotAllowed
		}
	}
	for k := range orig.Args {
		if _, ok := edited.Args[k]; !ok && !intentEditableArgs[k] {
			return ErrIntentEditNotAllowed
		}
	}
	return nil
}

// Reject closes a pending intent without running it. The note goes to the agent, and
// is required.
func (s *IntentService) Reject(ctx context.Context, ws, actor, id pgtype.UUID, note string) (db.EffectIntent, error) {
	if !s.enabled(ctx) {
		return db.EffectIntent{}, ErrEffectsOff
	}
	note = strings.TrimSpace(note)
	if note == "" {
		return db.EffectIntent{}, ErrIntentNoteRequired
	}
	tx, err := s.TxStarter.Begin(ctx)
	if err != nil {
		return db.EffectIntent{}, err
	}
	defer tx.Rollback(ctx)
	q := s.Queries.WithTx(tx)
	it, err := q.GetEffectIntentForUpdate(ctx, db.GetEffectIntentForUpdateParams{ID: id, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return db.EffectIntent{}, ErrIntentNotFound
	}
	if err != nil {
		return db.EffectIntent{}, err
	}
	if it.State != "pending" {
		return db.EffectIntent{}, ErrIntentNotPending
	}
	n, err := q.RejectEffectIntent(ctx, db.RejectEffectIntentParams{
		ID: id, WorkspaceID: ws, DecidedBy: actor, DecisionNote: note,
	})
	if err != nil {
		return db.EffectIntent{}, err
	}
	if n == 0 {
		return db.EffectIntent{}, ErrIntentNotPending
	}
	if _, err := q.CancelEffectEventForIntent(ctx, db.CancelEffectEventForIntentParams{IntentID: id, WorkspaceID: ws}); err != nil {
		return db.EffectIntent{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return db.EffectIntent{}, err
	}
	s.wake(ctx, it, actor, "rejected: "+note)
	return s.Get(ctx, ws, id)
}

// Expire moves every pending intent past its expiry to expired and wakes its agent
// once. It returns how many it expired. The job runs it on a schedule.
func (s *IntentService) Expire(ctx context.Context) (int, error) {
	if !s.enabled(ctx) {
		return 0, nil
	}
	rows, err := s.Queries.ExpirePendingEffectIntents(ctx, pgtype.Timestamptz{Time: s.now(), Valid: true})
	if err != nil {
		return 0, err
	}
	for _, it := range rows {
		if _, err := s.Queries.CancelEffectEventForIntent(ctx, db.CancelEffectEventForIntentParams{
			IntentID: it.ID, WorkspaceID: it.WorkspaceID,
		}); err != nil {
			slog.Error("intent: cancel event on expiry", "error", err, "intent_id", util.UUIDToString(it.ID))
		}
		s.wake(ctx, it, pgtype.UUID{}, "expired without a decision after 72 hours; the action was not run")
	}
	return len(rows), nil
}

// memberOf is the member an intent's call runs for: the member of its awaiting event.
func (s *IntentService) memberOf(ctx context.Context, it db.EffectIntent) pgtype.UUID {
	ev, err := s.Queries.GetEffectEventByIntent(ctx, db.GetEffectEventByIntentParams{IntentID: it.ID, WorkspaceID: it.WorkspaceID})
	if err != nil {
		return pgtype.UUID{}
	}
	return ev.MemberID
}

// wake resumes the agent of an intent with the decision (durable spec 5.3). An
// intent with no issue has no run to resume, and the decision waits in the intent.
func (s *IntentService) wake(ctx context.Context, it db.EffectIntent, actor pgtype.UUID, note string) {
	if s.Tasks == nil || !it.IssueID.Valid || !it.AgentID.Valid {
		return
	}
	if !actor.Valid {
		actor = s.memberOf(ctx, it)
	}
	if err := s.Tasks.EnqueueIntentDecisionRun(ctx, it.IssueID, it.AgentID, actor, it.ID, note); err != nil {
		slog.Warn("intent: wake agent", "error", err, "intent_id", util.UUIDToString(it.ID))
	}
}

// intentPreview is the preview a person reads. A declared action has no call the
// platform can show, so its preview names what the agent declared.
func intentPreview(in connectedapps.EffectRequest, call connectedapps.CanonicalCall) Preview {
	if in.Source == connectedapps.SourceDeclared {
		return Preview{
			Kind:  "declared",
			Title: effectTruncate(in.Summary, 200),
			Fields: []PreviewField{
				{Label: "Target", After: in.Target},
				{Label: "Class", After: in.Class},
			},
		}
	}
	return BuildPreview(intentToolkit(in.Target), call.Tool, call.Args)
}

// intentToolkit is the toolkit part of a "toolkit/tool" target.
func intentToolkit(target string) string {
	toolkit, _, _ := strings.Cut(target, "/")
	return toolkit
}
