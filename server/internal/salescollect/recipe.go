// Package salescollect reads the sources of a Sales Loop squad. A source's recipe says how
// to read it; this package runs the recipe without a model and turns the items into signals
// the squad can score. The agent writes and repairs recipes; this code executes them.
//
// The recipe format is version 1 (see docs/engineering/sales-loop-squad.md, section 4.9). Only the
// collector methods run here: rss, json_api, html_jsonld, html_list and csv_drop. The other
// methods (social_connector, provider, human_share, kb_internal, web_search, mailbox) run inside an
// agent task and are refused here with the reason.
package salescollect

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// Families are the 17 signal families of the spec (section 4.3).
var Families = map[string]bool{
	"hiring": true, "funding": true, "expansion": true, "launch": true, "regulatory": true,
	"tender": true, "event": true, "pain": true, "site_change": true, "registry": true,
	"competitor_customer": true, "trade_flow": true, "leadership_change": true, "directory": true,
	"own_data": true, "inbound": true, "ad_activity": true,
}

// Methods are the adapter methods a recipe may name.
var Methods = map[string]bool{
	"rss": true, "json_api": true, "html_jsonld": true, "html_list": true, "csv_drop": true,
	"kb_internal": true, "web_search": true, "social_connector": true, "provider": true,
	"human_share": true, "mailbox": true,
}

// collectorMethods run inside this package.
var collectorMethods = map[string]bool{
	"rss": true, "json_api": true, "html_jsonld": true, "html_list": true, "csv_drop": true,
}

// MaxItemsCeiling bounds max_items in any recipe.
const MaxItemsCeiling = 1000

// Recipe is version 1 of a source recipe. Field names are the JSON names of the recipe.
type Recipe struct {
	Version       int        `json:"version"`
	Rung          int        `json:"rung"`
	Method        string     `json:"method"`
	Fetch         Fetch      `json:"fetch"`
	Parse         Parse      `json:"parse"`
	Filter        Filter     `json:"filter"`
	Map           Map        `json:"map"`
	Provider      *Provider  `json:"provider,omitempty"`
	Cadence       string     `json:"cadence"`
	RateLimit     string     `json:"rate_limit"`
	MaxItems      int        `json:"max_items"`
	VerifiedAt    string     `json:"verified_at"`
	SampleCount   int        `json:"sample_count"`
	FieldCoverage float64    `json:"field_coverage"`
	Fallbacks     []Fallback `json:"fallbacks"`
}

// Fetch says where the items are and how to get them.
type Fetch struct {
	Engine     string            `json:"engine"`
	URL        string            `json:"url"`
	Params     map[string]string `json:"params,omitempty"`
	Pagination *Pagination       `json:"pagination,omitempty"`
	Render     string            `json:"render"`
}

// Pagination pages a JSON or feed URL by a query parameter.
type Pagination struct {
	Type     string `json:"type"`
	MaxPages int    `json:"max_pages"`
	Param    string `json:"param,omitempty"`
}

// Parse maps the items of a response to the four fields of a signal.
type Parse struct {
	Items  string          `json:"items"`
	Fields Fields          `json:"fields"`
	Schema json.RawMessage `json:"schema,omitempty"`
}

// Fields names where each field lives in an item: a tag, a dot path, a selector, or a column.
type Fields struct {
	Entity     string `json:"entity"`
	URL        string `json:"url"`
	ObservedAt string `json:"observed_at"`
	Text       string `json:"text"`
	Website    string `json:"website,omitempty"`
}

// Filter keeps the items that match. Empty keyword lists match everything.
type Filter struct {
	KeywordsAny  []string `json:"keywords_any"`
	KeywordsNone []string `json:"keywords_none"`
	Language     string   `json:"language"`
	SinceDays    int      `json:"since_days"`
}

// Map gives each signal its family and how its entity is named.
type Map struct {
	Family     string `json:"family"`
	EntityFrom string `json:"entity_from"`
}

// Provider names a paid provider and its cost cap. A provider recipe does not run here.
type Provider struct {
	Toolkit    string         `json:"toolkit"`
	Actor      string         `json:"actor"`
	Input      map[string]any `json:"input"`
	CostCapUSD float64        `json:"cost_cap_usd"`
}

// Fallback names a rung to try when this recipe stops working.
type Fallback struct {
	Rung int `json:"rung"`
}

var (
	secretKeyPattern = regexp.MustCompile(`(?i)token|api[_-]?key|apikey|password|secret|cookie|authorization|session`)
	dayPattern       = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)
)

// ParseRecipe decodes a recipe and validates it. Unknown fields are refused, so a typo in a
// recipe is an error rather than a silent default.
func ParseRecipe(raw []byte) (*Recipe, error) {
	if err := rejectSecrets(raw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r Recipe
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("recipe: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// rejectSecrets refuses a recipe that holds a token, key, password or cookie under any key.
// Secrets live in the connection the agent owns, never in a recipe.
func rejectSecrets(raw []byte) error {
	var generic any
	if err := json.Unmarshal(raw, &generic); err != nil {
		return fmt.Errorf("recipe: %w", err)
	}
	if path := findSecretKey(generic, "recipe"); path != "" {
		return fmt.Errorf("recipe holds a secret-looking key at %s: a recipe never stores a credential", path)
	}
	return nil
}

func findSecretKey(v any, path string) string {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			if secretKeyPattern.MatchString(k) {
				return path + "." + k
			}
			if hit := findSecretKey(child, path+"."+k); hit != "" {
				return hit
			}
		}
	case []any:
		for i, child := range t {
			if hit := findSecretKey(child, fmt.Sprintf("%s[%d]", path, i)); hit != "" {
				return hit
			}
		}
	}
	return ""
}

// Validate checks a recipe that the collector is about to run: its shape, and the fields a
// collector method needs to read an item.
func (r *Recipe) Validate() error {
	if err := r.ValidateDraft(); err != nil {
		return err
	}
	var problems []string
	if collectorMethods[r.Method] && r.Method != "csv_drop" && strings.TrimSpace(r.Fetch.URL) == "" {
		problems = append(problems, fmt.Sprintf("fetch.url is required for method %s", r.Method))
	}
	if collectorMethods[r.Method] && (r.Parse.Fields.Entity == "" || r.Parse.Fields.URL == "") {
		problems = append(problems, fmt.Sprintf("parse.fields needs entity and url for method %s", r.Method))
	}
	if len(problems) > 0 {
		return errors.New("recipe: " + strings.Join(problems, "; "))
	}
	return nil
}

// ValidateDraft checks the shape of a recipe that is still being written: the version, rung,
// method, families, and enumerated values. A draft may leave its selectors empty; the Scout
// fills them in during onboarding, and only a recipe that passes Validate runs.
func (r *Recipe) ValidateDraft() error {
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }

	if r.Version != 1 {
		add("version must be 1, got %d", r.Version)
	}
	if r.Rung < 1 || r.Rung > 7 {
		add("rung must be between 1 and 7, got %d", r.Rung)
	}
	if !Methods[r.Method] {
		add("method %q is not one of the sales methods", r.Method)
	}
	if r.Map.Family != "" && !Families[r.Map.Family] {
		add("map.family %q is not one of the 17 families", r.Map.Family)
	}
	switch r.Map.EntityFrom {
	case "", "field", "page_name", "link_domain", "author":
	default:
		add("map.entity_from %q is not field, page_name, link_domain or author", r.Map.EntityFrom)
	}
	switch r.Fetch.Engine {
	case "", "http", "crawl4ai":
	default:
		add("fetch.engine must be http or crawl4ai, got %q", r.Fetch.Engine)
	}
	switch r.Fetch.Render {
	case "", "none", "browser":
	default:
		add("fetch.render must be none or browser, got %q", r.Fetch.Render)
	}
	switch r.Cadence {
	case "", "daily", "weekly", "manual":
	default:
		add("cadence must be daily, weekly or manual, got %q", r.Cadence)
	}
	if r.MaxItems < 0 || r.MaxItems > MaxItemsCeiling {
		add("max_items must be between 0 and %d, got %d", MaxItemsCeiling, r.MaxItems)
	}
	if r.FieldCoverage < 0 || r.FieldCoverage > 1 {
		add("field_coverage must be between 0 and 1, got %v", r.FieldCoverage)
	}
	if r.VerifiedAt != "" && !dayPattern.MatchString(r.VerifiedAt) {
		add("verified_at must be YYYY-MM-DD, got %q", r.VerifiedAt)
	}
	if r.Fetch.Pagination != nil {
		switch r.Fetch.Pagination.Type {
		case "none", "page", "cursor":
		default:
			add("fetch.pagination.type must be none, page or cursor, got %q", r.Fetch.Pagination.Type)
		}
		if r.Fetch.Pagination.MaxPages < 0 || r.Fetch.Pagination.MaxPages > 20 {
			add("fetch.pagination.max_pages must be between 0 and 20")
		}
	}
	if r.Method == "provider" && r.Provider == nil {
		add("a provider recipe names its provider")
	}
	if r.Provider != nil && r.Provider.CostCapUSD < 0 {
		add("provider.cost_cap_usd must not be negative")
	}
	for _, f := range r.Fallbacks {
		if f.Rung < 1 || f.Rung > 7 {
			add("fallbacks rung must be between 1 and 7, got %d", f.Rung)
		}
	}
	if len(problems) > 0 {
		return errors.New("recipe: " + strings.Join(problems, "; "))
	}
	return nil
}

// Collectable reports whether the collector runs this recipe itself. Other methods run in an
// agent task, and the reason is returned when they do not.
func (r *Recipe) Collectable() error {
	if collectorMethods[r.Method] {
		if r.usesEngine() && r.Method != "html_list" && r.Method != "html_jsonld" {
			return fmt.Errorf("a browser render applies to html_list and html_jsonld, not %s", r.Method)
		}
		return nil
	}
	return fmt.Errorf("method %s runs in an agent task, not in multica sales collect", r.Method)
}

// usesEngine reports whether the recipe reads its page through the Web engine.
func (r *Recipe) usesEngine() bool {
	return r.Fetch.Render == "browser" || r.Fetch.Engine == "crawl4ai"
}

// MaxItemsOrDefault is the item cap of a run.
func (r *Recipe) MaxItemsOrDefault() int {
	if r.MaxItems <= 0 {
		return 100
	}
	return r.MaxItems
}

// now is replaced in tests.
var now = time.Now
