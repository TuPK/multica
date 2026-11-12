package litscout

import (
	"bytes"
	"encoding/csv"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// goldenState rebuilds the state of LitScout's recorded example run from its own output files: the three selected papers,
// plus placeholder records so the counts match (72 retrieved, 52 unique, 3 rounds, 6 warnings).
func goldenState(t *testing.T) *State {
	t.Helper()
	dir := filepath.Join("testdata", "legal_rag_run")
	raw, err := os.ReadFile(filepath.Join(dir, "papers.csv"))
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(bytes.NewReader(raw)).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	cfg := DefaultConfig()
	cfg.MaxPapers, cfg.MinPapers, cfg.ResultsPerQuery = 15, 10, 30
	s, err := Init("Vietnamese legal GraphRAG", cfg)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rows[1:] {
		score, _ := strconv.ParseFloat(r[9], 64)
		m := paper.Metadata{
			ID: "p-" + r[0], Title: r[1], Authors: strings.Split(r[2], "; "), PublicationDate: r[3], DOI: r[4],
			OpenAlexID: r[5], URL: r[6], MetadataSource: r[7], AbstractSource: r[8], Abstract: "abstract text",
		}
		s.Unique = append(s.Unique, m)
		var warns []string
		if r[12] != "" {
			warns = strings.Split(r[12], "; ")
		}
		s.Scored = append(s.Scored, ScoredEntry{ID: m.ID, Analysis: paper.Analysis{RelevanceScore: score, RelevanceReason: r[10], Summary: r[11], ScoredFromAbstract: true}, Warnings: warns})
	}
	for len(s.Unique) < 52 {
		s.Unique = append(s.Unique, paper.Metadata{ID: "p-filler" + strconv.Itoa(len(s.Unique)), Title: "filler"})
	}
	s.Raw = make([]paper.Metadata, 72)
	s.SearchRound = 3
	digest, err := os.ReadFile(filepath.Join(dir, "research_digest.md"))
	if err != nil {
		t.Fatal(err)
	}
	inWarnings := false
	for _, l := range strings.Split(string(digest), "\n") {
		switch {
		case l == "### Warnings":
			inWarnings = true
		case strings.HasPrefix(l, "## "):
			inWarnings = false
		case inWarnings && strings.HasPrefix(l, "- "):
			s.Warnings = append(s.Warnings, strings.TrimPrefix(l, "- "))
		}
	}
	if len(s.Warnings) != 6 {
		t.Fatalf("fixture warnings: %d", len(s.Warnings))
	}
	return s
}

func TestMarkdownReproducesLitScoutDigest(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "legal_rag_run", "research_digest.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := goldenState(t).Markdown()
	if got != string(want) {
		t.Fatalf("digest differs from the LitScout example.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestCSVReproducesLitScoutPapersTable(t *testing.T) {
	want, err := os.ReadFile(filepath.Join("testdata", "legal_rag_run", "papers.csv"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := goldenState(t).CSV(ExportOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Python's csv module ends rows with CRLF; Go's writer uses LF. The cells are what must match.
	norm := func(b []byte) string { return strings.ReplaceAll(string(b), "\r\n", "\n") }
	if norm(got) != norm(want) {
		t.Fatalf("papers.csv differs from the LitScout example.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestCSVNeutralisesFormulasAndAddsBOM(t *testing.T) {
	s := newRun(t, nil)
	s.Unique = []paper.Metadata{{ID: "p-1", Title: "=HYPERLINK(\"http://x\")", Authors: []string{"@evil", "Ann"}, DOI: "10.1/x", MetadataSource: "OpenAlex"}}
	s.Scored = []ScoredEntry{{ID: "p-1", Analysis: paper.Analysis{RelevanceScore: 7.5, RelevanceReason: "-1+1", Summary: "line one\nline two, with comma"}}}
	b, err := s.CSV(ExportOptions{BOM: true})
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(b, []byte("\ufeff")) {
		t.Fatal("BOM missing")
	}
	rows, err := csv.NewReader(bytes.NewReader(bytes.TrimPrefix(b, []byte("\ufeff")))).ReadAll()
	if err != nil || len(rows) != 2 {
		t.Fatalf("%v %v", rows, err)
	}
	r := rows[1]
	if r[1] != "'=HYPERLINK(\"http://x\")" || r[2] != "'@evil; Ann" || r[10] != "'-1+1" || r[9] != "7.5" || r[11] != "line one\nline two, with comma" {
		t.Fatalf("%q", r)
	}
}

func TestMarkdownAddsSynthesisAndKeepsFreeTextOutOfHeadings(t *testing.T) {
	s := selectedRun(t)
	s.Scored[0].RelevanceReason = "# Not a heading"
	s.Claims = []Claim{{Text: "Graph retrieval helps", PaperIDs: []string{s.Scored[0].ID, s.Scored[1].ID}}}
	s.DOIStatus = map[string]string{s.Scored[0].ID: "unresolved"}
	md := s.Markdown()
	for _, want := range []string{"## Synthesis", "- Graph retrieval helps [1, 2]", "\\# Not a heading", "⚠ DOI not found at Crossref or doi.org.", "⚠ Abstract unavailable"} {
		if !strings.Contains(md, want) {
			t.Errorf("digest lacks %q:\n%s", want, md)
		}
	}
	if !strings.Contains(md, "does not download PDFs") {
		t.Error("default scope sentence changed")
	}
	s.DOIStatus[s.Scored[0].ID] = "ok_other_registry"
	if !strings.Contains(s.Markdown(), "DOI confirmed by doi.org (registered outside Crossref") {
		t.Error("a DOI known only to another agency must say so")
	}
	s.Config.AllowFullText = true
	if strings.Contains(s.Markdown(), "does not download PDFs") {
		t.Error("scope must say full text is possible when the run allows it")
	}
}

func TestExportRefusesToWriteWhenVerificationFails(t *testing.T) {
	s := selectedRun(t) // the gate has not run
	out := t.TempDir()
	_, paths, err := s.Export(out, ExportOptions{}, VerifyOptions{})
	if err == nil || paths != nil {
		t.Fatalf("export must fail: %v", err)
	}
	if entries, _ := os.ReadDir(out); len(entries) != 0 {
		t.Fatalf("nothing may be written, found %d files", len(entries))
	}
	s.Gate(false)
	_, paths, err = s.Export(out, ExportOptions{}, VerifyOptions{})
	if err != nil || len(paths) != 2 || len(s.OutputFiles) != 2 {
		t.Fatalf("%v %v", paths, err)
	}
}
