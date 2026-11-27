package connectedapps

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/util"
)

// RationaleParam is the argument an agent passes to say why it wants a call. The
// gateway takes it out before the call is sent, and the preview never shows it
// (durable spec 5.2).
const RationaleParam = "_rationale"

// ErrIntentAccountChanged: the account a rehearsed call was made with is no longer the
// one the member would act through. The call is not run; the agent asks again.
var ErrIntentAccountChanged = errors.New("the account for this call changed since it was rehearsed")

// CanonicalCall is one tool call as the platform runs it (durable spec 5.2): the tool,
// the connected account, and the arguments. An approval binds to the SHA-256 of its
// JSON, so the call that runs is the call that was approved.
type CanonicalCall struct {
	Tool      string         `json:"tool"`
	AccountID string         `json:"account_id"`
	Args      map[string]any `json:"args"`
}

// IntentGate turns a rehearsed call into an intent a person approves (durable spec
// 5.2). The service package implements it; the gateway takes it as an interface
// because the service package imports this one. eventID is the awaiting event the
// gate wrote for the call, which the intent links to.
type IntentGate interface {
	Rehearse(ctx context.Context, eventID pgtype.UUID, in EffectRequest, call CanonicalCall, rationale string) (pgtype.UUID, error)
}

// takeRationale removes the agent's rationale from a call's arguments and returns it.
// A call's arguments are sent to the app as they are, so the rationale must leave
// them first.
func takeRationale(args map[string]any) string {
	raw, ok := args[RationaleParam]
	if !ok {
		return ""
	}
	delete(args, RationaleParam)
	s, _ := raw.(string)
	return strings.TrimSpace(s)
}

// ExecuteIntent runs an approved call as the member who made it, through the account
// the gateway resolves for them. If that account is not the one the call was
// rehearsed with, it refuses rather than run the call on another account.
func (s *Service) ExecuteIntent(ctx context.Context, workspaceID, memberID pgtype.UUID, toolkit string, call CanonicalCall) (map[string]any, error) {
	data, _, err := s.executeAs(ctx, workspaceID, memberID, toolkit, call.Tool, call.AccountID, call.Args)
	if err != nil {
		return nil, err
	}
	return data, nil
}

// rehearseCall stops a call the policy rehearses: it becomes an intent a person
// approves, and the agent is told to wait. Without an intent gate it is refused like
// a call that needs approval.
func (s *Service) rehearseCall(ctx context.Context, t effectTicket, toolkit, tool, accountID string, args map[string]any, rationale string) map[string]any {
	if s.intents == nil || !t.event.Valid {
		return textResult("this call needs the person's approval before it runs, so it was not sent. Do not retry; tell the person what you would have done.", true)
	}
	call := CanonicalCall{Tool: tool, AccountID: accountID, Args: args}
	intentID, err := s.intents.Rehearse(ctx, t.event, t.req, call, rationale)
	if err != nil {
		slog.Error("rehearse call: record intent", "error", err, "tool", tool)
		return textResult("could not record the action for approval, so it was not sent. Do not retry; tell the person what you would have done.", true)
	}
	raw, _ := json.Marshal(map[string]any{
		"status":    "awaiting_approval",
		"intent_id": util.UUIDToString(intentID),
		"note":      "The action was not sent. A person approves it on the intents page; the run resumes with their decision. End your turn.",
	})
	return textResult(string(raw), true)
}
