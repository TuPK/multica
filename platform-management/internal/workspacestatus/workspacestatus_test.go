package workspacestatus

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/multica-ai/multica/platform-management/audit"
	"github.com/multica-ai/multica/platform-management/workspacestate"
)

const (
	wsID    = "11223344-5566-7788-99aa-bbccddeeff00"
	staffID = "aabbccdd-eeff-4011-8233-445566778899"
)

// fakeTx records what one transaction did, in order.
type fakeTx struct {
	log      *[]string
	cur      workspacestate.Record
	found    bool
	curErr   error
	writeErr error
	noWrite  bool // Write reports no row written
	auditErr error
	written  []Change
	audits   []audit.Entry
}

func (t *fakeTx) Current(_ context.Context, id string) (workspacestate.Record, bool, error) {
	*t.log = append(*t.log, "current "+id)
	return t.cur, t.found, t.curErr
}

func (t *fakeTx) Write(_ context.Context, c Change) (bool, error) {
	*t.log = append(*t.log, "write")
	if t.writeErr != nil {
		return false, t.writeErr
	}
	t.written = append(t.written, c)
	return !t.noWrite, nil
}

func (t *fakeTx) InsertAuditEvent(_ context.Context, e audit.Entry) error {
	*t.log = append(*t.log, "audit")
	if t.auditErr != nil {
		return t.auditErr
	}
	t.audits = append(t.audits, e)
	return nil
}

// fakeRunner stands for the database: it runs fn once and reports commit or
// rollback in the log.
type fakeRunner struct {
	tx    *fakeTx
	log   *[]string
	begun int
}

func (r *fakeRunner) InTx(_ context.Context, fn func(Tx) error) error {
	r.begun++
	*r.log = append(*r.log, "begin")
	if err := fn(r.tx); err != nil {
		*r.log = append(*r.log, "rollback")
		return err
	}
	*r.log = append(*r.log, "commit")
	return nil
}

type fakeCache struct {
	log    *[]string
	delErr error
	dels   []string
}

func (c *fakeCache) Get(context.Context, string) ([]byte, bool, error) { return nil, false, nil }
func (c *fakeCache) Set(context.Context, string, []byte, time.Duration) error {
	return nil
}
func (c *fakeCache) Del(_ context.Context, key string) error {
	*c.log = append(*c.log, "del "+key)
	c.dels = append(c.dels, key)
	return c.delErr
}

type fixture struct {
	svc    *Service
	runner *fakeRunner
	tx     *fakeTx
	cache  *fakeCache
	log    *[]string
}

func newFixture() *fixture {
	log := &[]string{}
	tx := &fakeTx{log: log}
	runner := &fakeRunner{tx: tx, log: log}
	cache := &fakeCache{log: log}
	// A real Guard over the fake cache, so the key is the one the reader uses.
	guard := workspacestate.New(nil, cache)
	return &fixture{
		svc:    New(runner, guard, slog.New(slog.NewTextHandler(io.Discard, nil))),
		runner: runner, tx: tx, cache: cache, log: log,
	}
}

func change(status workspacestate.Status) Change {
	return Change{WorkspaceID: wsID, Status: status, Reason: "chargeback", ChangedBy: staffID}
}

func TestConstants(t *testing.T) {
	if AuditAction != "workspace.status_changed" {
		t.Errorf("AuditAction = %q", AuditAction)
	}
	if got := workspacestate.Key(wsID); got != "mul:plat:ws:"+wsID {
		t.Errorf("cache key = %q", got)
	}
}

func TestStatusSetRejectsAnUnknownStatus(t *testing.T) {
	for _, s := range []workspacestate.Status{"", "deleted", "ACTIVE", "readonly", "archived"} {
		if ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = true", s)
		}
		f := newFixture()
		changed, err := f.svc.Set(context.Background(), change(s))
		if err == nil || changed {
			t.Errorf("Set(%q) = (%v, %v), want an error", s, changed, err)
		}
		if f.runner.begun != 0 || len(*f.log) != 0 {
			t.Errorf("Set(%q) touched the store: %v", s, *f.log)
		}
	}
	for _, s := range []workspacestate.Status{workspacestate.Active, workspacestate.ReadOnly, workspacestate.Suspended, workspacestate.PendingDeletion} {
		if !ValidStatus(s) {
			t.Errorf("ValidStatus(%q) = false", s)
		}
	}
}

func TestSetRejectsBadInput(t *testing.T) {
	scheduled := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	cases := map[string]Change{
		"no workspace":              {Status: workspacestate.Suspended, ChangedBy: staffID},
		"workspace not a uuid":      {WorkspaceID: "not-a-uuid", Status: workspacestate.Suspended, ChangedBy: staffID},
		"no actor":                  {WorkspaceID: wsID, Status: workspacestate.Suspended},
		"actor not a uuid":          {WorkspaceID: wsID, Status: workspacestate.Suspended, ChangedBy: "staff"},
		"schedule without deletion": {WorkspaceID: wsID, Status: workspacestate.Suspended, ChangedBy: staffID, DeletionScheduledAt: scheduled},
	}
	for name, c := range cases {
		f := newFixture()
		if _, err := f.svc.Set(context.Background(), c); err == nil {
			t.Errorf("%s: no error", name)
		}
		if f.runner.begun != 0 {
			t.Errorf("%s: the store was touched", name)
		}
	}
}

func TestSetWritesRowAndAuditInOneTransactionThenClearsTheCache(t *testing.T) {
	f := newFixture()
	scheduled := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	c := change(workspacestate.PendingDeletion)
	c.DeletionScheduledAt = scheduled
	changed, err := f.svc.Set(context.Background(), c)
	if err != nil || !changed {
		t.Fatalf("Set = (%v, %v)", changed, err)
	}
	want := []string{"begin", "current " + wsID, "write", "audit", "commit", "del mul:plat:ws:" + wsID}
	if strings.Join(*f.log, "|") != strings.Join(want, "|") {
		t.Fatalf("order = %v\nwant    %v", *f.log, want)
	}
	if f.runner.begun != 1 {
		t.Fatalf("transactions = %d, want 1", f.runner.begun)
	}
	if len(f.tx.written) != 1 || f.tx.written[0] != c {
		t.Fatalf("written = %+v", f.tx.written)
	}
	if len(f.tx.audits) != 1 {
		t.Fatalf("audit rows = %d", len(f.tx.audits))
	}
	e := f.tx.audits[0]
	if e.ActorType != audit.ActorStaff || e.ActorID != staffID || e.WorkspaceID != wsID ||
		e.Action != AuditAction || e.TargetType != "workspace" || e.TargetID != wsID {
		t.Errorf("audit entry = %+v", e)
	}
	if e.Detail["from"] != "active" || e.Detail["to"] != "pending_deletion" || e.Detail["reason"] != "chargeback" {
		t.Errorf("audit detail = %+v", e.Detail)
	}
	if e.Detail["deletion_scheduled_at"] != "2026-03-01T12:00:00Z" {
		t.Errorf("audit detail has deletion_scheduled_at %v", e.Detail["deletion_scheduled_at"])
	}
}

func TestSetRecordsThePreviousStatus(t *testing.T) {
	f := newFixture()
	f.tx.cur, f.tx.found = workspacestate.Record{Status: workspacestate.ReadOnly}, true
	if changed, err := f.svc.Set(context.Background(), change(workspacestate.Active)); err != nil || !changed {
		t.Fatalf("Set = (%v, %v)", changed, err)
	}
	if got := f.tx.audits[0].Detail["from"]; got != "read_only" {
		t.Errorf("from = %v, want read_only", got)
	}
}

func TestSetUnchangedStatusWritesNothing(t *testing.T) {
	cases := map[string]struct {
		cur   workspacestate.Record
		found bool
		set   workspacestate.Status
	}{
		"same status":            {workspacestate.Record{Status: workspacestate.Suspended}, true, workspacestate.Suspended},
		"no row, set active":     {workspacestate.Record{}, false, workspacestate.Active},
		"active row, set active": {workspacestate.Record{Status: workspacestate.Active}, true, workspacestate.Active},
	}
	for name, c := range cases {
		f := newFixture()
		f.tx.cur, f.tx.found = c.cur, c.found
		changed, err := f.svc.Set(context.Background(), change(c.set))
		if err != nil || changed {
			t.Errorf("%s: Set = (%v, %v)", name, changed, err)
		}
		if len(f.tx.written) != 0 || len(f.tx.audits) != 0 || len(f.cache.dels) != 0 {
			t.Errorf("%s: wrote %v, audited %v, cleared %v", name, f.tx.written, f.tx.audits, f.cache.dels)
		}
	}
}

func TestSetWriteThatChangesNoRowIsUnchanged(t *testing.T) {
	// The upsert's WHERE found the status already set: a concurrent writer won.
	f := newFixture()
	f.tx.noWrite = true
	changed, err := f.svc.Set(context.Background(), change(workspacestate.Suspended))
	if err != nil || changed {
		t.Fatalf("Set = (%v, %v)", changed, err)
	}
	if len(f.tx.audits) != 0 || len(f.cache.dels) != 0 {
		t.Fatalf("audited %v, cleared %v", f.tx.audits, f.cache.dels)
	}
}

func TestSetAuditFailureRollsBackAndKeepsTheCache(t *testing.T) {
	f := newFixture()
	boom := errors.New("audit insert failed")
	f.tx.auditErr = boom
	changed, err := f.svc.Set(context.Background(), change(workspacestate.Suspended))
	if !errors.Is(err, boom) || changed {
		t.Fatalf("Set = (%v, %v)", changed, err)
	}
	if got := strings.Join(*f.log, "|"); !strings.HasSuffix(got, "rollback") || len(f.cache.dels) != 0 {
		t.Fatalf("log = %v, cleared %v", *f.log, f.cache.dels)
	}
}

func TestSetWriteAndReadErrorsRollBack(t *testing.T) {
	for name, set := range map[string]func(*fakeTx, error){
		"read":  func(tx *fakeTx, e error) { tx.curErr = e },
		"write": func(tx *fakeTx, e error) { tx.writeErr = e },
	} {
		f := newFixture()
		boom := errors.New(name + " failed")
		set(f.tx, boom)
		if _, err := f.svc.Set(context.Background(), change(workspacestate.Suspended)); !errors.Is(err, boom) {
			t.Errorf("%s: err = %v", name, err)
		}
		if len(f.cache.dels) != 0 || len(f.tx.audits) != 0 {
			t.Errorf("%s: audited %v, cleared %v", name, f.tx.audits, f.cache.dels)
		}
	}
}

func TestSetCacheDeletionFailureIsLoggedNotReturned(t *testing.T) {
	f := newFixture()
	f.cache.delErr = errors.New("redis down")
	changed, err := f.svc.Set(context.Background(), change(workspacestate.Suspended))
	if err != nil || !changed {
		t.Fatalf("Set = (%v, %v), want the write to stand", changed, err)
	}
}

func TestSetCanonicalisesIDsSoTheRightKeyIsCleared(t *testing.T) {
	f := newFixture()
	c := change(workspacestate.Suspended)
	c.WorkspaceID = strings.ToUpper(wsID)
	c.ChangedBy = strings.ToUpper(staffID)
	if _, err := f.svc.Set(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	if len(f.cache.dels) != 1 || f.cache.dels[0] != "mul:plat:ws:"+wsID {
		t.Fatalf("cleared %v, want the lower-case key", f.cache.dels)
	}
	if f.tx.audits[0].WorkspaceID != wsID || f.tx.audits[0].ActorID != staffID {
		t.Fatalf("audit ids not canonical: %+v", f.tx.audits[0])
	}
}
