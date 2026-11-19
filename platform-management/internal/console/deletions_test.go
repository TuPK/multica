package console

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/deletionrequest"
)

const (
	delReqID      = "dddddddd-0000-0000-0000-0000000000d1"
	delForeignID  = "dddddddd-0000-0000-0000-0000000000d2"
	delByOwnAdmin = "dddddddd-0000-0000-0000-0000000000d3"
)

// fakeDeletions is the DeletionRequests dependency, recording what the handler
// passes on.
type fakeDeletions struct {
	rows       map[string]deletionrequest.Request
	created    bool
	err        error
	workspaces []fakeDelWorkspace
	tenants    []fakeDelTenant
	acts       []string
	filters    []deletionrequest.Filter
	accounts   []fakeDelAccount
	preview    deletionrequest.AccountPreview
}

type fakeDelAccount struct {
	by   deletionrequest.Actor
	user string
	in   deletionrequest.AccountInput
}

// RequestAccount and PreviewAccount record the call; the answers are the fake's fields.
func (f *fakeDeletions) RequestAccount(_ context.Context, by deletionrequest.Actor, user string, in deletionrequest.AccountInput) (deletionrequest.Request, bool, error) {
	f.accounts = append(f.accounts, fakeDelAccount{by: by, user: user, in: in})
	if f.err != nil {
		return deletionrequest.Request{}, false, f.err
	}
	return f.rows[delReqID], f.created, nil
}

func (f *fakeDeletions) PreviewAccount(_ context.Context, _ deletionrequest.Actor, _ string) (deletionrequest.AccountPreview, error) {
	if f.err != nil {
		return deletionrequest.AccountPreview{}, f.err
	}
	return f.preview, nil
}

type fakeDelWorkspace struct {
	by deletionrequest.Actor
	ws string
	in deletionrequest.Input
}

type fakeDelTenant struct {
	by     deletionrequest.Actor
	tenant string
	in     deletionrequest.Input
}

func newFakeDeletions() *fakeDeletions {
	return &fakeDeletions{
		created: true,
		rows: map[string]deletionrequest.Request{
			delReqID: {
				ID: delReqID, Kind: deletionrequest.KindWorkspace, TargetID: wsID, TargetLabel: "acme", Reason: "r",
				State: deletionrequest.StatePending, TenantID: tenantID, RequestedBy: tadminID, RequestedByLabel: tadminMail,
				CreatedAt: t0, UpdatedAt: t0,
			},
			delForeignID: {
				ID: delForeignID, Kind: deletionrequest.KindWorkspace, TargetID: foreignWSID, TargetLabel: "far", Reason: "r",
				State: deletionrequest.StatePending, TenantID: otherTenantID, RequestedBy: adminID, RequestedByLabel: adminMail,
				CreatedAt: t0, UpdatedAt: t0,
			},
			delByOwnAdmin: {
				ID: delByOwnAdmin, Kind: deletionrequest.KindWorkspace, TargetID: wsID, TargetLabel: "acme", Reason: "r",
				State: deletionrequest.StateFailed, TenantID: tenantID, RequestedBy: adminID, RequestedByLabel: adminMail,
				CreatedAt: t0, UpdatedAt: t0,
			},
		},
	}
}

func (f *fakeDeletions) RequestWorkspace(_ context.Context, by deletionrequest.Actor, ws string, in deletionrequest.Input) (deletionrequest.Request, bool, error) {
	f.workspaces = append(f.workspaces, fakeDelWorkspace{by, ws, in})
	if f.err != nil {
		return deletionrequest.Request{}, false, f.err
	}
	return f.rows[delReqID], f.created, nil
}

func (f *fakeDeletions) RequestTenant(_ context.Context, by deletionrequest.Actor, tenant string, in deletionrequest.Input) (deletionrequest.Request, bool, error) {
	f.tenants = append(f.tenants, fakeDelTenant{by, tenant, in})
	if f.err != nil {
		return deletionrequest.Request{}, false, f.err
	}
	r := f.rows[delReqID]
	r.Kind, r.TargetID = deletionrequest.KindTenant, tenant
	return r, f.created, nil
}

func (f *fakeDeletions) Cancel(_ context.Context, _ deletionrequest.Actor, id string) (deletionrequest.Request, error) {
	f.acts = append(f.acts, "cancel:"+id)
	if f.err != nil {
		return deletionrequest.Request{}, f.err
	}
	r := f.rows[id]
	r.State = deletionrequest.StateCanceled
	return r, nil
}

func (f *fakeDeletions) Retry(_ context.Context, _ deletionrequest.Actor, id string) (deletionrequest.Request, error) {
	f.acts = append(f.acts, "retry:"+id)
	if f.err != nil {
		return deletionrequest.Request{}, f.err
	}
	r := f.rows[id]
	r.State, r.Attempts = deletionrequest.StatePending, 0
	return r, nil
}

func (f *fakeDeletions) List(_ context.Context, flt deletionrequest.Filter) ([]deletionrequest.Request, error) {
	f.filters = append(f.filters, flt)
	var out []deletionrequest.Request
	for _, r := range f.rows {
		if flt.TenantID == "" || r.TenantID == flt.TenantID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (f *fakeDeletions) Get(_ context.Context, id string) (deletionrequest.Request, bool, error) {
	r, ok := f.rows[id]
	return r, ok, nil
}

func newDeletionEnv(t *testing.T) *env {
	t.Helper()
	e := newEnv(t)
	e.reader.wsTenant[foreignWSID] = otherTenantID
	return e
}

const delBody = `{"reason":"customer closed the account","confirm":"acme"}`

func TestDeleteNowAdminInOwnTenantIsAccepted(t *testing.T) {
	e := newDeletionEnv(t)
	rec := e.do(tadminID, "POST", "/api/platform/workspaces/"+wsID+"/delete-now", delBody)
	wantStatus(t, rec, http.StatusAccepted)
	body := decode(t, rec)
	if body["request_id"] != delReqID || body["state"] != "pending" {
		t.Errorf("body = %s", rec.Body.String())
	}
	if len(e.delete.workspaces) != 1 || e.delete.workspaces[0].by.ID != tadminID || e.delete.workspaces[0].in.Confirm != "acme" {
		t.Errorf("service got %+v", e.delete.workspaces)
	}
}

func TestDeleteNowAdminOutsideTenantIsNotFound(t *testing.T) {
	e := newDeletionEnv(t)
	wantCode(t, e.do(tadminID, "POST", "/api/platform/workspaces/"+foreignWSID+"/delete-now", delBody), 404, "not_found")
	if len(e.delete.workspaces) != 0 {
		t.Errorf("the service was reached for a workspace outside the tenant")
	}
}

func TestDeleteNowTenantIsSystemAdminOnly(t *testing.T) {
	e := newDeletionEnv(t)
	for _, u := range []string{tadminID, suppID, billID} {
		wantUnknownRouteOrNotFound(t, e.do(u, "POST", "/api/platform/tenants/"+tenantID+"/delete-now", `{"reason":"r","confirm":"Acme"}`))
	}
	if len(e.delete.tenants) != 0 {
		t.Errorf("the service was reached by a non-system_admin")
	}
	rec := e.do(adminID, "POST", "/api/platform/tenants/"+tenantID+"/delete-now", `{"reason":"r","confirm":"Acme"}`)
	wantStatus(t, rec, http.StatusAccepted)
	if len(e.delete.tenants) != 1 || e.delete.tenants[0].tenant != tenantID {
		t.Errorf("tenant service got %+v", e.delete.tenants)
	}
}

// wantUnknownRouteOrNotFound accepts the flat 404 of a route the role may not use.
func wantUnknownRouteOrNotFound(t *testing.T, rec *httptest.ResponseRecorder) {
	t.Helper()
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rec.Code, rec.Body.String())
	}
}

func TestDeleteNowRefusalsCarryTheirCode(t *testing.T) {
	e := newDeletionEnv(t)
	e.delete.err = &deletionrequest.Error{Status: 400, Code: deletionrequest.CodeConfirmationMismatch, Message: "no"}
	wantCode(t, e.do(tadminID, "POST", "/api/platform/workspaces/"+wsID+"/delete-now", delBody), 400, "confirmation_mismatch")
	e.delete.err = errors.New("db password is hunter2")
	rec := e.do(tadminID, "POST", "/api/platform/workspaces/"+wsID+"/delete-now", delBody)
	wantCode(t, rec, 500, "internal")
}

func TestDeletionRequestVisibilityForAdmin(t *testing.T) {
	e := newDeletionEnv(t)
	rec := e.do(tadminID, "GET", "/api/platform/deletion-requests", "")
	wantStatus(t, rec, http.StatusOK)
	if got := e.delete.filters[len(e.delete.filters)-1].TenantID; got != tenantID {
		t.Errorf("admin list filtered by %q, want its tenant", got)
	}
	wantCode(t, e.do(tadminID, "GET", "/api/platform/deletion-requests/"+delForeignID, ""), 404, "not_found")
	wantStatus(t, e.do(tadminID, "GET", "/api/platform/deletion-requests/"+delReqID, ""), http.StatusOK)
	rec = e.do(adminID, "GET", "/api/platform/deletion-requests", "")
	wantStatus(t, rec, http.StatusOK)
	if got := e.delete.filters[len(e.delete.filters)-1].TenantID; got != "" {
		t.Errorf("system_admin list filtered by %q, want none", got)
	}
}

func TestDeletionRequestFilterRejectsUnknownValues(t *testing.T) {
	e := newDeletionEnv(t)
	wantCode(t, e.do(adminID, "GET", "/api/platform/deletion-requests?kind=bogus", ""), 400, "invalid_filter")
	wantCode(t, e.do(adminID, "GET", "/api/platform/deletion-requests?state=bogus", ""), 400, "invalid_filter")
}

func TestDeletionCancelAndRetryOnlyOwnForAdmin(t *testing.T) {
	e := newDeletionEnv(t)
	wantCode(t, e.do(tadminID, "POST", "/api/platform/deletion-requests/"+delForeignID+"/cancel", ""), 404, "not_found")
	// delByOwnAdmin was made by system_admin: the admin may not retry it.
	wantCode(t, e.do(tadminID, "POST", "/api/platform/deletion-requests/"+delByOwnAdmin+"/retry", ""), 404, "not_found")
	wantStatus(t, e.do(tadminID, "POST", "/api/platform/deletion-requests/"+delReqID+"/cancel", ""), http.StatusOK)
	// A request made by system_admin on a workspace of the admin's tenant is not the admin's to cancel.
	e.delete.rows[delReqID] = deletionrequest.Request{ID: delReqID, Kind: deletionrequest.KindWorkspace, TargetID: wsID, State: deletionrequest.StatePending, TenantID: tenantID, RequestedBy: adminID}
	wantCode(t, e.do(tadminID, "POST", "/api/platform/deletion-requests/"+delReqID+"/cancel", ""), 404, "not_found")
	wantStatus(t, e.do(adminID, "POST", "/api/platform/deletion-requests/"+delReqID+"/cancel", ""), http.StatusOK)
}

const delAccountBody = `{"reason":"legal request","confirm":"leaver@example.test","resolutions":[{"workspace_id":"` + wsID + `","action":"delete"}]}`

func TestDeleteAccountNowIsSystemAdminOnly(t *testing.T) {
	e := newDeletionEnv(t)
	for _, u := range []string{tadminID, suppID, billID} {
		wantUnknownRouteOrNotFound(t, e.do(u, "POST", "/api/platform/users/"+custID+"/delete-now", delAccountBody))
		wantUnknownRouteOrNotFound(t, e.do(u, "GET", "/api/platform/users/"+custID+"/delete-now-preview", ""))
	}
	if len(e.delete.accounts) != 0 {
		t.Fatalf("the service was reached by a non-system_admin")
	}
}

func TestDeleteAccountNowPassesResolutionsAndAnswers202(t *testing.T) {
	e := newDeletionEnv(t)
	rec := e.do(adminID, "POST", "/api/platform/users/"+custID+"/delete-now", delAccountBody)
	wantStatus(t, rec, http.StatusAccepted)
	if body := decode(t, rec); body["request_id"] != delReqID {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if len(e.delete.accounts) != 1 {
		t.Fatalf("service calls = %d", len(e.delete.accounts))
	}
	got := e.delete.accounts[0]
	if got.user != custID || got.by.ID != adminID || len(got.in.Resolutions) != 1 || got.in.Resolutions[0].Action != "delete" {
		t.Fatalf("service got %+v", got)
	}
}

func TestDeleteAccountNowSoleOwnerUnresolvedIs409WithList(t *testing.T) {
	e := newDeletionEnv(t)
	e.delete.err = &deletionrequest.Error{Status: 409, Code: deletionrequest.CodeSoleOwnerUnresolved, Message: "choose",
		Extra: map[string]any{"workspace_ids": []string{wsID}}}
	rec := e.do(adminID, "POST", "/api/platform/users/"+custID+"/delete-now", `{"reason":"r","confirm":"x"}`)
	wantCode(t, rec, 409, "sole_owner_unresolved")
	ids, _ := decode(t, rec)["workspace_ids"].([]any)
	if len(ids) != 1 || ids[0] != wsID {
		t.Fatalf("workspace_ids = %v", decode(t, rec)["workspace_ids"])
	}
}

func TestDeleteAccountPreviewReturnsWorkspaces(t *testing.T) {
	e := newDeletionEnv(t)
	e.delete.preview = deletionrequest.AccountPreview{Email: "leaver@example.test",
		NeedChoice: []deletionrequest.AccountWorkspace{{ID: wsID, Slug: "acme", Role: "owner", OwnerCount: 1, MemberCount: 3}}}
	rec := e.do(adminID, "GET", "/api/platform/users/"+custID+"/delete-now-preview", "")
	wantStatus(t, rec, http.StatusOK)
	body := decode(t, rec)
	if body["email"] != "leaver@example.test" {
		t.Fatalf("body = %s", rec.Body.String())
	}
	if nc, _ := body["need_choice"].([]any); len(nc) != 1 {
		t.Fatalf("need_choice = %v", body["need_choice"])
	}
	if dw, ok := body["deleted_with"].([]any); !ok || dw == nil {
		t.Fatalf("deleted_with must be an empty list, not null: %s", rec.Body.String())
	}
}
