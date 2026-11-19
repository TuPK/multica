package console

import (
	"context"
	"net/http"
	"testing"

	"github.com/multica-ai/multica/platform-management/internal/deletionrequest"
	"github.com/multica-ai/multica/platform-management/internal/legalhold"
)

// Spec R6 section 3c.7 (QT11, M50): only system_admin places, releases and sees
// holds; a deletion a hold covers answers 409 legal_hold, with the hold's id
// shown to system_admin alone.

type fakeLegalHolds struct {
	placed []string
}

func (f *fakeLegalHolds) Place(_ context.Context, _ legalhold.Actor, kind, scopeID, _, reason, caseRef string) (legalhold.Hold, error) {
	f.placed = append(f.placed, kind+" "+scopeID)
	return legalhold.Hold{ID: "h1", ScopeKind: kind, ScopeID: scopeID, Reason: reason, CaseRef: caseRef}, nil
}

func (f *fakeLegalHolds) Release(_ context.Context, _ legalhold.Actor, id, _ string) (legalhold.Hold, error) {
	return legalhold.Hold{ID: id, Released: true}, nil
}

func (f *fakeLegalHolds) List(context.Context, int) ([]legalhold.Hold, error) {
	return []legalhold.Hold{{ID: "h1", ScopeKind: "tenant", Reason: "r", CaseRef: "c"}}, nil
}

func TestLegalHoldRoutesAreSystemAdminOnly(t *testing.T) {
	e := newEnv(t)
	body := `{"scope_kind":"tenant","scope_id":"` + tenantID + `","reason":"court order","case_ref":"CASE-1"}`
	for _, u := range []string{tadminID, suppID, billID} {
		wantUnknownRouteOrNotFound(t, e.do(u, "POST", "/api/platform/legal-holds", body))
		wantUnknownRouteOrNotFound(t, e.do(u, "GET", "/api/platform/legal-holds", ""))
	}
	wantStatus(t, e.do(adminID, "POST", "/api/platform/legal-holds", body), http.StatusCreated)
	if len(e.holds.placed) != 1 {
		t.Fatalf("placed = %v", e.holds.placed)
	}
}

func TestHoldRefusalShowsItsIDOnlyToSystemAdmin(t *testing.T) {
	e := newEnv(t)
	e.delete.err = &deletionrequest.Error{Status: 409, Code: deletionrequest.CodeLegalHold, Message: "held",
		Extra: map[string]any{"legal_hold_id": "h-hidden-id"}}
	path := "/api/platform/workspaces/" + wsID + "/delete-now"
	rec := e.do(adminID, "POST", path, delBody)
	wantCode(t, rec, 409, "legal_hold")
	if _, ok := decode(t, rec)["legal_hold_id"]; !ok {
		t.Fatal("system_admin must see the hold id")
	}
	rec = e.do(tadminID, "POST", path, delBody)
	wantCode(t, rec, 409, "legal_hold")
	if _, ok := decode(t, rec)["legal_hold_id"]; ok {
		t.Fatalf("admin must not see the hold id: %s", rec.Body.String())
	}
}
