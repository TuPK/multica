package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/auditstore"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/logger"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/planlimit"
	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	"github.com/multica-ai/multica/server/internal/workspacedeletion"
	"github.com/multica-ai/multica/server/internal/workspaceguard"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

var nonAlpha = regexp.MustCompile(`[^a-zA-Z]`)
var nonAlphanumeric = regexp.MustCompile(`[^a-zA-Z0-9]`)
var workspaceSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var issuePrefixPattern = regexp.MustCompile(`^[A-Z0-9]{1,10}$`)

// defaultIssuePrefixFromSlug derives a new workspace's default issue prefix
// from its slug: alphanumerics only, first 4 chars, uppercased.
// Examples: "acme" → "ACME", "front-end" → "FRON", "team-2" → "TEAM".
//
// The slug — not the name — is the derivation source on purpose (MUL-6050).
// Name is the one field in the create flow that accepts non-ASCII, so deriving
// an ASCII-only prefix from it forced every CJK/emoji-named workspace onto the
// same "WS" fallback. Slug is validated as `^[a-z0-9]+(-[a-z0-9]+)*$` and
// required, so it always yields a non-empty, workspace-specific prefix.
//
// Kept byte-for-byte in sync with the client-side preview in
// packages/views/onboarding/steps/step-workspace.tsx — what the user sees on
// the create screen has to be what the server would derive.
func defaultIssuePrefixFromSlug(slug string) string {
	head := nonAlphanumeric.ReplaceAllString(slug, "")
	if head == "" {
		// Unreachable for a validated slug; keeps the function total so a
		// future caller can never persist an empty prefix.
		return "WS"
	}
	if len(head) > 4 {
		head = head[:4]
	}
	return strings.ToUpper(head)
}

// legacyIssuePrefixFromName is the pre-MUL-6050 derivation: first 3 ASCII
// letters of the workspace name, uppercased, "WS" when the name has none.
//
// FROZEN — do not "fix" this to match defaultIssuePrefixFromSlug. Its only
// caller is getIssuePrefix's fallback for workspaces whose stored prefix is
// empty (rows predating the column). Issue identifiers are computed at read
// time from the current prefix, so changing what this returns would silently
// rewrite the identifier of every historical issue in those workspaces.
// Deliberate product decision: no backfill, existing workspaces stay as they
// are; only newly created ones follow the slug-derived rule.
func legacyIssuePrefixFromName(name string) string {
	letters := nonAlpha.ReplaceAllString(name, "")
	if len(letters) == 0 {
		return "WS"
	}
	letters = strings.ToUpper(letters)
	if len(letters) > 3 {
		letters = letters[:3]
	}
	return letters
}

// normalizeIssuePrefix trims and uppercases a client-supplied issue prefix.
// An all-whitespace value means "not provided" and reports ("", true) so the
// caller can fall back to its default. Anything else must be 1-10 chars of
// A-Z0-9, mirroring the client-side guardrail in
// packages/views/settings/components/workspace-tab.tsx.
func normalizeIssuePrefix(raw string) (string, bool) {
	prefix := strings.ToUpper(strings.TrimSpace(raw))
	if prefix == "" {
		return "", true
	}
	if !issuePrefixPattern.MatchString(prefix) {
		return "", false
	}
	return prefix, true
}

const issuePrefixFormatError = "issue prefix must be 1-10 uppercase letters or digits"

type WorkspaceResponse struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	Context     *string `json:"context"`
	Settings    any     `json:"settings"`
	Repos       any     `json:"repos"`
	IssuePrefix string  `json:"issue_prefix"`
	AvatarURL   *string `json:"avatar_url"`
	// SecretaryAgentID names the agent that writes the boss's briefs; null when
	// the workspace has none.
	SecretaryAgentID *string `json:"secretary_agent_id"`
	CreatedAt        string  `json:"created_at"`
	UpdatedAt        string  `json:"updated_at"`
	PinnedAt         *string `json:"pinned_at,omitempty"`
}

func (h *Handler) workspaceToResponse(w db.Workspace) WorkspaceResponse {
	var settings any
	if w.Settings != nil {
		json.Unmarshal(w.Settings, &settings)
	}
	if settings == nil {
		settings = map[string]any{}
	}
	var repos any
	if w.Repos != nil {
		json.Unmarshal(w.Repos, &repos)
	}
	if repos == nil {
		repos = []any{}
	}
	return WorkspaceResponse{
		ID:               uuidToString(w.ID),
		Name:             w.Name,
		Slug:             w.Slug,
		Description:      textToPtr(w.Description),
		Context:          textToPtr(w.Context),
		Settings:         settings,
		Repos:            repos,
		IssuePrefix:      w.IssuePrefix,
		AvatarURL:        h.resolveAvatarURLPtr(textToPtr(w.AvatarUrl)),
		SecretaryAgentID: uuidToPtr(w.SecretaryAgentID),
		CreatedAt:        timestampToString(w.CreatedAt),
		UpdatedAt:        timestampToString(w.UpdatedAt),
	}
}

type MemberResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	UserID      string `json:"user_id"`
	Role        string `json:"role"`
	CreatedAt   string `json:"created_at"`
}

// recordAudit writes one audit event for the request. With a non-nil tx the
// row commits or rolls back with the change it records; without one it is a
// plain write. The actor defaults to the requesting user, labelled with their
// email. Callers must fail the request when it returns an error.
func (h *Handler) recordAudit(r *http.Request, tx pgx.Tx, e audit.Entry) error {
	q := h.Queries
	if tx != nil {
		q = h.Queries.WithTx(tx)
	}
	return h.recordAuditWith(r, q, e)
}

// recordAuditWith is recordAudit for queries that are already bound to the
// transaction of the change, or to the pool when there is none.
func (h *Handler) recordAuditWith(r *http.Request, q *db.Queries, e audit.Entry) error {
	ctx := r.Context()
	if e.ActorType == "" {
		uid, err := util.ParseUUID(requestUserID(r))
		if err != nil {
			return err
		}
		user, err := q.GetUser(ctx, uid)
		if err != nil {
			return err
		}
		e.ActorType = audit.ActorMember
		e.ActorID = uuidToString(user.ID)
		e.ActorLabel = user.Email
	}
	e.IP, e.UserAgent, e.RequestID = auditstore.RequestFields(r, h.clientIPForRateLimit)
	return audit.Record(ctx, auditstore.New(q), e)
}

// recordLogin writes the auth.login audit row for user. On failure it answers
// 500 and returns false, so no session credential reaches the client.
func (h *Handler) recordLogin(w http.ResponseWriter, r *http.Request, user db.User, method string) bool {
	err := h.recordAudit(r, nil, audit.Entry{
		ActorType:   audit.ActorMember,
		ActorID:     uuidToString(user.ID),
		ActorLabel:  user.Email,
		Action:      audit.ActionAuthLogin,
		TargetType:  "user",
		TargetID:    uuidToString(user.ID),
		TargetLabel: user.Email,
		Detail:      map[string]any{"method": method},
	})
	if err != nil {
		slog.Warn("audit login failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to record login")
		return false
	}
	return true
}

func memberToResponse(m db.Member) MemberResponse {
	return MemberResponse{
		ID:          uuidToString(m.ID),
		WorkspaceID: uuidToString(m.WorkspaceID),
		UserID:      uuidToString(m.UserID),
		Role:        m.Role,
		CreatedAt:   timestampToString(m.CreatedAt),
	}
}

func (h *Handler) ListWorkspaces(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	workspaces, err := h.Queries.ListWorkspaces(r.Context(), parseUUID(userID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list workspaces")
		return
	}

	resp := make([]WorkspaceResponse, len(workspaces))
	for i, ws := range workspaces {
		resp[i] = h.workspaceToResponse(db.Workspace{
			ID: ws.ID, Name: ws.Name, Slug: ws.Slug, Description: ws.Description,
			Settings: ws.Settings, CreatedAt: ws.CreatedAt, UpdatedAt: ws.UpdatedAt,
			Context: ws.Context, Repos: ws.Repos, IssuePrefix: ws.IssuePrefix,
			IssueCounter: ws.IssueCounter, AvatarUrl: ws.AvatarUrl,
			AttributionFailClosed: ws.AttributionFailClosed,
			SecretaryAgentID:      ws.SecretaryAgentID,
		})
		if ws.PinnedAt.Valid {
			pinnedAt := timestampToString(ws.PinnedAt)
			resp[i].PinnedAt = &pinnedAt
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) SetWorkspacePinned(w http.ResponseWriter, r *http.Request) {
	id, ok := parseUUIDOrBadRequest(w, workspaceIDFromURL(r, "id"), "workspace id")
	if !ok {
		return
	}
	member, ok := h.workspaceMember(w, r, uuidToString(id))
	if !ok {
		return
	}
	var req struct {
		Pinned *bool `json:"pinned"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Pinned == nil {
		writeError(w, http.StatusBadRequest, "pinned must be a boolean")
		return
	}
	pinnedAt, err := h.Queries.SetWorkspacePinned(r.Context(), db.SetWorkspacePinnedParams{
		WorkspaceID: id, UserID: member.UserID, Pinned: *req.Pinned,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workspace pin")
		return
	}
	var result *string
	if pinnedAt.Valid {
		value := timestampToString(pinnedAt)
		result = &value
	}
	writeJSON(w, http.StatusOK, map[string]any{"pinned_at": result})
}

func (h *Handler) GetWorkspace(w http.ResponseWriter, r *http.Request) {
	id := workspaceIDFromURL(r, "id")
	idUUID, ok := parseUUIDOrBadRequest(w, id, "workspace id")
	if !ok {
		return
	}

	ws, err := h.Queries.GetWorkspace(r.Context(), idUUID)
	if err != nil {
		writeError(w, http.StatusNotFound, "workspace not found")
		return
	}
	writeJSON(w, http.StatusOK, h.workspaceToResponse(ws))
}

type CreateWorkspaceRequest struct {
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description *string `json:"description"`
	Context     *string `json:"context"`
	IssuePrefix *string `json:"issue_prefix"`
}

// freeOwnedWorkspaceLimitCode is the code of the refusal. It is not
// plan_limit_reached: no plan gate is involved and the response has no gate.
const freeOwnedWorkspaceLimitCode = "free_workspace_limit_reached"

// freeOwnedWorkspaceLimit reads FREE_PLAN_OWNED_WORKSPACE_LIMIT. Unset or 0 is
// no limit, and then CreateWorkspace takes no lock and runs no query.
func freeOwnedWorkspaceLimit(r *http.Request) int {
	limit, err := planlimit.ParseOwnerLimit(os.Getenv(planlimit.OwnerLimitEnv))
	if err != nil {
		// An operator typo must not lock everyone out of creating workspaces.
		slog.Error("ignoring invalid owned-workspace limit", append(logger.RequestAttrs(r), "error", err)...)
		return 0
	}
	return limit
}

func (h *Handler) CreateWorkspace(w http.ResponseWriter, r *http.Request) {
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}

	// Self-host gate (#3433): when the operator has set
	// DISABLE_WORKSPACE_CREATION=true, no caller — including existing
	// workspace owners — may create additional workspaces. The frontend
	// hides every "Create workspace" affordance via /api/config, but the
	// 403 here is the only authoritative check.
	if h.cfg.DisableWorkspaceCreation {
		writeError(w, http.StatusForbidden, "workspace creation is disabled for this instance")
		return
	}

	// Self-host allowlist (#9400): when WORKSPACE_CREATION_ALLOWED_EMAILS is
	// set, only those accounts may create workspaces. The UI does not hide
	// the entry points for other users; this 403 is the enforcement point.
	if len(h.cfg.WorkspaceCreationAllowedEmails) > 0 {
		user, err := h.Queries.GetUser(r.Context(), parseUUID(userID))
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				writeError(w, http.StatusUnauthorized, "user not found")
				return
			}
			writeError(w, http.StatusInternalServerError, "failed to load user")
			return
		}
		if !contains(h.cfg.WorkspaceCreationAllowedEmails, strings.TrimSpace(user.Email)) {
			writeError(w, http.StatusForbidden, "workspace creation is restricted to selected users on this instance")
			return
		}
	}

	// Account-level cap (platform spec 5.7): a user may own only so many
	// workspaces while each is on the free plan. Not a plan gate, so it does
	// not go through the entitlement policy. It is checked in the creating
	// transaction below, under the user's advisory lock.
	ownedLimit := freeOwnedWorkspaceLimit(r)

	var req CreateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	req.Name = strings.TrimSpace(req.Name)
	req.Slug = strings.ToLower(strings.TrimSpace(req.Slug))
	if req.Name == "" || req.Slug == "" {
		writeError(w, http.StatusBadRequest, "name and slug are required")
		return
	}
	if !workspaceSlugPattern.MatchString(req.Slug) {
		writeError(w, http.StatusBadRequest, "slug must contain only lowercase letters, numbers, and hyphens")
		return
	}
	if isReservedSlug(req.Slug) {
		writeError(w, http.StatusBadRequest, "slug is reserved")
		return
	}

	// Resolve the prefix before opening the transaction: an invalid one is a
	// 400 that should never cost a connection.
	issuePrefix := ""
	if req.IssuePrefix != nil {
		prefix, ok := normalizeIssuePrefix(*req.IssuePrefix)
		if !ok {
			writeError(w, http.StatusBadRequest, issuePrefixFormatError)
			return
		}
		issuePrefix = prefix
	}
	if issuePrefix == "" {
		issuePrefix = defaultIssuePrefixFromSlug(req.Slug)
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}
	defer tx.Rollback(r.Context())

	qtx := h.Queries.WithTx(tx)
	if _, err := planlimit.CheckOwnerLimit(r.Context(), qtx, parseUUID(userID), ownedLimit); err != nil {
		var limitErr *planlimit.OwnerLimitError
		if errors.As(err, &limitErr) {
			writeErrorCode(w, http.StatusForbidden, freeOwnedWorkspaceLimitCode,
				"you own the maximum number of free workspaces; upgrade one of them to create another")
			return
		}
		slog.Warn("owned workspace limit check failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}
	ws, err := qtx.CreateWorkspace(r.Context(), db.CreateWorkspaceParams{
		Name:        req.Name,
		Slug:        req.Slug,
		Description: ptrToText(req.Description),
		Context:     ptrToText(req.Context),
		IssuePrefix: issuePrefix,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeError(w, http.StatusConflict, "workspace slug already exists")
			return
		}
		slog.Warn("create workspace failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	_, err = qtx.CreateMember(r.Context(), db.CreateMemberParams{
		WorkspaceID: ws.ID,
		UserID:      parseUUID(userID),
		Role:        "owner",
	})
	if err != nil {
		slog.Warn("add owner failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to add owner")
		return
	}

	// Seed the 7 built-in issue statuses inside the same transaction, so a
	// workspace is never visible without its status catalog — an issue cannot
	// be created before its status can be resolved. (MUL-6243)
	if err := issuestatus.Ensure(r.Context(), qtx, ws.ID); err != nil {
		slog.Warn("seed issue statuses failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to seed issue statuses")
		return
	}

	if err := h.recordAudit(r, tx, audit.Entry{
		WorkspaceID: uuidToString(ws.ID),
		Action:      audit.ActionWorkspaceCreated,
		TargetType:  "workspace",
		TargetID:    uuidToString(ws.ID),
		TargetLabel: ws.Name,
	}); err != nil {
		slog.Warn("audit workspace created failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	// NOTE: CreateWorkspace deliberately does NOT mark the user as
	// onboarded. The `onboarded_at` flag is owned by CompleteOnboarding
	// (Step 3 of the flow) and by AcceptInvitation (invitee joining an
	// existing workspace). This decouples "the user has a workspace"
	// from "the user has finished setup"; the workspace-layer route
	// gate (web layout / desktop App.tsx overlay) redirects un-onboarded
	// users back to /onboarding instead.

	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to create workspace")
		return
	}

	wsID := uuidToString(ws.ID)

	// "Is this the user's first workspace?" is derived in PostHog by looking
	// at whether they have a prior workspace_created event, not stamped at
	// emit time. Stamping here would race under concurrent creates without
	// a schema change, and the event stream answers the question exactly.
	obsmetrics.RecordEvent(h.Analytics, h.Metrics, analytics.WorkspaceCreated(userID, wsID))
	h.notifyDaemonWorkspacesChanged(userID)

	// Knowledge is on by default. The work runs off the request path; a failure
	// is logged and retried by the hourly pass.
	if h.Knowledge != nil {
		go h.Knowledge.AutoEnable(ws.ID, parseUUID(userID))
	}

	slog.Info("workspace created", append(logger.RequestAttrs(r), "workspace_id", wsID, "name", ws.Name, "slug", ws.Slug)...)
	writeJSON(w, http.StatusCreated, h.workspaceToResponse(ws))
}

type UpdateWorkspaceRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	Context     *string `json:"context"`
	Settings    any     `json:"settings"`
	Repos       any     `json:"repos"`
	IssuePrefix *string `json:"issue_prefix"`
	AvatarURL   *string `json:"avatar_url"`
	// SecretaryAgentID is tri-state: absent leaves the Secretary alone, null
	// unsets it, a string names the agent. Hence the raw message.
	SecretaryAgentID json.RawMessage `json:"secretary_agent_id"`
}

type workspaceRepoRef struct {
	URL         string `json:"url"`
	Description string `json:"description,omitempty"`
}

func validateAndNormalizeWorkspaceRepos(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}

	var repos []workspaceRepoRef
	if err := json.Unmarshal(raw, &repos); err != nil {
		return nil, fmt.Errorf("repos must be an array of repository objects: %w", err)
	}

	normalized := make([]workspaceRepoRef, 0, len(repos))
	seen := make(map[string]struct{}, len(repos))
	for i, repo := range repos {
		repo.URL = strings.TrimSpace(repo.URL)
		repo.Description = strings.TrimSpace(repo.Description)
		if repo.URL == "" {
			return nil, fmt.Errorf("repos[%d]: url is required", i)
		}
		if !isValidGitRepoURL(repo.URL) {
			return nil, fmt.Errorf("repos[%d]: url must be a valid http(s) or ssh git URL", i)
		}
		if _, ok := seen[repo.URL]; ok {
			continue
		}
		seen[repo.URL] = struct{}{}
		normalized = append(normalized, repo)
	}

	out, err := json.Marshal(normalized)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (h *Handler) UpdateWorkspace(w http.ResponseWriter, r *http.Request) {
	id := workspaceIDFromURL(r, "id")
	idUUID, ok := parseUUIDOrBadRequest(w, id, "workspace id")
	if !ok {
		return
	}

	var req UpdateWorkspaceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	params := db.UpdateWorkspaceParams{
		ID: idUUID,
	}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" {
			writeError(w, http.StatusBadRequest, "name is required")
			return
		}
		params.Name = pgtype.Text{String: name, Valid: true}
	}
	if req.Description != nil {
		params.Description = pgtype.Text{String: *req.Description, Valid: true}
	}
	if req.Context != nil {
		params.Context = pgtype.Text{String: *req.Context, Valid: true}
	}
	if req.Settings != nil {
		if incoming, ok := req.Settings.(map[string]any); ok {
			// Only an old client's flip of the retired PR switch needs the
			// stored value; see reconcilePRMergeSettings.
			var stored map[string]any
			if _, sent := incoming[prAutoCompleteLegacyKey]; sent {
				if existing, err := h.Queries.GetWorkspace(r.Context(), idUUID); err == nil {
					_ = json.Unmarshal(existing.Settings, &stored)
				}
			}
			reconcilePRMergeSettings(stored, incoming)
		}
		s, _ := json.Marshal(req.Settings)
		params.Settings = s
	}
	if req.Repos != nil {
		reposJSON, err := validateAndNormalizeWorkspaceRepos(req.Repos)
		if err != nil {
			writeError(w, http.StatusBadRequest, err.Error())
			return
		}
		params.Repos = reposJSON
	}
	if req.IssuePrefix != nil {
		prefix, ok := normalizeIssuePrefix(*req.IssuePrefix)
		if !ok {
			writeError(w, http.StatusBadRequest, issuePrefixFormatError)
			return
		}
		if prefix != "" {
			params.IssuePrefix = pgtype.Text{String: prefix, Valid: true}
		}
	}
	if req.AvatarURL != nil {
		// Read the stored value so an unchanged re-send skips revalidation —
		// this handler is the one avatar writer that doesn't already have the
		// row in hand.
		var current string
		if existing, err := h.Queries.GetWorkspace(r.Context(), idUUID); err == nil {
			current = existing.AvatarUrl.String
		}
		accepted, ok := h.acceptAvatarURL(w, r, *req.AvatarURL, current)
		if !ok {
			return
		}
		params.AvatarUrl = pgtype.Text{String: accepted, Valid: true}
	}
	if len(req.SecretaryAgentID) > 0 {
		params.SetSecretaryAgent = true
		if string(req.SecretaryAgentID) != "null" {
			var agentID string
			if err := json.Unmarshal(req.SecretaryAgentID, &agentID); err != nil {
				writeError(w, http.StatusBadRequest, "secretary_agent_id must be an agent id or null")
				return
			}
			agentUUID, ok := parseUUIDOrBadRequest(w, agentID, "secretary_agent_id")
			if !ok {
				return
			}
			agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{ID: agentUUID, WorkspaceID: idUUID})
			if err != nil || agent.ArchivedAt.Valid {
				writeError(w, http.StatusBadRequest, "secretary_agent_id must be a live agent of this workspace")
				return
			}
			params.SecretaryAgentID = agentUUID
		}
	}

	changedFields := []string{}
	if params.Name.Valid {
		changedFields = append(changedFields, "name")
	}
	if params.Description.Valid {
		changedFields = append(changedFields, "description")
	}
	if params.Context.Valid {
		changedFields = append(changedFields, "context")
	}
	if params.Settings != nil {
		changedFields = append(changedFields, "settings")
	}
	if params.Repos != nil {
		changedFields = append(changedFields, "repos")
	}
	if params.IssuePrefix.Valid {
		changedFields = append(changedFields, "issue_prefix")
	}
	if params.AvatarUrl.Valid {
		changedFields = append(changedFields, "avatar_url")
	}
	if params.SetSecretaryAgent {
		changedFields = append(changedFields, "secretary_agent_id")
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workspace")
		return
	}
	defer tx.Rollback(r.Context())

	ws, err := h.Queries.WithTx(tx).UpdateWorkspace(r.Context(), params)
	if err != nil {
		slog.Warn("update workspace failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", id)...)
		writeError(w, http.StatusInternalServerError, "failed to update workspace")
		return
	}
	if err := h.recordAudit(r, tx, audit.Entry{
		WorkspaceID: uuidToString(ws.ID),
		Action:      audit.ActionWorkspaceUpdated,
		TargetType:  "workspace",
		TargetID:    uuidToString(ws.ID),
		TargetLabel: ws.Name,
		Detail:      map[string]any{"changed_fields": changedFields},
	}); err != nil {
		slog.Warn("audit workspace updated failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", id)...)
		writeError(w, http.StatusInternalServerError, "failed to update workspace")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update workspace")
		return
	}

	slog.Info("workspace updated", append(logger.RequestAttrs(r), "workspace_id", id)...)
	userID := requestUserID(r)
	h.publish(protocol.EventWorkspaceUpdated, uuidToString(ws.ID), "member", userID, map[string]any{"workspace": h.workspaceToResponse(ws)})
	// A rename changes what daemons display; a settings edit changes how they
	// behave — the GitHub master switch and the Co-authored-by toggle are read
	// from this JSONB. Daemons cache settings and have no other way to learn
	// they moved, so both edits have to wake every member's daemons.
	if req.Name != nil || req.Settings != nil {
		if members, err := h.Queries.ListMembers(r.Context(), ws.ID); err == nil {
			userIDs := make([]string, 0, len(members))
			for _, member := range members {
				userIDs = append(userIDs, uuidToString(member.UserID))
			}
			h.notifyDaemonWorkspacesChanged(userIDs...)
		}
	}

	writeJSON(w, http.StatusOK, h.workspaceToResponse(ws))
}

func (h *Handler) ListMembers(w http.ResponseWriter, r *http.Request) {
	workspaceID := chi.URLParam(r, "id")
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}

	members, err := h.Queries.ListMembers(r.Context(), member.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}

	resp := make([]MemberResponse, len(members))
	for i, m := range members {
		resp[i] = memberToResponse(m)
	}

	writeJSON(w, http.StatusOK, resp)
}

type MemberWithUserResponse struct {
	ID          string  `json:"id"`
	WorkspaceID string  `json:"workspace_id"`
	UserID      string  `json:"user_id"`
	Role        string  `json:"role"`
	CreatedAt   string  `json:"created_at"`
	Name        string  `json:"name"`
	Email       string  `json:"email"`
	AvatarURL   *string `json:"avatar_url"`
}

func (h *Handler) ListMembersWithUser(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}

	members, err := h.Queries.ListMembersWithUser(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list members")
		return
	}

	resp := make([]MemberWithUserResponse, len(members))
	for i, m := range members {
		resp[i] = MemberWithUserResponse{
			ID:          uuidToString(m.ID),
			WorkspaceID: uuidToString(m.WorkspaceID),
			UserID:      uuidToString(m.UserID),
			Role:        m.Role,
			CreatedAt:   timestampToString(m.CreatedAt),
			Name:        m.UserName,
			Email:       m.UserEmail,
			AvatarURL:   h.resolveAvatarURLPtr(textToPtr(m.UserAvatarUrl)),
		}
	}

	writeJSON(w, http.StatusOK, resp)
}

type CreateMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

func (h *Handler) memberWithUserResponse(member db.Member, user db.User) MemberWithUserResponse {
	return MemberWithUserResponse{
		ID:          uuidToString(member.ID),
		WorkspaceID: uuidToString(member.WorkspaceID),
		UserID:      uuidToString(member.UserID),
		Role:        member.Role,
		CreatedAt:   timestampToString(member.CreatedAt),
		Name:        user.Name,
		Email:       user.Email,
		AvatarURL:   h.resolveAvatarURLPtr(textToPtr(user.AvatarUrl)),
	}
}

func normalizeMemberRole(role string) (string, bool) {
	if role == "" {
		return "member", true
	}

	role = strings.TrimSpace(role)
	switch role {
	case "owner", "admin", "member", workspaceguard.RoleViewer:
		return role, true
	default:
		return "", false
	}
}

// canAssignMemberRole reports whether a member with requesterRole may give a
// member the role (spec 6.6): an owner assigns any role, an admin assigns every
// role but owner, and no one else assigns a role.
func canAssignMemberRole(requesterRole, role string) bool {
	switch requesterRole {
	case "owner":
		return true
	case "admin":
		return role != "owner"
	}
	return false
}

type UpdateMemberRequest struct {
	Role string `json:"role"`
}

func (h *Handler) UpdateMember(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}

	memberID := chi.URLParam(r, "memberId")
	memberUUID, ok := parseUUIDOrBadRequest(w, memberID, "member id")
	if !ok {
		return
	}
	target, err := h.Queries.GetMember(r.Context(), memberUUID)
	if err != nil || uuidToString(target.WorkspaceID) != uuidToString(requester.WorkspaceID) {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}

	var req UpdateMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if strings.TrimSpace(req.Role) == "" {
		writeError(w, http.StatusBadRequest, "role is required")
		return
	}

	role, valid := normalizeMemberRole(req.Role)
	if !valid {
		writeError(w, http.StatusBadRequest, "invalid member role")
		return
	}

	if !canAssignMemberRole(requester.Role, role) || (target.Role == "owner" && requester.Role != "owner") {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	if target.Role == "owner" && role != "owner" {
		members, err := h.Queries.ListMembers(r.Context(), target.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to update member")
			return
		}
		if countOwners(members) <= 1 {
			writeError(w, http.StatusBadRequest, "workspace must have at least one owner")
			return
		}
	}

	user, err := h.Queries.GetUser(r.Context(), target.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load member")
		return
	}

	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update member")
		return
	}
	defer tx.Rollback(r.Context())

	updatedMember, err := h.Queries.WithTx(tx).UpdateMemberRole(r.Context(), db.UpdateMemberRoleParams{
		ID:   target.ID,
		Role: role,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update member")
		return
	}
	if err := h.recordAudit(r, tx, audit.Entry{
		WorkspaceID: uuidToString(target.WorkspaceID),
		Action:      audit.ActionMemberRoleChanged,
		TargetType:  "member",
		TargetID:    uuidToString(target.UserID),
		TargetLabel: user.Email,
		Detail:      map[string]any{"from": target.Role, "to": role},
	}); err != nil {
		slog.Warn("audit member role changed failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to update member")
		return
	}
	if err := tx.Commit(r.Context()); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to update member")
		return
	}

	h.MembershipCache.Invalidate(r.Context(), uuidToString(target.UserID), workspaceID)

	userID := requestUserID(r)
	h.publish(protocol.EventMemberUpdated, uuidToString(requester.WorkspaceID), "member", userID, map[string]any{
		"member": h.memberWithUserResponse(updatedMember, user),
	})

	writeJSON(w, http.StatusOK, h.memberWithUserResponse(updatedMember, user))
}

func (h *Handler) DeleteMember(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}

	memberID := chi.URLParam(r, "memberId")
	memberUUID, ok := parseUUIDOrBadRequest(w, memberID, "member id")
	if !ok {
		return
	}
	target, err := h.Queries.GetMember(r.Context(), memberUUID)
	if err != nil || uuidToString(target.WorkspaceID) != uuidToString(requester.WorkspaceID) {
		writeError(w, http.StatusNotFound, "member not found")
		return
	}

	if target.Role == "owner" && requester.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	if target.Role == "owner" {
		members, err := h.Queries.ListMembers(r.Context(), target.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to delete member")
			return
		}
		if countOwners(members) <= 1 {
			writeError(w, http.StatusBadRequest, "workspace must have at least one owner")
			return
		}
	}

	targetUser, err := h.Queries.GetUser(r.Context(), target.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete member")
		return
	}

	requesterUserID := requestUserID(r)
	result, err := h.revokeAndRemoveMember(r.Context(), target.WorkspaceID, target.UserID, target.ID, parseUUID(requesterUserID), func(tx pgx.Tx) error {
		return h.recordAudit(r, tx, audit.Entry{
			WorkspaceID: uuidToString(target.WorkspaceID),
			Action:      audit.ActionMemberRemoved,
			TargetType:  "member",
			TargetID:    uuidToString(target.UserID),
			TargetLabel: targetUser.Email,
		})
	})
	if err != nil {
		slog.Warn("delete member failed", append(logger.RequestAttrs(r), "error", err, "member_id", memberID, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to delete member")
		return
	}
	h.Hub.DisconnectMember(uuidToString(target.WorkspaceID), uuidToString(target.UserID), uuidToString(target.ID))
	h.settleMemberCapacityRelease(r.Context(), uuid.UUID(target.WorkspaceID.Bytes), uuid.UUID(target.ID.Bytes))

	h.MembershipCache.Invalidate(r.Context(), uuidToString(target.UserID), workspaceID)

	wsIDStr := uuidToString(requester.WorkspaceID)
	logRevocation(result, wsIDStr, uuidToString(target.UserID))
	h.publishRevocation(r.Context(), result, wsIDStr, "member", requesterUserID)

	slog.Info("member removed", append(logger.RequestAttrs(r), "member_id", uuidToString(target.ID), "workspace_id", workspaceID, "user_id", uuidToString(target.UserID))...)
	h.publish(protocol.EventMemberRemoved, wsIDStr, "member", requesterUserID, map[string]any{
		"member_id":    uuidToString(target.ID),
		"workspace_id": wsIDStr,
		"user_id":      uuidToString(target.UserID),
	})
	h.notifyDaemonWorkspacesChanged(uuidToString(target.UserID))

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) LeaveWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}

	if member.Role == "owner" {
		members, err := h.Queries.ListMembers(r.Context(), member.WorkspaceID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to leave workspace")
			return
		}
		if countOwners(members) <= 1 {
			writeError(w, http.StatusBadRequest, "workspace must have at least one owner")
			return
		}
	}

	leaver, err := h.Queries.GetUser(r.Context(), member.UserID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to leave workspace")
		return
	}

	result, err := h.revokeAndRemoveMember(r.Context(), member.WorkspaceID, member.UserID, member.ID, member.UserID, func(tx pgx.Tx) error {
		return h.recordAudit(r, tx, audit.Entry{
			WorkspaceID: uuidToString(member.WorkspaceID),
			Action:      audit.ActionMemberLeft,
			TargetType:  "member",
			TargetID:    uuidToString(member.UserID),
			TargetLabel: leaver.Email,
		})
	})
	if err != nil {
		slog.Warn("leave workspace failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to leave workspace")
		return
	}
	h.Hub.DisconnectMember(uuidToString(member.WorkspaceID), uuidToString(member.UserID), uuidToString(member.ID))
	h.settleMemberCapacityRelease(r.Context(), uuid.UUID(member.WorkspaceID.Bytes), uuid.UUID(member.ID.Bytes))

	h.MembershipCache.Invalidate(r.Context(), uuidToString(member.UserID), workspaceID)

	userID := requestUserID(r)
	logRevocation(result, workspaceID, uuidToString(member.UserID))
	h.publishRevocation(r.Context(), result, workspaceID, "member", userID)

	slog.Info("member removed", append(logger.RequestAttrs(r), "member_id", uuidToString(member.ID), "workspace_id", workspaceID, "user_id", uuidToString(member.UserID))...)
	h.publish(protocol.EventMemberRemoved, workspaceID, "member", userID, map[string]any{
		"member_id":    uuidToString(member.ID),
		"workspace_id": workspaceID,
		"user_id":      uuidToString(member.UserID),
	})
	h.notifyDaemonWorkspacesChanged(uuidToString(member.UserID))

	w.WriteHeader(http.StatusNoContent)
}

// workspaceDeleteLockTimeout bounds every lock wait inside the workspace
// teardown transaction.
//
// The teardown waits on three kinds of lock: the workspace row (FOR UPDATE),
// every chat_session row in the workspace (FOR UPDATE), and the global
// advisory lock 4246 that serialises it against the task_usage_hourly rollup.
// The advisory lock is the dangerous one — it is global and batch-held. A
// rollup tick may hold it for as long as its 25 min scheduler RunTimeout, the
// backfill commands (cmd/backfill_task_usage_hourly, cmd/backfill_codex_usage_cache)
// hold it for an entire run, and before migration 272 a tick whose query was
// cancelled leaked it for the remaining life of its pooled connection.
//
// Waiting on that lock without a cap is what the user sees as "delete does
// nothing": the request never returns, and no layer above it times out — the
// browser/Electron fetch in packages/core/api/client.ts has no deadline and
// the delete dialog stays in its "Deleting…" state forever (MUL-5983). A
// bounded wait turns the same contention into a retryable error.
//
// 10 s is far above the millisecond-scale waits an uncontended teardown sees,
// and short enough to fail while the user is still watching the dialog.
const workspaceDeleteLockTimeout = 10 * time.Second

// workspaceDeleteLockTimeoutOverride, when non-zero, replaces
// workspaceDeleteLockTimeout for the duration of a test. Never read outside
// effectiveWorkspaceDeleteLockTimeout — see workspace_delete_lock_test.go.
var workspaceDeleteLockTimeoutOverride time.Duration

func effectiveWorkspaceDeleteLockTimeout() time.Duration {
	if workspaceDeleteLockTimeoutOverride > 0 {
		return workspaceDeleteLockTimeoutOverride
	}
	return workspaceDeleteLockTimeout
}

// isRetryableLockFailure reports whether err is one of the two transient lock
// outcomes a teardown can hit: lock_not_available (55P03), which is what
// `SET LOCAL lock_timeout` raises when a wait exceeds its budget, and
// deadlock_detected (40P01), which the owner-row locks can hit against a
// concurrent task update that takes the same locks in the opposite order (task
// row first, then its owner). Neither says anything about the workspace itself:
// the transaction rolled back untouched and the caller can retry.
func isRetryableLockFailure(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == "55P03" || pgErr.Code == "40P01"
	}
	return false
}

// failWorkspaceDelete logs a failed teardown step and writes the response for
// it. A lock timeout or deadlock is transient and retryable, so it answers 503
// with a message the delete dialog can show; every other failure stays a 500.
func failWorkspaceDelete(w http.ResponseWriter, r *http.Request, workspaceID, step string, err error) {
	attrs := append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID, "step", step)
	if isRetryableLockFailure(err) {
		slog.Warn("workspace delete blocked by lock contention", attrs...)
		writeError(w, http.StatusServiceUnavailable, "workspace deletion is temporarily blocked by another operation, please try again")
		return
	}
	slog.Warn("workspace delete step failed", attrs...)
	writeError(w, http.StatusInternalServerError, "failed to delete workspace")
}

// workspaceDeleteTaskPageSize bounds one task page: the ids held in this process,
// the array parameter sent back to Postgres, and the row count of a single
// DELETE. A whole workspace's task set is not bounded by anything — one busy
// agent can own millions of historical rows — so it must never be materialized at
// once (MUL-5999 review).
//
// This and workspaceDeleteOwnerPageSize are variables only so the paging tests
// can cross a page boundary without seeding thousands of rows; nothing outside
// tests assigns them.
var workspaceDeleteTaskPageSize int32 = 1000

// workspaceDeleteOwnerPageSize bounds owner enumeration the same way. A workspace
// with a very large agent or issue set must not have its whole id list held here
// either.
var workspaceDeleteOwnerPageSize int32 = 500

// workspaceDeleteVerifyPasses caps how many times a single owner may be swept.
//
// Pass 1 removes what is there; pass 2 confirms the owner came back empty. This is
// a cheap self-check, NOT the concurrency guarantee: a task inserted after the
// final empty scan and before COMMIT would not be caught by any number of passes.
// What closes that window is the in-transaction fence on task writes
// (migration 284) plus the workspace row lock teardown already holds. The passes
// exist so that anything unexpected — a path that somehow bypassed the fence — is
// either cleaned up or turns into a failed, rolled-back teardown rather than a
// tenant with rows left behind.
const workspaceDeleteVerifyPasses = 3

// deleteWorkspaceTasks removes every agent_task_queue row a workspace owns,
// together with the rows that hang off those tasks, in bounded keyset pages.
//
// Shape, and why:
//
//   - One owner key per query. agent_task_queue has no workspace_id and all three
//     ownership paths must stay (nothing enforces that a task's agent, runtime and
//     issue share a workspace), but combining them in one statement — as the
//     previous three-way OR, as a UNION of joins, or as `= ANY(array)` over a
//     whole workspace's agents — leaves the planner estimating a proportional
//     share of the global table and choosing a Seq Scan.
//   - Keyset pages, so the SCAN is bounded and not just the result. Each page is
//     an index range scan that resumes where the previous one stopped. Bounding
//     only the result (`ORDER BY id LIMIT n` with no id in the index) puts a Sort
//     over the owner's whole task set in front of every page, which costs roughly
//     O(N² / page) for a busy owner.
//   - Row locks, not a read snapshot. Each page takes FOR UPDATE, so under READ
//     COMMITTED Postgres re-checks ownership after acquiring each lock: a task a
//     concurrent commit already moved elsewhere is skipped rather than deleted on
//     a stale claim, and a task we locked cannot move before we delete it.
//   - Children first, in the application layer. DetachTaskBatchReferences breaks
//     inbound references and DeleteTaskBatch removes task_usage, task_message,
//     task_token, the two outbound card tables and chat_draft_restore before the
//     task rows themselves. The legacy FK cascades stay a safety net only.
//
// Late arrivals are handled by the fence, not here: migration 284 makes every task
// write take FOR KEY SHARE on its owners' workspace rows, which conflicts with the
// FOR UPDATE that LockWorkspaceForDelete holds for this whole transaction.
func deleteWorkspaceTasks(ctx context.Context, qtx *db.Queries, workspaceID pgtype.UUID) error {
	ownerKinds := []struct {
		label          string
		ownerFirstPage func(context.Context, int32) ([]pgtype.UUID, error)
		ownerPage      func(context.Context, pgtype.UUID, int32) ([]pgtype.UUID, error)
		taskFirstPage  func(context.Context, pgtype.UUID, int32) ([]pgtype.UUID, error)
		taskPage       func(context.Context, pgtype.UUID, pgtype.UUID, int32) ([]pgtype.UUID, error)
		sweepAgent     bool
	}{
		{
			label: "agent",
			ownerFirstPage: func(ctx context.Context, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListWorkspaceAgentIDFirstPage(ctx, db.ListWorkspaceAgentIDFirstPageParams{
					WorkspaceID: workspaceID, Limit: limit,
				})
			},
			ownerPage: func(ctx context.Context, cursor pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListWorkspaceAgentIDPage(ctx, db.ListWorkspaceAgentIDPageParams{
					WorkspaceID: workspaceID, ID: cursor, Limit: limit,
				})
			},
			taskFirstPage: func(ctx context.Context, owner pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListTaskIDsByAgentFirstPage(ctx, db.ListTaskIDsByAgentFirstPageParams{
					AgentID: owner, Limit: limit,
				})
			},
			taskPage: func(ctx context.Context, owner, cursor pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListTaskIDsByAgentPage(ctx, db.ListTaskIDsByAgentPageParams{
					AgentID: owner, ID: cursor, Limit: limit,
				})
			},
			// The third explicit task_token path. workspace_id (leaf data) and
			// task_id (DeleteTaskBatch) leave one shape uncovered: a token whose
			// own workspace_id points at a neighbour while its agent lives here.
			sweepAgent: true,
		},
		{
			label: "issue",
			ownerFirstPage: func(ctx context.Context, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListWorkspaceIssueIDFirstPage(ctx, db.ListWorkspaceIssueIDFirstPageParams{
					WorkspaceID: workspaceID, Limit: limit,
				})
			},
			ownerPage: func(ctx context.Context, cursor pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListWorkspaceIssueIDPage(ctx, db.ListWorkspaceIssueIDPageParams{
					WorkspaceID: workspaceID, ID: cursor, Limit: limit,
				})
			},
			taskFirstPage: func(ctx context.Context, owner pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListTaskIDsByIssueFirstPage(ctx, db.ListTaskIDsByIssueFirstPageParams{
					IssueID: owner, Limit: limit,
				})
			},
			taskPage: func(ctx context.Context, owner, cursor pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListTaskIDsByIssuePage(ctx, db.ListTaskIDsByIssuePageParams{
					IssueID: owner, ID: cursor, Limit: limit,
				})
			},
		},
		{
			label: "runtime",
			ownerFirstPage: func(ctx context.Context, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListWorkspaceRuntimeIDFirstPage(ctx, db.ListWorkspaceRuntimeIDFirstPageParams{
					WorkspaceID: workspaceID, Limit: limit,
				})
			},
			ownerPage: func(ctx context.Context, cursor pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListWorkspaceRuntimeIDPage(ctx, db.ListWorkspaceRuntimeIDPageParams{
					WorkspaceID: workspaceID, ID: cursor, Limit: limit,
				})
			},
			taskFirstPage: func(ctx context.Context, owner pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListTaskIDsByRuntimeFirstPage(ctx, db.ListTaskIDsByRuntimeFirstPageParams{
					RuntimeID: owner, Limit: limit,
				})
			},
			taskPage: func(ctx context.Context, owner, cursor pgtype.UUID, limit int32) ([]pgtype.UUID, error) {
				return qtx.ListTaskIDsByRuntimePage(ctx, db.ListTaskIDsByRuntimePageParams{
					RuntimeID: owner, ID: cursor, Limit: limit,
				})
			},
		},
	}

	for _, kind := range ownerKinds {
		owners, err := kind.ownerFirstPage(ctx, workspaceDeleteOwnerPageSize)
		if err != nil {
			return fmt.Errorf("list workspace %ss: %w", kind.label, err)
		}
		for len(owners) > 0 {
			for _, ownerID := range owners {
				if err := sweepTasksForOwner(ctx, kind.label, ownerID,
					kind.taskFirstPage, kind.taskPage,
					qtx.DetachTaskBatchReferences, qtx.DeleteTaskBatch); err != nil {
					return err
				}
				if kind.sweepAgent {
					if err := qtx.DeleteTaskTokensByAgent(ctx, ownerID); err != nil {
						return fmt.Errorf("delete task tokens by agent: %w", err)
					}
				}
			}
			if owners, err = kind.ownerPage(ctx, owners[len(owners)-1], workspaceDeleteOwnerPageSize); err != nil {
				return fmt.Errorf("list workspace %ss: %w", kind.label, err)
			}
		}
	}

	return nil
}

// sweepTasksForOwner deletes one owner's tasks, then re-verifies that owner.
//
// Each pass walks the owner's tasks by keyset and deletes each page. A pass that
// deletes nothing means the owner is clean. A later pass that still finds tasks
// means something reached agent_task_queue without going through the fence, which
// is logged and retried; exhausting the passes fails the whole teardown rather
// than committing a workspace with tasks left behind.
func sweepTasksForOwner(
	ctx context.Context,
	label string,
	ownerID pgtype.UUID,
	taskFirstPage func(context.Context, pgtype.UUID, int32) ([]pgtype.UUID, error),
	taskPage func(context.Context, pgtype.UUID, pgtype.UUID, int32) ([]pgtype.UUID, error),
	detach func(context.Context, []pgtype.UUID) error,
	deletePage func(context.Context, []pgtype.UUID) error,
) error {
	for pass := 1; pass <= workspaceDeleteVerifyPasses; pass++ {
		deleted := 0
		taskIDs, err := taskFirstPage(ctx, ownerID, workspaceDeleteTaskPageSize)
		if err != nil {
			return fmt.Errorf("list tasks by %s: %w", label, err)
		}
		for len(taskIDs) > 0 {
			if err := detach(ctx, taskIDs); err != nil {
				return fmt.Errorf("detach task page references: %w", err)
			}
			if err := deletePage(ctx, taskIDs); err != nil {
				return fmt.Errorf("delete task page: %w", err)
			}
			deleted += len(taskIDs)
			if taskIDs, err = taskPage(ctx, ownerID, taskIDs[len(taskIDs)-1], workspaceDeleteTaskPageSize); err != nil {
				return fmt.Errorf("list tasks by %s: %w", label, err)
			}
		}
		if deleted == 0 {
			return nil
		}
		if pass > 1 {
			slog.Warn("workspace delete swept tasks that appeared after the owner was fenced",
				"owner_kind", label, "owner_id", uuidToString(ownerID), "pass", pass, "deleted", deleted)
		}
	}
	return fmt.Errorf("tasks kept appearing for %s %s after %d sweep passes; refusing to commit a partial teardown",
		label, uuidToString(ownerID), workspaceDeleteVerifyPasses)
}

// lockWorkspaceTaskOwners locks every row through which a task can belong to this
// workspace.
//
// Defence in depth on top of the real fence. The fence that closes the
// insert-after-sweep window is migration 284's lock_task_owner_rows, a predicate
// every ownership-writing statement calls in its own WHERE clause: it takes
// FOR KEY SHARE on the workspace rows behind the task's owners and then on the
// owner rows themselves, which conflicts with the FOR UPDATE that
// LockWorkspaceForDelete holds for this whole transaction. Called from the writing
// statement, so it is in the writer's transaction by construction, and independent
// of the legacy foreign keys.
//
// These owner-row locks add a second, narrower barrier — they also block
// ownership-changing UPDATEs of tasks that already exist — and cost nothing to
// hold, since teardown deletes these very rows a few steps later. They are taken
// set-based and return no ids, so a workspace with a huge owner set costs no
// memory here.
//
// The wait is bounded by SET LOCAL lock_timeout, and taking these locks can
// deadlock against a concurrent task update that locks in the opposite order
// (task row, then owner); both outcomes surface as a retryable 503.
func lockWorkspaceTaskOwners(ctx context.Context, qtx *db.Queries, workspaceID pgtype.UUID) error {
	if err := qtx.LockWorkspaceTaskOwnerAgents(ctx, workspaceID); err != nil {
		return fmt.Errorf("lock workspace agents: %w", err)
	}
	if err := qtx.LockWorkspaceTaskOwnerIssues(ctx, workspaceID); err != nil {
		return fmt.Errorf("lock workspace issues: %w", err)
	}
	if err := qtx.LockWorkspaceTaskOwnerRuntimes(ctx, workspaceID); err != nil {
		return fmt.Errorf("lock workspace runtimes: %w", err)
	}
	return nil
}

func (h *Handler) DeleteWorkspace(w http.ResponseWriter, r *http.Request) {
	workspaceID := workspaceIDFromURL(r, "id")

	// Defense in depth: the route is already gated by the
	// RequireWorkspaceRoleFromURL("owner") middleware, but we re-check here
	// so that the handler is safe regardless of how it gets wired up
	// (direct calls in tests, future router refactors, etc.).
	requester, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return
	}
	if requester.Role != "owner" {
		writeError(w, http.StatusForbidden, "insufficient permissions")
		return
	}

	// With a grace period configured the deletion is scheduled, not carried out;
	// the workspace_deletion job deletes it through the teardown below.
	if scheduleAt := workspacedeletion.Decide(time.Now(), h.WorkspaceDeletionDays); scheduleAt != nil {
		h.scheduleWorkspaceDeletion(w, r, requester, *scheduleAt)
		return
	}

	// Stop remote work before deleting the only durable session journal.
	// A pending step/cancellation leaves the workspace intact for a safe retry.
	if err := h.prepareQoderWorkspaceDeletion(r.Context(), uuidToString(requester.WorkspaceID)); err != nil {
		writeError(w, http.StatusConflict, "Qoder cleanup is pending; retry workspace deletion shortly")
		return
	}

	// Invalidate membership cache for all workspace members before deletion.
	// After CASCADE deletes the member rows, cache entries become harmless
	// orphans (downstream lookups for the deleted workspace will fail), but
	// proactive invalidation prevents any stale-access window up to TTL.
	var affectedUserIDs []string
	if members, err := h.Queries.ListMembers(r.Context(), requester.WorkspaceID); err == nil {
		affectedUserIDs = make([]string, 0, len(members))
		for _, m := range members {
			userID := uuidToString(m.UserID)
			h.MembershipCache.Invalidate(r.Context(), userID, workspaceID)
			affectedUserIDs = append(affectedUserIDs, userID)
		}
	}

	// The teardown runs in one transaction so the chat_session row locks below
	// are still held when DeleteWorkspace sweeps chat_draft_restore. Without
	// them, FinalizeDeferredCancelledChat could commit a restore for one of
	// these sessions after the sweep's snapshot was taken: the session cascades
	// away, the restore has no FK to follow it (MUL-3515) and no reaper, and the
	// user's prompt is stranded forever (#5219). The finalizer takes the same
	// lock before inserting, so it either blocks until the session is gone and
	// skips the insert, or commits first and the sweep sees its row.
	//
	// The workspace row is locked first, because the session locks only cover
	// sessions that already exist: a CreateChatSession committing inside the
	// delete window would otherwise slip in a session nobody locked, and its
	// restore would outlive the cascade the same way. Holding the workspace row
	// FOR UPDATE blocks that insert on its workspace FK (FOR KEY SHARE).
	tx, err := h.TxStarter.Begin(r.Context())
	if err != nil {
		slog.Warn("begin workspace delete tx failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}
	defer tx.Rollback(r.Context())
	teardown, err := h.deleteWorkspaceData(r.Context(), tx, requester.WorkspaceID)
	if err != nil {
		var stepErr *workspaceDeleteStepError
		if errors.As(err, &stepErr) {
			failWorkspaceDelete(w, r, workspaceID, stepErr.step, stepErr.err)
		} else {
			failWorkspaceDelete(w, r, workspaceID, "delete workspace data", err)
		}
		return
	}

	if err := h.recordAudit(r, tx, audit.Entry{
		WorkspaceID: uuidToString(requester.WorkspaceID),
		Action:      audit.ActionWorkspaceDeleted,
		TargetType:  "workspace",
		TargetID:    uuidToString(requester.WorkspaceID),
		TargetLabel: teardown.Name,
	}); err != nil {
		failWorkspaceDelete(w, r, workspaceID, "audit", err)
		return
	}

	if err := tx.Commit(r.Context()); err != nil {
		slog.Warn("commit workspace delete failed", append(logger.RequestAttrs(r), "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "failed to delete workspace")
		return
	}
	for _, runtimeID := range teardown.RuntimeIDs {
		h.NotifyRuntimeGone(uuidToString(runtimeID))
	}
	h.deleteS3Objects(r.Context(), teardown.ObjectURLs)
	h.deleteHubObjects(r.Context(), workspaceID)

	slog.Info("workspace deleted", append(logger.RequestAttrs(r), "workspace_id", workspaceID)...)
	h.publish(protocol.EventWorkspaceDeleted, workspaceID, "member", requestUserID(r), map[string]any{
		"workspace_id": workspaceID,
	})
	h.notifyDaemonWorkspacesChanged(affectedUserIDs...)

	w.WriteHeader(http.StatusNoContent)
}

// workspaceTeardown is what a workspace deletion leaves for its caller to
// finish once the transaction has committed.
type workspaceTeardown struct {
	// Name is the workspace name, for the audit row.
	Name string
	// RuntimeIDs are the runtimes whose daemons must be told they are gone.
	RuntimeIDs []pgtype.UUID
	// ObjectURLs are stored objects to delete.
	ObjectURLs []string
}

// workspaceDeleteStepError names the teardown step that failed, so callers log
// the same step names whether a handler or the account deletion job ran it.
type workspaceDeleteStepError struct {
	step string
	err  error
}

func (e *workspaceDeleteStepError) Error() string { return e.step + ": " + e.err.Error() }
func (e *workspaceDeleteStepError) Unwrap() error { return e.err }

// deleteWorkspaceData runs the whole workspace teardown inside tx, which the
// caller begins, commits and rolls back. It does not write the audit row or
// notify anyone: both belong to the caller, who knows the actor.
func (h *Handler) deleteWorkspaceData(ctx context.Context, tx pgx.Tx, wsID pgtype.UUID) (workspaceTeardown, error) {
	var teardown workspaceTeardown
	qtx := h.Queries.WithTx(tx)
	doomed, err := qtx.GetWorkspace(ctx, wsID)
	if err != nil {
		return teardown, &workspaceDeleteStepError{step: "load workspace for audit", err: err}
	}
	var sourceContextAttachmentURLs []string
	var sourceContextIntentURLs []string

	// SET LOCAL is transaction-scoped, so pgxpool hands this connection back
	// out with the default (unbounded) lock_timeout after COMMIT / ROLLBACK.
	// It caps waiting for a lock, not the teardown work itself — deleting a
	// large workspace is allowed to take as long as it takes.
	lockTimeoutMs := int(effectiveWorkspaceDeleteLockTimeout() / time.Millisecond)
	if _, err := tx.Exec(ctx, fmt.Sprintf("SET LOCAL lock_timeout = %d", lockTimeoutMs)); err != nil {
		return teardown, &workspaceDeleteStepError{step: "set lock timeout", err: err}
	}

	if _, err := qtx.LockWorkspaceForDelete(ctx, wsID); err != nil {
		return teardown, &workspaceDeleteStepError{step: "lock workspace", err: err}
	}
	// Take a best-effort snapshot for post-commit daemon invalidation. Runtime
	// registration does not participate in the workspace delete lock protocol,
	// so PR1 retains the heartbeat lookup as the correctness fallback for a
	// registration that races this snapshot.
	runtimeIDs, err := qtx.ListAgentRuntimeIDsByWorkspace(ctx, wsID)
	if err != nil {
		return teardown, &workspaceDeleteStepError{step: "list runtimes", err: err}
	}

	if _, err := qtx.LockChatSessionsByWorkspace(ctx, wsID); err != nil {
		return teardown, &workspaceDeleteStepError{step: "lock chat sessions", err: err}
	}
	if sourceContextAttachmentURLs, err = qtx.ListSourceContextAttachmentURLsByWorkspace(ctx, wsID); err != nil {
		return teardown, &workspaceDeleteStepError{step: "list source context attachment objects", err: err}
	}
	if sourceContextIntentURLs, err = qtx.ListSourceContextObjectIntentURLsByWorkspace(ctx, wsID); err != nil {
		return teardown, &workspaceDeleteStepError{step: "list source context pending objects", err: err}
	}

	// Keep the relationship graph in the application layer. Each step is a
	// set-based delete scoped by workspace_id; the legacy cascades remain only
	// as an expand-phase safety net until a later schema contract.
	deleteSteps := []struct {
		name string
		run  func() error
	}{
		{
			name: "set teardown mode",
			run:  func() error { return qtx.SetWorkspaceTeardownMode(ctx) },
		},
		{
			// Fences task enqueue / reassignment for the rest of the
			// transaction. Must run before anything sweeps tasks — see
			// lockWorkspaceTaskOwners.
			name: "lock task owners",
			run:  func() error { return lockWorkspaceTaskOwners(ctx, qtx, wsID) },
		},
		{
			name: "prepare relationship graph",
			run:  func() error { return qtx.PrepareWorkspaceDeletionLinks(ctx, wsID) },
		},
		{
			// These FK-free intents deliberately survive the transaction so the
			// worker can release every invitation/member seat after local rows
			// disappear. Skipped for self-hosted/unmanaged deployments.
			name: "prepare seat capacity release",
			run: func() error {
				if !h.seatCapacityEnabled() {
					return nil
				}
				if err := qtx.DeleteSeatCapacityConfirmIntentsForWorkspaceDeletion(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.PrepareSeatCapacityOperationReleasesForWorkspaceDeletion(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.PrepareSeatCapacityInvitationReleasesForWorkspaceDeletion(ctx, wsID); err != nil {
					return err
				}
				return qtx.PrepareSeatCapacityMemberReleasesForWorkspaceDeletion(ctx, wsID)
			},
		},
		{
			name: "delete chat pins",
			run:  func() error { return qtx.DeleteChatPinnedAgentsByWorkspace(ctx, wsID) },
		},
		{
			// This is the first stage that touches usage rollups. Keep the
			// global rollup lock out of relationship preparation so unrelated
			// workspaces skip the shortest possible rollup window. This wait
			// is bounded by workspaceDeleteLockTimeout — 4246 is held by
			// batch jobs, so an unbounded wait here is what hangs the delete.
			name: "lock task usage rollup",
			run:  func() error { return qtx.LockTaskUsageRollupForWorkspaceDelete(ctx) },
		},
		{
			// Bounded batches, after the rollup lock because it deletes
			// task_usage. Replaces both the task-keyed arms of the old leaf
			// data statement and the separate whole-workspace task delete.
			name: "delete tasks",
			run:  func() error { return deleteWorkspaceTasks(ctx, qtx, wsID) },
		},
		{
			name: "delete Qoder Cloud Agents connection",
			run: func() error {
				if _, err := tx.Exec(ctx, `DELETE FROM daemon_token WHERE id IN (SELECT token_id FROM cloud_agent_connection WHERE provider='qoder_cloud' AND workspace_id=$1)`, wsID); err != nil {
					return err
				}
				_, err := tx.Exec(ctx, `DELETE FROM cloud_agent_connection WHERE provider='qoder_cloud' AND workspace_id=$1`, wsID)
				return err
			},
		},
		{
			// The WeKnora tenant is not removed here: it is queued and removed
			// after its hold (plan 10.14). Runs before the final statement that
			// deletes the knowledge registry the queue reads.
			name: "queue knowledge teardown",
			run: func() error {
				return service.QueueKnowledgeTeardown(ctx, qtx, wsID, time.Now())
			},
		},
		{
			name: "delete leaf data",
			run:  func() error { return qtx.DeleteWorkspaceLeafData(ctx, wsID) },
		},
		{
			name: "delete autopilot runs",
			run:  func() error { return qtx.DeleteWorkspaceAutopilotRuns(ctx, wsID) },
		},
		{
			name: "delete autopilot quota reservations",
			run:  func() error { return qtx.DeleteWorkspaceAutopilotQuotaReservations(ctx, wsID) },
		},
		{
			name: "delete autopilot quota periods",
			run:  func() error { return qtx.DeleteWorkspaceAutopilotQuotaPeriods(ctx, wsID) },
		},
		{
			name: "delete chat messages",
			run:  func() error { return qtx.DeleteWorkspaceChatMessages(ctx, wsID) },
		},
		{
			name: "delete communication roots",
			run:  func() error { return qtx.DeleteWorkspaceCommunicationRoots(ctx, wsID) },
		},
		{
			name: "delete comments",
			run:  func() error { return qtx.DeleteWorkspaceComments(ctx, wsID) },
		},
		{
			// Teardown mode keeps the triggers from logging the deletes above
			// and below; this clears what normal writes logged before.
			name: "delete search index changes",
			run:  func() error { return qtx.DeleteWorkspaceSearchIndexChanges(ctx, wsID) },
		},
		// Keep source-context object intents after the workspace row is gone.
		// They are the durable retry ledger for an upload that began before the
		// workspace lock but completed after this transaction's immediate object
		// deletion pass. With no attachment left, the reconciler will delete the
		// object and then the intent; deleting the ledger here would make that
		// crash window permanently leak the late object.
		{
			name: "delete source context attachments",
			run:  func() error { return qtx.DeleteSourceContextAttachmentsByWorkspace(ctx, wsID) },
		},
		{
			name: "delete source contexts",
			run: func() error {
				_, err := qtx.DeleteIssueSourceContextsForWorkspace(ctx, wsID)
				return err
			},
		},
		{
			name: "delete issue roots",
			run:  func() error { return qtx.DeleteWorkspaceIssueRoots(ctx, wsID) },
		},
		{
			// sso_connection and sso_domain carry no foreign key, so nothing
			// else removes them. A verified domain left behind would stay
			// claimed by a workspace that no longer exists.
			name: "delete sso connection",
			run: func() error {
				_, err := qtx.DeleteSSOConnection(ctx, wsID)
				return err
			},
		},
		{
			name: "delete sso domains",
			run:  func() error { return qtx.DeleteSSODomainsByWorkspace(ctx, wsID) },
		},
		{
			// issue_status carries no foreign key by project rule, so its rows
			// are swept explicitly. Placed after the issue deletes so no issue
			// row outlives the catalog its status key resolves against.
			// (MUL-6243)
			name: "delete issue statuses",
			run: func() error {
				return qtx.DeleteIssueStatusEntriesForWorkspace(ctx, wsID)
			},
		},
		{
			name: "delete flow drafts",
			run:  func() error { return qtx.DeleteFlowDraftsForWorkspace(ctx, wsID) },
		},
		{
			name: "delete squad pack installs",
			run:  func() error { return qtx.DeleteSquadPackInstallsForWorkspace(ctx, wsID) },
		},
		{
			// issue_workflow references the status catalog and projects by
			// value only, so it is swept alongside the catalog. (MUL-7420)
			name: "delete issue workflows",
			run: func() error {
				return qtx.DeleteIssueWorkflowsForWorkspace(ctx, wsID)
			},
		},
		{
			name: "delete squad routines",
			run:  func() error { return qtx.DeleteWorkspaceSquadRoutines(ctx, wsID) },
		},
		{
			name: "delete autopilot children",
			run:  func() error { return qtx.DeleteWorkspaceAutopilotChildren(ctx, wsID) },
		},
		{
			name: "delete autopilots",
			run:  func() error { return qtx.DeleteWorkspaceAutopilots(ctx, wsID) },
		},
		{
			name: "delete pull requests",
			run:  func() error { return qtx.DeleteWorkspacePullRequests(ctx, wsID) },
		},
		{
			name: "delete integrations",
			run:  func() error { return qtx.DeleteWorkspaceConnections(ctx, wsID) },
		},
		{
			name: "delete squad charters",
			run:  func() error { return qtx.DeleteWorkspaceSquadCharters(ctx, wsID) },
		},
		{
			name: "delete squad attention",
			run:  func() error { return qtx.DeleteWorkspaceSquadAttention(ctx, wsID) },
		},
		{
			name: "delete squads and skills",
			run:  func() error { return qtx.DeleteWorkspaceSquadsAndSkills(ctx, wsID) },
		},
		{
			// Brief pushes and the member preferences behind them carry no
			// foreign keys, so they are swept explicitly (plan 9.10).
			name: "delete brief deliveries",
			run:  func() error { return qtx.DeleteWorkspaceBriefDeliveries(ctx, wsID) },
		},
		{
			name: "delete brief push preferences",
			run:  func() error { return qtx.DeleteWorkspaceBriefPushPreferences(ctx, wsID) },
		},
		{
			// Ratings of briefs have no foreign keys either, and rate the briefs
			// swept next, so they go first (plan 9.12).
			name: "delete brief feedback",
			run:  func() error { return qtx.DeleteWorkspaceBriefFeedback(ctx, wsID) },
		},
		{
			name: "delete briefs",
			run:  func() error { return qtx.DeleteWorkspaceBriefs(ctx, wsID) },
		},
		{
			name: "delete brief inquiries",
			run:  func() error { return qtx.DeleteWorkspaceBriefInquiries(ctx, wsID) },
		},
		{
			// Revisions of proposals; the proposals themselves go next.
			name: "delete proposal revisions",
			run:  func() error { return qtx.DeleteWorkspaceProposalRevisions(ctx, wsID) },
		},
		{
			name: "delete proposals",
			run:  func() error { return qtx.DeleteWorkspaceProposals(ctx, wsID) },
		},
		{
			// Initiatives hold references to their backing issues and pipelines;
			// the timeline and checklists go first, then the rows they belong to.
			name: "delete initiatives",
			run: func() error {
				if err := qtx.DeleteWorkspaceInitiativeArtifacts(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceInitiativeStageEvents(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceInitiatives(ctx, wsID); err != nil {
					return err
				}
				return qtx.DeleteWorkspacePipelines(ctx, wsID)
			},
		},
		{
			// Budgets hold references to tasks; the holds go first, then the
			// periods and the budgets.
			name: "delete budgets",
			run: func() error {
				if err := qtx.DeleteWorkspaceBudgetHolds(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceBudgetPeriods(ctx, wsID); err != nil {
					return err
				}
				return qtx.DeleteWorkspaceBudgets(ctx, wsID)
			},
		},
		{
			// The governance log goes with its workspace; it is the one DELETE
			// on governance_event (see governance_event.sql).
			name: "delete governance log",
			run:  func() error { return qtx.DeleteWorkspaceGovernanceEvents(ctx, wsID) },
		},
		{
			// Org change applications, reviews, scorecards and instruction
			// revisions. None has a foreign key, so their order does not matter.
			name: "delete org records",
			run: func() error {
				if err := qtx.DeleteWorkspaceOrgChangeApplications(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceReviews(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceScorecards(ctx, wsID); err != nil {
					return err
				}
				return qtx.DeleteWorkspaceAgentInstructionRevisions(ctx, wsID)
			},
		},
		{
			// Goals and their key results, the planning issues' records, and the
			// metrics with their readings. None of them has a foreign key.
			name: "delete goals and metrics",
			run: func() error {
				if err := qtx.DeleteWorkspacePlanningCycles(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceKeyResults(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceGoals(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceMetricReadings(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceMetricTokens(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceRetroRequests(ctx, wsID); err != nil {
					return err
				}
				return qtx.DeleteWorkspaceMetrics(ctx, wsID)
			},
		},
		{
			// Retrospective triggers and floor chiefs have no foreign keys.
			name: "delete retro triggers and floor chiefs",
			run: func() error {
				if err := qtx.DeleteWorkspaceRetroTriggers(ctx, wsID); err != nil {
					return err
				}
				return qtx.DeleteWorkspaceFloorChiefs(ctx, wsID)
			},
		},
		{
			// Standing approvals go with their workspace. Proposals refer to them by
			// id only, so the order against the proposal rows does not matter.
			name: "delete approval policies",
			run:  func() error { return qtx.DeleteWorkspaceApprovalPolicies(ctx, wsID) },
		},
		{
			// Content Studio publishing: the send log first, then the channels, the keys and
			// the profile key. None of these tables has a foreign key.
			name: "delete publishing channels",
			run: func() error {
				if err := qtx.DeleteWorkspacePublishAttempts(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspacePublishChannels(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspacePublishSettings(ctx, wsID); err != nil {
					return err
				}
				return qtx.DeleteWorkspacePublishProfile(ctx, wsID)
			},
		},
		{
			// Research Lab source settings (sealed API keys) go with their workspace.
			name: "delete research settings",
			run:  func() error { return qtx.DeleteWorkspaceResearchSettings(ctx, wsID) },
		},
		{
			name: "delete hub data",
			run:  func() error { return qtx.DeleteWorkspaceHubData(ctx, wsID) },
		},
		{
			name: "delete knowledge property history",
			run:  func() error { return qtx.DeleteWorkspaceKnowledgePropertyHistory(ctx, wsID) },
		},
		{
			name: "delete plugin data",
			run:  func() error { return qtx.DeleteWorkspacePluginData(ctx, wsID) },
		},
		{
			name: "delete agents",
			run:  func() error { return qtx.DeleteWorkspaceAgents(ctx, wsID) },
		},
		{
			name: "delete runtimes and projects",
			run:  func() error { return qtx.DeleteWorkspaceRuntimesAndProjects(ctx, wsID) },
		},
		{
			name: "delete administration data",
			run:  func() error { return qtx.DeleteWorkspaceAdministration(ctx, wsID) },
		},
		{
			// Platform state the workspace owns. billing_order and the plan and
			// account tables are kept on purpose (see workspace_delete_manifest_test.go).
			// The export archives in storage are not removed here.
			name: "delete platform state",
			run: func() error {
				if err := qtx.DeleteWorkspaceBillingNotices(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspacePlatformNotes(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspacePlatformSeatOperations(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspacePlatformUsageDaily(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceSubscriptions(ctx, wsID); err != nil {
					return err
				}
				if err := qtx.DeleteWorkspaceExports(ctx, wsID); err != nil {
					return err
				}
				return qtx.DeletePlatformWorkspace(ctx, wsID)
			},
		},
		{
			name: "detach audit events",
			run:  func() error { return qtx.DetachWorkspaceAuditEvents(ctx, wsID) },
		},
		{
			// The caller has resolved the workspace to a valid UUID. The existing final
			// statement also sweeps any expand-phase compatibility leftovers.
			name: "delete workspace",
			run:  func() error { return qtx.DeleteWorkspace(ctx, wsID) },
		},
	}
	for _, step := range deleteSteps {
		if err := step.run(); err != nil {
			return teardown, &workspaceDeleteStepError{step: step.name, err: err}
		}
	}
	teardown.Name = doomed.Name
	teardown.RuntimeIDs = runtimeIDs
	teardown.ObjectURLs = append(sourceContextAttachmentURLs, sourceContextIntentURLs...)
	return teardown, nil
}
