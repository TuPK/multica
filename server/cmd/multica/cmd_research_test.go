package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/multica-ai/multica/server/internal/litscout"
	"github.com/multica-ai/multica/server/internal/litscout/source"
)

// resetResearchFlags returns every flag of the research commands to its default, because cobra keeps
// flag values between two Execute calls on the same command objects.
func resetResearchFlags(t *testing.T) {
	t.Helper()
	reset := func(c *cobra.Command) {
		c.Flags().VisitAll(func(f *pflag.Flag) {
			if sv, ok := f.Value.(pflag.SliceValue); ok {
				_ = sv.Replace(nil)
				if f.DefValue != "[]" {
					_ = sv.Replace(strings.Split(strings.Trim(f.DefValue, "[]"), ","))
				}
			} else {
				_ = f.Value.Set(f.DefValue)
			}
			f.Changed = false
		})
	}
	reset(researchCmd)
	for _, c := range researchCmd.Commands() {
		reset(c)
	}
}

func fakeResearchServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/oa/works"):
			_, _ = w.Write([]byte(`{"results":[
			 {"id":"https://openalex.org/W1","doi":"https://doi.org/10.1/a","title":"Vietnamese legal GraphRAG","publication_date":"2025-01-01","type":"article",
			  "abstract_inverted_index":{"Graph":[0],"retrieval":[1],"improves":[2],"legal":[3],"answers":[4]},"authorships":[{"author":{"display_name":"Ann"}}]},
			 {"id":"https://openalex.org/W2","doi":"https://doi.org/10.1/b","title":"Cooking with graphs","publication_date":"2024-01-01","type":"article",
			  "abstract_inverted_index":{"Recipes":[0]},"authorships":[{"author":{"display_name":"Bo"}}]},
			 {"id":"https://openalex.org/W3","title":"Proceedings front matter","is_paratext":true,"authorships":[{"author":{"display_name":"Ed"}}]}
			]}`))
		case strings.HasPrefix(r.URL.Path, "/cr/works/"):
			if strings.Contains(r.URL.Path, "10.1/a") {
				_, _ = w.Write([]byte(`{"message":{"DOI":"10.1/a","title":["T"]}}`))
				return
			}
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func runResearch(t *testing.T, args ...string) (string, error) {
	t.Helper()
	resetResearchFlags(t)
	var out bytes.Buffer
	rootCmd.SetOut(&out)
	rootCmd.SetErr(&bytes.Buffer{})
	rootCmd.SetArgs(append([]string{"research"}, args...))
	t.Cleanup(func() { rootCmd.SetOut(nil); rootCmd.SetErr(nil); rootCmd.SetArgs(nil) })
	err := rootCmd.Execute()
	return out.String(), err
}

func TestResearchCommandsRunAWholeLiteratureRunOffline(t *testing.T) {
	srv := fakeResearchServer(t)
	old := newResearchServices
	newResearchServices = func(*cobra.Command) litscout.Services {
		c := func() *source.Client {
			return &source.Client{MaxRetries: -1, Sleep: func(context.Context, time.Duration) error { return nil }}
		}
		oa := &source.OpenAlex{Client: c(), BaseURL: srv.URL + "/oa"}
		cr := &source.Crossref{Client: c(), BaseURL: srv.URL + "/cr"}
		return litscout.Services{Sources: map[string]source.Source{"openalex": oa}, OpenAlex: oa, Crossref: cr}
	}
	t.Cleanup(func() { newResearchServices = old })

	dir := filepath.Join(t.TempDir(), "run")
	step := func(args ...string) string {
		t.Helper()
		out, err := runResearch(t, append(args, "--dir", dir)...)
		if err != nil {
			t.Fatalf("research %s: %v\n%s", strings.Join(args, " "), err, out)
		}
		return out
	}

	step("init", "--query", "Vietnamese legal GraphRAG", "--min-papers", "1", "--max-papers", "5", "--max-rounds", "2")
	if _, err := runResearch(t, "init", "--query", "again", "--dir", dir); err == nil {
		t.Fatal("init must not overwrite a run without --force")
	}

	out := step("search", "--query", "Vietnamese legal GraphRAG")
	if !strings.Contains(out, `"count": 3`) {
		t.Fatalf("search output: %s", out)
	}
	if out := step("dedup"); !strings.Contains(out, `"unique_papers": 2`) || !strings.Contains(out, `"excluded_by_type": 1`) {
		t.Fatalf("dedup output: %s", out)
	}
	step("prefilter")

	var cands struct {
		Candidates []litscout.Candidate `json:"candidates"`
	}
	raw, err := os.ReadFile(filepath.Join(dir, "candidates.json"))
	if err != nil || json.Unmarshal(raw, &cands) != nil || len(cands.Candidates) != 2 {
		t.Fatalf("candidates.json: %v %s", err, raw)
	}
	scores := filepath.Join(dir, "scores.json")
	var items []string
	for _, c := range cands.Candidates {
		score := "2"
		if strings.Contains(c.Title, "GraphRAG") {
			score = "9"
		}
		items = append(items, `{"paper_index":`+string(rune('0'+c.PaperIndex))+`,"score":`+score+`,"reason":"on topic"}`)
	}
	must(t, os.WriteFile(scores, []byte(`{"scores":[`+strings.Join(items, ",")+`]}`), 0o600))

	if _, err := runResearch(t, "ingest-scores", "--dir", dir, "--file", filepath.Join(dir, "missing.json")); err == nil {
		t.Fatal("a missing scores file must fail")
	}
	step("ingest-scores", "--file", scores)

	// The provenance gate refuses to pass before the quality gate ran.
	if out, err := runResearch(t, "verify", "--dir", dir); err == nil || !strings.Contains(out, "GATE_NOT_RUN") {
		t.Fatalf("verify before gate: %v %s", err, out)
	}
	if out := step("gate"); !strings.Contains(out, `"decision": "summarize"`) {
		t.Fatalf("gate output: %s", out)
	}

	state, err := litscout.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	sel := state.SelectedPapers()
	if len(sel) != 1 {
		t.Fatalf("selected %d", len(sel))
	}
	summaries := filepath.Join(dir, "summaries.json")
	must(t, os.WriteFile(summaries, []byte(`{"summaries":[{"paper_id":"`+sel[0].Entry.ID+`","summary":"Graph retrieval helps."}],"claims":[{"text":"Graph retrieval helps legal QA","paper_ids":["`+sel[0].Entry.ID+`"],"quote":"Graph retrieval improves legal answers"}]}`), 0o600))
	step("ingest-summaries", "--file", summaries)
	if out := step("doi-check"); !strings.Contains(out, `"ok": 1`) {
		t.Fatalf("doi-check output: %s", out)
	}
	step("verify", "--require-doi-check")
	if out := step("export", "--bom"); !strings.Contains(out, "papers.csv") {
		t.Fatalf("export output: %s", out)
	}
	csvBytes, err := os.ReadFile(filepath.Join(dir, "papers.csv"))
	if err != nil || !bytes.HasPrefix(csvBytes, []byte("\ufeffrank,title")) || !strings.Contains(string(csvBytes), "Vietnamese legal GraphRAG") {
		t.Fatalf("papers.csv: %v %q", err, csvBytes)
	}
	md, _ := os.ReadFile(filepath.Join(dir, "research_digest.md"))
	if !strings.Contains(string(md), "- Graph retrieval helps legal QA [1]") || !strings.Contains(string(md), "DOI confirmed by Crossref.") {
		t.Fatalf("digest: %s", md)
	}

	gold := filepath.Join(dir, "gold.json")
	must(t, os.WriteFile(gold, []byte(`{"relevant":[{"doi":"10.1/a"},{"doi":"10.9/missing"}]}`), 0o600))
	if out := step("eval", "--gold", gold); !strings.Contains(out, `"recall_selected": 0.5`) || !strings.Contains(out, `"precision_at_selected": 1`) {
		t.Fatalf("eval output: %s", out)
	}
	if out := step("status"); !strings.Contains(out, `"search_round": 1`) {
		t.Fatalf("status: %s", out)
	}
}

func TestResearchSearchRefusesPastMaxRoundsAndUnknownSources(t *testing.T) {
	srv := fakeResearchServer(t)
	old := newResearchServices
	newResearchServices = func(*cobra.Command) litscout.Services {
		oa := &source.OpenAlex{Client: &source.Client{MaxRetries: -1}, BaseURL: srv.URL + "/oa"}
		return litscout.Services{Sources: map[string]source.Source{"openalex": oa}, OpenAlex: oa}
	}
	t.Cleanup(func() { newResearchServices = old })
	dir := filepath.Join(t.TempDir(), "run")
	if _, err := runResearch(t, "init", "--query", "q", "--max-rounds", "1", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runResearch(t, "search", "--query", "a", "--dir", dir); err != nil {
		t.Fatal(err)
	}
	if _, err := runResearch(t, "search", "--query", "b", "--dir", dir); err == nil || !strings.Contains(err.Error(), "max_rounds") {
		t.Fatalf("second round must be refused: %v", err)
	}
	if _, err := runResearch(t, "search", "--query", "c", "--source", "arxiv", "--dir", dir); err == nil || !strings.Contains(err.Error(), "not one of this run's sources") {
		t.Fatalf("source outside the run: %v", err)
	}
	// The failed calls left the run as it was.
	s, err := litscout.Load(dir)
	if err != nil || s.SearchRound != 1 || len(s.UsedQueries) != 1 {
		t.Fatalf("%v %+v", err, s)
	}
}

func TestResearchInitRejectsBadLimits(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "run")
	for _, args := range [][]string{
		{"init", "--query", "q", "--max-rounds", "9"},
		{"init", "--query", "q", "--min-papers", "50", "--max-papers", "5"},
		{"init", "--query", "q", "--sources", "bing"},
		{"init", "--query", ""},
	} {
		if _, err := runResearch(t, append(args, "--dir", dir)...); err == nil {
			t.Errorf("accepted: %v", args)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, "state.json")); err == nil {
		t.Fatal("a rejected init wrote a state")
	}
}

func credentialsCmd() *cobra.Command {
	c := &cobra.Command{Use: "x"}
	c.Flags().Bool("no-remote-credentials", false, "")
	return c
}

func TestResearchCredentialsComeFromTheWorkspaceUnlessTheEnvironmentHasThem(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		if r.URL.Path != "/api/research/credentials" || r.Header.Get("Authorization") != "Bearer mat_test" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		_, _ = w.Write([]byte(`{"openalex_api_key":"REMOTE-OA","semantic_scholar_api_key":"REMOTE-S2","contact_email":"lab@example.org"}`))
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_TOKEN", "mat_test")
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	for _, k := range []string{"OPENALEX_API_KEY", "SEMANTIC_SCHOLAR_API_KEY", "S2_API_KEY", "MULTICA_RESEARCH_EMAIL", "CROSSREF_MAILTO", "UNPAYWALL_EMAIL"} {
		t.Setenv(k, "")
	}

	got := resolveResearchCredentials(credentialsCmd())
	if got.OpenAlexKey != "REMOTE-OA" || got.S2Key != "REMOTE-S2" || got.Email != "lab@example.org" {
		t.Fatalf("%+v", got)
	}

	// The environment wins over the workspace, field by field.
	t.Setenv("OPENALEX_API_KEY", "ENV-OA")
	got = resolveResearchCredentials(credentialsCmd())
	if got.OpenAlexKey != "ENV-OA" || got.S2Key != "REMOTE-S2" {
		t.Fatalf("%+v", got)
	}

	// --no-remote-credentials never asks.
	before := hits
	c := credentialsCmd()
	_ = c.Flags().Set("no-remote-credentials", "true")
	t.Setenv("OPENALEX_API_KEY", "")
	if got := resolveResearchCredentials(c); got.OpenAlexKey != "" || hits != before {
		t.Fatalf("%+v hits %d->%d", got, before, hits)
	}
}

func TestResearchCredentialsForAPlainMemberFallBackToTheEnvironmentAndSayWhy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"error":"only an owner, an admin or an agent task may read research credentials"}`))
	}))
	defer srv.Close()
	t.Setenv("MULTICA_SERVER_URL", srv.URL)
	t.Setenv("MULTICA_TOKEN", "mul_member")
	t.Setenv("MULTICA_WORKSPACE_ID", "ws-1")
	t.Setenv("OPENALEX_API_KEY", "")
	t.Setenv("SEMANTIC_SCHOLAR_API_KEY", "")
	t.Setenv("MULTICA_RESEARCH_EMAIL", "")

	var stderr bytes.Buffer
	c := credentialsCmd()
	c.SetErr(&stderr)
	got := resolveResearchCredentials(c)
	if got.OpenAlexKey != "" || got.S2Key != "" {
		t.Fatalf("a refused request must not produce keys: %+v", got)
	}
	if !strings.Contains(stderr.String(), "using environment variables only") {
		t.Fatalf("stderr = %q", stderr.String())
	}
}

// The skills and the squad instructions name these commands; packages/core/squads/research-lab.test.ts
// reads this list from cmd_research.go too, so a command added or renamed here fails there until the
// skills are updated.
func TestResearchCommandSet(t *testing.T) {
	want := []string{"init", "search", "dedup", "prefilter", "ingest-scores", "gate", "ingest-summaries", "verify",
		"doi-check", "chase", "fulltext", "export", "eval", "status"}
	var got []string
	for _, c := range researchCmd.Commands() {
		got = append(got, c.Name())
		if c.Short == "" || c.Flags().Lookup("dir") == nil {
			t.Errorf("%s: needs a Short description and a --dir flag", c.Name())
		}
	}
	sort.Strings(want)
	sort.Strings(got)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("commands = %v, want %v", got, want)
	}
}
