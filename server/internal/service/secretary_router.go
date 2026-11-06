package service

import (
	"context"
	"encoding/json"
	"strings"
	"time"
)

// The route a Secretary chat turn is given: what kind of question the boss's
// message asks. The platform gives it to the turn; the Secretary's router skill
// reads it and follows the skill it names. A missing route is never an error: the
// Secretary still classifies the message itself.
const (
	ChatRouteFact     = "fact"
	ChatRouteJudgment = "judgment"
	ChatRouteDecision = "decision"
	ChatRouteClarify  = "clarify"
)

// chatRouteTimeout bounds the classification. A slow model must not hold the
// boss's send; a missing route only means the turn starts unrouted.
const chatRouteTimeout = 8 * time.Second

const chatRouteSystemPrompt = `You route a message from the boss of a company to the company's Secretary. Classify the message as exactly one of:
- "fact": asks a plain fact the company's digest holds, such as a count, a status, a date, or what shipped or is late.
- "judgment": asks the departments' judgment: progress against a plan, a risk, a cause, why something slipped, whether something is on track.
- "decision": the boss decides or confirms a decision that a leader must carry out.
- "clarify": none of these, or the project, period or scope is unclear.
Return only JSON of this shape: {"route": "fact"}.`

// ChatRouteFor asks the model which route a boss's message takes. It returns the
// route, or "" when the model is off, fails, or answers with a route it does not know.
func ChatRouteFor(ctx context.Context, model JSONLLM, message string) string {
	if model == nil || !model.Enabled() {
		return ""
	}
	message = strings.TrimSpace(message)
	if message == "" {
		return ""
	}
	if len(message) > 2000 {
		message = message[:2000]
	}
	ctx, cancel := context.WithTimeout(ctx, chatRouteTimeout)
	defer cancel()
	raw, err := model.GenerateJSON(ctx, "", chatRouteSystemPrompt, message, 0, 60)
	if err != nil {
		return ""
	}
	var out struct {
		Route string `json:"route"`
	}
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return ""
	}
	switch out.Route {
	case ChatRouteFact, ChatRouteJudgment, ChatRouteDecision, ChatRouteClarify:
		return out.Route
	}
	return ""
}
