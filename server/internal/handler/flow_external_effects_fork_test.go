package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/service"
	"github.com/multica-ai/multica/server/internal/util"
)

// TestExternalFlowTaskWaitsForApproval: a flow task marked external (durable spec 5.4) does
// not open when its step opens. The engine records one pending declared intent for it, and
// the task opens on the first advance after a person approves that intent. The request starts
// and its other tasks open as usual.
func TestExternalFlowTaskWaitsForApproval(t *testing.T) {
	f := newTradeFlowFixtureWith(t, func(body map[string]any) {
		collect := body["steps"].([]map[string]any)[1]["tasks"].([]map[string]any)
		collect[0]["effects"] = "external"
	})
	intents := &service.IntentService{Queries: testHandler.Queries, TxStarter: testPool, Flags: effectFlagsOn()}
	previous := testHandler.durable.Intents
	testHandler.durable.Intents = intents
	t.Cleanup(func() { testHandler.durable.Intents = previous })

	request := f.newRequest(t)
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM effect_intent WHERE issue_id = $1`, request.ID)
	})
	state := f.plan(t, request, nil)
	if state.Step != "collect" {
		t.Fatalf("after the plan the request is at %q, want collect", state.Step)
	}

	if got := tasksAt(t, request, "collect", 0); got["docs"] != "" {
		t.Fatalf("the held docs task opened before approval: %v", got)
	}
	const target = "flow/collect.docs#0"
	pending := func() int {
		return dbfx.Count(t, `SELECT count(*) FROM effect_intent WHERE issue_id = $1 AND target = $2 AND state = 'pending'`, request.ID, target)
	}
	if n := pending(); n != 1 {
		t.Fatalf("pending intents for %s = %d, want 1", target, n)
	}
	advance(t, request)
	if n := pending(); n != 1 {
		t.Fatalf("a second advance made %d pending intents, want 1", n)
	}
	if got := tasksAt(t, request, "collect", 0); got["docs"] != "" {
		t.Fatalf("the docs task opened while its intent is pending: %v", got)
	}

	var intentID string
	if err := testPool.QueryRow(context.Background(), `SELECT id::text FROM effect_intent WHERE issue_id = $1 AND target = $2`, request.ID, target).Scan(&intentID); err != nil {
		t.Fatalf("load intent: %v", err)
	}
	if _, err := intents.Approve(context.Background(), util.MustParseUUID(testWorkspaceID), util.MustParseUUID(testUserID), util.MustParseUUID(intentID), nil, "go"); err != nil {
		t.Fatalf("Approve: %v", err)
	}
	advance(t, request)
	if got := tasksAt(t, request, "collect", 0); got["docs"] == "" {
		t.Fatalf("after approval the docs task did not open: %v", got)
	}
}
