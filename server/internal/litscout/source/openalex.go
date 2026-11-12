package source

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

const openAlexSelect = "id,doi,title,authorships,abstract_inverted_index,publication_date,primary_location,ids,type,is_paratext,referenced_works,open_access"

// OpenAlex searches https://api.openalex.org/works.
type OpenAlex struct {
	Client  *Client
	BaseURL string // default https://api.openalex.org
	APIKey  string
}

func (o *OpenAlex) Name() string { return "openalex" }

func (o *OpenAlex) base() string {
	if o.BaseURL != "" {
		return strings.TrimRight(o.BaseURL, "/")
	}
	return "https://api.openalex.org"
}

func (o *OpenAlex) fetch(ctx context.Context, query string, params url.Values) ([]paper.Metadata, error) {
	params.Set("select", openAlexSelect)
	req := Request{Source: "openalex", Query: query, URL: o.base() + "/works", Params: params}
	if o.APIKey != "" {
		req.Secret = url.Values{"api_key": {o.APIKey}}
	}
	body, err := o.Client.Get(ctx, req)
	if err != nil {
		return nil, err
	}
	var page struct {
		Results []json.RawMessage `json:"results"`
	}
	if err := json.Unmarshal(body, &page); err != nil {
		return nil, fmt.Errorf("openalex: malformed response")
	}
	out := make([]paper.Metadata, 0, len(page.Results))
	for _, raw := range page.Results {
		var w openAlexWork
		if err := json.Unmarshal(raw, &w); err != nil {
			continue // one malformed work does not spoil the page
		}
		out = append(out, w.normalize())
	}
	return out, nil
}

// Search runs a text search.
func (o *OpenAlex) Search(ctx context.Context, q Query) ([]paper.Metadata, error) {
	if q.Limit <= 0 {
		return nil, nil
	}
	p := url.Values{"search": {q.Text}, "per_page": {strconv.Itoa(min(q.Limit, 100))}}
	if q.FromYear > 0 {
		p.Set("filter", fmt.Sprintf("from_publication_date:%d-01-01", q.FromYear))
	}
	return o.fetch(ctx, q.Text, p)
}

// CitedBy returns works that cite the given OpenAlex work.
func (o *OpenAlex) CitedBy(ctx context.Context, workID string, limit int) ([]paper.Metadata, error) {
	id := paper.NormalizeOpenAlexID(workID)
	if id == "" || limit <= 0 {
		return nil, nil
	}
	p := url.Values{"filter": {"cites:" + id}, "per_page": {strconv.Itoa(min(limit, 100))}}
	return o.fetch(ctx, "cites:"+id, p)
}

// ByIDs returns the given OpenAlex works (at most 50 per call).
func (o *OpenAlex) ByIDs(ctx context.Context, ids []string) ([]paper.Metadata, error) {
	clean := make([]string, 0, len(ids))
	for _, id := range ids {
		if n := paper.NormalizeOpenAlexID(id); n != "" {
			clean = append(clean, n)
		}
	}
	if len(clean) == 0 {
		return nil, nil
	}
	if len(clean) > 50 {
		clean = clean[:50]
	}
	joined := strings.Join(clean, "|")
	p := url.Values{"filter": {"openalex:" + joined}, "per_page": {strconv.Itoa(len(clean))}}
	return o.fetch(ctx, "ids:"+joined, p)
}

type openAlexWork struct {
	ID              string           `json:"id"`
	DOI             string           `json:"doi"`
	Title           string           `json:"title"`
	PublicationDate string           `json:"publication_date"`
	Type            string           `json:"type"`
	IsParatext      bool             `json:"is_paratext"`
	AbstractIndex   map[string][]int `json:"abstract_inverted_index"`
	ReferencedWorks []string         `json:"referenced_works"`
	Authorships     []struct {
		Author *struct {
			DisplayName string `json:"display_name"`
		} `json:"author"`
	} `json:"authorships"`
	IDs struct {
		DOI string `json:"doi"`
	} `json:"ids"`
	PrimaryLocation *struct {
		LandingPageURL string `json:"landing_page_url"`
	} `json:"primary_location"`
	OpenAccess *struct {
		OAURL string `json:"oa_url"`
	} `json:"open_access"`
}

func (w openAlexWork) normalize() paper.Metadata {
	abstract := RestoreAbstract(w.AbstractIndex)
	var authors []string
	for _, a := range w.Authorships {
		if a.Author != nil && a.Author.DisplayName != "" {
			authors = append(authors, a.Author.DisplayName)
		}
	}
	doi := w.DOI
	if doi == "" {
		doi = w.IDs.DOI
	}
	m := paper.Metadata{
		Title:           strings.TrimSpace(w.Title),
		Authors:         authors,
		Abstract:        abstract,
		DOI:             paper.NormalizeDOI(doi),
		OpenAlexID:      paper.NormalizeOpenAlexID(w.ID),
		URL:             w.ID,
		Type:            strings.ToLower(w.Type),
		MetadataSource:  "OpenAlex",
		ReferencedWorks: w.ReferencedWorks,
	}
	if w.PrimaryLocation != nil && w.PrimaryLocation.LandingPageURL != "" {
		m.URL = w.PrimaryLocation.LandingPageURL
	}
	if w.IsParatext {
		m.Type = "paratext"
	}
	if abstract != "" {
		m.AbstractSource = "OpenAlex"
	}
	if _, err := time.Parse("2006-01-02", w.PublicationDate); err == nil {
		m.PublicationDate = w.PublicationDate
	}
	if w.OpenAccess != nil {
		m.OpenAccessURL = w.OpenAccess.OAURL
	}
	return m
}

// RestoreAbstract rebuilds text from OpenAlex's inverted index. Words are ordered by position, and
// words at the same position by spelling, so the result does not depend on map order.
func RestoreAbstract(index map[string][]int) string {
	type pw struct {
		pos  int
		word string
	}
	var all []pw
	for word, positions := range index {
		for _, p := range positions {
			all = append(all, pw{p, word})
		}
	}
	if len(all) == 0 {
		return ""
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].pos != all[j].pos {
			return all[i].pos < all[j].pos
		}
		return all[i].word < all[j].word
	})
	words := make([]string, len(all))
	for i, x := range all {
		words[i] = x.word
	}
	return strings.Join(words, " ")
}
