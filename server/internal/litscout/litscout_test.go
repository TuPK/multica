package litscout

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
	"github.com/multica-ai/multica/server/internal/litscout/source"
)

func md(title, author, doi string) paper.Metadata {
	m := paper.Metadata{Title: title, MetadataSource: "OpenAlex", DOI: doi}
	if author != "" {
		m.Authors = []string{author}
	}
	return m
}

func TestNormalizeFoldsCaseAccentsAndPunctuation(t *testing.T) {
	cases := map[string]string{
		"  Graph-RAG: A Survey! ":      "graph rag a survey",
		"Phân tích Pháp luật Việt Nam": "phan tich phap luat viet nam",
		"Straße":                       "strasse",
		"Café—Déjà vu":                 "cafe deja vu",
	}
	for in, want := range cases {
		if got := Normalize(in); got != want {
			t.Errorf("Normalize(%q)=%q want %q", in, got, want)
		}
	}
	if got := Tokens("Việt Nam's GraphRAG"); strings.Join(got, "|") != "việt|nam|s|graphrag" {
		t.Errorf("Tokens=%v", got)
	}
}

func TestIndelRatioIsNotLevenshtein(t *testing.T) {
	if got := IndelRatio("abc", "abd"); math.Abs(got-66.6667) > 0.01 {
		t.Fatalf("abc/abd = %v, want 66.67 (Indel: 2*2/6)", got)
	}
	if IndelRatio("same", "same") != 100 || IndelRatio("", "") != 100 || IndelRatio("a", "") != 0 {
		t.Fatal("edge cases")
	}
	// One substitution in a 100 character title: Indel counts two edits, so it is 99, not 99 as Levenshtein would round.
	a := strings.Repeat("a", 100)
	b := strings.Repeat("a", 99) + "b"
	if got := IndelRatio(a, b); got != 99 {
		t.Fatalf("got %v", got)
	}
}

func TestSameUsesEvidenceInOrder(t *testing.T) {
	cases := []struct {
		name string
		a, b paper.Metadata
		want bool
	}{
		{"same doi", md("A", "X", "10.1/a"), md("B", "Y", "10.1/a"), true},
		{"same openalex id", paper.Metadata{Title: "A", OpenAlexID: "W1"}, paper.Metadata{Title: "B", OpenAlexID: "W1"}, true},
		{"same arxiv id", paper.Metadata{Title: "A", ArxivID: "2501.1"}, paper.Metadata{Title: "B", ArxivID: "2501.1"}, true},
		{"same title and first author", md("Graph RAG", "Ann Lee", ""), md("graph-rag", "ann lee", ""), true},
		{"same title, no author evidence", md("Graph RAG", "", ""), md("Graph RAG", "", ""), false},
		{"same title, other author", md("Graph RAG", "Ann Lee", ""), md("Graph RAG", "Bo Chan", ""), false},
		{"fuzzy title needs the author", md("A Survey of Retrieval Augmented Generation Methods for Legal Question Answering", "Ann Lee", ""),
			md("A Survey of Retrieval Augmented Generation Methods for Legal Question Answerings", "Ann Lee", ""), true},
		{"fuzzy title without author evidence", md("A Survey of Retrieval Augmented Generation Methods for Legal Question Answering", "", ""),
			md("A Survey of Retrieval Augmented Generation Methods for Legal Question Answerings", "", ""), false},
		{"different doi but same title and author still merge", md("T", "A", "10.1/a"), md("T", "A", "10.1/b"), true},
	}
	for _, c := range cases {
		if got := Same(c.a, c.b); got != c.want {
			t.Errorf("%s: Same=%v want %v", c.name, got, c.want)
		}
	}
}

func TestMergeFillsGapsAndKeepsProvenance(t *testing.T) {
	a := paper.Metadata{ID: "p-1", Title: "T", Authors: []string{"A"}, MetadataSource: "OpenAlex"}
	b := paper.Metadata{Title: "T", DOI: "10.1/x", Abstract: "Abs", MetadataSource: "Crossref", AbstractSource: "Crossref", URL: "u"}
	got := Merge(a, b)
	if got.ID != "p-1" || got.MetadataSource != "OpenAlex" || got.DOI != "10.1/x" || got.Abstract != "Abs" || got.AbstractSource != "Crossref" || got.URL != "u" {
		t.Fatalf("%+v", got)
	}
	// An existing abstract is never replaced.
	a.Abstract, a.AbstractSource = "Mine", "OpenAlex"
	if got := Merge(a, b); got.Abstract != "Mine" || got.AbstractSource != "OpenAlex" {
		t.Fatalf("abstract replaced: %+v", got)
	}
}

func TestDeduplicateKeepsIDsAcrossPasses(t *testing.T) {
	first := Deduplicate(nil, []paper.Metadata{md("Graph RAG", "Ann", ""), md("Other", "Bo", "10.1/o")})
	id := first[0].ID
	second := Deduplicate(first, []paper.Metadata{{Title: "graph rag", Authors: []string{"ann"}, DOI: "10.1/g", MetadataSource: "Crossref"}, md("New", "Cy", "")})
	if len(second) != 3 || second[0].ID != id || second[0].DOI != "10.1/g" {
		t.Fatalf("%+v", second)
	}
}

func TestRouteBranches(t *testing.T) {
	cases := []struct {
		sel, min, round, max int
		noNew                bool
		want                 Decision
	}{
		{10, 10, 1, 3, false, DecisionSummarize},
		{12, 10, 3, 3, false, DecisionSummarize},
		{3, 10, 1, 3, false, DecisionRefineQuery},
		{3, 10, 2, 3, false, DecisionRefineQuery},
		{3, 10, 3, 3, false, DecisionSummarizeAvailable},
		{3, 10, 1, 3, true, DecisionStopNoNewQueries},
		{10, 10, 1, 3, true, DecisionSummarize},
	}
	for _, c := range cases {
		if got := Route(c.sel, c.min, c.round, c.max, c.noNew); got != c.want {
			t.Errorf("Route%+v=%s want %s", c, got, c.want)
		}
	}
}

func TestPrefilterRanksAndCaps(t *testing.T) {
	ps := []paper.Metadata{
		{Title: "Cooking", Abstract: "recipes"},
		{Title: "Vietnamese legal GraphRAG", Abstract: "x"},
		{Title: "Legal systems", Abstract: "vietnamese law"},
		{Title: "Other", Abstract: "legal"},
	}
	got := Prefilter("Vietnamese legal GraphRAG", ps, 2)
	if len(got) != 2 || got[0].Title != "Vietnamese legal GraphRAG" || got[1].Title != "Legal systems" {
		t.Fatalf("%+v", got)
	}
	if all := Prefilter("q", ps, 10); len(all) != 4 {
		t.Fatal("below the cap nothing is dropped")
	}
	if all := Prefilter("q", ps, 0); len(all) != 4 {
		t.Fatal("limit 0 means no cap")
	}
}

func TestPrefilterIsStableForEqualRank(t *testing.T) {
	ps := []paper.Metadata{{Title: "a1"}, {Title: "a2"}, {Title: "a3"}, {Title: "a4"}}
	got := Prefilter("zzz", ps, 3)
	if got[0].Title != "a1" || got[1].Title != "a2" || got[2].Title != "a3" {
		t.Fatalf("%+v", got)
	}
}

func TestHeuristicScore(t *testing.T) {
	s, reason := HeuristicScore("legal graph", paper.Metadata{Title: "Legal systems", Abstract: "none"})
	if s != 5 || !strings.Contains(reason, "fallback") {
		t.Fatalf("%v %q", s, reason)
	}
}

func TestValidatePaper(t *testing.T) {
	ok := paper.Metadata{Title: "T", Authors: []string{"A"}, DOI: "10.1/x", MetadataSource: "OpenAlex"}
	if errs := ValidatePaper(ok); len(errs) != 0 {
		t.Fatal(errs)
	}
	bad := paper.Metadata{MetadataSource: "Wikipedia", Abstract: "x", AbstractSource: "LLM"}
	want := []string{"INVALID_METADATA_SOURCE", "INVALID_ABSTRACT_SOURCE", "MISSING_TITLE", "MISSING_AUTHORS", "MISSING_RESOLVABLE_SOURCE_IDENTIFIER"}
	got := ValidatePaper(bad)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%v", got)
	}
	noIDButURL := paper.Metadata{Title: "T", Authors: []string{"A"}, URL: "https://x", MetadataSource: "arXiv"}
	if len(ValidatePaper(noIDButURL)) != 0 {
		t.Fatal("a URL is a resolvable identifier")
	}
}

type fakeSource struct {
	name   string
	byText map[string][]paper.Metadata
	err    error
	calls  int
}

func (f *fakeSource) Name() string { return f.name }
func (f *fakeSource) Search(_ context.Context, q source.Query) ([]paper.Metadata, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	return f.byText[q.Text], nil
}

func newRun(t *testing.T, mut func(*Config)) *State {
	t.Helper()
	cfg := DefaultConfig()
	cfg.MinPapers, cfg.MaxPapers, cfg.MaxRounds = 2, 5, 2
	if mut != nil {
		mut(&cfg)
	}
	s, err := Init("legal graph rag", cfg)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func rec(title, author, doi, abstract string) paper.Metadata {
	m := md(title, author, doi)
	m.Abstract = abstract
	if abstract != "" {
		m.AbstractSource = "OpenAlex"
	}
	return m
}

func TestConfigValidation(t *testing.T) {
	bad := []func(*Config){
		func(c *Config) { c.MinPapers = 30 },
		func(c *Config) { c.MaxRounds = 6 },
		func(c *Config) { c.MaxRounds = 0 },
		func(c *Config) { c.Threshold = 11 },
		func(c *Config) { c.Sources = []string{"bing"} },
		func(c *Config) { c.Sources = nil },
		func(c *Config) { c.ResultsPerQuery = 101 },
	}
	for i, m := range bad {
		cfg := DefaultConfig()
		m(&cfg)
		if _, err := Init("q", cfg); err == nil {
			t.Errorf("case %d accepted", i)
		}
	}
	if _, err := Init("  ", DefaultConfig()); err == nil {
		t.Fatal("empty query accepted")
	}
}

func TestFullRunWithGate(t *testing.T) {
	ctx := context.Background()
	fs := &fakeSource{name: "openalex", byText: map[string][]paper.Metadata{
		"legal graph rag": {
			rec("Legal GraphRAG", "Ann", "10.1/a", "A legal graph rag system."),
			rec("legal graphrag", "Ann", "", ""), // duplicate of the first, merges
			rec("Cooking", "Bo", "10.1/c", "Recipes."),
			rec("No abstract paper", "Cy", "10.1/n", ""),
			{Title: "Front matter", MetadataSource: "OpenAlex", Type: "paratext", Authors: []string{"X"}, URL: "u"},
		},
		"graph rag for law": {rec("Law and graphs", "Di", "10.1/l", "Graph rag for law.")},
	}}
	svc := Services{Sources: map[string]source.Source{"openalex": fs}}
	s := newRun(t, nil)

	rep, err := s.Search(ctx, svc, []string{"legal graph rag", "Legal  Graph RAG"}, nil)
	if err != nil || rep.Round != 1 || len(rep.Results) != 1 || rep.Results[0].Count != 5 {
		t.Fatalf("%+v %v", rep, err)
	}
	d := s.Dedup()
	if d.Unique != 3 || d.Excluded != 1 || d.MissingAbstract != 1 {
		t.Fatalf("%+v", d)
	}
	if !hasWarning(s, "MISSING_ABSTRACT: No abstract paper") || !hasWarning(s, "EXCLUDED_TYPE[paratext]: Front matter") {
		t.Fatalf("%v", s.Warnings)
	}
	cands := s.Candidates()
	if len(cands) != 3 || len(s.Pending) != 3 {
		t.Fatalf("%+v", cands)
	}
	scores := `{"scores":[` + scoreJSON(cands, map[string]float64{"Legal GraphRAG": 9, "Cooking": 1, "No abstract paper": 7}) + `]}`
	if err := s.IngestScores([]byte(scores)); err != nil {
		t.Fatal(err)
	}
	// Two of three pass the threshold of 6, but this run wants three.
	s.Config.MinPapers = 3
	g := s.Gate(false)
	if g.Decision != DecisionRefineQuery || g.Selected != 2 || len(g.BorderlineTitles) != 1 || g.BorderlineTitles[0] != "Cooking" {
		t.Fatalf("%+v", g)
	}

	// Round 2 finds one more paper; the gate is then satisfied.
	if _, err := s.Search(ctx, svc, []string{"graph rag for law"}, nil); err != nil {
		t.Fatal(err)
	}
	s.Dedup()
	c2 := s.Candidates()
	if len(c2) != 1 || c2[0].Title != "Law and graphs" {
		t.Fatalf("only the new paper may be offered: %+v", c2)
	}
	if err := s.IngestScores([]byte(`{"scores":[` + scoreJSON(c2, map[string]float64{"Law and graphs": 8}) + `]}`)); err != nil {
		t.Fatal(err)
	}
	if g := s.Gate(false); g.Decision != DecisionSummarize || g.Selected != 3 || g.Round != 2 {
		t.Fatalf("%+v", g)
	}
}

func hasWarning(s *State, w string) bool {
	for _, x := range s.Warnings {
		if x == w {
			return true
		}
	}
	return false
}

func scoreJSON(cands []Candidate, byTitle map[string]float64) string {
	var parts []string
	for _, c := range cands {
		parts = append(parts, fmt.Sprintf(`{"paper_index":%d,"score":%g,"reason":"because"}`, c.PaperIndex, byTitle[c.Title]))
	}
	return strings.Join(parts, ",")
}

func TestRoundCapIsEnforcedAndNoNewQueryCostsNoRound(t *testing.T) {
	ctx := context.Background()
	fs := &fakeSource{name: "openalex", byText: map[string][]paper.Metadata{}}
	svc := Services{Sources: map[string]source.Source{"openalex": fs}}
	s := newRun(t, nil)
	if _, err := s.Search(ctx, svc, []string{"a"}, nil); err != nil {
		t.Fatal(err)
	}
	rep, err := s.Search(ctx, svc, []string{"A"}, nil)
	if err != nil || !rep.NoNewQueries || s.SearchRound != 1 || fs.calls != 1 {
		t.Fatalf("%+v %v round=%d calls=%d", rep, err, s.SearchRound, fs.calls)
	}
	if _, err := s.Search(ctx, svc, []string{"b"}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Search(ctx, svc, []string{"c"}, nil); !errors.Is(err, ErrRoundCap) {
		t.Fatalf("third round must be refused, got %v", err)
	}
	if s.Gate(false).Decision != DecisionSummarizeAvailable || !hasWarning(s, "INSUFFICIENT_RESULTS_AFTER_MAX_ROUNDS") {
		t.Fatalf("%v", s.Warnings)
	}
}

func TestSearchIsolatesSourceFailureAndDisablesQuotaSources(t *testing.T) {
	ctx := context.Background()
	bad := &fakeSource{name: "openalex", err: source.ErrQuotaExhausted}
	good := &fakeSource{name: "arxiv", byText: map[string][]paper.Metadata{"q": {rec("T", "A", "10.1/t", "x")}}}
	svc := Services{Sources: map[string]source.Source{"openalex": bad, "arxiv": good}}
	s := newRun(t, func(c *Config) { c.Sources = []string{"openalex", "arxiv"}; c.MaxRounds = 3 })
	if _, err := s.Search(ctx, svc, []string{"q"}, nil); err != nil {
		t.Fatal(err)
	}
	if len(s.Raw) != 1 || s.DisabledSources["openalex"] == "" || !hasPrefix(s, "SEARCH_FAILED[openalex]") {
		t.Fatalf("%+v %v", s.DisabledSources, s.Warnings)
	}
	if _, err := s.Search(ctx, svc, []string{"q2"}, nil); err != nil {
		t.Fatal(err)
	}
	if bad.calls != 1 {
		t.Fatalf("a quota-exhausted source must not be called again, calls=%d", bad.calls)
	}
	if !hasPrefix(s, "SOURCE_DISABLED[openalex]") {
		t.Fatal("missing SOURCE_DISABLED warning")
	}
}

func hasPrefix(s *State, p string) bool {
	for _, w := range s.Warnings {
		if strings.HasPrefix(w, p) {
			return true
		}
	}
	return false
}

func TestCandidatesCapWarningAndReuseOfScores(t *testing.T) {
	s := newRun(t, func(c *Config) { c.MaxScoringCandidates = 2 })
	s.AddRaw([]paper.Metadata{rec("a legal", "A", "10.1/1", "x"), rec("b legal", "B", "10.1/2", "x"), rec("c legal", "C", "10.1/3", "x")})
	s.Dedup()
	c1 := s.Candidates()
	if len(c1) != 2 || !hasWarning(s, "SCORING_CANDIDATES_CAPPED: 3 -> 2 new papers") {
		t.Fatalf("%+v %v", c1, s.Warnings)
	}
	if err := s.IngestHeuristic(); err != nil {
		t.Fatal(err)
	}
	c2 := s.Candidates()
	if len(c2) != 1 || hasPrefix(s, "SCORING_CANDIDATES_CAPPED") {
		t.Fatalf("scored papers must not be offered again: %+v %v", c2, s.Warnings)
	}
}

func TestIngestScoresRejectsBadFilesWhole(t *testing.T) {
	newPending := func() *State {
		s := newRun(t, nil)
		s.AddRaw([]paper.Metadata{rec("a", "A", "10.1/1", "x"), rec("b", "B", "10.1/2", "")})
		s.Dedup()
		s.Candidates()
		return s
	}
	bad := map[string]string{
		"missing paper":    `{"scores":[{"paper_index":0,"score":5,"reason":"r"}]}`,
		"out of range":     `{"scores":[{"paper_index":0,"score":11,"reason":"r"},{"paper_index":1,"score":5,"reason":"r"}]}`,
		"negative":         `{"scores":[{"paper_index":0,"score":-1,"reason":"r"},{"paper_index":1,"score":5,"reason":"r"}]}`,
		"empty reason":     `{"scores":[{"paper_index":0,"score":5,"reason":" "},{"paper_index":1,"score":5,"reason":"r"}]}`,
		"duplicate":        `{"scores":[{"paper_index":0,"score":5,"reason":"r"},{"paper_index":0,"score":5,"reason":"r"},{"paper_index":1,"score":5,"reason":"r"}]}`,
		"bad index":        `{"scores":[{"paper_index":7,"score":5,"reason":"r"},{"paper_index":0,"score":5,"reason":"r"},{"paper_index":1,"score":5,"reason":"r"}]}`,
		"wrong paper id":   `{"scores":[{"paper_index":0,"paper_id":"p-nope","score":5,"reason":"r"},{"paper_index":1,"score":5,"reason":"r"}]}`,
		"extra field":      `{"scores":[{"paper_index":0,"score":5,"reason":"r","title":"hacked"},{"paper_index":1,"score":5,"reason":"r"}]}`,
		"top-level extra":  `{"scores":[],"doi":"10.1/x"}`,
		"not json":         `scores`,
		"trailing garbage": `{"scores":[{"paper_index":0,"score":5,"reason":"r"},{"paper_index":1,"score":5,"reason":"r"}]} {}`,
	}
	for name, body := range bad {
		s := newPending()
		err := s.IngestScores([]byte(body))
		if err == nil {
			t.Errorf("%s: accepted", name)
			continue
		}
		if len(s.Scored) != 0 || len(s.Pending) != 2 {
			t.Errorf("%s: partially applied", name)
		}
	}
	s := newPending()
	if err := s.IngestScores([]byte(`{"scores":[{"paper_index":0,"score":5,"reason":"fine"},{"paper_index":1,"score":6.5,"reason":"title only"}]}`)); err != nil {
		t.Fatal(err)
	}
	var titleOnly ScoredEntry
	for _, e := range s.Scored {
		if e.RelevanceScore == 6.5 {
			titleOnly = e
		}
	}
	if titleOnly.ScoredFromAbstract || strings.Join(titleOnly.Warnings, ",") != "SCORED_FROM_TITLE_ONLY,MISSING_ABSTRACT" {
		t.Fatalf("%+v", titleOnly)
	}
	if err := s.IngestScores([]byte(`{"scores":[]}`)); err == nil {
		t.Fatal("scores without pending candidates must fail")
	}
}

func TestLanguageMismatchIsFlagged(t *testing.T) {
	s := newRun(t, func(c *Config) { c.Language = "vi" })
	s.AddRaw([]paper.Metadata{rec("a", "A", "10.1/1", "x")})
	s.Dedup()
	s.Candidates()
	if err := s.IngestScores([]byte(`{"scores":[{"paper_index":0,"score":9,"reason":"明确使用Graph-RAG，高度相关"}]}`)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(s.Scored[0].Warnings, ","), "LANGUAGE_MISMATCH") {
		t.Fatalf("%+v", s.Scored[0].Warnings)
	}
	if scriptMismatch("明确", "zh") || scriptMismatch("Phân tích", "vi") || !scriptMismatch("привет", "en") {
		t.Fatal("scriptMismatch")
	}
}

func selectedRun(t *testing.T) *State {
	t.Helper()
	s := newRun(t, nil)
	s.AddRaw([]paper.Metadata{rec("Legal GraphRAG", "Ann", "10.1/a", "Graph retrieval improves legal answers."), rec("No abstract", "Bo", "10.1/b", "")})
	s.Dedup()
	s.Candidates()
	if err := s.IngestScores([]byte(`{"scores":[{"paper_index":0,"score":9,"reason":"on topic"},{"paper_index":1,"score":7,"reason":"title match"}]}`)); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestIngestSummariesRules(t *testing.T) {
	s := selectedRun(t)
	idWith, idWithout := s.Scored[0].ID, s.Scored[1].ID
	cases := map[string]string{
		"summary without abstract": `{"summaries":[{"paper_id":"` + idWithout + `","summary":"invented"}]}`,
		"unselected paper":         `{"summaries":[{"paper_id":"p-zzz","summary":"x"}]}`,
		"claim without source":     `{"summaries":[],"claims":[{"text":"c","paper_ids":[]}]}`,
		"claim cites unselected":   `{"summaries":[],"claims":[{"text":"c","paper_ids":["p-zzz"]}]}`,
		"quote not in abstract":    `{"summaries":[],"claims":[{"text":"c","paper_ids":["` + idWith + `"],"quote":"quantum chromodynamics"}]}`,
		"unknown field":            `{"summaries":[],"title":"x"}`,
	}
	for name, body := range cases {
		if err := s.IngestSummaries([]byte(body)); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	if s.Scored[0].Summary != "" || len(s.Claims) != 0 {
		t.Fatal("a rejected file must apply nothing")
	}
	ok := `{"summaries":[{"paper_id":"` + idWith + `","summary":"Graph retrieval helps."}],"claims":[{"text":"Graph retrieval helps legal QA","paper_ids":["` + idWith + `"],"quote":"graph retrieval  improves LEGAL answers"}]}`
	if err := s.IngestSummaries([]byte(ok)); err != nil {
		t.Fatal(err)
	}
	if s.Scored[0].Summary == "" || len(s.Claims) != 1 {
		t.Fatal("not applied")
	}
	var ve *ValidationError
	err := s.IngestSummaries([]byte(`{"summaries":[{"paper_id":"p-1","summary":"x"},{"paper_id":"p-2","summary":"y"}]}`))
	if !errors.As(err, &ve) || len(ve.Problems) != 2 {
		t.Fatalf("all problems must be listed: %v", err)
	}
}

func TestVerifyGate(t *testing.T) {
	s := selectedRun(t)
	if r := s.Verify(VerifyOptions{}); r.OK || !strings.Contains(r.ErrorSummary(), "GATE_NOT_RUN") {
		t.Fatalf("%+v", r)
	}
	s.Gate(false)
	r := s.Verify(VerifyOptions{})
	if !r.OK || r.Selected != 2 {
		t.Fatalf("%+v", r)
	}
	if !contains2(r.Warnings, "MISSING_SUMMARY: Legal GraphRAG") {
		t.Fatalf("%v", r.Warnings)
	}
	if r := s.Verify(VerifyOptions{RequireDOICheck: true}); r.OK {
		t.Fatal("DOI check required but not run")
	}
	s.DOIStatus = map[string]string{s.Scored[0].ID: "ok", s.Scored[1].ID: "unresolved"}
	r = s.Verify(VerifyOptions{RequireDOICheck: true})
	if !r.OK || !contains2(r.Warnings, "DOI_UNRESOLVED: No abstract") {
		t.Fatalf("%+v", r)
	}
	// Metadata that lost its provenance blocks export.
	s.Unique[0].MetadataSource = "Made up"
	if r := s.Verify(VerifyOptions{}); r.OK {
		t.Fatal("invalid source passed")
	}
	// A claim citing a paper that is no longer selected blocks export.
	s.Unique[0].MetadataSource = "OpenAlex"
	s.Claims = []Claim{{Text: "c", PaperIDs: []string{"p-gone"}}}
	if r := s.Verify(VerifyOptions{}); r.OK {
		t.Fatal("claim citing an unselected paper passed")
	}
	s.Claims = nil
	s.Pending = []string{"p-x"}
	if r := s.Verify(VerifyOptions{}); r.OK {
		t.Fatal("pending candidates passed")
	}
}

func contains2(xs []string, x string) bool {
	for _, y := range xs {
		if y == x {
			return true
		}
	}
	return false
}

func TestSelectedPapersOrderThresholdAndCap(t *testing.T) {
	s := newRun(t, func(c *Config) { c.MaxPapers = 2; c.MinPapers = 1; c.Threshold = 5 })
	for i, sc := range []float64{4.9, 7, 7, 9} {
		id := "p-" + string(rune('a'+i))
		s.Unique = append(s.Unique, paper.Metadata{ID: id, Title: id})
		s.Scored = append(s.Scored, ScoredEntry{ID: id, Analysis: paper.Analysis{RelevanceScore: sc}})
	}
	got := s.SelectedPapers()
	if len(got) != 2 || got[0].Entry.ID != "p-d" || got[1].Entry.ID != "p-b" {
		t.Fatalf("%+v", got)
	}
}

func TestStateSaveLoadRoundTripAndVersionCheck(t *testing.T) {
	dir := t.TempDir()
	s := selectedRun(t)
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil || len(got.Scored) != 2 || got.UserQuery != s.UserQuery {
		t.Fatalf("%v %+v", err, got)
	}
	s.SchemaVersion = 99
	if err := s.Save(dir); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(dir); err == nil || !strings.Contains(err.Error(), "schema_version") {
		t.Fatalf("unknown version accepted: %v", err)
	}
	if _, err := Load(t.TempDir()); err == nil {
		t.Fatal("missing state accepted")
	}
}

func TestEvaluate(t *testing.T) {
	s := selectedRun(t)
	idWith := s.Scored[0].ID
	s.Claims = []Claim{
		{Text: "g", PaperIDs: []string{idWith}, Quote: "graph retrieval improves legal answers"},
		{Text: "u", PaperIDs: []string{idWith}},
	}
	g := Gold{
		Relevant: []GoldPaper{{DOI: "https://doi.org/10.1/A"}, {Title: "Missed paper"}},
		MergePairs: []GoldPair{
			{A: md("Same title", "Ann", ""), B: md("same title", "ann", ""), Same: true},
			{A: md("One", "Ann", "10.1/x"), B: md("Two", "Bo", "10.1/x"), Same: false}, // a false merge by DOI
		},
	}
	r := s.Evaluate(g)
	if r.Selected != 2 || *r.PrecisionAtSelected != 0.5 || *r.RecallSelected != 0.5 || *r.RecallRetrieved != 0.5 {
		t.Fatalf("%+v", r)
	}
	if *r.DedupFalseMerges != 1 || *r.DedupMissedMerges != 0 {
		t.Fatalf("dedup %d %d", *r.DedupFalseMerges, *r.DedupMissedMerges)
	}
	if r.ClaimsTotal != 2 || r.ClaimsGrounded != 1 || *r.ClaimGrounding != 0.5 || *r.LanguageCompliance != 1 {
		t.Fatalf("%+v", r)
	}
	if _, err := ParseGold([]byte(`{}`)); err == nil {
		t.Fatal("empty gold accepted")
	}
	if _, err := ParseGold([]byte(`{"relevant":[{"doi":"10.1/a","extra":1}]}`)); err == nil {
		t.Fatal("unknown field accepted")
	}
}

func TestSaveLeavesOnlyStateJSONAndReplacesItWhole(t *testing.T) {
	dir := t.TempDir()
	s := selectedRun(t)
	for i := 0; i < 3; i++ {
		s.SearchRound = i
		if err := s.Save(dir); err != nil {
			t.Fatal(err)
		}
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 || entries[0].Name() != StateFile {
		t.Fatalf("directory holds %v, want only %s (no temp files left)", entries, StateFile)
	}
	got, err := Load(dir)
	if err != nil || got.SearchRound != 2 {
		t.Fatalf("%v %+v", err, got)
	}
	// A directory that cannot be created leaves any earlier state untouched and reports the error.
	blocker := filepath.Join(dir, StateFile, "x")
	if err := s.Save(blocker); err == nil {
		t.Fatal("saving under a file must fail")
	}
	if again, err := Load(dir); err != nil || again.SearchRound != 2 {
		t.Fatalf("earlier state damaged: %v", err)
	}
}
