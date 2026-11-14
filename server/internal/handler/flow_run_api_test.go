package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The run API and the close rules of the general engine (plan P12). The workflows
// are small documents of schema 2, built here, because each test needs one feature.

func docRole(key, kind string) map[string]any { return map[string]any{"key": key, "kind": kind} }

func docEdge(id, to string, when map[string]any) map[string]any {
	e := map[string]any{"id": id, "label": id, "to": to}
	if when != nil {
		e["when"] = when
	}
	return e
}

func docStopEdge(id, stop string, when map[string]any) map[string]any {
	e := map[string]any{"id": id, "label": id, "stop": stop}
	if when != nil {
		e["when"] = when
	}
	return e
}

func joinIs(result string) map[string]any {
	return map[string]any{"field": "join.result", "op": "eq", "value": result}
}

func docTask(key, kind, role string, extra map[string]any) map[string]any {
	t := map[string]any{"key": key, "title": key, "kind": kind, "role": role, "instructions": "Do " + key + "."}
	for k, v := range extra {
		t[k] = v
	}
	return t
}

// docStep is a tasks step with its own board status.
func docStep(key string, tasks []any, edges ...any) map[string]any {
	return map[string]any{"key": key, "title": key, "kind": "tasks", "status": map[string]any{"name": key, "category": "started"}, "tasks": tasks, "edges": edges}
}

func docManual(key, exitRole string, edges ...any) map[string]any {
	step := map[string]any{"key": key, "title": key, "kind": "manual", "status": map[string]any{"key": "in_review"}, "notice": "stop2", "edges": edges}
	if exitRole != "" {
		step["exit_role"] = exitRole
	}
	return step
}

func manualEdge(id, to string) map[string]any {
	return map[string]any{"id": id, "label": id, "to": to, "trigger": "manual"}
}

// docLead is the plan step: it sets these variables and goes to the step.
func docLead(next string, sets ...string) map[string]any {
	lead := map[string]any{"sets": sets}
	if sets == nil {
		lead = map[string]any{"sets": []any{}}
	}
	return map[string]any{"key": "plan", "title": "Plan", "kind": "lead", "status": map[string]any{"key": "todo"}, "lead": lead,
		"edges": []any{docStopEdge("plan_blocked", "grounds_missing", map[string]any{"field": "lead.verdict", "op": "eq", "value": "blocked"}), docEdge("plan_ok", next, nil)}}
}

func baseDoc(roles []any, steps ...any) map[string]any {
	return map[string]any{
		"version": 2, "start": "plan", "roles": roles, "steps": steps,
		"terminals": []any{map[string]any{"key": "done", "status": "done"}},
		"stops": []any{
			map[string]any{"code": "grounds_missing", "class": "missing_grounds", "message": "stop1", "legacy_no": 1},
			map[string]any{"code": "loop_exhausted", "class": "loop_exhausted", "message": "stop3", "legacy_no": 3},
			map[string]any{"code": "undecided", "class": "custom", "message": "stop1"},
		},
		"messages": map[string]any{"stop1": "BLOCKED · stop\n{request}\n{missing}", "stop3": "BLOCKED · used\n{request}", "stop2": "IN REVIEW\n{request}"},
		"policy":   map[string]any{"allow_unattended_release": true},
	}
}

func attachFile(t *testing.T, issueID, uploaderType, uploaderID, name, contentType string) {
	t.Helper()
	dbfx.Insert(t, "attachment", testutil.Cols{
		"workspace_id": testWorkspaceID, "issue_id": issueID,
		"uploader_type": uploaderType, "uploader_id": uploaderID,
		"filename": name, "url": "https://files.example.test/" + name,
		"content_type": contentType, "size_bytes": 12,
	})
}

// closeAs closes a task as a member: the user whose wrapper it is.
func closeAs(t *testing.T, as func(*http.Request) *http.Request, issueID string, body map[string]any) *testutil.Response {
	t.Helper()
	req := as(withURLParam(newRequest("PUT", "/api/issues/"+issueID, body), "id", issueID))
	return testutil.Call(t, testHandler.UpdateIssue, req)
}

func firstRow(t *testing.T, rig *graphRig, request db.Issue, step, key string) string {
	t.Helper()
	id := rig.rows(request, step, 0)[key]
	if id == "" {
		t.Fatalf("no row %s/%s: %v", step, key, rig.rows(request, step, 0))
	}
	return id
}

// triageDoc is a gate with a verdict set of its own: ship passes, fix fails, defer is neutral.
func triageDoc() map[string]any {
	doc := baseDoc([]any{docRole("checker", "agent")},
		docLead("review"),
		docStep("review", []any{docTask("triage", "gate", "checker", map[string]any{"verdicts": "triage"})},
			docEdge("r_pass", "done", joinIs("pass")),
			docStopEdge("r_fail", "loop_exhausted", joinIs("fail")),
			docStopEdge("r_block", "grounds_missing", joinIs("block")),
			docStopEdge("r_none", "undecided", nil)))
	doc["verdicts"] = []any{map[string]any{"key": "triage", "verdicts": []any{
		map[string]any{"key": "ship", "class": "pass"}, map[string]any{"key": "fix", "class": "fail"}, map[string]any{"key": "defer", "class": "neutral"},
	}}}
	return doc
}

func TestVerdictSet_CustomAcceptedOthersRejected(t *testing.T) {
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	gate := firstRow(t, rig, request, "review", "triage")
	checker := rig.agents["checker"][0]
	put := func(body map[string]any) *testutil.Response {
		return agentOnIssue(t, gate, checker, "review", testHandler.UpdateIssue, "PUT", "/api/issues/"+gate, body)
	}

	// The verdict of the old kinds is not in this set; the answer names the keys that are.
	body := wantFlowRejection(t, put(map[string]any{"status": "done", "outcome": "approved"}), 422, "flow_outcome_invalid")
	if keys, _ := body["allowed_outcomes"].([]any); len(keys) != 3 || keys[0] != "ship" || keys[1] != "fix" || keys[2] != "defer" {
		t.Fatalf("allowed_outcomes = %v, want the keys of the set", body["allowed_outcomes"])
	}
	wantFlowRejection(t, put(map[string]any{"status": "done"}), 422, "flow_outcome_required")
	// A key that is not shaped like a verdict never reaches the rules.
	put(map[string]any{"status": "done", "outcome": "Ship It"}).Want(400)
	put(map[string]any{"status": "done", "outcome": "ship"}).Want(200)

	// The same key is refused on an issue that no flow runs: the four outcomes stay the only ones.
	plain := dbfx.Issue(t, "plain issue "+workflowTestSuffix(), nil)
	updateIssueForTest(t, plain, map[string]any{"status": "done", "outcome": "ship"}).Want(400)
}

func TestVerdict_BlockedAlwaysAllowed(t *testing.T) {
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	gate := firstRow(t, rig, request, "review", "triage")
	checker := rig.agents["checker"][0]
	put := func(body map[string]any) *testutil.Response {
		return agentOnIssue(t, gate, checker, "review", testHandler.UpdateIssue, "PUT", "/api/issues/"+gate, body)
	}
	// blocked is in no set and is always allowed, with a blocked status; never with done.
	wantFlowRejection(t, put(map[string]any{"status": "done", "outcome": "blocked"}), 422, "flow_outcome_invalid")
	wantFlowRejection(t, put(map[string]any{"status": "blocked", "outcome": "ship"}), 422, "flow_outcome_invalid")
	put(map[string]any{"status": "blocked", "outcome": "blocked"}).Want(200)
	endRuns(t, gate)
	st := rig.advance(request)
	if st.Stop == nil || st.Stop.Code != "grounds_missing" || st.Stop.LegacyNo != 1 {
		t.Fatalf("stop %+v, want grounds_missing (legacy 1)", st.Stop)
	}
}

func TestVerdict_ClassDrivesJoinNotTheName(t *testing.T) {
	cases := []struct {
		verdict string
		stop    string
		closed  bool
	}{
		{"ship", "", true},
		{"fix", "loop_exhausted", false},
		{"defer", "undecided", false},
	}
	for _, tc := range cases {
		t.Run(tc.verdict, func(t *testing.T) {
			rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
			request := rig.newRequest()
			rig.complete(request, nil)
			rig.finishAll(request, "review", 0, always(tc.verdict))
			st := rig.advance(request)
			switch {
			case tc.closed:
				rig.wantClosedAt(request, "done")
			case st.Stop == nil || st.Stop.Code != tc.stop:
				t.Fatalf("%s: stop %+v, want %s", tc.verdict, st.Stop, tc.stop)
			}
		})
	}
}

// filesDoc has a work task that needs two markdown reports (at most three) and a csv.
func filesDoc() map[string]any {
	return baseDoc([]any{docRole("dev", "agent")},
		docLead("work"),
		docStep("work", []any{docTask("report", "work", "dev", map[string]any{"requires": map[string]any{"files": []any{
			map[string]any{"slot": "report", "min": 2, "max": 3, "ext": []any{".md"}},
			map[string]any{"slot": "data", "min": 1, "ext": []any{".csv"}, "content_types": []any{"text/csv"}},
		}}})}, docEdge("w_ok", "done", nil)))
}

func TestFiles_SlotExtensionAndMinEnforced(t *testing.T) {
	rig := newGraphRigDoc(t, "files", filesDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	task := firstRow(t, rig, request, "work", "report")
	dev := rig.agents["dev"][0]
	closeTask := func() *testutil.Response {
		return agentOnIssue(t, task, dev, "work", testHandler.UpdateIssue, "PUT", "/api/issues/"+task, map[string]any{"status": "done"})
	}
	slotOf := func(body map[string]any) (slot string, have float64) {
		slot, _ = body["slot"].(string)
		have, _ = body["have"].(float64)
		return slot, have
	}

	// One report of two: the slot report is short, and says by how much.
	attachFile(t, task, "agent", dev, "a.md", "text/markdown")
	if slot, have := slotOf(wantFlowRejection(t, closeTask(), 422, "flow_attachment_required")); slot != "report" || have != 1 {
		t.Fatalf("slot %q have %v, want report with 1", slot, have)
	}
	// Two reports and a file of the wrong kind: the data slot is still empty.
	attachFile(t, task, "agent", dev, "b.md", "text/markdown")
	attachFile(t, task, "agent", dev, "notes.txt", "text/plain")
	if slot, _ := slotOf(wantFlowRejection(t, closeTask(), 422, "flow_attachment_required")); slot != "data" {
		t.Fatalf("slot %q, want data", slot)
	}
	// A csv by name but not by content type does not fit either.
	attachFile(t, task, "agent", dev, "rows.csv", "application/octet-stream")
	if slot, _ := slotOf(wantFlowRejection(t, closeTask(), 422, "flow_attachment_required")); slot != "data" {
		t.Fatalf("slot %q, want data", slot)
	}
	// Four reports are more than the slot takes.
	attachFile(t, task, "agent", dev, "rows2.csv", "text/csv")
	attachFile(t, task, "agent", dev, "c.md", "text/markdown")
	attachFile(t, task, "agent", dev, "d.md", "text/markdown")
	if slot, have := slotOf(wantFlowRejection(t, closeTask(), 422, "flow_attachment_required")); slot != "report" || have != 4 {
		t.Fatalf("slot %q have %v, want report with 4", slot, have)
	}
	dbfx.Exec(t, `DELETE FROM attachment WHERE issue_id = $1 AND filename = 'd.md'`, task)
	closeTask().Want(200)
}

func TestFiles_OnBehalfCountsTheManagersUploads(t *testing.T) {
	rig := newGraphRigDoc(t, "files", filesDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	task := firstRow(t, rig, request, "work", "report")
	// The agent attached nothing. Another member's files are nobody's: they do not count.
	otherID, _ := addWorkflowTestMember(t, "uploader")
	attachFile(t, task, "member", otherID, "x.md", "text/markdown")
	attachFile(t, task, "member", otherID, "y.md", "text/markdown")
	attachFile(t, task, "member", otherID, "z.csv", "text/csv")
	wantFlowRejection(t, closeIssue(t, task, nil), 422, "flow_attachment_required")
	// The files of the manager who closes it do.
	attachFile(t, task, "member", testUserID, "m1.md", "text/markdown")
	attachFile(t, task, "member", testUserID, "m2.md", "text/markdown")
	attachFile(t, task, "member", testUserID, "m.csv", "text/csv")
	closeIssue(t, task, nil).Want(200)
}

// memberDoc has a task for a member, then a task for the managers.
func memberDoc() map[string]any {
	return baseDoc([]any{docRole("editor", "member"), docRole("ops", "managers")},
		docLead("write"),
		docStep("write", []any{docTask("draft", "work", "editor", nil)}, docEdge("w_ok", "check", nil)),
		docStep("check", []any{docTask("sign", "work", "ops", nil)}, docEdge("c_ok", "done", nil)))
}

func TestMemberTask_AssignedToMemberAndClosedByThem(t *testing.T) {
	rig := newGraphRigDoc(t, "members", memberDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	task := firstRow(t, rig, request, "write", "draft")
	editor := rig.members["editor"][0]
	if typ, who := rig.assigneeOf(task); typ != "member" || who != editor {
		t.Fatalf("assignee %s %s, want the bound member %s", typ, who, editor)
	}
	if n := queuedRuns(t, task); n != 0 {
		t.Fatalf("%d runs queued for a task of a person", n)
	}
	// Someone else cannot close it; the member can.
	_, asOther := addWorkflowTestMember(t, "bystander")
	wantFlowRejection(t, closeAs(t, asOther, task, map[string]any{"status": "done"}), 403, "flow_task_not_assignee")
	agent := goldenLeadAgent(t, seededReadyAgentID(t))
	wantFlowRejection(t, agentOnIssue(t, task, agent, "write", testHandler.UpdateIssue, "PUT", "/api/issues/"+task, map[string]any{"status": "done"}), 403, "flow_task_not_assignee")
	endRuns(t, task) // the refused agent had a run of its own on the task
	closeAs(t, rig.as[editor], task, map[string]any{"status": "done"}).Want(200)
	if status, _, _ := issueStatusAndAssignee(t, task); status != "done" {
		t.Fatalf("the task is %q after its member closed it", status)
	}
	if n := dbfx.Count(t, `SELECT count(*) FROM activity_log WHERE issue_id = $1 AND action = 'flow_manual_close'`, task); n != 0 {
		t.Fatalf("a member closing their own task is logged as a close on behalf (%d)", n)
	}
	if st := rig.advance(request); st.Step != "check" {
		t.Fatalf("step %q stop %+v, want check: %q", st.Step, st.Stop, lastComment(t, request))
	}
}

func TestMemberTask_ManagersRoleAnyoneWithRightCloses(t *testing.T) {
	rig := newGraphRigDoc(t, "members", memberDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "write", 0, always(""))
	rig.advance(request)
	task := firstRow(t, rig, request, "check", "sign")
	if typ, who := rig.assigneeOf(task); typ != "" || who != "" {
		t.Fatalf("a task of the managers is given to %s %s, want nobody", typ, who)
	}
	_, asPlain := addWorkflowTestMember(t, "plain")
	wantFlowRejection(t, closeAs(t, asPlain, task, map[string]any{"status": "done"}), 403, "flow_task_not_assignee")
	// The workspace owner manages the squad; the close is logged as one on behalf.
	closeIssue(t, task, nil).Want(200)
	details := activityDetails(t, task, "flow_manual_close")
	if details["task"] != "sign" {
		t.Fatalf("close details = %v", details)
	}
}

func TestLeadStep_CompleteSetsVarsAtomically(t *testing.T) {
	doc := baseDoc([]any{docRole("dev", "agent")},
		docLead("work", "flag", "size"),
		docStep("work", []any{docTask("go", "work", "dev", nil)}, docEdge("w_ok", "done", nil)))
	doc["vars"] = []any{
		map[string]any{"key": "flag", "type": "bool", "default": false, "set_by": []any{"lead"}, "set_at": []any{"plan"}},
		map[string]any{"key": "size", "type": "enum", "values": []any{"s", "m"}, "default": "s", "set_by": []any{"lead"}, "set_at": []any{"plan"}},
		map[string]any{"key": "other", "type": "bool", "default": false, "set_by": []any{"managers"}, "set_at": []any{"*"}},
	}
	rig := newGraphRigDoc(t, "lead-step", doc, graphRigOpts{})
	call := func(request db.Issue, h http.HandlerFunc, path string, body map[string]any, params ...string) *testutil.Response {
		id := uuidToString(request.ID)
		return testutil.Call(t, h, withURLParams(newRequest("POST", path, body), append([]string{"id", id}, params...)...))
	}
	request := rig.newRequest()
	path := "/api/issues/" + uuidToString(request.ID) + "/flow/steps/plan/complete"

	// One value is wrong: nothing is set, nothing moves.
	before := rig.state(request)
	wantFlowRejection(t, call(request, testHandler.CompleteIssueFlowStep, path, map[string]any{"verdict": "planned", "vars": map[string]any{"flag": true, "size": "xl"}}, "step", "plan"), 422, "flow_var_invalid")
	wantFlowRejection(t, call(request, testHandler.CompleteIssueFlowStep, path, map[string]any{"verdict": "planned", "vars": map[string]any{"other": true}}, "step", "plan"), 403, "flow_var_forbidden")
	wantFlowRejection(t, call(request, testHandler.CompleteIssueFlowStep, path, map[string]any{"verdict": "shipped"}, "step", "plan"), 422, "flow_outcome_invalid")
	wantFlowRejection(t, call(request, testHandler.CompleteIssueFlowStep, path, map[string]any{"verdict": "planned"}, "step", "work"), 409, "flow_wrong_step")
	if after := rig.state(request); after.Step != before.Step || after.Vars["flag"] != false || len(rig.rows(request, "work", 0)) != 0 {
		t.Fatalf("a refused completion changed the request: %+v", after)
	}

	// The lead of the squad completes it: the variables and the verdict together, and the first task opens.
	var leader string
	dbfx.QueryRow(t, `SELECT leader_id::text FROM squad WHERE id = $1`, rig.squadID).Scan(&leader)
	res := agentOnIssue(t, uuidToString(request.ID), leader, "plan", testHandler.CompleteIssueFlowStep, "POST", path,
		map[string]any{"verdict": "planned", "vars": map[string]any{"flag": true, "size": "m"}}, "step", "plan")
	var state IssueFlowStateResponse
	res.Want(200).JSON(&state)
	if state.Step != "work" || state.Vars["flag"] != true || state.Vars["size"] != "m" || len(rig.rows(request, "work", 0)) != 1 {
		t.Fatalf("state after the completion = step %q vars %v", state.Step, state.Vars)
	}

	// The plan route is the same call on the plan step.
	second := rig.newRequest()
	var planned IssueFlowStateResponse
	testutil.Call(t, testHandler.PlanIssueFlow, withURLParam(newRequest("POST", "/api/issues/"+uuidToString(second.ID)+"/flow/plan",
		map[string]any{"switches": map[string]any{"flag": true}}), "id", uuidToString(second.ID))).Want(200).JSON(&planned)
	if planned.Step != "work" || planned.Vars["flag"] != true {
		t.Fatalf("plan alias: step %q vars %v", planned.Step, planned.Vars)
	}
}

// varsDoc has a variable for each person who may set one.
func varsDoc() map[string]any {
	doc := baseDoc([]any{docRole("dev", "agent")},
		docLead("work", "notes"),
		docStep("work", []any{docTask("go", "work", "dev", nil)}, docEdge("w_ok", "done", nil)))
	doc["vars"] = []any{
		map[string]any{"key": "scope", "type": "enum", "values": []any{"small", "big"}, "default": "small", "set_by": []any{"requester"}, "set_at": []any{"work"}},
		map[string]any{"key": "notes", "type": "string", "max_len": 20, "default": "", "set_by": []any{"lead"}, "set_at": []any{"plan"}},
	}
	return doc
}

func TestVars_SetByAndSetAtEnforced(t *testing.T) {
	rig := newGraphRigDoc(t, "vars", varsDoc(), graphRigOpts{})
	requesterID, asRequester := addWorkflowTestMember(t, "requester")
	id := dbfx.Issue(t, "Graph request "+workflowTestSuffix(), testutil.Cols{
		"status": "todo", "assignee_type": "squad", "assignee_id": rig.squadID, "creator_type": "member", "creator_id": requesterID,
	})
	request, err := testHandler.EnsureFlowStarted(context.Background(), loadIssueRow(t, id))
	if err != nil {
		t.Fatal(err)
	}
	var leader string
	dbfx.QueryRow(t, `SELECT leader_id::text FROM squad WHERE id = $1`, rig.squadID).Scan(&leader)
	path := "/api/issues/" + id + "/flow/vars/"
	asMember := func(as func(*http.Request) *http.Request, key string, value any) *testutil.Response {
		return testutil.Call(t, testHandler.SetIssueFlowVar, as(withURLParams(newRequest("PUT", path+key, map[string]any{"value": value}), "id", id, "key", key)))
	}
	asLead := func(key string, value any) *testutil.Response {
		return agentOnIssue(t, id, leader, "plan", testHandler.SetIssueFlowVar, "PUT", path+key, map[string]any{"value": value}, "key", key)
	}

	// At the plan step the lead sets what it owns; the requester's variable is set later.
	wantFlowRejection(t, asMember(asRequester, "scope", "big"), 403, "flow_var_forbidden")
	wantFlowRejection(t, asMember(asRequester, "notes", "x"), 403, "flow_var_forbidden")
	wantFlowRejection(t, asLead("scope", "big"), 403, "flow_var_forbidden")
	wantFlowRejection(t, asLead("notes", "this note is longer than twenty characters"), 422, "flow_var_invalid")
	wantFlowRejection(t, asLead("nowhere", "x"), 422, "flow_var_invalid")
	asLead("notes", "go ahead").Want(200)

	// At the work step the requester sets theirs, and the lead has nothing left to set.
	rig.complete(request, nil)
	asMember(asRequester, "scope", "big").Want(200)
	wantFlowRejection(t, asLead("notes", "late"), 403, "flow_var_forbidden")
	wantFlowRejection(t, asMember(asRequester, "scope", "huge"), 422, "flow_var_invalid")
	if st := rig.state(request); st.Vars["scope"] != "big" || st.Vars["notes"] != "go ahead" {
		t.Fatalf("vars = %v", st.Vars)
	}
}

func TestVars_ManagerAnytimeIsLogged(t *testing.T) {
	rig := newGraphRigDoc(t, "vars", varsDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	id := uuidToString(request.ID)
	// "notes" belongs to the lead at the plan step; the workspace owner sets it at the work step.
	testutil.Call(t, testHandler.SetIssueFlowVar, withURLParams(newRequest("PUT", "/api/issues/"+id+"/flow/vars/notes", map[string]any{"value": "from the manager"}), "id", id, "key", "notes")).Want(200)
	details := activityDetails(t, id, "flow_var_set")
	if details["var"] != "notes" || details["step"] != "work" || details["by"] != testUserID {
		t.Fatalf("log details = %v", details)
	}
	// The old route is the same call.
	testutil.Call(t, testHandler.SetIssueFlowSwitch, withURLParams(newRequest("PUT", "/api/issues/"+id+"/flow/switches/notes", map[string]any{"value": "again"}), "id", id, "key", "notes")).Want(200)
	if st := rig.state(request); st.Vars["notes"] != "again" {
		t.Fatalf("vars = %v", st.Vars)
	}
}

// exitDoc has a manual step the counsel member leaves, by one of two exits.
func exitDoc() map[string]any {
	return baseDoc([]any{docRole("dev", "agent"), docRole("counsel", "member")},
		docLead("work"),
		docStep("work", []any{docTask("go", "work", "dev", nil)}, docEdge("w_ok", "sign", nil)),
		docManual("sign", "counsel", manualEdge("sign_go", "done"), manualEdge("sign_back", "work")))
}

func TestExits_ManualEdgeByExitRole(t *testing.T) {
	rig := newGraphRigDoc(t, "exits", exitDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "work", 0, always(""))
	st := rig.advance(request)
	if st.Step != "sign" || !st.Unreviewed {
		t.Fatalf("step %q unreviewed %v; want the manual step with the agent's work unreviewed", st.Step, st.Unreviewed)
	}
	counsel := rig.members["counsel"][0]
	id := uuidToString(request.ID)
	var done IssueFlowStateResponse
	testutil.Call(t, testHandler.ExitIssueFlow, rig.as[counsel](withURLParams(newRequest("POST", "/api/issues/"+id+"/flow/exits/sign_go", nil), "id", id, "edgeId", "sign_go"))).Want(200).JSON(&done)
	if len(done.Checkpoints) != 1 || done.Checkpoints[0].By != counsel || done.Checkpoints[0].Step != "sign" || done.Unreviewed {
		t.Fatalf("checkpoints %+v unreviewed %v; want the counsel's checkpoint and nothing unreviewed", done.Checkpoints, done.Unreviewed)
	}
	rig.wantClosedAt(request, "done")
}

func TestExits_ForbiddenForOthers(t *testing.T) {
	rig := newGraphRigDoc(t, "exits", exitDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	id := uuidToString(request.ID)
	exit := func(as func(*http.Request) *http.Request, edge string) *testutil.Response {
		req := withURLParams(newRequest("POST", "/api/issues/"+id+"/flow/exits/"+edge, nil), "id", id, "edgeId", edge)
		if as != nil {
			req = as(req)
		}
		return testutil.Call(t, testHandler.ExitIssueFlow, req)
	}
	// Not at the manual step yet.
	wantFlowRejection(t, exit(nil, "sign_go"), 409, "flow_wrong_step")
	rig.finishAll(request, "work", 0, always(""))
	rig.advance(request)

	_, asPlain := addWorkflowTestMember(t, "plain-exit")
	wantFlowRejection(t, exit(asPlain, "sign_go"), 403, "flow_exit_forbidden")
	wantFlowRejection(t, exit(rig.as[rig.members["counsel"][0]], "nowhere"), 409, "flow_edge_unavailable")
	// The owner manages the squad: they may choose, and the other exit goes back.
	var back IssueFlowStateResponse
	exit(nil, "sign_back").Want(200).JSON(&back)
	if back.Step != "work" {
		t.Fatalf("step %q after the exit back, want work", back.Step)
	}
}

func TestClose_OnBehalfLogsAndShowsInState(t *testing.T) {
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	gate := firstRow(t, rig, request, "review", "triage")
	_, assignee := rig.assigneeOf(gate)
	closeVia := func(as func(*http.Request) *http.Request, body map[string]any) *testutil.Response {
		req := withURLParam(newRequest("POST", "/api/issues/"+gate+"/flow/close", body), "id", gate)
		if as != nil {
			req = as(req)
		}
		return testutil.Call(t, testHandler.CloseIssueFlowTask, req)
	}
	_, asPlain := addWorkflowTestMember(t, "plain-close")
	wantFlowRejection(t, closeVia(asPlain, map[string]any{"verdict": "ship"}), 403, "flow_forbidden")
	wantFlowRejection(t, closeVia(nil, map[string]any{"verdict": "approved"}), 422, "flow_outcome_invalid")
	wantFlowRejection(t, closeVia(nil, map[string]any{}), 422, "flow_outcome_required")

	var out struct {
		Task  IssueFlowTaskResponse  `json:"task"`
		State IssueFlowStateResponse `json:"state"`
	}
	closeVia(nil, map[string]any{"verdict": "ship", "note": "Agent offline; checked by hand"}).Want(200).JSON(&out)
	if out.Task.ClosedBy == nil || out.Task.ClosedBy.Type != "member" || out.Task.ClosedBy.ID != testUserID || !out.Task.ClosedBy.OnBehalf {
		t.Fatalf("closed_by = %+v, want the manager on behalf", out.Task.ClosedBy)
	}
	if out.Task.Outcome == nil || *out.Task.Outcome != "ship" || out.Task.Class != issueworkflow.ClassPass {
		t.Fatalf("task outcome %v class %q, want ship and pass", out.Task.Outcome, out.Task.Class)
	}
	details := activityDetails(t, gate, "flow_manual_close")
	if details["verdict"] != "ship" || details["on_behalf_of"] != assignee || details["note"] != "Agent offline; checked by hand" || details["task"] != "triage" {
		t.Fatalf("log details = %v", details)
	}
	// The state read afterwards says the same.
	if got := rig.state(request).Tasks[0].ClosedBy; got == nil || !got.OnBehalf {
		t.Fatalf("state closed_by = %+v", got)
	}
}

func TestOutcome_SupersededIsEngineOnly(t *testing.T) {
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	gate := firstRow(t, rig, request, "review", "triage")
	checker := rig.agents["checker"][0]
	body := map[string]any{"status": "done", "outcome": "superseded"}
	// Neither the agent, nor a manager, nor a batch update writes it.
	wantFlowRejection(t, agentOnIssue(t, gate, checker, "review", testHandler.UpdateIssue, "PUT", "/api/issues/"+gate, body), 422, "flow_outcome_invalid")
	wantFlowRejection(t, updateIssueForTest(t, gate, body), 422, "flow_outcome_invalid")
	batch := testutil.Call(t, testHandler.BatchUpdateIssues, newRequest("POST", "/api/issues/batch-update", map[string]any{"issue_ids": []string{gate}, "updates": body}))
	wantFlowRejection(t, batch, 422, "flow_outcome_invalid")
	if status, _, _ := issueStatusAndAssignee(t, gate); status == "done" {
		t.Fatal("the task was closed with an outcome only the engine writes")
	}
	// The engine writes it: a quorum that is complete supersedes the reviews still open.
	legal := newGraphRig(t, "legal", graphRigOpts{})
	lr := legal.newRequest()
	legal.complete(lr, nil)
	legal.finishAll(lr, "intake", 0, always(""))
	legal.advance(lr)
	legal.finishAll(lr, "redraft_clauses", 0, always(""))
	legal.advance(lr)
	rows := legal.rows(lr, "review", 0)
	for i := 1; i <= 3; i++ {
		legal.finish(rows["reviewer~"+string(rune('0'+i))], "approve")
	}
	legal.advance(lr)
	if n := dbfx.Count(t, `SELECT count(*) FROM issue WHERE parent_issue_id = $1 AND outcome = 'superseded'`, lr.ID); n != 2 {
		t.Fatalf("%d rows superseded by the engine, want 2", n)
	}
}

// taskVarDoc has a task that sets a list, and a task that must set a number.
func taskVarDoc() map[string]any {
	doc := baseDoc([]any{docRole("planner", "agent"), docRole("other", "agent")},
		docLead("planning"),
		docStep("planning", []any{docTask("make_plan", "work", "planner", map[string]any{
			"sets": []any{"topics"}, "requires": map[string]any{"vars": []any{"topics"}}})},
			docEdge("p_ok", "done", nil)))
	doc["vars"] = []any{
		map[string]any{"key": "topics", "type": "list_string", "max_items": 3, "default": []any{}, "set_by": []any{"planner"}, "set_at": []any{"planning"}},
		map[string]any{"key": "note", "type": "string", "max_len": 40, "default": "", "set_by": []any{"managers"}, "set_at": []any{"*"}},
	}
	return doc
}

func putTaskVar(t *testing.T, task, who string, key string, value any) *testutil.Response {
	t.Helper()
	return agentOnIssue(t, task, who, "planning", testHandler.SetIssueFlowVar, "PUT", "/api/issues/"+task+"/flow/vars/"+key, map[string]any{"value": value}, "key", key)
}

func TestTaskVar_AssigneeSetsDeclaredVarWhileOpen(t *testing.T) {
	rig := newGraphRigDoc(t, "taskvar", taskVarDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	task := firstRow(t, rig, request, "planning", "make_plan")
	planner := rig.agents["planner"][0]
	var st IssueFlowStateResponse
	putTaskVar(t, task, planner, "topics", []string{"a", "b"}).Want(200).JSON(&st)
	if got, _ := st.Vars["topics"].([]any); len(got) != 2 {
		t.Fatalf("vars = %v, want the two topics", st.Vars)
	}
	var recorded int
	dbfx.QueryRow(t, `SELECT COALESCE((flow->'var_sets'->>'planning.make_plan.topics')::int, 0) FROM issue WHERE id = $1`, request.ID).Scan(&recorded)
	if recorded != 1 {
		t.Fatalf("var_sets records %d, want the activation 1", recorded)
	}
	details := activityDetails(t, uuidToString(request.ID), "flow_var_set")
	if details["var"] != "topics" || details["task"] != "make_plan" || details["by"] != planner {
		t.Fatalf("log details = %v", details)
	}
	// Too many items is a value error, not a permission one.
	wantFlowRejection(t, putTaskVar(t, task, planner, "topics", []string{"a", "b", "c", "d"}), 422, "flow_var_invalid")
}

func TestTaskVar_OtherVarOrClosedTaskForbidden(t *testing.T) {
	rig := newGraphRigDoc(t, "taskvar", taskVarDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	task := firstRow(t, rig, request, "planning", "make_plan")
	planner, other := rig.agents["planner"][0], rig.agents["other"][0]
	// A variable the task does not list, and someone who is not its assignee.
	wantFlowRejection(t, putTaskVar(t, task, planner, "note", "x"), 403, "flow_var_forbidden")
	wantFlowRejection(t, putTaskVar(t, task, other, "topics", []string{"a"}), 403, "flow_var_forbidden")
	putTaskVar(t, task, planner, "topics", []string{"a"}).Want(200)
	attachAs(t, task, planner)
	agentOnIssue(t, task, planner, "planning", testHandler.UpdateIssue, "PUT", "/api/issues/"+task, map[string]any{"status": "done"}).Want(200)
	// Closed: the task no longer sets anything.
	wantFlowRejection(t, putTaskVar(t, task, planner, "topics", []string{"b"}), 403, "flow_var_forbidden")
}

func TestTaskVar_RequiredBeforeClose(t *testing.T) {
	rig := newGraphRigDoc(t, "taskvar", taskVarDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	task := firstRow(t, rig, request, "planning", "make_plan")
	planner := rig.agents["planner"][0]
	closeIt := func() *testutil.Response {
		return agentOnIssue(t, task, planner, "planning", testHandler.UpdateIssue, "PUT", "/api/issues/"+task, map[string]any{"status": "done"})
	}
	body := wantFlowRejection(t, closeIt(), 422, "flow_vars_required")
	if vars, _ := body["vars"].([]any); len(vars) != 1 || vars[0] != "topics" {
		t.Fatalf("vars = %v, want topics", body["vars"])
	}
	// A manager sets it on the planner's behalf (the request route), which is not the task setting it.
	testutil.Call(t, testHandler.SetIssueFlowVar, withURLParams(newRequest("PUT", "/api/issues/"+uuidToString(request.ID)+"/flow/vars/topics", map[string]any{"value": []string{"z"}}),
		"id", uuidToString(request.ID), "key", "topics")).Want(200)
	wantFlowRejection(t, closeIt(), 422, "flow_vars_required")
	putTaskVar(t, task, planner, "topics", []string{"a"}).Want(200)
	closeIt().Want(200)
}

func TestTaskVar_FrozenWhileFanOutRuns(t *testing.T) {
	rig := newGraphRig(t, "planner_fanout", graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	plan := firstRow(t, rig, request, "planning", "make_plan")
	planner := rig.agents["planner"][0]
	agentOnIssue(t, plan, planner, "planning", testHandler.SetIssueFlowVar, "PUT", "/api/issues/"+plan+"/flow/vars/topics", map[string]any{"value": []string{"a", "b"}}, "key", "topics").Want(200)
	attachAs(t, plan, planner)
	agentOnIssue(t, plan, planner, "planning", testHandler.UpdateIssue, "PUT", "/api/issues/"+plan, map[string]any{"status": "done"}).Want(200)
	endRuns(t, plan)
	rig.advance(request)
	if got := rig.rows(request, "fanout", 0); len(got) != 2 {
		t.Fatalf("fan-out rows = %v, want 2", got)
	}
	id := uuidToString(request.ID)
	frozen := testutil.Call(t, testHandler.SetIssueFlowVar, withURLParams(newRequest("PUT", "/api/issues/"+id+"/flow/vars/topics", map[string]any{"value": []string{"a", "b", "c"}}), "id", id, "key", "topics"))
	body := wantFlowRejection(t, frozen, 409, "flow_var_in_use")
	if body["task"] != "research_topic" {
		t.Fatalf("body = %v", body)
	}
	if st := rig.state(request); len(st.Vars["topics"].([]any)) != 2 {
		t.Fatalf("topics = %v, want the list unchanged", st.Vars["topics"])
	}
}

func TestStatePayload_AddsFieldsOnly(t *testing.T) {
	// A request of schema 1: every field of the old answer is there with its old type.
	f := newRehearsalFixture(t)
	request := f.newRequest(t)
	f.plan(t, request, nil)
	id := uuidToString(request.ID)
	var raw map[string]any
	testutil.Call(t, testHandler.GetIssueFlow, withURLParam(newRequest("GET", "/api/issues/"+id+"/flow", nil), "id", id)).Want(200).JSON(&raw)
	for _, key := range []string{"managed", "role", "workflow_id", "workflow_name", "step", "rework", "stop", "switches", "auto", "entered_at", "tasks", "next", "viewer", "rework_step"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("the answer lost %q", key)
		}
	}
	// Spec 7.1: stop is null or an object {code, legacy_no}. An older client reads any other value as "stalled".
	if obj, isObject := raw["stop"].(map[string]any); raw["stop"] != nil && !isObject {
		t.Errorf("stop = %v, want null or an object", raw["stop"])
	} else if isObject {
		t.Errorf("a request that is not parked has stop %v", obj)
	}
	for _, key := range []string{"schema", "loops", "activations", "vars", "facts", "checkpoints", "unreviewed", "trail", "trial"} {
		if _, ok := raw[key]; !ok {
			t.Errorf("the answer has no %q", key)
		}
	}
	if raw["schema"].(float64) != 1 || len(raw["trail"].([]any)) != 1 {
		t.Errorf("schema %v trail %v; want schema 1 and the plan hop", raw["schema"], raw["trail"])
	}
	task := raw["tasks"].([]any)[0].(map[string]any)
	for _, key := range []string{"issue_id", "identifier", "step", "task", "round", "kind", "status", "outcome", "assignee_id", "closed_at", "closed_by", "instance", "class", "slot_status", "inputs", "feedback", "sets"} {
		if _, ok := task[key]; !ok {
			t.Errorf("a task of the answer has no %q", key)
		}
	}

	// A request of schema 2 parked at a stop: the object carries the code and the number schema 1 gave it.
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	second := rig.newRequest()
	rig.complete(second, nil)
	rig.finishAll(second, "review", 0, always("fix"))
	rig.advance(second)
	var v2 map[string]any
	sid := uuidToString(second.ID)
	testutil.Call(t, testHandler.GetIssueFlow, withURLParam(newRequest("GET", "/api/issues/"+sid+"/flow", nil), "id", sid)).Want(200).JSON(&v2)
	stop, _ := v2["stop"].(map[string]any)
	if stop["code"] != "loop_exhausted" || stop["legacy_no"] != float64(3) || v2["schema"] != float64(2) {
		t.Fatalf("stop %v schema %v; want the stop loop_exhausted (legacy 3) on schema 2", v2["stop"], v2["schema"])
	}
	if _, old := v2["stop_info"]; old {
		t.Fatalf("the answer still carries stop_info: %v", v2["stop_info"])
	}
}

func TestBrief_ListsInputsFeedbackSkillsAndVarCommands(t *testing.T) {
	doc := baseDoc([]any{docRole("dev", "agent"), docRole("checker", "agent")},
		docLead("build"),
		docStep("build", []any{docTask("make", "work", "dev", map[string]any{
			"skills": []any{"pdf-reader"}, "sets": []any{"score"},
			"requires": map[string]any{"files": []any{map[string]any{"slot": "report", "min": 1, "ext": []any{".md"}}}},
			"verdicts": "made",
		})}, docEdge("b_ok", "check", nil)),
		docStep("check", []any{docTask("verify", "gate", "checker", map[string]any{"inputs": []any{map[string]any{"from": "build.make", "slot": "report"}}})},
			docEdge("c_ok", "done", joinIs("pass")),
			docEdge("c_redo", "build", map[string]any{"all": []any{joinIs("fail"), map[string]any{"field": "loop.redo.count", "op": "lt", "value": 1}}}),
			docStopEdge("c_stop", "loop_exhausted", nil)))
	doc["steps"].([]any)[2].(map[string]any)["edges"].([]any)[1].(map[string]any)["loop"] = "redo"
	doc["loops"] = []any{map[string]any{"key": "redo", "header": "build", "max": 1}}
	doc["vars"] = []any{map[string]any{"key": "score", "type": "int", "min": 0, "max": 10, "default": 0, "set_by": []any{"dev"}, "set_at": []any{"build"}}}
	doc["verdicts"] = []any{map[string]any{"key": "made", "verdicts": []any{map[string]any{"key": "made", "class": "pass"}, map[string]any{"key": "partial", "class": "neutral"}}}}
	rig := newGraphRigDoc(t, "brief", doc, graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)

	make1 := firstRow(t, rig, request, "build", "make")
	brief := testHandler.claimFlowBrief(context.Background(), loadIssueRow(t, make1))
	if brief == nil {
		t.Fatal("no brief for a task of a schema 2 flow")
	}
	for _, want := range []string{
		"--outcome made", "--outcome partial", // the keys of its own set
		"slot `report`: at least 1, extension .md", // the file it must attach
		"multica issue flow set", " score ",        // the command for the variable it sets
		"pdf-reader", // the skill it uses
		"multica issue status",
	} {
		if !containsAll(brief.Brief, want) {
			t.Errorf("the brief of the task lacks %q:\n%s", want, brief.Brief)
		}
	}
	if containsAll(brief.Brief, "--outcome approved") {
		t.Errorf("the brief names a verdict the task does not have:\n%s", brief.Brief)
	}

	// The gate names its input, and after a rejection the task answers it.
	attachFile(t, make1, "agent", rig.agents["dev"][0], "report.md", "text/markdown")
	rig.finish(make1, "made")
	rig.advance(request)
	verify := firstRow(t, rig, request, "check", "verify")
	gateBrief := testHandler.claimFlowBrief(context.Background(), loadIssueRow(t, verify))
	if !containsAll(gateBrief.Brief, "· make", "slot report", "--outcome approved", "--outcome rejected") {
		t.Errorf("the brief of the gate lacks its input or its verdicts:\n%s", gateBrief.Brief)
	}
	rig.finish(verify, "rejected")
	rig.advance(request)
	redo := rig.rows(request, "build", 1)["make"]
	if redo == "" {
		t.Fatalf("the rework did not open: %v", rig.rows(request, "build", 1))
	}
	if got := testHandler.claimFlowBrief(context.Background(), loadIssueRow(t, redo)).Brief; !containsAll(got, "round 2", "· verify") {
		t.Errorf("the brief of the rework does not name the gate that rejected:\n%s", got)
	}
	if !containsAll(taskDescription(t, redo), "Feedback", "<id>") {
		t.Errorf("the description of the rework lacks the feedback:\n%s", taskDescription(t, redo))
	}
}

func TestRelease_UnreviewedStallsThenManualExitReleases(t *testing.T) {
	rig := newGraphRig(t, "trade", graphRigOpts{Mutate: func(doc map[string]any) {
		doc["policy"] = map[string]any{}
		for _, step := range asMaps(doc["steps"]) {
			if step["key"] != "handover" {
				continue
			}
			for _, edge := range asMaps(step["edges"]) {
				if edge["id"] == "handover_ok" {
					edge["to"] = "done"
				}
			}
		}
	}})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "collect", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "draft", 0, always(""))
	rig.advance(request)
	rig.finishAll(request, "gate", 0, always("approved"))
	rig.advance(request)
	rig.finishAll(request, "handover", 0, always("approved"))
	st := rig.advance(request)
	if st.Stop == nil || !st.Unreviewed || st.Step != "handover" {
		t.Fatalf("stop %+v unreviewed %v step %q; want a stall at the handover with work nobody saw", st.Stop, st.Unreviewed, st.Step)
	}
	// A resume that does not say so cannot stand in for the check nobody made (spec 4.7, 4.8).
	id := uuidToString(request.ID)
	refused := testutil.Call(t, testHandler.ResumeIssueFlow, withURLParam(newRequest("POST", "/api/issues/"+id+"/flow/resume", map[string]any{"to_step": "release"}), "id", id))
	body := wantFlowRejection(t, refused, 422, "flow_checkpoint_ack_required")
	if body["step"] != "release" {
		t.Fatalf("body = %v, want the step the resume was for", body)
	}
	if st := rig.state(request); st.Stop == nil || !st.Unreviewed || st.Step != "handover" || len(st.Checkpoints) != 0 {
		t.Fatalf("a refused resume changed the request: step %q stop %+v unreviewed %v checkpoints %v", st.Step, st.Stop, st.Unreviewed, st.Checkpoints)
	}
	// A manager who says they know takes the place of the checkpoint: it is recorded, and the request moves on.
	var resumed IssueFlowStateResponse
	testutil.Call(t, testHandler.ResumeIssueFlow, withURLParam(newRequest("POST", "/api/issues/"+id+"/flow/resume", map[string]any{"to_step": "release", "acknowledge_skip_checkpoint": true}), "id", id)).Want(200).JSON(&resumed)
	if resumed.Step != "release" || resumed.Unreviewed || resumed.Stop != nil {
		t.Fatalf("after the resume: step %q unreviewed %v stop %+v", resumed.Step, resumed.Unreviewed, resumed.Stop)
	}
	if n := len(resumed.Checkpoints); n != 1 || resumed.Checkpoints[0].Step != "release" || resumed.Checkpoints[0].By != testUserID {
		t.Fatalf("checkpoints = %+v, want one at release by the manager %s", resumed.Checkpoints, testUserID)
	}
	rig.mustExit(request, "release_done", memberActor(t, testUserID))
	rig.wantClosedAt(request, "done")
}

func TestOpen_GateSkipsWorkerInPool(t *testing.T) {
	doc := baseDoc([]any{docRole("staff", "agent")},
		docLead("write"),
		docStep("write", []any{docTask("draft", "work", "staff", nil)}, docEdge("w_ok", "check", nil)),
		docStep("check", []any{docTask("judge", "gate", "staff", map[string]any{"inputs": []any{map[string]any{"from": "write.draft"}}})}, docEdge("c_ok", "done", nil)))
	rig := newGraphRigDoc(t, "pool", doc, graphRigOpts{Pool: map[string]int{"staff": 2}})
	request := rig.newRequest()
	rig.complete(request, nil)
	writer := func() string { _, who := rig.assigneeOf(firstRow(t, rig, request, "write", "draft")); return who }()
	rig.finishAll(request, "write", 0, always(""))
	if st := rig.advance(request); st.Stop != nil {
		t.Fatalf("stopped: %+v %q", st.Stop, lastComment(t, request))
	}
	if _, judge := rig.assigneeOf(firstRow(t, rig, request, "check", "judge")); judge == writer || judge == "" {
		t.Fatalf("the gate went to %q, who wrote the draft (%q); want the other one in the pool", judge, writer)
	}
}

func TestOpen_RequiredInputMissingStalls(t *testing.T) {
	rig := newGraphRigDoc(t, "required", branchDoc(true), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, map[string]any{"branch": "a"})
	rig.finishAll(request, "work_a", 0, always(""))
	st := rig.advance(request)
	if st.Stop == nil || st.Stop.Code != issueworkflow.DocStopStalled {
		t.Fatalf("stop %+v, want stalled", st.Stop)
	}
	if n := len(rig.rows(request, "check", 0)); n != 0 {
		t.Fatalf("%d tasks opened without their required input", n)
	}
	if !containsAll(lastComment(t, request), "needs the output of a step the request has not run") {
		t.Fatalf("stall comment = %q", lastComment(t, request))
	}
}

var _ = json.Marshal

// Only a stall caused by unseen agent work needs the acknowledgement: any other resume goes on as before.
func TestResume_AckIsNotAskedWhenNothingIsUnseen(t *testing.T) {
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	rig.finishAll(request, "review", 0, always("fix"))
	st := rig.advance(request)
	if st.Stop == nil || st.Stop.Code != "loop_exhausted" {
		t.Fatalf("stop %+v, want loop_exhausted", st.Stop)
	}
	id := uuidToString(request.ID)
	testutil.Call(t, testHandler.ResumeIssueFlow, withURLParam(newRequest("POST", "/api/issues/"+id+"/flow/resume", map[string]any{}), "id", id)).Want(200)
}

// Decision of the integration round: the state tells each task which verdicts it closes with, so a
// screen offers the buttons of the task's own set and not two fixed ones.
func TestStatePayload_TaskCarriesItsVerdictSet(t *testing.T) {
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	state := rig.state(request)
	var gate *IssueFlowTaskResponse
	for i := range state.Tasks {
		if state.Tasks[i].Task == "triage" {
			gate = &state.Tasks[i]
		}
	}
	if gate == nil {
		t.Fatalf("no triage task in %+v", state.Tasks)
	}
	want := []IssueFlowVerdictResponse{{Key: "ship", Class: "pass"}, {Key: "fix", Class: "fail"}, {Key: "defer", Class: "neutral"}}
	if len(gate.Verdicts) != len(want) {
		t.Fatalf("verdicts = %+v, want %+v", gate.Verdicts, want)
	}
	for i := range want {
		if gate.Verdicts[i] != want[i] {
			t.Fatalf("verdicts = %+v, want %+v", gate.Verdicts, want)
		}
	}

	// A task that names no set has the built-in one of its kind: done for work, approved or rejected for a gate.
	plain := newGraphRigDoc(t, "members", memberDoc(), graphRigOpts{})
	req2 := plain.newRequest()
	plain.complete(req2, nil)
	work := plain.state(req2).Tasks[0]
	if len(work.Verdicts) != 1 || work.Verdicts[0] != (IssueFlowVerdictResponse{Key: "done", Class: "pass"}) {
		t.Fatalf("work verdicts = %+v, want done", work.Verdicts)
	}
	builtin := newGraphRigDoc(t, "builtin-gate", baseDoc([]any{docRole("checker", "agent")}, docLead("review"),
		docStep("review", []any{docTask("check", "gate", "checker", nil)}, docEdge("r_ok", "done", joinIs("pass")), docStopEdge("r_no", "loop_exhausted", nil))), graphRigOpts{})
	req3 := builtin.newRequest()
	builtin.complete(req3, nil)
	got := builtin.state(req3).Tasks[0].Verdicts
	if len(got) != 2 || got[0].Key != "approved" || got[1].Key != "rejected" || got[1].Class != "fail" {
		t.Fatalf("gate verdicts = %+v, want approved and rejected", got)
	}
}

// A manager closes a gate for its agent through the close route with a verdict of the task's set; it is logged.
func TestCloseRoute_ManagerClosesOnBehalfWithACustomVerdictAndIsLogged(t *testing.T) {
	rig := newGraphRigDoc(t, "triage", triageDoc(), graphRigOpts{})
	request := rig.newRequest()
	rig.complete(request, nil)
	gate := firstRow(t, rig, request, "review", "triage")

	// A verdict the task's set does not have is refused, with the keys it has.
	wrong := testutil.Call(t, testHandler.CloseIssueFlowTask, withURLParam(newRequest("POST", "/api/issues/"+gate+"/flow/close", map[string]any{"verdict": "approved"}), "id", gate))
	wantFlowRejection(t, wrong, 422, "flow_outcome_invalid")

	var out struct {
		Task  IssueFlowTaskResponse  `json:"task"`
		State IssueFlowStateResponse `json:"state"`
	}
	testutil.Call(t, testHandler.CloseIssueFlowTask, withURLParam(newRequest("POST", "/api/issues/"+gate+"/flow/close", map[string]any{"verdict": "ship", "note": "agent offline"}), "id", gate)).Want(200).JSON(&out)
	if got := issueOutcome(t, gate); got != "ship" {
		t.Fatalf("outcome = %q, want ship", got)
	}
	details := activityDetails(t, gate, "flow_manual_close")
	if details["verdict"] != "ship" || details["task"] != "triage" {
		t.Fatalf("details = %v, want the verdict ship on task triage", details)
	}
	if out.Task.ClosedBy == nil || !out.Task.ClosedBy.OnBehalf {
		t.Fatalf("closed_by = %+v, want a close on behalf", out.Task.ClosedBy)
	}
}
