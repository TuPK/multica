package litscout

import (
	"fmt"
	"sort"
	"strings"
)

// VerifyOptions tunes the checks.
type VerifyOptions struct {
	// RequireDOICheck makes a selected paper with a DOI and no DOI status an error.
	RequireDOICheck bool
}

// VerifyResult is what the provenance gate found.
type VerifyResult struct {
	OK       bool     `json:"ok"`
	Errors   []string `json:"errors"`
	Warnings []string `json:"warnings"`
	Selected int      `json:"selected"`
}

// Verify is the gate before export. Errors block it; warnings are reported in the digest.
func (s *State) Verify(opt VerifyOptions) VerifyResult {
	res := VerifyResult{Errors: []string{}, Warnings: []string{}}
	sel := s.SelectedPapers()
	res.Selected = len(sel)
	if s.QualityGateDecision == "" {
		res.Errors = append(res.Errors, "GATE_NOT_RUN: run `gate` and follow its decision before verifying")
	}
	if len(s.Pending) > 0 {
		res.Errors = append(res.Errors, fmt.Sprintf("PENDING_CANDIDATES_NOT_SCORED: %d candidates await scores", len(s.Pending)))
	}
	selected := map[string]bool{}
	for _, x := range sel {
		selected[x.Entry.ID] = true
		for _, e := range ValidatePaper(x.Metadata) {
			res.Errors = append(res.Errors, fmt.Sprintf("%s: %s", e, x.Metadata.Title))
		}
		if x.Entry.Summary != "" && x.Metadata.Abstract == "" {
			res.Errors = append(res.Errors, "SUMMARY_WITHOUT_ABSTRACT: "+x.Metadata.Title)
		}
		if x.Metadata.Abstract != "" && x.Entry.Summary == "" {
			res.Warnings = append(res.Warnings, "MISSING_SUMMARY: "+x.Metadata.Title)
		}
		if x.Metadata.DOI != "" {
			switch s.DOIStatus[x.Entry.ID] {
			case "", "ok", "ok_other_registry":
				if s.DOIStatus[x.Entry.ID] == "" && opt.RequireDOICheck {
					res.Errors = append(res.Errors, "DOI_NOT_CHECKED: "+x.Metadata.Title)
				}
			case "unresolved":
				res.Warnings = append(res.Warnings, "DOI_UNRESOLVED: "+x.Metadata.Title)
			}
		}
	}
	for i, c := range s.Claims {
		for _, id := range c.PaperIDs {
			if !selected[id] {
				res.Errors = append(res.Errors, fmt.Sprintf("CLAIM_CITES_UNSELECTED_PAPER: claim %d cites %s", i, id))
			}
		}
	}
	res.Warnings = append(res.Warnings, s.Warnings...)
	res.Warnings = dedupStrings(res.Warnings)
	sort.Strings(res.Errors)
	res.OK = len(res.Errors) == 0
	return res
}

func dedupStrings(in []string) []string {
	seen := map[string]bool{}
	out := in[:0:0]
	for _, x := range in {
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

// ErrorSummary joins the errors for a CLI message.
func (r VerifyResult) ErrorSummary() string { return strings.Join(r.Errors, "; ") }
