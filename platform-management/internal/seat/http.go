package seat

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/trust"
)

// The routes of the seat client (server/internal/seatcapacity/client.go, do and
// its callers). Each is under /api/v1/internal/subscriptions/{workspaceId}/capacity/.
const (
	base = "/api/v1/internal/subscriptions/{workspaceId}/capacity/"

	ReservePattern       = "POST " + base + "reserve"
	ClaimPattern         = "POST " + base + "claim"
	ConsumePattern       = "POST " + base + "consume"
	ConfirmPattern       = "POST " + base + "confirm"
	ReleasePattern       = "POST " + base + "release"
	ReleaseMemberPattern = "POST " + base + "release-member"
	OperationPattern     = "GET " + base + "operations/{token}"
)

// Patterns lists the patterns Handler serves.
var Patterns = []string{
	ReservePattern, ClaimPattern, ConsumePattern, ConfirmPattern,
	ReleasePattern, ReleaseMemberPattern, OperationPattern,
}

const maxBody = 64 << 10

// Handler serves Patterns. The seat client sends no credential, and the API
// listener is internal (see package trust). It never answers 429: the client
// reads that status as a rate limit and backs off (Appendix B rule 3).
type Handler struct {
	svc *Service
	log *slog.Logger
}

// NewHandler returns the HTTP handler of svc.
func NewHandler(svc *Service, log *slog.Logger) *Handler {
	if log == nil {
		log = slog.Default()
	}
	return &Handler{svc: svc, log: log}
}

// Mux returns a mux with every pattern of Patterns.
func (h *Handler) Mux() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc(ReservePattern, h.reserve)
	mux.HandleFunc(ClaimPattern, h.claim)
	mux.HandleFunc(ConsumePattern, h.consume)
	mux.HandleFunc(ConfirmPattern, h.confirm)
	mux.HandleFunc(ReleasePattern, h.release)
	mux.HandleFunc(ReleaseMemberPattern, h.releaseMember)
	mux.HandleFunc(OperationPattern, h.operation)
	return mux
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeError(w http.ResponseWriter, status int, code, msg string) {
	writeJSON(w, status, map[string]string{"error": msg, "code": code})
}

func (h *Handler) finish(w http.ResponseWriter, r *http.Request, d Decision, err error) {
	switch {
	case err == nil:
		writeJSON(w, http.StatusOK, d)
	case errors.Is(err, ErrOvercommitted):
		writeError(w, http.StatusConflict, CodeOvercommitted, "workspace members exceed purchased seats")
	case errors.Is(err, ErrNotFound):
		writeError(w, http.StatusNotFound, CodeNotFound, "operation not found")
	default:
		// The cause may name the database, so it goes to the log only.
		h.log.Error("seat api: request failed", "path", r.URL.Path, "error", err)
		writeError(w, http.StatusInternalServerError, CodeInternal, "internal error")
	}
}

func invalid(w http.ResponseWriter, msg string) {
	writeError(w, http.StatusBadRequest, CodeInvalid, msg)
}

// workspace returns the canonical workspace ID of the path.
func workspace(w http.ResponseWriter, r *http.Request) (string, bool) {
	ws, ok := trust.CanonicalUUID(r.PathValue("workspaceId"))
	if !ok {
		invalid(w, "workspaceId must be a UUID")
	}
	return ws, ok
}

// body is every field any request carries.
type body struct {
	Token     string     `json:"token"`
	Kind      string     `json:"kind"`
	SubjectID string     `json:"subject_id"`
	ExpiresAt *time.Time `json:"expires_at"`
	MemberID  string     `json:"member_id"`
}

func decode(w http.ResponseWriter, r *http.Request) (body, bool) {
	var b body
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBody))
	if err == nil {
		err = json.Unmarshal(raw, &b)
	}
	if err != nil {
		invalid(w, "body must be a JSON object")
		return body{}, false
	}
	return b, true
}

// id canonicalises a UUID field; what names it is the field's JSON name.
func id(w http.ResponseWriter, value, name string) (string, bool) {
	v, ok := trust.CanonicalUUID(value)
	if !ok {
		invalid(w, name+" must be a UUID")
	}
	return v, ok
}

func (h *Handler) reserve(w http.ResponseWriter, r *http.Request) {
	ws, ok := workspace(w, r)
	if !ok {
		return
	}
	b, ok := decode(w, r)
	if !ok {
		return
	}
	token, ok := id(w, b.Token, "token")
	if !ok {
		return
	}
	subject, ok := id(w, b.SubjectID, "subject_id")
	if !ok {
		return
	}
	if b.Kind != KindInvitation || b.ExpiresAt == nil || b.ExpiresAt.IsZero() {
		invalid(w, "kind must be invitation and expires_at is required")
		return
	}
	d, err := h.svc.Reserve(r.Context(), ws, token, subject, b.ExpiresAt.UTC())
	h.finish(w, r, d, err)
}

func (h *Handler) claim(w http.ResponseWriter, r *http.Request) {
	ws, ok := workspace(w, r)
	if !ok {
		return
	}
	b, ok := decode(w, r)
	if !ok {
		return
	}
	token, ok := id(w, b.Token, "token")
	if !ok {
		return
	}
	subject, ok := id(w, b.SubjectID, "subject_id")
	if !ok {
		return
	}
	if b.Kind != KindShareJoin {
		invalid(w, "kind must be share_join")
		return
	}
	d, err := h.svc.Claim(r.Context(), ws, token, subject)
	h.finish(w, r, d, err)
}

// withToken serves the routes whose body names one token.
func (h *Handler) withToken(w http.ResponseWriter, r *http.Request, member bool, call func(ws, token string) (Decision, error)) {
	ws, ok := workspace(w, r)
	if !ok {
		return
	}
	b, ok := decode(w, r)
	if !ok {
		return
	}
	token, ok := id(w, b.Token, "token")
	if !ok {
		return
	}
	if member {
		if _, ok := id(w, b.MemberID, "member_id"); !ok {
			return
		}
	}
	d, err := call(ws, token)
	h.finish(w, r, d, err)
}

func (h *Handler) consume(w http.ResponseWriter, r *http.Request) {
	h.withToken(w, r, false, func(ws, token string) (Decision, error) { return h.svc.Consume(r.Context(), ws, token) })
}

func (h *Handler) confirm(w http.ResponseWriter, r *http.Request) {
	h.withToken(w, r, true, func(ws, token string) (Decision, error) { return h.svc.Confirm(r.Context(), ws, token) })
}

func (h *Handler) release(w http.ResponseWriter, r *http.Request) {
	h.withToken(w, r, false, func(ws, token string) (Decision, error) { return h.svc.Release(r.Context(), ws, token) })
}

func (h *Handler) releaseMember(w http.ResponseWriter, r *http.Request) {
	ws, ok := workspace(w, r)
	if !ok {
		return
	}
	b, ok := decode(w, r)
	if !ok {
		return
	}
	if _, ok := id(w, b.MemberID, "member_id"); !ok {
		return
	}
	d, err := h.svc.ReleaseMember(r.Context(), ws)
	h.finish(w, r, d, err)
}

func (h *Handler) operation(w http.ResponseWriter, r *http.Request) {
	ws, ok := workspace(w, r)
	if !ok {
		return
	}
	token, ok := id(w, r.PathValue("token"), "token")
	if !ok {
		return
	}
	d, err := h.svc.Get(r.Context(), ws, token)
	h.finish(w, r, d, err)
}
