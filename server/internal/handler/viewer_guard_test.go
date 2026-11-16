package handler

import (
	"bytes"
	"context"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// Spec 6.6: a viewer reaches no write through the daemon routes, the runtime
// registration, or a file upload. Each refusal sits before the write.

// membershipRedisSpy counts the writes to the membership cache. Reads miss, so
// every request falls through to the database lookup of the member.
type membershipRedisSpy struct {
	redis.UniversalClient
	sets int
}

func (s *membershipRedisSpy) Get(_ context.Context, _ string) *redis.StringCmd {
	return redis.NewStringResult("", redis.Nil)
}

func (s *membershipRedisSpy) Set(_ context.Context, _ string, _ interface{}, _ time.Duration) *redis.StatusCmd {
	s.sets++
	return redis.NewStatusResult("OK", nil)
}

// A viewer's request through the daemon routes is refused and writes nothing
// to the membership cache; a member's request passes and caches. The member
// case is the control that shows the refusal is the role and not the setup.
func TestRequireDaemonWorkspaceAccessRefusesAViewer(t *testing.T) {
	const workspace = "22222222-2222-4222-8222-222222222222"
	for _, tc := range []struct {
		role   string
		ok     bool
		sets   int
		status int
	}{
		{role: "member", ok: true, sets: 1},
		{role: "viewer", ok: false, sets: 0, status: http.StatusForbidden},
	} {
		t.Run(tc.role, func(t *testing.T) {
			spy := &membershipRedisSpy{}
			h := &Handler{
				Queries:         db.New(wsStateMemberDB{role: tc.role}),
				MembershipCache: auth.NewMembershipCache(spy),
			}
			req := httptest.NewRequest(http.MethodPost, "/api/daemon/register", nil)
			req.Header.Set("X-User-ID", wsStateTestUser)
			rec := httptest.NewRecorder()

			if got := h.requireDaemonWorkspaceAccess(rec, req, workspace); got != tc.ok {
				t.Fatalf("requireDaemonWorkspaceAccess = %v, want %v (body %q)", got, tc.ok, rec.Body.String())
			}
			if tc.status != 0 && rec.Code != tc.status {
				t.Errorf("status %d, want %d", rec.Code, tc.status)
			}
			if spy.sets != tc.sets {
				t.Errorf("membership cache writes = %d, want %d", spy.sets, tc.sets)
			}
		})
	}
}

// A viewer cannot register a runtime through the user credential path. The
// refusal comes before any write, so the fake database (which serves only the
// member lookup) is never asked for more.
func TestDaemonRegisterRefusesAViewer(t *testing.T) {
	h := &Handler{Queries: db.New(wsStateMemberDB{role: "viewer"})}
	body := `{"workspace_id":"22222222-2222-4222-8222-222222222222","daemon_id":"viewer-daemon","device_name":"laptop",` +
		`"runtimes":[{"name":"Claude","type":"claude","version":"1.0","status":"online"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/daemon/register", strings.NewReader(body))
	req.Header.Set("X-User-ID", wsStateTestUser)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	h.DaemonRegister(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403 (body %q)", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), workspaceguard.CodeViewerReadOnly) {
		t.Errorf("body %q does not name %s", rec.Body.String(), workspaceguard.CodeViewerReadOnly)
	}
}

// A viewer cannot store a file in the workspace. The membership check that
// precedes the upload is where the refusal sits, so storage is never reached.
func TestUploadFileRefusesAViewer(t *testing.T) {
	h := &Handler{Queries: db.New(wsStateMemberDB{role: "viewer"})}
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", "note.txt")
	if err != nil {
		t.Fatal(err)
	}
	part.Write([]byte("hello"))
	mw.Close()
	req := httptest.NewRequest(http.MethodPost, "/api/upload-file", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-User-ID", wsStateTestUser)
	req.Header.Set("X-Workspace-ID", wsStateTestWorkspace)
	rec := httptest.NewRecorder()

	h.UploadFile(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status %d, want 403 (body %q)", rec.Code, rec.Body.String())
	}
}
