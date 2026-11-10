package salescollect

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// ProbeReport is what `multica sales probe` prints: the platform of an input, a verdict, and one
// candidate recipe per method that answered, each with a sample of signals. Probe uses no model
// and never uses a paid provider or a login.
type ProbeReport struct {
	Input      string           `json:"input"`
	Platform   string           `json:"platform"`
	Verdict    string           `json:"verdict"`
	Reason     string           `json:"reason"`
	WebEngine  WebEngineStatus  `json:"web_engine"`
	Candidates []ProbeCandidate `json:"candidates"`
}

// WebEngineStatus says whether a Web engine is reachable. The build reads plain HTTP, so it
// reports not_running; the engine is wired in a later phase.
type WebEngineStatus struct {
	Status string `json:"status"`
}

// ProbeCandidate is one way to read the input: its recipe, what the fetch returned, and a sample.
type ProbeCandidate struct {
	Rung          int      `json:"rung"`
	Method        string   `json:"method"`
	Note          string   `json:"note,omitempty"`
	Status        string   `json:"status"`
	HTTPStatus    int      `json:"http_status"`
	ItemsSeen     int      `json:"items_seen"`
	FieldCoverage float64  `json:"field_coverage"`
	Recipe        *Recipe  `json:"recipe,omitempty"`
	Sample        []Signal `json:"sample"`
	Warnings      []string `json:"warnings,omitempty"`
	Error         string   `json:"error,omitempty"`
}

// unlocks says, per social platform, which connection opens it. The sentence is the one the
// Source Studio shows: the connection, what it opens, and what it costs.
var unlocks = map[string]string{
	"facebook":  "Connect Facebook for your own Page (free, OAuth) for its inbox and lead forms; reading other Pages or public groups needs a data provider with a monthly cap (rung 6) or a share (rung 7).",
	"instagram": "Connect Instagram for your own business account (free, OAuth); other public profiles need a data provider with a monthly cap (rung 6).",
	"linkedin":  "Connect LinkedIn for your own account (Composio). Company pages are not read without a login; a list you export, or a post you share, is rung 7.",
	"tiktok":    "Connect your TikTok Business account (free, OAuth) for lead forms and mentions; public videos need a data provider with a monthly cap (rung 6).",
	"zalo":      "Create a Zalo app and grant it to your Official Account (some features need a verified OA or a paid package).",
	"x":         "Connect an X API app (paid per read, about 0.005 USD a post). Off by default.",
	"youtube":   "Connect YouTube (Composio) or add a Google API key; channel comments and videos fit within 10,000 units a day.",
	"maps":      "Add a Places API key (billed per request) or connect Apify with a monthly cap.",
}

var (
	feedHintPattern = regexp.MustCompile(`(?i)<link[^>]+type\s*=\s*["']application/(rss|atom)\+xml["'][^>]*>`)
	hrefPattern     = regexp.MustCompile(`(?i)href\s*=\s*["']([^"']+)["']`)
	ldTypePattern   = regexp.MustCompile(`"@type"\s*:\s*"([A-Za-z]+)"`)
)

// detectPlatform names the platform of an input from its host and path.
func detectPlatform(input string) string {
	host := domainOf(input)
	u, err := url.Parse(normalizeInput(input))
	path := ""
	if err == nil {
		path = strings.ToLower(u.Path)
	}
	switch {
	case host == "facebook.com" || strings.HasSuffix(host, ".facebook.com") || host == "fb.com" || host == "fb.watch":
		return "facebook"
	case host == "instagram.com" || strings.HasSuffix(host, ".instagram.com"):
		return "instagram"
	case host == "linkedin.com" || strings.HasSuffix(host, ".linkedin.com"):
		return "linkedin"
	case host == "tiktok.com" || strings.HasSuffix(host, ".tiktok.com"):
		return "tiktok"
	case host == "youtube.com" || host == "youtu.be" || strings.HasSuffix(host, ".youtube.com"):
		return "youtube"
	case host == "zalo.me" || strings.HasSuffix(host, ".zalo.me"):
		return "zalo"
	case host == "x.com" || host == "twitter.com":
		return "x"
	case host == "reddit.com" || strings.HasSuffix(host, ".reddit.com"):
		return "reddit"
	case host == "t.me" || host == "telegram.me":
		return "telegram"
	case host == "google.com" && strings.HasPrefix(path, "/maps"):
		return "maps"
	case strings.Contains(path, "/forums/") || strings.Contains(path, "/threads/"):
		return "forum"
	}
	return "web"
}

// normalizeInput adds a scheme to a bare domain or handle-like host.
func normalizeInput(input string) string {
	s := strings.TrimSpace(input)
	if s == "" || strings.Contains(s, "://") {
		return s
	}
	return "https://" + s
}

// Probe reads an input the cheap way: it names the platform, fetches the page or feed once,
// tries each open format the response suggests, and runs each candidate on a sample.
func Probe(ctx context.Context, input string, opts Options) ProbeReport {
	report := ProbeReport{
		Input:     strings.TrimSpace(input),
		Platform:  detectPlatform(input),
		WebEngine: WebEngineStatus{Status: "not_running"},
	}
	if opts.Limit == 0 {
		opts.Limit = SampleBar * 4
	}
	target := normalizeInput(input)
	if _, err := url.ParseRequestURI(target); err != nil {
		report.Verdict = "blocked"
		report.Reason = "the input is not a URL or a domain"
		return report
	}
	f := newFetcher(opts)

	if reason, ok := unlocks[report.Platform]; ok {
		report.Verdict = "needs_connector"
		report.Reason = reason
		// One fetch of the public page, for the status it gives, so the Probe log has a line.
		if resp, err := f.get(ctx, target); err == nil {
			report.Candidates = append(report.Candidates, ProbeCandidate{
				Rung: 4, Method: "html_list", Status: "degraded", HTTPStatus: resp.Status,
				Note: "a public page of this platform returns its shell without posts when read without a login",
			})
		}
		return report
	}

	resp, err := f.get(ctx, target)
	if err != nil {
		report.Verdict = "blocked"
		report.Reason = err.Error()
		return report
	}
	if resp.Status < 200 || resp.Status > 299 {
		report.Verdict = "blocked"
		report.Reason = fmt.Sprintf("the page answered %d without a login", resp.Status)
		report.Candidates = append(report.Candidates, ProbeCandidate{Rung: 4, Method: "html_list", Status: "error", HTTPStatus: resp.Status})
		return report
	}

	var candidates []*Recipe
	body := strings.TrimSpace(string(resp.Body))
	switch {
	case strings.HasPrefix(body, "<?xml") || strings.HasPrefix(body, "<rss") || strings.HasPrefix(body, "<feed"):
		candidates = append(candidates, feedRecipe(target, body))
	case strings.HasPrefix(body, "{") || strings.HasPrefix(body, "["):
		if rec := jsonRecipe(target, body); rec != nil {
			candidates = append(candidates, rec)
		}
	default:
		for _, m := range feedHintPattern.FindAllString(body, -1) {
			if href := hrefPattern.FindStringSubmatch(m); href != nil {
				candidates = append(candidates, feedRecipe(absolute(target, href[1]), ""))
			}
		}
		for _, typ := range jsonLDTypes(body) {
			candidates = append(candidates, jsonLDRecipe(target, typ))
		}
	}
	if len(candidates) == 0 {
		report.Verdict = "needs_review"
		report.Reason = "the page has no feed, JSON or JSON-LD list the collector can read; the Scout writes a selector for it"
		report.Candidates = append(report.Candidates, ProbeCandidate{Rung: 4, Method: "html_list", Status: "degraded", HTTPStatus: resp.Status})
		return report
	}

	for _, rec := range candidates {
		res := Run(ctx, rec, opts)
		c := ProbeCandidate{
			Rung: rec.Rung, Method: rec.Method, Status: res.Status, HTTPStatus: res.HTTPStatus,
			ItemsSeen: res.ItemsSeen, FieldCoverage: res.FieldCoverage, Sample: res.Signals,
			Warnings: res.Warnings, Error: res.Error,
		}
		if res.Status == "ok" {
			rec.VerifiedAt = today()
			rec.SampleCount = res.ItemsSeen
			rec.FieldCoverage = res.FieldCoverage
			c.Recipe = rec
		}
		report.Candidates = append(report.Candidates, c)
	}
	for _, c := range report.Candidates {
		if c.Status == "ok" {
			report.Verdict = "active"
			report.Reason = fmt.Sprintf("%s passes the sample: %d items, %.0f%% with all four fields", c.Method, c.ItemsSeen, c.FieldCoverage*100)
			return report
		}
	}
	report.Verdict = "needs_review"
	report.Reason = "no candidate passes the sample bar (5 items, 80% with all four fields); see each candidate's warnings"
	return report
}

// feedRecipe guesses the element names of a feed from its text: a feed usually has pubDate or
// published, and description or summary. body may be empty, in which case the names default.
func feedRecipe(feedURL, body string) *Recipe {
	date := "pubDate"
	switch {
	case strings.Contains(body, "<published"):
		date = "published"
	case strings.Contains(body, "<updated"):
		date = "updated"
	}
	text := "description"
	if !strings.Contains(body, "<description") && strings.Contains(body, "<summary") {
		text = "summary"
	}
	r := skeleton(2, "rss", feedURL)
	r.Parse.Fields = Fields{Entity: "title", URL: "link", ObservedAt: date, Text: text}
	return r
}

// jsonRecipe finds the array of objects in a JSON document and guesses the field names.
func jsonRecipe(target, body string) *Recipe {
	var doc any
	if err := json.Unmarshal([]byte(body), &doc); err != nil {
		return nil
	}
	items := ""
	switch t := doc.(type) {
	case []any:
	case map[string]any:
		keys := make([]string, 0, len(t))
		for k := range t {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if arr, ok := t[k].([]any); ok && len(arr) > 0 {
				if _, isObj := arr[0].(map[string]any); isObj {
					items = k
					break
				}
			}
		}
		if items == "" {
			return nil
		}
	default:
		return nil
	}
	sample := map[string]any{}
	if arr, ok := valueAtPath(doc, items).([]any); ok && len(arr) > 0 {
		if obj, ok := arr[0].(map[string]any); ok {
			sample = obj
		}
	}
	pick := func(candidates ...string) string {
		for _, c := range candidates {
			if _, ok := sample[c]; ok {
				return c
			}
		}
		return candidates[0]
	}
	r := skeleton(3, "json_api", target)
	r.Parse.Items = items
	r.Parse.Fields = Fields{
		Entity:     pick("company_name", "company", "name", "title", "author"),
		URL:        pick("url", "apply_url", "link"),
		ObservedAt: pick("publication_date", "date", "created_at", "published", "updated_at"),
		Text:       pick("title", "description", "summary", "text"),
	}
	return r
}

// jsonLDTypes lists the schema.org types of the page's JSON-LD objects that a list can use.
func jsonLDTypes(body string) []string {
	seen := map[string]bool{}
	var out []string
	for _, m := range jsonLDPattern.FindAllStringSubmatch(body, -1) {
		for _, t := range ldTypePattern.FindAllStringSubmatch(m[1], -1) {
			if !seen[t[1]] && (t[1] == "ItemList" || t[1] == "ListItem" || t[1] == "JobPosting" || t[1] == "Event" || t[1] == "Organization") {
				seen[t[1]] = true
				out = append(out, t[1])
			}
		}
	}
	return out
}

// jsonLDRecipe builds the recipe that reads one schema.org type from a page's JSON-LD.
func jsonLDRecipe(target, typ string) *Recipe {
	r := skeleton(4, "html_jsonld", target)
	r.Parse.Items = typ
	switch typ {
	case "JobPosting":
		r.Parse.Fields = Fields{Entity: "hiringOrganization.name", URL: "url", ObservedAt: "datePosted", Text: "title"}
	case "Event":
		r.Parse.Fields = Fields{Entity: "organizer.name", URL: "url", ObservedAt: "startDate", Text: "name"}
	case "Organization":
		r.Parse.Fields = Fields{Entity: "name", URL: "url", Text: "description"}
	default:
		r.Parse.Fields = Fields{Entity: "item.name", URL: "url", ObservedAt: "", Text: "item.name"}
	}
	return r
}

// skeleton is a recipe with the method and address set, and the rest at its defaults.
func skeleton(rung int, method, target string) *Recipe {
	return &Recipe{
		Version:   1,
		Rung:      rung,
		Method:    method,
		Fetch:     Fetch{Engine: "http", URL: target, Render: "none"},
		Filter:    Filter{SinceDays: 30},
		Cadence:   "daily",
		RateLimit: "1/s",
		MaxItems:  100,
	}
}

// today is the date a recipe is verified on, in the same form the documents use.
func today() string { return now().UTC().Format("2006-01-02") }
