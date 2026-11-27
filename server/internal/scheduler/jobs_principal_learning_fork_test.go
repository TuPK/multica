package scheduler

import (
	"context"
	"testing"
	"time"
)

type fakePrincipalLearner struct {
	calls int
	n     int
}

func (f *fakePrincipalLearner) LearnBatches(context.Context, time.Time) (int, error) {
	f.calls++
	return f.n, nil
}

func TestPrincipalLearningJob(t *testing.T) {
	learner := &fakePrincipalLearner{n: 2}
	spec := PrincipalLearningJob(learner)
	if spec.Name != "principal_learning" {
		t.Fatalf("job name = %q, want principal_learning", spec.Name)
	}
	if spec.Cadence != 6*time.Hour {
		t.Fatalf("cadence = %s, want 6h", spec.Cadence)
	}
	res, err := spec.Handler(context.Background(), HandlerInput{})
	if err != nil {
		t.Fatal(err)
	}
	if learner.calls != 1 {
		t.Fatalf("LearnBatches calls = %d, want 1", learner.calls)
	}
	if res.RowsAffected != 2 {
		t.Fatalf("RowsAffected = %d, want 2", res.RowsAffected)
	}
}
