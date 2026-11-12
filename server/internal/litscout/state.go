// Package litscout is the deterministic half of the Research Lab's literature workflow. It keeps
// the run state of one research question, searches scholarly sources, deduplicates, ranks
// candidates, validates what agents write back, applies the quality gate and provenance checks,
// and exports the result. It never calls a language model: planning, scoring, refining and
// summarising are done by agents, and this package checks their output.
package litscout

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// SchemaVersion is the version of state.json this build reads and writes.
const SchemaVersion = 1

// MaxRoundsLimit is the largest max_rounds a run may ask for.
const MaxRoundsLimit = 5

// StateFile is the name of the state file inside a run directory.
const StateFile = "state.json"

// KnownSources are the source names a run may list in Config.Sources.
var KnownSources = map[string]bool{"openalex": true, "crossref": true, "arxiv": true, "semanticscholar": true}

// Config holds the limits of one run. Every loop and every cost is bounded by a field here.
type Config struct {
	FromYear             int      `json:"from_year,omitempty"`
	MinPapers            int      `json:"min_papers"`
	MaxPapers            int      `json:"max_papers"`
	MaxRounds            int      `json:"max_rounds"`
	ResultsPerQuery      int      `json:"results_per_query"`
	MaxScoringCandidates int      `json:"max_scoring_candidates"`
	Threshold            float64  `json:"threshold"`
	Language             string   `json:"language"`
	Sources              []string `json:"sources"`
	ExcludeTypes         []string `json:"exclude_types"`
	AllowFullText        bool     `json:"allow_fulltext"`
}

// DefaultConfig returns LitScout's defaults.
func DefaultConfig() Config {
	return Config{
		MinPapers: 10, MaxPapers: 20, MaxRounds: 3, ResultsPerQuery: 10, Threshold: 6,
		Language: "en", Sources: []string{"openalex"}, ExcludeTypes: []string{"paratext", "erratum"},
	}
}

// Validate fills derived defaults and rejects an unusable configuration.
func (c *Config) Validate() error {
	if c.MaxScoringCandidates <= 0 {
		c.MaxScoringCandidates = max(c.MaxPapers, 1)
	}
	switch {
	case c.MinPapers < 1 || c.MaxPapers < 1:
		return errors.New("min_papers and max_papers must be at least 1")
	case c.MinPapers > c.MaxPapers:
		return errors.New("min_papers must not exceed max_papers")
	case c.MaxRounds < 1 || c.MaxRounds > MaxRoundsLimit:
		return fmt.Errorf("max_rounds must be between 1 and %d", MaxRoundsLimit)
	case c.ResultsPerQuery < 1 || c.ResultsPerQuery > 100:
		return errors.New("results_per_query must be between 1 and 100")
	case c.Threshold < 0 || c.Threshold > 10:
		return errors.New("threshold must be between 0 and 10")
	case strings.TrimSpace(c.Language) == "":
		return errors.New("language must not be empty")
	case len(c.Sources) == 0:
		return errors.New("at least one source is required")
	}
	for _, s := range c.Sources {
		if !KnownSources[s] {
			return fmt.Errorf("unknown source %q", s)
		}
	}
	return nil
}

// ScoredEntry is the analysis an agent wrote for one paper.
type ScoredEntry struct {
	ID string `json:"id"`
	paper.Analysis
	Warnings []string `json:"warnings,omitempty"`
}

// Claim is one statement of a synthesis, tied to the papers that support it. Quote, when present,
// must appear in the abstract of one of the cited papers.
type Claim struct {
	Text     string   `json:"text"`
	PaperIDs []string `json:"paper_ids"`
	Quote    string   `json:"quote,omitempty"`
}

// FulltextInfo records what happened to the open-access full text of one paper.
type FulltextInfo struct {
	Status string `json:"status"` // ok, not_open_access, failed
	Path   string `json:"path,omitempty"`
	Source string `json:"source,omitempty"`
	Chars  int    `json:"chars,omitempty"`
}

// State is everything one run knows. It lives in the run directory as state.json. It is not a cache
// and it is not a database.
type State struct {
	SchemaVersion       int                     `json:"schema_version"`
	UserQuery           string                  `json:"user_query"`
	Config              Config                  `json:"config"`
	UsedQueries         []string                `json:"used_queries"`
	Raw                 []paper.Metadata        `json:"raw_papers"`
	RawMerged           int                     `json:"raw_merged"`
	Unique              []paper.Metadata        `json:"unique_papers"`
	Scored              []ScoredEntry           `json:"scored_papers"`
	Pending             []string                `json:"pending_candidates,omitempty"`
	SearchRound         int                     `json:"search_round"`
	Warnings            []string                `json:"warnings"`
	QualityGateDecision string                  `json:"quality_gate_decision,omitempty"`
	DisabledSources     map[string]string       `json:"disabled_sources,omitempty"`
	DOIStatus           map[string]string       `json:"doi_status,omitempty"`
	Fulltext            map[string]FulltextInfo `json:"fulltext,omitempty"`
	Claims              []Claim                 `json:"claims,omitempty"`
	OutputFiles         []string                `json:"output_files,omitempty"`
}

// Init creates a state for query. It does not write it.
func Init(query string, cfg Config) (*State, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query must not be empty")
	}
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &State{SchemaVersion: SchemaVersion, UserQuery: query, Config: cfg}, nil
}

// Load reads dir/state.json and refuses a schema version it does not know.
func Load(dir string) (*State, error) {
	b, err := os.ReadFile(filepath.Join(dir, StateFile))
	if err != nil {
		return nil, fmt.Errorf("read state: %w", err)
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, fmt.Errorf("parse state: %w", err)
	}
	if s.SchemaVersion != SchemaVersion {
		return nil, fmt.Errorf("state schema_version %d is not supported (this build reads %d)", s.SchemaVersion, SchemaVersion)
	}
	return &s, nil
}

// Save writes dir/state.json atomically: a temp file, then a rename.
func (s *State) Save(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	b, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, "state-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	if _, err := tmp.Write(append(b, '\n')); err != nil {
		tmp.Close()
		os.Remove(name)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(name)
		return err
	}
	if err := os.Rename(name, filepath.Join(dir, StateFile)); err != nil {
		os.Remove(name)
		return err
	}
	return nil
}

// AddWarning appends a warning once.
func (s *State) AddWarning(w string) {
	for _, x := range s.Warnings {
		if x == w {
			return
		}
	}
	s.Warnings = append(s.Warnings, w)
}

func (s *State) replaceWarnings(prefix string, with []string) {
	kept := s.Warnings[:0:0]
	for _, w := range s.Warnings {
		if !strings.HasPrefix(w, prefix) {
			kept = append(kept, w)
		}
	}
	s.Warnings = kept
	for _, w := range with {
		s.AddWarning(w)
	}
}

// ErrRoundCap is returned when a run asks for a search past max_rounds.
var ErrRoundCap = errors.New("max_rounds reached: no further search round is allowed")

// BeginRound opens the next search round, or refuses when the cap is reached.
func (s *State) BeginRound() error {
	if s.SearchRound >= s.Config.MaxRounds {
		return ErrRoundCap
	}
	s.SearchRound++
	return nil
}

// NoteQuery records a query as used and reports false when an equivalent one was already run.
func (s *State) NoteQuery(q string) bool {
	norm := strings.Join(strings.Fields(strings.ToLower(q)), " ")
	for _, u := range s.UsedQueries {
		if strings.Join(strings.Fields(strings.ToLower(u)), " ") == norm {
			return false
		}
	}
	s.UsedQueries = append(s.UsedQueries, strings.TrimSpace(q))
	return true
}

// AddRaw appends records from a source.
func (s *State) AddRaw(ms []paper.Metadata) { s.Raw = append(s.Raw, ms...) }

// DedupResult reports what a dedup pass did.
type DedupResult struct {
	Raw             int `json:"raw_papers"`
	Unique          int `json:"unique_papers"`
	NewUnique       int `json:"new_unique_papers"`
	Excluded        int `json:"excluded_by_type"`
	MissingAbstract int `json:"missing_abstracts"`
}

// Dedup merges the raw records added since the last pass into the unique list, drops excluded
// document types and untitled records, and refreshes the MISSING_ABSTRACT warnings.
func (s *State) Dedup() DedupResult {
	excluded := map[string]bool{}
	for _, t := range s.Config.ExcludeTypes {
		excluded[strings.ToLower(t)] = true
	}
	before := len(s.Unique)
	res := DedupResult{}
	var incoming []paper.Metadata
	for _, r := range s.Raw[s.RawMerged:] {
		switch {
		case strings.TrimSpace(r.Title) == "":
			res.Excluded++
		case excluded[r.Type]:
			res.Excluded++
			s.AddWarning(fmt.Sprintf("EXCLUDED_TYPE[%s]: %s", r.Type, r.Title))
		default:
			incoming = append(incoming, r)
		}
	}
	s.Unique = Deduplicate(s.Unique, incoming)
	s.RawMerged = len(s.Raw)

	var missing []string
	for _, u := range s.Unique {
		if u.Abstract == "" {
			missing = append(missing, missingAbstractWarning(u.Title))
		}
	}
	s.replaceWarnings("MISSING_ABSTRACT:", missing)
	res.Raw, res.Unique, res.NewUnique, res.MissingAbstract = len(s.Raw), len(s.Unique), len(s.Unique)-before, len(missing)
	return res
}

func (s *State) scoredIDs() map[string]bool {
	m := make(map[string]bool, len(s.Scored))
	for _, e := range s.Scored {
		m[e.ID] = true
	}
	return m
}

// Paper returns the unique paper with the given ID.
func (s *State) Paper(id string) (paper.Metadata, bool) {
	for _, p := range s.Unique {
		if p.ID == id {
			return p, true
		}
	}
	return paper.Metadata{}, false
}

// Selected is a paper that passed the threshold, joined with its analysis.
type Selected struct {
	Metadata paper.Metadata
	Entry    ScoredEntry
}

// SelectedPapers returns the scored papers at or above the threshold, best first, at most
// max_papers. Papers with equal scores keep the order in which they were scored.
func (s *State) SelectedPapers() []Selected {
	entries := append([]ScoredEntry(nil), s.Scored...)
	sort.SliceStable(entries, func(i, j int) bool { return entries[i].RelevanceScore > entries[j].RelevanceScore })
	var out []Selected
	for _, e := range entries {
		if e.RelevanceScore < s.Config.Threshold {
			continue
		}
		if p, ok := s.Paper(e.ID); ok {
			out = append(out, Selected{Metadata: p, Entry: e})
		}
		if len(out) == s.Config.MaxPapers {
			break
		}
	}
	return out
}

// Candidate is one paper offered to the screener.
type Candidate struct {
	PaperIndex        int    `json:"paper_index"`
	PaperID           string `json:"paper_id"`
	Title             string `json:"title"`
	Abstract          string `json:"abstract"`
	AbstractAvailable bool   `json:"abstract_available"`
	Year              string `json:"year,omitempty"`
}

// Candidates picks the unscored papers to send to scoring, capped by max_scoring_candidates, and
// remembers them as pending so the next scores file can be checked against them.
func (s *State) Candidates() []Candidate {
	done := s.scoredIDs()
	var unscored []paper.Metadata
	for _, p := range s.Unique {
		if !done[p.ID] {
			unscored = append(unscored, p)
		}
	}
	picked := Prefilter(s.UserQuery, unscored, s.Config.MaxScoringCandidates)
	var capped []string
	if len(picked) < len(unscored) {
		capped = []string{fmt.Sprintf("SCORING_CANDIDATES_CAPPED: %d -> %d new papers", len(unscored), len(picked))}
	}
	s.replaceWarnings("SCORING_CANDIDATES_CAPPED:", capped)
	s.Pending = s.Pending[:0:0]
	out := make([]Candidate, len(picked))
	for i, p := range picked {
		s.Pending = append(s.Pending, p.ID)
		out[i] = Candidate{PaperIndex: i, PaperID: p.ID, Title: p.Title, Abstract: p.Abstract, AbstractAvailable: p.Abstract != "", Year: p.Year()}
	}
	return out
}

// GateResult is the quality gate's answer.
type GateResult struct {
	Decision         Decision `json:"decision"`
	Selected         int      `json:"selected"`
	MinPapers        int      `json:"min_papers"`
	Round            int      `json:"search_round"`
	MaxRounds        int      `json:"max_rounds"`
	UsedQueries      []string `json:"used_queries"`
	BorderlineTitles []string `json:"borderline_titles"`
	Unscored         int      `json:"unscored_papers"`
}

// Gate runs the deterministic router and records the decision. noNewQueries tells the gate that a
// refinement produced nothing new to search.
func (s *State) Gate(noNewQueries bool) GateResult {
	sel := s.SelectedPapers()
	d := Route(len(sel), s.Config.MinPapers, s.SearchRound, s.Config.MaxRounds, noNewQueries)
	s.QualityGateDecision = string(d)
	switch d {
	case DecisionSummarizeAvailable:
		s.AddWarning("INSUFFICIENT_RESULTS_AFTER_MAX_ROUNDS")
	case DecisionStopNoNewQueries:
		s.AddWarning("WORKFLOW_STOPPED_NO_NEW_QUERIES")
	}
	chosen := map[string]bool{}
	for _, x := range sel {
		chosen[x.Entry.ID] = true
	}
	ranked := append([]ScoredEntry(nil), s.Scored...)
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].RelevanceScore > ranked[j].RelevanceScore })
	border := []string{}
	for _, e := range ranked {
		if chosen[e.ID] || len(border) == 10 {
			continue
		}
		if p, ok := s.Paper(e.ID); ok {
			border = append(border, p.Title)
		}
	}
	done := s.scoredIDs()
	unscored := 0
	for _, p := range s.Unique {
		if !done[p.ID] {
			unscored++
		}
	}
	return GateResult{Decision: d, Selected: len(sel), MinPapers: s.Config.MinPapers, Round: s.SearchRound,
		MaxRounds: s.Config.MaxRounds, UsedQueries: append([]string{}, s.UsedQueries...), BorderlineTitles: border, Unscored: unscored}
}

func decodeStrict(b []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("unexpected data after the JSON document")
	}
	return nil
}
