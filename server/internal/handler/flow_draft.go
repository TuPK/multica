package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Drafts of a flow workflow (docs/engineering/flow-engine-generalization-spec.md,
// sections 7.6 and 8): a person who manages a squad writes a document of schema 2,
// checks it, simulates it and publishes it as a locked version of a lineage.

const flowManifestFormat = "multica.flow-manifest/v1"

// FlowDraftResponse is a draft with its document.
type FlowDraftResponse struct {
	ID             string          `json:"id"`
	SquadID        string          `json:"squad_id"`
	Name           string          `json:"name"`
	LineageID      *string         `json:"lineage_id"`
	BaseWorkflowID *string         `json:"base_workflow_id"`
	Revision       int32           `json:"revision"`
	Doc            json.RawMessage `json:"doc"`
	Origin         string          `json:"origin"`
	SourceRef      json.RawMessage `json:"source_ref"`
	CreatedBy      string          `json:"created_by"`
	UpdatedBy      string          `json:"updated_by"`
	CreatedAt      string          `json:"created_at"`
	UpdatedAt      string          `json:"updated_at"`
}

// FlowDraftSummary is a draft without its document, with what a list shows.
type FlowDraftSummary struct {
	ID             string  `json:"id"`
	SquadID        string  `json:"squad_id"`
	Name           string  `json:"name"`
	LineageID      *string `json:"lineage_id"`
	BaseWorkflowID *string `json:"base_workflow_id"`
	Revision       int32   `json:"revision"`
	Origin         string  `json:"origin"`
	CreatedBy      string  `json:"created_by"`
	UpdatedBy      string  `json:"updated_by"`
	CreatedAt      string  `json:"created_at"`
	UpdatedAt      string  `json:"updated_at"`
	Summary        struct {
		Steps int `json:"steps"`
		Tasks int `json:"tasks"`
		Gates int `json:"gates"`
	} `json:"summary"`
}

func nullableJSON(raw []byte) json.RawMessage {
	if len(raw) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(raw)
}

func flowDraftToResponse(d db.IssueWorkflowDraft) FlowDraftResponse {
	return FlowDraftResponse{
		ID: uuidToString(d.ID), SquadID: uuidToString(d.SquadID), Name: d.Name,
		LineageID: uuidToPtr(d.LineageID), BaseWorkflowID: uuidToPtr(d.BaseWorkflowID),
		Revision: d.Revision, Doc: nullableJSON(d.Doc), Origin: d.Origin, SourceRef: nullableJSON(d.SourceRef),
		CreatedBy: uuidToString(d.CreatedBy), UpdatedBy: uuidToString(d.UpdatedBy),
		CreatedAt: timestampToString(d.CreatedAt), UpdatedAt: timestampToString(d.UpdatedAt),
	}
}

func flowDraftToSummary(d db.IssueWorkflowDraft) FlowDraftSummary {
	s := FlowDraftSummary{
		ID: uuidToString(d.ID), SquadID: uuidToString(d.SquadID), Name: d.Name,
		LineageID: uuidToPtr(d.LineageID), BaseWorkflowID: uuidToPtr(d.BaseWorkflowID),
		Revision: d.Revision, Origin: d.Origin, CreatedBy: uuidToString(d.CreatedBy), UpdatedBy: uuidToString(d.UpdatedBy),
		CreatedAt: timestampToString(d.CreatedAt), UpdatedAt: timestampToString(d.UpdatedAt),
	}
	var doc issueworkflow.FlowDoc
	if json.Unmarshal(d.Doc, &doc) == nil {
		s.Summary.Steps = len(doc.Steps)
		for _, step := range doc.Steps {
			for _, t := range step.Tasks {
				s.Summary.Tasks++
				if t.Kind == issueworkflow.KindGate {
					s.Summary.Gates++
				}
			}
		}
	}
	return s
}

// flowAuthoring is who calls a route of the authoring family and for which squad.
type flowAuthoring struct {
	member      db.Member
	squad       db.Squad
	workspaceID string
	wsUUID      pgtype.UUID
}

// authorFlow runs the checks every authoring route shares: the flag, a member
// (never an agent) of the workspace, and a squad of it. Every failure is written.
func (h *Handler) authorFlow(w http.ResponseWriter, r *http.Request) (flowAuthoring, bool) {
	if !featureflags.FlowAuthoringEnabled(r.Context(), h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled")
		return flowAuthoring{}, false
	}
	workspaceID := workspaceIDFromURL(r, "workspaceId")
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return flowAuthoring{}, false
	}
	if h.refuseFlowAgent(w, r, member, workspaceID, "flow_authoring_member_only", "an agent does not draft or publish a workflow") {
		return flowAuthoring{}, false
	}
	squad, _, ok := h.loadSquadInWorkspace(w, r)
	if !ok {
		return flowAuthoring{}, false
	}
	return flowAuthoring{member: member, squad: squad, workspaceID: workspaceID, wsUUID: squad.WorkspaceID}, true
}

// refuseFlowAgent answers 403 and reports true when the request is an agent's, whoever's
// credentials it carries (spec 16.4: authoring, activation and the runner switch are for
// people). It is the one place the rule is written.
func (h *Handler) refuseFlowAgent(w http.ResponseWriter, r *http.Request, member db.Member, workspaceID, code, message string) bool {
	if actorType, _ := h.resolveActor(r, uuidToString(member.UserID), workspaceID); actorType != "agent" {
		return false
	}
	writeErrorCode(w, http.StatusForbidden, code, message)
	return true
}

// authorFlowManager is authorFlow for a route that needs a member who manages the squad.
func (h *Handler) authorFlowManager(w http.ResponseWriter, r *http.Request, code string) (flowAuthoring, bool) {
	a, ok := h.authorFlow(w, r)
	if !ok {
		return flowAuthoring{}, false
	}
	if !canManageSquad(a.member, a.squad) {
		writeErrorCode(w, http.StatusForbidden, code, "only a member who manages the squad can do this")
		return flowAuthoring{}, false
	}
	return a, true
}

func (a flowAuthoring) draftID(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	return parseUUIDOrBadRequest(w, chi.URLParam(r, "draftId"), "draft id")
}

func (h *Handler) loadFlowDraft(w http.ResponseWriter, r *http.Request, a flowAuthoring) (db.IssueWorkflowDraft, bool) {
	id, ok := a.draftID(w, r)
	if !ok {
		return db.IssueWorkflowDraft{}, false
	}
	draft, err := h.Queries.GetFlowDraft(r.Context(), db.GetFlowDraftParams{ID: id, WorkspaceID: a.wsUUID, SquadID: a.squad.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		writeErrorCode(w, http.StatusNotFound, "flow_draft_not_found", "draft not found")
		return db.IssueWorkflowDraft{}, false
	}
	if err != nil {
		writeFlowError(w, r, err)
		return db.IssueWorkflowDraft{}, false
	}
	return draft, true
}

// readFlowBody reads a JSON body of at most the size of a document and some room
// for the fields around it. tooLarge is true when the body is bigger.
func readFlowBody(w http.ResponseWriter, r *http.Request, dest any) (ok, tooLarge bool) {
	limit := int64(issueworkflow.DocMaxBytes) + 64<<10
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, limit))
	if err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			return false, true
		}
		writeError(w, http.StatusBadRequest, "failed to read request body")
		return false, false
	}
	if len(raw) == 0 {
		return true, false
	}
	if err := json.Unmarshal(raw, dest); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return false, false
	}
	return true, false
}

func writeDraftTooLarge(w http.ResponseWriter) {
	writeErrorCode(w, http.StatusUnprocessableEntity, "flow_draft_too_large",
		fmt.Sprintf("a document is at most %d bytes", issueworkflow.DocMaxBytes))
}

func (h *Handler) publishFlowDraftUpdated(a flowAuthoring, draftID pgtype.UUID) {
	h.publish(protocol.EventFlowDraftUpdated, a.workspaceID, "member", uuidToString(a.member.UserID), map[string]any{
		"squad_id": uuidToString(a.squad.ID), "draft_id": uuidToString(draftID),
	})
}

// ListFlowDrafts lists the drafts of a squad.
func (h *Handler) ListFlowDrafts(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	rows, err := h.Queries.ListFlowDrafts(r.Context(), db.ListFlowDraftsParams{WorkspaceID: a.wsUUID, SquadID: a.squad.ID})
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	out := make([]FlowDraftSummary, len(rows))
	for i, d := range rows {
		out[i] = flowDraftToSummary(d)
	}
	writeJSON(w, http.StatusOK, map[string]any{"drafts": out, "total": len(out)})
}

// GetFlowDraft returns one draft.
func (h *Handler) GetFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	draft, ok := h.loadFlowDraft(w, r, a)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, flowDraftToResponse(draft))
}

type flowDraftFrom struct {
	Kind       string          `json:"kind"`
	WorkflowID string          `json:"workflow_id"`
	KeepBind   bool            `json:"keep_bind"`
	Manifest   json.RawMessage `json:"manifest"`
	// AgentID, References and Note ask an agent to draft (kind proposal): the agent, ids of files
	// the caller uploaded, and a few lines on the workflow wanted.
	AgentID    string   `json:"agent_id"`
	References []string `json:"references"`
	Note       string   `json:"note"`
}

type createFlowDraftRequest struct {
	Name string        `json:"name"`
	From flowDraftFrom `json:"from"`
}

// blankFlowDoc is a document that parses and has one step to start from: the plan
// step, which finishes the request.
func blankFlowDoc() json.RawMessage {
	doc := map[string]any{
		"version": 2, "start": "plan",
		"roles":     []any{},
		"terminals": []any{map[string]any{"key": "done", "status": "done"}},
		"stops":     []any{},
		"steps": []any{map[string]any{
			"key": "plan", "title": "Plan", "kind": "lead", "status": map[string]any{"key": issueworkflow.StartStatus},
			"lead":  map[string]any{"sets": []any{}},
			"edges": []any{map[string]any{"id": "plan_done", "label": "Done", "to": "done"}},
		}},
		"policy": map[string]any{},
	}
	raw, _ := json.Marshal(doc)
	return raw
}

// stripBind removes the people from the roles of a document: a manifest and a copy
// that is not rebound carry no binding.
func stripBind(raw []byte) ([]byte, error) {
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, err
	}
	if roles, ok := doc["roles"].([]any); ok {
		for _, role := range roles {
			if m, ok := role.(map[string]any); ok {
				delete(m, "bind")
			}
		}
	}
	return json.Marshal(doc)
}

// CreateFlowDraft makes a draft: empty, a copy of a workflow of the squad (a
// fork), an imported manifest, or an empty one that an agent is asked to fill
// (a proposal).
func (h *Handler) CreateFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	var req createFlowDraftRequest
	if ok, tooLarge := readFlowBody(w, r, &req); !ok {
		if tooLarge {
			writeDraftTooLarge(w)
		}
		return
	}
	ctx := r.Context()
	if a.squad.ArchivedAt.Valid {
		writeError(w, http.StatusConflict, "squad is archived")
		return
	}
	name := strings.TrimSpace(req.Name)
	var (
		doc       []byte
		origin    = "manual"
		lineage   pgtype.UUID
		base      pgtype.UUID
		sourceRef []byte
		proposal  *proposalRequest
	)
	switch req.From.Kind {
	case "", "blank":
		doc = blankFlowDoc()
	case "workflow":
		var status int
		var code, msg string
		doc, lineage, base, name, status, code, msg = h.forkDoc(ctx, a, req.From, name)
		if status != 0 {
			writeErrorCode(w, status, code, msg)
			return
		}
		origin = "fork"
	case "manifest":
		if !featureflags.FlowGraphV2Enabled(ctx, h.FeatureFlags) {
			writeErrorCode(w, http.StatusNotFound, "flow_graph_v2_disabled", "writing a flow document of schema 2 is not enabled")
			return
		}
		var manifestName string
		var issues []issueworkflow.Issue
		doc, manifestName, issues = parseFlowManifest(req.From.Manifest)
		if len(issues) > 0 {
			writeJSON(w, http.StatusBadRequest, map[string]any{"error": "the manifest is not valid", "code": "flow_manifest_invalid", "issues": issues})
			return
		}
		if name == "" {
			name = manifestName
		}
		origin = "import"
	case "proposal":
		if !featureflags.FlowPacksEnabled(ctx, h.FeatureFlags) {
			writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled")
			return
		}
		asked, status, code, msg := h.readProposalRequest(ctx, r, a, req.From)
		if status != 0 {
			writeErrorCode(w, status, code, msg)
			return
		}
		proposal = &asked
		doc = blankFlowDoc()
		origin = "proposal"
	default:
		writeErrorCode(w, http.StatusBadRequest, "flow_draft_from_unsupported", fmt.Sprintf("a draft cannot start from %q", req.From.Kind))
		return
	}
	if name == "" || utf8.RuneCountInString(name) > issueworkflow.MaxDraftNameRunes {
		writeErrorCode(w, http.StatusBadRequest, "flow_draft_name_invalid", fmt.Sprintf("a draft name is 1 to %d characters", issueworkflow.MaxDraftNameRunes))
		return
	}
	if len(doc) > issueworkflow.DocMaxBytes {
		writeDraftTooLarge(w)
		return
	}
	count, err := h.Queries.CountFlowDrafts(ctx, db.CountFlowDraftsParams{WorkspaceID: a.wsUUID, SquadID: a.squad.ID})
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	if count >= issueworkflow.MaxDraftsPerSquad {
		writeJSON(w, http.StatusConflict, map[string]any{"error": "the squad has the most drafts it may have", "code": "flow_limit_reached", "limit": "drafts", "max": issueworkflow.MaxDraftsPerSquad})
		return
	}
	if proposal != nil {
		issueID, err := h.createProposalIssue(ctx, a, name, *proposal)
		if writeIssueLimitReached(w, err) {
			return
		}
		if err != nil {
			slog.Warn("create proposal issue failed", append(logger.RequestAttrs(r), "error", err)...)
			writeErrorCode(w, http.StatusInternalServerError, "flow_proposal_failed", "could not create the issue for the agent")
			return
		}
		sourceRef, err = json.Marshal(proposalSourceRef{IssueID: uuidToString(issueID), AgentID: uuidToString(proposal.agent.ID)})
		if err != nil {
			writeFlowError(w, r, err)
			return
		}
	}
	draft, err := h.Queries.CreateFlowDraft(ctx, db.CreateFlowDraftParams{
		WorkspaceID: a.wsUUID, SquadID: a.squad.ID, LineageID: lineage, BaseWorkflowID: base,
		Name: name, Doc: doc, Origin: origin, SourceRef: sourceRef, ActorID: a.member.UserID,
	})
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	h.publishFlowDraftUpdated(a, draft.ID)
	writeJSON(w, http.StatusCreated, flowDraftToResponse(draft))
}

// forkDoc reads the document of a workflow for a new draft. A workflow of schema 2
// gives its document (rebound only if the caller keeps the binding); one of
// schema 1, a catalog flow included, is lifted to a clean document of schema 2
// whose roles are its agents (spec 8.2). The copy has no template_ref, so an
// upgrade of the catalog flow never reaches it.
func (h *Handler) forkDoc(ctx context.Context, a flowAuthoring, from flowDraftFrom, name string) (doc []byte, lineage, base pgtype.UUID, draftName string, status int, code, msg string) {
	id, err := parseFlowUUID(from.WorkflowID)
	if err != nil {
		return nil, lineage, base, "", http.StatusBadRequest, "invalid_id", "invalid workflow id"
	}
	row, err := h.Queries.GetIssueWorkflow(ctx, db.GetIssueWorkflowParams{ID: id, WorkspaceID: a.wsUUID})
	if err != nil {
		return nil, lineage, base, "", http.StatusNotFound, "workflow_not_found", "workflow not found"
	}
	if len(row.FlowConfig) == 0 || (row.SquadID.Valid && row.SquadID != a.squad.ID) {
		return nil, lineage, base, "", http.StatusNotFound, "workflow_not_found", "the squad has no such flow workflow"
	}
	def, err := issueworkflow.Decode(row)
	if err != nil {
		return nil, lineage, base, "", http.StatusUnprocessableEntity, "flow_draft_from_invalid", err.Error()
	}
	if def.Graph.Compat == issueworkflow.CompatV1 {
		doc, err = h.liftForFork(ctx, a, def)
	} else if from.KeepBind {
		doc = row.FlowConfig
	} else {
		doc, err = stripBind(row.FlowConfig)
	}
	if err != nil {
		return nil, lineage, base, "", http.StatusInternalServerError, "internal", "failed to read the workflow"
	}
	if name == "" {
		name = baseWorkflowName(row.Name)
	}
	return doc, row.LineageID, row.ID, name, 0, "", ""
}

// baseWorkflowName is a draft name taken from a version name: the version suffix goes.
func baseWorkflowName(name string) string {
	if i := strings.LastIndex(name, " · v"); i > 0 {
		name = name[:i]
	}
	runes := []rune(strings.TrimSpace(name))
	if len(runes) > issueworkflow.MaxDraftNameRunes {
		runes = runes[:issueworkflow.MaxDraftNameRunes]
	}
	return string(runes)
}

// liftForFork turns a workflow of schema 1 into the document of a draft: roles
// named a_<8 hex of the agent>, labelled with the agent's name, already bound.
func (h *Handler) liftForFork(ctx context.Context, a flowAuthoring, def issueworkflow.Definition) ([]byte, error) {
	graph, err := issueworkflow.Lift(def, issueworkflow.LiftOpts{})
	if err != nil {
		return nil, err
	}
	doc := graph.Doc
	// A schema 1 flow without texts of its own is read with the engine's defaults; the document
	// has to carry the texts its stops and notices name, or it would not publish.
	if doc.Messages == nil {
		doc.Messages = map[string]string{}
	}
	named := []string{}
	for _, stop := range doc.Stops {
		named = append(named, stop.Message)
	}
	for _, step := range doc.Steps {
		named = append(named, step.Notice)
	}
	for _, key := range named {
		if text, ok := defaultStopMessages[key]; ok && strings.TrimSpace(doc.Messages[key]) == "" {
			doc.Messages[key] = text
		}
	}
	for i, role := range doc.Roles {
		if role.Bind == nil || len(role.Bind.AgentIDs) == 0 {
			continue
		}
		agentID, err := parseFlowUUID(role.Bind.AgentIDs[0])
		if err != nil {
			continue
		}
		if agent, err := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: a.wsUUID}); err == nil {
			label := []rune(agent.Name)
			if len(label) > 80 {
				label = label[:80]
			}
			doc.Roles[i].Label = string(label)
		}
	}
	return json.Marshal(doc)
}

func parseFlowUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	err := u.Scan(strings.TrimSpace(s))
	return u, err
}

// parseFlowManifest reads a manifest strictly: a known format, no key it does not
// know, a document that parses, and no more bytes than a document may have. The
// roles of the result are unbound.
func parseFlowManifest(raw json.RawMessage) (doc []byte, name string, issues []issueworkflow.Issue) {
	manifest, issue := readFlowManifestEnvelope(raw)
	if issue != nil {
		return nil, "", []issueworkflow.Issue{*issue}
	}
	if _, found := issueworkflow.ParseFlowDoc(manifest.Doc); len(found) > 0 {
		return nil, "", found
	}
	unbound, err := stripBind(manifest.Doc)
	if err != nil {
		return nil, "", []issueworkflow.Issue{flowManifestIssue("the document is not valid: %v", err)}
	}
	return unbound, strings.TrimSpace(manifest.Name), nil
}

type flowManifestEnvelope struct {
	Format string          `json:"format"`
	Name   string          `json:"name"`
	Doc    json.RawMessage `json:"doc"`
}

func flowManifestIssue(format string, args ...any) issueworkflow.Issue {
	return issueworkflow.Issue{Code: "flow_manifest_invalid", Severity: issueworkflow.SeverityError, Message: fmt.Sprintf(format, args...)}
}

// readFlowManifestEnvelope reads the wrapper of a manifest strictly: a known format, no
// key it does not know, and no more bytes than a document may have. The document itself
// is left for the caller to check.
func readFlowManifestEnvelope(raw json.RawMessage) (flowManifestEnvelope, *issueworkflow.Issue) {
	var manifest flowManifestEnvelope
	bad := func(format string, args ...any) (flowManifestEnvelope, *issueworkflow.Issue) {
		issue := flowManifestIssue(format, args...)
		return manifest, &issue
	}
	if len(raw) == 0 {
		return bad("a manifest is needed")
	}
	if len(raw) > issueworkflow.DocMaxBytes+4<<10 {
		return bad("a manifest is at most %d bytes", issueworkflow.DocMaxBytes)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&manifest); err != nil {
		return bad("the manifest is not valid: %v", err)
	}
	if manifest.Format != flowManifestFormat {
		return bad("the format %q is not %s", manifest.Format, flowManifestFormat)
	}
	if len(manifest.Doc) > issueworkflow.DocMaxBytes {
		return bad("a document is at most %d bytes", issueworkflow.DocMaxBytes)
	}
	return manifest, nil
}

type updateFlowDraftRequest struct {
	Revision *int32          `json:"revision"`
	Name     *string         `json:"name"`
	Doc      json.RawMessage `json:"doc"`
}

// UpdateFlowDraft saves a draft at the revision the caller read.
func (h *Handler) UpdateFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	var req updateFlowDraftRequest
	if ok, tooLarge := readFlowBody(w, r, &req); !ok {
		if tooLarge {
			writeDraftTooLarge(w)
		}
		return
	}
	if req.Revision == nil {
		writeErrorCode(w, http.StatusBadRequest, "flow_draft_revision_required", "revision is required")
		return
	}
	id, ok := a.draftID(w, r)
	if !ok {
		return
	}
	params := db.UpdateFlowDraftParams{ID: id, WorkspaceID: a.wsUUID, SquadID: a.squad.ID, Revision: *req.Revision, ActorID: a.member.UserID}
	if req.Name != nil {
		name := strings.TrimSpace(*req.Name)
		if name == "" || utf8.RuneCountInString(name) > issueworkflow.MaxDraftNameRunes {
			writeErrorCode(w, http.StatusBadRequest, "flow_draft_name_invalid", fmt.Sprintf("a draft name is 1 to %d characters", issueworkflow.MaxDraftNameRunes))
			return
		}
		params.Name = pgtype.Text{String: name, Valid: true}
	}
	if len(req.Doc) > 0 && !bytes.Equal(bytes.TrimSpace(req.Doc), []byte("null")) {
		if len(req.Doc) > issueworkflow.DocMaxBytes {
			writeDraftTooLarge(w)
			return
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(req.Doc, &object); err != nil {
			writeErrorCode(w, http.StatusBadRequest, "flow_draft_doc_invalid", "doc must be a JSON object")
			return
		}
		params.Doc = req.Doc
	}
	draft, err := h.Queries.UpdateFlowDraft(r.Context(), params)
	if errors.Is(err, pgx.ErrNoRows) {
		current, getErr := h.Queries.GetFlowDraft(r.Context(), db.GetFlowDraftParams{ID: id, WorkspaceID: a.wsUUID, SquadID: a.squad.ID})
		if getErr != nil {
			writeErrorCode(w, http.StatusNotFound, "flow_draft_not_found", "draft not found")
			return
		}
		writeJSON(w, http.StatusConflict, map[string]any{
			"error": "the draft was saved by someone else: read it again", "code": "flow_draft_conflict", "revision": current.Revision,
		})
		return
	}
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	h.publishFlowDraftUpdated(a, draft.ID)
	writeJSON(w, http.StatusOK, flowDraftToResponse(draft))
}

// DeleteFlowDraft deletes a draft.
func (h *Handler) DeleteFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	id, ok := a.draftID(w, r)
	if !ok {
		return
	}
	n, err := h.Queries.DeleteFlowDraft(r.Context(), db.DeleteFlowDraftParams{ID: id, WorkspaceID: a.wsUUID, SquadID: a.squad.ID})
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	if n == 0 {
		writeErrorCode(w, http.StatusNotFound, "flow_draft_not_found", "draft not found")
		return
	}
	h.publishFlowDraftUpdated(a, id)
	w.WriteHeader(http.StatusNoContent)
}

// FlowValidationResponse is the answer of a check: the issues, the bounds of the
// document and the facts each expression of it may read.
type FlowValidationResponse struct {
	OK          bool                                `json:"ok"`
	Revision    int32                               `json:"revision"`
	Issues      []issueworkflow.Issue               `json:"issues"`
	Analysis    *issueworkflow.Analysis             `json:"analysis"`
	FactCatalog map[string][]issueworkflow.FactInfo `json:"fact_catalog"`
}

// flowValidateContext reads what a document is checked against: who the squad has,
// the statuses of the workspace and its members.
func (h *Handler) flowValidateContext(ctx context.Context, q *db.Queries, a flowAuthoring, publish bool) (issueworkflow.ValidateContext, error) {
	roster, err := h.squadRoster(ctx, q, a.squad, true)
	if err != nil {
		return issueworkflow.ValidateContext{}, err
	}
	return h.flowValidateContextWith(ctx, q, a, roster, publish)
}

// flowValidateContextWith is flowValidateContext for a roster the caller has read, or
// an empty one when the document is checked for no squad.
func (h *Handler) flowValidateContextWith(ctx context.Context, q *db.Queries, a flowAuthoring, roster issueworkflow.Roster, publish bool) (issueworkflow.ValidateContext, error) {
	catalog, err := h.workflowStatusCatalog(ctx, q, a.wsUUID)
	if err != nil {
		return issueworkflow.ValidateContext{}, err
	}
	statuses := issueworkflow.StatusCatalog{}
	for key, entry := range catalog {
		statuses[key] = issueworkflow.StatusInfo{Category: entry.Category, Archived: entry.Archived}
	}
	members, err := q.ListMembers(ctx, a.wsUUID)
	if err != nil {
		return issueworkflow.ValidateContext{}, err
	}
	workspaceMembers := make(map[string]bool, len(members))
	for _, m := range members {
		workspaceMembers[uuidToString(m.UserID)] = true
	}
	return issueworkflow.ValidateContext{
		Roster: roster, Statuses: statuses, WorkspaceMembers: workspaceMembers,
		CanCreateStatuses: isWorkspaceAdmin(a.member), Publish: publish,
	}, nil
}

// squadRoster is who a squad has: its lead, its agent members besides the lead, which of them are
// archived, and with skills the skills each runs with.
func (h *Handler) squadRoster(ctx context.Context, q *db.Queries, squad db.Squad, withSkills bool) (issueworkflow.Roster, error) {
	roster := issueworkflow.Roster{
		Lead: uuidToString(squad.LeaderID), Archived: map[string]bool{}, AgentSkills: map[string][]string{}, RoleKeys: map[string]string{},
	}
	members, err := q.ListSquadMembers(ctx, squad.ID)
	if err != nil {
		return roster, err
	}
	for _, m := range members {
		if m.MemberType != "agent" {
			continue
		}
		id := uuidToString(m.MemberID)
		// The lead plans and takes no task: it is the squad's lead, not one of its workers.
		if id == roster.Lead {
			continue
		}
		roster.Agents = append(roster.Agents, id)
		agent, err := q.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: m.MemberID, WorkspaceID: squad.WorkspaceID})
		if err != nil || agent.ArchivedAt.Valid {
			roster.Archived[id] = true
			continue
		}
		if !withSkills {
			continue
		}
		skills, err := q.ListEffectiveAgentSkills(ctx, m.MemberID)
		if err != nil {
			return roster, err
		}
		for _, s := range skills {
			roster.AgentSkills[id] = append(roster.AgentSkills[id], s.Name)
		}
	}
	return roster, nil
}

func newFlowValidation(revision int32, raw []byte, ctx issueworkflow.ValidateContext) FlowValidationResponse {
	out := FlowValidationResponse{Revision: revision}
	out.Issues = issueworkflow.ValidateFlowDocJSON(raw, ctx)
	if out.Issues == nil {
		out.Issues = []issueworkflow.Issue{}
	}
	out.OK = !issueworkflow.HasErrors(out.Issues)
	if doc, parseIssues := issueworkflow.ParseFlowDoc(raw); len(parseIssues) == 0 {
		analysis := issueworkflow.Analyze(doc)
		out.Analysis = &analysis
		out.FactCatalog = issueworkflow.FactCatalog(doc)
	} else {
		out.FactCatalog = map[string][]issueworkflow.FactInfo{}
	}
	return out
}

// ValidateFlowDraft checks a draft, or the document in the body, and answers 200
// with every issue it finds.
func (h *Handler) ValidateFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	var req struct {
		Doc json.RawMessage `json:"doc"`
	}
	if ok, tooLarge := readFlowBody(w, r, &req); !ok {
		if tooLarge {
			writeDraftTooLarge(w)
		}
		return
	}
	draft, ok := h.loadFlowDraft(w, r, a)
	if !ok {
		return
	}
	doc := []byte(draft.Doc)
	if len(req.Doc) > 0 && !bytes.Equal(bytes.TrimSpace(req.Doc), []byte("null")) {
		doc = req.Doc
	}
	vctx, err := h.flowValidateContext(r.Context(), h.Queries, a, false)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, newFlowValidation(draft.Revision, doc, vctx))
}

// FlowTraceResponse is a simulation: the hops the request made and how it ended.
type FlowTraceResponse struct {
	Hops      []FlowTraceHop `json:"hops"`
	End       FlowTraceEnd   `json:"end"`
	Truncated bool           `json:"truncated"`
}

// FlowTraceHop is one entry into a step and how it ended.
type FlowTraceHop struct {
	N          int               `json:"n"`
	Step       string            `json:"step"`
	Activation int               `json:"activation"`
	Opened     []string          `json:"opened,omitempty"`
	Closed     map[string]string `json:"closed,omitempty"`
	Join       string            `json:"join,omitempty"`
	Edge       string            `json:"edge,omitempty"`
	To         string            `json:"to,omitempty"`
	Loops      map[string]int    `json:"loops,omitempty"`
}

// FlowTraceEnd says how a simulated request ended: terminal, stop, stalled or waiting.
type FlowTraceEnd struct {
	Kind string `json:"kind"`
	Code string `json:"code,omitempty"`
	From string `json:"from,omitempty"`
}

// SimulateFlowDraft runs the document through the engine's own decision on made-up
// task rows. It writes nothing and runs no agent: it shows the routing, the loop
// limits and the stops, not that an agent does the work.
func (h *Handler) SimulateFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	var req struct {
		Doc     json.RawMessage                `json:"doc"`
		Vars    map[string]any                 `json:"vars"`
		Script  map[string][]map[string]string `json:"script"`
		MaxHops int                            `json:"max_hops"`
	}
	if ok, tooLarge := readFlowBody(w, r, &req); !ok {
		if tooLarge {
			writeDraftTooLarge(w)
		}
		return
	}
	draft, ok := h.loadFlowDraft(w, r, a)
	if !ok {
		return
	}
	raw := []byte(draft.Doc)
	if len(req.Doc) > 0 && !bytes.Equal(bytes.TrimSpace(req.Doc), []byte("null")) {
		raw = req.Doc
	}
	doc, issues := issueworkflow.ParseFlowDoc(raw)
	if len(issues) > 0 {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "the document cannot be simulated", "code": "flow_draft_invalid", "issues": issues})
		return
	}
	hops := req.MaxHops
	if hops <= 0 {
		hops = 60
	}
	if hops > issueworkflow.MaxSimulateHops {
		hops = issueworkflow.MaxSimulateHops
	}
	trace := issueworkflow.Simulate(issueworkflow.NewGraph(doc), req.Vars, req.Script, hops)
	out := FlowTraceResponse{Hops: make([]FlowTraceHop, len(trace.Steps)), Truncated: trace.Truncated}
	for i, s := range trace.Steps {
		out.Hops[i] = FlowTraceHop{N: i, Step: s.Step, Activation: s.Activation, Opened: s.Opened, Closed: s.Classes, Join: s.JoinResult, Edge: s.Edge, To: s.To, Loops: s.Loops}
	}
	out.End = FlowTraceEnd{Kind: trace.Outcome, Code: trace.Code}
	if n := len(trace.Steps); n > 0 {
		out.End.From = trace.Steps[n-1].Step
	}
	writeJSON(w, http.StatusOK, out)
}

// FlowManifestResponse is a document without its people, for another squad or
// another workspace to import.
type FlowManifestResponse struct {
	Format string          `json:"format"`
	Name   string          `json:"name"`
	Doc    json.RawMessage `json:"doc"`
}

// ExportFlowDraft exports a draft as a manifest.
func (h *Handler) ExportFlowDraft(w http.ResponseWriter, r *http.Request) {
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	draft, ok := h.loadFlowDraft(w, r, a)
	if !ok {
		return
	}
	doc, err := stripBind(draft.Doc)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, FlowManifestResponse{Format: flowManifestFormat, Name: draft.Name, Doc: doc})
}

// GetIssueWorkflowFlowManifest exports a flow workflow as a manifest. A workflow
// of schema 1 is lifted to a clean document of schema 2.
func (h *Handler) GetIssueWorkflowFlowManifest(w http.ResponseWriter, r *http.Request) {
	if !featureflags.FlowAuthoringEnabled(r.Context(), h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled")
		return
	}
	workspaceID := h.resolveWorkspaceID(r)
	wsUUID, ok := parseUUIDOrBadRequest(w, workspaceID, "workspace id")
	if !ok {
		return
	}
	member, ok := h.requireWorkspaceMember(w, r, workspaceID, "workspace not found")
	if !ok {
		return
	}
	if h.refuseFlowAgent(w, r, member, workspaceID, "flow_authoring_member_only", "an agent does not export a workflow") {
		return
	}
	id, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "workflow id")
	if !ok {
		return
	}
	row, err := h.Queries.GetIssueWorkflow(r.Context(), db.GetIssueWorkflowParams{ID: id, WorkspaceID: wsUUID})
	if err != nil || len(row.FlowConfig) == 0 {
		writeErrorCode(w, http.StatusNotFound, "workflow_not_found", "flow workflow not found")
		return
	}
	if !h.workflowManagedBy(r.Context(), member, wsUUID, row) {
		writeErrorCode(w, http.StatusForbidden, "flow_forbidden", "only a member who manages the squad of the workflow can export it")
		return
	}
	def, err := issueworkflow.Decode(row)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	var doc []byte
	if def.Graph.Compat == issueworkflow.CompatV1 {
		doc, err = h.liftForFork(r.Context(), flowAuthoring{wsUUID: wsUUID}, def)
		if err == nil {
			doc, err = stripBind(doc)
		}
	} else {
		doc, err = stripBind(row.FlowConfig)
	}
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, FlowManifestResponse{Format: flowManifestFormat, Name: baseWorkflowName(row.Name), Doc: doc})
}
