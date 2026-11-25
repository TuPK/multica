package connectedapps

import (
	"context"

	"github.com/jackc/pgx/v5/pgtype"
)

// Sources of an effect event (durable spec 4.5).
const (
	SourceConnectedApp   = "connected_app"
	SourcePluginAction   = "plugin_action"
	SourceCLI            = "cli"
	SourceChannel        = "channel"
	SourceWebhook        = "webhook"
	SourcePublishChannel = "publish_channel"
	SourceDeclared       = "declared"
)

// UndoSpec says how to take an effect back: call Tool with Args. An arg value that
// is a string starting with "result." is a path into the forward call's result,
// resolved when the forward call succeeds (ResolveUndo).
type UndoSpec struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args"`
}

// EffectRequest is one action as the gate sees it.
type EffectRequest struct {
	WS            pgtype.UUID
	TaskID        pgtype.UUID
	AgentID       pgtype.UUID
	MemberID      pgtype.UUID
	Source        string
	Target        string
	Class         string
	Reversibility string
	Summary       string
	Undo          *UndoSpec
	AppToolCallID pgtype.UUID
	IssueID       pgtype.UUID
	// Prior is the decision the caller already reached (the account rule and the
	// agent level). The gate can only make it stricter. It must be set.
	Prior string
}

// EffectRecord is the gate's answer: the final decision, and the event the gate
// wrote (invalid while durable_effects is off, when nothing is written).
type EffectRecord struct {
	Decision string
	EventID  pgtype.UUID
}

// EffectGate is the effect log and policy (durable spec 4.4). The service package
// implements it. The gateway takes it as an interface because the service package
// imports this one.
type EffectGate interface {
	Gate(ctx context.Context, in EffectRequest) (EffectRecord, error)
	Finish(ctx context.Context, workspaceID, eventID pgtype.UUID, outcome string, undo *UndoSpec) error
}

// ResolveUndo fills the "result." args of an undo from the forward call's result.
// It returns nil when spec is nil or an arg cannot be resolved, so the event keeps
// no undo rather than one that would call the wrong object.
func ResolveUndo(spec *UndoSpec, result map[string]any) *UndoSpec {
	if spec == nil {
		return nil
	}
	out := &UndoSpec{Tool: spec.Tool, Args: make(map[string]any, len(spec.Args))}
	for k, v := range spec.Args {
		s, ok := v.(string)
		if !ok || len(s) <= len("result.") || s[:len("result.")] != "result." {
			out.Args[k] = v
			continue
		}
		val, found := result[s[len("result."):]]
		if !found || val == nil {
			return nil
		}
		out.Args[k] = val
	}
	return out
}
