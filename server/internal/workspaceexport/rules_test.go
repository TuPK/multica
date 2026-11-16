package workspaceexport

import (
	"errors"
	"testing"
	"time"
)

var t0 = time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

func TestAdmitDailyRule(t *testing.T) {
	cases := []struct {
		name   string
		now    time.Time
		latest *Latest
		want   error
	}{
		{"no export yet", t0, nil, nil},
		{"23 hours after a finished export", t0.Add(23 * time.Hour), &Latest{StatusDone, t0}, ErrDailyLimit},
		{"25 hours after a finished export", t0.Add(25 * time.Hour), &Latest{StatusDone, t0}, nil},
		{"23 hours after a failed export", t0.Add(23 * time.Hour), &Latest{StatusFailed, t0}, ErrDailyLimit},
		{"25 hours after an expired export", t0.Add(25 * time.Hour), &Latest{StatusExpired, t0}, nil},
		{"one running export", t0.Add(time.Minute), &Latest{StatusRunning, t0}, ErrInProgress},
		{"one queued export", t0.Add(time.Minute), &Latest{StatusQueued, t0}, ErrInProgress},
	}
	for _, c := range cases {
		if got := Admit(c.now, c.latest); !errors.Is(got, c.want) {
			t.Errorf("%s: Admit = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestExpiryIsSevenDaysAfterFinish(t *testing.T) {
	finished := t0
	if got := ExpiresAt(finished); !got.Equal(finished.Add(7 * 24 * time.Hour)) {
		t.Fatalf("ExpiresAt = %v, want exactly 7 days after %v", got, finished)
	}
}

func TestDownloadLinkIsAbsentAfterExpiry(t *testing.T) {
	finished := t0
	expires := ExpiresAt(finished)
	e := Export{Status: StatusDone, FinishedAt: &finished, ExpiresAt: &expires}

	if d, ok := DownloadWindow(e, finished.Add(24*time.Hour)); !ok || d != 6*24*time.Hour {
		t.Errorf("a day in: window = %v, %v; want the 6 days that remain", d, ok)
	}
	if d, ok := DownloadWindow(e, expires.Add(-time.Second)); !ok || d != time.Second {
		t.Errorf("one second before: window = %v, %v", d, ok)
	}
	if _, ok := DownloadWindow(e, expires); ok {
		t.Error("a link is offered at the expiry instant")
	}
	if _, ok := DownloadWindow(e, expires.Add(time.Hour)); ok {
		t.Error("a link is offered after the expiry")
	}
	if got := EffectiveStatus(e, expires.Add(time.Hour)); got != StatusExpired {
		t.Errorf("EffectiveStatus after expiry = %q, want expired", got)
	}
	if got := EffectiveStatus(e, finished.Add(time.Hour)); got != StatusDone {
		t.Errorf("EffectiveStatus before expiry = %q, want done", got)
	}
}

func TestNoLinkUnlessDone(t *testing.T) {
	expires := t0.Add(time.Hour)
	for _, status := range []string{StatusQueued, StatusRunning, StatusFailed, StatusExpired} {
		if _, ok := DownloadWindow(Export{Status: status, ExpiresAt: &expires}, t0); ok {
			t.Errorf("status %q is offered a link", status)
		}
	}
	if _, ok := DownloadWindow(Export{Status: StatusDone}, t0); ok {
		t.Error("a done export with no expiry is offered a link")
	}
}
