package scheduler

import (
	"context"
	"testing"
	"time"
)

type fakeShadowReconciler struct {
	calls int
	n     int
}

func (f *fakeShadowReconciler) ReconcileUnjudgedPairs(context.Context) (int, error) {
	f.calls++
	return f.n, nil
}

func TestDurableShadowReconcileJob(t *testing.T) {
	reconciler := &fakeShadowReconciler{n: 3}
	spec := DurableShadowReconcileJob(reconciler)
	if spec.Name != "durable_shadow_reconcile" {
		t.Fatalf("job name = %q", spec.Name)
	}
	if spec.Cadence != time.Minute {
		t.Fatalf("cadence = %s, want 1m", spec.Cadence)
	}
	res, err := spec.Handler(context.Background(), HandlerInput{})
	if err != nil {
		t.Fatal(err)
	}
	if reconciler.calls != 1 {
		t.Fatalf("ReconcileUnjudgedPairs calls = %d, want 1", reconciler.calls)
	}
	if res.RowsAffected != 3 {
		t.Fatalf("RowsAffected = %d, want 3", res.RowsAffected)
	}
}
