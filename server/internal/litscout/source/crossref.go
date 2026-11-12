package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// Crossref searches api.crossref.org and resolves DOIs. Mailto puts the caller in Crossref's
// polite pool; it is a contact address, not a secret.
type Crossref struct {
	Client  *Client
	BaseURL string
	Mailto  string
	// HandleURL is the DOI proxy's handle API, which knows a DOI whatever agency registered it.
	// Default https://doi.org/api/handles.
	HandleURL string
}

func (c *Crossref) Name() string { return "crossref" }

func (c *Crossref) base() string {
	if c.BaseURL != "" {
		return strings.TrimRight(c.BaseURL, "/")
	}
	return "https://api.crossref.org"
}

type crossrefWork struct {
	DOI      string   `json:"DOI"`
	URL      string   `json:"URL"`
	Type     string   `json:"type"`
	Title    []string `json:"title"`
	Abstract string   `json:"abstract"`
	Author   []struct {
		Given  string `json:"given"`
		Family string `json:"family"`
		Name   string `json:"name"`
	} `json:"author"`
	Issued struct {
		DateParts [][]int `json:"date-parts"`
	} `json:"issued"`
}

func (w crossrefWork) normalize() paper.Metadata {
	m := paper.Metadata{
		DOI:            paper.NormalizeDOI(w.DOI),
		URL:            w.URL,
		Type:           strings.ToLower(w.Type),
		MetadataSource: "Crossref",
	}
	if len(w.Title) > 0 {
		m.Title = strings.TrimSpace(w.Title[0])
	}
	for _, a := range w.Author {
		name := strings.TrimSpace(a.Given + " " + a.Family)
		if name == "" {
			name = strings.TrimSpace(a.Name)
		}
		if name != "" {
			m.Authors = append(m.Authors, name)
		}
	}
	if ab := StripTags(w.Abstract); ab != "" {
		m.Abstract, m.AbstractSource = ab, "Crossref"
	}
	if len(w.Issued.DateParts) > 0 && len(w.Issued.DateParts[0]) > 0 {
		p := w.Issued.DateParts[0]
		m.PublicationDate = fmt.Sprintf("%04d", p[0])
		if len(p) > 1 {
			m.PublicationDate += fmt.Sprintf("-%02d", p[1])
		}
		if len(p) > 2 {
			m.PublicationDate += fmt.Sprintf("-%02d", p[2])
		}
	}
	return m
}

// StripTags removes markup (Crossref abstracts are JATS XML) and collapses whitespace.
func StripTags(s string) string {
	var b strings.Builder
	in := false
	for _, r := range s {
		switch {
		case r == '<':
			in = true
			b.WriteRune(' ')
		case r == '>':
			in = false
		case !in:
			b.WriteRune(r)
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func (c *Crossref) Search(ctx context.Context, q Query) ([]paper.Metadata, error) {
	if q.Limit <= 0 {
		return nil, nil
	}
	p := url.Values{
		"query.bibliographic": {q.Text},
		"rows":                {strconv.Itoa(min(q.Limit, 100))},
		"select":              {"DOI,title,author,abstract,issued,URL,type"},
	}
	if q.FromYear > 0 {
		p.Set("filter", fmt.Sprintf("from-pub-date:%d-01-01", q.FromYear))
	}
	if c.Mailto != "" {
		p.Set("mailto", c.Mailto)
	}
	body, err := c.Client.Get(ctx, Request{Source: "crossref", Query: q.Text, URL: c.base() + "/works", Params: p})
	if err != nil {
		return nil, err
	}
	var resp struct {
		Message struct {
			Items []crossrefWork `json:"items"`
		} `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("crossref: malformed response")
	}
	out := make([]paper.Metadata, 0, len(resp.Message.Items))
	for _, w := range resp.Message.Items {
		out = append(out, w.normalize())
	}
	return out, nil
}

// Resolve looks a DOI up. found is false when Crossref has no such DOI (HTTP 404).
func (c *Crossref) Resolve(ctx context.Context, doi string) (m paper.Metadata, found bool, err error) {
	doi = paper.NormalizeDOI(doi)
	if doi == "" {
		return m, false, nil
	}
	p := url.Values{}
	if c.Mailto != "" {
		p.Set("mailto", c.Mailto)
	}
	body, err := c.Client.Get(ctx, Request{Source: "crossref", Query: "doi:" + doi, URL: c.base() + "/works/" + url.PathEscape(doi), Params: p})
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			return m, false, nil
		}
		return m, false, err
	}
	var resp struct {
		Message crossrefWork `json:"message"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return m, false, fmt.Errorf("crossref: malformed response")
	}
	return resp.Message.normalize(), true, nil
}

// Registered asks the DOI proxy (doi.org) whether a DOI is registered at all. Crossref only knows
// DOIs that Crossref registered; the DOIs of arXiv (10.48550) and Zenodo belong to DataCite and are
// a 404 at Crossref although they resolve. A 404 here means the DOI does not exist.
func (c *Crossref) Registered(ctx context.Context, doi string) (bool, error) {
	doi = paper.NormalizeDOI(doi)
	if doi == "" {
		return false, nil
	}
	base := strings.TrimRight(c.HandleURL, "/")
	if base == "" {
		base = "https://doi.org/api/handles"
	}
	body, err := c.Client.Get(ctx, Request{Source: "doi", Query: "doi:" + doi, URL: base + "/" + doi})
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			return false, nil
		}
		return false, err
	}
	var resp struct {
		ResponseCode int `json:"responseCode"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return false, fmt.Errorf("doi: malformed response")
	}
	return resp.ResponseCode == 1, nil
}
