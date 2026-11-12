package litscout

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"unicode"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// ScoreFile is what the screener writes. Unknown fields are rejected.
type ScoreFile struct {
	Scores []ScoreItem `json:"scores"`
}

// ScoreItem scores one candidate. PaperID is optional; when present it must match the candidate.
type ScoreItem struct {
	PaperIndex int     `json:"paper_index"`
	PaperID    string  `json:"paper_id,omitempty"`
	Score      float64 `json:"score"`
	Reason     string  `json:"reason"`
}

// ValidationError lists every problem found in an agent's file, so the agent can fix them in one pass.
type ValidationError struct {
	File     string
	Problems []string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s rejected, nothing was applied:\n- %s", e.File, strings.Join(e.Problems, "\n- "))
}

// scriptMismatch reports whether text contains letters of a script that a report in lang does not
// use. It catches a model that answered in the wrong language, as in LitScout's own example run
// where relevance reasons came back in Chinese. It only judges Latin-script report languages.
func scriptMismatch(text, lang string) bool {
	switch strings.ToLower(strings.SplitN(lang, "-", 2)[0]) {
	case "en", "vi", "es", "fr", "de", "pt", "it", "nl", "id":
	default:
		return false
	}
	for _, r := range text {
		if unicode.Is(unicode.Han, r) || unicode.Is(unicode.Hiragana, r) || unicode.Is(unicode.Katakana, r) ||
			unicode.Is(unicode.Hangul, r) || unicode.Is(unicode.Cyrillic, r) || unicode.Is(unicode.Arabic, r) ||
			unicode.Is(unicode.Thai, r) {
			return true
		}
	}
	return false
}

func titleOnlyWarnings(p paper.Metadata) []string {
	if p.Abstract != "" {
		return nil
	}
	return []string{"SCORED_FROM_TITLE_ONLY", "MISSING_ABSTRACT"}
}

// IngestScores validates a score file against the pending candidates and applies it. A file is
// accepted whole or not at all: every candidate exactly once, scores within 0-10, a reason for each.
func (s *State) IngestScores(data []byte) error {
	ve := &ValidationError{File: "scores file"}
	if len(s.Pending) == 0 {
		return errors.New("no pending candidates: run `prefilter` first")
	}
	var f ScoreFile
	if err := decodeStrict(data, &f); err != nil {
		ve.Problems = append(ve.Problems, "invalid JSON: "+err.Error())
		return ve
	}
	seen := map[int]ScoreItem{}
	for _, it := range f.Scores {
		switch {
		case it.PaperIndex < 0 || it.PaperIndex >= len(s.Pending):
			ve.Problems = append(ve.Problems, fmt.Sprintf("paper_index %d is not a candidate (0..%d)", it.PaperIndex, len(s.Pending)-1))
			continue
		case it.PaperID != "" && it.PaperID != s.Pending[it.PaperIndex]:
			ve.Problems = append(ve.Problems, fmt.Sprintf("paper_index %d: paper_id %q does not match the candidate", it.PaperIndex, it.PaperID))
		}
		if _, dup := seen[it.PaperIndex]; dup {
			ve.Problems = append(ve.Problems, fmt.Sprintf("paper_index %d scored twice", it.PaperIndex))
		}
		seen[it.PaperIndex] = it
		if math.IsNaN(it.Score) || it.Score < 0 || it.Score > 10 {
			ve.Problems = append(ve.Problems, fmt.Sprintf("paper_index %d: score %v is outside 0-10", it.PaperIndex, it.Score))
		}
		if strings.TrimSpace(it.Reason) == "" {
			ve.Problems = append(ve.Problems, fmt.Sprintf("paper_index %d: reason is empty", it.PaperIndex))
		}
	}
	for i := range s.Pending {
		if _, ok := seen[i]; !ok {
			ve.Problems = append(ve.Problems, fmt.Sprintf("paper_index %d has no score", i))
		}
	}
	if len(ve.Problems) > 0 {
		return ve
	}
	for i, id := range s.Pending {
		it := seen[i]
		p, _ := s.Paper(id)
		warns := titleOnlyWarnings(p)
		if scriptMismatch(it.Reason, s.Config.Language) {
			warns = append(warns, "LANGUAGE_MISMATCH")
		}
		s.Scored = append(s.Scored, ScoredEntry{
			ID:       id,
			Analysis: paper.Analysis{RelevanceScore: it.Score, RelevanceReason: strings.TrimSpace(it.Reason), ScoredFromAbstract: p.Abstract != ""},
			Warnings: warns,
		})
	}
	s.Pending = nil
	return nil
}

// IngestHeuristic scores the pending candidates without a model, by keyword overlap.
func (s *State) IngestHeuristic() error {
	if len(s.Pending) == 0 {
		return errors.New("no pending candidates: run `prefilter` first")
	}
	for _, id := range s.Pending {
		p, _ := s.Paper(id)
		score, reason := HeuristicScore(s.UserQuery, p)
		s.Scored = append(s.Scored, ScoredEntry{
			ID:       id,
			Analysis: paper.Analysis{RelevanceScore: score, RelevanceReason: reason, ScoredFromAbstract: p.Abstract != ""},
			Warnings: append(titleOnlyWarnings(p), "HEURISTIC_SCORING"),
		})
	}
	s.Pending = nil
	return nil
}

// SummaryFile is what the synthesizer writes.
type SummaryFile struct {
	Summaries []SummaryItem `json:"summaries"`
	Claims    []Claim       `json:"claims,omitempty"`
}

// SummaryItem is the summary of one selected paper.
type SummaryItem struct {
	PaperID string `json:"paper_id"`
	Summary string `json:"summary"`
}

// IngestSummaries applies summaries and claims. A summary is accepted only for a selected paper
// that has an abstract; a claim only when every paper it cites is selected and its quote, if any,
// is found in a cited abstract. Nothing is applied when anything is wrong.
func (s *State) IngestSummaries(data []byte) error {
	ve := &ValidationError{File: "summaries file"}
	var f SummaryFile
	if err := decodeStrict(data, &f); err != nil {
		ve.Problems = append(ve.Problems, "invalid JSON: "+err.Error())
		return ve
	}
	selected := map[string]Selected{}
	for _, x := range s.SelectedPapers() {
		selected[x.Entry.ID] = x
	}
	summaries := map[string]string{}
	for _, it := range f.Summaries {
		x, ok := selected[it.PaperID]
		switch {
		case !ok:
			ve.Problems = append(ve.Problems, fmt.Sprintf("summary for %q: not a selected paper", it.PaperID))
			continue
		case x.Metadata.Abstract == "":
			ve.Problems = append(ve.Problems, fmt.Sprintf("summary for %q: the paper has no abstract, so no summary may be written", it.PaperID))
			continue
		case strings.TrimSpace(it.Summary) == "":
			ve.Problems = append(ve.Problems, fmt.Sprintf("summary for %q is empty", it.PaperID))
			continue
		}
		summaries[it.PaperID] = strings.TrimSpace(it.Summary)
		if scriptMismatch(it.Summary, s.Config.Language) {
			s.AddWarning("LANGUAGE_MISMATCH: summary of " + x.Metadata.Title)
		}
	}
	for i, c := range f.Claims {
		label := fmt.Sprintf("claim %d", i)
		if strings.TrimSpace(c.Text) == "" {
			ve.Problems = append(ve.Problems, label+": text is empty")
		}
		if len(c.PaperIDs) == 0 {
			ve.Problems = append(ve.Problems, label+": paper_ids is empty; every claim needs a source")
		}
		found := c.Quote == ""
		nq := Normalize(c.Quote)
		for _, id := range c.PaperIDs {
			x, ok := selected[id]
			if !ok {
				ve.Problems = append(ve.Problems, fmt.Sprintf("%s: %q is not a selected paper", label, id))
				continue
			}
			if !found && nq != "" && strings.Contains(Normalize(x.Metadata.Abstract), nq) {
				found = true
			}
		}
		if !found {
			ve.Problems = append(ve.Problems, label+": the quote is not in the abstract of any cited paper")
		}
	}
	if len(ve.Problems) > 0 {
		return ve
	}
	for i := range s.Scored {
		if sum, ok := summaries[s.Scored[i].ID]; ok {
			s.Scored[i].Summary = sum
		}
	}
	s.Claims = f.Claims
	return nil
}
