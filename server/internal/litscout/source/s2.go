package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// SemanticScholar searches the Semantic Scholar Graph API. The key is optional; without it the
// shared rate limit is low.
type SemanticScholar struct {
	Client  *Client
	BaseURL string // default https://api.semanticscholar.org/graph/v1
	APIKey  string
}

func (s *SemanticScholar) Name() string { return "semanticscholar" }

func (s *SemanticScholar) Search(ctx context.Context, q Query) ([]paper.Metadata, error) {
	if q.Limit <= 0 {
		return nil, nil
	}
	base := strings.TrimRight(s.BaseURL, "/")
	if base == "" {
		base = "https://api.semanticscholar.org/graph/v1"
	}
	p := url.Values{
		"query":  {q.Text},
		"limit":  {strconv.Itoa(min(q.Limit, 100))},
		"fields": {"title,abstract,authors,year,externalIds,url,publicationDate,openAccessPdf,publicationTypes"},
	}
	if q.FromYear > 0 {
		p.Set("year", fmt.Sprintf("%d-", q.FromYear))
	}
	req := Request{Source: "semanticscholar", Query: q.Text, URL: base + "/paper/search", Params: p}
	if s.APIKey != "" {
		req.Headers = map[string]string{"x-api-key": s.APIKey}
	}
	body, err := s.Client.Get(ctx, req)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Data []struct {
			PaperID         string  `json:"paperId"`
			Title           string  `json:"title"`
			Abstract        *string `json:"abstract"`
			URL             string  `json:"url"`
			Year            int     `json:"year"`
			PublicationDate string  `json:"publicationDate"`
			Authors         []struct {
				Name string `json:"name"`
			} `json:"authors"`
			ExternalIDs      map[string]any `json:"externalIds"`
			PublicationTypes []string       `json:"publicationTypes"`
			OpenAccessPdf    *struct {
				URL string `json:"url"`
			} `json:"openAccessPdf"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("semanticscholar: malformed response")
	}
	out := make([]paper.Metadata, 0, len(resp.Data))
	for _, d := range resp.Data {
		m := paper.Metadata{
			Title:          strings.TrimSpace(d.Title),
			S2ID:           d.PaperID,
			URL:            d.URL,
			MetadataSource: "SemanticScholar",
		}
		if d.Abstract != nil && strings.TrimSpace(*d.Abstract) != "" {
			m.Abstract, m.AbstractSource = strings.TrimSpace(*d.Abstract), "SemanticScholar"
		}
		for _, a := range d.Authors {
			if a.Name != "" {
				m.Authors = append(m.Authors, a.Name)
			}
		}
		if v, ok := d.ExternalIDs["DOI"].(string); ok {
			m.DOI = paper.NormalizeDOI(v)
		}
		if v, ok := d.ExternalIDs["ArXiv"].(string); ok {
			m.ArxivID = v
		}
		switch {
		case len(d.PublicationDate) == 10:
			m.PublicationDate = d.PublicationDate
		case d.Year > 0:
			m.PublicationDate = fmt.Sprintf("%04d", d.Year)
		}
		if len(d.PublicationTypes) > 0 {
			m.Type = strings.ToLower(d.PublicationTypes[0])
		}
		if d.OpenAccessPdf != nil {
			m.OpenAccessURL = d.OpenAccessPdf.URL
		}
		out = append(out, m)
	}
	return out, nil
}
