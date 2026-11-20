package handler

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/storage"
	"github.com/multica-ai/multica/server/internal/util"
)

// exportLinkTTL is how long the link to a workspace archive stays valid (spec R6 3c.9).
const exportLinkTTL = 7 * 24 * time.Hour

// NotifyExportReady mails the workspace's owners a link to its archive, valid for
// seven days. The archive is not an object of the teardown, so the link still
// works after the workspace is deleted. Where the store cannot sign a link (local
// disk), no link is sent; the archive stays available through the export route.
func (h *Handler) NotifyExportReady(ctx context.Context, workspaceID, storageKey string) error {
	wsID, err := util.ParseUUID(workspaceID)
	if err != nil {
		return fmt.Errorf("parse workspace id: %w", err)
	}
	presigner, ok := h.Storage.(storage.DownloadPresigner)
	if !ok {
		slog.Info("workspace export: store cannot sign a link; no link sent", "workspace_id", workspaceID)
		return nil
	}
	link, err := presigner.PresignGetWithContentDisposition(ctx, storageKey, exportLinkTTL, storage.AttachmentContentDisposition("workspace-export.zip"))
	if errors.Is(err, storage.ErrPresignUnsupported) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("sign export link: %w", err)
	}
	ws, err := h.Queries.GetWorkspace(ctx, wsID)
	if err != nil {
		return fmt.Errorf("load workspace: %w", err)
	}
	members, err := h.Queries.ListMembers(ctx, wsID)
	if err != nil {
		return fmt.Errorf("list members: %w", err)
	}
	for _, m := range members {
		if m.Role != "owner" {
			continue
		}
		owner, err := h.Queries.GetUser(ctx, m.UserID)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return fmt.Errorf("load owner: %w", err)
		}
		if h.EmailService == nil {
			continue
		}
		if err := h.EmailService.SendWorkspaceExportReadyEmail(owner.Email, ws.Name, link); err != nil {
			return fmt.Errorf("send export link: %w", err)
		}
	}
	return nil
}
