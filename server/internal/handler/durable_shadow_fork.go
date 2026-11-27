package handler

import (
	"context"
	"net/http"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// RewireShadowService gives the task service its shadow hook again. handler.New builds it before
// the router sets the feature flags, so the hook would otherwise sample with nil flags.
func (h *Handler) RewireShadowService() {
	h.TaskService.Shadow = h.shadowService()
}

// shadowService is the shadow run service the task service judges runs with (durable spec
// 9.4). It is built from the handler's stores, so a run's juror and its runtime change use the
// same proposal and organisation services as everything else.
func (h *Handler) shadowService() *service.ShadowService {
	svc := &service.ShadowService{
		Queries:   h.Queries,
		TxStarter: h.TxStarter,
		Tasks:     h.TaskService,
		Flags:     h.FeatureFlags,
		Runtime: &service.RuntimeChangeService{
			Queries:   h.Queries,
			Proposals: h.proposalService(),
			Changes:   h.orgChanges(),
			Flags:     h.FeatureFlags,
		},
	}
	// A necessity run is decided by its verdicts, not by the runtime change (durable spec 10.2).
	svc.Necessity = &service.NecessityService{
		Queries: h.Queries, TxStarter: h.TxStarter, Shadow: svc, Proposals: h.proposalService(), Flags: h.FeatureFlags,
	}
	return svc
}

// NecessityService is the scaffolding sunset service (durable spec 10): the scheduler's weekly and
// monthly jobs and the routes use it.
func (h *Handler) NecessityService() *service.NecessityService {
	return h.shadowService().Necessity
}

// shadowClaimContext carries the daemon's shadow capability into the claim: a daemon that did not
// advertise it is never given a shadow task (durable spec 9.4, D30).
func shadowClaimContext(r *http.Request) context.Context {
	return service.WithShadowCapableClaim(r.Context(), requestHasClientCapability(r, protocol.DaemonCapabilityShadowV1))
}

// applyShadowClaimFields marks a claimed shadow task for the daemon and leaves it without the
// project resources a worktree would come from (durable spec 9.4 (c)). Any other task is untouched.
func applyShadowClaimFields(resp *AgentTaskResponse, taskContext []byte) {
	fields := service.ShadowClaimFieldsOf(taskContext)
	if !fields.Shadow {
		return
	}
	resp.Shadow = true
	resp.OmitSkills = fields.OmitSkills
	resp.ProjectResources = nil
	if fields.OmitSkills && resp.Agent != nil {
		resp.Agent.Skills = nil
		resp.Agent.SkillRefs = nil
	}
}
