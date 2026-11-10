package salescollect

import (
	"strings"
	"testing"
)

const validRSS = `{
  "version": 1, "rung": 2, "method": "rss",
  "fetch": {"engine": "http", "url": "https://example.vn/feed", "render": "none"},
  "parse": {"items": "", "fields": {"entity": "title", "url": "link", "observed_at": "pubDate", "text": "description"}},
  "filter": {"keywords_any": ["khai trương"], "keywords_none": [], "language": "vi", "since_days": 30},
  "map": {"family": "expansion", "entity_from": "field"},
  "cadence": "daily", "rate_limit": "1/s", "max_items": 100,
  "verified_at": "2026-11-10", "sample_count": 5, "field_coverage": 1, "fallbacks": [{"rung": 4}]
}`

func TestParseRecipeAcceptsVersionOne(t *testing.T) {
	r, err := ParseRecipe([]byte(validRSS))
	if err != nil {
		t.Fatalf("a valid recipe was refused: %v", err)
	}
	if r.Method != "rss" || r.Map.Family != "expansion" || r.MaxItemsOrDefault() != 100 {
		t.Fatalf("parsed recipe = %+v", r)
	}
}

func TestParseRecipeRefusesUnknownFieldsSoTyposAreErrors(t *testing.T) {
	bad := strings.Replace(validRSS, `"cadence": "daily"`, `"cadance": "daily"`, 1)
	if _, err := ParseRecipe([]byte(bad)); err == nil {
		t.Fatal("a misspelled key was accepted")
	}
}

func TestParseRecipeRefusesAnySecretLookingKey(t *testing.T) {
	for _, key := range []string{"api_key", "token", "Authorization", "session_cookie", "password"} {
		raw := strings.Replace(validRSS, `"fallbacks": [{"rung": 4}]`, `"fallbacks": [{"rung": 4, "`+key+`": "x"}]`, 1)
		_, err := ParseRecipe([]byte(raw))
		if err == nil || !strings.Contains(err.Error(), "secret") {
			t.Errorf("key %q was not refused as a secret: %v", key, err)
		}
	}
}

func TestValidateNamesEachProblem(t *testing.T) {
	cases := map[string]string{
		"version must be 1":               strings.Replace(validRSS, `"version": 1`, `"version": 2`, 1),
		"rung must be":                    strings.Replace(validRSS, `"rung": 2`, `"rung": 9`, 1),
		"is not one of the sales methods": strings.Replace(validRSS, `"method": "rss"`, `"method": "scrape"`, 1),
		"not one of the 17 families":      strings.Replace(validRSS, `"family": "expansion"`, `"family": "gardening"`, 1),
		"max_items must be":               strings.Replace(validRSS, `"max_items": 100`, `"max_items": 5000`, 1),
		"verified_at must be":             strings.Replace(validRSS, `"2026-11-10"`, `"10/11/2026"`, 1),
	}
	for want, raw := range cases {
		_, err := ParseRecipe([]byte(raw))
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("want an error containing %q, got %v", want, err)
		}
	}
}

func TestCollectableRefusesWhatRunsElsewhere(t *testing.T) {
	provider := strings.Replace(validRSS, `"method": "rss"`, `"method": "provider", "provider": {"toolkit": "APIFY", "actor": "x", "input": {}, "cost_cap_usd": 0}`, 1)
	r, err := ParseRecipe([]byte(provider))
	if err != nil {
		t.Fatal(err)
	}
	if err := r.Collectable(); err == nil || !strings.Contains(err.Error(), "agent task") {
		t.Fatalf("a provider recipe must be refused here, got %v", err)
	}
	browser := strings.Replace(validRSS, `"render": "none"`, `"render": "browser"`, 1)
	r, _ = ParseRecipe([]byte(browser))
	if err := r.Collectable(); err == nil || !strings.Contains(err.Error(), "html_list and html_jsonld") {
		t.Fatalf("a browser render on a feed must be refused with the methods it applies to, got %v", err)
	}
}

func TestProviderRecipeNeedsItsProvider(t *testing.T) {
	raw := strings.Replace(validRSS, `"method": "rss"`, `"method": "provider"`, 1)
	if _, err := ParseRecipe([]byte(raw)); err == nil || !strings.Contains(err.Error(), "names its provider") {
		t.Fatalf("a provider recipe without a provider was accepted: %v", err)
	}
}
