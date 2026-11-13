package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Project workflow API (MUL-7420).
//
// Reading and creating workflows is open to any member: every client needs
// them to render a project's board, and a new workflow affects no project
// until someone switches one to it. Editing and deleting a workflow, handoffs
// included, is open to its owner (the member who created it) and to workspace
// owners/admins (MUL-8120). The Default workflow has no owner, so only
// owners/admins change it. Which workflow a project uses is open to
// owners/admins and to the project's lead.

func isWorkspaceAdmin(member db.Member) bool {
	return roleAllowed(member.Role, "owner", "admin")
}

// canEditIssueWorkflow reports whether member may edit or delete workflow:
// workspace owners and admins, and the member who owns it.
func canEditIssueWorkflow(member db.Member, workflow db.IssueWorkflow) bool {
	if isWorkspaceAdmin(member) {
		return true
	}
	return workflow.OwnerID.Valid && workflow.OwnerID == member.UserID
}

// canManageProjectWorkflow reports whether member may change a project's own
// workflow settings: owners and admins, and the member who leads the project.
func canManageProjectWorkflow(member db.Member, project db.Project) bool {
	if isWorkspaceAdmin(member) {
		return true
	}
	return project.LeadType.Valid && project.LeadType.String == "member" && project.LeadID == member.UserID
}

type IssueWorkflowHandlerPayload struct {
	Type string `json:"type"`
	ID   string `json:"id,omitempty"`
}

// IssueWorkflowStepPayload is one status of a workflow, in board order, with
// what happens when an issue enters it and where it routes when it leaves.
// Transitions are canonical. next_status_key and back_status_key are derived
// from them on responses, and still accepted on requests from clients that
// predate transitions.
type IssueWorkflowStepPayload struct {
	StatusKey     string                      `json:"status_key"`
	Handler       IssueWorkflowHandlerPayload `json:"handler"`
	Instructions  string                      `json:"instructions"`
	Transitions   []issueworkflow.Transition  `json:"transitions"`
	NextStatusKey string                      `json:"next_status_key,omitempty"`
	BackStatusKey string                      `json:"back_status_key,omitempty"`
	// Layout is the step's designer position; omitted when none is stored.
	Layout *issueworkflow.Layout `json:"layout,omitempty"`
	// Tasks and HumanOnly belong to a workflow that runs on a flow. They are
	// always present on a response; no write reads them from a handoff payload.
	Tasks     []issueworkflow.Task `json:"tasks"`
	HumanOnly bool                 `json:"human_only"`
}

type IssueWorkflowResponse struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	Name        string `json:"name"`
	Description string `json:"description"`
	IsDefault   bool   `json:"is_default"`
	// OwnerID is the member who owns the workflow; null for Default.
	OwnerID *string `json:"owner_id"`
	// SquadID is the squad whose library the workflow belongs to; null for a
	// workspace-level workflow. (MUL-7420)
	SquadID    *string                    `json:"squad_id"`
	Steps      []IssueWorkflowStepPayload `json:"steps"`
	ProjectIDs []string                   `json:"project_ids"`
	CreatedAt  string                     `json:"created_at"`
	UpdatedAt  string                     `json:"updated_at"`
	// FlowConfig is set on a workflow that runs requests on a flow, and
	// TemplateRef on one made from a catalog flow (read-only). FlowAuto lets the
	// background runner advance its requests. OpenFlowRequests counts the
	// requests pinned to it that are not closed.
	FlowConfig       *issueworkflow.FlowConfig `json:"flow_config"`
	TemplateRef      *string                   `json:"template_ref"`
	FlowAuto         bool                      `json:"flow_auto"`
	OpenFlowRequests int                       `json:"open_flow_requests"`
	// LineageID, Version, PublishedAt and PublishedBy are set on a version a person published
	// (spec 8.2): the row is locked against edits from then on. A schema 2 row also carries its
	// document, which flow_config (the schema 1 shape) cannot.
	LineageID   *string         `json:"lineage_id"`
	Version     *int32          `json:"version"`
	PublishedAt *string         `json:"published_at"`
	PublishedBy *string         `json:"published_by"`
	FlowDoc     json.RawMessage `json:"flow_doc,omitempty"`
}

// StatusMappingRequirement names a status some issues are on that the target
// workflow does not list. SuggestedStatusKey is the server's default choice.
type StatusMappingRequirement struct {
	StatusKey          string `json:"status_key"`
	IssueCount         int64  `json:"issue_count"`
	SuggestedStatusKey string `json:"suggested_status_key"`
}

// StatusIssueCount is a status some issues are on that the target workflow
// also lists, so they keep it.
type StatusIssueCount struct {
	StatusKey  string `json:"status_key"`
	IssueCount int64  `json:"issue_count"`
}

type workflowMappingPlan struct {
	Required       []StatusMappingRequirement `json:"required"`
	Unchanged      []StatusIssueCount         `json:"unchanged"`
	TotalIssues    int64                      `json:"total_issues"`
	AffectedIssues int64                      `json:"affected_issues"`
}

func stepToPayload(s issueworkflow.Step) IssueWorkflowStepPayload {
	transitions := s.Transitions
	if transitions == nil {
		transitions = []issueworkflow.Transition{}
	}
	tasks := s.Tasks
	if tasks == nil {
		tasks = []issueworkflow.Task{}
	}
	return IssueWorkflowStepPayload{
		StatusKey:     s.StatusKey,
		Handler:       IssueWorkflowHandlerPayload{Type: s.Handler.Type, ID: s.Handler.ID},
		Instructions:  s.Instructions,
		Transitions:   transitions,
		NextStatusKey: s.NextStatusKey(),
		BackStatusKey: s.BackStatusKey(),
		Layout:        s.Layout,
		Tasks:         tasks,
		HumanOnly:     s.HumanOnly,
	}
}

// payloadToStep reads a step a client wrote. prev is the stored step of the
// same status, or nil for a new one; see stepRouting. The layout is not a
// client input here: a handoff edit keeps the position the designer stored.
func payloadToStep(statusKey string, in IssueWorkflowStepPayload, prev *issueworkflow.Step) issueworkflow.Step {
	step := issueworkflow.Step{
		StatusKey:    statusKey,
		Handler:      issueworkflow.Handler{Type: in.Handler.Type, ID: in.Handler.ID},
		Instructions: in.Instructions,
		Transitions:  stepRouting(in.Transitions, in.NextStatusKey, in.BackStatusKey, prev),
	}
	if prev != nil {
		step.Layout = prev.Layout
		// A handoff edit never changes the flow fields of the step.
		step.Tasks = prev.Tasks
		step.HumanOnly = prev.HumanOnly
	}
	return step
}

// stepRouting resolves the transitions of a step a client wrote. A client that
// sends transitions sets them, an empty list included. A client that sends
// only the legacy next and back keys gets them converted, except when those
// keys match what the stored step already routes to: then the stored
// transitions stay, so an edit that leaves routing alone keeps every branch
// (for example a conditional branch the legacy keys cannot express).
func stepRouting(transitions []issueworkflow.Transition, next, back string, prev *issueworkflow.Step) []issueworkflow.Transition {
	if transitions != nil {
		return transitions
	}
	if prev != nil && prev.NextStatusKey() == next && prev.BackStatusKey() == back {
		return prev.Transitions
	}
	return issueworkflow.LegacyTransitions(next, back)
}

func issueWorkflowToResponse(row db.IssueWorkflow, def issueworkflow.Definition, projectIDs []string) IssueWorkflowResponse {
	if projectIDs == nil {
		projectIDs = []string{}
	}
	listed := def.Steps
	var doc json.RawMessage
	if def.HasFlow() && def.Graph.Compat != issueworkflow.CompatV1 {
		// A workflow of schema 2 lists its statuses; the old shape of steps carries the tasks of
		// the steps on each, so an older client draws something (spec 7.5).
		listed = issueworkflow.ProjectLegacySteps(def)
		doc = row.FlowConfig
	}
	steps := make([]IssueWorkflowStepPayload, len(listed))
	for i, s := range listed {
		steps[i] = stepToPayload(s)
	}
	var version *int32
	if row.Version.Valid {
		version = &row.Version.Int32
	}
	return IssueWorkflowResponse{
		ID:          uuidToString(row.ID),
		WorkspaceID: uuidToString(row.WorkspaceID),
		Name:        row.Name,
		Description: row.Description,
		IsDefault:   row.IsDefault,
		OwnerID:     uuidToPtr(row.OwnerID),
		SquadID:     uuidToPtr(row.SquadID),
		Steps:       steps,
		ProjectIDs:  projectIDs,
		CreatedAt:   timestampToString(row.CreatedAt),
		UpdatedAt:   timestampToString(row.UpdatedAt),
		FlowConfig:  def.Flow,
		TemplateRef: textToPtr(row.TemplateRef),
		FlowAuto:    row.FlowAuto,
		LineageID:   uuidToPtr(row.LineageID),
		Version:     version,
		PublishedAt: timestampToPtr(row.PublishedAt),
		PublishedBy: uuidToPtr(row.PublishedBy),
		FlowDoc:     doc,
	}
}

// openFlowRequestCounts counts the open flow requests pinned to each workflow
// of the workspace, keyed by workflow id.
func (h *Handler) openFlowRequestCounts(ctx context.Context, wsUUID pgtype.UUID) map[string]int {
	rows, err := h.Queries.CountOpenFlowRequestsByWorkflow(ctx, wsUUID)
	if err != nil {
		slog.Warn("count open flow requests failed", "workspace_id", uuidToString(wsUUID), "error", err)
		return map[string]int{}
	}
	out := make(map[string]int, len(rows))
	for _, row := range rows {
		out[row.WorkflowID] = int(row.OpenCount)
	}
	return out
}

// projectIDsByWorkflow groups the workspace's projects by the workflow row
// they use, so a list response can say where each workflow applies. Projects
// without a workflow, or pointing at a deleted one, count for Default.
func (h *Handler) projectIDsByWorkflow(ctx context.Context, wsUUID pgtype.UUID, rows []db.IssueWorkflow) (map[string][]string, error) {
	projects, err := h.Queries.ListProjects(ctx, db.ListProjectsParams{WorkspaceID: wsUUID})
	if err != nil {
		return nil, err
	}
	known := map[string]bool{}
	defaultID := ""
	for _, row := range rows {
		id := uuidToString(row.ID)
		known[id] = true
		if row.IsDefault {
			defaultID = id
		}
	}
	out := map[string][]string{}
	for _, p := range projects {
		key := uuidToString(p.WorkflowID)
		if !p.WorkflowID.Valid || !known[key] {
			key = defaultID
		}
		if key != "" {
			out[key] = append(out[key], uuidToString(p.ID))
		}
	}
	return out, nil
}

// workflowStatusCatalog loads the workspace catalog keyed by status key,
// including archived rows so validation can say why a key is unusable.
func (h *Handler) workflowStatusCatalog(ctx context.Context, q *db.Queries, wsUUID pgtype.UUID) (map[string]issueworkflow.CatalogEntry, error) {
	if err := issuestatus.Ensure(ctx, q, wsUUID); err != nil {
		return nil, err
	}
	rows, err := q.ListIssueStatusEntries(ctx, db.ListIssueStatusEntriesParams{WorkspaceID: wsUUID, IncludeArchived: true})
	if err != nil {
		return nil, err
	}
	out := make(map[string]issueworkflow.CatalogEntry, len(rows))
	for _, row := range rows {
		// The lifecycle category, not the legacy wire value: a switch maps
		// in_progress onto in_review because both are "started".
		category, _ := issuestatus.ParseCategory(row.Category)
		out[row.Key] = issueworkflow.CatalogEntry{
			Key:      row.Key,
			Name:     row.Name,
			Category: category,
			Archived: row.ArchivedAt.Valid,
		}
	}
	return out, nil
}

func (h *Handler) ListIssueWorkflows(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	// Store Default on the first read so every client sees the same workflow.
	if _, err := issueworkflow.EnsureDefault(r.Context(), h.Queries, wsUUID); err != nil {
		slog.Warn("ListIssueWorkflows ensure default failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	rows, err := h.Queries.ListIssueWorkflows(r.Context(), wsUUID)
	if err != nil {
		slog.Warn("ListIssueWorkflows failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	byWorkflow, err := h.projectIDsByWorkflow(r.Context(), wsUUID, rows)
	if err != nil {
		slog.Warn("ListIssueWorkflows projects failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to list workflows")
		return
	}
	openRequests := h.openFlowRequestCounts(r.Context(), wsUUID)
	resp := make([]IssueWorkflowResponse, 0, len(rows))
	for _, row := range rows {
		def, err := issueworkflow.Decode(row)
		if err != nil {
			slog.Warn("ListIssueWorkflows decode failed", append(logger.RequestAttrs(r), "workflow_id", uuidToString(row.ID), "error", err)...)
			continue
		}
		item := issueWorkflowToResponse(row, def, byWorkflow[uuidToString(row.ID)])
		item.OpenFlowRequests = openRequests[item.ID]
		resp = append(resp, item)
	}
	writeJSON(w, http.StatusOK, map[string]any{"workflows": resp, "total": len(resp)})
}

// IssueWorkflowTemplateNewStatus is a status a template's save creates. Its key
// is the one the server derives from the name, which the template's transitions
// already name.
type IssueWorkflowTemplateNewStatus struct {
	Name     string `json:"name"`
	Category string `json:"category"`
}

// IssueWorkflowTemplateStepPayload is one column of a template. A column is
// either an existing status by key or a status the save creates. Handlers are
// always none, and role_hint is a suggestion the client may show, never stored.
type IssueWorkflowTemplateStepPayload struct {
	StatusKey    string                          `json:"status_key,omitempty"`
	NewStatus    *IssueWorkflowTemplateNewStatus `json:"new_status,omitempty"`
	Handler      IssueWorkflowHandlerPayload     `json:"handler"`
	RoleHint     string                          `json:"role_hint,omitempty"`
	Instructions string                          `json:"instructions"`
	Transitions  []issueworkflow.Transition      `json:"transitions"`
}

// IssueWorkflowTemplateResponse is a system template for a new draft. The
// client translates its name and description by id.
type IssueWorkflowTemplateResponse struct {
	ID    string                             `json:"id"`
	Steps []IssueWorkflowTemplateStepPayload `json:"steps"`
}

type ListIssueWorkflowTemplatesResponse struct {
	Templates []IssueWorkflowTemplateResponse `json:"templates"`
}

// ListIssueWorkflowTemplates lists the system templates a new workflow can
// start from. Any member may read them, as with the workflow list.
func (h *Handler) ListIssueWorkflowTemplates(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	if _, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id"); !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	templates := issueworkflow.Templates()
	resp := make([]IssueWorkflowTemplateResponse, 0, len(templates))
	for _, t := range templates {
		steps := make([]IssueWorkflowTemplateStepPayload, len(t.Steps))
		for i, s := range t.Steps {
			steps[i] = templateStepToPayload(s)
		}
		resp = append(resp, IssueWorkflowTemplateResponse{ID: t.ID, Steps: steps})
	}
	writeJSON(w, http.StatusOK, ListIssueWorkflowTemplatesResponse{Templates: resp})
}

func templateStepToPayload(s issueworkflow.TemplateStep) IssueWorkflowTemplateStepPayload {
	transitions := s.Transitions
	if transitions == nil {
		transitions = []issueworkflow.Transition{}
	}
	out := IssueWorkflowTemplateStepPayload{
		StatusKey:    s.StatusKey,
		Handler:      IssueWorkflowHandlerPayload{Type: issueworkflow.HandlerNone},
		RoleHint:     s.RoleHint,
		Instructions: s.Instructions,
		Transitions:  transitions,
	}
	if s.New != nil {
		out.NewStatus = &IssueWorkflowTemplateNewStatus{Name: s.New.Name, Category: s.New.Category}
	}
	return out
}

func (h *Handler) GetIssueWorkflow(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	row, err := h.Queries.GetIssueWorkflow(r.Context(), db.GetIssueWorkflowParams{ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	h.writeIssueWorkflow(w, r, http.StatusOK, row)
}

// storedStepsByKey indexes a stored workflow's steps by status key, so an
// edit can tell which routing it leaves alone.
func storedStepsByKey(row db.IssueWorkflow) map[string]issueworkflow.Step {
	out := map[string]issueworkflow.Step{}
	def, err := issueworkflow.Decode(row)
	if err != nil {
		return out
	}
	for _, s := range def.Steps {
		out[s.StatusKey] = s
	}
	return out
}

// IssueWorkflowUsage says where one workflow applies: the projects that use
// it, the squads whose active workflow it is, and the agents its steps and
// branches hand work to. Default's projects are those that name no workflow.
type IssueWorkflowUsage struct {
	WorkflowID string   `json:"workflow_id"`
	ProjectIDs []string `json:"project_ids"`
	SquadIDs   []string `json:"squad_ids"`
	AgentIDs   []string `json:"agent_ids"`
}

// ListIssueWorkflowUsage reports where each workflow is assigned, for the
// workflow manage view. Archived squads do not count.
func (h *Handler) ListIssueWorkflowUsage(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	ctx := r.Context()
	rows, err := h.Queries.ListIssueWorkflows(ctx, wsUUID)
	if err != nil {
		slog.Warn("ListIssueWorkflowUsage failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load workflow usage")
		return
	}
	byProject, err := h.projectIDsByWorkflow(ctx, wsUUID, rows)
	if err != nil {
		slog.Warn("ListIssueWorkflowUsage projects failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load workflow usage")
		return
	}
	squads, err := h.Queries.ListSquads(ctx, wsUUID)
	if err != nil {
		slog.Warn("ListIssueWorkflowUsage squads failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load workflow usage")
		return
	}
	bySquad := map[string][]string{}
	for _, squad := range squads {
		if squad.ActiveWorkflowID.Valid {
			key := uuidToString(squad.ActiveWorkflowID)
			bySquad[key] = append(bySquad[key], uuidToString(squad.ID))
		}
	}
	usage := make([]IssueWorkflowUsage, 0, len(rows))
	for _, row := range rows {
		def, err := issueworkflow.Decode(row)
		if err != nil {
			continue
		}
		id := uuidToString(row.ID)
		usage = append(usage, IssueWorkflowUsage{
			WorkflowID: id,
			ProjectIDs: orEmptyIDs(byProject[id]),
			SquadIDs:   orEmptyIDs(bySquad[id]),
			AgentIDs:   handlerAgentIDs(def),
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"usage": usage, "total": len(usage)})
}

func orEmptyIDs(ids []string) []string {
	if ids == nil {
		return []string{}
	}
	return ids
}

// handlerAgentIDs lists the agents a workflow's steps and branch overrides
// hand work to, each once.
func handlerAgentIDs(def issueworkflow.Definition) []string {
	seen := map[string]bool{}
	out := []string{}
	add := func(h issueworkflow.Handler) {
		if h.Type == issueworkflow.HandlerAgent && h.ID != "" && !seen[h.ID] {
			seen[h.ID] = true
			out = append(out, h.ID)
		}
	}
	for _, s := range def.Steps {
		add(s.Handler)
		for _, t := range s.Transitions {
			if t.Handler != nil {
				add(*t.Handler)
			}
		}
	}
	return out
}

func (h *Handler) writeIssueWorkflow(w http.ResponseWriter, r *http.Request, status int, row db.IssueWorkflow) {
	writeJSON(w, status, h.issueWorkflowResponse(r.Context(), row))
}

func (h *Handler) issueWorkflowResponse(ctx context.Context, row db.IssueWorkflow) IssueWorkflowResponse {
	def, err := issueworkflow.Decode(row)
	if err != nil {
		def = issueworkflow.Definition{}
	}
	rows, err := h.Queries.ListIssueWorkflows(ctx, row.WorkspaceID)
	if err != nil {
		rows = []db.IssueWorkflow{row}
	}
	byWorkflow, err := h.projectIDsByWorkflow(ctx, row.WorkspaceID, rows)
	if err != nil {
		byWorkflow = map[string][]string{}
	}
	resp := issueWorkflowToResponse(row, def, byWorkflow[uuidToString(row.ID)])
	resp.OpenFlowRequests = h.openFlowRequestCounts(ctx, row.WorkspaceID)[resp.ID]
	return resp
}

// NewWorkflowStatus creates a catalog status as part of a workflow write, so
// a column can be added without the status settings page.
type NewWorkflowStatus struct {
	Name     string `json:"name"`
	Category string `json:"category"`
	Color    string `json:"color"`
}

// IssueWorkflowStepInput names a step by an existing status key or by a
// status to create, with its handoff. Transitions route the issue when it
// leaves the step; next and back keys are the legacy form of two of them.
type IssueWorkflowStepInput struct {
	StatusKey     string                       `json:"status_key,omitempty"`
	NewStatus     *NewWorkflowStatus           `json:"new_status,omitempty"`
	Handler       *IssueWorkflowHandlerPayload `json:"handler,omitempty"`
	Instructions  string                       `json:"instructions,omitempty"`
	Transitions   *[]issueworkflow.Transition  `json:"transitions,omitempty"`
	NextStatusKey string                       `json:"next_status_key,omitempty"`
	BackStatusKey string                       `json:"back_status_key,omitempty"`
	// Layout is the step's designer position. Omitted: the designer places it.
	Layout *issueworkflow.Layout `json:"layout,omitempty"`
	// Tasks and HumanOnly belong to a flow. Omitted, the step keeps what is
	// stored, so a client that predates them does not erase them; an empty list
	// or false sets them.
	Tasks     *[]issueworkflow.Task `json:"tasks,omitempty"`
	HumanOnly *bool                 `json:"human_only,omitempty"`
}

type IssueWorkflowWriteRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	// FlowConfig and TemplateRef make a new workflow a flow made from a
	// catalog flow. Only a workspace admin may create one, and an update never
	// changes them.
	FlowConfig  *issueworkflow.FlowConfig `json:"flow_config,omitempty"`
	TemplateRef *string                   `json:"template_ref,omitempty"`
	// SquadID, on a create, names the squad the new flow is written for: the flow
	// is then checked against who is in the squad and the row is kept as the
	// squad's. Without it the roster is not checked.
	SquadID *string `json:"squad_id,omitempty"`
	// Steps replace the workflow's steps and handoffs. Todo, done and
	// cancelled are added when missing; backlog and blocked are listed only
	// when included. Fixed statuses take their fixed places.
	Steps *[]IssueWorkflowStepInput `json:"steps"`
	// StatusMapping moves issues off statuses an edit removes, keyed by the
	// removed status. Required for every removed status that issues of the
	// workflow's projects still use.
	StatusMapping map[string]string `json:"status_mapping,omitempty"`
	// DryRun validates the edit and returns the mapping plan without writing.
	DryRun bool `json:"dry_run,omitempty"`
}

var newStatusDefaultColor = map[string]string{
	"unstarted": "#94a3b8",
	"started":   "#3b82f6",
	"done":      "#22c55e",
	"closed":    "#64748b",
}

// errStatusCreateForbidden reports a workflow write that would add a status
// to the workspace catalog for a caller who may not: statuses are shared by
// every board, so only owners/admins create them (MUL-8120). Reusing an
// existing status, by key or by name, stays open.
var errStatusCreateForbidden = errors.New("only workspace admins can create statuses")

// resolveStepInputs turns the request's steps into workflow steps, creating
// the new statuses they name. previous holds the stored steps by status key,
// for routing that an edit leaves alone (see stepRouting). allowCreate is
// false for callers who may not add statuses; a step naming one that does not
// exist yet then fails with errStatusCreateForbidden.
func resolveStepInputs(ctx context.Context, qtx *db.Queries, wsUUID pgtype.UUID, inputs []IssueWorkflowStepInput, previous map[string]issueworkflow.Step, allowCreate bool) ([]issueworkflow.Step, []db.IssueStatus, string, error) {
	steps := make([]issueworkflow.Step, 0, len(inputs))
	var created []db.IssueStatus
	for _, in := range inputs {
		step := issueworkflow.Step{
			StatusKey:    in.StatusKey,
			Instructions: in.Instructions,
			Layout:       in.Layout,
		}
		if in.Handler != nil {
			step.Handler = issueworkflow.Handler{Type: in.Handler.Type, ID: in.Handler.ID}
		}
		if in.NewStatus != nil {
			entry, isNew, msg, err := createWorkflowStatus(ctx, qtx, wsUUID, *in.NewStatus, allowCreate)
			if msg != "" || err != nil {
				return nil, nil, msg, err
			}
			if isNew {
				created = append(created, entry)
			}
			step.StatusKey = entry.Key
		}
		var sent []issueworkflow.Transition
		if in.Transitions != nil {
			sent = *in.Transitions
		}
		var prev *issueworkflow.Step
		if s, ok := previous[step.StatusKey]; ok {
			prev = &s
		}
		step.Transitions = stepRouting(sent, in.NextStatusKey, in.BackStatusKey, prev)
		switch {
		case in.Tasks != nil:
			step.Tasks = *in.Tasks
		case prev != nil:
			step.Tasks = prev.Tasks
		}
		switch {
		case in.HumanOnly != nil:
			step.HumanOnly = *in.HumanOnly
		case prev != nil:
			step.HumanOnly = prev.HumanOnly
		}
		steps = append(steps, step)
	}
	return steps, created, "", nil
}

func createWorkflowStatus(ctx context.Context, qtx *db.Queries, wsUUID pgtype.UUID, in NewWorkflowStatus, allowCreate bool) (db.IssueStatus, bool, string, error) {
	name := strings.TrimSpace(in.Name)
	if name == "" || utf8.RuneCountInString(name) > 64 {
		return db.IssueStatus{}, false, "status name must be 1-64 characters", nil
	}
	category, ok := issuestatus.ParseCategory(in.Category)
	if !ok {
		category = "started"
	}
	color := newStatusDefaultColor[category]
	if strings.TrimSpace(in.Color) != "" {
		c, err := normalizeColor(in.Color)
		if err != nil {
			return db.IssueStatus{}, false, err.Error(), nil
		}
		color = c
	}
	// A status with this name already exists: reuse it instead of failing,
	// so the same template applied twice lands on the same statuses.
	entries, err := qtx.ListIssueStatusEntries(ctx, db.ListIssueStatusEntriesParams{WorkspaceID: wsUUID, IncludeArchived: false})
	if err != nil {
		return db.IssueStatus{}, false, "", err
	}
	for _, e := range entries {
		if strings.EqualFold(e.Name, name) {
			return e, false, "", nil
		}
	}
	if !allowCreate {
		return db.IssueStatus{}, false, "", errStatusCreateForbidden
	}
	entry, msg, err := createIssueStatusEntryLocked(ctx, qtx, wsUUID, category, db.CreateIssueStatusEntryParams{
		WorkspaceID: wsUUID,
		Name:        name,
		Category:    category,
		Color:       color,
	})
	if err != nil && isUniqueViolation(err) {
		return db.IssueStatus{}, false, fmt.Sprintf("a status named %q already exists", name), nil
	}
	return entry, err == nil && msg == "", msg, err
}

// validateHandoffHandlers checks that each concrete handler exists in the
// workspace and that the caller may hand work to it — the same gate an
// assignment passes. Only changed handlers are checked, so an edit never
// fails on a handler someone else picked.
func (h *Handler) validateHandoffHandlers(ctx context.Context, r *http.Request, workspaceID string, steps, previous []issueworkflow.Step) (int, string) {
	before := map[issueworkflow.Handler]bool{}
	for _, s := range previous {
		before[s.Handler] = true
		for _, task := range s.Tasks {
			before[task.Handler] = true
		}
	}
	for _, s := range steps {
		if !before[s.Handler] {
			if status, msg := h.validateHandoffHandler(ctx, r, workspaceID, s); status != 0 {
				return status, msg
			}
		}
		for _, task := range s.Tasks {
			if before[task.Handler] {
				continue
			}
			if status, msg := h.validateHandoffHandler(ctx, r, workspaceID, issueworkflow.Step{Handler: task.Handler}); status != 0 {
				return status, msg
			}
		}
	}
	return 0, ""
}

// validateHandoffHandler checks one step's handler like
// validateHandoffHandlers.
func (h *Handler) validateHandoffHandler(ctx context.Context, r *http.Request, workspaceID string, s issueworkflow.Step) (int, string) {
	switch s.Handler.Type {
	case issueworkflow.HandlerAgent, issueworkflow.HandlerSquad, issueworkflow.HandlerMember:
		id, err := parseUUIDString(s.Handler.ID)
		if err != nil {
			return http.StatusBadRequest, "invalid handler id"
		}
		return h.validateAssigneePair(ctx, r, workspaceID, pgtype.Text{String: s.Handler.Type, Valid: true}, id)
	}
	return 0, ""
}

func parseUUIDString(s string) (pgtype.UUID, error) {
	var id pgtype.UUID
	if err := id.Scan(s); err != nil || !id.Valid {
		return pgtype.UUID{}, errors.New("invalid uuid")
	}
	return id, nil
}

// stepInputsCarryFlowFields reports whether a write sets tasks or human-only
// on any step.
func stepInputsCarryFlowFields(inputs []IssueWorkflowStepInput) bool {
	for _, in := range inputs {
		if in.Tasks != nil && len(*in.Tasks) > 0 {
			return true
		}
		if in.HumanOnly != nil && *in.HumanOnly {
			return true
		}
	}
	return false
}

// encodeFlowConfig serializes a flow_config for storage; nil stays NULL.
func encodeFlowConfig(cfg *issueworkflow.FlowConfig) ([]byte, error) {
	if cfg == nil {
		return nil, nil
	}
	return json.Marshal(cfg)
}

// writeManagedByTemplate answers a write to a workflow a catalog flow made.
func writeManagedByTemplate(w http.ResponseWriter) {
	writeErrorCode(w, http.StatusConflict, "workflow_managed_by_template", "this workflow is managed by its template and cannot be edited")
}

// workflowLocked reports whether a workflow refuses edits: a catalog row (its
// template owns it) or a version a person published (spec 8.1, I2).
func workflowLocked(row db.IssueWorkflow) bool { return row.TemplateRef.Valid || row.PublishedAt.Valid }

// writeWorkflowLocked answers a write to a locked workflow with the reason: the
// template of a catalog row, or the publication of a version.
func writeWorkflowLocked(w http.ResponseWriter, row db.IssueWorkflow) {
	if row.TemplateRef.Valid {
		writeManagedByTemplate(w)
		return
	}
	writeErrorCode(w, http.StatusConflict, "workflow_locked", "this version is published and cannot be edited: make a draft from it")
}

func (h *Handler) CreateIssueWorkflow(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	var req IssueWorkflowWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var name, description string
	if req.Name != nil {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}
	var inputs []IssueWorkflowStepInput
	if req.Steps != nil {
		inputs = *req.Steps
	}
	templateRef := ""
	if req.TemplateRef != nil {
		templateRef = strings.TrimSpace(*req.TemplateRef)
	}
	if (req.FlowConfig != nil || templateRef != "" || stepInputsCarryFlowFields(inputs)) && !isWorkspaceAdmin(member) {
		writeErrorCode(w, http.StatusForbidden, "flow_workflow_forbidden", "only workspace admins can create a workflow that runs on a flow")
		return
	}
	if templateRef != "" && req.FlowConfig == nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_workflow", "template_ref needs a flow_config")
		return
	}
	if req.FlowConfig != nil && req.FlowConfig.Version >= issueworkflow.FlowDocVersion {
		writeErrorCode(w, http.StatusBadRequest, "flow_schema_unsupported", "a flow of schema 2 is written as a draft and published")
		return
	}
	var squad *db.Squad
	if req.SquadID != nil && *req.SquadID != "" {
		squadID, ok := parseUUIDOrBadRequest(w, *req.SquadID, "squad id")
		if !ok {
			return
		}
		found, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: wsUUID})
		if err != nil {
			writeError(w, http.StatusNotFound, "squad not found")
			return
		}
		squad = &found
	}
	flowConfig, err := encodeFlowConfig(req.FlowConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}

	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := qtx.LockIssueStatusCatalog(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	// Store Default before creating statuses for this workflow, so they
	// never join Default's snapshot.
	if _, err := issueworkflow.EnsureDefault(ctx, qtx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	steps, createdStatuses, msg, err := resolveStepInputs(ctx, qtx, wsUUID, inputs, nil, isWorkspaceAdmin(member))
	if msg != "" {
		writeErrorCode(w, http.StatusBadRequest, "invalid_workflow", msg)
		return
	}
	if errors.Is(err, errStatusCreateForbidden) {
		writeErrorCode(w, http.StatusForbidden, "status_create_forbidden", err.Error())
		return
	}
	if err != nil {
		slog.Warn("CreateIssueWorkflow statuses failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	name, description, steps = issueworkflow.Normalize(name, description, steps)
	catalog, err := h.workflowStatusCatalog(ctx, qtx, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load issue statuses")
		return
	}
	if err := issueworkflow.ValidateWithFlow(name, description, steps, false, catalog, req.FlowConfig); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	if status, msg := h.validateHandoffHandlers(ctx, r, workspaceID, steps, nil); status != 0 {
		writeErrorCode(w, status, "invalid_handoff", msg)
		return
	}
	var squadID pgtype.UUID
	if squad != nil && req.FlowConfig != nil {
		roster, err := h.squadRoster(ctx, qtx, *squad, false)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save workflow")
			return
		}
		issues := issueworkflow.ValidateSchema1ForSquad(issueworkflow.Definition{Name: name, Steps: steps, Flow: req.FlowConfig}, roster)
		if issueworkflow.HasErrors(issues) {
			writeJSON(w, http.StatusUnprocessableEntity, map[string]any{
				"error": "the flow does not fit the squad", "code": "flow_workflow_invalid", "issues": issues,
			})
			return
		}
	}
	if squad != nil {
		squadID = squad.ID
	}
	raw, err := issueworkflow.EncodeSteps(steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	row, err := qtx.CreateIssueWorkflow(ctx, db.CreateIssueWorkflowParams{
		WorkspaceID:      wsUUID,
		Name:             name,
		Description:      description,
		InitialStatusKey: issueworkflow.StartStatus,
		Steps:            raw,
		OwnerID:          member.UserID,
		SquadID:          squadID,
		FlowConfig:       flowConfig,
		TemplateRef:      pgtype.Text{String: templateRef, Valid: templateRef != ""},
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeErrorCode(w, http.StatusConflict, "workflow_name_taken", "a workflow with this name already exists")
			return
		}
		slog.Warn("CreateIssueWorkflow failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	if len(createdStatuses) > 0 {
		h.publishIssueStatusChanged(workspaceID, member, "created")
	}
	h.publishIssueWorkflowChanged(workspaceID, member, "created")
	h.writeIssueWorkflow(w, r, http.StatusCreated, row)
}

// mappingPlan computes which statuses issues of projectIDs are on that
// allowed() rejects, with a suggested target for each.
func mappingPlan(counts []db.CountProjectIssuesByStatusRow, allowed func(string) bool, suggest func(string) string) workflowMappingPlan {
	plan := workflowMappingPlan{Required: []StatusMappingRequirement{}, Unchanged: []StatusIssueCount{}}
	for _, c := range counts {
		plan.TotalIssues += c.IssueCount
		if allowed(c.Status) {
			plan.Unchanged = append(plan.Unchanged, StatusIssueCount{StatusKey: c.Status, IssueCount: c.IssueCount})
			continue
		}
		plan.AffectedIssues += c.IssueCount
		plan.Required = append(plan.Required, StatusMappingRequirement{
			StatusKey:          c.Status,
			IssueCount:         c.IssueCount,
			SuggestedStatusKey: suggest(c.Status),
		})
	}
	sort.Slice(plan.Required, func(i, j int) bool { return plan.Required[i].StatusKey < plan.Required[j].StatusKey })
	return plan
}

// checkMapping verifies a mapping covers every required status with a target
// the destination accepts. It returns the first problem as a message.
func checkMapping(plan workflowMappingPlan, mapping map[string]string, targetAllowed func(string) bool) string {
	for _, req := range plan.Required {
		target, ok := mapping[req.StatusKey]
		if !ok || target == "" {
			return fmt.Sprintf("choose a destination status for the %d issue(s) on %q", req.IssueCount, req.StatusKey)
		}
		if !targetAllowed(target) {
			return fmt.Sprintf("destination status %q for %q is not part of the target workflow", target, req.StatusKey)
		}
	}
	return ""
}

func writeMappingRequired(w http.ResponseWriter, plan workflowMappingPlan, msg string) {
	writeJSON(w, http.StatusConflict, map[string]any{
		"error": msg,
		"code":  "workflow_status_mapping_required",
		"plan":  plan,
	})
}

// remapIssues applies a validated mapping inside the caller's transaction to the
// issues of projectIDs that the project's workflow governs. An issue it moves
// into a done or closed status ends its wakeups, as any status write does; the
// returned runs are for the caller's post-commit broadcast.
func remapIssues(ctx context.Context, qtx *db.Queries, wsUUID pgtype.UUID, projectIDs []pgtype.UUID, plan workflowMappingPlan, mapping map[string]string) ([]db.Issue, []db.AgentTaskQueue, error) {
	var changed []db.Issue
	var cancelled []db.AgentTaskQueue
	for _, req := range plan.Required {
		rows, err := qtx.RemapProjectIssueStatus(ctx, db.RemapProjectIssueStatusParams{
			WorkspaceID: wsUUID,
			ProjectIds:  projectIDs,
			FromStatus:  req.StatusKey,
			ToStatus:    mapping[req.StatusKey],
		})
		if err != nil {
			return nil, nil, err
		}
		stopped, err := stopWakeupsOfMoved(ctx, qtx, rows)
		if err != nil {
			return nil, nil, err
		}
		cancelled = append(cancelled, stopped...)
		changed = append(changed, rows...)
	}
	return changed, cancelled, nil
}

// stopWakeupsOfMoved ends the wakeups of issues a remap moved into a closed
// status, and returns the runs it cancelled.
func stopWakeupsOfMoved(ctx context.Context, qtx *db.Queries, moved []db.Issue) ([]db.AgentTaskQueue, error) {
	var cancelled []db.AgentTaskQueue
	for _, issue := range moved {
		stopped, err := service.StopClosedIssueWakeups(ctx, qtx, issue)
		if err != nil {
			return nil, err
		}
		cancelled = append(cancelled, stopped...)
	}
	return cancelled, nil
}

func (h *Handler) UpdateIssueWorkflow(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	var req IssueWorkflowWriteRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	// The exclusive catalog lock keeps a status archive from racing the step
	// validation below, and serializes concurrent edits of one workspace's
	// workflows.
	if err := qtx.LockIssueStatusCatalog(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	current, err := qtx.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if !canEditIssueWorkflow(member, current) {
		writeError(w, http.StatusForbidden, "only the workflow owner and workspace admins can change this workflow")
		return
	}
	if workflowLocked(current) {
		writeWorkflowLocked(w, current)
		return
	}
	currentFlow, err := issueworkflow.ParseFlowConfig(current.FlowConfig)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read workflow")
		return
	}
	var projects []db.Project
	if current.IsDefault {
		projects, err = qtx.ListProjectsUsingDefaultIssueWorkflow(ctx, wsUUID)
	} else {
		projects, err = qtx.ListProjectsUsingIssueWorkflow(ctx, db.ListProjectsUsingIssueWorkflowParams{WorkspaceID: wsUUID, WorkflowID: current.ID})
	}
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	currentSteps, err := issueworkflow.DecodeSteps(current.Steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read workflow")
		return
	}
	name, description, steps := current.Name, current.Description, currentSteps
	if req.Name != nil && !current.IsDefault {
		name = *req.Name
	}
	if req.Description != nil {
		description = *req.Description
	}
	var createdStatuses []db.IssueStatus
	if req.Steps != nil {
		var msg string
		steps, createdStatuses, msg, err = resolveStepInputs(ctx, qtx, wsUUID, *req.Steps, storedStepsByKey(current), isWorkspaceAdmin(member))
		if msg != "" {
			writeErrorCode(w, http.StatusBadRequest, "invalid_workflow", msg)
			return
		}
		if errors.Is(err, errStatusCreateForbidden) {
			writeErrorCode(w, http.StatusForbidden, "status_create_forbidden", err.Error())
			return
		}
		if err != nil {
			slog.Warn("UpdateIssueWorkflow statuses failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to save workflow")
			return
		}
	}
	name, description, steps = issueworkflow.Normalize(name, description, steps)
	catalog, err := h.workflowStatusCatalog(ctx, qtx, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load issue statuses")
		return
	}
	if err := issueworkflow.ValidateWithFlow(name, description, steps, current.IsDefault, catalog, currentFlow); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_workflow", err.Error())
		return
	}
	if status, msg := h.validateHandoffHandlers(ctx, r, workspaceID, steps, currentSteps); status != 0 {
		writeErrorCode(w, status, "invalid_handoff", msg)
		return
	}

	// Default accepts every status, so editing it never moves issues; issues
	// on a status it drops stay there and show under hidden columns.
	plan := workflowMappingPlan{Required: []StatusMappingRequirement{}, Unchanged: []StatusIssueCount{}}
	projectIDs := make([]pgtype.UUID, len(projects))
	for i, p := range projects {
		projectIDs[i] = p.ID
	}
	// A squad that uses this workflow as its active one routes its issues
	// through it too, so an edit remaps those issues along with the projects'.
	var squads []db.Squad
	if !current.IsDefault {
		squads, err = qtx.ListSquadsUsingIssueWorkflow(ctx, db.ListSquadsUsingIssueWorkflowParams{WorkspaceID: wsUUID, WorkflowID: current.ID})
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to save workflow")
			return
		}
	}
	if !current.IsDefault && currentFlow == nil && (len(projectIDs) > 0 || len(squads) > 0) {
		next := issueworkflow.Definition{ID: current.ID, Name: name, Steps: steps}
		var counts []db.CountProjectIssuesByStatusRow
		if len(projectIDs) > 0 {
			counts, err = qtx.CountProjectIssuesByStatus(ctx, db.CountProjectIssuesByStatusParams{WorkspaceID: wsUUID, ProjectIds: projectIDs})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to save workflow")
				return
			}
		}
		for _, squad := range squads {
			squadCounts, err := qtx.CountSquadIssuesByStatus(ctx, db.CountSquadIssuesByStatusParams{WorkspaceID: wsUUID, SquadID: squad.ID})
			if err != nil {
				writeError(w, http.StatusInternalServerError, "failed to save workflow")
				return
			}
			counts = mergeStatusCounts(counts, squadCountsAsProject(squadCounts))
		}
		categoryOf := func(key string) string { return catalog[key].Category }
		plan = mappingPlan(counts, next.Has, func(key string) string { return issueworkflow.MoveTarget(next, key, categoryOf) })
		if req.DryRun {
			writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "plan": plan})
			return
		}
		if msg := checkMapping(plan, req.StatusMapping, next.Has); msg != "" {
			writeMappingRequired(w, plan, msg)
			return
		}
	} else if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "plan": plan})
		return
	}
	raw, err := issueworkflow.EncodeSteps(steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	row, err := qtx.UpdateIssueWorkflow(ctx, db.UpdateIssueWorkflowParams{
		ID:               current.ID,
		WorkspaceID:      wsUUID,
		Name:             name,
		Description:      description,
		InitialStatusKey: issueworkflow.StartStatus,
		Steps:            raw,
	})
	if err != nil {
		if isUniqueViolation(err) {
			writeErrorCode(w, http.StatusConflict, "workflow_name_taken", "a workflow with this name already exists")
			return
		}
		slog.Warn("UpdateIssueWorkflow failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	changed, cancelledWakeups, err := remapIssues(ctx, qtx, wsUUID, projectIDs, plan, req.StatusMapping)
	if err != nil {
		slog.Warn("UpdateIssueWorkflow remap failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to move issues to the new statuses")
		return
	}
	for _, squad := range squads {
		squadChanged, squadCancelled, err := remapSquadIssues(ctx, qtx, wsUUID, squad.ID, plan, req.StatusMapping)
		if err != nil {
			slog.Warn("UpdateIssueWorkflow squad remap failed", append(logger.RequestAttrs(r), "error", err)...)
			writeError(w, http.StatusInternalServerError, "failed to move issues to the new statuses")
			return
		}
		changed = append(changed, squadChanged...)
		cancelledWakeups = append(cancelledWakeups, squadCancelled...)
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save workflow")
		return
	}
	// Remapping is an administrative move: snapshots only, so it neither
	// notifies subscribers per issue nor looks like a handoff.
	h.publishIssueSnapshots(ctx, changed, "member", uuidToString(member.UserID))
	h.broadcastCancelledWakeups(ctx, wsUUID, cancelledWakeups)
	if len(createdStatuses) > 0 {
		h.publishIssueStatusChanged(workspaceID, member, "created")
	}
	h.publishIssueWorkflowChanged(workspaceID, member, "updated")
	h.writeIssueWorkflow(w, r, http.StatusOK, row)
}

// openFlowRequestIDsLimit bounds the request ids a delete conflict lists.
const openFlowRequestIDsLimit = 20

// workflowBlocker is one reason a workflow cannot be deleted. The delete
// response lists every reason so the UI can explain all of them.
type workflowBlocker struct {
	Code    string   `json:"code"`
	Count   int      `json:"count"`
	IDs     []string `json:"ids"`
	Message string   `json:"message"`
}

// deleteBlockers lists why a workflow cannot be deleted yet: projects that use
// it and squads whose active workflow it is. Either remaps first, so the
// workflow is never deleted under live work.
func deleteBlockers(projects []db.Project, squads []db.Squad, flowRequests []string) []workflowBlocker {
	var out []workflowBlocker
	if len(projects) > 0 {
		ids := make([]string, len(projects))
		for i, p := range projects {
			ids[i] = uuidToString(p.ID)
		}
		out = append(out, workflowBlocker{
			Code:    "workflow_in_use",
			Count:   len(projects),
			IDs:     ids,
			Message: fmt.Sprintf("%d project(s) still use this workflow; switch them to another workflow first", len(projects)),
		})
	}
	if len(squads) > 0 {
		ids := make([]string, len(squads))
		for i, s := range squads {
			ids[i] = uuidToString(s.ID)
		}
		out = append(out, workflowBlocker{
			Code:    "workflow_is_squad_active",
			Count:   len(squads),
			IDs:     ids,
			Message: fmt.Sprintf("%d squad(s) use this workflow as their active workflow; make another workflow active first", len(squads)),
		})
	}
	if len(flowRequests) > 0 {
		out = append(out, workflowBlocker{
			Code:    "workflow_has_flow_requests",
			Count:   len(flowRequests),
			IDs:     flowRequests,
			Message: fmt.Sprintf("%d request(s) still run on this workflow; close or detach them first", len(flowRequests)),
		})
	}
	return out
}

func (h *Handler) DeleteIssueWorkflow(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := qtx.LockIssueStatusCatalog(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	row, err := qtx.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if !canEditIssueWorkflow(member, row) {
		writeError(w, http.StatusForbidden, "only the workflow owner and workspace admins can change this workflow")
		return
	}
	if row.IsDefault {
		writeErrorCode(w, http.StatusBadRequest, "workflow_is_default", "the Default workflow cannot be deleted")
		return
	}
	projects, err := qtx.ListProjectsUsingIssueWorkflow(ctx, db.ListProjectsUsingIssueWorkflowParams{WorkspaceID: wsUUID, WorkflowID: idUUID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	squads, err := qtx.ListSquadsUsingIssueWorkflow(ctx, db.ListSquadsUsingIssueWorkflowParams{WorkspaceID: wsUUID, WorkflowID: idUUID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	flowRequestIDs, err := qtx.ListOpenFlowRequestIDsByWorkflow(ctx, db.ListOpenFlowRequestIDsByWorkflowParams{
		WorkspaceID: wsUUID,
		WorkflowID:  uuidToString(idUUID),
		PageLimit:   openFlowRequestIDsLimit,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	flowRequests := make([]string, len(flowRequestIDs))
	for i, id := range flowRequestIDs {
		flowRequests[i] = uuidToString(id)
	}
	if blockers := deleteBlockers(projects, squads, flowRequests); len(blockers) > 0 {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":         blockers[0].Message,
			"code":          blockers[0].Code,
			"reasons":       blockers,
			"project_count": len(projects),
			"squad_count":   len(squads),
		})
		return
	}
	n, err := qtx.DeleteIssueWorkflow(ctx, db.DeleteIssueWorkflowParams{ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	if n == 0 {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to delete workflow")
		return
	}
	h.publishIssueWorkflowChanged(workspaceID, member, "deleted")
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) publishIssueWorkflowChanged(workspaceID string, actor db.Member, action string) {
	h.publish(protocol.EventIssueWorkflowChanged, workspaceID, "member", uuidToString(actor.UserID), map[string]any{
		"action": action,
	})
}

// publishProjectWorkflowUpdate announces a project whose workflow settings
// changed and returns the project as the API shapes it.
func (h *Handler) publishProjectWorkflowUpdate(ctx context.Context, workspaceID string, member db.Member, project db.Project) ProjectResponse {
	resp := projectToResponse(project)
	resp.IssueCount, resp.DoneCount = h.loadProjectIssueStats(ctx, project.WorkspaceID, project.ID)
	resp.ResourceCount = h.loadProjectResourceCount(ctx, project.ID)
	h.publish(protocol.EventProjectUpdated, workspaceID, "member", uuidToString(member.UserID), map[string]any{"project": resp})
	return resp
}

// loadProjectForWorkflowWrite resolves the {id} project and checks that the
// caller may change its workflow settings.
func (h *Handler) loadProjectForWorkflowWrite(w http.ResponseWriter, r *http.Request) (db.Project, db.Member, string, bool) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return db.Project{}, db.Member{}, "", false
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return db.Project{}, db.Member{}, "", false
	}
	projectUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "project id")
	if !ok {
		return db.Project{}, db.Member{}, "", false
	}
	project, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: projectUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return db.Project{}, db.Member{}, "", false
	}
	if !canManageProjectWorkflow(member, project) {
		writeError(w, http.StatusForbidden, "only workspace admins and the project lead can change this project's workflow")
		return db.Project{}, db.Member{}, "", false
	}
	return project, member, workspaceID, true
}

type SetProjectWorkflowRequest struct {
	// WorkflowID is the workflow to use; null switches back to the workspace
	// Default workflow.
	WorkflowID    *string           `json:"workflow_id"`
	StatusMapping map[string]string `json:"status_mapping,omitempty"`
	DryRun        bool              `json:"dry_run,omitempty"`
}

// SetProjectWorkflow switches the workflow a project uses. Issues on statuses
// the new workflow does not list move to the statuses the caller maps them to,
// in the same transaction. The move is administrative: it neither reassigns
// issues nor starts runs.
func (h *Handler) SetProjectWorkflow(w http.ResponseWriter, r *http.Request) {
	loaded, member, workspaceID, ok := h.loadProjectForWorkflowWrite(w, r)
	if !ok {
		return
	}
	wsUUID := loaded.WorkspaceID
	var req SetProjectWorkflowRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var targetID pgtype.UUID
	if req.WorkflowID != nil && *req.WorkflowID != "" {
		id, ok := parseUUIDOrBadRequest(w, *req.WorkflowID, "workflow_id")
		if !ok {
			return
		}
		targetID = id
	}
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	// Shared catalog lock: a workflow edit (exclusive) cannot change the
	// target's steps between the census and the remap below.
	if err := qtx.LockIssueStatusCatalogShared(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	project, err := qtx.LockProjectForWorkflowChange(ctx, db.LockProjectForWorkflowChangeParams{ID: loaded.ID, WorkspaceID: wsUUID})
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	var target *issueworkflow.Definition
	if targetID.Valid {
		row, err := qtx.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: targetID, WorkspaceID: wsUUID})
		if err != nil {
			writeError(w, http.StatusBadRequest, "workflow not found in this workspace")
			return
		}
		def, err := issueworkflow.Decode(row)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to read workflow")
			return
		}
		if def.IsDefault {
			// Choosing the Default row is the same as choosing Default.
			targetID = pgtype.UUID{}
		} else {
			target = &def
		}
	}
	catalog, err := h.workflowStatusCatalog(ctx, qtx, wsUUID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to load issue statuses")
		return
	}
	// The Default workflow accepts every catalog status, archived included:
	// issues already on an archived status keep it, as they always have.
	allowed := func(key string) bool {
		if target == nil {
			_, ok := catalog[key]
			return ok
		}
		return target.Has(key)
	}
	targetAccepts := func(key string) bool {
		if target == nil {
			entry, ok := catalog[key]
			return ok && !entry.Archived
		}
		return target.Has(key)
	}
	counts, err := qtx.CountProjectIssuesByStatus(ctx, db.CountProjectIssuesByStatusParams{WorkspaceID: wsUUID, ProjectIds: []pgtype.UUID{project.ID}})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	categoryOf := func(key string) string { return catalog[key].Category }
	plan := mappingPlan(counts, allowed, func(key string) string {
		if target == nil {
			return issuestatus.Todo
		}
		return issueworkflow.MoveTarget(*target, key, categoryOf)
	})
	if req.DryRun {
		writeJSON(w, http.StatusOK, map[string]any{"dry_run": true, "plan": plan})
		return
	}
	if msg := checkMapping(plan, req.StatusMapping, targetAccepts); msg != "" {
		writeMappingRequired(w, plan, msg)
		return
	}
	updated, err := qtx.SetProjectWorkflow(ctx, db.SetProjectWorkflowParams{ID: project.ID, WorkspaceID: wsUUID, WorkflowID: targetID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	changed, cancelledWakeups, err := remapIssues(ctx, qtx, wsUUID, []pgtype.UUID{project.ID}, plan, req.StatusMapping)
	if err != nil {
		slog.Warn("SetProjectWorkflow remap failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to move issues to the new statuses")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to change workflow")
		return
	}
	h.publishIssueSnapshots(ctx, changed, "member", uuidToString(member.UserID))
	h.broadcastCancelledWakeups(ctx, wsUUID, cancelledWakeups)
	resp := h.publishProjectWorkflowUpdate(ctx, workspaceID, member, updated)
	writeJSON(w, http.StatusOK, map[string]any{
		"project":        resp,
		"updated_issues": len(changed),
		"plan":           plan,
	})
}

// SetIssueWorkflowHandoffRequest is a column's handoff. CopyForProjectID,
// when set, leaves the workflow as it is and saves the handoff on a copy of
// it that only that project uses.
type SetIssueWorkflowHandoffRequest struct {
	IssueWorkflowStepPayload
	CopyForProjectID string `json:"copy_for_project_id,omitempty"`
}

// SetIssueWorkflowHandoff saves what a workflow does when an issue enters
// one of its columns: who it is handed to, the instructions their run gets,
// and where the handler moves it next. It applies to every project using the
// workflow, unless the caller asks for a copy for one project: the project
// then switches to the copy in the same transaction. The copy lists the same
// statuses, so no issue moves.
func (h *Handler) SetIssueWorkflowHandoff(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	var req SetIssueWorkflowHandoffRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	var copyFor pgtype.UUID
	if req.CopyForProjectID != "" {
		copyFor, ok = parseUUIDOrBadRequest(w, req.CopyForProjectID, "copy_for_project_id")
		if !ok {
			return
		}
	}
	ctx := r.Context()
	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	if err := qtx.LockIssueStatusCatalog(ctx, wsUUID); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	current, err := qtx.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	if !canEditIssueWorkflow(member, current) {
		writeError(w, http.StatusForbidden, "only the workflow owner and workspace admins can change this workflow")
		return
	}
	if workflowLocked(current) {
		writeWorkflowLocked(w, current)
		return
	}
	def, err := issueworkflow.Decode(current)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read workflow")
		return
	}
	status := chi.URLParam(r, "status")
	var prev *issueworkflow.Step
	if s, ok := def.Step(status); ok {
		prev = &s
	}
	step := issueworkflow.NormalizeHandoff(payloadToStep(status, req.IssueWorkflowStepPayload, prev))
	if err := issueworkflow.ValidateHandoff(step, def); err != nil {
		writeErrorCode(w, http.StatusBadRequest, "invalid_handoff", err.Error())
		return
	}
	if status, msg := h.validateHandoffHandlers(ctx, r, workspaceID, []issueworkflow.Step{step}, def.Steps); status != 0 {
		writeErrorCode(w, status, "invalid_handoff", msg)
		return
	}
	for i, s := range def.Steps {
		if s.StatusKey == step.StatusKey {
			def.Steps[i] = step
		}
	}
	raw, err := issueworkflow.EncodeSteps(def.Steps)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	if copyFor.Valid {
		h.saveHandoffOnProjectCopy(w, r, qtx, tx, member, workspaceID, current, raw, copyFor)
		return
	}
	row, err := qtx.UpdateIssueWorkflow(ctx, db.UpdateIssueWorkflowParams{
		ID:               current.ID,
		WorkspaceID:      wsUUID,
		Name:             current.Name,
		Description:      current.Description,
		InitialStatusKey: issueworkflow.StartStatus,
		Steps:            raw,
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	h.publishIssueWorkflowChanged(workspaceID, member, "updated")
	h.writeIssueWorkflow(w, r, http.StatusOK, row)
}

// saveHandoffOnProjectCopy finishes SetIssueWorkflowHandoff for a copy:
// steps already carry the new handoff. The caller holds the exclusive catalog
// lock in tx.
func (h *Handler) saveHandoffOnProjectCopy(w http.ResponseWriter, r *http.Request, qtx *db.Queries, tx pgx.Tx, member db.Member, workspaceID string, source db.IssueWorkflow, steps []byte, projectID pgtype.UUID) {
	ctx := r.Context()
	if source.IsDefault {
		writeErrorCode(w, http.StatusBadRequest, "invalid_handoff", "the Default workflow cannot be copied for one project")
		return
	}
	project, err := qtx.LockProjectForWorkflowChange(ctx, db.LockProjectForWorkflowChangeParams{ID: projectID, WorkspaceID: source.WorkspaceID})
	if err != nil {
		writeError(w, http.StatusNotFound, "project not found")
		return
	}
	if project.WorkflowID != source.ID {
		writeErrorCode(w, http.StatusConflict, "invalid_handoff", "the project does not use this workflow")
		return
	}
	// The copy switches the project's workflow, which only admins and the
	// project's lead may do.
	if !canManageProjectWorkflow(member, project) {
		writeError(w, http.StatusForbidden, "only workspace admins and the project lead can change this project's workflow")
		return
	}
	existing, err := qtx.ListIssueWorkflows(ctx, source.WorkspaceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	taken := make(map[string]bool, len(existing))
	for _, row := range existing {
		taken[strings.ToLower(row.Name)] = true
	}
	row, err := qtx.CreateIssueWorkflow(ctx, db.CreateIssueWorkflowParams{
		WorkspaceID:      source.WorkspaceID,
		Name:             projectCopyName(source.Name, project.Title, taken),
		Description:      source.Description,
		InitialStatusKey: issueworkflow.StartStatus,
		Steps:            steps,
		OwnerID:          member.UserID,
		FlowConfig:       source.FlowConfig,
	})
	if err != nil {
		slog.Warn("SetIssueWorkflowHandoff copy failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	updated, err := qtx.SetProjectWorkflow(ctx, db.SetProjectWorkflowParams{ID: project.ID, WorkspaceID: source.WorkspaceID, WorkflowID: row.ID})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	if err := tx.Commit(ctx); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save handoff")
		return
	}
	h.publishIssueWorkflowChanged(workspaceID, member, "created")
	h.publishProjectWorkflowUpdate(ctx, workspaceID, member, updated)
	h.writeIssueWorkflow(w, r, http.StatusCreated, row)
}

// projectCopyName names a workflow copied for one project, "Workflow
// (Project)", numbered when that name is taken and cut to the name limit.
func projectCopyName(workflow, project string, taken map[string]bool) string {
	for n := 1; ; n++ {
		suffix := " (" + project + ")"
		if n > 1 {
			suffix = " (" + project + " " + strconv.Itoa(n) + ")"
		}
		base := []rune(workflow)
		if room := issueworkflow.MaxNameRunes - utf8.RuneCountInString(suffix); room < len(base) {
			if room < 1 {
				suffix = " " + strconv.Itoa(n)
				room = issueworkflow.MaxNameRunes - len(suffix)
			}
			base = base[:room]
		}
		name := strings.TrimSpace(string(base)) + suffix
		if !taken[strings.ToLower(name)] {
			return name
		}
	}
}

// PreviewIssueWorkflowBriefRequest is a handoff being edited. ProjectID, when
// set, names the project the preview is for.
type PreviewIssueWorkflowBriefRequest struct {
	IssueWorkflowStepPayload
	ProjectID string `json:"project_id,omitempty"`
}

// PreviewIssueWorkflowBrief renders the brief a column's handler would
// receive, for a handoff being edited before it is saved.
func (h *Handler) PreviewIssueWorkflowBrief(w http.ResponseWriter, r *http.Request) {
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	if _, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found"); !ok {
		return
	}
	idUUID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	var req PreviewIssueWorkflowBriefRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	ctx := r.Context()
	row, err := h.Queries.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: idUUID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "workflow not found")
		return
	}
	def, err := issueworkflow.Decode(row)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to read workflow")
		return
	}
	projectName := ""
	if req.ProjectID != "" {
		if id, err := parseUUIDString(req.ProjectID); err == nil {
			if p, err := h.Queries.GetProjectInWorkspace(ctx, db.GetProjectInWorkspaceParams{ID: id, WorkspaceID: wsUUID}); err == nil {
				projectName = p.Title
			}
		}
	}
	step := issueworkflow.NormalizeHandoff(payloadToStep(req.StatusKey, req.IssueWorkflowStepPayload, nil))
	if !def.Has(step.StatusKey) {
		writeError(w, http.StatusBadRequest, "status_key is not a column of this workflow")
		return
	}
	// The brief names the draft's handler, so an unchecked id would let the
	// caller read the name of a user outside this workspace.
	if status, msg := h.validateHandoffHandlers(ctx, r, workspaceID, []issueworkflow.Step{step}, def.Steps); status != 0 {
		writeError(w, status, msg)
		return
	}
	for i, s := range def.Steps {
		if s.StatusKey == step.StatusKey {
			def.Steps[i] = step
		}
	}
	resolver := issuestatus.NewResolver(wsUUID)
	brief := issueworkflow.Brief(issueworkflow.BriefInput{
		Workflow:        def,
		ProjectName:     projectName,
		StatusKey:       step.StatusKey,
		IssueIdentifier: h.getIssuePrefix(ctx, wsUUID) + "-123",
		StatusName: func(key string) string {
			return resolver.Name(ctx, h.Queries, key)
		},
		HandlerName: func(s issueworkflow.Step) string {
			switch s.Handler.Type {
			case issueworkflow.HandlerAgent, issueworkflow.HandlerSquad, issueworkflow.HandlerMember:
				id, err := parseUUIDString(s.Handler.ID)
				if err != nil {
					return s.Handler.Type
				}
				return h.actorDisplayName(ctx, wsUUID, s.Handler.Type, id)
			case issueworkflow.HandlerProjectLead:
				return "the project lead"
			case issueworkflow.HandlerCreator:
				return "the issue creator"
			}
			return ""
		},
	})
	writeJSON(w, http.StatusOK, map[string]any{"brief": brief})
}

// WorkflowHandoffRun is an active run of the agent or squad a handoff takes
// the issue from.
type WorkflowHandoffRun struct {
	TaskID    string  `json:"task_id"`
	AgentID   string  `json:"agent_id"`
	Status    string  `json:"status"`
	StartedAt *string `json:"started_at"`
}

type WorkflowHandoffPreviewResponse struct {
	Handoff              bool                 `json:"handoff"`
	WorkflowName         string               `json:"workflow_name,omitempty"`
	FromStatus           string               `json:"from_status"`
	ToStatus             string               `json:"to_status"`
	HandlerType          string               `json:"handler_type,omitempty"`
	HandlerID            string               `json:"handler_id,omitempty"`
	PreviousAssigneeType *string              `json:"previous_assignee_type"`
	PreviousAssigneeID   *string              `json:"previous_assignee_id"`
	PreviousRuns         []WorkflowHandoffRun `json:"previous_runs"`
	Brief                string               `json:"brief,omitempty"`
}

// PreviewWorkflowHandoff says what moving an issue to a status would do under
// its project's workflow, for the confirmation shown before a handoff: who
// the issue would go to, which runs of its current agent are still active,
// and the brief the new handler would receive. With project_id it previews
// moving the issue into that project and entering the step it lands on
// (status then optional). It writes nothing.
func (h *Handler) PreviewWorkflowHandoff(w http.ResponseWriter, r *http.Request) {
	issue, ok := h.loadIssueForUser(w, r, chi.URLParam(r, "id"))
	if !ok {
		return
	}
	status := r.URL.Query().Get("status")
	projectParam := r.URL.Query().Get("project_id")
	if status == "" && projectParam == "" {
		writeError(w, http.StatusBadRequest, "status is required")
		return
	}
	params := db.UpdateIssueParams{
		ProjectID:    issue.ProjectID,
		AssigneeType: issue.AssigneeType,
		AssigneeID:   issue.AssigneeID,
	}
	if status != "" {
		params.Status = pgtype.Text{String: status, Valid: true}
	}
	enterOnMove := false
	if projectParam != "" {
		projectID, ok := parseUUIDOrBadRequest(w, projectParam, "project_id")
		if !ok {
			return
		}
		if _, err := h.Queries.GetProjectInWorkspace(r.Context(), db.GetProjectInWorkspaceParams{ID: projectID, WorkspaceID: issue.WorkspaceID}); err != nil {
			writeError(w, http.StatusNotFound, "project not found")
			return
		}
		params.ProjectID = projectID
		enterOnMove = true
	}
	hand, err := h.applyIssueWorkflow(r.Context(), issue, &params, status != "", false, enterOnMove, nil, "")
	if err != nil {
		if writeIssueWorkflowError(w, err) {
			return
		}
		writeError(w, http.StatusInternalServerError, "failed to resolve project workflow")
		return
	}
	if params.Status.Valid {
		status = params.Status.String
	} else {
		status = issue.Status
	}
	resp := WorkflowHandoffPreviewResponse{
		FromStatus:           issue.Status,
		ToStatus:             status,
		PreviousAssigneeType: textToPtr(issue.AssigneeType),
		PreviousAssigneeID:   uuidToPtr(issue.AssigneeID),
		PreviousRuns:         []WorkflowHandoffRun{},
	}
	if hand == nil {
		writeJSON(w, http.StatusOK, resp)
		return
	}
	resp.WorkflowName = hand.def.Name
	resp.Handoff = true
	resp.HandlerType = params.AssigneeType.String
	resp.HandlerID = uuidToString(params.AssigneeID)
	next := issue
	next.Status = status
	next.ProjectID = params.ProjectID
	next.AssigneeType = params.AssigneeType
	next.AssigneeID = params.AssigneeID
	resp.Brief = h.workflowHandoffBrief(r.Context(), next, hand)
	if issue.AssigneeType != params.AssigneeType || issue.AssigneeID != params.AssigneeID {
		for _, task := range h.assigneeActiveRuns(r.Context(), issue) {
			started := task.StartedAt
			if !started.Valid {
				started = task.CreatedAt
			}
			resp.PreviousRuns = append(resp.PreviousRuns, WorkflowHandoffRun{
				TaskID:    uuidToString(task.ID),
				AgentID:   uuidToString(task.AgentID),
				Status:    task.Status,
				StartedAt: timestampToPtr(started),
			})
		}
	}
	writeJSON(w, http.StatusOK, resp)
}
