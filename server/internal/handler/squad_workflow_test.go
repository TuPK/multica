package handler

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// workspaceRequest is an authenticated request into a workspace other than the
// default test workspace.
func workspaceRequest(userID, workspaceID, method, path string, body any) *http.Request {
	req := newRequest(method, path, body)
	req.Header.Set("X-User-ID", userID)
	req.Header.Set("X-Workspace-ID", workspaceID)
	return req
}

// squadActiveWorkflowName reads the name of a squad's active workflow, or "".
func squadActiveWorkflowName(t *testing.T, squadID string) string {
	t.Helper()
	var name string
	dbfx.QueryRow(t, `SELECT COALESCE(w.name, '') FROM squad s
		LEFT JOIN issue_workflow w ON w.id = s.active_workflow_id WHERE s.id = $1`, squadID).Scan(&name)
	return name
}

// squadActiveWorkflowID reads a squad's active workflow id, or "" when it has none.
func squadActiveWorkflowID(t *testing.T, squadID string) string {
	t.Helper()
	var id *string
	dbfx.QueryRow(t, `SELECT active_workflow_id::text FROM squad WHERE id = $1`, squadID).Scan(&id)
	if id == nil {
		return ""
	}
	return *id
}

// agentNamedInSquad returns the id of the squad's agent with this name.
func agentNamedInSquad(t *testing.T, squadID, name string) string {
	t.Helper()
	var id string
	dbfx.QueryRow(t, `SELECT a.id::text FROM squad_member sm JOIN agent a ON a.id = sm.member_id
		WHERE sm.squad_id = $1 AND sm.member_type = 'agent' AND a.name = $2`, squadID, name).Scan(&id)
	return id
}

// squadWorkflowIssueStatus reads an issue's status.
func squadWorkflowIssueStatus(t *testing.T, issueID string) string {
	t.Helper()
	var status string
	dbfx.QueryRow(t, `SELECT status FROM issue WHERE id = $1`, issueID).Scan(&status)
	return status
}

// insertSquadWorkflow stores a workflow in a squad's library, listing the given
// statuses with no handoffs, and removes it when the test ends.
func insertSquadWorkflow(t *testing.T, workspaceID, squadID, name string, keys ...string) string {
	t.Helper()
	steps := make([]issueworkflow.Step, 0, len(keys))
	for _, key := range keys {
		steps = append(steps, issueworkflow.Step{StatusKey: key, Handler: issueworkflow.Handler{Type: issueworkflow.HandlerNone}})
	}
	raw, err := issueworkflow.EncodeSteps(issueworkflow.Arrange(steps))
	if err != nil {
		t.Fatalf("encode steps: %v", err)
	}
	row, err := testHandler.Queries.CreateIssueWorkflow(context.Background(), db.CreateIssueWorkflowParams{
		WorkspaceID:      util.MustParseUUID(workspaceID),
		Name:             name,
		InitialStatusKey: issueworkflow.StartStatus,
		Steps:            raw,
		SquadID:          util.MustParseUUID(squadID),
	})
	if err != nil {
		t.Fatalf("insert squad workflow: %v", err)
	}
	id := util.UUIDToString(row.ID)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, id)
	})
	return id
}

// setSquadActiveWorkflow posts a squad's active-workflow switch as the workspace admin.
func setSquadActiveWorkflow(t *testing.T, fx iniWorkspace, squadID, workflowID string, mapping map[string]string, dryRun bool) *testutil.Response {
	t.Helper()
	body := map[string]any{"workflow_id": workflowID}
	if mapping != nil {
		body["status_mapping"] = mapping
	}
	if dryRun {
		body["dry_run"] = true
	}
	req := newRequestAs(fx.AdminID, "POST", "/api/squads/"+squadID+"/workflow", body)
	req = withURLParams(req, "workspaceId", fx.ID, "id", squadID)
	return testutil.Call(t, testHandler.SetSquadActiveWorkflow, req)
}

// TestSquadsOnOneProjectRouteIssuesThroughTheirActiveWorkflows: two squads on one
// project each use their own active workflow for their issues. An agent's issue
// follows its home squad, and an issue with no assignee keeps the project's.
func TestSquadsOnOneProjectRouteIssuesThroughTheirActiveWorkflows(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Shared routing")
	second := sdlcSquad(t, fx, "Routing second")
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
	assignSquadProject(t, fx, second, projectID, nil).Want(http.StatusOK)
	if got := squadActiveWorkflowName(t, second); !strings.HasPrefix(got, "SDLC Auto (") {
		t.Fatalf("second squad active workflow = %q, want its own SDLC Auto copy", got)
	}

	const customName = "Second custom"
	custom := insertSquadWorkflow(t, fx.ID, second, customName+" "+fx.ID[:8], "todo", "in_progress")
	setSquadActiveWorkflow(t, fx, second, custom, nil, false).Want(http.StatusOK)

	// The developer is a member of both squads. Its home squad decides its workflow.
	developer := agentNamedInSquad(t, fx.Build, "SDLC Developer")
	dbfx.Exec(t, `UPDATE agent SET home_squad_id = $1 WHERE id = $2`, second, developer)

	workspace := util.MustParseUUID(fx.ID)
	project := util.MustParseUUID(projectID)
	routedName := func(assigneeType, assigneeID string) string {
		t.Helper()
		def, _, err := issueworkflow.ForIssue(context.Background(), testHandler.Queries, workspace, project,
			pgtype.Text{String: assigneeType, Valid: true}, util.MustParseUUID(assigneeID))
		if err != nil || def == nil {
			t.Fatalf("resolve %s %s: def=%v err=%v", assigneeType, assigneeID, def, err)
		}
		return def.Name
	}
	if got := routedName("squad", fx.Build); got != "SDLC Auto" {
		t.Fatalf("build squad's issue uses %q, want SDLC Auto", got)
	}
	if got := routedName("squad", second); got != customName+" "+fx.ID[:8] {
		t.Fatalf("second squad's issue uses %q, want its custom workflow", got)
	}
	if got := routedName("agent", developer); got != customName+" "+fx.ID[:8] {
		t.Fatalf("developer's issue uses %q, want its home squad's custom workflow", got)
	}
	def, _, err := issueworkflow.ForIssue(context.Background(), testHandler.Queries, workspace, project, pgtype.Text{}, pgtype.UUID{})
	if err != nil || def == nil || !def.IsDefault {
		t.Fatalf("issue with no assignee: def=%v err=%v, want the project's Default", def, err)
	}
}

// TestSwitchingASquadsWorkflowRemapsOnlyItsOwnIssues: the switch censuses and
// moves only the issues routed to that squad. The other squad's issues and the
// project's own issues keep their status.
func TestSwitchingASquadsWorkflowRemapsOnlyItsOwnIssues(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Remap scope")
	second := sdlcSquad(t, fx, "Remap second")
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
	assignSquadProject(t, fx, second, projectID, nil).Want(http.StatusOK)
	target := insertSquadWorkflow(t, fx.ID, second, "Remap target "+fx.ID[:8], "todo", "in_progress")

	buildIssue := dbfx.Issue(t, "build review", testutil.Cols{
		"workspace_id": fx.ID, "project_id": projectID, "status": "in_review",
		"assignee_type": "squad", "assignee_id": fx.Build,
	})
	secondIssue := dbfx.Issue(t, "second review", testutil.Cols{
		"workspace_id": fx.ID, "project_id": projectID, "status": "in_review",
		"assignee_type": "squad", "assignee_id": second,
	})
	plainIssue := dbfx.Issue(t, "project review", testutil.Cols{
		"workspace_id": fx.ID, "project_id": projectID, "status": "in_review",
	})

	var dry struct {
		Plan struct {
			Required []struct {
				StatusKey  string `json:"status_key"`
				IssueCount int64  `json:"issue_count"`
			} `json:"required"`
		} `json:"plan"`
	}
	setSquadActiveWorkflow(t, fx, second, target, nil, true).Want(http.StatusOK).JSON(&dry)
	if len(dry.Plan.Required) != 1 || dry.Plan.Required[0].StatusKey != "in_review" || dry.Plan.Required[0].IssueCount != 1 {
		t.Fatalf("dry-run plan = %+v, want one in_review issue, the second squad's", dry.Plan)
	}

	resp := setSquadActiveWorkflow(t, fx, second, target, nil, false).Want(http.StatusConflict)
	if !strings.Contains(resp.Text(), "workflow_status_mapping_required") {
		t.Fatalf("body = %s, want workflow_status_mapping_required", resp.Text())
	}

	setSquadActiveWorkflow(t, fx, second, target, map[string]string{"in_review": "in_progress"}, false).Want(http.StatusOK)
	if got := issueStatusOf(t, secondIssue); got != "in_progress" {
		t.Fatalf("second squad's issue status = %q, want in_progress", got)
	}
	if got := issueStatusOf(t, buildIssue); got != "in_review" {
		t.Fatalf("build squad's issue status = %q, want in_review: another squad's switch must not touch it", got)
	}
	if got := issueStatusOf(t, plainIssue); got != "in_review" {
		t.Fatalf("project issue status = %q, want in_review", got)
	}
	if got := squadActiveWorkflowID(t, second); got != target {
		t.Fatalf("second squad active workflow = %s, want %s", got, target)
	}
}

// TestSquadWorkflowLibraryListsCopiesAndMarksTheActiveOne: the library lists the
// squad's workflows with the active one named, and a copy joins the library
// without becoming active.
func TestSquadWorkflowLibraryListsCopiesAndMarksTheActiveOne(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Library")
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
	activeID := squadActiveWorkflowID(t, fx.Build)

	var list struct {
		ActiveWorkflowID *string `json:"active_workflow_id"`
		Workflows        []struct {
			ID      string  `json:"id"`
			Name    string  `json:"name"`
			SquadID *string `json:"squad_id"`
		} `json:"workflows"`
	}
	req := newRequestAs(fx.AdminID, "GET", "/api/squads/"+fx.Build+"/workflows", nil)
	req = withURLParams(req, "workspaceId", fx.ID, "id", fx.Build)
	testutil.Call(t, testHandler.ListSquadWorkflows, req).Want(http.StatusOK).JSON(&list)
	if list.ActiveWorkflowID == nil || *list.ActiveWorkflowID != activeID {
		t.Fatalf("active_workflow_id = %v, want %s", list.ActiveWorkflowID, activeID)
	}
	if len(list.Workflows) != 1 || list.Workflows[0].Name != "SDLC Auto" ||
		list.Workflows[0].SquadID == nil || *list.Workflows[0].SquadID != fx.Build {
		t.Fatalf("library = %+v, want the squad's SDLC Auto", list.Workflows)
	}

	var created struct {
		ID      string  `json:"id"`
		SquadID *string `json:"squad_id"`
	}
	body := map[string]any{"name": "Build copy " + fx.ID[:8], "source_workflow_id": activeID}
	req = newRequestAs(fx.AdminID, "POST", "/api/squads/"+fx.Build+"/workflows", body)
	req = withURLParams(req, "workspaceId", fx.ID, "id", fx.Build)
	testutil.Call(t, testHandler.CreateSquadWorkflow, req).Want(http.StatusCreated).JSON(&created)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM issue_workflow WHERE id = $1`, created.ID)
	})
	if created.SquadID == nil || *created.SquadID != fx.Build {
		t.Fatalf("copy squad_id = %v, want %s", created.SquadID, fx.Build)
	}
	if got := squadActiveWorkflowID(t, fx.Build); got != activeID {
		t.Fatalf("active workflow = %s after copying, want it unchanged at %s", got, activeID)
	}
}

// TestSquadWorkflowRefusesAnotherSquadsWorkflow: a workflow in one squad's
// library cannot become another squad's active workflow.
func TestSquadWorkflowRefusesAnotherSquadsWorkflow(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Ownership")
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
	other := sdlcSquad(t, fx, "Ownership other")

	resp := setSquadActiveWorkflow(t, fx, other, squadActiveWorkflowID(t, fx.Build), nil, false).Want(http.StatusConflict)
	if !strings.Contains(resp.Text(), "workflow_belongs_to_other_squad") {
		t.Fatalf("body = %s, want workflow_belongs_to_other_squad", resp.Text())
	}
}

// TestDeleteSquadActiveWorkflowIsRefused: a workflow a squad uses cannot be
// deleted until another one is active.
func TestDeleteSquadActiveWorkflowIsRefused(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Delete guard")
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
	activeID := squadActiveWorkflowID(t, fx.Build)

	req := workspaceRequest(fx.AdminID, fx.ID, "DELETE", "/api/issue-workflows/"+activeID, nil)
	req = withURLParam(req, "id", activeID)
	resp := testutil.Call(t, testHandler.DeleteIssueWorkflow, req).Want(http.StatusConflict)
	if !strings.Contains(resp.Text(), "workflow_is_squad_active") {
		t.Fatalf("body = %s, want workflow_is_squad_active", resp.Text())
	}
}

// TestProjectDetailShowsEachSquadsActiveWorkflow: the project detail lists every
// assigned squad with the workflow it uses now, read live.
func TestProjectDetailShowsEachSquadsActiveWorkflow(t *testing.T) {
	fx := newIniWorkspace(t)
	projectID := dbfxProject(t, fx, "Detail")
	second := sdlcSquad(t, fx, "Detail second")
	assignSquadProject(t, fx, fx.Build, projectID, nil).Want(http.StatusOK)
	assignSquadProject(t, fx, second, projectID, nil).Want(http.StatusOK)
	custom := insertSquadWorkflow(t, fx.ID, second, "Detail custom "+fx.ID[:8], "todo", "in_progress")
	setSquadActiveWorkflow(t, fx, second, custom, nil, false).Want(http.StatusOK)

	var detail struct {
		Squads []struct {
			SquadID            string  `json:"squad_id"`
			ActiveWorkflowName *string `json:"active_workflow_name"`
		} `json:"squads"`
	}
	req := workspaceRequest(fx.AdminID, fx.ID, "GET", "/api/projects/"+projectID, nil)
	req = withURLParam(req, "id", projectID)
	testutil.Call(t, testHandler.GetProject, req).Want(http.StatusOK).JSON(&detail)

	names := map[string]string{}
	for _, s := range detail.Squads {
		if s.ActiveWorkflowName != nil {
			names[s.SquadID] = *s.ActiveWorkflowName
		}
	}
	if len(detail.Squads) != 2 || names[fx.Build] != "SDLC Auto" || names[second] != "Detail custom "+fx.ID[:8] {
		t.Fatalf("project squads = %+v, want the build squad on SDLC Auto and the second on its custom workflow", detail.Squads)
	}
}

// TestSquadWorkflowBackfillLinksExistingWorkflows runs the two backfill
// migrations on workflows stored before squads owned them. Both run in one
// transaction that is rolled back, so the shared database keeps its rows.
func TestSquadWorkflowBackfillLinksExistingWorkflows(t *testing.T) {
	fx := newIniWorkspace(t)
	ctx := context.Background()
	developer := agentNamedInSquad(t, fx.Build, "SDLC Developer")
	legacySteps, err := issueworkflow.EncodeSteps(issueworkflow.Arrange([]issueworkflow.Step{
		{StatusKey: "todo", Handler: issueworkflow.Handler{Type: issueworkflow.HandlerAgent, ID: developer}},
		{StatusKey: "done", Handler: issueworkflow.Handler{Type: issueworkflow.HandlerNone}},
	}))
	if err != nil {
		t.Fatalf("encode steps: %v", err)
	}
	legacy, err := testHandler.Queries.CreateIssueWorkflow(ctx, db.CreateIssueWorkflowParams{
		WorkspaceID: util.MustParseUUID(fx.ID), Name: "SDLC Auto", InitialStatusKey: "todo", Steps: legacySteps,
	})
	if err != nil {
		t.Fatalf("create legacy SDLC workflow: %v", err)
	}
	t.Cleanup(func() { testPool.Exec(ctx, `DELETE FROM issue_workflow WHERE id = $1`, legacy.ID) })
	buildProject := dbfxProject(t, fx, "Legacy build")
	dbfx.Exec(t, `UPDATE project SET workflow_id = $1, squad_ids = ARRAY[$2::uuid] WHERE id = $3`,
		util.UUIDToString(legacy.ID), fx.Build, buildProject)

	planning, lead := planningSquad(t, fx, "Legacy planning")
	createPlanningWorkflow(t, fx, lead)
	var planningWorkflow string
	dbfx.QueryRow(t, `SELECT id::text FROM issue_workflow WHERE workspace_id = $1 AND name = 'BMAD Planning Auto'`, fx.ID).Scan(&planningWorkflow)
	planningProject := dbfxProject(t, fx, "Legacy planning project")
	dbfx.Exec(t, `UPDATE project SET workflow_id = $1, squad_ids = ARRAY[$2::uuid] WHERE id = $3`,
		planningWorkflow, planning, planningProject)

	tx, err := testPool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer tx.Rollback(ctx)
	for _, file := range []string{"975_issue_workflow_squad_backfill.up.sql", "976_squad_active_workflow_backfill.up.sql"} {
		sql, err := os.ReadFile(filepath.Join("..", "..", "migrations", file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := tx.Exec(ctx, string(sql)); err != nil {
			t.Fatalf("run %s: %v", file, err)
		}
	}

	var legacySquad, planningSquadID, buildActive, planningActive *string
	if err := tx.QueryRow(ctx, `SELECT squad_id::text FROM issue_workflow WHERE id = $1`, util.UUIDToString(legacy.ID)).Scan(&legacySquad); err != nil {
		t.Fatalf("read legacy squad: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT squad_id::text FROM issue_workflow WHERE id = $1`, planningWorkflow).Scan(&planningSquadID); err != nil {
		t.Fatalf("read planning squad: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT active_workflow_id::text FROM squad WHERE id = $1`, fx.Build).Scan(&buildActive); err != nil {
		t.Fatalf("read build active: %v", err)
	}
	if err := tx.QueryRow(ctx, `SELECT active_workflow_id::text FROM squad WHERE id = $1`, planning).Scan(&planningActive); err != nil {
		t.Fatalf("read planning active: %v", err)
	}
	if legacySquad == nil || *legacySquad != fx.Build {
		t.Fatalf("SDLC workflow squad_id = %v, want the build squad %s", legacySquad, fx.Build)
	}
	if planningSquadID == nil || *planningSquadID != planning {
		t.Fatalf("planning workflow squad_id = %v, want %s", planningSquadID, planning)
	}
	if buildActive == nil || *buildActive != util.UUIDToString(legacy.ID) {
		t.Fatalf("build squad active = %v, want the legacy SDLC workflow", buildActive)
	}
	if planningActive == nil || *planningActive != planningWorkflow {
		t.Fatalf("planning squad active = %v, want %s", planningActive, planningWorkflow)
	}
}
