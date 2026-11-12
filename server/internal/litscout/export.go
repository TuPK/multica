package litscout

import (
	"bytes"
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// CSVColumns are the columns of papers.csv, in LitScout's order.
var CSVColumns = []string{
	"rank", "title", "authors", "publication_date", "doi", "openalex_id", "url", "metadata_source",
	"abstract_source", "relevance_score", "relevance_reason", "summary", "warning",
}

// ExportOptions tunes the files.
type ExportOptions struct {
	// BOM writes a UTF-8 byte order mark first so a spreadsheet reads Vietnamese text correctly.
	BOM bool
}

// pyFloat formats a score like Python's repr of a float: 10.0, 7.5.
func pyFloat(f float64) string {
	s := strconv.FormatFloat(f, 'f', -1, 64)
	if !strings.Contains(s, ".") {
		s += ".0"
	}
	return s
}

// csvText neutralises a cell that a spreadsheet would run as a formula. Titles, authors and
// abstracts come from outside, so a cell may start with = + - @.
func csvText(s string) string {
	if s != "" && strings.ContainsRune("=+-@\t\r", rune(s[0])) {
		return "'" + s
	}
	return s
}

// CSV renders papers.csv for the selected papers.
func (s *State) CSV(opt ExportOptions) ([]byte, error) {
	var buf bytes.Buffer
	if opt.BOM {
		buf.WriteString("\ufeff")
	}
	w := csv.NewWriter(&buf)
	if err := w.Write(CSVColumns); err != nil {
		return nil, err
	}
	for i, x := range s.SelectedPapers() {
		m, e := x.Metadata, x.Entry
		err := w.Write([]string{
			strconv.Itoa(i + 1), csvText(m.Title), csvText(strings.Join(m.Authors, "; ")), m.PublicationDate, m.DOI,
			m.OpenAlexID, m.URL, m.MetadataSource, m.AbstractSource, pyFloat(e.RelevanceScore),
			csvText(e.RelevanceReason), csvText(e.Summary), strings.Join(e.Warnings, "; "),
		})
		if err != nil {
			return nil, err
		}
	}
	w.Flush()
	return buf.Bytes(), w.Error()
}

func oneLine(s string) string { return strings.Join(strings.Fields(s), " ") }

// mdBlock keeps free text from becoming a heading of its own.
func mdBlock(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		if strings.HasPrefix(strings.TrimLeft(l, " "), "#") {
			lines[i] = `\` + strings.TrimLeft(l, " ")
		}
	}
	return strings.Join(lines, "\n")
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// Markdown renders research_digest.md.
func (s *State) Markdown() string {
	sel := s.SelectedPapers()
	rank := map[string]int{}
	for i, x := range sel {
		rank[x.Entry.ID] = i + 1
	}
	warnings := []string{"- None"}
	if len(s.Warnings) > 0 {
		warnings = warnings[:0]
		for _, w := range s.Warnings {
			warnings = append(warnings, "- "+oneLine(w))
		}
	}
	lines := []string{
		"# Research Digest", "", "## Query", "", oneLine(s.UserQuery), "", "## Search Summary", "",
		fmt.Sprintf("- Search rounds: %d", s.SearchRound),
		fmt.Sprintf("- Papers retrieved: %d", len(s.Raw)),
		fmt.Sprintf("- Unique papers: %d", len(s.Unique)),
		fmt.Sprintf("- Selected papers: %d", len(sel)),
		fmt.Sprintf("- Warnings: %d", len(s.Warnings)),
		"", "### Warnings", "",
	}
	lines = append(lines, warnings...)
	lines = append(lines, "", "## Selected Papers", "")
	for i, x := range sel {
		m, a := x.Metadata, x.Entry
		lines = append(lines,
			fmt.Sprintf("### %d. %s", i+1, oneLine(m.Title)), "",
			fmt.Sprintf("Authors: %s  ", orDefault(oneLine(strings.Join(m.Authors, ", ")), "Unknown")),
			fmt.Sprintf("Date: %s  ", orDefault(m.PublicationDate, "Unavailable")),
			fmt.Sprintf("DOI: %s  ", orDefault(m.DOI, "Unavailable")),
			fmt.Sprintf("URL: %s", orDefault(m.URL, "Unavailable")), "",
			fmt.Sprintf("Metadata source: %s  ", m.MetadataSource),
			fmt.Sprintf("Abstract source: %s", orDefault(m.AbstractSource, "Unavailable")), "",
			fmt.Sprintf("Relevance score: %g/10", a.RelevanceScore), "",
			"#### Relevance", "", mdBlock(a.RelevanceReason), "",
			"#### Summary", "",
		)
		if m.Abstract != "" {
			lines = append(lines, orDefault(mdBlock(a.Summary), "Summary unavailable."), "",
				"#### Provenance", "",
				fmt.Sprintf("Summary generated only from the abstract returned by %s.", m.AbstractSource))
			switch s.DOIStatus[a.ID] {
			case "ok":
				lines = append(lines, "", "DOI confirmed by Crossref.")
			case "ok_other_registry":
				lines = append(lines, "", "DOI confirmed by doi.org (registered outside Crossref, for example by arXiv).")
			case "unresolved":
				lines = append(lines, "", "⚠ DOI not found at Crossref or doi.org.")
			}
			if ft, ok := s.Fulltext[a.ID]; ok && ft.Status == "ok" {
				lines = append(lines, "", "Open-access full text was extracted from "+ft.Source+" (low confidence; not used for the summary).")
			}
			lines = append(lines, "")
		} else {
			lines = append(lines, "⚠ Abstract unavailable from configured APIs.  ", "No generated summary was produced.", "")
		}
	}
	if len(s.Claims) > 0 {
		lines = append(lines, "## Synthesis", "")
		for _, c := range s.Claims {
			var refs []string
			for _, id := range c.PaperIDs {
				if r, ok := rank[id]; ok {
					refs = append(refs, strconv.Itoa(r))
				}
			}
			lines = append(lines, fmt.Sprintf("- %s [%s]", oneLine(c.Text), strings.Join(refs, ", ")))
		}
		lines = append(lines, "")
	}
	scope := "This workflow uses metadata and abstracts from configured APIs only. " +
		"It does not download PDFs, scrape restricted full text, or bypass paywalls."
	if s.Config.AllowFullText {
		scope = "This workflow uses metadata and abstracts from configured APIs, and may extract text from open-access PDFs " +
			"that Unpaywall reports. It does not scrape restricted full text or bypass paywalls."
	}
	lines = append(lines, "## Scope", "", scope, "")
	return strings.Join(lines, "\n")
}

// Export verifies the state, then writes papers.csv and research_digest.md into out. It refuses to
// write anything when the provenance gate fails.
func (s *State) Export(out string, opt ExportOptions, vopt VerifyOptions) (VerifyResult, []string, error) {
	res := s.Verify(vopt)
	if !res.OK {
		return res, nil, fmt.Errorf("provenance validation failed: %s", res.ErrorSummary())
	}
	csvBytes, err := s.CSV(opt)
	if err != nil {
		return res, nil, err
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return res, nil, err
	}
	files := map[string][]byte{"papers.csv": csvBytes, "research_digest.md": []byte(s.Markdown())}
	var paths []string
	for _, name := range []string{"papers.csv", "research_digest.md"} {
		p := filepath.Join(out, name)
		if err := os.WriteFile(p, files[name], 0o644); err != nil {
			return res, nil, err
		}
		paths = append(paths, p)
	}
	s.OutputFiles = paths
	return res, paths, nil
}
