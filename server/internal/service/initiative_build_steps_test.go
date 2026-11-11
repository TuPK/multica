package service

import (
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
)

// A blocked SDLC issue must be handed to the squad's lead, which sends it back
// to In Progress for the developer; with no handler it sits unowned.
func TestSDLCStepsHandBlockedIssueToLeadAndBackToInProgress(t *testing.T) {
	agents := map[string]pgtype.UUID{}
	for i, role := range []string{"lead", "developer", "reviewer", "qa"} {
		agents[role] = pgtype.UUID{Bytes: [16]byte{byte(i + 1)}, Valid: true}
	}
	var blocked issueworkflow.Step
	found := false
	for _, s := range sdlcSteps(agents) {
		if s.StatusKey == issuestatus.Blocked {
			blocked, found = s, true
		}
	}
	if !found {
		t.Fatal("SDLC workflow has no blocked step")
	}
	if blocked.Handler.Type != issueworkflow.HandlerAgent || blocked.Handler.ID != util.UUIDToString(agents["lead"]) {
		t.Fatalf("blocked handler = %+v, want agent lead", blocked.Handler)
	}
	if blocked.BackStatusKey != sdlcInProgressStatusKey {
		t.Fatalf("blocked back status = %q, want %q", blocked.BackStatusKey, sdlcInProgressStatusKey)
	}
	if blocked.Instructions == "" {
		t.Fatal("blocked step has no instructions for the lead")
	}
}

func sdlcTestAgents(roles ...string) map[string]pgtype.UUID {
	agents := map[string]pgtype.UUID{}
	for i, role := range roles {
		agents[role] = pgtype.UUID{Bytes: [16]byte{byte(i + 1)}, Valid: true}
	}
	return agents
}

func sdlcStepByKey(steps []issueworkflow.Step, key string) (issueworkflow.Step, bool) {
	for _, s := range steps {
		if s.StatusKey == key {
			return s, true
		}
	}
	return issueworkflow.Step{}, false
}

// A squad with the specialists gets a step for each, so the lead can hand an
// unblock issue to the member who owns the blocker.
func TestSDLCStepsHandSpecialistStagesToTheirOwners(t *testing.T) {
	agents := sdlcTestAgents("lead", "developer", "reviewer", "qa", "analyst", "ux", "architect", "security", "devops")
	steps := sdlcSteps(agents)
	want := map[string]string{
		"analysis": "analyst", "design": "ux", "architecture": "architect", "plan_review": "reviewer",
		"security_review": "security", "release_prep": "devops", sdlcEnvironmentStatusKey: "devops", "harness": "architect",
	}
	for key, role := range want {
		step, ok := sdlcStepByKey(steps, key)
		if !ok {
			t.Fatalf("no %s step", key)
		}
		if step.Handler.ID != util.UUIDToString(agents[role]) {
			t.Fatalf("%s handler = %s, want %s", key, step.Handler.ID, role)
		}
		if step.NextStatusKey != "stage_done" || step.BackStatusKey != issuestatus.Blocked {
			t.Fatalf("%s next/back = %q/%q, want stage_done/blocked", key, step.NextStatusKey, step.BackStatusKey)
		}
	}
	for _, key := range []string{"stage_done", "plan_ready"} {
		if _, ok := sdlcStepByKey(steps, key); !ok {
			t.Fatalf("no %s step", key)
		}
	}
	// The build steps are unchanged.
	if dev, _ := sdlcStepByKey(steps, issuestatus.Todo); dev.Handler.ID != util.UUIDToString(agents["developer"]) {
		t.Fatal("todo is not the developer's")
	}
}

// A squad without a specialist has no step for it; the lead's playbook names the substitute.
func TestSDLCStepsOmitStagesWithoutAnAgent(t *testing.T) {
	steps := sdlcSteps(sdlcTestAgents("lead", "developer", "reviewer", "qa"))
	for _, key := range []string{"analysis", "design", "architecture", "security_review", "release_prep", sdlcEnvironmentStatusKey, "harness"} {
		if _, ok := sdlcStepByKey(steps, key); ok {
			t.Fatalf("unexpected %s step with no agent for it", key)
		}
	}
	if _, ok := sdlcStepByKey(steps, "plan_review"); !ok {
		t.Fatal("the reviewer is required, so plan_review stays")
	}
}

// Every step key the workflow names needs a status; the workflow create call fails otherwise.
func TestSDLCStatusesCoverEveryStageStep(t *testing.T) {
	agents := sdlcTestAgents("lead", "developer", "reviewer", "qa", "analyst", "ux", "architect", "security", "devops")
	have := map[string]bool{}
	for _, st := range sdlcStatusesFor(agents) {
		have[st.key] = true
	}
	for _, s := range sdlcSteps(agents) {
		if issueworkflow.IsFixed(s.StatusKey) || s.StatusKey == issuestatus.Todo || s.StatusKey == issuestatus.InProgress ||
			s.StatusKey == issuestatus.InReview {
			continue
		}
		if !have[s.StatusKey] {
			t.Fatalf("step %s has no status in sdlcStatusesFor", s.StatusKey)
		}
	}
}

// The blocked brief is repeated in templates.ts; the two must not drift.
func TestSDLCBlockedBriefMatchesTheTemplate(t *testing.T) {
	src, err := os.ReadFile("../../../packages/core/squads/templates.ts")
	if err != nil {
		t.Skipf("templates.ts not found: %v", err)
	}
	head := sdlcBlockedInstructions[:200]
	if !strings.Contains(string(src), head) {
		t.Fatalf("templates.ts does not start its blocked brief with %q", head)
	}
}
