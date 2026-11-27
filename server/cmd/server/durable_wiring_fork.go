package main

import (
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/multica-ai/multica/server/internal/handler"
	"github.com/multica-ai/multica/server/internal/scheduler"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
	"log/slog"
)

func registerDurableWiring(schedulerMgr *scheduler.Manager, queries *db.Queries, pool *pgxpool.Pool, taskSvc *service.TaskService, h *handler.Handler, flags *featureflag.Service) {
	// Every minute: a transition into done opens a work outcome, and due outcomes are settled (durable spec 2.4).
	durableJury := service.NewJuryService(queries, pool, taskSvc)
	durableJury.Flags = flags
	durableChecks := &service.VerificationService{Queries: queries, TxStarter: pool, Tasks: taskSvc, Jury: durableJury, Flags: flags}
	trackRecords := &service.TrackRecordService{Queries: queries, TxStarter: pool, Flags: flags}
	durableDossier := &service.DossierService{Queries: queries, TxStarter: pool, Issues: h.IssueService, Flags: flags}
	durableOutcomes := &service.OutcomeService{Queries: queries, TxStarter: pool, Flags: flags, Verification: durableChecks, TrackRecords: trackRecords, Dossier: durableDossier}
	if err := schedulerMgr.Register(scheduler.DurableOutcomeJob(durableOutcomes)); err != nil {
		slog.Error("scheduler: register durable outcome", "error", err)
	}
	if err := schedulerMgr.Register(scheduler.DurableSeatCleanupJob(durableDossier)); err != nil {
		slog.Error("scheduler: register durable seat cleanup", "error", err)
	}
	durablePredictions := &service.PredictionService{Queries: queries, TxStarter: pool, Tasks: taskSvc, Flags: flags}
	if err := schedulerMgr.Register(scheduler.PrincipalLearningJob(durablePredictions)); err != nil {
		slog.Error("scheduler: register principal learning", "error", err)
	}
	if err := schedulerMgr.Register(scheduler.DurableMetricJob(durableOutcomes)); err != nil {
		slog.Error("scheduler: register durable metric", "error", err)
	}
	if err := schedulerMgr.Register(scheduler.DurableJuryTimeoutJob(durableJury)); err != nil {
		slog.Error("scheduler: register durable jury timeout", "error", err)
	}
	if h.DurableIntents() != nil {
		if err := schedulerMgr.Register(scheduler.DurableIntentExpiryJob(h.DurableIntents())); err != nil {
			slog.Error("scheduler: register durable intent expiry", "error", err)
		}
	}
	// Daily: every pair with a settled outcome in 90 days is recomputed (durable spec 3.2).
	if err := schedulerMgr.Register(scheduler.DurableTrackRecordJob(trackRecords)); err != nil {
		slog.Error("scheduler: register durable track record", "error", err)
	}
	// Daily: squads move inside their autonomy envelopes on evidence (durable spec 3.4).
	envelopes := &service.EnvelopeService{Queries: queries, TxStarter: pool, Flags: flags}
	if err := schedulerMgr.Register(scheduler.DurableEnvelopeJob(envelopes)); err != nil {
		slog.Error("scheduler: register durable envelope", "error", err)
	}
	registerDurableShadowJobs(schedulerMgr, taskSvc.Shadow)
}
