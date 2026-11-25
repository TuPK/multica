package handler

import (
	"encoding/json"
	"testing"

	"github.com/multica-ai/multica/server/internal/connectedapps"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// optionFor returns the option of a cell for one decision.
func optionFor(t *testing.T, cell EffectPolicyCellResponse, decision string) EffectPolicyOptionResponse {
	t.Helper()
	for _, o := range cell.Options {
		if o.Decision == decision {
			return o
		}
	}
	t.Fatalf("no option %q in cell L%d %s", decision, cell.AutonomyLevel, cell.EffectClass)
	return EffectPolicyOptionResponse{}
}

// The grid shows the table's default, not one run's outcome: at L1 a send rehearses even
// though the run's send count is zero.
func TestEffectPolicyCellDefaultIsTheTable(t *testing.T) {
	cell := effectPolicyCell(1, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, nil)
	if cell.Default != connectedapps.PolicyRehearse {
		t.Fatalf("default = %q, want rehearse", cell.Default)
	}
	if cell.Override != nil {
		t.Fatalf("override = %v, want none", *cell.Override)
	}
}

// A decision looser than the default is locked below L3, with its reason; a stricter one is
// always open. The lock must match what the service refuses to save.
func TestEffectPolicyCellLocksLoosenings(t *testing.T) {
	cell := effectPolicyCell(2, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, nil)

	if got := optionFor(t, cell, connectedapps.PolicyAllow); got.Allowed || got.Reason != service.LockLooserAtL3 {
		t.Fatalf("allow at L2 = %+v, want locked with %q", got, service.LockLooserAtL3)
	}
	if got := optionFor(t, cell, connectedapps.PolicyBlock); !got.Allowed || got.Reason != "" {
		t.Fatalf("block at L2 = %+v, want open", got)
	}
}

func TestEffectPolicyCellNeverAllowsSpend(t *testing.T) {
	cell := effectPolicyCell(3, connectedapps.EffectSpend, connectedapps.ReversibilityIrreversible, nil)
	if got := optionFor(t, cell, connectedapps.PolicyAllow); got.Allowed || got.Reason != service.LockNeverAllow {
		t.Fatalf("allow for spend at L3 = %+v, want locked with %q", got, service.LockNeverAllow)
	}
}

func TestEffectPolicyCellReportsTheSavedOverride(t *testing.T) {
	overrides := []service.Override{{AutonomyLevel: 1, Effect: connectedapps.EffectSend, Reversibility: connectedapps.ReversibilityIrreversible, Decision: connectedapps.PolicyBlock}}
	cell := effectPolicyCell(1, connectedapps.EffectSend, connectedapps.ReversibilityIrreversible, overrides)
	if cell.Override == nil || *cell.Override != connectedapps.PolicyBlock {
		t.Fatalf("override = %v, want block", cell.Override)
	}
}

// The intent answer carries the call an approval runs, so the person can edit its text
// (durable spec 5.3). A declared intent with no call answers null.
func TestIntentResponseCarriesTheCall(t *testing.T) {
	it := db.EffectIntent{Call: []byte(`{"tool":"GMAIL_SEND_EMAIL","account_id":"acc-1","args":{"body":"hi"}}`), Preview: []byte(`{}`)}
	out := intentResponseOf(it)
	var got connectedapps.CanonicalCall
	if err := json.Unmarshal(out.Call, &got); err != nil {
		t.Fatalf("call does not decode: %v", err)
	}
	if got.Tool != "GMAIL_SEND_EMAIL" || got.Args["body"] != "hi" {
		t.Fatalf("call = %+v", got)
	}

	declared, err := json.Marshal(intentResponseOf(db.EffectIntent{Preview: []byte(`{}`)}))
	if err != nil {
		t.Fatalf("encode declared intent: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(declared, &fields); err != nil {
		t.Fatalf("decode declared intent: %v", err)
	}
	if string(fields["call"]) != "null" {
		t.Fatalf("declared call = %s, want null", fields["call"])
	}
}
