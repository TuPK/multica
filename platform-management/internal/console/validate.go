package console

import (
	"encoding/base64"
	"errors"
	"net/mail"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/multica-ai/multica/platform-management/accountguard"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/internal/staff"
	"github.com/multica-ai/multica/platform-management/internal/workspacestatus"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const (
	// maxReasonChars bounds a status or account reason; it is stored on the
	// state row and copied into the audit detail.
	maxReasonChars = 1000
	// maxNoteChars is the contract's limit on an internal note.
	maxNoteChars  = 4000
	maxEmailChars = 254

	defaultLimit = 50
	maxLimit     = 100
)

// apiError is a refusal the caller sees: the HTTP status, a stable code and a
// message. Handlers return it; they never leak an internal error.
type apiError struct {
	Status  int
	Code    string
	Message string
}

func (e *apiError) Error() string { return e.Code + ": " + e.Message }

func badRequest(code, msg string) *apiError {
	return &apiError{Status: 400, Code: code, Message: msg}
}

func invalidFilter(msg string) *apiError { return badRequest("invalid_filter", msg) }

// hasNUL reports whether s has a NUL character, which Postgres text and jsonb
// reject; letting one through would be a 500.
func hasNUL(s string) bool { return strings.IndexByte(s, 0) >= 0 }

// cleanReason trims reason and checks it. A reason is required for every
// status change and account change.
func cleanReason(reason string) (string, *apiError) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		return "", badRequest("reason_required", "a reason is required")
	}
	if hasNUL(reason) {
		return "", badRequest("invalid_body", "reason contains a NUL character")
	}
	if utf8.RuneCountInString(reason) > maxReasonChars {
		return "", badRequest("reason_too_long", "reason is longer than "+strconv.Itoa(maxReasonChars)+" characters")
	}
	return reason, nil
}

// cleanNoteBody trims a note and checks it: non-blank, at most maxNoteChars
// characters.
func cleanNoteBody(body string) (string, *apiError) {
	body = strings.TrimSpace(body)
	if body == "" {
		return "", badRequest("body_required", "a note body is required")
	}
	if hasNUL(body) {
		return "", badRequest("invalid_body", "body contains a NUL character")
	}
	if utf8.RuneCountInString(body) > maxNoteChars {
		return "", badRequest("body_too_long", "body is longer than "+strconv.Itoa(maxNoteChars)+" characters")
	}
	return body, nil
}

func parseTenantStatus(s string) (workspacestate.Status, *apiError) {
	st := workspacestate.Status(s)
	if !workspacestatus.ValidStatus(st) {
		return "", badRequest("invalid_status", "status must be active, read_only, suspended or pending_deletion")
	}
	return st, nil
}

// parseAccountStatus accepts the two states staff may set. pending_deletion
// and deleted belong to account deletion.
func parseAccountStatus(s string) (accountguard.State, *apiError) {
	switch st := accountguard.State(s); st {
	case accountguard.Active, accountguard.Suspended:
		return st, nil
	}
	return "", badRequest("invalid_status", "status must be active or suspended")
}

func parseStaffRole(s string) (staff.Role, *apiError) {
	r, ok := staff.ParseRole(s)
	if !ok {
		return "", badRequest("invalid_role", "role must be system_admin, admin, support or billing")
	}
	return r, nil
}

// parseEmail lower-cases and trims an address, as Multica stores them, and
// accepts a bare address only.
func parseEmail(s string) (string, *apiError) {
	s = strings.ToLower(strings.TrimSpace(s))
	if s == "" || len(s) > maxEmailChars {
		return "", badRequest("invalid_email", "a valid email is required")
	}
	addr, err := mail.ParseAddress(s)
	if err != nil || addr.Address != s || addr.Name != "" {
		return "", badRequest("invalid_email", "a valid email is required")
	}
	return s, nil
}

// likePattern turns a search term into an ILIKE pattern that matches it as a
// substring. Percent, underscore and backslash in the term match themselves.
// An empty term gives nil: no filter.
func likePattern(q string) (*string, *apiError) {
	q = strings.TrimSpace(q)
	if q == "" {
		return nil, nil
	}
	if hasNUL(q) {
		return nil, invalidFilter("q contains a NUL character")
	}
	esc := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`).Replace(q)
	p := "%" + esc + "%"
	return &p, nil
}

// Cursor is the (created_at, id) of the last row of a page.
type Cursor struct {
	CreatedAt time.Time
	ID        string
}

func encodeCursor(createdAt time.Time, id string) string {
	raw := createdAt.UTC().Format(time.RFC3339Nano) + "|" + id
	return base64.RawURLEncoding.EncodeToString([]byte(raw))
}

func decodeCursor(s string) (Cursor, error) {
	raw, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		return Cursor{}, err
	}
	ts, id, ok := strings.Cut(string(raw), "|")
	if !ok {
		return Cursor{}, errors.New("malformed cursor")
	}
	at, err := time.Parse(time.RFC3339Nano, ts)
	if err != nil {
		return Cursor{}, err
	}
	canon, ok := canonicalUUID(id)
	if !ok {
		return Cursor{}, errors.New("malformed cursor id")
	}
	return Cursor{CreatedAt: at, ID: canon}, nil
}

type listParams struct {
	limit  int
	cursor *Cursor
}

func parseListParams(q url.Values) (listParams, *apiError) {
	p := listParams{limit: defaultLimit}
	if q.Has("limit") {
		n, err := strconv.Atoi(q.Get("limit"))
		if err != nil || n < 1 || n > maxLimit {
			return p, invalidFilter("limit must be an integer from 1 to " + strconv.Itoa(maxLimit))
		}
		p.limit = n
	}
	if q.Has("cursor") {
		c, err := decodeCursor(q.Get("cursor"))
		if err != nil {
			return p, badRequest("invalid_cursor", "invalid cursor")
		}
		p.cursor = &c
	}
	return p, nil
}

// auditActions are the names the action filter accepts: every action of the
// audit package's catalogue, the one list that the workspace log of Multica
// reads too.
var auditActions = func() map[string]bool {
	m := make(map[string]bool, len(audit.Catalog))
	for _, a := range audit.Catalog {
		m[a] = true
	}
	return m
}()

func isAuditAction(s string) bool { return auditActions[s] }

type auditParams struct {
	listParams
	action  *string
	actorID *string
	from    *time.Time
	to      *time.Time
}

// parseAuditParams reads the audit list filters with the rules of the
// workspace log in Multica: limit, cursor, action, actor_id, from (inclusive)
// and to (exclusive).
func parseAuditParams(q url.Values) (auditParams, *apiError) {
	var p auditParams
	var err *apiError
	if p.listParams, err = parseListParams(q); err != nil {
		return p, err
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
		if a == "" || hasNUL(a) {
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
		t, perr := time.Parse(time.RFC3339, q.Get(f.name))
		if perr != nil {
			return p, invalidFilter(f.name + " must be an RFC 3339 timestamp")
		}
		t = t.UTC()
		*f.dst = &t
	}
	if p.from != nil && p.to != nil && p.from.After(*p.to) {
		return p, invalidFilter("from must not be after to")
	}
	return p, nil
}
