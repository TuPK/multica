package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
)

// The fake worker is this test binary, run again on TestFakeWorkerHelper. It is the
// worker of worker-protocol.md, step 5: it reads the prompt on stdin, writes its
// progress on stdout, and exits. It sees the task token in its environment, as a
// real worker does, and must not be able to leak it through the adapter.

const fakeTaskToken = "mat_fake_task_token_for_test"

func TestFakeWorkerHelper(t *testing.T) {
	if os.Getenv("OCF_FAKE_WORKER") != "1" {
		return
	}
	prompt, _ := io.ReadAll(os.Stdin)
	first, _, _ := strings.Cut(string(prompt), "\n")
	fmt.Printf("received %d bytes\n", len(prompt))
	fmt.Printf("prompt starts with: %s\n", first)
	if os.Getenv("MULTICA_TOKEN") != "" {
		fmt.Println("task token is in the environment")
	}
	fmt.Println("done")
	if os.Getenv("OCF_FAKE_MODE") == "fail" {
		fmt.Fprintln(os.Stderr, "the fake worker failed on purpose")
		os.Exit(3)
	}
	os.Exit(0)
}

// fakeWorkerCommand is the --worker value that runs TestFakeWorkerHelper.
func fakeWorkerCommand(t *testing.T) string {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatalf("locate test binary: %v", err)
	}
	return exe + " -test.run=^TestFakeWorkerHelper$"
}

// adapterEvents decodes the adapter's stdout, one JSON event per line.
func adapterEvents(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var events []map[string]any
	for _, line := range strings.Split(strings.TrimSpace(stdout), "\n") {
		if line == "" {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			t.Fatalf("adapter printed a line that is not JSON: %q: %v", line, err)
		}
		events = append(events, ev)
	}
	return events
}

func fakeWorkerEnv(mode string) []string {
	env := append(os.Environ(), "OCF_FAKE_WORKER=1", "MULTICA_TOKEN="+fakeTaskToken)
	if mode != "" {
		env = append(env, "OCF_FAKE_MODE="+mode)
	}
	return env
}

func TestAdapterRunsWorkerEndToEnd(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"-p", "--mode", "json", "--model", "any-model", "--worker", fakeWorkerCommand(t)}
	code := run(context.Background(), args, strings.NewReader("Write the weekly report\nwith the figures."),
		&stdout, &stderr, fakeWorkerEnv(""))
	if code != exitOK {
		t.Fatalf("exit = %d, want %d; stderr: %s", code, exitOK, stderr.String())
	}

	events := adapterEvents(t, stdout.String())
	if len(events) < 3 {
		t.Fatalf("events = %v, want agent_start, message updates and agent_end", events)
	}
	if events[0]["type"] != "agent_start" {
		t.Fatalf("first event = %v, want agent_start", events[0]["type"])
	}

	var streamed strings.Builder
	for _, ev := range events[1 : len(events)-1] {
		if ev["type"] != "message_update" {
			t.Fatalf("middle event = %v, want message_update", ev)
		}
		update := ev["assistantMessageEvent"].(map[string]any)
		if update["type"] != "text_delta" {
			t.Fatalf("update = %v, want text_delta", update)
		}
		streamed.WriteString(update["delta"].(string))
	}
	for _, want := range []string{"received ", "prompt starts with: Write the weekly report", "task token is in the environment", "done"} {
		if !strings.Contains(streamed.String(), want) {
			t.Fatalf("streamed text lacks %q:\n%s", want, streamed.String())
		}
	}

	last := events[len(events)-1]
	if last["type"] != "agent_end" {
		t.Fatalf("last event = %v, want agent_end", last["type"])
	}
	message := last["messages"].([]any)[0].(map[string]any)
	if message["stopReason"] != "stop" {
		t.Fatalf("stopReason = %v, want stop", message["stopReason"])
	}
	final := message["content"].([]any)[0].(map[string]any)["text"]
	if final != streamed.String() {
		t.Fatalf("final text differs from the streamed text:\nfinal:    %q\nstreamed: %q", final, streamed.String())
	}

	if strings.Contains(stdout.String(), fakeTaskToken) || strings.Contains(stderr.String(), fakeTaskToken) {
		t.Fatal("the task token appeared in the adapter's output")
	}
}

func TestAdapterReportsWorkerFailure(t *testing.T) {
	var stdout, stderr bytes.Buffer
	args := []string{"-p", "--mode", "json", "--worker", fakeWorkerCommand(t)}
	code := run(context.Background(), args, strings.NewReader("Fail on purpose"), &stdout, &stderr, fakeWorkerEnv("fail"))
	if code != exitWorker {
		t.Fatalf("exit = %d, want %d", code, exitWorker)
	}

	events := adapterEvents(t, stdout.String())
	var sawError bool
	for _, ev := range events {
		if ev["type"] == "error" && ev["message"] == "the worker exited with code 3" {
			sawError = true
		}
	}
	if !sawError {
		t.Fatalf("no error event for the failed worker: %v", events)
	}
	last := events[len(events)-1]
	message := last["messages"].([]any)[0].(map[string]any)
	if last["type"] != "agent_end" || message["stopReason"] != "error" {
		t.Fatalf("last event = %v, want agent_end with stopReason error", last)
	}
}

func TestAdapterRefusesEmptyPrompt(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"--worker", fakeWorkerCommand(t)}, strings.NewReader("  \n"),
		&stdout, &stderr, fakeWorkerEnv(""))
	if code != exitBadInput {
		t.Fatalf("exit = %d, want %d", code, exitBadInput)
	}
	if stdout.Len() != 0 {
		t.Fatalf("an empty prompt wrote events: %s", stdout.String())
	}
}

func TestAdapterNeedsAWorker(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := run(context.Background(), []string{"-p", "--mode", "json"}, strings.NewReader("hi"),
		&stdout, &stderr, []string{"PATH=/usr/bin"})
	if code != exitBadInput || !strings.Contains(stderr.String(), workerEnv) {
		t.Fatalf("exit = %d, stderr = %q; want %d naming %s", code, stderr.String(), exitBadInput, workerEnv)
	}
}
