//go:build !windows

package main

import (
	"context"
	"errors"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestDaemonExitReason(t *testing.T) {
	signalCtx, cancel := context.WithCancelCause(context.Background())
	cancel(errors.New("signal: terminated"))
	live := context.Background()

	cases := []struct {
		name    string
		ctx     context.Context
		runErr  error
		restart string
		want    string
	}{
		{"signal", signalCtx, nil, "", "signal: terminated"},
		{"run error", live, errors.New("boom"), "", "error: boom"},
		{"restart", live, nil, "/usr/local/bin/multica", "restart into updated binary /usr/local/bin/multica"},
		{"health endpoint", live, nil, "", "shutdown requested through the daemon health endpoint"},
		{"cancelled run is not an error", live, context.Canceled, "", "shutdown requested through the daemon health endpoint"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := daemonExitReason(tc.ctx, tc.runErr, tc.restart); !strings.HasPrefix(got, tc.want) {
				t.Fatalf("reason = %q, want prefix %q", got, tc.want)
			}
		})
	}
}

func TestNotifyShutdownContextRecordsSignal(t *testing.T) {
	ctx, stop := notifyShutdownContextWithCause(context.Background(), syscall.SIGUSR1)
	defer stop()
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGUSR1); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("context was not cancelled by the signal")
	}
	if got := daemonExitReason(ctx, nil, ""); got != "signal: user defined signal 1" {
		t.Fatalf("reason = %q", got)
	}
}

func TestUnrecordedExitNotice(t *testing.T) {
	killed := daemonShutdownResult{PID: 4242}
	if got := unrecordedExitNotice(killed, true, 99); !strings.Contains(got, "pid 4242") {
		t.Fatalf("a record with no reason must be reported, got %q", got)
	}
	for name, tc := range map[string]struct {
		prev daemonShutdownResult
		ok   bool
		pid  int
	}{
		"no record":                {killed, false, 99},
		"own record":               {killed, true, 4242},
		"clean exit":               {daemonShutdownResult{PID: 4242, CleanupConfirmed: true, Reason: "signal: terminated"}, true, 99},
		"unconfirmed yet reasoned": {daemonShutdownResult{PID: 4242, Reason: "signal: terminated"}, true, 99},
	} {
		if got := unrecordedExitNotice(tc.prev, tc.ok, tc.pid); got != "" {
			t.Errorf("%s: unexpected notice %q", name, got)
		}
	}
}
