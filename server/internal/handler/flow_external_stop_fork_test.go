package handler

import (
	"context"
	"strings"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
	"github.com/multica-ai/multica/server/internal/service"
)

func TestExternalFlowTerminalIntentStops(t *testing.T) {
	for _, state := range []string{"rejected", "expired"} {
		t.Run(state, func(t *testing.T) {
			f := newTradeFlowFixtureWith(t, func(body map[string]any) {
				collect := body["steps"].([]map[string]any)[1]["tasks"].([]map[string]any)
				collect[0]["effects"] = "external"
			})
			previous := testHandler.durable.Intents
			testHandler.durable.Intents = &service.IntentService{Queries: testHandler.Queries, TxStarter: testPool, Flags: effectFlagsOn()}
			t.Cleanup(func() { testHandler.durable.Intents = previous })
			request := f.newRequest(t)
			t.Cleanup(func() {
				testPool.Exec(context.Background(), `DELETE FROM effect_intent WHERE issue_id = $1`, request.ID)
			})
			f.plan(t, request, nil)
			if _, err := testPool.Exec(context.Background(), `UPDATE effect_intent SET state = $1 WHERE workspace_id = $2 AND issue_id = $3`, state, testWorkspaceID, request.ID); err != nil {
				t.Fatal(err)
			}
			result := advance(t, request)
			if result.State.Step != "blocked" || stopOf(result.State) != issueworkflow.StopStalled {
				t.Fatalf("state = %+v, want stalled and blocked", result.State)
			}
			if got := tasksAt(t, request, "collect", 0)["docs"]; got != "" {
				t.Fatalf("terminal intent opened task %s", got)
			}
			var comments string
			if err := testPool.QueryRow(context.Background(), `SELECT coalesce(string_agg(content, ' '), '') FROM comment WHERE issue_id = $1`, request.ID).Scan(&comments); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(comments, state) {
				t.Fatalf("stop comment %q does not identify %s", comments, state)
			}
		})
	}
}
