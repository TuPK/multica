package console

import (
	"errors"
	"testing"
)

func mfaResetPath(id string) string { return "/api/platform/users/" + id + "/mfa-reset" }

func TestMFAResetDeletesTheRowAuditsAndMailsOnce(t *testing.T) {
	e := newEnv(t)
	rec := e.do(suppID, "POST", mfaResetPath(custID), "")
	wantStatus(t, rec, 200)
	if got := decode(t, rec)["reset"]; got != true {
		t.Fatalf("response = %v", decode(t, rec))
	}
	if e.store.mfaRows[custID] {
		t.Fatal("the user_mfa row survived the reset")
	}

	a := onlyAudit(t, e)
	wantStaffActor(t, a, suppID, suppMail)
	if a.Action != "user.mfa_reset" {
		t.Fatalf("action = %q", a.Action)
	}
	if a.TargetType != "user" || a.TargetID != custID || a.TargetLabel != custMail {
		t.Fatalf("target = %q %q %q", a.TargetType, a.TargetID, a.TargetLabel)
	}
	if len(e.mail.sent) != 1 || e.mail.sent[0] != custMail {
		t.Fatalf("mails = %v, want one to %s", e.mail.sent, custMail)
	}
}

func TestMFAResetDoesNotSignTheUserOut(t *testing.T) {
	e := newEnv(t)
	wantStatus(t, e.do(adminID, "POST", mfaResetPath(custID), ""), 200)
	if _, ok := e.store.sessions[custID]; ok || len(e.pub.sent) != 0 || len(e.cache.dropped) != 0 {
		t.Fatalf("the reset touched the user's sessions: %v %v %v", e.store.sessions, e.pub.sent, e.cache.dropped)
	}
}

func TestMFAResetAsSupportAndAdminOnly(t *testing.T) {
	for _, user := range []string{suppID, adminID} {
		e := newEnv(t)
		wantStatus(t, e.do(user, "POST", mfaResetPath(custID), ""), 200)
	}
	e := newEnv(t)
	wantUnknownRoute(t, e.do(billID, "POST", mfaResetPath(custID), ""))
	if !e.store.mfaRows[custID] || len(e.store.audits) != 0 || len(e.mail.sent) != 0 || e.store.txs != 0 {
		t.Fatal("a refused reset changed something")
	}
}

func TestMFAResetOfTheStaffMembersOwnSecondFactorIsRefused(t *testing.T) {
	e := newEnv(t)
	rec := e.do(suppID, "POST", mfaResetPath(suppID), "")
	wantCode(t, rec, 403, "self_reset_refused")
	if !e.store.mfaRows[suppID] || len(e.store.audits) != 0 || len(e.mail.sent) != 0 || e.store.txs != 0 {
		t.Fatal("a refused self reset changed something")
	}
}

func TestMFAResetOfAUserWithoutOneChangesNothing(t *testing.T) {
	e := newEnv(t)
	delete(e.store.mfaRows, custID)
	rec := e.do(suppID, "POST", mfaResetPath(custID), "")
	wantStatus(t, rec, 200)
	if got := decode(t, rec)["reset"]; got != false {
		t.Fatalf("response = %v", decode(t, rec))
	}
	if len(e.store.audits) != 0 || len(e.mail.sent) != 0 {
		t.Fatalf("nothing was reset, but %d audit rows and %d mails were written", len(e.store.audits), len(e.mail.sent))
	}
}

func TestMFAResetOfAnUnknownUserIsNotFound(t *testing.T) {
	e := newEnv(t)
	wantCode(t, e.do(adminID, "POST", mfaResetPath(noOneID), ""), 404, "not_found")
	wantCode(t, e.do(adminID, "POST", mfaResetPath("not-a-uuid"), ""), 404, "not_found")
}

func TestMFAResetRollsBackWhenTheAuditRowFails(t *testing.T) {
	e := newEnv(t)
	e.store.failAudit = errors.New("audit down")
	wantStatus(t, e.do(suppID, "POST", mfaResetPath(custID), ""), 500)
	if !e.store.mfaRows[custID] || len(e.mail.sent) != 0 {
		t.Fatal("the row was deleted or a mail was sent although the audit write failed")
	}
}

func TestMFAResetSucceedsWhenTheMailFails(t *testing.T) {
	e := newEnv(t)
	e.mail.err = errors.New("smtp down")
	wantStatus(t, e.do(suppID, "POST", mfaResetPath(custID), ""), 200)
	if e.store.mfaRows[custID] || len(e.store.audits) != 1 {
		t.Fatal("the reset was not kept after a mail failure")
	}
	if e.logs.Len() == 0 {
		t.Error("the mail failure was not logged")
	}
}

func TestMFAResetWithoutAMailerStillResets(t *testing.T) {
	e := newEnvMail(t, false)
	wantStatus(t, e.do(suppID, "POST", mfaResetPath(custID), ""), 200)
	if e.store.mfaRows[custID] || len(e.store.audits) != 1 {
		t.Fatal("the reset did not happen without a mailer")
	}
}

func TestMFAResetNeedsTheConsoleOrigin(t *testing.T) {
	e := newEnv(t)
	rec := e.doWith(suppID, "POST", mfaResetPath(custID), "", "https://evil.example.com")
	wantCode(t, rec, 403, "bad_origin")
	if !e.store.mfaRows[custID] {
		t.Fatal("a request from a foreign origin deleted the row")
	}
}
