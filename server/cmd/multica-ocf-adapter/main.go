// multica-ocf-adapter is the reference adapter of the worker protocol
// (docs/specs/durable-platform/ocf/worker-protocol.md, ocf-worker/1). The daemon runs
// it as a custom runtime with protocol_family "pi": it reads the prompt from stdin,
// starts the worker named by --worker (or MULTICA_OCF_WORKER), and writes the
// worker's output to stdout as pi-style JSON events. The daemon turns those events
// into messages and the final complete or fail call.
//
// The adapter does not call the Delegora API. Commands the worker runs with `multica`
// use the task token the daemon put in the environment; the adapter does not read,
// print or log it.
package main

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mattn/go-shellwords"
)

// workerEnv names the worker command when --worker is not given.
const workerEnv = "MULTICA_OCF_WORKER"

// Exit codes. A nonzero exit makes the daemon report the task failed; the error
// event printed first says why.
const (
	exitOK       = 0
	exitWorker   = 1
	exitBadInput = 2
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, os.Environ()))
}

// run is main without the process. args are the daemon's arguments: a pi-family
// command line whose flags this adapter does not use (--model, --session, -p,
// --mode) are accepted and ignored; --worker names the worker command.
func run(ctx context.Context, args []string, stdin io.Reader, stdout, stderr io.Writer, env []string) int {
	worker, err := workerCommand(args, env)
	if err != nil {
		fmt.Fprintln(stderr, "multica-ocf-adapter:", err)
		return exitBadInput
	}
	prompt, err := io.ReadAll(stdin)
	if err != nil {
		fmt.Fprintln(stderr, "multica-ocf-adapter: read prompt:", err)
		return exitBadInput
	}
	if strings.TrimSpace(string(prompt)) == "" {
		fmt.Fprintln(stderr, "multica-ocf-adapter: the prompt on stdin is empty")
		return exitBadInput
	}

	out := newEventWriter(stdout)
	out.emit(map[string]any{"type": "agent_start"})
	text, err := runWorker(ctx, worker, prompt, env, stderr, out)
	if err != nil {
		out.emit(map[string]any{"type": "error", "message": err.Error()})
		out.end(text, err.Error())
		fmt.Fprintln(stderr, "multica-ocf-adapter:", err)
		return exitWorker
	}
	out.end(text, "")
	return exitOK
}

// workerCommand returns the worker's argv: --worker, then MULTICA_OCF_WORKER.
func workerCommand(args []string, env []string) ([]string, error) {
	raw := ""
	for i := 0; i < len(args); i++ {
		if args[i] == "--worker" {
			if i+1 >= len(args) {
				return nil, errors.New("--worker needs a command")
			}
			raw = args[i+1]
			i++
		}
	}
	if raw == "" {
		raw = envValue(env, workerEnv)
	}
	argv, err := shellwords.Parse(raw)
	if err != nil {
		return nil, fmt.Errorf("worker command: %w", err)
	}
	if len(argv) == 0 {
		return nil, errors.New("no worker: pass --worker or set " + workerEnv)
	}
	return argv, nil
}

// envValue returns the last value of key in env, the one a later entry sets.
func envValue(env []string, key string) string {
	value := ""
	for _, kv := range env {
		if v, ok := strings.CutPrefix(kv, key+"="); ok {
			value = v
		}
	}
	return value
}

// runWorker starts the worker with the prompt on its stdin and streams its stdout
// as text deltas. It returns the full text the worker wrote.
func runWorker(ctx context.Context, argv []string, prompt []byte, env []string, stderr io.Writer, out *eventWriter) (string, error) {
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Env = env
	cmd.Stdin = strings.NewReader(string(prompt))
	cmd.Stderr = stderr
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("worker stdout: %w", err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("start worker: %w", err)
	}

	var text strings.Builder
	scanner := bufio.NewScanner(pipe)
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := scanner.Text() + "\n"
		text.WriteString(line)
		out.emit(map[string]any{
			"type":                  "message_update",
			"assistantMessageEvent": map[string]any{"type": "text_delta", "delta": line},
		})
	}
	scanErr := scanner.Err()
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return text.String(), errors.New("the task was cancelled")
	}
	if scanErr != nil {
		return text.String(), fmt.Errorf("read worker output: %w", scanErr)
	}
	if waitErr != nil {
		var exitErr *exec.ExitError
		if errors.As(waitErr, &exitErr) {
			return text.String(), fmt.Errorf("the worker exited with code %d", exitErr.ExitCode())
		}
		return text.String(), fmt.Errorf("wait for worker: %w", waitErr)
	}
	return text.String(), nil
}

// eventWriter writes one JSON event per line and flushes it at once, so the daemon
// sees progress while the worker runs.
type eventWriter struct {
	w io.Writer
}

func newEventWriter(w io.Writer) *eventWriter { return &eventWriter{w: w} }

func (e *eventWriter) emit(event map[string]any) {
	raw, err := json.Marshal(event)
	if err != nil {
		return
	}
	_, _ = e.w.Write(append(raw, '\n'))
}

// end closes the run. Its last assistant message holds the final text, or the
// error that stopped the run.
func (e *eventWriter) end(text, errText string) {
	message := map[string]any{"role": "assistant", "content": []map[string]any{{"type": "text", "text": text}}}
	if errText != "" {
		message["stopReason"] = "error"
		message["errorMessage"] = errText
	} else {
		message["stopReason"] = "stop"
	}
	e.emit(map[string]any{"type": "agent_end", "messages": []any{message}})
}
