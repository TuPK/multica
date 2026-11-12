package litscout

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
	"github.com/multica-ai/multica/server/internal/litscout/source"
)

// Services are the network-facing parts a run uses. A nil member means that capability is off.
type Services struct {
	Sources   map[string]source.Source
	OpenAlex  *source.OpenAlex
	Crossref  *source.Crossref
	Unpaywall *source.Unpaywall
	PDF       *PDFFetcher
}

// SourceReport is what one source returned for one search command.
type SourceReport struct {
	Source string `json:"source"`
	Query  string `json:"query"`
	Count  int    `json:"count"`
	Error  string `json:"error,omitempty"`
}

// SearchReport is the outcome of a search command.
type SearchReport struct {
	Round        int            `json:"search_round"`
	MaxRounds    int            `json:"max_rounds"`
	NoNewQueries bool           `json:"no_new_queries"`
	Skipped      []string       `json:"skipped_queries,omitempty"`
	Results      []SourceReport `json:"results"`
	RawTotal     int            `json:"raw_papers"`
}

// Search runs the new queries on every enabled source as one round. A query already used is skipped.
// When no query is new, no round is spent. A source that fails becomes a SEARCH_FAILED warning and
// the other sources still run; only, when not empty, narrows the run's sources for this call; a source that is out of budget or rate limited stays off for the
// rest of the run.
func (s *State) Search(ctx context.Context, svc Services, queries []string, only []string) (SearchReport, error) {
	rep := SearchReport{MaxRounds: s.Config.MaxRounds}
	var fresh []string
	seen := map[string]bool{}
	for _, q := range queries {
		key := strings.Join(strings.Fields(strings.ToLower(q)), " ")
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		used := false
		for _, u := range s.UsedQueries {
			if strings.Join(strings.Fields(strings.ToLower(u)), " ") == key {
				used = true
			}
		}
		if used {
			rep.Skipped = append(rep.Skipped, q)
			continue
		}
		fresh = append(fresh, strings.TrimSpace(q))
	}
	rep.Round, rep.RawTotal = s.SearchRound, len(s.Raw)
	if len(fresh) == 0 {
		rep.NoNewQueries = true
		s.AddWarning("NO_NEW_REFINED_QUERIES")
		return rep, nil
	}
	if err := s.BeginRound(); err != nil {
		return rep, err
	}
	rep.Round = s.SearchRound
	if s.DisabledSources == nil {
		s.DisabledSources = map[string]string{}
	}
	sources := s.Config.Sources
	if len(only) > 0 {
		sources = only
	}
	for _, q := range fresh {
		for _, name := range sources {
			sr := SourceReport{Source: name, Query: q}
			src, ok := svc.Sources[name]
			switch {
			case !ok:
				sr.Error = "source is not available in this build"
				s.AddWarning(fmt.Sprintf("SEARCH_FAILED[%s]: %s", name, sr.Error))
			case s.DisabledSources[name] != "":
				sr.Error = "disabled for this run: " + s.DisabledSources[name]
				s.AddWarning(fmt.Sprintf("SOURCE_DISABLED[%s]: %s", name, s.DisabledSources[name]))
			default:
				ms, err := src.Search(ctx, source.Query{Text: q, FromYear: s.Config.FromYear, Limit: s.Config.ResultsPerQuery})
				if err != nil {
					if ctx.Err() != nil {
						return rep, ctx.Err()
					}
					sr.Error = err.Error()
					s.AddWarning(fmt.Sprintf("SEARCH_FAILED[%s]: %s", name, err.Error()))
					switch {
					case errors.Is(err, source.ErrQuotaExhausted):
						s.DisabledSources[name] = "quota exhausted"
					case errors.Is(err, source.ErrRateLimited):
						s.DisabledSources[name] = "rate limited"
					}
				} else {
					s.AddRaw(ms)
					sr.Count = len(ms)
				}
			}
			rep.Results = append(rep.Results, sr)
		}
		s.NoteQuery(q)
	}
	rep.RawTotal = len(s.Raw)
	return rep, nil
}

// Chase follows citations from the top selected papers: the works that cite them and the works
// they cite, through OpenAlex. It spends one search round.
func (s *State) Chase(ctx context.Context, svc Services, top, perPaper int) (SearchReport, error) {
	rep := SearchReport{MaxRounds: s.Config.MaxRounds, Round: s.SearchRound}
	if svc.OpenAlex == nil {
		return rep, errors.New("citation chasing needs OpenAlex")
	}
	if top < 1 || perPaper < 1 {
		return rep, errors.New("top and per-paper must be at least 1")
	}
	if err := s.BeginRound(); err != nil {
		return rep, err
	}
	rep.Round = s.SearchRound
	sel := s.SelectedPapers()
	if len(sel) > top {
		sel = sel[:top]
	}
	for _, x := range sel {
		m := x.Metadata
		if m.OpenAlexID == "" {
			s.AddWarning("CHASE_SKIPPED_NO_OPENALEX_ID: " + m.Title)
			continue
		}
		label := "chase:" + m.OpenAlexID
		if !s.NoteQuery(label) {
			continue
		}
		sr := SourceReport{Source: "openalex", Query: label}
		cited, err := svc.OpenAlex.CitedBy(ctx, m.OpenAlexID, perPaper)
		if err == nil {
			refs := m.ReferencedWorks
			if len(refs) > perPaper {
				refs = refs[:perPaper]
			}
			var more []paper.Metadata
			more, err = svc.OpenAlex.ByIDs(ctx, refs)
			cited = append(cited, more...)
		}
		if err != nil {
			if ctx.Err() != nil {
				return rep, ctx.Err()
			}
			sr.Error = err.Error()
			s.AddWarning(fmt.Sprintf("SEARCH_FAILED[openalex]: %s", err.Error()))
			rep.Results = append(rep.Results, sr)
			if errors.Is(err, source.ErrQuotaExhausted) || errors.Is(err, source.ErrRateLimited) {
				break
			}
			continue
		}
		s.AddRaw(cited)
		sr.Count = len(cited)
		rep.Results = append(rep.Results, sr)
	}
	rep.RawTotal = len(s.Raw)
	return rep, nil
}

// DOIReport counts the outcomes of a DOI check.
type DOIReport struct {
	Checked    int `json:"checked"`
	OK         int `json:"ok"`
	Unresolved int `json:"unresolved"`
	Errors     int `json:"errors"`
}

// CheckDOIs asks Crossref whether each selected paper's DOI exists, and the DOI proxy when Crossref
// does not know it. Status ok means Crossref has it; ok_other_registry that another agency (DataCite for
// arXiv) registered it; unresolved that neither knows it.
func (s *State) CheckDOIs(ctx context.Context, svc Services) (DOIReport, error) {
	var rep DOIReport
	if svc.Crossref == nil {
		return rep, errors.New("DOI check needs Crossref")
	}
	if s.DOIStatus == nil {
		s.DOIStatus = map[string]string{}
	}
	for _, x := range s.SelectedPapers() {
		m := x.Metadata
		if m.DOI == "" {
			continue
		}
		rep.Checked++
		_, found, err := svc.Crossref.Resolve(ctx, m.DOI)
		other := false
		if err == nil && !found {
			// Not a Crossref DOI. It may still be registered elsewhere (arXiv and Zenodo use DataCite).
			other, err = svc.Crossref.Registered(ctx, m.DOI)
		}
		switch {
		case err != nil:
			if ctx.Err() != nil {
				return rep, ctx.Err()
			}
			rep.Errors++
			s.DOIStatus[m.ID] = "error"
			s.AddWarning("DOI_CHECK_FAILED: " + m.Title)
		case found:
			rep.OK++
			s.DOIStatus[m.ID] = "ok"
		case other:
			rep.OK++
			s.DOIStatus[m.ID] = "ok_other_registry"
		default:
			rep.Unresolved++
			s.DOIStatus[m.ID] = "unresolved"
			s.AddWarning("DOI_UNRESOLVED: " + m.Title)
		}
	}
	return rep, nil
}

// FullTextReport counts the outcomes of a full-text pass.
type FullTextReport struct {
	Fetched       int `json:"fetched"`
	NotOpenAccess int `json:"not_open_access"`
	Failed        int `json:"failed"`
}

// FetchFullText downloads the open-access PDF of each selected paper and stores the extracted text
// under dir/fulltext. It runs only when the run allowed full text; a paper that is not open
// access is left alone.
func (s *State) FetchFullText(ctx context.Context, svc Services, dir string) (FullTextReport, error) {
	var rep FullTextReport
	if !s.Config.AllowFullText {
		return rep, errors.New("this run does not allow full text: initialise it with --allow-fulltext")
	}
	if svc.PDF == nil {
		return rep, errors.New("full text is not available in this build")
	}
	if s.Fulltext == nil {
		s.Fulltext = map[string]FulltextInfo{}
	}
	outDir := filepath.Join(dir, "fulltext")
	for _, x := range s.SelectedPapers() {
		m := x.Metadata
		pdfURL, from := "", ""
		if m.DOI != "" && svc.Unpaywall != nil {
			u, ok, err := svc.Unpaywall.OALocation(ctx, m.DOI)
			if err != nil {
				if ctx.Err() != nil {
					return rep, ctx.Err()
				}
				s.AddWarning("FULLTEXT_LOOKUP_FAILED: " + m.Title)
			} else if ok {
				pdfURL, from = u, "Unpaywall"
			}
		}
		if pdfURL == "" && (m.ArxivID != "" || strings.HasSuffix(strings.ToLower(m.OpenAccessURL), ".pdf")) && m.OpenAccessURL != "" {
			pdfURL, from = m.OpenAccessURL, m.MetadataSource
		}
		if pdfURL == "" {
			s.Fulltext[m.ID] = FulltextInfo{Status: "not_open_access"}
			rep.NotOpenAccess++
			continue
		}
		text, err := svc.PDF.Fetch(ctx, pdfURL)
		if err != nil {
			if ctx.Err() != nil {
				return rep, ctx.Err()
			}
			s.Fulltext[m.ID] = FulltextInfo{Status: "failed", Source: from}
			s.AddWarning(fmt.Sprintf("FULLTEXT_FAILED: %s: %s", m.Title, err.Error()))
			rep.Failed++
			continue
		}
		if err := os.MkdirAll(outDir, 0o755); err != nil {
			return rep, err
		}
		path := filepath.Join(outDir, m.ID+".txt")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			return rep, err
		}
		s.Fulltext[m.ID] = FulltextInfo{Status: "ok", Path: filepath.Join("fulltext", m.ID+".txt"), Source: from, Chars: len([]rune(text))}
		rep.Fetched++
	}
	if rep.Fetched > 0 {
		s.AddWarning("FULLTEXT_LOW_CONFIDENCE: extracted PDF text can lose columns, tables and formulas; the summaries still come from abstracts")
	}
	return rep, nil
}
