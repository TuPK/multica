package handler

import (
	"net/http"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// FlowCapabilitiesResponse is what this server understands of a flow document, so
// a client reads the limits and the lists from here and never hard-codes them
// (spec 2.7).
type FlowCapabilitiesResponse struct {
	Schemas        []int                    `json:"schemas"`
	Limits         issueworkflow.FlowLimits `json:"limits"`
	StepKinds      []string                 `json:"step_kinds"`
	TaskKinds      []string                 `json:"task_kinds"`
	JoinModes      []string                 `json:"join_modes"`
	JoinOver       []string                 `json:"join_over"`
	JoinBlock      []string                 `json:"join_block"`
	RoleKinds      []string                 `json:"role_kinds"`
	VerdictClasses []string                 `json:"verdict_classes"`
	VarTypes       []string                 `json:"var_types"`
	StopClasses    []string                 `json:"stop_classes"`
	EdgeTriggers   []string                 `json:"edge_triggers"`
	LoopResets     []string                 `json:"loop_resets"`
	OpsByType      map[string][]string      `json:"ops_by_type"`
	BuiltinFacts   []issueworkflow.FactInfo `json:"builtin_facts"`
	Features       FlowCapabilityFeatures   `json:"features"`
}

// FlowCapabilityFeatures says which parts of the model the server has. A feature
// the server does not have is false; the flags tell what an operator turned on.
type FlowCapabilityFeatures struct {
	Packs           bool `json:"packs"`
	Proposals       bool `json:"proposals"`
	SystemPerformer bool `json:"system_performer"`
	Subflow         bool `json:"subflow"`
	ForkJoin        bool `json:"fork_join"`
	Authoring       bool `json:"authoring"`
	GraphV2         bool `json:"graph_v2"`
}

// GetFlowCapabilities answers a member of the workspace.
func (h *Handler) GetFlowCapabilities(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	writeJSON(w, http.StatusOK, FlowCapabilitiesResponse{
		Schemas:        issueworkflow.SupportedSchemas,
		Limits:         issueworkflow.Limits(),
		StepKinds:      []string{issueworkflow.StepKindTasks, issueworkflow.StepKindLead, issueworkflow.StepKindManual},
		TaskKinds:      []string{issueworkflow.KindWork, issueworkflow.KindGate},
		JoinModes:      []string{issueworkflow.JoinModeAll, issueworkflow.JoinModeAny, issueworkflow.JoinModeQuorum},
		JoinOver:       []string{issueworkflow.JoinOverGates, issueworkflow.JoinOverAll, issueworkflow.JoinOverTasks},
		JoinBlock:      []string{issueworkflow.JoinBlockImmediate, issueworkflow.JoinBlockAtClose},
		RoleKinds:      []string{issueworkflow.RoleKindAgent, issueworkflow.RoleKindMember, issueworkflow.RoleKindLead, issueworkflow.RoleKindRequester, issueworkflow.RoleKindManagers},
		VerdictClasses: []string{issueworkflow.ClassPass, issueworkflow.ClassFail, issueworkflow.ClassNeutral},
		VarTypes:       []string{issueworkflow.VarTypeBool, issueworkflow.VarTypeEnum, issueworkflow.VarTypeInt, issueworkflow.VarTypeString, issueworkflow.VarTypeListString},
		StopClasses:    []string{"missing_grounds", "loop_exhausted", "custom"},
		EdgeTriggers:   []string{issueworkflow.EdgeTriggerAuto, issueworkflow.EdgeTriggerManual},
		LoopResets:     []string{issueworkflow.LoopResetOnEntry, issueworkflow.LoopResetNever},
		OpsByType:      issueworkflow.OpsByType(),
		BuiltinFacts:   issueworkflow.BuiltinFacts(),
		Features: FlowCapabilityFeatures{
			Authoring: featureflags.FlowAuthoringEnabled(r.Context(), h.FeatureFlags),
			GraphV2:   featureflags.FlowGraphV2Enabled(r.Context(), h.FeatureFlags),
		},
	})
}
