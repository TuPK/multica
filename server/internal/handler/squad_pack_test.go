package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// Installing a pack of skills and agents (plan P24; spec 16.1 to 16.3; criteria N1 to N6). The bundles are the
// fixtures of server/internal/pack/testdata, sent as zip files.

// packWS is a workspace of its own, so a test names its skills and agents as the bundle does and nothing
// of another test is in the way.
type packWS struct {
	ID        string
	AdminID   string
	MemberID  string
	RuntimeID string
}

func newPackWorkspace(t *testing.T) packWS {
	t.Helper()
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	w := packWS{}
	w.ID = dbfx.Workspace(t, "Pack "+suffix, "pack-"+suffix)
	// Registered after the workspace, so it runs before the workspace row goes.
	t.Cleanup(func() { cleanPackRows(w.ID) })
	w.AdminID = dbfx.User(t, "Pack Admin "+suffix, "pack-admin-"+suffix+"@example.com")
	dbfx.Member(t, w.ID, w.AdminID, "admin")
	w.MemberID = dbfx.User(t, "Pack Member "+suffix, "pack-member-"+suffix+"@example.com")
	dbfx.Member(t, w.ID, w.MemberID, "member")
	w.RuntimeID = dbfx.Runtime(t, "Pack runtime "+suffix, testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID, "visibility": "public"})
	// The board statuses a flow names exist from the start; a preview must not be the one that seeds them.
	if err := issuestatus.Ensure(context.Background(), testHandler.Queries, parseUUID(w.ID)); err != nil {
		t.Fatalf("seed statuses: %v", err)
	}
	return w
}

// cleanPackRows removes what an install wrote in a test workspace.
func cleanPackRows(ws string) {
	ctx := context.Background()
	for _, stmt := range []string{
		`DELETE FROM squad_pack_install WHERE workspace_id = $1`,
		`DELETE FROM issue_workflow_draft WHERE workspace_id = $1`,
		`DELETE FROM issue_workflow WHERE workspace_id = $1`,
		`DELETE FROM squad_member WHERE squad_id IN (SELECT id FROM squad WHERE workspace_id = $1)`,
		`DELETE FROM squad WHERE workspace_id = $1`,
		`DELETE FROM company_floor WHERE workspace_id = $1`,
		`DELETE FROM squad_charter WHERE workspace_id = $1`,
		`DELETE FROM agent_skill WHERE agent_id IN (SELECT id FROM agent WHERE workspace_id = $1)`,
		`DELETE FROM agent WHERE workspace_id = $1`,
		`DELETE FROM skill_version WHERE skill_id IN (SELECT id FROM skill WHERE workspace_id = $1)`,
		`DELETE FROM skill WHERE workspace_id = $1`,
		`DELETE FROM issue_status WHERE workspace_id = $1`,
		`DELETE FROM agent_runtime WHERE workspace_id = $1`,
	} {
		testPool.Exec(ctx, stmt, ws)
	}
}

// packFlags turns the flags of packs and flow authoring on or off for one test.
func packFlags(t *testing.T, packs, authoring, graphV2 bool) {
	t.Helper()
	provider := featureflag.NewStaticProvider()
	provider.Set(featureflags.FlowPacks, featureflag.Rule{Default: packs})
	provider.Set(featureflags.FlowAuthoring, featureflag.Rule{Default: authoring})
	provider.Set(featureflags.FlowGraphV2, featureflag.Rule{Default: graphV2})
	flags := featureflag.NewService(provider)
	prev := testHandler.FeatureFlags
	testHandler.FeatureFlags = flags
	t.Cleanup(func() { testHandler.FeatureFlags = prev })
}

// withPacks is the setting of most tests: packs on, authoring on.
func withPacks(t *testing.T) { t.Helper(); packFlags(t, true, true, true) }

// packFixture reads a folder of server/internal/pack/testdata as the files of a zip.
func packFixture(t *testing.T, dir string) map[string]string {
	t.Helper()
	root := filepath.Join("..", "pack", "testdata", filepath.FromSlash(dir))
	files := map[string]string{}
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		content, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(root, p)
		files[filepath.ToSlash(rel)] = string(content)
		return nil
	})
	if err != nil {
		t.Fatalf("read fixture %s: %v", dir, err)
	}
	return files
}

func packZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	return hubImportZip(t, files)
}

func (w packWS) request(userID, method, path string, body any) *http.Request {
	return workspaceRequest(userID, w.ID, method, path, body)
}

// preview uploads a zip as a person of the workspace.
func (w packWS) preview(t *testing.T, userID string, zipBytes []byte) *testutil.Response {
	t.Helper()
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, err := form.CreateFormFile("file", "pack.zip")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write(zipBytes)
	_ = form.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/squads/packs/preview", &body)
	req.Header.Set("Content-Type", form.FormDataContentType())
	req.Header.Set("X-Workspace-ID", w.ID)
	req.Header.Set("X-User-ID", userID)
	return testutil.Call(t, testHandler.PreviewSquadPack, req)
}

func (w packWS) previewOK(t *testing.T, files map[string]string) PackPreviewResponse {
	t.Helper()
	return w.previewOKAs(t, w.AdminID, files)
}

// previewOKAs previews as a person; only that person may commit what they previewed.
func (w packWS) previewOKAs(t *testing.T, userID string, files map[string]string) PackPreviewResponse {
	t.Helper()
	var out PackPreviewResponse
	w.preview(t, userID, packZip(t, files)).Want(http.StatusOK).JSON(&out)
	return out
}

func (w packWS) commit(t *testing.T, userID string, body map[string]any) *testutil.Response {
	t.Helper()
	return testutil.Call(t, testHandler.CommitSquadPack, w.request(userID, "POST", "/api/squads/packs/commit", body))
}

// createChoices are the agents of a bundle, all made on the workspace's runtime.
func (w packWS) createChoices(keys ...string) []map[string]any {
	var out []map[string]any
	for _, k := range keys {
		out = append(out, map[string]any{"key": k, "action": "create", "runtime_id": w.RuntimeID})
	}
	return out
}

// installNative previews and commits the native bundle: two agents, the first its leader.
func (w packWS) installNative(t *testing.T, extra map[string]any) PackCommitResponse {
	t.Helper()
	files := packFixture(t, "native_bundle")
	preview := w.previewOK(t, files)
	body := map[string]any{
		"digest": preview.Digest,
		"squad":  map[string]any{"name": "Newsletter desk"},
		"agents": w.createChoices("planner", "writer"),
		"leader": map[string]any{"agent": "planner"},
	}
	for k, v := range extra {
		body[k] = v
	}
	var out PackCommitResponse
	w.commit(t, w.AdminID, body).Want(http.StatusCreated).JSON(&out)
	return out
}

// packCounts counts what an install writes in the workspace.
func packCounts(t *testing.T, ws string) map[string]int {
	t.Helper()
	counts := map[string]int{}
	for name, query := range map[string]string{
		"skill":       `SELECT count(*) FROM skill WHERE workspace_id = $1`,
		"agent":       `SELECT count(*) FROM agent WHERE workspace_id = $1`,
		"squad":       `SELECT count(*) FROM squad WHERE workspace_id = $1`,
		"member":      `SELECT count(*) FROM squad_member WHERE squad_id IN (SELECT id FROM squad WHERE workspace_id = $1)`,
		"install":     `SELECT count(*) FROM squad_pack_install WHERE workspace_id = $1`,
		"draft":       `SELECT count(*) FROM issue_workflow_draft WHERE workspace_id = $1`,
		"workflow":    `SELECT count(*) FROM issue_workflow WHERE workspace_id = $1`,
		"agent_skill": `SELECT count(*) FROM agent_skill WHERE agent_id IN (SELECT id FROM agent WHERE workspace_id = $1)`,
		"hub_item":    `SELECT count(*) FROM hub_item WHERE workspace_id = $1`,
		"status":      `SELECT count(*) FROM issue_status WHERE workspace_id = $1`,
		"activity":    `SELECT count(*) FROM activity_log WHERE workspace_id = $1`,
	} {
		counts[name] = dbfx.Count(t, query, ws)
	}
	return counts
}

func sameCounts(t *testing.T, what string, before, after map[string]int) {
	t.Helper()
	for k, v := range before {
		if after[k] != v {
			t.Errorf("%s: %s rows went from %d to %d", what, k, v, after[k])
		}
	}
}

func wantPackError(t *testing.T, resp *testutil.Response, status int, code string) map[string]any {
	t.Helper()
	body := resp.Want(status).Map()
	if body["code"] != code {
		t.Fatalf("code = %v, want %s (body %v)", body["code"], code, body)
	}
	return body
}

func TestPackPreview_FourFormats(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)

	// An agent and a skill the workspace already has, so that the preview can say so.
	existingAgent := dbfx.Agent(t, "Writer", w.RuntimeID, testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID})
	outline := packFixture(t, "native_bundle")["skills/outline/SKILL.md"]
	if _, err := testHandler.createSkillWithFiles(context.Background(), skillCreateInput{
		WorkspaceID: parseUUID(w.ID), CreatorID: parseUUID(w.AdminID), Name: "outline", Description: "d", Content: outline,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := testHandler.createSkillWithFiles(context.Background(), skillCreateInput{
		WorkspaceID: parseUUID(w.ID), CreatorID: parseUUID(w.AdminID), Name: "tone-check", Description: "d", Content: "another text",
	}); err != nil {
		t.Fatal(err)
	}

	t.Run("a bundle of the squad's own format (native)", func(t *testing.T) {
		p := w.previewOK(t, packFixture(t, "native_bundle"))
		if p.Format != "native_bundle" || p.ID != "newsletter-desk" || p.Name != "Newsletter Desk" || p.Version != "1.0.0" {
			t.Fatalf("preview = %+v", p)
		}
		if !strings.HasPrefix(p.Digest, "sha256:") || len(p.Digest) != len("sha256:")+64 {
			t.Errorf("digest %q", p.Digest)
		}
		if p.Leader == nil || *p.Leader != "planner" || len(p.Agents) != 2 || len(p.Skills) != 2 {
			t.Errorf("leader %v agents %d skills %d", p.Leader, len(p.Agents), len(p.Skills))
		}
		if !p.Flow.Present || p.Flow.Validation == nil || !p.Flow.Validation.OK {
			t.Errorf("flow %+v", p.Flow)
		}
		for _, a := range p.Agents {
			if a.Key == "writer" && (a.ExistingAgentID == nil || *a.ExistingAgentID != existingAgent) {
				t.Errorf("the writer is not matched to the agent the workspace has: %+v", a)
			}
			if a.Key == "planner" && a.ExistingAgentID != nil {
				t.Errorf("the planner is matched to %s", *a.ExistingAgentID)
			}
			if a.InstructionsBytes == 0 {
				t.Errorf("agent %s has no instructions bytes", a.Key)
			}
		}
		for _, s := range p.Skills {
			switch s.Name {
			case "outline":
				if s.ExistingSkillID == nil || !s.SameContent {
					t.Errorf("outline: %+v, want the existing skill with the same content", s)
				}
			case "tone-check":
				if s.ExistingSkillID == nil || s.SameContent {
					t.Errorf("tone-check: %+v, want the existing skill with other content", s)
				}
			}
		}
		if len(p.References) != 6 {
			t.Errorf("references %+v", p.References)
		}
	})

	t.Run("an agent pack of Multica", func(t *testing.T) {
		p := w.previewOK(t, packFixture(t, "multica_pack"))
		if p.Format != "multica_pack" || p.ID != "field-researcher" || len(p.Agents) != 1 || len(p.Skills) != 2 || p.Leader != nil {
			t.Fatalf("preview = %+v", p)
		}
		if p.Agents[0].Key != "field_researcher" || !p.Agents[0].SkillsDeclared || len(p.Agents[0].Skills) != 2 {
			t.Errorf("agent %+v", p.Agents[0])
		}
		if len(p.Ignored) != 1 || p.Ignored[0].Key != "runtime_provider" {
			t.Errorf("ignored %+v", p.Ignored)
		}
		if p.Flow.Present {
			t.Errorf("flow %+v", p.Flow)
		}
	})

	t.Run("a folder of agents and skills (Claude Code)", func(t *testing.T) {
		p := w.previewOK(t, packFixture(t, "claude_agents"))
		if p.Format != "claude_agents" || len(p.Agents) != 7 || len(p.Skills) != 18 || p.Leader != nil {
			t.Fatalf("preview = %d agents %d skills, format %s", len(p.Agents), len(p.Skills), p.Format)
		}
		// The tools and the model of the agents are shown as not applied.
		if len(p.Ignored) != 14 {
			t.Errorf("ignored %d, want tools and model of 7 agents", len(p.Ignored))
		}
		// Every instructions file and SKILL.md is listed, though there are more than 20 of them.
		if len(p.References) != 25 || p.ReferencesOmitted != 0 {
			t.Errorf("references %d, omitted %d, want the 7 agents and 18 skills, none omitted", len(p.References), p.ReferencesOmitted)
		}
		for _, i := range p.Issues {
			if i.Code == "pack_references_capped" {
				t.Errorf("issue %+v: the count of omitted docs says it now", i)
			}
		}
		for _, a := range p.Agents {
			if a.SkillsDeclared {
				t.Errorf("agent %s declares skills it does not", a.Key)
			}
		}
	})

	t.Run("the preview says how many doc files it left out, as references_omitted", func(t *testing.T) {
		files := packFixture(t, "claude_agents")
		for i := 0; i < 23; i++ {
			files[fmt.Sprintf("docs/d%02d.md", i)] = "# Doc\n"
		}
		var raw map[string]any
		w.preview(t, w.AdminID, packZip(t, files)).Want(http.StatusOK).JSON(&raw)
		if got, ok := raw["references_omitted"].(float64); !ok || got != 3 {
			t.Errorf("references_omitted = %v, want 3", raw["references_omitted"])
		}
		if refs, _ := raw["references"].([]any); len(refs) != 45 {
			t.Errorf("%d references, want the 7 agents, 18 skills and 20 docs", len(refs))
		}

		// With nothing omitted the field is still there, as 0.
		var plain map[string]any
		w.preview(t, w.AdminID, packZip(t, packFixture(t, "claude_agents"))).Want(http.StatusOK).JSON(&plain)
		if got, ok := plain["references_omitted"].(float64); !ok || got != 0 {
			t.Errorf("references_omitted = %v, want 0", plain["references_omitted"])
		}
	})

	t.Run("folders of skills and no agent", func(t *testing.T) {
		p := w.previewOK(t, packFixture(t, "skills_only"))
		if p.Format != "skills_only" || len(p.Agents) != 0 || len(p.Skills) != 3 || p.Leader != nil {
			t.Fatalf("preview = %+v", p)
		}
		for _, s := range p.Skills {
			if s.Name == "outline-first" && s.Files != 2 {
				t.Errorf("outline-first has %d files", s.Files)
			}
		}
	})
}

func TestPackPreview_WritesNothing(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	before := packCounts(t, w.ID)
	for _, dir := range []string{"native_bundle", "multica_pack", "claude_agents", "skills_only"} {
		w.previewOK(t, packFixture(t, dir))
		sameCounts(t, "after the preview of "+dir, before, packCounts(t, w.ID))
	}
	// A bundle that cannot be installed is shown too, and writes as little.
	var broken PackPreviewResponse
	w.preview(t, w.AdminID, packZip(t, packFixture(t, "broken/missing_skill"))).Want(http.StatusOK).JSON(&broken)
	if len(broken.Issues) == 0 {
		t.Error("a pack that names a skill it does not have shows no issue")
	}
	sameCounts(t, "after the preview of a broken pack", before, packCounts(t, w.ID))
}

func TestPackCommit_CreatesSquadAgentsSkillsAtomically(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)

	t.Run("the native bundle makes a squad of its agents with the skills they ask for", func(t *testing.T) {
		w := newPackWorkspace(t)
		before := packCounts(t, w.ID)
		res := w.installNative(t, nil)
		if res.AlreadyInstalled || res.SquadID == nil || len(res.Agents) != 2 || len(res.Skills) != 2 || res.InstallID == "" {
			t.Fatalf("result = %+v", res)
		}
		after := packCounts(t, w.ID)
		if after["skill"]-before["skill"] != 2 || after["agent"]-before["agent"] != 2 || after["squad"] != 1 || after["member"] != 2 || after["install"] != 1 {
			t.Fatalf("rows after = %v", after)
		}
		// The squad is led by the agent the bundle names, and the other has the label of its role.
		var leader, leaderName, name string
		dbfx.QueryRow(t, `SELECT s.leader_id::text, a.name, s.name FROM squad s JOIN agent a ON a.id = s.leader_id WHERE s.id = $1`, *res.SquadID).Scan(&leader, &leaderName, &name)
		if leaderName != "Planner" || name != "Newsletter desk" {
			t.Errorf("leader %q squad %q", leaderName, name)
		}
		var roles []string
		rows, err := testPool.Query(context.Background(), `SELECT sm.role FROM squad_member sm WHERE sm.squad_id = $1`, *res.SquadID)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var role string
			_ = rows.Scan(&role)
			roles = append(roles, role)
		}
		rows.Close()
		sort.Strings(roles)
		if got := strings.Join(roles, ","); got != "Writes the issue,leader" {
			t.Errorf("member roles = %q", got)
		}
		// Each agent has its instructions (the body of the file) and the skills it names.
		var instructions string
		var skills string
		dbfx.QueryRow(t, `SELECT instructions FROM agent WHERE workspace_id = $1 AND name = 'Writer'`, w.ID).Scan(&instructions)
		dbfx.QueryRow(t, `SELECT string_agg(s.name, ',' ORDER BY s.name) FROM agent_skill ags JOIN skill s ON s.id = ags.skill_id JOIN agent a ON a.id = ags.agent_id WHERE a.workspace_id = $1 AND a.name = 'Writer'`, w.ID).Scan(&skills)
		if !strings.HasPrefix(instructions, "# Writer") || skills != "outline,tone-check" {
			t.Errorf("writer instructions %.30q skills %q", instructions, skills)
		}
		// The skills come from the bundle, with their files and where they came from.
		var content, origin string
		dbfx.QueryRow(t, `SELECT content, config->'origin'->>'type' FROM skill WHERE workspace_id = $1 AND name = 'outline'`, w.ID).Scan(&content, &origin)
		if !strings.Contains(content, "Outline an issue of the newsletter") || origin != "pack" {
			t.Errorf("skill content %.40q origin %q", content, origin)
		}
		// What the install recorded.
		var packID, version, format, digest, roster string
		dbfx.QueryRow(t, `SELECT pack_id, pack_version, format, digest, roster::text FROM squad_pack_install WHERE id = $1`, res.InstallID).Scan(&packID, &version, &format, &digest, &roster)
		if packID != "newsletter-desk" || version != "1.0.0" || format != "native_bundle" || !strings.HasPrefix(digest, "sha256:") || !strings.Contains(roster, "planner") || !strings.Contains(roster, "outline") {
			t.Errorf("install = %q %q %q %q %s", packID, version, format, digest, roster)
		}
	})

	t.Run("an error at the second agent leaves nothing of the install", func(t *testing.T) {
		w := newPackWorkspace(t)
		before := packCounts(t, w.ID)
		packCommitFailpoint = func(stage string, n int) error {
			if stage == "agent" && n == 2 {
				return errors.New("forced failure at the second agent")
			}
			return nil
		}
		t.Cleanup(func() { packCommitFailpoint = nil })

		preview := w.previewOK(t, packFixture(t, "native_bundle"))
		w.commit(t, w.AdminID, map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Newsletter desk"},
			"agents": w.createChoices("planner", "writer"), "leader": map[string]any{"agent": "planner"},
		}).Want(http.StatusInternalServerError)
		sameCounts(t, "after the failed install", before, packCounts(t, w.ID))
		if got := packCounts(t, w.ID); got["agent"] != 0 || got["skill"] != 0 || got["squad"] != 0 || got["install"] != 0 {
			t.Errorf("rows left: %v", got)
		}

		// With the failure gone, the same preview installs.
		packCommitFailpoint = nil
		w.commit(t, w.AdminID, map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Newsletter desk"},
			"agents": w.createChoices("planner", "writer"), "leader": map[string]any{"agent": "planner"},
		}).Want(http.StatusCreated)
	})
}

func TestPackCommit_SettingsOfTheBundleAreNotApplied(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	files := packFixture(t, "claude_agents")
	files["agents/gom-du-lieu-shop.md"] = strings.Replace(files["agents/gom-du-lieu-shop.md"], "model: inherit\n", "model: opus\nenv:\n  API_KEY: not-for-you\nmcpServers:\n  x: {command: run}\n", 1)
	preview := w.previewOK(t, files)
	w.commit(t, w.AdminID, map[string]any{
		"digest": preview.Digest,
		"agents": w.createChoices("gom_du_lieu_shop"),
	}).Want(http.StatusCreated)

	var model *string
	var env, mcp, tools string
	dbfx.QueryRow(t, `SELECT model, custom_env::text, COALESCE(mcp_config::text, ''), custom_args::text FROM agent WHERE workspace_id = $1 AND name = 'gom-du-lieu-shop'`, w.ID).Scan(&model, &env, &mcp, &tools)
	if model != nil && *model != "" || env != "{}" || mcp != "" || tools != "[]" {
		t.Errorf("model %v env %s mcp %q args %s: a setting of the bundle was applied", model, env, mcp, tools)
	}
}

func TestPackCommit_BrokenPacksRejected(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	before := packCounts(t, w.ID)

	// Bundles the preview shows and the commit refuses, with the field each issue names (N3).
	for _, tc := range []struct {
		name  string
		files map[string]string
		code  string
		path  string
	}{
		{"a skill without a description", packFixture(t, "broken/skill_no_description"), "pack_field_missing", "skills[only].description"},
		{"a skill without a name", packFixture(t, "broken/skill_no_name"), "pack_field_missing", "skills[.].name"},
		{"an agent without a description", packFixture(t, "broken/agent_no_description"), "pack_field_missing", "agents[bad_agent].description"},
		{"an agent that names a skill the bundle lacks", packFixture(t, "broken/agent_skill_missing"), "pack_skill_missing", "agents[reader].skills[ghost]"},
		{"a pack that names a skill it lacks", packFixture(t, "broken/missing_skill"), "pack_skill_missing", "agents[reader].skills[ghost]"},
		{"two agents with one key", packFixture(t, "broken/duplicate_agent_key"), "pack_agent_key_duplicate", "agents[a_b]"},
		{"a leader that is no agent", packFixture(t, "broken/leader_missing"), "pack_leader_invalid", "leader"},
		{"instructions that leave the bundle", packFixture(t, "broken/path_escape"), "pack_path_unsafe", "agents[beta].instructions"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var preview PackPreviewResponse
			w.preview(t, w.AdminID, packZip(t, tc.files)).Want(http.StatusOK).JSON(&preview)
			found := false
			for _, i := range preview.Issues {
				found = found || (i.Code == tc.code && i.Path == tc.path)
			}
			if !found {
				t.Fatalf("the preview lacks %s at %s: %+v", tc.code, tc.path, preview.Issues)
			}
			body := wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}), http.StatusBadRequest, "pack_invalid")
			issues, _ := body["issues"].([]any)
			found = false
			for _, raw := range issues {
				i, _ := raw.(map[string]any)
				found = found || (i["code"] == tc.code && i["path"] == tc.path)
			}
			if !found {
				t.Errorf("the refusal lacks %s at %s: %v", tc.code, tc.path, body)
			}
		})
	}

	// Bundles that cannot even be read are refused by the preview.
	files := map[string]string{}
	for i := 0; i < 600; i++ {
		files[fmt.Sprintf("only/data/f%04d.txt", i)] = "x"
	}
	files["only/SKILL.md"] = "---\nname: only\ndescription: d\n---\n"
	for name, tc := range map[string]struct {
		files map[string]string
		code  string
	}{
		"more files than the limit": {files, ""},
		"a path that leaves the bundle": {map[string]string{
			"../outside.md": "x", "only/SKILL.md": "---\nname: only\ndescription: d\n---\n",
		}, ""},
		"two signs of a format at the root": {packFixture(t, "broken/ambiguous_root"), "pack_invalid"},
		"nothing a pack is made of":         {packFixture(t, "broken/nothing"), "pack_invalid"},
		"a manifest that is cut":            {packFixture(t, "broken/bad_manifest"), "pack_invalid"},
	} {
		resp := w.preview(t, w.AdminID, packZip(t, tc.files)).Want(http.StatusBadRequest)
		if tc.code != "" && resp.Map()["code"] != tc.code {
			t.Errorf("%s: %v, want %s", name, resp.Map(), tc.code)
		}
	}
	sameCounts(t, "after the refused bundles", before, packCounts(t, w.ID))
}

func TestPackCommit_ScanBlockRejected_WarnNeedsAck(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	before := packCounts(t, w.ID)

	blocked := packFixture(t, "skills_only")
	blocked["outline-first/bin/tool"] = "\x7fELF\x02\x01\x01\x00 an executable"
	var preview PackPreviewResponse
	w.preview(t, w.AdminID, packZip(t, blocked)).Want(http.StatusOK).JSON(&preview)
	var sawBlock bool
	for _, f := range preview.Scan {
		sawBlock = sawBlock || (f.Severity == "block" && f.Path == "outline-first/bin/tool")
	}
	if !sawBlock {
		t.Fatalf("the preview does not show the blocked file: %+v", preview.Scan)
	}
	// A block is not given away by an acknowledgement.
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest, "ack_scan_warnings": true}), http.StatusUnprocessableEntity, "pack_scan_blocked")

	warned := packFixture(t, "skills_only")
	warned["outline-first/helper.sh"] = "#!/bin/sh\necho hello\n"
	var warnedPreview PackPreviewResponse
	w.preview(t, w.AdminID, packZip(t, warned)).Want(http.StatusOK).JSON(&warnedPreview)
	var sawWarn bool
	for _, f := range warnedPreview.Scan {
		sawWarn = sawWarn || (f.Severity == "warn" && f.Path == "outline-first/helper.sh")
	}
	if !sawWarn {
		t.Fatalf("the preview does not show the warning: %+v", warnedPreview.Scan)
	}
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": warnedPreview.Digest}), http.StatusUnprocessableEntity, "pack_scan_ack_required")
	sameCounts(t, "after the refusals", before, packCounts(t, w.ID))
	w.commit(t, w.AdminID, map[string]any{"digest": warnedPreview.Digest, "ack_scan_warnings": true}).Want(http.StatusCreated)
	if got := packCounts(t, w.ID)["skill"]; got != 3 {
		t.Errorf("%d skills after the acknowledged install, want 3", got)
	}
}

func TestPackCommit_SameDigestIsNoop_OtherDigestConflicts(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	first := w.installNative(t, nil)
	afterFirst := packCounts(t, w.ID)

	// The same bundle again changes nothing and answers with the result it had.
	files := packFixture(t, "native_bundle")
	preview := w.previewOK(t, files)
	var again PackCommitResponse
	w.commit(t, w.AdminID, map[string]any{
		"digest": preview.Digest, "squad": map[string]any{"name": "Newsletter desk"},
		"agents": w.createChoices("planner", "writer"), "leader": map[string]any{"agent": "planner"},
	}).Want(http.StatusOK).JSON(&again)
	if !again.AlreadyInstalled || again.InstallID != first.InstallID || again.SquadID == nil || *again.SquadID != *first.SquadID || len(again.Agents) != 2 || len(again.Skills) != 2 {
		t.Fatalf("again = %+v, first = %+v", again, first)
	}
	sameCounts(t, "after the same bundle again", afterFirst, packCounts(t, w.ID))

	// Another version of the pack (the same id) is a conflict, and it overwrites nothing.
	files["agents/writer/instructions.md"] = "# Writer\n\nA new version of the instructions.\n"
	files["squad-bundle.json"] = strings.Replace(files["squad-bundle.json"], `"version": "1.0.0"`, `"version": "1.1.0"`, 1)
	newer := w.previewOK(t, files)
	if newer.Digest == preview.Digest {
		t.Fatal("the changed bundle has the same digest")
	}
	body := wantPackError(t, w.commit(t, w.AdminID, map[string]any{
		"digest": newer.Digest, "squad": map[string]any{"name": "Newsletter desk 2"},
		"agents": w.createChoices("planner", "writer"), "leader": map[string]any{"agent": "planner"},
	}), http.StatusConflict, "pack_conflict")
	previous, _ := body["previous"].(map[string]any)
	if previous["install_id"] != first.InstallID || previous["squad_id"] != *first.SquadID || previous["version"] != "1.0.0" {
		t.Errorf("the conflict names %v, want the first install", previous)
	}
	sameCounts(t, "after the conflict", afterFirst, packCounts(t, w.ID))

	// Choosing a new install makes a new squad; the agents that exist are used, not rewritten.
	existing := map[string]string{}
	for _, a := range first.Agents {
		existing[a.Key] = a.AgentID
	}
	var second PackCommitResponse
	w.commit(t, w.AdminID, map[string]any{
		"digest": newer.Digest, "new_install": true, "squad": map[string]any{"name": "Newsletter desk 2"},
		"agents": []map[string]any{
			{"key": "planner", "action": "use_existing", "existing_agent_id": existing["planner"]},
			{"key": "writer", "action": "use_existing", "existing_agent_id": existing["writer"]},
		},
		"leader": map[string]any{"agent": "planner"},
		"skills": map[string]any{"outline": "keep_existing", "tone-check": "keep_existing"},
	}).Want(http.StatusCreated).JSON(&second)
	if second.SquadID == nil || *second.SquadID == *first.SquadID || second.InstallID == first.InstallID {
		t.Fatalf("second = %+v", second)
	}
	var writerInstructions string
	dbfx.QueryRow(t, `SELECT instructions FROM agent WHERE id = $1`, existing["writer"]).Scan(&writerInstructions)
	if strings.Contains(writerInstructions, "A new version") {
		t.Error("an agent that exists was overwritten by a new version of the pack")
	}
	got := packCounts(t, w.ID)
	if got["agent"] != 2 || got["skill"] != 2 || got["squad"] != 2 || got["install"] != 2 {
		t.Errorf("rows after the second install = %v", got)
	}
}

func TestPackCommit_NamesThatAreTakenNeedAChoice(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	dbfx.Agent(t, "Writer", w.RuntimeID, testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID})
	if _, err := testHandler.createSkillWithFiles(context.Background(), skillCreateInput{
		WorkspaceID: parseUUID(w.ID), CreatorID: parseUUID(w.AdminID), Name: "tone-check", Description: "mine", Content: "my own text",
	}); err != nil {
		t.Fatal(err)
	}
	before := packCounts(t, w.ID)
	preview := w.previewOK(t, packFixture(t, "native_bundle"))
	body := map[string]any{
		"digest": preview.Digest, "squad": map[string]any{"name": "Desk"},
		"agents": w.createChoices("planner", "writer"), "leader": map[string]any{"agent": "planner"},
	}

	// A skill name that is taken is not decided for the person.
	conflict := wantPackError(t, w.commit(t, w.AdminID, body), http.StatusConflict, "pack_name_conflict")
	if names, _ := conflict["skills"].([]any); len(names) != 1 || names[0] != "tone-check" {
		t.Errorf("conflict %v, want the skill tone-check", conflict)
	}
	sameCounts(t, "after the first refusal", before, packCounts(t, w.ID))

	// So is an agent name.
	body["skills"] = map[string]any{"tone-check": "import_as_new"}
	conflict = wantPackError(t, w.commit(t, w.AdminID, body), http.StatusConflict, "pack_name_conflict")
	if names, _ := conflict["agents"].([]any); len(names) != 1 || names[0] != "Writer" {
		t.Errorf("conflict %v, want the agent Writer", conflict)
	}
	sameCounts(t, "after the second refusal", before, packCounts(t, w.ID))

	// A new name for the agent and a new name for the skill settle both; the skill that was there stays.
	body["agents"] = []map[string]any{
		{"key": "planner", "action": "create", "runtime_id": w.RuntimeID},
		{"key": "writer", "action": "create", "runtime_id": w.RuntimeID, "name": "Issue Writer"},
	}
	w.commit(t, w.AdminID, body).Want(http.StatusCreated)
	var mine, theirs string
	dbfx.QueryRow(t, `SELECT content FROM skill WHERE workspace_id = $1 AND name = 'tone-check'`, w.ID).Scan(&mine)
	dbfx.QueryRow(t, `SELECT content FROM skill WHERE workspace_id = $1 AND name = 'tone-check-2'`, w.ID).Scan(&theirs)
	if mine != "my own text" || !strings.Contains(theirs, "Check that the text keeps the voice") {
		t.Errorf("skills tone-check %q, tone-check-2 %.40q", mine, theirs)
	}
}

func TestPackCommit_SkillsOnly_NoAgentsNoSquad(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	preview := w.previewOK(t, packFixture(t, "skills_only"))
	var res PackCommitResponse
	w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}).Want(http.StatusCreated).JSON(&res)
	if res.SquadID != nil || len(res.Agents) != 0 || len(res.Skills) != 3 {
		t.Fatalf("result = %+v", res)
	}
	got := packCounts(t, w.ID)
	if got["skill"] != 3 || got["agent"] != 0 || got["squad"] != 0 || got["install"] != 1 || got["hub_item"] != 0 {
		t.Fatalf("rows = %v", got)
	}
	var squadID *string
	dbfx.QueryRow(t, `SELECT squad_id::text FROM squad_pack_install WHERE id = $1`, res.InstallID).Scan(&squadID)
	if squadID != nil {
		t.Errorf("the install names squad %s", *squadID)
	}
	// Again: nothing is written.
	var again PackCommitResponse
	w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}).Want(http.StatusOK).JSON(&again)
	if !again.AlreadyInstalled || len(again.Skills) != 3 {
		t.Errorf("again = %+v", again)
	}
	sameCounts(t, "after the same bundle again", got, packCounts(t, w.ID))

	// N5: the person makes an agent with the tools they have, and gives it a skill the pack brought.
	var skillID string
	for _, s := range res.Skills {
		if s.Name == "clear-brief" {
			skillID = s.SkillID
		}
	}
	var agent AgentResponse
	create := w.request(w.AdminID, "POST", "/api/agents", map[string]any{
		"name": "Brief writer", "runtime_id": w.RuntimeID, "skill_ids": []string{skillID},
	})
	testutil.Call(t, testHandler.CreateAgent, create).Want(http.StatusCreated).JSON(&agent)
	if len(agent.Skills) != 1 || agent.Skills[0].Name != "clear-brief" {
		t.Errorf("the new agent has skills %+v", agent.Skills)
	}

	t.Run("a name that is taken is kept or imported as new", func(t *testing.T) {
		w := newPackWorkspace(t)
		taken, err := testHandler.createSkillWithFiles(context.Background(), skillCreateInput{
			WorkspaceID: parseUUID(w.ID), CreatorID: parseUUID(w.AdminID), Name: "data-check", Description: "mine", Content: "mine",
		})
		if err != nil {
			t.Fatal(err)
		}
		preview := w.previewOK(t, packFixture(t, "skills_only"))
		wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}), http.StatusConflict, "pack_name_conflict")
		wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest, "skills": map[string]any{"data-check": "overwrite"}}), http.StatusBadRequest, "pack_choice_invalid")

		var kept PackCommitResponse
		w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest, "skills": map[string]any{"data-check": "keep_existing"}}).Want(http.StatusCreated).JSON(&kept)
		for _, s := range kept.Skills {
			if s.Name == "data-check" && (s.Created || s.SkillID != taken.ID) {
				t.Errorf("data-check %+v, want the one that was there", s)
			}
		}
		if got := packCounts(t, w.ID)["skill"]; got != 3 {
			t.Errorf("%d skills, want the 2 new ones and the one that was there", got)
		}
		var content string
		dbfx.QueryRow(t, `SELECT content FROM skill WHERE id = $1`, taken.ID).Scan(&content)
		if content != "mine" {
			t.Errorf("the skill that was there now says %q", content)
		}

		other := newPackWorkspace(t)
		dbfx.Exec(t, `INSERT INTO skill (workspace_id, name, description, content, config, created_by) VALUES ($1, 'data-check', 'mine', 'mine', '{}'::jsonb, $2)`, other.ID, other.AdminID)
		otherPreview := other.previewOK(t, packFixture(t, "skills_only"))
		var renamed PackCommitResponse
		other.commit(t, other.AdminID, map[string]any{"digest": otherPreview.Digest, "skills": map[string]any{"data-check": "import_as_new"}}).Want(http.StatusCreated).JSON(&renamed)
		var imported int
		dbfx.QueryRow(t, `SELECT count(*) FROM skill WHERE workspace_id = $1 AND name IN ('data-check', 'data-check-2')`, other.ID).Scan(&imported)
		if imported != 2 {
			t.Errorf("%d of data-check and data-check-2 exist after the import as new, want both", imported)
		}
		for _, s := range renamed.Skills {
			if s.Name == "data-check" && !s.Created {
				t.Errorf("data-check %+v, want the copy that was made", s)
			}
		}
	})
}

func TestPackCommit_ExtraAgentsAndGenericLead(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	preview := w.previewOK(t, packFixture(t, "skills_only"))
	before := packCounts(t, w.ID)

	// Agents without a lead make no squad; the commit says what is missing and writes nothing.
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{
		"digest":       preview.Digest,
		"extra_agents": []map[string]any{{"key": "editor", "name": "Editor", "instructions": "Edit.", "runtime_id": w.RuntimeID}},
	}), http.StatusBadRequest, "pack_choice_invalid")
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{
		"digest": preview.Digest, "squad": map[string]any{"name": "Desk"},
		"leader": map[string]any{"generic": true},
	}), http.StatusBadRequest, "pack_choice_invalid")
	sameCounts(t, "after the refusals", before, packCounts(t, w.ID))

	var res PackCommitResponse
	w.commit(t, w.AdminID, map[string]any{
		"digest": preview.Digest, "squad": map[string]any{"name": "Briefing desk"},
		"extra_agents": []map[string]any{{
			"key": "editor", "name": "Editor", "instructions": "Edit the brief.", "runtime_id": w.RuntimeID, "skills": []string{"clear-brief"},
		}},
		"leader": map[string]any{"generic": true, "runtime_id": w.RuntimeID},
	}).Want(http.StatusCreated).JSON(&res)
	if res.SquadID == nil || len(res.Agents) != 2 || len(res.Skills) != 3 {
		t.Fatalf("result = %+v", res)
	}
	// The lead was made from the catalog template: it follows the workflow, it does not split the work.
	var leaderInstructions, leaderName string
	dbfx.QueryRow(t, `SELECT a.instructions, a.name FROM squad s JOIN agent a ON a.id = s.leader_id WHERE s.id = $1`, *res.SquadID).Scan(&leaderInstructions, &leaderName)
	if !strings.Contains(leaderInstructions, "multica issue flow show") || !strings.Contains(leaderInstructions, "You do not create sub-issues") {
		t.Errorf("the lead %q has instructions %.80q", leaderName, leaderInstructions)
	}
	var editorSkills, roles string
	dbfx.QueryRow(t, `SELECT COALESCE(string_agg(s.name, ','), '') FROM agent_skill ags JOIN skill s ON s.id = ags.skill_id JOIN agent a ON a.id = ags.agent_id WHERE a.workspace_id = $1 AND a.name = 'Editor'`, w.ID).Scan(&editorSkills)
	dbfx.QueryRow(t, `SELECT string_agg(role, ',' ORDER BY role) FROM squad_member WHERE squad_id = $1`, *res.SquadID).Scan(&roles)
	if editorSkills != "clear-brief" || roles != ",leader" {
		t.Errorf("editor skills %q member roles %q", editorSkills, roles)
	}
	if got := packCounts(t, w.ID); got["agent"] != 2 || got["squad"] != 1 || got["member"] != 2 || got["skill"] != 3 {
		t.Errorf("rows = %v", got)
	}
}

func TestPackCommit_NativeFlowBecomesUnboundDraft(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)

	t.Run("roles are matched by the key the bundle declares, not by the name of an agent", func(t *testing.T) {
		w := newPackWorkspace(t)
		// An agent of the workspace is named like the role's key. It is not the one the person chose.
		decoy := dbfx.Agent(t, "writer", w.RuntimeID, testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID, "visibility": "workspace", "permission_mode": "public_to"})
		preview := w.previewOK(t, packFixture(t, "native_bundle"))
		if !preview.Flow.Present || preview.Flow.Validation == nil || !preview.Flow.Validation.OK {
			t.Fatalf("preview flow %+v", preview.Flow)
		}
		var res PackCommitResponse
		w.commit(t, w.AdminID, map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Newsletter desk"},
			"agents": []map[string]any{
				{"key": "planner", "action": "create", "runtime_id": w.RuntimeID},
				{"key": "writer", "action": "create", "runtime_id": w.RuntimeID, "name": "Issue Writer"},
			},
			"leader":                 map[string]any{"agent": "planner"},
			"create_draft_from_flow": true,
		}).Want(http.StatusCreated).JSON(&res)
		if res.DraftID == nil || len(res.FlowIssues) != 0 {
			t.Fatalf("result = %+v", res)
		}
		var origin, doc, sourceRef string
		dbfx.QueryRow(t, `SELECT origin, doc::text, source_ref::text FROM issue_workflow_draft WHERE id = $1 AND squad_id = $2`, *res.DraftID, *res.SquadID).Scan(&origin, &doc, &sourceRef)
		if origin != "pack" || !strings.Contains(sourceRef, res.InstallID) {
			t.Errorf("origin %q source_ref %s", origin, sourceRef)
		}
		var parsed struct {
			Roles []struct {
				Key  string `json:"key"`
				Kind string `json:"kind"`
				Bind *struct {
					AgentIDs []string `json:"agent_ids"`
				} `json:"bind"`
				Source *struct {
					Bundle   string `json:"bundle"`
					AgentKey string `json:"agent_key"`
				} `json:"source"`
			} `json:"roles"`
		}
		if err := json.Unmarshal([]byte(doc), &parsed); err != nil {
			t.Fatal(err)
		}
		var writerID string
		for _, a := range res.Agents {
			if a.Key == "writer" {
				writerID = a.AgentID
			}
		}
		if writerID == "" || writerID == decoy || len(parsed.Roles) != 1 {
			t.Fatalf("writer %q decoy %q roles %+v", writerID, decoy, parsed.Roles)
		}
		role := parsed.Roles[0]
		if role.Key != "writer" || role.Bind == nil || len(role.Bind.AgentIDs) != 1 || role.Bind.AgentIDs[0] != writerID {
			t.Errorf("the role is bound to %+v, want the agent the person chose (%s)", role.Bind, writerID)
		}
		if role.Source == nil || role.Source.AgentKey != "writer" || role.Source.Bundle != res.InstallID {
			t.Errorf("role source %+v", role.Source)
		}
		var rosterDraft string
		dbfx.QueryRow(t, `SELECT roster->>'draft_id' FROM squad_pack_install WHERE id = $1`, res.InstallID).Scan(&rosterDraft)
		if rosterDraft != *res.DraftID {
			t.Errorf("the install records draft %q", rosterDraft)
		}
	})

	t.Run("the draft publishes as it is: every role of the bundle is bound by its key", func(t *testing.T) {
		w := newPackWorkspace(t)
		preview := w.previewOK(t, packFixture(t, "native_bundle"))
		var res PackCommitResponse
		w.commit(t, w.AdminID, map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Desk"}, "agents": w.createChoices("planner", "writer"),
			"leader": map[string]any{"agent": "planner"}, "create_draft_from_flow": true,
		}).Want(http.StatusCreated).JSON(&res)
		var revision int32
		dbfx.QueryRow(t, `SELECT revision FROM issue_workflow_draft WHERE id = $1`, *res.DraftID).Scan(&revision)
		req := w.request(w.AdminID, "POST", "/api/squads/"+*res.SquadID+"/flow-drafts/"+*res.DraftID+"/publish", map[string]any{"revision": revision, "ack": []string{"no_gate"}})
		req = withURLParams(req, "workspaceId", w.ID, "id", *res.SquadID, "draftId", *res.DraftID)
		var published IssueWorkflowResponse
		testutil.Call(t, testHandler.PublishFlowDraft, req).Want(http.StatusCreated).JSON(&published)
		if published.SquadID == nil || *published.SquadID != *res.SquadID {
			t.Errorf("published = %+v", published)
		}
	})

	t.Run("a role that names no agent of the bundle stays unbound", func(t *testing.T) {
		w := newPackWorkspace(t)
		files := packFixture(t, "native_bundle")
		files["flow.json"] = strings.Replace(files["flow.json"], `"roles": [{ "key": "writer", "kind": "agent" }]`, `"roles": [{ "key": "writer", "kind": "agent" }, { "key": "stranger", "kind": "agent" }]`, 1)
		files["flow.json"] = strings.Replace(files["flow.json"], `"role": "writer" }]`, `"role": "writer" }, { "key": "extra_it", "title": "Extra", "kind": "work", "role": "stranger" }]`, 1)
		preview := w.previewOK(t, files)
		var warned bool
		for _, i := range preview.Flow.Issues {
			warned = warned || (i.Code == "pack_flow_role_unmatched" && i.Severity == "warning")
		}
		if !warned {
			t.Errorf("the preview does not say the role stranger has no agent: %+v", preview.Flow)
		}
		var res PackCommitResponse
		w.commit(t, w.AdminID, map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Desk"}, "agents": w.createChoices("planner", "writer"),
			"leader": map[string]any{"agent": "planner"}, "create_draft_from_flow": true,
		}).Want(http.StatusCreated).JSON(&res)
		if res.DraftID == nil {
			t.Fatalf("no draft: %+v", res.FlowIssues)
		}
		var bound int
		dbfx.QueryRow(t, `SELECT count(*) FROM issue_workflow_draft d, jsonb_array_elements(d.doc->'roles') r WHERE d.id = $1 AND r ? 'bind'`, *res.DraftID).Scan(&bound)
		if bound != 1 {
			t.Errorf("%d roles are bound, want only the writer", bound)
		}
	})

	t.Run("a flow that names the leader as a role is refused", func(t *testing.T) {
		w := newPackWorkspace(t)
		files := packFixture(t, "native_bundle")
		files["flow.json"] = strings.Replace(files["flow.json"], `"roles": [{ "key": "writer", "kind": "agent" }]`, `"roles": [{ "key": "planner", "kind": "agent" }, { "key": "writer", "kind": "agent" }]`, 1)
		preview := w.previewOK(t, files)
		body := wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}), http.StatusBadRequest, "pack_invalid")
		if !strings.Contains(fmt.Sprint(body["issues"]), "pack_flow_role_leader") {
			t.Errorf("body %v", body)
		}
	})

	t.Run("a flow that does not check makes no draft, and the install goes on", func(t *testing.T) {
		w := newPackWorkspace(t)
		files := packFixture(t, "native_bundle")
		files["flow.json"] = strings.Replace(files["flow.json"], `"role": "writer"`, `"role": "nobody"`, 1)
		preview := w.previewOK(t, files)
		if preview.Flow.Validation == nil || preview.Flow.Validation.OK {
			t.Fatalf("the preview takes the flow for good: %+v", preview.Flow)
		}
		var res PackCommitResponse
		w.commit(t, w.AdminID, map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Desk"}, "agents": w.createChoices("planner", "writer"),
			"leader": map[string]any{"agent": "planner"}, "create_draft_from_flow": true,
		}).Want(http.StatusCreated).JSON(&res)
		if res.DraftID != nil || len(res.FlowIssues) == 0 || res.SquadID == nil {
			t.Fatalf("result = %+v", res)
		}
		if got := packCounts(t, w.ID); got["draft"] != 0 || got["squad"] != 1 {
			t.Errorf("rows = %v", got)
		}
	})

	t.Run("no draft is made unless it is asked for", func(t *testing.T) {
		w := newPackWorkspace(t)
		res := w.installNative(t, nil)
		if res.DraftID != nil || packCounts(t, w.ID)["draft"] != 0 {
			t.Errorf("result = %+v", res)
		}
	})

	t.Run("a draft cannot be made while the flow documents of schema 2 are off, and nothing is written", func(t *testing.T) {
		packFlags(t, true, true, false)
		w := newPackWorkspace(t)
		preview := w.previewOK(t, packFixture(t, "native_bundle"))
		before := packCounts(t, w.ID)
		wantPackError(t, w.commit(t, w.AdminID, map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Desk"}, "agents": w.createChoices("planner", "writer"),
			"leader": map[string]any{"agent": "planner"}, "create_draft_from_flow": true,
		}), http.StatusNotFound, "flow_graph_v2_disabled")
		sameCounts(t, "after the refusal", before, packCounts(t, w.ID))
	})
}

func TestPackCommit_UseExistingAgentNeedsWirePermission(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	// An agent of the admin that nobody else may call.
	private := dbfx.Agent(t, "Private writer", w.RuntimeID, testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID, "visibility": "private", "permission_mode": "private"})
	public := dbfx.Agent(t, "Public writer", w.RuntimeID, testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID, "visibility": "workspace", "permission_mode": "public_to"})
	dbfx.Exec(t, `INSERT INTO agent_invocation_target (agent_id, target_type, target_id) VALUES ($1, 'workspace', $2) ON CONFLICT (agent_id, target_type, target_id) DO NOTHING`, public, w.ID)
	preview := w.previewOKAs(t, w.MemberID, packFixture(t, "native_bundle"))
	before := packCounts(t, w.ID)
	body := func(writer string) map[string]any {
		return map[string]any{
			"digest": preview.Digest, "squad": map[string]any{"name": "Desk"},
			"agents": []map[string]any{
				{"key": "planner", "action": "create", "runtime_id": w.RuntimeID},
				{"key": "writer", "action": "use_existing", "existing_agent_id": writer},
			},
			"leader": map[string]any{"agent": "planner"},
		}
	}

	// A member who may not call the agent may not put it in a squad, as with the route that adds a member.
	w.commit(t, w.MemberID, body(private)).Want(http.StatusForbidden)
	sameCounts(t, "after the refusal", before, packCounts(t, w.ID))
	// An agent anyone may call is theirs to use.
	var res PackCommitResponse
	w.commit(t, w.MemberID, body(public)).Want(http.StatusCreated).JSON(&res)
	for _, a := range res.Agents {
		if a.Key == "writer" && (a.Created || a.AgentID != public) {
			t.Errorf("writer %+v, want the existing agent %s", a, public)
		}
	}
	// An admin may use any agent.
	other := newPackWorkspace(t)
	adminPrivate := dbfx.Agent(t, "Private writer", other.RuntimeID, testutil.Cols{"workspace_id": other.ID, "owner_id": other.MemberID, "visibility": "private", "permission_mode": "private"})
	otherPreview := other.previewOK(t, packFixture(t, "native_bundle"))
	other.commit(t, other.AdminID, map[string]any{
		"digest": otherPreview.Digest, "squad": map[string]any{"name": "Desk"},
		"agents": []map[string]any{
			{"key": "planner", "action": "create", "runtime_id": other.RuntimeID},
			{"key": "writer", "action": "use_existing", "existing_agent_id": adminPrivate},
		},
		"leader": map[string]any{"agent": "planner"},
	}).Want(http.StatusCreated)

	// An agent of another workspace is not an agent here.
	foreign := newPackWorkspace(t)
	foreignAgent := dbfx.Agent(t, "Foreign", foreign.RuntimeID, testutil.Cols{"workspace_id": foreign.ID, "owner_id": foreign.AdminID})
	w2 := newPackWorkspace(t)
	p2 := w2.previewOK(t, packFixture(t, "native_bundle"))
	w2.commit(t, w2.AdminID, map[string]any{
		"digest": p2.Digest, "squad": map[string]any{"name": "Desk"},
		"agents": []map[string]any{
			{"key": "planner", "action": "create", "runtime_id": w2.RuntimeID},
			{"key": "writer", "action": "use_existing", "existing_agent_id": foreignAgent},
		},
		"leader": map[string]any{"agent": "planner"},
	}).Want(http.StatusBadRequest)
}

func TestPackCommit_RuntimeOfAnotherPersonNeedsAccess(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	private := dbfx.Runtime(t, "Private runtime", testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID, "visibility": "private"})
	preview := w.previewOKAs(t, w.MemberID, packFixture(t, "multica_pack"))
	before := packCounts(t, w.ID)
	// The rule of the agent create: a runtime that is private is used by its owner.
	w.commit(t, w.MemberID, map[string]any{
		"digest": preview.Digest, "agents": []map[string]any{{"key": "field_researcher", "action": "create", "runtime_id": private}},
		"leader": map[string]any{"agent": "field_researcher"}, "squad": map[string]any{"name": "Desk"},
	}).Want(http.StatusForbidden)
	sameCounts(t, "after the refusal", before, packCounts(t, w.ID))
}

func TestPack_AgentActor403(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	agent := dbfx.Agent(t, "Caller", w.RuntimeID, testutil.Cols{"workspace_id": w.ID, "owner_id": w.AdminID})
	asAgent := func(req *http.Request) *http.Request {
		req.Header.Set("X-Actor-Source", "task_token")
		req.Header.Set("X-Agent-ID", agent)
		return req
	}
	var body bytes.Buffer
	form := multipart.NewWriter(&body)
	part, _ := form.CreateFormFile("file", "pack.zip")
	_, _ = part.Write(packZip(t, packFixture(t, "skills_only")))
	_ = form.Close()
	preview := httptest.NewRequest(http.MethodPost, "/api/squads/packs/preview", &body)
	preview.Header.Set("Content-Type", form.FormDataContentType())
	preview.Header.Set("X-Workspace-ID", w.ID)
	preview.Header.Set("X-User-ID", w.AdminID)
	wantPackError(t, testutil.Call(t, testHandler.PreviewSquadPack, asAgent(preview)), http.StatusForbidden, "pack_member_only")

	ok := w.previewOK(t, packFixture(t, "skills_only"))
	before := packCounts(t, w.ID)
	commit := asAgent(w.request(w.AdminID, "POST", "/api/squads/packs/commit", map[string]any{"digest": ok.Digest}))
	wantPackError(t, testutil.Call(t, testHandler.CommitSquadPack, commit), http.StatusForbidden, "pack_member_only")
	sameCounts(t, "after the refusal", before, packCounts(t, w.ID))

	// Somebody who is not a member of the workspace is not let in either.
	stranger := dbfx.User(t, "Stranger", testutil.UniqueEmail("pack-stranger"))
	testutil.Call(t, testHandler.CommitSquadPack, w.request(stranger, "POST", "/api/squads/packs/commit", map[string]any{"digest": ok.Digest})).Want(http.StatusNotFound)
}

func TestPack_FlagOff404(t *testing.T) {
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	withPacks(t)
	preview := w.previewOK(t, packFixture(t, "skills_only"))
	before := packCounts(t, w.ID)

	// Packs off, whatever else is on.
	packFlags(t, false, true, true)
	wantPackError(t, w.preview(t, w.AdminID, packZip(t, packFixture(t, "skills_only"))), http.StatusNotFound, "flow_authoring_disabled")
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}), http.StatusNotFound, "flow_authoring_disabled")
	sameCounts(t, "with packs off", before, packCounts(t, w.ID))

	// What was installed stays and works with packs off; the flag only stops new installs.
	packFlags(t, true, true, true)
	w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}).Want(http.StatusCreated)
	packFlags(t, false, false, false)
	if got := packCounts(t, w.ID)["skill"]; got != 3 {
		t.Errorf("%d skills with packs off", got)
	}
}

func TestPack_CapabilitiesReportsTheFlag(t *testing.T) {
	read := func() FlowCapabilitiesResponse {
		var out FlowCapabilitiesResponse
		testutil.Call(t, testHandler.GetFlowCapabilities, newRequest("GET", "/api/flow/capabilities", nil)).Want(http.StatusOK).JSON(&out)
		return out
	}
	packFlags(t, false, true, true)
	if read().Features.Packs {
		t.Error("features.packs is on with the flag off")
	}
	packFlags(t, true, true, true)
	if !read().Features.Packs {
		t.Error("features.packs is off with the flag on")
	}
}

func TestPackCommit_StagedBundleExpires(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	hubPreviewTTL = -time.Minute
	preview := w.previewOK(t, packFixture(t, "skills_only"))
	before := packCounts(t, w.ID)
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": preview.Digest}), http.StatusNotFound, "pack_preview_not_found")
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": "sha256:" + strings.Repeat("0", 64)}), http.StatusNotFound, "pack_preview_not_found")
	wantPackError(t, w.commit(t, w.AdminID, map[string]any{"digest": "not a digest"}), http.StatusBadRequest, "pack_choice_invalid")
	sameCounts(t, "after the refusals", before, packCounts(t, w.ID))
}

func TestPackPreview_FromURLZip(t *testing.T) {
	withPacks(t)
	setupHubImportTest(t)
	w := newPackWorkspace(t)
	zipBytes := packZip(t, packFixture(t, "multica_pack"))
	server := httptest.NewServer(http.HandlerFunc(func(rw http.ResponseWriter, _ *http.Request) {
		rw.Header().Set("Content-Type", "application/zip")
		_, _ = rw.Write(zipBytes)
	}))
	defer server.Close()
	req := w.request(w.AdminID, "POST", "/api/squads/packs/preview", map[string]any{"source": map[string]any{"kind": "url_zip", "url": server.URL + "/pack.zip"}})
	var out PackPreviewResponse
	testutil.Call(t, testHandler.PreviewSquadPack, req).Want(http.StatusOK).JSON(&out)
	if out.Format != "multica_pack" || len(out.Agents) != 1 {
		t.Fatalf("preview = %+v", out)
	}
	bad := w.request(w.AdminID, "POST", "/api/squads/packs/preview", map[string]any{"source": map[string]any{"kind": "ftp", "url": "ftp://x"}})
	testutil.Call(t, testHandler.PreviewSquadPack, bad).Want(http.StatusBadRequest)
}

func TestSquadPackInstallQueries(t *testing.T) {
	w := newPackWorkspace(t)
	ctx := context.Background()
	ws := parseUUID(w.ID)
	mk := func(packID, digest string) {
		t.Helper()
		if _, err := testHandler.Queries.CreateSquadPackInstall(ctx, db.CreateSquadPackInstallParams{
			ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, WorkspaceID: ws, PackID: packID, PackVersion: "1", Format: "native_bundle", Digest: digest,
			Source: []byte(`{}`), Roster: []byte(`{}`), InstalledBy: parseUUID(w.AdminID),
		}); err != nil {
			t.Fatal(err)
		}
	}
	mk("desk", "sha256:a")
	time.Sleep(5 * time.Millisecond)
	mk("desk", "sha256:b")
	byDigest, err := testHandler.Queries.GetSquadPackInstallByDigest(ctx, db.GetSquadPackInstallByDigestParams{WorkspaceID: ws, Digest: "sha256:a"})
	if err != nil || byDigest.Digest != "sha256:a" || byDigest.SquadID.Valid {
		t.Fatalf("by digest: %+v, %v", byDigest, err)
	}
	latest, err := testHandler.Queries.GetLatestSquadPackInstallByPackID(ctx, db.GetLatestSquadPackInstallByPackIDParams{WorkspaceID: ws, PackID: "desk"})
	if err != nil || latest.Digest != "sha256:b" {
		t.Fatalf("latest of the pack: %+v, %v", latest, err)
	}
	// Another workspace does not see them.
	other := newPackWorkspace(t)
	if _, err := testHandler.Queries.GetSquadPackInstallByDigest(ctx, db.GetSquadPackInstallByDigestParams{WorkspaceID: parseUUID(other.ID), Digest: "sha256:a"}); err == nil {
		t.Error("an install of another workspace was found")
	}
}

// A deleted workspace takes its installs with it: there is no foreign key to do it (spec 16.3).
func TestSquadPackInstall_WorkspaceTeardownSweepsInstalls(t *testing.T) {
	a, b := newPackWorkspace(t), newPackWorkspace(t)
	for _, w := range []packWS{a, b} {
		dbfx.Exec(t, `INSERT INTO squad_pack_install (workspace_id, format, digest, installed_by) VALUES ($1, 'skills_only', 'sha256:x', $2)`, w.ID, w.AdminID)
	}
	if err := testHandler.Queries.DeleteSquadPackInstallsForWorkspace(context.Background(), parseUUID(a.ID)); err != nil {
		t.Fatal(err)
	}
	if got := packCounts(t, a.ID)["install"]; got != 0 {
		t.Errorf("%d installs left in the workspace that was deleted", got)
	}
	if got := packCounts(t, b.ID)["install"]; got != 1 {
		t.Errorf("%d installs in the other workspace, want 1", got)
	}
}
