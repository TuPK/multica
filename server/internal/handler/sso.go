package handler

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auth"
	"github.com/multica-ai/multica/server/internal/logger"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/mfa"
	"github.com/multica-ai/multica/server/internal/middleware"
	"github.com/multica-ai/multica/server/internal/seatcapacity"
	"github.com/multica-ai/multica/server/internal/sso"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// ssoProviderTimeout bounds the calls a request makes to an identity provider.
const ssoProviderTimeout = 10 * time.Second

const (
	ssoStatusActive   = "active"
	ssoStatusDisabled = "disabled"
	ssoRoleMember     = "member"
	ssoRoleAdmin      = "admin"
)

// SSOHandler serves the OIDC connection routes and the sign-in routes of the
// platform spec 6.3.
type SSOHandler struct {
	h      *Handler
	client sso.Client
	// AllowInsecureIssuer lets a connection use an http issuer. Production
	// leaves it false; tests run the fake provider on a plain http port.
	AllowInsecureIssuer bool
}

func NewSSOHandler(h *Handler, client sso.Client) *SSOHandler {
	return &SSOHandler{h: h, client: client}
}

// ConnectionRoutes mounts the connection routes under /api/workspaces/{id}. The
// caller wraps them in the owner-only role check.
func (s *SSOHandler) ConnectionRoutes(r chi.Router) {
	r.Get("/sso/connection", s.GetConnection)
	r.Put("/sso/connection", s.PutConnection)
	r.Delete("/sso/connection", s.DeleteConnection)
}

// AuthRoutes mounts the sign-in routes under /auth/sso. They are public: the
// person is not signed in yet.
func (s *SSOHandler) AuthRoutes(r chi.Router) {
	r.Post("/discover", s.Discover)
	r.Get("/start", s.Start)
	r.Get("/callback", s.Callback)
}

// publicBase is where the identity provider and the browser reach this server:
// MULTICA_PUBLIC_URL, or the app URL that proxies /auth to it, as the daemon
// setup falls back. Empty when neither is configured.
func ssoPublicBase() string {
	if base := normalizePublicURL(os.Getenv("MULTICA_PUBLIC_URL")); base != "" {
		return base
	}
	return resolveFrontendAppURL()
}

func (s *SSOHandler) redirectURI() string {
	if base := ssoPublicBase(); base != "" {
		return base + "/auth/sso/callback"
	}
	return ""
}

// ---- connection routes ----

// SSOConnectionResponse is a connection as the owner sees it. The client secret
// is never part of it; ClientSecretSet says whether one is stored.
type SSOConnectionResponse struct {
	ID              string `json:"id"`
	WorkspaceID     string `json:"workspace_id"`
	Issuer          string `json:"issuer"`
	ClientID        string `json:"client_id"`
	ClientSecretSet bool   `json:"client_secret_set"`
	DefaultRole     string `json:"default_role"`
	Status          string `json:"status"`
	RedirectURI     string `json:"redirect_uri"`
	CreatedAt       string `json:"created_at"`
	UpdatedAt       string `json:"updated_at"`
}

func (s *SSOHandler) connectionToResponse(c db.SsoConnection) SSOConnectionResponse {
	return SSOConnectionResponse{
		ID:              uuidToString(c.ID),
		WorkspaceID:     uuidToString(c.WorkspaceID),
		Issuer:          c.Issuer,
		ClientID:        c.ClientID,
		ClientSecretSet: len(c.ClientSecretEnc) > 0,
		DefaultRole:     c.DefaultRole,
		Status:          c.Status,
		RedirectURI:     s.redirectURI(),
		CreatedAt:       timestampToString(c.CreatedAt),
		UpdatedAt:       timestampToString(c.UpdatedAt),
	}
}

// GetConnection returns the connection of the workspace. Owner only.
// GET /api/workspaces/{id}/sso/connection
func (s *SSOHandler) GetConnection(w http.ResponseWriter, r *http.Request) {
	ws, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	conn, err := s.h.Queries.GetSSOConnectionByWorkspace(r.Context(), ws)
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "sso_connection_not_found", "this workspace has no SSO connection")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load the connection")
		return
	}
	writeJSON(w, http.StatusOK, s.connectionToResponse(conn))
}

type putSSOConnectionRequest struct {
	Issuer       string `json:"issuer"`
	ClientID     string `json:"client_id"`
	ClientSecret string `json:"client_secret"`
	DefaultRole  string `json:"default_role"`
	Status       string `json:"status"`
}

func (s *SSOHandler) validIssuer(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return false
	}
	return u.Scheme == "https" || (u.Scheme == "http" && s.AllowInsecureIssuer)
}

// PutConnection creates or updates the connection of the workspace. The
// workspace needs a verified domain first. An update that leaves the secret out
// keeps the stored one, unless the issuer or the client id changes: a secret
// is never sent to a provider the owner did not enter it for.
// PUT /api/workspaces/{id}/sso/connection
func (s *SSOHandler) PutConnection(w http.ResponseWriter, r *http.Request) {
	ws, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	var req putSSOConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	req.Issuer = strings.TrimSpace(req.Issuer)
	req.ClientID = strings.TrimSpace(req.ClientID)
	if !s.validIssuer(req.Issuer) {
		writeErrorCode(w, http.StatusBadRequest, "invalid_issuer", "the issuer must be an https URL")
		return
	}
	if req.ClientID == "" {
		writeErrorCode(w, http.StatusBadRequest, "invalid_client_id", "client_id is required")
		return
	}

	existing, err := s.h.Queries.GetSSOConnectionByWorkspace(r.Context(), ws)
	found := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusInternalServerError, "failed to load the connection")
		return
	}

	role, status := req.DefaultRole, req.Status
	if role == "" {
		role = ssoRoleMember
		if found {
			role = existing.DefaultRole
		}
	}
	if role != ssoRoleMember && role != ssoRoleAdmin {
		writeErrorCode(w, http.StatusBadRequest, "invalid_role", "default_role must be member or admin")
		return
	}
	if status == "" {
		status = ssoStatusActive
		if found {
			status = existing.Status
		}
	}
	if status != ssoStatusActive && status != ssoStatusDisabled {
		writeErrorCode(w, http.StatusBadRequest, "invalid_status", "status must be active or disabled")
		return
	}

	domains, err := s.h.Queries.ListVerifiedSSODomainNames(r.Context(), ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check the verified domains")
		return
	}
	if len(domains) == 0 {
		writeErrorCode(w, http.StatusUnprocessableEntity, "domain_not_verified", "verify a domain of the workspace before connecting an identity provider")
		return
	}

	crypto, err := mfa.LoadCrypto()
	if err != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "sso_unavailable", "single sign-on is not configured on this server")
		return
	}
	sealed := existing.ClientSecretEnc
	identityChanged := !found || existing.Issuer != req.Issuer || existing.ClientID != req.ClientID
	switch {
	case req.ClientSecret != "":
		if sealed, err = crypto.Seal(req.ClientSecret); err != nil {
			writeError(w, http.StatusInternalServerError, "failed to store the client secret")
			return
		}
	case identityChanged:
		writeErrorCode(w, http.StatusBadRequest, "client_secret_required", "enter the client secret of this provider")
		return
	}

	if identityChanged {
		ctx, cancel := context.WithTimeout(r.Context(), ssoProviderTimeout)
		defer cancel()
		if err := s.client.Discover(ctx, req.Issuer); err != nil {
			slog.Warn("sso issuer discovery failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", uuidToString(ws))...)
			writeErrorCode(w, http.StatusUnprocessableEntity, "issuer_unreachable", "the issuer does not serve OpenID Connect discovery")
			return
		}
	}

	row, err := s.h.Queries.UpsertSSOConnection(r.Context(), db.UpsertSSOConnectionParams{
		WorkspaceID:     ws,
		Issuer:          req.Issuer,
		ClientID:        req.ClientID,
		ClientSecretEnc: sealed,
		DefaultRole:     role,
		Status:          status,
		CreatedBy:       parseUUID(userID),
	})
	if err != nil {
		slog.Warn("sso connection save failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", uuidToString(ws))...)
		writeError(w, http.StatusInternalServerError, "failed to save the connection")
		return
	}
	code := http.StatusOK
	if !found {
		code = http.StatusCreated
	}
	writeJSON(w, code, s.connectionToResponse(row))
}

// DeleteConnection removes the connection of the workspace. Sessions signed in
// through it stay valid until they end.
// DELETE /api/workspaces/{id}/sso/connection
func (s *SSOHandler) DeleteConnection(w http.ResponseWriter, r *http.Request) {
	ws, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	n, err := s.h.Queries.DeleteSSOConnection(r.Context(), ws)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to remove the connection")
		return
	}
	if n == 0 {
		writeErrorCode(w, http.StatusNotFound, "sso_connection_not_found", "this workspace has no SSO connection")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ---- sign-in routes ----

// Discover tells the sign-in page whether an email signs in through SSO. It
// answers the same shape for every email, and says nothing beyond whether a
// verified domain of a workspace with an active connection matches.
// POST /auth/sso/discover
func (s *SSOHandler) Discover(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	addr, err := mail.ParseAddress(strings.TrimSpace(req.Email))
	domain := ""
	if err == nil {
		domain = sso.EmailDomain(addr.Address)
	}
	if domain == "" {
		writeJSON(w, http.StatusOK, map[string]any{"sso": false})
		return
	}
	conn, err := s.h.Queries.GetActiveSSOConnectionByVerifiedDomain(r.Context(), domain)
	if errors.Is(err, pgx.ErrNoRows) {
		writeJSON(w, http.StatusOK, map[string]any{"sso": false})
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to look up single sign-on")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"sso":       true,
		"start_url": ssoPublicBase() + "/auth/sso/start?connection=" + uuidToString(conn.ID),
	})
}

// ssoLoginFailure sends the browser back to the sign-in page with a code. The
// callback is a browser navigation, so a JSON error would be a blank page.
func ssoLoginFailure(w http.ResponseWriter, r *http.Request, code string) {
	http.Redirect(w, r, resolveFrontendAppURL()+"/login?sso_error="+url.QueryEscape(code), http.StatusFound)
}

func clearSSOPending(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name: sso.PendingCookie, Value: "", Path: "/auth/sso", MaxAge: -1,
		HttpOnly: true, Secure: secure, SameSite: http.SameSiteLaxMode,
	})
}

// safeNext keeps a post-login target on this site: a path, never a URL.
func safeNext(next string) string {
	if next == "" || len(next) > 512 || !strings.HasPrefix(next, "/") || strings.HasPrefix(next, "//") || strings.ContainsAny(next, "\\\r\n") {
		return "/"
	}
	return next
}

// Start sends the browser to the identity provider of a connection, with the
// state, nonce and PKCE challenge of a fresh login. The login itself waits in a
// sealed, expiring, HttpOnly cookie.
// GET /auth/sso/start?connection={id}&next={path}
func (s *SSOHandler) Start(w http.ResponseWriter, r *http.Request) {
	connID, err := util.ParseUUID(r.URL.Query().Get("connection"))
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_connection", "invalid connection")
		return
	}
	base := ssoPublicBase()
	crypto, cryptoErr := mfa.LoadCrypto()
	if base == "" || cryptoErr != nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "sso_unavailable", "single sign-on is not configured on this server")
		return
	}
	conn, err := s.h.Queries.GetSSOConnection(r.Context(), connID)
	if err != nil || conn.Status != ssoStatusActive {
		ssoLoginFailure(w, r, "sso_unavailable")
		return
	}
	secret, err := crypto.Open(conn.ClientSecretEnc)
	if err != nil {
		slog.Warn("sso client secret cannot be opened", append(logger.RequestAttrs(r), "connection_id", uuidToString(conn.ID))...)
		ssoLoginFailure(w, r, "sso_unavailable")
		return
	}
	pend, err := sso.NewPending(uuidToString(conn.ID), safeNext(r.URL.Query().Get("next")), time.Now())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start the sign-in")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), ssoProviderTimeout)
	defer cancel()
	authURL, err := s.client.AuthURL(ctx, sso.Provider{Issuer: conn.Issuer, ClientID: conn.ClientID, ClientSecret: secret}, s.redirectURI(), pend)
	if err != nil {
		slog.Warn("sso provider unavailable at start", append(logger.RequestAttrs(r), "error", err, "connection_id", uuidToString(conn.ID))...)
		ssoLoginFailure(w, r, "sso_provider_unavailable")
		return
	}
	sealed, err := sso.SealPending(crypto, pend)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start the sign-in")
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: sso.PendingCookie, Value: sealed, Path: "/auth/sso", MaxAge: int(sso.PendingTTL.Seconds()),
		HttpOnly: true, Secure: strings.HasPrefix(base, "https://"), SameSite: http.SameSiteLaxMode,
	})
	w.Header().Set("Cache-Control", "no-store")
	http.Redirect(w, r, authURL, http.StatusFound)
}

// Callback finishes a login. It requires the pending login the browser holds
// to match the state, verifies the ID token, requires the email's domain to be
// a verified domain of the workspace that owns the connection, then finds or
// creates the user, adds them with the connection's default role, and issues a
// session whose amr names the connection.
// GET /auth/sso/callback
func (s *SSOHandler) Callback(w http.ResponseWriter, r *http.Request) {
	secure := strings.HasPrefix(ssoPublicBase(), "https://")
	clearSSOPending(w, secure)
	w.Header().Set("Cache-Control", "no-store")
	fail := func(code string) { ssoLoginFailure(w, r, code) }

	q := r.URL.Query()
	if q.Get("error") != "" {
		// The provider refused or the person cancelled. Its description is
		// not shown: it is text from a third party.
		fail("sso_login_failed")
		return
	}
	crypto, err := mfa.LoadCrypto()
	if err != nil {
		fail("sso_unavailable")
		return
	}
	cookie, err := r.Cookie(sso.PendingCookie)
	if err != nil {
		fail("sso_login_failed")
		return
	}
	pend, err := sso.OpenPending(crypto, cookie.Value, time.Now())
	if err != nil {
		fail("sso_login_failed")
		return
	}
	connID, err := util.ParseUUID(pend.ConnectionID)
	if err != nil {
		fail("sso_login_failed")
		return
	}
	conn, err := s.h.Queries.GetSSOConnection(r.Context(), connID)
	if err != nil || conn.Status != ssoStatusActive {
		fail("sso_unavailable")
		return
	}
	secret, err := crypto.Open(conn.ClientSecretEnc)
	if err != nil {
		slog.Warn("sso client secret cannot be opened", append(logger.RequestAttrs(r), "connection_id", uuidToString(conn.ID))...)
		fail("sso_unavailable")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), ssoProviderTimeout)
	defer cancel()
	identity, err := s.client.Complete(ctx, sso.Provider{Issuer: conn.Issuer, ClientID: conn.ClientID, ClientSecret: secret},
		s.redirectURI(), q.Get("code"), q.Get("state"), pend)
	if err != nil {
		s.logSignInFailure(r, conn, err)
		if errors.Is(err, sso.ErrEmailNotVerified) {
			fail("email_not_verified")
			return
		}
		fail("sso_login_failed")
		return
	}

	// The core safety rule: the provider is the workspace owner's, so it is
	// trusted for emails on the owner's verified domains and for nothing else.
	// Without this a workspace could claim any email and take over its account.
	onDomain, err := s.h.Queries.HasVerifiedSSODomain(r.Context(), db.HasVerifiedSSODomainParams{
		WorkspaceID: conn.WorkspaceID, Domain: sso.EmailDomain(identity.Email),
	})
	if err != nil {
		slog.Warn("sso domain check failed", append(logger.RequestAttrs(r), "error", err)...)
		fail("sso_login_failed")
		return
	}
	if !onDomain {
		slog.Warn("sso sign-in refused: email outside the verified domains",
			append(logger.RequestAttrs(r), "connection_id", uuidToString(conn.ID), "domain", sso.EmailDomain(identity.Email))...)
		fail("domain_not_allowed")
		return
	}
	if code := s.workspaceRefusal(r, conn.WorkspaceID); code != "" {
		fail(code)
		return
	}

	user, isNew, err := s.h.findOrCreateUser(r.Context(), identity.Email)
	if err != nil {
		var signupErr SignupError
		switch {
		case errors.Is(err, auth.ErrTemporarilyDisabledUser):
			fail("account_unavailable")
		case errors.As(err, &signupErr):
			fail("signup_not_allowed")
		default:
			slog.Warn("sso user lookup or creation failed", append(logger.RequestAttrs(r), "error", err)...)
			fail("sso_login_failed")
		}
		return
	}
	if isNew {
		obsmetrics.RecordEvent(s.h.Analytics, s.h.Metrics, analytics.Signup(uuidToString(user.ID), user.Email, signupSourceFromRequest(r)))
	}

	if code := s.joinWorkspace(r, conn, user); code != "" {
		fail(code)
		return
	}

	token, err := s.h.issueJWT(user, []string{sso.AMR(uuidToString(conn.ID))})
	if err != nil {
		if errors.Is(err, auth.ErrTemporarilyDisabledUser) {
			fail("account_unavailable")
			return
		}
		slog.Warn("sso login failed", append(logger.RequestAttrs(r), "error", err)...)
		fail("sso_login_failed")
		return
	}
	if !s.h.recordLogin(w, r, user, "sso") {
		return
	}
	if err := auth.SetAuthCookies(w, token); err != nil {
		slog.Warn("failed to set auth cookies", "error", err)
	}
	if s.h.CFSigner != nil {
		for _, c := range s.h.CFSigner.SignedCookies(time.Now().Add(auth.AuthTokenTTL())) {
			http.SetCookie(w, c)
		}
	}
	slog.Info("user logged in with sso", append(logger.RequestAttrs(r), "user_id", uuidToString(user.ID), "connection_id", uuidToString(conn.ID))...)
	http.Redirect(w, r, resolveFrontendAppURL()+safeNext(pend.Next), http.StatusFound)
}

// logSignInFailure records why a sign-in failed. The sentinel errors carry no
// text from the token endpoint, so the client secret cannot reach the log.
func (s *SSOHandler) logSignInFailure(r *http.Request, conn db.SsoConnection, err error) {
	slog.Warn("sso sign-in refused", append(logger.RequestAttrs(r), "error", err, "connection_id", uuidToString(conn.ID))...)
}

// workspaceRefusal returns the refusal code of a workspace that takes no new
// member in its current state, or "".
func (s *SSOHandler) workspaceRefusal(r *http.Request, workspaceID pgtype.UUID) string {
	guard := middleware.WorkspaceStateFromContext(r.Context())
	if guard == nil {
		return ""
	}
	rec, err := guard.State(r.Context(), uuidToString(workspaceID))
	if err != nil {
		slog.Warn("sso: workspace state unavailable", append(logger.RequestAttrs(r), "error", err)...)
		return workspaceguard.CodeStateUnavailable
	}
	return workspaceguard.CodeFor(string(rec.Status))
}

// joinWorkspace adds the user to the connection's workspace with its default
// role, through the same seat reservation and membership steps as a share-link
// join. A user who is already a member keeps the role they have. It returns a
// refusal code, or "" when the user is a member afterwards.
func (s *SSOHandler) joinWorkspace(r *http.Request, conn db.SsoConnection, user db.User) string {
	ctx := r.Context()
	_, err := s.h.Queries.GetMemberByUserAndWorkspace(ctx, db.GetMemberByUserAndWorkspaceParams{UserID: user.ID, WorkspaceID: conn.WorkspaceID})
	if err == nil {
		return ""
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "sso_login_failed"
	}

	wsUUID, connUUID, userUUID := uuid.UUID(conn.WorkspaceID.Bytes), uuid.UUID(conn.ID.Bytes), uuid.UUID(user.ID.Bytes)
	var capacityToken uuid.UUID
	if s.h.seatCapacityEnabled() {
		// The connection stands where the share link stands in the reservation:
		// it is the entry the person joins through.
		capacityToken, err = s.h.beginShareJoinCapacity(ctx, wsUUID, connUUID, userUUID)
		if err != nil {
			return seatCapacityErrorCode(err)
		}
	}
	tx, err := s.h.TxStarter.Begin(ctx)
	if err != nil {
		s.h.compensateCapacityIntent(ctx, capacityToken)
		return "sso_login_failed"
	}
	defer tx.Rollback(ctx)
	qtx := s.h.Queries.WithTx(tx)
	member, err := qtx.CreateMember(ctx, db.CreateMemberParams{WorkspaceID: conn.WorkspaceID, UserID: user.ID, Role: conn.DefaultRole})
	if err != nil {
		s.h.compensateCapacityIntent(ctx, capacityToken)
		if isUniqueViolation(err) {
			return "" // joined by another request in between
		}
		return "sso_login_failed"
	}
	if _, err := qtx.MarkUserOnboarded(ctx, user.ID); err != nil {
		s.h.compensateCapacityIntent(ctx, capacityToken)
		return "sso_login_failed"
	}
	if capacityToken != uuid.Nil {
		if err := transitionCapacityIntentToConfirm(ctx, qtx, capacityToken, uuid.UUID(member.ID.Bytes), seatcapacity.ActionClaimShareJoin); err != nil {
			return "sso_login_failed"
		}
	}
	if err := tx.Commit(ctx); err != nil {
		s.h.compensateCapacityIntent(ctx, capacityToken)
		return "sso_login_failed"
	}
	if capacityToken != uuid.Nil {
		s.h.confirmCapacityIntent(ctx, wsUUID, capacityToken, uuid.UUID(member.ID.Bytes))
	}

	wsID := uuidToString(conn.WorkspaceID)
	slog.Info("user joined through sso", "user_id", uuidToString(user.ID), "workspace_id", wsID, "role", conn.DefaultRole)
	s.h.publish(protocol.EventMemberAdded, wsID, "member", uuidToString(user.ID), map[string]any{
		"member": s.h.memberWithUserResponse(member, user),
	})
	s.h.notifyDaemonWorkspacesChanged(uuidToString(user.ID))
	return ""
}

// seatCapacityErrorCode is the code writeSeatCapacityError gives the same
// error, for a caller that cannot answer with JSON.
func seatCapacityErrorCode(err error) string {
	switch {
	case seatcapacity.IsRateLimited(err):
		return "seat_capacity_rate_limited"
	case errors.Is(err, errSeatCapacityOvercommitted):
		return "seat_capacity_overcommitted"
	case errors.Is(err, errSeatCapacityFull):
		return "seat_capacity_full"
	default:
		return "seat_capacity_unavailable"
	}
}
