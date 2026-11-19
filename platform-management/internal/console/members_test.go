package console

import (
	"context"
	"errors"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/memberrequest"
)

const (
	memReqID      = "dddddddd-0000-0000-0000-0000000000a1"
	foreignReqID  = "dddddddd-0000-0000-0000-0000000000a2"
	foreignWSID   = "bbbbbbbb-0000-0000-0000-0000000000f1"
	memberReqBody = `{"email":"New@Example.com","role":"member"}`
)

// fakeMembers is the MemberRequests dependency, recording what the handler
// passes on.
type fakeMembers struct {
	rows    map[string]memberrequest.Request
	created bool
	err     error

	creates []fakeMemberCreate
	cancels []string
	listed  []string
}

type fakeMemberCreate struct {
	by          memberrequest.Actor
	workspaceID string
	in          memberrequest.Input
}

func newFakeMembers() *fakeMembers {
	return &fakeMembers{
		created: true,
		rows: map[string]memberrequest.Request{
			memReqID: {
				ID: memReqID, WorkspaceID: wsID, Kind: memberrequest.KindAdd, Email: "old@example.com", Role: "member",
				State: memberrequest.StatePending, RequestedBy: adminID, RequestedByLabel: adminMail, CreatedAt: t0, UpdatedAt: t0,
			},
			foreignReqID: {
				ID: foreignReqID, WorkspaceID: foreignWSID, Kind: memberrequest.KindAdd, Email: "far@example.com", Role: "viewer",
				State: memberrequest.StatePending, RequestedBy: adminID, RequestedByLabel: adminMail, CreatedAt: t0, UpdatedAt: t0,
			},
		},
	}
}

func (f *fakeMembers) Create(_ context.Context, by memberrequest.Actor, ws string, in memberrequest.Input) (memberrequest.Request, bool, error) {
	f.creates = append(f.creates, fakeMemberCreate{by, ws, in})
	if f.err != nil {
		return memberrequest.Request{}, false, f.err
	}
	r := f.rows[memReqID]
	r.WorkspaceID, r.Email, r.Role = ws, strings.ToLower(in.Email), in.Role
	return r, f.created, nil
}

func (f *fakeMembers) Cancel(_ context.Context, _ memberrequest.Actor, id string) (memberrequest.Request, error) {
	f.cancels = append(f.cancels, id)
	if f.err != nil {
		return memberrequest.Request{}, f.err
	}
	r := f.rows[id]
	r.State = memberrequest.StateCanceled
	return r, nil
}

func (f *fakeMembers) List(_ context.Context, ws string) ([]memberrequest.Request, error) {
	f.listed = append(f.listed, ws)
	if f.err != nil {
		return nil, f.err
	}
	var out []memberrequest.Request
	for _, r := range f.rows {
		if r.WorkspaceID == ws {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeMembers) Get(_ context.Context, id string) (memberrequest.Request, bool, error) {
	r, ok := f.rows[id]
	return r, ok, nil
}

func memberEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.reader.wsTenant[foreignWSID] = otherTenantID
	return e
}

func TestMemberRequestCreateAnswers202WithTheRequestAndPassesTheActor(t *testing.T) {
	e := memberEnv(t)
	rec := e.do(tadminID, "POST", "/api/platform/workspaces/"+wsID+"/members", memberReqBody)
	wantStatus(t, rec, http.StatusAccepted)
	body := decode(t, rec)
	if body["id"] != memReqID || body["state"] != "pending" || body["email"] != "new@example.com" || body["role"] != "member" {
		t.Errorf("body = %s", rec.Body.String())
	}
	if len(e.member.creates) != 1 {
		t.Fatalf("creates = %d", len(e.member.creates))
	}
	c := e.member.creates[0]
	if c.by.ID != tadminID || c.by.Label != tadminMail || c.workspaceID != wsID || c.in.Email != "New@Example.com" || c.in.Role != "member" {
		t.Errorf("service got %+v", c)
	}
}

func TestMemberRequestResendAnswers200(t *testing.T) {
	e := memberEnv(t)
	e.member.created = false
	wantStatus(t, e.do(adminID, "POST", "/api/platform/workspaces/"+wsID+"/members", memberReqBody), http.StatusOK)
}

func TestMemberRequestRefusalsCarryTheirCodeAndExtras(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
		extra  map[string]any
	}{
		{"invalid", &memberrequest.Error{Status: 400, Code: memberrequest.CodeInvalid, Message: "bad", Fields: []string{"email"}}, 400, "member_request_invalid", map[string]any{"fields": []any{"email"}}},
		{"role", &memberrequest.Error{Status: 400, Code: memberrequest.CodeRoleInvalid, Message: "bad role"}, 400, "member_role_invalid", nil},
		{"not active", &memberrequest.Error{Status: 409, Code: memberrequest.CodeWorkspaceNotActive, Message: "no", State: "suspended"}, 409, "workspace_not_active", map[string]any{"status": "suspended"}},
		{"staff", &memberrequest.Error{Status: 409, Code: memberrequest.CodeStaffCannotBeAdded, Message: "no"}, 409, "staff_cannot_be_added", nil},
		{"member", &memberrequest.Error{Status: 409, Code: memberrequest.CodeAlreadyMember, Message: "no"}, 409, "already_member", nil},
		{"missing workspace", memberrequest.ErrNotFound, 404, "not_found", nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := memberEnv(t)
			e.member.err = tc.err
			rec := e.do(adminID, "POST", "/api/platform/workspaces/"+wsID+"/members", memberReqBody)
			wantCode(t, rec, tc.status, tc.code)
			body := decode(t, rec)
			for k, v := range tc.extra {
				if got := body[k]; !equalJSON(got, v) {
					t.Errorf("%s = %v, want %v", k, got, v)
				}
			}
		})
	}
}

func equalJSON(a, b any) bool {
	as, aok := a.([]any)
	bs, bok := b.([]any)
	if aok && bok {
		if len(as) != len(bs) {
			return false
		}
		for i := range as {
			if as[i] != bs[i] {
				return false
			}
		}
		return true
	}
	return a == b
}

func TestMemberRequestInternalErrorIsHiddenBehind500(t *testing.T) {
	e := memberEnv(t)
	e.member.err = errors.New("db password is hunter2")
	rec := e.do(adminID, "POST", "/api/platform/workspaces/"+wsID+"/members", memberReqBody)
	wantCode(t, rec, 500, "internal")
	if strings.Contains(rec.Body.String(), "hunter2") {
		t.Errorf("body leaks the cause: %s", rec.Body.String())
	}
}

func TestMemberRequestUnreadableBodyIsRefusedBeforeTheService(t *testing.T) {
	e := memberEnv(t)
	for _, body := range []string{`{"email":"a@example.com","role":"member","extra":1}`, `not json`} {
		rec := e.do(adminID, "POST", "/api/platform/workspaces/"+wsID+"/members", body)
		wantStatus(t, rec, 400)
	}
	if len(e.member.creates) != 0 {
		t.Errorf("service reached with an unreadable body")
	}
}

func TestMemberRequestListGetAndCancel(t *testing.T) {
	e := memberEnv(t)
	rec := e.do(tadminID, "GET", "/api/platform/workspaces/"+wsID+"/member-requests", "")
	wantStatus(t, rec, 200)
	list, _ := decode(t, rec)["requests"].([]any)
	if len(list) != 1 || list[0].(map[string]any)["id"] != memReqID {
		t.Errorf("list = %s", rec.Body.String())
	}
	rec = e.do(tadminID, "GET", "/api/platform/member-requests/"+memReqID, "")
	wantStatus(t, rec, 200)
	if b := decode(t, rec); b["error_code"] != "" || b["state"] != "pending" || b["workspace_id"] != wsID {
		t.Errorf("get = %s", rec.Body.String())
	}
	rec = e.do(tadminID, "POST", "/api/platform/member-requests/"+memReqID+"/cancel", "")
	wantStatus(t, rec, 200)
	if decode(t, rec)["state"] != "canceled" || len(e.member.cancels) != 1 {
		t.Errorf("cancel = %s, cancels %v", rec.Body.String(), e.member.cancels)
	}
}

func TestMemberRequestNotCancelableCarriesState(t *testing.T) {
	e := memberEnv(t)
	e.member.err = &memberrequest.Error{Status: 409, Code: memberrequest.CodeNotCancelable, Message: "no", State: "completed"}
	rec := e.do(adminID, "POST", "/api/platform/member-requests/"+memReqID+"/cancel", "")
	wantCode(t, rec, 409, "member_request_not_cancelable")
	if decode(t, rec)["state"] != "completed" {
		t.Errorf("body = %s", rec.Body.String())
	}
}

// An admin outside the tenant gets the 404 of a missing id, before the service
// is reached, on all four routes.
func TestMemberRequestOutOfScopeAdminNeverReachesTheService(t *testing.T) {
	e := memberEnv(t)
	const missing = "99999999-0000-4000-8000-000000000003"
	for _, c := range []struct{ method, foreign, missing, body string }{
		{"POST", "/workspaces/" + foreignWSID + "/members", "/workspaces/" + missing + "/members", memberReqBody},
		{"POST", "/workspaces/" + foreignWSID + "/members", "/workspaces/" + missing + "/members", `not json`},
		{"GET", "/workspaces/" + foreignWSID + "/member-requests", "/workspaces/" + missing + "/member-requests", ""},
		{"GET", "/member-requests/" + foreignReqID, "/member-requests/" + missing, ""},
		{"POST", "/member-requests/" + foreignReqID + "/cancel", "/member-requests/" + missing + "/cancel", ""},
	} {
		a := e.do(tadminID, c.method, "/api/platform"+c.foreign, c.body)
		b := e.do(tadminID, c.method, "/api/platform"+c.missing, c.body)
		if a.Code != 404 || a.Body.String() != b.Body.String() {
			t.Errorf("%s %s: foreign = %d %q, missing = %d %q", c.method, c.foreign, a.Code, a.Body.String(), b.Code, b.Body.String())
		}
	}
	if len(e.member.creates)+len(e.member.cancels)+len(e.member.listed) != 0 {
		t.Errorf("the service was reached: %+v %v %v", e.member.creates, e.member.cancels, e.member.listed)
	}
}

func TestMemberRequestWriteRoutesNeedTheConsoleOrigin(t *testing.T) {
	e := memberEnv(t)
	for _, path := range []string{"/api/platform/workspaces/" + wsID + "/members", "/api/platform/member-requests/" + memReqID + "/cancel"} {
		if rec := e.doWith(adminID, "POST", path, memberReqBody, "https://evil.example.com"); rec.Code == 200 || rec.Code == 202 {
			t.Errorf("POST %s from a foreign origin = %d", path, rec.Code)
		}
	}
	if len(e.member.creates)+len(e.member.cancels) != 0 {
		t.Error("a request from a foreign origin reached the service")
	}
}

func TestMemberRequestRoutesPassConsoleIsolationScan(t *testing.T) {
	for _, name := range []string{"members.go", "../../queries/member_request.sql"} {
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		var hits []string
		if strings.HasSuffix(name, ".sql") {
			hits = scanSQL(string(src))
		} else {
			hits = scanGo(t, name, src)
		}
		if len(hits) > 0 {
			t.Errorf("%s names a content table: %v", name, hits)
		}
	}
}
