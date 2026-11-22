package aicost

import (
	"math"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/pricing"
)

func TestSinceWindows(t *testing.T) {
	now := time.Date(2026, 11, 22, 12, 0, 0, 0, time.UTC)
	created := time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC)
	cases := map[string]time.Time{
		WindowAll: created,
		Window7d:  now.Add(-7 * 24 * time.Hour),
		Window30d: now.Add(-30 * 24 * time.Hour),
		Window90d: now.Add(-90 * 24 * time.Hour),
	}
	for window, want := range cases {
		got, ok := Since(window, created, now)
		if !ok || !got.Equal(want) {
			t.Fatalf("Since(%q) = %v, %v; want %v", window, got, ok, want)
		}
	}
	if _, ok := Since("1y", created, now); ok {
		t.Fatal("Since accepted an unknown window")
	}
}

func TestSummarizeAddsProviderCostAndPricesUncostedTokens(t *testing.T) {
	const provider, model = "anthropic", "claude-sonnet-4.6"
	est, ok := pricing.EstimateCostUSD(provider, model, 1_000_000, 100_000, 0, 0)
	if !ok {
		t.Fatalf("test model %s/%s has no rate row", provider, model)
	}
	rows := []Row{
		{Provider: provider, Model: model, CostUSDTicks: 2 * pricing.CostUSDTicksPerUSD},
		{Provider: provider, Model: model, UncostedInput: 1_000_000, UncostedOutput: 100_000},
	}
	got := Summarize(WindowAll, time.Time{}, time.Time{}, rows)
	if want := 2 + est; math.Abs(got.CostUSD-want) > 1e-9 {
		t.Fatalf("CostUSD = %v, want %v", got.CostUSD, want)
	}
	if !got.Complete {
		t.Fatal("Complete = false for fully priced rows")
	}
}

func TestSummarizeMarksUnpricedTokensIncomplete(t *testing.T) {
	rows := []Row{
		{Provider: "anthropic", Model: "no-such-model-xyz", CostUSDTicks: pricing.CostUSDTicksPerUSD, UncostedInput: 500},
	}
	got := Summarize(Window7d, time.Time{}, time.Time{}, rows)
	if got.CostUSD != 1 {
		t.Fatalf("CostUSD = %v, want the provider cost 1 with no estimate", got.CostUSD)
	}
	if got.Complete {
		t.Fatal("Complete = true with uncosted tokens that have no rate")
	}
}

func TestSummarizeNoRowsIsZeroAndComplete(t *testing.T) {
	got := Summarize(Window30d, time.Time{}, time.Time{}, nil)
	if got.CostUSD != 0 || !got.Complete {
		t.Fatalf("got %+v, want zero cost and complete", got)
	}
}
