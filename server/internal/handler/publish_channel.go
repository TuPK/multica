package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/multica-ai/multica/server/internal/publish"
)

// Publishing channels (Content Studio). A channel is one publishing account of
// the workspace. Accounts are linked through Ayrshare, under one User Profile per
// workspace. Connecting and removing channels is for owners and admins; reading
// a channel and dry-running a post is for any member. Agents never manage
// channels.

// PublishChannelResponse is one channel on the wire.
type PublishChannelResponse struct {
	ID            string         `json:"id"`
	Platform      string         `json:"platform"`
	Market        string         `json:"market"`
	AccountName   string         `json:"account_name"`
	Status        string         `json:"status"`
	StatusDetail  string         `json:"status_detail"`
	LastCheckedAt *string        `json:"last_checked_at"`
	Limits        publish.Limits `json:"limits"`
}

// CreatePublishChannelRequest is the POST /api/publish-channels body.
type CreatePublishChannelRequest struct {
	Platform    string `json:"platform"`
	Market      string `json:"market"`
	AccountName string `json:"account_name"`
}

// DryRunPublishChannelRequest is the POST /{id}/dry-run body.
type DryRunPublishChannelRequest struct {
	Text     string   `json:"text"`
	MediaURL string   `json:"media_url"`
	Hashtags []string `json:"hashtags"`
}

func publishChannelToResponse(c publish.Channel) PublishChannelResponse {
	resp := PublishChannelResponse{
		ID:           c.ID,
		Platform:     c.Platform,
		Market:       c.Market,
		AccountName:  c.AccountName,
		Status:       c.Status,
		StatusDetail: c.StatusDetail,
		Limits:       c.Limits,
	}
	if c.LastCheckedAt != nil {
		s := c.LastCheckedAt.UTC().Format(time.RFC3339)
		resp.LastCheckedAt = &s
	}
	return resp
}

// publishReady refuses a request when the publishing service is missing. The
// router always builds it, so this only guards a handler built without it.
func (h *Handler) publishReady(w http.ResponseWriter) bool {
	if h.Publish == nil {
		writeErrorCode(w, http.StatusServiceUnavailable, "ayrshare_not_configured", publish.ErrNotConfigured.Error())
		return false
	}
	return true
}

// publishManager resolves the caller as an owner or admin of the workspace.
func (h *Handler) publishManager(w http.ResponseWriter, r *http.Request) (string, bool) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return "", false
	}
	if isMachineCredentialActor(r) {
		writeError(w, http.StatusForbidden, "only an owner or admin may manage publishing channels; agents cannot")
		return "", false
	}
	member, ok := h.workspaceMember(w, r, workspaceID)
	if !ok {
		return "", false
	}
	if !roleAllowed(member.Role, "owner", "admin") {
		writeError(w, http.StatusForbidden, "only an owner or admin may manage publishing channels")
		return "", false
	}
	return workspaceID, true
}

// ListPublishChannels (GET /api/publish-channels) returns the workspace's channels.
func (h *Handler) ListPublishChannels(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	channels, err := h.Publish.List(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to list publishing channels")
		return
	}
	out := make([]PublishChannelResponse, 0, len(channels))
	for _, c := range channels {
		out = append(out, publishChannelToResponse(c))
	}
	writeJSON(w, http.StatusOK, map[string]any{"channels": out})
}

// CreatePublishChannel (POST /api/publish-channels) adds a channel that is not
// connected yet.
func (h *Handler) CreatePublishChannel(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID, ok := h.publishManager(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req CreatePublishChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	channel, err := h.Publish.Create(r.Context(), wsUUID, strings.TrimSpace(req.Platform), strings.TrimSpace(req.Market), req.AccountName)
	switch {
	case errors.Is(err, publish.ErrUnknownPlatform), errors.Is(err, publish.ErrUnknownMarket), errors.Is(err, publish.ErrAccountName):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case errors.Is(err, publish.ErrPlatformTaken):
		writeErrorCode(w, http.StatusConflict, "platform_taken", err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to create publishing channel")
		return
	}
	writeJSON(w, http.StatusCreated, publishChannelToResponse(channel))
}

// DeletePublishChannel (DELETE /api/publish-channels/{id}) removes a channel of
// the workspace. A channel of another workspace is a 404.
func (h *Handler) DeletePublishChannel(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID, ok := h.publishManager(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "channel id")
	if !ok {
		return
	}
	if err := h.Publish.Delete(r.Context(), wsUUID, id); err != nil {
		if errors.Is(err, publish.ErrChannelNotFound) {
			writeError(w, http.StatusNotFound, "publishing channel not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to delete publishing channel")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ConnectPublishChannel (POST /api/publish-channels/{id}/connect) returns the
// link the user opens to connect the channel's account.
func (h *Handler) ConnectPublishChannel(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID, ok := h.publishManager(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "channel id")
	if !ok {
		return
	}
	url, err := h.Publish.Connect(r.Context(), wsUUID, id)
	switch {
	case errors.Is(err, publish.ErrChannelNotFound):
		writeError(w, http.StatusNotFound, "publishing channel not found")
		return
	case errors.Is(err, publish.ErrNotConfigured):
		writeErrorCode(w, http.StatusServiceUnavailable, "ayrshare_not_configured", err.Error())
		return
	case err != nil:
		writeError(w, http.StatusBadGateway, "failed to start the account link")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"url": url})
}

// CheckPublishChannel (POST /api/publish-channels/{id}/check) asks Ayrshare whether
// the account is linked and returns the channel with its new status.
func (h *Handler) CheckPublishChannel(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "channel id")
	if !ok {
		return
	}
	channel, err := h.Publish.Check(r.Context(), wsUUID, id)
	if errors.Is(err, publish.ErrChannelNotFound) {
		writeError(w, http.StatusNotFound, "publishing channel not found")
		return
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to check publishing channel")
		return
	}
	writeJSON(w, http.StatusOK, publishChannelToResponse(channel))
}

// DryRunPublishChannel (POST /api/publish-channels/{id}/dry-run) returns the exact
// Ayrshare request body a post on this channel would send. Nothing is sent.
func (h *Handler) DryRunPublishChannel(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "channel id")
	if !ok {
		return
	}
	var req DryRunPublishChannelRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	body, err := h.Publish.DryRun(r.Context(), wsUUID, id, req.Text, strings.TrimSpace(req.MediaURL), req.Hashtags)
	switch {
	case errors.Is(err, publish.ErrChannelNotFound):
		writeError(w, http.StatusNotFound, "publishing channel not found")
		return
	case errors.Is(err, publish.ErrTextEmpty), errors.Is(err, publish.ErrTextTooLong):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to build the post")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"body": body})
}

// PublishSettingsResponse says which of a workspace's Ayrshare keys are set. It
// never carries key material.
type PublishSettingsResponse struct {
	AyrshareConfigured bool `json:"ayrshare_configured"`
	XConfigured        bool `json:"x_configured"`
}

// PublishSettingsRequest is a partial write of the keys. A field left out is
// unchanged; an empty string clears that key.
type PublishSettingsRequest struct {
	AyrshareAPIKey *string `json:"ayrshare_api_key"`
	XAPIKey        *string `json:"x_api_key"`
	XAPISecret     *string `json:"x_api_secret"`
}

func publishSettingsResponse(s publish.Settings) PublishSettingsResponse {
	return PublishSettingsResponse{AyrshareConfigured: s.AyrshareConfigured, XConfigured: s.XConfigured}
}

// GetPublishSettings (GET /api/publish-channels/settings) reports which keys the
// workspace has set. Any member may read it. The keys are never returned.
func (h *Handler) GetPublishSettings(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := requireUserID(w, r); !ok {
		return
	}
	if _, ok := h.workspaceMember(w, r, workspaceID); !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	settings, err := h.Publish.Settings(r.Context(), wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read publishing settings")
		return
	}
	writeJSON(w, http.StatusOK, publishSettingsResponse(settings))
}

// PutPublishSettings (PUT /api/publish-channels/settings) saves the workspace's
// Ayrshare keys. Only owners and admins may write. The keys are sealed before they
// are stored, and the answer carries only which keys are set.
func (h *Handler) PutPublishSettings(w http.ResponseWriter, r *http.Request) {
	if !h.publishReady(w) {
		return
	}
	workspaceID, ok := h.publishManager(w, r)
	if !ok {
		return
	}
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !ok {
		return
	}
	var req PublishSettingsRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	settings, err := h.Publish.SaveSettings(r.Context(), wsUUID, publish.SettingsPatch{
		AyrshareKey: req.AyrshareAPIKey,
		XAPIKey:     req.XAPIKey,
		XAPISecret:  req.XAPISecret,
	})
	switch {
	case errors.Is(err, publish.ErrSecretsNotConfigured):
		writeErrorCode(w, http.StatusServiceUnavailable, "publish_secret_not_configured", err.Error())
		return
	case errors.Is(err, publish.ErrKeyInvalid):
		writeError(w, http.StatusBadRequest, err.Error())
		return
	case err != nil:
		writeError(w, http.StatusInternalServerError, "failed to save publishing settings")
		return
	}
	writeJSON(w, http.StatusOK, publishSettingsResponse(settings))
}
