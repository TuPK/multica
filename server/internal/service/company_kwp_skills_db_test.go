package service

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func agentSkillNames(t *testing.T, e *companyEnv, agentID pgtype.UUID) []string {
	t.Helper()
	rows, err := e.ke.q.ListAgentSkillSummaries(e.ctx, agentID)
	if err != nil {
		t.Fatalf("ListAgentSkillSummaries: %v", err)
	}
	var names []string
	for _, r := range rows {
		names = append(names, r.Name)
	}
	slices.Sort(names)
	return names
}

// TestCompanyApplyKwpSkillsAttachedAndIdempotent checks that starter roles get their
// KWP skills attached, and that rerunning apply does not duplicate agents or skills.
func TestCompanyApplyKwpSkillsAttachedAndIdempotent(t *testing.T) {
	e := newCompanyEnv(t)
	e.enableKnowledge()

	res1 := e.apply([]string{"support", "release_ops"}, CompanyStartLeavePaused)
	if failed := failedSteps(res1); len(failed) > 0 {
		t.Fatalf("first apply failed: %+v", failed)
	}

	supportAgent := e.agentNamed("Support Agent")
	agentSkills := agentSkillNames(t, e, supportAgent.ID)
	wantSupportAgent := []string{"kwp-kb-article", "kwp-ticket-triage", "kwp-untrusted-content"}
	for _, want := range wantSupportAgent {
		if !slices.Contains(agentSkills, want) {
			t.Errorf("Support Agent missing skill %q; has %v", want, agentSkills)
		}
	}

	supportLead := e.agentNamed("Support Lead")
	leadSkills := agentSkillNames(t, e, supportLead.ID)
	if !slices.Contains(leadSkills, "kwp-customer-escalation") {
		t.Errorf("Support Lead missing kwp-customer-escalation; has %v", leadSkills)
	}

	releaseEng := e.agentNamed("Release Engineer")
	relSkills := agentSkillNames(t, e, releaseEng.ID)
	wantRel := []string{"kwp-change-request", "kwp-incident-response", "kwp-runbook"}
	for _, want := range wantRel {
		if !slices.Contains(relSkills, want) {
			t.Errorf("Release Engineer missing skill %q; has %v", want, relSkills)
		}
	}

	before := e.snapshot()

	// Second apply must be idempotent
	res2 := e.apply([]string{"support", "release_ops"}, CompanyStartLeavePaused)
	if failed := failedSteps(res2); len(failed) > 0 {
		t.Fatalf("second apply failed: %+v", failed)
	}

	for _, step := range res2.Steps {
		if step.Status != CompanyStepReused {
			t.Errorf("step %s had status %s (%s), want %s", step.Key, step.Status, step.Message, CompanyStepReused)
		}
	}

	after := e.snapshot()
	if before["agents"] != after["agents"] {
		t.Errorf("agents changed on rerun: %d -> %d", before["agents"], after["agents"])
	}
}

// TestCompanyApplyPreexistingSkillNotOverwritten checks that a user-created skill
// with the same name as a starter skill is not overwritten or attached as a starter skill.
func TestCompanyApplyPreexistingSkillNotOverwritten(t *testing.T) {
	e := newCompanyEnv(t)
	e.enableKnowledge()

	// Remove the starter skill installed by setup to simulate a pre-existing custom skill
	for _, stmt := range []string{
		`DELETE FROM agent_skill WHERE skill_id IN (SELECT id FROM skill WHERE workspace_id = $1 AND name = $2)`,
		`DELETE FROM skill_file WHERE skill_id IN (SELECT id FROM skill WHERE workspace_id = $1 AND name = $2)`,
		`DELETE FROM skill WHERE workspace_id = $1 AND name = $2`,
	} {
		if _, err := e.ke.pool.Exec(e.ctx, stmt, e.ws, "kwp-ticket-triage"); err != nil {
			t.Fatalf("delete starter skill: %v", err)
		}
	}

	// User creates a custom skill named kwp-ticket-triage before apply
	customContent := "User custom skill content"
	_, err := e.ke.q.CreateSkill(e.ctx, db.CreateSkillParams{
		WorkspaceID: e.ws,
		Name:        "kwp-ticket-triage",
		Description: "Boss triage skill",
		Content:     customContent,
		Config:      []byte(`{"custom": true}`),
		CreatedBy:   e.user,
	})
	if err != nil {
		t.Fatalf("CreateSkill: %v", err)
	}

	res := e.apply([]string{"support"}, CompanyStartLeavePaused)
	if failed := failedSteps(res); len(failed) > 0 {
		t.Fatalf("apply failed: %+v", failed)
	}

	skill, err := e.ke.q.GetSkillByWorkspaceAndName(e.ctx, db.GetSkillByWorkspaceAndNameParams{
		WorkspaceID: e.ws,
		Name:        "kwp-ticket-triage",
	})
	if err != nil {
		t.Fatalf("GetSkillByWorkspaceAndName: %v", err)
	}
	if skill.Content != customContent {
		t.Errorf("custom skill was overwritten: got %q, want %q", skill.Content, customContent)
	}

	// Because it's not a starter skill config, attachStarterSkills skips it
	supportAgent := e.agentNamed("Support Agent")
	skills := agentSkillNames(t, e, supportAgent.ID)
	if slices.Contains(skills, "kwp-ticket-triage") {
		t.Errorf("user-owned skill kwp-ticket-triage should not have been attached to agent; got %v", skills)
	}
}

// TestCompanyRerunOnExistingSquadAttachesNewRoleSkills (Gate G4) verifies that
// when existing squads (growth_gtm, support) are already present in the workspace,
// rerunning apply preserves existing agents, creates any missing role on an existing
// squad, leaves growth_gtm without KWP skills, and attaches new role skills to support roles.
func TestCompanyRerunOnExistingSquadAttachesNewRoleSkills(t *testing.T) {
	e := newCompanyEnv(t)
	e.enableKnowledge()

	supportCard, ok := e.data.squad("support")
	if !ok {
		t.Fatal("support squad card not found in data")
	}
	growthCard, ok := e.data.squad("growth_gtm")
	if !ok {
		t.Fatal("growth_gtm squad card not found in data")
	}

	fx := testutil.New(e.ke.pool, e.wsID, e.userID)
	launchFloorID := fx.Insert(t, "company_floor", testutil.Cols{"workspace_id": e.wsID, "name": e.data.FloorNames[supportCard.Floor], "position": 0})

	// Simulate an older apply: growth_gtm exists, and support exists with Support Lead
	// and Support Agent having 0 skills.
	growthLeadID := fx.Agent(t, "Growth Lead", "")
	growthSquadID := fx.Squad(t, growthCard.Name, growthLeadID)
	fx.Exec(t, `UPDATE squad SET floor_id = $1, company_slot = 0 WHERE id = $2`, launchFloorID, growthSquadID)
	fx.SquadMember(t, growthSquadID, "agent", growthLeadID, testutil.Cols{"role": "leader"})

	leadAgentID := fx.Agent(t, "Support Lead", "")
	workerAgentID := fx.Agent(t, "Support Agent", "")
	squadID := fx.Squad(t, supportCard.Name, leadAgentID)
	fx.Exec(t, `UPDATE squad SET floor_id = $1, company_slot = 1 WHERE id = $2`, launchFloorID, squadID)
	fx.SquadMember(t, squadID, "agent", leadAgentID, testutil.Cols{"role": "leader"})
	fx.SquadMember(t, squadID, "agent", workerAgentID, testutil.Cols{"role": "member"})

	// Simulate release_ops existing with Release Lead only (missing Release Engineer).
	releaseCard, ok := e.data.squad("release_ops")
	if !ok {
		t.Fatal("release_ops squad card not found in data")
	}
	releaseLeadID := fx.Agent(t, "Release Lead", "")
	releaseSquadID := fx.Squad(t, releaseCard.Name, releaseLeadID)
	fx.Exec(t, `UPDATE squad SET floor_id = $1, company_slot = 2 WHERE id = $2`, launchFloorID, releaseSquadID)
	fx.SquadMember(t, releaseSquadID, "agent", releaseLeadID, testutil.Cols{"role": "leader"})

	// Verify Support Agent and Growth Lead have 0 skills before apply.
	if beforeSkills := agentSkillNames(t, e, mustUUID(t, workerAgentID)); len(beforeSkills) != 0 {
		t.Fatalf("expected 0 skills before apply, got %v", beforeSkills)
	}
	if beforeGrowth := agentSkillNames(t, e, mustUUID(t, growthLeadID)); len(beforeGrowth) != 0 {
		t.Fatalf("expected 0 skills on Growth Lead before apply, got %v", beforeGrowth)
	}

	// Run apply with the new company template.
	res := e.apply([]string{"growth_gtm", "support", "release_ops"}, CompanyStartLeavePaused)
	if failed := failedSteps(res); len(failed) > 0 {
		t.Fatalf("apply failed: %+v", failed)
	}

	// Existing agents keep their exact IDs.
	fetchedGrowth := e.agentNamed("Growth Lead")
	if fetchedGrowth.ID != mustUUID(t, growthLeadID) {
		t.Errorf("Growth Lead agent ID changed: got %v, want %v", fetchedGrowth.ID, growthLeadID)
	}
	growthSkills := agentSkillNames(t, e, fetchedGrowth.ID)
	for _, sk := range growthSkills {
		if len(sk) >= 4 && sk[:4] == "kwp-" {
			t.Errorf("growth_gtm should not receive KWP skills, got %v", growthSkills)
		}
	}

	fetchedLead := e.agentNamed("Support Lead")
	if fetchedLead.ID != mustUUID(t, leadAgentID) {
		t.Errorf("Support Lead agent ID changed: got %v, want %v", fetchedLead.ID, leadAgentID)
	}
	leadSkills := agentSkillNames(t, e, fetchedLead.ID)
	for _, want := range []string{StarterSkillSquadReporting, StarterSkillDirectiveExecution, "kwp-customer-escalation"} {
		if !slices.Contains(leadSkills, want) {
			t.Errorf("Support Lead missing skill %q after rerun apply; has %v", want, leadSkills)
		}
	}

	fetchedWorker := e.agentNamed("Support Agent")
	if fetchedWorker.ID != mustUUID(t, workerAgentID) {
		t.Errorf("worker agent ID changed: got %v, want %v", fetchedWorker.ID, workerAgentID)
	}

	// Support Agent now has its role skills attached.
	afterSkills := agentSkillNames(t, e, fetchedWorker.ID)
	wantWorkerSkills := []string{"kwp-kb-article", "kwp-ticket-triage", "kwp-untrusted-content"}
	for _, want := range wantWorkerSkills {
		if !slices.Contains(afterSkills, want) {
			t.Errorf("Support Agent missing skill %q after rerun apply; has %v", want, afterSkills)
		}
	}

	// Missing role (Release Engineer) in an existing squad (release_ops) is created and receives its KWP skills.
	releaseEng := e.agentNamed("Release Engineer")
	relEngSkills := agentSkillNames(t, e, releaseEng.ID)
	for _, want := range []string{"kwp-change-request", "kwp-incident-response", "kwp-runbook"} {
		if !slices.Contains(relEngSkills, want) {
			t.Errorf("created Release Engineer missing skill %q; has %v", want, relEngSkills)
		}
	}
}
