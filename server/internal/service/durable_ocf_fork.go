package service

// Open Company Format, ocf/1 (durable E2). A package is a zip: manifest.json,
// structure.json (the company export, unchanged) and, when history is asked for,
// history/*.ndjson. The schema and the rules it cannot state are in
// docs/specs/durable-platform/ocf/; durable_ocf_v1.schema.json here is a copy of
// the docs schema, and a test keeps the two equal.

import (
	"archive/zip"
	"bytes"
	"context"
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/santhosh-tekuri/jsonschema/v6"

	"github.com/multica-ai/multica/server/internal/util"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

//go:embed durable_ocf_v1.schema.json
var ocfSchemaJSON []byte

// OCFFormat is the format a package declares, and the only one this reader takes.
const OCFFormat = "ocf/1"

// ocfGenerator names the writer in manifest.json. Its pattern is in the schema.
const ocfGenerator = "delegora/dev"

// Limits on a package read on import. A package larger than these is refused
// before it is parsed; the export of a large workspace stays under them.
const (
	ocfMaxPackageBytes = 64 << 20
	ocfMaxFileBytes    = 32 << 20
	// ocfMaxEntries and ocfMaxUncompressedBytes bound what a small, highly compressed package
	// can expand into: its total decompressed size, not only each file's.
	ocfMaxEntries           = 1000
	ocfMaxUncompressedBytes = 256 << 20
)

// ErrNotOCFPackage means the bytes are not a zip with a manifest.json. The
// caller then reads them as a legacy company export.
var ErrNotOCFPackage = errors.New("not an Open Company Format package: no manifest.json in a zip")

// Governance kinds an imported history row is written under. Each row keeps its
// source kind in its details, so the import is one kind per part, not per record.
const (
	ocfImportedOutcomeKind     = "ocf.imported.outcome"
	ocfImportedTrackRecordKind = "ocf.imported.track_record"
	ocfImportedGovernanceKind  = "ocf.imported.governance"
	ocfImportedDossierKind     = "ocf.imported.dossier"
	ocfHistoryImportedKind     = "ocf.history_imported"
)

// The history parts a package can carry, by name in manifest.json.
const (
	ocfPartStructure    = "structure"
	ocfPartOutcomes     = "history/outcomes"
	ocfPartTrackRecords = "history/track_records"
	ocfPartGovernance   = "history/governance"
	ocfPartDossiers     = "history/dossiers"
	ocfPartPrincipals   = "history/principals"
	ocfPartAttestations = "attestations"
)

// ocfPartFile maps a part to the file it is written to. attestations names a folder.
func ocfPartFile(part string) string {
	if part == ocfPartAttestations {
		return "attestations/"
	}
	if part == ocfPartStructure {
		return "structure.json"
	}
	return part + ".ndjson"
}

// ocfPartDef maps a history part to the $defs entry each of its lines follows.
var ocfPartDef = map[string]string{
	ocfPartOutcomes:     "outcome",
	ocfPartTrackRecords: "trackRecord",
	ocfPartGovernance:   "governanceEvent",
	ocfPartDossiers:     "dossierEntry",
	ocfPartPrincipals:   "principal",
}

// OCFPrincipal is one person's exported profile (spec 6.6). A person is exported
// only with a ConsentAt; the export refuses the whole package otherwise.
type OCFPrincipal struct {
	Member          string              `json:"member"`
	ConsentAt       time.Time           `json:"consent_at"`
	Status          string              `json:"status"`
	ShareWithOwners bool                `json:"share_with_owners"`
	Version         int                 `json:"version"`
	Summary         string              `json:"summary"`
	Traits          []OCFPrincipalTrait `json:"traits"`
}

// OCFPrincipalTrait is one active trait of a principal.
type OCFPrincipalTrait struct {
	DecisionKind string    `json:"decision_kind"`
	Statement    string    `json:"statement"`
	Origin       string    `json:"origin"`
	CreatedAt    time.Time `json:"created_at"`
}

// ExportOptions says what a package carries beyond the structure.
type ExportOptions struct {
	// History adds the history parts: settled outcomes, track records and the
	// governance log. Without it the package is the structure alone.
	History bool
	// Principals are the people whose profiles are exported. They need History,
	// and each must have consented.
	Principals []OCFPrincipal
	// PrincipalMember identifies the requesting person who consented to export.
	PrincipalMember pgtype.UUID
	// Now stamps the package. Zero means the current time.
	Now time.Time
}

// ocfManifest is manifest.json.
type ocfManifest struct {
	Format     string       `json:"format"`
	Generator  string       `json:"generator"`
	ExportedAt time.Time    `json:"exported_at"`
	Workspace  ocfWorkspace `json:"workspace"`
	Parts      []string     `json:"parts"`
}

type ocfWorkspace struct {
	Name string `json:"name"`
	Slug string `json:"slug"`
}

// ocfOutcome is one line of history/outcomes.ndjson. Agent and squad are names;
// agent is null when a member did the work.
type ocfOutcome struct {
	Agent                  *string `json:"agent"`
	Squad                  *string `json:"squad"`
	WorkClass              string  `json:"work_class"`
	RuntimeProvider        string  `json:"runtime_provider"`
	Model                  string  `json:"model"`
	VerificationMode       string  `json:"verification_mode"`
	Sampled                bool    `json:"sampled"`
	Verdict                string  `json:"verdict"`
	Severity               string  `json:"severity"`
	FailCause              string  `json:"fail_cause"`
	CostMicros             int64   `json:"cost_micros"`
	VerificationCostMicros int64   `json:"verification_cost_micros"`
	SettledAt              string  `json:"settled_at"`
	Superseded             bool    `json:"superseded"`
}

// ocfTrackRecord is one line of history/track_records.ndjson.
type ocfTrackRecord struct {
	Agent            string  `json:"agent"`
	WorkClass        string  `json:"work_class"`
	WindowDays       int32   `json:"window_days"`
	Model            string  `json:"model"`
	NPass            int32   `json:"n_pass"`
	NPartial         int32   `json:"n_partial"`
	NFail            int32   `json:"n_fail"`
	NUnverified      int32   `json:"n_unverified"`
	NHighFail        int32   `json:"n_high_fail"`
	ScoreLower       float64 `json:"score_lower"`
	VerificationMode string  `json:"verification_mode"`
	ModePinned       bool    `json:"mode_pinned"`
	ComputedAt       string  `json:"computed_at"`
}

// ocfGovernanceEvent is one line of history/governance.ndjson. The actor is its
// type only.
type ocfGovernanceEvent struct {
	Kind        string          `json:"kind"`
	ActorType   string          `json:"actor_type"`
	SubjectType string          `json:"subject_type"`
	Summary     string          `json:"summary"`
	Details     json.RawMessage `json:"details"`
	CreatedAt   string          `json:"created_at"`
}

type ocfDossierEntry struct {
	Agent     string          `json:"agent"`
	Kind      string          `json:"kind"`
	Text      string          `json:"text"`
	WorkClass string          `json:"work_class"`
	Evidence  json.RawMessage `json:"evidence"`
	Origin    string          `json:"origin"`
	Uses      int32           `json:"uses"`
	CreatedAt time.Time       `json:"created_at"`
}

// ExportOCF writes the workspace ws as an ocf/1 package. The package is validated
// against the schema before it is returned, so an export that breaks the format
// fails here rather than on someone's import.
func ExportOCF(ctx context.Context, q *db.Queries, ws pgtype.UUID, opts ExportOptions) (io.Reader, error) {
	now := opts.Now
	if now.IsZero() {
		now = time.Now()
	}
	if (len(opts.Principals) > 0 || opts.PrincipalMember.Valid) && !opts.History {
		return nil, errors.New("principals are exported with the history part only")
	}
	if opts.PrincipalMember.Valid {
		profile, err := q.GetPrincipalProfile(ctx, db.GetPrincipalProfileParams{WorkspaceID: ws, MemberID: opts.PrincipalMember})
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, fmt.Errorf("export principal profile: %w", err)
		}
		if err == nil && profile.ConsentAt.Valid {
			user, err := q.GetUser(ctx, opts.PrincipalMember)
			if err != nil {
				return nil, fmt.Errorf("export principal member: %w", err)
			}
			traits, err := q.ListActivePrincipalTraits(ctx, db.ListActivePrincipalTraitsParams{WorkspaceID: ws, MemberID: opts.PrincipalMember})
			if err != nil {
				return nil, fmt.Errorf("export principal traits: %w", err)
			}
			principal := OCFPrincipal{Member: user.Name, ConsentAt: profile.ConsentAt.Time, Status: profile.Status, ShareWithOwners: profile.ShareWithOwners, Version: int(profile.Version), Summary: profile.Summary, Traits: []OCFPrincipalTrait{}}
			for _, trait := range traits {
				principal.Traits = append(principal.Traits, OCFPrincipalTrait{DecisionKind: trait.DecisionKind, Statement: trait.Statement, Origin: trait.Origin, CreatedAt: trait.CreatedAt.Time})
			}
			opts.Principals = append(opts.Principals, principal)
		}
	}
	for _, p := range opts.Principals {
		if p.ConsentAt.IsZero() {
			return nil, fmt.Errorf("principal %q has not consented to export", p.Member)
		}
	}

	doc, err := ExportCompany(ctx, q, ws, now)
	if err != nil {
		return nil, err
	}
	structure, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("encode structure: %w", err)
	}
	files := map[string][]byte{"structure.json": structure}
	parts := []string{ocfPartStructure}

	if opts.History {
		history, err := ocfHistoryFiles(ctx, q, ws, opts.Principals)
		if err != nil {
			return nil, err
		}
		for name, body := range history {
			files[name] = body
		}
		parts = append(parts, ocfPartOutcomes, ocfPartTrackRecords, ocfPartGovernance, ocfPartDossiers)
		if len(opts.Principals) > 0 {
			parts = append(parts, ocfPartPrincipals)
		}
	}

	wsRow, err := q.GetWorkspace(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("export workspace: %w", err)
	}
	manifest, err := json.Marshal(ocfManifest{
		Format: OCFFormat, Generator: ocfGenerator, ExportedAt: now.UTC(),
		Workspace: ocfWorkspace{Name: wsRow.Name, Slug: wsRow.Slug}, Parts: parts,
	})
	if err != nil {
		return nil, fmt.Errorf("encode manifest: %w", err)
	}
	files["manifest.json"] = manifest

	if errs := ValidateOCF(manifest, files); len(errs) > 0 {
		return nil, fmt.Errorf("the export does not validate against ocf/1: %s", formatOCFErrors(errs))
	}
	pkg, err := zipOCF(files)
	if err != nil {
		return nil, err
	}
	return bytes.NewReader(pkg), nil
}

// ocfHistoryFiles builds the history files of ws. Each file is present, even when
// empty, because the manifest lists its part.
func ocfHistoryFiles(ctx context.Context, q *db.Queries, ws pgtype.UUID, principals []OCFPrincipal) (map[string][]byte, error) {
	agents, err := q.ListAgents(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("export agents: %w", err)
	}
	agentName := map[string]string{}
	for _, a := range agents {
		agentName[util.UUIDToString(a.ID)] = a.Name
	}
	squads, err := q.ListAllSquads(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("export squads: %w", err)
	}
	squadName := map[string]string{}
	for _, sq := range squads {
		squadName[util.UUIDToString(sq.ID)] = sq.Name
	}

	outcomes, err := q.ListSettledWorkOutcomesForOCF(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("export outcomes: %w", err)
	}
	var outcomeLines []ocfOutcome
	for _, o := range outcomes {
		row := ocfOutcome{
			WorkClass: o.WorkClass, RuntimeProvider: o.RuntimeProvider, Model: o.Model,
			VerificationMode: o.VerificationMode, Sampled: o.Sampled, Verdict: o.Verdict,
			Severity: o.Severity, FailCause: o.FailCause, CostMicros: o.CostMicros,
			VerificationCostMicros: o.VerificationCostMicros,
			SettledAt:              o.SettledAt.Time.UTC().Format(time.RFC3339Nano),
			Superseded:             o.SupersededBy.Valid,
		}
		if o.AgentID.Valid {
			if name, ok := agentName[util.UUIDToString(o.AgentID)]; ok {
				row.Agent = &name
			}
		}
		if o.SquadID.Valid {
			if name, ok := squadName[util.UUIDToString(o.SquadID)]; ok {
				row.Squad = &name
			}
		}
		outcomeLines = append(outcomeLines, row)
	}

	records, err := q.ListTrackRecordsForOCF(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("export track records: %w", err)
	}
	var trackLines []ocfTrackRecord
	for _, r := range records {
		name, ok := agentName[util.UUIDToString(r.AgentID)]
		if !ok {
			// The agent was deleted; a record without a name means nothing outside.
			continue
		}
		trackLines = append(trackLines, ocfTrackRecord{
			Agent: name, WorkClass: r.WorkClass, WindowDays: r.WindowDays, Model: r.Model,
			NPass: r.NPass, NPartial: r.NPartial, NFail: r.NFail, NUnverified: r.NUnverified,
			NHighFail: r.NHighFail, ScoreLower: r.ScoreLower, VerificationMode: r.VerificationMode,
			ModePinned: r.ModePinned, ComputedAt: r.ComputedAt.Time.UTC().Format(time.RFC3339Nano),
		})
	}

	events, err := ocfAllGovernanceEvents(ctx, q, ws)
	if err != nil {
		return nil, fmt.Errorf("export governance: %w", err)
	}
	var govLines []ocfGovernanceEvent
	for _, e := range events {
		details := json.RawMessage(e.Details)
		if !isJSONObject(details) {
			details = json.RawMessage("{}")
		}
		govLines = append(govLines, ocfGovernanceEvent{
			Kind: e.Kind, ActorType: e.ActorType, SubjectType: e.SubjectType, Summary: e.Summary,
			Details: details, CreatedAt: e.CreatedAt.Time.UTC().Format(time.RFC3339Nano),
		})
	}

	allAgents, err := q.ListAllAgentsAnyKind(ctx, ws)
	if err != nil {
		return nil, fmt.Errorf("export dossier agents: %w", err)
	}
	var dossierLines []ocfDossierEntry
	for _, agent := range allAgents {
		entries, err := q.ListSeatDossierEntries(ctx, db.ListSeatDossierEntriesParams{WorkspaceID: ws, AgentID: agent.ID, Status: "active", RowLimit: dossierMaxActive})
		if err != nil {
			return nil, fmt.Errorf("export dossier entries: %w", err)
		}
		for _, entry := range entries {
			dossierLines = append(dossierLines, ocfDossierEntry{agent.Name, entry.Kind, entry.Text, entry.WorkClass, entry.Evidence, entry.Origin, entry.Uses, entry.CreatedAt.Time})
		}
	}

	files := map[string][]byte{}
	if files[ocfPartFile(ocfPartOutcomes)], err = ndjson(outcomeLines); err != nil {
		return nil, err
	}
	if files[ocfPartFile(ocfPartTrackRecords)], err = ndjson(trackLines); err != nil {
		return nil, err
	}
	if files[ocfPartFile(ocfPartGovernance)], err = ndjson(govLines); err != nil {
		return nil, err
	}
	if files[ocfPartFile(ocfPartDossiers)], err = ndjson(dossierLines); err != nil {
		return nil, err
	}
	if len(principals) > 0 {
		if files[ocfPartFile(ocfPartPrincipals)], err = ndjson(principals); err != nil {
			return nil, err
		}
	}
	return files, nil
}

// ocfAllGovernanceEvents returns every governance event of ws, oldest first, by
// paging through ListGovernanceEvents (the log is append-only and has no other
// full read).
func ocfAllGovernanceEvents(ctx context.Context, q *db.Queries, ws pgtype.UUID) ([]db.GovernanceEvent, error) {
	const pageSize = 1000
	params := db.ListGovernanceEventsParams{WorkspaceID: ws, RowLimit: pageSize}
	var newestFirst []db.GovernanceEvent
	for {
		rows, err := q.ListGovernanceEvents(ctx, params)
		if err != nil {
			return nil, err
		}
		newestFirst = append(newestFirst, rows...)
		if len(rows) < pageSize {
			break
		}
		last := rows[len(rows)-1]
		params.BeforeCreatedAt = last.CreatedAt
		params.BeforeID = last.ID
	}
	for i, j := 0, len(newestFirst)-1; i < j; i, j = i+1, j-1 {
		newestFirst[i], newestFirst[j] = newestFirst[j], newestFirst[i]
	}
	return newestFirst, nil
}

// ndjson encodes rows, one JSON object per line.
func ndjson[T any](rows []T) ([]byte, error) {
	buf := bytes.NewBuffer(make([]byte, 0, 256))
	enc := json.NewEncoder(buf)
	for _, row := range rows {
		if err := enc.Encode(row); err != nil {
			return nil, fmt.Errorf("encode history row: %w", err)
		}
	}
	return buf.Bytes(), nil
}

// isJSONObject reports whether raw is a JSON object.
func isJSONObject(raw []byte) bool {
	trimmed := bytes.TrimSpace(raw)
	return len(trimmed) > 0 && trimmed[0] == '{'
}

// zipOCF packs the files of a package. The map's order does not matter: the zip
// is read by name.
func zipOCF(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			return nil, fmt.Errorf("package entry %s: %w", name, err)
		}
		if _, err := w.Write(body); err != nil {
			return nil, fmt.Errorf("package entry %s: %w", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		return nil, fmt.Errorf("close package: %w", err)
	}
	return buf.Bytes(), nil
}

// ValidationError is one failure of a package: the file, the line of an ndjson
// file (0 when not a line), and the JSON pointer of the value inside it.
type ValidationError struct {
	File    string `json:"file"`
	Line    int    `json:"line,omitempty"`
	Path    string `json:"path"`
	Message string `json:"message"`
}

func (e ValidationError) String() string {
	where := e.File
	if e.Line > 0 {
		where = fmt.Sprintf("%s:%d", where, e.Line)
	}
	return fmt.Sprintf("%s %s: %s", where, e.Path, e.Message)
}

// ocfDefs are the schemas a package is checked against: the root (manifest.json),
// and the $defs entries of structure.json, attestations and the history lines.
var ocfDefs = []string{"", "structure", "attestation", "outcome", "trackRecord", "governanceEvent", "dossierEntry", "principal"}

var (
	ocfSchemaOnce sync.Once
	ocfSchemas    map[string]*jsonschema.Schema
	ocfSchemaErr  error
)

// ocfCompiled compiles the embedded schema once. A compiled schema is read-only, so
// exports and imports running at once share it.
func ocfCompiled() (map[string]*jsonschema.Schema, error) {
	ocfSchemaOnce.Do(func() {
		doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(ocfSchemaJSON))
		if err != nil {
			ocfSchemaErr = fmt.Errorf("parse embedded ocf schema: %w", err)
			return
		}
		c := jsonschema.NewCompiler()
		c.AssertFormat()
		if err := c.AddResource("urn:ocf:v1", doc); err != nil {
			ocfSchemaErr = fmt.Errorf("add ocf schema: %w", err)
			return
		}
		compiled := map[string]*jsonschema.Schema{}
		for _, def := range ocfDefs {
			loc := "urn:ocf:v1"
			if def != "" {
				loc += "#/$defs/" + def
			}
			sch, err := c.Compile(loc)
			if err != nil {
				ocfSchemaErr = fmt.Errorf("compile ocf schema %s: %w", loc, err)
				return
			}
			compiled[def] = sch
		}
		ocfSchemas = compiled
	})
	return ocfSchemas, ocfSchemaErr
}

// ocfSchemaFor returns the compiled schema of one $defs entry, or the root when def is empty.
func ocfSchemaFor(def string) (*jsonschema.Schema, error) {
	schemas, err := ocfCompiled()
	if err != nil {
		return nil, err
	}
	sch, ok := schemas[def]
	if !ok {
		return nil, fmt.Errorf("no ocf schema for %q", def)
	}
	return sch, nil
}

// ValidateOCF checks a package against the schema and the rules the schema cannot
// state (README, "Quy tắc ngoài schema"). manifest is manifest.json; parts holds
// the other files by their names in the package. An empty result is a valid package.
//
// Rules 5 (attestation signatures), 6 (the 30-outcome minimum) and 7 (principal
// consent) are not checked here: the verifier, the issuer and the export do them.
func ValidateOCF(manifest []byte, parts map[string][]byte) []ValidationError {
	var errs []ValidationError
	root, err := ocfSchemaFor("")
	if err != nil {
		return []ValidationError{{File: "urn:ocf:v1", Message: err.Error()}}
	}
	mdoc, err := jsonschema.UnmarshalJSON(bytes.NewReader(manifest))
	if err != nil {
		return []ValidationError{{File: "manifest.json", Message: "not JSON: " + err.Error()}}
	}
	errs = append(errs, schemaErrors("manifest.json", 0, root, mdoc)...)

	var m ocfManifest
	if json.Unmarshal(manifest, &m) != nil || m.Format != OCFFormat {
		// Without a readable manifest the parts cannot be matched; the error above says why.
		return errs
	}
	errs = append(errs, ocfPartErrors(m.Parts, parts)...)

	for _, part := range m.Parts {
		switch {
		case part == ocfPartStructure:
			_, present := parts["structure.json"]
			errs = append(errs, validateStructureFile(parts["structure.json"], present)...)
		case part == ocfPartAttestations:
			for name, body := range parts {
				if strings.HasPrefix(name, "attestations/") {
					errs = append(errs, validateSingleDoc(name, body, "attestation")...)
				}
			}
		default:
			def, known := ocfPartDef[part]
			if !known {
				continue
			}
			file := ocfPartFile(part)
			if _, ok := parts[file]; !ok {
				continue // ocfPartErrors reports the missing file
			}
			errs = append(errs, validateNDJSON(file, parts[file], def)...)
		}
	}
	return errs
}

// ocfPartErrors checks that the manifest's parts and the files of the package agree.
func ocfPartErrors(listed []string, parts map[string][]byte) []ValidationError {
	var errs []ValidationError
	want := map[string]bool{}
	for _, part := range listed {
		if part == ocfPartAttestations {
			want["attestations/"] = true
			continue
		}
		file := ocfPartFile(part)
		want[file] = true
		if _, ok := parts[file]; !ok {
			errs = append(errs, ValidationError{File: "manifest.json", Path: "/parts",
				Message: fmt.Sprintf("part %q is listed but %s is not in the package", part, file)})
		}
	}
	for name := range parts {
		if name == "manifest.json" {
			continue
		}
		listedFile := want[name] || (strings.HasPrefix(name, "attestations/") && want["attestations/"])
		if !listedFile {
			errs = append(errs, ValidationError{File: name, Path: "",
				Message: "the package has this file but manifest.parts does not list it"})
		}
	}
	return errs
}

// validateStructureFile checks structure.json against $defs/structure, then the
// rules the schema cannot state on it.
func validateStructureFile(body []byte, present bool) []ValidationError {
	if !present {
		return nil // ocfPartErrors reports the missing file
	}
	sch, err := ocfSchemaFor("structure")
	if err != nil {
		return []ValidationError{{File: "structure.json", Message: err.Error()}}
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return []ValidationError{{File: "structure.json", Message: "not JSON: " + err.Error()}}
	}
	errs := schemaErrors("structure.json", 0, sch, doc)

	var s struct {
		Envelopes []struct {
			FloorLevel   int `json:"floor_level"`
			CeilingLevel int `json:"ceiling_level"`
		} `json:"envelopes"`
		Goals []struct {
			Title      string `json:"title"`
			Parent     string `json:"parent"`
			KeyResults []struct {
				StartValue  float64 `json:"start_value"`
				TargetValue float64 `json:"target_value"`
			} `json:"key_results"`
		} `json:"goals"`
	}
	if json.Unmarshal(body, &s) != nil {
		return errs
	}
	// Rule 2: an envelope's floor does not exceed its ceiling.
	for i, e := range s.Envelopes {
		if e.FloorLevel > e.CeilingLevel {
			errs = append(errs, ValidationError{File: "structure.json", Path: fmt.Sprintf("/envelopes/%d", i),
				Message: "floor_level is above ceiling_level"})
		}
	}
	// Rule 3: a key result moves; its target differs from its start.
	for i, g := range s.Goals {
		for j, kr := range g.KeyResults {
			if kr.TargetValue == kr.StartValue {
				errs = append(errs, ValidationError{File: "structure.json",
					Path:    fmt.Sprintf("/goals/%d/key_results/%d", i, j),
					Message: "target_value equals start_value"})
			}
		}
	}
	// Rule 4: a parent is named by title. Titles are not unique, so a title that
	// names two goals is an error, and so is one that names none.
	titles := map[string]int{}
	for _, g := range s.Goals {
		titles[g.Title]++
	}
	for i, g := range s.Goals {
		if g.Parent == "" {
			continue
		}
		switch n := titles[g.Parent]; {
		case n == 0:
			errs = append(errs, ValidationError{File: "structure.json", Path: fmt.Sprintf("/goals/%d/parent", i),
				Message: fmt.Sprintf("no goal is titled %q", g.Parent)})
		case n > 1:
			errs = append(errs, ValidationError{File: "structure.json", Path: fmt.Sprintf("/goals/%d/parent", i),
				Message: fmt.Sprintf("%d goals are titled %q; the parent is ambiguous", n, g.Parent)})
		}
	}
	return errs
}

// validateSingleDoc checks a whole JSON file against one $defs entry.
func validateSingleDoc(file string, body []byte, def string) []ValidationError {
	sch, err := ocfSchemaFor(def)
	if err != nil {
		return []ValidationError{{File: file, Message: err.Error()}}
	}
	doc, err := jsonschema.UnmarshalJSON(bytes.NewReader(body))
	if err != nil {
		return []ValidationError{{File: file, Message: "not JSON: " + err.Error()}}
	}
	return schemaErrors(file, 0, sch, doc)
}

// validateNDJSON checks each line of an ndjson file against one $defs entry.
func validateNDJSON(file string, body []byte, def string) []ValidationError {
	sch, err := ocfSchemaFor(def)
	if err != nil {
		return []ValidationError{{File: file, Message: err.Error()}}
	}
	var errs []ValidationError
	for i, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		doc, err := jsonschema.UnmarshalJSON(strings.NewReader(line))
		if err != nil {
			errs = append(errs, ValidationError{File: file, Line: i + 1, Message: "not JSON: " + err.Error()})
			continue
		}
		errs = append(errs, schemaErrors(file, i+1, sch, doc)...)
	}
	return errs
}

// schemaErrors flattens a validation result into one error per failing value.
func schemaErrors(file string, line int, sch *jsonschema.Schema, doc any) []ValidationError {
	err := sch.Validate(doc)
	if err == nil {
		return nil
	}
	var ve *jsonschema.ValidationError
	if !errors.As(err, &ve) {
		return []ValidationError{{File: file, Line: line, Message: err.Error()}}
	}
	var out []ValidationError
	var walk func(e *jsonschema.ValidationError)
	walk = func(e *jsonschema.ValidationError) {
		if len(e.Causes) == 0 {
			out = append(out, ValidationError{File: file, Line: line,
				Path: jsonPointer(e.InstanceLocation), Message: e.Error()})
			return
		}
		for _, c := range e.Causes {
			walk(c)
		}
	}
	walk(ve)
	return out
}

// jsonPointer renders an instance location as a JSON pointer, "" for the root.
func jsonPointer(tokens []string) string {
	var b strings.Builder
	for _, t := range tokens {
		b.WriteByte('/')
		b.WriteString(strings.NewReplacer("~", "~0", "/", "~1").Replace(t))
	}
	return b.String()
}

func formatOCFErrors(errs []ValidationError) string {
	parts := make([]string, 0, len(errs))
	for _, e := range errs {
		parts = append(parts, e.String())
	}
	return strings.Join(parts, "; ")
}

// ImportOCF recreates a package in ws. A zip without a manifest.json is refused with
// ErrNotOCFPackage, and the caller reads it as a legacy export. The package is
// validated before anything is written. The structure goes through ImportCompany, so
// it is matched and written the same way; the history is then written in its own
// transaction, marked imported, and written once per exported_at.
func ImportOCF(ctx context.Context, q *db.Queries, txStarter TxStarter, ws pgtype.UUID, r io.ReaderAt, size int64, opts ImportOptions) (ImportReport, error) {
	files, err := readOCFPackage(r, size)
	if err != nil {
		return ImportReport{}, err
	}
	manifest := files["manifest.json"]
	if errs := ValidateOCF(manifest, files); len(errs) > 0 {
		return ImportReport{}, fmt.Errorf("the package does not validate against ocf/1: %s", formatOCFErrors(errs))
	}
	var m ocfManifest
	if err := json.Unmarshal(manifest, &m); err != nil {
		return ImportReport{}, fmt.Errorf("read manifest: %w", err)
	}
	var doc CompanyExport
	if err := json.Unmarshal(files["structure.json"], &doc); err != nil {
		return ImportReport{}, fmt.Errorf("read structure: %w", err)
	}
	report, err := ImportCompany(ctx, q, txStarter, ws, doc, opts)
	if err != nil {
		return ImportReport{}, err
	}
	if err := importOCFHistory(ctx, q, txStarter, ws, m, files, opts, &report); err != nil {
		return ImportReport{}, err
	}
	return report, nil
}

// readOCFPackage reads the zip's files, refusing a package that is too large.
func readOCFPackage(r io.ReaderAt, size int64) (map[string][]byte, error) {
	if size > ocfMaxPackageBytes {
		return nil, fmt.Errorf("the package is larger than %d MB", ocfMaxPackageBytes>>20)
	}
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, ErrNotOCFPackage
	}
	if len(zr.File) > ocfMaxEntries {
		return nil, fmt.Errorf("the package has more than %d files", ocfMaxEntries)
	}
	files := map[string][]byte{}
	var total int64
	for _, f := range zr.File {
		if f.UncompressedSize64 > ocfMaxFileBytes {
			return nil, fmt.Errorf("%s is larger than %d MB", f.Name, ocfMaxFileBytes>>20)
		}
		budget := int64(ocfMaxUncompressedBytes) - total
		limit := int64(ocfMaxFileBytes)
		if budget < limit {
			limit = budget
		}
		rc, err := f.Open()
		if err != nil {
			return nil, fmt.Errorf("open %s: %w", f.Name, err)
		}
		// Read one byte past the limit so an oversize file is detected, not truncated.
		body, err := io.ReadAll(io.LimitReader(rc, limit+1))
		rc.Close()
		if err != nil {
			return nil, fmt.Errorf("read %s: %w", f.Name, err)
		}
		if int64(len(body)) > limit {
			return nil, fmt.Errorf("the package expands beyond %d MB", ocfMaxUncompressedBytes>>20)
		}
		total += int64(len(body))
		files[f.Name] = body
	}
	if _, ok := files["manifest.json"]; !ok {
		return nil, ErrNotOCFPackage
	}
	return files, nil
}

// importOCFHistory writes history as marked governance events and restores dossier
// entries. A package already imported (the same exported_at) writes nothing.
func importOCFHistory(ctx context.Context, q *db.Queries, txStarter TxStarter, ws pgtype.UUID, m ocfManifest, files map[string][]byte, opts ImportOptions, report *ImportReport) error {
	exportedAt := m.ExportedAt.UTC().Format(time.RFC3339Nano)
	marked, err := q.ListGovernanceEvents(ctx, db.ListGovernanceEventsParams{
		WorkspaceID: ws, Kind: pgtype.Text{String: ocfHistoryImportedKind, Valid: true}, RowLimit: 1000,
	})
	if err != nil {
		return fmt.Errorf("check earlier history import: %w", err)
	}
	for _, e := range marked {
		var d map[string]any
		if json.Unmarshal(e.Details, &d) == nil && d["exported_at"] == exportedAt {
			for _, part := range m.Parts {
				switch part {
				case ocfPartOutcomes, ocfPartTrackRecords, ocfPartGovernance, ocfPartDossiers:
					report.add("history", part, ImportExists, "imported before")
				case ocfPartPrincipals:
					report.add("history", part, ImportSkipped, "member identity and consent cannot be established for import")
				case ocfPartAttestations:
					report.add("history", part, ImportSkipped, "signature verification is not available for import")
				}
			}
			return nil
		}
	}

	var writes []func(q *db.Queries) error
	for _, part := range m.Parts {
		switch part {
		case ocfPartOutcomes, ocfPartTrackRecords, ocfPartGovernance, ocfPartDossiers:
		case ocfPartPrincipals:
			report.add("history", part, ImportSkipped, "member identity and consent cannot be established for import")
			continue
		case ocfPartAttestations:
			report.add("history", part, ImportSkipped, "signature verification is not available for import")
			continue
		default:
			continue
		}
		rows, err := ndjsonLines(files[ocfPartFile(part)])
		if err != nil {
			return fmt.Errorf("read %s: %w", part, err)
		}
		report.add("history", part, ImportCreate, fmt.Sprintf("%d rows", len(rows)))
		if opts.DryRun {
			continue
		}
		for _, row := range rows {
			writes = append(writes, importRowWriter(ctx, ws, opts.ActorID, part, row))
		}
	}
	if opts.DryRun || len(writes) == 0 {
		return nil
	}

	marker, _ := json.Marshal(map[string]any{"imported": true, "exported_at": exportedAt, "parts": m.Parts})
	writes = append(writes, func(q *db.Queries) error {
		return insertOCFEvent(ctx, q, ws, opts.ActorID, ocfHistoryImportedKind, "workspace",
			"History imported from an Open Company Format package.", marker)
	})

	tx, err := txStarter.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin history import: %w", err)
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)
	for _, write := range writes {
		if err := write(qtx); err != nil {
			return err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit history import: %w", err)
	}
	return nil
}

// ndjsonLines decodes each non-empty line of an ndjson file as a JSON object.
func ndjsonLines(body []byte) ([]json.RawMessage, error) {
	var out []json.RawMessage
	for _, line := range strings.Split(string(body), "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		out = append(out, json.RawMessage(line))
	}
	return out, nil
}

// importRowWriter returns the write of one history row. The row keeps what it said
// in its source kind; a track record is written with no mode: its verification mode
// is full and it is not pinned, whatever the package says (spec 12.2).
func importRowWriter(ctx context.Context, ws, actor pgtype.UUID, part string, row json.RawMessage) func(q *db.Queries) error {
	return func(q *db.Queries) error {
		var fields map[string]any
		if err := json.Unmarshal(row, &fields); err != nil {
			return fmt.Errorf("read %s row: %w", part, err)
		}
		switch part {
		case ocfPartOutcomes:
			return writeImportedRow(ctx, q, ws, actor, ocfImportedOutcomeKind, "outcome",
				fmt.Sprintf("Imported outcome: %v, %v.", fields["verdict"], fields["work_class"]), fields)
		case ocfPartTrackRecords:
			source := fields["verification_mode"]
			fields["verification_mode"] = "full"
			fields["mode_pinned"] = false
			fields["source_verification_mode"] = source
			return writeImportedRow(ctx, q, ws, actor, ocfImportedTrackRecordKind, "track_record",
				fmt.Sprintf("Imported track record for %v, %v. Shown for viewing only.", fields["agent"], fields["work_class"]), fields)
		case ocfPartDossiers:
			var entry ocfDossierEntry
			if err := json.Unmarshal(row, &entry); err != nil {
				return fmt.Errorf("read dossier row: %w", err)
			}
			agents, err := q.ListAgents(ctx, ws)
			if err != nil {
				return fmt.Errorf("list dossier agents: %w", err)
			}
			var seat pgtype.UUID
			for _, agent := range agents {
				if agent.Name == entry.Agent {
					seat = agent.ID
				}
			}
			if !seat.Valid {
				return fmt.Errorf("import dossier: agent %q not found", entry.Agent)
			}
			if err := lockSeatInWorkspace(ctx, q, ws, seat); err != nil {
				return err
			}
			count, err := q.CountActiveSeatDossierEntries(ctx, db.CountActiveSeatDossierEntriesParams{WorkspaceID: ws, AgentID: seat})
			if err != nil {
				return fmt.Errorf("count dossier entries: %w", err)
			}
			if count >= dossierMaxActive {
				return fmt.Errorf("import dossier for %q: %w", entry.Agent, ErrDossierFull)
			}
			_, err = q.InsertSeatDossierEntry(ctx, db.InsertSeatDossierEntryParams{
				ID: pgtype.UUID{Bytes: uuid.New(), Valid: true}, WorkspaceID: ws, AgentID: seat,
				Kind: entry.Kind, Text: entry.Text, WorkClass: entry.WorkClass, Evidence: entry.Evidence, Origin: entry.Origin,
			})
			if err != nil {
				return fmt.Errorf("import dossier for %q: %w", entry.Agent, err)
			}
			return writeImportedRow(ctx, q, ws, actor, ocfImportedDossierKind, "dossier",
				fmt.Sprintf("Imported dossier entry for %v.", fields["agent"]), fields)
		default:
			return writeImportedRow(ctx, q, ws, actor, ocfImportedGovernanceKind, "governance",
				fmt.Sprintf("Imported governance event: %v.", fields["summary"]), fields)
		}
	}
}

// writeImportedRow writes one imported row as a governance event. Its source fields
// go in details, with imported set.
func writeImportedRow(ctx context.Context, q *db.Queries, ws, actor pgtype.UUID, kind, subject, summary string, fields map[string]any) error {
	fields["imported"] = true
	details, err := json.Marshal(fields)
	if err != nil {
		return fmt.Errorf("encode imported %s: %w", kind, err)
	}
	return insertOCFEvent(ctx, q, ws, actor, kind, subject, ocfTruncate(summary, 500), details)
}

// insertOCFEvent appends one governance event as the importing member.
func insertOCFEvent(ctx context.Context, q *db.Queries, ws, actor pgtype.UUID, kind, subject, summary string, details []byte) error {
	_, err := q.InsertGovernanceEvent(ctx, db.InsertGovernanceEventParams{
		ID:          pgtype.UUID{Bytes: uuid.New(), Valid: true},
		WorkspaceID: ws,
		Kind:        kind,
		ActorType:   GovernanceActorMember,
		ActorID:     actor,
		SubjectType: subject,
		Summary:     summary,
		Details:     details,
	})
	if err != nil {
		return fmt.Errorf("write %s: %w", kind, err)
	}
	return nil
}

// ocfTruncate cuts s to at most n runes; the governance summary has a length limit.
func ocfTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
