package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// shadowE2EClaim is the {"tasks":[...]} envelope of a batch claim, with the fields a shadow run
// depends on.
type shadowE2EClaim struct {
	Tasks []struct {
		ID         string `json:"id"`
		Shadow     bool   `json:"shadow"`
		OmitSkills bool   `json:"omit_skills"`
	} `json:"tasks"`
}

// TestShadowRunFromTheClaimToItsVerdictAndCompletion runs a whole shadow run through the real
// claim handler and the completion handler, over the database. Five pairs are needed: the run
// is completed only at its target, and the schema holds a target of at least five. Each pair:
// the seat's primary is claimed (the shadow task is made then), the candidate's shadow task is
// claimed by a shadow-capable daemon only, both sides complete, the juror's verification task is
// claimed and completes with a verdict, and the candidate wins it. At the fifth verdict the run
// completes and its runtime change is filed for the owner (spec 9.4 to 9.6).
func TestShadowRunFromTheClaimToItsVerdictAndCompletion(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
	shadowE2EWireFlags(t)
	ctx := context.Background()
	suffix := uuid.NewString()[:8]

	seatRuntime := createClaimReclaimRuntime(t, ctx, "shadow e2e seat "+suffix)
	seatAgent, _ := createClaimReclaimAgentAndIssue(t, ctx, seatRuntime, "shadow e2e seat "+suffix)
	squad := digestEnv{WorkspaceID: testWorkspaceID, RuntimeID: seatRuntime, AgentID: seatAgent}.squad(t, "shadow e2e squad "+suffix)
	if _, err := testPool.Exec(ctx, `UPDATE agent SET home_squad_id = $2 WHERE id = $1`, seatAgent, squad); err != nil {
		t.Fatal(err)
	}
	// The completed run files a pending runtime change in the workspace. Pending proposals count
	// against the workspace-wide limit that other tests in this package measure, so remove them.
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM proposal WHERE squad_id = $1`, squad)
	})
	candidateRuntime := createClaimReclaimRuntime(t, ctx, "shadow e2e candidate "+suffix)
	jurorRuntime := createClaimReclaimRuntime(t, ctx, "shadow e2e juror "+suffix)
	jurorAgent := dbfx.Agent(t, "shadow e2e juror "+suffix, jurorRuntime)
	shadowE2EJuryPool(t, ctx, jurorAgent)

	w := httptest.NewRecorder()
	req := seatRequest("POST", "/api/agents/"+seatAgent+"/shadow", testWorkspaceID, map[string]any{
		"candidate_runtime_id": candidateRuntime, "candidate_model": "candidate-" + suffix,
		"sample_rate": 1, "target_pairs": 5, "confirm_cost": true,
	}, "")
	req = withURLParam(req, "id", seatAgent)
	testHandler.StartShadowRun(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("start the run: status %d: %s", w.Code, w.Body.String())
	}
	var configID string
	if err := testPool.QueryRow(ctx, `SELECT id::text FROM shadow_config WHERE agent_id = $1 AND state = 'running'`, seatAgent).Scan(&configID); err != nil {
		t.Fatalf("load the running shadow run: %v", err)
	}

	for pair := 1; pair <= 5; pair++ {
		issue := shadowE2EIssue(t, ctx, "shadow e2e pair "+suffix)
		primary := seedQueuedIssueTask(t, ctx, seatAgent, seatRuntime, issue)

		// The seat's primary is claimed by a shadow-capable daemon. The claim samples it, so
		// its pair and its shadow task now exist.
		got := testutil.Decode[shadowE2EClaim](t, testHandler.ClaimTasksByRuntime,
			batchClaimRequest(testWorkspaceID, []string{seatRuntime}, 1, protocol.DaemonCapabilityShadowV1), http.StatusOK)
		if len(got.Tasks) != 1 || got.Tasks[0].ID != primary || got.Tasks[0].Shadow {
			t.Fatalf("pair %d: seat claim = %+v, want its primary %s as an ordinary task", pair, got.Tasks, primary)
		}
		var pairID, shadowTask string
		if err := testPool.QueryRow(ctx, `
			SELECT id::text, shadow_task_id::text FROM shadow_pair
			WHERE primary_task_id = $1 AND config_id = $2`, primary, configID).Scan(&pairID, &shadowTask); err != nil {
			t.Fatalf("pair %d: the primary's claim made no pair: %v", pair, err)
		}

		// The shadow task waits for a shadow-capable daemon on the candidate runtime.
		refused := testutil.Decode[shadowE2EClaim](t, testHandler.ClaimTasksByRuntime,
			batchClaimRequest(testWorkspaceID, []string{candidateRuntime}, 1, ""), http.StatusOK)
		if len(refused.Tasks) != 0 {
			t.Fatalf("pair %d: a daemon without shadow-v1 was given %+v", pair, refused.Tasks)
		}
		var status string
		if err := testPool.QueryRow(ctx, `SELECT status FROM agent_task_queue WHERE id = $1`, shadowTask).Scan(&status); err != nil {
			t.Fatal(err)
		}
		if status != "queued" {
			t.Fatalf("pair %d: refused shadow task is %q, want queued", pair, status)
		}

		shadowClaim := testutil.Decode[shadowE2EClaim](t, testHandler.ClaimTasksByRuntime,
			batchClaimRequest(testWorkspaceID, []string{candidateRuntime}, 1, protocol.DaemonCapabilityShadowV1), http.StatusOK)
		if len(shadowClaim.Tasks) != 1 || shadowClaim.Tasks[0].ID != shadowTask {
			t.Fatalf("pair %d: shadow-v1 claim = %+v, want the shadow task %s", pair, shadowClaim.Tasks, shadowTask)
		}
		if !shadowClaim.Tasks[0].Shadow || shadowClaim.Tasks[0].OmitSkills {
			t.Fatalf("pair %d: claimed model_trial shadow task = %+v, want shadow=true and omit_skills=false", pair, shadowClaim.Tasks[0])
		}

		shadowE2ETaskCall(t, primary, "start", map[string]any{}, testHandler.StartTask)
		shadowE2ETaskCall(t, primary, "complete", map[string]any{"output": "primary answer"}, testHandler.CompleteTask)
		shadowE2ETaskCall(t, shadowTask, "start", map[string]any{}, testHandler.StartTask)
		shadowE2ETaskCall(t, shadowTask, "complete", map[string]any{"output": "shadow answer"}, testHandler.CompleteTask)

		// Both sides completed: a juror's verification task is given to the pool agent.
		var jurorTask, jurorAgentID string
		if err := testPool.QueryRow(ctx, `
			SELECT id::text, agent_id::text FROM agent_task_queue
			WHERE context->>'durable_kind' = 'verification' AND context->>'shadow_pair_id' = $1`, pairID).Scan(&jurorTask, &jurorAgentID); err != nil {
			t.Fatalf("pair %d: no juror task after both sides completed: %v", pair, err)
		}
		if jurorAgentID != jurorAgent {
			t.Fatalf("pair %d: juror task belongs to %s, want the pool agent %s", pair, jurorAgentID, jurorAgent)
		}

		juror := testutil.Decode[shadowE2EClaim](t, testHandler.ClaimTasksByRuntime,
			batchClaimRequest(testWorkspaceID, []string{jurorRuntime}, 1, ""), http.StatusOK)
		if len(juror.Tasks) != 1 || juror.Tasks[0].ID != jurorTask {
			t.Fatalf("pair %d: juror claim = %+v, want %s", pair, juror.Tasks, jurorTask)
		}
		// The juror sees labels, not sides. The candidate's label is the one that is not the primary's.
		var pairUUID string
		if err := testPool.QueryRow(ctx, `SELECT id::text FROM shadow_pair WHERE id = $1`, pairID).Scan(&pairUUID); err != nil {
			t.Fatal(err)
		}
		verdict := "A"
		if service.ShadowPrimaryIsA(pairUUID) {
			verdict = "B"
		}
		shadowE2ETaskCall(t, jurorTask, "start", map[string]any{}, testHandler.StartTask)
		shadowE2ETaskCall(t, jurorTask, "complete", map[string]any{"output": "The second answer is better.\nVERDICT: " + verdict}, testHandler.CompleteTask)

		var comparison string
		if err := testPool.QueryRow(ctx, `SELECT comparison FROM shadow_pair WHERE id = $1`, pairID).Scan(&comparison); err != nil {
			t.Fatal(err)
		}
		if comparison != string(service.ComparisonShadow) {
			t.Fatalf("pair %d: comparison = %q, want %q (the candidate's answer won the verdict %s)", pair, comparison, service.ComparisonShadow, verdict)
		}
	}

	var state string
	var completedAt *string
	if err := testPool.QueryRow(ctx, `SELECT state, completed_at::text FROM shadow_config WHERE id = $1`, configID).Scan(&state, &completedAt); err != nil {
		t.Fatal(err)
	}
	if state != "completed" || completedAt == nil {
		t.Fatalf("shadow run state = %q (completed_at %v) after five verdicts, want completed", state, completedAt)
	}
	var proposals int
	if err := testPool.QueryRow(ctx, `SELECT count(*) FROM proposal WHERE workspace_id = $1 AND squad_id = $2`, testWorkspaceID, squad).Scan(&proposals); err != nil {
		t.Fatal(err)
	}
	if proposals == 0 {
		t.Fatalf("the completed run filed no runtime change for its squad")
	}
}

// shadowE2EWireFlags points the task service's shadow run at the test's flags. withFeatureFlag
// replaces the handler's flags, but the shadow service the task service was built with keeps the
// flags it was given at construction; production shares one flags service, so only the test needs
// this. The original flags are restored when the test ends.
func shadowE2EWireFlags(t *testing.T) {
	t.Helper()
	shadow := testHandler.TaskService.Shadow
	if shadow == nil {
		t.Fatal("the task service has no shadow run service")
	}
	origFlags, origRuntimeFlags := shadow.Flags, shadow.Runtime.Flags
	shadow.Flags, shadow.Runtime.Flags = testHandler.FeatureFlags, testHandler.FeatureFlags
	t.Cleanup(func() { shadow.Flags, shadow.Runtime.Flags = origFlags, origRuntimeFlags })
}

// shadowE2EJuryPool makes the test's juror the workspace's only jury pool agent, and restores the
// workspace settings when the test ends.
func shadowE2EJuryPool(t *testing.T, ctx context.Context, agentID string) {
	t.Helper()
	var orig string
	if err := testPool.QueryRow(ctx, `SELECT COALESCE(settings::text, '{}') FROM workspace WHERE id = $1`, testWorkspaceID).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	pool, err := json.Marshal([]string{agentID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `
		UPDATE workspace SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{jury_pool}', $2::jsonb)
		WHERE id = $1`, testWorkspaceID, string(pool)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `UPDATE workspace SET settings = $2::jsonb WHERE id = $1`, testWorkspaceID, orig)
	})
}

// shadowE2EIssue inserts an in-progress issue in the test workspace, so each pair has its own.
func shadowE2EIssue(t *testing.T, ctx context.Context, title string) string {
	t.Helper()
	var id string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_id, creator_type, number, position)
		VALUES (
			$1, $2, 'in_progress', 'none', $3, 'member',
			(SELECT COALESCE(MAX(number), 82649) + 1 FROM issue WHERE workspace_id = $1),
			0
		)
		RETURNING id::text`, testWorkspaceID, title+" "+uuid.NewString()[:8], testUserID).Scan(&id); err != nil {
		t.Fatalf("insert issue: %v", err)
	}
	return id
}

// shadowE2ETaskCall drives one task through a daemon handler (start or complete) as its owner.
func shadowE2ETaskCall(t *testing.T, taskID, action string, body map[string]any, handler func(http.ResponseWriter, *http.Request)) {
	t.Helper()
	w := httptest.NewRecorder()
	req := newOwnerDaemonRequest("POST", "/api/daemon/tasks/"+taskID+"/"+action, body)
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add("taskId", taskID)
	req = req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("%s task %s: status %d: %s", action, taskID, w.Code, w.Body.String())
	}
}

// TestShadowClaimSkillsPolicyInHandler verifies that a model_trial shadow claim keeps the seat's
// skills (OmitSkills=false) while keeping write isolation (Shadow=true, ProjectResources=nil),
// necessity_test omits skills, and the normal non-shadow path is unchanged (durable spec 9.4, 10.2).
func TestShadowClaimSkillsPolicyInHandler(t *testing.T) {
	t.Run("applyShadowClaimFields by purpose and normal path", func(t *testing.T) {
		sampleSkills := []service.AgentSkillData{{ID: "s1", Name: "seat-skill"}}
		sampleRefs := []service.AgentSkillRefData{{ID: "s1", Name: "seat-skill"}}
		sampleResources := []ProjectResourceData{{ID: "r1", ResourceType: "git_repo"}}

		// model_trial: shadow=true, omit_skills=false, skills kept, project resources stripped.
		mt := AgentTaskResponse{
			Agent:            &TaskAgentData{Skills: sampleSkills, SkillRefs: sampleRefs},
			ProjectResources: sampleResources,
		}
		applyShadowClaimFields(&mt, []byte(`{"durable_kind":"shadow","shadow_purpose":"model_trial","omit_skills":false}`))
		if !mt.Shadow || mt.OmitSkills {
			t.Fatalf("model_trial: shadow=%v omit_skills=%v, want true/false", mt.Shadow, mt.OmitSkills)
		}
		if len(mt.Agent.Skills) != 1 || len(mt.Agent.SkillRefs) != 1 {
			t.Fatalf("model_trial stripped skills: skills=%v refs=%v", mt.Agent.Skills, mt.Agent.SkillRefs)
		}
		if mt.ProjectResources != nil {
			t.Fatalf("model_trial kept project resources: %v", mt.ProjectResources)
		}

		// necessity_test: shadow=true, omit_skills=true, skills stripped, project resources stripped.
		nt := AgentTaskResponse{
			Agent:            &TaskAgentData{Skills: sampleSkills, SkillRefs: sampleRefs},
			ProjectResources: sampleResources,
		}
		applyShadowClaimFields(&nt, []byte(`{"durable_kind":"shadow","shadow_purpose":"necessity_test","omit_skills":true}`))
		if !nt.Shadow || !nt.OmitSkills {
			t.Fatalf("necessity_test: shadow=%v omit_skills=%v, want true/true", nt.Shadow, nt.OmitSkills)
		}
		if nt.Agent.Skills != nil || nt.Agent.SkillRefs != nil {
			t.Fatalf("necessity_test kept skills: skills=%v refs=%v", nt.Agent.Skills, nt.Agent.SkillRefs)
		}
		if nt.ProjectResources != nil {
			t.Fatalf("necessity_test kept project resources: %v", nt.ProjectResources)
		}

		// normal (non-shadow) task: untouched.
		plain := AgentTaskResponse{
			Agent:            &TaskAgentData{Skills: sampleSkills, SkillRefs: sampleRefs},
			ProjectResources: sampleResources,
		}
		applyShadowClaimFields(&plain, []byte(`{}`))
		if plain.Shadow || plain.OmitSkills {
			t.Fatalf("normal task: shadow=%v omit_skills=%v, want false/false", plain.Shadow, plain.OmitSkills)
		}
		if len(plain.Agent.Skills) != 1 || len(plain.Agent.SkillRefs) != 1 || len(plain.ProjectResources) != 1 {
			t.Fatalf("normal task mutated: skills=%v refs=%v resources=%v", plain.Agent.Skills, plain.Agent.SkillRefs, plain.ProjectResources)
		}
	})

	t.Run("model_trial claim handler returns seat skills with omit_skills=false", func(t *testing.T) {
		if testHandler == nil || testPool == nil {
			t.Skip("database not available")
		}
		withFeatureFlag(t, testHandler, featureflags.DurableSeats, true)
		shadowE2EWireFlags(t)
		ctx := context.Background()
		suffix := uuid.NewString()[:8]

		seatRuntime := createClaimReclaimRuntime(t, ctx, "shadow skills seat "+suffix)
		seatAgent, _ := createClaimReclaimAgentAndIssue(t, ctx, seatRuntime, "shadow skills seat "+suffix)
		candidateRuntime := createClaimReclaimRuntime(t, ctx, "shadow skills cand "+suffix)

		skillName := "shadow-seat-skill-" + suffix
		var skillID string
		if err := testPool.QueryRow(ctx, `
			INSERT INTO skill (workspace_id, name, description, content)
			VALUES ($1, $2, 'seat skill', '# seat skill')
			RETURNING id::text`, testWorkspaceID, skillName).Scan(&skillID); err != nil {
			t.Fatalf("insert skill: %v", err)
		}
		t.Cleanup(func() {
			_, _ = testPool.Exec(context.Background(), `DELETE FROM agent_skill WHERE skill_id = $1`, skillID)
			_, _ = testPool.Exec(context.Background(), `DELETE FROM skill WHERE id = $1`, skillID)
		})
		if _, err := testPool.Exec(ctx, `INSERT INTO agent_skill (agent_id, skill_id) VALUES ($1, $2)`, seatAgent, skillID); err != nil {
			t.Fatalf("bind seat skill: %v", err)
		}

		w := httptest.NewRecorder()
		req := seatRequest("POST", "/api/agents/"+seatAgent+"/shadow", testWorkspaceID, map[string]any{
			"candidate_runtime_id": candidateRuntime, "candidate_model": "candidate-" + suffix,
			"sample_rate": 1, "target_pairs": 5, "confirm_cost": true,
		}, "")
		req = withURLParam(req, "id", seatAgent)
		testHandler.StartShadowRun(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("start shadow run: status %d: %s", w.Code, w.Body.String())
		}

		issue := shadowE2EIssue(t, ctx, "shadow skills issue "+suffix)
		primary := seedQueuedIssueTask(t, ctx, seatAgent, seatRuntime, issue)

		// Claim primary -> creates shadow pair and task.
		primaryClaim := testutil.Decode[struct {
			Tasks []AgentTaskResponse `json:"tasks"`
		}](t, testHandler.ClaimTasksByRuntime,
			batchClaimRequest(testWorkspaceID, []string{seatRuntime}, 1, protocol.DaemonCapabilityShadowV1), http.StatusOK)
		if len(primaryClaim.Tasks) != 1 || primaryClaim.Tasks[0].ID != primary {
			t.Fatalf("primary claim = %+v, want %s", primaryClaim.Tasks, primary)
		}

		// Claim shadow task -> must keep seat's skill and set Shadow=true, OmitSkills=false.
		shadowClaim := testutil.Decode[struct {
			Tasks []AgentTaskResponse `json:"tasks"`
		}](t, testHandler.ClaimTasksByRuntime,
			batchClaimRequest(testWorkspaceID, []string{candidateRuntime}, 1, protocol.DaemonCapabilityShadowV1), http.StatusOK)
		if len(shadowClaim.Tasks) != 1 {
			t.Fatalf("shadow claim got %d tasks, want 1", len(shadowClaim.Tasks))
		}
		got := shadowClaim.Tasks[0]
		if !got.Shadow || got.OmitSkills {
			t.Fatalf("model_trial shadow claim: shadow=%v omit_skills=%v, want true/false", got.Shadow, got.OmitSkills)
		}
		if got.Agent == nil {
			t.Fatalf("model_trial shadow claim has nil agent")
		}
		hasSeatSkill := false
		for _, sk := range got.Agent.Skills {
			if sk.Name == skillName {
				hasSeatSkill = true
			}
		}
		if !hasSeatSkill {
			t.Fatalf("model_trial shadow claim missing seat skill %q; got skills=%+v", skillName, got.Agent.Skills)
		}
	})
}
