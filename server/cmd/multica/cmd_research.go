package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/litscout"
	"github.com/multica-ai/multica/server/internal/litscout/cache"
	"github.com/multica-ai/multica/server/internal/litscout/source"
)

// `multica research` is the deterministic half of a Research Lab squad's literature workflow.
// It uses no language model. The agents plan queries, score, refine and summarise; each of
// their results goes through one of these commands, which checks it and updates the run's
// state.json (docs/engineering/research-lab/litscout-research-spec.md).

var researchCmd = &cobra.Command{
	Use:   "research",
	Short: "Run the deterministic steps of a literature search",
	Long: `Search scholarly sources, deduplicate, pick scoring candidates, check what the agents
write back, apply the quality gate and the provenance check, and export the result.
Every command prints JSON and keeps the run in <dir>/state.json. None of them calls a model.`,
}

var researchInitCmd = &cobra.Command{
	Use:   "init",
	Short: "Start a run: write state.json with the question and its limits",
	Args:  cobra.NoArgs,
	RunE:  runResearchInit,
}

var researchSearchCmd = &cobra.Command{
	Use:   "search",
	Short: "Run new queries on the run's sources as one search round",
	Long: `Runs every --query that has not been used on every source of the run. One command is one
round, and a round past max_rounds is refused. A query already used is skipped without costing a round.`,
	Args: cobra.NoArgs,
	RunE: runResearchSearch,
}

var researchDedupCmd = &cobra.Command{
	Use:   "dedup",
	Short: "Merge the new records into the unique list",
	Args:  cobra.NoArgs,
	RunE:  runResearchDedup,
}

var researchPrefilterCmd = &cobra.Command{
	Use:   "prefilter",
	Short: "Pick the unscored papers to send to the screener and write candidates.json",
	Args:  cobra.NoArgs,
	RunE:  runResearchPrefilter,
}

var researchIngestScoresCmd = &cobra.Command{
	Use:   "ingest-scores",
	Short: "Check and apply the screener's scores.json (or score by keyword overlap with --heuristic)",
	Args:  cobra.NoArgs,
	RunE:  runResearchIngestScores,
}

var researchGateCmd = &cobra.Command{
	Use:   "gate",
	Short: "Apply the quality gate and print the next step",
	Long: `Prints decision summarize, refine_query, summarize_available or stop_no_new_queries.
The decision is made from counts alone. Follow it; do not decide to continue or stop yourself.`,
	Args: cobra.NoArgs,
	RunE: runResearchGate,
}

var researchIngestSummariesCmd = &cobra.Command{
	Use:   "ingest-summaries",
	Short: "Check and apply the synthesizer's summaries.json (summaries and claims)",
	Args:  cobra.NoArgs,
	RunE:  runResearchIngestSummaries,
}

var researchVerifyCmd = &cobra.Command{
	Use:   "verify",
	Short: "Run the provenance gate; exits non-zero when it fails",
	Args:  cobra.NoArgs,
	RunE:  runResearchVerify,
}

var researchDOICheckCmd = &cobra.Command{
	Use:   "doi-check",
	Short: "Ask Crossref whether each selected paper's DOI exists",
	Args:  cobra.NoArgs,
	RunE:  runResearchDOICheck,
}

var researchChaseCmd = &cobra.Command{
	Use:   "chase",
	Short: "Follow citations from the top selected papers (spends one search round)",
	Args:  cobra.NoArgs,
	RunE:  runResearchChase,
}

var researchFulltextCmd = &cobra.Command{
	Use:   "fulltext",
	Short: "Extract the text of open-access PDFs of the selected papers (only if the run allows it)",
	Args:  cobra.NoArgs,
	RunE:  runResearchFulltext,
}

var researchExportCmd = &cobra.Command{
	Use:   "export",
	Short: "Write papers.csv and research_digest.md after the provenance gate passes",
	Args:  cobra.NoArgs,
	RunE:  runResearchExport,
}

var researchEvalCmd = &cobra.Command{
	Use:   "eval",
	Short: "Measure a finished run against a human-labelled gold file",
	Args:  cobra.NoArgs,
	RunE:  runResearchEval,
}

var researchStatusCmd = &cobra.Command{
	Use:   "status",
	Short: "Print the counts of a run",
	Args:  cobra.NoArgs,
	RunE:  runResearchStatus,
}

func init() {
	for _, c := range []*cobra.Command{
		researchInitCmd, researchSearchCmd, researchDedupCmd, researchPrefilterCmd, researchIngestScoresCmd,
		researchGateCmd, researchIngestSummariesCmd, researchVerifyCmd, researchDOICheckCmd, researchChaseCmd,
		researchFulltextCmd, researchExportCmd, researchEvalCmd, researchStatusCmd,
	} {
		c.Flags().String("dir", "research-run", "The run directory (holds state.json)")
		researchCmd.AddCommand(c)
	}
	for _, c := range []*cobra.Command{researchSearchCmd, researchChaseCmd, researchDOICheckCmd, researchFulltextCmd} {
		c.Flags().Bool("no-cache", false, "Do not read or write the response cache")
		c.Flags().Duration("cache-ttl", 24*time.Hour, "How long a cached response stays valid")
		c.Flags().String("cache-dir", "", "Cache directory (default: the user cache directory)")
		c.Flags().Bool("no-remote-credentials", false, "Do not fetch source keys from the workspace; use environment variables only")
	}

	d := litscout.DefaultConfig()
	researchInitCmd.Flags().String("query", "", "The research question (required)")
	researchInitCmd.Flags().Int("from-year", 0, "Only papers from this year on")
	researchInitCmd.Flags().Int("min-papers", d.MinPapers, "Papers the quality gate wants before it stops refining")
	researchInitCmd.Flags().Int("max-papers", d.MaxPapers, "Most papers in the result")
	researchInitCmd.Flags().Int("max-rounds", d.MaxRounds, fmt.Sprintf("Hard limit of search rounds (1-%d)", litscout.MaxRoundsLimit))
	researchInitCmd.Flags().Int("results-per-query", d.ResultsPerQuery, "Results requested per query and source (1-100)")
	researchInitCmd.Flags().Int("max-scoring-candidates", 0, "New papers offered for scoring per round (default: max-papers)")
	researchInitCmd.Flags().Float64("threshold", d.Threshold, "Relevance score a paper needs, on a 0-10 scale")
	researchInitCmd.Flags().String("language", d.Language, "Language of the report text (en, vi, ...)")
	researchInitCmd.Flags().StringSlice("sources", d.Sources, "Sources: openalex, crossref, arxiv, semanticscholar")
	researchInitCmd.Flags().StringSlice("exclude-type", d.ExcludeTypes, "Document types to drop (OpenAlex type)")
	researchInitCmd.Flags().Bool("allow-fulltext", false, "Allow extracting open-access PDF text")
	researchInitCmd.Flags().Bool("force", false, "Overwrite an existing state.json")

	researchSearchCmd.Flags().StringArray("query", nil, "A search query; repeat the flag for several (required)")
	researchSearchCmd.Flags().StringSlice("source", nil, "Search only these sources (a subset of the run's)")
	researchPrefilterCmd.Flags().String("out", "", "Where to write candidates.json (default: <dir>/candidates.json)")
	researchIngestScoresCmd.Flags().String("file", "", "The screener's scores.json")
	researchIngestScoresCmd.Flags().Bool("heuristic", false, "Score the pending candidates by keyword overlap instead of a file")
	researchGateCmd.Flags().Bool("no-new-queries", false, "The refinement produced no new query")
	researchIngestSummariesCmd.Flags().String("file", "", "The synthesizer's summaries.json (required)")
	researchVerifyCmd.Flags().Bool("require-doi-check", false, "Fail when a selected paper's DOI was not checked")
	researchChaseCmd.Flags().Int("top", 5, "Chase from this many top selected papers")
	researchChaseCmd.Flags().Int("per-paper", 10, "Works to fetch per paper and direction")
	researchExportCmd.Flags().String("out", "", "Output directory (default: the run directory)")
	researchExportCmd.Flags().Bool("bom", false, "Start papers.csv with a UTF-8 byte order mark (for spreadsheets)")
	researchExportCmd.Flags().Bool("require-doi-check", false, "Fail when a selected paper's DOI was not checked")
	researchEvalCmd.Flags().String("gold", "", "The gold file (required)")

	rootCmd.AddCommand(researchCmd)
}

func researchDir(cmd *cobra.Command) string {
	dir, _ := cmd.Flags().GetString("dir")
	return dir
}

func loadRun(cmd *cobra.Command) (*litscout.State, string, error) {
	dir := researchDir(cmd)
	s, err := litscout.Load(dir)
	if err != nil {
		return nil, dir, fmt.Errorf("%w (start a run with `multica research init`)", err)
	}
	return s, dir, nil
}

func runResearchInit(cmd *cobra.Command, _ []string) error {
	f := cmd.Flags()
	dir := researchDir(cmd)
	query, _ := f.GetString("query")
	cfg := litscout.Config{}
	cfg.FromYear, _ = f.GetInt("from-year")
	cfg.MinPapers, _ = f.GetInt("min-papers")
	cfg.MaxPapers, _ = f.GetInt("max-papers")
	cfg.MaxRounds, _ = f.GetInt("max-rounds")
	cfg.ResultsPerQuery, _ = f.GetInt("results-per-query")
	cfg.MaxScoringCandidates, _ = f.GetInt("max-scoring-candidates")
	cfg.Threshold, _ = f.GetFloat64("threshold")
	cfg.Language, _ = f.GetString("language")
	cfg.Sources, _ = f.GetStringSlice("sources")
	cfg.ExcludeTypes, _ = f.GetStringSlice("exclude-type")
	cfg.AllowFullText, _ = f.GetBool("allow-fulltext")
	for i, s := range cfg.Sources {
		cfg.Sources[i] = strings.ToLower(strings.TrimSpace(s))
	}
	state, err := litscout.Init(query, cfg)
	if err != nil {
		return err
	}
	if force, _ := f.GetBool("force"); !force {
		if _, err := os.Stat(filepath.Join(dir, litscout.StateFile)); err == nil {
			return fmt.Errorf("%s already holds a run; use --force to replace it", dir)
		}
	}
	if err := state.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), map[string]any{"dir": dir, "state": filepath.Join(dir, litscout.StateFile), "config": state.Config})
}

// researchCredentials are the source keys and the contact address. The environment wins; a missing
// value is then asked of the workspace, where an owner or admin set it on the Research settings page.
type researchCredentials struct {
	OpenAlexKey string `json:"openalex_api_key"`
	S2Key       string `json:"semantic_scholar_api_key"`
	Email       string `json:"contact_email"`
}

func firstEnv(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

func resolveResearchCredentials(cmd *cobra.Command) researchCredentials {
	c := researchCredentials{
		OpenAlexKey: firstEnv("OPENALEX_API_KEY"),
		S2Key:       firstEnv("SEMANTIC_SCHOLAR_API_KEY", "S2_API_KEY"),
		Email:       firstEnv("MULTICA_RESEARCH_EMAIL", "CROSSREF_MAILTO", "UNPAYWALL_EMAIL"),
	}
	if skip, _ := cmd.Flags().GetBool("no-remote-credentials"); skip || (c.OpenAlexKey != "" && c.S2Key != "" && c.Email != "") {
		return c
	}
	client, err := newAPIClient(cmd)
	if err != nil {
		return c
	}
	ctx, cancel := context.WithTimeout(commandContext(cmd), 8*time.Second)
	defer cancel()
	var remote researchCredentials
	if err := client.GetJSON(ctx, "/api/research/credentials", &remote); err != nil {
		var he *cli.HTTPError
		if errors.As(err, &he) && he.StatusCode == http.StatusForbidden {
			fmt.Fprintln(cmd.ErrOrStderr(), "research: only an owner, an admin or an agent task can read the workspace's source keys; using environment variables only (OPENALEX_API_KEY, SEMANTIC_SCHOLAR_API_KEY, MULTICA_RESEARCH_EMAIL)")
		}
		return c
	}
	if c.OpenAlexKey == "" {
		c.OpenAlexKey = remote.OpenAlexKey
	}
	if c.S2Key == "" {
		c.S2Key = remote.S2Key
	}
	if c.Email == "" {
		c.Email = remote.Email
	}
	return c
}

// newResearchServices builds the network services; tests replace it with fakes.
var newResearchServices = researchServices

func researchServices(cmd *cobra.Command) litscout.Services {
	creds := resolveResearchCredentials(cmd)
	var cch *cache.Cache
	if no, _ := cmd.Flags().GetBool("no-cache"); !no {
		dir, _ := cmd.Flags().GetString("cache-dir")
		if dir == "" {
			dir, _ = cache.DefaultDir()
		}
		ttl, _ := cmd.Flags().GetDuration("cache-ttl")
		if dir != "" {
			cch = &cache.Cache{Dir: dir, TTL: ttl}
		}
	}
	client := func(gap time.Duration) *source.Client { return &source.Client{Cache: cch, MinInterval: gap} }
	s2gap := 1100 * time.Millisecond
	if creds.S2Key == "" {
		s2gap = 3 * time.Second
	}
	oa := &source.OpenAlex{Client: client(0), APIKey: creds.OpenAlexKey}
	cr := &source.Crossref{Client: client(0), Mailto: creds.Email}
	ax := &source.Arxiv{Client: client(3 * time.Second)}
	s2 := &source.SemanticScholar{Client: client(s2gap), APIKey: creds.S2Key}
	return litscout.Services{
		Sources:  map[string]source.Source{"openalex": oa, "crossref": cr, "arxiv": ax, "semanticscholar": s2},
		OpenAlex: oa, Crossref: cr,
		Unpaywall: &source.Unpaywall{Client: client(0), Email: creds.Email},
		PDF:       &litscout.PDFFetcher{},
	}
}

func runResearchSearch(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	queries, _ := cmd.Flags().GetStringArray("query")
	if len(queries) == 0 {
		return errors.New("at least one --query is required")
	}
	only, _ := cmd.Flags().GetStringSlice("source")
	allowed := map[string]bool{}
	for _, n := range s.Config.Sources {
		allowed[n] = true
	}
	for i, n := range only {
		n = strings.ToLower(strings.TrimSpace(n))
		if !allowed[n] {
			return fmt.Errorf("source %q is not one of this run's sources (%s)", n, strings.Join(s.Config.Sources, ", "))
		}
		only[i] = n
	}
	rep, err := s.Search(commandContext(cmd), newResearchServices(cmd), queries, only)
	if errors.Is(err, litscout.ErrRoundCap) {
		cmd.SilenceUsage = true
	}
	if err != nil {
		return err
	}
	if serr := s.Save(dir); serr != nil {
		return serr
	}
	return writeJSON(cmd.OutOrStdout(), rep)
}

func runResearchDedup(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	res := s.Dedup()
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), res)
}

func runResearchPrefilter(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	cands := s.Candidates()
	out, _ := cmd.Flags().GetString("out")
	if out == "" {
		out = filepath.Join(dir, "candidates.json")
	}
	b, err := litscout.MarshalIndent(map[string]any{"query": s.UserQuery, "language": s.Config.Language, "candidates": cands})
	if err != nil {
		return err
	}
	if err := os.WriteFile(out, append(b, '\n'), 0o644); err != nil {
		return err
	}
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), map[string]any{"candidates": len(cands), "file": out})
}

func runResearchIngestScores(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	if heuristic, _ := cmd.Flags().GetBool("heuristic"); heuristic {
		err = s.IngestHeuristic()
	} else {
		file, _ := cmd.Flags().GetString("file")
		if file == "" {
			return errors.New("--file is required (or use --heuristic)")
		}
		var data []byte
		if data, err = os.ReadFile(file); err != nil {
			return fmt.Errorf("read scores: %w", err)
		}
		err = s.IngestScores(data)
	}
	if err != nil {
		cmd.SilenceUsage = true
		return err
	}
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), map[string]any{"scored": len(s.Scored), "selected": len(s.SelectedPapers())})
}

func runResearchGate(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	noNew, _ := cmd.Flags().GetBool("no-new-queries")
	res := s.Gate(noNew)
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), res)
}

func runResearchIngestSummaries(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	file, _ := cmd.Flags().GetString("file")
	if file == "" {
		return errors.New("--file is required")
	}
	data, err := os.ReadFile(file)
	if err != nil {
		return fmt.Errorf("read summaries: %w", err)
	}
	if err := s.IngestSummaries(data); err != nil {
		cmd.SilenceUsage = true
		return err
	}
	if err := s.Save(dir); err != nil {
		return err
	}
	withSummary := 0
	for _, x := range s.SelectedPapers() {
		if x.Entry.Summary != "" {
			withSummary++
		}
	}
	return writeJSON(cmd.OutOrStdout(), map[string]any{"summaries": withSummary, "claims": len(s.Claims)})
}

func runResearchVerify(cmd *cobra.Command, _ []string) error {
	s, _, err := loadRun(cmd)
	if err != nil {
		return err
	}
	req, _ := cmd.Flags().GetBool("require-doi-check")
	res := s.Verify(litscout.VerifyOptions{RequireDOICheck: req})
	if err := writeJSON(cmd.OutOrStdout(), res); err != nil {
		return err
	}
	if !res.OK {
		cmd.SilenceUsage = true
		return fmt.Errorf("provenance validation failed: %s", res.ErrorSummary())
	}
	return nil
}

func runResearchDOICheck(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	rep, err := s.CheckDOIs(commandContext(cmd), newResearchServices(cmd))
	if err != nil {
		return err
	}
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), rep)
}

func runResearchChase(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	top, _ := cmd.Flags().GetInt("top")
	per, _ := cmd.Flags().GetInt("per-paper")
	rep, err := s.Chase(commandContext(cmd), newResearchServices(cmd), top, per)
	if err != nil {
		if errors.Is(err, litscout.ErrRoundCap) {
			cmd.SilenceUsage = true
		}
		return err
	}
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), rep)
}

func runResearchFulltext(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	rep, err := s.FetchFullText(commandContext(cmd), newResearchServices(cmd), dir)
	if err != nil {
		return err
	}
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), rep)
}

func runResearchExport(cmd *cobra.Command, _ []string) error {
	s, dir, err := loadRun(cmd)
	if err != nil {
		return err
	}
	out, _ := cmd.Flags().GetString("out")
	if out == "" {
		out = dir
	}
	bom, _ := cmd.Flags().GetBool("bom")
	req, _ := cmd.Flags().GetBool("require-doi-check")
	res, files, err := s.Export(out, litscout.ExportOptions{BOM: bom}, litscout.VerifyOptions{RequireDOICheck: req})
	if err != nil {
		cmd.SilenceUsage = true
		_ = writeJSON(cmd.OutOrStdout(), res)
		return err
	}
	if err := s.Save(dir); err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), map[string]any{"files": files, "selected": res.Selected, "warnings": res.Warnings})
}

func runResearchEval(cmd *cobra.Command, _ []string) error {
	s, _, err := loadRun(cmd)
	if err != nil {
		return err
	}
	path, _ := cmd.Flags().GetString("gold")
	if path == "" {
		return errors.New("--gold is required")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("read gold: %w", err)
	}
	gold, err := litscout.ParseGold(raw)
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), s.Evaluate(gold))
}

func runResearchStatus(cmd *cobra.Command, _ []string) error {
	s, _, err := loadRun(cmd)
	if err != nil {
		return err
	}
	return writeJSON(cmd.OutOrStdout(), map[string]any{
		"query": s.UserQuery, "config": s.Config, "search_round": s.SearchRound, "raw_papers": len(s.Raw),
		"unique_papers": len(s.Unique), "scored_papers": len(s.Scored), "pending_candidates": len(s.Pending),
		"selected_papers": len(s.SelectedPapers()), "quality_gate_decision": s.QualityGateDecision,
		"disabled_sources": s.DisabledSources, "warnings": s.Warnings,
	})
}
