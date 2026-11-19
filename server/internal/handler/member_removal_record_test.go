package handler

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/multica-ai/multica/server/internal/testutil"
	"github.com/multica-ai/multica/server/internal/util"
)

// member_removal feeds the usage snapshot's count of members removed during the
// day. These tests prove the write path: the row is written by
// revokeAndRemoveMember in the same transaction that deletes the member.

func memberRemovalFixture(t *testing.T) (ws, user, memberID string) {
	t.Helper()
	requirePlanLimitDB(t)
	user = dbfx.User(t, "Removed", testutil.UniqueEmail("member-removal"))
	ws = dbfx.Workspace(t, "Member removal", "member-removal-"+user[:8])
	memberID = dbfx.Member(t, ws, user, "member")
	dbfx.Cleanup(t, `DELETE FROM member_removal WHERE workspace_id = $1`, ws)
	return ws, user, memberID
}

func memberRowCount(t *testing.T, ws, user string) int {
	t.Helper()
	var n int
	if err := testPool.QueryRow(context.Background(),
		`SELECT count(*) FROM member WHERE workspace_id = $1 AND user_id = $2`, ws, user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestRevokeAndRemoveMemberRecordsRemovalInTheSameTransaction(t *testing.T) {
	ws, user, memberID := memberRemovalFixture(t)
	ctx := context.Background()

	before := time.Now().Add(-time.Second)
	if _, err := testHandler.revokeAndRemoveMember(ctx,
		util.MustParseUUID(ws), util.MustParseUUID(user), util.MustParseUUID(memberID),
		util.MustParseUUID(testUserID), nil,
	); err != nil {
		t.Fatalf("revokeAndRemoveMember: %v", err)
	}
	after := time.Now().Add(time.Second)

	var n int
	var removedAt time.Time
	if err := testPool.QueryRow(ctx,
		`SELECT count(*), coalesce(max(removed_at), 'epoch') FROM member_removal WHERE workspace_id = $1 AND user_id = $2`,
		ws, user).Scan(&n, &removedAt); err != nil {
		t.Fatal(err)
	}
	if n != 1 {
		t.Fatalf("member_removal rows = %d, want 1", n)
	}
	if removedAt.Before(before) || removedAt.After(after) {
		t.Errorf("removed_at = %v, want between %v and %v", removedAt, before, after)
	}
	if got := memberRowCount(t, ws, user); got != 0 {
		t.Errorf("member rows = %d, want 0", got)
	}
}

func TestRevokeAndRemoveMemberRollsBackRemovalWhenCommitFails(t *testing.T) {
	ws, user, memberID := memberRemovalFixture(t)
	ctx := context.Background()

	boom := errors.New("audit write failed")
	_, err := testHandler.revokeAndRemoveMember(ctx,
		util.MustParseUUID(ws), util.MustParseUUID(user), util.MustParseUUID(memberID),
		util.MustParseUUID(testUserID), func(pgx.Tx) error { return boom },
	)
	if !errors.Is(err, boom) {
		t.Fatalf("err = %v, want the beforeCommit error", err)
	}

	if got := memberRowCount(t, ws, user); got != 1 {
		t.Errorf("member rows = %d, want 1 (rolled back)", got)
	}
	var n int
	if err := testPool.QueryRow(ctx,
		`SELECT count(*) FROM member_removal WHERE workspace_id = $1 AND user_id = $2`, ws, user).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Errorf("member_removal rows = %d, want 0 (rolled back)", n)
	}
}
