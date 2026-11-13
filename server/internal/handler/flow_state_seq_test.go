package handler

import (
	"context"
	"testing"

	"github.com/multica-ai/multica/server/internal/issueworkflow"
)

// Every write of issue.flow names the seq it read and stores seq+1, so two
// decisions made from the same state cannot both win even when they leave the
// step and the rework count as they were (spec 4.1, plan P10).

func flowSeqOf(t *testing.T, requestID string) (seq int, present bool) {
	t.Helper()
	var raw *int
	dbfx.QueryRow(t, `SELECT (flow->>'seq')::int FROM issue WHERE id = $1`, requestID).Scan(&raw)
	if raw == nil {
		return 0, false
	}
	return *raw, true
}

func TestSetIssueFlow_ConcurrentWritersLoseOnSeq(t *testing.T) {
	ctx := context.Background()
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	id := uuidToString(request.ID)
	first, err := testHandler.loadFlowRun(ctx, testHandler.Queries, loadIssueRow(t, id))
	if err != nil {
		t.Fatal(err)
	}
	second, err := testHandler.loadFlowRun(ctx, testHandler.Queries, loadIssueRow(t, id))
	if err != nil {
		t.Fatal(err)
	}

	// Both writers keep the step and the rework count; only the seq tells them apart.
	nextFirst := first.state.Clone()
	nextFirst.PlanDone = true
	if saved, err := testHandler.saveFlowState(ctx, testHandler.Queries, first, nextFirst); err != nil || !saved {
		t.Fatalf("the first writer: saved %v, err %v; want saved", saved, err)
	}
	nextSecond := second.state.Clone()
	nextSecond.Vars = map[string]any{"customs_lookup": true}
	if saved, err := testHandler.saveFlowState(ctx, testHandler.Queries, second, nextSecond); err != nil || saved {
		t.Fatalf("the second writer, from the same state: saved %v, err %v; want a lost write", saved, err)
	}
	stored := loadIssueRow(t, id)
	state, _ := flowRequestState(stored)
	if !state.PlanDone || state.Switches["customs_lookup"] == true {
		t.Fatalf("the losing write changed the stored state: %s", stored.Flow)
	}

	// The winner holds the state it wrote and can write again; the loser re-reads first.
	if first.state.Seq != 1 {
		t.Fatalf("the writer's state has seq %d after its write, want 1", first.state.Seq)
	}
	next := first.state.Clone()
	next.EnteredAt = "2026-11-13T00:00:00Z"
	if saved, err := testHandler.saveFlowState(ctx, testHandler.Queries, first, next); err != nil || !saved {
		t.Fatalf("a second write from the winner's own state: saved %v, err %v", saved, err)
	}
	if seq, _ := flowSeqOf(t, id); seq != 2 {
		t.Fatalf("stored seq = %d after two writes, want 2", seq)
	}
}

func TestSetIssueFlow_StateWithoutSeqCountsAsZero(t *testing.T) {
	ctx := context.Background()
	f := newTradeFlowFixture(t)
	request := f.newRequest(t)
	id := uuidToString(request.ID)
	// A request started before seq existed has no such key.
	dbfx.Exec(t, `UPDATE issue SET flow = flow - 'seq' WHERE id = $1`, request.ID)
	if _, present := flowSeqOf(t, id); present {
		t.Fatal("the fixture state still has a seq")
	}
	run, err := testHandler.loadFlowRun(ctx, testHandler.Queries, loadIssueRow(t, id))
	if err != nil {
		t.Fatal(err)
	}
	if run.state.Seq != 0 {
		t.Fatalf("a state without seq reads as %d, want 0", run.state.Seq)
	}
	stale := *run

	next := run.state.Clone()
	next.PlanDone = true
	if saved, err := testHandler.saveFlowState(ctx, testHandler.Queries, run, next); err != nil || !saved {
		t.Fatalf("the first write of a state without seq: saved %v, err %v", saved, err)
	}
	if seq, present := flowSeqOf(t, id); !present || seq != 1 {
		t.Fatalf("stored seq = %d (present %v), want 1", seq, present)
	}
	lost := stale.state.Clone()
	lost.Stop = &issueworkflow.StopRecord{Code: issueworkflow.DocStopStalled}
	if saved, err := testHandler.saveFlowState(ctx, testHandler.Queries, &stale, lost); err != nil || saved {
		t.Fatalf("a writer that read the state without seq, after the first write: saved %v, err %v; want a lost write", saved, err)
	}
}
