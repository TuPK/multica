package service

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	proposalkinds "github.com/multica-ai/multica/server/internal/service/proposal_kinds"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// A runtime change moves a seat to the runtime and model its shadow run tested (durable
// spec 9.6). It is an org_change of kind runtime_change, so the owner decides it, and it
// is reverted and judged on probation like the other changes.

// runtimeNotLosingMin is the lower bound of "not losing" a candidate must reach (spec 9.6).
const runtimeNotLosingMin = 0.5

// runtimeChangePayload is the payload of a runtime_change. ShadowConfigID names the run
// whose report is the evidence; the run must be completed and belong to the seat.
type runtimeChangePayload struct {
	Type           string              `json:"type"`
	AgentID        string              `json:"agent_id"`
	ToRuntimeID    string              `json:"to_runtime_id"`
	ToModel        string              `json:"to_model"`
	ShadowConfigID string              `json:"shadow_config_id"`
	Report         runtimeChangeReport `json:"report"`
	Rationale      string              `json:"rationale"`
}

// runtimeChangeReport is the shadow report a runtime change rests on, as the proposal
// shows it.
type runtimeChangeReport struct {
	Pairs          int     `json:"pairs"`
	Shadow         int     `json:"shadow"`
	Tie            int     `json:"tie"`
	Primary        int     `json:"primary"`
	NotLosingLower float64 `json:"not_losing_lower"`
}

// PromotionReady applies spec 9.6 to a finished run: the Wilson lower bound of "not
// losing" reaches 0.5, and the candidate wins at least as often as it loses.
func PromotionReady(r ShadowReport) bool {
	return r.NotLosingLower >= runtimeNotLosingMin && r.Shadow >= r.Primary
}

// validateRuntimeChange checks a runtime change against its workspace, its seat and its
// shadow run. The run must be completed, so no change is filed ahead of its evidence.
func validateRuntimeChange(ctx context.Context, q *db.Queries, ws pgtype.UUID, p runtimeChangePayload) error {
	if err := requireRationale(p.Rationale); err != nil {
		return err
	}
	agent, err := orgAgent(ctx, q, ws, p.AgentID)
	if err != nil {
		return err
	}
	toRuntime, err := util.ParseUUID(p.ToRuntimeID)
	if err != nil {
		return fmt.Errorf("to_runtime_id: %w", err)
	}
	rt, err := q.GetAgentRuntime(ctx, toRuntime)
	if err != nil || rt.WorkspaceID != ws {
		return errors.New("to_runtime_id is not a runtime of this workspace")
	}
	cfgID, err := util.ParseUUID(p.ShadowConfigID)
	if err != nil {
		return fmt.Errorf("shadow_config_id: %w", err)
	}
	cfg, err := q.GetShadowConfig(ctx, db.GetShadowConfigParams{ID: cfgID, WorkspaceID: ws})
	if err != nil {
		return errors.New("shadow_config_id names no shadow run in this workspace")
	}
	if cfg.AgentID != agent.ID {
		return errors.New("the shadow run is for another seat")
	}
	if cfg.State != "completed" {
		return errors.New("the shadow run has not completed")
	}
	return nil
}

// applyRuntimeChange moves the seat to the payload's runtime and model, and writes
// seat_model_changed when durable_seats is on. It returns the runtime and model it replaced.
func (s *OrgChangeService) applyRuntimeChange(ctx context.Context, q *db.Queries, ws pgtype.UUID, actor GovernanceActor, p runtimeChangePayload) (orgSubject, orgBefore, error) {
	agent, err := orgAgent(ctx, q, ws, p.AgentID)
	if err != nil {
		return orgSubject{}, orgBefore{}, err
	}
	toRuntime, err := util.ParseUUID(p.ToRuntimeID)
	if err != nil {
		return orgSubject{}, orgBefore{}, err
	}
	after, err := q.SetAgentRuntimeAndModel(ctx, db.SetAgentRuntimeAndModelParams{
		ID: agent.ID, WorkspaceID: ws, RuntimeID: toRuntime, Model: textOf(p.ToModel),
	})
	if err != nil {
		return orgSubject{}, orgBefore{}, fmt.Errorf("set seat runtime: %w", err)
	}
	if err := RecordSeatModelChange(ctx, q, s.Flags, actor, agent, after); err != nil {
		return orgSubject{}, orgBefore{}, err
	}
	before := orgBefore{Model: agent.Model.String}
	if agent.RuntimeID.Valid {
		before.RuntimeID = util.UUIDToString(agent.RuntimeID)
	}
	return orgSubject{OrgSubjectAgent, agent.ID}, before, nil
}

// revertRuntimeChange puts the seat back on the runtime and model it had before the change.
func (s *OrgChangeService) revertRuntimeChange(ctx context.Context, q *db.Queries, ws pgtype.UUID, app db.OrgChangeApplication, before orgBefore, actor GovernanceActor) error {
	agent, err := q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: app.SubjectID, WorkspaceID: ws})
	if err != nil {
		return fmt.Errorf("load seat: %w", err)
	}
	var runtimeID pgtype.UUID
	if before.RuntimeID != "" {
		if runtimeID, err = util.ParseUUID(before.RuntimeID); err != nil {
			return err
		}
	}
	after, err := q.SetAgentRuntimeAndModel(ctx, db.SetAgentRuntimeAndModelParams{
		ID: agent.ID, WorkspaceID: ws, RuntimeID: runtimeID, Model: textOf(before.Model),
	})
	if err != nil {
		return fmt.Errorf("restore seat runtime: %w", err)
	}
	return RecordSeatModelChange(ctx, q, s.Flags, actor, agent, after)
}

// textOf is a text column's value: NULL for the empty string.
func textOf(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: s != ""}
}

// RuntimeChangeService files the runtime change a finished shadow run earns (spec 9.6).
type RuntimeChangeService struct {
	Queries   *db.Queries
	Proposals *ProposalService
	Changes   *OrgChangeService
	Flags     *featureflag.Service
}

// OnShadowCompleted marks the run completed and, when the candidate passes spec 9.6, files
// a runtime_change for the owner. With auto_promote the owner's standing setting decides it
// at once. Nothing is written while durable_seats is off.
func (s *RuntimeChangeService) OnShadowCompleted(ctx context.Context, cfg db.ShadowConfig, r ShadowReport) error {
	if s.Flags == nil || !featureflags.DurableSeatsEnabled(ctx, s.Flags) {
		return nil
	}
	if _, err := s.Queries.SetShadowConfigState(ctx, db.SetShadowConfigStateParams{
		ID: cfg.ID, WorkspaceID: cfg.WorkspaceID, State: "completed",
	}); err != nil {
		return fmt.Errorf("complete shadow run: %w", err)
	}
	if !PromotionReady(r) {
		return nil
	}
	seat, err := s.Queries.GetAgent(ctx, cfg.AgentID)
	if err != nil {
		return fmt.Errorf("load seat: %w", err)
	}
	if !seat.HomeSquadID.Valid {
		return errors.New("the seat has no squad to file its runtime change in")
	}
	payload, err := json.Marshal(runtimeChangePayload{
		Type:           OrgChangeRuntime,
		AgentID:        util.UUIDToString(cfg.AgentID),
		ToRuntimeID:    util.UUIDToString(cfg.CandidateRuntimeID),
		ToModel:        cfg.CandidateModel,
		ShadowConfigID: util.UUIDToString(cfg.ID),
		Report: runtimeChangeReport{
			Pairs: r.Shadow + r.Tie + r.Primary, Shadow: r.Shadow, Tie: r.Tie, Primary: r.Primary,
			NotLosingLower: r.NotLosingLower,
		},
		Rationale: "A shadow trial found the candidate at least as good as the seat's model.",
	})
	if err != nil {
		return fmt.Errorf("encode runtime change: %w", err)
	}
	res, err := s.Proposals.Create(ctx, ProposalCreateParams{
		WorkspaceID: cfg.WorkspaceID,
		SquadID:     seat.HomeSquadID,
		Content: ProposalContent{
			Title:          "Move the seat to the tested model",
			Summary:        "A shadow trial ran the candidate beside the seat's own work.",
			Evidence:       []string{fmt.Sprintf("Shadow run %s", util.UUIDToString(cfg.ID))},
			Recommendation: "Approve the change.",
			Options:        []string{"Approve", "Keep the seat's current model"},
			CostRisk:       "The candidate's cost is in the shadow report.",
			Ask:            "Approve the runtime change?",
		},
		CreatorType: GovernanceActorMember,
		CreatorID:   cfg.CreatedBy,
		Kind:        proposalkinds.KeyOrgChange,
		Payload:     payload,
	})
	if err != nil {
		return fmt.Errorf("file runtime change: %w", err)
	}
	if !cfg.AutoPromote {
		return nil
	}
	_, err = s.Proposals.Decide(ctx, ProposalDecideParams{
		WorkspaceID: cfg.WorkspaceID, ProposalID: res.Proposal.ID, DeciderID: cfg.CreatedBy,
		Decision: ProposalDecisionApprove,
	})
	return err
}
