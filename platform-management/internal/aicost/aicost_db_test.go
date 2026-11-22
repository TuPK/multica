package aicost

import (
	"context"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/testdb"
	"github.com/multica-ai/multica/platform-management/pricing"
)

// Load sums the rollup rows of one workspace since a cutoff, merging provider
// case variants and leaving other workspaces and older hours out.
func TestLoadSumsTheWorkspaceRollupSinceTheCutoff(t *testing.T) {
	pool := testdb.Open(t)
	ctx := context.Background()
	var ws, runtime, agent string
	if err := pool.QueryRow(ctx, `SELECT gen_random_uuid()::text, gen_random_uuid()::text, gen_random_uuid()::text`).Scan(&ws, &runtime, &agent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM task_usage_hourly WHERE workspace_id = $1::uuid`, ws)
	})

	now := time.Now().UTC().Truncate(time.Hour)
	insert := func(hoursAgo int, provider, model string, ticks, uncostedInput int64) {
		t.Helper()
		_, err := pool.Exec(ctx, `
INSERT INTO task_usage_hourly (bucket_hour, workspace_id, runtime_id, agent_id, provider, model,
    input_tokens, output_tokens, cache_read_tokens, cache_write_tokens, cost_usd_ticks,
    uncosted_input_tokens, uncosted_output_tokens, uncosted_cache_read_tokens, uncosted_cache_write_tokens)
VALUES ($1, $2::uuid, $3::uuid, $4::uuid, $5, $6, $7, 0, 0, 0, $8, $9, 0, 0, 0)`,
			now.Add(-time.Duration(hoursAgo)*time.Hour), ws, runtime, agent, provider, model,
			uncostedInput, ticks, uncostedInput)
		if err != nil {
			t.Fatal(err)
		}
	}
	insert(1, "Anthropic", "claude-sonnet-4.6", 3*pricing.CostUSDTicksPerUSD, 0)
	insert(2, "anthropic", "claude-sonnet-4.6", 0, 1_000_000)
	insert(48, "anthropic", "claude-sonnet-4.6", 100*pricing.CostUSDTicksPerUSD, 0)

	since := now.Add(-24 * time.Hour)
	rows, err := Load(ctx, pool, ws, since)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %+v, want one merged provider/model row", rows)
	}
	got := rows[0]
	if got.Provider != "anthropic" || got.CostUSDTicks != 3*pricing.CostUSDTicksPerUSD || got.UncostedInput != 1_000_000 {
		t.Fatalf("merged row = %+v", got)
	}
	summary := Summarize(WindowAll, since, now, rows)
	if !summary.Complete || summary.CostUSD < 3 {
		t.Fatalf("summary = %+v, want the provider 3 plus the priced estimate", summary)
	}
}
