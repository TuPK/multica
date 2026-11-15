package handler

import (
	"encoding/base64"
	"encoding/csv"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/util"
)

const (
	auditDefaultLimit = 50
	auditMaxLimit     = 100
	// auditCSVMaxRows caps one CSV export. The query asks for one more row to
	// learn whether the cap cut the result off.
	auditCSVMaxRows = 10000
)

// auditActionCatalog lists every action name the audit package defines. The
// action filter accepts exactly these.
var auditActionCatalog = []string{
	audit.ActionMemberRoleChanged,
	audit.ActionMemberRemoved,
	audit.ActionMemberLeft,
	audit.ActionInvitationCreated,
	audit.ActionInvitationRevoked,
	audit.ActionInvitationAccepted,
	audit.ActionInvitationDeclined,
	audit.ActionShareLinkCreated,
	audit.ActionShareLinkRevoked,
	audit.ActionShareLinkJoined,
	audit.ActionWorkspaceCreated,
	audit.ActionWorkspaceUpdated,
	audit.ActionWorkspaceDeleted,
	audit.ActionAuthLogin,
	audit.ActionAuthSessionsRevoked,
	audit.ActionPATCreated,
	audit.ActionPATRevoked,
	audit.ActionAccountExportRequested,
	audit.ActionAccountDeletionRequest,
	audit.ActionAccountDeletionCanceled,
	audit.ActionConsentAccepted,
}

func isAuditAction(s string) bool {
	for _, a := range auditActionCatalog {
		if a == s {
			return true
		}
	}
	return false
}

// auditParamError is a bad query parameter, reported as a 400 with Code.
type auditParamError struct {
	code string
	msg  string
}

func (e *auditParamError) Error() string { return e.code + ": " + e.msg }

func invalidFilter(msg string) *auditParamError {
	return &auditParamError{code: "invalid_filter", msg: msg}
}

// auditCursor is the (created_at, id) of the last row of a page.
type auditCursor struct {
	createdAt time.Time
	id        string
}

func encodeAuditCursor(createdAt time.Time, id string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeAuditCursor(s string) (auditCursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return auditCursor{}, err
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return auditCursor{}, errors.New("malformed cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return auditCursor{}, err
	}
	if _, err := util.ParseUUID(id); err != nil {
		return auditCursor{}, err
	}
	return auditCursor{createdAt: at, id: id}, nil
}

// auditListParams is the parsed query string of an audit list request.
type auditListParams struct {
	limit   int
	cursor  *auditCursor
	action  *string
	actorID *string
	from    *time.Time
	to      *time.Time
	csv     bool
}

// parseAuditListParams validates the query. With workspace false only limit and
// cursor are read; the account endpoint ignores the rest.
func parseAuditListParams(q url.Values, workspace bool) (auditListParams, *auditParamError) {
	p := auditListParams{limit: auditDefaultLimit}
	if q.Has("limit") {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil || n < 1 || n > auditMaxLimit {
			return p, invalidFilter("limit must be an integer from 1 to 100")
		}
		p.limit = n
	}
	if q.Has("cursor") {
		cur, err := decodeAuditCursor(q.Get("cursor"))
		if err != nil {
			return p, &auditParamError{code: "invalid_cursor", msg: "invalid cursor"}
		}
		p.cursor = &cur
	}
	if !workspace {
		return p, nil
	}
	if q.Has("action") {
		a := q.Get("action")
		if !isAuditAction(a) {
			return p, invalidFilter("unknown action")
		}
		p.action = &a
	}
	if q.Has("actor_id") {
		a := q.Get("actor_id")
		if a == "" {
			return p, invalidFilter("actor_id must not be empty")
		}
		p.actorID = &a
	}
	for _, f := range []struct {
		name string
		dst  **time.Time
	}{{"from", &p.from}, {"to", &p.to}} {
		if !q.Has(f.name) {
			continue
		}
		t, err := time.Parse(time.RFC3339, q.Get(f.name))
		if err != nil {
			return p, invalidFilter(f.name + " must be an RFC 3339 timestamp")
		}
		t = t.UTC()
		*f.dst = &t
	}
	if p.from != nil && p.to != nil && p.from.After(*p.to) {
		return p, invalidFilter("from must not be after to")
	}
	if q.Has("format") {
		switch q.Get("format") {
		case "json":
		case "csv":
			p.csv = true
		default:
			return p, invalidFilter("format must be json or csv")
		}
	}
	return p, nil
}

// auditEventJSON is the wire shape of one audit event.
type auditEventJSON struct {
	ID          string          `json:"id"`
	WorkspaceID *string         `json:"workspace_id"`
	ActorType   string          `json:"actor_type"`
	ActorID     *string         `json:"actor_id"`
	ActorLabel  string          `json:"actor_label"`
	Action      string          `json:"action"`
	TargetType  string          `json:"target_type"`
	TargetID    string          `json:"target_id"`
	TargetLabel string          `json:"target_label"`
	Detail      json.RawMessage `json:"detail"`
	IP          string          `json:"ip"`
	UserAgent   string          `json:"user_agent"`
	RequestID   string          `json:"request_id"`
	CreatedAt   string          `json:"created_at"`
}

var auditCSVHeader = []string{
	"id", "workspace_id", "actor_type", "actor_id", "actor_label", "action",
	"target_type", "target_id", "target_label", "detail", "ip", "user_agent",
	"request_id", "created_at",
}

// csvCell neutralises spreadsheet formulas: labels, user agents and detail
// carry text chosen by other people, and a cell that starts with one of these
// characters is evaluated by Excel and Sheets.
func csvCell(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

func derefOrEmpty(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// writeAuditCSV writes the header and one row per event, then flushes.
func writeAuditCSV(w *csv.Writer, events []auditEventJSON) error {
	if err := w.Write(auditCSVHeader); err != nil {
		return err
	}
	for _, e := range events {
		row := []string{
			e.ID, derefOrEmpty(e.WorkspaceID), e.ActorType, derefOrEmpty(e.ActorID),
			csvCell(e.ActorLabel), e.Action, e.TargetType, csvCell(e.TargetID),
			csvCell(e.TargetLabel), csvCell(string(e.Detail)), csvCell(e.IP),
			csvCell(e.UserAgent), e.RequestID, e.CreatedAt,
		}
		if err := w.Write(row); err != nil {
			return err
		}
	}
	w.Flush()
	return w.Error()
}
