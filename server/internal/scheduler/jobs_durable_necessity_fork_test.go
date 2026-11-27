package scheduler

import (
	"context"
	"testing"
	"time"
)

type fakeNecessity struct {
	calls int
	n     int
}

func (f *fakeNecessity) Weekly(context.Context) (int, error) {
	f.calls++
	return f.n, nil
}

func (f *fakeNecessity) MonthlyFlowStages(context.Context) (int, error) {
	f.calls++
	return f.n, nil
}

// TestDurableNecessityJobsRunOnTheirCadence: the weekly necessity schedule and the monthly flow
// stage count each run on their cadence and report how many tests or candidates they found (spec 10.3, 10.4).
func TestDurableNecessityJobsRunOnTheirCadence(t *testing.T) {
	weekly := &fakeNecessity{n: 2}
	spec := DurableNecessityJob(weekly)
	if spec.Cadence != 7*24*time.Hour {
		t.Fatalf("weekly cadence = %v, want 7 days", spec.Cadence)
	}
	res, err := spec.Handler(context.Background(), HandlerInput{})
	if err != nil || weekly.calls != 1 || res.RowsAffected != 2 {
		t.Fatalf("weekly run: calls %d rows %d err %v, want 1 call and 2 rows", weekly.calls, res.RowsAffected, err)
	}

	monthly := &fakeNecessity{n: 1}
	spec = DurableFlowStageJob(monthly)
	if spec.Cadence != 30*24*time.Hour {
		t.Fatalf("monthly cadence = %v, want 30 days", spec.Cadence)
	}
	res, err = spec.Handler(context.Background(), HandlerInput{})
	if err != nil || monthly.calls != 1 || res.RowsAffected != 1 {
		t.Fatalf("monthly run: calls %d rows %d err %v, want 1 call and 1 row", monthly.calls, res.RowsAffected, err)
	}
}
