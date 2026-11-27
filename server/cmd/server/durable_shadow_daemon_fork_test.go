package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/multica-ai/multica/server/internal/analytics"
	"github.com/multica-ai/multica/server/internal/cli"
	"github.com/multica-ai/multica/server/internal/daemon"
	"github.com/multica-ai/multica/server/internal/daemon/execenv"
	"github.com/multica-ai/multica/server/internal/events"
	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/realtime"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
	"github.com/multica-ai/multica/server/pkg/protocol"
)

// The daemon prepares each task's environment in a helper process that re-executes its own binary
// with execenv.PreparationHelperArg (the multica CLI answers it in main). In this test binary that
// argument would run the whole suite, so the helper is answered here, before TestMain runs.
func init() {
	if len(os.Args) == 2 && os.Args[1] == execenv.PreparationHelperArg {
		if err := execenv.RunPreparationHelper(os.Stdin, os.Stdout, slog.New(slog.NewTextHandler(os.Stderr, nil))); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
}

// syncBuffer collects the daemon's log lines. The daemon logs from several goroutines.
type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// waitForValue polls until query returns a row, or fails with the daemon's log after timeout.
func waitForValue(t *testing.T, timeout time.Duration, logs *syncBuffer, what string, query func() (bool, error)) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for {
		ok, err := query()
		if err != nil {
			t.Fatalf("%s: %v", what, err)
		}
		if ok {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("%s: not reached in %s; daemon log:\n%s", what, timeout, logs.String())
		}
		time.Sleep(200 * time.Millisecond)
	}
}

// isolatedShadowWorkspace makes a user, a workspace and an owner membership of this test's own,
// and returns the user's login token. The package's shared integration fixture is not used: every
// cmd/server test process deletes the integration-tests workspace and user when it starts, so a
// run that depends on them fails whenever two runs share the database.
func isolatedShadowWorkspace(t *testing.T, ctx context.Context, suffix string) (userID, wsID, token string) {
	t.Helper()
	email := "shadow-e2e-" + suffix + "@example.test"
	if err := testPool.QueryRow(ctx, `
		INSERT INTO "user" (name, email)
		VALUES ($1, $2)
		RETURNING id::text`, "Shadow E2E "+suffix, email).Scan(&userID); err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := testPool.QueryRow(ctx, `
		INSERT INTO workspace (name, slug, description)
		VALUES ($1, $2, $3)
		RETURNING id::text`, "Shadow E2E "+suffix, "shadow-e2e-"+suffix, "durable shadow daemon test").Scan(&wsID); err != nil {
		t.Fatalf("create workspace: %v", err)
	}
	if _, err := testPool.Exec(ctx, `INSERT INTO member (workspace_id, user_id, role) VALUES ($1, $2, 'owner')`, wsID, userID); err != nil {
		t.Fatalf("create member: %v", err)
	}
	// Registered before the fixtures and the daemon, so it runs after both have been torn down.
	t.Cleanup(func() {
		c := context.Background()
		_, _ = testPool.Exec(c, `DELETE FROM agent_runtime WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(c, `DELETE FROM member WHERE workspace_id = $1`, wsID)
		_, _ = testPool.Exec(c, `DELETE FROM workspace WHERE id = $1`, wsID)
		_, _ = testPool.Exec(c, `DELETE FROM "user" WHERE id = $1`, userID)
	})
	var err error
	token, err = generateTestJWT(userID, email, "Shadow E2E "+suffix)
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	return userID, wsID, token
}

// TestShadowRunThroughTheRealDaemonAndRouter runs the whole shadow path with the real router over
// HTTP and the real daemon loop (daemon.Run) as the client: the daemon registers its runtime, the
// shadow run starts through the route, a queued primary is claimed by the daemon, the shadow task
// is claimed by the same daemon with the shadow-v1 capability, the shadow task runs in a temporary
// directory, both sides complete over HTTP, the juror's verification task is claimed and completes
// with a verdict, and the verdict is recorded on the pair.
//
// The daemon authenticates with the test's own owner token (the same token the CLI keeps), and
// the fake agent CLI answers every task on stdout, so the verdict it gives is not a judgment of
// the answers.
func TestShadowRunThroughTheRealDaemonAndRouter(t *testing.T) {
	if testPool == nil {
		t.Skip("integration database not available")
	}
	ctx := context.Background()
	suffix := uuid.NewString()[:8]
	userID, wsID, token := isolatedShadowWorkspace(t, ctx, suffix)

	flagProvider := featureflag.NewStaticProvider()
	flagProvider.Set(featureflags.DurableSeats, featureflag.Rule{Default: true})
	router, _ := NewRouterWithOptions(testPool, realtime.NewHub(), events.New(), analytics.NoopClient{}, nil,
		RouterOptions{FeatureFlags: featureflag.NewService(flagProvider)})

	// The claim requests the daemon sends go through this recorder, then to the real router.
	var (
		claimMu   sync.Mutex
		claimCaps []string
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/daemon/tasks/claim" {
			claimMu.Lock()
			claimCaps = append(claimCaps, r.Header.Get("X-Client-Capabilities"))
			claimMu.Unlock()
		}
		router.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)

	// The daemon reads its login from the CLI config under HOME.
	t.Setenv("HOME", t.TempDir())
	cfgPath, err := cli.CLIConfigPathForProfile("")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(cfgPath), 0o700); err != nil {
		t.Fatal(err)
	}
	cfgData, err := json.Marshal(cli.CLIConfig{ServerURL: srv.URL, WorkspaceID: wsID, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(cfgPath, cfgData, 0o600); err != nil {
		t.Fatal(err)
	}

	// One fake agent CLI named droid, on PATH. Its version probe succeeds, and every run appends
	// its working directory to attempts and answers with a completion that ends in a verdict.
	binDir := t.TempDir()
	attempts := filepath.Join(t.TempDir(), "attempts")
	fakeDroid := filepath.Join(binDir, "droid")
	if err := os.WriteFile(fakeDroid, []byte(fmt.Sprintf(`#!/bin/sh
case "$*" in *--version*) echo 1.8.0; exit 0;; esac
cat >/dev/null
pwd >> '%s'
printf '%%s\n' '{"type":"system","subtype":"init","session_id":"e2e-session"}' '{"type":"completion","finalText":"done\nVERDICT: A"}'
`, attempts)), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULTICA_DROID_PATH", fakeDroid)
	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))

	workspacesRoot := filepath.Join(t.TempDir(), "workspaces")
	cfg, err := daemon.LoadConfig(daemon.Overrides{
		ServerURL:      srv.URL,
		WorkspacesRoot: workspacesRoot,
		PollInterval:   200 * time.Millisecond,
		// The primary is inserted straight into the database, so no wakeup reaches the daemon over
		// its WebSocket; the safety poll is what claims it, and the default is three minutes.
		WSClaimPollInterval: 500 * time.Millisecond,
		HeartbeatInterval:   time.Second,
		DaemonID:            "shadow-e2e-" + suffix,
		DeviceName:          "shadow e2e",
	})
	if err != nil {
		t.Fatalf("load daemon config: %v", err)
	}
	var logs syncBuffer
	logger := slog.New(slog.NewTextHandler(&logs, &slog.HandlerOptions{Level: slog.LevelDebug}))
	d := daemon.New(cfg, logger)
	healthLn, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	runCtx, stopDaemon := context.WithCancel(ctx)
	runDone := make(chan error, 1)
	go func() { runDone <- d.RunWithHealthListener(runCtx, healthLn) }()
	t.Cleanup(func() {
		stopDaemon()
		select {
		case <-runDone:
		case <-time.After(30 * time.Second):
			t.Errorf("the daemon did not stop; log:\n%s", logs.String())
		}
	})

	// The daemon registers its droid runtime for the test workspace.
	var runtimeID string
	waitForValue(t, 60*time.Second, &logs, "the daemon registers its droid runtime", func() (bool, error) {
		err := testPool.QueryRow(ctx, `
			SELECT id::text FROM agent_runtime
			WHERE daemon_id = $1 AND provider = 'droid' AND workspace_id = $2`,
			cfg.DaemonID, wsID).Scan(&runtimeID)
		if err != nil {
			return false, nil
		}
		return true, nil
	})

	fx := testutil.New(testPool, wsID, userID)
	seatAgent := fx.Agent(t, "shadow daemon seat "+suffix, runtimeID)
	juror := fx.Agent(t, "shadow daemon juror "+suffix, runtimeID)
	setJuryPool(t, ctx, wsID, juror)

	// The run starts through the real route, as the owner.
	startBody, err := json.Marshal(map[string]any{
		"candidate_runtime_id": runtimeID, "candidate_model": "candidate-" + suffix,
		"sample_rate": 1, "target_pairs": 5, "confirm_cost": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	status, body := callRouter(t, srv.URL, token, wsID, http.MethodPost, "/api/agents/"+seatAgent+"/shadow", startBody)
	if status != http.StatusCreated {
		t.Fatalf("start the shadow run: status %d: %s", status, body)
	}
	var configID string
	if err := testPool.QueryRow(ctx, `SELECT id::text FROM shadow_config WHERE agent_id = $1 AND state = 'running'`, seatAgent).Scan(&configID); err != nil {
		t.Fatalf("load the running shadow run: %v", err)
	}

	// The primary is queued after the run starts, so the daemon's claim samples it.
	var issueID string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO issue (workspace_id, title, status, priority, creator_id, creator_type, number, position)
		VALUES (
			$1, $2, 'in_progress', 'none', $3, 'member',
			(SELECT COALESCE(MAX(number), 82649) + 1 FROM issue WHERE workspace_id = $1),
			0
		)
		RETURNING id::text`, wsID, "shadow daemon "+suffix, userID).Scan(&issueID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { testPool.Exec(context.Background(), `DELETE FROM issue WHERE id = $1`, issueID) })
	var primary string
	if err := testPool.QueryRow(ctx, `
		INSERT INTO agent_task_queue (agent_id, runtime_id, issue_id, status, priority)
		VALUES ($1, $2, $3, 'queued', 0)
		RETURNING id::text`, seatAgent, runtimeID, issueID).Scan(&primary); err != nil {
		t.Fatal(err)
	}

	// The verdict is recorded on the pair once the juror completes.
	waitForValue(t, 180*time.Second, &logs, "the pair's verdict is recorded", func() (bool, error) {
		var comparison string
		err := testPool.QueryRow(ctx, `
			SELECT comparison FROM shadow_pair
			WHERE config_id = $1 AND primary_task_id = $2 AND comparison IS NOT NULL`,
			configID, primary).Scan(&comparison)
		return err == nil, nil
	})

	var shadowTask string
	if err := testPool.QueryRow(ctx, `SELECT shadow_task_id::text FROM shadow_pair WHERE config_id = $1 AND primary_task_id = $2`,
		configID, primary).Scan(&shadowTask); err != nil {
		t.Fatalf("load the pair's shadow task: %v", err)
	}
	for _, task := range []string{primary, shadowTask} {
		var st string
		if err := testPool.QueryRow(ctx, `SELECT status FROM agent_task_queue WHERE id = $1`, task).Scan(&st); err != nil {
			t.Fatal(err)
		}
		if st != "completed" {
			t.Fatalf("task %s is %q after its verdict, want completed; daemon log:\n%s", task, st, logs.String())
		}
	}

	claimMu.Lock()
	caps := append([]string(nil), claimCaps...)
	claimMu.Unlock()
	sawShadowV1 := false
	for _, c := range caps {
		if strings.Contains(c, protocol.DaemonCapabilityShadowV1) {
			sawShadowV1 = true
		}
	}
	if !sawShadowV1 {
		t.Fatalf("no claim sent %s; claim capabilities = %q", protocol.DaemonCapabilityShadowV1, caps)
	}

	rawAttempts, err := os.ReadFile(attempts)
	if err != nil {
		t.Fatalf("the fake agent did not run: %v", err)
	}
	var shadowRuns int
	for _, dir := range strings.Split(strings.TrimSpace(string(rawAttempts)), "\n") {
		if strings.Contains(dir, "shadow-run-") {
			shadowRuns++
			if strings.HasPrefix(dir, workspacesRoot) {
				t.Fatalf("the shadow run ran inside the workspace root: %s", dir)
			}
		}
	}
	if shadowRuns == 0 {
		t.Fatalf("no run happened in a shadow run directory; attempts:\n%s", rawAttempts)
	}
}

// setJuryPool makes juror the workspace's only jury pool agent, and restores the workspace settings
// when the test ends.
func setJuryPool(t *testing.T, ctx context.Context, wsID, juror string) {
	t.Helper()
	var orig string
	if err := testPool.QueryRow(ctx, `SELECT COALESCE(settings::text, '{}') FROM workspace WHERE id = $1`, wsID).Scan(&orig); err != nil {
		t.Fatal(err)
	}
	pool, err := json.Marshal([]string{juror})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := testPool.Exec(ctx, `
		UPDATE workspace SET settings = jsonb_set(COALESCE(settings, '{}'::jsonb), '{jury_pool}', $2::jsonb)
		WHERE id = $1`, wsID, string(pool)); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = testPool.Exec(context.Background(), `UPDATE workspace SET settings = $2::jsonb WHERE id = $1`, wsID, orig)
	})
}

// callRouter sends one owner request through the real router and returns its status and body.
func callRouter(t *testing.T, baseURL, token, wsID, method, path string, body []byte) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, baseURL+path, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Workspace-ID", wsID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	out, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(out)
}
