package plan

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"

	"github.com/multica-ai/multica/platform-management/audit"
)

// fakeStore is the plan table in memory, with transaction semantics: a failing fn
// leaves nothing behind.
type fakeStore struct {
	rows      map[string]Plan
	order     []string
	subs      map[string]int64 // plan key -> subscription rows
	audits    []audit.Entry
	failAfter int // inserts allowed before Insert fails; -1 for never
	inserts   int
	txs       int
}

func newFakeStore() *fakeStore {
	return &fakeStore{rows: map[string]Plan{}, subs: map[string]int64{}, failAfter: -1}
}

func (s *fakeStore) InTx(_ context.Context, fn func(Tx) error) error {
	s.txs++
	rows, order, audits := map[string]Plan{}, append([]string(nil), s.order...), append([]audit.Entry(nil), s.audits...)
	for k, v := range s.rows {
		rows[k] = v
	}
	if err := fn(&fakeTx{s: s}); err != nil {
		s.rows, s.order, s.audits = rows, order, audits
		return err
	}
	return nil
}

type fakeTx struct{ s *fakeStore }

func (t *fakeTx) Count(context.Context) (int64, error) { return int64(len(t.s.rows)), nil }

func (t *fakeTx) InsertIfAbsent(_ context.Context, p Plan) (bool, error) {
	if t.s.failAfter >= 0 && t.s.inserts >= t.s.failAfter {
		return false, errors.New("insert failed")
	}
	t.s.inserts++
	if _, ok := t.s.rows[p.Key]; ok {
		return false, nil
	}
	t.s.rows[p.Key] = p
	t.s.order = append(t.s.order, p.Key)
	return true, nil
}

func (t *fakeTx) Get(_ context.Context, key string) (Plan, bool, error) {
	p, ok := t.s.rows[key]
	return p, ok, nil
}

func (t *fakeTx) Upsert(_ context.Context, p Plan) error {
	if _, ok := t.s.rows[p.Key]; !ok {
		t.s.order = append(t.s.order, p.Key)
	}
	t.s.rows[p.Key] = p
	return nil
}

func (t *fakeTx) Delete(_ context.Context, key string) (bool, error) {
	_, ok := t.s.rows[key]
	delete(t.s.rows, key)
	return ok, nil
}

func (t *fakeTx) CountSubscriptions(_ context.Context, key string) (int64, error) {
	return t.s.subs[key], nil
}

func (t *fakeTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	t.s.audits = append(t.s.audits, e)
	return nil
}

func discard() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestSeedLoadsTheCatalogueIntoAnEmptyTable(t *testing.T) {
	s := newFakeStore()
	n, err := Seed(context.Background(), s, discard())
	if err != nil {
		t.Fatal(err)
	}
	want, _ := Catalogue()
	if n != len(want) || len(s.rows) != len(want) {
		t.Fatalf("seeded %d, table has %d, want %d", n, len(s.rows), len(want))
	}
	for _, p := range want {
		if _, ok := s.rows[p.Key]; !ok {
			t.Errorf("plan %s was not seeded", p.Key)
		}
	}
}

// A table that has rows is staff's now: the catalogue is never merged into it.
func TestSeedDoesNothingOnANonEmptyTable(t *testing.T) {
	s := newFakeStore()
	edited := Plan{Key: "free", Name: "Free (edited)", Currency: "vnd"}
	s.rows["free"], s.order = edited, []string{"free"}

	n, err := Seed(context.Background(), s, discard())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || len(s.rows) != 1 || s.rows["free"].Name != "Free (edited)" || s.inserts != 0 {
		t.Fatalf("seeded %d, rows %d, inserts %d; want a no-op that keeps the staff edit", n, len(s.rows), s.inserts)
	}
}

// A plan that staff deleted stays deleted once the table has any other row.
func TestSeedDoesNotRestoreADeletedPlan(t *testing.T) {
	s := newFakeStore()
	if _, err := Seed(context.Background(), s, discard()); err != nil {
		t.Fatal(err)
	}
	delete(s.rows, "enterprise")
	if n, err := Seed(context.Background(), s, discard()); err != nil || n != 0 {
		t.Fatalf("second seed = %d, %v", n, err)
	}
	if _, ok := s.rows["enterprise"]; ok {
		t.Fatal("the deleted plan came back")
	}
}

// The seed is one transaction: a failure part-way leaves the table empty, so the
// next start tries again instead of finding half a catalogue.
func TestSeedIsAllOrNothing(t *testing.T) {
	s := newFakeStore()
	s.failAfter = 2
	if _, err := Seed(context.Background(), s, discard()); err == nil {
		t.Fatal("no error from a failing insert")
	}
	if len(s.rows) != 0 {
		t.Fatalf("%d plans were left behind", len(s.rows))
	}
}
