package service

import (
	"testing"

	"github.com/google/uuid"
)

// A task on a runtime staff disabled is refused at the claim gate and stays
// queued, and it is admitted again once the runtime is enabled (spec R6 section
// 3b.5, D98).
func TestClaimRefusesTaskOnDisabledRuntime(t *testing.T) {
	f := newRunVisibilityFixture(t)
	taskID := f.queuedTask(t, "")

	f.Exec(t, `INSERT INTO platform_runtime (runtime_id, changed_by) VALUES ($1::uuid, $2::uuid)`, f.runtimeID, uuid.NewString())
	refusal := f.refusal(t, taskID)
	if refusal == nil || refusal.Reason != VerdictReasonRuntimeDisabled {
		t.Fatalf("refusal = %+v, want runtime_disabled", refusal)
	}
	if row := f.taskRow(t, taskID); row.Status != "queued" {
		t.Fatalf("task status = %q, want queued", row.Status)
	}

	f.Exec(t, `DELETE FROM platform_runtime WHERE runtime_id = $1::uuid`, f.runtimeID)
	if refusal := f.refusal(t, taskID); refusal != nil && refusal.Reason == VerdictReasonRuntimeDisabled {
		t.Fatalf("enabled runtime still refused: %+v", refusal)
	}
}
