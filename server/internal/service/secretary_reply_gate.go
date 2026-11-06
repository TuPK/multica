package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The Secretary's replies to the boss are checked by the server, by kind. A reply
// that states the departments' judgment must come from the leaders' answers; the
// server can check that the answers exist and that each claim in the assessment
// is supported by one of them. These are the rules that make it so.

// JSONLLM is the model seam for the server's own checks: the reply router and
// the assessment verifier. *llm.Client satisfies it. A nil or disabled client
// turns the model checks off; the deterministic checks still apply.
type JSONLLM interface {
	Enabled() bool
	GenerateJSON(ctx context.Context, model, systemPrompt, userPrompt string, temperature float64, maxCompletionTokens int64) (string, error)
}

const (
	// ChatReplyKindAck: the Secretary says what it did, for example that it has asked
	// the leaders and when it expects the answers. Always allowed.
	ChatReplyKindAck = "ack"
	// ChatReplyKindFact: a plain fact the digest holds. Always allowed.
	ChatReplyKindFact = "fact"
	// ChatReplyKindClarify: a question back to the boss, or a note that needs no
	// evidence. Always allowed, and the default when no kind is given.
	ChatReplyKindClarify = "clarify"
	// ChatReplyKindAssessment: the answer to a judgment question, built from the
	// leaders' answers on a request. Gated below.
	ChatReplyKindAssessment = "assessment"
)

var (
	ErrChatReplyKind                  = errors.New("kind must be one of: ack, fact, clarify, assessment")
	ErrChatReplyAssessmentNotRequest  = errors.New("an assessment is written from a request's answers: reply from the run that reads a Secretary request")
	ErrChatReplyAssessmentTooEarly    = errors.New("not every leader has closed its answer yet and the deadline has not passed; wait for the answers or the deadline before you assess")
	ErrChatReplyAssessmentUnsupported = errors.New("the assessment states claims no leader answer supports; revise them or challenge the leader")
)

// ParseChatReplyKind returns the kind a reply is written as. An empty kind is a
// clarification, the least a reply can claim.
func ParseChatReplyKind(raw string) (string, error) {
	kind := strings.TrimSpace(raw)
	if kind == "" {
		return ChatReplyKindClarify, nil
	}
	switch kind {
	case ChatReplyKindAck, ChatReplyKindFact, ChatReplyKindClarify, ChatReplyKindAssessment:
		return kind, nil
	}
	return "", ErrChatReplyKind
}

// checkAssessmentReply applies the assessment gate. Only a run that reads a
// Secretary request may assess, and only when its leaders have all closed their
// answers, or when the run is the request's deadline run. When the verifier is
// available, each claim is checked against the leaders' answers; an unsupported
// claim refuses the reply and names the claim.
func checkAssessmentReply(ctx context.Context, q *db.Queries, run db.AgentTaskQueue, content string, verifier JSONLLM) error {
	if !run.IssueID.Valid {
		return ErrChatReplyAssessmentNotRequest
	}
	request, err := q.GetIssue(ctx, run.IssueID)
	if err != nil {
		return fmt.Errorf("load request: %w", err)
	}
	ws, err := q.GetWorkspace(ctx, request.WorkspaceID)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	ok, err := isSecretaryRequest(ctx, q, ws, request)
	if err != nil {
		return err
	}
	if !ok {
		return ErrChatReplyAssessmentNotRequest
	}

	children, err := q.ListChildIssues(ctx, request.ID)
	if err != nil {
		return fmt.Errorf("list request children: %w", err)
	}
	open := 0
	for _, c := range children {
		if c.Status != "done" && c.Status != "cancelled" {
			open++
		}
	}
	if open > 0 {
		deadline, err := isDeadlineRun(ctx, q, run, request.WorkspaceID)
		if err != nil {
			return err
		}
		if !deadline {
			return ErrChatReplyAssessmentTooEarly
		}
		passed, err := requestDeadlinePassed(request, time.Now())
		if err != nil {
			return err
		}
		if !passed {
			return ErrChatReplyAssessmentTooEarly
		}
	}

	if verifier == nil || !verifier.Enabled() {
		return nil
	}
	answers, err := requestAnswers(ctx, q, children)
	if err != nil {
		return err
	}
	unsupported, err := verifyAssessment(ctx, verifier, answers, content)
	if err != nil {
		// A failed check must not freeze the Secretary: the deterministic gate has
		// already passed, and the failure is logged for the operator.
		slog.Warn("assessment verifier failed; the reply is allowed on the deterministic gate", "error", err, "run_id", util.UUIDToString(run.ID))
		return nil
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("%w: %s", ErrChatReplyAssessmentUnsupported, strings.Join(unsupported, "; "))
	}
	return nil
}

// requestDeadlinePassed reports whether the deadline recorded in the request's
// metadata when it was filed has come. A deadline wakeup can be run by hand before
// its time, so the wakeup's kind alone does not say the time is up.
func requestDeadlinePassed(request db.Issue, now time.Time) (bool, error) {
	var meta struct {
		Deadline string `json:"deadline"`
	}
	if err := json.Unmarshal(request.Metadata, &meta); err != nil {
		return false, fmt.Errorf("read request metadata: %w", err)
	}
	deadline, err := time.Parse(time.RFC3339, meta.Deadline)
	if err != nil {
		return false, fmt.Errorf("request has no valid deadline: %w", err)
	}
	return !deadline.After(now), nil
}

// isDeadlineRun reports whether the run was started by a deadline wakeup, which
// fires once the request's time is up whether or not every leader answered.
func isDeadlineRun(ctx context.Context, q *db.Queries, run db.AgentTaskQueue, workspaceID pgtype.UUID) (bool, error) {
	var c struct {
		WakeupID string `json:"wakeup_id"`
	}
	if err := json.Unmarshal(run.Context, &c); err != nil || c.WakeupID == "" {
		return false, nil
	}
	wakeupID, err := util.ParseUUID(c.WakeupID)
	if err != nil {
		return false, nil
	}
	wakeup, err := q.GetIssueWakeup(ctx, db.GetIssueWakeupParams{ID: wakeupID, WorkspaceID: workspaceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("load wakeup: %w", err)
	}
	return wakeup.Kind == "at", nil
}

// requestAnswers gathers each leader's answer: the agent comments on each child,
// in the order they were written, each cut to a bounded length.
func requestAnswers(ctx context.Context, q *db.Queries, children []db.Issue) (map[string][]string, error) {
	const perChild, perComment = 6, 1500
	out := make(map[string][]string, len(children))
	for _, child := range children {
		comments, err := q.ListCommentsForIssue(ctx, db.ListCommentsForIssueParams{IssueID: child.ID, WorkspaceID: child.WorkspaceID, Limit: 500})
		if err != nil {
			return nil, fmt.Errorf("list answers on %s: %w", util.UUIDToString(child.ID), err)
		}
		var texts []string
		for _, c := range comments {
			if c.AuthorType != "agent" {
				continue
			}
			text := strings.TrimSpace(c.Content)
			if len(text) > perComment {
				text = text[:perComment] + "..."
			}
			texts = append(texts, fmt.Sprintf("[comment %s] %s", util.UUIDToString(c.ID), text))
		}
		if len(texts) > perChild {
			texts = texts[len(texts)-perChild:]
		}
		out[util.UUIDToString(child.ID)] = texts
	}
	return out, nil
}

const assessmentVerifierSystemPrompt = `You check an assessment that a Secretary is about to send to a company's boss.
You are given the leaders' answers on the request, each with its comment id, and the assessment.
Find each factual claim in the assessment that no leader answer supports. Ignore the
recommendations, the numbered decisions (D1, D2 ...), and any statement the assessment
marks as an inference. A claim supported by any answer, even paraphrased, is supported.
Return only JSON of this shape: {"unsupported": ["the claim, in the assessment's words"]}.
An empty list means every factual claim is supported.`

// verifyAssessment asks the model which claims of the assessment no answer
// supports. It returns those claims, or nothing when every claim is supported.
func verifyAssessment(ctx context.Context, verifier JSONLLM, answers map[string][]string, assessment string) ([]string, error) {
	var b strings.Builder
	b.WriteString("Leaders' answers, by child issue:\n")
	for child, texts := range answers {
		fmt.Fprintf(&b, "\nChild %s:\n", child)
		if len(texts) == 0 {
			b.WriteString("(no answer)\n")
		}
		for _, t := range texts {
			b.WriteString(t + "\n")
		}
	}
	b.WriteString("\nAssessment:\n")
	b.WriteString(assessment)
	raw, err := verifier.GenerateJSON(ctx, "", assessmentVerifierSystemPrompt, b.String(), 0, 800)
	if err != nil {
		return nil, err
	}
	var out struct {
		Unsupported []string `json:"unsupported"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, fmt.Errorf("verifier returned no JSON: %w", err)
	}
	var claims []string
	for _, c := range out.Unsupported {
		if strings.TrimSpace(c) != "" {
			claims = append(claims, c)
		}
	}
	return claims, nil
}
