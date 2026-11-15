package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
)

type daemonShutdownResult struct {
	PID              int  `json:"pid"`
	CleanupConfirmed bool `json:"cleanup_confirmed"`
	// Reason says why the daemon left Run. It is empty in the record written at
	// startup, so a daemon that was killed (SIGKILL, systemd-oomd, power loss)
	// leaves a record with no reason: nothing could write one.
	Reason string `json:"reason,omitempty"`
}

// notifyShutdownContextWithCause is signal.NotifyContext that also records
// which signal ended the context, readable through context.Cause.
func notifyShutdownContextWithCause(parent context.Context, signals ...os.Signal) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancelCause(parent)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, signals...)
	go func() {
		select {
		case sig := <-ch:
			cancel(fmt.Errorf("signal: %s", sig))
		case <-ctx.Done():
		}
	}()
	return ctx, func() {
		signal.Stop(ch)
		cancel(context.Canceled)
	}
}

// daemonExitReason names why Run returned, using only what this process knows.
// Run's own context is cancelled by exactly two paths besides the signal
// context: the /shutdown endpoint (daemon stop and restart) and the self-update
// restart, which sets restartBinary.
func daemonExitReason(ctx context.Context, runErr error, restartBinary string) string {
	if runErr != nil && !errors.Is(runErr, context.Canceled) {
		return "error: " + runErr.Error()
	}
	if cause := context.Cause(ctx); cause != nil && !errors.Is(cause, context.Canceled) {
		return cause.Error()
	}
	if restartBinary != "" {
		return "restart into updated binary " + restartBinary
	}
	return "shutdown requested through the daemon health endpoint (multica daemon stop or restart)"
}

// unrecordedExitNotice describes a previous daemon that never wrote a reason.
// It returns "" when the previous record shows a daemon that exited through
// its own shutdown path, or when there is no previous record from another PID.
func unrecordedExitNotice(previous daemonShutdownResult, ok bool, pid int) string {
	if !ok || previous.PID <= 1 || previous.PID == pid || previous.CleanupConfirmed || previous.Reason != "" {
		return ""
	}
	return fmt.Sprintf("previous daemon (pid %d) ended without recording a shutdown reason: it was killed (SIGKILL, for example systemd-oomd under memory pressure, 'daemon stop --force'), crashed, or the host lost power. Check 'journalctl --user -u multica-daemon' and 'journalctl -u systemd-oomd'", previous.PID)
}

func daemonShutdownResultPath(profile string) string {
	return filepath.Join(daemonDirForProfile(profile), "daemon.shutdown.json")
}

// Written only while the daemon holds the profile lock, first as unconfirmed
// at startup, then with the final cleanup outcome before releasing the lock.
func writeDaemonShutdownResult(profile string, result daemonShutdownResult) error {
	data, err := json.Marshal(result)
	if err != nil {
		return err
	}
	return os.WriteFile(daemonShutdownResultPath(profile), data, 0o600)
}

func confirmDaemonCleanup(profile string, pid int) error {
	result, err := readDaemonShutdownResult(profile)
	if err != nil || pid <= 1 || result.PID != pid || !result.CleanupConfirmed {
		// A daemon without a matching result (for example one started by an
		// older CLI) leaves nothing behind after it exits. Persist the failure so
		// a repeated stop cannot treat the now-idle profile as cleanly stopped.
		// The next daemon start overwrites this record with its own PID.
		if err != nil || result.PID != pid || result.CleanupConfirmed {
			_ = writeDaemonShutdownResult(profile, daemonShutdownResult{PID: max(pid, 0)})
		}
		return fmt.Errorf("daemon exited but task cleanup could not be confirmed; background tasks may still be running")
	}
	return nil
}

func readDaemonShutdownResult(profile string) (daemonShutdownResult, error) {
	var result daemonShutdownResult
	data, err := os.ReadFile(daemonShutdownResultPath(profile))
	if err == nil {
		err = json.Unmarshal(data, &result)
	}
	return result, err
}

func daemonStopPID(profile string, health map[string]any) int {
	if value, ok := health["pid"].(float64); ok && value > 1 && float64(int(value)) == value {
		return int(value)
	}
	if data, err := os.ReadFile(daemonPIDPathForProfile(profile)); err == nil {
		if pid, err := strconv.Atoi(strings.TrimSpace(string(data))); err == nil && pid > 1 {
			return pid
		}
	}
	if record, ok := readDaemonPortRecord(profile); ok {
		return record.PID
	}
	return 0
}

func readDaemonShutdownResultOK(profile string) (daemonShutdownResult, bool) {
	result, err := readDaemonShutdownResult(profile)
	return result, err == nil
}
