package backfill

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

// fakeTenants is the set of workspaces with no tenant link, and the writes made
// to it.
type fakeTenants struct {
	orphans   []string
	batchSize []int
	committed []string // workspaces whose batch committed
	failBatch int      // 1-based batch to fail, 0 for none
	runs      int
}

func (f *fakeTenants) InTx(_ context.Context, fn func(TenantTx) error) error {
	f.runs++
	tx := &fakeTenantTx{f: f, pending: map[string]bool{}}
	if err := fn(tx); err != nil {
		return err // a failed batch changes nothing
	}
	if f.failBatch == f.runs {
		return errors.New("commit failed")
	}
	f.committed = append(f.committed, tx.order...)
	remaining := f.orphans[:0:0]
	for _, id := range f.orphans {
		if !tx.pending[id] {
			remaining = append(remaining, id)
		}
	}
	f.orphans = remaining
	return nil
}

type fakeTenantTx struct {
	f       *fakeTenants
	pending map[string]bool
	order   []string
}

func (t *fakeTenantTx) WithoutTenant(_ context.Context, limit int) ([]Candidate, error) {
	t.f.batchSize = append(t.f.batchSize, limit)
	var out []Candidate
	for _, id := range t.f.orphans {
		if len(out) == limit {
			break
		}
		out = append(out, Candidate{ID: id, Name: "w " + id})
	}
	return out, nil
}

func (t *fakeTenantTx) InsertTenant(_ context.Context, id string) (bool, error) {
	t.pending[id] = true
	t.order = append(t.order, id)
	return true, nil
}

func (t *fakeTenantTx) LinkWorkspace(context.Context, string) (bool, error) { return true, nil }

func tenantLog() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestRunTenantsWalksTheOrphansInBatchesUntilNoneRemain(t *testing.T) {
	f := &fakeTenants{orphans: []string{"w1", "w2", "w3", "w4", "w5"}}
	res, err := RunTenants(context.Background(), f, 2, tenantLog())
	if err != nil {
		t.Fatal(err)
	}
	if res.Batches != 3 || res.Tenants != 5 || res.Linked != 5 {
		t.Errorf("result = %+v, want 3 batches, 5 tenants, 5 links", res)
	}
	if strings.Join(f.committed, ",") != "w1,w2,w3,w4,w5" {
		t.Errorf("committed = %v, want every orphan in id order", f.committed)
	}
	for _, n := range f.batchSize {
		if n != 2 {
			t.Errorf("a batch asked for %d workspaces, want the batch size 2", n)
		}
	}
	// A second run finds nothing and writes nothing.
	again, err := RunTenants(context.Background(), f, 2, tenantLog())
	if err != nil || again.Batches != 0 || again.Tenants != 0 {
		t.Errorf("second run = %+v, %v, want nothing to do", again, err)
	}
}

func TestRunTenantsRejectsABatchSizeBelowOne(t *testing.T) {
	if _, err := RunTenants(context.Background(), &fakeTenants{}, 0, tenantLog()); err == nil {
		t.Fatal("a batch size of 0 was accepted")
	}
}

// A batch that fails does not undo the batches before it, and a later run
// finishes the rest.
func TestRunTenantsBatchFailureKeepsEarlierBatches(t *testing.T) {
	f := &fakeTenants{orphans: []string{"w1", "w2", "w3"}, failBatch: 2}
	res, err := RunTenants(context.Background(), f, 1, tenantLog())
	if err == nil || !strings.Contains(err.Error(), "batch 2") {
		t.Fatalf("err = %v, want an error naming batch 2", err)
	}
	if res.Batches != 1 || strings.Join(f.committed, ",") != "w1" {
		t.Errorf("result %+v committed %v, want only batch 1 kept", res, f.committed)
	}
	f.failBatch = 0
	if _, err := RunTenants(context.Background(), f, 1, tenantLog()); err != nil {
		t.Fatal(err)
	}
	if strings.Join(f.committed, ",") != "w1,w2,w3" {
		t.Errorf("committed after the rerun = %v", f.committed)
	}
}
