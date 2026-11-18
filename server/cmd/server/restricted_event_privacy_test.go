package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gorilla/websocket"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/projectaccess"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/testutil"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// wholeWorkspaceAudiences answers every project with "the whole workspace".
// Listener tests that are not about project visibility use it.
type wholeWorkspaceAudiences struct{}

func (wholeWorkspaceAudiences) Audience(context.Context, pgtype.UUID, pgtype.UUID) (projectaccess.Recipients, error) {
	return projectaccess.Recipients{Workspace: true}, nil
}

func (wholeWorkspaceAudiences) Users(context.Context, pgtype.UUID, pgtype.UUID) ([]string, error) {
	return nil, nil
}

func (wholeWorkspaceAudiences) IssueProject(context.Context, pgtype.UUID) (pgtype.UUID, error) {
	return pgtype.UUID{}, nil
}

const stopFrame = "test:stop"

// person is one connected user.
type person struct {
	id    string
	token string
	conn  *websocket.Conn
}

// restrictedWorld is a workspace with one person of every kind, all
// connected, behind the real router and the production listeners.
type restrictedWorld struct {
	t      *testing.T
	fx     *testutil.Fixture
	hub    *realtime.Hub
	bus    *events.Bus
	server *httptest.Server
	people map[string]*person
	member map[string]string // person name -> member row id
}

// newRestrictedWorld connects owner, admin, member, viewer, other (a member
// no project grants anything to), guest and otherGuest.
func newRestrictedWorld(t *testing.T, names ...string) *restrictedWorld {
	t.Helper()
	w := &restrictedWorld{
		t: t, fx: workspaceFixture(t),
		people: map[string]*person{}, member: map[string]string{},
	}
	w.hub = realtime.NewHub()
	go w.hub.Run()
	w.bus = events.New()
	registerListeners(w.bus, w.hub, queryAudiences{q: db.New(testPool)})
	router, _ := NewRouterWithOptions(testPool, w.hub, w.bus, analytics.NoopClient{}, nil, RouterOptions{})
	w.server = httptest.NewServer(router)
	t.Cleanup(w.server.Close)

	for _, name := range names {
		switch name {
		case "owner":
			w.people[name] = &person{id: testUserID}
		default:
			id := w.fx.User(t, "Realtime "+name, testutil.UniqueEmail("rt-"+name))
			role, cols := "member", testutil.Cols{}
			switch name {
			case "admin":
				role = "admin"
			case "viewer":
				role = "viewer"
			case "guest", "otherGuest":
				cols["is_guest"] = true
			}
			w.member[name] = w.fx.Member(t, testWorkspaceID, id, role, cols)
			w.people[name] = &person{id: id}
		}
	}
	for _, p := range w.people {
		w.connect(p)
	}
	return w
}

func (w *restrictedWorld) connect(p *person) {
	w.t.Helper()
	p.conn, p.token = realtimeMembershipConn(w.t, w.server, p.id, testWorkspaceID)
}

func (w *restrictedWorld) uid(name string) string { return w.people[name].id }

// project inserts a project of the given access and grants it to the named
// people.
func (w *restrictedWorld) project(title, access string, granted ...string) string {
	w.t.Helper()
	id := w.fx.Project(w.t, title, testutil.Cols{"access": access})
	for _, name := range granted {
		w.grant(id, name)
	}
	return id
}

func (w *restrictedWorld) grant(projectID, name string) {
	w.t.Helper()
	w.fx.Exec(w.t, `INSERT INTO project_member (project_id, workspace_id, user_id) VALUES ($1, $2, $3)`,
		projectID, testWorkspaceID, w.uid(name))
	w.fx.Cleanup(w.t, `DELETE FROM project_member WHERE project_id = $1 AND user_id = $2`, projectID, w.uid(name))
}

// api sends a request as name and fails on a non-2xx status.
func (w *restrictedWorld) api(name, method, path string, body any) {
	w.t.Helper()
	chatPrivacyRequest(w.t, w.server, w.people[name].token, method, path, body, nil)
}

// publishIssue publishes issue:updated for an issue of projectID ("" for none).
func (w *restrictedWorld) publishIssue(eventType, issueID, projectID, title string) {
	issue := map[string]any{"id": issueID, "title": title, "project_id": nil}
	if projectID != "" {
		issue["project_id"] = projectID
	}
	w.bus.Publish(events.Event{
		Type: eventType, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: map[string]any{"issue": issue},
	})
}

// received returns what name's socket got since the last call, by sending a
// personal marker frame that every connection, guest or not, receives.
func (w *restrictedWorld) received(name string) []chatPrivacyFrame {
	w.t.Helper()
	p := w.people[name]
	w.hub.SendToUser(p.id, []byte(`{"type":"`+stopFrame+`"}`))
	return readChatPrivacyFrames(w.t, p.conn, stopFrame)
}

func ofType(frames []chatPrivacyFrame, typ string) []chatPrivacyFrame {
	var out []chatPrivacyFrame
	for _, f := range frames {
		if f.Type == typ {
			out = append(out, f)
		}
	}
	return out
}

func types(frames []chatPrivacyFrame) []string {
	out := make([]string, 0, len(frames))
	for _, f := range frames {
		out = append(out, f.Type)
	}
	return out
}

func (w *restrictedWorld) wantGot(name string, frames []chatPrivacyFrame, typ string, want bool) {
	w.t.Helper()
	if got := len(ofType(frames, typ)) > 0; got != want {
		w.t.Errorf("%s: received %s = %v, want %v (frames %v)", name, typ, got, want, types(frames))
	}
}

func TestRestrictedIssueEventReachesOnlyGranted(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "admin", "member", "other", "guest", "otherGuest")
	project := w.project("Restricted realtime", "restricted", "member", "guest")
	const secret = "restricted-title-5d21"

	w.publishIssue(protocol.EventIssueUpdated, "issue-1", project, secret)

	for name, want := range map[string]bool{
		"owner": true, "admin": true, "member": true, "guest": true,
		"other": false, "otherGuest": false,
	} {
		frames := w.received(name)
		w.wantGot(name, frames, protocol.EventIssueUpdated, want)
		for _, f := range frames {
			if !want && strings.Contains(f.raw, secret) {
				t.Errorf("%s leaked the restricted title: %s", name, f.raw)
			}
		}
	}
}

func TestGuestNeverJoinsWorkspaceRoom(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "member", "guest")

	// No project and a workspace-access project both fan out to the
	// workspace room. A guest is in neither audience unless granted.
	open := w.project("Open realtime", "workspace")
	w.publishIssue(protocol.EventIssueUpdated, "issue-1", "", "no project")
	w.publishIssue(protocol.EventIssueUpdated, "issue-2", open, "open project")
	w.bus.Publish(events.Event{Type: protocol.EventAgentStatus, WorkspaceID: testWorkspaceID, Payload: map[string]any{}})

	if frames := w.received("guest"); len(frames) != 0 {
		t.Fatalf("guest received workspace-room frames: %v", types(frames))
	}
	if got := len(ofType(w.received("member"), protocol.EventIssueUpdated)); got != 2 {
		t.Errorf("member received %d workspace issue events, want 2", got)
	}

	// The guest still receives what is addressed to them: their own user scope
	// (received() relies on it) and their member scope.
	w.hub.BroadcastToScope(realtime.ScopeWorkspaceMember, realtime.WorkspaceMemberScopeID(testWorkspaceID, w.uid("guest")),
		[]byte(`{"type":"member-scope-probe"}`))
	if len(ofType(w.received("guest"), "member-scope-probe")) != 1 {
		t.Error("guest did not receive a frame sent to their member scope")
	}
}

func TestMoveIssueSplitsAudience(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "admin", "member", "other", "guest", "otherGuest")
	// "member" sees both projects, "other" neither, and each guest one.
	// Group names say which project a person is granted:
	//   oldOnly -> from, newOnly -> to, both -> member.
	from := w.project("Move from", "restricted", "member", "guest")
	to := w.project("Move to", "restricted", "member", "otherGuest")
	issueID := w.fx.Issue(t, "moving issue", testutil.Cols{"project_id": from})
	const title = "moving issue"

	w.api("owner", http.MethodPut, "/api/issues/"+issueID, map[string]any{"project_id": to})

	type want struct{ created, updated, deleted bool }
	for name, exp := range map[string]want{
		"owner":      {updated: true},
		"admin":      {updated: true},
		"member":     {updated: true}, // in both audiences
		"guest":      {deleted: true}, // only in the old audience
		"otherGuest": {created: true}, // only in the new audience
		"other":      {},
	} {
		frames := w.received(name)
		w.wantGot(name, frames, protocol.EventIssueCreated, exp.created)
		w.wantGot(name, frames, protocol.EventIssueUpdated, exp.updated)
		w.wantGot(name, frames, protocol.EventIssueDeleted, exp.deleted)
		for _, f := range ofType(frames, protocol.EventIssueDeleted) {
			if strings.Contains(f.raw, title) || !strings.Contains(f.raw, issueID) {
				t.Errorf("%s: issue:deleted must carry the issue id and no content, got %s", name, f.raw)
			}
		}
		for _, f := range ofType(frames, protocol.EventIssueCreated) {
			if !strings.Contains(f.raw, title) || strings.Contains(f.raw, "prev_project_id") {
				t.Errorf("%s: issue:created must carry the issue and no internal keys, got %s", name, f.raw)
			}
		}
		for _, f := range frames {
			if strings.Contains(f.raw, "prev_project_id") {
				t.Errorf("%s: frame leaked prev_project_id: %s", name, f.raw)
			}
		}
	}

	// A move out of a restricted project into one every member sees is a
	// create for the members who could not see the old one.
	open := w.project("Move to open", "workspace")
	w.api("owner", http.MethodPut, "/api/issues/"+issueID, map[string]any{"project_id": open})
	for name, exp := range map[string]want{
		"owner":      {updated: true},
		"member":     {updated: true},
		"other":      {created: true},
		"otherGuest": {deleted: true},
		"guest":      {},
	} {
		// `to` was restricted to member and otherGuest; guest saw neither.
		frames := w.received(name)
		w.wantGot(name, frames, protocol.EventIssueCreated, exp.created)
		w.wantGot(name, frames, protocol.EventIssueUpdated, exp.updated)
		w.wantGot(name, frames, protocol.EventIssueDeleted, exp.deleted)
	}
}

func TestAccessChangedEventCarriesNoContent(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "admin", "member", "other", "guest")
	const title = "access-title-8c0e"
	project := w.project(title, "workspace", "guest")

	w.api("owner", http.MethodPut, "/api/projects/"+project+"/access",
		map[string]any{"access": "restricted", "member_ids": []string{w.uid("member")}})

	// Before the change everyone in the workspace could see the project, and
	// the granted guest too. Nobody else gets the event.
	for name, want := range map[string]bool{"owner": true, "admin": true, "member": true, "other": true, "guest": true} {
		frames := w.received(name)
		w.wantGot(name, frames, protocol.EventProjectAccessChanged, want)
		for _, f := range ofType(frames, protocol.EventProjectAccessChanged) {
			var frame struct {
				Payload map[string]any `json:"payload"`
			}
			if err := json.Unmarshal([]byte(f.raw), &frame); err != nil {
				t.Fatal(err)
			}
			if len(frame.Payload) != 2 || frame.Payload["project_id"] != project || frame.Payload["access"] != "restricted" {
				t.Errorf("%s: payload must be exactly project_id and access, got %v", name, frame.Payload)
			}
			if strings.Contains(f.raw, title) || strings.Contains(f.raw, "recipient") {
				t.Errorf("%s: frame carries content: %s", name, f.raw)
			}
		}
	}

	// Switching back goes only to who could see it while restricted.
	w.api("owner", http.MethodPut, "/api/projects/"+project+"/access", map[string]any{"access": "workspace"})
	for name, want := range map[string]bool{"owner": true, "admin": true, "member": true, "guest": true, "other": false} {
		w.wantGot(name, w.received(name), protocol.EventProjectAccessChanged, want)
	}
}

// Creating a restricted project must not announce it to people outside its
// audience.
func TestRestrictedProjectCreationEmitsOnlyToAudience(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "admin", "member", "other", "guest")
	project := w.project("Hidden project", "restricted", "member")

	for _, typ := range []string{protocol.EventProjectCreated, protocol.EventProjectUpdated} {
		payload := map[string]any{"project": map[string]any{"id": project, "title": "Hidden project"}}
		w.bus.Publish(events.Event{Type: typ, WorkspaceID: testWorkspaceID, Payload: payload})
		for name, want := range map[string]bool{"owner": true, "admin": true, "member": true, "other": false, "guest": false} {
			w.wantGot(name, w.received(name), typ, want)
		}
	}
}

// project:deleted carries its audience from the handler, because the grants
// are gone by the time the event is published. Without it the event is dropped.
func TestProjectDeletedWithoutAudienceIsDropped(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "member")
	w.bus.Publish(events.Event{Type: protocol.EventProjectDeleted, WorkspaceID: testWorkspaceID,
		Payload: map[string]any{"project_id": "00000000-0000-4000-8000-000000000998"}})
	for _, name := range []string{"owner", "member"} {
		w.wantGot(name, w.received(name), protocol.EventProjectDeleted, false)
	}
}

func TestRevokedPersonStopsReceivingAtNextEvent(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "member")
	project := w.project("Revocation", "restricted", "member")

	w.publishIssue(protocol.EventIssueUpdated, "issue-1", project, "before revoke")
	w.wantGot("member", w.received("member"), protocol.EventIssueUpdated, true)

	w.api("owner", http.MethodDelete, "/api/projects/"+project+"/members/"+w.uid("member"), nil)

	w.publishIssue(protocol.EventIssueUpdated, "issue-1", project, "after revoke")
	w.wantGot("member", w.received("member"), protocol.EventIssueUpdated, false)
	w.wantGot("owner", w.received("owner"), protocol.EventIssueUpdated, true)

	// The connection stays open: revoking a grant only changes the recipient
	// list. received() sending and reading a frame proves the socket is alive.
	if got := w.received("member"); len(got) != 0 {
		t.Errorf("member received %v after revocation", types(got))
	}
}

func TestGuestFlagChangeDisconnectsUser(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "member")
	patch := func(guest bool) {
		w.api("owner", http.MethodPatch, "/api/workspaces/"+testWorkspaceID+"/members/"+w.member["member"],
			map[string]any{"role": "member", "is_guest": guest})
	}

	patch(true)
	wantRealtimeMembershipClosed(t, w.people["member"].conn)

	// Rejoined as a guest: no workspace room.
	w.connect(w.people["member"])
	w.publishIssue(protocol.EventIssueUpdated, "issue-1", "", "workspace wide")
	if frames := w.received("member"); len(frames) != 0 {
		t.Fatalf("guest connection received workspace-room frames: %v", types(frames))
	}

	patch(false)
	wantRealtimeMembershipClosed(t, w.people["member"].conn)
	w.connect(w.people["member"])
	w.publishIssue(protocol.EventIssueUpdated, "issue-1", "", "workspace wide")
	w.wantGot("member", w.received("member"), protocol.EventIssueUpdated, true)

	// Owner's connection was not touched.
	w.wantGot("owner", w.received("owner"), protocol.EventIssueUpdated, true)
}

// Regression: with no restricted project and no guest, every event still
// reaches every non-guest member through the workspace room.
func TestWorkspaceWideEventsStillReachNonGuests(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "admin", "member", "viewer", "guest", "otherGuest")
	open := w.project("Workspace wide", "workspace", "guest")

	w.publishIssue(protocol.EventIssueCreated, "issue-1", "", "no project")
	w.publishIssue(protocol.EventIssueUpdated, "issue-2", open, "open project")
	w.bus.Publish(events.Event{Type: protocol.EventAgentStatus, WorkspaceID: testWorkspaceID, Payload: map[string]any{}})
	w.bus.Publish(events.Event{Type: protocol.EventProjectUpdated, WorkspaceID: testWorkspaceID,
		Payload: map[string]any{"project": map[string]any{"id": open}}})

	for _, name := range []string{"owner", "admin", "member", "viewer"} {
		frames := w.received(name)
		for _, typ := range []string{protocol.EventIssueCreated, protocol.EventIssueUpdated, protocol.EventAgentStatus, protocol.EventProjectUpdated} {
			w.wantGot(name, frames, typ, true)
		}
	}

	// A granted guest receives the workspace project's events, once each, and
	// nothing else; an ungranted guest receives nothing.
	frames := w.received("guest")
	if got := types(frames); len(got) != 2 || len(ofType(frames, protocol.EventIssueUpdated)) != 1 || len(ofType(frames, protocol.EventProjectUpdated)) != 1 {
		t.Errorf("granted guest got %v, want issue:updated and project:updated once each", got)
	}
	if frames := w.received("otherGuest"); len(frames) != 0 {
		t.Errorf("ungranted guest got %v", types(frames))
	}
}

// issueIn inserts an issue of projectID ("" for none) and returns its id.
func (w *restrictedWorld) issueIn(title, projectID string) string {
	w.t.Helper()
	if projectID == "" {
		return w.fx.Issue(w.t, title)
	}
	return w.fx.Issue(w.t, title, testutil.Cols{"project_id": projectID})
}

func (w *restrictedWorld) publishDerived(eventType string, payload map[string]any) {
	w.bus.Publish(events.Event{
		Type: eventType, WorkspaceID: testWorkspaceID, ActorType: "member", ActorID: testUserID,
		Payload: payload,
	})
}

// wantDerivedAudience publishes the event built by payload for an issue of a
// restricted project and for an issue with no project, and checks who got it.
func wantDerivedAudience(t *testing.T, eventType string, payload func(issueID, secret string) map[string]any) {
	t.Helper()
	w := newRestrictedWorld(t, "owner", "admin", "member", "other", "guest", "otherGuest")
	project := w.project("Restricted derived", "restricted", "member", "guest")
	const secret = "derived-secret-7c3e"
	restricted := w.issueIn("restricted issue", project)
	open := w.issueIn("open issue", "")

	w.publishDerived(eventType, payload(restricted, secret))
	for name, want := range map[string]bool{
		"owner": true, "admin": true, "member": true, "guest": true,
		"other": false, "otherGuest": false,
	} {
		frames := w.received(name)
		w.wantGot(name, frames, eventType, want)
		for _, f := range frames {
			if !want && strings.Contains(f.raw, secret) {
				t.Errorf("%s leaked %s content: %s", name, eventType, f.raw)
			}
		}
	}

	// An issue with no project keeps workspace routing: members get it, guests do not.
	w.publishDerived(eventType, payload(open, secret))
	for name, want := range map[string]bool{"owner": true, "other": true, "member": true, "guest": false, "otherGuest": false} {
		w.wantGot(name, w.received(name), eventType, want)
	}
}

func TestRestrictedCommentEventReachesOnlyGranted(t *testing.T) {
	for _, typ := range []string{protocol.EventCommentCreated, protocol.EventCommentUpdated, protocol.EventCommentResolved, protocol.EventCommentUnresolved} {
		t.Run(typ, func(t *testing.T) {
			wantDerivedAudience(t, typ, func(issueID, secret string) map[string]any {
				return map[string]any{"comment": map[string]any{"id": "c1", "issue_id": issueID, "content": secret}}
			})
		})
	}
	t.Run(protocol.EventCommentDeleted, func(t *testing.T) {
		wantDerivedAudience(t, protocol.EventCommentDeleted, func(issueID, secret string) map[string]any {
			return map[string]any{"comment_id": "c1", "issue_id": issueID, "note": secret}
		})
	})
}

func TestRestrictedReactionEventReachesOnlyGranted(t *testing.T) {
	for _, typ := range []string{protocol.EventReactionAdded, protocol.EventReactionRemoved, protocol.EventIssueReactionAdded, protocol.EventIssueReactionRemoved} {
		t.Run(typ, func(t *testing.T) {
			wantDerivedAudience(t, typ, func(issueID, secret string) map[string]any {
				return map[string]any{"issue_id": issueID, "issue_title": secret, "reaction": map[string]any{"emoji": "x"}}
			})
		})
	}
}

func TestRestrictedActivityEventReachesOnlyGranted(t *testing.T) {
	wantDerivedAudience(t, protocol.EventActivityCreated, func(issueID, secret string) map[string]any {
		return map[string]any{"issue_id": issueID, "entry": map[string]any{"type": "activity", "action": secret}}
	})
}

func TestRestrictedSubscriberEventReachesOnlyGranted(t *testing.T) {
	for _, typ := range []string{protocol.EventSubscriberAdded, protocol.EventSubscriberRemoved} {
		t.Run(typ, func(t *testing.T) {
			wantDerivedAudience(t, typ, func(issueID, secret string) map[string]any {
				return map[string]any{"issue_id": issueID, "user_type": "member", "user_id": secret}
			})
		})
	}
}

func TestRestrictedLabelEventReachesOnlyGranted(t *testing.T) {
	wantDerivedAudience(t, protocol.EventIssueLabelsChanged, func(issueID, secret string) map[string]any {
		return map[string]any{"issue_id": issueID, "labels": []map[string]any{{"name": secret}}}
	})
}

// An issue the database does not know has no knowable audience: the event is
// dropped for everyone, never widened to the workspace.
func TestDerivedEventWithUnknownIssueIsDropped(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "member")
	w.publishDerived(protocol.EventCommentCreated, map[string]any{
		"comment": map[string]any{"id": "c1", "issue_id": "00000000-0000-4000-8000-000000000999", "content": "x"},
	})
	for _, name := range []string{"owner", "member"} {
		w.wantGot(name, w.received(name), protocol.EventCommentCreated, false)
	}
}

func TestProjectDeletedReachesOnlyGranted(t *testing.T) {
	w := newRestrictedWorld(t, "owner", "admin", "member", "other", "guest", "otherGuest")
	restricted := w.project("Doomed restricted", "restricted", "member", "guest")
	w.api("owner", "DELETE", "/api/projects/"+restricted, nil)
	for name, want := range map[string]bool{
		"owner": true, "admin": true, "member": true, "guest": true,
		"other": false, "otherGuest": false,
	} {
		w.wantGot(name, w.received(name), protocol.EventProjectDeleted, want)
	}

	open := w.project("Doomed open", "workspace", "guest")
	w.api("owner", "DELETE", "/api/projects/"+open, nil)
	for name, want := range map[string]bool{
		"owner": true, "admin": true, "member": true, "other": true, "guest": true,
		"otherGuest": false,
	} {
		w.wantGot(name, w.received(name), protocol.EventProjectDeleted, want)
	}
}
