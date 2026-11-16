package plan

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"maps"
	"reflect"
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

// Deleted reports whether a plan.changed row with deleted=true names key.
func (t *fakeTx) Deleted(_ context.Context, key string) (bool, error) {
	for _, e := range t.s.audits {
		if e.Action == audit.ActionPlanChanged && e.TargetType == "plan" && e.TargetID == key && e.Detail["deleted"] == true {
			return true, nil
		}
	}
	return false, nil
}

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

// catalogueRow returns the embedded catalogue's plan with key.
func catalogueRow(t *testing.T, key string) Plan {
	t.Helper()
	cat, err := Catalogue()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range cat {
		if p.Key == key {
			return p
		}
	}
	t.Fatalf("catalogue has no plan %q", key)
	return Plan{}
}

// A table holding only free gains the paid plans: the catalogue fills what is missing.
func TestSeedAddsTheMissingPlansToATableWithOnlyFree(t *testing.T) {
	s := newFakeStore()
	s.rows["free"], s.order = catalogueRow(t, "free"), []string{"free"}

	n, err := Seed(context.Background(), s, discard())
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 {
		t.Fatalf("seeded %d plans, want 4", n)
	}
	for _, key := range []string{"free", "pro", "business", "enterprise", "unlimited"} {
		if _, ok := s.rows[key]; !ok {
			t.Errorf("plan %s is missing after Seed", key)
		}
	}
}

// A second Seed over a complete catalogue changes nothing.
func TestSeedSecondRunChangesNothing(t *testing.T) {
	s := newFakeStore()
	if _, err := Seed(context.Background(), s, discard()); err != nil {
		t.Fatal(err)
	}
	before := maps.Clone(s.rows)
	n, err := Seed(context.Background(), s, discard())
	if err != nil {
		t.Fatal(err)
	}
	if n != 0 || !reflect.DeepEqual(before, s.rows) {
		t.Fatalf("second seed inserted %d and changed rows: %v", n, s.rows)
	}
}

// A row staff has edited keeps its values; the missing plans are still added.
func TestSeedKeepsAnEditedRow(t *testing.T) {
	s := newFakeStore()
	edited := catalogueRow(t, "pro")
	edited.Name = "Pro (edited)"
	s.rows["pro"], s.order = edited, []string{"pro"}

	if _, err := Seed(context.Background(), s, discard()); err != nil {
		t.Fatal(err)
	}
	if got := s.rows["pro"]; got.Name != "Pro (edited)" {
		t.Fatalf("pro name = %q, want the staff edit kept", got.Name)
	}
	for _, key := range []string{"free", "business", "enterprise"} {
		if _, ok := s.rows[key]; !ok {
			t.Errorf("plan %s is missing after Seed", key)
		}
	}
}

// A plan that staff deleted stays deleted. The console writes a plan.changed row
// with deleted=true when it deletes a plan, and the seed reads that row.
func TestSeedDoesNotRestoreADeletedPlan(t *testing.T) {
	s := newFakeStore()
	if _, err := Seed(context.Background(), s, discard()); err != nil {
		t.Fatal(err)
	}
	delete(s.rows, "enterprise")
	s.audits = append(s.audits, audit.Entry{
		Action: audit.ActionPlanChanged, TargetType: "plan", TargetID: "enterprise",
		Detail: map[string]any{"deleted": true},
	})
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
