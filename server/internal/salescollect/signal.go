package salescollect

import (
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Signal is one item from a source, normalised. It is the shape the Scout scores (spec 4.4).
type Signal struct {
	SourceID   string `json:"source_id,omitempty"`
	Family     string `json:"family"`
	EntityName string `json:"entity_name"`
	EntityKey  string `json:"entity_key"`
	URL        string `json:"url"`
	ObservedAt string `json:"observed_at"`
	Summary    string `json:"summary"`
	RawRef     string `json:"raw_ref"`
	Region     string `json:"region"`
	SectorHint string `json:"sector_hint"`
}

// record is one item as the adapter read it, with the four field values already mapped.
type record struct {
	Entity     string
	URL        string
	ObservedAt string
	Text       string
	Website    string
	RawRef     string
}

// fieldsComplete reports whether an item has all four fields the sample check needs.
func (r record) fieldsComplete() bool {
	return r.Entity != "" && r.URL != "" && r.ObservedAt != "" && r.Text != ""
}

var (
	taxCodePattern = regexp.MustCompile(`\b\d{10}(?:-\d{3})?\b`)
	spacePattern   = regexp.MustCompile(`\s+`)
	tagPattern     = regexp.MustCompile(`<[^>]{1,200}>`)
)

// maxSummary is the longest summary a signal keeps.
const maxSummary = 280

// normalizeDate returns YYYY-MM-DD for the date formats sources use, or "" when it cannot tell.
// A bare number of ten or more digits is a Unix time in seconds.
func normalizeDate(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil && len(s) >= 9 {
		return time.Unix(n, 0).UTC().Format("2006-01-02")
	}
	layouts := []string{
		time.RFC3339, time.RFC3339Nano, time.RFC1123Z, time.RFC1123,
		"2006-01-02T15:04:05", "2006-01-02 15:04:05", "2006-01-02", "02/01/2006",
		"Mon, 2 Jan 2006 15:04:05 MST", "Mon, 02 Jan 2006 15:04:05 -0700",
	}
	for _, layout := range layouts {
		if t, err := time.Parse(layout, s); err == nil {
			return t.UTC().Format("2006-01-02")
		}
	}
	return ""
}

// cleanText drops markup, then trims a summary to its first maxSummary runes on one line.
func cleanText(s string) string {
	s = tagPattern.ReplaceAllString(s, " ")
	s = spacePattern.ReplaceAllString(strings.TrimSpace(s), " ")
	if r := []rune(s); len(r) > maxSummary {
		return string(r[:maxSummary])
	}
	return s
}

// domainOf returns the lower-case host of a URL or a bare domain, without "www.".
func domainOf(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	if !strings.Contains(s, "://") {
		s = "https://" + s
	}
	u, err := url.Parse(s)
	if err != nil {
		return ""
	}
	host := strings.ToLower(u.Hostname())
	return strings.TrimPrefix(host, "www.")
}

// EntityKey is the stable key an organisation is deduplicated by (spec 4.4): its web domain,
// else its tax code, else its normalised name with a province. The same organisation gets the
// same key on every run.
func EntityKey(name, website, text string) string {
	if d := domainOf(website); d != "" {
		return "domain:" + d
	}
	if m := taxCodePattern.FindString(text); m != "" {
		return "tax:" + m
	}
	return "name:" + normalizeName(name)
}

// normalizeName lower-cases a name, drops punctuation, and collapses spaces. Accents are kept:
// a Vietnamese name stays distinct from its unaccented twin only when the source spells it so.
func normalizeName(s string) string {
	var b strings.Builder
	space := false
	for _, r := range strings.ToLower(s) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			space = false
		case unicode.IsSpace(r) || r == '-' || r == '.' || r == '&' || r == ',' || r == '\'':
			if !space && b.Len() > 0 {
				b.WriteByte(' ')
				space = true
			}
		}
	}
	return strings.TrimSpace(b.String())
}

// toSignal turns a record into a signal for a recipe. Empty fields stay empty; the caller
// decides what to keep.
func toSignal(rec record, r *Recipe, sourceID string, opts Options) Signal {
	family := r.Map.Family
	name := cleanText(rec.Entity)
	return Signal{
		SourceID:   sourceID,
		Family:     family,
		EntityName: name,
		EntityKey:  EntityKey(name, rec.Website, rec.Text),
		URL:        strings.TrimSpace(rec.URL),
		ObservedAt: normalizeDate(rec.ObservedAt),
		Summary:    cleanText(rec.Text),
		RawRef:     rec.RawRef,
		Region:     opts.Region,
		SectorHint: opts.Sector,
	}
}
