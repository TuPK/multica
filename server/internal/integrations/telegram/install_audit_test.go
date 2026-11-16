package telegram

import (
	"context"
	"errors"
	"testing"
)

// The install writes integration.connected in its own transaction: one audit
// call for the installation it saved, made before the commit.
func TestPersistInstallAuditsTheConnectInTheInstallTransaction(t *testing.T) {
	q := &fakeTelegramInstallQueries{rowID: telegramTestUUID(9)}
	tx := &fakeTelegramTx{}
	svc, err := newInstallService(q, fakeTelegramTxStarter{tx: tx}, telegramInstallTestBox(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	inst, err := svc.persistInstall(context.Background(), installPersist{
		wsID: telegramTestUUID(1), agentID: telegramTestUUID(2), installerID: telegramTestUUID(3), appIDKey: "app-1", configJSON: []byte(`{"app_id":"app-1"}`),
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(q.connected) != 1 || q.connected[0].ID != inst.ID || q.connected[0].InstallerUserID != telegramTestUUID(3) {
		t.Fatalf("audited installations = %+v, want the one saved", q.connected)
	}
	if !tx.committed {
		t.Fatal("the install did not commit")
	}
}

// A failed audit write fails the install and the transaction does not commit,
// so no installation is saved without its row.
func TestPersistInstallRollsBackWhenTheAuditWriteFails(t *testing.T) {
	q := &fakeTelegramInstallQueries{rowID: telegramTestUUID(9), auditErr: errors.New("audit insert failed")}
	tx := &fakeTelegramTx{}
	svc, err := newInstallService(q, fakeTelegramTxStarter{tx: tx}, telegramInstallTestBox(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.persistInstall(context.Background(), installPersist{
		wsID: telegramTestUUID(1), agentID: telegramTestUUID(2), installerID: telegramTestUUID(3), appIDKey: "app-1", configJSON: []byte(`{"app_id":"app-1"}`),
	})
	if err == nil {
		t.Fatal("persistInstall returned no error for a failed audit write")
	}
	if tx.committed {
		t.Fatal("the install committed although its audit write failed")
	}
}
