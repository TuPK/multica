package handler

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
)

// RunDurableJobsForTest runs the durable jobs once, now, the way the scheduler runs them:
// the outcome job (new transitions, then the outcomes that are due), the track record
// nightly, the envelope nightly, and principal learning (durable spec 2.4, 3.2, 3.4, 6.3). The clock is the `now`
// query parameter (RFC 3339, default the server's time), so an end-to-end test can move a
// week on and settle the outcomes that are due. Owners and admins only. Like the retro
// route, it exists only when MULTICA_TEST_ROUTES=1 (runbook phase 4, section 0).
//
// The verification service is not wired here, so no verification task is scheduled: a
// test records its checks through POST /api/outcomes/{id}/verifications.
//
//	POST /api/test/durable/jobs?now=2026-12-03T10:00:00Z
func (h *Handler) RunDurableJobsForTest(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.companyActorFor(w, r, workspaceID); !ok {
		return
	}
	now := time.Now()
	if raw := r.URL.Query().Get("now"); raw != "" {
		parsed, err := time.Parse(time.RFC3339Nano, raw)
		if err != nil {
			writeError(w, http.StatusBadRequest, "now must be an RFC 3339 time")
			return
		}
		now = parsed
	}
	clock := func() time.Time { return now }
	tracks := &service.TrackRecordService{Queries: h.Queries, TxStarter: h.TxStarter, Flags: h.FeatureFlags, Now: clock}
	outcomes := &service.OutcomeService{
		Queries: h.Queries, TxStarter: h.TxStarter, Flags: h.FeatureFlags, Now: clock, TrackRecords: tracks,
	}
	envelopes := &service.EnvelopeService{Queries: h.Queries, TxStarter: h.TxStarter, Flags: h.FeatureFlags, Now: clock}

	ctx := r.Context()
	fail := func(step string, err error) {
		slog.Warn("test durable jobs failed", append(logger.RequestAttrs(r), "step", step, "error", err, "workspace_id", workspaceID)...)
		writeError(w, http.StatusInternalServerError, "durable job failed: "+step)
	}
	opened, err := outcomes.ProcessTransitions(ctx)
	if err != nil {
		fail("transitions", err)
		return
	}
	settled, err := outcomes.Settle(ctx, now)
	if err != nil {
		fail("settle", err)
		return
	}
	pairs, err := tracks.RecomputeAll(ctx)
	if err != nil {
		fail("track_record", err)
		return
	}
	squads, err := envelopes.Nightly(ctx)
	if err != nil {
		fail("envelope", err)
		return
	}
	learning, err := (&service.PredictionService{Queries: h.Queries, TxStarter: h.TxStarter, Flags: h.FeatureFlags}).LearnBatches(ctx, now)
	if err != nil {
		fail("principal_learning", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int{
		"opened":         opened,
		"settled":        settled,
		"track_pairs":    pairs,
		"squads_moved":   squads,
		"learning_tasks": learning,
	})
}
