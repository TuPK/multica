package workspaceguard

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/go-redis/redismock/v9"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/platform-management/workspacestate"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

// stateDB answers the WorkspaceState query with a fixed row, no row, or an error.
type stateDB struct {
	db.DBTX
	status string
	at     pgtype.Timestamptz
	err    error
	args   []any
}

func (d *stateDB) QueryRow(_ context.Context, _ string, args ...any) pgx.Row {
	d.args = args
	return stateRow{d: d}
}

type stateRow struct {
	pgx.Row
	d *stateDB
}

func (r stateRow) Scan(dest ...any) error {
	if r.d.err != nil {
		return r.d.err
	}
	status, ok := dest[0].(*string)
	at, ok2 := dest[1].(*pgtype.Timestamptz)
	if !ok || !ok2 || len(dest) != 2 {
		return errors.New("unexpected WorkspaceState column layout")
	}
	*status, *at = r.d.status, r.d.at
	return nil
}

func TestStoreReadsTheRow(t *testing.T) {
	when := time.Date(2026, 2, 3, 4, 5, 6, 0, time.UTC)
	d := &stateDB{status: "pending_deletion", at: pgtype.Timestamptz{Time: when, Valid: true}}
	rec, found, err := NewStore(db.New(d)).WorkspaceState(context.Background(), testUUIDString)
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if rec.Status != workspacestate.PendingDeletion || !rec.DeletionScheduledAt.Equal(when) {
		t.Fatalf("rec = %+v", rec)
	}
	if got, _ := d.args[0].(pgtype.UUID); got != testUUID() {
		t.Fatalf("queried %v, want the workspace id", d.args)
	}
}

func TestStoreNoRowIsNotFound(t *testing.T) {
	rec, found, err := NewStore(db.New(&stateDB{err: pgx.ErrNoRows})).WorkspaceState(context.Background(), testUUIDString)
	if err != nil || found || rec != (workspacestate.Record{}) {
		t.Fatalf("rec=%+v found=%v err=%v", rec, found, err)
	}
}

func TestStoreDatabaseErrorIsReturned(t *testing.T) {
	boom := errors.New("connection reset")
	if _, _, err := NewStore(db.New(&stateDB{err: boom})).WorkspaceState(context.Background(), testUUIDString); !errors.Is(err, boom) {
		t.Fatalf("err = %v", err)
	}
}

func TestStoreInvalidIDIsAnError(t *testing.T) {
	d := &stateDB{}
	if _, _, err := NewStore(db.New(d)).WorkspaceState(context.Background(), "not-a-uuid"); err == nil {
		t.Fatal("an invalid id was queried")
	}
	if d.args != nil {
		t.Fatal("the database was queried for an invalid id")
	}
}

func TestRedisCacheUsesTheGivenKeyAndTTL(t *testing.T) {
	rdb, mock := redismock.NewClientMock()
	c := NewRedisCache(rdb)
	ctx := context.Background()
	key := workspacestate.Key(testUUIDString)

	mock.ExpectGet(key).RedisNil()
	if _, ok, err := c.Get(ctx, key); ok || err != nil {
		t.Fatalf("miss: ok=%v err=%v", ok, err)
	}
	mock.ExpectGet(key).SetVal(`{"found":false}`)
	if b, ok, err := c.Get(ctx, key); !ok || err != nil || string(b) != `{"found":false}` {
		t.Fatalf("hit: %q ok=%v err=%v", b, ok, err)
	}
	boom := errors.New("redis down")
	mock.ExpectGet(key).SetErr(boom)
	if _, ok, err := c.Get(ctx, key); ok || !errors.Is(err, boom) {
		t.Fatalf("error: ok=%v err=%v", ok, err)
	}
	mock.ExpectSet(key, []byte("v"), workspacestate.TTL).SetVal("OK")
	if err := c.Set(ctx, key, []byte("v"), workspacestate.TTL); err != nil {
		t.Fatalf("set: %v", err)
	}
	mock.ExpectDel(key).SetVal(1)
	if err := c.Del(ctx, key); err != nil {
		t.Fatalf("del: %v", err)
	}
	if err := mock.ExpectationsWereMet(); err != nil {
		t.Fatal(err)
	}
}

func TestNewWithoutRedisReadsTheStoreEveryTime(t *testing.T) {
	d := &stateDB{status: "suspended"}
	g := New(db.New(d), nil)
	for i := 0; i < 2; i++ {
		rec, err := g.State(context.Background(), testUUIDString)
		if err != nil || rec.Status != workspacestate.Suspended {
			t.Fatalf("rec=%+v err=%v", rec, err)
		}
	}
	if err := g.Invalidate(context.Background(), testUUIDString); err != nil {
		t.Fatalf("invalidate without redis: %v", err)
	}
}
