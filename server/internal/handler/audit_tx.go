package handler

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// auditWriteError reports that the audit row of a change could not be written.
// It does not wrap the cause: a handler that maps pgx.ErrNoRows or a unique
// violation to a client error must not read a failed audit write as one.
type auditWriteError struct{ msg string }

func (e *auditWriteError) Error() string { return "audit write failed: " + e.msg }

// isAuditWriteError reports whether err is a failed audit write, for a handler
// that maps its other errors to a client status.
func isAuditWriteError(err error) bool {
	var target *auditWriteError
	return errors.As(err, &target)
}

// auditTx runs change in one transaction and records the audit entry it
// returns in the same transaction, so the change and its row commit or roll
// back together. An error from change is returned unchanged; a failed audit
// write is returned as *auditWriteError. A change that returns an entry with no
// action made nothing worth recording, and only commits.
func (h *Handler) auditTx(r *http.Request, change func(qtx *db.Queries) (audit.Entry, error)) error {
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	entry, err := change(qtx)
	if err != nil {
		return err
	}
	if entry.Action != "" {
		if err := h.recordAuditWith(r, qtx, entry); err != nil {
			return &auditWriteError{msg: err.Error()}
		}
	}
	return tx.Commit(ctx)
}

// recordAuditTx is recordAudit for a caller that holds the transaction as
// queries already bound to it.
func (h *Handler) recordAuditTx(r *http.Request, qtx *db.Queries, e audit.Entry) error {
	if err := h.recordAuditWith(r, qtx, e); err != nil {
		return &auditWriteError{msg: err.Error()}
	}
	return nil
}

// integrationEntry is the audit entry of a connected or disconnected
// integration. provider names the integration; label is the account or bot name
// the settings page shows. It never carries credentials or configuration.
func integrationEntry(action, workspaceID, id, label, provider string) audit.Entry {
	return audit.Entry{
		WorkspaceID: workspaceID,
		Action:      action,
		TargetType:  "integration",
		TargetID:    id,
		TargetLabel: label,
		Detail:      map[string]any{"provider": provider},
	}
}

// revokeChannelInstallation flips a Slack, Lark, Telegram, WeCom or DingTalk
// installation to revoked and records integration.disconnected in the same
// transaction. The five integrations share the channel_installation row and
// this status change, so one path serves them. On failure it answers 500 and
// returns false.
func (h *Handler) revokeChannelInstallation(w http.ResponseWriter, r *http.Request, provider string, wsUUID, instUUID pgtype.UUID) bool {
	err := h.auditTx(r, func(qtx *db.Queries) (audit.Entry, error) {
		if err := qtx.SetChannelInstallationStatus(r.Context(), db.SetChannelInstallationStatusParams{
			ID:     instUUID,
			Status: "revoked",
		}); err != nil {
			return audit.Entry{}, err
		}
		return integrationEntry(audit.ActionIntegrationDisconnected, uuidToString(wsUUID), uuidToString(instUUID), "", provider), nil
	})
	if err != nil {
		slog.Warn("revoke installation failed", "provider", provider, "installation_id", uuidToString(instUUID), "error", err)
		writeError(w, http.StatusInternalServerError, "failed to revoke installation")
		return false
	}
	return true
}

// pluginAudit returns the hook that records action for a plugin change in the
// transaction of the change. extra adds detail fields; it may be nil. The
// detail always names the plugin and its version, and never a configuration
// value, a secret or a token.
func (h *Handler) pluginAudit(r *http.Request, action string, extra func(c service.PluginChange) map[string]any) service.PluginTxHook {
	return func(_ context.Context, q *db.Queries, c service.PluginChange) error {
		detail := map[string]any{"plugin": c.Installation.PluginKey, "version": c.Installation.Version}
		if extra != nil {
			for k, v := range extra(c) {
				detail[k] = v
			}
		}
		return h.recordAuditTx(r, q, audit.Entry{
			WorkspaceID: uuidToString(c.Installation.WorkspaceID),
			Action:      action,
			TargetType:  "plugin",
			TargetID:    uuidToString(c.Installation.ID),
			TargetLabel: c.Installation.PluginKey,
			Detail:      detail,
		})
	}
}
