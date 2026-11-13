package handler

import (
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// The rules every Kit Viet flow manifest (catalog/workflows/flow_kv_*) must keep. They are the rules
// Trade was fixed for (O2 and O5): a collect task that makes a file requires an attachment, and a gate
// tells a blocked verdict (stop 1, grounds missing) apart from a rejected one (rework).

// kitVietFlowIDs lists the 21 squad flows. A new squad flow is added here.
var kitVietFlowIDs = []string{
	"flow_kv_academic", "flow_kv_accounting", "flow_kv_admin", "flow_kv_b2b", "flow_kv_banking",
	"flow_kv_construction", "flow_kv_ecom", "flow_kv_education", "flow_kv_factory", "flow_kv_fnb",
	"flow_kv_health", "flow_kv_hospitality", "flow_kv_insurance", "flow_kv_legal", "flow_kv_marketing",
	"flow_kv_officeops", "flow_kv_pr", "flow_kv_realestate", "flow_kv_talent", "flow_kv_teaching",
	"flow_kv_trade",
}

// flowManifestViolations returns the rules the flow breaks, one message each.
func flowManifestViolations(flow catalogFlow) []string {
	var out []string
	add := func(format string, args ...any) { out = append(out, fmt.Sprintf(format, args...)) }
	for _, s := range flow.Steps {
		hasGate := false
		for _, task := range s.Tasks {
			ins := strings.ToLower(task.Instructions)
			for _, r := range task.Requires {
				if r != issueworkflow.RequireOutcome && r != issueworkflow.RequireAttachment {
					add("task %s requires %q; the engine knows only outcome and attachment", task.Key, r)
				}
			}
			needsFile := slices.Contains(task.Requires, issueworkflow.RequireAttachment)
			if needsFile && !strings.Contains(ins, "đính kèm") {
				add("task %s requires an attachment but its instructions never say to attach one", task.Key)
			}
			if strings.Contains(ins, "thành tệp") && !needsFile {
				add("task %s says to attach its output as a file but does not require an attachment", task.Key)
			}
			if task.Kind != string(issueworkflow.KindGate) {
				continue
			}
			hasGate = true
			if !slices.Contains(task.Requires, issueworkflow.RequireOutcome) {
				add("gate %s does not require an outcome", task.Key)
			}
			for _, want := range []string{"blocked --outcome blocked", "rejected", "chỉ dùng rejected khi lỗi nằm ở bản nháp"} {
				if !strings.Contains(ins, want) {
					add("gate %s instructions do not contain %q, so the gate cannot tell blocked from rejected", task.Key, want)
				}
			}
		}
		if hasGate && !stepTransitionsTo(s.Transitions, "blocked") {
			add("step %s has gates but no transition to blocked (stop 1)", s.StatusKey)
		}
	}
	return out
}

// stepTransitionsTo reports whether one of the transitions leads to the status key.
func stepTransitionsTo(transitions []struct {
	ID     string          `json:"id"`
	Label  string          `json:"label"`
	When   json.RawMessage `json:"when"`
	To     string          `json:"to"`
	Rework bool            `json:"rework"`
}, to string) bool {
	for _, tr := range transitions {
		if tr.To == to {
			return true
		}
	}
	return false
}

func TestFlowManifests_RulesHoldForEverySquad(t *testing.T) {
	if len(kitVietFlowIDs) != 21 {
		t.Fatalf("kitVietFlowIDs lists %d flows, want the 21 squads", len(kitVietFlowIDs))
	}
	for _, id := range kitVietFlowIDs {
		t.Run(id, func(t *testing.T) {
			for _, v := range flowManifestViolations(loadCatalogFlow(t, id)) {
				t.Error(v)
			}
		})
	}
}

// Each manifest also compiles through the server: the rehearsal rig creates its workflow the way the
// product does, and the server validates it there.
func TestFlowManifests_EveryManifestCompiles(t *testing.T) {
	for _, id := range kitVietFlowIDs {
		t.Run(id, func(t *testing.T) {
			newSquadRig(t, strings.TrimPrefix(id, "flow_"))
		})
	}
}
