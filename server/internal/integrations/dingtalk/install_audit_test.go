package dingtalk

import (
	"context"
	"errors"
	"testing"
)

// The install writes integration.connected in its own transaction: one audit
// call for the installation it saved, made before the commit.
func TestPersistInstallAuditsTheConnectInTheInstallTransaction(t *testing.T) {
	q := &fakeInstallQueries{rowID: mustUUID(t, "00000000-0000-0000-0000-000000000009")}
	tx := &fakeTx{}
	svc, err := newInstallService(q, &fakeTxStarter{tx: tx}, testBox(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := svc.persistInstall(context.Background(), installPersist{
		wsID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), agentID: mustUUID(t, "00000000-0000-0000-0000-000000000002"), installerID: mustUUID(t, "00000000-0000-0000-0000-000000000003"), appIDKey: "app-1", configJSON: []byte(`{"app_id":"app-1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.connected) != 1 || q.connected[0].ID != inst.ID || q.connected[0].InstallerUserID != mustUUID(t, "00000000-0000-0000-0000-000000000003") {
		t.Fatalf("audited installations = %+v, want the one saved", q.connected)
	}
	if !tx.committed {
		t.Fatal("the install did not commit")
	}
}

// A failed audit write fails the install and the transaction does not commit,
// so no installation is saved without its row.
func TestPersistInstallRollsBackWhenTheAuditWriteFails(t *testing.T) {
	q := &fakeInstallQueries{rowID: mustUUID(t, "00000000-0000-0000-0000-000000000009"), auditErr: errors.New("audit insert failed")}
	tx := &fakeTx{}
	svc, err := newInstallService(q, &fakeTxStarter{tx: tx}, testBox(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.persistInstall(context.Background(), installPersist{
		wsID: mustUUID(t, "00000000-0000-0000-0000-000000000001"), agentID: mustUUID(t, "00000000-0000-0000-0000-000000000002"), installerID: mustUUID(t, "00000000-0000-0000-0000-000000000003"), appIDKey: "app-1", configJSON: []byte(`{"app_id":"app-1"}`),
	})
	if err == nil {
		t.Fatal("persistInstall returned no error for a failed audit write")
	}
	if tx.committed {
		t.Fatal("the install committed although its audit write failed")
	}
}
