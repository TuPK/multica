package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	"github.com/multica-ai/multica/server/internal/pack"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// A proposal is a draft that an agent writes the first version of (spec 16.4). The
// agent gets an ordinary issue and answers with one manifest file attached to it; a
// person who manages the squad imports the file, and nothing the agent wrote is a
// workflow until that person publishes it.

const (
	maxProposalReferences = 10
	maxProposalNoteRunes  = 4000
	maxBriefMembers       = 50
	maxBriefSkills        = 12
	flowManifestFileEnd   = "flow.json"
)

// proposalRequest is what a member asks for in `from` of a draft of kind proposal.
type proposalRequest struct {
	agent      db.Agent
	note       string
	references []db.Attachment
}

// proposalSourceRef is source_ref of a draft of origin proposal: the issue the agent was given and
// the agent, so the file that counts is the one that agent attached and not one of whoever is
// assigned later.
type proposalSourceRef struct {
	IssueID string `json:"issue_id"`
	AgentID string `json:"agent_id"`
}

// readProposalRequest checks the agent and the references of a request to draft by proposal.
func (h *Handler) readProposalRequest(ctx context.Context, r *http.Request, a flowAuthoring, from flowDraftFrom) (req proposalRequest, status int, code, msg string) {
	agentID, err := parseFlowUUID(from.AgentID)
	if err != nil || !agentID.Valid {
		return req, http.StatusBadRequest, "flow_proposal_agent_invalid", "agent_id must be an agent of this workspace"
	}
	agent, err := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: a.wsUUID})
	if err != nil || agent.ArchivedAt.Valid {
		return req, http.StatusBadRequest, "flow_proposal_agent_invalid", "agent_id must be an agent of this workspace"
	}
	// The request becomes an issue given to the agent, so the member must be allowed to give it work.
	assignee := pgtype.Text{String: "agent", Valid: true}
	if st, _ := h.validateAssigneePair(ctx, r, a.workspaceID, assignee, agent.ID); st != 0 {
		if st == http.StatusForbidden {
			return req, st, "flow_proposal_agent_forbidden", "you cannot give work to this agent"
		}
		return req, http.StatusBadRequest, "flow_proposal_agent_invalid", "agent_id must be an agent of this workspace"
	}
	req.agent = agent

	req.note = strings.TrimSpace(from.Note)
	if utf8.RuneCountInString(req.note) > maxProposalNoteRunes {
		return req, http.StatusBadRequest, "flow_proposal_note_invalid", fmt.Sprintf("a note is at most %d characters", maxProposalNoteRunes)
	}

	if len(from.References) > maxProposalReferences {
		return req, http.StatusBadRequest, "flow_proposal_reference_invalid", fmt.Sprintf("a proposal has at most %d references", maxProposalReferences)
	}
	seen := map[string]bool{}
	var ids []pgtype.UUID
	for _, ref := range from.References {
		id, err := parseFlowUUID(ref)
		if err != nil || !id.Valid {
			return req, http.StatusBadRequest, "flow_proposal_reference_invalid", "a reference is the id of a file you uploaded"
		}
		if !seen[uuidToString(id)] {
			seen[uuidToString(id)] = true
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return req, 0, "", ""
	}
	found, err := h.Queries.ListAttachmentsByIDs(ctx, db.ListAttachmentsByIDsParams{AttachmentIds: ids, WorkspaceID: a.wsUUID})
	if err != nil {
		return req, http.StatusInternalServerError, "flow_proposal_failed", "could not read the references"
	}
	// A reference is a file the member uploaded and that belongs to nothing yet: a proposal
	// does not pull in a file of somebody else or one that is already attached elsewhere.
	for _, att := range found {
		own := att.UploaderType == "member" && att.UploaderID == a.member.UserID
		free := !att.IssueID.Valid && !att.CommentID.Valid && !att.ChatSessionID.Valid && !att.ChatMessageID.Valid && !att.SourceContextID.Valid
		if !own || !free {
			return req, http.StatusBadRequest, "flow_proposal_reference_invalid", "a reference is the id of a file you uploaded and have not attached yet"
		}
	}
	if len(found) != len(ids) {
		return req, http.StatusBadRequest, "flow_proposal_reference_invalid", "a reference is the id of a file you uploaded and have not attached yet"
	}
	req.references = found
	return req, 0, "", ""
}

// createProposalIssue makes the issue the agent works on: an ordinary issue given to the
// agent, on no flow, whose description is the brief.
func (h *Handler) createProposalIssue(ctx context.Context, a flowAuthoring, draftName string, req proposalRequest) (pgtype.UUID, error) {
	members, err := h.proposalBriefMembers(ctx, a.squad)
	if err != nil {
		return pgtype.UUID{}, err
	}
	names := make([]string, 0, len(req.references))
	attachmentIDs := make([]pgtype.UUID, 0, len(req.references))
	for _, att := range req.references {
		names = append(names, att.Filename)
		attachmentIDs = append(attachmentIDs, att.ID)
	}
	brief := buildProposalBrief(proposalBrief{
		SquadID: uuidToString(a.squad.ID), SquadName: a.squad.Name, DraftName: draftName,
		Members: members, References: names, Note: req.note,
	})
	res, err := h.IssueService.Create(ctx, service.IssueCreateParams{
		WorkspaceID:   a.wsUUID,
		Title:         "Draft a workflow: " + draftName,
		Description:   pgtype.Text{String: brief, Valid: true},
		Status:        "todo",
		Priority:      "none",
		AssigneeType:  pgtype.Text{String: "agent", Valid: true},
		AssigneeID:    req.agent.ID,
		CreatorType:   "member",
		CreatorID:     a.member.UserID,
		AttachmentIDs: attachmentIDs,
	}, service.IssueCreateOpts{ActorID: uuidToString(a.member.UserID)})
	if err != nil {
		return pgtype.UUID{}, err
	}
	return res.Issue.ID, nil
}

// proposalBriefMembers reads who the squad has for the brief: the lead, then the agents that can
// still take work, each with the skills it runs with. Nothing else of an agent is read.
func (h *Handler) proposalBriefMembers(ctx context.Context, squad db.Squad) ([]proposalBriefMember, error) {
	rows, err := h.Queries.ListSquadMembers(ctx, squad.ID)
	if err != nil {
		return nil, err
	}
	var lead *proposalBriefMember
	var workers []proposalBriefMember
	used := map[string]bool{"lead": true, "requester": true, "managers": true}
	for _, m := range rows {
		if m.MemberType != "agent" {
			continue
		}
		agent, err := h.Queries.GetAgentInWorkspace(ctx, db.GetAgentInWorkspaceParams{ID: m.MemberID, WorkspaceID: squad.WorkspaceID})
		if err != nil || agent.ArchivedAt.Valid {
			continue
		}
		if m.MemberID == squad.LeaderID {
			lead = &proposalBriefMember{Key: "lead", Name: agent.Name, Lead: true}
			continue
		}
		skills, err := h.Queries.ListEffectiveAgentSkills(ctx, m.MemberID)
		if err != nil {
			return nil, err
		}
		member := proposalBriefMember{Key: uniqueRoleKey(agent.Name, used), Name: agent.Name}
		for _, s := range skills {
			member.Skills = append(member.Skills, s.Name)
		}
		workers = append(workers, member)
	}
	if len(workers) > maxBriefMembers {
		workers = workers[:maxBriefMembers]
	}
	if lead != nil {
		return append([]proposalBriefMember{*lead}, workers...), nil
	}
	return workers, nil
}

// uniqueRoleKey suggests the key of a role from an agent's name: a valid document key that no
// other member of the brief has.
func uniqueRoleKey(name string, used map[string]bool) string {
	base := pack.AgentKey(name)
	for strings.Contains(base, "__") {
		base = strings.ReplaceAll(base, "__", "_")
	}
	base = strings.Trim(base, "_")
	if base == "" || base[0] < 'a' || base[0] > 'z' {
		base = "agent_" + base
		base = strings.TrimRight(base, "_")
	}
	if len(base) > 28 {
		base = strings.TrimRight(base[:28], "_")
	}
	key := base
	for n := 2; used[key]; n++ {
		key = fmt.Sprintf("%s_%d", base, n)
	}
	used[key] = true
	return key
}

type proposalBriefMember struct {
	Key, Name string
	Skills    []string
	Lead      bool
}

type proposalBrief struct {
	SquadID, SquadName, DraftName, Note string
	Members                             []proposalBriefMember
	References                          []string
}

// buildProposalBrief is the description of the proposal issue. It names the squad's members by
// key and name, never by id, and carries no instruction text or setting of any agent: a brief
// is read by an agent that is not part of the squad.
func buildProposalBrief(in proposalBrief) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Draft a workflow for %s\n\n", in.SquadName)
	b.WriteString("Turn a written procedure into a workflow for this squad. Your product is one file, `flow.json`. " +
		"A person who manages the squad imports it as the draft \"" + in.DraftName + "\", reads it, tests it and publishes it. " +
		"You write nothing else: you do not create drafts, publish, or bind people to roles.\n\n")
	b.WriteString("Use the `flow-authoring` skill. If it is not installed on your runtime, run `multica flow capabilities` for the schema and the limits.\n\n")

	b.WriteString("## The squad\n\n")
	b.WriteString("Use these keys as `roles[].key` (kind `agent`) for the members that do tasks. They are suggestions; the person binds each role to a member when publishing.\n\n")
	if len(in.Members) == 0 {
		b.WriteString("The squad has no members that can take a task yet. Write the roles the procedure needs.\n")
	}
	for _, m := range in.Members {
		if m.Lead {
			fmt.Fprintf(&b, "- `%s`: %s (lead; plans the request and takes no task, use the built-in role `lead` for a step the lead decides)\n", m.Key, m.Name)
			continue
		}
		skills := "no skills"
		if len(m.Skills) > 0 {
			shown := m.Skills
			more := ""
			if len(shown) > maxBriefSkills {
				shown, more = shown[:maxBriefSkills], ", ..."
			}
			skills = "skills: " + strings.Join(shown, ", ") + more
		}
		fmt.Fprintf(&b, "- `%s`: %s (%s)\n", m.Key, m.Name, skills)
	}

	b.WriteString("\n## What to read\n\n")
	if len(in.References) == 0 {
		b.WriteString("No file is attached.")
	} else {
		b.WriteString("Attached to this issue:\n")
		for _, name := range in.References {
			fmt.Fprintf(&b, "- %s\n", name)
		}
	}
	if note := strings.TrimSpace(in.Note); note != "" {
		b.WriteString("\n\nNote from the person who asked:\n\n")
		for _, line := range strings.Split(note, "\n") {
			b.WriteString("> " + strings.TrimRight(line, "\r") + "\n")
		}
	} else {
		b.WriteString("\n")
	}

	b.WriteString("\n## What to deliver\n\n")
	b.WriteString("One file named `flow.json` (or ending in `flow.json`), attached to this issue:\n\n")
	b.WriteString("- a manifest, `{\"format\": \"multica.flow-manifest/v1\", \"name\": \"...\", \"doc\": {...}}`, whose `doc` is a flow document of schema 2;\n")
	b.WriteString("- with no `bind` on any role;\n")
	fmt.Fprintf(&b, "- checked with `multica flow validate --file flow.json --squad %s` until it prints no errors. Warnings may remain; say which in your comment.\n\n", in.SquadID)
	b.WriteString("Attach it in a comment: `multica issue comment add <this issue> --content \"...\" --attachment flow.json`. " +
		"Attach a new file after each change; the newest one you attached is read.\n")
	return b.String()
}

// ImportFlowProposal reads the file an agent attached for a proposal draft and makes it the
// document of the draft (spec 16.4). The file is checked as an imported manifest is.
func (h *Handler) ImportFlowProposal(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	if !featureflags.FlowPacksEnabled(ctx, h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled")
		return
	}
	a, ok := h.authorFlowManager(w, r, "flow_forbidden")
	if !ok {
		return
	}
	if !featureflags.FlowGraphV2Enabled(ctx, h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_graph_v2_disabled", "writing a flow document of schema 2 is not enabled")
		return
	}
	var req struct {
		Revision *int32 `json:"revision"`
	}
	if ok, _ := readFlowBody(w, r, &req); !ok {
		return
	}
	if req.Revision == nil {
		writeErrorCode(w, http.StatusBadRequest, "flow_draft_revision_required", "revision is required")
		return
	}
	draft, ok := h.loadFlowDraft(w, r, a)
	if !ok {
		return
	}
	if draft.Origin != "proposal" {
		writeErrorCode(w, http.StatusBadRequest, "flow_draft_not_proposal", "only a draft made from a proposal has a file to import")
		return
	}
	if draft.Revision != *req.Revision {
		flowRejectJSON(w, http.StatusConflict, "flow_draft_conflict", "the draft was saved by someone else: read it again", map[string]any{"revision": draft.Revision})
		return
	}

	att, found, err := h.latestProposalFile(ctx, a, draft)
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	if !found {
		writeErrorCode(w, http.StatusNotFound, "flow_proposal_missing", "the agent has not attached a flow.json to the issue yet")
		return
	}
	raw, tooLarge, err := h.readProposalFile(ctx, att)
	if err != nil {
		slog.Warn("read proposal file failed", append(logger.RequestAttrs(r), "attachment_id", uuidToString(att.ID), "error", err)...)
		writeErrorCode(w, http.StatusNotFound, "flow_proposal_missing", "the file the agent attached cannot be read")
		return
	}
	if tooLarge {
		writeFlowManifestInvalid(w, flowManifestIssue("a manifest is at most %d bytes", issueworkflow.DocMaxBytes))
		return
	}
	doc, _, issues := parseFlowManifest(raw)
	if len(issues) > 0 {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "the manifest is not valid", "code": "flow_manifest_invalid", "issues": issues})
		return
	}

	updated, err := h.Queries.UpdateFlowDraft(ctx, db.UpdateFlowDraftParams{
		ID: draft.ID, WorkspaceID: a.wsUUID, SquadID: a.squad.ID, Revision: *req.Revision, ActorID: a.member.UserID, Doc: doc,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		revision := draft.Revision
		if current, getErr := h.Queries.GetFlowDraft(ctx, db.GetFlowDraftParams{ID: draft.ID, WorkspaceID: a.wsUUID, SquadID: a.squad.ID}); getErr == nil {
			revision = current.Revision
		}
		flowRejectJSON(w, http.StatusConflict, "flow_draft_conflict", "the draft was saved by someone else: read it again", map[string]any{"revision": revision})
		return
	}
	if err != nil {
		writeFlowError(w, r, err)
		return
	}
	h.publishFlowDraftUpdated(a, updated.ID)
	writeJSON(w, http.StatusOK, flowDraftToResponse(updated))
}

// latestProposalFile is the newest file whose name ends with flow.json that the drafting agent
// attached to the issue of the proposal. A file of anybody else, or of the same agent on another
// issue, is never read.
func (h *Handler) latestProposalFile(ctx context.Context, a flowAuthoring, draft db.IssueWorkflowDraft) (db.Attachment, bool, error) {
	var ref proposalSourceRef
	if err := json.Unmarshal(draft.SourceRef, &ref); err != nil {
		return db.Attachment{}, false, nil
	}
	issueID, err := parseFlowUUID(ref.IssueID)
	if err != nil || !issueID.Valid {
		return db.Attachment{}, false, nil
	}
	agentID, err := parseFlowUUID(ref.AgentID)
	if err != nil || !agentID.Valid {
		return db.Attachment{}, false, nil
	}
	atts, err := h.Queries.ListAttachmentsByIssue(ctx, db.ListAttachmentsByIssueParams{IssueID: issueID, WorkspaceID: a.wsUUID})
	if err != nil {
		return db.Attachment{}, false, err
	}
	var newest db.Attachment
	found := false
	for _, att := range atts {
		if att.UploaderType != "agent" || att.UploaderID != agentID || !strings.HasSuffix(att.Filename, flowManifestFileEnd) {
			continue
		}
		if !found || !att.CreatedAt.Time.Before(newest.CreatedAt.Time) {
			newest, found = att, true
		}
	}
	return newest, found, nil
}

// readProposalFile reads the bytes of an attachment, at most one manifest's worth.
func (h *Handler) readProposalFile(ctx context.Context, att db.Attachment) (raw []byte, tooLarge bool, err error) {
	limit := int64(issueworkflow.DocMaxBytes) + 4<<10
	if att.SizeBytes > limit {
		return nil, true, nil
	}
	if h.Storage == nil {
		return nil, false, errors.New("no storage")
	}
	reader, err := h.Storage.GetReader(ctx, h.Storage.KeyFromURL(att.Url))
	if err != nil {
		return nil, false, err
	}
	defer reader.Close()
	raw, err = io.ReadAll(io.LimitReader(reader, limit+1))
	if err != nil {
		return nil, false, err
	}
	if int64(len(raw)) > limit {
		return nil, true, nil
	}
	return raw, false, nil
}
