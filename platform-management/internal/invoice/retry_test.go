package invoice

import (
	"errors"
	"testing"
	"time"
)

// TestBackoffSchedule: attempt n waits 1 minute doubled n-1 times, capped at 6
// hours; Decide stops at maxAttempts with failed (spec 7.3 rule 5).
func TestBackoffSchedule(t *testing.T) {
	want := []time.Duration{
		time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute,
		16 * time.Minute, 32 * time.Minute, 64 * time.Minute, 128 * time.Minute,
		256 * time.Minute,
	}
	for i, w := range want {
		if got := Backoff(i + 1); got != w {
			t.Errorf("Backoff(%d) = %s, want %s", i+1, got, w)
		}
	}
	for _, n := range []int{10, 20, 1000} {
		if got := Backoff(n); got != 6*time.Hour {
			t.Errorf("Backoff(%d) = %s, want the 6h cap", n, got)
		}
	}

	transient := &ProviderError{Kind: KindTransient, Code: "HTTP_503"}
	if d := Decide(transient, 7, 8); d.State != StatePending || d.Delay != 64*time.Minute {
		t.Errorf("Decide(transient, 7, 8) = %+v, want pending after 64m", d)
	}
	if d := Decide(transient, 8, 8); d.State != StateFailed {
		t.Errorf("Decide(transient, 8, 8) = %+v, want failed at max attempts", d)
	}
	if d := Decide(errors.New("dial tcp: i/o timeout"), 1, 8); d.State != StatePending || d.Delay != time.Minute {
		t.Errorf("Decide(network error, 1, 8) = %+v, want pending after 1m", d)
	}
	unknown := &ProviderError{Kind: KindUnknownOutcome, Code: "TIMEOUT"}
	if d := Decide(unknown, 1, 8); d.State != StatePending || d.Delay != time.Minute {
		t.Errorf("Decide(unknown outcome, 1, 8) = %+v, want pending after 1m", d)
	}
}

// TestPermanentErrorFailsImmediately: a permanent rejection fails the record
// at once, even on the first attempt, with no delay.
func TestPermanentErrorFailsImmediately(t *testing.T) {
	err := &ProviderError{Kind: KindPermanent, Code: "VALIDATION_ERROR"}
	d := Decide(err, 1, 8)
	if d.State != StateFailed || d.Delay != 0 {
		t.Fatalf("Decide(permanent, 1, 8) = %+v, want failed with no delay", d)
	}
}

// TestMissingFieldErrorFromProviderIsNeedsInfo: a provider that names missing
// fields moves the record to needs_info, not failed and not pending.
func TestMissingFieldErrorFromProviderIsNeedsInfo(t *testing.T) {
	err := &ProviderError{Kind: KindMissingFields, Code: "VALIDATION_ERROR", Fields: []string{"address"}}
	d := Decide(err, 1, 8)
	if d.State != StateNeedsInfo {
		t.Fatalf("Decide(missing fields, 1, 8) = %+v, want needs_info", d)
	}
}
