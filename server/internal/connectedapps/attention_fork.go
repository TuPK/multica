package connectedapps

import (
	"context"
	"log/slog"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// AttentionGate puts an app action request in the attention ledger, so the request counts
// against the squad's attention budget (durable spec 7.3). The service implements it; this
// package cannot import the service, which imports it.
type AttentionGate interface {
	AdmitAppRequest(ctx context.Context, req db.AppActionRequest) error
}

// admitAttention is the hook createRequest calls once a request is stored. A ledger failure
// is logged and never fails the request.
func (s *Service) admitAttention(ctx context.Context, req db.AppActionRequest) {
	if s.attention == nil {
		return
	}
	if err := s.attention.AdmitAppRequest(ctx, req); err != nil {
		slog.Warn("attention ledger: app action request not admitted",
			"request_id", util.UUIDToString(req.ID), "error", err)
	}
}
