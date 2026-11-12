package service

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/catalog"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/util"
)

// The workflow templates of the catalog (catalog/workflows), as `pnpm catalog:build` writes them to
// the JSON the server embeds. The squad quickstart (packages/core/squads) reads the same manifests,
// so a workflow the web client creates and the one the server builds for an SDLC squad cannot drift.

type catalogWorkflowStep struct {
	StatusKey    string  `json:"statusKey"`
	Role         *string `json:"role"`
	OptionalRole bool    `json:"optionalRole"`
	Instructions string  `json:"instructions"`
	Next         string  `json:"next"`
	Back         string  `json:"back"`
	// ImplicitNext: the build leaves this step's forward target to the board order, as it did
	// before the catalog existed (spec 5.4). Dropping the flag gives the step an explicit default transition.
	ImplicitNext bool `json:"implicitNext"`
}

type catalogWorkflow struct {
	Name        string                `json:"name"`
	Description string                `json:"description"`
	Steps       []catalogWorkflowStep `json:"steps"`
}

var loadCatalogWorkflows = sync.OnceValues(func() (map[string]catalogWorkflow, error) {
	raw := catalog.WorkflowsJSON()
	if len(raw) == 0 {
		return nil, fmt.Errorf("catalog workflows: %s", catalog.BuildHint)
	}
	var workflows map[string]catalogWorkflow
	if err := json.Unmarshal(raw, &workflows); err != nil {
		return nil, fmt.Errorf("parse catalog workflows: %w", err)
	}
	return workflows, nil
})

// catalogWorkflowSteps turns a catalog workflow into steps that hand to this squad's agents. A role
// has an agent in `agents`; a step with an optional role and no agent is left out.
func catalogWorkflowSteps(id string, agents map[string]pgtype.UUID) ([]issueworkflow.Step, error) {
	workflows, err := loadCatalogWorkflows()
	if err != nil {
		return nil, err
	}
	wf, ok := workflows[id]
	if !ok {
		return nil, fmt.Errorf("catalog has no workflow %q", id)
	}
	steps := make([]issueworkflow.Step, 0, len(wf.Steps))
	for _, st := range wf.Steps {
		handler := issueworkflow.Handler{Type: issueworkflow.HandlerNone}
		if st.Role != nil {
			agent, has := agents[*st.Role]
			if !has && st.OptionalRole {
				continue
			}
			handler = issueworkflow.Handler{Type: issueworkflow.HandlerAgent, ID: util.UUIDToString(agent)}
		}
		next := st.Next
		if st.ImplicitNext {
			next = ""
		}
		steps = append(steps, issueworkflow.Step{
			StatusKey:    st.StatusKey,
			Handler:      handler,
			Instructions: st.Instructions,
			Transitions:  issueworkflow.LegacyTransitions(next, st.Back),
		})
	}
	return steps, nil
}
