package billing

import (
	"errors"
	"testing"
	"time"
)

var (
	created = time.Date(2025, 3, 1, 10, 0, 0, 0, time.UTC)
	expires = created.Add(OrderLifetime)
)

func TestOrderLifetimeIsSevenDays(t *testing.T) {
	if OrderLifetime != 7*24*time.Hour {
		t.Fatalf("OrderLifetime = %v", OrderLifetime)
	}
}

func TestAllowedTransitions(t *testing.T) {
	cases := []struct {
		from, to Status
		now      time.Time
	}{
		{StatusPending, StatusPaid, created},
		{StatusPending, StatusPaid, expires}, // the expiry instant itself is not past it
		{StatusPending, StatusExpired, expires.Add(time.Nanosecond)},
		{StatusPending, StatusCanceled, created},
		{StatusPending, StatusCanceled, expires.Add(time.Hour)}, // an overdue pending order may still be canceled
		{StatusPaid, StatusRefunded, expires.Add(24 * time.Hour)},
	}
	for _, c := range cases {
		if err := Transition(c.from, c.to, expires, c.now); err != nil {
			t.Errorf("%s -> %s at %v: %v", c.from, c.to, c.now, err)
		}
	}
}

func TestRefusedTransitions(t *testing.T) {
	all := []Status{StatusPending, StatusPaid, StatusExpired, StatusCanceled, StatusRefunded}
	allowed := map[[2]Status]bool{
		{StatusPending, StatusPaid}:     true,
		{StatusPending, StatusExpired}:  true,
		{StatusPending, StatusCanceled}: true,
		{StatusPaid, StatusRefunded}:    true,
	}
	for _, from := range all {
		for _, to := range all {
			if allowed[[2]Status{from, to}] {
				continue
			}
			err := Transition(from, to, expires, created)
			if !errors.Is(err, ErrInvalidTransition) {
				t.Errorf("%s -> %s: err = %v, want ErrInvalidTransition", from, to, err)
			}
		}
	}
	for _, bad := range []Status{"", "unknown", "PENDING"} {
		if err := Transition(bad, StatusPaid, expires, created); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("from %q: err = %v", bad, err)
		}
		if err := Transition(StatusPending, bad, expires, created); !errors.Is(err, ErrInvalidTransition) {
			t.Errorf("to %q: err = %v", bad, err)
		}
	}
}

func TestExpiryRules(t *testing.T) {
	past := expires.Add(time.Nanosecond)
	if err := Transition(StatusPending, StatusPaid, expires, past); !errors.Is(err, ErrOrderExpired) {
		t.Errorf("paying an expired order: err = %v, want ErrOrderExpired", err)
	}
	if err := Transition(StatusPending, StatusExpired, expires, expires); !errors.Is(err, ErrNotExpired) {
		t.Errorf("expiring at the expiry instant: err = %v, want ErrNotExpired", err)
	}
	if err := Transition(StatusPending, StatusExpired, expires, created); !errors.Is(err, ErrNotExpired) {
		t.Errorf("expiring a fresh order: err = %v, want ErrNotExpired", err)
	}
	if IsExpired(expires, expires) || !IsExpired(expires, past) {
		t.Error("IsExpired must be true only after expires_at")
	}
}
