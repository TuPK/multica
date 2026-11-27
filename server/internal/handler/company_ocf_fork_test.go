package handler

import (
	"archive/zip"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
)

func TestOCFHandlerPrincipalExportConsent(t *testing.T) {
	if testHandler == nil || testPool == nil {
		t.Skip("database not available")
	}
	withFeatureFlag(t, testHandler, featureflags.DurableOpen, true)
	e := newDigestEnv(t)
	other := dbfx.User(t, "Other Principal", uuid.NewString()+"@example.test")
	dbfx.Member(t, e.WorkspaceID, other, "admin")
	for _, id := range []string{testUserID, other} {
		dbfx.InsertNoID(t, "principal_profile", testutil.Cols{"workspace_id": e.WorkspaceID, "member_id": id, "status": "active", "consent_at": testutil.Raw("now()"), "summary": "private profile"}, "workspace_id = $1 AND member_id = $2", e.WorkspaceID, id)
	}
	for _, tc := range []struct {
		query string
		want  bool
	}{
		{"?format=ocf&history=true", false},
		{"?format=ocf&history=true&include_principal=true", true},
	} {
		w := httptest.NewRecorder()
		testHandler.serveOCFExport(w, seatRequest("GET", "/api/company/export"+tc.query, e.WorkspaceID, nil, ""))
		if w.Code != http.StatusOK {
			t.Fatalf("%s: status %d: %s", tc.query, w.Code, w.Body.String())
		}
		zr, err := zip.NewReader(bytes.NewReader(w.Body.Bytes()), int64(w.Body.Len()))
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, f := range zr.File {
			if f.Name != "history/principals.ndjson" {
				continue
			}
			found = true
			r, err := f.Open()
			if err != nil {
				t.Fatal(err)
			}
			body, err := io.ReadAll(r)
			_ = r.Close()
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(body), "Other Principal") || bytes.Count(body, []byte("\n")) != 1 {
				t.Fatalf("unexpected principals: %s", body)
			}
		}
		if found != tc.want {
			t.Errorf("%s: principal file present = %v, want %v", tc.query, found, tc.want)
		}
	}
	withFeatureFlag(t, testHandler, featureflags.DurableOpen, false)
	w := httptest.NewRecorder()
	testHandler.serveOCFExport(w, seatRequest("GET", "/api/company/export?format=ocf&history=true", e.WorkspaceID, nil, ""))
	if w.Code != http.StatusNotFound {
		t.Fatalf("flag off: status %d, want 404", w.Code)
	}
}
