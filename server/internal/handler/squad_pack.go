package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/hub"
	"github.com/multica-ai/multica/server/internal/issuestatus"
	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/logger"
	obsmetrics "github.com/multica-ai/multica/server/internal/metrics"
	"github.com/multica-ai/multica/server/internal/pack"
	"github.com/multica-ai/multica/server/internal/service"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// Installing a pack of skills and agents (spec 16.1 to 16.3): preview reads a bundle and writes nothing to the
// database; commit makes the squad, the agents and the skills in one transaction. The permission rules are
// the ones of the routes that make each thing, called through the same functions: the member who imports
// needs a runtime they may use (insertAgentTx), may only put an agent in a squad that they may call
// (memberCanWireAgent), and a skill is imported the way the Hub does (createSkillWithFilesInTx).

// PackPreviewRequest is the JSON body of a preview; a zip upload is sent as multipart instead.
type PackPreviewRequest struct {
	Source struct {
		Kind   string `json:"kind"`
		URL    string `json:"url"`
		Ref    string `json:"ref,omitempty"`
		Subdir string `json:"subdir,omitempty"`
		SHA256 string `json:"sha256,omitempty"`
	} `json:"source"`
}

// PackPreviewAgent is an agent of the bundle as the person who imports it sees it.
type PackPreviewAgent struct {
	Key               string   `json:"key"`
	Name              string   `json:"name"`
	Description       string   `json:"description"`
	InstructionsBytes int      `json:"instructions_bytes"`
	Skills            []string `json:"skills"`
	// SkillsDeclared is false when the bundle does not say which skills the agent has: the person chooses.
	SkillsDeclared  bool    `json:"skills_declared"`
	ExistingAgentID *string `json:"existing_agent_id"`
}

// PackPreviewSkill is a skill of the bundle.
type PackPreviewSkill struct {
	Name            string  `json:"name"`
	Files           int     `json:"files"`
	ExistingSkillID *string `json:"existing_skill_id"`
	SameContent     bool    `json:"same_content"`
}

// PackPreviewFlow says whether the bundle carries a flow and what the check makes of it.
type PackPreviewFlow struct {
	Present    bool                    `json:"present"`
	Validation *FlowValidationResponse `json:"validation,omitempty"`
	// Issues are what the bundle's flow says about the roles of the bundle's agents.
	Issues []pack.Issue `json:"issues,omitempty"`
}

// PackPreviewReference is a markdown file that may hold a procedure.
type PackPreviewReference struct {
	Path  string `json:"path"`
	Bytes int    `json:"bytes"`
	Kind  string `json:"kind"`
}

// PackPreviewFinding is a finding of the security scan.
type PackPreviewFinding struct {
	Path     string `json:"path"`
	Severity string `json:"severity"`
	Rule     string `json:"rule"`
}

// PackPreviewResponse is what the person importing a bundle is shown (spec 16.2).
type PackPreviewResponse struct {
	Digest     string                 `json:"digest"`
	Format     string                 `json:"format"`
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Version    string                 `json:"version"`
	Agents     []PackPreviewAgent     `json:"agents"`
	Skills     []PackPreviewSkill     `json:"skills"`
	Leader     *string                `json:"leader"`
	Flow       PackPreviewFlow        `json:"flow"`
	References []PackPreviewReference `json:"references"`
	// ReferencesOmitted is how many doc files (README, docs/, workflows/) the list of references left out. The
	// instructions of an agent and a SKILL.md are never left out.
	ReferencesOmitted int                  `json:"references_omitted"`
	Ignored           []pack.Ignored       `json:"ignored"`
	Scan              []PackPreviewFinding `json:"scan"`
	Issues            []pack.Issue         `json:"issues"`
}

// PackAgentChoice is what the person decided for one agent of the bundle: create, use_existing or skip.
type PackAgentChoice struct {
	Key             string `json:"key"`
	Action          string `json:"action"`
	ExistingAgentID string `json:"existing_agent_id,omitempty"`
	RuntimeID       string `json:"runtime_id,omitempty"`
	// Name overrides the name of an agent that is created.
	Name string `json:"name,omitempty"`
	// Skills overrides the skills of an agent that is created; absent means the ones the bundle declares.
	Skills *[]string `json:"skills,omitempty"`
}

// PackExtraAgent is an agent the person adds to the squad that the bundle does not have.
type PackExtraAgent struct {
	Key          string   `json:"key"`
	Name         string   `json:"name"`
	Instructions string   `json:"instructions"`
	RuntimeID    string   `json:"runtime_id"`
	Skills       []string `json:"skills"`
}

// PackLeaderChoice is the lead of the squad: an agent of the bundle (or an extra one), or a generic lead made
// from the catalog.
type PackLeaderChoice struct {
	Agent     string `json:"agent,omitempty"`
	Generic   bool   `json:"generic,omitempty"`
	RuntimeID string `json:"runtime_id,omitempty"`
}

// PackCommitRequest is what the person decided (spec 16.3).
type PackCommitRequest struct {
	Digest string `json:"digest"`
	Squad  *struct {
		Name string `json:"name"`
	} `json:"squad"`
	Agents      []PackAgentChoice `json:"agents"`
	ExtraAgents []PackExtraAgent  `json:"extra_agents"`
	Leader      *PackLeaderChoice `json:"leader"`
	// Skills says, for a skill whose name is taken, keep_existing or import_as_new.
	Skills              map[string]string `json:"skills"`
	CreateDraftFromFlow bool              `json:"create_draft_from_flow"`
	AckScanWarnings     bool              `json:"ack_scan_warnings"`
	// NewInstall installs a bundle again, or another version of a pack already installed, as a new squad.
	NewInstall bool `json:"new_install"`
}

// PackCommitAgent and PackCommitSkill say what an install made or took.
type PackCommitAgent struct {
	Key     string `json:"key"`
	AgentID string `json:"agent_id"`
	Created bool   `json:"created"`
}

type PackCommitSkill struct {
	Name    string `json:"name"`
	SkillID string `json:"skill_id"`
	Created bool   `json:"created"`
}

// PackCommitResponse is the result of an install; AlreadyInstalled is true when the same bundle was
// installed before and nothing was written.
type PackCommitResponse struct {
	InstallID        string            `json:"install_id"`
	AlreadyInstalled bool              `json:"already_installed"`
	SquadID          *string           `json:"squad_id"`
	Agents           []PackCommitAgent `json:"agents"`
	Skills           []PackCommitSkill `json:"skills"`
	DraftID          *string           `json:"draft_id"`
	// FlowIssues are the errors that kept the flow of the bundle from becoming a draft.
	FlowIssues []issueworkflow.Issue `json:"flow_issues,omitempty"`
}

// packCommitFailpoint lets a test make the install fail at a step ("skill" or "agent", n); it is nil in the server.
var packCommitFailpoint func(stage string, n int) error

const (
	packStagingPrefix   = "pack-"
	packMaxStagedBytes  = 16 << 20
	packCommitBodyLimit = 1 << 20
	// packSeverityWarning is a warning of the flow validator; pack has only errors and info.
	packSeverityWarning = "warning"
)

var (
	packDigestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	packExtraKeyPattern = regexp.MustCompile(`^[a-z][a-z0-9_]{1,31}$`)
)

// packSource says where a bundle came from; the install records it.
type packSource struct {
	Type string `json:"type"`
	URL  string `json:"url,omitempty"`
	Ref  string `json:"ref,omitempty"`
}

// packStaged is a previewed bundle kept in the Hub's staging storage until it is committed or expires.
// The files are kept as they are, so the bundle read at commit is the one that was shown.
type packStaged struct {
	WorkspaceID string          `json:"workspace_id"`
	UserID      string          `json:"user_id"`
	Source      packSource      `json:"source"`
	ExpiresAt   time.Time       `json:"expires_at"`
	Files       []hub.FileEntry `json:"files"`
}

// packCaller runs the checks both routes share: the flag, a member of the workspace, and not an agent.
func (h *Handler) packCaller(w http.ResponseWriter, r *http.Request) (workspaceID string, ws pgtype.UUID, userID string, member db.Member, ok bool) {
	if !featureflags.FlowPacksEnabled(r.Context(), h.FeatureFlags) {
		writeErrorCode(w, http.StatusNotFound, "flow_authoring_disabled", "installing a pack is not enabled")
		return
	}
	workspaceID = h.resolveWorkspaceID(r)
	ws, parsed := parseUUIDOrBadRequest(w, workspaceID, "workspace_id")
	if !parsed {
		return
	}
	userID, parsed = requireUserID(w, r)
	if !parsed {
		return
	}
	member, parsed = h.workspaceMember(w, r, workspaceID)
	if !parsed {
		return
	}
	if actorType, _ := h.resolveActor(r, userID, workspaceID); actorType == "agent" {
		writeErrorCode(w, http.StatusForbidden, "pack_member_only", "only a member of the workspace can install a pack")
		return
	}
	return workspaceID, ws, userID, member, true
}

func packDigestHex(digest string) (string, bool) {
	if !packDigestPattern.MatchString(digest) {
		return "", false
	}
	return strings.TrimPrefix(digest, "sha256:"), true
}

// packBundle reads the bundle of a preview: an uploaded zip, or a source the Hub knows how to fetch.
func (h *Handler) packBundle(w http.ResponseWriter, r *http.Request) (*hub.Bundle, packSource, bool) {
	if isMultipartForm(r) {
		bundle, ok := h.hubUploadBundle(w, r)
		return bundle, packSource{Type: string(hub.SourceUpload)}, ok
	}
	r.Body = http.MaxBytesReader(w, r.Body, hubImportBodyLimit)
	var req PackPreviewRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return nil, packSource{}, false
	}
	var kind hub.SourceType
	switch req.Source.Kind {
	case "github":
		kind = hub.SourceGitHub
	case "url_zip":
		kind = hub.SourceURLZip
	default:
		writeError(w, http.StatusBadRequest, "source.kind must be github or url_zip")
		return nil, packSource{}, false
	}
	url := strings.TrimSpace(req.Source.URL)
	if url == "" || len(url) > hubImportMaxURLLength {
		writeError(w, http.StatusBadRequest, "source.url is required and at most 2048 characters")
		return nil, packSource{}, false
	}
	ctx, cancel := context.WithTimeout(r.Context(), hubImportFetchTimeout)
	defer cancel()
	ref := hub.SourceRef{Type: kind, URL: url, Ref: req.Source.Ref, Subdir: req.Source.Subdir, SHA256: req.Source.SHA256}
	importer, ref, err := hubImporterFor(ref)
	if errors.Is(err, errHubURLMismatch) {
		writeError(w, http.StatusBadRequest, "url does not match the source type")
		return nil, packSource{}, false
	}
	if err != nil {
		writeError(w, http.StatusBadRequest, "unsupported source type")
		return nil, packSource{}, false
	}
	bundle, meta, err := importer.Fetch(ctx, hubImportFetcher, ref)
	if err != nil {
		status, msg := hubImportFailure(ctx, err)
		writeError(w, status, msg)
		return nil, packSource{}, false
	}
	return bundle, packSource{Type: string(kind), URL: ref.URL, Ref: meta.SourceRef}, true
}

// packRead is a bundle read into a pack, with every problem found in it.
type packRead struct {
	bundle *hub.Bundle
	pack   *pack.Pack
	// issues are the problems of the pack and of the roles of its flow.
	issues []pack.Issue
	// flowDoc is the document of the bundle's flow without any binding, nil when it has none or cannot be read.
	flowDoc  []byte
	flowName string
	scan     hub.ScanResult
	digest   string
}

func (p *packRead) hasErrors() bool {
	for _, i := range p.issues {
		if i.Severity == pack.SeverityError {
			return true
		}
	}
	return false
}

// readPack recognises, reads and checks a bundle. A bundle whose format cannot be told, or whose manifest cannot
// be read, comes back as an error that wraps pack.ErrInvalid; every other problem is an issue of the result.
func readPack(bundle *hub.Bundle) (*packRead, error) {
	p, err := pack.Normalize(bundle)
	if err != nil {
		return nil, err
	}
	read := &packRead{bundle: bundle, pack: p, issues: pack.Validate(bundle, p), scan: hub.Scan(bundle), digest: hub.Digest(bundle)}
	read.readFlow()
	return read, nil
}

// readFlow reads the flow of the bundle and checks that its roles name agents of the bundle: a role is matched
// to an agent by the key the bundle declares (spec 16.3), and the lead of the squad is not a role.
func (p *packRead) readFlow() {
	if p.pack.Flow == "" {
		return
	}
	var raw []byte
	for _, f := range p.bundle.Files {
		if f.Path == p.pack.Flow {
			raw = f.Content
		}
	}
	if raw == nil {
		return // Validate said so
	}
	doc, name, issues := parseFlowManifest(raw)
	if len(issues) > 0 {
		p.issues = append(p.issues, pack.Issue{Code: "pack_flow_invalid", Severity: pack.SeverityError, Path: "flow", Message: issues[0].Message})
		return
	}
	parsed, parseIssues := issueworkflow.ParseFlowDoc(doc)
	if len(parseIssues) > 0 {
		p.issues = append(p.issues, pack.Issue{Code: "pack_flow_invalid", Severity: pack.SeverityError, Path: "flow", Message: parseIssues[0].Message})
		return
	}
	p.flowDoc, p.flowName = doc, name
	keys := map[string]bool{}
	for _, a := range p.pack.Agents {
		keys[a.Key] = true
	}
	for _, role := range parsed.Roles {
		if role.Kind != issueworkflow.RoleKindAgent {
			continue
		}
		at := "flow.roles[" + role.Key + "]"
		switch {
		case p.pack.Leader != "" && role.Key == p.pack.Leader:
			p.issues = append(p.issues, pack.Issue{Code: "pack_flow_role_leader", Severity: pack.SeverityError, Path: at,
				Message: "the lead of the squad plans and takes no task, so it is not a role of the flow"})
		case !keys[role.Key]:
			p.issues = append(p.issues, pack.Issue{Code: "pack_flow_role_unmatched", Severity: packSeverityWarning, Path: at,
				Message: "no agent of the bundle has this key, so the role stays unbound"})
		}
	}
}

// flowIssues are the issues of the roles of the flow, for the preview.
func (p *packRead) flowIssues() []pack.Issue {
	var out []pack.Issue
	for _, i := range p.issues {
		if strings.HasPrefix(i.Code, "pack_flow_") {
			out = append(out, i)
		}
	}
	return out
}

// packStatuses is the status catalog a preview checks a flow against. It reads and never seeds: a workspace
// that has no statuses yet is checked without them.
func (h *Handler) packStatuses(ctx context.Context, ws pgtype.UUID) (issueworkflow.StatusCatalog, error) {
	rows, err := h.Queries.ListIssueStatusEntries(ctx, db.ListIssueStatusEntriesParams{WorkspaceID: ws, IncludeArchived: true})
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	out := issueworkflow.StatusCatalog{}
	for _, row := range rows {
		category, _ := issuestatus.ParseCategory(row.Category)
		out[row.Key] = issueworkflow.StatusInfo{Category: category, Archived: row.ArchivedAt.Valid}
	}
	return out, nil
}

// PreviewSquadPack reads a bundle and shows what installing it would do. It writes nothing to the database: the
// bundle is kept in the Hub's staging storage until it is committed or expires.
func (h *Handler) PreviewSquadPack(w http.ResponseWriter, r *http.Request) {
	workspaceID, ws, userID, member, ok := h.packCaller(w, r)
	if !ok {
		return
	}
	if h.HubStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}
	ctx := r.Context()
	bundle, source, ok := h.packBundle(w, r)
	if !ok {
		return
	}
	read, err := readPack(bundle)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "pack_invalid", err.Error())
		return
	}
	resp, err := h.packPreviewResponse(ctx, ws, member, read)
	if err != nil {
		slog.Warn("pack preview failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to read the pack")
		return
	}
	if err := h.packStage(ctx, workspaceID, userID, read, source); err != nil {
		slog.Warn("pack staging failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to stage the pack")
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *Handler) packPreviewResponse(ctx context.Context, ws pgtype.UUID, member db.Member, read *packRead) (PackPreviewResponse, error) {
	p := read.pack
	resp := PackPreviewResponse{
		Digest: "sha256:" + read.digest, Format: string(p.Format), ID: p.ID, Name: p.Name, Version: p.Version,
		Agents: []PackPreviewAgent{}, Skills: []PackPreviewSkill{}, References: []PackPreviewReference{},
		Ignored: p.Ignored, Scan: []PackPreviewFinding{}, Issues: read.issues,
	}
	if resp.Ignored == nil {
		resp.Ignored = []pack.Ignored{}
	}
	if resp.Issues == nil {
		resp.Issues = []pack.Issue{}
	}
	if p.Leader != "" {
		leader := p.Leader
		resp.Leader = &leader
	}
	for _, a := range p.Agents {
		out := PackPreviewAgent{Key: a.Key, Name: a.Name, Description: a.Description, InstructionsBytes: len(a.Instructions), Skills: a.Skills, SkillsDeclared: a.SkillsDeclared}
		if out.Skills == nil {
			out.Skills = []string{}
		}
		existing, err := h.Queries.GetAgentByNameInWorkspace(ctx, db.GetAgentByNameInWorkspaceParams{WorkspaceID: ws, Name: a.Name})
		switch {
		case err == nil && !existing.ArchivedAt.Valid:
			id := uuidToString(existing.ID)
			out.ExistingAgentID = &id
		case err != nil && !errors.Is(err, pgx.ErrNoRows):
			return resp, err
		}
		resp.Agents = append(resp.Agents, out)
	}
	for _, s := range p.Skills {
		out := PackPreviewSkill{Name: s.Name, Files: len(s.Files)}
		existing, err := h.Queries.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{WorkspaceID: ws, Name: s.Name})
		switch {
		case err == nil:
			id := uuidToString(existing.ID)
			out.ExistingSkillID = &id
			content, _ := hubSkillParts(read.bundle, skillMDPath(read.bundle, s))
			out.SameContent = existing.Content == content
		case !errors.Is(err, pgx.ErrNoRows):
			return resp, err
		}
		resp.Skills = append(resp.Skills, out)
	}
	for _, ref := range p.References {
		resp.References = append(resp.References, PackPreviewReference{Path: ref.Path, Bytes: ref.Bytes, Kind: ref.Kind})
	}
	resp.ReferencesOmitted = p.ReferencesOmitted
	for _, f := range read.scan.Findings {
		resp.Scan = append(resp.Scan, PackPreviewFinding{Path: f.Path, Severity: string(f.Severity), Rule: f.Rule})
	}
	resp.Flow = PackPreviewFlow{Present: p.Flow != "", Issues: read.flowIssues()}
	if read.flowDoc != nil {
		statuses, err := h.packStatuses(ctx, ws)
		if err != nil {
			return resp, err
		}
		members, err := h.Queries.ListMembers(ctx, ws)
		if err != nil {
			return resp, err
		}
		workspaceMembers := make(map[string]bool, len(members))
		for _, m := range members {
			workspaceMembers[uuidToString(m.UserID)] = true
		}
		validation := newFlowValidation(0, read.flowDoc, issueworkflow.ValidateContext{
			Statuses: statuses, WorkspaceMembers: workspaceMembers, CanCreateStatuses: isWorkspaceAdmin(member),
		})
		resp.Flow.Validation = &validation
	}
	return resp, nil
}

// skillMDPath is the path of the SKILL.md of a skill of the bundle.
func skillMDPath(b *hub.Bundle, s pack.Skill) string {
	for _, p := range s.Files {
		if strings.EqualFold(p[strings.LastIndex(p, "/")+1:], "SKILL.md") {
			return p
		}
	}
	return ""
}

func (h *Handler) packStage(ctx context.Context, workspaceID, userID string, read *packRead, source packSource) error {
	data, err := json.Marshal(packStaged{
		WorkspaceID: workspaceID, UserID: userID, Source: source,
		ExpiresAt: time.Now().Add(hubPreviewTTL), Files: read.bundle.Files,
	})
	if err != nil {
		return err
	}
	_, err = h.HubStorage.Upload(ctx, hubStagingMetaKey(workspaceID, packStagingPrefix+read.digest), data, hubImportStagingContentType, "")
	return err
}

// loadPackStaged reads a previewed bundle back. A bundle that is gone, expired or another person's is "not found".
func (h *Handler) loadPackStaged(ctx context.Context, workspaceID, userID, hexDigest string) (*hub.Bundle, packSource, bool, error) {
	rc, err := h.HubStorage.GetReader(ctx, hubStagingMetaKey(workspaceID, packStagingPrefix+hexDigest))
	if err != nil {
		var noSuchKey *types.NoSuchKey
		if errors.Is(err, os.ErrNotExist) || errors.As(err, &noSuchKey) {
			return nil, packSource{}, false, nil
		}
		return nil, packSource{}, false, err
	}
	defer rc.Close()
	var staged packStaged
	if err := json.NewDecoder(io.LimitReader(rc, packMaxStagedBytes)).Decode(&staged); err != nil {
		return nil, packSource{}, false, err
	}
	if staged.WorkspaceID != workspaceID || staged.UserID != userID || !time.Now().Before(staged.ExpiresAt) {
		return nil, packSource{}, false, nil
	}
	bundle := &hub.Bundle{Files: staged.Files}
	for _, f := range bundle.Files {
		bundle.TotalSize += int64(len(f.Content))
	}
	return bundle, staged.Source, true, nil
}

// packPlan is what a commit will do, checked before anything is written.
type packPlan struct {
	read      *packRead
	squadName string
	makeSquad bool
	// leader is the key of the lead, one of agents.
	leader string
	skills []plannedSkill
	agents []plannedAgent
	draft  bool
}

type plannedSkill struct {
	skill pack.Skill
	// name is the name the skill is imported under; existing is set when the person keeps the skill that was there.
	name     string
	existing *db.Skill
}

type plannedAgent struct {
	key          string
	name         string
	description  string
	instructions string
	roleLabel    string
	maxTasks     int
	skillNames   []string
	// existing is set for an agent the person chose to use as it is; otherwise the agent is created on runtime.
	existing *db.Agent
	runtime  db.AgentRuntime
}

// packRefusal is a refusal of a commit, with the status and the body the API answers.
type packRefusal struct {
	status int
	body   map[string]any
}

func refusal(status int, code, message string, extra map[string]any) *packRefusal {
	body := map[string]any{"error": message, "code": code}
	for k, v := range extra {
		body[k] = v
	}
	return &packRefusal{status: status, body: body}
}

func choiceInvalid(field, message string) *packRefusal {
	return refusal(http.StatusBadRequest, "pack_choice_invalid", message, map[string]any{"field": field})
}

func (h *Handler) writeRefusal(w http.ResponseWriter, f *packRefusal) {
	writeJSON(w, f.status, f.body)
}

// CommitSquadPack installs a previewed bundle: its skills, its agents, a squad with a lead, and the install
// record, in one transaction. It never overwrites an agent or a skill that exists, and it never applies the
// settings of the bundle (tools, model, environment, MCP).
func (h *Handler) CommitSquadPack(w http.ResponseWriter, r *http.Request) {
	workspaceID, ws, userID, member, ok := h.packCaller(w, r)
	if !ok {
		return
	}
	if h.HubStorage == nil {
		writeError(w, http.StatusServiceUnavailable, "file storage is not configured")
		return
	}
	ctx := r.Context()
	r.Body = http.MaxBytesReader(w, r.Body, packCommitBodyLimit)
	var req PackCommitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	hexDigest, ok := packDigestHex(req.Digest)
	if !ok {
		h.writeRefusal(w, choiceInvalid("digest", "digest is sha256: and 64 hexadecimal characters"))
		return
	}
	bundle, source, found, err := h.loadPackStaged(ctx, workspaceID, userID, hexDigest)
	if err != nil {
		slog.Warn("pack staging read failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to load the preview")
		return
	}
	if !found {
		writeErrorCode(w, http.StatusNotFound, "pack_preview_not_found", "the preview is gone or expired; preview the pack again")
		return
	}
	read, err := readPack(bundle)
	if err != nil {
		writeErrorCode(w, http.StatusBadRequest, "pack_invalid", err.Error())
		return
	}
	if read.digest != hexDigest {
		writeError(w, http.StatusConflict, "the preview no longer matches its bundle; preview the pack again")
		return
	}
	if read.hasErrors() {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "the pack has errors", "code": "pack_invalid", "issues": read.issues})
		return
	}
	findings := make([]PackPreviewFinding, 0, len(read.scan.Findings))
	for _, f := range read.scan.Findings {
		findings = append(findings, PackPreviewFinding{Path: f.Path, Severity: string(f.Severity), Rule: f.Rule})
	}
	switch {
	case read.scan.Status == hub.ScanBlocked:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "the pack failed the security scan and cannot be installed", "code": "pack_scan_blocked", "findings": findings})
		return
	case read.scan.Status == hub.ScanWarn && !req.AckScanWarnings:
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "the pack has scan warnings; acknowledge them to install", "code": "pack_scan_ack_required", "findings": findings})
		return
	}
	if !req.NewInstall {
		done, refused, err := h.packPreviousInstall(ctx, ws, read)
		if err != nil {
			writeError(w, http.StatusInternalServerError, "failed to check for an earlier install")
			return
		}
		if refused != nil {
			h.writeRefusal(w, refused)
			return
		}
		if done != nil {
			writeJSON(w, http.StatusOK, done)
			return
		}
	}
	plan, refused, err := h.planPackInstall(r, ws, member, read, req)
	if err != nil {
		slog.Warn("pack plan failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to plan the install")
		return
	}
	if refused != nil {
		h.writeRefusal(w, refused)
		return
	}
	h.installPack(w, r, installPackInput{
		workspaceID: workspaceID, ws: ws, userID: userID, member: member, plan: plan, source: source, hexDigest: hexDigest,
	})
}

// packPreviousInstall answers a bundle that was installed before: the same bundle gives the result it had,
// and another version of a pack that says its id is a conflict.
func (h *Handler) packPreviousInstall(ctx context.Context, ws pgtype.UUID, read *packRead) (*PackCommitResponse, *packRefusal, error) {
	same, err := h.Queries.GetSquadPackInstallByDigest(ctx, db.GetSquadPackInstallByDigestParams{WorkspaceID: ws, Digest: "sha256:" + read.digest})
	if err == nil {
		resp := packInstallResponse(same)
		resp.AlreadyInstalled = true
		return &resp, nil, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, err
	}
	if read.pack.ID == "" {
		return nil, nil, nil
	}
	earlier, err := h.Queries.GetLatestSquadPackInstallByPackID(ctx, db.GetLatestSquadPackInstallByPackIDParams{WorkspaceID: ws, PackID: read.pack.ID})
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil, nil
	}
	if err != nil {
		return nil, nil, err
	}
	previous := map[string]any{
		"install_id": uuidToString(earlier.ID), "version": earlier.PackVersion, "digest": earlier.Digest,
		"installed_at": earlier.InstalledAt.Time.UTC().Format(time.RFC3339),
	}
	if earlier.SquadID.Valid {
		previous["squad_id"] = uuidToString(earlier.SquadID)
	} else {
		previous["squad_id"] = nil
	}
	return nil, refusal(http.StatusConflict, "pack_conflict",
		"a pack with this id is installed already; keep it, or install this one as a new squad with new_install", map[string]any{"previous": previous}), nil
}

// packRoster is what an install records of what it made or took.
type packRoster struct {
	Agents        map[string]string `json:"agents"`
	Skills        map[string]string `json:"skills"`
	CreatedAgents []string          `json:"created_agents"`
	CreatedSkills []string          `json:"created_skills"`
	Leader        string            `json:"leader,omitempty"`
	DraftID       string            `json:"draft_id,omitempty"`
}

// packInstallResponse is the result of an install, read back from its record.
func packInstallResponse(row db.SquadPackInstall) PackCommitResponse {
	resp := PackCommitResponse{InstallID: uuidToString(row.ID), Agents: []PackCommitAgent{}, Skills: []PackCommitSkill{}}
	if row.SquadID.Valid {
		id := uuidToString(row.SquadID)
		resp.SquadID = &id
	}
	var roster packRoster
	_ = json.Unmarshal(row.Roster, &roster)
	created := func(list []string) map[string]bool {
		out := map[string]bool{}
		for _, k := range list {
			out[k] = true
		}
		return out
	}
	madeAgents, madeSkills := created(roster.CreatedAgents), created(roster.CreatedSkills)
	for _, key := range sortedKeys(roster.Agents) {
		resp.Agents = append(resp.Agents, PackCommitAgent{Key: key, AgentID: roster.Agents[key], Created: madeAgents[key]})
	}
	for _, name := range sortedKeys(roster.Skills) {
		resp.Skills = append(resp.Skills, PackCommitSkill{Name: name, SkillID: roster.Skills[name], Created: madeSkills[name]})
	}
	if roster.DraftID != "" {
		resp.DraftID = &roster.DraftID
	}
	return resp
}

// planPackInstall checks every choice of the person against the bundle and the workspace, and refuses with
// the first thing wrong. Nothing is written. The rules are those of the routes that make each thing.
func (h *Handler) planPackInstall(r *http.Request, ws pgtype.UUID, member db.Member, read *packRead, req PackCommitRequest) (*packPlan, *packRefusal, error) {
	ctx := r.Context()
	p := read.pack
	plan := &packPlan{read: read}

	// The squad: the bundle names its lead; for the other formats the person asks for a squad by choosing one.
	native := p.Format == pack.FormatNativeBundle
	leaderKey := ""
	switch {
	case req.Leader != nil && req.Leader.Generic:
		plan.makeSquad = true
	case req.Leader != nil && req.Leader.Agent != "":
		if native && req.Leader.Agent != p.Leader {
			return nil, choiceInvalid("leader.agent", "the lead of this bundle is "+p.Leader), nil
		}
		plan.makeSquad, leaderKey = true, req.Leader.Agent
	case req.Leader != nil:
		return nil, choiceInvalid("leader", "leader names an agent or asks for the generic lead"), nil
	case native:
		plan.makeSquad, leaderKey = true, p.Leader
	}
	if !plan.makeSquad && len(req.ExtraAgents) > 0 {
		return nil, choiceInvalid("leader", "agents added to a squad need a lead to make the squad"), nil
	}
	if plan.makeSquad {
		if req.Squad != nil {
			plan.squadName = strings.TrimSpace(req.Squad.Name)
		}
		if plan.squadName == "" {
			plan.squadName = p.Name
		}
		if plan.squadName == "" || utf8.RuneCountInString(plan.squadName) > 200 {
			return nil, choiceInvalid("squad.name", "a squad needs a name of 1 to 200 characters"), nil
		}
	}
	if req.CreateDraftFromFlow {
		switch {
		case p.Flow == "" || read.flowDoc == nil:
			return nil, choiceInvalid("create_draft_from_flow", "the bundle has no flow to make a draft from"), nil
		case !plan.makeSquad:
			return nil, choiceInvalid("create_draft_from_flow", "a draft belongs to a squad"), nil
		case !featureflags.FlowAuthoringEnabled(ctx, h.FeatureFlags):
			return nil, refusal(http.StatusNotFound, "flow_authoring_disabled", "flow authoring is not enabled", nil), nil
		case !featureflags.FlowGraphV2Enabled(ctx, h.FeatureFlags):
			return nil, refusal(http.StatusNotFound, "flow_graph_v2_disabled", "writing a flow document of schema 2 is not enabled", nil), nil
		}
		plan.draft = true
	}

	// Skills: a name that is taken is kept or imported as new, as the person says.
	taken := map[string]bool{}
	conflicts := map[string][]string{}
	for _, s := range p.Skills {
		switch choice := req.Skills[s.Name]; choice {
		case "", "keep_existing", "import_as_new":
		default:
			return nil, choiceInvalid("skills."+s.Name, "choose keep_existing or import_as_new"), nil
		}
		item := plannedSkill{skill: s, name: s.Name}
		existing, err := h.Queries.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{WorkspaceID: ws, Name: s.Name})
		switch {
		case err == nil:
			switch req.Skills[s.Name] {
			case "keep_existing":
				item.existing = &existing
			case "import_as_new":
				name, err := h.freeSkillName(ctx, ws, s.Name, taken)
				if err != nil {
					return nil, nil, err
				}
				item.name = name
			default:
				conflicts["skills"] = append(conflicts["skills"], s.Name)
			}
		case !errors.Is(err, pgx.ErrNoRows):
			return nil, nil, err
		}
		taken[item.name] = true
		plan.skills = append(plan.skills, item)
	}
	skillNames := map[string]bool{}
	for _, s := range p.Skills {
		skillNames[s.Name] = true
	}

	// Agents of the bundle.
	choices := map[string]PackAgentChoice{}
	for _, c := range req.Agents {
		if _, dup := choices[c.Key]; dup {
			return nil, choiceInvalid("agents."+c.Key, "an agent is chosen once"), nil
		}
		choices[c.Key] = c
	}
	known := map[string]bool{}
	for _, a := range p.Agents {
		known[a.Key] = true
	}
	for key := range choices {
		if !known[key] {
			return nil, choiceInvalid("agents."+key, "the bundle has no agent with this key"), nil
		}
	}
	usedNames := map[string]bool{}
	preflight := func(rt string, field string, skills int) (db.AgentRuntime, *packRefusal, error) {
		runtimeID, err := parseUUIDString(rt)
		if rt == "" || err != nil {
			return db.AgentRuntime{}, choiceInvalid(field, "runtime_id is the id of a runtime of this workspace"), nil
		}
		runtime, err := h.Queries.GetAgentRuntimeForWorkspace(ctx, db.GetAgentRuntimeForWorkspaceParams{ID: runtimeID, WorkspaceID: ws})
		if errors.Is(err, pgx.ErrNoRows) {
			return db.AgentRuntime{}, choiceInvalid(field, "invalid runtime_id"), nil
		}
		if err != nil {
			return db.AgentRuntime{}, nil, err
		}
		if !canUseRuntimeForAgent(member, runtime) {
			return db.AgentRuntime{}, refusal(http.StatusForbidden, "forbidden", "this runtime is private; ask its owner to grant you access before creating agents", nil), nil
		}
		if status, msg := h.checkAgentRuntimeSettings(ctx, runtime, CreateAgentRequest{SkillIDs: make([]string, skills)}); status != 0 {
			return db.AgentRuntime{}, refusal(status, "pack_choice_invalid", msg, map[string]any{"field": field}), nil
		}
		return runtime, nil, nil
	}
	nameFree := func(name string) (bool, error) {
		if usedNames[name] {
			return false, nil
		}
		usedNames[name] = true
		_, err := h.Queries.GetAgentByNameInWorkspace(ctx, db.GetAgentByNameInWorkspaceParams{WorkspaceID: ws, Name: name})
		if errors.Is(err, pgx.ErrNoRows) {
			return true, nil
		}
		return false, err
	}
	for _, a := range p.Agents {
		c := choices[a.Key]
		field := "agents." + a.Key
		switch c.Action {
		case "", "skip":
		case "use_existing":
			agent, refused, err := h.packExistingAgent(r, ws, member, c.ExistingAgentID, field)
			if err != nil || refused != nil {
				return nil, refused, err
			}
			plan.agents = append(plan.agents, plannedAgent{key: a.Key, name: agent.Name, roleLabel: a.RoleLabel, existing: &agent})
		case "create":
			names := a.Skills
			if c.Skills != nil {
				names = *c.Skills
			}
			for _, n := range names {
				if !skillNames[n] {
					return nil, choiceInvalid(field+".skills", "the bundle has no skill named "+n), nil
				}
			}
			runtime, refused, err := preflight(c.RuntimeID, field+".runtime_id", len(names))
			if err != nil || refused != nil {
				return nil, refused, err
			}
			name := firstNonEmptyString(strings.TrimSpace(c.Name), a.Name)
			if name == "" {
				return nil, choiceInvalid(field+".name", "an agent needs a name"), nil
			}
			free, err := nameFree(name)
			if err != nil {
				return nil, nil, err
			}
			if !free {
				conflicts["agents"] = append(conflicts["agents"], name)
			}
			plan.agents = append(plan.agents, plannedAgent{
				key: a.Key, name: name, description: a.Description, instructions: a.Instructions, roleLabel: a.RoleLabel,
				maxTasks: a.MaxConcurrentTasks, skillNames: names, runtime: runtime,
			})
		default:
			return nil, choiceInvalid(field+".action", "action is create, use_existing or skip"), nil
		}
	}
	// Agents the person adds.
	for i, e := range req.ExtraAgents {
		field := fmt.Sprintf("extra_agents[%d]", i)
		if !packExtraKeyPattern.MatchString(e.Key) || known[e.Key] {
			return nil, choiceInvalid(field+".key", "a key is 2 to 32 characters of a-z, 0-9 and _, starting with a letter, and not the key of an agent of the bundle"), nil
		}
		known[e.Key] = true
		name := strings.TrimSpace(e.Name)
		if name == "" {
			return nil, choiceInvalid(field+".name", "an agent needs a name"), nil
		}
		for _, n := range e.Skills {
			if !skillNames[n] {
				return nil, choiceInvalid(field+".skills", "the bundle has no skill named "+n), nil
			}
		}
		runtime, refused, err := preflight(e.RuntimeID, field+".runtime_id", len(e.Skills))
		if err != nil || refused != nil {
			return nil, refused, err
		}
		free, err := nameFree(name)
		if err != nil {
			return nil, nil, err
		}
		if !free {
			conflicts["agents"] = append(conflicts["agents"], name)
		}
		plan.agents = append(plan.agents, plannedAgent{key: e.Key, name: name, instructions: strings.TrimSpace(e.Instructions), skillNames: e.Skills, runtime: runtime})
	}
	// The generic lead: an agent made from the catalog template.
	if req.Leader != nil && req.Leader.Generic {
		runtime, refused, err := preflight(req.Leader.RuntimeID, "leader.runtime_id", 0)
		if err != nil || refused != nil {
			return nil, refused, err
		}
		instructions, err := service.AgentInstructionTemplate("flow_lead")
		if err != nil {
			return nil, nil, err
		}
		name := plan.squadName + " lead"
		free, err := nameFree(name)
		if err != nil {
			return nil, nil, err
		}
		if !free {
			conflicts["agents"] = append(conflicts["agents"], name)
		}
		leaderKey = "lead"
		for known[leaderKey] {
			leaderKey += "_"
		}
		plan.agents = append(plan.agents, plannedAgent{
			key: leaderKey, name: name, description: "Follows the reminders of the workflow of the squad", instructions: instructions, runtime: runtime,
		})
	}
	if len(conflicts) > 0 {
		extra := map[string]any{}
		for k, v := range conflicts {
			extra[k] = v
		}
		return nil, refusal(http.StatusConflict, "pack_name_conflict", "a name is taken: choose keep_existing or import_as_new for a skill, and use_existing or another name for an agent", extra), nil
	}
	if plan.makeSquad {
		found := false
		for _, a := range plan.agents {
			found = found || a.key == leaderKey
		}
		if !found {
			return nil, choiceInvalid("leader", "the lead has to be an agent that the install creates or uses"), nil
		}
		plan.leader = leaderKey
	}
	return plan, nil, nil
}

// packExistingAgent resolves an agent the person chose to use as it is. They may only put in a squad an agent
// they may call, as with the route that adds a member to a squad.
func (h *Handler) packExistingAgent(r *http.Request, ws pgtype.UUID, member db.Member, id, field string) (db.Agent, *packRefusal, error) {
	agentID, err := parseUUIDString(id)
	if id == "" || err != nil {
		return db.Agent{}, choiceInvalid(field+".existing_agent_id", "existing_agent_id is the id of an agent of this workspace"), nil
	}
	agent, err := h.Queries.GetAgentInWorkspace(r.Context(), db.GetAgentInWorkspaceParams{ID: agentID, WorkspaceID: ws})
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && agent.ArchivedAt.Valid) {
		return db.Agent{}, choiceInvalid(field+".existing_agent_id", "agent not found in this workspace"), nil
	}
	if err != nil {
		return db.Agent{}, nil, err
	}
	if !h.memberCanWireAgent(r.Context(), member, agent, uuidToString(ws)) {
		return db.Agent{}, refusal(http.StatusForbidden, "forbidden", "you can only use an agent you have access to", nil), nil
	}
	return agent, nil, nil
}

// freeSkillName is name-2, name-3, ... : the first that is neither a skill of the workspace nor taken by this install.
func (h *Handler) freeSkillName(ctx context.Context, ws pgtype.UUID, name string, taken map[string]bool) (string, error) {
	for n := 2; n < 1000; n++ {
		candidate := fmt.Sprintf("%s-%d", name, n)
		if taken[candidate] {
			continue
		}
		_, err := h.Queries.GetSkillByWorkspaceAndName(ctx, db.GetSkillByWorkspaceAndNameParams{WorkspaceID: ws, Name: candidate})
		if errors.Is(err, pgx.ErrNoRows) {
			return candidate, nil
		}
		if err != nil {
			return "", err
		}
	}
	return "", fmt.Errorf("no free name for skill %q", name)
}

func firstNonEmptyString(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}

type installPackInput struct {
	workspaceID string
	ws          pgtype.UUID
	userID      string
	member      db.Member
	plan        *packPlan
	source      packSource
	hexDigest   string
}

// installPack writes the plan in one transaction and announces what it made once it has committed.
func (h *Handler) installPack(w http.ResponseWriter, r *http.Request, in installPackInput) {
	ctx := r.Context()
	plan, read := in.plan, in.plan.read
	p := read.pack
	user := parseUUID(in.userID)
	isFirstAgent := false
	if existing, err := h.Queries.ListAgents(ctx, in.ws); err == nil {
		isFirstAgent = len(existing) == 0
	}

	tx, err := h.TxStarter.Begin(ctx)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "failed to start the install")
		return
	}
	defer tx.Rollback(ctx)
	qtx := h.Queries.WithTx(tx)
	fail := func(err error) {
		slog.Warn("pack install failed", append(logger.RequestAttrs(r), "error", err)...)
		writeError(w, http.StatusInternalServerError, "failed to install the pack")
	}
	installID := uuid.New()
	installUUID := pgtype.UUID{Bytes: installID, Valid: true}

	// Skills, the way the Hub makes one (spec 16.3), without a hub item: a skill of a pack has no remote to sync with.
	roster := packRoster{Agents: map[string]string{}, Skills: map[string]string{}, CreatedAgents: []string{}, CreatedSkills: []string{}}
	skillIDs := map[string]pgtype.UUID{}
	var createdSkills []SkillWithFilesResponse
	for i, item := range plan.skills {
		if item.existing != nil {
			skillIDs[item.skill.Name] = item.existing.ID
			roster.Skills[item.skill.Name] = uuidToString(item.existing.ID)
			continue
		}
		mdPath := skillMDPath(read.bundle, item.skill)
		content, files := hubSkillParts(read.bundle, mdPath)
		created, err := createSkillWithFilesInTx(ctx, qtx, skillCreateInput{
			VersionSource: skillVersionSourceImport, WorkspaceID: in.ws, CreatorID: user, Name: item.name,
			Description: item.skill.Description, Content: content, Files: files,
			Config: map[string]any{"origin": map[string]any{"type": "pack", "pack_id": p.ID, "digest": "sha256:" + read.digest}},
		})
		if err == nil && packCommitFailpoint != nil {
			err = packCommitFailpoint("skill", i+1)
		}
		if err != nil {
			if isUniqueViolation(err) {
				writeErrorCode(w, http.StatusConflict, "pack_name_conflict", "a skill with the name "+item.name+" was made while this install ran")
				return
			}
			fail(err)
			return
		}
		skillIDs[item.skill.Name] = parseUUID(created.ID)
		roster.Skills[item.skill.Name] = created.ID
		roster.CreatedSkills = append(roster.CreatedSkills, item.skill.Name)
		createdSkills = append(createdSkills, created)
	}

	// Agents. The rules of the create route run for each: the runtime the member may use, the limits, the skills.
	type madeAgent struct {
		planned plannedAgent
		agent   db.Agent
		runtime db.AgentRuntime
		created bool
	}
	var agents []madeAgent
	createdN := 0
	for _, pa := range plan.agents {
		if pa.existing != nil {
			agents = append(agents, madeAgent{planned: pa, agent: *pa.existing})
			roster.Agents[pa.key] = uuidToString(pa.existing.ID)
			continue
		}
		var skills []pgtype.UUID
		for _, n := range pa.skillNames {
			skills = append(skills, skillIDs[n])
		}
		insert, status, msg := h.packAgentInsert(w, r, in, pa, skills)
		if status != 0 {
			writeError(w, status, msg)
			return
		}
		created, runtime, failure := insertAgentTx(r, qtx, insert)
		if failure != nil {
			writeError(w, failure.Status, failure.Message)
			return
		}
		createdN++
		if packCommitFailpoint != nil {
			if err := packCommitFailpoint("agent", createdN); err != nil {
				fail(err)
				return
			}
		}
		agents = append(agents, madeAgent{planned: pa, agent: created, runtime: runtime, created: true})
		roster.Agents[pa.key] = uuidToString(created.ID)
		roster.CreatedAgents = append(roster.CreatedAgents, pa.key)
	}

	// The squad: the lead, then the others with the label of their role.
	var (
		squad         db.Squad
		seat          *companyAutoSeat
		changedAgents []db.Agent
		members       []pgtype.UUID
	)
	if plan.makeSquad {
		var leaderID pgtype.UUID
		for _, a := range agents {
			if a.planned.key == plan.leader {
				leaderID = a.agent.ID
			}
		}
		squad, seat, changedAgents, err = createSquadTx(ctx, qtx, newSquad{
			WorkspaceID: in.ws, CreatorID: in.member.UserID, Name: plan.squadName, Description: p.Name,
			LeaderID: leaderID, FloorName: "",
		})
		if err != nil {
			fail(err)
			return
		}
		roster.Leader = uuidToString(leaderID)
		for _, a := range agents {
			if a.planned.key == plan.leader {
				continue
			}
			if _, err := qtx.AddSquadMember(ctx, db.AddSquadMemberParams{SquadID: squad.ID, MemberType: "agent", MemberID: a.agent.ID, Role: a.planned.roleLabel}); err != nil {
				fail(err)
				return
			}
			homed, err := claimHomeSquad(ctx, qtx, in.ws, a.agent.ID, squad.ID)
			if err != nil {
				fail(err)
				return
			}
			if homed != nil {
				changedAgents = append(changedAgents, *homed)
			}
			members = append(members, a.agent.ID)
		}
	}

	// The flow of the bundle becomes a draft whose roles are bound by the key the bundle declares.
	var draftID pgtype.UUID
	var flowIssues []issueworkflow.Issue
	if plan.draft {
		authoring := flowAuthoring{member: in.member, squad: squad, workspaceID: in.workspaceID, wsUUID: in.ws}
		draft, issues, err := h.packDraft(ctx, qtx, authoring, plan, roster.Agents, installUUID, user)
		if err != nil {
			fail(err)
			return
		}
		flowIssues = issues
		if draft != nil {
			draftID = draft.ID
			roster.DraftID = uuidToString(draft.ID)
		}
	}

	rosterJSON, _ := json.Marshal(roster)
	sourceJSON, _ := json.Marshal(in.source)
	row, err := qtx.CreateSquadPackInstall(ctx, db.CreateSquadPackInstallParams{
		ID: installUUID, WorkspaceID: in.ws, SquadID: squad.ID, PackID: p.ID, PackVersion: p.Version, Format: string(p.Format),
		Digest: "sha256:" + read.digest, Source: sourceJSON, Roster: rosterJSON, InstalledBy: user,
	})
	if err != nil {
		fail(err)
		return
	}
	if err := tx.Commit(ctx); err != nil {
		fail(err)
		return
	}

	// After the commit: what the routes that make each thing announce.
	actorType, actorID := h.resolveActor(r, in.userID, in.workspaceID)
	for _, s := range createdSkills {
		h.publish(protocol.EventSkillCreated, in.workspaceID, actorType, actorID, map[string]any{"skill": s})
	}
	first := isFirstAgent
	for _, a := range agents {
		if a.created {
			h.announceCreatedAgent(r, a.agent, a.runtime, in.userID, in.workspaceID, "pack", first)
			first = false
		}
	}
	if plan.makeSquad {
		if resp, err := h.squadToResponseWithPreview(ctx, squad); err == nil {
			if seat != nil && seat.NewFloor {
				h.publish(protocol.EventCompanyFloorCreated, in.workspaceID, "member", in.userID, map[string]any{"floor": companyFloorToResponse(seat.Floor)})
			}
			h.publishAgentsChanged(in.workspaceID, in.userID, changedAgents)
			h.publish(protocol.EventSquadCreated, in.workspaceID, "member", in.userID, map[string]any{"squad": resp})
			obsmetrics.RecordEvent(h.Analytics, h.Metrics, analytics.SquadCreated(in.userID, in.workspaceID, uuidToString(squad.ID), 1))
		}
		for _, id := range members {
			h.recomputeKnowledgeAccess(r, in.ws, id)
		}
		if draftID.Valid {
			h.publishFlowDraftUpdated(flowAuthoring{member: in.member, squad: squad, workspaceID: in.workspaceID, wsUUID: in.ws}, draftID)
		}
	}

	resp := packInstallResponse(row)
	resp.FlowIssues = flowIssues
	writeJSON(w, http.StatusCreated, resp)
}

// packAgentInsert builds the agent a pack makes, with the defaults of an agent made by hand and nothing of the
// bundle's settings: no environment, no MCP, no model, no custom arguments. It returns a status and message when
// a field the create route checks is not valid.
func (h *Handler) packAgentInsert(w http.ResponseWriter, r *http.Request, in installPackInput, pa plannedAgent, skills []pgtype.UUID) (agentInsert, int, string) {
	rawFields := map[string]json.RawMessage{}
	maxTasks := int32(pa.maxTasks)
	if pa.maxTasks > 0 {
		rawFields["max_concurrent_tasks"] = json.RawMessage(fmt.Sprint(pa.maxTasks))
	}
	if err := defaultAndValidateAgentMaxConcurrentTasks(rawFields, &maxTasks); err != nil {
		return agentInsert{}, http.StatusBadRequest, err.Error()
	}
	if utf8.RuneCountInString(pa.description) > maxAgentDescriptionLength {
		return agentInsert{}, http.StatusBadRequest, fmt.Sprintf("description must be %d characters or fewer", maxAgentDescriptionLength)
	}
	visibility := "private"
	perm, _, err := parsePermissionInput(in.ws, nil, nil, false, false, &visibility)
	if err != nil {
		return agentInsert{}, http.StatusBadRequest, err.Error()
	}
	avatar, ok := h.newAgentAvatar(w, r, nil)
	if !ok {
		return agentInsert{}, http.StatusInternalServerError, "failed to make the avatar"
	}
	return agentInsert{
		WorkspaceID: in.ws, OwnerID: in.userID, Member: in.member, RuntimeID: pa.runtime.ID,
		Name: pa.name, Description: pa.description, Instructions: pa.instructions, AvatarURL: avatar,
		RuntimeConfig: []byte("{}"), CustomEnv: []byte("{}"), CustomArgs: []byte("[]"), ConversationStarters: []byte("[]"),
		Permission: perm, MaxConcurrentTasks: maxTasks, SkillIDs: skills,
	}, 0, ""
}

// packDraft makes the draft of the bundle's flow in the install's transaction. Its roles are bound by key to
// the agents the install made or took; a role the bundle does not give an agent stays unbound. A flow that does
// not check against the squad it would run in makes no draft, and its errors are returned.
func (h *Handler) packDraft(ctx context.Context, qtx *db.Queries, a flowAuthoring, plan *packPlan, agentIDs map[string]string, installID, user pgtype.UUID) (*db.IssueWorkflowDraft, []issueworkflow.Issue, error) {
	// The lead of the squad is not a role: a role keyed like it was refused when the bundle was read.
	bindable := make(map[string]string, len(agentIDs))
	for key, id := range agentIDs {
		if key != plan.leader {
			bindable[key] = id
		}
	}
	doc, err := bindPackRoles(plan.read.flowDoc, bindable, uuidToString(installID))
	if err != nil {
		return nil, nil, err
	}
	roster, err := h.squadRoster(ctx, qtx, a.squad, true)
	if err != nil {
		return nil, nil, err
	}
	vctx, err := h.flowValidateContextWith(ctx, qtx, a, roster, false)
	if err != nil {
		return nil, nil, err
	}
	issues := issueworkflow.ValidateFlowDocJSON(doc, vctx)
	if issueworkflow.HasErrors(issues) {
		return nil, issues, nil
	}
	name := firstNonEmptyString(plan.read.flowName, plan.read.pack.Name, plan.squadName)
	if runes := []rune(strings.TrimSpace(name)); len(runes) > issueworkflow.MaxDraftNameRunes {
		name = string(runes[:issueworkflow.MaxDraftNameRunes])
	}
	sourceRef, _ := json.Marshal(map[string]string{"pack_install_id": uuidToString(installID)})
	draft, err := qtx.CreateFlowDraft(ctx, db.CreateFlowDraftParams{
		WorkspaceID: a.wsUUID, SquadID: a.squad.ID, Name: name, Doc: doc, Origin: "pack", SourceRef: sourceRef, ActorID: user,
	})
	if err != nil {
		return nil, nil, err
	}
	return &draft, nil, nil
}

// bindPackRoles binds the agent roles of a flow document to agents by the key of the role, and says in the
// role's source which bundle and which key it came from (spec 2.2, "Role.source").
func bindPackRoles(doc []byte, agentIDs map[string]string, installID string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(doc, &m); err != nil {
		return nil, err
	}
	roles, _ := m["roles"].([]any)
	for _, raw := range roles {
		role, _ := raw.(map[string]any)
		key, _ := role["key"].(string)
		id, matched := agentIDs[key]
		if !matched || role["kind"] != issueworkflow.RoleKindAgent {
			continue
		}
		role["bind"] = map[string]any{"agent_ids": []string{id}}
		role["source"] = map[string]any{"bundle": installID, "agent_key": key}
	}
	return json.Marshal(m)
}
