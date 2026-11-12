package litscout

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// Gold is a human-labelled answer for one topic. Only a person writes it.
type Gold struct {
	Topic      string      `json:"topic,omitempty"`
	Relevant   []GoldPaper `json:"relevant"`
	MergePairs []GoldPair  `json:"merge_pairs,omitempty"`
}

// GoldPaper identifies a relevant paper by any identifier it has, or by title.
type GoldPaper struct {
	DOI        string `json:"doi,omitempty"`
	OpenAlexID string `json:"openalex_id,omitempty"`
	ArxivID    string `json:"arxiv_id,omitempty"`
	Title      string `json:"title,omitempty"`
}

// GoldPair is a labelled pair of records: Same says whether they are one paper.
type GoldPair struct {
	A    paper.Metadata `json:"a"`
	B    paper.Metadata `json:"b"`
	Same bool           `json:"same"`
}

// ParseGold reads a gold file strictly.
func ParseGold(b []byte) (Gold, error) {
	var g Gold
	if err := decodeStrict(b, &g); err != nil {
		return g, fmt.Errorf("gold file: %w", err)
	}
	if len(g.Relevant) == 0 && len(g.MergePairs) == 0 {
		return g, fmt.Errorf("gold file: needs relevant papers or merge pairs")
	}
	return g, nil
}

func (g GoldPaper) matches(m paper.Metadata) bool {
	switch {
	case g.DOI != "" && paper.NormalizeDOI(g.DOI) == m.DOI:
		return true
	case g.OpenAlexID != "" && paper.NormalizeOpenAlexID(g.OpenAlexID) == m.OpenAlexID:
		return true
	case g.ArxivID != "" && g.ArxivID == m.ArxivID:
		return true
	case g.Title != "" && Normalize(g.Title) == Normalize(m.Title):
		return true
	}
	return false
}

// EvalReport holds the quality measures of a finished run. A measure without data is nil.
type EvalReport struct {
	Selected            int      `json:"selected"`
	RelevantInGold      int      `json:"relevant_in_gold"`
	PrecisionAtSelected *float64 `json:"precision_at_selected,omitempty"`
	RecallSelected      *float64 `json:"recall_selected,omitempty"`
	RecallRetrieved     *float64 `json:"recall_retrieved,omitempty"`
	DedupFalseMerges    *int     `json:"dedup_false_merges,omitempty"`
	DedupMissedMerges   *int     `json:"dedup_missed_merges,omitempty"`
	ProvenanceFailures  int      `json:"provenance_failures"`
	DOIUnresolved       int      `json:"doi_unresolved"`
	LanguageCompliance  *float64 `json:"language_compliance,omitempty"`
	ClaimsTotal         int      `json:"claims_total"`
	ClaimsGrounded      int      `json:"claims_grounded"`
	ClaimGrounding      *float64 `json:"claim_grounding,omitempty"`
	SearchRounds        int      `json:"search_rounds"`
	QueriesUsed         int      `json:"queries_used"`
	RawPapers           int      `json:"raw_papers"`
	UniquePapers        int      `json:"unique_papers"`
	Warnings            int      `json:"warnings"`
}

func ratio(num, den int) *float64 {
	if den == 0 {
		return nil
	}
	v := float64(num) / float64(den)
	return &v
}

// Evaluate measures the state against a gold file. A claim counts as grounded when it cites only
// selected papers and carries a quote found in one of their abstracts.
func (s *State) Evaluate(g Gold) EvalReport {
	sel := s.SelectedPapers()
	r := EvalReport{Selected: len(sel), RelevantInGold: len(g.Relevant), SearchRounds: s.SearchRound,
		QueriesUsed: len(s.UsedQueries), RawPapers: len(s.Raw), UniquePapers: len(s.Unique), Warnings: len(s.Warnings)}
	hitSel, hitAll := 0, 0
	for _, gp := range g.Relevant {
		for _, u := range s.Unique {
			if gp.matches(u) {
				hitAll++
				break
			}
		}
		for _, x := range sel {
			if gp.matches(x.Metadata) {
				hitSel++
				break
			}
		}
	}
	if len(g.Relevant) > 0 {
		relSelected := 0
		for _, x := range sel {
			for _, gp := range g.Relevant {
				if gp.matches(x.Metadata) {
					relSelected++
					break
				}
			}
		}
		r.PrecisionAtSelected = ratio(relSelected, len(sel))
		r.RecallSelected = ratio(hitSel, len(g.Relevant))
		r.RecallRetrieved = ratio(hitAll, len(g.Relevant))
	}
	if len(g.MergePairs) > 0 {
		fm, mm := 0, 0
		for _, p := range g.MergePairs {
			got := Same(p.A, p.B)
			if got && !p.Same {
				fm++
			}
			if !got && p.Same {
				mm++
			}
		}
		r.DedupFalseMerges, r.DedupMissedMerges = &fm, &mm
	}
	good, total := 0, 0
	for _, x := range sel {
		if len(ValidatePaper(x.Metadata)) > 0 {
			r.ProvenanceFailures++
		}
		if s.DOIStatus[x.Entry.ID] == "unresolved" {
			r.DOIUnresolved++
		}
		for _, text := range []string{x.Entry.RelevanceReason, x.Entry.Summary} {
			if text == "" {
				continue
			}
			total++
			if !scriptMismatch(text, s.Config.Language) {
				good++
			}
		}
	}
	r.LanguageCompliance = ratio(good, total)
	selected := map[string]Selected{}
	for _, x := range sel {
		selected[x.Entry.ID] = x
	}
	r.ClaimsTotal = len(s.Claims)
	for _, c := range s.Claims {
		if claimGrounded(c, selected) {
			r.ClaimsGrounded++
		}
	}
	r.ClaimGrounding = ratio(r.ClaimsGrounded, r.ClaimsTotal)
	return r
}

func claimGrounded(c Claim, selected map[string]Selected) bool {
	nq := Normalize(c.Quote)
	if len(c.PaperIDs) == 0 || nq == "" {
		return false
	}
	found := false
	for _, id := range c.PaperIDs {
		x, ok := selected[id]
		if !ok {
			return false
		}
		if contains(Normalize(x.Metadata.Abstract), nq) {
			found = true
		}
	}
	return found
}

func contains(h, n string) bool { return strings.Contains(h, n) }

// MarshalIndent is a helper for CLI output.
func MarshalIndent(v any) ([]byte, error) { return json.MarshalIndent(v, "", "  ") }
