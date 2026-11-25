package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/dbid"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Chi phí phút chú ý ước tính của câu hỏi gửi người quyết định (durable-platform spec 7.2).
// Bảng ở đây là giá trị mặc định; job hằng tháng hiệu chỉnh bằng số đo thật, không tự đổi bảng.

const (
	// Dự đoán F5 đủ chắc thì câu hỏi tốn ít thời gian hơn một nửa.
	attentionPredictedConfidence = 0.8
	attentionPredictedDiscount   = 0.5
	// Mỗi câu thêm trong lô cộng thêm chừng này (spec 7.2).
	attentionExtraAskMinutes = 0.5
	// Proposal: cứ mỗi bấy nhiêu từ thân thì cộng thêm một phút.
	attentionWordsPerMinute = 400
)

// EstimateMinutes trả phút ước tính cho một câu hỏi và cho một lô gồm batchSize câu
// cùng loại, cùng đầu vào (batchSize ≥ 1). Lô gồm câu khác loại: dùng BatchMinutes
// với phút của từng câu từ askMinutes.
//
// Giả định:
//   - Số từ của proposal làm tròn xuống: 399 từ không cộng phút, 400 từ cộng một.
//   - words, irreversible bị bỏ qua với loại không liên quan.
//   - kind không biết trả 0 kể cả khi batchSize > 1.
func EstimateMinutes(kind string, words int, irreversible bool, predictedConf float64, batchSize int) float64 {
	per, known := askMinutes(kind, words, irreversible, predictedConf)
	if !known {
		return 0
	}
	return per + attentionExtraAskMinutes*float64(batchSize-1)
}

// BatchMinutes trả phút của cả lô: câu tốn nhiều nhất, cộng 0.5 cho mỗi câu còn lại.
func BatchMinutes(perAsk []float64) float64 {
	if len(perAsk) == 0 {
		return 0
	}
	largest := perAsk[0]
	for _, m := range perAsk[1:] {
		if m > largest {
			largest = m
		}
	}
	return largest + attentionExtraAskMinutes*float64(len(perAsk)-1)
}

// askMinutes trả phút của một câu và false khi kind không có trong bảng.
func askMinutes(kind string, words int, irreversible bool, predictedConf float64) (float64, bool) {
	var m float64
	switch kind {
	case "proposal":
		m = 3 + float64(words/attentionWordsPerMinute)
	case "org_change":
		m = 6
	case "effect_intent":
		m = 1
		if irreversible {
			m++
		}
	case "outcome_check":
		m = 4
	case "app_action_request":
		m = 1
	case "inquiry":
		m = 2
	default:
		return 0, false
	}
	if predictedConf >= attentionPredictedConfidence {
		m *= attentionPredictedDiscount
	}
	return m, true
}

// Attention budget and ask ledger (durable spec 7.3, F6). The budget is in minutes per
// person decider per week, shared by the live squads by equal shares or by active goals.
// A question the squad's share cannot hold is queued: it reaches neither the inbox nor the
// chat, and the next brief lists it. A question with no squad (system) and an urgent one
// are delivered. The flag durable_principal gates all of it: off, nothing is written.

const (
	// AttentionDelivered: the question reaches the person now.
	AttentionDelivered = "delivered"
	// AttentionQueued: the squad's share is used up; the brief lists the question.
	AttentionQueued = "queued"
	// AttentionDelegated: the envelope (F5) decided it; it spends no minutes.
	AttentionDelegated = "delegated"

	// attentionMaxSeconds cuts the measured time of a decision at 30 minutes (spec 7.2).
	attentionMaxSeconds = 30 * 60
	// attentionWhyMax is the most characters a why_not_decide may have (spec 7.4).
	attentionWhyMax = 500
	// attentionDigestMax bounds the queued asks the brief lists.
	attentionDigestMax = 20
	// attentionDefaultReason is a person's question with no reason given.
	attentionDefaultReason = "judgment"

	attentionSplitGoals = "goals"
)

var (
	// ErrAttentionReasonRequired: an agent's question has no reason, an unknown reason, or no
	// why_not_decide (spec 7.4).
	ErrAttentionReasonRequired = errors.New("a question an agent asks needs a reason and why_not_decide")
	// ErrAttentionSearchedRequired: missing_info names no searched source, or an unknown one.
	ErrAttentionSearchedRequired = errors.New("missing_info needs the sources searched: knowledge, issues, comments, web or attachments")
	// ErrAttentionWhyTooLong: why_not_decide is over attentionWhyMax characters.
	ErrAttentionWhyTooLong = errors.New("why_not_decide must be at most 500 characters")
)

// attentionReasons are the reasons of spec 7.3; attentionSources the sources of spec 7.4.
var (
	attentionReasons = map[string]bool{
		"judgment": true, "irreversible": true, "missing_access": true,
		"missing_info": true, "policy": true, "verification": true,
	}
	attentionSources = map[string]bool{
		"knowledge": true, "issues": true, "comments": true, "web": true, "attachments": true,
	}
)

// AttentionErrorCode is the machine code of an attention refusal, "" for any other error.
func AttentionErrorCode(err error) string {
	switch {
	case errors.Is(err, ErrAttentionReasonRequired):
		return "attention_reason_required"
	case errors.Is(err, ErrAttentionSearchedRequired):
		return "attention_searched_required"
	case errors.Is(err, ErrAttentionWhyTooLong):
		return "attention_why_too_long"
	}
	return ""
}

// CheckAgentAsk is the rule of spec 7.4 for a question an agent puts to a person: a reason,
// a why_not_decide, and for missing_info the sources it searched. A person needs none of it.
func CheckAgentAsk(reason, whyNotDecide string, searched []string) error {
	if !attentionReasons[reason] || strings.TrimSpace(whyNotDecide) == "" {
		return ErrAttentionReasonRequired
	}
	if utf8.RuneCountInString(whyNotDecide) > attentionWhyMax {
		return ErrAttentionWhyTooLong
	}
	if reason == "missing_info" {
		if len(searched) == 0 {
			return ErrAttentionSearchedRequired
		}
		for _, source := range searched {
			if !attentionSources[source] {
				return ErrAttentionSearchedRequired
			}
		}
	}
	return nil
}

// AskInput is one question put to a person to decide. ByAgent marks a question an agent
// asked: it must pass CheckAgentAsk. EstimateKind names the row of the minutes table; empty
// means SubjectType.
type AskInput struct {
	WorkspaceID   pgtype.UUID
	SquadID       pgtype.UUID // the asking squad; invalid for a system question
	SubjectType   string      // proposal | effect_intent | outcome_check | app_action_request | inquiry
	SubjectID     pgtype.UUID
	EstimateKind  string
	Words         int
	Irreversible  bool
	PredictedConf float64
	Reason        string
	WhyNotDecide  string
	Searched      []string
	ByAgent       bool
	Urgent        bool
	Delegated     bool
}

// AttentionService keeps the ledger and admits each question. Flags is read on every call,
// so the flag can change while the service runs.
type AttentionService struct {
	Queries *db.Queries
	Flags   *featureflag.Service
	// Now is the clock; nil means time.Now.
	Now func() time.Time
}

func (s *AttentionService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// on reports whether the ledger is written for this context.
func (s *AttentionService) on(ctx context.Context) bool {
	return featureflags.DurablePrincipalEnabled(ctx, s.Flags)
}

// Admit writes the question's ledger row and returns its state: delivered or queued, or
// delegated when the envelope decided it. The first row of a question stays as it is, so
// admitting the same question again returns the state it has. With the flag off it writes
// nothing and returns delivered.
func (s *AttentionService) Admit(ctx context.Context, ask AskInput) (string, error) {
	if !s.on(ctx) {
		return AttentionDelivered, nil
	}
	reason := ask.Reason
	if ask.ByAgent {
		if err := CheckAgentAsk(reason, ask.WhyNotDecide, ask.Searched); err != nil {
			return "", err
		}
	} else if reason == "" {
		reason = attentionDefaultReason
	}
	if !attentionReasons[reason] {
		return "", ErrAttentionReasonRequired
	}

	kind := ask.EstimateKind
	if kind == "" {
		kind = ask.SubjectType
	}
	minutes := EstimateMinutes(kind, ask.Words, ask.Irreversible, ask.PredictedConf, 1)
	week := attentionWeekStart(s.now())
	state := AttentionDelivered
	switch {
	case ask.Delegated:
		state, minutes = AttentionDelegated, 0
	case ask.Urgent || !ask.SquadID.Valid:
		// An urgent question is never queued, and a system question has no squad share.
	default:
		over, err := s.overBudget(ctx, ask.WorkspaceID, ask.SquadID, week, minutes)
		if err != nil {
			return "", err
		}
		if over {
			state = AttentionQueued
		}
	}

	stored, err := s.Queries.InsertAttentionLedger(ctx, db.InsertAttentionLedgerParams{
		ID:               dbid.NewV7(),
		WorkspaceID:      ask.WorkspaceID,
		SquadID:          ask.SquadID,
		SubjectType:      ask.SubjectType,
		SubjectID:        ask.SubjectID,
		Reason:           reason,
		EstimatedMinutes: minutes,
		State:            state,
		WeekStart:        pgtype.Date{Time: week, Valid: true},
	})
	if err != nil {
		return "", fmt.Errorf("record attention ask: %w", err)
	}
	return stored, nil
}

// overBudget reports whether the squad's minutes this week, with minutes more, pass its share.
// A squad that is not live has no share to pass, so its question is not queued.
func (s *AttentionService) overBudget(ctx context.Context, ws, squad pgtype.UUID, week time.Time, minutes float64) (bool, error) {
	settings, err := s.Queries.GetWorkspaceAttentionSettings(ctx, ws)
	if err != nil {
		return false, fmt.Errorf("load attention settings: %w", err)
	}
	squads, err := s.Queries.ListLiveSquadAttentionWeights(ctx, ws)
	if err != nil {
		return false, fmt.Errorf("load squad weights: %w", err)
	}
	budget, live := attentionBudget(float64(settings.AttentionMinutesPerWeek), settings.AttentionSplit, squads, squad)
	if !live {
		return false, nil
	}
	spent, err := s.Queries.SumSquadAttentionMinutes(ctx, db.SumSquadAttentionMinutesParams{
		WorkspaceID: ws, SquadID: squad, WeekStart: pgtype.Date{Time: week, Valid: true},
	})
	if err != nil {
		return false, fmt.Errorf("sum squad minutes: %w", err)
	}
	return spent+minutes > budget, nil
}

// attentionBudget is a squad's share of the week's minutes. With split equal every live
// squad weighs the same; with goals a squad weighs its active goals. When no squad has an
// active goal the goals split falls back to equal shares. ok is false for a squad not live.
func attentionBudget(perWeek float64, split string, squads []db.ListLiveSquadAttentionWeightsRow, squad pgtype.UUID) (float64, bool) {
	weigh := func(r db.ListLiveSquadAttentionWeightsRow) float64 {
		if split == attentionSplitGoals {
			return float64(r.ActiveGoals)
		}
		return 1
	}
	var total, mine float64
	found := false
	for _, r := range squads {
		total += weigh(r)
		if r.SquadID == squad {
			mine, found = weigh(r), true
		}
	}
	if !found {
		return 0, false
	}
	if total == 0 {
		return perWeek / float64(len(squads)), true
	}
	return perWeek * mine / total, true
}

// Answered records that a person decided a question, with the seconds from opening it to
// deciding. The seconds are cut at 30 minutes; a question with no row is left alone.
func (s *AttentionService) Answered(ctx context.Context, subjectType string, subjectID pgtype.UUID, actualSeconds *int) error {
	var seconds pgtype.Int4
	if actualSeconds != nil {
		seconds = pgtype.Int4{Int32: int32(min(max(*actualSeconds, 0), attentionMaxSeconds)), Valid: true}
	}
	_, err := s.Queries.MarkAttentionAnswered(ctx, db.MarkAttentionAnsweredParams{
		SubjectType: subjectType, SubjectID: subjectID, ActualSeconds: seconds,
	})
	if err != nil {
		return fmt.Errorf("record attention answer: %w", err)
	}
	return nil
}

// AdmitAppRequest puts an app action request in the ledger. A connect request is missing
// access; any other one waits for the connected app's policy (connectedapps.AttentionGate).
func (s *AttentionService) AdmitAppRequest(ctx context.Context, req db.AppActionRequest) error {
	if !s.on(ctx) {
		return nil
	}
	reason := "policy"
	if req.Kind == connectedapps.KindConnect {
		reason = "missing_access"
	}
	squad, err := s.agentSquad(ctx, req.WorkspaceID, req.AgentID)
	if err != nil {
		return err
	}
	_, err = s.Admit(ctx, AskInput{
		WorkspaceID: req.WorkspaceID, SquadID: squad, SubjectType: "app_action_request",
		SubjectID: req.ID, Reason: reason,
	})
	return err
}

// agentSquad is the squad an agent works for, which its questions are charged to. An agent
// with no home squad, or no agent, has no squad: its questions are system questions.
func (s *AttentionService) agentSquad(ctx context.Context, ws, agent pgtype.UUID) (pgtype.UUID, error) {
	if !agent.Valid {
		return pgtype.UUID{}, nil
	}
	squad, err := s.Queries.GetAgentHomeSquadID(ctx, db.GetAgentHomeSquadIDParams{ID: agent, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) {
		return pgtype.UUID{}, nil
	}
	if err != nil {
		return pgtype.UUID{}, fmt.Errorf("load agent squad: %w", err)
	}
	return squad, nil
}

// admitIntent puts a rehearsed call's intent in the ledger: a person approves or rejects it.
// The reason is the call's reversibility; the rationale is what the agent gave for it.
func (s *IntentService) admitIntent(ctx context.Context, in connectedapps.EffectRequest, intentID pgtype.UUID, call connectedapps.CanonicalCall, rationale string) {
	att := &AttentionService{Queries: s.Queries, Flags: s.Flags}
	if !att.on(ctx) {
		return
	}
	reason, irreversible := "judgment", in.Reversibility == connectedapps.ReversibilityIrreversible
	if irreversible {
		reason = "irreversible"
	}
	squad, err := att.agentSquad(ctx, in.WS, in.AgentID)
	if err == nil {
		_, err = att.Admit(ctx, AskInput{
			WorkspaceID: in.WS, SquadID: squad, SubjectType: "effect_intent", SubjectID: intentID,
			Reason: reason, Irreversible: irreversible,
		})
	}
	if err != nil {
		slog.Warn("attention ledger: intent not admitted", "intent_id", util.UUIDToString(intentID), "error", err)
	}
}

// admitManualCheck puts a sampled outcome's manual check in the ledger: a person checks it.
// One question per outcome, whatever the number of manual criteria, because the ledger keeps
// one row per subject.
func (s *VerificationService) admitManualCheck(ctx context.Context, o db.WorkOutcome) {
	att := &AttentionService{Queries: s.Queries, Flags: s.Flags}
	if !att.on(ctx) {
		return
	}
	_, err := att.Admit(ctx, AskInput{
		WorkspaceID: o.WorkspaceID, SquadID: o.SquadID, SubjectType: "outcome_check",
		SubjectID: o.ID, Reason: "verification",
	})
	if err != nil {
		slog.Warn("attention ledger: manual check not admitted", "outcome_id", util.UUIDToString(o.ID), "error", err)
	}
}

// QueuedAsk is a question waiting for its squad's share, as the brief lists it.
type QueuedAsk struct {
	SubjectType      string    `json:"subject_type"`
	SubjectID        string    `json:"subject_id"`
	SquadID          *string   `json:"squad_id"`
	Reason           string    `json:"reason"`
	EstimatedMinutes float64   `json:"estimated_minutes"`
	CreatedAt        time.Time `json:"created_at"`
}

// QueuedAttentionAsks lists a workspace's queued questions, the reasons that block a decision
// first and then the oldest, at most attentionDigestMax of them.
func QueuedAttentionAsks(ctx context.Context, q *db.Queries, ws pgtype.UUID) ([]QueuedAsk, error) {
	rows, err := q.ListQueuedAttentionAsks(ctx, db.ListQueuedAttentionAsksParams{WorkspaceID: ws, RowLimit: attentionDigestMax})
	if err != nil {
		return nil, fmt.Errorf("list queued asks: %w", err)
	}
	out := make([]QueuedAsk, 0, len(rows))
	for _, r := range rows {
		item := QueuedAsk{
			SubjectType:      r.SubjectType,
			SubjectID:        util.UUIDToString(r.SubjectID),
			Reason:           r.Reason,
			EstimatedMinutes: r.EstimatedMinutes,
			CreatedAt:        r.CreatedAt.Time,
		}
		if r.SquadID.Valid {
			squad := util.UUIDToString(r.SquadID)
			item.SquadID = &squad
		}
		out = append(out, item)
	}
	return out, nil
}

// attentionWeekStart is the Monday 00:00 UTC of the week t falls in. The budget is per week
// from that day.
func attentionWeekStart(t time.Time) time.Time {
	t = t.UTC()
	offset := (int(t.Weekday()) + 6) % 7
	y, m, d := t.Date()
	return time.Date(y, m, d-offset, 0, 0, 0, 0, time.UTC)
}

// filterQueuedToSquads keeps the queued asks of a floor's squads, so a floor's brief does not
// show another floor's questions.
func filterQueuedToSquads(asks []QueuedAsk, squads map[string]bool) []QueuedAsk {
	out := make([]QueuedAsk, 0, len(asks))
	for _, a := range asks {
		if a.SquadID != nil && squads[*a.SquadID] {
			out = append(out, a)
		}
	}
	return out
}
