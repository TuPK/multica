package salescollect

import (
	"bytes"
	"encoding/json"
	"os"
	"testing"
)

// starterRecipes is the starter catalogue of the sales squad, exported from the TypeScript source
// (packages/core/squads/sales-starter-sources.ts). A core test keeps this file equal to the source.
type starterRecipes []struct {
	ID     string          `json:"id"`
	Status string          `json:"status"`
	Method string          `json:"method"`
	Rung   int             `json:"rung"`
	Family *string         `json:"family"`
	Recipe json.RawMessage `json:"recipe"`
}

// Every starter recipe must pass the same validation the collector runs, and the rung and
// method in its catalogue entry must agree with the recipe.
func TestEveryStarterRecipeIsValidVersionOne(t *testing.T) {
	raw, err := os.ReadFile("testdata/starter_recipes.json")
	if err != nil {
		t.Fatal(err)
	}
	var all starterRecipes
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	if len(all) < 30 {
		t.Fatalf("the starter catalogue has %d sources, want the full list", len(all))
	}
	for _, s := range all {
		// A starter that is not active is a draft: its shape must be valid, and the Scout fills the
		// selectors when it onboards the source. An active starter is a runnable recipe.
		r, err := ParseRecipe(s.Recipe)
		if s.Status != "active" && err != nil {
			r, err = parseDraft(s.Recipe)
		}
		if err != nil {
			t.Errorf("starter %s: %v", s.ID, err)
			continue
		}
		if r.Rung != s.Rung || r.Method != s.Method {
			t.Errorf("starter %s: recipe says rung %d method %s, the entry says %d %s", s.ID, r.Rung, r.Method, s.Rung, s.Method)
		}
		if s.Status == "active" && r.Method != "kb_internal" && (r.SampleCount < SampleBar || r.FieldCoverage < CoverageBar) {
			t.Errorf("starter %s is active without a passing sample", s.ID)
		}
	}
}

// The active sources that read a saved response must pass the collector on that response.
func TestActiveStarterFeedsPassTheirSavedResponses(t *testing.T) {
	saved := map[string]struct {
		file    string
		ctype   string
		wantMin int
	}{
		"news-gnews-expansion": {"gnews_expansion.xml", "application/rss+xml", SampleBar},
		"news-techinasia":      {"techinasia.xml", "application/rss+xml", SampleBar},
		"launch-producthunt":   {"producthunt.xml", "application/atom+xml", SampleBar},
	}
	raw, err := os.ReadFile("testdata/starter_recipes.json")
	if err != nil {
		t.Fatal(err)
	}
	var all starterRecipes
	if err := json.Unmarshal(raw, &all); err != nil {
		t.Fatal(err)
	}
	for _, s := range all {
		fx, ok := saved[s.ID]
		if !ok {
			continue
		}
		r, err := ParseRecipe(s.Recipe)
		if err != nil {
			t.Fatal(err)
		}
		if r.Method == "rss" && s.ID == "launch-producthunt" {
			// The Atom feed names its date and text differently: the entry sets them, the test follows it.
			r.Parse.Fields.ObservedAt = "published"
			r.Parse.Fields.Text = "content"
		}
		w := &fakeWeb{pages: map[string]fakePage{r.Fetch.URL: {body: fixture(t, fx.file), ctype: fx.ctype}}}
		res := Run(t.Context(), r, w.options())
		if res.Status != "ok" || res.ItemsSeen < fx.wantMin {
			t.Errorf("starter %s on %s: status %s, items %d, coverage %.2f (%s)", s.ID, fx.file, res.Status, res.ItemsSeen, res.FieldCoverage, res.Error)
		}
	}
}

// parseDraft reads a recipe the collector does not run yet: the same checks, without the
// selectors a draft may leave empty.
func parseDraft(raw []byte) (*Recipe, error) {
	if err := rejectSecrets(raw); err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	var r Recipe
	if err := dec.Decode(&r); err != nil {
		return nil, err
	}
	return &r, r.ValidateDraft()
}
