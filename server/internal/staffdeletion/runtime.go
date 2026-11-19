package staffdeletion

import (
	"context"
	"errors"

	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// runRuntime unregisters the runtime of a staff request through the owner's
// teardown. A refusal the owner's delete also gives (active agents, a managed
// connection, a live profile instance) fails the request at once with its code.
func (r *Runner) runRuntime(ctx context.Context, req db.PlatformDeletionRequest, origin handler.DeletionOrigin) error {
	_, err := r.d.Eraser.DeleteRuntimeForStaff(ctx, util.UUIDToString(req.TargetID), origin)
	var refused *handler.StaffRuntimeError
	if errors.As(err, &refused) {
		return &permanentError{Code: refused.Code}
	}
	return err
}
