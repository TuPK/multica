package source

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/multica-ai/multica/server/internal/litscout/paper"
)

// Unpaywall finds a legal open-access copy of a DOI. It never returns a paywalled address.
type Unpaywall struct {
	Client  *Client
	BaseURL string // default https://api.unpaywall.org/v2
	Email   string // required by Unpaywall
}

// OALocation returns the PDF URL of the best open-access copy. found is false when the paper is
// not open access or Unpaywall has no record.
func (u *Unpaywall) OALocation(ctx context.Context, doi string) (pdfURL string, found bool, err error) {
	doi = paper.NormalizeDOI(doi)
	if doi == "" {
		return "", false, nil
	}
	if u.Email == "" {
		return "", false, errors.New("unpaywall: a contact email is required (set the contact email in Research settings or UNPAYWALL_EMAIL)")
	}
	base := strings.TrimRight(u.BaseURL, "/")
	if base == "" {
		base = "https://api.unpaywall.org/v2"
	}
	body, err := u.Client.Get(ctx, Request{
		Source: "unpaywall", Query: "doi:" + doi,
		URL: base + "/" + url.PathEscape(doi), Params: url.Values{"email": {u.Email}},
	})
	if err != nil {
		var he *HTTPError
		if errors.As(err, &he) && he.Status == 404 {
			return "", false, nil
		}
		return "", false, err
	}
	var resp struct {
		IsOA           bool `json:"is_oa"`
		BestOALocation *struct {
			URLForPDF string `json:"url_for_pdf"`
		} `json:"best_oa_location"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", false, fmt.Errorf("unpaywall: malformed response")
	}
	if !resp.IsOA || resp.BestOALocation == nil || resp.BestOALocation.URLForPDF == "" {
		return "", false, nil
	}
	return resp.BestOALocation.URLForPDF, true, nil
}
