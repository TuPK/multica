package handler

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/server/internal/featureflags"
	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/pkg/featureflag"
)

// TestDurableBriefBodyFlowStageCandidates: durableBriefBody appends flow-stage merge/drop
// candidates under a company brief's Progress section when durable_seats is on and a gate meets
// the spec 10.4 threshold (>= 95% first-round approval on >= 30 passes), and omits the section
// when there are no candidates, when the brief is floor-scoped, or when durable_seats is off.
func TestDurableBriefBodyFlowStageCandidates(t *testing.T) {
	e := newDigestEnv(t)
	ws := e.WorkspaceID
	wsUUID := parseUUID(ws)
	ctx := context.Background()
	now := time.Now()
	start := now.Add(-7 * 24 * time.Hour)
	baseBody := "# Weekly Brief\n\n## Progress\nShipped v1.\n\n## Next\nPlan v2.\n"

	handlerWithSeats := func(on bool) *Handler {
		p := featureflag.NewStaticProvider()
		p.Set(featureflags.DurableSeats, featureflag.Rule{Default: on})
		h := *testHandler
		h.FeatureFlags = featureflag.NewService(p)
		return &h
	}

	hOn := handlerWithSeats(true)

	// No candidates yet: the brief stays unchanged and has no flow-stage section.
	withoutCandidates := hOn.durableBriefBody(ctx, wsUUID, briefScopeCompany, baseBody, start, now)
	if strings.Contains(withoutCandidates, "Ứng viên gộp hoặc bỏ cổng") {
		t.Fatalf("brief without candidates unexpectedly included flow-stage section:\n%s", withoutCandidates)
	}
	if withoutCandidates != baseBody {
		t.Fatalf("brief without candidates changed:\ngot:\n%s\nwant:\n%s", withoutCandidates, baseBody)
	}

	// Seed 30 round-0 verdicts (29 approved, 1 rejected -> 96.7%) for review:gate_check, plus
	// 4.5 minutes of human attention on a proposal linked to one of the gate issues.
	squadID := e.squad(t, "Brief Squad")
	t.Cleanup(func() {
		testPool.Exec(context.Background(), `DELETE FROM attention_ledger WHERE workspace_id = $1`, ws)
		testPool.Exec(context.Background(), `DELETE FROM proposal WHERE workspace_id = $1`, ws)
	})

	var firstIssueID string
	for i := 0; i < 29; i++ {
		id := dbfx.Issue(t, fmt.Sprintf("gate pass %d", i), testutil.Cols{
			"workspace_id": ws, "flow_step": "review", "flow_task": "gate_check", "flow_round": 0, "outcome": "approved",
		})
		if i == 0 {
			firstIssueID = id
		}
	}
	dbfx.Issue(t, "gate resend", testutil.Cols{
		"workspace_id": ws, "flow_step": "review", "flow_task": "gate_check", "flow_round": 0, "outcome": "rejected",
	})
	propID := dbfx.Insert(t, "proposal", testutil.Cols{
		"workspace_id": ws, "squad_id": squadID, "issue_id": firstIssueID,
		"created_by_type": "member", "created_by_id": testUserID,
		"state": "approved", "kind": "general",
	})
	dbfx.Insert(t, "attention_ledger", testutil.Cols{
		"workspace_id": ws, "squad_id": squadID,
		"subject_type": "proposal", "subject_id": propID,
		"reason": "judgment", "estimated_minutes": 4.5, "state": "answered", "week_start": "2026-11-23",
	})

	withCandidates := hOn.durableBriefBody(ctx, wsUUID, briefScopeCompany, baseBody, start, now)
	for _, want := range []string{"Ứng viên gộp hoặc bỏ cổng", "review:gate_check", "29/30", "4.5 phút"} {
		if !strings.Contains(withCandidates, want) {
			t.Fatalf("company brief with candidates missing %q:\n%s", want, withCandidates)
		}
	}
	if strings.Index(withCandidates, "Ứng viên gộp hoặc bỏ cổng") > strings.Index(withCandidates, "## Next") {
		t.Fatalf("flow-stage section was not placed under ## Progress:\n%s", withCandidates)
	}

	// Floor briefs keep the author's body untouched.
	if floorBody := hOn.durableBriefBody(ctx, wsUUID, briefScopeFloor, baseBody, start, now); floorBody != baseBody {
		t.Fatalf("floor brief was modified:\n%s", floorBody)
	}

	// With durable_seats off, no flow-stage section is added.
	hOff := handlerWithSeats(false)
	if offBody := hOff.durableBriefBody(ctx, wsUUID, briefScopeCompany, baseBody, start, now); offBody != baseBody {
		t.Fatalf("brief with durable_seats off was modified:\n%s", offBody)
	}
}
