package staffdeletion

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/internal/util"
)

// exportWait bounds how long an export_first request waits for the archives of
// its workspaces (spec R6 3c.9). Past it the request fails and keeps its data.
const exportWait = 24 * time.Hour

// exportTimeoutCode is the error_code of a request whose archive did not finish in time.
const exportTimeoutCode = "export_timeout"

// awaitExports holds a workspace or tenant request until every workspace it
// covers has a finished archive, then mails the owners a link to each one. The
// link is sent before the teardown, which keeps the archive. An archive that does
// not finish within exportWait fails the request. A request without export_first
// does not call this.
func (r *Runner) awaitExports(ctx context.Context, req db.PlatformDeletionRequest, wsIDs []pgtype.UUID) error {
	type ready struct{ wsID, key string }
	var archives []ready
	for _, id := range wsIDs {
		row, err := r.d.Queries.GetReadyWorkspaceExport(ctx, id)
		if errors.Is(err, pgx.ErrNoRows) {
			if time.Since(req.CreatedAt.Time) >= exportWait {
				return &permanentError{Code: exportTimeoutCode}
			}
			return errWaitingForChildren
		}
		if err != nil {
			return fmt.Errorf("read archive: %w", err)
		}
		archives = append(archives, ready{wsID: util.UUIDToString(id), key: row.StorageKey})
	}
	for _, a := range archives {
		if err := r.d.Eraser.NotifyExportReady(ctx, a.wsID, a.key); err != nil {
			return fmt.Errorf("notify export: %w", err)
		}
	}
	return nil
}
