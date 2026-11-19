package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// staffAuditWriter is the audit.Writer the platform console uses, bound to the
// test queries: it maps an audit.Entry to the audit_event insert unchanged.
type staffAuditWriter struct{ q *db.Queries }

func (w staffAuditWriter) InsertAuditEvent(ctx context.Context, e audit.Entry) error {
	var ws pgtype.UUID
	if e.WorkspaceID != "" {
		if err := ws.Scan(e.WorkspaceID); err != nil {
			return err
		}
	}
	detail := e.Detail
	if detail == nil {
		detail = map[string]any{}
	}
	raw, err := json.Marshal(detail)
	if err != nil {
		return err
	}
	return w.q.InsertAuditEvent(ctx, db.InsertAuditEventParams{
		WorkspaceID: ws,
		ActorType:   e.ActorType,
		ActorID:     pgtype.Text{String: e.ActorID, Valid: e.ActorID != ""},
		ActorLabel:  e.ActorLabel,
		Action:      e.Action,
		TargetType:  e.TargetType,
		TargetID:    e.TargetID,
		TargetLabel: e.TargetLabel,
		Detail:      raw,
		Ip:          e.IP,
		UserAgent:   e.UserAgent,
		RequestID:   e.RequestID,
	})
}

// Spec 5.2 and section 11: a staff action on a workspace is recorded with
// actor_type = staff and the workspace_id, and the workspace owner and admins
// read it in their own audit log. A plain member cannot.
func TestWorkspaceOwnerSeesStaffActionsInAuditLog(t *testing.T) {
	requirePlatformTestDatabase(t)
	router := auditTestRouter()
	ws, owner, admin, member := auditWorkspace(t)
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `DELETE FROM audit_event WHERE workspace_id = $1`, ws)
	})

	staffEmail := "operator-" + uuid.NewString()[:8] + "@platform.example"
	requestID := "staff-visibility-" + uuid.NewString()
	err := audit.Record(context.Background(), staffAuditWriter{q: testHandler.Queries}, audit.Entry{
		WorkspaceID: ws,
		ActorType:   audit.ActorStaff,
		ActorID:     uuid.NewString(),
		ActorLabel:  staffEmail,
		Action:      audit.ActionWorkspaceStatusChanged,
		TargetType:  "workspace",
		TargetID:    ws,
		TargetLabel: "Audit workspace",
		Detail:      map[string]any{"from": "active", "to": "suspended"},
		RequestID:   requestID,
	})
	if err != nil {
		t.Fatalf("record staff audit entry: %v", err)
	}
	var rowID string
	if err := testPool.QueryRow(context.Background(),
		`SELECT id::text FROM audit_event WHERE request_id = $1`, requestID).Scan(&rowID); err != nil {
		t.Fatalf("find staff audit row: %v", err)
	}

	path := "/api/workspaces/" + ws + "/audit-events?action=" + audit.ActionWorkspaceStatusChanged
	for name, uid := range map[string]string{"owner": owner, "admin": admin} {
		rec := auditGet(t, router, uid, path)
		if rec.Code != http.StatusOK {
			t.Fatalf("%s: status %d, want 200: %s", name, rec.Code, rec.Body.String())
		}
		var found *auditEventJSON
		page := decodeAuditPage(t, rec)
		for i := range page.Events {
			if page.Events[i].ID == rowID {
				found = &page.Events[i]
			}
		}
		if found == nil {
			t.Fatalf("%s: staff row %s is not in the workspace audit log", name, rowID)
		}
		if found.ActorType != audit.ActorStaff {
			t.Errorf("%s: actor_type = %q, want staff", name, found.ActorType)
		}
		if found.WorkspaceID == nil || *found.WorkspaceID != ws {
			t.Errorf("%s: workspace_id = %v, want %s", name, found.WorkspaceID, ws)
		}
		if found.Action != audit.ActionWorkspaceStatusChanged {
			t.Errorf("%s: action = %q", name, found.Action)
		}
		if found.ActorLabel != staffEmail {
			t.Errorf("%s: actor_label = %q, want %q", name, found.ActorLabel, staffEmail)
		}
	}

	if rec := auditGet(t, router, member, path); rec.Code != http.StatusForbidden {
		t.Fatalf("member: status %d, want 403", rec.Code)
	}
}
