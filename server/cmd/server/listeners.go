package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/projectaccess"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// internalOnlyPayloadKeys lists payload keys that exist purely for in-process
// listeners and must never be serialized to a WebSocket client.
//
// `issue:updated` carries prev_description and prev_title so the in-process
// listeners can diff against the new values: subscriber_listeners.go adds newly
// @mentioned users, notification_listeners.go builds mention notifications, and
// activity_listeners.go records the title change. Those all run on
// bus.Subscribe, which Publish dispatches BEFORE the SubscribeAll forwarder
// below, so removing the keys on the way out cannot affect them.
//
// No client reads either key — IssueUpdatedPayload in
// packages/core/types/events.ts does not declare them. They reached the wire
// only because the forwarder reuses the producer's payload map verbatim, which
// meant every description autosave broadcast TWO full copies of the description
// (the new one inside `issue`, plus prev_description) to every connection in the
// workspace, including users who did not have the issue open. The DB write is
// O(1); the fanout was O(workspace connections × description size) (MUL-5492).
//
// This is a table rather than an `if` on one event type because the bug was
// structural, not a typo: the next large field added to a published payload
// inherits the same cost silently. Keeping the list declarative puts the
// internal/external payload boundary in one reviewable place.
var internalOnlyPayloadKeys = map[string][]string{
	// prev_project_id routes a move between projects (routeIssueMove).
	protocol.EventIssueUpdated: {"prev_description", "prev_title", "prev_project_id"},
	// recipient_user_ids is who could see the project before the change; the
	// client gets only project_id and access.
	protocol.EventProjectAccessChanged: {"recipient_user_ids"},
	// project:deleted has no grants left to read; the handler carries the
	// audience computed before the delete.
	protocol.EventProjectDeleted: {"recipient_workspace", "recipient_user_ids"},
	// task:failed error text is consumed synchronously by channel outbounds.
	// It may contain provider/runtime detail that belongs in the originating
	// chat transcript, not in the workspace-wide realtime fanout.
	protocol.EventTaskFailed: {"error"},
}

// projectOutbound returns payload with the event type's internal-only keys
// removed, ready to serialize for external consumers.
//
// The input map is never mutated. In-process listeners have already run by the
// time this is called, but the producer still owns the map and a second
// forwarder may yet read it, so mutating it in place would be a landmine.
func projectOutbound(eventType string, payload any) any {
	keys := internalOnlyPayloadKeys[eventType]
	if len(keys) == 0 {
		return payload
	}
	m, ok := payload.(map[string]any)
	if !ok {
		return payload
	}
	projected := make(map[string]any, len(m))
	for k, v := range m {
		projected[k] = v
	}
	for _, k := range keys {
		delete(projected, k)
	}
	return projected
}

// isChatPrivateEvent reports whether e carries chat session content that only
// the session's creator may receive. Every chat:* event qualifies, including
// ones added later. Of the task events only task:message does: it streams the
// agent's transcript. Chat-task lifecycle events (task:queued, task:completed,
// ...) keep workspace fanout because every member's agent presence and
// workload views depend on them; they carry ids and status, and task:failed
// has its error text stripped by projectOutbound.
func isChatPrivateEvent(e events.Event) bool {
	if strings.HasPrefix(e.Type, "chat:") {
		return true
	}
	return e.Type == protocol.EventTaskMessage && e.ChatSessionID != ""
}

// memberPrivateEvents never leave their member, so one published without
// MemberPrivateUserID is dropped instead of reaching the workspace.
var memberPrivateEvents = map[string]bool{
	protocol.EventIssueViewPreferenceChanged: true,
}

// projectScopedEvents carry an issue or a project, so their recipients depend
// on the project's audience (spec 7.1 rule 13). Other events keep workspace
// fanout.
var projectScopedEvents = map[string]bool{
	protocol.EventIssueCreated:           true,
	protocol.EventIssueUpdated:           true,
	protocol.EventIssueDeleted:           true,
	protocol.EventProjectCreated:         true,
	protocol.EventProjectUpdated:         true,
	protocol.EventProjectDeleted:         true,
	protocol.EventProjectResourceCreated: true,
	protocol.EventProjectResourceUpdated: true,
	protocol.EventProjectResourceDeleted: true,
}

// eventAudiences answers who receives the events of a project.
type eventAudiences interface {
	Audience(ctx context.Context, workspaceID, projectID pgtype.UUID) (projectaccess.Recipients, error)
	// Users lists every person who sees the project.
	Users(ctx context.Context, workspaceID, projectID pgtype.UUID) ([]string, error)
	// IssueProject is the project of an issue; invalid when it has none.
	IssueProject(ctx context.Context, issueID pgtype.UUID) (pgtype.UUID, error)
}

type queryAudiences struct{ q *db.Queries }

func (a queryAudiences) Audience(ctx context.Context, ws, project pgtype.UUID) (projectaccess.Recipients, error) {
	return projectaccess.Audience(ctx, a.q, ws, project)
}

func (a queryAudiences) Users(ctx context.Context, ws, project pgtype.UUID) ([]string, error) {
	return projectaccess.AudienceUsers(ctx, a.q, ws, project)
}

func (a queryAudiences) IssueProject(ctx context.Context, issueID pgtype.UUID) (pgtype.UUID, error) {
	return a.q.GetIssueProjectID(ctx, issueID)
}

const audienceLookupTimeout = 5 * time.Second

// projectRef is what an event says about the project it belongs to.
type projectRef struct {
	project pgtype.UUID // invalid: no project
	issueID string
	issue   json.RawMessage
	// prev is the project an issue moved out of. hasPrev is false when the
	// event says nothing about a move.
	prev    pgtype.UUID
	hasPrev bool
}

func parseProjectRef(e events.Event) (projectRef, error) {
	raw, err := json.Marshal(e.Payload)
	if err != nil {
		return projectRef{}, err
	}
	var p struct {
		Issue   json.RawMessage `json:"issue"`
		IssueID string          `json:"issue_id"`
		Project *struct {
			ID string `json:"id"`
		} `json:"project"`
		ProjectID     *string         `json:"project_id"`
		PrevProjectID json.RawMessage `json:"prev_project_id"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		return projectRef{}, err
	}
	ref := projectRef{issueID: p.IssueID, issue: p.Issue}
	project := p.ProjectID
	if len(p.Issue) > 0 {
		var issue struct {
			ID        string  `json:"id"`
			ProjectID *string `json:"project_id"`
		}
		if err := json.Unmarshal(p.Issue, &issue); err != nil {
			return projectRef{}, err
		}
		ref.issueID, project = issue.ID, issue.ProjectID
	} else if p.Project != nil {
		project = &p.Project.ID
	}
	if project != nil && *project != "" {
		if ref.project, err = util.ParseUUID(*project); err != nil {
			return projectRef{}, err
		}
	}
	if len(p.PrevProjectID) > 0 {
		ref.hasPrev = true
		var prev *string
		if err := json.Unmarshal(p.PrevProjectID, &prev); err != nil {
			return projectRef{}, err
		}
		if prev != nil && *prev != "" {
			if ref.prev, err = util.ParseUUID(*prev); err != nil {
				return projectRef{}, err
			}
		}
	}
	return ref, nil
}

// routeProjectEvent delivers an issue or project event to the project's
// audience. It never widens: an audience that cannot be computed drops the
// event.
func routeProjectEvent(b realtime.Broadcaster, aud eventAudiences, e events.Event, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), audienceLookupTimeout)
	defer cancel()
	drop := func(err error) {
		slog.Warn("realtime: dropping project event, audience unknown", "event_type", e.Type, "error", err)
	}
	ws, err := util.ParseUUID(e.WorkspaceID)
	if err != nil {
		drop(err)
		return
	}
	ref, err := parseProjectRef(e)
	if err != nil {
		drop(err)
		return
	}
	// An issue event that names the issue but not its project asks the
	// database, so a slim payload is not treated as "no project".
	if len(ref.issue) == 0 && !ref.project.Valid && ref.issueID != "" && strings.HasPrefix(e.Type, "issue:") && e.Type != protocol.EventIssueDeleted {
		id, err := util.ParseUUID(ref.issueID)
		if err == nil {
			ref.project, err = aud.IssueProject(ctx, id)
		}
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			drop(err)
			return
		}
	}
	if e.Type == protocol.EventIssueUpdated && ref.hasPrev && ref.prev != ref.project {
		if err := routeIssueMove(ctx, b, aud, e, ws, ref, data); err != nil {
			drop(err)
		}
		return
	}
	var r projectaccess.Recipients
	if e.Type == protocol.EventProjectDeleted {
		// The row is gone, and with it the project's grants: use the audience
		// the handler read before deleting.
		var ok bool
		if r, ok = deletedProjectRecipients(e.Payload); !ok {
			drop(errors.New("project:deleted without recipients"))
			return
		}
	} else if r, err = aud.Audience(ctx, ws, ref.project); err != nil {
		drop(err)
		return
	}
	realtime.M.RecordEvent(e.Type)
	sendToRecipients(b, e.WorkspaceID, r, data)
}

// derivedIssueID returns the issue a derived event (comment, reaction,
// activity, subscriber, label, task, ...) is about, read from the payload's
// issue_id or, for comment events, the comment's issue_id. ok is false when
// the payload names no issue.
func derivedIssueID(payload any) (id string, ok bool, err error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return "", false, err
	}
	var p struct {
		IssueID string `json:"issue_id"`
		Comment *struct {
			IssueID string `json:"issue_id"`
		} `json:"comment"`
	}
	if err := json.Unmarshal(raw, &p); err != nil {
		// A payload that is not an object names no issue.
		return "", false, nil
	}
	if p.IssueID == "" && p.Comment != nil {
		p.IssueID = p.Comment.IssueID
	}
	return p.IssueID, p.IssueID != "", nil
}

// routeDerivedEvent delivers an event about an issue to the audience of the
// issue's project. An issue with no project keeps workspace routing. An issue
// whose project cannot be determined drops the event.
func routeDerivedEvent(b realtime.Broadcaster, aud eventAudiences, e events.Event, issueID string, data []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), audienceLookupTimeout)
	defer cancel()
	drop := func(err error) {
		slog.Warn("realtime: dropping issue event, audience unknown", "event_type", e.Type, "issue_id", issueID, "error", err)
	}
	ws, err := util.ParseUUID(e.WorkspaceID)
	if err != nil {
		drop(err)
		return
	}
	id, err := util.ParseUUID(issueID)
	if err != nil {
		drop(err)
		return
	}
	project, err := aud.IssueProject(ctx, id)
	if err != nil {
		drop(err)
		return
	}
	r, err := aud.Audience(ctx, ws, project)
	if err != nil {
		drop(err)
		return
	}
	realtime.M.RecordEvent(e.Type)
	sendToRecipients(b, e.WorkspaceID, r, data)
}

// deletedProjectRecipients reads the pre-delete audience from a project:deleted
// payload. ok is false when the payload carries none.
func deletedProjectRecipients(payload any) (r projectaccess.Recipients, ok bool) {
	m, isMap := payload.(map[string]any)
	if !isMap {
		return r, false
	}
	workspace, hasWorkspace := m["recipient_workspace"].(bool)
	users, hasUsers := m["recipient_user_ids"].([]string)
	if !hasWorkspace || !hasUsers {
		return r, false
	}
	return projectaccess.Recipients{Workspace: workspace, UserIDs: users}, true
}

// sendToRecipients sends data to the workspace room when r says so, and to
// each listed person's member room.
func sendToRecipients(b realtime.Broadcaster, workspaceID string, r projectaccess.Recipients, data []byte) {
	if r.Workspace {
		b.BroadcastToWorkspace(workspaceID, data)
	}
	for _, uid := range r.UserIDs {
		b.BroadcastToScope(realtime.ScopeWorkspaceMember, realtime.WorkspaceMemberScopeID(workspaceID, uid), data)
	}
}

// routeIssueMove sends an issue that changed project to both audiences. A
// person in both gets the update, a person only in the new audience gets the
// issue as created, and a person only in the old one gets it as deleted.
func routeIssueMove(ctx context.Context, b realtime.Broadcaster, aud eventAudiences, e events.Event, ws pgtype.UUID, ref projectRef, updated []byte) error {
	oldR, err := aud.Audience(ctx, ws, ref.prev)
	if err != nil {
		return err
	}
	newR, err := aud.Audience(ctx, ws, ref.project)
	if err != nil {
		return err
	}
	oldUsers, newUsers := oldR.UserIDs, newR.UserIDs
	sharedRoom := oldR.Workspace && newR.Workspace
	if !sharedRoom {
		// A workspace room cannot be split, so compare person by person.
		if oldUsers, err = aud.Users(ctx, ws, ref.prev); err != nil {
			return err
		}
		if newUsers, err = aud.Users(ctx, ws, ref.project); err != nil {
			return err
		}
	}
	createdPayload := map[string]any{"issue_id": ref.issueID}
	if len(ref.issue) > 0 {
		createdPayload = map[string]any{"issue": ref.issue}
	}
	frame := func(typ string, payload any) []byte {
		out, err := json.Marshal(map[string]any{"type": typ, "payload": payload, "actor_id": e.ActorID, "actor_type": e.ActorType})
		if err != nil {
			return nil
		}
		return out
	}
	created := frame(protocol.EventIssueCreated, createdPayload)
	deleted := frame(protocol.EventIssueDeleted, map[string]any{"issue_id": ref.issueID})
	if created == nil || deleted == nil {
		return errors.New("marshal move frames")
	}

	realtime.M.RecordEvent(e.Type)
	if sharedRoom {
		b.BroadcastToWorkspace(e.WorkspaceID, updated)
	}
	inOld := make(map[string]bool, len(oldUsers))
	for _, u := range oldUsers {
		inOld[u] = true
	}
	inNew := make(map[string]bool, len(newUsers))
	for _, u := range newUsers {
		inNew[u] = true
		data := created
		if inOld[u] {
			data = updated
		}
		b.BroadcastToScope(realtime.ScopeWorkspaceMember, realtime.WorkspaceMemberScopeID(e.WorkspaceID, u), data)
	}
	for _, u := range oldUsers {
		if !inNew[u] {
			b.BroadcastToScope(realtime.ScopeWorkspaceMember, realtime.WorkspaceMemberScopeID(e.WorkspaceID, u), deleted)
		}
	}
	return nil
}

// routeAccessChanged sends project:access_changed to the people who could see
// the project before the change. Without a recipient list the event is
// dropped.
func routeAccessChanged(b realtime.Broadcaster, e events.Event, data []byte) {
	payload, _ := e.Payload.(map[string]any)
	recipients, _ := payload["recipient_user_ids"].([]string)
	if len(recipients) == 0 || e.WorkspaceID == "" {
		slog.Warn("realtime: dropping project:access_changed without recipients")
		return
	}
	realtime.M.RecordEvent(e.Type)
	sendToRecipients(b, e.WorkspaceID, projectaccess.Recipients{UserIDs: recipients}, data)
}

// registerListeners wires up event bus listeners for WS broadcasting.
// Personal events (inbox, invites) are sent only to the target user via
// SendToUser, chat content only to the chat session's creator, and
// member-private events only to that member's connections to the workspace.
// All other events are broadcast to the workspace room.
//
// The broadcaster parameter is intentionally typed as the realtime.Broadcaster
// interface (not *realtime.Hub) so that this layer can later be swapped out
// for a Redis-backed relay or a feature-flagged dual-write implementation
// without touching any of the event listeners below. This is Phase 0 of the
// horizontal-scaling plan tracked in MUL-1138.
func registerListeners(bus *events.Bus, b realtime.Broadcaster, audiences eventAudiences) {
	// Personal events should NOT be broadcast to the whole workspace.
	personalEvents := map[string]bool{
		protocol.EventInboxNew:           true,
		protocol.EventInboxRead:          true,
		protocol.EventInboxArchived:      true,
		protocol.EventInboxUnarchived:    true,
		protocol.EventInboxBatchRead:     true,
		protocol.EventInboxBatchArchived: true,
		protocol.EventInvitationCreated:  true,
		protocol.EventInvitationRevoked:  true,
		protocol.EventIssueUnreadChanged: true,
	}

	// Helper: marshal event and send to a specific user.
	sendToRecipient := func(b realtime.Broadcaster, e events.Event, recipientID string) {
		if recipientID == "" {
			return
		}
		data, err := json.Marshal(map[string]any{"type": e.Type, "payload": projectOutbound(e.Type, e.Payload), "actor_id": e.ActorID, "actor_type": e.ActorType})
		if err != nil {
			return
		}
		realtime.M.RecordEvent(e.Type)
		b.SendToUser(recipientID, data)
	}

	// inbox:new — extract recipient from nested item
	bus.Subscribe(protocol.EventInboxNew, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		item, ok := payload["item"].(map[string]any)
		if !ok {
			return
		}
		recipientID, _ := item["recipient_id"].(string)
		sendToRecipient(b, e, recipientID)
	})

	// inbox:read, inbox:archived, inbox:unarchived, inbox:batch-read,
	// inbox:batch-archived, issue_unread:changed — extract recipient from
	// top-level payload
	for _, eventType := range []string{
		protocol.EventInboxRead, protocol.EventInboxArchived, protocol.EventInboxUnarchived,
		protocol.EventInboxBatchRead, protocol.EventInboxBatchArchived,
		protocol.EventIssueUnreadChanged,
	} {
		bus.Subscribe(eventType, func(e events.Event) {
			payload, ok := e.Payload.(map[string]any)
			if !ok {
				return
			}
			recipientID, _ := payload["recipient_id"].(string)
			sendToRecipient(b, e, recipientID)
		})
	}

	// invitation:created — send to the invitee so they see the invitation in real time.
	bus.Subscribe(protocol.EventInvitationCreated, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		inv, ok := payload["invitation"].(handler.InvitationResponse)
		if !ok {
			// Fallback for map encoding.
			if invMap, ok := payload["invitation"].(map[string]any); ok {
				if uid, _ := invMap["invitee_user_id"].(*string); uid != nil && *uid != "" {
					data, err := json.Marshal(map[string]any{"type": e.Type, "payload": projectOutbound(e.Type, e.Payload), "actor_id": e.ActorID, "actor_type": e.ActorType})
					if err != nil {
						return
					}
					realtime.M.RecordEvent(e.Type)
					b.SendToUser(*uid, data)
				}
			}
			return
		}
		if inv.InviteeUserID != nil && *inv.InviteeUserID != "" {
			data, err := json.Marshal(map[string]any{"type": e.Type, "payload": projectOutbound(e.Type, e.Payload), "actor_id": e.ActorID, "actor_type": e.ActorType})
			if err != nil {
				return
			}
			realtime.M.RecordEvent(e.Type)
			b.SendToUser(*inv.InviteeUserID, data)
		}
	})

	// invitation:revoked — send to the invitee so their pending list updates.
	bus.Subscribe(protocol.EventInvitationRevoked, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		uid, _ := payload["invitee_user_id"].(*string)
		if uid != nil && *uid != "" {
			sendToRecipient(b, e, *uid)
		}
	})

	// invitation:accepted / invitation:declined — also send to the invitee so
	// their pending list updates. The actor is the invitee on every producer
	// path, but they are usually NOT in the workspace room yet: a client binds
	// its socket to the workspace it currently has open, so a user concluding
	// an invitation with no workspace open (or a different one open) never
	// receives the broadcast below and their stale pending row survives until
	// restart (#8432). Pass excludeWorkspace so clients already in the room
	// (reached via BroadcastToWorkspace in SubscribeAll) don't get it twice.
	// invitation:revoked keeps its invitee_user_id routing above: its actor is
	// the revoking admin, not the affected invitee.
	for _, eventType := range []string{protocol.EventInvitationAccepted, protocol.EventInvitationDeclined} {
		bus.Subscribe(eventType, func(e events.Event) {
			if e.ActorID == "" {
				return
			}
			data, err := json.Marshal(map[string]any{"type": e.Type, "payload": projectOutbound(e.Type, e.Payload), "actor_id": e.ActorID, "actor_type": e.ActorType})
			if err != nil {
				return
			}
			realtime.M.RecordEvent(e.Type)
			b.SendToUser(e.ActorID, data, e.WorkspaceID)
		})
	}

	// member:added — also send to the invited user so they discover the new workspace.
	// Pass excludeWorkspace so clients already in the target room (reached via
	// BroadcastToWorkspace in SubscribeAll) don't receive the event twice.
	bus.Subscribe(protocol.EventMemberAdded, func(e events.Event) {
		payload, ok := e.Payload.(map[string]any)
		if !ok {
			return
		}
		var userID string
		switch m := payload["member"].(type) {
		case handler.MemberWithUserResponse:
			userID = m.UserID
		case map[string]any:
			userID, _ = m["user_id"].(string)
		default:
			slog.Warn("member:added: unexpected member payload type", "type", fmt.Sprintf("%T", payload["member"]))
		}
		if userID == "" {
			return
		}
		data, err := json.Marshal(map[string]any{"type": e.Type, "payload": projectOutbound(e.Type, e.Payload), "actor_id": e.ActorID, "actor_type": e.ActorType})
		if err != nil {
			return
		}
		realtime.M.RecordEvent(e.Type)
		b.SendToUser(userID, data, e.WorkspaceID)
	})

	// SubscribeAll handles workspace-broadcast for non-personal events.
	bus.SubscribeAll(func(e events.Event) {
		// Skip personal events — they are handled by type-specific listeners above.
		if personalEvents[e.Type] {
			return
		}

		// A chat session is creator-private: its events and its tasks'
		// transcript go only to the creator's connections bound to the
		// session's workspace. Not the whole user scope: a connection to
		// another workspace is authorized against that workspace, so it would
		// keep receiving this content after the creator is removed from this
		// one. The scope is joined at connect time, so this needs no client
		// subscribe frame. Without a known owner and workspace the event is
		// dropped, never widened.
		if isChatPrivateEvent(e) {
			if e.ChatSessionOwnerID == "" || e.WorkspaceID == "" {
				slog.Warn("realtime: dropping chat event without session owner",
					"event_type", e.Type, "chat_session_id", e.ChatSessionID, "task_id", e.TaskID)
				return
			}
			data, err := json.Marshal(map[string]any{"type": e.Type, "payload": projectOutbound(e.Type, e.Payload), "actor_id": e.ActorID, "actor_type": e.ActorType})
			if err != nil {
				slog.Error("failed to marshal event", "event_type", e.Type, "error", err)
				return
			}
			realtime.M.RecordEvent(e.Type)
			b.BroadcastToScope(realtime.ScopeWorkspaceMember, realtime.WorkspaceMemberScopeID(e.WorkspaceID, e.ChatSessionOwnerID), data)
			return
		}

		msg := map[string]any{
			"type":       e.Type,
			"payload":    projectOutbound(e.Type, e.Payload),
			"actor_id":   e.ActorID,
			"actor_type": e.ActorType,
		}
		data, err := json.Marshal(msg)
		if err != nil {
			slog.Error("failed to marshal event", "event_type", e.Type, "error", err)
			return
		}

		if e.MemberPrivateUserID != "" || memberPrivateEvents[e.Type] {
			if e.MemberPrivateUserID == "" || e.WorkspaceID == "" {
				slog.Warn("realtime: dropping member-private event without recipient", "event_type", e.Type)
				return
			}
			realtime.M.RecordEvent(e.Type)
			b.BroadcastToScope(realtime.ScopeWorkspaceMember, realtime.WorkspaceMemberScopeID(e.WorkspaceID, e.MemberPrivateUserID), data)
			return
		}

		// Issue and project events go to the project's audience: the workspace
		// room for a project everyone sees, each person's member room for a
		// restricted one. Guests are not in the workspace room.
		if e.Type == protocol.EventProjectAccessChanged {
			routeAccessChanged(b, e, data)
			return
		}
		if projectScopedEvents[e.Type] && e.WorkspaceID != "" {
			routeProjectEvent(b, audiences, e, data)
			return
		}

		// Events about an issue (comments, reactions, activity, subscribers,
		// labels) follow the issue's project.
		if e.WorkspaceID != "" {
			issueID, ok, err := derivedIssueID(e.Payload)
			if err != nil {
				slog.Warn("realtime: dropping event with unreadable payload", "event_type", e.Type, "error", err)
				return
			}
			if ok {
				routeDerivedEvent(b, audiences, e, issueID, data)
				return
			}
		}

		// Phase 1 (MUL-1138): the per-resource scope routing for high-frequency
		// task/chat events is intentionally NOT enabled yet. The server-side
		// pieces — Hub.subscribe/unsubscribe protocol, ScopeAuthorizer, Redis
		// Streams relay — have all landed, but the client (WSClient + the
		// per-page chat/task hooks) does not yet send `subscribe` frames or
		// replay subscriptions on reconnect. Routing these events through
		// `BroadcastToScope("task"|"chat", ...)` today would silently drop
		// every chat/task message on the floor, breaking the live chat
		// timeline, chat unread badges, and pending-task UI.
		//
		// Until the client lands its scope-subscription PR, task events stay
		// on workspace fanout, except chat-task transcripts, which take the
		// creator-only user-scope route above. The `Event.TaskID` /
		// `Event.ChatSessionID` hints are still populated by producers so
		// that flipping the switch later is a one-line change here. See
		// review on PR #1429 for context.

		if e.WorkspaceID != "" {
			realtime.M.RecordEvent(e.Type)
			b.BroadcastToWorkspace(e.WorkspaceID, data)
		} else if strings.HasPrefix(e.Type, "daemon:") {
			realtime.M.RecordEvent(e.Type)
			b.Broadcast(data)
		}
		// Otherwise drop — no global broadcast for non-daemon events without a workspace.
	})
}
