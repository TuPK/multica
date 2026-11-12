package source

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// Arxiv searches the arXiv Atom API. Its terms of use ask for about one request every three
// seconds, so the Client that backs it should set MinInterval.
type Arxiv struct {
	Client  *Client
	BaseURL string // default https://export.arxiv.org/api/query
}

func (a *Arxiv) Name() string { return "arxiv" }

type atomFeed struct {
	Entries []struct {
		ID        string `xml:"id"`
		Title     string `xml:"title"`
		Summary   string `xml:"summary"`
		Published string `xml:"published"`
		DOI       string `xml:"doi"`
		Authors   []struct {
			Name string `xml:"name"`
		} `xml:"author"`
		Links []struct {
			Href  string `xml:"href,attr"`
			Title string `xml:"title,attr"`
			Type  string `xml:"type,attr"`
		} `xml:"link"`
	} `xml:"entry"`
}

// ArxivID extracts the identifier from an abs URL and drops the version suffix.
func ArxivID(absURL string) string {
	s := absURL
	if i := strings.Index(s, "/abs/"); i >= 0 {
		s = s[i+len("/abs/"):]
	}
	if i := strings.LastIndex(s, "v"); i > 0 {
		if _, err := strconv.Atoi(s[i+1:]); err == nil {
			s = s[:i]
		}
	}
	return s
}

func (a *Arxiv) Search(ctx context.Context, q Query) ([]paper.Metadata, error) {
	if q.Limit <= 0 {
		return nil, nil
	}
	words := strings.Fields(q.Text)
	if len(words) > 8 {
		words = words[:8]
	}
	if len(words) == 0 {
		return nil, nil
	}
	terms := make([]string, len(words))
	for i, w := range words {
		terms[i] = "all:" + strings.Trim(w, `"():`)
	}
	base := a.BaseURL
	if base == "" {
		base = "https://export.arxiv.org/api/query"
	}
	p := url.Values{
		"search_query": {strings.Join(terms, " AND ")},
		"start":        {"0"},
		"max_results":  {strconv.Itoa(min(q.Limit, 100))},
		"sortBy":       {"relevance"},
	}
	body, err := a.Client.Get(ctx, Request{Source: "arxiv", Query: q.Text, URL: base, Params: p})
	if err != nil {
		return nil, err
	}
	var feed atomFeed
	if err := xml.Unmarshal(body, &feed); err != nil {
		return nil, fmt.Errorf("arxiv: malformed response")
	}
	var out []paper.Metadata
	for _, e := range feed.Entries {
		m := paper.Metadata{
			Title:          strings.Join(strings.Fields(e.Title), " "),
			Abstract:       strings.Join(strings.Fields(e.Summary), " "),
			DOI:            paper.NormalizeDOI(e.DOI),
			ArxivID:        ArxivID(strings.TrimSpace(e.ID)),
			URL:            strings.TrimSpace(e.ID),
			Type:           "preprint",
			MetadataSource: "arXiv",
		}
		if m.Abstract != "" {
			m.AbstractSource = "arXiv"
		}
		for _, au := range e.Authors {
			if n := strings.TrimSpace(au.Name); n != "" {
				m.Authors = append(m.Authors, n)
			}
		}
		if len(e.Published) >= 10 {
			m.PublicationDate = e.Published[:10]
		}
		for _, l := range e.Links {
			if l.Title == "pdf" || l.Type == "application/pdf" {
				m.OpenAccessURL = l.Href
			}
		}
		if q.FromYear > 0 && m.Year() != "" {
			if y, err := strconv.Atoi(m.Year()); err == nil && y < q.FromYear {
				continue
			}
		}
		out = append(out, m)
	}
	return out, nil
}
