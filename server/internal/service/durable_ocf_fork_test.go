package service

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
)

// The Open Company Format (ocf/1) tests. Export and import run against a real
// workspace; the schema, the rules outside it and the embedded copy are checked
// without one.

// ocfPlantHistory gives the source workspace one settled outcome, one track
// record row (pinned to spot verification, so a later mode would show) and one
// governance event. It returns the agent's id.
func ocfPlantHistory(t *testing.T, f principalFixture, agentID, squadID string) {
	t.Helper()
	issue := f.Issue(t, "OCF outcome issue")
	f.Insert(t, "work_outcome", testutil.Cols{
		"workspace_id": f.WorkspaceID, "issue_id": issue, "done_transition_id": uuid.NewString(),
		"agent_id": agentID, "squad_id": squadID, "runtime_provider": "claude", "model": "model-a",
		"work_class": "general", "verification_mode": "spot", "sampled": false,
		"verdict": "pass", "severity": "none", "fail_cause": "", "cost_micros": 1200,
		"settle_at": testutil.Raw("now()"), "settled_at": testutil.Raw("now()"),
	})
	f.Exec(t, `INSERT INTO track_record (workspace_id, agent_id, work_class, window_days, model,
		n_pass, n_partial, n_fail, n_unverified, n_high_fail, score_lower, verification_mode, mode_pinned, computed_at)
		VALUES ($1, $2, 'general', 30, '', 1, 0, 0, 0, 0, 0.2, 'spot', true, now())`, f.WorkspaceID, agentID)
	f.Cleanup(t, `DELETE FROM track_record WHERE workspace_id = $1`, f.WorkspaceID)
	f.Exec(t, `INSERT INTO governance_event (id, workspace_id, kind, actor_type, subject_type, summary, details)
		VALUES (gen_random_uuid(), $1, 'budget.updated', 'member', 'budget', 'Budget changed.', '{}'::jsonb)`, f.WorkspaceID)
	f.Cleanup(t, `DELETE FROM governance_event WHERE workspace_id = $1`, f.WorkspaceID)
}

// ocfSourceCompany is a workspace with one agent, a squad and the history above.
func ocfSourceCompany(t *testing.T) (principalFixture, string) {
	t.Helper()
	src, _ := companyWorkspace(t)
	_, leader := ownerAndLeader(t, src, "OCF Lead")
	squadID := src.Squad(t, "OCF Squad", leader)
	src.Exec(t, `UPDATE agent SET home_squad_id = $1 WHERE id = $2`, squadID, leader)
	ocfPlantHistory(t, src, leader, squadID)
	return src, leader
}

// exportOCF builds a package of ws and returns its bytes.
func exportOCF(t *testing.T, f principalFixture, opts ExportOptions) []byte {
	t.Helper()
	r, err := ExportOCF(context.Background(), f.q, util.MustParseUUID(f.WorkspaceID), opts)
	if err != nil {
		t.Fatalf("export ocf: %v", err)
	}
	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read package: %v", err)
	}
	return data
}

// readOCFZip returns the files of a package by their names in the package.
func readOCFZip(t *testing.T, data []byte) map[string][]byte {
	t.Helper()
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		t.Fatalf("open package: %v", err)
	}
	files := map[string][]byte{}
	for _, f := range zr.File {
		rc, err := f.Open()
		if err != nil {
			t.Fatalf("open %s: %v", f.Name, err)
		}
		body, err := io.ReadAll(rc)
		rc.Close()
		if err != nil {
			t.Fatalf("read %s: %v", f.Name, err)
		}
		files[f.Name] = body
	}
	return files
}

// writeOCFZip packages files, the way a hand-made or edited package is built.
func writeOCFZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, body := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s: %v", name, err)
		}
		if _, err := w.Write(body); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close package: %v", err)
	}
	return buf.Bytes()
}

// importOCF imports data into f's workspace with a runtime for any agents.
func importOCF(t *testing.T, f principalFixture, runtimeID string, data []byte) (ImportReport, error) {
	t.Helper()
	opts := ImportOptions{ActorID: util.MustParseUUID(f.UserID), RuntimeID: util.MustParseUUID(runtimeID)}
	return ImportOCF(context.Background(), f.q, f.Pool, util.MustParseUUID(f.WorkspaceID), bytes.NewReader(data), int64(len(data)), opts)
}

// ocfGovernanceByKind counts the governance events of ws by kind.
func ocfGovernanceByKind(t *testing.T, f principalFixture) map[string][]map[string]any {
	t.Helper()
	events, err := ocfAllGovernanceEvents(context.Background(), f.q, util.MustParseUUID(f.WorkspaceID))
	if err != nil {
		t.Fatalf("list governance: %v", err)
	}
	out := map[string][]map[string]any{}
	for _, e := range events {
		var details map[string]any
		if err := json.Unmarshal(e.Details, &details); err != nil {
			t.Fatalf("details of %s: %v", e.Kind, err)
		}
		out[e.Kind] = append(out[e.Kind], details)
	}
	return out
}

func TestExportHistoryValidatesAgainstSchema(t *testing.T) {
	src, _ := ocfSourceCompany(t)

	parts := readOCFZip(t, exportOCF(t, src, ExportOptions{History: true}))
	for _, name := range []string{"manifest.json", "structure.json", "history/outcomes.ndjson",
		"history/track_records.ndjson", "history/governance.ndjson"} {
		if _, ok := parts[name]; !ok {
			t.Fatalf("package has no %s; files: %v", name, ocfFileNames(parts))
		}
	}
	if errs := ValidateOCF(parts["manifest.json"], parts); len(errs) > 0 {
		t.Fatalf("exported package fails the schema: %v", errs)
	}
}

func TestImportMarksHistoryImported(t *testing.T) {
	src, _ := ocfSourceCompany(t)
	data := exportOCF(t, src, ExportOptions{History: true})

	dst, _ := companyWorkspace(t)
	runtimeID := dst.Runtime(t, "OCF import runtime")
	report, err := importOCF(t, dst, runtimeID, data)
	if err != nil {
		t.Fatalf("import: %v", err)
	}
	if report.Blocked != 0 {
		t.Fatalf("import blocked items: %+v", report.Items)
	}

	kinds := ocfGovernanceByKind(t, dst)
	if n := len(kinds["ocf.history_imported"]); n != 1 {
		t.Fatalf("history marker events = %d, want 1", n)
	}
	for _, kind := range []string{"ocf.imported.outcome", "ocf.imported.track_record", "ocf.imported.governance"} {
		rows := kinds[kind]
		if len(rows) == 0 {
			t.Fatalf("no %s rows imported; kinds = %v", kind, ocfKindNames(kinds))
		}
		for _, row := range rows {
			if row["imported"] != true {
				t.Fatalf("%s row is not marked imported: %v", kind, row)
			}
		}
	}

	// A second import of the same package writes no history again.
	if _, err := importOCF(t, dst, runtimeID, data); err != nil {
		t.Fatalf("second import: %v", err)
	}
	again := ocfGovernanceByKind(t, dst)
	if n := len(again["ocf.imported.outcome"]); n != len(kinds["ocf.imported.outcome"]) {
		t.Fatalf("second import wrote outcome rows: %d, was %d", n, len(kinds["ocf.imported.outcome"]))
	}
}

func TestImportedTrackRecordsGiveNoMode(t *testing.T) {
	src, _ := ocfSourceCompany(t)
	data := exportOCF(t, src, ExportOptions{History: true})

	dst, _ := companyWorkspace(t)
	runtimeID := dst.Runtime(t, "OCF mode runtime")
	if _, err := importOCF(t, dst, runtimeID, data); err != nil {
		t.Fatalf("import: %v", err)
	}

	rows := ocfGovernanceByKind(t, dst)["ocf.imported.track_record"]
	if len(rows) == 0 {
		t.Fatal("no track record rows imported")
	}
	for _, row := range rows {
		if row["verification_mode"] != "full" {
			t.Fatalf("imported track record has mode %v, want full: %v", row["verification_mode"], row)
		}
		if row["mode_pinned"] != false {
			t.Fatalf("imported track record is pinned: %v", row)
		}
	}

	// The imported rows never reach the track record table that decides the mode.
	var count int
	if err := dst.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM track_record WHERE workspace_id = $1`, dst.WorkspaceID).Scan(&count); err != nil {
		t.Fatalf("count track records: %v", err)
	}
	if count != 0 {
		t.Fatalf("import wrote %d track_record rows; imported records must grant no mode", count)
	}
}

func TestImportLegacyExport(t *testing.T) {
	src, _ := ocfSourceCompany(t)
	doc, err := ExportCompany(context.Background(), src.q, util.MustParseUUID(src.WorkspaceID), time.Now())
	if err != nil {
		t.Fatalf("legacy export: %v", err)
	}
	legacy, err := json.Marshal(doc)
	if err != nil {
		t.Fatalf("encode legacy export: %v", err)
	}

	dst, _ := companyWorkspace(t)
	_, err = ImportOCF(context.Background(), dst.q, dst.Pool, util.MustParseUUID(dst.WorkspaceID),
		bytes.NewReader(legacy), int64(len(legacy)), ImportOptions{DryRun: true})
	if !errors.Is(err, ErrNotOCFPackage) {
		t.Fatalf("ImportOCF on a legacy export: err = %v, want ErrNotOCFPackage", err)
	}

	// The legacy document still imports through the existing path, structure only.
	var legacyDoc CompanyExport
	if err := json.Unmarshal(legacy, &legacyDoc); err != nil {
		t.Fatalf("decode legacy export: %v", err)
	}
	report, err := ImportCompany(context.Background(), dst.q, dst.Pool, util.MustParseUUID(dst.WorkspaceID), legacyDoc,
		ImportOptions{DryRun: true, ActorID: util.MustParseUUID(dst.UserID)})
	if err != nil {
		t.Fatalf("legacy import: %v", err)
	}
	if !report.DryRun {
		t.Fatal("legacy dry run reported a live import")
	}
}

func TestUnknownFieldRejectedWithPath(t *testing.T) {
	src, _ := ocfSourceCompany(t)
	parts := readOCFZip(t, exportOCF(t, src, ExportOptions{History: true}))

	var manifest map[string]any
	if err := json.Unmarshal(parts["manifest.json"], &manifest); err != nil {
		t.Fatalf("decode manifest: %v", err)
	}
	manifest["workspace"].(map[string]any)["owner_email"] = "someone@example.com"
	edited, err := json.Marshal(manifest)
	if err != nil {
		t.Fatalf("encode manifest: %v", err)
	}
	parts["manifest.json"] = edited

	errs := ValidateOCF(parts["manifest.json"], parts)
	if !hasOCFErrorAt(errs, "manifest.json", "/workspace") {
		t.Fatalf("no error at /workspace in %v", errs)
	}

	// The importer refuses the package before it writes anything.
	dst, _ := companyWorkspace(t)
	data := writeOCFZip(t, parts)
	if _, err := importOCF(t, dst, dst.Runtime(t, "OCF reject runtime"), data); err == nil ||
		!strings.Contains(err.Error(), "/workspace") {
		t.Fatalf("import of an edited package: err = %v, want an error naming /workspace", err)
	}
	if kinds := ocfGovernanceByKind(t, dst); len(kinds) != 0 {
		t.Fatalf("a rejected package wrote governance events: %v", ocfKindNames(kinds))
	}
}

func TestPrincipalsExportedOnlyWithConsent(t *testing.T) {
	src, _ := ocfSourceCompany(t)
	ctx := context.Background()
	ws := util.MustParseUUID(src.WorkspaceID)

	unconsented := OCFPrincipal{Member: "Dana", Status: "active", Version: 1, Summary: "Prefers short replies.",
		Traits: []OCFPrincipalTrait{}}
	if _, err := ExportOCF(ctx, src.q, ws, ExportOptions{History: true, Principals: []OCFPrincipal{unconsented}}); err == nil ||
		!strings.Contains(err.Error(), "Dana") {
		t.Fatalf("export of an unconsented principal: err = %v, want a refusal naming Dana", err)
	}

	consented := unconsented
	consented.ConsentAt = time.Now()
	parts := readOCFZip(t, exportOCF(t, src, ExportOptions{History: true, Principals: []OCFPrincipal{consented}}))
	if _, ok := parts["history/principals.ndjson"]; !ok {
		t.Fatalf("consented principal not exported; files: %v", ocfFileNames(parts))
	}
	if errs := ValidateOCF(parts["manifest.json"], parts); len(errs) > 0 {
		t.Fatalf("package with a consented principal fails the schema: %v", errs)
	}

	// Without the history part a principal is not exported at all.
	if _, err := ExportOCF(ctx, src.q, ws, ExportOptions{Principals: []OCFPrincipal{consented}}); err == nil {
		t.Fatal("principals without history were accepted")
	}

	without := readOCFZip(t, exportOCF(t, src, ExportOptions{History: true}))
	if _, ok := without["history/principals.ndjson"]; ok {
		t.Fatal("a package with no principals carries history/principals.ndjson")
	}
}

func TestEmbeddedSchemaMatchesDocs(t *testing.T) {
	docs, err := os.ReadFile("../../../docs/specs/durable-platform/ocf/v1.schema.json")
	if err != nil {
		t.Fatalf("read docs schema: %v", err)
	}
	if !bytes.Equal(ocfSchemaJSON, docs) {
		t.Fatal("the embedded schema differs from docs/specs/durable-platform/ocf/v1.schema.json; copy the docs file over it")
	}
}

// TestValidateOCFRulesOutsideSchema covers the rules the schema cannot state
// (README, "Quy tắc ngoài schema"), on packages that are otherwise valid.
func TestValidateOCFRulesOutsideSchema(t *testing.T) {
	structureOnly := func(structure string) map[string][]byte {
		return map[string][]byte{
			"manifest.json":  []byte(`{"format":"ocf/1","generator":"delegora/0.9.0","exported_at":"2026-11-25T00:00:00Z","workspace":{"name":"A","slug":"a"},"parts":["structure"]}`),
			"structure.json": []byte(structure),
		}
	}

	t.Run("floor above ceiling", func(t *testing.T) {
		parts := structureOnly(emptyStructureJSON(`"envelopes":[{"squad":"S","floor_level":3,"ceiling_level":1,"reason":""}]`))
		if !hasOCFErrorAt(ValidateOCF(parts["manifest.json"], parts), "structure.json", "/envelopes/0") {
			t.Fatal("floor above ceiling was accepted")
		}
	})

	t.Run("parts and files disagree", func(t *testing.T) {
		parts := structureOnly(emptyStructureJSON(""))
		parts["stray.json"] = []byte("{}")
		if len(ValidateOCF(parts["manifest.json"], parts)) == 0 {
			t.Fatal("a package whose parts do not match its files was accepted")
		}
	})

	t.Run("key result target equals start", func(t *testing.T) {
		goal := `{"title":"G","why":"","scope_type":"company","scope":"","parent":"","period_start":"","period_end":"","owner_type":"member","owner":"","status":"draft","key_results":[{"title":"K","metric_key":"k","scope_type":"company","scope":"","start_value":5,"target_value":5,"weight":1}]}`
		parts := structureOnly(emptyStructureJSON(`"goals":[` + goal + `]`))
		if !hasOCFErrorAt(ValidateOCF(parts["manifest.json"], parts), "structure.json", "/goals/0/key_results/0") {
			t.Fatal("a key result with target equal to start was accepted")
		}
	})

	t.Run("goal parent title is ambiguous", func(t *testing.T) {
		goal := func(parent string) string {
			return `{"title":"Same","why":"","scope_type":"company","scope":"","parent":"` + parent + `","period_start":"","period_end":"","owner_type":"member","owner":"","status":"draft","key_results":[]}`
		}
		parts := structureOnly(emptyStructureJSON(`"goals":[` + goal("") + `,` + goal("") + `,` + goal("Same") + `]`))
		if len(ValidateOCF(parts["manifest.json"], parts)) == 0 {
			t.Fatal("a parent title shared by two goals was accepted")
		}
	})
}

// emptyStructureJSON is a structure.json with every required key, plus extra
// top-level members.
func emptyStructureJSON(extra string) string {
	body := `{"schema_version":1,"exported_at":"2026-11-25T00:00:00Z","retention_note":"","excluded":[],` +
		`"floors":[],"squads":[],"agents":[],"floor_chiefs":[],"pipelines":[],"initiatives":[],` +
		`"knowledge_kinds":[],"goals":[],"budgets":[],"approval_policies":[],"governance":{}`
	if extra != "" {
		body += "," + extra
	}
	return body + "}"
}

// hasOCFErrorAt reports whether errs has an error in file at path.
func hasOCFErrorAt(errs []ValidationError, file, path string) bool {
	for _, e := range errs {
		if e.File == file && e.Path == path {
			return true
		}
	}
	return false
}

func ocfFileNames(m map[string][]byte) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

func ocfKindNames(m map[string][]map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
