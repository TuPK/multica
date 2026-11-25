package service

import (
	"reflect"
	"testing"
)

var (
	juryDoer = JurorCandidate{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"}
)

func jurorIDs(jurors []JurorCandidate) []string {
	ids := make([]string, 0, len(jurors))
	for _, j := range jurors {
		ids = append(ids, j.AgentID)
	}
	return ids
}

func TestPickJurorsDiversity(t *testing.T) {
	t.Run("three providers picks two of other providers", func(t *testing.T) {
		pool := []JurorCandidate{
			{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
			{AgentID: "agent-b", Provider: "google", Model: "gemini"},
			{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
			{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"},
		}

		jurors, diversity := PickJurors(pool, juryDoer, 2)

		if len(jurors) != 2 {
			t.Fatalf("want 2 jurors, got %d: %v", len(jurors), jurorIDs(jurors))
		}
		seen := map[string]bool{}
		for _, j := range jurors {
			if j.Provider == juryDoer.Provider {
				t.Errorf("juror %s shares provider %q with doer", j.AgentID, j.Provider)
			}
			if seen[j.Provider] {
				t.Errorf("jurors share provider %q", j.Provider)
			}
			seen[j.Provider] = true
		}
		if diversity != "provider" {
			t.Errorf("diversity = %q, want %q", diversity, "provider")
		}
	})

	t.Run("two providers needs model_only", func(t *testing.T) {
		pool := []JurorCandidate{
			{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
			{AgentID: "agent-b", Provider: "anthropic", Model: "claude-y"},
			{AgentID: "agent-c", Provider: "anthropic", Model: "claude-z"},
		}

		jurors, diversity := PickJurors(pool, juryDoer, 2)

		if len(jurors) != 2 {
			t.Fatalf("want 2 jurors, got %d: %v", len(jurors), jurorIDs(jurors))
		}
		if diversity != "model_only" {
			t.Errorf("diversity = %q, want %q", diversity, "model_only")
		}
	})

	t.Run("only same model available is none but still fills", func(t *testing.T) {
		pool := []JurorCandidate{
			{AgentID: "agent-a", Provider: "anthropic", Model: "claude-x"},
		}

		jurors, diversity := PickJurors(pool, juryDoer, 2)

		if got := jurorIDs(jurors); !reflect.DeepEqual(got, []string{"agent-a"}) {
			t.Errorf("jurors = %v, want [agent-a]", got)
		}
		if diversity != "none" {
			t.Errorf("diversity = %q, want %q", diversity, "none")
		}
	})
}

func TestPickJurorsSkipsDoer(t *testing.T) {
	pool := []JurorCandidate{
		{AgentID: "agent-doer", Provider: "openai", Model: "gpt-4"},
		{AgentID: "agent-b", Provider: "google", Model: "gemini"},
		{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
	}
	doer := JurorCandidate{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"}

	jurors, _ := PickJurors(pool, doer, 3)

	for _, j := range jurors {
		if j.AgentID == doer.AgentID {
			t.Fatalf("doer %s was picked as its own juror: %v", doer.AgentID, jurorIDs(jurors))
		}
	}
	if len(jurors) != 2 {
		t.Errorf("want the 2 non-doer candidates, got %v", jurorIDs(jurors))
	}
}

func TestPickJurorsNoEligible(t *testing.T) {
	pool := []JurorCandidate{juryDoer}

	jurors, diversity := PickJurors(pool, juryDoer, 2)

	if len(jurors) != 0 {
		t.Errorf("want no jurors, got %v", jurorIDs(jurors))
	}
	if diversity != "none" {
		t.Errorf("diversity = %q, want %q", diversity, "none")
	}
}

func TestPickJurorsDeterministic(t *testing.T) {
	pool := []JurorCandidate{
		{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
		{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
		{AgentID: "agent-b", Provider: "google", Model: "gemini"},
		{AgentID: "agent-doer", Provider: "anthropic", Model: "claude-x"},
	}
	reversed := []JurorCandidate{pool[3], pool[2], pool[1], pool[0]}

	first, firstDiversity := PickJurors(pool, juryDoer, 2)
	second, secondDiversity := PickJurors(pool, juryDoer, 2)
	reordered, reorderedDiversity := PickJurors(reversed, juryDoer, 2)

	if !reflect.DeepEqual(first, second) || firstDiversity != secondDiversity {
		t.Errorf("same input gave different results: %v/%q then %v/%q", jurorIDs(first), firstDiversity, jurorIDs(second), secondDiversity)
	}
	if !reflect.DeepEqual(first, reordered) || firstDiversity != reorderedDiversity {
		t.Errorf("pool order changed the result: %v/%q then %v/%q", jurorIDs(first), firstDiversity, jurorIDs(reordered), reorderedDiversity)
	}
}

func TestPickJurorsSizeCap(t *testing.T) {
	pool := []JurorCandidate{
		{AgentID: "agent-a", Provider: "openai", Model: "gpt-4"},
		{AgentID: "agent-b", Provider: "google", Model: "gemini"},
		{AgentID: "agent-c", Provider: "mistral", Model: "mistral-l"},
		{AgentID: "agent-d", Provider: "cohere", Model: "command"},
	}

	cases := []struct {
		name string
		size int
		want int
	}{
		{"size zero", 0, 0},
		{"size two", 2, 2},
		{"size three", 3, 3},
		{"size larger than pool", 10, 4},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			jurors, _ := PickJurors(pool, juryDoer, tc.size)
			if len(jurors) != tc.want {
				t.Errorf("size %d: got %d jurors, want %d: %v", tc.size, len(jurors), tc.want, jurorIDs(jurors))
			}
		})
	}
}
