package service

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// maxSquadRequestHops bounds the squad_request parent walk. Hand-offs normally
// nest one or two levels deep; the bound stops a corrupt origin cycle from
// looping forever.
const maxSquadRequestHops = 8

// squadRequestRootIssue follows squad_request origins up to the first issue that
// is not itself a squad hand-off. It returns false when the chain is too deep, a
// parent is missing, or a parent lies outside the issue's workspace (MUL-4252).
func (s *TaskService) squadRequestRootIssue(ctx context.Context, issue db.Issue) (db.Issue, bool) {
	cur := issue
	for hop := 0; hop < maxSquadRequestHops; hop++ {
		if !cur.OriginType.Valid || cur.OriginType.String != "squad_request" || !cur.OriginID.Valid {
			return cur, true
		}
		parent, err := s.Queries.GetIssue(ctx, cur.OriginID)
		if err != nil || parent.WorkspaceID != issue.WorkspaceID {
			return db.Issue{}, false
		}
		cur = parent
	}
	return db.Issue{}, false
}

type chatRouteKey struct{}

// WithChatRoute carries the route the platform chose for a Secretary chat turn
// (see ChatRouteFor) down to the send paths that do not take it as an argument.
func WithChatRoute(ctx context.Context, route string) context.Context {
	return context.WithValue(ctx, chatRouteKey{}, route)
}

func chatRouteFromContext(ctx context.Context) string {
	route, _ := ctx.Value(chatRouteKey{}).(string)
	return route
}

// BranchMerge is the outcome of a completed run's opt-in merge into the user's
// checked-out branch. The zero value means the run did not ask for one.
type BranchMerge struct {
	Status string
	Error  string
}

// FailedRunLandingStatus is the status an in_progress issue takes when its run
// failed for good and no retry or other run follows: blocked, the status for
// work that cannot continue until someone acts. A workflow that does not list
// blocked keeps the status, and the failure comment says what happened. Nothing
// is queued for the landed status, so no run starts on its own.
func FailedRunLandingStatus(ctx context.Context, q *db.Queries, issue db.Issue) string {
	def, _, err := issueworkflow.Resolve(ctx, q, issueworkflow.RoomIssueOf(issue))
	if err != nil {
		return ""
	}
	if def != nil && issueworkflow.CheckStatus(def, issuestatus.Blocked) != nil {
		return ""
	}
	return issuestatus.Blocked
}

// taskEventScopeTimeout bounds the workspace lookup for an event whose
// transaction has already committed. The caller's context may be spent by then.
const taskEventScopeTimeout = 2 * time.Second

// PublishChatReplyDone announces a Secretary reply on the run that wrote it, so
// the channels bound to the session (Telegram, WeCom and the rest) deliver it.
// Only the caller decides that a route exists (see ChatReplyRoute); a reply to a
// session with no channel binding is not announced here.
func (s *TaskService) PublishChatReplyDone(ctx context.Context, run db.AgentTaskQueue, msg db.ChatMessage) {
	s.broadcastChatDone(ctx, run, &msg, false)
}

// runAfterIDStrings renders a queued issue's waits for the websocket payload,
// always as an array so clients can read it without a null check.
func runAfterIDStrings(ids []pgtype.UUID) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, util.UUIDToString(id))
	}
	return out
}
