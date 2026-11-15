package engine

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/multica-ai/multica/server/internal/entitlement"
	"github.com/multica-ai/multica/server/internal/entitlement/entitlementtest"
	"github.com/multica-ai/multica/server/internal/integrations/channel"
	db "github.com/multica-ai/multica/server/pkg/db/generated"
)

func (f *fakeSessionQueries) LockPlanGate(_ context.Context, arg db.LockPlanGateParams) error {
	f.quotaEvents = append(f.quotaEvents, "lock:"+arg.Gate)
	return nil
}

func (f *fakeSessionQueries) SumPlanLimitAttachmentBytes(context.Context, pgtype.UUID) (int64, error) {
	f.quotaEvents = append(f.quotaEvents, "sum")
	return f.quotaUsed, nil
}

// storageLimit makes the workspace's storage_bytes gate enforce limit.
func storageLimit(t *testing.T, workspace pgtype.UUID, limit int) {
	t.Helper()
	stub := entitlementtest.New()
	stub.Set(uuid.UUID(workspace.Bytes), entitlement.GateStorageBytes, entitlement.Decision{
		Gate: entitlement.Gate{Action: entitlement.ActionEnforce, Limit: &limit},
	})
	SetEntitlements(stub)
	t.Cleanup(func() { SetEntitlements(nil) })
}

func mediaRef(key string, size int64) channel.MediaRef {
	return channel.MediaRef{
		Type: channel.MsgTypeImage, StorageKey: key, StorageURL: "https://cdn.example.test/" + key,
		Filename: key + ".png", MimeType: "image/png", SizeBytes: size,
	}
}

func bindRefs(t *testing.T, f *fakeSessionQueries, refs ...channel.MediaRef) (BindMediaResult, error) {
	t.Helper()
	return newTestSession(f).BindMediaRefsWithResult(context.Background(), BindMediaInput{
		MessageID: uid(42), SessionID: uid(1), WorkspaceID: uid(9), Sender: uid(7), Body: "hello", MediaRefs: refs,
	})
}

// An attachment the plan has no room for is skipped and recorded. It does not
// fail the bind: the message already exists, and the other media still attach.
func TestBindMediaRefsSkipsAnAttachmentThePlanHasNoRoomFor(t *testing.T) {
	storageLimit(t, uid(9), 10)
	f := newFake()
	f.quotaUsed = 2

	res, err := bindRefs(t, f, mediaRef("fits", 4), mediaRef("too-big", 9), mediaRef("fits-too", 3))
	if err != nil {
		t.Fatalf("a refused attachment must not fail the bind: %v", err)
	}

	if len(f.attachments) != 2 || f.attachments[0].SizeBytes != 4 || f.attachments[1].SizeBytes != 3 {
		t.Fatalf("attachments = %+v, want the two that fit", f.attachments)
	}
	if len(res.Refused) != 1 {
		t.Fatalf("Refused = %+v, want one entry", res.Refused)
	}
	got := res.Refused[0]
	if got.StorageKey != "too-big" || got.Gate != entitlement.GateStorageBytes || got.Limit != 10 || got.Used != 6 {
		t.Errorf("refusal = %+v, want too-big, storage_bytes, used 6 of 10", got)
	}
	// The message's pending marker is still cleared, so the placeholder is not left waiting.
	if f.mediaCleared != 1 {
		t.Errorf("media pending cleared %d times, want 1", f.mediaCleared)
	}
}

// The refused object's ledger entry is left alone, so the reconciler deletes
// the object. Claiming it would orphan it in storage.
func TestBindMediaRefsLeavesTheRefusedObjectsIntentForTheReconciler(t *testing.T) {
	storageLimit(t, uid(9), 5)
	f := newFake()

	if _, err := bindRefs(t, f, mediaRef("fits", 4), mediaRef("refused", 4)); err != nil {
		t.Fatal(err)
	}
	if want := [][]string{{"fits"}}; !reflect.DeepEqual(f.claimRequests, want) {
		t.Fatalf("claimed keys = %v, want only %v", f.claimRequests, want)
	}
}

func TestBindMediaRefsWhenNothingFitsCreatesNothingAndSucceeds(t *testing.T) {
	storageLimit(t, uid(9), 1)
	f := newFake()

	res, err := bindRefs(t, f, mediaRef("a", 4), mediaRef("b", 4))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if len(f.attachments) != 0 || f.linked.ChatMessageID.Valid {
		t.Fatalf("attachments = %+v, linked = %+v, want none", f.attachments, f.linked)
	}
	if len(res.Refused) != 2 {
		t.Fatalf("Refused = %+v, want both", res.Refused)
	}
}

// The advisory lock is taken before the byte count, in the bind's transaction.
func TestBindMediaRefsTakesTheQuotaLockBeforeCounting(t *testing.T) {
	storageLimit(t, uid(9), 100)
	f := newFake()

	if _, err := bindRefs(t, f, mediaRef("a", 4)); err != nil {
		t.Fatal(err)
	}
	if want := []string{"lock:storage_bytes", "sum"}; !reflect.DeepEqual(f.quotaEvents, want) {
		t.Fatalf("events = %v, want %v", f.quotaEvents, want)
	}
}

// With no gate (no cloud URL, or a plan without the quota) the bind runs no
// quota statement at all.
func TestBindMediaRefsWithoutAGateRunsNoQuotaStatement(t *testing.T) {
	SetEntitlements(nil)
	f := newFake()

	res, err := bindRefs(t, f, mediaRef("a", 1<<40))
	if err != nil || len(res.Refused) != 0 || len(f.attachments) != 1 {
		t.Fatalf("res = %+v, err = %v, attachments = %d", res, err, len(f.attachments))
	}
	if len(f.quotaEvents) != 0 {
		t.Fatalf("quota events = %v, want none", f.quotaEvents)
	}
}
