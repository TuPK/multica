// Package aicost reads what a workspace spent on AI models, for the staff
// console. The number is the one the workspace dashboard shows: the provider's
// own charge where it reported one, plus the rate-table estimate for the tokens
// it did not price (pricing.EstimateCostUSD).
//
// It reads task_usage_hourly, the rollup Delegora writes, so the newest hour can
// lag the live task rows, the same lag the dashboard has.
package aicost

import (
	"context"
	"fmt"
	"time"

	"github.com/multica-ai/multica/platform-management/internal/db"
	"github.com/multica-ai/multica/platform-management/pricing"
)

// Window names accepted by the console. "all" covers the workspace's life, from
// its creation to now.
const (
	WindowAll = "all"
	Window7d  = "7d"
	Window30d = "30d"
	Window90d = "90d"
)

// Row is one (provider, model) of a workspace over a window. The uncosted
// token counts are the tokens of rows the provider did not price.
type Row struct {
	Provider           string
	Model              string
	CostUSDTicks       int64
	UncostedInput      int64
	UncostedOutput     int64
	UncostedCacheRead  int64
	UncostedCacheWrite int64
}

// Summary is the console's answer for one window. Complete is false when some
// uncosted tokens belong to a model with no rate row, so CostUSD is a floor.
type Summary struct {
	Window   string    `json:"window"`
	Since    time.Time `json:"since"`
	Until    time.Time `json:"until"`
	CostUSD  float64   `json:"cost_usd"`
	Complete bool      `json:"complete"`
}

// Since returns the start of window ending at now. The "all" window starts at
// the workspace's creation. ok is false for a window name this package does not
// know.
func Since(window string, created, now time.Time) (since time.Time, ok bool) {
	switch window {
	case WindowAll:
		return created, true
	case Window7d:
		return now.Add(-7 * 24 * time.Hour), true
	case Window30d:
		return now.Add(-30 * 24 * time.Hour), true
	case Window90d:
		return now.Add(-90 * 24 * time.Hour), true
	}
	return time.Time{}, false
}

// Summarize sums rows into the console's answer. Provider-reported cost is taken
// as is; uncosted tokens are priced with the server's rate table.
func Summarize(window string, since, until time.Time, rows []Row) Summary {
	out := Summary{Window: window, Since: since, Until: until, Complete: true}
	for _, r := range rows {
		out.CostUSD += float64(r.CostUSDTicks) / pricing.CostUSDTicksPerUSD
		uncosted := r.UncostedInput + r.UncostedOutput + r.UncostedCacheRead + r.UncostedCacheWrite
		if uncosted == 0 {
			continue
		}
		est, priced := pricing.EstimateCostUSD(r.Provider, r.Model, r.UncostedInput, r.UncostedOutput, r.UncostedCacheRead, r.UncostedCacheWrite)
		if !priced {
			out.Complete = false
			continue
		}
		out.CostUSD += est
	}
	return out
}

// Load reads the workspace's rows since the given instant, one per provider and
// model. Provider is lowercased as the dashboard rollup does, so case variants
// from older writes merge.
func Load(ctx context.Context, q db.DBTX, workspaceID string, since time.Time) ([]Row, error) {
	rows, err := q.Query(ctx, `
SELECT LOWER(provider) AS provider,
       model,
       COALESCE(SUM(cost_usd_ticks), 0)::bigint AS cost_usd_ticks,
       COALESCE(SUM(COALESCE(uncosted_input_tokens, input_tokens)), 0)::bigint AS uncosted_input,
       COALESCE(SUM(COALESCE(uncosted_output_tokens, output_tokens)), 0)::bigint AS uncosted_output,
       COALESCE(SUM(COALESCE(uncosted_cache_read_tokens, cache_read_tokens)), 0)::bigint AS uncosted_cache_read,
       COALESCE(SUM(COALESCE(uncosted_cache_write_tokens, cache_write_tokens)), 0)::bigint AS uncosted_cache_write
FROM task_usage_hourly
WHERE workspace_id = $1::uuid
  AND bucket_hour >= $2::timestamptz
GROUP BY LOWER(provider), model`, workspaceID, since)
	if err != nil {
		return nil, fmt.Errorf("aicost: query usage: %w", err)
	}
	defer rows.Close()
	var out []Row
	for rows.Next() {
		var r Row
		if err := rows.Scan(&r.Provider, &r.Model, &r.CostUSDTicks, &r.UncostedInput, &r.UncostedOutput, &r.UncostedCacheRead, &r.UncostedCacheWrite); err != nil {
			return nil, fmt.Errorf("aicost: scan usage: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
