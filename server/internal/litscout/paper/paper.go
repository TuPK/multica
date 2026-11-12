// Package paper holds the data model of a literature run. Metadata comes from a source
// API only; Analysis is what an agent adds. They are separate types so generated text has
// no field through which it could overwrite a title, an author or an identifier.
package paper

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"
)

// Metadata is a paper as a source reported it. An absent field is the empty string or a nil slice;
// nothing here is ever guessed.
type Metadata struct {
	// ID is a stable handle assigned when the paper first enters the unique list. It does not
	// change when a later record is merged into it.
	ID              string   `json:"id,omitempty"`
	Title           string   `json:"title"`
	Authors         []string `json:"authors"`
	Abstract        string   `json:"abstract,omitempty"`
	DOI             string   `json:"doi,omitempty"`
	OpenAlexID      string   `json:"openalex_id,omitempty"`
	ArxivID         string   `json:"arxiv_id,omitempty"`
	S2ID            string   `json:"s2_id,omitempty"`
	URL             string   `json:"url,omitempty"`
	PublicationDate string   `json:"publication_date,omitempty"` // YYYY-MM-DD, or YYYY / YYYY-MM when the source gives less
	Type            string   `json:"type,omitempty"`             // the source's document type, lower case
	MetadataSource  string   `json:"metadata_source"`
	AbstractSource  string   `json:"abstract_source,omitempty"`
	OpenAccessURL   string   `json:"open_access_url,omitempty"`
	// ReferencedWorks are OpenAlex work ids this paper cites. Used only by citation chasing.
	ReferencedWorks []string `json:"referenced_works,omitempty"`
}

// Analysis is the part an agent writes about a paper.
type Analysis struct {
	RelevanceScore     float64 `json:"relevance_score"`
	RelevanceReason    string  `json:"relevance_reason"`
	Summary            string  `json:"summary,omitempty"`
	ScoredFromAbstract bool    `json:"scored_from_abstract"`
}

// NormalizeDOI lower-cases a DOI and removes a resolver prefix. It returns "" for an empty input.
func NormalizeDOI(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	for _, p := range []string{"https://doi.org/", "http://doi.org/", "https://dx.doi.org/", "http://dx.doi.org/", "doi:"} {
		s = strings.TrimPrefix(s, p)
	}
	return strings.TrimSpace(s)
}

// NormalizeOpenAlexID returns the trailing W-id of an OpenAlex URL.
func NormalizeOpenAlexID(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.LastIndex(s, "/"); i >= 0 {
		s = s[i+1:]
	}
	return s
}

// NewID derives the stable handle from a seed such as a DOI or a title.
func NewID(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	return "p-" + hex.EncodeToString(sum[:])[:10]
}

// Year returns the four-digit year of PublicationDate, or "".
func (m Metadata) Year() string {
	if len(m.PublicationDate) >= 4 {
		return m.PublicationDate[:4]
	}
	return ""
}
