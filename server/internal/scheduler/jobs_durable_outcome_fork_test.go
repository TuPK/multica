package scheduler

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeOutcomeRunner struct {
	processErr  error
	scheduleErr error
	settleErr   error
	processN    int
	scheduleN   int
	settleN     int
}

func (f *fakeOutcomeRunner) ProcessTransitions(context.Context) (int, error) {
	f.processN++
	return 0, f.processErr
}

func (f *fakeOutcomeRunner) ScheduleChecks(context.Context) (int, error) {
	f.scheduleN++
	return 0, f.scheduleErr
}

func (f *fakeOutcomeRunner) Settle(context.Context, time.Time) (int, error) {
	f.settleN++
	return 0, f.settleErr
}

func TestDurableOutcomeJobContinuesOnPhaseError(t *testing.T) {
	runner := &fakeOutcomeRunner{processErr: errors.New("process failed")}
	spec := DurableOutcomeJob(runner)
	_, err := spec.Handler(context.Background(), HandlerInput{})
	if err == nil {
		t.Fatal("expected error from failing phase, got nil")
	}
	if runner.processN != 1 || runner.scheduleN != 1 || runner.settleN != 1 {
		t.Fatalf("phase counts = (process=%d, schedule=%d, settle=%d), want (1, 1, 1)", runner.processN, runner.scheduleN, runner.settleN)
	}
}
