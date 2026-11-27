package main

import (
	"log/slog"

	"github.com/multica-ai/multica/server/internal/scheduler"
	"github.com/multica-ai/multica/server/internal/service"
)

// registerDurableShadowJobs registers the scheduled reconciliation job for shadow runs.
func registerDurableShadowJobs(mgr *scheduler.Manager, shadow *service.ShadowService) {
	if shadow == nil {
		return
	}
	if err := mgr.Register(scheduler.DurableShadowReconcileJob(shadow)); err != nil {
		slog.Error("scheduler: register durable shadow reconcile", "error", err)
	}
}
