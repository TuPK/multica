package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
)

// Hub quotas cap what one workspace may hold: its skill and plugin items, and the
// bytes of their bundles. Limits come from the environment, so a self-hosted
// deployment can change them without a release. A limit of 0 (or any value
// below 1) means unlimited, and the quota route reports it as 0 too.

const (
	hubMaxItemsEnv            = "HUB_MAX_ITEMS_PER_WORKSPACE"
	hubMaxStorageBytesEnv     = "HUB_MAX_STORAGE_BYTES_PER_WORKSPACE"
	hubDefaultMaxItems        = 500
	hubDefaultMaxStorageBytes = 1 << 30
)

// Resource names carried by HubQuotaError.
const (
	hubQuotaResourceItems   = "items"
	hubQuotaResourceStorage = "storage_bytes"
)

type hubQuotaLimits struct {
	Items        int64
	StorageBytes int64
}

func hubQuotaLimitsFromEnv() hubQuotaLimits {
	return hubQuotaLimits{
		Items:        hubEnvLimit(hubMaxItemsEnv, hubDefaultMaxItems),
		StorageBytes: hubEnvLimit(hubMaxStorageBytesEnv, hubDefaultMaxStorageBytes),
	}
}

// hubEnvLimit reads one limit. An unset or unparsable value takes the default,
// and an explicit value of zero or less means unlimited (returned as 0).
func hubEnvLimit(name string, def int64) int64 {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return def
	}
	n, err := strconv.ParseInt(raw, 10, 64)
	if err != nil {
		slog.Warn("hub quota: ignoring invalid limit, using default", "env", name, "value", raw)
		return def
	}
	if n < 1 {
		return 0
	}
	return n
}

// HubQuotaError is returned by checkHubQuota when an addition would put the
// workspace over a limit. writeHubQuotaError turns it into the 409 response.
type HubQuotaError struct {
	Resource string
	Used     int64
	Add      int64
	Limit    int64
}

func (e *HubQuotaError) Error() string {
	return fmt.Sprintf("hub %s quota exceeded: %d used + %d requested > limit %d", e.Resource, e.Used, e.Add, e.Limit)
}

// checkHubQuota reports a HubQuotaError when used+add exceeds a limit. It returns
// any other error only when the usage read itself fails. Callers that add items
// call it before writing, and the import and sync paths share this check.
func (h *Handler) checkHubQuota(ctx context.Context, workspaceID pgtype.UUID, addItems int, addBytes int64) error {
	usage, err := h.Queries.CountHubUsage(ctx, workspaceID)
	if err != nil {
		return fmt.Errorf("count hub usage: %w", err)
	}
	limits := hubQuotaLimitsFromEnv()
	if limits.Items > 0 && usage.ItemCount+int64(addItems) > limits.Items {
		return &HubQuotaError{Resource: hubQuotaResourceItems, Used: usage.ItemCount, Add: int64(addItems), Limit: limits.Items}
	}
	if limits.StorageBytes > 0 && usage.StorageBytes+addBytes > limits.StorageBytes {
		return &HubQuotaError{Resource: hubQuotaResourceStorage, Used: usage.StorageBytes, Add: addBytes, Limit: limits.StorageBytes}
	}
	return nil
}

// writeHubQuotaError writes the 409 for a HubQuotaError and reports true. Any
// other error is left to the caller, which reports it as a server failure.
func writeHubQuotaError(w http.ResponseWriter, err error) bool {
	var quotaErr *HubQuotaError
	if !errors.As(err, &quotaErr) {
		return false
	}
	msg := "hub item quota exceeded"
	if quotaErr.Resource == hubQuotaResourceStorage {
		msg = "hub storage quota exceeded"
	}
	writeErrorCode(w, http.StatusConflict, "quota_exceeded", msg)
	return true
}

type HubQuotaMeter struct {
	Used  int64 `json:"used"`
	Limit int64 `json:"limit"`
}

type HubQuotaResponse struct {
	Items        HubQuotaMeter `json:"items"`
	StorageBytes HubQuotaMeter `json:"storage_bytes"`
}

// GetHubQuota (GET /api/hub/quota) reports current usage and the limits in force.
// Every member may read it, so the UI can show headroom before an import fails.
func (h *Handler) GetHubQuota(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	usage, err := h.Queries.CountHubUsage(r.Context(), parseUUID(workspaceID))
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read hub usage")
		return
	}
	limits := hubQuotaLimitsFromEnv()
	writeJSON(w, http.StatusOK, HubQuotaResponse{
		Items:        HubQuotaMeter{Used: usage.ItemCount, Limit: limits.Items},
		StorageBytes: HubQuotaMeter{Used: usage.StorageBytes, Limit: limits.StorageBytes},
	})
}
