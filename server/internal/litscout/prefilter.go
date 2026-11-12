package litscout

import (
	"fmt"
	"math"
	"sort"
	"strings"

	"golang.org/x/text/cases"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

func queryTerms(query string) []string {
	seen := map[string]bool{}
	var terms []string
	for _, t := range Tokens(query) {
		if !seen[t] {
			seen[t] = true
			terms = append(terms, t)
		}
	}
	return terms
}

// Prefilter caps the number of papers sent to scoring with a deterministic keyword ranking:
// title hits count three times an abstract hit, and a paper with an abstract wins a tie. Papers
// of equal rank keep their original order.
func Prefilter(query string, papers []paper.Metadata, limit int) []paper.Metadata {
	if limit <= 0 || len(papers) <= limit {
		return papers
	}
	terms := queryTerms(query)
	fold := cases.Fold()
	type ranked struct {
		m        paper.Metadata
		score    int
		hasAbstr int
	}
	rs := make([]ranked, len(papers))
	for i, p := range papers {
		title, abstract := fold.String(p.Title), fold.String(p.Abstract)
		th, ah := 0, 0
		for _, t := range terms {
			if strings.Contains(title, t) {
				th++
			}
			if strings.Contains(abstract, t) {
				ah++
			}
		}
		has := 0
		if p.Abstract != "" {
			has = 1
		}
		rs[i] = ranked{p, th*3 + ah, has}
	}
	sort.SliceStable(rs, func(i, j int) bool {
		if rs[i].score != rs[j].score {
			return rs[i].score > rs[j].score
		}
		return rs[i].hasAbstr > rs[j].hasAbstr
	})
	out := make([]paper.Metadata, limit)
	for i := range out {
		out[i] = rs[i].m
	}
	return out
}

// HeuristicScore is the model-free fallback: the share of query terms found in the title and
// abstract, on a 0-10 scale rounded to two decimals.
func HeuristicScore(query string, p paper.Metadata) (float64, string) {
	terms := queryTerms(query)
	text := cases.Fold().String(p.Title + " " + p.Abstract)
	hit := 0
	for _, t := range terms {
		if strings.Contains(text, t) {
			hit++
		}
	}
	n := len(terms)
	if n == 0 {
		n = 1
	}
	score := math.Round(float64(hit)/float64(n)*10*100) / 100
	return score, "Deterministic keyword-overlap fallback."
}

func missingAbstractWarning(title string) string { return fmt.Sprintf("MISSING_ABSTRACT: %s", title) }
