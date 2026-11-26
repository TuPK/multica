package handler

import (
	"github.com/multica-ai/multica/server/internal/service"
)

// orgChanges returns the organisation change service the proposal kinds and the
// probation job apply through. Its autonomy hook is wired where the autonomy
// service is built.
func (h *Handler) orgChanges() *service.OrgChangeService {
	svc := service.NewOrgChangeService()
	svc.Entitlements = h.Entitlements
	svc.Flags = h.FeatureFlags
	return svc
}
