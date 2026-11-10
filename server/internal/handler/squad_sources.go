package handler

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// The Source Studio's intake (spec 7.5). A person adds a source or shares a post to a Sales Loop
// squad: the request is written to the knowledge base as a document of the matching kind, and an
// issue is assigned to the squad, which the leader routes (route E for a source, route F for a share).
// Nothing here runs a source or contacts anyone.

const (
	salesLoopTemplateKey = "sales_loop"
	// maxSalesIntakeInput bounds what a person can paste into one request.
	maxSalesIntakeInput = 4000
)

// salesIntakeRequest is the body of both intake endpoints.
type salesIntakeRequest struct {
	// Input is a link, a handle, a search query, a feed, or a description in words.
	Input string `json:"input"`
	// Lane is the lane key the intake belongs to, optional.
	Lane string `json:"lane"`
	// Text is the shared content, pasted, for a share.
	Text string `json:"text"`
}

// SalesSourceIntakeResponse is what an intake returns: the knowledge document and the issue.
type SalesSourceIntakeResponse struct {
	Document service.KnowledgeDocumentView `json:"document"`
	IssueID  string                        `json:"issue_id"`
}

// CreateSquadSource (POST /api/squads/{id}/sources) adds a source to a Sales Loop squad.
func (h *Handler) CreateSquadSource(w http.ResponseWriter, r *http.Request) {
	h.salesIntake(w, r, "source")
}

// CreateSquadShare (POST /api/squads/{id}/shares) adds a shared post to a Sales Loop squad.
func (h *Handler) CreateSquadShare(w http.ResponseWriter, r *http.Request) {
	h.salesIntake(w, r, "share")
}

func (h *Handler) salesIntake(w http.ResponseWriter, r *http.Request, kind string) {
	workspaceID := h.resolveWorkspaceID(r)
	userID, ok := requireUserID(w, r)
	if !ok {
		return
	}
	squadID, ok := parseUUIDOrBadRequest(w, chi.URLParam(r, "id"), "id")
	if !ok {
		return
	}
	var req salesIntakeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	input := strings.TrimSpace(req.Input)
	if kind == "share" && strings.TrimSpace(req.Text) != "" {
		input = strings.TrimSpace(input + "\n" + req.Text)
	}
	if input == "" {
		writeError(w, http.StatusBadRequest, "input is required")
		return
	}
	if len([]rune(input)) > maxSalesIntakeInput {
		writeError(w, http.StatusBadRequest, "input is longer than 4000 characters")
		return
	}
	wsUUID := parseUUID(workspaceID)
	squad, err := h.Queries.GetSquadInWorkspace(r.Context(), db.GetSquadInWorkspaceParams{ID: squadID, WorkspaceID: wsUUID})
	if err != nil {
		writeError(w, http.StatusNotFound, "squad not found")
		return
	}
	if !squad.TemplateKey.Valid || squad.TemplateKey.String != salesLoopTemplateKey {
		writeError(w, http.StatusBadRequest, "this squad is not a Sales Loop squad")
		return
	}

	sum := sha256.Sum256([]byte(squadID.String() + "\x00" + kind + "\x00" + input))
	key := "sales:" + kind + ":" + hex.EncodeToString(sum[:16])
	docKind := "sales_source"
	base := "sales"
	title := "Source: " + oneLine(input, 80)
	if kind == "share" {
		docKind = "sales_share"
		title = "Share: " + oneLine(input, 80)
	}
	doc, _, err := h.Knowledge.CreateDocument(r.Context(), wsUUID, h.knowledgeReadActor(r, workspaceID), service.KnowledgeDocumentInput{
		KB: base, Kind: docKind, Title: title, Content: salesIntakeDocument(kind, input, req.Lane),
		SourceType: "upload", Publish: true, IdempotencyKey: key,
	})
	if err != nil {
		writeKnowledgeError(w, r, workspaceID, err)
		return
	}

	issueTitle := "Thêm nguồn: " + oneLine(input, 80)
	if kind == "share" {
		issueTitle = "Chia sẻ: " + oneLine(input, 80)
	}
	description := "Knowledge document: " + doc.ID + "\n\n" + input
	res, err := h.IssueService.Create(r.Context(), service.IssueCreateParams{
		WorkspaceID:  wsUUID,
		Title:        issueTitle,
		Description:  pgtype.Text{String: description, Valid: true},
		Status:       "todo",
		Priority:     "none",
		AssigneeType: pgtype.Text{String: "squad", Valid: true},
		AssigneeID:   squadID,
		CreatorType:  "member",
		CreatorID:    parseUUID(userID),
	}, service.IssueCreateOpts{ActorID: userID})
	if err != nil {
		if errors.Is(err, service.ErrActiveDuplicate) {
			writeError(w, http.StatusConflict, "this source is already being handled")
			return
		}
		writeError(w, http.StatusInternalServerError, "could not create the request issue")
		return
	}
	writeJSON(w, http.StatusCreated, SalesSourceIntakeResponse{Document: doc, IssueID: util.UUIDToString(res.Issue.ID)})
}

// salesIntakeDocument is the body of a new source or share document: the Record the squad reads,
// and the headings its kind requires. The Scout fills the rest when it onboards the source.
func salesIntakeDocument(kind, input, lane string) string {
	if kind == "share" {
		return "# Shared post\n\n## Record\n\nplatform: \nstate: new\nintent: \nshared_by: person\nshared_at: \naccount: \n\n## Shared content\n\n" +
			input + "\n\n## What it says\n\nNot read yet.\n\n## Outcome\n\nNot processed yet.\n"
	}
	laneLine := ""
	if lane != "" {
		laneLine = "lane: " + lane + "\n"
	}
	return "# Source added by a person\n\n## Record\n\n" + laneLine + "status: proposed\nadded_by: user\nurl: " +
		firstLine(input) + "\n\n## What it gives\n\nAdded by a person. The Scout has not read it yet.\n\n## How to read it\n\nNot probed yet.\n\n" +
		"## Recipe\n\nNone yet: the Scout writes it on onboarding.\n\n## Terms and limits\n\nUnknown until the Scout reads the source.\n\n## Probe log\n\nNo attempt yet.\n"
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(s, "\n")
	return strings.TrimSpace(line)
}
